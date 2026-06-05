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
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupTaskTestDB initializes an in-memory SQLite database for Task DAO tests.
func setupTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}

	// Migrate task table (Task depends on Document for the doc_id FK,
	// but SQLite doesn't enforce FKs by default)
	if err := db.AutoMigrate(&entity.Task{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return db
}

func TestGetByDocID_FindsTasks(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewTaskDAO()

	// Insert tasks for two different documents
	task1 := &entity.Task{ID: "task-1", DocID: "doc-1"}
	task2 := &entity.Task{ID: "task-2", DocID: "doc-1"}
	task3 := &entity.Task{ID: "task-3", DocID: "doc-2"}
	for _, tk := range []*entity.Task{task1, task2, task3} {
		if err := dao.Create(tk); err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	tasks, err := dao.GetByDocID("doc-1")
	if err != nil {
		t.Fatalf("GetByDocID failed: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks for doc-1, got %d", len(tasks))
	}

	// Verify task IDs match
	ids := make(map[string]bool)
	for _, tk := range tasks {
		ids[tk.ID] = true
	}
	if !ids["task-1"] || !ids["task-2"] {
		t.Fatalf("expected task-1 and task-2, got %v", ids)
	}
}

func TestGetByDocID_NoTasks(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewTaskDAO()

	tasks, err := dao.GetByDocID("nonexistent")
	if err != nil {
		t.Fatalf("GetByDocID failed: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestGetByDocID_EmptyDocID(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewTaskDAO()

	// Insert a task with empty doc_id to verify edge case
	task := &entity.Task{ID: "task-1", DocID: ""}
	if err := dao.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	tasks, err := dao.GetByDocID("")
	if err != nil {
		t.Fatalf("GetByDocID failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task for empty doc_id, got %d", len(tasks))
	}
	if tasks[0].ID != "task-1" {
		t.Fatalf("expected task-1, got %s", tasks[0].ID)
	}
}
