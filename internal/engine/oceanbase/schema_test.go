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
	"encoding/json"
	"os"
	"path/filepath"
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

func TestTableCollationMatchesEngine(t *testing.T) {
	for _, tc := range []struct{ engine, collation string }{{"oceanbase", "utf8mb4_unicode_ci"}, {"seekdb", "utf8mb4_general_ci"}} {
		t.Run(tc.engine, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			engine := newEngineWithDB(tc.engine, "test", db)
			query := regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")
			mock.ExpectQuery(query).WithArgs("test", "memory_test").WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))
			mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE `memory_test` (`id` VARCHAR(32)) DEFAULT CHARSET=utf8mb4 COLLATE=" + tc.collation + " ORGANIZATION=heap")).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery(query).WithArgs("test", "memory_test").WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
			if err := engine.ensureTable(t.Context(), "memory_test", []columnDefinition{{name: "id", typeSQL: "VARCHAR(32)"}}, "test_"); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// conf/infinity_mapping.json is the field list readers select by name. This
// engine rejects an unknown name before any SQL is sent, so a mapping field the
// engine has no column for fails at parse time once the tenant table exists.
// Keep the two in step instead of discovering the gap in production.
func TestChunkColumnsCoverInfinityMapping(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "conf", "infinity_mapping.json"))
	if err != nil {
		t.Fatalf("read conf/infinity_mapping.json: %v", err)
	}
	var mapping map[string]json.RawMessage
	if err := json.Unmarshal(raw, &mapping); err != nil {
		t.Fatalf("parse conf/infinity_mapping.json: %v", err)
	}
	// Fields the mapping keeps as aliases of columns this engine already models,
	// plus chunk_order_int, which mapChunkField folds onto _order_id.
	aliases := map[string]bool{
		"docnm": true, "important_keywords": true, "questions": true,
		"content": true, "authors": true, "chunk_order_int": true,
	}
	selectable := validColumns("chunk")
	for field := range mapping {
		if aliases[field] {
			continue
		}
		if !selectable[field] {
			t.Errorf("field %q is declared in conf/infinity_mapping.json but is not a chunk column", field)
		}
		if _, _, err := buildSelectFields([]string{field}, "chunk"); err != nil {
			t.Errorf("selecting %q from a chunk table: %v", field, err)
		}
	}
}

// A column added to chunkColumns is only written into an existing table by
// CreateChunkStore, which walks chunkExtraColumns. Leaving a knowledge-compile
// field out of that list keeps ChunkStoreExists false and the ALTER never runs.
func TestKnowledgeCompileColumnsAreUpgraded(t *testing.T) {
	upgraded := make(map[string]bool, len(chunkExtraColumns))
	for _, column := range chunkExtraColumns {
		upgraded[column.name] = true
	}
	// Fields the parse and knowledge-compilation readers select.
	for _, name := range []string{
		"compile_kwd", "compilation_template_kind_kwd", "plan_kwd", "scope_kwd",
		"slug_kwd", "title_kwd", "page_type_kwd", "topic_kwd", "plan_group_kwd",
		"generation_kwd", "summary_with_weight", "name_kwd", "mention_count_int",
		"entity_names_kwd", "related_kb_pages_kwd", "outlinks_kwd", "outlinks_int",
		"depth_int", "source_chunk_ids", "source_doc_ids", "compilation_template_ids",
	} {
		if !upgraded[name] {
			t.Errorf("%q is missing from chunkExtraColumns, so existing tables never gain the column", name)
		}
	}
}

// The value encoders are driven by their own name sets, so a column stored as
// an array or JSON must be registered there or a read returns the raw string.
func TestMappingListColumnsKeepTheirEncoding(t *testing.T) {
	types := make(map[string]string, len(chunkColumns))
	for _, column := range chunkColumns {
		types[column.name] = column.typeSQL
	}
	for _, name := range []string{
		"source_chunk_ids", "source_doc_ids", "compilation_template_ids",
		"doc_ids_kwd", "entity_names_kwd", "entity_names", "outlinks_kwd",
		"related_kb_pages_kwd", "page_ids", "rechunked_from_chunk_ids", "aliases",
	} {
		if !arrayColumns[name] {
			t.Errorf("%q stores a list but is not registered in arrayColumns", name)
		}
		if !strings.Contains(types[name], "ARRAY(") {
			t.Errorf("%q is not an ARRAY column: %q", name, types[name])
		}
	}
	for _, name := range []string{"claims", "source_chunk_hashes"} {
		if !jsonColumns[name] {
			t.Errorf("%q stores an object payload but is not registered in jsonColumns", name)
		}
		if !strings.Contains(types[name], "JSON") {
			t.Errorf("%q is not a JSON column: %q", name, types[name])
		}
	}
}

// ChunkStoreExists is the gate that makes InsertChunks call CreateChunkStore,
// which is the only place an existing table gains a new column. A table that
// predates compile_kwd must therefore report not-ready, otherwise the additive
// DDL never runs and selects of that column fail.
func TestChunkStoreExistsDetectsMissingKnowledgeCompileColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB("seekdb", "legacy_doc", db)
	engine.flags.enableFullTextSearch = false

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")).
		WithArgs("legacy_doc", "ragflow_tenant_1").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	columnQuery := regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?")
	for _, column := range chunkExtraColumns {
		present := 1
		if column.name == "compile_kwd" {
			present = 0
		}
		mock.ExpectQuery(columnQuery).
			WithArgs("legacy_doc", "ragflow_tenant_1", column.name).
			WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(present))
		if present == 0 {
			break // an absent column answers false immediately
		}
	}

	ready, err := engine.ChunkStoreExists(t.Context(), "ragflow_tenant_1", "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	if ready {
		t.Fatal("a chunk table without compile_kwd reported ready, so the additive upgrade would never run")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A chunk table gains columns only when CreateChunkStore runs, and reads can
// run first. The default projection must therefore follow the table's real
// schema instead of a static list that includes a column the table lacks.
func TestChunkDefaultProjectionFollowsTheTableSchema(t *testing.T) {
	fieldsSQL, _, err := buildSelectFields(nil, "chunk")
	if err != nil {
		t.Fatalf("buildSelectFields: %v", err)
	}
	if fieldsSQL != "*" {
		t.Errorf("default chunk projection = %q, want %q", fieldsSQL, "*")
	}
	qualified, _, err := buildQualifiedSelectFields(nil, "chunk", "t")
	if err != nil {
		t.Fatalf("buildQualifiedSelectFields: %v", err)
	}
	if qualified != "`t`.*" {
		t.Errorf("qualified default chunk projection = %q, want %q", qualified, "`t`.*")
	}
}
