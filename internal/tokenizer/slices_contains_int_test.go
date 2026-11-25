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

package tokenizer

import (
	"slices"
	"testing"
)

// TestSlicesContainsIntSemantics pins the contract of the removed containsInt
// helper, including negative and zero values.
func TestSlicesContainsIntSemantics(t *testing.T) {
	cases := []struct {
		name string
		list []int
		v    int
		want bool
	}{
		{name: "nil", list: nil, v: 1, want: false},
		{name: "empty", list: []int{}, v: 1, want: false},
		{name: "single hit", list: []int{1}, v: 1, want: true},
		{name: "single miss", list: []int{1}, v: 2, want: false},
		{name: "zero", list: []int{-1, 0, 1}, v: 0, want: true},
		{name: "negative", list: []int{-1, 0, 1}, v: -1, want: true},
		{name: "last", list: []int{-1, 0, 1}, v: 1, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := slices.Contains(c.list, c.v); got != c.want {
				t.Errorf("slices.Contains(%v, %d) = %v, want %v", c.list, c.v, got, c.want)
			}
		})
	}
}
