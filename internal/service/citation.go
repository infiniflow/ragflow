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

package service

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// sentenceSplitRE splits text on Chinese / English / Arabic sentence-ending
// punctuation.  Matches the Python regex in rag/nlp/search.py:insert_citations.
var sentenceSplitRE = regexp.MustCompile(`([^\|][；。？!！,؛؟.\n]|[a-z؀-ۿ][.?;!،؛؟][ \n])`)

const minSentenceLen = 5

// Embedder abstracts embedding-model access so InsertCitations is testable.
type Embedder interface {
	Encode(ctx context.Context, texts []string) ([][]float64, error)
}

// CitationMarkerPattern matches "[ID:N]" or bare "[N]" with Arabic digit support,
// allowing optional whitespace after "ID:" (e.g. "[ID: 12]").
var CitationMarkerPattern = regexp.MustCompile(`\[(?:ID:\s*)?([0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+)\]`)

// badCitationPatterns match malformed citation shapes that LLMs sometimes emit
var badCitationPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\(\s*ID\s*[:： ]*\s*([0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+)\s*\)`), // (ID: 12)
	regexp.MustCompile(`\[\s*ID\s*[:： ]*\s*([0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+)\s*\]`), // [ID: 12]
	regexp.MustCompile(`【\s*ID\s*[:： ]*\s*([0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+)\s*】`),   // 【ID: 12】
	regexp.MustCompile(`(?i)\bref\s*([0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+)\b`),           // ref12
	// (**ID:5**) / (*ID: 5*) — markdown-asterisk-wrapped parenthetical cites that
	// models sometimes emit instead of [ID:5]. Mirrors Python agentic_rag.py:885
	// `re.sub(r"\(\**(ID:\d+)\**\)", r"[\1]", ...)`, which deep-research runs over
	// its final answer before it reaches the UI.
	regexp.MustCompile(`\(\s*\**\s*ID\s*[:： ]*\s*([0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+)\s*\**\s*\)`), // (**ID: 12**)
}

// InsertCitations decorates answer with [ID:n] citation markers.
//
// Algorithm mirrors Python Dealer.insert_citations:
//  1. Split into sentences, preserving ``` code blocks.
//  2. Drop sentences shorter than minSentenceLen.
//  3. Encode sentences → sentence vectors.
//  4. Compute cosine similarity between each sentence and each chunk vector.
//  5. Threshold descent (0.63 → 0.3, ×0.8 per round): find chunks where
//     similarity > max*0.99.  Up to 4 chunks per sentence.
//  6. Rebuild answer text with [ID:n] markers inserted after cited sentences,
//     where n is the cited chunk's position in chunks (see applyCitations).
//
// Returns the decorated answer and the set of cited chunk indices.
func InsertCitations(ctx context.Context, answer string, chunks []SourcedChunk, embedder Embedder, chunkVectors [][]float64) (string, []int) {
	sentences, sentenceIdx := splitAnswer(answer)
	if len(sentences) == 0 || len(chunks) == 0 || len(chunkVectors) == 0 {
		return answer, nil
	}

	sentenceVecs, err := embedder.Encode(ctx, sentences)
	if err != nil || len(sentenceVecs) == 0 {
		return answer, nil
	}

	return InsertCitationsWithVectors(answer, chunks, sentenceVecs, chunkVectors, sentences, sentenceIdx)
}

// InsertCitationsWithVectors is the pure core: pre-split sentences, pre-encoded
// vectors.  Separated from the encoding step for testability.
func InsertCitationsWithVectors(
	answer string,
	chunks []SourcedChunk,
	sentenceVecs, chunkVectors [][]float64,
	sentences []string,
	sentenceIdx []int,
) (string, []int) {
	if len(sentences) != len(sentenceVecs) {
		n := len(sentenceVecs)
		if n < len(sentences) {
			sentences = sentences[:n]
			sentenceIdx = sentenceIdx[:n]
		}
	}

	sim := cosineSimMatrix(sentenceVecs, chunkVectors)
	cites := findCitations(sim)

	return applyCitations(answer, sentences, sentenceIdx, cites, chunks)
}

// splitAnswer splits answer text into sentences, preserving ``` code blocks.
func splitAnswer(answer string) ([]string, []int) {
	blocks := strings.Split(answer, "```")
	var rawPieces []string
	for i, block := range blocks {
		if i%2 == 1 {
			// Code block — keep intact, won't receive citations.
			rawPieces = append(rawPieces, "```"+block+"```\n")
		} else {
			// Regular text — split on sentence boundaries.
			rawPieces = append(rawPieces, sentenceSplit(block)...)
		}
	}
	// Rejoin the trailing punctuation that the regex captured as a separate piece.
	for i := 1; i < len(rawPieces); i++ {
		if sentenceSplitRE.MatchString(rawPieces[i]) {
			r := []rune(rawPieces[i])
			rawPieces[i-1] += string(r[0])
			rawPieces[i] = string(r[1:])
		}
	}
	// Filter out short pieces.
	var sentences []string
	var sentenceIdx []int
	for i, t := range rawPieces {
		if len(strings.TrimSpace(t)) >= minSentenceLen {
			sentences = append(sentences, t)
			sentenceIdx = append(sentenceIdx, i)
		}
	}
	return sentences, sentenceIdx
}

func sentenceSplit(text string) []string {
	indices := sentenceSplitRE.FindAllStringIndex(text, -1)
	if len(indices) == 0 {
		return []string{text}
	}
	var result []string
	prev := 0
	for _, idx := range indices {
		result = append(result, text[prev:idx[1]])
		prev = idx[1]
	}
	if prev < len(text) {
		result = append(result, text[prev:])
	}
	return result
}

// applyCitations rebuilds the answer text with [ID:n] markers inserted after
// each cited sentence position.
//
// n is the cited chunk's POSITION in chunks, not its chunk id: the caller returns
// that same list as the reference, and the client resolves a marker by indexing it
// with the number the marker carries. A chunk id (opaque text, only digits parse)
// would come back as a dead marker on both consumers.
func applyCitations(answer string, sentences []string, sentenceIdx []int, cites map[int][]int, chunks []SourcedChunk) (string, []int) {
	blocks := strings.Split(answer, "```")
	var rawPieces []string
	for i, block := range blocks {
		if i%2 == 1 {
			rawPieces = append(rawPieces, "```"+block+"```\n")
		} else {
			rawPieces = append(rawPieces, sentenceSplit(block)...)
		}
	}
	for i := 1; i < len(rawPieces); i++ {
		if sentenceSplitRE.MatchString(rawPieces[i]) {
			r := []rune(rawPieces[i])
			rawPieces[i-1] += string(r[0])
			rawPieces[i] = string(r[1:])
		}
	}

	// Map sentence position → chunk IDs to insert.
	citedChunks := make(map[int]string)
	seenChunks := make(map[int]bool)
	var citedIndices []int
	for i, rawIdx := range sentenceIdx {
		if chunkIdxs, ok := cites[i]; ok {
			var markers []string
			for _, ci := range chunkIdxs {
				if ci < len(chunks) && !seenChunks[ci] {
					seenChunks[ci] = true
					markers = append(markers, " [ID:"+strconv.Itoa(ci)+"]")
					citedIndices = append(citedIndices, ci)
				}
			}
			citedChunks[rawIdx] = strings.Join(markers, "")
		}
	}

	var b strings.Builder
	for i, p := range rawPieces {
		b.WriteString(p)
		if markers, ok := citedChunks[i]; ok {
			b.WriteString(markers)
		}
	}
	return b.String(), citedIndices
}

// ---- Pure computation helpers ----

func cosineSimMatrix(a, b [][]float64) [][]float64 {
	m := make([][]float64, len(a))
	for i := range a {
		m[i] = make([]float64, len(b))
		na := vecNorm(a[i])
		if na == 0 {
			continue
		}
		for j := range b {
			nb := vecNorm(b[j])
			if nb == 0 {
				continue
			}
			m[i][j] = dot(a[i], b[j]) / (na * nb)
		}
	}
	return m
}

func vecNorm(v []float64) float64 {
	var s float64
	for _, x := range v {
		s += x * x
	}
	return math.Sqrt(s)
}

func dot(a, b []float64) float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var s float64
	for i := 0; i < n; i++ {
		s += a[i] * b[i]
	}
	return s
}

func findCitations(sim [][]float64) map[int][]int {
	cites := make(map[int][]int)
	thr := 0.63
	for thr > 0.3 && len(cites) == 0 {
		for i := range sim {
			mx := maxRow(sim[i]) * 0.99
			if mx < thr {
				continue
			}
			var matches []int
			for j, s := range sim[i] {
				if s > mx {
					matches = append(matches, j)
				}
			}
			if len(matches) > 4 {
				matches = matches[:4]
			}
			if len(matches) > 0 {
				cites[i] = matches
			}
		}
		thr *= 0.8
	}
	return cites
}

func maxRow(row []float64) float64 {
	if len(row) == 0 {
		return 0
	}
	mx := row[0]
	for _, v := range row[1:] {
		if v > mx {
			mx = v
		}
	}
	return mx
}

// normalizeArabicDigits converts Arabic-Indic (U+0660-0669) and
// Eastern Arabic-Indic (U+06F0-06F9) digits to ASCII.
func normalizeArabicDigits(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 0x0660 && r <= 0x0669:
			b.WriteRune(r - 0x0660 + '0')
		case r >= 0x06F0 && r <= 0x06F9:
			b.WriteRune(r - 0x06F0 + '0')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HasCitationMarkers reports whether answer already contains canonical citation markers.
func HasCitationMarkers(answer string) bool {
	if answer == "" {
		return false
	}
	return CitationMarkerPattern.MatchString(normalizeArabicDigits(answer))
}

// ExtractCitationMarkers returns chunk indices from citation markers within [0, maxIndex).
// Preserves first-seen order, no duplicates.
func ExtractCitationMarkers(answer string, maxIndex int) []int {
	if answer == "" || maxIndex <= 0 {
		return nil
	}
	seen := make(map[int]struct{})
	var out []int
	for _, m := range CitationMarkerPattern.FindAllStringSubmatch(normalizeArabicDigits(answer), -1) {
		if len(m) < 2 {
			continue
		}
		var n int
		for _, r := range m[1] {
			if r < '0' || r > '9' {
				n = 0
				break
			}
			n = n*10 + int(r-'0')
		}
		if n < 0 || n >= maxIndex {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// RepairBadCitationFormats rewrites bad citation shapes into canonical "[ID:N]" form
func RepairBadCitationFormats(answer string) string {
	if answer == "" {
		return answer
	}
	working := answer
	for _, pat := range badCitationPatterns {
		matches := pat.FindAllStringSubmatchIndex(working, -1)
		if len(matches) == 0 {
			continue
		}
		var b strings.Builder
		b.Grow(len(working))
		last := 0
		for _, m := range matches {
			b.WriteString(working[last:m[0]])
			digits := normalizeArabicDigits(working[m[2]:m[3]])
			b.WriteString("[ID:")
			b.WriteString(digits)
			b.WriteString("]")
			last = m[1]
		}
		b.WriteString(working[last:])
		working = b.String()
	}
	return working
}

// slotCitationPattern matches slot-table citations that leak into the final
// answer. The compose prompt's Research Summary renders the slot draft
// ("- slot 0 [entity]: ..."), and models occasionally cite those internal
// lines with the citation format instead of the evidence blocks:
// "[ID:Slot 0]", "[ID: slot 0]", "[Slot 0]". Such markers index the internal
// slot table — no chunk the user can open — so they must be rewritten into
// citations of the chunk the slot was filled from, or dropped.
var slotCitationPattern = regexp.MustCompile(`(?i)\[\s*(?:ID\s*[:： ]*\s*)?slot\s*([0-9]+)\s*\]`)

// RepairSlotCitations rewrites leaked slot-table citations into locatable
// chunk citations. slotCitations maps a slot-table id ("0", "1", ...) to the
// evidence chunk ids that produced the slot's candidate; chunks is the ordered
// evidence list the compose rendered for the model, whose block ids are 0-based
// ("[ID:N]" where N is the block's index in that list — the index the client
// resolves against the reference). A marker whose slot has no evidence chunk in
// the list is dropped outright — an internal marker the user cannot resolve must
// not reach the answer. Python parity note: the same leak exists upstream
// (agentic_rag.py:885 only repairs "(ID:5)" shapes; the pre_summary slot
// draft reaches Python's compose prompt too), so this repair is a Go-side
// decoration-layer fix under the "every citation must locate a chunk" rule.
func RepairSlotCitations(answer string, slotCitations map[string][]string, chunks []map[string]interface{}) string {
	if answer == "" || len(slotCitations) == 0 || !slotCitationPattern.MatchString(answer) {
		return answer
	}
	posByID := make(map[string]int, len(chunks))
	for i, c := range chunks {
		if id := chunkCitationID(c); id != "" {
			if _, dup := posByID[id]; !dup {
				posByID[id] = i
			}
		}
	}
	var b strings.Builder
	b.Grow(len(answer))
	last := 0
	for _, m := range slotCitationPattern.FindAllStringSubmatchIndex(answer, -1) {
		slotID := answer[m[2]:m[3]]
		repl := ""
		for _, eid := range slotCitations[slotID] {
			if pos, ok := posByID[eid]; ok {
				repl = "[ID:" + strconv.Itoa(pos) + "]"
				break
			}
		}
		b.WriteString(answer[last:m[0]])
		b.WriteString(repl)
		last = m[1]
	}
	b.WriteString(answer[last:])
	return b.String()
}

// chunkCitationID returns a pool chunk's id under the keys the harness uses
// (chunk_id, then id — harness.ChunkIDOf's order).
func chunkCitationID(c map[string]interface{}) string {
	for _, k := range []string{"chunk_id", "id"} {
		if s, ok := c[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// rangeCitationPattern matches range-merged citations: "[ID:1-3]", "[ID: 1 - 3]",
// "[ID:1~3]". Models sometimes compress consecutive individual citations
// ([ID:1][ID:2][ID:3]) into such a range on their own — no code path produces
// it, and no chunk resolves through it, so it must be normalized defensively.
var rangeCitationPattern = regexp.MustCompile(`(?i)\[\s*ID\s*[:： ]*\s*([0-9]+)\s*[-–—~～]\s*([0-9]+)\s*\]`)

// ExpandRangeCitations expands range-merged citations back into individual
// ones: [ID:1-3] → [ID:1][ID:2][ID:3]. A range expands only when BOTH bounds
// name valid evidence positions (0-based, bounded by poolSize — the numbering
// the compose renders and the client indexes); reversed bounds are swapped
// first. Anything out of range is dropped outright — an unresolvable marker must
// not reach the user's answer, matching the slot-citation repair above. poolSize
// is the length of the ordered evidence list the compose rendered
// (harness.Kbinfos.CiteChunkIDs).
func ExpandRangeCitations(answer string, poolSize int) string {
	if answer == "" || !rangeCitationPattern.MatchString(answer) {
		return answer
	}
	var b strings.Builder
	b.Grow(len(answer) * 2)
	last := 0
	for _, m := range rangeCitationPattern.FindAllStringSubmatchIndex(answer, -1) {
		a, _ := strconv.Atoi(normalizeArabicDigits(answer[m[2]:m[3]]))
		c, _ := strconv.Atoi(normalizeArabicDigits(answer[m[4]:m[5]]))
		if a > c {
			a, c = c, a
		}
		b.WriteString(answer[last:m[0]])
		if a >= 0 && c < poolSize {
			for i := a; i <= c; i++ {
				b.WriteString("[ID:" + strconv.Itoa(i) + "]")
			}
		}
		last = m[1]
	}
	b.WriteString(answer[last:])
	return b.String()
}

// Keep an unfinished citation until its closing delimiter arrives. Retaining
// the trailing word also preserves the word boundaries used by refN markers.
var citationStreamTailPattern = regexp.MustCompile(`(?i)[\[(【][\s*idslot:：0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}–—~～-]*$|#{1,2}(?:[0-9]+\$?)?$|\bref\s*[0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]*$|\w+$`)

func stripCitations(text string) string {
	text = cleanCitationMarkers(text)
	text = canonicalIDMarkerPattern.ReplaceAllString(text, "")
	text = slotCitationPattern.ReplaceAllString(text, "")
	text = rangeCitationPattern.ReplaceAllString(text, "")
	for _, pattern := range badCitationPatterns {
		text = pattern.ReplaceAllString(text, "")
	}
	return text
}

type citationStreamFilter struct {
	pending string
}

func (f *citationStreamFilter) write(delta string) string {
	f.pending += delta
	end := len(f.pending)
	if tail := citationStreamTailPattern.FindStringIndex(f.pending); tail != nil {
		end = tail[0]
	}
	text := stripCitations(f.pending[:end])
	f.pending = f.pending[end:]
	return text
}

func (f *citationStreamFilter) flush() string {
	text := stripCitations(f.pending)
	f.pending = ""
	return text
}

// ResolveCitationMarkers drops the answer's canonical [ID:n] markers that name
// nothing and returns the pool positions of the markers that do resolve.
//
// citeIdx maps a rendered evidence position to its chunk's index in the pool the
// reference is built from (see citePoolIdx); -1 is a block whose chunk could not
// be located. The rendered blocks are numbered 0-based, so a marker's number IS
// the client's index into the reference — resolvable markers therefore keep the
// number the model wrote.
//
// Only a marker carrying the literal "ID:" prefix is removed when it cannot be
// resolved: the compose's citation rules prescribe that form, while a bare "[5]"
// in prose is as likely to be a footnote, a version or a year, and deleting
// user-visible text is worse than leaving a marker the client cannot open.
//
// The returned positions are pool indexes, de-duplicated, in first-seen order.
func ResolveCitationMarkers(answer string, citeIdx []int) (string, []int) {
	if answer == "" || len(citeIdx) == 0 {
		return answer, nil
	}
	var b strings.Builder
	b.Grow(len(answer))
	last := 0
	seen := make(map[int]struct{})
	var out []int
	for _, m := range CitationMarkerPattern.FindAllStringSubmatchIndex(answer, -1) {
		n, ok := markerNumber(answer[m[2]:m[3]])
		resolved := -1
		if ok && n >= 0 && n < len(citeIdx) {
			resolved = citeIdx[n]
		}
		if resolved >= 0 {
			if _, dup := seen[resolved]; !dup {
				seen[resolved] = struct{}{}
				out = append(out, resolved)
			}
			continue
		}
		if canonicalIDMarkerPattern.MatchString(answer[m[0]:m[1]]) {
			// A canonical citation that names no rendered block: the user could
			// never open it, so it must not reach the answer.
			b.WriteString(answer[last:m[0]])
			last = m[1]
		}
	}
	b.WriteString(answer[last:])
	return b.String(), out
}

// canonicalIDMarkerPattern matches a citation marker that carries the literal
// "ID:" prefix — the form the citation rules prescribe, and the only one that can
// be removed from the answer without risking ordinary text like "[2024]".
var canonicalIDMarkerPattern = regexp.MustCompile(
	`(?i)\[\s*ID\s*[:： ]*\s*[0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+\s*\]`)

// rawCitationMarkers returns the citation numbers the model wrote, in order and
// including duplicates — the repetition is what the citation observability log in
// decorateHarnessAnswer looks at — capped at limit entries. They are read before
// any repair, so they are what the model actually emitted.
func rawCitationMarkers(answer string, limit int) []int {
	if answer == "" || limit <= 0 {
		return nil
	}
	out := make([]int, 0, limit)
	for _, m := range CitationMarkerPattern.FindAllStringSubmatch(answer, -1) {
		if len(m) < 2 {
			continue
		}
		n, ok := markerNumber(m[1])
		if !ok {
			continue
		}
		out = append(out, n)
		if len(out) >= limit {
			break
		}
	}
	return out
}

// notFoundPhrases are the shipped system-prompt lines that instruct the model to
// answer "no answer in the knowledge base" verbatim (web/src/locales/zh.ts,
// systemInitialValue / emptyResponsePlaceholder). They are the fallback signal
// when the dialog configures no empty_response of its own.
var notFoundPhrases = []string{
	"知识库中未找到您要的答案",
	"在知识库中未找到您要寻找的答案",
}

// reportsNoAnswer reports whether the answer only announces that the knowledge
// base holds no answer. Such an answer cites nothing, so it must not carry
// citation markers or a document reference.
//
// The dialog's configured empty_response is the primary signal; when it is unset
// the shipped not-found lines above are the fallback. Both sides are stripped of
// citation markers and whitespace before matching, so a marker the model injected
// mid-sentence ("因 [ID:3]此") does not hide the phrase.
func reportsNoAnswer(answer, emptyResponse string) bool {
	flat := flattenForMatch(stripCitations(answer))
	if flat == "" {
		return false
	}
	if phrase := flattenForMatch(emptyResponse); phrase != "" && strings.Contains(flat, phrase) {
		return true
	}
	for _, phrase := range notFoundPhrases {
		if strings.Contains(flat, phrase) {
			return true
		}
	}
	return false
}

// decorateQuote reports whether the answer should be decorated with its citation
// markers and document reference. An answer that only reports the knowledge base
// holds no answer is decorated as if quoting were off — otherwise the markers and
// the document list present sources for a reply that cited nothing.
func decorateQuote(quote bool, answer, emptyResponse string) bool {
	return quote && !reportsNoAnswer(answer, emptyResponse)
}

// flattenForMatch drops all whitespace so a phrase match survives line breaks and
// the spaces a citation marker leaves behind when it is stripped.
func flattenForMatch(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	for _, r := range text {
		switch r {
		case ' ', '\t', '\n', '\r', '\v', '\f', '\u00a0', '\u3000':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// markerNumber parses a citation marker's digits (Arabic-Indic digits accepted)
// into a position. ok is false for an empty or non-numeric capture.
func markerNumber(digits string) (int, bool) {
	s := normalizeArabicDigits(digits)
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
		if n > 1<<30 {
			return 0, false
		}
	}
	return n, true
}
