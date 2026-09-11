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
	"sort"
	"strings"
	"unicode"
)

// Raw-chunk memory store.
//
// Mirrors Python harness/memory.py, which maintains the lossless store backing
// the (lossy) kbinfos.Chunks list that feeds the LLM. Retrieval narrows chunks
// to the sentences that answer the current query, so the raw text has to be
// kept somewhere: a later gap query often needs a fact the earlier narrowing
// already threw away.
//
// The store lives on Kbinfos.Memory so it travels with the request.

const (
	// grepMaxChunks is Python memory.grep's default cap (_GREP_MAX_CHUNKS = 6).
	// MemoryGrep takes the limit explicitly and does NOT substitute this for a
	// non-positive value (Python has no such guard), so callers that want the
	// Python default pass it.
	grepMaxChunks = 6
	// grepMaxSentences caps sentences kept per chunk (hit + context).
	grepMaxSentences = 4
	// grepContextChars is the per-side char budget when expanding context.
	grepContextChars = 400
	// shortChunkChars: chunks at or below this length are kept whole, since
	// answers often live in short chunks.
	shortChunkChars = 200

	// MemorySearch tuning — mirror Python memory.search defaults.
	memoryDefaultTopN = 6
	// memoryMinRatio: a chunk is relevant when it shares >= 1 term AND >= this
	// fraction of the query's significant terms (normalized overlap bar so CN /
	// EN queries behave alike). Python's min_overlap param is declared but
	// unused in search(), so only the ratio matters.
	memoryMinRatio = 0.12
	// memoryMaxTerms caps how many significant terms we extract from a query.
	memoryMaxTerms = 18
)

var (
	// reTermPunct strips leading/trailing punctuation before escaping.
	reTermPunct = regexp.MustCompile(`^[\s.,:;!?'"()\[\]{}]+|[\s.,:;!?'"()\[\]{}]+$`)
	// reCJK detects CJK / kana / hangul, which get no \b anchor (a word boundary
	// never matches between CJK characters).
	reCJK = regexp.MustCompile(`[\x{4e00}-\x{9fff}\x{3040}-\x{30ff}\x{ac00}-\x{d7af}]`)
	// reCJKRun matches a maximal run of CJK/kana/hangul for 3-gram splitting.
	reCJKRun = regexp.MustCompile(`[\x{4e00}-\x{9fff}\x{3040}-\x{30ff}\x{ac00}-\x{d7af}]+`)
	// reLatinNum matches a maximal alphanumeric run (Latin words + numbers).
	reLatinNum = regexp.MustCompile(`[A-Za-z0-9]+`)
	// reDigits matches a pure digit run.
	reDigits = regexp.MustCompile(`\d+`)
)

// memoryStopwords mirrors Python memory._STOPWORDS: dropped from Latin query
// terms so "what / the / is" style words do not dominate the overlap score.
var memoryStopwords = map[string]struct{}{
	"what": {}, "which": {}, "how": {}, "many": {}, "much": {}, "does": {},
	"did": {}, "do": {}, "the": {}, "a": {}, "an": {}, "is": {}, "are": {},
	"was": {}, "were": {}, "be": {}, "been": {}, "being": {}, "of": {},
	"for": {}, "to": {}, "in": {}, "on": {}, "with": {}, "and": {}, "or": {},
	"by": {}, "from": {}, "at": {}, "it": {}, "its": {}, "this": {}, "that": {},
	"these": {}, "those": {}, "who": {}, "when": {}, "where": {}, "why": {},
	"than": {}, "then": {}, "there": {}, "their": {}, "they": {}, "them": {},
	"his": {}, "her": {}, "him": {}, "she": {}, "he": {}, "we": {}, "you": {},
	"your": {},
}

// MemoryAdd mirrors Python memory.add: merges raw retrieved chunks into the
// central store, losslessly, skipping chunks already present and those with no
// text.
func MemoryAdd(kb *Kbinfos, chunks []map[string]any) {
	if kb == nil || len(chunks) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(kb.Memory))
	for _, c := range kb.Memory {
		seen[chunkKey(c)] = struct{}{}
	}
	added := 0
	for _, c := range chunks {
		if c == nil || chunkText(c) == "" {
			continue
		}
		k := chunkKey(c)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		kb.Memory = append(kb.Memory, c)
		added++
	}
	if added > 0 {
		_LOG.Printf("[Memory] stored %d new raw chunk(s); memory now has %d.", added, len(kb.Memory))
	}
}

// MemorySize mirrors Python memory.size.
func MemorySize(kb *Kbinfos) int {
	if kb == nil {
		return 0
	}
	return len(kb.Memory)
}

// MemoryClear mirrors Python memory.clear.
func MemoryClear(kb *Kbinfos) {
	if kb != nil {
		kb.Memory = nil
	}
}

// MemoryGrep mirrors Python memory.grep: returns memory chunks containing any of
// terms, narrowed to the matching sentence plus a small context window.
//
// terms are plain strings (entities / numbers / key phrases) as emitted by the
// analysis LLM. Each returned chunk carries a narrowed "content" so the caller
// can splice it straight into an evidence list. Empty on no-hit / no-memory.
//
// limit is the maximum number of chunks returned and is NOT normalized: Python's
// grep has no such guard, so a limit <= 0 makes the `len(hits) >= limit` check
// fire on the first hit (at most one chunk comes back). Callers wanting Python's
// default pass grepMaxChunks.
func MemoryGrep(kb *Kbinfos, terms []string, limit int) []map[string]any {
	if kb == nil || len(kb.Memory) == 0 || len(terms) == 0 {
		return nil
	}
	patterns, prefixPatterns := compileTerms(terms)
	if len(patterns) == 0 && len(prefixPatterns) == 0 {
		return nil
	}
	match := func(text string) bool {
		for _, p := range patterns {
			if p.MatchString(text) {
				return true
			}
		}
		// Prefix fallback: morphological tolerance. A gap term often differs from
		// the chunk's word by a suffix (gap "abbreviation" vs chunk "abbreviated").
		// Matching the leading stem at the START of a word lets a shared root hit.
		for _, p := range prefixPatterns {
			if p.MatchString(text) {
				return true
			}
		}
		return false
	}

	var hits []map[string]any
	for _, c := range kb.Memory {
		text := chunkText(c)
		if len(text) <= shortChunkChars {
			// Short chunk: keep whole (its answer may live anywhere in it).
			if match(text) {
				hits = append(hits, map[string]any{
					"content":  text,
					"doc_id":   c["doc_id"],
					"chunk_id": c["chunk_id"],
				})
				// This branch continues past the shared cap below, so enforce the
				// limit here too — otherwise a memory store full of matching short
				// chunks comes back whole. Python's memory.grep enforces it in the
				// same place.
				if len(hits) >= limit {
					break
				}
			}
			continue
		}
		sents := SplitSentences(text)
		var kept []string
		for i, s := range sents {
			if match(s) {
				for _, w := range sentenceSpanWindow(sents, i) {
					if !containsStr(kept, w) {
						kept = append(kept, w)
					}
				}
			}
			if len(kept) >= grepMaxSentences {
				break
			}
		}
		if len(kept) > 0 {
			hits = append(hits, map[string]any{
				"content":  strings.Join(kept, "\n"),
				"doc_id":   c["doc_id"],
				"chunk_id": c["chunk_id"],
			})
		}
		if len(hits) >= limit {
			break
		}
	}
	return hits
}

// compileTerms mirrors Python memory.grep's pattern construction: the escaped
// term (word-anchored when it is long enough and wholly alphanumeric) plus the
// leading-stem prefix pattern used as a fallback.
func compileTerms(terms []string) (patterns, prefixPatterns []*regexp.Regexp) {
	for _, t := range terms {
		if frag := escapeTerm(t); frag != "" {
			if p, err := regexp.Compile("(?i)" + frag); err == nil {
				patterns = append(patterns, p)
			}
		}
		stripped := strings.TrimSpace(t)
		prefix := ""
		if len([]rune(stripped)) >= 6 {
			prefix = string([]rune(stripped)[:5])
		}
		if prefix != "" && !reCJK.MatchString(prefix) {
			if p, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(prefix)); err == nil {
				prefixPatterns = append(prefixPatterns, p)
			}
		}
	}
	return patterns, prefixPatterns
}

// escapeTerm mirrors Python _escape_term: strip surrounding punctuation, escape
// regex metacharacters, then anchor on word boundaries — except for CJK, where
// a \b anchor would never match.
func escapeTerm(term string) string {
	t := strings.TrimSpace(term)
	if t == "" {
		return ""
	}
	t = reTermPunct.ReplaceAllString(t, "")
	if t == "" {
		return ""
	}
	escaped := regexp.QuoteMeta(t)
	if reCJK.MatchString(t) {
		return escaped
	}
	rs := []rune(t)
	if len(rs) >= 3 && isAlnum(rs[0]) && isAlnum(rs[len(rs)-1]) {
		return `\b` + escaped + `\b`
	}
	return escaped
}

// sentenceSpanWindow mirrors Python _sentence_span_window: the hit sentence plus
// up to one neighbour on each side, clamped to a total length.
func sentenceSpanWindow(sents []string, idx int) []string {
	if idx < 0 || idx >= len(sents) {
		return nil
	}
	lo, hi := max(0, idx-1), min(len(sents), idx+2)
	total := 0
	var kept []string
	for _, s := range sents[lo:hi] {
		total += len(s)
		if total > grepContextChars*2 {
			break
		}
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		return []string{sents[idx]}
	}
	return kept
}

func isAlnum(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// MemorySearch mirrors Python memory.search: relevance-ranked retrieval over the
// raw-chunk memory store (a retrieval-reuse cache, NOT a noise-injection source).
// Unlike MemoryGrep (loose keyword hit) it keeps only chunks whose overlap with the
// query's SIGNIFICANT terms clears a normalized bar, so a fact retrieved earlier can
// be reused instead of re-querying the index.
//
// Language-agnostic term extraction (mirrors Python _significant_terms):
//   - numbers kept verbatim;
//   - CJK runs split into character 3-grams (no word boundaries exist);
//   - Latin alphanumeric runs lowercased, stopword-filtered, len >= 3.
//
// A chunk is relevant when it shares >= 1 term AND >= minRatio of the query's
// significant terms; results are ranked by hit count then text length, capped at
// topN. Returns nil when nothing clears the bar (the caller falls back to a
// knowledge-base search). minRatio <= 0 falls back to memoryMinRatio.
func MemorySearch(kb *Kbinfos, query string, topN int, minRatio float64) []map[string]any {
	if kb == nil || len(kb.Memory) == 0 {
		return nil
	}
	if topN <= 0 {
		topN = memoryDefaultTopN
	}
	if minRatio <= 0 {
		minRatio = memoryMinRatio
	}
	terms := significantTerms(query)
	if len(terms) == 0 {
		return nil
	}
	matchers := buildTermMatchers(terms)
	n := len(terms)

	type scoredChunk struct {
		hits int
		text string
		c    map[string]any
	}
	var scored []scoredChunk
	for _, c := range kb.Memory {
		text := chunkText(c)
		if text == "" {
			continue
		}
		hits := 0
		for _, m := range matchers {
			if m.matches(text) {
				hits++
			}
		}
		if hits < 1 {
			continue
		}
		if float64(hits)/float64(n) < minRatio {
			continue
		}
		scored = append(scored, scoredChunk{hits: hits, text: text, c: c})
	}
	if len(scored) == 0 {
		return nil
	}
	// Rank by hit count desc, then text length desc (Python: (-hits, -len)).
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].hits != scored[j].hits {
			return scored[i].hits > scored[j].hits
		}
		return len(scored[i].text) > len(scored[j].text)
	})
	if len(scored) > topN {
		scored = scored[:topN]
	}
	out := make([]map[string]any, 0, len(scored))
	for _, s := range scored {
		out = append(out, map[string]any{
			"content":    s.text,
			"doc_id":     s.c["doc_id"],
			"chunk_id":   s.c["chunk_id"],
			"similarity": float64(s.hits),
		})
	}
	rq := []rune(query)
	if len(rq) > 60 {
		rq = rq[:60]
	}
	_LOG.Printf("[Memory.search] query=%q -> %d relevant chunk(s) (ratio>=%.2f, %d terms)", string(rq), len(out), minRatio, n)
	return out
}

// significantTerms mirrors Python memory._significant_terms: language-agnostic
// significant-term extraction, de-duplicated and capped at memoryMaxTerms.
func significantTerms(text string) []string {
	var out []string
	seen := make(map[string]struct{})
	push := func(tok string) {
		if tok == "" {
			return
		}
		if _, dup := seen[tok]; dup {
			return
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	// Numbers anywhere.
	for _, m := range reDigits.FindAllString(text, -1) {
		push(m)
		if len(out) >= memoryMaxTerms {
			return out
		}
	}
	// CJK runs -> 3-grams (and the whole run if shorter than 3).
	for _, run := range reCJKRun.FindAllString(text, -1) {
		rs := []rune(run)
		if len(rs) < 3 {
			push(run)
		} else {
			for i := 0; i+3 <= len(rs); i++ {
				push(string(rs[i : i+3]))
			}
		}
		if len(out) >= memoryMaxTerms {
			return out
		}
	}
	// Latin words (stopword-filtered, len >= 3); pure-digit runs already handled.
	for _, m := range reLatinNum.FindAllString(text, -1) {
		if isAllDigits(m) {
			continue
		}
		low := strings.ToLower(m)
		if len(low) >= 3 {
			if _, stop := memoryStopwords[low]; !stop {
				push(low)
			}
		}
		if len(out) >= memoryMaxTerms {
			return out
		}
	}
	return out
}

// termMatcher precompiles one query term's match predicate so MemorySearch
// does not recompile regexes per chunk.
type termMatcher struct {
	// kind: 0 = CJK substring, 1 = digit substring, 2 = Latin word-boundary
	// (with a short prefix fallback for inflectional variants).
	kind int
	sub  string
	re   *regexp.Regexp
	pre  *regexp.Regexp
}

func buildTermMatchers(terms []string) []termMatcher {
	matchers := make([]termMatcher, 0, len(terms))
	for _, t := range terms {
		if run := reCJKRun.FindString(t); run != "" && run == t {
			// Pure CJK term: literal substring (no word boundary).
			matchers = append(matchers, termMatcher{kind: 0, sub: t})
			continue
		}
		if isAllDigits(t) {
			matchers = append(matchers, termMatcher{kind: 1, sub: t})
			continue
		}
		rs := []rune(t)
		re, _ := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(t) + `\b`)
		var pre *regexp.Regexp
		if len(rs) >= 6 {
			pre, _ = regexp.Compile(`(?i)\b` + regexp.QuoteMeta(string(rs[:5])))
		}
		matchers = append(matchers, termMatcher{kind: 2, re: re, pre: pre})
	}
	return matchers
}

func (m termMatcher) matches(text string) bool {
	switch m.kind {
	case 0, 1:
		return strings.Contains(text, m.sub)
	default:
		if m.re != nil && m.re.MatchString(text) {
			return true
		}
		if m.pre != nil && m.pre.MatchString(text) {
			return true
		}
		return false
	}
}

// IsStopword reports whether w is one of Python memory._STOPWORDS.
//
// Go's fan-out needs it because Python aliases the same set as
// _FANOUT_STOPWORDS (agentic_rag_graph.py:_expand_fanouts) when filtering candidate terms.
func IsStopword(w string) bool {
	_, ok := memoryStopwords[w]
	return ok
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
