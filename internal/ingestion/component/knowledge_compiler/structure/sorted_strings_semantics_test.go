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

package structure

import "testing"

// TestSortedStringsDedupAndSort pins that the bubble-sort replacement keeps
// the two guarantees of the old helper: first-seen de-duplication and byte-wise
// ascending order. Chinese keys are adversarial for the ordering contract.
func TestSortedStringsDedupAndSort(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "nil", in: nil, want: nil},
		{name: "empty", in: []string{}, want: nil},
		{name: "single", in: []string{"a"}, want: []string{"a"}},
		{name: "duplicates", in: []string{"b", "a", "b", "c", "a"}, want: []string{"a", "b", "c"}},
		{name: "chinese", in: []string{"中", "文", "中", "a"}, want: []string{"a", "中", "文"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sortedStrings(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("len(got)=%d, want %d (got=%v want=%v)", len(got), len(c.want), got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("item[%d]=%q, want %q; got=%v want=%v", i, got[i], c.want[i], got, c.want)
				}
			}
		})
	}
}
