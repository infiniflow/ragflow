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
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// newFixturePostgresConnector builds a PostgreSQL connector backed by sqlmock.
func newFixturePostgresConnector(t *testing.T, config map[string]any, expect func(mock sqlmock.Sqlmock)) *PostgreSQLConnector {
	t.Helper()
	if config == nil {
		config = map[string]any{
			"host":     "127.0.0.1",
			"port":     "5432",
			"database": "mydb",
			"credentials": map[string]any{
				"username": "postgres",
				"password": "secret",
			},
		}
	}
	connector, err := NewPostgreSQLConnector(config)
	if err != nil {
		t.Fatalf("NewPostgreSQLConnector failed: %v", err)
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

// TestPostgreSQLConnectorOpenSyncCustomQuery verifies a custom query produces documents.
func TestPostgreSQLConnectorOpenSyncCustomQuery(t *testing.T) {
	query := "SELECT * FROM products WHERE status = 'active'"
	updatedAt := mustTime(t, "2026-01-02T03:04:05Z")
	connector := newFixturePostgresConnector(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             "5432",
		"database":         "mydb",
		"query":            query,
		"content_columns":  "title,description",
		"metadata_columns": "id,category,updated_at",
		"id_column":        "id",
		"timestamp_column": "updated_at",
		"credentials": map[string]any{
			"username": "postgres",
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
	if doc.SourceID != "postgresql:mydb:7" {
		t.Fatalf("source id = %q", doc.SourceID)
	}
	if doc.SemanticIdentifier != "Hello/World" {
		t.Fatalf("semantic identifier = %q", doc.SemanticIdentifier)
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
	if _, err = session.NextBatch(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("NextBatch EOF = %v", err)
	}
}

// TestPostgreSQLConnectorOpenSyncIncrementalWindow verifies the ISO-8601 timestamp filter SQL.
func TestPostgreSQLConnectorOpenSyncIncrementalWindow(t *testing.T) {
	connector := newFixturePostgresConnector(t, map[string]any{
		"host":             "127.0.0.1",
		"port":             "5432",
		"database":         "mydb",
		"query":            "SELECT * FROM products",
		"timestamp_column": "updated_at",
		"credentials": map[string]any{
			"username": "postgres",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		expected := "SELECT * FROM (SELECT * FROM products) AS ragflow_src " +
			"WHERE ragflow_src.updated_at >= '2026-01-01T00:00:00Z' AND ragflow_src.updated_at <= '2026-01-02T00:00:00Z'"
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

// TestPostgreSQLConnectorOpenSyncAllTables verifies the information_schema table listing.
func TestPostgreSQLConnectorOpenSyncAllTables(t *testing.T) {
	connector := newFixturePostgresConnector(t, map[string]any{
		"host":      "127.0.0.1",
		"port":      "5432",
		"database":  "mydb",
		"id_column": "id",
		"credentials": map[string]any{
			"username": "postgres",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'")).WillReturnRows(
			sqlmock.NewRows([]string{"table_name"}).AddRow("products").AddRow("orders"),
		)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "public"."products"`)).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Product"),
		)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "public"."orders"`)).WillReturnRows(
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
	if len(ids) != 2 || ids[0] != "postgresql:mydb:1" || ids[1] != "postgresql:mydb:2" {
		t.Fatalf("source ids = %v", ids)
	}
}

// TestPostgreSQLConnectorOpenPrune verifies Python-compatible slim IDs.
func TestPostgreSQLConnectorOpenPrune(t *testing.T) {
	connector := newFixturePostgresConnector(t, map[string]any{
		"host":            "127.0.0.1",
		"port":            "5432",
		"database":        "mydb",
		"query":           "SELECT * FROM products",
		"content_columns": "title,description",
		"id_column":       "id",
		"credentials": map[string]any{
			"username": "postgres",
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
		batch.Documents[0].SourceID != "postgresql:mydb:3" ||
		batch.Documents[1].SourceID != "postgresql:mydb:4" {
		t.Fatalf("slim documents = %+v", batch.Documents)
	}
}

// TestPostgreSQLConnectorDSN verifies connector-controlled sslmode and connect_timeout.
func TestPostgreSQLConnectorDSN(t *testing.T) {
	var captured string
	connector := newFixturePostgresConnector(t, map[string]any{
		"host":     "127.0.0.1",
		"port":     "5432",
		"database": "mydb",
		"credentials": map[string]any{
			"username": "postgres",
			"password": "p@ss:word",
		},
	}, nil)
	connector.openDB = func(dsn string) (*sql.DB, error) {
		captured = dsn
		return nil, nil
	}
	if _, err := connector.open(); err != nil {
		t.Fatalf("open failed: %v", err)
	}
	parsed, err := url.Parse(captured)
	if err != nil {
		t.Fatalf("parse dsn %q: %v", captured, err)
	}
	if got := parsed.Query().Get("sslmode"); got != "prefer" {
		t.Fatalf("sslmode = %q", got)
	}
	if got := parsed.Query().Get("connect_timeout"); got != "30" {
		t.Fatalf("connect_timeout = %q", got)
	}
	if parsed.User.Username() != "postgres" {
		t.Fatalf("username = %q", parsed.User.Username())
	}
	if pass, _ := parsed.User.Password(); pass != "p@ss:word" {
		t.Fatalf("password = %q", pass)
	}
	if parsed.Host != "127.0.0.1:5432" || parsed.Path != "/mydb" {
		t.Fatalf("host/path = %q %q", parsed.Host, parsed.Path)
	}
}

// TestPostgreSQLConnectorOpenSyncMixedCaseTable verifies schema-qualified
// quoting of a catalog-discovered mixed-case table name.
func TestPostgreSQLConnectorOpenSyncMixedCaseTable(t *testing.T) {
	connector := newFixturePostgresConnector(t, map[string]any{
		"host":      "127.0.0.1",
		"port":      "5432",
		"database":  "mydb",
		"id_column": "id",
		"credentials": map[string]any{
			"username": "postgres",
			"password": "secret",
		},
	}, func(mock sqlmock.Sqlmock) {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_type = 'BASE TABLE'")).WillReturnRows(
			sqlmock.NewRows([]string{"table_name"}).AddRow("MixedCase"),
		)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "public"."MixedCase"`)).WillReturnRows(
			sqlmock.NewRows([]string{"id", "name"}).AddRow(1, "Item"),
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
	if len(batch.Documents) != 1 || batch.Documents[0].SourceID != "postgresql:mydb:1" {
		t.Fatalf("documents = %+v", batch.Documents)
	}
}

// TestPostgreSQLConnectorValidate verifies the probe and dialect-specific error message.
func TestPostgreSQLConnectorValidate(t *testing.T) {
	connector := newFixturePostgresConnector(t, nil, func(mock sqlmock.Sqlmock) {
		mock.ExpectPing()
	})
	if err := connector.Validate(context.Background()); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	missing := newFixturePostgresConnector(t, map[string]any{
		"host":     "127.0.0.1",
		"port":     "5432",
		"database": "mydb",
		"credentials": map[string]any{
			"username": "",
			"password": "secret",
		},
	}, nil)
	if err := missing.Validate(context.Background()); err == nil || !strings.Contains(err.Error(), "postgresql") {
		t.Fatalf("Validate error = %v", err)
	}
}
