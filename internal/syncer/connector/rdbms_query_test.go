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

package connector

import (
	"slices"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/utility/sqllex"
)

func TestRDBMSOrderedLimit(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"))
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	// In priority order the IDs are 3, 4, 1, 2. The configured window selects
	// 4 and 1; the full sync must emit those two rows in checkpoint (ID) order.
	const base = `SELECT id FROM (
		SELECT 1 AS id, 30 AS priority UNION ALL SELECT 2, 5
		UNION ALL SELECT 3, 90 UNION ALL SELECT 4, 60
	) AS products ORDER BY priority DESC LIMIT 2 OFFSET 1`
	for dialect, builder := range map[string]interface {
		buildSyncQueries([]rdbmsQuery, SyncRequest) []rdbmsSyncQuery
		buildSlimQuery(string) string
	}{
		"mysql":      &MySQLConnector{idColumn: "id"},
		"postgresql": &PostgreSQLConnector{idColumn: "id"},
	} {
		full := builder.buildSyncQueries([]rdbmsQuery{{sql: base + "; -- exported query"}}, SyncRequest{FromBeginning: true})[0]
		for name, query := range map[string]string{
			"full":     full.sql,
			"fallback": full.fallback,
			"prune":    builder.buildSlimQuery(base + "; /* skip the highest priority */"),
		} {
			t.Run(dialect+"/"+name, func(t *testing.T) {
				var ids []int
				if err := db.Raw(query).Scan(&ids).Error; err != nil {
					t.Fatal(err)
				}
				if name != "full" {
					slices.Sort(ids)
				}
				if !slices.Equal(ids, []int{1, 4}) {
					t.Errorf("IDs = %v, want [1 4]", ids)
				}
			})
		}
	}
}

func TestRDBMSWrappedQueryTerminator(t *testing.T) {
	// Each example gives the source SQL and the exact text following it. Only
	// the statement delimiter between them should disappear from the wrapper.
	tests := []struct {
		name      string
		dialect   sqllex.Dialect
		statement string
		trailing  string
	}{
		{"plain", sqllex.MySQL, "SELECT 1", ""},
		{"line and block comments", sqllex.PostgreSQL, "SELECT 1", " -- ;\r\n/* more; */"},
		{"quoted markers", sqllex.MySQL, `SELECT 'it''s; --', "a; /*"`, " -- exported;"},
		{"quoted identifier", sqllex.MySQL, "SELECT `id;--` FROM products", " # exported;"},
		{"mysql subtraction", sqllex.MySQL, "SELECT 5--2", " -- exported"},
		{"mysql escape", sqllex.MySQL, `SELECT 'it\'s; #'`, " # exported"},
		{"postgres dollar quote", sqllex.PostgreSQL, `SELECT $tag$it'; -- /*$tag$`, " -- exported"},
		{"postgres UTF-8 dollar quote", sqllex.PostgreSQL, `SELECT $标签$query'; --$标签$`, " -- exported"},
		{"postgres escape", sqllex.PostgreSQL, `SELECT E'it\'s; --'`, " /* outer /* inner */ end */"},
		{"postgres operator", sqllex.PostgreSQL, "SELECT 6 # 3", " -- exported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "SELECT * FROM (" + tt.statement + tt.trailing + "\n) AS ragflow_src"
			for _, delimiter := range []string{"", ";"} {
				input := tt.statement + delimiter + tt.trailing
				if got := wrapRDBMSQuery(input, "*", tt.dialect); got != want {
					t.Errorf("wrapRDBMSQuery(%q) = %q, want %q", input, got, want)
				}
			}
		})
	}
}
