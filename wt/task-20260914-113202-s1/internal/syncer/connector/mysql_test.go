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
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func newFixtureMySQLConnector(t *testing.T, config map[string]any, expect func(mock sqlmock.Sqlmock)) *MySQLConnector {
	t.Helper()
	if config == nil {
		config = map[string]any{
			"host":     "127.0.0.1",
			"port":     "3306",
			"database": "mydb",
			"credentials": map[string]any{
				"username": "root",
				"password": "secret",
			},
		}
	}
	connector, err := NewMySQLConnector(config)
	if err != nil {
		t.Fatalf("NewMySQLConnector failed: %v", err)
	}
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
	})
	connector.openDB = func(dsn string) (*sql.DB, error) {
		return db, nil
	}
	if expect != nil {
		expect(mock)
	}
	return connector
}

// TestMySQLConnectorOpenSyncCustomQuery verifies a custom query produces documents.
func TestMySQLConnectorOpenSyncCustomQuery(t *testing.T) {
	query := "SELECT * FROM products WHERE status = 'active'"
	updatedAt := mustTime(t, "2026-01-02T03:04:05Z")
	connector := newFixtureMySQLConnector(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             "3306",
		"database":         "mydb",
		"query":            query,
		"content_columns":  "title,description",
		"metadata_columns": "id,category,updated_at",
		"id_column":        "id",
		"timestamp_column": "updated_at",
		"credentials": map[string]any{
			"username": "root",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnRows(
			sqlmock.NewRows([]string{"id", "title", "description", "category", "updated_at"}).
				AddRow(7, "Hello/World", "Some body", "news", updatedAt),
		)
	})

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	doc := batch.Documents[0]
	if doc.SourceID != "mysql:mydb:7" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "Hello/World" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
	}
	if doc.Extension != ".txt" {
		t.Fatalf("extension = %q", doc.Extension)
	}
	blob := string(doc.Blob)
	if !strings.Contains(blob, "【title】:\nHello/World") || !strings.Contains(blob, "【description】:\nSome body") {
		t.Fatalf("blob = %q", blob)
	}
	if !doc.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("updated at = %s", doc.UpdatedAt)
	}
	if doc.Metadata["category"] != "news" || doc.Metadata["id"] != "7" {
		t.Fatalf("metadata = %v", doc.Metadata)
	}
	if doc.Metadata["updated_at"] != updatedAt.Format(time.RFC3339) {
		t.Fatalf("metadata updated_at = %v", doc.Metadata["updated_at"])
	}
	if doc.Fingerprint == "" {
		t.Fatalf("fingerprint is empty")
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

// TestMySQLConnectorOpenSyncIncrementalWindow verifies the timestamp filter SQL.
func TestMySQLConnectorOpenSyncIncrementalWindow(t *testing.T) {
	connector := newFixtureMySQLConnector(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             3306,
		"database":         "mydb",
		"query":            "SELECT * FROM products",
		"timestamp_column": "updated_at",
		"credentials": map[string]any{
			"username": "root",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		expected := "SELECT * FROM (SELECT * FROM products) AS ragflow_src " +
			"WHERE ragflow_src.updated_at >= '2026-01-01 00:00:00' AND ragflow_src.updated_at <= '2026-01-02 00:00:00'"
		mock.ExpectQuery(regexp.QuoteMeta(expected)).WillReturnRows(
			sqlmock.NewRows([]string{"updated_at"}).AddRow(mustTime(t, "2026-01-01T12:00:00Z")),
		)
	})

	start := mustTime(t, "2026-01-01T00:00:00Z")
	end := mustTime(t, "2026-01-02T00:00:00Z")
	session, err := connector.OpenSync(t.Context(), SyncRequest{WindowStart: &start, WindowEnd: end})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
}

// TestMySQLConnectorOpenSyncAllTables verifies SHOW TABLES expands to per-table queries.
func TestMySQLConnectorOpenSyncAllTables(t *testing.T) {
	connector := newFixtureMySQLConnector(t, map[string]any{
		"host":      "127.0.0.1",
		"port":      3306,
		"database":  "mydb",
		"id_column": "id",
		"credentials": map[string]any{
			"username": "root",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta("SHOW TABLES")).WillReturnRows(
			sqlmock.NewRows([]string{"Tables_in_mydb"}).AddRow("products").AddRow("orders"),
		)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM products")).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Product"),
		)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM orders")).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(2, "Order"),
		)
	})

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	var ids []string
	for {
		batch, err := session.NextBatch(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("NextBatch failed: %v", err)
		}
		for _, doc := range batch.Documents {
			ids = append(ids, doc.SourceID)
		}
	}
	if len(ids) != 2 || ids[0] != "mysql:mydb:1" || ids[1] != "mysql:mydb:2" {
		t.Fatalf("source ids = %v", ids)
	}
}

// TestMySQLConnectorOpenPrune verifies the slim query and Python-compatible IDs.
func TestMySQLConnectorOpenPrune(t *testing.T) {
	connector := newFixtureMySQLConnector(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            3306,
		"database":        "mydb",
		"query":           "SELECT * FROM products",
		"content_columns": "title,description",
		"id_column":       "id",
		"credentials": map[string]any{
			"username": "root",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		expected := "SELECT ragflow_src.id FROM (SELECT * FROM products) AS ragflow_src"
		mock.ExpectQuery(regexp.QuoteMeta(expected)).WillReturnRows(
			sqlmock.NewRows([]string{"id"}).AddRow(3).AddRow(4),
		)
	})

	session, err := connector.OpenPrune(t.Context(), PruneRequest{})
	if err != nil {
		t.Fatalf("OpenPrune failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 2 ||
		batch.Documents[0].SourceID != "mysql:mydb:3" ||
		batch.Documents[1].SourceID != "mysql:mydb:4" {
		t.Fatalf("slim documents = %+v", batch.Documents)
	}
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

// TestMySQLConnectorMD5FallbackID verifies the content-hash document id.
func TestMySQLConnectorMD5FallbackID(t *testing.T) {
	connector := newFixtureMySQLConnector(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            3306,
		"database":        "mydb",
		"query":           "SELECT * FROM products",
		"content_columns": "title,description",
		"credentials": map[string]any{
			"username": "root",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM products")).WillReturnRows(
			sqlmock.NewRows([]string{"title", "description"}).AddRow("Hello", "World"),
		)
	})

	session, err := connector.OpenSync(t.Context(), SyncRequest{FromBeginning: true})
	if err != nil {
		t.Fatalf("OpenSync failed: %v", err)
	}
	batch, err := session.NextBatch(context.Background())
	if err != nil {
		t.Fatalf("NextBatch failed: %v", err)
	}
	if len(batch.Documents) != 1 {
		t.Fatalf("documents len = %d, want 1", len(batch.Documents))
	}
	content := "【title】:\nHello\n\n【description】:\nWorld"
	sum := md5.Sum([]byte(content))
	want := "mysql:mydb:" + hex.EncodeToString(sum[:])
	if batch.Documents[0].SourceID != want {
		t.Fatalf("source id = %q, want %q", batch.Documents[0].SourceID, want)
	}
}

// TestMySQLConnectorValidate verifies the connection probe and failure paths.
func TestMySQLConnectorValidate(t *testing.T) {
	connector := newFixtureMySQLConnector(t, nil, func(mock sqlmock.Sqlmock) {
		mock.ExpectPing()
	})
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	missing := newFixtureMySQLConnector(t, map[string]any{
		"host":     "127.0.0.1",
		"port":     3306,
		"database": "mydb",
		"credentials": map[string]any{
			"username": "",
			"password": "secret",
		},
	}, nil)
	if err := missing.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("Validate error = %v", err)
	}
}

// TestMySQLConnectorSanitizeQuery verifies markdown fence tolerance.
func TestMySQLConnectorSanitizeQuery(t *testing.T) {
	connector := &MySQLConnector{}
	cases := []struct {
		in   string
		want string
	}{
		{in: "SELECT * FROM t", want: "SELECT * FROM t"},
		{in: "```sql\nSELECT * FROM t\n```", want: "SELECT * FROM t"},
		{in: "sql\nSELECT * FROM t", want: "SELECT * FROM t"},
		{in: "   ", want: ""},
	}
	for _, tc := range cases {
		if got := connector.sanitizeQuery(tc.in); got != tc.want {
			t.Fatalf("sanitizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestMySQLConnectorStripOrderBy verifies trailing top-level ORDER BY removal.
func TestMySQLConnectorStripOrderBy(t *testing.T) {
	connector := &MySQLConnector{}
	cases := []struct {
		in   string
		want string
	}{
		{in: "SELECT * FROM t ORDER BY id", want: "SELECT * FROM t"},
		{in: "SELECT ROW_NUMBER() OVER (ORDER BY id) FROM t", want: "SELECT ROW_NUMBER() OVER (ORDER BY id) FROM t"},
		{in: "SELECT * FROM t", want: "SELECT * FROM t"},
	}
	for _, tc := range cases {
		if got := connector.stripOrderBy(tc.in); got != tc.want {
			t.Fatalf("stripOrderBy(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
