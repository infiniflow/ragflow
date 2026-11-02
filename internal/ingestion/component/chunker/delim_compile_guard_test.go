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

package chunker

import (
	"testing"

	"ragflow/internal/parser/chunk"
)

// TestCompileDelimPatternContract pins the shared delimiter-compiler contract
// that both the main Delimiters list and the children_delimiters list go
// through (compileChildrenPattern was merged into compileDelimPattern because
// the two were byte-identical). The assertions follow
// chunk.CompileDelimiterPatternList: empty entries are inactive, backtick
// entries contribute their inner content, bare entries stay active, duplicates
// collapse, and the alternation is longest-first and regex-escaped.
func TestCompileDelimPatternContract(t *testing.T) {
	cases := []struct {
		name   string
		delims []string
		want   string // expected pattern string; "" means nil pattern
	}{
		{"nil list", nil, ""},
		{"empty list", []string{}, ""},
		{"only empty entry", []string{""}, ""},
		{"only empty backticks", []string{"``"}, ""},
		{"lone backtick is bare", []string{"`"}, "`"},
		{"single bare", []string{"a"}, "a"},
		{"backtick inner", []string{"`end`"}, "end"},
		{"bare duplicates collapse", []string{"a", "a"}, "a"},
		{"backtick and bare dedupe", []string{"`a`", "a"}, "a"},
		{"empty among actives", []string{"", "b", ""}, "b"},
		{"longest first", []string{"ab", "`abc`"}, `abc|ab`},
		{"regex metacharacters escaped", []string{"a.b"}, `a\.b`},
		{"unicode delimiters", []string{"。", "，"}, `。|，`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := compileDelimPattern(c.delims)
			if c.want == "" {
				if got != nil {
					t.Fatalf("compileDelimPattern(%q) = %q, want nil", c.delims, got.String())
				}
				return
			}
			if got == nil {
				t.Fatalf("compileDelimPattern(%q) = nil, want %q", c.delims, c.want)
			}
			if got.String() != c.want {
				t.Fatalf("compileDelimPattern(%q) = %q, want %q", c.delims, got.String(), c.want)
			}
		})
	}
}

// TestCompileDelimPatternMatchesLongestFirst guards the alternation order in
// action: on overlapping text the longest delimiter must win, otherwise a
// shorter prefix would silently shadow the longer delimiter.
func TestCompileDelimPatternMatchesLongestFirst(t *testing.T) {
	pattern := compileDelimPattern([]string{"ab", "`abc`"})
	if pattern == nil {
		t.Fatal("compileDelimPattern returned nil")
	}
	got := pattern.FindAllString("xxabcyy", -1)
	if len(got) != 1 || got[0] != "abc" {
		t.Fatalf("matches on \"xxabcyy\" = %v, want [abc] (longest first)", got)
	}
}

// TestCompileDelimPatternIsCaseSensitive: the compiler must not emit (?i); the
// chunker's delimiter matching is case-sensitive by contract.
func TestCompileDelimPatternIsCaseSensitive(t *testing.T) {
	pattern := compileDelimPattern([]string{"`end`"})
	if pattern == nil {
		t.Fatal("compileDelimPattern(`end`) returned nil")
	}
	if pattern.MatchString("END") {
		t.Error("pattern matched uppercase END; delimiter matching must be case-sensitive")
	}
	if !pattern.MatchString("the end") {
		t.Error("pattern did not match lowercase end")
	}
}

// TestCompileDelimPatternDelegatesToChunk pins that the wrapper stays a thin
// delegation to chunk.CompileDelimiterPatternList(..., keepBare=true): any
// divergence would silently fork children_delimiters behaviour from the main
// delimiter list.
func TestCompileDelimPatternDelegatesToChunk(t *testing.T) {
	inputs := [][]string{
		nil,
		{},
		{""},
		{"a"},
		{"`end`", "b"},
		{"。", "，", "`；`"},
		{"a.b", "c*d"},
	}
	for _, delims := range inputs {
		got := compileDelimPattern(delims)
		want := chunk.CompileDelimiterPatternList(delims, true)
		if (got == nil) != (want == nil) {
			t.Fatalf("compileDelimPattern(%q) nil-ness = %v, want %v", delims, got == nil, want == nil)
		}
		if got != nil && got.String() != want.String() {
			t.Fatalf("compileDelimPattern(%q) = %q, want %q", delims, got.String(), want.String())
		}
	}
}
