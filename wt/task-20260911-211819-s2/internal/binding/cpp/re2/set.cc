// Copyright 2010 The RE2 Authors.  All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

#include "re2/set.h"

#include <algorithm>
#include <memory>
#include <stddef.h>
#include <utility>

#include "re2/pod_array.h"
#include "re2/prog.h"
#include "re2/re2.h"
#include "re2/regexp.h"
#include "re2/stringpiece.h"
#include "util/logging.h"
#include "util/util.h"

namespace re2 {

RE2::Set::Set(const RE2::Options &options, RE2::Anchor anchor) : options_(options), anchor_(anchor), compiled_(false), size_(0) {
    options_.set_never_capture(true); // might unblock some optimisations
}

RE2::Set::~Set() {
    for (size_t i = 0; i < elem_.size(); i++)
        elem_[i].second->Decref();
}

RE2::Set::Set(Set &&other)
    : options_(other.options_), anchor_(other.anchor_), elem_(std::move(other.elem_)), compiled_(other.compiled_), size_(other.size_),
      prog_(std::move(other.prog_)) {
    other.elem_.clear();
    other.elem_.shrink_to_fit();
    other.compiled_ = false;
    other.size_ = 0;
    other.prog_.reset();
}

RE2::Set &RE2::Set::operator=(Set &&other) {
    this->~Set();
    (void)new (this) Set(std::move(other));
    return *this;
}

int RE2::Set::Add(const StringPiece &pattern, std::string *error) {
    if (compiled_) {
        LOG(DFATAL) << "RE2::Set::Add() called after compiling";
        return -1;
    }

    Regexp::ParseFlags pf = static_cast<Regexp::ParseFlags>(options_.ParseFlags());
    RegexpStatus status;
    re2::Regexp *re = Regexp::Parse(pattern, pf, &status);
    if (re == NULL) {
        if (error != NULL)
            *error = status.Text();
        if (options_.log_errors())
            LOG(ERROR) << "Error parsing '" << pattern << "': " << status.Text();
        return -1;
    }

    // Concatenate with match index and push on vector.
    int n = static_cast<int>(elem_.size());
    re2::Regexp *m = re2::Regexp::HaveMatch(n, pf);
    if (re->op() == kRegexpConcat) {
        int nsub = re->nsub();
        PODArray<re2::Regexp *> sub(nsub + 1);
        for (int i = 0; i < nsub; i++)
            sub[i] = re->sub()[i]->Incref();
        sub[nsub] = m;
        re->Decref();
        re = re2::Regexp::Concat(sub.data(), nsub + 1, pf);
    } else {
        re2::Regexp *sub[2];
        sub[0] = re;
        sub[1] = m;
        re = re2::Regexp::Concat(sub, 2, pf);
    }
    elem_.emplace_back(std::string(pattern), re);
    return n;
}

bool RE2::Set::Compile() {
    if (compiled_) {
        LOG(DFATAL) << "RE2::Set::Compile() called more than once";
        return false;
    }
    compiled_ = true;
    size_ = static_cast<int>(elem_.size());

    // Sort the elements by their patterns. This is good enough for now
    // until we have a Regexp comparison function. (Maybe someday...)
    std::sort(elem_.begin(), elem_.end(), [](const Elem &a, const Elem &b) -> bool { return a.first < b.first; });

    PODArray<re2::Regexp *> sub(size_);
    for (int i = 0; i < size_; i++)
        sub[i] = elem_[i].second;
    elem_.clear();
    elem_.shrink_to_fit();

    Regexp::ParseFlags pf = static_cast<Regexp::ParseFlags>(options_.ParseFlags());
    re2::Regexp *re = re2::Regexp::Alternate(sub.data(), size_, pf);

    prog_.reset(Prog::CompileSet(re, anchor_, options_.max_mem()));
    re->Decref();
    return prog_ != nullptr;
}

bool RE2::Set::Match(const StringPiece &text, std::vector<int> *v) const { return Match(text, v, NULL); }

bool RE2::Set::Match(const StringPiece &text, std::vector<int> *v, ErrorInfo *error_info) const {
    if (!compiled_) {
        if (error_info != NULL)
            error_info->kind = kNotCompiled;
        LOG(DFATAL) << "RE2::Set::Match() called before compiling";
        return false;
    }
#ifdef RE2_HAVE_THREAD_LOCAL
    hooks::context = NULL;
#endif
    bool dfa_failed = false;
    std::unique_ptr<SparseSet> matches;
    if (v != NULL) {
        matches.reset(new SparseSet(size_));
        v->clear();
    }
    bool ret = prog_->SearchDFA(text, text, Prog::kAnchored, Prog::kManyMatch, NULL, &dfa_failed, matches.get());
    if (dfa_failed) {
        if (options_.log_errors())
            LOG(ERROR) << "DFA out of memory: "
                       << "program size " << prog_->size() << ", "
                       << "list count " << prog_->list_count() << ", "
                       << "bytemap range " << prog_->bytemap_range();
        if (error_info != NULL)
            error_info->kind = kOutOfMemory;
        return false;
    }
    if (ret == false) {
        if (error_info != NULL)
            error_info->kind = kNoError;
        return false;
    }
    if (v != NULL) {
        if (matches->empty()) {
            if (error_info != NULL)
                error_info->kind = kInconsistent;
            LOG(DFATAL) << "RE2::Set::Match() matched, but no matches returned?!";
            return false;
        }
        v->assign(matches->begin(), matches->end());
    }
    if (error_info != NULL)
        error_info->kind = kNoError;
    return true;
}

} // namespace re2
