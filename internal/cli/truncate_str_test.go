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

package cli

import "testing"

// TestTruncateStrCharacterization pins the current truncation contract:
// leading/trailing whitespace is trimmed first, and an over-budget string is
// cut at a maxLen-rune boundary before a literal "..." is appended (so the
// result may be up to maxLen+3 runes).
func TestTruncateStrCharacterization(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"empty", "", 5, ""},
		{"within budget", "hello", 10, "hello"},
		{"exact budget keeps ellipsis off", "abc", 3, "abc"},
		{"trims surrounding whitespace", "  hello  ", 10, "hello"},
		{"ascii truncation appends ellipsis", "hello world", 5, "hello..."},
		{"one over budget", "abcd", 3, "abc..."},
		{"rune boundary", "日本語のテキスト", 3, "日本語..."},
		{"trim then truncate", "  hello world  ", 5, "hello..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncateStr(tc.s, tc.max); got != tc.want {
				t.Fatalf("truncateStr(%q, %d) = %q, want %q", tc.s, tc.max, got, tc.want)
			}
		})
	}
}
