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
	"testing"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAutoMigrateRuntimeModelsCreatesIngestionTaskTables(t *testing.T) {
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
