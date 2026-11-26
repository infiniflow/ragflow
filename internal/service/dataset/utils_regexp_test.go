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

package dataset

import "testing"

// TestDatasetCleanEmbeddingText_Characterization pins the current HTML-tag
// stripping used before embedding.
func TestDatasetCleanEmbeddingText_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "hello world"},
		{"empty", "", ""},
		{"simple tag", "<b>hi</b>", "hi"},
		{"nested tags", "<a><b>x</b></a>", "x"},
		{"unclosed tag stays", "a<b", "a<b"},
		{"angle in text stays", "x > y", "x > y"},
		{"multiple tags", "a<span>b</span>c", "abc"},
		{"tag with attributes", `<p class="x">text</p>`, "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := datasetCleanEmbeddingText(tt.in); got != tt.want {
				t.Fatalf("datasetCleanEmbeddingText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
