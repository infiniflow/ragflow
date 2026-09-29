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

package runtime

import (
	"context"
	"crypto/md5"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Text processing: keyword narrowing of retrieved chunks.
//
// This covers the narrowing family (keep-or-narrow, keyword narrowing, content
// narrowing, term narrowing) and the compiled-structure grep_sed narrowing. It lives next
// to search.go so the only two consumers (HybridSearch and the structure-nav grepper)
// both reach it.

// NarrowOrKeep: narrow chunks to
// keyword-bearing sentences, but keep the originals when narrowing would drop
// everything.
//
// The all-or-nothing behaviour is the point: no keyword overlap does NOT mean
// irrelevant. The retriever already ranked these chunks, and a sub-question's
// wording need not contain the parent question's keywords. Dropping them all
// produced empty results, unverified claims and pointless retry cycles.
//
// Both outcomes are reported to the DEVELOPER log only (Python parity), never to
// the think block: the resize ratio is pool bookkeeping, and the leg's own result
// line already reports what came back.
func NarrowOrKeep(ctx context.Context, chunks []map[string]any, keywords, label string, logger *log.Logger) []map[string]any {
	if strings.TrimSpace(keywords) == "" || len(chunks) == 0 {
		return chunks
	}
	if logger == nil {
		logger = _LOG
	}
	// LOG-ONLY, and worded exactly as Python words it (text_processing.py:464/:466):
	// how the keyword filter resized the candidate pool is a developer's diagnostic,
	// not something a reader acts on — the leg's own result line already reports what
	// came back, including the narrowed count. It used to be a step (and, before
	// that, a rewritten sentence), which put pool bookkeeping in front of the user.
	//
	// ctx is unused here on purpose: the signature stays uniform with the other
	// narrowing entry points, and a future step would have it available.
	narrowed := NarrowByKeywords(chunks, keywords)
	if len(narrowed) > 0 {
		logger.Printf("[%s] Kept %d of %d passage(s) that actually mention the keywords.",
			label, len(narrowed), len(chunks))
		return narrowed
	}
	logger.Printf("[%s] Keyword narrowing matched nothing — keeping all %d retrieved passage(s).",
		label, len(chunks))
	return chunks
}

// NarrowByKeywords narrows each chunk to the sentences mentioning any keyword
// (+/-1 neighbour) and drops keyword-less chunks.
//
// Unlike NarrowOrKeep this is the strict form: it may return an empty slice, and callers
// that must not lose evidence should use NarrowOrKeep instead.
func NarrowByKeywords(chunks []map[string]any, keywords string) []map[string]any {
	kwds := SplitKeywords(keywords)
	// The input is returned unchanged when there is nothing to narrow on. A nil return
	// would wipe the whole evidence pool, so return the original chunks verbatim instead.
	if len(kwds) == 0 || len(chunks) == 0 {
		return chunks
	}
	out := make([]map[string]any, 0, len(chunks))
	seen := make(map[string]bool, len(chunks))
	for _, c := range chunks {
		if c == nil {
			continue
		}
		narrowed, ok := NarrowContent(ChunkTextOf(c), kwds)
		if !ok {
			continue
		}
		// Dedup identical narrowed passages (chunks whose narrowed text hashes the same).
		h := md5.Sum([]byte(narrowed))
		key := fmt.Sprintf("%x", h)
		if seen[key] {
			continue
		}
		seen[key] = true
		cp := make(map[string]any, len(c)+1)
		for k, v := range c {
			cp[k] = v
		}
		// content_with_weight is
		// always overwritten with the narrowed text, "content" is mirrored ONLY
		// when the original chunk already carried a "content" key, and the
		// pre-narrow "highlight" spans are dropped because they no longer apply.
		// (withNarrowedText in grep_sed_narrow.go applies the identical rule for
		// the term path; keep the two in lock-step.)
		cp["content_with_weight"] = narrowed
		if _, ok := cp["content"]; ok {
			cp["content"] = narrowed
		}
		delete(cp, "highlight")
		out = append(out, cp)
	}
	return out
}

// SplitKeywords normalizes a keyword string into search terms. When fewer than
// 3 comma terms exist, falls back to space-split bigrams — a bare keyword blob
// ("finale run time") is more discriminative as bigrams than as single words.
// This is the term construction used by NarrowByKeywords.
func SplitKeywords(keywords string) []string {
	if strings.TrimSpace(keywords) == "" {
		return nil
	}
	kwds := make([]string, 0, 8)
	for _, k := range strings.Split(keywords, ",") {
		if k = strings.TrimSpace(k); k != "" {
			kwds = append(kwds, strings.ToLower(k))
		}
	}
	if len(kwds) < 3 {
		words := make([]string, 0, 8)
		for _, w := range strings.Fields(keywords) {
			words = append(words, strings.ToLower(w))
		}
		bigrams := make([]string, 0, len(words))
		for i := 0; i+1 < len(words); i++ {
			bigrams = append(bigrams, words[i]+" "+words[i+1])
		}
		if len(bigrams) > 0 {
			return bigrams
		}
	}
	return kwds
}

// Stem-aware keyword matching: the stemmed forms of a keyword and of the text's words are
// matched, so e.g. "nominated" is highlighted for the keyword "nominations".

var wordRe = regexp.MustCompile("[a-z0-9]+")

// wordLetterRe finds the words to stem-match. It is deliberately NOT the shared lowercase
// wordRe, which on capitalized text matches only fragments ("New" -> "ew", "Nominated" ->
// "ominated") and therefore never yields the stem term those words should contribute.
var wordLetterRe = regexp.MustCompile("[A-Za-z]+")

// containedInPhrase reports whether low occurs inside any keyword phrase.
func containedInPhrase(low string, phrases map[string]struct{}) bool {
	for p := range phrases {
		if strings.Contains(p, low) {
			return true
		}
	}
	return false
}

func isAlphaOnly(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// stemmable: len>=4 and purely ASCII letters.
func stemmable(token string) bool { return len(token) >= 4 && isAlphaOnly(token) }

// stem is the stemmer used throughout: porterStem is a faithful port of nltk's
// PorterStemmer (NLTK_EXTENSIONS mode), so the stems match word-for-word. The old
// suffix-stripping fallback is gone: it diverged on exactly the words that matter for
// keyword narrowing.
func stem(word string) string { return porterStem(word) }

// keywordForms: verbatim keeps forms containing
// any non-stemmable token (matched by substring); stemmed holds all-ASCII-letter
// keyword forms as stem tuples (matched by a contiguous stem sequence).
func keywordForms(kwds []string) (verbatim []string, stemmed [][]string) {
	for _, kw := range kwds {
		k := strings.ToLower(strings.TrimSpace(kw))
		if k == "" {
			continue
		}
		tokens := wordRe.FindAllString(k, -1)
		if len(tokens) > 0 && allStemmable(tokens) {
			seq := make([]string, len(tokens))
			for i, t := range tokens {
				seq[i] = stem(t)
			}
			stemmed = append(stemmed, seq)
		} else {
			verbatim = append(verbatim, k)
		}
	}
	return
}

func allStemmable(tokens []string) bool {
	for _, t := range tokens {
		if !stemmable(t) {
			return false
		}
	}
	return true
}

// sentenceStems
func sentenceStems(sentence string) []string {
	tokens := wordRe.FindAllString(strings.ToLower(sentence), -1)
	out := make([]string, len(tokens))
	for i, t := range tokens {
		if stemmable(t) {
			out[i] = stem(t)
		} else {
			out[i] = t
		}
	}
	return out
}

// sentenceMatches: any verbatim substring OR a
// contiguous stemmed sequence.
func sentenceMatches(low string, stems, verbatim []string, stemmed [][]string) bool {
	for _, v := range verbatim {
		if strings.Contains(low, v) {
			return true
		}
	}
	for _, seq := range stemmed {
		width := len(seq)
		if width == 0 || width > len(stems) {
			continue
		}
		for start := 0; start+width <= len(stems); start++ {
			if slices.Equal(stems[start:start+width], seq) {
				return true
			}
		}
	}
	return false
}

// NarrowContent returns the keyword-bearing sentences (+/-2 neighbours) with
// the keywords highlighted, or ("", false) when no keyword occurs.
// Keyword sentences are kept within a +/-2 window, AND fact-dense sentences (numbers /
// years / percentages / proper nouns) within a +/-1 window even without a keyword hit, so
// numeric or named-entity answers survive narrowing. Block-level tables and markdown
// pipe-tables (>=3 rows) are returned whole — keyword-window narrowing would otherwise
// truncate them.
func NarrowContent(content string, kwds []string) (string, bool) {
	if strings.TrimSpace(content) == "" || len(kwds) == 0 {
		return "", false
	}
	lowContent := strings.ToLower(content)
	if strings.Contains(lowContent, "<table") || strings.Contains(lowContent, "<tr") || strings.Contains(lowContent, "<td") {
		// Serialize the table to Markdown before the model sees it: the format
		// comparison over 11 serializations ranks Markdown-KV first for field lookups
		// (key: value beats header/position alignment) and a Markdown table as the
		// cost/accuracy compromise; raw HTML is the expensive and least readable
		// option. The row set is NOT pruned — rank/order and completeness decide table
		// answers. Falls back to the raw text when nothing renders.
		return "..." + HighlightKeywords(TableViewOrRaw(content), kwds) + "...", true
	}
	pipeRows := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.Count(line, "|") >= 2 {
			pipeRows++
		}
	}
	if pipeRows >= 3 {
		return "..." + HighlightKeywords(content, kwds) + "...", true
	}
	sents := SplitSentences(content)
	if len(sents) == 0 {
		return "", false
	}
	verbatim, stemmed := keywordForms(kwds)
	keep := make(map[int]bool, len(sents))
	matched := false
	for i, s := range sents {
		low := strings.ToLower(s)
		hit := sentenceMatches(low, sentenceStems(s), verbatim, stemmed)
		if hit {
			matched = true
			for j := max(0, i-2); j < min(len(sents), i+3); j++ {
				keep[j] = true
			}
		} else if IsFactDenseSentence(s) {
			// Keep fact-dense sentences even without a keyword hit so the answer
			// value (a bare figure, a date, a proper noun) is never lost.
			for j := max(0, i-1); j < min(len(sents), i+2); j++ {
				keep[j] = true
			}
		}
	}
	if !matched {
		return "", false
	}
	var b strings.Builder
	for i := range sents {
		if keep[i] {
			b.WriteString(sents[i])
		}
	}
	return "..." + HighlightKeywords(b.String(), kwds) + "...", true
}

// HighlightKeywords stars keyword occurrences, longest term first so a longer keyword is
// not partially consumed by a shorter one. The marker is a STAR, not an XML tag — a
// multi-word entity must stay ONE contiguous span
// ("*Atlanta Braves*", never "*Atlanta* *Braves*") for the downstream
// entity cross-check. The <em> tags elsewhere in this port are the ENGINE's
// highlight markup (rag/utils/*_conn.py, agentic_search.go), a different layer.
func HighlightKeywords(text string, kwds []string) string {
	if len(kwds) == 0 {
		return text
	}
	terms := append([]string(nil), kwds...)
	// The phrase set: keywords trimmed, lowercased and deduplicated. It guards the stem
	// terms added just below.
	phrases := make(map[string]struct{}, len(kwds))
	for _, kw := range kwds {
		if p := strings.ToLower(strings.TrimSpace(kw)); p != "" {
			phrases[p] = struct{}{}
		}
	}
	// Stem-based highlight terms: a stemmed form that matches in the text is wrapped too,
	// so e.g. "nominated" is highlighted for keyword "nominations".
	_, stemmed := keywordForms(kwds)
	if len(stemmed) > 0 {
		stemSet := make(map[string]bool, 8)
		for _, seq := range stemmed {
			for _, s := range seq {
				stemSet[s] = true
			}
		}
		for _, word := range wordLetterRe.FindAllString(text, -1) {
			low := strings.ToLower(word)
			// A stem-matched word is added only when it is NOT already inside a keyword phrase:
			// "nominated" is starred for "nominations", while the "Braves" of "Atlanta Braves"
			// is left to the phrase's own span instead of being starred on its own elsewhere.
			if stemmable(low) && stemSet[stem(low)] && !containedInPhrase(low, phrases) {
				terms = append(terms, low)
			}
		}
	}
	// Match in RUNE space. `strings.ToLower` is not byte-length-preserving: "İ"
	// is 2 bytes and folds to the 1-byte "i", so a byte offset taken from the
	// original indexes the folded string at a different position. The loop then
	// slices past the end of the folded string (panic: slice bounds out of
	// range) or cuts a rune in half and emits invalid UTF-8. Go's case mapping is
	// 1:1 per RUNE, so a rune index is valid in both strings. (A regex-based rewrite is
	// immune for a different reason: it re-emits the matched group from the ORIGINAL text
	// instead of re-slicing it.)
	rs := []rune(text)
	lows := []rune(strings.ToLower(text))
	if len(lows) != len(rs) {
		// Unreachable while the fold stays rune-for-rune; kept so a future switch
		// to a full case fold (which does change the rune count) degrades to
		// plain text instead of misaligned spans.
		return text
	}
	// Longest term first, compared by rune count — i.e. by code points.
	termRunes := make([][]rune, 0, len(terms))
	for _, t := range terms {
		// Fold the TERM the same way the haystack was folded: the match below
		// compares against `lows`, and only the stem-derived terms appended above
		// were already lowercase — a caller-supplied "Rocket" or "New York" kept
		// its casing and therefore never matched, silently dropping the highlight.
		// The phrase list is built with a strip+lower pass and matched case-insensitively, so
		// the trim and the case fold both belong here.
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			termRunes = append(termRunes, []rune(t))
		}
	}
	sort.SliceStable(termRunes, func(i, j int) bool { return len(termRunes[i]) > len(termRunes[j]) })
	var b strings.Builder
	for i := 0; i < len(rs); {
		bestLen := 0
		for _, t := range termRunes {
			if len(t) > bestLen && runesHavePrefix(lows[i:], t) {
				bestLen = len(t)
			}
		}
		if bestLen == 0 {
			b.WriteRune(rs[i])
			i++
			continue
		}
		// Emit the ORIGINAL runes, so the highlight keeps the source casing
		// wrapped in the star marker.
		b.WriteString("*")
		b.WriteString(string(rs[i : i+bestLen]))
		b.WriteString("*")
		i += bestLen
	}
	return b.String()
}

// runesHavePrefix reports whether hay starts with needle.
func runesHavePrefix(hay, needle []rune) bool {
	if len(needle) == 0 || len(needle) > len(hay) {
		return false
	}
	for i, r := range needle {
		if hay[i] != r {
			return false
		}
	}
	return true
}

// IsFactDenseSentence reports whether a sentence carries a fact-bearing signal: a
// number / year / percentage / magnitude word or a proper noun that is not part of an
// abbreviation run. It keeps only informative sentences when narrowing / grepping, so a
// numeric or entity answer is never dropped just because it lacks the query keywords.
// Deliberately NO quoted-span or ≥6-token rule, and no bare digit counts as a fact
// signal — those widened the gate far beyond the strict definition.
func IsFactDenseSentence(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// The regex is case-insensitive, so a single (?i) search covers both the original and
	// the lowercased form.
	if factPattern.MatchString(s) {
		return true
	}
	if hasProperNoun(s) {
		return true
	}
	return false
}

var (
	// factPattern: a number that
	// may carry an ordinal suffix (st/nd/rd/th) or a percent sign and may use
	// comma/dot group separators, a 1900-2099 four-digit year, or a magnitude
	// word (percent/million/billion/thousand/km/km2/sq km/m above/m). re.IGNORECASE
	// makes the words case-insensitive.
	factPattern = regexp.MustCompile(`(?i)(?:\d[\d,\.]*(?:st|nd|rd|th)?%?)|(?:19|20)\d{2}|\b(?:percent|percentage|million|billion|thousand|km|km2|sq\s*km|m\s*above|m)\b`)

	// properNounPattern: word pattern: a
	// capitalized word of at least three letters, normally wrapped in the negative lookbehind
	// (?<![.!?]\.); RE2 has no lookbehind, so that abbreviation guard is applied by
	// hasProperNoun instead of in the regex.
	properNounPattern = regexp.MustCompile(`\b[A-Z][a-z]{2,}\b`)
)

// hasProperNoun reports whether s contains a capitalized proper noun that is not
// preceded by an abbreviation point. The pattern is
// (?<![.!?]\.)\b[A-Z][a-z]{2,}\b: the guard rejects a match whose start sits right
// after the two characters "X." where X ∈ {., !, ?} (e.g. a "U.S."-style run).
func hasProperNoun(s string) bool {
	for _, loc := range properNounPattern.FindAllStringIndex(s, -1) {
		idx := loc[0]
		// The negative lookbehind (?<![.!?]\.) fails when s[idx-2] ∈ {. ! ?} and
		// s[idx-1] == '.'. A match at idx < 2 cannot be guarded, so keep it.
		if idx >= 2 && (s[idx-2] == '.' || s[idx-2] == '!' || s[idx-2] == '?') && s[idx-1] == '.' {
			continue
		}
		return true
	}
	return false
}

// Sentence segmentation.
//
// The foundation for every narrowing / highlighting / fact-density step in the
// retrieval. Two properties matter:
//
//  1. terminators (。！？；!?; and a digit-guarded English period) are KEPT on
//     their sentence, so "3.14" and "v1.2" do not split;
//  2. block-level HTML elements (table/div/p/ul/li/... — see htmlBlockTags) and
//     markdown tables are ATOMIC and never split internally, so a whole table /
//     list / block counts as ONE "sentence" for keyword matching (a keyword
//     inside one keeps the whole block). The scanning is nesting-aware, not a bare
//     <table> regex.
//
// Go's RE2 lacks lookbehind, so the digit guard is a manual scan rather than a
// regex assertion.

// htmlBlockTags are the block-level HTML containers kept atomic during sentence
// splitting. Inline tags like <b>/<i>/<em> are deliberately excluded, so ordinary prose
// still splits.
var htmlBlockTags = map[string]bool{
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true,
	"td": true, "th": true, "caption": true, "colgroup": true, "ul": true,
	"ol": true, "li": true, "dl": true, "dt": true, "dd": true, "div": true,
	"p": true, "pre": true, "blockquote": true, "section": true, "article": true,
	"aside": true, "nav": true, "main": true, "figure": true, "figcaption": true,
	"header": true, "footer": true, "address": true, "details": true, "summary": true,
	"form": true, "fieldset": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true,
}

var htmlTagRe = regexp.MustCompile(`(?i)<(/?)([a-zA-Z][a-zA-Z0-9]*)\b([^>]*)>`)

// mdTableRe: header row with a pipe, a separator row
// of dashes/colons/pipes, then zero+ body rows with a pipe.
var mdTableRe = regexp.MustCompile("(?m)^[ \t]*\\|?[^\n]*\\|[\n][ \t]*\\|?[ \t]*:?-{1,}:?[ \t]*(?:\\|[ \t]*:?-{1,}:?[ \t]*)+\\|?[ \t]*\r?\n(?:[ \t]*\\|?[^\n]*\\|[^\n]*\r?\n?)*")

// htmlBlockSpans returns outermost balanced block-level HTML element spans
// (nesting-aware) via a tag stack.
func htmlBlockSpans(text string) [][2]int {
	type stackItem struct {
		name  string
		start int
	}
	var spans [][2]int
	var stack []stackItem
	for _, m := range htmlTagRe.FindAllStringSubmatchIndex(text, -1) {
		name := strings.ToLower(text[m[4]:m[5]])
		if !htmlBlockTags[name] {
			continue
		}
		closing := m[2] != -1 && m[2] != m[3] // group 1 captured => closing tag
		if closing {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == name {
					start := stack[i].start
					stack = stack[:i]
					if len(stack) == 0 { // closed an outermost block
						spans = append(spans, [2]int{start, m[1]})
					}
					break
				}
			}
		} else {
			attrs := text[m[6]:m[7]]
			if strings.HasSuffix(strings.TrimSpace(attrs), "/") {
				continue // self-closing
			}
			stack = append(stack, stackItem{name: name, start: m[2]})
		}
	}
	return spans
}

// protectedSpans returns non-overlapping atomic (start, end) spans in order, covering
// block-level HTML elements and markdown tables (overlaps are unioned).
func protectedSpans(text string) [][2]int {
	spans := htmlBlockSpans(text)
	for _, m := range mdTableRe.FindAllStringIndex(text, -1) {
		spans = append(spans, [2]int{m[0], m[1]})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i][0] < spans[j][0] })
	var merged [][2]int
	lastEnd := -1
	for _, s := range spans {
		if s[0] < lastEnd { // overlaps an already-kept span -> union it in
			if s[1] > lastEnd {
				merged[len(merged)-1][1] = s[1]
				lastEnd = s[1]
			}
			continue
		}
		merged = append(merged, s)
		lastEnd = s[1]
	}
	return merged
}

// SplitSentences splits text into sentences, treating each block-level HTML
// element and markdown table as one atomic unit.
func SplitSentences(text string) []string {
	if text == "" {
		return nil
	}
	spans := protectedSpans(text)
	if len(spans) == 0 {
		return splitPlainSentences(text)
	}
	var sents []string
	pos := 0
	for _, m := range spans {
		if m[0] > pos {
			sents = append(sents, splitPlainSentences(text[pos:m[0]])...)
		}
		if block := text[m[0]:m[1]]; strings.TrimSpace(block) != "" {
			sents = append(sents, block)
		}
		pos = m[1]
	}
	if pos < len(text) {
		sents = append(sents, splitPlainSentences(text[pos:])...)
	}
	return sents
}

// splitPlainSentences splits plain text (no table/block spans) into sentences,
// keeping each terminator attached and guarding decimal periods. Operates on
// runes; rune indices == byte indices for the ASCII terminators we emit.
//
// The split is LOSSLESS: inter-sentence whitespace is kept as the prefix of the
// FOLLOWING sentence (only whitespace-only segments are dropped), so
// `"".join(sents)` reproduces the input, and the content narrowing depends on that
// property — it rejoins the kept sentences with "", and trimming each sentence would
// collapse a multi-line chunk into one line, which the grep term-window (line-based) then
// matches wholesale instead of line by line. Observed as the grep leg keeping an entire
// trailing paragraph.
func splitPlainSentences(text string) []string {
	rs := []rune(text)
	var sents []string
	start := 0
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if !isSentTerminator(r) {
			continue
		}
		// ASCII period guarded against decimals (digit on BOTH sides).
		if r == '.' && i > 0 && i+1 < len(rs) && isASCIIDigit(rs[i-1]) && isASCIIDigit(rs[i+1]) {
			continue
		}
		// Consume a run of terminators (e.g. "。！？" or "...").
		j := i + 1
		for j < len(rs) && isSentTerminator(rs[j]) && rs[j] != '.' {
			j++
		}
		seg := string(rs[start:j])
		if strings.TrimSpace(seg) != "" {
			sents = append(sents, seg)
		}
		start = j
		i = j - 1
	}
	if start < len(rs) {
		if tail := string(rs[start:]); strings.TrimSpace(tail) != "" {
			sents = append(sents, tail)
		}
	}
	return sents
}

func isSentTerminator(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '!', '?', ';', '.':
		return true
	}
	return false
}

func isASCIIDigit(r rune) bool { return r >= '0' && r <= '9' }

// Keyword compaction.
//
// Post-processing for keyword extraction: dedupe (preserving order)
// and cap the compacted keyword string at compactMaxKeywords terms. The
// extraction prompt asks for 3-10 terms PLUS 2-3 synonyms each, which models
// answer with a 40-60 word redundant synonym run; appending that whole run onto
// the query diluted the vector leg and dragged BM25 onto unrelated docs. This
// keeps the recall terms but drops the redundancy, so keywords stay a compact
// hint instead of a pollution source.
//
// It lives here because it is a text-processing primitive of the runtime, not an
// agentic-pipe stage.

// compactMaxKeywords caps the compacted keyword string.
const compactMaxKeywords = 15

// CompactKeywords dedupes (preserving order) and caps at compactMaxKeywords.
//
// Accepts both space- and comma-separated input (single-turn extract_keywords
// emits spaces; multi-turn formalize emits commas).
func CompactKeywords(kw string) string {
	if strings.TrimSpace(kw) == "" {
		return ""
	}
	var seen []string
	for _, t := range regexp.MustCompile(`[,\s]+`).Split(strings.TrimSpace(kw), -1) {
		t = strings.TrimSpace(t)
		if t == "" || containsStr(seen, t) {
			continue
		}
		seen = append(seen, t)
		if len(seen) >= compactMaxKeywords {
			break
		}
	}
	return strings.Join(seen, " ")
}
