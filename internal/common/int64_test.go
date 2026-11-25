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

// TestToInt64Contract pins the engine count conversion. Unlike ToInt, the
// string branch uses ParseInt without trimming, so a padded number must still
// return zero — that is the adversarial case distinguishing it from Atoi.
func TestToInt64Contract(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
	}{
		{name: "int64", in: int64(7), want: 7},
		{name: "int", in: 7, want: 7},
		{name: "float64 truncates", in: 7.9, want: 7},
		{name: "string", in: "7", want: 7},
		{name: "padded string stays zero", in: " 7 ", want: 0},
		{name: "invalid string", in: "abc", want: 0},
		{name: "bool unsupported", in: true, want: 0},
		{name: "nil", in: nil, want: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ToInt64(c.in); got != c.want {
				t.Errorf("ToInt64(%v) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}
