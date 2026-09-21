//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package tokenizer

// Byte-pair-encoding counters: the Qwen and Llama/Mistral embedding families.
//
// Two flavours share one merge engine, because they differ only in how text is
// prepared before merging. Both are read from the model's `tokenizer.json`
// (vocab + merges), and both are verified sample-by-sample against that same
// file through scripts/gen_tokenizer_oracle.py:
//
//	llama-bpe (e5-mistral, Mistral/Llama SentencePiece-BPE)
//	  normalizer    Sequence(Prepend("▁"), Replace(" " -> "▁"))
//	  pre_tokenizer none - the whole text is one merge unit
//	  model         BPE, byte_fallback=true, fuse_unk=true, unk="<unk>"
//
//	qwen-bpe (Qwen3-Embedding)
//	  normalizer    NFC
//	  pre_tokenizer Sequence(Split(<regex>, isolated), ByteLevel(use_regex=false))
//	  model         BPE over the GPT-2 byte alphabet, no unk, no byte_fallback
//
// The Qwen pre-tokenizer regex cannot be compiled by Go's regexp (RE2): it uses a
// negative lookahead (`\s+(?!\S)`). It is implemented as an explicit scanner in
// pretokenizeQwen below, which is what the oracle test exists to keep honest.

import (
	"container/heap"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// normNFC is the normalizer step Qwen's tokenizer.json declares.
func normNFC(s string) string { return norm.NFC.String(s) }

// mergeCacheLimit bounds the per-piece memo. Pieces repeat heavily across chunks
// ("▁the", "▁and", ...), so a small cache removes most of the merge work.
const mergeCacheLimit = 8192

// bpeFlavor selects the text preparation used before merging.
type bpeFlavor int

const (
	// bpeFlavorLlama is SentencePiece-BPE: prepend+replace to ▁, merge the whole
	// text, unknown runes fall back to <0xXX> byte pieces.
	bpeFlavorLlama bpeFlavor = iota
	// bpeFlavorQwen is byte-level BPE: NFC, split by regex, map bytes through the
	// GPT-2 alphabet, then merge inside each split piece.
	bpeFlavorQwen
)

// bpeModel is a tokenizer.json BPE model plus the flavor it is served with.
type bpeModel struct {
	vocab  map[string]int32
	merges map[string]int
	unk    string

	byteFallback bool
	byteLevel    bool
	flavor       bpeFlavor

	mu   sync.Mutex
	memo map[string]int

	// names is the id -> token map, built on first use (only the oracle test asks
	// for token texts).
	nameOnce sync.Once
	names    map[int32]string
}

// tokenizerJSON is the slice of tokenizer.json this package reads. Everything
// else (post_processors, decoders) cannot change a token count.
type tokenizerJSON struct {
	Model struct {
		Type         string           `json:"type"`
		Vocab        map[string]int32 `json:"vocab"`
		Merges       json.RawMessage  `json:"merges"`
		UnkToken     *string          `json:"unk_token"`
		ByteFallback bool             `json:"byte_fallback"`
	} `json:"model"`
	AddedTokens []struct {
		Content string `json:"content"`
	} `json:"added_tokens"`
}

// parseMerges accepts both spellings seen in the wild: an array of "a b" strings
// (HF) and an array of ["a","b"] pairs.
func parseMerges(raw json.RawMessage) (map[string]int, error) {
	if len(raw) == 0 {
		return map[string]int{}, nil
	}
	var asStrings []string
	if err := json.Unmarshal(raw, &asStrings); err == nil {
		out := make(map[string]int, len(asStrings))
		for i, m := range asStrings {
			out[m] = i
		}
		return out, nil
	}
	var asPairs [][]string
	if err := json.Unmarshal(raw, &asPairs); err != nil {
		return nil, fmt.Errorf("bpe: unsupported merges format: %w", err)
	}
	out := make(map[string]int, len(asPairs))
	for i, pair := range asPairs {
		if len(pair) != 2 {
			return nil, fmt.Errorf("bpe: merge %d has %d parts", i, len(pair))
		}
		out[pair[0]+" "+pair[1]] = i
	}
	return out, nil
}

func loadBPEModel(path string, flavor bpeFlavor) (*bpeModel, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("bpe: reading %s: %w", path, err)
	}
	var parsed tokenizerJSON
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("bpe: parsing %s: %w", path, err)
	}
	if !strings.EqualFold(parsed.Model.Type, "BPE") {
		return nil, fmt.Errorf("bpe: %s declares model type %q, want BPE", path, parsed.Model.Type)
	}
	merges, err := parseMerges(parsed.Model.Merges)
	if err != nil {
		return nil, err
	}
	if len(parsed.Model.Vocab) == 0 {
		return nil, fmt.Errorf("bpe: %s has an empty vocabulary", path)
	}
	m := &bpeModel{
		vocab:        parsed.Model.Vocab,
		merges:       merges,
		byteFallback: parsed.Model.ByteFallback,
		byteLevel:    flavor == bpeFlavorQwen,
		flavor:       flavor,
		memo:         make(map[string]int),
	}
	if parsed.Model.UnkToken != nil {
		m.unk = *parsed.Model.UnkToken
	}
	return m, nil
}

// normalize prepares text the way the served tokenizer does.
func (m *bpeModel) normalize(text string) string {
	if text == "" {
		return ""
	}
	switch m.flavor {
	case bpeFlavorQwen:
		// normalizer: NFC
		return normNFC(text)
	default:
		// normalizer: Sequence(Prepend("▁"), Replace(" " -> "▁"))
		s := strings.ReplaceAll(text, " ", "\u2581")
		return "\u2581" + s
	}
}

// countTokens counts the merge units of text.
func (m *bpeModel) countTokens(text string) int {
	if text == "" {
		return 0
	}
	normalized := m.normalize(text)
	if normalized == "" {
		return 0
	}
	pieces := []string{normalized}
	if m.flavor == bpeFlavorQwen {
		pieces = pretokenizeQwen(normalized)
	}
	total := 0
	for _, piece := range pieces {
		total += m.countPiece(piece)
	}
	return total
}

// countPiece is the BPE merge loop for one unit.
func (m *bpeModel) countPiece(piece string) int {
	if piece == "" {
		return 0
	}
	if cached, ok := m.memoGet(piece); ok {
		return cached
	}
	var symbols []string
	if m.byteLevel {
		// Byte-level BPE starts from the GPT-2 alphabet, one symbol per byte.
		for i := 0; i < len(piece); i++ {
			symbols = append(symbols, string(byteToRune[piece[i]]))
		}
	} else {
		for _, r := range piece {
			symbols = append(symbols, string(r))
		}
	}
	if len(symbols) > 1 {
		symbols = m.merge(symbols)
	}
	count := 0
	for _, sym := range symbols {
		if _, ok := m.vocab[sym]; ok {
			count++
			continue
		}
		// A symbol the vocabulary does not contain: byte fallback emits one
		// <0xXX> piece per UTF-8 byte of the symbol, otherwise it is a single
		// unknown token.
		if m.byteFallback && len(sym) > 0 {
			count += len(sym)
			continue
		}
		count++
	}
	m.memoPut(piece, count)
	return count
}

// tokenIDs returns the exact token ids, for the oracle test's id-level comparison.
// It deliberately does not use the count memo: the memo stores numbers, and a
// count-neutral segmentation bug is exactly what the id comparison exists to find.
func (m *bpeModel) tokenIDs(text string) []int32 {
	if text == "" {
		return nil
	}
	normalized := m.normalize(text)
	if normalized == "" {
		return nil
	}
	pieces := []string{normalized}
	if m.flavor == bpeFlavorQwen {
		pieces = pretokenizeQwen(normalized)
	}
	var ids []int32
	for _, piece := range pieces {
		ids = append(ids, m.pieceIDs(piece)...)
	}
	return ids
}

// tokenNames returns the token text of every id, which is what the served
// tokenizer reports and what the oracle test compares.
func (m *bpeModel) tokenNames(text string) []string {
	ids := m.tokenIDs(text)
	if len(ids) == 0 {
		return nil
	}
	m.nameOnce.Do(func() {
		m.names = make(map[int32]string, len(m.vocab))
		for token, id := range m.vocab {
			m.names[id] = token
		}
	})
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, m.names[id])
	}
	return names
}

// pieceIDs is countPiece with the ids kept: same symbol construction, same merge
// loop, same byte fallback.
func (m *bpeModel) pieceIDs(piece string) []int32 {
	if piece == "" {
		return nil
	}
	var symbols []string
	if m.byteLevel {
		for i := 0; i < len(piece); i++ {
			symbols = append(symbols, string(byteToRune[piece[i]]))
		}
	} else {
		for _, r := range piece {
			symbols = append(symbols, string(r))
		}
	}
	if len(symbols) > 1 {
		symbols = m.merge(symbols)
	}
	ids := make([]int32, 0, len(symbols))
	for _, sym := range symbols {
		if id, ok := m.vocab[sym]; ok {
			ids = append(ids, id)
			continue
		}
		if m.byteFallback && len(sym) > 0 {
			if m.byteLevel {
				// A byte-level vocabulary contains every byte symbol, so this
				// branch is unreachable for the shipped byte-level models; map
				// back to the original byte if it ever is reached.
				if r, ok := singleRune(sym); ok {
					if b, ok := runeToByte[r]; ok {
						ids = append(ids, m.bytePieceID(b))
						continue
					}
				}
				ids = append(ids, m.unkID())
				continue
			}
			// Non-byte-level fallback: one <0xXX> token per UTF-8 byte.
			for i := 0; i < len(sym); i++ {
				ids = append(ids, m.bytePieceID(sym[i]))
			}
			continue
		}
		ids = append(ids, m.unkID())
	}
	return ids
}

// bytePieceID is the id of the "<0xXX>" token for one byte, or unk when the
// vocabulary has no byte-fallback token for it.
func (m *bpeModel) bytePieceID(b byte) int32 {
	if id, ok := m.vocab[fmt.Sprintf("<0x%02X>", b)]; ok {
		return id
	}
	return m.unkID()
}

// unkID is the id of the vocabulary's unknown token, or -1 when it has none.
func (m *bpeModel) unkID() int32 {
	if m.unk != "" {
		if id, ok := m.vocab[m.unk]; ok {
			return id
		}
	}
	return -1
}

// singleRune reports the only rune of s, when s holds exactly one.
func singleRune(s string) (rune, bool) {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 || size != len(s) {
		return 0, false
	}
	return r, true
}

// merge applies BPE merges in the canonical order: repeatedly take the
// lowest-rank adjacent pair and merge EVERY occurrence of it before moving to the
// next rank. Ranks are unique per pair, so "every occurrence of the lowest rank"
// is well defined.
//
// Two implementations were tried and rejected before this one, both caught by the
// oracle test:
//
//   - "merge one occurrence, then re-evaluate": lets a newly created pair be
//     merged before the remaining occurrences of the current pair, which changes
//     the segmentation. On a run of 32 'a's it returns 9 tokens where the model
//     returns 7.
//   - "rescan the whole unit for the lowest rank each round": correct but O(n^2)
//     — a 78 KB chunk (the Mistral flavour merges a whole chunk as one unit) took
//     nine seconds.
//
// This version keeps the symbols in a doubly linked list and the candidate pairs
// in a min-heap keyed by rank, and drains the heap per rank: O(n log n). Stale
// entries are detected with per-symbol version counters, and candidates that
// belong to a LOWER rank than the round in progress are deferred until the round
// ends, which is what makes the result identical to the batch-per-rank definition.
func (m *bpeModel) merge(symbols []string) []string {
	n := len(symbols)
	if n < 2 {
		return symbols
	}
	prev := make([]int32, n)
	next := make([]int32, n)
	version := make([]int32, n)
	alive := make([]bool, n)
	for i := 0; i < n; i++ {
		prev[i] = int32(i - 1)
		next[i] = int32(i + 1)
		alive[i] = true
	}
	next[n-1] = -1

	h := make(mergeHeap, 0, n)
	pushPair := func(left int32) {
		if left < 0 {
			return
		}
		right := next[left]
		if right < 0 {
			return
		}
		if rank, ok := m.merges[symbols[left]+" "+symbols[right]]; ok {
			heap.Push(&h, mergeEntry{rank: rank, left: left, right: right, leftVer: version[left], rightVer: version[right]})
		}
	}
	for i := int32(0); i < int32(n); i++ {
		pushPair(i)
	}
	heap.Init(&h)

	roundRank := -1
	var deferred []mergeEntry
	for h.Len() > 0 || len(deferred) > 0 {
		if h.Len() == 0 {
			// The heap drained while this round's cheaper-than-roundRank candidates
			// were still deferred, so nothing is left to trigger them. Re-queue them
			// instead of dropping them: a dropped candidate leaves the piece split
			// into more symbols than the merge table allows, which over-counts - the
			// direction that makes a provider reject a request. A training-consistent
			// table never reaches this state (a merged pair's rank is always higher
			// than the rank that created it); a hand-written or converted one can.
			for _, d := range deferred {
				heap.Push(&h, d)
			}
			deferred = deferred[:0]
			roundRank = -1
		}
		entry := heap.Pop(&h).(mergeEntry)
		if !alive[entry.left] || !alive[entry.right] ||
			version[entry.left] != entry.leftVer || version[entry.right] != entry.rightVer ||
			next[entry.left] != entry.right {
			continue
		}
		if roundRank >= 0 {
			if entry.rank > roundRank {
				// The current rank is exhausted; this candidate starts the next
				// round. Re-queue it along with anything deferred from this round.
				heap.Push(&h, entry)
				for _, d := range deferred {
					heap.Push(&h, d)
				}
				deferred = deferred[:0]
				roundRank = -1
				continue
			}
			if entry.rank < roundRank {
				// A merge created a pair that is cheaper than the round in
				// progress. The batch definition says the round finishes first.
				deferred = append(deferred, entry)
				continue
			}
		}
		roundRank = entry.rank

		// Merge right into left.
		symbols[entry.left] += symbols[entry.right]
		version[entry.left]++
		alive[entry.right] = false
		rightNext := next[entry.right]
		next[entry.left] = rightNext
		if rightNext >= 0 {
			prev[rightNext] = entry.left
		}
		pushPair(prev[entry.left])
		pushPair(entry.left)
	}

	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if alive[i] && prev[i] < 0 {
			for cur := int32(i); cur >= 0; cur = next[cur] {
				out = append(out, symbols[cur])
			}
			break
		}
	}
	if len(out) == 0 {
		// Defensive: never report zero tokens for non-empty symbols.
		return symbols
	}
	return out
}

// mergeEntry is one candidate merge of the adjacent symbols left|right, carrying
// the symbol versions it was computed from so staleness is detectable.
type mergeEntry struct {
	rank              int
	left, right       int32
	leftVer, rightVer int32
}

// mergeHeap is a min-heap over mergeEntry: by rank, then by position.
//
// The position tie-break is not cosmetic. A rank identifies a single pair, and
// every occurrence of it has that same rank, so the heap must hand them back
// leftmost-first: that is what the batch definition does (one left-to-right pass
// merging non-overlapping occurrences). Ordering by rank alone lets a middle
// occurrence be merged first, which then makes both of its neighbours
// unmergeable. On "aaaa" that yields [a, aa, a] instead of [aa, aa].
type mergeHeap []mergeEntry

func (h mergeHeap) Len() int { return len(h) }
func (h mergeHeap) Less(i, j int) bool {
	if h[i].rank != h[j].rank {
		return h[i].rank < h[j].rank
	}
	return h[i].left < h[j].left
}
func (h mergeHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *mergeHeap) Push(x any) { *h = append(*h, x.(mergeEntry)) }

func (h *mergeHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func (m *bpeModel) memoGet(piece string) (int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.memo[piece]
	return v, ok
}

func (m *bpeModel) memoPut(piece string, count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.memo) >= mergeCacheLimit {
		// Cheap eviction: the cache is an optimisation, not a correctness
		// structure, so clearing it beats tracking recency.
		m.memo = make(map[string]int, mergeCacheLimit)
	}
	m.memo[piece] = count
}

// trimToLimit returns the longest rune prefix with at most limit tokens.
func (m *bpeModel) trimToLimit(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if m.countTokens(text) <= limit {
		return text
	}
	runes := []rune(text)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if m.countTokens(string(runes[:mid])) <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo])
}

// ---------------------------------------------------------------------------
// Qwen pre-tokenization
// ---------------------------------------------------------------------------

// pretokenizeQwen reproduces the Split behavior (behavior="isolated") of the
// Qwen pre-tokenizer regex:
//
//	(?i:'s|'t|'re|'ve|'m|'ll|'d)
//	|[^\r\n\p{L}\p{N}]?\p{L}+
//	|\p{N}
//	| ?[^\s\p{L}\p{N}]+[\r\n]*
//	|\s*[\r\n]+
//	|\s+(?!\S)
//	|\s+
//
// Alternatives are tried in order at each position, exactly like the regex
// engine's leftmost-first match, and every match is its own piece.
func pretokenizeQwen(s string) []string {
	var out []string
	rs := []rune(s)
	i := 0
	for i < len(rs) {
		if piece, next, ok := matchQwenAlternative(rs, i); ok && next > i {
			out = append(out, piece)
			i = next
			continue
		}
		// No alternative matched (cannot happen for the pattern above, but be
		// safe): emit the character alone so counting never stalls.
		out = append(out, string(rs[i]))
		i++
	}
	return out
}

func matchQwenAlternative(rs []rune, i int) (string, int, bool) {
	// 1. contractions, case-insensitive
	for _, suffix := range []string{"'s", "'t", "'re", "'ve", "'m", "'ll", "'d"} {
		if hasRunePrefixFold(rs[i:], suffix) {
			return string(rs[i : i+len([]rune(suffix))]), i + len([]rune(suffix)), true
		}
	}
	// 2. [^\r\n\p{L}\p{N}]?\p{L}+
	{
		start := i
		if !isLetter(rs[start]) && rs[start] != '\r' && rs[start] != '\n' && !isNumber(rs[start]) {
			start++
		}
		if start < len(rs) && isLetter(rs[start]) {
			end := start
			for end < len(rs) && isLetter(rs[end]) {
				end++
			}
			return string(rs[i:end]), end, true
		}
	}
	// 3. \p{N} - exactly one digit
	if isNumber(rs[i]) {
		return string(rs[i]), i + 1, true
	}
	// 4. " ?[^\s\p{L}\p{N}]+[\r\n]*"
	{
		start := i
		if rs[start] == ' ' {
			start++
		}
		if start < len(rs) && !isSpace(rs[start]) && !isLetter(rs[start]) && !isNumber(rs[start]) {
			end := start
			for end < len(rs) && !isSpace(rs[end]) && !isLetter(rs[end]) && !isNumber(rs[end]) {
				end++
			}
			for end < len(rs) && (rs[end] == '\r' || rs[end] == '\n') {
				end++
			}
			return string(rs[i:end]), end, true
		}
	}
	// 5. \s*[\r\n]+ - whitespace ENDING in a newline. Greedy \s* then >=1
	// newline: the match therefore stops at the LAST newline of the run, leaving
	// any trailing spaces for the next alternative. That is why "a \n b" splits
	// as [" \n", " b"] and not as [" \n ", "b"].
	{
		end := i
		for end < len(rs) && isSpace(rs[end]) {
			end++
		}
		lastNewline := -1
		for k := i; k < end; k++ {
			if rs[k] == '\r' || rs[k] == '\n' {
				lastNewline = k
			}
		}
		if lastNewline >= 0 {
			return string(rs[i : lastNewline+1]), lastNewline + 1, true
		}
	}
	// 6/7. Whitespace runs without a newline.
	//
	// "\s+(?!\S)" comes first and, being backtracking, matches the whole run
	// MINUS its last character whenever something non-space follows: the
	// lookahead then sees the remaining space and succeeds. The leftover space is
	// picked up by the next alternative, which is how "a   b" becomes
	// ["a", "  ", " b"] - with " b" costing one token instead of two. Fall back to
	// the full run when "!"(\S) cannot hold, i.e. at the end of the text.
	{
		end := i
		for end < len(rs) && isSpace(rs[end]) {
			end++
		}
		if end > i {
			if end-i >= 2 && end < len(rs) && !isSpace(rs[end]) {
				return string(rs[i : end-1]), end - 1, true
			}
			return string(rs[i:end]), end, true
		}
	}
	return "", i, false
}

func hasRunePrefixFold(rs []rune, suffix string) bool {
	s := []rune(suffix)
	if len(rs) < len(s) {
		return false
	}
	for i, r := range s {
		if unicode.ToLower(rs[i]) != unicode.ToLower(r) {
			return false
		}
	}
	return true
}

func hasNewline(rs []rune) bool {
	for _, r := range rs {
		if r == '\r' || r == '\n' {
			return true
		}
	}
	return false
}

func isLetter(r rune) bool { return unicode.IsLetter(r) }
func isNumber(r rune) bool { return unicode.IsNumber(r) }
func isSpace(r rune) bool  { return unicode.IsSpace(r) }

// byteToRune is the GPT-2 byte -> unicode alphabet used by byte-level BPE.
var byteToRune = buildByteToRune()

// runeToByte is its inverse, needed to report the original byte when a byte-level
// symbol has no token of its own.
var runeToByte = buildRuneToByte()

func buildRuneToByte() map[rune]byte {
	m := make(map[rune]byte, len(byteToRune))
	for b, r := range byteToRune {
		m[r] = byte(b)
	}
	return m
}

func buildByteToRune() [256]rune {
	var bs, cs []int
	for i := '!'; i <= '~'; i++ {
		bs = append(bs, int(i))
	}
	for i := 0xA1; i <= 0xAC; i++ {
		bs = append(bs, int(i))
	}
	for i := 0xAE; i <= 0xFF; i++ {
		bs = append(bs, int(i))
	}
	cs = append(cs, bs...)
	extra := 0
	for b := 0; b < 256; b++ {
		if containsInt(bs, b) {
			continue
		}
		bs = append(bs, b)
		cs = append(cs, 256+extra)
		extra++
	}
	var table [256]rune
	for i := range bs {
		table[bs[i]] = rune(cs[i])
	}
	return table
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Counters
// ---------------------------------------------------------------------------

type bpeCounter struct {
	model     *bpeModel
	path      string
	counterID string
}

func (c *bpeCounter) ID() string { return c.counterID }

// IDs reports the token ids, for the oracle test's id-level comparison.
func (c *bpeCounter) IDs(text string) []int32 { return c.model.tokenIDs(text) }

// Names reports the token text of every token, which the oracle test compares
// directly with the served tokenizer's tokens.
func (c *bpeCounter) Names(text string) []string { return c.model.tokenNames(text) }

// SourcePath is the tokenizer.json this counter loaded.
func (c *bpeCounter) SourcePath() string { return c.path }

func (c *bpeCounter) Count(text string) int { return c.model.countTokens(text) }

func (c *bpeCounter) TrimToLimit(text string, limit int) string {
	return c.model.trimToLimit(text, limit)
}

func (c *bpeCounter) Available() bool { return c.model != nil }

// Pinned digests of the shipped tokenizer.json assets.
var expectedBPEHashes = map[string]string{
	"Qwen/Qwen3-Embedding-0.6B/tokenizer.json":       "e6592f4d8d1678e963da9188090ac3eee916ed6b",
	"intfloat/e5-mistral-7b-instruct/tokenizer.json": "92cb22e16e9be5e5d24851a89b3f912cbfcda26b",
}

const (
	llamaBPEAssetPinKey = "intfloat/e5-mistral-7b-instruct/tokenizer.json"
	qwenBPEAssetPinKey  = "Qwen/Qwen3-Embedding-0.6B/tokenizer.json"
)

var llamaBPEAssetNames = []string{
	"ragflow_deps/huggingface.co/intfloat/e5-mistral-7b-instruct/tokenizer.json",
	"llama_bpe_tokenizer.json",
}

var qwenBPEAssetNames = []string{
	"ragflow_deps/huggingface.co/Qwen/Qwen3-Embedding-0.6B/tokenizer.json",
	"qwen_bpe_tokenizer.json",
}

var (
	llamaBPEOnce struct {
		sync.Once
		counter Counter
		err     error
	}
	qwenBPEOnce struct {
		sync.Once
		counter Counter
		err     error
	}
)

// LoadLlamaBPECounter loads (once) the Llama/Mistral BPE counter.
func LoadLlamaBPECounter() (Counter, error) {
	llamaBPEOnce.Do(func() {
		llamaBPEOnce.counter, llamaBPEOnce.err = loadBPECounter(llamaBPEAssetNames, llamaBPEAssetPinKey, bpeFlavorLlama, CounterLlamaBPE)
	})
	return llamaBPEOnce.counter, llamaBPEOnce.err
}

// LoadQwenBPECounter loads (once) the Qwen byte-level BPE counter.
func LoadQwenBPECounter() (Counter, error) {
	qwenBPEOnce.Do(func() {
		qwenBPEOnce.counter, qwenBPEOnce.err = loadBPECounter(qwenBPEAssetNames, qwenBPEAssetPinKey, bpeFlavorQwen, CounterQwenBPE)
	})
	return qwenBPEOnce.counter, qwenBPEOnce.err
}

func loadBPECounter(names []string, pinKey string, flavor bpeFlavor, counterID string) (Counter, error) {
	path, _, err := readTokenizerAsset(names, pinKey, expectedBPEHashes)
	if err != nil {
		return nil, err
	}
	model, err := loadBPEModel(path, flavor)
	if err != nil {
		return nil, err
	}
	return &bpeCounter{model: model, path: path, counterID: counterID}, nil
}

func init() {
	RegisterCounterLoader(CounterLlamaBPE, LoadLlamaBPECounter)
	RegisterCounterLoader(CounterQwenBPE, LoadQwenBPECounter)
}
