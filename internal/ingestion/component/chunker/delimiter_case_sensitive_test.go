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

// Regression tests for case-sensitive delimiter parsing (#17384 / PR #17386).
//
// Locks Go parity with the Python fix that dropped re.I from get_delimiters
// and parser_txt. Delimiter matching must preserve letter casing:
//
//   - bare "a" matches only "a", not "A"
//   - backtick-wrapped "`end`" matches only "end", not "End"/"END"/etc.
//
// Go's regexp package is case-sensitive by default; these tests guard against
// a future (?i) flag or case-folding change creeping into the delimiter path.

package chunker

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/parser/chunk"
)

func getDelimiters(delimiters string) string {
	// Mirror the live path: backtick-wrapped entries contribute their inner
	// content (see chunk.CompileDelimiterPatternList).
	p := chunk.CompileDelimiterPatternList([]string{delimiters}, true)
	if p == nil {
		return ""
	}
	return p.String()
}

func TestGetDelimiters_BareCharAReturnsLiteralPattern(t *testing.T) {
	// Bare-char delimiter "a" must produce the pattern "a", not "a|A".
	if got, want := getDelimiters("a"), "a"; got != want {
		t.Fatalf("getDelimiters(%q) = %q, want %q", "a", got, want)
	}
}

func TestGetDelimiters_BareCharAUpperReturnsLiteralPattern(t *testing.T) {
	if got, want := getDelimiters("A"), "A"; got != want {
		t.Fatalf("getDelimiters(%q) = %q, want %q", "A", got, want)
	}
}

func TestGetDelimiters_BacktickEndReturnsExactToken(t *testing.T) {
	// Backtick-wrapped delimiter must preserve the captured group verbatim.
	if got, want := getDelimiters("`end`"), "end"; got != want {
		t.Fatalf("getDelimiters(%q) = %q, want %q", "`end`", got, want)
	}
}

func TestGetDelimiters_PatternSplitsCaseSensitively(t *testing.T) {
	// The pattern returned by getDelimiters must split case-sensitively.
	pat := getDelimiters("a")
	re := regexp.MustCompile("(" + pat + ")")
	// Only the lowercase 'a' splits; uppercase 'A' is preserved intact.
	got := splitKeepingCapture("AaBb", re)
	want := []string{"A", "a", "Bb"}
	if len(got) != len(want) {
		t.Fatalf("split = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("split[%d] = %q, want %q (full=%#v)", i, got[i], want[i], got)
		}
	}
}

// splitKeepingCapture mirrors Python re.split("("+pat+")", text):
// returns [text, delim, text, delim, ...] including empty leading/trailing.
func splitKeepingCapture(text string, re *regexp.Regexp) []string {
	idxs := re.FindAllStringIndex(text, -1)
	if len(idxs) == 0 {
		return []string{text}
	}
	var out []string
	cursor := 0
	for _, idx := range idxs {
		start, end := idx[0], idx[1]
		out = append(out, text[cursor:start])
		out = append(out, text[start:end])
		cursor = end
	}
	out = append(out, text[cursor:])
	return out
}

func TestCompileDelimPattern_BacktickEndIsCaseSensitive(t *testing.T) {
	p := compileDelimPattern([]string{"`end`"})
	if p == nil {
		t.Fatal("compileDelimPattern(`end`) returned nil")
	}
	if p.MatchString("End") || p.MatchString("END") || p.MatchString("eNd") {
		t.Fatalf("pattern %q must not match case variants of end", p.String())
	}
	if !p.MatchString("end") {
		t.Fatalf("pattern %q must match exact lowercase end", p.String())
	}
	// Pattern string itself must not carry a case-insensitive flag.
	if strings.HasPrefix(p.String(), "(?i)") || strings.Contains(p.String(), "(?i)") {
		t.Fatalf("compileDelimPattern must not emit (?i); got %q", p.String())
	}
}

func TestCompileDelimPattern_BareCharIsNotActivePattern(t *testing.T) {
	// Plain delimiters are not compiled — only backtick-wrapped tokens are.
	if p := compileDelimPattern([]string{"a"}); p != nil {
		t.Fatalf("compileDelimPattern([a]) = %v, want nil", p)
	}
}

func TestCompileDelimPattern_ExtractsPerEntryIndependently(t *testing.T) {
	// Adjacent entries must not form a cross-boundary backtick pair.
	// Concatenating "`aa" + "`bb`" would invent an "aa`bb" token; per-entry
	// extraction only sees the complete "`bb`" pair in the second entry.
	p := compileDelimPattern([]string{"`aa", "`bb`"})
	if p == nil {
		t.Fatal("expected pattern from second entry `bb`")
	}
	if got := p.String(); got != "bb" {
		t.Fatalf("pattern = %q, want %q (no cross-entry token)", got, "bb")
	}
	if p.MatchString("aa") {
		t.Fatalf("must not match incomplete first-entry token; pattern=%q", p.String())
	}
	if !p.MatchString("bb") {
		t.Fatalf("must match token from second entry; pattern=%q", p.String())
	}

	// Multiple well-formed entries still combine.
	p2 := compileDelimPattern([]string{"`end`", "`foo`"})
	if p2 == nil {
		t.Fatal("expected combined pattern")
	}
	if !p2.MatchString("end") || !p2.MatchString("foo") {
		t.Fatalf("combined pattern %q must match both tokens", p2.String())
	}
}

func TestTokenChunker_BacktickEndSplitsOnlyAtLowercase(t *testing.T) {
	// End-to-end: delimiter-mode with "`end`" must split only at lowercase
	// "end", leaving "End" / "END" intact inside the following segment.
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"`end`"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "the end and End and END come",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	got := make([]string, 0, len(chunks))
	for _, ck := range chunks {
		text, _ := ck["text"].(string)
		text = strings.TrimSpace(text)
		if text != "" {
			got = append(got, text)
		}
	}
	// Python's _split_text_by_pattern drops the matched delimiter and
	// .strip()s each segment: "the" | "and End and END come"
	want := []string{"the", "and End and END come"}
	if len(got) != len(want) {
		t.Fatalf("chunks = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk[%d] = %q, want %q (full=%#v)", i, got[i], want[i], got)
		}
	}
}

func TestTokenChunker_BacktickASplitsOnlyAtLowercase(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode": "delimiter",
		"delimiters":     []string{"`a`"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "BaAb",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	got := make([]string, 0, len(chunks))
	for _, ck := range chunks {
		text, _ := ck["text"].(string)
		text = strings.TrimSpace(text)
		if text != "" {
			got = append(got, text)
		}
	}
	// "B" + "a" glued → "Ba"; remainder "Ab". Python drops the matched
	// delimiter and .strip()s, leaving "B" | "Ab".
	want := []string{"B", "Ab"}
	if len(got) != len(want) {
		t.Fatalf("chunks = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunk[%d] = %q, want %q (full=%#v)", i, got[i], want[i], got)
		}
	}
}

func TestBacktickDelimiterIsCaseSensitive(t *testing.T) {
	// Extraction and compiled pattern must both preserve letter casing.
	p := chunk.CompileDelimiterPatternList([]string{"`End`"}, true)
	if p == nil {
		t.Fatal("CompileDelimiterPatternList returned nil")
	}
	if !p.MatchString("End") || p.MatchString("end") || p.MatchString("END") {
		t.Fatalf("pattern %q is not case-sensitive", p.String())
	}
	if strings.Contains(p.String(), "(?i)") {
		t.Fatalf("compiled pattern must not carry (?i); got %q", p.String())
	}
}
