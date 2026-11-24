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
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"ragflow/internal/entity"
)

// TestTenantModelInstanceAPIKeyColumnIsBoundedVarchar pins the MySQL migration
// type of tenant_model_instance.api_key. The column used to be declared
// varchar(512), so saving a provider key longer than 512 characters failed at
// the database layer with MySQL Error 1406 (#22422). Per the maintainer review
// of #22424 the replacement is a bounded varchar(4096), not TEXT: long keys
// must fit, but the column keeps a declared maximum. AutoMigrate derives the
// column type from the entity tag, so asserting the dialector's full data type
// guards both the bound change and the widening of existing installations.
func TestTenantModelInstanceAPIKeyColumnIsBoundedVarchar(t *testing.T) {
	db, _ := openMySQLSpecMock(t)

	s, err := schema.Parse(&entity.TenantModelInstance{}, &sync.Map{}, db.NamingStrategy)
	if err != nil {
		t.Fatalf("failed to parse TenantModelInstance schema: %v", err)
	}
	field, ok := s.FieldsByDBName["api_key"]
	if !ok {
		t.Fatal("api_key column missing from TenantModelInstance schema")
	}

	fullType := db.Migrator().FullDataTypeOf(field).SQL
	if fullType != "varchar(4096) NOT NULL" {
		t.Fatalf("mysql full data type for api_key = %q, want the bounded varchar(4096) NOT NULL form", fullType)
	}
	if strings.Contains(fullType, "512") || strings.Contains(fullType, "text") {
		t.Fatalf("mysql full data type for api_key = %q, must be neither the legacy varchar(512) nor TEXT", fullType)
	}
}

// openMySQLSpecMock opens a gorm session on the real mysql dialector with a
// sqlmock connection, so migration code paths that read INFORMATION_SCHEMA or
// issue DDL can be driven without a live server.
func openMySQLSpecMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	shared, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { shared.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "test:test@tcp(localhost:3306)/test?parseTime=True",
		Conn:                      shared,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}

// expectSchemaLookups mocks the catalog reads applyColumnSpec performs before
// deciding on DDL: the dialector's current-database resolution, the HasTable
// probe, and the live column definition.
func expectSchemaLookups(mock sqlmock.Sqlmock, table, column, liveType, isNullable string) {
	mock.ExpectQuery(`SELECT DATABASE\(\)`).
		WillReturnRows(sqlmock.NewRows([]string{"DATABASE()"}).AddRow("test"))
	mock.ExpectQuery(`SELECT SCHEMA_NAME from Information_schema.SCHEMATA`).
		WithArgs("test%", "test").
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}).AddRow("test"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM information_schema.tables`).
		WithArgs("test", table, "BASE TABLE").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(1))
	mock.ExpectQuery(`SELECT COLUMN_TYPE AS column_type, IS_NULLABLE AS is_nullable`).
		WithArgs(table, column).
		WillReturnRows(sqlmock.NewRows([]string{"column_type", "is_nullable"}).AddRow(liveType, isNullable))
}

// TestTenantModelInstanceAPIKeyWidenedBeforeBackfill pins the pre-backfill
// widening of tenant_model_instance.api_key. migrateModelData — whose base
// step copies tenant_llm keys into the column — runs before AutoMigrate, so
// the revision that lifts the 512-character cap also has to widen tables that
// still hold the legacy varchar(512): otherwise a long provider key fails the
// backfill INSERT (strict mode Error 1406, aborting startup) before
// AutoMigrate ever reaches the column. The spec must issue the ALTER exactly
// once on the narrow shape, to the bounded varchar(4096) target, and no DDL
// once converged.
func TestTenantModelInstanceAPIKeyWidenedBeforeBackfill(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	// sqlmock rejects unregistered statements but ExpectationsWereMet does not
	// report unexpected calls, so a regressed convergence guard could still
	// pass. Count the ALTER statements the session issues and assert the
	// converged call adds none.
	var alterStatements int
	if err := db.Callback().Raw().Before("gorm:raw").Register("test_count_alter_statements", func(tx *gorm.DB) {
		if strings.Contains(strings.ToUpper(tx.Statement.SQL.String()), "ALTER TABLE") {
			alterStatements++
		}
	}); err != nil {
		t.Fatalf("failed to register the ALTER counter: %v", err)
	}

	expectSchemaLookups(mock, "tenant_model_instance", "api_key", "varchar(512)", "NO")
	mock.ExpectExec(
		`ALTER TABLE tenant_model_instance MODIFY COLUMN api_key varchar\(4096\) NOT NULL`,
	).WillReturnResult(sqlmock.NewResult(0, 0))

	if err := applyColumnSpec(context.Background(), db, tenantModelInstanceAPIKeyColumnSpec); err != nil {
		t.Fatalf("applyColumnSpec on the legacy narrow column: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
	if alterStatements != 1 {
		t.Fatalf("ALTER statements issued on the narrow column = %d, want exactly the one widening ALTER", alterStatements)
	}

	// Converged shape: the ALTER must not repeat on later startups.
	expectSchemaLookups(mock, "tenant_model_instance", "api_key", "varchar(4096)", "NO")
	if err := applyColumnSpec(context.Background(), db, tenantModelInstanceAPIKeyColumnSpec); err != nil {
		t.Fatalf("applyColumnSpec on the converged column: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the converged column must trigger no further DDL: %v", err)
	}
	if alterStatements != 1 {
		t.Fatalf("ALTER statements after the converged call = %d, want no repeated DDL", alterStatements)
	}
}

// TestTenantModelInstanceAPIKeySpecSkipsFreshInstalls pins the guards: on a
// database whose tenant_model_instance table does not exist yet (a fresh or
// pre-split install), the spec must leave the schema untouched — the table is
// created later with the varchar(4096) column already in place.
func TestTenantModelInstanceAPIKeySpecSkipsFreshInstalls(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	mock.ExpectQuery(`SELECT DATABASE\(\)`).
		WillReturnRows(sqlmock.NewRows([]string{"DATABASE()"}).AddRow("test"))
	mock.ExpectQuery(`SELECT SCHEMA_NAME from Information_schema.SCHEMATA`).
		WithArgs("test%", "test").
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}).AddRow("test"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM information_schema.tables`).
		WithArgs("test", "tenant_model_instance", "BASE TABLE").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(0))

	if err := applyColumnSpec(context.Background(), db, tenantModelInstanceAPIKeyColumnSpec); err != nil {
		t.Fatalf("applyColumnSpec on a missing table: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a missing table must trigger no catalog column reads or DDL: %v", err)
	}
}

// expectTableProbe mocks the catalog reads the mysql dialector issues for one
// Migrator().HasTable probe: the current-database resolution and the
// information_schema.tables lookup, reporting whether the table is present.
func expectTableProbe(mock sqlmock.Sqlmock, table string, present bool) {
	count := 0
	if present {
		count = 1
	}
	mock.ExpectQuery(`SELECT DATABASE\(\)`).
		WillReturnRows(sqlmock.NewRows([]string{"DATABASE()"}).AddRow("test"))
	mock.ExpectQuery(`SELECT SCHEMA_NAME from Information_schema.SCHEMATA`).
		WithArgs("test%", "test").
		WillReturnRows(sqlmock.NewRows([]string{"SCHEMA_NAME"}).AddRow("test"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM information_schema.tables`).
		WithArgs("test", table, "BASE TABLE").
		WillReturnRows(sqlmock.NewRows([]string{"count(*)"}).AddRow(count))
}

// expectLegacyIndexCatalogRead mocks the INFORMATION_SCHEMA.STATISTICS lookup
// dropIndexIfExists performs for the legacy composite unique index, and
// reports whether the catalog says the index is present.
func expectLegacyIndexCatalogRead(mock sqlmock.Sqlmock, present bool) {
	count := 0
	if present {
		count = 1
	}
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM INFORMATION_SCHEMA.STATISTICS`).
		WithArgs("tenant_model_instance", "idx_api_key_provider_id").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(count))
}

// TestLegacyModelInstanceAPIKeyIndexDropped pins that the legacy composite
// unique index idx_api_key_provider_id (provider_id, api_key) is removed: the
// releases between #14423 and #15460 declared it through the entity's
// uniqueIndex tags, and while it is in place the api_key widening makes MySQL
// rebuild a key whose utf8mb4 length exceeds the 3072-byte InnoDB limit — the
// ALTER fails and the upgrade cannot start (review of #22424).
func TestLegacyModelInstanceAPIKeyIndexDropped(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	expectTableProbe(mock, "tenant_model_instance", true)
	expectLegacyIndexCatalogRead(mock, true)
	mock.ExpectExec(`ALTER TABLE tenant_model_instance DROP INDEX idx_api_key_provider_id`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := dropLegacyTenantModelInstanceAPIKeyIndex(context.Background(), db); err != nil {
		t.Fatalf("dropLegacyTenantModelInstanceAPIKeyIndex with the legacy index present: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sqlmock expectations: %v", err)
	}
}

// TestLegacyModelInstanceAPIKeyIndexDropSkipsMissingIndexAndTable pins the
// guards: without the index (a database created outside the #14423–#15460
// window, or already migrated) only the catalog read runs and no DDL is
// issued, and without the table the step touches nothing at all.
func TestLegacyModelInstanceAPIKeyIndexDropSkipsMissingIndexAndTable(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	expectTableProbe(mock, "tenant_model_instance", true)
	expectLegacyIndexCatalogRead(mock, false)

	if err := dropLegacyTenantModelInstanceAPIKeyIndex(context.Background(), db); err != nil {
		t.Fatalf("dropLegacyTenantModelInstanceAPIKeyIndex with the index already gone: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a missing index must trigger no DDL: %v", err)
	}

	expectTableProbe(mock, "tenant_model_instance", false)

	if err := dropLegacyTenantModelInstanceAPIKeyIndex(context.Background(), db); err != nil {
		t.Fatalf("dropLegacyTenantModelInstanceAPIKeyIndex on a missing table: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("a missing table must trigger no catalog reads or DDL: %v", err)
	}
}

// TestTenantModelInstanceAPIKeyWideningFailureIsFatal pins that a failed
// widening ALTER stops the migration instead of being logged and ignored.
// The backfill that copies tenant_llm keys into tenant_model_instance.api_key
// runs after this step and reads the column through the bound the widening
// establishes, so a swallowed ALTER error leaves the narrow varchar(512)
// column in place: the backfill then either aborts the whole startup (MySQL
// strict mode, Error 1406) or stores a truncated credential (non-strict
// session). Failing at the widening is the only outcome that keeps the two in
// step, and the error must name the column it is about.
func TestTenantModelInstanceAPIKeyWideningFailureIsFatal(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	expectSchemaLookups(mock, "tenant_model_instance", "api_key", "varchar(512)", "NO")
	mock.ExpectExec(
		`ALTER TABLE tenant_model_instance MODIFY COLUMN api_key varchar\(4096\) NOT NULL`,
	).WillReturnError(errors.New("ERROR 1071 (42000): Specified key was too long; max key length is 3072 bytes"))

	err := applyColumnSpec(context.Background(), db, tenantModelInstanceAPIKeyColumnSpec)
	if err == nil {
		t.Fatal("applyColumnSpec must fail when the widening ALTER fails, not swallow the error")
	}
	if !strings.Contains(err.Error(), "tenant_model_instance") || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("the widening error must name the table and column, got %q", err.Error())
	}
}

// TestNonStrictColumnSpecsStillTolerateAFailedAlter pins that the strict
// behaviour is scoped to the specs that need it. The other specs share
// applyColumnSpec, and a failed ALTER there has never stopped the startup;
// widening api_key must not change that for the rest of the schema.
func TestNonStrictColumnSpecsStillTolerateAFailedAlter(t *testing.T) {
	db, mock := openMySQLSpecMock(t)

	nonStrict := columnSpec{
		table:      "knowledgebase",
		column:     "raptor_task_finish_at",
		columnType: "datetime",
		nullable:   true,
		definition: "datetime",
	}
	expectSchemaLookups(mock, "knowledgebase", "raptor_task_finish_at", "varchar(32)", "YES")
	mock.ExpectExec(
		`ALTER TABLE knowledgebase MODIFY COLUMN raptor_task_finish_at datetime`,
	).WillReturnError(errors.New("ERROR 1064 (42000): You have an error in your SQL syntax"))

	if err := applyColumnSpec(context.Background(), db, nonStrict); err != nil {
		t.Fatalf("a non-strict spec must keep tolerating a failed ALTER, got %v", err)
	}
}
