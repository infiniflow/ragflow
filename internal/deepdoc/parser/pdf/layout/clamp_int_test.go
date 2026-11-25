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

package layout

import "testing"

// TestClampIntSemantics pins the boundary contract before the local helper is
// replaced by min(max(v, lo), hi). Callers pass lo=0 and hi=nb-1 with nb>=1
// (columnHistogramBins), so lo<=hi always holds.
func TestClampIntSemantics(t *testing.T) {
	cases := []struct {
		name      string
		v, lo, hi int
		want      int
	}{
		{name: "below", v: -3, lo: 0, hi: 5, want: 0},
		{name: "interior", v: 3, lo: 0, hi: 5, want: 3},
		{name: "above", v: 9, lo: 0, hi: 5, want: 5},
		{name: "equal bounds", v: 2, lo: 4, hi: 4, want: 4},
		{name: "single bin", v: 9, lo: 0, hi: 0, want: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := min(max(c.v, c.lo), c.hi); got != c.want {
				t.Errorf("clamp(%d, %d, %d) = %d, want %d", c.v, c.lo, c.hi, got, c.want)
			}
		})
	}
}
