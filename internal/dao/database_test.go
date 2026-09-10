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
