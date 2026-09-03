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

package dao

import (
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMigrateTenantLLMPrimaryKeyScopesUniqueIndexLookup(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta("SELECT DATABASE()")).
		WillReturnRows(sqlmock.NewRows([]string{"DATABASE()"}).AddRow("ragflow"))
	mock.ExpectQuery("SELECT SCHEMA_NAME from Information_schema.SCHEMATA").
		WithArgs("ragflow%", "ragflow").
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}).AddRow("ragflow"))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM information_schema.tables").
		WithArgs("ragflow", "tenant_llm", "BASE TABLE").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM INFORMATION_SCHEMA.COLUMNS").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM INFORMATION_SCHEMA.COLUMNS").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))
	mock.ExpectExec("ALTER TABLE tenant_llm ADD COLUMN id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY FIRST").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM INFORMATION_SCHEMA.STATISTICS\\s+WHERE TABLE_SCHEMA = DATABASE\\(\\)\\s+AND TABLE_NAME = 'tenant_llm' AND INDEX_NAME = 'idx_tenant_llm_unique'").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(0))
	mock.ExpectExec("ALTER TABLE tenant_llm ADD UNIQUE INDEX idx_tenant_llm_unique \\(tenant_id, llm_factory, llm_name\\)").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := migrateTenantLLMPrimaryKey(t.Context(), db); err != nil {
		t.Fatalf("migrateTenantLLMPrimaryKey: %v", err)
	}
}
