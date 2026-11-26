//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import "testing"

// Citation positions belong to one retrieval result, so a previous answer's
// positions must not be sent back to the model against a new knowledge context.
func TestCleanCitationMarkersRemovesPreviousTurnIDs(t *testing.T) {
	cases := []struct {
		name string
		text string
		want string
	}{
		{"employee answer", "张伟的基础信息[ID:7]。", "张伟的基础信息。"},
		{"multiple sources", "Employee[ID:0][ID:7] details##2$$.", "Employee details."},
		{"spaced ID", "Employee [ID: 12].", "Employee ."},
		{"Arabic digits", "Employee[ID:٧][ID:۷].", "Employee."},
		{"ordinary brackets", "Records [2024], array[7], [guide](https://example.com).", "Records [2024], array[7], [guide](https://example.com)."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanCitationMarkers(tc.text); got != tc.want {
				t.Fatalf("history content = %q, want %q", got, tc.want)
			}
		})
	}
}
