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
	"regexp"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"

	"github.com/DATA-DOG/go-sqlmock"
)

// An existing row is updated in place: doc_id and create_time survive the
// replace, and no INSERT runs.
func TestUpdateMetadataUpdatesExistingRowInPlace(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "ragflow_doc_meta_t1" SET meta_fields = $1 WHERE id = $2`)).
		WithArgs(`{"k":"v"}`, "md-1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := engine.UpdateMetadata(context.Background(), "md-1", "kb-1",
		map[string]interface{}{"k": "v"}, "t1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A missing row falls back to INSERT, which must carry doc_id so later
// doc_id-filtered lookups still find the metadata.
func TestUpdateMetadataInsertsMissingRowWithDocID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "ragflow_doc_meta_t1" SET meta_fields = $1 WHERE id = $2`)).
		WithArgs(`{"k":"v"}`, "md-2").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(regexp.QuoteMeta(
		`INSERT INTO "ragflow_doc_meta_t1" (id, kb_id, doc_id, meta_fields) VALUES ($1, $2, $3, $4)`)).
		WithArgs("md-2", "kb-1", "md-2", `{"k":"v"}`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := engine.UpdateMetadata(context.Background(), "md-2", "kb-1",
		map[string]interface{}{"k": "v"}, "t1"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// SearchMetadata pages with LIMIT <limit> OFFSET <offset>: with the two values
// swapped, the first page of a scan (offset 0) read no rows at all.
func TestSearchMetadataLimitAndOffset(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	engine := newEngineWithDB(config.VastbaseConfig{DBCompatibility: compatPG}, db)
	const table = "ragflow_doc_meta_t1"

	expectCount(mock, tableCountQuery(), table) // tableExists
	expectCount(mock, tableCountQuery(), table) // listTableColumns
	columns := sqlmock.NewRows([]string{"column_name", "data_type", "column_default", "is_nullable"})
	for _, column := range []string{"id", "kb_id", "meta_fields"} {
		columns.AddRow(column, "text", nil, "YES")
	}
	mock.ExpectQuery(regexp.QuoteMeta(listColumnsQuery)).WithArgs(table).WillReturnRows(columns)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(id) FROM "ragflow_doc_meta_t1"`)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(12345))
	mock.ExpectQuery(`LIMIT 5000 OFFSET 10000$`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "kb_id", "meta_fields"}).AddRow("doc-10000", "kb-1", `{}`))

	res, err := engine.SearchMetadata(context.Background(), &types.SearchMetadataRequest{
		TenantID: "t1",
		Offset:   10000,
		Limit:    5000,
		Filter:   map[string]interface{}{"kb_id": []string{"kb-1"}},
		OrderBy:  (&types.OrderByExpr{}).Asc("id"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.MetadataRecords) != 1 || res.Total != 12345 {
		t.Fatalf("SearchMetadata = %d rows, total %d; want 1 row, total 12345", len(res.MetadataRecords), res.Total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
