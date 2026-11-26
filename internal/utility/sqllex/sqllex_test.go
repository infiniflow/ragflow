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

package sqllex

import "testing"

func TestTrimTerminator(t *testing.T) {
	// Each example gives the source SQL and the exact text following it. Only
	// the statement delimiter between them should disappear.
	tests := []struct {
		name      string
		dialect   Dialect
		statement string
		trailing  string
		mode      string
	}{
		{"plain", MySQL, "SELECT 1", "", ""},
		{"line and block comments", PostgreSQL, "SELECT 1", " -- ;\r\n/* more; */", ""},
		{"quoted markers", MySQL, `SELECT 'it''s; --', "a; /*"`, " -- exported;", ""},
		{"quoted identifier", MySQL, "SELECT `id;--` FROM products", " # exported;", ""},
		{"mysql subtraction", MySQL, "SELECT 5--2", " -- exported", ""},
		{"mysql escape", MySQL, `SELECT 'it\'s; #'`, " # exported", ""},
		{"postgres dollar quote", PostgreSQL, `SELECT $tag$it'; -- /*$tag$`, " -- exported", ""},
		{"postgres UTF-8 dollar quote", PostgreSQL, `SELECT $标签$query'; --$标签$`, " -- exported", ""},
		{"postgres escape", PostgreSQL, `SELECT E'it\'s; --'`, " /* outer /* inner */ end */", ""},
		{"postgres operator", PostgreSQL, "SELECT 6 # 3", " -- exported", ""},
		{"mysql no backslash escapes", MySQL, `SELECT id, 'C:\' AS path FROM products ORDER BY id`, " -- exported;", "STRICT_TRANS_TABLES,NO_BACKSLASH_ESCAPES"},
		{"mysql ansi quoted identifier", MySQL, `SELECT "path\"`, " -- exported", "ANSI_QUOTES"},
		{"postgres ordinary escape string", PostgreSQL, `SELECT 'it\'s; --'`, " -- exported", "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.statement + tt.trailing
			for _, delimiter := range []string{"", ";"} {
				input := tt.statement + delimiter + tt.trailing
				if got := tt.dialect.TrimTerminator(input, tt.mode); got != want {
					t.Errorf("TrimTerminator(%q) = %q, want %q", input, got, want)
				}
			}
		})
	}
}
