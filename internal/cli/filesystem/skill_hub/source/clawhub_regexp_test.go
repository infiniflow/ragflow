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

package source

import (
	"reflect"
	"testing"
)

// TestExtractQueryTerms_Characterization pins the current query-term split:
// lowercase then split on every run of non-[a-z0-9], dropping empty pieces.
func TestExtractQueryTerms_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"underscore split", "Hello_World", []string{"hello", "world"}},
		{"spaces", "a  b", []string{"a", "b"}},
		{"empty", "", []string{}},
		{"only separators", "---", []string{}},
		{"mixed case and digits", "ABC123", []string{"abc123"}},
		{"multiple separators", "a-b_c", []string{"a", "b", "c"}},
		{"unicode dropped", "技能a", []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractQueryTerms(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("extractQueryTerms(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
