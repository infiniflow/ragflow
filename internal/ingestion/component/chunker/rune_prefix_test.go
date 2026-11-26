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

import "testing"

// TestRunePrefixPreservesOriginalBytes pins this splitter's byte-prefix
// contract. Unlike the decision-head variant it guards n<=0, but both must
// return bytes from the original string so rest[len(head):] stays in range.
func TestRunePrefixPreservesOriginalBytes(t *testing.T) {
	cases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "invalid bytes", s: "\xff\xfe", n: 1, want: "\xff"},
		{name: "truncated multibyte", s: "\xe4\xb8", n: 1, want: "\xe4"},
		{name: "valid cjk", s: "中文", n: 1, want: "中"},
		{name: "ascii", s: "abc", n: 2, want: "ab"},
		{name: "zero", s: "中文", n: 0, want: ""},
		{name: "negative", s: "中文", n: -1, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runePrefix(c.s, c.n)
			if got != c.want {
				t.Errorf("runePrefix(%x, %d) = %x, want %x", c.s, c.n, got, c.want)
			}
			if len(got) > len(c.s) {
				t.Fatalf("runePrefix returned %d bytes for a %d-byte input", len(got), len(c.s))
			}
		})
	}
}
