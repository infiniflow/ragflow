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

package document

import "testing"

// TestIsTimeString_Characterization pins the current ISO datetime shape gate.
// Note the regex checks shape only: out-of-range components still pass.
func TestIsTimeString_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid", "2026-01-01T00:00:00", true},
		{"valid max", "9999-12-31T23:59:59", true},
		{"out of range shape still passes", "2026-13-40T99:99:99", true},
		{"date only rejected", "2026-01-01", false},
		{"with Z suffix rejected", "2026-01-01T00:00:00Z", false},
		{"with fraction rejected", "2026-01-01T00:00:00.000", false},
		{"single digits rejected", "2026-1-1T00:00:00", false},
		{"empty rejected", "", false},
		{"space separator rejected", "2026-01-01 00:00:00", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTimeString(tt.in); got != tt.want {
				t.Fatalf("isTimeString(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
