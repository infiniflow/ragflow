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
	"testing"
	"unicode/utf8"
)

func TestEscapeTermAsciiWord(t *testing.T) {
	got := EscapeTerm("cat")
	if got != `\bcat\b` {
		t.Errorf("EscapeTerm(cat) = %q, want \\bcat\\b (case-insensitivity is added by TermsToPatterns via (?i))", got)
	}
	if _, err := regexp.Compile(got); err != nil {
		t.Errorf("bad regexp: %v", err)
	}
}

func TestEscapeTermCJK(t *testing.T) {
	got := EscapeTerm("机器")
	if got != "机器" {
		t.Errorf("EscapeTerm(机器) = %q, want bare CJK", got)
	}
	if _, err := regexp.Compile(got); err != nil {
		t.Errorf("bad regexp: %v", err)
	}
}

func TestEscapeTermSymbols(t *testing.T) {
	got := EscapeTerm("c++")
	// + is regex-special -> escaped to "\+".
	if got != `c\+\+` {
		t.Errorf("EscapeTerm(c++) = %q, want c\\+\\+", got)
	}
	re, err := regexp.Compile(got)
	if err != nil {
		t.Errorf("bad regexp: %v", err)
	}
	if !re.MatchString("c++") {
		t.Error("pattern should match literal c++")
	}
	if re.MatchString("c--") {
		t.Error("pattern should NOT match c--")
	}
}

func TestEscapeTermEmpty(t *testing.T) {
	if EscapeTerm("") != "" {
		t.Error("empty term must stay empty so TermsToPatterns can skip it")
	}
}

func TestTermsToPatternsCapsAndSkipsEmpty(t *testing.T) {
	// 40 terms -> capped at maxGrepTerms (16); empties skipped.
	terms := make([]string, 40)
	for i := range terms {
		terms[i] = "t"
	}
	pats := TermsToPatterns(terms)
	if len(pats) != maxGrepTerms {
		t.Errorf("len = %d, want capped at %d", len(pats), maxGrepTerms)
	}
	// Mixed empties (whitespace) are skipped.
	pats = TermsToPatterns([]string{"", "  ", ",,,", "cat"})
	if len(pats) != 1 {
		t.Errorf("len = %d, want 1 (only 'cat' compiles)", len(pats))
	}
}

// mustNarrow is a convenience for the full NarrowByTerms signature with the
// default context and caps used by the term path.
func mustNarrow(chunks []map[string]any, terms, fallbackTerms []string, keywords string) NarrowResult {
	return NarrowByTerms(chunks, terms, fallbackTerms, keywords, NarrowContext{Before: 2, After: 2}, defaultOutCharsPerChunk, defaultOutTotalChars)
}

func TestNarrowByTermsKeepsAllChunks(t *testing.T) {
	// NarrowByTerms NEVER drops a chunk: short chunks are kept whole, long ones
	// are narrowed to the hit window. Only Matched / the returned text vary.
	chunks := []map[string]any{
		{"content": "Culdcept was made by OmiyaSoft in 1999."},
		{"content": "The weather is nice today."},
		{"content": "OmiyaSoft released Culdcept."},
	}
	res := mustNarrow(chunks, []string{"culdcept", "omiya"}, nil, "")
	if len(res.Kept) != len(chunks) {
		t.Fatalf("Kept len = %d, want all %d kept", len(res.Kept), len(chunks))
	}
	if !res.Stats.Matched {
		t.Error("Matched = false, want true (terms hit)")
	}
	if res.Stats.ChunksKpt != len(chunks) {
		t.Errorf("ChunksKpt = %d, want %d", res.Stats.ChunksKpt, len(chunks))
	}
}

func TestNarrowByTermsFallbackTermsTriedOnNoPrimaryHit(t *testing.T) {
	// Primary terms hit nothing -> the fallback term list is tried mechanically.
	chunks := []map[string]any{
		{"content": "Culdcept was made by OmiyaSoft."},
		{"content": "The weather is nice today."},
	}
	res := mustNarrow(chunks, []string{"zzzz"}, []string{"culdcept"}, "")
	if !res.Stats.Matched {
		t.Error("Matched = false, want true after the fallback term path hit")
	}
	if len(res.Kept) != len(chunks) {
		t.Errorf("Kept len = %d, want both retained", len(res.Kept))
	}
}

func TestNarrowWithFallbackKeywordAndOriginals(t *testing.T) {
	chunks := []map[string]any{
		{"content": "Culdcept was made by OmiyaSoft."},
		{"content": "The weather is nice today."},
	}
	// Keyword narrowing matches -> narrowed subset.
	if got := NarrowWithFallbackKeyword(chunks, "culdcept"); len(got) != 1 {
		t.Errorf("keyword fallback kept %d, want 1 (only the matching chunk)", len(got))
	}
	// No keyword -> originals kept (never drop).
	if got := NarrowWithFallbackKeyword(chunks, ""); len(got) != len(chunks) {
		t.Errorf("len = %d, want all %d on empty keyword", len(got), len(chunks))
	}
}

func TestIsFactDenseSentence(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"Culdcept was released in 1999 by OmiyaSoft with 50 cards.", true}, // year + digit + proper nouns
		{"Revenue grew by 5% this quarter.", true},                          // percent
		{"A 3.14 version was shipped.", true},                               // decimal number
		{"The project spans over two million users.", true},                 // magnitude word (case-insensitive)
		{"Culdcept was released by OmiyaSoft.", true},                       // proper noun
		// A quoted span or a ≥6-token clause is NOT a fact signal on its own
		// (Python _is_fact_dense_sentence has no such rules).
		{"He said \"hello\" to the group about the meeting.", false},      // quoted only
		{"the cat sat on the mat and looked at the bird outside.", false}, // >=6 tokens only
		{"the cat sat", false}, // short, no signal
		// Abbreviation guard: a proper noun right after ".", "!" or "?" followed by
		// a dot is not counted (Python's (?<![.!?]\.) lookbehind). The lead "It" is
		// only one lowercase letter so it is not a proper noun either; Atlas is the
		// sole candidate and it is excluded.
		{"It was wonderful!!..Atlas shrugged", false},
	}
	for _, c := range cases {
		if got := IsFactDenseSentence(c.s); got != c.want {
			t.Errorf("IsFactDenseSentence(%q) = %v, want %v", c.s, got, c.want)
		}
	}
}

func TestNarrowByTermsNilInput(t *testing.T) {
	res := mustNarrow(nil, nil, nil, "")
	if res.Kept != nil || res.Stats.ChunksIn != 0 {
		t.Errorf("nil input -> Kept len %d, ChunksIn %d, want (nil, 0)", len(res.Kept), res.Stats.ChunksIn)
	}
}

// TestNarrowByKeywordsEmptyReturnsChunks pins the divergence from Python
// _narrow_by_keywords: `if not kwds or not chunks: return chunks`. An empty
// keyword string must return the input chunks unchanged, NOT an empty/nil
// slice that would wipe the whole evidence pool.
func TestNarrowByKeywordsEmptyReturnsChunks(t *testing.T) {
	chunks := []map[string]any{
		{"content": "some passage"},
		{"content_with_weight": "other", "content": "other"},
	}
	if got := NarrowByKeywords(chunks, ""); len(got) != len(chunks) {
		t.Fatalf("empty keyword kept %d, want all %d", len(got), len(chunks))
	}
	if got := NarrowByKeywords(nil, "culdcept"); got != nil {
		t.Errorf("nil chunks must return nil, got %d", len(got))
	}
}

// TestNarrowByKeywordsMirrorsContentOnlyWhenPresent pins the two text-writing
// rules of Python _narrow_by_keywords: content_with_weight is always overwritten
// with the narrowed text, "content" is mirrored ONLY when the chunk already had
// a "content" key, and a pre-narrow "highlight" key is dropped (no longer valid).
func TestNarrowByKeywordsMirrorsContentOnlyWhenPresent(t *testing.T) {
	// Chunk with only content_with_weight (no "content"): narrowing keeps
	// content_with_weight, and must NOT invent a "content" key.
	chunks := []map[string]any{
		{"content_with_weight": "Alpha foo bar. Culdcept is a game by OmiyaSoft. Beta baz qux."},
	}
	got := NarrowByKeywords(chunks, "culdcept")
	if len(got) != 1 {
		t.Fatalf("want 1 narrowed chunk, got %d", len(got))
	}
	if _, ok := got[0]["content"]; ok {
		t.Error("narrowing must NOT add a 'content' key when the source lacked one")
	}
	if got[0]["content_with_weight"] == chunks[0]["content_with_weight"] {
		t.Error("content_with_weight was not narrowed")
	}
	if _, ok := got[0]["highlight"]; ok {
		t.Error("'highlight' must be dropped after narrowing")
	}

	// Chunk that has a pre-narrow "highlight": it must be removed.
	chunks2 := []map[string]any{
		{"content": "Culdcept was made by OmiyaSoft.", "highlight": []any{"old"}},
	}
	got2 := NarrowByKeywords(chunks2, "culdcept")
	if len(got2) != 1 {
		t.Fatalf("want 1 narrowed chunk, got %d", len(got2))
	}
	if _, ok := got2[0]["highlight"]; ok {
		t.Error("'highlight' must be dropped after narrowing")
	}
	if got2[0]["content"] == chunks2[0]["content"] {
		t.Error("'content' was not narrowed")
	}
}

func TestNarrowByTermsNoMatchKeepsOriginalUntouched(t *testing.T) {
	// A long chunk that the grep terms never hit must be returned verbatim
	// (full text, no head-truncation), exactly like Python _apply_narrow's
	// else branch. This guards against the old bug where unmatched evidence
	// was clobbered with a head-truncated copy.
	long := "The weather was calm and the birds were singing while the river flowed gently past the old stone bridge under a pale morning sky that promised a quiet day for the villagers who had risen early to tend their small gardens and fields. " +
		"The old mill stood silent at the edge of the wood where the children used to play among the ferns and the brook that chattered over smooth grey stones all through the long golden afternoons of a summer that nobody wanted to end."
	if len(long) <= headFallbackChars {
		t.Fatalf("test fixture too short: %d bytes, need > %d", len(long), headFallbackChars)
	}
	chunks := []map[string]any{
		{"content_with_weight": "Culdcept was created by OmiyaSoft and released in 1999.", "content": "Culdcept was created by OmiyaSoft and released in 1999."},
		{"content_with_weight": long, "content": long},
	}
	res := mustNarrow(chunks, []string{"culdcept", "omiya"}, nil, "")
	if !res.Stats.Matched {
		t.Fatal("expected a match on the first chunk")
	}
	if len(res.Kept) != 2 {
		t.Fatalf("Kept = %d, want 2", len(res.Kept))
	}
	if got := ChunkTextOf(res.Kept[1]); got != long {
		t.Errorf("non-matching chunk was altered (len %d, want %d); grep narrowing must not truncate unmatched evidence", len(got), len(long))
	}
}

func TestTruncHeadRuneAware(t *testing.T) {
	// CJK: 3 bytes per rune. A naive byte slice s[:n] would split a rune and
	// produce invalid UTF-8; truncHead must keep n codepoints instead.
	zh := strings.Repeat("中", 50)
	got := truncHead(zh, 10)
	if charLen(got) != 10 {
		t.Errorf("truncHead codepoint count = %d, want 10", charLen(got))
	}
	if got != strings.Repeat("中", 10) {
		t.Errorf("truncHead did not keep the correct rune prefix")
	}
	if !utf8.ValidString(got) {
		t.Error("truncHead produced invalid UTF-8")
	}
	// Short input: returned unchanged and byte-identical.
	if s := "hello"; truncHead(s, 100) != s {
		t.Errorf("truncHead short input changed: %q", truncHead(s, 100))
	}
}
