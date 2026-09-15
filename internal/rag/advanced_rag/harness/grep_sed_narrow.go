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
	"fmt"
	"log"
	"regexp"
	"strings"
	"unicode"
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

// cjkPhraseRunes is the length at which a CJK token stops being a term and
// becomes a clause: see GrepTermsFromQuery. Four is the longest Chinese proper
// name that is still read as one token (成吉思汗), so a longer run is prose.
const cjkPhraseRunes = 4

// GrepOutCharsPerChunk is the grep narrow's per-chunk output cap (Python
// _GREP_OUT_CHARS_PER_CHUNK).
const GrepOutCharsPerChunk = 700

// GrepOutTotalChars is the grep narrow's total output cap (Python
// _GREP_OUT_TOTAL_CHARS).
const GrepOutTotalChars = 8000

// GrepTermsFromQuery mirrors Python _grep_terms_from_query — bare alnum words of
// length>=2, deduped (order-preserving) and capped — and extends it to CJK.
//
// The Python original is ALNUM-ONLY, so a Chinese query yields no term at all and
// GrepSearch's own guard (`if not chunks or not terms: return res`) returns whole
// chunks instead of located windows. Measured on a Chinese question: the
// "Keyword-first locate" line was logged, a narrowed line never was, and every
// hit was a ~1200-char chunk. That is expensive (a name lives in one clause of
// those 1200 chars) and it is what made an enumeration unreadable to the model,
// which cannot see WHICH clause a name sits in.
//
// The derivation keeps Python's shape and adds CJK, with no regex:
//
//   - an ALTERNATION is the caller's own term list — "颜良|文丑|荀正" is how the
//     count protocol tells a session to batch its probes — so it is split on "|"
//     and its pieces are kept at any length ("关羽|斩": the predicate is a term);
//   - otherwise the query is split on whitespace and punctuation, Latin tokens
//     keep Python's two-character floor, and CJK tokens keep a two-CJK-rune
//     floor (a lone Chinese character is a particle, not a term);
//   - an unbroken CJK clause (a question written without separators) yields no
//     token either way, so its two-rune windows are used, left to right: the
//     names in the clause still locate, and a window that occurs nowhere costs
//     one failed lookup inside the narrowing pass and nothing else.
func GrepTermsFromQuery(query string) []string {
	return grepTermsFromQuery(query, true)
}

// GrepWordsFromQuery is GrepTermsFromQuery restricted to the caller's OWN words:
// the pieces of an alternation, and the tokens separated by whitespace or
// punctuation. It has NO CJK-window fallback.
//
// The distinction matters because the two answers are used for different jobs.
// The windows exist to LOCATE a term inside an unbroken CJK clause — the thing
// grep is for, and a failed lookup there costs nothing. As terms to SEARCH FOR,
// they are our guesses rather than the caller's words, and each one would spend a
// retrieval of its own. Measured (2026-09-15): a pass built from windows probed
// 羽斩 / 杀的 / 的有 / 领名 / 单温 and reported 14 of 20 probes "absent" — fourteen
// retrievals spent on fragments nobody asked about, while every name the question
// was missing stayed out of the list entirely.
//
// So a caller that READS these (the named-term seat pass) uses the words; a
// caller that LOCATES with them uses the terms.
func GrepWordsFromQuery(query string) []string {
	return grepTermsFromQuery(query, false)
}

func grepTermsFromQuery(query string, windows bool) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	terms := make([]string, 0, GrepTermsMax)
	seen := make(map[string]struct{}, GrepTermsMax)
	add := func(t string) (full bool) {
		if t == "" {
			return len(terms) >= GrepTermsMax
		}
		low := strings.ToLower(t)
		if _, dup := seen[low]; dup {
			return len(terms) >= GrepTermsMax
		}
		seen[low] = struct{}{}
		terms = append(terms, t)
		return len(terms) >= GrepTermsMax
	}

	if strings.Contains(q, "|") {
		for _, part := range strings.Split(q, "|") {
			if add(trimTermEdges(part)) {
				break
			}
		}
		return terms
	}

	for _, token := range strings.FieldsFunc(q, isTermSeparator) {
		token = trimTermEdges(token)
		if token == "" {
			continue
		}
		if hasCJK(token) {
			runes := utf8.RuneCountInString(token)
			if runes < 2 {
				continue
			}
			// A CJK token past the name boundary is a CLAUSE, not a term: its
			// literal form rarely occurs in the corpus, so it locates nothing.
			// The two jobs part company here:
			//
			//   LOCATING (windows) — decompose the clause into the two-rune
			//   windows a name can actually be found in;
			//   the caller's WORDS (GrepWordsFromQuery) — drop it. A clause names
			//   no individual, and its own phrase search already covers it.
			if !windows {
				if runes > cjkPhraseRunes {
					continue
				}
			} else if runes >= cjkPhraseRunes {
				full := len(terms) >= GrepTermsMax
				for _, window := range cjkWindowsOf(token, GrepTermsMax) {
					if add(window) {
						full = true
						break
					}
				}
				if full {
					break
				}
				continue
			}
		} else if len(token) < 2 {
			continue
		}
		if add(token) {
			break
		}
	}
	if len(terms) > 0 || !windows {
		return terms
	}
	return cjkWindowsOf(q, GrepTermsMax)
}

// trimTermEdges strips the punctuation a token can carry instead of the regex
// Python uses for the same job (reTermEdgePunct).
func trimTermEdges(t string) string {
	return strings.Trim(t, " \t\r\n.,:;!?'\"()[]{}<>“”‘’（）【】《》「」〈〉—…·_-")
}

// isTermSeparator reports whether a rune separates terms. "." and "," do;
// "-", "_" and "." INSIDE a Latin token do not (Python keeps
// [A-Za-z0-9_.-] as token material), so they are not listed here.
func isTermSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\r', '\n', '\v', '\f',
		',', ';', ':', '!', '?', '/', '\\', '(', ')', '[', ']', '{', '}', '<', '>', '"', '\'', '|',
		'，', '。', '、', '；', '：', '！', '？', '（', '）', '【', '】', '《', '》', '「', '」', '〈', '〉',
		'“', '”', '‘', '’', '—', '…', '·':
		return true
	}
	return false
}

// hasCJK reports whether a token contains a CJK / kana / hangul rune — the same
// classes EscapeTerm refuses to wrap in \b, because no word boundary exists
// between them.
func hasCJK(s string) bool {
	for _, r := range s {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

// isCJKRune reports whether a rune belongs to a script whose text has no word
// boundaries.
func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

// cjkWindowsOf returns the two-rune windows of a string's CJK runs, in order,
// deduped and capped. It is the last-resort term derivation for a query that is
// one unbroken clause (see GrepTermsFromQuery).
func cjkWindowsOf(query string, limit int) []string {
	out := make([]string, 0, limit)
	run := make([]rune, 0, 16)
	flush := func() {
		if len(run) >= 2 {
			for i := 0; i+2 <= len(run) && len(out) < limit; i++ {
				window := string(run[i : i+2])
				dup := false
				for _, o := range out {
					if o == window {
						dup = true
						break
					}
				}
				if !dup {
					out = append(out, window)
				}
			}
		}
		run = run[:0]
	}
	for _, r := range query {
		if isCJKRune(r) {
			run = append(run, r)
			continue
		}
		flush()
	}
	flush()
	if len(out) == 0 {
		return nil
	}
	return out
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
		// A line window is unusable when the LINE IS THE WHOLE CHUNK, and that is
		// the CJK case by default: a Chinese chunk carries no newlines, so its
		// one line is the entire passage and the window hands back all of it. The
		// length test catches the same thing for a chunk that does have a few
		// very long lines. Either way, fall back to the SENTENCES the hit sits
		// in — same per-side budget, cut at 。！？； instead of mid-clause.
		if len(lines) <= 1 || (fragE-fragS > contextCharBudget*2 && (fragE-fragS) > (r[1]-r[0])) {
			if window := sentenceWindow(content, r, contextCharBudget); window[1]-window[0] < fragE-fragS {
				fragS, fragE = window[0], window[1]
			}
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

// sentenceWindow expands a hit to the sentence it sits in, bounded by budget
// bytes on each side.
//
// It exists for text with no line structure. A CJK chunk is one long line, so
// the line-window path returns the whole chunk for a hit that needs one clause of
// it; cutting at sentence punctuation instead keeps the window bounded AND keeps
// the clause intact, which is what makes the result readable to the model (a
// name is read off the clause around the verb). Terminators are the sentence
// punctuation of both scripts; no regex is involved.
func sentenceWindow(content string, hit [2]int, budget int) [2]int {
	lo := hit[0]
	for lo > 0 && hit[0]-lo < budget {
		r, size := utf8.DecodeLastRuneInString(content[:lo])
		if isSentenceTerminator(r) {
			break
		}
		lo -= size
	}
	hi := hit[1]
	for hi < len(content) && hi-hit[1] < budget {
		r, size := utf8.DecodeRuneInString(content[hi:])
		hi += size
		if isSentenceTerminator(r) {
			break
		}
	}
	return [2]int{lo, hi}
}

// isSentenceTerminator reports whether a rune ends a sentence in either script.
func isSentenceTerminator(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '\n', '!', '?', ';', '.':
		return true
	}
	return false
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

// grepPatternSyntax are the constructs that make a query a PATTERN rather than a
// phrase: an alternation, an ordering constraint, or a word boundary.
//
// They are read from the model's own string and mean what they mean in every
// regex dialect there is, so nothing has to be compiled, escaped or translated —
// and none of them occurs in ordinary prose, which is what keeps the guard safe:
// a plain question is not a pattern and takes the term-locate path it always took.
var grepPatternSyntax = []string{"|", ".*", ".+", `\b`}

// grepPatternOf compiles the query as a pattern, or returns nil when the query
// carries no pattern syntax.
func grepPatternOf(query string) *regexp.Regexp {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	patterned := false
	for _, tok := range grepPatternSyntax {
		if strings.Contains(q, tok) {
			patterned = true
			break
		}
	}
	if !patterned {
		return nil
	}
	re, err := regexp.Compile("(?i)" + q)
	if err != nil {
		return nil
	}
	return re
}

// grepPatternOperators are the characters a pattern uses as operators. They are
// removed before the operands are derived, because a keyword leg cannot search
// for syntax.
var grepPatternOperators = []string{".*", ".+", "|", ".", "*", "+", "?", "^", "$", `\b`, `\d`, `\w`, `\s`, "(", ")", "[", "]", "{", "}", `\`}

// GrepPatternOperands returns the literal runs a PATTERN asks the corpus for.
//
// A pattern's operands are what a keyword leg can search: "华雄|荀正" names two,
// "关公.*斩" names two, and the operators between them are not terms. They cannot
// go through the phrase path (GrepTermsFromQuery), which reads an unbroken CJK run
// as a clause and decomposes it into windows — on "关公.*斩" that yielded 关公 alone
// and dropped the 斩, so recall never asked about half the pattern.
//
// Single CJK runes are KEPT here, unlike the general two-rune floor: inside a
// pattern the caller wrote that literal deliberately, so it is not the stray
// particle the floor exists to drop.
func GrepPatternOperands(query string) []string {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	for _, op := range grepPatternOperators {
		q = strings.ReplaceAll(q, op, " ")
	}
	var out []string
	seen := make(map[string]bool, GrepTermsMax)
	for _, tok := range strings.Fields(q) {
		tok = trimTermEdges(tok)
		if tok == "" {
			continue
		}
		low := strings.ToLower(tok)
		if seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, tok)
		if len(out) >= GrepTermsMax {
			break
		}
	}
	return out
}

// matchGrepPattern is the grep: it runs the PATTERN over candidate content and
// keeps the candidates that MATCH, each narrowed to the clause its match sits in.
//
// This is the half of grep_search that a term-locate pass cannot be: a candidate
// the pattern does not match is not evidence, whatever its retrieval score, and
// the pattern keeps its own semantics — `A|B` accepts either alternative, while
// `A.*B` requires the written order, which is how one asks for the WAY a thing
// was done instead of for its name.
//
// The haystack is the candidate set the keyword leg returned (see
// retrieveGrepCandidates), so the pattern is applied to what the engine could
// reach, exactly: a name the candidates carry is found even if the phrase's
// ranking would have buried it, and no pattern syntax ever reaches the engine —
// which is why `|` and `.*` need no escaping anywhere on this path.
//
// The window is the sentence around the FIRST match, so the model reads the
// clause that carries the term instead of a whole chunk.
//
// Returns the matching candidates plus how many matched; the caller keeps the raw
// candidates when nothing matched, so evidence is never dropped.
func matchGrepPattern(chunks []map[string]any, re *regexp.Regexp, budget, maxOutTotalChars int) ([]map[string]any, int) {
	if re == nil || len(chunks) == 0 {
		return nil, 0
	}
	if budget <= 0 {
		budget = contextCharBudget
	}
	var kept []map[string]any
	used := 0
	for _, c := range chunks {
		text := ChunkTextOf(c)
		if text == "" {
			continue
		}
		span := re.FindStringIndex(text)
		if span == nil {
			continue
		}
		win := sentenceWindow(text, [2]int{span[0], span[1]}, budget)
		fragment := text[win[0]:win[1]]
		if maxOutTotalChars > 0 {
			n := utf8.RuneCountInString(fragment)
			if used+n > maxOutTotalChars && len(kept) > 0 {
				break
			}
			used += n
		}
		kept = append(kept, withNarrowedText(cloneMap(c), fragment))
	}
	return kept, len(kept)
}

// logGrepReach prints what ONE grep reached, term by term, and what nothing
// reached — the fact the model cannot read off the passages themselves.
//
// Every name in a batch looks the same whether the search found it or never
// looked, and the wording matters: "not reached by THIS query" is not "absent
// from the corpus". A model that reads a miss as absence stops enumerating, which
// is the failure this line exists to prevent.
func logGrepReach(logger *log.Logger, query string, candidates []map[string]any, terms []string) {
	if logger == nil {
		return
	}
	if body := reachBody(query, candidates, terms); body != "" {
		logger.Printf("[Grep search] %s", body)
	}
}

// GrepReachLine is reachBody, prefixed for the MODEL.
//
// The reach report was log-only, and that was the last piece of the loop missing:
// a batch of names ("华雄|颜良|蔡阳") came back as passages, every name looking the
// same whether the query reached it or never looked — so the loop could not do
// what a search-driven loop does with it, act on WHICH alternative came back
// empty. The engine, the pattern matcher and the per-term accounting already
// existed; only the reader was missing.
func GrepReachLine(query string, candidates []map[string]any, terms []string) string {
	body := reachBody(query, candidates, terms)
	if body == "" {
		return ""
	}
	return "[reach] " + body
}

// reachBody renders what ONE query reached, term by term, and what nothing
// reached. Empty when there are no terms to report on.
func reachBody(query string, candidates []map[string]any, terms []string) string {
	if len(terms) == 0 {
		return ""
	}
	located, counts, absent := termReach(candidates, terms)
	parts := make([]string, 0, len(located))
	for i, t := range located {
		parts = append(parts, fmt.Sprintf("%s(%d)", t, counts[i]))
	}
	line := fmt.Sprintf("%d candidate(s) for %q carry: %s", len(candidates), trunc(query, 60), strings.Join(parts, " "))
	if len(absent) > 0 {
		line += fmt.Sprintf(" | NOT reached by this query (not necessarily absent from the corpus): %s", strings.Join(absent, " "))
	}
	return line
}

// ReachTermsOf returns the terms a query's reach is reported over: the operands
// for a PATTERN (its operands are what a keyword leg searched for), the extracted
// terms otherwise. It mirrors the choice GrepSearch makes, so the line the model
// reads and the line the log carries describe the same search.
func ReachTermsOf(query string) []string {
	if grepPatternOf(query) != nil {
		return GrepPatternOperands(query)
	}
	return GrepTermsFromQuery(query)
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
