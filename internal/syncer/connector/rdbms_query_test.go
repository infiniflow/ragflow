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
	"time"

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
		buildSyncQueries([]rdbmsQuery, SyncRequest, rdbmsSyncMode) []rdbmsSyncQuery
		buildSlimQuery(string) string
	}{
		"mysql":      &MySQLConnector{idColumn: "id", query: query, openDB: openDB},
		"postgresql": &PostgreSQLConnector{idColumn: "id", query: query, openDB: openDB},
	} {
		_, bases, err := builder.openQueries(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		full := builder.buildSyncQueries(bases, SyncRequest{FromBeginning: true}, rdbmsOrderByID)[0]
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

func TestRDBMSTimestampOnlyIncremental(t *testing.T) {
	const query = "SELECT '2026-01-01' AS updated_at, 'hello' AS body; -- exported;"
	openDB := func(string) (*sql.DB, error) {
		db, err := gorm.Open(sqlite.Open(":memory:"))
		if err != nil {
			return nil, err
		}
		return db.DB()
	}
	for dialect, connector := range map[string]interface {
		OpenSync(context.Context, SyncRequest) (SyncSession, error)
	}{
		"mysql":      &MySQLConnector{query: query, timestampColumn: "updated_at", batchSize: 2, openDB: openDB},
		"postgresql": &PostgreSQLConnector{query: query, timestampColumn: "updated_at", batchSize: 2, openDB: openDB},
	} {
		t.Run(dialect, func(t *testing.T) {
			session, err := connector.OpenSync(t.Context(), SyncRequest{WindowEnd: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.Close() })
			batch, err := session.NextBatch(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Documents) != 1 || batch.Checkpoint != nil {
				t.Fatalf("got %d documents and checkpoint %v, want one document and no checkpoint", len(batch.Documents), batch.Checkpoint)
			}
		})
	}
}
