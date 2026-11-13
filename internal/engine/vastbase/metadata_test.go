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
