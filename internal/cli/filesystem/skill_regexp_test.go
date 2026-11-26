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

package filesystem

import "testing"

// TestNormalizeSkillName_Characterization pins the current slug normalization.
func TestNormalizeSkillName_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lower and spaces", "My Skill", "my-skill"},
		{"underscores to dashes", "my_skill", "my-skill"},
		{"collapse runs", "a---b", "a-b"},
		{"strip illegal", "a@b#c", "a-b-c"},
		{"trim dashes", "--abc--", "abc"},
		{"unicode dropped", "技能abc", "abc"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeSkillName(tt.in); got != tt.want {
				t.Fatalf("normalizeSkillName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestSanitizeRelPath_Characterization pins the current relative-path gate.
func TestSanitizeRelPath_Characterization(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "a/b.txt", "a/b.txt"},
		{"dot prefix", "./a/b.txt", "a/b.txt"},
		{"multiple dot prefix", ".//a/b.txt", "a/b.txt"},
		{"leading slash", "/a/b.txt", "a/b.txt"},
		{"parent traversal", "../a", ""},
		{"embedded parent", "a/../b", ""},
		{"backslash rejected", `a\b`, ""},
		{"trailing slash", "a/b/", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeRelPath(tt.in); got != tt.want {
				t.Fatalf("sanitizeRelPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestMatchPattern_Characterization pins the current glob matching behavior,
// including that '*' does not cross a path separator.
func TestMatchPattern_Characterization(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		pattern string
		want    bool
	}{
		{"exact", "a/b.txt", "a/b.txt", true},
		{"star matches within component", "a.txt", "*.txt", true},
		{"star does not cross slash", "a/b.txt", "*.txt", false},
		{"double star crosses slash", "a/b.txt", "**.txt", true},
		{"question mark", "a1.txt", "a?.txt", true},
		{"dir pattern prefix", "sub/a.txt", "sub/", true},
		{"dir pattern exact", "sub", "sub/", true},
		{"dir pattern mismatch", "other/a.txt", "sub/", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchPattern(tt.path, tt.pattern); got != tt.want {
				t.Fatalf("matchPattern(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

// BenchmarkIsValidSkillName guards the regexp-hoist on the skill-name
// validation path, which compiled its pattern on every call.
func BenchmarkIsValidSkillName(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		isValidSkillName("my-skill-name")
	}
}
