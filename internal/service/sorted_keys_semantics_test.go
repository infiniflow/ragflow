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

package service

import (
	"maps"
	"slices"
	"sort"
	"testing"
)

// TestSortedKeysByteIdentical pins that slices.Sorted(maps.Keys(m)) produces
// exactly the sequence the removed sortedKeys helper did, because the result
// feeds deterministic SQL and artifact IDs. Chinese keys and mixed lengths are
// adversarial: any locale-sensitive ordering would break the bytes.
func TestSortedKeysByteIdentical(t *testing.T) {
	cases := []struct {
		name string
		m    map[string]bool
	}{
		{name: "nil", m: nil},
		{name: "empty", m: map[string]bool{}},
		{name: "single", m: map[string]bool{"a": true}},
		{name: "multiple", m: map[string]bool{"c": true, "a": true, "b": true}},
		{name: "chinese and ascii", m: map[string]bool{"中": true, "文": true, "a": true, "A": true}},
		{name: "prefix lengths", m: map[string]bool{"a": true, "aa": true, "ab": true, "b": true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := slices.Sorted(maps.Keys(c.m))
			want := make([]string, 0, len(c.m))
			for k := range c.m {
				want = append(want, k)
			}
			sort.Strings(want)
			if len(got) != len(want) {
				t.Fatalf("len(got)=%d, want %d (got=%v, want=%v)", len(got), len(want), got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("key[%d]=%q, want %q; full got=%v want=%v", i, got[i], want[i], got, want)
				}
			}
		})
	}
}
