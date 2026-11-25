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

package utility

import "testing"

// TestAbsSemanticsAcrossBuiltinSwitch pins the hand-written abs contract,
// including the minimum-int adversarial case where negation overflows.
func TestAbsSemantics(t *testing.T) {
	minInt := -int(^uint(0)>>1) - 1
	cases := []struct {
		in   int
		want int
	}{
		{in: -9, want: 9},
		{in: -1, want: 1},
		{in: 0, want: 0},
		{in: 1, want: 1},
		{in: 9, want: 9},
		{in: minInt, want: minInt},
	}
	for _, c := range cases {
		if got := abs(c.in); got != c.want {
			t.Errorf("abs(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
