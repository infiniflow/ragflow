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

func TestNormalizeTextNewlines(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"a\nb", "a\nb"},
		{"a\r\nb", "a\nb"},
		{"a\rb", "a\nb"},
		{"a\r\nb\rc", "a\nb\nc"},
	}
	for _, tc := range tests {
		if got := NormalizeTextNewlines(tc.in); got != tc.want {
			t.Errorf("NormalizeTextNewlines(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

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

func TestParseDelimiterField(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  []string
	}{
		{"empty", "", nil},
		{"single bare", "!", []string{"!"}},
		{"bare pair", "!?", []string{"!", "?"}},
		{"space", " ", []string{" "}},
		{"newline", "\n", []string{"\n"}},
		{"crlf field", "\r\n", []string{"\n"}},
		{"bare cr", "\r", []string{"\n"}},
		{"wrapped end", "`end`", []string{"end"}},
		{"wrapped hierarchy", "`###``##``#`", []string{"###", "##", "#"}},
		{"tooltip example", "\n`##`;", []string{"##", "\n", ";"}},
		{"dedupe", "`a`a`a`", []string{"a"}},
		{"wrapped double newline", "`\n\n`", []string{"\n\n"}},
		{"crlf wrapped", "`\r\n`", []string{"\n"}},
		{"shipped default", "\n!?;。；！？", []string{"\n", "!", "?", ";", "。", "；", "！", "？"}},
		{"chinese", "。；", []string{"。", "；"}},
		{"mixed order equal length", "`##`#\n", []string{"##", "#", "\n"}},
		{"empty backticks bare", "``", []string{"`"}},
		{"double space bare dedupe", "  ", []string{" "}},
		{"wrapped double space", "`  `", []string{"  "}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseDelimiterField(tc.field)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseDelimiterField(%q) = %#v, want %#v", tc.field, got, tc.want)
			}
		})
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
	pat := CompileDelimiterPattern(ParseDelimiterField("\n!?;。；！？"))
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
	pat := CompileDelimiterPattern(ParseDelimiterField("a"))
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

func TestHasCustomDelimiterList(t *testing.T) {
	if HasCustomDelimiterList([]string{"\n", "!"}) {
		t.Fatal("bare list should be false")
	}
	if !HasCustomDelimiterList([]string{"\n", "`##`"}) {
		t.Fatal("wrapped list entry should be true")
	}
}
