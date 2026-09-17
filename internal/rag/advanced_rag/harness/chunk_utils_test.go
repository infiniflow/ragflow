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

import "testing"

// TestSnippet covers Python _snippet parity: trim both ends, cut to limit,
// right-trim ALL trailing whitespace, then add "...".
func TestSnippet(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		// Short values pass through trimmed, no ellipsis.
		{"  hello  ", 10, "hello"},
		// No truncation needed.
		{"hello", 10, "hello"},
		// Truncation that lands on a plain space boundary.
		{"hello world", 5, "hello..."},
		// Cut ends mid-run of plain spaces: right-trim them.
		{"hello   world", 6, "hello..."},
		// Cut ends mid-run of a tab: right-trim ALL whitespace, like .rstrip().
		{"hello\t\tworld", 6, "hello..."},
		// Cut ends on a newline run.
		{"hello\n\nworld", 6, "hello..."},
		// Mixed trailing whitespace after the cut.
		{"hello \t\n world", 6, "hello..."},
		// CJK chars are multibyte in UTF-8; the limit is in code points
		// (Python str semantics), so no rune is ever split. "中文" (2 code
		// points) fits within n=2 and stays intact.
		{"  中文  ", 2, "中文"},
		// Truncation lands on a code-point boundary: first 3 of 4 CJK chars.
		{"中文中文", 3, "中文中..."},
		// A small byte limit must not split a 3-byte CJK rune.
		{"中文abc", 2, "中文..."},
	}
	for _, c := range cases {
		if got := Snippet(c.in, c.n); got != c.want {
			t.Errorf("Snippet(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
