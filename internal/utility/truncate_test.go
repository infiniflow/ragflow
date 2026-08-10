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

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"empty", "", 10, ""},
		{"ascii shorter than max", "hello", 10, "hello"},
		{"ascii exactly max", "hello", 5, "hello"},
		{"ascii truncated", "hello world", 5, "hello"},
		// Multi-byte: each CJK rune is 3 bytes; ensure truncation counts RUNES,
		// not bytes, so we keep exactly `max` characters, not `max` bytes.
		{"unicode truncated by rune", "中文测试abc", 4, "中文测试"},
		{"unicode shorter than max", "中文", 10, "中文"},
		// Guard: non-positive max yields empty string (safe default).
		{"max zero", "hello", 0, ""},
		{"max negative", "hello", -3, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TruncateRunes(c.in, c.max); got != c.want {
				t.Errorf("TruncateRunes(%q, %d) = %q, want %q", c.in, c.max, got, c.want)
			}
		})
	}
}
