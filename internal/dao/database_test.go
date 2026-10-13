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
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestAutoMigrateRuntimeModelsCreatesGoRuntimeTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	// Verify tables do not exist initially
	if db.Migrator().HasTable(&entity.IngestionTask{}) {
		t.Fatal("expected ingestion_task to not exist initially")
	}
	if db.Migrator().HasTable(&entity.IngestionTaskLog{}) {
		t.Fatal("expected ingestion_task_log to not exist initially")
	}
	if db.Migrator().HasTable(&entity.MemoryTask{}) {
		t.Fatal("expected memory_task to not exist initially")
	}

	ctx := context.Background()
	if err = autoMigrateRuntimeModels(ctx, db); err != nil {
		t.Fatalf("autoMigrateRuntimeModels failed: %v", err)
	}

	// Verify tables exist after auto migration
	if !db.Migrator().HasTable(&entity.IngestionTask{}) {
		t.Fatal("expected ingestion_task to exist after autoMigrateRuntimeModels")
	}
	if !db.Migrator().HasTable(&entity.IngestionTaskLog{}) {
		t.Fatal("expected ingestion_task_log to exist after autoMigrateRuntimeModels")
	}
	if !db.Migrator().HasTable(&entity.MemoryTask{}) {
		t.Fatal("expected memory_task to exist after autoMigrateRuntimeModels")
	}
	if !db.Migrator().HasIndex(&entity.MemoryTask{}, "idx_memory_task_due") {
		t.Fatal("expected memory_task due index to exist after autoMigrateRuntimeModels")
	}

	memoryTask := &entity.MemoryTask{
		TaskID:   "task-1",
		MemoryID: "memory-1",
		SourceID: 42,
		Input: entity.JSONMap{
			"user_id":    "user-1",
			"user_input": "remember this",
		},
		State: entity.MemoryTaskStatePending,
		Extraction: entity.JSONSlice{
			map[string]interface{}{"message_id": float64(7), "content": "remembered"},
		},
		LastError: "",
	}
	if err = db.Create(memoryTask).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	var stored entity.MemoryTask
	if err = db.First(&stored, "task_id = ?", memoryTask.TaskID).Error; err != nil {
		t.Fatalf("load memory task: %v", err)
	}
	if stored.State != entity.MemoryTaskStatePending {
		t.Fatalf("memory task state = %q, want %q", stored.State, entity.MemoryTaskStatePending)
	}
	if got := stored.Input["user_input"]; got != "remember this" {
		t.Fatalf("memory task input user_input = %#v, want %q", got, "remember this")
	}
	if len(stored.Extraction) != 1 {
		t.Fatalf("memory task extraction length = %d, want 1", len(stored.Extraction))
	}

	// Verify idempotency
	if err = autoMigrateRuntimeModels(ctx, db); err != nil {
		t.Fatalf("second autoMigrateRuntimeModels failed: %v", err)
	}
}

// TestMigrateIngestionTaskPipelineLogID covers the upgrade path AutoMigrate
// cannot handle on MySQL: an existing ingestion_task created before
// pipeline_log_id existed must gain the column, and running the migration again
// must be a no-op.
func TestMigrateIngestionTaskPipelineLogID(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	ctx := context.Background()

	// An ingestion_task table without the column, as created by an older build.
	if err := db.Exec(`CREATE TABLE ingestion_task (
		id varchar(32) PRIMARY KEY,
		user_id varchar(32) NOT NULL,
		document_id varchar(32) NOT NULL,
		dataset_id varchar(32) NOT NULL,
		status varchar(32) NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	migrator := db.Migrator()
	if migrator.HasColumn("ingestion_task", "pipeline_log_id") {
		t.Fatal("precondition failed: column already present")
	}

	if err := migrateIngestionTaskPipelineLogID(ctx, db); err != nil {
		t.Fatalf("migrateIngestionTaskPipelineLogID: %v", err)
	}
	if !migrator.HasColumn("ingestion_task", "pipeline_log_id") {
		t.Fatal("expected pipeline_log_id to be added")
	}

	// Idempotent: a second run must not fail on the existing column.
	if err := migrateIngestionTaskPipelineLogID(ctx, db); err != nil {
		t.Fatalf("second migrateIngestionTaskPipelineLogID: %v", err)
	}

	// Missing table is a no-op, not an error.
	fresh, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open second sqlite: %v", err)
	}
	if err := migrateIngestionTaskPipelineLogID(ctx, fresh); err != nil {
		t.Fatalf("migration on a table-less database should be a no-op: %v", err)
	}
}

// TestMigrateIngestionLogRunIdentity verifies that legacy pipeline and event
// tables gain the run identity columns without assigning a fake run number to
// pre-existing rows. A NULL legacy run_count lets the unique document/run
// index protect only newly allocated runs.
func TestMigrateIngestionLogRunIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	ctx := context.Background()
	if err := db.Exec(`CREATE TABLE pipeline_operation_log (
		id varchar(32) PRIMARY KEY,
		document_id varchar(32) NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create legacy pipeline log table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE ingestion_task_log (
		id integer PRIMARY KEY,
		task_id varchar(32) NOT NULL,
		checkpoint text NOT NULL,
		phase integer,
		component varchar(64),
		message text
	)`).Error; err != nil {
		t.Fatalf("create legacy ingestion event table: %v", err)
	}
	if err := db.Exec(`INSERT INTO pipeline_operation_log (id, document_id) VALUES ('legacy-log', 'doc-1')`).Error; err != nil {
		t.Fatalf("seed legacy pipeline log: %v", err)
	}

	if err := migrateIngestionLogRunIdentity(ctx, db); err != nil {
		t.Fatalf("migrateIngestionLogRunIdentity: %v", err)
	}
	migrator := db.Migrator()
	for _, column := range []string{"run_count"} {
		if !migrator.HasColumn(&entity.PipelineOperationLog{}, column) {
			t.Fatalf("pipeline_operation_log missing %s", column)
		}
	}
	for _, column := range []string{"pipeline_log_id", "event_type"} {
		if !migrator.HasColumn(&entity.IngestionTaskLog{}, column) {
			t.Fatalf("ingestion_task_log missing %s", column)
		}
	}
	if !migrator.HasIndex(&entity.IngestionTaskLog{}, "idx_ingestion_task_log_pipeline_id") {
		t.Fatal("ingestion_task_log missing pipeline event index")
	}
	if !migrator.HasIndex(&entity.PipelineOperationLog{}, "idx_pipeline_operation_log_document_run") {
		t.Fatal("pipeline_operation_log missing document run unique index")
	}

	var legacyRunCount *int
	if err := db.Raw(`SELECT run_count FROM pipeline_operation_log WHERE id = 'legacy-log'`).Scan(&legacyRunCount).Error; err != nil {
		t.Fatalf("read legacy run count: %v", err)
	}
	if legacyRunCount != nil {
		t.Fatalf("legacy run_count = %d, want NULL", *legacyRunCount)
	}
	if err := db.Exec(`INSERT INTO pipeline_operation_log (id, document_id, run_count) VALUES ('run-1', 'doc-1', 1)`).Error; err != nil {
		t.Fatalf("insert first numbered run: %v", err)
	}
	if err := db.Exec(`INSERT INTO pipeline_operation_log (id, document_id, run_count) VALUES ('run-2', 'doc-1', 1)`).Error; err == nil {
		t.Fatal("duplicate numbered run was accepted")
	}

	if err := migrateIngestionLogRunIdentity(ctx, db); err != nil {
		t.Fatalf("second migrateIngestionLogRunIdentity: %v", err)
	}
}

type ingestionLogSchemaMigratorStub struct {
	indexExistsAfterCreate bool
}

func (s *ingestionLogSchemaMigratorStub) HasIndex(any, string) bool {
	return s.indexExistsAfterCreate
}

func (s *ingestionLogSchemaMigratorStub) CreateIndex(any, string) error {
	s.indexExistsAfterCreate = true
	return errors.New("connection closed after index creation")
}

func TestCreateIngestionLogSchemaObjectsRechecksAfterCreateError(t *testing.T) {
	migrator := &ingestionLogSchemaMigratorStub{}
	if err := createIngestionLogIndexIfMissing(migrator, &entity.IngestionTaskLog{}, "idx_ingestion_task_log_pipeline_id", "add ingestion_task_log pipeline index"); err != nil {
		t.Fatalf("createIngestionLogIndexIfMissing: %v", err)
	}
}

// uniqueColumnType is a gorm.ColumnType that answers Unique() and nothing
// else, which is all MigrateColumnUnique branches on.
type uniqueColumnType struct {
	unique bool
}

func (c uniqueColumnType) Name() string                      { return "document_id" }
func (c uniqueColumnType) DatabaseTypeName() string          { return "VARCHAR" }
func (c uniqueColumnType) ColumnType() (string, bool)        { return "varchar(32)", true }
func (c uniqueColumnType) PrimaryKey() (bool, bool)          { return false, true }
func (c uniqueColumnType) AutoIncrement() (bool, bool)       { return false, true }
func (c uniqueColumnType) Length() (int64, bool)             { return 32, true }
func (c uniqueColumnType) DecimalSize() (int64, int64, bool) { return 0, 0, false }
func (c uniqueColumnType) Nullable() (bool, bool)            { return false, true }
func (c uniqueColumnType) Unique() (bool, bool)              { return c.unique, true }
func (c uniqueColumnType) ScanType() reflect.Type            { return reflect.TypeOf("") }
func (c uniqueColumnType) Comment() (string, bool)           { return "", false }
func (c uniqueColumnType) DefaultValue() (string, bool)      { return "", false }

// TestNamedIndexMigratorSkipsPhantomUniqueDrop pins the behaviour that kept
// failing every startup on MySQL: a unique index created as an index is not a
// column-level unique constraint, so nothing exists to drop. The other three
// combinations must fall through to the stock migrator, which decides whether
// the constraint it names has to be created.
func TestNamedIndexMigratorSkipsPhantomUniqueDrop(t *testing.T) {
	for _, tc := range []struct {
		name     string
		unique   bool
		tag      bool
		wantSkip bool
	}{
		{"named index with no unique tag", true, false, true},
		{"column is not unique", false, false, false},
		{"unique tag already set", false, true, false},
		{"index and tag agree", true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := phantomUniqueDrop(&schema.Field{Unique: tc.tag}, uniqueColumnType{unique: tc.unique})
			if got != tc.wantSkip {
				t.Fatalf("phantomUniqueDrop(unique=%v, tag=%v) = %v, want %v", tc.unique, tc.tag, got, tc.wantSkip)
			}
		})
	}
}

// TestWrappersExposeEverythingWrappedTypeDoes is the guard that made the
// concrete embedding necessary: embedding gorm's interfaces instead would drop
// the twenty migrator methods and the three dialector methods that gorm reaches
// by type assertion, and losing one surfaces as a panic part-way through
// AutoMigrate. Embedding the concrete types promotes all of them by
// construction; this catches a future refactor that reverts to the interfaces.
func TestWrappersExposeEverythingWrappedTypeDoes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		wrapped  reflect.Type
		wrapper  reflect.Type
		override string
	}{
		{"dialector", reflect.TypeOf(&mysql.Dialector{}), reflect.TypeOf(migrationAwareDialector{}), "Migrator"},
		{"migrator", reflect.TypeOf(mysql.Migrator{}), reflect.TypeOf(namedIndexMigrator{}), "MigrateColumnUnique"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < tc.wrapped.NumMethod(); i++ {
				name := tc.wrapped.Method(i).Name
				if name == tc.override {
					continue
				}
				if _, ok := tc.wrapper.MethodByName(name); !ok {
					t.Errorf("wrapper does not expose %s; gorm reaches some capabilities by type assertion on the wrapped type rather than through its interfaces", name)
				}
			}
		})
	}
}

func TestAutoMigrateRuntimeModelsNamesFailingTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	if err = sqlDB.Close(); err != nil {
		t.Fatalf("close sql database: %v", err)
	}

	err = autoMigrateRuntimeModels(t.Context(), db)
	if err == nil || !strings.Contains(err.Error(), "runtime table ingestion_task") {
		t.Fatalf("autoMigrateRuntimeModels error = %v, want failing table name", err)
	}
}
