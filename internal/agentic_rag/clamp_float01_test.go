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

package agentic_rag

import (
	"math"
	"testing"
)

// TestClampFloat01Semantics pins the boundary behaviour that the removed
// clampFloat01 helper provided. NaN is adversarial: the hand-written
// comparisons both miss and pass the value through, which min(max(...)) also
// does.
func TestClampFloat01Semantics(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "negative infinity", in: math.Inf(-1), want: 0},
		{name: "negative", in: -0.5, want: 0},
		{name: "lower bound", in: 0, want: 0},
		{name: "interior", in: 0.5, want: 0.5},
		{name: "upper bound", in: 1, want: 1},
		{name: "above", in: 2, want: 1},
		{name: "positive infinity", in: math.Inf(1), want: 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := min(max(c.in, 0.0), 1.0); got != c.want {
				t.Errorf("clamp(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
	if got := min(max(math.NaN(), 0.0), 1.0); !math.IsNaN(got) {
		t.Errorf("clamp(NaN) = %v, want NaN", got)
	}
}
