//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package chunk

import (
	"reflect"
	"regexp"
	"testing"
)

func TestHasWrappedDelimiter(t *testing.T) {
	if HasWrappedDelimiter("") {
		t.Fatal("empty should be false")
	}
	if HasWrappedDelimiter("!;") {
		t.Fatal("bare chars should be false")
	}
	if !HasWrappedDelimiter("`##`") {
		t.Fatal("wrapped should be true")
	}
	if !HasWrappedDelimiter("\n`##`;") {
		t.Fatal("mixed should be true")
	}
	if !HasWrappedDelimiter("`;`") {
		t.Fatal("wrapped single char should be true")
	}
}

func TestCompileDelimiterPattern(t *testing.T) {
	if CompileDelimiterPattern(nil) != nil {
		t.Fatal("empty list should yield nil")
	}
	if CompileDelimiterPattern([]string{}) != nil {
		t.Fatal("empty slice should yield nil")
	}

	pat := CompileDelimiterPattern([]string{"##", "#"})
	if pat == nil {
		t.Fatal("expected pattern")
	}
	if got := pat.FindString("###"); got != "##" {
		t.Errorf("longest match: got %q, want ##", got)
	}

	// Metacharacters match literally.
	for _, ch := range []string{".", "(", "?", "+"} {
		p := CompileDelimiterPattern([]string{ch})
		if p == nil || !p.MatchString(ch) {
			t.Errorf("pattern for %q should match itself", ch)
		}
		if ch != "." && p.MatchString("z") {
			t.Errorf("pattern for %q should not match z", ch)
		}
	}
}

func TestCompileDelimiterPatternShippedDefault(t *testing.T) {
	pat := CompileDelimiterPattern([]string{"\n", "!", "?", ";", "。", "；", "！", "？"})
	if pat == nil {
		t.Fatal("expected pattern")
	}
	for _, ch := range []string{"\n", "!", "?", ";", "。", "；", "！", "？"} {
		if !pat.MatchString(ch) {
			t.Errorf("default pattern must match %q", ch)
		}
	}
	if pat.MatchString("a") {
		t.Error("default pattern must not match a")
	}
}

func TestCompileDelimiterPatternCaseSensitive(t *testing.T) {
	pat := CompileDelimiterPattern([]string{"a"})
	parts := regexp.MustCompile("("+pat.String()+")").Split("AaBb", -1)
	// Split removes matches; "A" + "Bb" with "a" consumed.
	if len(parts) < 2 {
		t.Fatalf("parts=%v", parts)
	}
	// Only lowercase a splits.
	re := regexp.MustCompile("(" + pat.String() + ")")
	got := re.Split("AaBb", -1)
	want := []string{"A", "Bb"}
	// re.Split in Go does not keep delimiters; with capturing group behavior
	// differs — just check case sensitivity via FindAll.
	matches := pat.FindAllString("AaBb", -1)
	if !reflect.DeepEqual(matches, []string{"a"}) {
		t.Errorf("matches=%v want [a]; split parts=%v want-ish %v", matches, got, want)
	}
}

func TestCompileDelimiterListPattern(t *testing.T) {
	// Bare list entries are ignored (TokenChunker list API).
	if CompileDelimiterListPattern([]string{"\n", "!"}) != nil {
		t.Fatal("bare list entries should not produce a pattern")
	}
	pat := CompileDelimiterListPattern([]string{"`##`", "`#`"})
	if pat == nil {
		t.Fatal("expected pattern from wrapped list entries")
	}
	if got := pat.FindString("###"); got != "##" {
		t.Errorf("got %q want ##", got)
	}

	// Verify unescaped rune length sorting order: "a.b" (3 runes) vs "ab" (2 runes with meta chars)
	patMeta := CompileDelimiterListPattern([]string{"`a`", "`a.b`"})
	if patMeta.String() != `a\.b|a` {
		t.Errorf("got pattern %q, want %q", patMeta.String(), `a\.b|a`)
	}
}

func TestCompileDelimiterPatternListKeepBare(t *testing.T) {
	// keepBare=true (children_delimiters): bare entries stay active and
	// backtick entries contribute their inner content, all QuoteMeta'd and
	// sorted longest-first by rune count.
	pat := CompileDelimiterPatternList([]string{". ", "`###`", "#"}, true)
	if pat == nil {
		t.Fatal("expected a pattern from mixed bare + wrapped entries")
	}
	want := `###|\. |#`
	if got := pat.String(); got != want {
		t.Errorf("pattern = %q, want %q", got, want)
	}
	// Backtick is stripped: splitting on "###" matches the inner content, not
	// the literal wrapped token.
	if got := pat.FindString("a###b"); got != "###" {
		t.Errorf("FindString(a###b) = %q, want %q", got, "###")
	}
	// Bare ". " stays active and is matched as the meta-escaped ". ".
	if got := pat.FindString("alpha. beta"); got != ". " {
		t.Errorf("FindString(alpha. beta) = %q, want %q", got, ". ")
	}
}

func TestCompileDelimiterPatternListKeepBareFalse(t *testing.T) {
	// keepBare=false must equal CompileDelimiterListPattern: bare entries
	// ignored, only wrapped inner content participates.
	if CompileDelimiterPatternList([]string{"\n", "!"}, false) != nil {
		t.Fatal("bare entries should be ignored when keepBare=false")
	}
	pat := CompileDelimiterPatternList([]string{"`##`", "`#`"}, false)
	if pat == nil {
		t.Fatal("expected pattern from wrapped entries")
	}
	if got := pat.FindString("###"); got != "##" {
		t.Errorf("got %q want ##", got)
	}
}

func TestCompileDelimiterPatternListDedup(t *testing.T) {
	// Equivalent quoted + bare delimiters must not produce a redundant
	// alternation. ["`#`", "#"] with keepBare=true resolves to the single
	// active value `#`.
	pat := CompileDelimiterPatternList([]string{"`#`", "#"}, true)
	if pat == nil {
		t.Fatal("expected a pattern from equivalent quoted + bare entries")
	}
	if got := pat.String(); got != "#" {
		t.Errorf("dedup pattern = %q, want %q", got, "#")
	}
	// Repeated wrapped entries are also deduplicated.
	pat = CompileDelimiterPatternList([]string{"`##`", "`##`", "`#`"}, false)
	if pat == nil {
		t.Fatal("expected a pattern from wrapped entries")
	}
	if got := pat.String(); got != `##|#` {
		t.Errorf("dedup pattern = %q, want %q", got, `##|#`)
	}
}

func TestCompileDelimiterPatternListDedupOrderIndependent(t *testing.T) {
	// Dedup must hold regardless of input order: wrapped entries key on their
	// inner content while bare entries key on themselves, so a wrapped/bare
	// collision collapses no matter which appears first.
	pat := CompileDelimiterPatternList([]string{"#", "`#`"}, true)
	if pat == nil {
		t.Fatal("expected a pattern from equivalent bare + quoted entries")
	}
	if got := pat.String(); got != "#" {
		t.Errorf("dedup pattern = %q, want %q", got, "#")
	}
	// Multiple collisions across mixed order collapse to one alternation.
	pat = CompileDelimiterPatternList([]string{"`#`", "#", "`#`", "#"}, true)
	if got := pat.String(); got != "#" {
		t.Errorf("dedup pattern = %q, want %q", got, "#")
	}
	// Wrapped duplicates deduplicate no matter where they appear.
	pat = CompileDelimiterPatternList([]string{"`##`", "`#`", "`##`"}, false)
	if got := pat.String(); got != `##|#` {
		t.Errorf("dedup pattern = %q, want %q", got, `##|#`)
	}
}

func TestCompileDelimiterListPatternDedup(t *testing.T) {
	// CompileDelimiterListPattern is the main-delimiter live path
	// (keepBare=false); it must propagate the same dedup so the main
	// delimiters list cannot accumulate redundant alternations either.
	pat := CompileDelimiterListPattern([]string{"`##`", "`##`", "`#`"})
	if pat == nil {
		t.Fatal("expected a pattern from wrapped entries")
	}
	if got := pat.String(); got != `##|#` {
		t.Errorf("dedup pattern = %q, want %q", got, `##|#`)
	}
	// Bare entries are ignored on this path, so only the wrapped inner content
	// participates and is deduplicated.
	pat = CompileDelimiterListPattern([]string{"`#`", "#"})
	if got := pat.String(); got != "#" {
		t.Errorf("dedup pattern = %q, want %q", got, "#")
	}
}

func TestCompileDelimiterPatternListChildrenPrefixOrder(t *testing.T) {
	// keepBare=true (children_delimiters): a longer delimiter must win over a
	// shorter prefix inside it because entries are sorted longest-first by
	// rune count.
	pat := CompileDelimiterPatternList([]string{"##", "#"}, true)
	if pat == nil {
		t.Fatal("expected a pattern from bare prefix entries")
	}
	if got := pat.String(); got != `##|#` {
		t.Errorf("pattern = %q, want %q", got, `##|#`)
	}
	// Longest-first ordering ensures "###" splits on "##", not "#".
	if got := pat.FindString("a###b"); got != "##" {
		t.Errorf("FindString(a###b) = %q, want %q", got, "##")
	}
	// Multi-byte delimiter ordering is by rune count, not byte length:
	// "###" (3 runes) must be tried before "。" (1 rune).
	pat = CompileDelimiterPatternList([]string{"。", "###"}, true)
	if got := pat.String(); got != `###|。` {
		t.Errorf("pattern = %q, want %q", got, `###|。`)
	}
}

func TestHasCustomDelimiterList(t *testing.T) {
	if HasCustomDelimiterList([]string{"\n", "!"}) {
		t.Fatal("bare list should be false")
	}
	if !HasCustomDelimiterList([]string{"\n", "`##`"}) {
		t.Fatal("wrapped list entry should be true")
	}
}
