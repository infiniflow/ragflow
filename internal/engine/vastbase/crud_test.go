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
	"errors"
	"reflect"
	"regexp"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

const listColumnsQuery = "SELECT column_name, data_type, column_default, is_nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1"

// expectExistingChunkStore wires the probes ChunkStoreExists runs against a
// fully provisioned PG-mode chunk table.
func expectExistingChunkStore(mock sqlmock.Sqlmock, table string) {
	expectCount(mock, tableCountQuery(), table)
	for _, column := range chunkIndexColumns {
		expectCount(mock, indexCountQuery(), table, regularIndexName(table, column))
	}
	expectCount(mock, indexCountQuery(), table, "text_gin_idx_"+table)
}

func expectListColumns(mock sqlmock.Sqlmock, table string, columns ...[2]string) {
	expectCount(mock, tableCountQuery(), table)
	rows := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
	for _, column := range columns {
		rows.AddRow(column[0], column[1], nil, "YES")
	}
	mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs(table).WillReturnRows(rows)
}

func TestInsertChunksRunsDeleteThenInsertInOneTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	// ChunkStoreExists: table present with every static index.
	expectExistingChunkStore(mock, "t1")
	// Existing vector column and graph index: no DDL. ensureDynamicColumns
	// also probes the vector column (it is not part of the static schema).
	expectCount(mock, columnCountQuery(), "t1", "q_3_vec")
	expectCount(mock, indexCountQuery(), "t1", regularIndexName("t1", "q_3_vec_graph"))
	expectCount(mock, columnCountQuery(), "t1", "q_3_vec")
	// backfillVectorColumns probes the live column set.
	expectListColumns(mock, "t1",
		[2]string{"id", "varchar"}, [2]string{"kb_id", "varchar"},
		[2]string{"content_ltks", "text"}, [2]string{"q_3_vec", "text"},
	)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM "t1" WHERE "id" IN ($1)`)).
		WithArgs("c1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO "t1" ("content_ltks", "id", "kb_id", "q_3_vec") VALUES ($1,$2,$3,$4)`)).
		WithArgs("hello", "c1", "kb-1", "[0.1,0.2,0.3]").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	inserted, err := engine.InsertChunks(context.Background(), []map[string]interface{}{{
		"id": "c1", "content_ltks": "hello", "q_3_vec": []interface{}{0.1, 0.2, 0.3},
	}}, "t1", "kb-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(inserted) != 0 {
		t.Fatalf("InsertChunks returns the Python connector's empty id list, got %#v", inserted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertChunksGroupsByColumnSet(t *testing.T) {
	documents := []map[string]interface{}{
		{"id": "a", "title_tks": "x"},
		{"id": "b", "title_tks": "y", "available_int": 1},
		{"id": "c", "title_tks": "z"},
	}
	groups := groupByColumnSet(documents)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	// Groups are ordered by their sorted column-set key, so the
	// available_int group sorts first.
	if len(groups[0]) != 1 || len(groups[1]) != 2 {
		t.Fatalf("group sizes = %d, %d", len(groups[0]), len(groups[1]))
	}
}

func TestInsertChunksRequiresVectorSizeForNewStore(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	if _, err := engine.InsertChunks(context.Background(),
		[]map[string]interface{}{{"id": "c1", "content_ltks": "x"}}, "t1", "kb-1"); err == nil {
		t.Fatal("creating a new store without an inferable vector dimension must fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChunksShiftsWherePlaceholdersAfterSet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectCount(mock, tableCountQuery(), "t1")
	expectListColumns(mock, "t1",
		[2]string{"id", "varchar"}, [2]string{"kb_id", "varchar"}, [2]string{"title_tks", "text"},
	)
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE "t1" SET "title_tks" = $1 WHERE "id" = $2 AND "kb_id" = $3`)).
		WithArgs("new title", "c1", "kb-1").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := engine.UpdateChunks(context.Background(),
		map[string]interface{}{"id": "c1"},
		map[string]interface{}{"title_tks": "new title"},
		"t1", "kb-1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChunksRemoveStringResetsColumnDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectCount(mock, tableCountQuery(), "t1")
	expectListColumns(mock, "t1",
		[2]string{"id", "varchar"}, [2]string{"kb_id", "varchar"}, [2]string{"title_tks", "text"},
	)
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE "t1" SET "title_tks" = DEFAULT WHERE "id" = $1 AND "kb_id" = $2`)).
		WithArgs("c1", "kb-1").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := engine.UpdateChunks(context.Background(),
		map[string]interface{}{"id": "c1"},
		map[string]interface{}{"remove": "title_tks"},
		"t1", "kb-1"); err != nil {
		t.Fatal(err)
	}
	// Removing a column the table lacks is an error, not a silent pass.
	expectCount(mock, tableCountQuery(), "t1")
	expectListColumns(mock, "t1", [2]string{"id", "varchar"}, [2]string{"kb_id", "varchar"})
	if err := engine.UpdateChunks(context.Background(),
		map[string]interface{}{"id": "c1"},
		map[string]interface{}{"remove": "ghost_col"},
		"t1", "kb-1"); err == nil {
		t.Fatal("unknown remove column must fail")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChunksKeywordRemovalReadsModifiesWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectCount(mock, tableCountQuery(), "t1")
	expectListColumns(mock, "t1",
		[2]string{"id", "varchar"}, [2]string{"kb_id", "varchar"},
		[2]string{"important_kwd", "text"},
	)
	// Read affected rows' current lists.
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT "important_kwd", "id" FROM "t1" WHERE "id" = $1 AND "kb_id" = $2`)).
		WithArgs("c1", "kb-1").
		WillReturnRows(sqlmock.NewRows([]string{"important_kwd", "id"}).AddRow("alpha###beta", "c1"))
	// Write the shortened list for the matching rows. The filter args keep
	// their $1/$2 numbering (they are bound first); the SET value continues
	// as $3 and the id list after it.
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE "t1" SET "important_kwd" = $3 WHERE "id" = $1 AND "kb_id" = $2 AND id IN ($4)`)).
		WithArgs("c1", "kb-1", "alpha", "c1").WillReturnResult(sqlmock.NewResult(0, 1))

	if err := engine.UpdateChunks(context.Background(),
		map[string]interface{}{"id": "c1"},
		map[string]interface{}{"remove": map[string]interface{}{"important_kwd": "beta"}},
		"t1", "kb-1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChunksMemoryContentRewritesTokenizedCompanion(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatB}, db)

	expectCount(mock, tableCountQuery(), "memory_t1")
	expectListColumns(mock, "memory_t1",
		[2]string{"id", "varchar"}, [2]string{"memory_id", "varchar"},
		[2]string{"content_ltks", "text"}, [2]string{"tokenized_content_ltks", "text"},
	)
	mock.ExpectExec(regexp.QuoteMeta(
		`UPDATE "memory_t1" SET "content_ltks" = $1, "tokenized_content_ltks" = $2 WHERE "id" = $3 AND "memory_id" = $4`)).
		WithArgs("new text", sqlmock.AnyArg(), "m1_1", "mem-1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := engine.UpdateChunks(context.Background(),
		map[string]interface{}{"id": "m1_1"},
		map[string]interface{}{"content": "new text"},
		"memory_t1", "mem-1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChunksMissingTableReportsIndexNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	err = engine.UpdateChunks(context.Background(),
		map[string]interface{}{"id": "c1"}, map[string]interface{}{"title_tks": "x"}, "t1", "kb-1")
	if !errors.Is(err, types.ErrIndexNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteChunksScopesByDataset(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectCount(mock, tableCountQuery(), "t1")
	expectListColumns(mock, "t1",
		[2]string{"id", "varchar"}, [2]string{"kb_id", "varchar"}, [2]string{"doc_id", "varchar"},
	)
	mock.ExpectExec(regexp.QuoteMeta(
		`DELETE FROM "t1" WHERE "doc_id" = $1 AND "kb_id" = $2`)).
		WithArgs("doc-1", "kb-1").WillReturnResult(sqlmock.NewResult(0, 4))

	deleted, err := engine.DeleteChunks(context.Background(),
		map[string]interface{}{"doc_id": "doc-1"}, "t1", "kb-1")
	if err != nil || deleted != 4 {
		t.Fatalf("deleted = %d err = %v", deleted, err)
	}

	// Missing tables delete nothing without error.
	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	deleted, err = engine.DeleteChunks(context.Background(), nil, "t1", "kb-1")
	if err != nil || deleted != 0 {
		t.Fatalf("deleted = %d err = %v", deleted, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGetChunkScopedQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	expectCount(mock, tableCountQuery(), "t1")
	expectListColumns(mock, "t1",
		[2]string{"id", "varchar"}, [2]string{"kb_id", "varchar"},
		[2]string{"content_ltks", "text"}, [2]string{"important_kwd", "text"},
	)
	mock.ExpectQuery(regexp.QuoteMeta(
		`SELECT * FROM "t1" WHERE "id" = $1 AND "kb_id" IN ($2) LIMIT 1`)).
		WithArgs("c1", "kb-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "content_ltks", "important_kwd"}).
			AddRow("c1", "hello", "alpha###beta"))

	document, err := engine.GetChunk(context.Background(), "t1", "c1", []string{"kb-1"})
	if err != nil {
		t.Fatal(err)
	}
	chunk, ok := document.(map[string]interface{})
	if !ok || chunk["id"] != "c1" || chunk["content_ltks"] != "hello" {
		t.Fatalf("chunk = %#v", document)
	}
	if !reflect.DeepEqual(chunk["important_kwd"], []string{"alpha", "beta"}) {
		t.Fatalf("keyword decode = %#v", chunk["important_kwd"])
	}

	// Memory rows report not-found instead of nil.
	mock.ExpectQuery(regexp.QuoteMeta(tableCountQuery())).WithArgs("memory_t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	if _, err := engine.GetChunk(context.Background(), "memory_t1", "m1_1", []string{"mem-1"}); !errors.Is(err, types.ErrDocumentNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTableKindRouting(t *testing.T) {
	cases := []struct {
		table    string
		datasets []string
		want     string
	}{
		{"memory_t1", []string{"mem-1"}, "memory"},
		{"skill_t1", nil, "skill"},
		{"t1", []string{"skill"}, "skill"},
		{"ragflow_doc_meta_tenant", nil, "metadata"},
		{"t1", []string{"kb-1"}, "chunk"},
	}
	for _, test := range cases {
		if got := tableKind(test.table, test.datasets...); got != test.want {
			t.Fatalf("tableKind(%q, %v) = %q, want %q", test.table, test.datasets, got, test.want)
		}
	}
}
