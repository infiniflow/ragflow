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
	"slices"
	"testing"
)

// TestSlicesContainsStringSemantics pins the contract of the removed
// containsStr helper: empty slices never match, and membership is exact.
func TestSlicesContainsStringSemantics(t *testing.T) {
	cases := []struct {
		name string
		list []string
		v    string
		want bool
	}{
		{name: "nil", list: nil, v: "a", want: false},
		{name: "empty", list: []string{}, v: "a", want: false},
		{name: "single hit", list: []string{"a"}, v: "a", want: true},
		{name: "single miss", list: []string{"a"}, v: "b", want: false},
		{name: "middle", list: []string{"a", "b", "c"}, v: "b", want: true},
		{name: "last", list: []string{"a", "b", "c"}, v: "c", want: true},
		{name: "empty string member", list: []string{"a", ""}, v: "", want: true},
		{name: "unicode", list: []string{"中", "文"}, v: "文", want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := slices.Contains(c.list, c.v); got != c.want {
				t.Errorf("slices.Contains(%v, %q) = %v, want %v", c.list, c.v, got, c.want)
			}
		})
	}
}
