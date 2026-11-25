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

package filesystem

import (
	"strings"
	"testing"
)

// TestStringsJoinMatchesJoinStringsSemantics pins the observable behaviour of
// the removed joinStrings helper: empty and single-element slices pass through,
// and every remaining element is appended with the separator. strings.Join must
// reproduce all of it before the call sites are migrated.
func TestStringsJoinMatchesJoinStringsSemantics(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		sep  string
		want string
	}{
		{name: "nil slice", in: nil, sep: "/", want: ""},
		{name: "empty slice", in: []string{}, sep: "/", want: ""},
		{name: "single element", in: []string{"a"}, sep: "/", want: "a"},
		{name: "two elements", in: []string{"a", "b"}, sep: "/", want: "a/b"},
		{name: "three elements multi-char separator", in: []string{"a", "b", "c"}, sep: "::", want: "a::b::c"},
		{name: "empty strings preserved", in: []string{"", "b"}, sep: "/", want: "/b"},
		{name: "unicode separator", in: []string{"中", "文"}, sep: "·", want: "中·文"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(tt.in, tt.sep); got != tt.want {
				t.Errorf("strings.Join(%v, %q) = %q, want %q", tt.in, tt.sep, got, tt.want)
			}
		})
	}
}
