// Copyright 2007 The RE2 Authors.  All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Compiled regular expression representation.
// Tested by compile_test.cc

#include "re2/prog.h"

#if defined(__AVX2__)
#include <immintrin.h>
#ifdef _MSC_VER
#include <intrin.h>
#endif
#endif
#include <algorithm>
#include <memory>
#include <stdint.h>
#include <string.h>
#include <utility>

#include "re2/bitmap256.h"
#include "re2/stringpiece.h"
#include "util/logging.h"
#include "util/strutil.h"
#include "util/util.h"

namespace re2 {

// Constructors per Inst opcode

void Prog::Inst::InitAlt(uint32_t out, uint32_t out1) {
    DCHECK_EQ(out_opcode_, 0);
    set_out_opcode(out, kInstAlt);
    out1_ = out1;
}

void Prog::Inst::InitByteRange(int lo, int hi, int foldcase, uint32_t out) {
    DCHECK_EQ(out_opcode_, 0);
    set_out_opcode(out, kInstByteRange);
    byte_range.lo_ = lo & 0xFF;
    byte_range.hi_ = hi & 0xFF;
    byte_range.hint_foldcase_ = foldcase & 1;
}

void Prog::Inst::InitCapture(int cap, uint32_t out) {
    DCHECK_EQ(out_opcode_, 0);
    set_out_opcode(out, kInstCapture);
    cap_ = cap;
}

void Prog::Inst::InitEmptyWidth(EmptyOp empty, uint32_t out) {
    DCHECK_EQ(out_opcode_, 0);
    set_out_opcode(out, kInstEmptyWidth);
    empty_ = empty;
}

void Prog::Inst::InitMatch(int32_t id) {
    DCHECK_EQ(out_opcode_, 0);
    set_opcode(kInstMatch);
    match_id_ = id;
}

void Prog::Inst::InitNop(uint32_t out) {
    DCHECK_EQ(out_opcode_, 0);
    set_opcode(kInstNop);
}

void Prog::Inst::InitFail() {
    DCHECK_EQ(out_opcode_, 0);
    set_opcode(kInstFail);
}

std::string Prog::Inst::Dump() {
    switch (opcode()) {
        default:
            return StringPrintf("opcode %d", static_cast<int>(opcode()));

        case kInstAlt:
            return StringPrintf("alt -> %d | %d", out(), out1_);

        case kInstAltMatch:
            return StringPrintf("altmatch -> %d | %d", out(), out1_);

        case kInstByteRange:
            return StringPrintf("byte%s [%02x-%02x] %d -> %d", foldcase() ? "/i" : "", byte_range.lo_, byte_range.hi_, hint(), out());

        case kInstCapture:
            return StringPrintf("capture %d -> %d", cap_, out());

        case kInstEmptyWidth:
            return StringPrintf("emptywidth %#x -> %d", static_cast<int>(empty_), out());

        case kInstMatch:
            return StringPrintf("match! %d", match_id());

        case kInstNop:
            return StringPrintf("nop -> %d", out());

        case kInstFail:
            return StringPrintf("fail");
    }
}

Prog::Prog()
    : anchor_start_(false), anchor_end_(false), reversed_(false), did_flatten_(false), did_onepass_(false), start_(0), start_unanchored_(0), size_(0),
      bytemap_range_(0), prefix_foldcase_(false), prefix_size_(0), list_count_(0), bit_state_text_max_size_(0), dfa_mem_(0), dfa_first_(NULL),
      dfa_longest_(NULL) {}

Prog::~Prog() {
    DeleteDFA(dfa_longest_);
    DeleteDFA(dfa_first_);
    if (prefix_foldcase_)
        delete[] prefix_dfa_;
}

typedef SparseSet Workq;

static inline void AddToQueue(Workq* q, int id) {
  if (id != 0)
    q->insert(id);
}

static std::string ProgToString(Prog* prog, Workq* q) {
  std::string s;
  for (Workq::iterator i = q->begin(); i != q->end(); ++i) {
    int id = *i;
    Prog::Inst* ip = prog->inst(id);
    s += StringPrintf("%d. %s\n", id, ip->Dump().c_str());
    AddToQueue(q, ip->out());
    if (ip->opcode() == kInstAlt || ip->opcode() == kInstAltMatch)
      AddToQueue(q, ip->out1());
  }
  return s;
}

static std::string FlattenedProgToString(Prog* prog, int start) {
  std::string s;
  for (int id = start; id < prog->size(); id++) {
    Prog::Inst* ip = prog->inst(id);
    if (ip->last())
      s += StringPrintf("%d. %s\n", id, ip->Dump().c_str());
    else
      s += StringPrintf("%d+ %s\n", id, ip->Dump().c_str());
  }
  return s;
}

std::string Prog::Dump() {
  if (did_flatten_)
    return FlattenedProgToString(this, start_);

  Workq q(size_);
  AddToQueue(&q, start_);
  return ProgToString(this, &q);
}

std::string Prog::DumpUnanchored() {
  if (did_flatten_)
    return FlattenedProgToString(this, start_unanchored_);

  Workq q(size_);
  AddToQueue(&q, start_unanchored_);
  return ProgToString(this, &q);
}

std::string Prog::DumpByteMap() {
  std::string map;
  for (int c = 0; c < 256; c++) {
    int b = bytemap_[c];
    int lo = c;
    while (c < 256-1 && bytemap_[c+1] == b)
      c++;
    int hi = c;
    map += StringPrintf("[%02x-%02x] -> %d\n", lo, hi, b);
  }
  return map;
}

// Is ip a guaranteed match at end of text, perhaps after some capturing?
static bool IsMatch(Prog* prog, Prog::Inst* ip) {
  for (;;) {
    switch (ip->opcode()) {
      default:
        LOG(DFATAL) << "Unexpected opcode in IsMatch: " << ip->opcode();
        return false;

      case kInstAlt:
      case kInstAltMatch:
      case kInstByteRange:
      case kInstFail:
      case kInstEmptyWidth:
        return false;

      case kInstCapture:
      case kInstNop:
        ip = prog->inst(ip->out());
        break;

      case kInstMatch:
        return true;
    }
  }
}

// Peep-hole optimizer.
void Prog::Optimize() {
  Workq q(size_);

  // Eliminate nops.  Most are taken out during compilation
  // but a few are hard to avoid.
  q.clear();
  AddToQueue(&q, start_);
  for (Workq::iterator i = q.begin(); i != q.end(); ++i) {
    int id = *i;

    Inst* ip = inst(id);
    int j = ip->out();
    Inst* jp;
    while (j != 0 && (jp=inst(j))->opcode() == kInstNop) {
      j = jp->out();
    }
    ip->set_out(j);
    AddToQueue(&q, ip->out());

    if (ip->opcode() == kInstAlt) {
      j = ip->out1();
      while (j != 0 && (jp=inst(j))->opcode() == kInstNop) {
        j = jp->out();
      }
      ip->out1_ = j;
      AddToQueue(&q, ip->out1());
    }
  }

  // Insert kInstAltMatch instructions
  // Look for
  //   ip: Alt -> j | k
  //	  j: ByteRange [00-FF] -> ip
  //    k: Match
  // or the reverse (the above is the greedy one).
  // Rewrite Alt to AltMatch.
  q.clear();
  AddToQueue(&q, start_);
  for (Workq::iterator i = q.begin(); i != q.end(); ++i) {
    int id = *i;
    Inst* ip = inst(id);
    AddToQueue(&q, ip->out());
    if (ip->opcode() == kInstAlt)
      AddToQueue(&q, ip->out1());

    if (ip->opcode() == kInstAlt) {
      Inst* j = inst(ip->out());
      Inst* k = inst(ip->out1());
      if (j->opcode() == kInstByteRange && j->out() == id &&
          j->lo() == 0x00 && j->hi() == 0xFF &&
          IsMatch(this, k)) {
        ip->set_opcode(kInstAltMatch);
        continue;
      }
      if (IsMatch(this, j) &&
          k->opcode() == kInstByteRange && k->out() == id &&
          k->lo() == 0x00 && k->hi() == 0xFF) {
        ip->set_opcode(kInstAltMatch);
      }
    }
  }
}

uint32_t Prog::EmptyFlags(const StringPiece& text, const char* p) {
  int flags = 0;

  // ^ and \A
  if (p == text.data())
    flags |= kEmptyBeginText | kEmptyBeginLine;
  else if (p[-1] == '\n')
    flags |= kEmptyBeginLine;

  // $ and \z
  if (p == text.data() + text.size())
    flags |= kEmptyEndText | kEmptyEndLine;
  else if (p < text.data() + text.size() && p[0] == '\n')
    flags |= kEmptyEndLine;

  // \b and \B
  if (p == text.data() && p == text.data() + text.size()) {
    // no word boundary here
  } else if (p == text.data()) {
    if (IsWordChar(p[0]))
      flags |= kEmptyWordBoundary;
  } else if (p == text.data() + text.size()) {
    if (IsWordChar(p[-1]))
      flags |= kEmptyWordBoundary;
  } else {
    if (IsWordChar(p[-1]) != IsWordChar(p[0]))
      flags |= kEmptyWordBoundary;
  }
  if (!(flags & kEmptyWordBoundary))
    flags |= kEmptyNonWordBoundary;

  return flags;
}

// ByteMapBuilder implements a coloring algorithm.
//
// The first phase is a series of "mark and merge" batches: we mark one or more
// [lo-hi] ranges, then merge them into our internal state. Batching is not for
// performance; rather, it means that the ranges are treated indistinguishably.
//
// Internally, the ranges are represented using a bitmap that stores the splits
// and a vector that stores the colors; both of them are indexed by the ranges'
// last bytes. Thus, in order to merge a [lo-hi] range, we split at lo-1 and at
// hi (if not already split), then recolor each range in between. The color map
// (i.e. from the old color to the new color) is maintained for the lifetime of
// the batch and so underpins this somewhat obscure approach to set operations.
//
// The second phase builds the bytemap from our internal state: we recolor each
// range, then store the new color (which is now the byte class) in each of the
// corresponding array elements. Finally, we output the number of byte classes.
class ByteMapBuilder {
 public:
  ByteMapBuilder() {
    // Initial state: the [0-255] range has color 256.
    // This will avoid problems during the second phase,
    // in which we assign byte classes numbered from 0.
    splits_.Set(255);
    colors_[255] = 256;
    nextcolor_ = 257;
  }

  void Mark(int lo, int hi);
  void Merge();
  void Build(uint8_t* bytemap, int* bytemap_range);

 private:
  int Recolor(int oldcolor);

  Bitmap256 splits_;
  int colors_[256];
  int nextcolor_;
  std::vector<std::pair<int, int>> colormap_;
  std::vector<std::pair<int, int>> ranges_;

  ByteMapBuilder(const ByteMapBuilder&) = delete;
  ByteMapBuilder& operator=(const ByteMapBuilder&) = delete;
};

void ByteMapBuilder::Mark(int lo, int hi) {
  DCHECK_GE(lo, 0);
  DCHECK_GE(hi, 0);
  DCHECK_LE(lo, 255);
  DCHECK_LE(hi, 255);
  DCHECK_LE(lo, hi);

  // Ignore any [0-255] ranges. They cause us to recolor every range, which
  // has no effect on the eventual result and is therefore a waste of time.
  if (lo == 0 && hi == 255)
    return;

  ranges_.emplace_back(lo, hi);
}

void ByteMapBuilder::Merge() {
  for (std::vector<std::pair<int, int>>::const_iterator it = ranges_.begin();
       it != ranges_.end();
       ++it) {
    int lo = it->first-1;
    int hi = it->second;

    if (0 <= lo && !splits_.Test(lo)) {
      splits_.Set(lo);
      int next = splits_.FindNextSetBit(lo+1);
      colors_[lo] = colors_[next];
    }
    if (!splits_.Test(hi)) {
      splits_.Set(hi);
      int next = splits_.FindNextSetBit(hi+1);
      colors_[hi] = colors_[next];
    }

    int c = lo+1;
    while (c < 256) {
      int next = splits_.FindNextSetBit(c);
      colors_[next] = Recolor(colors_[next]);
      if (next == hi)
        break;
      c = next+1;
    }
  }
  colormap_.clear();
  ranges_.clear();
}

void ByteMapBuilder::Build(uint8_t* bytemap, int* bytemap_range) {
  // Assign byte classes numbered from 0.
  nextcolor_ = 0;

  int c = 0;
  while (c < 256) {
    int next = splits_.FindNextSetBit(c);
    uint8_t b = static_cast<uint8_t>(Recolor(colors_[next]));
    while (c <= next) {
      bytemap[c] = b;
      c++;
    }
  }

  *bytemap_range = nextcolor_;
}

int ByteMapBuilder::Recolor(int oldcolor) {
  // Yes, this is a linear search. There can be at most 256
  // colors and there will typically be far fewer than that.
  // Also, we need to consider keys *and* values in order to
  // avoid recoloring a given range more than once per batch.
  std::vector<std::pair<int, int>>::const_iterator it =
      std::find_if(colormap_.begin(), colormap_.end(),
                   [=](const std::pair<int, int>& kv) -> bool {
                     return kv.first == oldcolor || kv.second == oldcolor;
                   });
  if (it != colormap_.end())
    return it->second;
  int newcolor = nextcolor_;
  nextcolor_++;
  colormap_.emplace_back(oldcolor, newcolor);
  return newcolor;
}

void Prog::ComputeByteMap() {
  // Fill in bytemap with byte classes for the program.
  // Ranges of bytes that are treated indistinguishably
  // will be mapped to a single byte class.
  ByteMapBuilder builder;

  // Don't repeat the work for ^ and $.
  bool marked_line_boundaries = false;
  // Don't repeat the work for \b and \B.
  bool marked_word_boundaries = false;

  for (int id = 0; id < size(); id++) {
    Inst* ip = inst(id);
    if (ip->opcode() == kInstByteRange) {
      int lo = ip->lo();
      int hi = ip->hi();
      builder.Mark(lo, hi);
      if (ip->foldcase() && lo <= 'z' && hi >= 'a') {
        int foldlo = lo;
        int foldhi = hi;
        if (foldlo < 'a')
          foldlo = 'a';
        if (foldhi > 'z')
          foldhi = 'z';
        if (foldlo <= foldhi) {
          foldlo += 'A' - 'a';
          foldhi += 'A' - 'a';
          builder.Mark(foldlo, foldhi);
        }
      }
      // If this Inst is not the last Inst in its list AND the next Inst is
      // also a ByteRange AND the Insts have the same out, defer the merge.
      if (!ip->last() &&
          inst(id+1)->opcode() == kInstByteRange &&
          ip->out() == inst(id+1)->out())
        continue;
      builder.Merge();
    } else if (ip->opcode() == kInstEmptyWidth) {
      if (ip->empty() & (kEmptyBeginLine|kEmptyEndLine) &&
          !marked_line_boundaries) {
        builder.Mark('\n', '\n');
        builder.Merge();
        marked_line_boundaries = true;
      }
      if (ip->empty() & (kEmptyWordBoundary|kEmptyNonWordBoundary) &&
          !marked_word_boundaries) {
        // We require two batches here: the first for ranges that are word
        // characters, the second for ranges that are not word characters.
        for (bool isword : {true, false}) {
          int j;
          for (int i = 0; i < 256; i = j) {
            for (j = i + 1; j < 256 &&
                            Prog::IsWordChar(static_cast<uint8_t>(i)) ==
                                Prog::IsWordChar(static_cast<uint8_t>(j));
                 j++)
              ;
            if (Prog::IsWordChar(static_cast<uint8_t>(i)) == isword)
              builder.Mark(i, j - 1);
          }
          builder.Merge();
        }
        marked_word_boundaries = true;
      }
    }
  }

  builder.Build(bytemap_, &bytemap_range_);

  if ((0)) {  // For debugging, use trivial bytemap.
    LOG(ERROR) << "Using trivial bytemap.";
    for (int i = 0; i < 256; i++)
      bytemap_[i] = static_cast<uint8_t>(i);
    bytemap_range_ = 256;
  }
}

// Prog::Flatten() implements a graph rewriting algorithm.
//
// The overall process is similar to epsilon removal, but retains some epsilon
// transitions: those from Capture and EmptyWidth instructions; and those from
// nullable subexpressions. (The latter avoids quadratic blowup in transitions
// in the worst case.) It might be best thought of as Alt instruction elision.
//
// In conceptual terms, it divides the Prog into "trees" of instructions, then
// traverses the "trees" in order to produce "lists" of instructions. A "tree"
// is one or more instructions that grow from one "root" instruction to one or
// more "leaf" instructions; if a "tree" has exactly one instruction, then the
// "root" is also the "leaf". In most cases, a "root" is the successor of some
// "leaf" (i.e. the "leaf" instruction's out() returns the "root" instruction)
// and is considered a "successor root". A "leaf" can be a ByteRange, Capture,
// EmptyWidth or Match instruction. However, this is insufficient for handling
// nested nullable subexpressions correctly, so in some cases, a "root" is the
// dominator of the instructions reachable from some "successor root" (i.e. it
// has an unreachable predecessor) and is considered a "dominator root". Since
// only Alt instructions can be "dominator roots" (other instructions would be
// "leaves"), only Alt instructions are required to be marked as predecessors.
//
// Dividing the Prog into "trees" comprises two passes: marking the "successor
// roots" and the predecessors; and marking the "dominator roots". Sorting the
// "successor roots" by their bytecode offsets enables iteration in order from
// greatest to least during the second pass; by working backwards in this case
// and flooding the graph no further than "leaves" and already marked "roots",
// it becomes possible to mark "dominator roots" without doing excessive work.
//
// Traversing the "trees" is just iterating over the "roots" in order of their
// marking and flooding the graph no further than "leaves" and "roots". When a
// "leaf" is reached, the instruction is copied with its successor remapped to
// its "root" number. When a "root" is reached, a Nop instruction is generated
// with its successor remapped similarly. As each "list" is produced, its last
// instruction is marked as such. After all of the "lists" have been produced,
// a pass over their instructions remaps their successors to bytecode offsets.
void Prog::Flatten() {
  if (did_flatten_)
    return;
  did_flatten_ = true;

  // Scratch structures. It's important that these are reused by functions
  // that we call in loops because they would thrash the heap otherwise.
  SparseSet reachable(size());
  std::vector<int> stk;
  stk.reserve(size());

  // First pass: Marks "successor roots" and predecessors.
  // Builds the mapping from inst-ids to root-ids.
  SparseArray<int> rootmap(size());
  SparseArray<int> predmap(size());
  std::vector<std::vector<int>> predvec;
  MarkSuccessors(&rootmap, &predmap, &predvec, &reachable, &stk);

  // Second pass: Marks "dominator roots".
  SparseArray<int> sorted(rootmap);
  std::sort(sorted.begin(), sorted.end(), sorted.less);
  for (SparseArray<int>::const_iterator i = sorted.end() - 1;
       i != sorted.begin();
       --i) {
    if (i->index() != start_unanchored() && i->index() != start())
      MarkDominator(i->index(), &rootmap, &predmap, &predvec, &reachable, &stk);
  }

  // Third pass: Emits "lists". Remaps outs to root-ids.
  // Builds the mapping from root-ids to flat-ids.
  std::vector<int> flatmap(rootmap.size());
  std::vector<Inst> flat;
  flat.reserve(size());
  for (SparseArray<int>::const_iterator i = rootmap.begin();
       i != rootmap.end();
       ++i) {
    flatmap[i->value()] = static_cast<int>(flat.size());
    EmitList(i->index(), &rootmap, &flat, &reachable, &stk);
    flat.back().set_last();
    // We have the bounds of the "list", so this is the
    // most convenient point at which to compute hints.
    ComputeHints(&flat, flatmap[i->value()], static_cast<int>(flat.size()));
  }

  list_count_ = static_cast<int>(flatmap.size());
  for (int i = 0; i < kNumInst; i++)
    inst_count_[i] = 0;

  // Fourth pass: Remaps outs to flat-ids.
  // Counts instructions by opcode.
  for (int id = 0; id < static_cast<int>(flat.size()); id++) {
    Inst* ip = &flat[id];
    if (ip->opcode() != kInstAltMatch)  // handled in EmitList()
      ip->set_out(flatmap[ip->out()]);
    inst_count_[ip->opcode()]++;
  }

#if !defined(NDEBUG)
  // Address a `-Wunused-but-set-variable' warning from Clang 13.x.
  size_t total = 0;
  for (int i = 0; i < kNumInst; i++)
    total += inst_count_[i];
  CHECK_EQ(total, flat.size());
#endif

  // Remap start_unanchored and start.
  if (start_unanchored() == 0) {
    DCHECK_EQ(start(), 0);
  } else if (start_unanchored() == start()) {
    set_start_unanchored(flatmap[1]);
    set_start(flatmap[1]);
  } else {
    set_start_unanchored(flatmap[1]);
    set_start(flatmap[2]);
  }

  // Finally, replace the old instructions with the new instructions.
  size_ = static_cast<int>(flat.size());
  inst_ = PODArray<Inst>(size_);
  memmove(inst_.data(), flat.data(), size_*sizeof inst_[0]);

  // Populate the list heads for BitState.
  // 512 instructions limits the memory footprint to 1KiB.
  if (size_ <= 512) {
    list_heads_ = PODArray<uint16_t>(size_);
    // 0xFF makes it more obvious if we try to look up a non-head.
    memset(list_heads_.data(), 0xFF, size_*sizeof list_heads_[0]);
    for (int i = 0; i < list_count_; ++i)
      list_heads_[flatmap[i]] = i;
  }

  // BitState allocates a bitmap of size list_count_ * (text.size()+1)
  // for tracking pairs of possibilities that it has already explored.
  const size_t kBitStateBitmapMaxSize = 256*1024;  // max size in bits
  bit_state_text_max_size_ = kBitStateBitmapMaxSize / list_count_ - 1;
}

void Prog::MarkSuccessors(SparseArray<int>* rootmap,
                          SparseArray<int>* predmap,
                          std::vector<std::vector<int>>* predvec,
                          SparseSet* reachable, std::vector<int>* stk) {
  // Mark the kInstFail instruction.
  rootmap->set_new(0, rootmap->size());

  // Mark the start_unanchored and start instructions.
  if (!rootmap->has_index(start_unanchored()))
    rootmap->set_new(start_unanchored(), rootmap->size());
  if (!rootmap->has_index(start()))
    rootmap->set_new(start(), rootmap->size());

  reachable->clear();
  stk->clear();
  stk->push_back(start_unanchored());
  while (!stk->empty()) {
    int id = stk->back();
    stk->pop_back();
  Loop:
    if (reachable->contains(id))
      continue;
    reachable->insert_new(id);

    Inst* ip = inst(id);
    switch (ip->opcode()) {
      default:
        LOG(DFATAL) << "unhandled opcode: " << ip->opcode();
        break;

      case kInstAltMatch:
      case kInstAlt:
        // Mark this instruction as a predecessor of each out.
        for (int out : {ip->out(), ip->out1()}) {
          if (!predmap->has_index(out)) {
            predmap->set_new(out, static_cast<int>(predvec->size()));
            predvec->emplace_back();
          }
          (*predvec)[predmap->get_existing(out)].emplace_back(id);
        }
        stk->push_back(ip->out1());
        id = ip->out();
        goto Loop;

      case kInstByteRange:
      case kInstCapture:
      case kInstEmptyWidth:
        // Mark the out of this instruction as a "root".
        if (!rootmap->has_index(ip->out()))
          rootmap->set_new(ip->out(), rootmap->size());
        id = ip->out();
        goto Loop;

      case kInstNop:
        id = ip->out();
        goto Loop;

      case kInstMatch:
      case kInstFail:
        break;
    }
  }
}

void Prog::MarkDominator(int root, SparseArray<int>* rootmap,
                         SparseArray<int>* predmap,
                         std::vector<std::vector<int>>* predvec,
                         SparseSet* reachable, std::vector<int>* stk) {
  reachable->clear();
  stk->clear();
  stk->push_back(root);
  while (!stk->empty()) {
    int id = stk->back();
    stk->pop_back();
  Loop:
    if (reachable->contains(id))
      continue;
    reachable->insert_new(id);

    if (id != root && rootmap->has_index(id)) {
      // We reached another "tree" via epsilon transition.
      continue;
    }

    Inst* ip = inst(id);
    switch (ip->opcode()) {
      default:
        LOG(DFATAL) << "unhandled opcode: " << ip->opcode();
        break;

      case kInstAltMatch:
      case kInstAlt:
        stk->push_back(ip->out1());
        id = ip->out();
        goto Loop;

      case kInstByteRange:
      case kInstCapture:
      case kInstEmptyWidth:
        break;

      case kInstNop:
        id = ip->out();
        goto Loop;

      case kInstMatch:
      case kInstFail:
        break;
    }
  }

  for (SparseSet::const_iterator i = reachable->begin();
       i != reachable->end();
       ++i) {
    int id = *i;
    if (predmap->has_index(id)) {
      for (int pred : (*predvec)[predmap->get_existing(id)]) {
        if (!reachable->contains(pred)) {
          // id has a predecessor that cannot be reached from root!
          // Therefore, id must be a "root" too - mark it as such.
          if (!rootmap->has_index(id))
            rootmap->set_new(id, rootmap->size());
        }
      }
    }
  }
}

void Prog::EmitList(int root, SparseArray<int>* rootmap,
                    std::vector<Inst>* flat,
                    SparseSet* reachable, std::vector<int>* stk) {
  reachable->clear();
  stk->clear();
  stk->push_back(root);
  while (!stk->empty()) {
    int id = stk->back();
    stk->pop_back();
  Loop:
    if (reachable->contains(id))
      continue;
    reachable->insert_new(id);

    if (id != root && rootmap->has_index(id)) {
      // We reached another "tree" via epsilon transition. Emit a kInstNop
      // instruction so that the Prog does not become quadratically larger.
      flat->emplace_back();
      flat->back().set_opcode(kInstNop);
      flat->back().set_out(rootmap->get_existing(id));
      continue;
    }

    Inst* ip = inst(id);
    switch (ip->opcode()) {
      default:
        LOG(DFATAL) << "unhandled opcode: " << ip->opcode();
        break;

      case kInstAltMatch:
        flat->emplace_back();
        flat->back().set_opcode(kInstAltMatch);
        flat->back().set_out(static_cast<int>(flat->size()));
        flat->back().out1_ = static_cast<uint32_t>(flat->size())+1;
        FALLTHROUGH_INTENDED;

      case kInstAlt:
        stk->push_back(ip->out1());
        id = ip->out();
        goto Loop;

      case kInstByteRange:
      case kInstCapture:
      case kInstEmptyWidth:
        flat->emplace_back();
        memmove(&flat->back(), ip, sizeof *ip);
        flat->back().set_out(rootmap->get_existing(ip->out()));
        break;

      case kInstNop:
        id = ip->out();
        goto Loop;

      case kInstMatch:
      case kInstFail:
        flat->emplace_back();
        memmove(&flat->back(), ip, sizeof *ip);
        break;
    }
  }
}

// For each ByteRange instruction in [begin, end), computes a hint to execution
// engines: the delta to the next instruction (in flat) worth exploring iff the
// current instruction matched.
//
// Implements a coloring algorithm related to ByteMapBuilder, but in this case,
// colors are instructions and recoloring ranges precisely identifies conflicts
// between instructions. Iterating backwards over [begin, end) is guaranteed to
// identify the nearest conflict (if any) with only linear complexity.
void Prog::ComputeHints(std::vector<Inst>* flat, int begin, int end) {
  Bitmap256 splits;
  int colors[256];

  bool dirty = false;
  for (int id = end; id >= begin; --id) {
    if (id == end ||
        (*flat)[id].opcode() != kInstByteRange) {
      if (dirty) {
        dirty = false;
        splits.Clear();
      }
      splits.Set(255);
      colors[255] = id;
      // At this point, the [0-255] range is colored with id.
      // Thus, hints cannot point beyond id; and if id == end,
      // hints that would have pointed to id will be 0 instead.
      continue;
    }
    dirty = true;

    // We recolor the [lo-hi] range with id. Note that first ratchets backwards
    // from end to the nearest conflict (if any) during recoloring.
    int first = end;
    auto Recolor = [&](int lo, int hi) {
      // Like ByteMapBuilder, we split at lo-1 and at hi.
      --lo;

      if (0 <= lo && !splits.Test(lo)) {
        splits.Set(lo);
        int next = splits.FindNextSetBit(lo+1);
        colors[lo] = colors[next];
      }
      if (!splits.Test(hi)) {
        splits.Set(hi);
        int next = splits.FindNextSetBit(hi+1);
        colors[hi] = colors[next];
      }

      int c = lo+1;
      while (c < 256) {
        int next = splits.FindNextSetBit(c);
        // Ratchet backwards...
        first = std::min(first, colors[next]);
        // Recolor with id - because it's the new nearest conflict!
        colors[next] = id;
        if (next == hi)
          break;
        c = next+1;
      }
    };

    Inst* ip = &(*flat)[id];
    int lo = ip->lo();
    int hi = ip->hi();
    Recolor(lo, hi);
    if (ip->foldcase() && lo <= 'z' && hi >= 'a') {
      int foldlo = lo;
      int foldhi = hi;
      if (foldlo < 'a')
        foldlo = 'a';
      if (foldhi > 'z')
        foldhi = 'z';
      if (foldlo <= foldhi) {
        foldlo += 'A' - 'a';
        foldhi += 'A' - 'a';
        Recolor(foldlo, foldhi);
      }
    }

    if (first != end) {
      uint16_t hint = static_cast<uint16_t>(std::min(first - id, 32767));
      ip->byte_range.hint_foldcase_ |= hint<<1;
    }
  }
}

// The final state will always be this, which frees up a register for the hot
// loop and thus avoids the spilling that can occur when building with Clang.
static const size_t kShiftDFAFinal = 9;

// This function takes the prefix as std::string (i.e. not const std::string&
// as normal) because it's going to clobber it, so a temporary is convenient.
static uint64_t* BuildShiftDFA(std::string prefix) {
  // This constant is for convenience now and also for correctness later when
  // we clobber the prefix, but still need to know how long it was initially.
  const size_t size = prefix.size();

  // Construct the NFA.
  // The table is indexed by input byte; each element is a bitfield of states
  // reachable by the input byte. Given a bitfield of the current states, the
  // bitfield of states reachable from those is - for this specific purpose -
  // always ((ncurr << 1) | 1). Intersecting the reachability bitfields gives
  // the bitfield of the next states reached by stepping over the input byte.
  // Credits for this technique: the Hyperscan paper by Geoff Langdale et al.
  uint16_t nfa[256]{};
  for (size_t i = 0; i < size; ++i) {
    uint8_t b = prefix[i];
    nfa[b] |= 1 << (i+1);
  }
  // This is the `\C*?` for unanchored search.
  for (int b = 0; b < 256; ++b)
    nfa[b] |= 1;

  // This maps from DFA state to NFA states; the reverse mapping is used when
  // recording transitions and gets implemented with plain old linear search.
  // The "Shift DFA" technique limits this to ten states when using uint64_t;
  // to allow for the initial state, we use at most nine bytes of the prefix.
  // That same limit is also why uint16_t is sufficient for the NFA bitfield.
  uint16_t states[kShiftDFAFinal+1]{};
  states[0] = 1;
  for (size_t dcurr = 0; dcurr < size; ++dcurr) {
    uint8_t b = prefix[dcurr];
    uint16_t ncurr = states[dcurr];
    uint16_t nnext = nfa[b] & ((ncurr << 1) | 1);
    size_t dnext = dcurr+1;
    if (dnext == size)
      dnext = kShiftDFAFinal;
    states[dnext] = nnext;
  }

  // Sort and unique the bytes of the prefix to avoid repeating work while we
  // record transitions. This clobbers the prefix, but it's no longer needed.
  std::sort(prefix.begin(), prefix.end());
  prefix.erase(std::unique(prefix.begin(), prefix.end()), prefix.end());

  // Construct the DFA.
  // The table is indexed by input byte; each element is effectively a packed
  // array of uint6_t; each array value will be multiplied by six in order to
  // avoid having to do so later in the hot loop as well as masking/shifting.
  // Credits for this technique: "Shift-based DFAs" on GitHub by Per Vognsen.
  uint64_t* dfa = new uint64_t[256]{};
  // Record a transition from each state for each of the bytes of the prefix.
  // Note that all other input bytes go back to the initial state by default.
  for (size_t dcurr = 0; dcurr < size; ++dcurr) {
    for (uint8_t b : prefix) {
      uint16_t ncurr = states[dcurr];
      uint16_t nnext = nfa[b] & ((ncurr << 1) | 1);
      size_t dnext = 0;
      while (states[dnext] != nnext)
        ++dnext;
      dfa[b] |= static_cast<uint64_t>(dnext * 6) << (dcurr * 6);
      // Convert ASCII letters to uppercase and record the extra transitions.
      // Note that ASCII letters are guaranteed to be lowercase at this point
      // because that's how the parser normalises them. #FunFact: 'k' and 's'
      // match U+212A and U+017F, respectively, so they won't occur here when
      // using UTF-8 encoding because the parser will emit character classes.
      if ('a' <= b && b <= 'z') {
        b -= 'a' - 'A';
        dfa[b] |= static_cast<uint64_t>(dnext * 6) << (dcurr * 6);
      }
    }
  }
  // This lets the final state "saturate", which will matter for performance:
  // in the hot loop, we check for a match only at the end of each iteration,
  // so we must keep signalling the match until we get around to checking it.
  for (int b = 0; b < 256; ++b)
    dfa[b] |= static_cast<uint64_t>(kShiftDFAFinal * 6) << (kShiftDFAFinal * 6);

  return dfa;
}

void Prog::ConfigurePrefixAccel(const std::string& prefix,
                                bool prefix_foldcase) {
  prefix_foldcase_ = prefix_foldcase;
  prefix_size_ = prefix.size();
  if (prefix_foldcase_) {
    // Use PrefixAccel_ShiftDFA().
    // ... and no more than nine bytes of the prefix. (See above for details.)
    prefix_size_ = std::min(prefix_size_, kShiftDFAFinal);
    prefix_dfa_ = BuildShiftDFA(prefix.substr(0, prefix_size_));
  } else if (prefix_size_ != 1) {
    // Use PrefixAccel_FrontAndBack().
    prefix_front_back.prefix_front_ = prefix.front();
	prefix_front_back.prefix_back_ = prefix.back();
  } else {
    // Use memchr(3).
	prefix_front_back.prefix_front_ = prefix.front();
  }
}

const void* Prog::PrefixAccel_ShiftDFA(const void* data, size_t size) {
  if (size < prefix_size_)
    return NULL;

  uint64_t curr = 0;

  // At the time of writing, rough benchmarks on a Broadwell machine showed
  // that this unroll factor (i.e. eight) achieves a speedup factor of two.
  if (size >= 8) {
    const uint8_t* p = reinterpret_cast<const uint8_t*>(data);
    const uint8_t* endp = p + (size&~7);
    do {
      uint8_t b0 = p[0];
      uint8_t b1 = p[1];
      uint8_t b2 = p[2];
      uint8_t b3 = p[3];
      uint8_t b4 = p[4];
      uint8_t b5 = p[5];
      uint8_t b6 = p[6];
      uint8_t b7 = p[7];

      uint64_t next0 = prefix_dfa_[b0];
      uint64_t next1 = prefix_dfa_[b1];
      uint64_t next2 = prefix_dfa_[b2];
      uint64_t next3 = prefix_dfa_[b3];
      uint64_t next4 = prefix_dfa_[b4];
      uint64_t next5 = prefix_dfa_[b5];
      uint64_t next6 = prefix_dfa_[b6];
      uint64_t next7 = prefix_dfa_[b7];

      uint64_t curr0 = next0 >> (curr  & 63);
      uint64_t curr1 = next1 >> (curr0 & 63);
      uint64_t curr2 = next2 >> (curr1 & 63);
      uint64_t curr3 = next3 >> (curr2 & 63);
      uint64_t curr4 = next4 >> (curr3 & 63);
      uint64_t curr5 = next5 >> (curr4 & 63);
      uint64_t curr6 = next6 >> (curr5 & 63);
      uint64_t curr7 = next7 >> (curr6 & 63);

      if ((curr7 & 63) == kShiftDFAFinal * 6) {
        // At the time of writing, using the same masking subexpressions from
        // the preceding lines caused Clang to clutter the hot loop computing
        // them - even though they aren't actually needed for shifting! Hence
        // these rewritten conditions, which achieve a speedup factor of two.
        if (((curr7-curr0) & 63) == 0) return p+1-prefix_size_;
        if (((curr7-curr1) & 63) == 0) return p+2-prefix_size_;
        if (((curr7-curr2) & 63) == 0) return p+3-prefix_size_;
        if (((curr7-curr3) & 63) == 0) return p+4-prefix_size_;
        if (((curr7-curr4) & 63) == 0) return p+5-prefix_size_;
        if (((curr7-curr5) & 63) == 0) return p+6-prefix_size_;
        if (((curr7-curr6) & 63) == 0) return p+7-prefix_size_;
        if (((curr7-curr7) & 63) == 0) return p+8-prefix_size_;
      }

      curr = curr7;
      p += 8;
    } while (p != endp);
    data = p;
    size = size&7;
  }

  const uint8_t* p = reinterpret_cast<const uint8_t*>(data);
  const uint8_t* endp = p + size;
  while (p != endp) {
    uint8_t b = *p++;
    uint64_t next = prefix_dfa_[b];
    curr = next >> (curr & 63);
    if ((curr & 63) == kShiftDFAFinal * 6)
      return p-prefix_size_;
  }
  return NULL;
}

#if defined(__AVX2__)
// Finds the least significant non-zero bit in n.
static int FindLSBSet(uint32_t n) {
  DCHECK_NE(n, 0);
#if defined(__GNUC__)
  return __builtin_ctz(n);
#elif defined(_MSC_VER) && (defined(_M_X64) || defined(_M_IX86))
  unsigned long c;
  _BitScanForward(&c, n);
  return static_cast<int>(c);
#else
  int c = 31;
  for (int shift = 1 << 4; shift != 0; shift >>= 1) {
    uint32_t word = n << shift;
    if (word != 0) {
      n = word;
      c -= shift;
    }
  }
  return c;
#endif
}
#endif

const void* Prog::PrefixAccel_FrontAndBack(const void* data, size_t size) {
  DCHECK_GE(prefix_size_, 2);
  if (size < prefix_size_)
    return NULL;
  // Don't bother searching the last prefix_size_-1 bytes for prefix_front_.
  // This also means that probing for prefix_back_ doesn't go out of bounds.
  size -= prefix_size_-1;

#if defined(__AVX2__)
  // Use AVX2 to look for prefix_front_ and prefix_back_ 32 bytes at a time.
  if (size >= sizeof(__m256i)) {
    const __m256i* fp = reinterpret_cast<const __m256i*>(
        reinterpret_cast<const char*>(data));
    const __m256i* bp = reinterpret_cast<const __m256i*>(
        reinterpret_cast<const char*>(data) + prefix_size_-1);
    const __m256i* endfp = fp + size/sizeof(__m256i);
    const __m256i f_set1 = _mm256_set1_epi8(prefix_front_back.prefix_front_);
    const __m256i b_set1 = _mm256_set1_epi8(prefix_front_back.prefix_back_);
    do {
      const __m256i f_loadu = _mm256_loadu_si256(fp++);
      const __m256i b_loadu = _mm256_loadu_si256(bp++);
      const __m256i f_cmpeq = _mm256_cmpeq_epi8(f_set1, f_loadu);
      const __m256i b_cmpeq = _mm256_cmpeq_epi8(b_set1, b_loadu);
      const int fb_testz = _mm256_testz_si256(f_cmpeq, b_cmpeq);
      if (fb_testz == 0) {  // ZF: 1 means zero, 0 means non-zero.
        const __m256i fb_and = _mm256_and_si256(f_cmpeq, b_cmpeq);
        const int fb_movemask = _mm256_movemask_epi8(fb_and);
        const int fb_ctz = FindLSBSet(fb_movemask);
        return reinterpret_cast<const char*>(fp-1) + fb_ctz;
      }
    } while (fp != endfp);
    data = fp;
    size = size%sizeof(__m256i);
  }
#endif

  const char* p0 = reinterpret_cast<const char*>(data);
  for (const char* p = p0;; p++) {
    DCHECK_GE(size, static_cast<size_t>(p-p0));
    p = reinterpret_cast<const char*>(memchr(p, prefix_front_back.prefix_front_, size - (p-p0)));
    if (p == NULL || p[prefix_size_-1] == prefix_front_back.prefix_back_)
      return p;
  }
}

} // namespace re2
