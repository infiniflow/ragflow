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

// TestSanitizeFilename_Characterization pins the current filename sanitizer
// behavior: path separators normalized, dot segments dropped, and every char
// outside [A-Za-z0-9_-/] stripped from each component. The dot between a name
// and its extension is stripped by the current pattern; preserving extensions
// is a separate concern, not part of this characterization.
func TestSanitizeFilename_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"plain", "report.pdf", "reportpdf"},
		{"spaces stripped", "my report.pdf", "myreportpdf"},
		{"backslash normalized", `dir\sub\file.txt`, "dir/sub/filetxt"},
		{"dot segments dropped", "./dir/../file.txt", "dir/filetxt"},
		{"illegal chars stripped", `a:b*?"<>|c.txt`, "abctxt"},
		{"unicode stripped", "文件.txt", "txt"},
		{"outer slashes trimmed", "/a/b/", "a/b"},
		{"trailing slash in component", "a/b/./", "a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeFilename(tt.in); got != tt.want {
				t.Fatalf("SanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
