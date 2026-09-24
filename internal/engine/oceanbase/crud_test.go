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
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInsertChunksFindsLaterVectorAndWaitsForSeekDBIndexRefresh(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := newEngineWithDB("seekdb", "legacy_doc", db)
	engine.flags.enableFullTextSearch = false
	engine.indexRefreshEnabled = true

	tableName := "memory_tenant_1"
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")).
		WithArgs("legacy_doc", tableName).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	for _, column := range memoryIndexColumns {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?")).
			WithArgs("legacy_doc", tableName, regularIndexName(tableName, column)).
			WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?")).
		WithArgs("legacy_doc", tableName, "q_2_vec").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?")).
		WithArgs("legacy_doc", tableName, "q_2_vec_idx").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectExec("REPLACE INTO `memory_tenant_1`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("REPLACE INTO `memory_tenant_1`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec(regexp.QuoteMeta("CALL DBMS_INDEX_MANAGER.REFRESH()")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	_, err = engine.InsertChunks(t.Context(), []map[string]interface{}{
		{
			"id": "memory-1_1", "message_id": "1", "memory_id": "memory-1",
			"content": "without a vector",
		},
		{
			"id": "memory-1_2", "message_id": "2", "memory_id": "memory-1",
			"content": "hello", "content_embed": []float64{0.1, 0.2},
		},
	}, tableName, "memory-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertChunksReturnsNormalizationErrorWithoutPanic(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := newEngineWithDB("seekdb", "legacy_doc", db)
	engine.flags.enableFullTextSearch = false
	tableName := "memory_tenant_1"
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")).
		WithArgs("legacy_doc", tableName).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	for _, column := range memoryIndexColumns {
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?")).
			WithArgs("legacy_doc", tableName, regularIndexName(tableName, column)).
			WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND COLUMN_NAME = ?")).
		WithArgs("legacy_doc", tableName, "q_2_vec").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = ?")).
		WithArgs("legacy_doc", tableName, "q_2_vec_idx").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))
	mock.ExpectBegin()
	mock.ExpectRollback()

	_, err = engine.InsertChunks(t.Context(), []map[string]interface{}{{
		"id": "memory-1_1", "message_id": "1", "q_2_vec": "invalid",
	}}, tableName, "memory-1")
	if err == nil {
		t.Fatal("InsertChunks() error = nil, want vector normalization error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChunksRejectsUnsupportedRemoveType(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := newEngineWithDB("oceanbase", "legacy_doc", db)
	tableName := "memory_tenant_1"
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?")).
		WithArgs("legacy_doc", tableName).
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(1))

	err = engine.UpdateChunks(t.Context(), map[string]interface{}{"id": "memory-1_1"},
		map[string]interface{}{"remove": []string{"forget_at"}}, tableName, "memory-1")
	if err == nil {
		t.Fatal("UpdateChunks() error = nil, want unsupported remove type error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForIndexRefreshIsNoOpWhenDisabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	engine := newEngineWithDB("seekdb", "legacy_doc", db)
	if err := engine.waitForIndexRefresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
