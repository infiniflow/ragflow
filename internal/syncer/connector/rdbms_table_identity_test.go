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
	"errors"
	"io"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRDBMSTableIdentity(t *testing.T) {
	for _, tc := range []struct {
		dialect  string
		listSQL  string
		tableSQL func(string) string
		fixture  func(*testing.T, map[string]any, func(sqlmock.Sqlmock)) Connector
	}{
		{
			dialect:  "mysql",
			listSQL:  "SHOW TABLES",
			tableSQL: func(table string) string { return "SELECT * FROM " + table },
			fixture: func(t *testing.T, config map[string]any, expect func(sqlmock.Sqlmock)) Connector {
				return newFixtureMySQLConnector(t, config, expect)
			},
		},
		{
			dialect:  "postgresql",
			listSQL:  "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'",
			tableSQL: func(table string) string { return `SELECT * FROM "public"."` + table + `"` },
			fixture: func(t *testing.T, config map[string]any, expect func(sqlmock.Sqlmock)) Connector {
				return newFixturePostgresConnector(t, config, expect)
			},
		},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			config := map[string]any{"database": "mydb", "id_column": "id", "content_columns": "body", "batch_size": 1}
			expectTables := func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(regexp.QuoteMeta(tc.listSQL)).WillReturnRows(sqlmock.NewRows([]string{"table"}).AddRow("orders").AddRow("products"))
			}
			expectSync := func(mock sqlmock.Sqlmock) {
				expectTables(mock)
				for _, table := range []string{"orders", "products"} {
					query := "SELECT * FROM (" + tc.tableSQL(table) + ") AS ragflow_src ORDER BY ragflow_src.id ASC"
					mock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnRows(sqlmock.NewRows([]string{"id", "body"}).AddRow(1, "same content"))
				}
			}
			connector := tc.fixture(t, config, expectSync)
			session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
			if err != nil {
				t.Fatal(err)
			}
			first, err := session.NextBatch(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Documents) != 1 || first.Documents[0].SourceID != tc.dialect+"-table:mydb:orders:1" {
				t.Fatalf("first batch = %+v", first)
			}
			if first.Checkpoint == nil {
				t.Fatal("missing checkpoint")
			}
			second, err := session.NextBatch(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(second.Documents) != 1 || second.Documents[0].SourceID != tc.dialect+"-table:mydb:products:1" {
				t.Fatalf("second batch = %+v", second)
			}
			if first.Documents[0].Fingerprint == second.Documents[0].Fingerprint {
				t.Fatal("colliding fingerprints")
			}
			if _, err := session.NextBatch(t.Context()); !errors.Is(err, io.EOF) {
				t.Fatalf("EOF = %v", err)
			}

			// A checkpoint at the end of one table must not consume the same id in
			// the next table, even when the batch boundary falls between tables.
			resumed := tc.fixture(t, config, expectSync)
			resumeSession, err := resumed.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: first.Checkpoint})
			if err != nil {
				t.Fatal(err)
			}
			batch, err := resumeSession.NextBatch(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Documents) != 1 || batch.Documents[0].SourceID != second.Documents[0].SourceID {
				t.Fatalf("resumed batch = %+v", batch)
			}
			if _, err := resumeSession.NextBatch(t.Context()); !errors.Is(err, io.EOF) {
				t.Fatalf("resumed EOF = %v", err)
			}

			pruneConnector := tc.fixture(t, config, func(mock sqlmock.Sqlmock) {
				expectTables(mock)
				for _, table := range []string{"orders", "products"} {
					query := "SELECT ragflow_src.id FROM (" + tc.tableSQL(table) + ") AS ragflow_src"
					mock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				}
			})
			prune, err := pruneConnector.OpenPrune(t.Context(), PruneRequest{})
			if err != nil {
				t.Fatal(err)
			}
			for _, expected := range []string{first.Documents[0].SourceID, second.Documents[0].SourceID} {
				batch, err := prune.NextBatch(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if len(batch.Documents) != 1 || batch.Documents[0].SourceID != expected {
					t.Fatalf("prune = %+v, want %s", batch, expected)
				}
			}
			if _, err := prune.NextBatch(t.Context()); !errors.Is(err, io.EOF) {
				t.Fatalf("prune EOF = %v", err)
			}

			t.Run("table-less checkpoint restarts window", func(t *testing.T) {
				old := tc.dialect + ":mydb:orders:1"
				connector := tc.fixture(t, config, expectSync)
				session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true, Resume: &SyncCheckpoint{Cursor: encodeRDBMSCursor("orders", "id", old), SourceID: old}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := session.NextBatch(t.Context()); !errors.Is(err, ErrSyncResumeInvalid) {
					t.Fatalf("old checkpoint error = %v", err)
				}
			})
		})
	}
}

func TestRDBMSDocumentIDTableNamespace(t *testing.T) {
	for _, tc := range []struct {
		dialect    string
		documentID func(string, map[string]any, []string) string
	}{
		{"mysql", (&MySQLConnector{database: "mydb", idColumn: "id"}).buildDocumentID},
		{"postgresql", (&PostgreSQLConnector{database: "mydb", idColumn: "id"}).buildDocumentID},
	} {
		t.Run(tc.dialect, func(t *testing.T) {
			row := map[string]any{"id": 1, "body": "same content"}
			columns := []string{"id", "body"}
			if tc.documentID("orders", row, columns) == tc.documentID("products", row, columns) {
				t.Fatal("table ids collide")
			}
			if tc.documentID("", row, columns) != tc.dialect+":mydb:1" {
				t.Fatal("custom query id changed")
			}
			if tc.documentID("orders", row, columns) == tc.documentID("", map[string]any{"id": "orders:1"}, []string{"id"}) {
				t.Fatal("table id aliases a table-less id")
			}
			// Escape the table delimiter: table a:b/id 1 differs from table a/id b:1.
			if tc.documentID("a:b", row, columns) == tc.documentID("a", map[string]any{"id": "b:1"}, []string{"id"}) {
				t.Fatal("table separator collides")
			}
			delete(row, "id")
			if tc.documentID("orders", row, columns) == tc.documentID("products", row, columns) {
				t.Fatal("content-hash ids collide across tables")
			}
			row["id"] = 1
			before := tc.documentID("orders", row, columns)
			row["body"] = "updated content"
			if tc.documentID("orders", row, columns) != before {
				t.Fatal("content update changed row identity")
			}
		})
	}
}
