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

package utility

import "testing"

// TestTruncateRunesContract pins the rune-safe truncation used by every
// migrated caller. Emoji (four-byte runes), CJK and non-positive limits are the
// adversarial cases: a byte cut would split a rune, and a naive range loop with
// n<=0 would return the whole string.
func TestTruncateRunesContract(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{name: "cjk", s: "中文测试", max: 2, want: "中文"},
		{name: "emoji", s: "a😀b", max: 2, want: "a😀"},
		{name: "within budget", s: "abc", max: 10, want: "abc"},
		{name: "zero", s: "abc", max: 0, want: ""},
		{name: "negative", s: "abc", max: -1, want: ""},
		{name: "empty", s: "", max: 3, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TruncateRunes(c.s, c.max); got != c.want {
				t.Errorf("TruncateRunes(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
		})
	}
}
