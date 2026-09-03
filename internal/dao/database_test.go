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
