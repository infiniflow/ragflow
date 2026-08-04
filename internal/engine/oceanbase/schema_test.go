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

	if err := engine.DropChunkStore(context.Background(), "memory_tenant_1", "memory_1"); err != nil {
		t.Fatalf("DropChunkStore() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
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

			if err := engine.DropChunkStore(context.Background(), test.tableName, test.datasetID); err != nil {
				t.Fatalf("DropChunkStore() error = %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
