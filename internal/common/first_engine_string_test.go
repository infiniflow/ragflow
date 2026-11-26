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

package common

import "testing"

// TestFirstEngineString pins the shared engine-field contract: a scalar string
// passes through, a list yields only element zero with no trimming and no
// searching for a later string. The mixed list is adversarial — a helper that
// skipped non-strings would wrongly return "later".
func TestFirstEngineString(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{name: "nil", in: nil, want: ""},
		{name: "scalar", in: "a", want: "a"},
		{name: "scalar with spaces", in: " a ", want: " a "},
		{name: "string slice", in: []string{"a", "b"}, want: "a"},
		{name: "empty string slice", in: []string{}, want: ""},
		{name: "any slice", in: []any{"a", "b"}, want: "a"},
		{name: "any slice non-string first", in: []any{1, "later"}, want: ""},
		{name: "empty any slice", in: []any{}, want: ""},
		{name: "unsupported type", in: 42, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FirstEngineString(c.in); got != c.want {
				t.Errorf("FirstEngineString(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
