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
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
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
	query := base + "; /* exported */ -- skip the highest priority"
	openDB := func(string) (*sql.DB, error) { return conn, nil }
	for dialect, builder := range map[string]interface {
		openQueries(context.Context, bool) (rdbmsSessionDB, []rdbmsQuery, error)
		buildSyncQueries([]rdbmsQuery, SyncRequest) []rdbmsSyncQuery
		buildSlimQuery(string) string
	}{
		"mysql":      &MySQLConnector{idColumn: "id", query: query, openDB: openDB},
		"postgresql": &PostgreSQLConnector{idColumn: "id", query: query, openDB: openDB},
	} {
		_, bases, err := builder.openQueries(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		full := builder.buildSyncQueries(bases, SyncRequest{FromBeginning: true})[0]
		for name, query := range map[string]string{
			"full":     full.sql,
			"fallback": full.fallback,
			"prune":    builder.buildSlimQuery(bases[0].sql),
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
