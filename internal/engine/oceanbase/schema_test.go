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

package oceanbase

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDropChunkStoreIgnoresMissingSharedTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("seekdb", "legacy_doc", db)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")).
		WithArgs("legacy_doc", "memory_tenant_1").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))

	if err = engine.DropChunkStore(t.Context(), "memory_tenant_1", "memory_1"); err != nil {
		t.Fatalf("DropChunkStore() error = %v", err)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDropChunkStoreDeletesOnlyScopedRows(t *testing.T) {
	tests := []struct {
		name      string
		tableName string
		datasetID string
		fieldName string
	}{
		{name: "memory", tableName: "memory_tenant_1", datasetID: "memory_1", fieldName: "memory_id"},
		{name: "chunk", tableName: "ragflow_tenant_1", datasetID: "kb_1", fieldName: "kb_id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			engine := newEngineWithDB("oceanbase", "legacy_doc", db)

			mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")).
				WithArgs("legacy_doc", test.tableName).
				WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
			deleteSQL := "DELETE FROM `" + test.tableName + "` WHERE `" + test.fieldName + "` = ?"
			mock.ExpectExec(regexp.QuoteMeta(deleteSQL)).
				WithArgs(test.datasetID).
				WillReturnResult(sqlmock.NewResult(0, 0))

			if err = engine.DropChunkStore(t.Context(), test.tableName, test.datasetID); err != nil {
				t.Fatalf("DropChunkStore() error = %v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDropChunkStoreDropsTableForUnscopedDataset(t *testing.T) {
	tests := []struct {
		name      string
		datasetID string
	}{
		{name: "empty dataset", datasetID: ""},
		{name: "skill dataset", datasetID: "skill"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			engine := newEngineWithDB("oceanbase", "legacy_doc", db)

			mock.ExpectExec(regexp.QuoteMeta("DROP TABLE IF EXISTS `ragflow_tenant_1`")).
				WillReturnResult(sqlmock.NewResult(0, 0))

			if err = engine.DropChunkStore(t.Context(), "ragflow_tenant_1", test.datasetID); err != nil {
				t.Fatalf("DropChunkStore() error = %v", err)
			}
			if err = mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRegularIndexNamePreservesShortNamesAndCapsLongNames(t *testing.T) {
	if got, want := regularIndexName("memory_tenant_1", "memory_id"), "ix_memory_tenant_1_memory_id"; got != want {
		t.Fatalf("regularIndexName() = %q, want %q", got, want)
	}

	tableName := strings.Repeat("tenant", 10)
	first := regularIndexName(tableName, "kb_id")
	second := regularIndexName(tableName, "doc_id")
	if len(first) > maxIndexNameLength {
		t.Fatalf("index name length = %d, want <= %d: %q", len(first), maxIndexNameLength, first)
	}
	if first == second {
		t.Fatalf("different index inputs produced the same name: %q", first)
	}
	if !identifierPattern.MatchString(first) || first != regularIndexName(tableName, "kb_id") {
		t.Fatalf("index name is invalid or nondeterministic: %q", first)
	}

	pythonTableName := "ragflow_12345678-1234-1234-1234-123456789012"
	if got, want := regularIndexName(pythonTableName, "create_timestamp_flt"), "ix_ragflow_12345678-1234-1234-1234-123456789012_create_t_69b6"; got != want {
		t.Fatalf("SQLAlchemy-compatible index name = %q, want %q", got, want)
	}
}

func TestFindVectorColumnUsesExpectedColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("seekdb", "legacy_doc", db)

	query := "SELECT COLUMN_NAME FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME REGEXP '^q_[0-9]+_vec$' AND COLUMN_NAME = ? ORDER BY COLUMN_NAME LIMIT 1"
	mock.ExpectQuery(regexp.QuoteMeta(query)).
		WithArgs("legacy_doc", "memory_tenant_1", "q_1024_vec").
		WillReturnRows(sqlmock.NewRows([]string{"COLUMN_NAME"}).AddRow("q_1024_vec"))

	column, err := engine.findVectorColumn(t.Context(), "memory_tenant_1", "q_1024_vec")
	if err != nil {
		t.Fatal(err)
	}
	if column != "q_1024_vec" {
		t.Fatalf("findVectorColumn() = %q, want q_1024_vec", column)
	}
	if err = mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
