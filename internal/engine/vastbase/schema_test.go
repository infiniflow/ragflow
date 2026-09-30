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

package vastbase

import (
	"context"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

// expectCount wires one information_schema/pg_indexes existence probe that
// scans a single integer.
func expectCount(mock sqlmock.Sqlmock, query string, args ...driver.Value) *sqlmock.ExpectedQuery {
	return mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
}

func tableCountQuery() string {
	return "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1"
}

func indexCountQuery() string {
	return "SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND tablename = $1 AND indexname = $2"
}

func columnCountQuery() string {
	return "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2"
}

// expectIndexProvision expects the withDDLLock triple for one index:
// absence probe, DDL statement, existence confirmation.
func expectIndexProvision(mock sqlmock.Sqlmock, table, indexName, ddl string) {
	mock.ExpectQuery(regexp.QuoteMeta(indexCountQuery())).WithArgs(table, indexName).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta(ddl)).WillReturnResult(sqlmock.NewResult(0, 0))
	expectCount(mock, indexCountQuery(), table, indexName)
}

func TestCreateMemoryStoreDDLGolden(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	createTable := `CREATE TABLE IF NOT EXISTS "memory_t1" (` +
		`"id" varchar(256) NOT NULL PRIMARY KEY, "message_id" varchar(256) DEFAULT '', ` +
		`"message_type_kwd" varchar(64) DEFAULT '', "source_id" text DEFAULT '', ` +
		`"memory_id" varchar(256) DEFAULT '', "user_id" varchar(256) DEFAULT '', ` +
		`"agent_id" varchar(256) DEFAULT '', "session_id" varchar(256) DEFAULT '', ` +
		`"zone_id" integer DEFAULT 0, "valid_at" varchar(64) DEFAULT '', ` +
		`"invalid_at" varchar(64) DEFAULT '', "forget_at" varchar(64) DEFAULT '', ` +
		`"status_int" integer DEFAULT 1, "content_ltks" text DEFAULT '', ` +
		`"tokenized_content_ltks" text DEFAULT '')`

	// ensureTable: absent probe, DDL, existence confirmation.
	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("memory_t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta(createTable)).WillReturnResult(sqlmock.NewResult(0, 0))
	expectCount(mock, tableCountQuery(), "memory_t1")

	for _, column := range memoryIndexColumns {
		expectIndexProvision(mock, "memory_t1", regularIndexName("memory_t1", column),
			fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON "memory_t1" (%s)`,
				quoteIdent(regularIndexName("memory_t1", column)), quoteIdent(column)))
	}
	// vectorSize 0: no vector column or index is created.

	if err := engine.CreateChunkStore(context.Background(), "memory_t1", "mem-1", 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateChunkStoreBModeProvisionsFullTextPerColumn(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatB}, db)

	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// The chunk DDL is asserted by fragments: PK on id, text DEFAULT '',
	// array columns without DEFAULT, and the literal column set from the
	// Python mapping.
	createChunkPattern := strings.Join([]string{
		regexp.QuoteMeta(`CREATE TABLE IF NOT EXISTS "t1" ("id" varchar(256) NOT NULL PRIMARY KEY`),
		regexp.QuoteMeta(`"docnm_kwd" text DEFAULT ''`),
		regexp.QuoteMeta(`"available_int" integer DEFAULT 1`),
		regexp.QuoteMeta(`"position_int" integer[]`),
		regexp.QuoteMeta(`"metadata" text DEFAULT ''`),
		regexp.QuoteMeta(`"mom_with_weight" text DEFAULT '')`),
	}, ".*")
	mock.ExpectExec(createChunkPattern).WillReturnResult(sqlmock.NewResult(0, 0))
	expectCount(mock, tableCountQuery(), "t1")

	for _, column := range chunkIndexColumns {
		expectIndexProvision(mock, "t1", regularIndexName("t1", column),
			fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON "t1" (%s)`,
				quoteIdent(regularIndexName("t1", column)), quoteIdent(column)))
	}
	for _, field := range fullTextFields {
		indexName := regularIndexName("t1", field+"_fulltext")
		expectIndexProvision(mock, "t1", indexName,
			fmt.Sprintf(`ALTER TABLE "t1" ADD INDEX %s USING "fulltext" (%s)`,
				quoteIdent(indexName), quoteIdent(field)))
	}

	if err := engine.CreateChunkStore(context.Background(), "t1", "kb-1", 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateChunkStoreBModeDegradesWhenFullTextIndexFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatB}, db)

	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta(`CREATE TABLE IF NOT EXISTS "t1" (`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectCount(mock, tableCountQuery(), "t1")
	for _, column := range chunkIndexColumns {
		expectIndexProvision(mock, "t1", regularIndexName("t1", column),
			fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s ON "t1" (%s)`,
				quoteIdent(regularIndexName("t1", column)), quoteIdent(column)))
	}
	// Every full-text index probe reports absence and the DDL fails: the
	// Python connector degrades to vector-only search here instead of
	// failing the store.
	for _, field := range fullTextFields {
		indexName := regularIndexName("t1", field+"_fulltext")
		mock.ExpectQuery(regexp.QuoteMeta(indexCountQuery())).WithArgs("t1", indexName).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(`ALTER TABLE "t1" ADD INDEX %s USING "fulltext"`,
			quoteIdent(indexName)))).WillReturnError(fmt.Errorf("fulltext extension missing"))
	}

	// vectorSize 0: no vector column or index is created; the failed
	// full-text DDL must not surface as an error.
	if err := engine.CreateChunkStore(context.Background(), "t1", "kb-1", 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureFullTextIndexesPGModeBuildsOneGIN(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expressions := make([]string, 0, len(fullTextFields))
	for _, field := range fullTextFields {
		expressions = append(expressions, fmt.Sprintf("to_tsvector('cn_tokenizer', %s)", quoteIdent(field)))
	}
	// The PG path probes and creates directly (no withDDLLock, failure only
	// warns), so there is no existence confirmation probe afterwards.
	mock.ExpectQuery(regexp.QuoteMeta(indexCountQuery())).WithArgs("t1", "text_gin_idx_t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(regexp.QuoteMeta(fmt.Sprintf(
		`CREATE INDEX IF NOT EXISTS "text_gin_idx_t1" ON "t1" USING gin (%s)`,
		strings.Join(expressions, ", ")))).WillReturnResult(sqlmock.NewResult(0, 0))

	if err := engine.ensureFullTextIndexes(context.Background(), "t1", fullTextFields); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureVectorColumnAndIndexUsesGraphIndex(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatB}, db)

	mock.ExpectQuery(regexp.QuoteMeta(columnCountQuery())).WithArgs("t1", "q_3_vec").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	// No IF NOT EXISTS in the ALTER: Vastbase rejects the clause, and
	// withDDLLock's existence probe makes it redundant.
	mock.ExpectExec(regexp.QuoteMeta(`ALTER TABLE "t1" ADD COLUMN "q_3_vec" floatvector(3)`)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectCount(mock, columnCountQuery(), "t1", "q_3_vec")

	expectIndexProvision(mock, "t1", regularIndexName("t1", "q_3_vec_graph"),
		`CREATE INDEX IF NOT EXISTS "ix_t1_q_3_vec_graph" ON "t1" USING graph_index ("q_3_vec" floatvector_cosine_ops) WITH (m=16, ef_construction=50)`)

	if err := engine.ensureVectorColumnAndIndex(context.Background(), "t1", 3); err != nil {
		t.Fatal(err)
	}
	if err := engine.ensureVectorColumnAndIndex(context.Background(), "t1", 0); err != nil {
		t.Fatal(err) // zero dimension is a no-op
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestDynamicColumnDefinitionTypes(t *testing.T) {
	if got := dynamicColumnDefinition("q_1024_vec"); got.typeSQL != "floatvector(1024)" || got.defaultSQL != "" {
		t.Fatalf("vector dynamic column = %#v", got)
	}
	if got := dynamicColumnDefinition("custom_field"); got.typeSQL != "varchar(256)" || got.defaultSQL != "''" {
		t.Fatalf("scalar dynamic column = %#v", got)
	}
}

func TestRegularIndexNameTruncatesWithinLimit(t *testing.T) {
	long := regularIndexName(strings.Repeat("t", 40), strings.Repeat("c", 30))
	// Truncation keeps headroom below the 63-char PG limit and appends a
	// stable 4-hex digest so distinct long names cannot collide after cut.
	if len(long) > maxIndexNameLength {
		t.Fatalf("truncated index name length = %d (%q)", len(long), long)
	}
	if !strings.HasPrefix(long, "ix_") || !regexp.MustCompile(`_[0-9a-f]{4}$`).MatchString(long) {
		t.Fatalf("truncated index name = %q", long)
	}
	if got := regularIndexName("t1", "kb_id"); got != "ix_t1_kb_id" {
		t.Fatalf("short index name = %q", got)
	}
}

func TestDropChunkStoreDeletesDatasetRowsButKeepsSharedTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatB}, db)

	expectCount(mock, tableCountQuery(), "t1")
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "t1" WHERE "kb_id" = $1`)).
		WithArgs("kb-1").WillReturnResult(sqlmock.NewResult(0, 7))
	if err := engine.DropChunkStore(context.Background(), "t1", "kb-1"); err != nil {
		t.Fatal(err)
	}

	expectCount(mock, tableCountQuery(), "memory_t1")
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "memory_t1" WHERE "memory_id" = $1`)).
		WithArgs("mem-1").WillReturnResult(sqlmock.NewResult(0, 3))
	if err := engine.DropChunkStore(context.Background(), "memory_t1", "mem-1"); err != nil {
		t.Fatal(err)
	}

	// Unscoped and skill drops remove the whole table.
	mock.ExpectExec(regexp.QuoteMeta(`DROP TABLE IF EXISTS "t1"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := engine.DropChunkStore(context.Background(), "t1", ""); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec(regexp.QuoteMeta(`DROP TABLE IF EXISTS "skill_t1"`)).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := engine.DropChunkStore(context.Background(), "skill_t1", "skill"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestChunkStoreExistsRequiresStaticIndexes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatB}, db)

	expectCount(mock, tableCountQuery(), "t1")
	for _, column := range chunkIndexColumns {
		expectCount(mock, indexCountQuery(), "t1", regularIndexName("t1", column))
	}
	for _, field := range fullTextFields {
		expectCount(mock, indexCountQuery(), "t1", regularIndexName("t1", field+"_fulltext"))
	}
	exists, err := engine.ChunkStoreExists(context.Background(), "t1", "kb-1")
	if err != nil || !exists {
		t.Fatalf("exists = %v err = %v", exists, err)
	}

	// One missing full-text index invalidates the store.
	expectCount(mock, tableCountQuery(), "t1")
	for _, column := range chunkIndexColumns {
		expectCount(mock, indexCountQuery(), "t1", regularIndexName("t1", column))
	}
	mock.ExpectQuery(regexp.QuoteMeta(indexCountQuery())).
		WithArgs("t1", regularIndexName("t1", fullTextFields[0]+"_fulltext")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	exists, err = engine.ChunkStoreExists(context.Background(), "t1", "kb-1")
	if err != nil || exists {
		t.Fatalf("exists = %v err = %v", exists, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
