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

package harness

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// In-memory grep+sed narrowing engine (term-driven, zero extra LLM rounds).
//
// Mirrors Python harness/grep_sed_narrow.py.
//
// It mirrors the Claude Code / Codex ``grep`` + ``sed`` workflow over chunks held in
// memory: terms already produced by the main-analysis LLM (entities, numbers, key
// phrases) become word-boundary regexes for locating (grep), and string transforms
// narrow the text (sed), dropping boilerplate instead of crude head-truncation.
//
// Fallback chain (never drops the answer):
//
//	narrow_by_terms (grep locate + sed transform)
//	  -> no hits / no terms
//	  -> _narrow_by_keywords (keyword sentence-level, zero LLM)
//	  -> original chunks returned as-is (caller's head-truncation as the final
//	     safety valve)
//
// Safety: regexp + pure string ops only, no eval; term count and context are capped.

// Cost / safety caps. Mirrors grep_sed_narrow.py's module constants.
const (
	// maxGrepTerms caps the terms compiled (Python _MAX_GREP_TERMS).
	maxGrepTerms = 16
	// maxContext is the +/- window of lines kept around a hit (Python _MAX_CONTEXT).
	maxContext = 2
	// defaultOutCharsPerChunk caps output per chunk (Python _DEFAULT_OUT_CHARS_PER_CHUNK).
	defaultOutCharsPerChunk = 1200
	// defaultOutTotalChars caps the total narrowed output (Python _DEFAULT_OUT_TOTAL_CHARS).
	defaultOutTotalChars = 16000
	// headFallbackChars is the head kept per chunk when there is no match
	// (Python _HEAD_FALLBACK_CHARS).
	headFallbackChars = 400
	// contextCharBudget is the absolute per-side char budget during context
	// expansion (Python _CONTEXT_CHAR_BUDGET).
	contextCharBudget = 600
	// minNarrowChars: chunks at or below this length are NOT narrowed — they are
	// already 1-2 lines, and answers often live in short chunks (Python _MIN_NARROW_CHARS).
	minNarrowChars = 200
)

var (
	// reTermEdgePunct strips leading/trailing punctuation from a term (Python
	// `re.sub(r"^[\s.,:;!?'\"()\[\]{}]+|...$", "", t)`).
	reTermEdgePunct = regexp.MustCompile(`^[\s.,:;!?'"()\[\]{}]+|[\s.,:;!?'"()\[\]{}]+$`)
	// reCJKTerm detects CJK / kana / hangul, which must NOT be wrapped in \b —
	// a word boundary never matches between CJK characters.
	reCJKTerm = regexp.MustCompile(`[\p{Han}\p{Hiragana}\p{Katakana}\p{Hangul}]`)
)

// EscapeTerm escapes a plain grep term into a safe, word-boundary regex fragment.
// Mirrors Python _escape_term: numbers/entities matched literally; 3+ char terms
// with alphanumeric edges get \b; CJK terms stay bare (never wrapped in \b).
func EscapeTerm(term string) string {
	t := strings.TrimSpace(term)
	if t == "" {
		return ""
	}
	t = reTermEdgePunct.ReplaceAllString(t, "")
	if t == "" {
		return ""
	}
	escaped := regexp.QuoteMeta(t)
	if reCJKTerm.MatchString(t) {
		return escaped
	}
	rs := []rune(t)
	if len(rs) >= 3 && isAlphaNumRune(rs[0]) && isAlphaNumRune(rs[len(rs)-1]) {
		return `\b` + escaped + `\b`
	}
	return escaped
}

func isAlphaNumRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// TermsToPatterns turns grep terms into a list of compiled regexes (one per
// term), capped at maxGrepTerms. Mirrors Python _terms_to_patterns.
func TermsToPatterns(terms []string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(terms))
	for _, term := range terms {
		if len(out) >= maxGrepTerms {
			break
		}
		frag := EscapeTerm(term)
		if frag == "" {
			continue
		}
		p, err := regexp.Compile("(?i)" + frag)
		if err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

// GrepTermsMax caps the terms extracted from a query (Python _GREP_TERMS_MAX).
const GrepTermsMax = 10

// GrepOutCharsPerChunk is the grep narrow's per-chunk output cap (Python
// _GREP_OUT_CHARS_PER_CHUNK).
const GrepOutCharsPerChunk = 700

// GrepOutTotalChars is the grep narrow's total output cap (Python
// _GREP_OUT_TOTAL_CHARS).
const GrepOutTotalChars = 8000

// GrepTermsFromQuery mirrors Python _grep_terms_from_query:
// extract compact grep terms from a query — bare alnum words of length>=2,
// deduped (order-preserving) and capped. Numbers/ids are preserved as-is.
func GrepTermsFromQuery(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	terms := make([]string, 0, GrepTermsMax)
	seen := make(map[string]struct{}, GrepTermsMax)
	re := regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9_.\-]{1,}`)
	for _, m := range re.FindAllString(q, -1) {
		t := strings.Trim(m, "._-")
		if len(t) < 2 {
			continue
		}
		low := strings.ToLower(t)
		if _, ok := seen[low]; ok {
			continue
		}
		seen[low] = struct{}{}
		terms = append(terms, t)
		if len(terms) >= GrepTermsMax {
			break
		}
	}
	return terms
}

// lineSpans returns line (start,end) spans, boundaries at "\n" (grep semantics).
// Mirrors Python _line_spans: line boundaries are exact (unlike lossy sentence
// splitting); start of line i is after the i-th "\n".
func lineSpans(content string) [][2]int {
	var spans [][2]int
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			spans = append(spans, [2]int{start, i})
			start = i + 1
		}
	}
	if start <= len(content) {
		spans = append(spans, [2]int{start, len(content)})
	}
	if len(spans) == 0 {
		spans = [][2]int{{0, len(content)}}
	}
	return spans
}

// NarrowContext is the +/- line-context window greed by term-grep (Python
// narrow_by_terms' `context` dict).
type NarrowContext struct {
	Before int
	After  int
}

// execOnText runs term-grep + line-context expansion against one chunk's text.
// Mirrors Python _exec_on_text:
//
//   - locate matches exactly (match.start()/end());
//   - merge overlapping/adjacent ranges, expand to whole lines, add before/after
//     context lines, with a per-side character-budget clamp;
//   - on no hit, keep fact-dense sentences (never drop everything), else head.
//
// Returns (narrowed, matched).
func execOnText(content string, patterns []*regexp.Regexp, before, after, outCharsPerChunk int) (string, bool) {
	if content == "" {
		return "", false
	}

	// Step 1: locate matches (exact positions from the regex engine).
	var hitRanges [][2]int
	for _, p := range patterns {
		locs := p.FindAllStringIndex(content, -1)
		for _, loc := range locs {
			hitRanges = append(hitRanges, [2]int{loc[0], loc[1]})
		}
	}
	if len(hitRanges) == 0 {
		// Keep fact-dense sentences to avoid dropping numbers/entities (Python's
		// no-hit path keeps dense sentences, else the raw head).
		var kept []string
		for _, s := range SplitSentences(content) {
			if IsFactDenseSentence(s) {
				kept = append(kept, s)
			}
		}
		narrowed := strings.TrimSpace(strings.Join(kept, ""))
		if narrowed != "" {
			return truncHead(narrowed, headFallbackChars*4), false
		}
		return truncHead(content, headFallbackChars), false
	}

	// Step 2: merge overlapping/adjacent matches.
	sort2DRanges(hitRanges)
	merged := make([][2]int, 0, len(hitRanges))
	for _, r := range hitRanges {
		if n := len(merged); n > 0 && r[0] <= merged[n-1][1] {
			if r[1] > merged[n-1][1] {
				merged[n-1][1] = r[1]
			}
		} else {
			merged = append(merged, r)
		}
	}

	lines := lineSpans(content)
	var expanded [][2]int
	for _, r := range merged {
		lo, hi := 0, 0
		for i, ls := range lines {
			if r[0] >= ls[0] && r[0] < ls[1] {
				lo = i
			}
			if r[1] > ls[0] && r[1] <= ls[1] {
				hi = i
			}
		}
		lo = max(0, lo-before)
		hi = min(len(lines)-1, hi+after)
		fragS, fragE := lines[lo][0], lines[hi][1]
		// Per-side character budget fallback.
		if fragE-fragS > contextCharBudget*2 && (fragE-fragS) > (r[1]-r[0]) {
			fragS = max(0, r[0]-contextCharBudget)
			fragE = min(len(content), r[1]+contextCharBudget)
		}
		expanded = append(expanded, [2]int{fragS, fragE})
	}

	// Step 3: dedupe, join, truncate.
	seen := map[string]bool{}
	var outParts []string
	for _, e := range expanded {
		p := strings.TrimSpace(content[e[0]:e[1]])
		if p == "" {
			continue
		}
		key := truncHead(p, 200)
		if seen[key] {
			continue
		}
		seen[key] = true
		outParts = append(outParts, p)
	}
	narrowed := strings.TrimSpace(strings.Join(outParts, "\n\n"))
	if charLen(narrowed) > outCharsPerChunk {
		narrowed = truncHead(narrowed, outCharsPerChunk)
	}
	if narrowed == "" {
		return truncHead(content, headFallbackChars), true
	}
	return narrowed, true
}

// NarrowStats carries the accounting Python narrow_by_terms returns in its
// "stats" dict.
type NarrowStats struct {
	ChunksIn  int
	ChunksKpt int
	CharsIn   int
	CharsOut  int
	Matched   bool
	UsedTerms int
}

// NarrowResult is the outcome of NarrowByTerms / GrepSedNarrow, mirroring
// Python's {"kept": [...], "stats": {...}} dict.
type NarrowResult struct {
	Kept  []map[string]any
	Stats NarrowStats
}

// NarrowByTerms narrows retrieval chunks by locating grep terms, mirroring
// Python narrow_by_terms:
//
//   - terms are plain strings (entities / numbers / key phrases);
//   - no usable terms -> keyword narrowing (zero LLM);
//   - primary terms hit nothing -> fallbackTerms tried once mechanically;
//   - still no hit -> narrowing abandoned, originals returned (matched=false);
//   - when matched, a total-length budget is distributed across chunks.
//
// Never raises.
func NarrowByTerms(chunks []map[string]any, terms []string, fallbackTerms []string, keywords string, context NarrowContext, maxOutCharsPerChunk, maxOutTotalChars int) NarrowResult {
	before := clampInt(context.Before, 0, maxContext)
	after := clampInt(context.After, 0, maxContext)

	patterns := TermsToPatterns(terms)
	stats := NarrowStats{
		ChunksIn:  len(chunks),
		UsedTerms: len(patterns),
	}
	for _, c := range chunks {
		stats.CharsIn += len(ChunkTextOf(c))
	}
	if len(chunks) == 0 {
		return NarrowResult{Kept: nil, Stats: stats}
	}
	if len(patterns) == 0 {
		narrowed := NarrowWithFallbackKeyword(chunks, keywords)
		stats.ChunksKpt = len(narrowed)
		for _, c := range narrowed {
			stats.CharsOut += len(ChunkTextOf(c))
		}
		return NarrowResult{Kept: narrowed, Stats: stats}
	}

	run := func(active []*regexp.Regexp) ([]map[string]any, []bool, int) {
		var kept []map[string]any
		var flags []bool
		matched := 0
		for _, c := range chunks {
			raw := ChunkTextOf(c)
			if charLen(raw) <= minNarrowChars {
				// Short chunk: kept whole. Flagged as matched so it still
				// participates in the total-budget distribution; Python
				// overwrites with the identical text and pops "highlight".
				kept = append(kept, withNarrowedText(c, raw))
				flags = append(flags, true)
				continue
			}
			text, ok := execOnText(raw, active, before, after, maxOutCharsPerChunk)
			if ok {
				matched++
				kept = append(kept, withNarrowedText(c, text))
			} else {
				// No match: keep the original chunk untouched (full text and any
				// "highlight" preserved) — mirrors Python _apply_narrow's else.
				kept = append(kept, cloneMap(c))
			}
			flags = append(flags, ok)
		}
		return kept, flags, matched
	}

	kept, matchedFlags, _ := run(patterns)

	// Gentle retry (no extra LLM): primary terms hit nothing -> fallback terms once.
	if len(fallbackTerms) > 0 && !anyBool(matchedFlags) {
		if fb := TermsToPatterns(fallbackTerms); len(fb) > 0 {
			kept, matchedFlags, _ = run(fb)
			if len(fb) > stats.UsedTerms {
				stats.UsedTerms = len(fb)
			}
		}
	}

	// Only apply the total-length cap when the grep actually matched: on a no-match
	// the chunks are returned untouched so the caller's compaction decides.
	if anyBool(matchedFlags) {
		totalOut := 0
		for _, c := range kept {
			totalOut += charLen(ChunkTextOf(c))
		}
		if totalOut > maxOutTotalChars {
			perChunk := max(200, min(maxOutCharsPerChunk, maxOutTotalChars/max(1, len(kept))))
			acc := 0
			var trimmed []map[string]any
			for _, c := range kept {
				t := ChunkTextOf(c)
				room := maxOutTotalChars - acc
				if room <= 0 {
					break
				}
				take := min(charLen(t), min(perChunk, room))
				if take <= 0 {
					break
				}
				if take < charLen(t) {
					c = withNarrowedText(cloneMap(c), truncHead(t, take))
				}
				trimmed = append(trimmed, c)
				acc += take
			}
			kept = trimmed
		}
	}

	stats.ChunksKpt = len(kept)
	for _, c := range kept {
		stats.CharsOut += len(ChunkTextOf(c))
	}
	stats.Matched = anyBool(matchedFlags)
	logGrepSed(stats)
	return NarrowResult{Kept: kept, Stats: stats}
}

// NarrowWithFallbackKeyword applies keyword narrowing (zero LLM), returning the
// originals when keyword narrowing yields nothing (Python _fallback_narrow_by_keywords).
func NarrowWithFallbackKeyword(chunks []map[string]any, keywords string) []map[string]any {
	if narrowed := NarrowByKeywords(chunks, keywords); len(narrowed) > 0 {
		return narrowed
	}
	return chunks
}

// fallbackStopwords mirrors Python _FALLBACK_STOPWORDS.
var fallbackStopwords = map[string]bool{
	"what": true, "which": true, "who": true, "where": true, "when": true, "how": true,
	"the": true, "a": true, "an": true, "of": true, "in": true, "on": true,
	"for": true, "to": true, "and": true, "or": true, "with": true, "is": true,
	"are": true, "was": true, "were": true, "list": true, "name": true, "give": true,
	"find": true, "tell": true, "me": true, "about": true, "from": true, "that": true,
	"this": true, "it": true, "its": true, "their": true, "they": true, "have": true,
	"has": true, "do": true, "does": true, "did": true, "based": true, "per": true,
	"according": true, "not": true,
}

// SplitFallbackTerms splits free text into fallback grep terms (zero LLM),
// mirroring Python split_fallback_terms: splits on sentence/comma boundaries,
// drops short/stopword tokens, keeps numbers and multi-word phrases.
func SplitFallbackTerms(texts ...string) []string {
	var terms []string
	seen := map[string]bool{}
	for _, v := range texts {
		for _, part := range reFallbackSplit.Split(strings.TrimSpace(v), -1) {
			part = strings.TrimSpace(strings.Trim(part, "'\"()[]{}"))
			if len(part) < 3 {
				continue
			}
			if fallbackStopwords[strings.ToLower(part)] {
				continue
			}
			if seen[part] {
				continue
			}
			seen[part] = true
			terms = append(terms, part)
			if len(terms) >= maxGrepTerms {
				return terms
			}
		}
	}
	return terms
}

var reFallbackSplit = regexp.MustCompile(`[\n。；;,.?!?]+`)

// GrepSedNarrow narrows chunks by grepping terms extracted directly from the
// claim (zero LLM), mirroring Python grep_sed_narrow. Terms are derived from the
// claim text via SplitFallbackTerms; no extra LLM call. Never raises.
func GrepSedNarrow(chunks []map[string]any, claimSources []string, maxOutCharsPerChunk, maxOutTotalChars int) NarrowResult {
	if len(chunks) == 0 {
		return NarrowResult{Kept: chunks, Stats: NarrowStats{ChunksIn: 0}}
	}
	terms := SplitFallbackTerms(claimSources...)
	return NarrowByTerms(chunks, terms, nil, strings.Join(claimSources, " "), NarrowContext{}, maxOutCharsPerChunk, maxOutTotalChars)
}

// GrepSummaryFromClaims mirrors Python grep_sed_narrow's public convenience: given
// claim/question texts, produce a compact narrowed evidence string (used by the
// compiled-structure grepper in search.go). Returns "" when nothing was kept.
func GrepSummaryFromClaims(chunks []map[string]any, claimSources []string) string {
	res := GrepSedNarrow(chunks, claimSources, defaultOutCharsPerChunk, defaultOutTotalChars)
	if len(res.Kept) == 0 {
		return ""
	}
	var b strings.Builder
	for _, c := range res.Kept {
		if t := ChunkTextOf(c); t != "" {
			b.WriteString(t)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func logGrepSed(s NarrowStats) {
	_LOG.Printf("[grep-sed] chunks=%d->%d chars=%d->%d matched=%t terms=%d",
		s.ChunksIn, s.ChunksKpt, s.CharsIn, s.CharsOut, s.Matched, s.UsedTerms)
}

// ---------------------------------------------------------------------------
// Local helpers shared with the narrowing paths
// ---------------------------------------------------------------------------

func cloneMap(c map[string]any) map[string]any {
	cp := make(map[string]any, len(c))
	for k, v := range c {
		cp[k] = v
	}
	return cp
}

// withNarrowedText returns a copy of the chunk with its narrowed text applied.
// Mirrors Python _apply_narrow's matched branch: it overwrites
// "content_with_weight", mirrors "content" only when that key already exists,
// and drops "highlight" (the pre-narrow highlight spans no longer apply).
func withNarrowedText(c map[string]any, narrowed string) map[string]any {
	cp := cloneMap(c)
	cp["content_with_weight"] = narrowed
	if _, ok := cp["content"]; ok {
		cp["content"] = narrowed
	}
	delete(cp, "highlight")
	return cp
}

// charLen is the codepoint length, matching Python's len(str). Go's len(string)
// is bytes, which diverges for CJK; the narrowing caps are codepoint budgets.
func charLen(s string) int { return utf8.RuneCountInString(s) }

// truncHead keeps the first n codepoints (not bytes) of s, mirroring Python's
// s[:n]. Byte slicing would corrupt multi-byte CJK and mis-size output; rune
// slicing keeps it faithful to the Python engine.
func truncHead(s string, n int) string {
	if charLen(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func sort2DRanges(ranges [][2]int) {
	for i := 0; i < len(ranges); i++ {
		for j := i + 1; j < len(ranges); j++ {
			if ranges[j][0] < ranges[i][0] {
				ranges[i], ranges[j] = ranges[j], ranges[i]
			}
		}
	}
}

func anyBool(flags []bool) bool {
	for _, f := range flags {
		if f {
			return true
		}
	}
	return false
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
