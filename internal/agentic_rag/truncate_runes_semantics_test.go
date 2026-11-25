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

package agentic_rag

import (
	"strings"
	"testing"
)

// TestTruncateRunesKeepsEllipsis pins that the caller-side ellipsis survives
// the migration to utility.TruncateRunes: only truncated strings get "...",
// and CJK content is never cut mid-rune.
func TestTruncateRunesKeepsEllipsis(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{name: "truncated cjk", s: "中文测试文本", max: 3, want: "中文测..."},
		{name: "within budget", s: "abc", max: 10, want: "abc"},
		{name: "zero", s: "abc", max: 0, want: "..."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := truncateRunes(c.s, c.max); got != c.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
		})
	}
}

// TestFirstRunesTrimsThenTruncates pins the error-slice helper's contract:
// leading whitespace is removed before counting, and the ellipsis is appended
// only when truncation actually happened.
func TestFirstRunesTrimsThenTruncates(t *testing.T) {
	cases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "trim then truncate", s: "  中文测试文本", n: 2, want: "中文..."},
		{name: "within budget", s: "  abc  ", n: 10, want: "abc"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := firstRunes(c.s, c.n); got != c.want {
				t.Errorf("firstRunes(%q, %d) = %q, want %q", c.s, c.n, got, c.want)
			}
		})
	}
	if got := strings.TrimSpace("  x  "); got != "x" {
		t.Fatalf("unexpected trim result %q", got)
	}
}
