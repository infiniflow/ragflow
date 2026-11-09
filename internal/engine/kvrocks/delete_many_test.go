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

package kvrocks

import (
	"reflect"
	"testing"
)

// TestSplitIntoBatches exercises the chunking math used by DeleteMany: every
// key must appear exactly once, input order must be preserved within and across
// chunks, and the boundary cases (empty, exact multiple, trailing partial
// batch, size 1, non-positive size) must split without dropping or duplicating.
// A naive split that computed the end index wrong would drop the trailing
// partial batch or emit an out-of-range slice.
func TestSplitIntoBatches(t *testing.T) {
	cases := []struct {
		name string
		keys []string
		size int
		want [][]string
	}{
		{
			name: "empty",
			keys: nil,
			size: 1024,
			want: nil,
		},
		{
			name: "single key within a large batch",
			keys: []string{"a"},
			size: 1024,
			want: [][]string{{"a"}},
		},
		{
			name: "exact single batch",
			keys: []string{"a", "b", "c"},
			size: 3,
			want: [][]string{{"a", "b", "c"}},
		},
		{
			name: "one element over the boundary spills into a second batch",
			keys: []string{"a", "b", "c", "d"},
			size: 3,
			want: [][]string{{"a", "b", "c"}, {"d"}},
		},
		{
			name: "trailing partial batch is kept, not dropped",
			keys: []string{"a", "b", "c", "d", "e"},
			size: 3,
			want: [][]string{{"a", "b", "c"}, {"d", "e"}},
		},
		{
			name: "exact two full batches",
			keys: []string{"a", "b", "c", "d", "e", "f"},
			size: 3,
			want: [][]string{{"a", "b", "c"}, {"d", "e", "f"}},
		},
		{
			name: "size one yields one key per chunk",
			keys: []string{"a", "b", "c"},
			size: 1,
			want: [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name: "non-positive size degrades to one key per chunk",
			keys: []string{"a", "b", "c"},
			size: 0,
			want: [][]string{{"a"}, {"b"}, {"c"}},
		},
		{
			name: "negative size degrades to one key per chunk",
			keys: []string{"a", "b", "c"},
			size: -1,
			want: [][]string{{"a"}, {"b"}, {"c"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitIntoBatches(tc.keys, tc.size)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitIntoBatches(%v, %d) = %v, want %v", tc.keys, tc.size, got, tc.want)
			}
		})
	}
}
