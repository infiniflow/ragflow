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

// TestMinPositive pins the exact fallback contract shared by the SQL engines:
// a non-positive operand is ignored, and negatives never win over positives.
func TestMinPositive(t *testing.T) {
	cases := []struct {
		name       string
		first, sec int
		want       int
	}{
		{name: "both positive ascending", first: 3, sec: 7, want: 3},
		{name: "both positive descending", first: 7, sec: 3, want: 3},
		{name: "first non-positive", first: 0, sec: 5, want: 5},
		{name: "second non-positive", first: 5, sec: -1, want: 5},
		{name: "both non-positive", first: -1, sec: -2, want: -2},
		{name: "equal", first: 4, sec: 4, want: 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MinPositive(c.first, c.sec); got != c.want {
				t.Errorf("MinPositive(%d, %d) = %d, want %d", c.first, c.sec, got, c.want)
			}
		})
	}
}

func TestPositiveOr(t *testing.T) {
	cases := []struct {
		name            string
		value, fallback int
		want            int
	}{
		{name: "positive", value: 5, fallback: 30, want: 5},
		{name: "zero", value: 0, fallback: 30, want: 30},
		{name: "negative", value: -1, fallback: 30, want: 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PositiveOr(c.value, c.fallback); got != c.want {
				t.Errorf("PositiveOr(%d, %d) = %d, want %d", c.value, c.fallback, got, c.want)
			}
		})
	}
}
