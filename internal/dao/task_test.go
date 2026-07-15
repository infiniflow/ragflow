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
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupTaskTestDB initializes an in-memory SQLite database for Task DAO tests.
func setupTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)

	// Migrate task table (Task depends on Document for the doc_id FK,
	// but SQLite doesn't enforce FKs by default)
	if err := db.AutoMigrate(&entity.Task{}); err != nil {
		t.Fatalf("failed to migrate Task: %v", err)
	}
	if err := db.AutoMigrate(&entity.IngestionTask{}); err != nil {
		t.Fatalf("failed to migrate IngestionTask: %v", err)
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

func TestDeleteIngestionTasksByDocIDs_Success(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewTaskDAO()

	// Insert ingestion tasks for two different documents
	task1 := &entity.IngestionTask{ID: "itask-1", DocumentID: "doc-1", UserID: "user-1", DatasetID: "ds-1", Status: "pending"}
	task2 := &entity.IngestionTask{ID: "itask-2", DocumentID: "doc-2", UserID: "user-1", DatasetID: "ds-1", Status: "pending"}
	for _, tk := range []*entity.IngestionTask{task1, task2} {
		if err := db.Create(tk).Error; err != nil {
			t.Fatalf("failed to create ingestion task: %v", err)
		}
	}

	// Delete tasks for doc-1
	rowsAffected, err := dao.DeleteIngestionTasksByDocIDs([]string{"doc-1"})
	if err != nil {
		t.Fatalf("DeleteIngestionTasksByDocIDs failed: %v", err)
	}
	if rowsAffected != 1 {
		t.Fatalf("expected 1 row affected, got %d", rowsAffected)
	}

	// Verify doc-1 tasks are gone, doc-2 remains
	var remaining []*entity.IngestionTask
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("failed to find remaining tasks: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected 1 task remaining, got %d", len(remaining))
	}
	if remaining[0].ID != "itask-2" {
		t.Fatalf("expected itask-2 to remain, got %s", remaining[0].ID)
	}
}

func TestDeleteIngestionTasksByDocIDs_EmptyIDs(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewTaskDAO()

	rowsAffected, err := dao.DeleteIngestionTasksByDocIDs([]string{})
	if err != nil {
		t.Fatalf("DeleteIngestionTasksByDocIDs failed: %v", err)
	}
	if rowsAffected != 0 {
		t.Fatalf("expected 0 rows affected, got %d", rowsAffected)
	}
}

func TestDeleteIngestionTasksByDocIDs_Nonexistent(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewTaskDAO()

	// Insert one task to make sure table isn't empty
	task := &entity.IngestionTask{ID: "itask-1", DocumentID: "doc-1", UserID: "user-1", DatasetID: "ds-1", Status: "pending"}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("failed to create ingestion task: %v", err)
	}

	rowsAffected, err := dao.DeleteIngestionTasksByDocIDs([]string{"nonexistent"})
	if err != nil {
		t.Fatalf("DeleteIngestionTasksByDocIDs failed: %v", err)
	}
	if rowsAffected != 0 {
		t.Fatalf("expected 0 rows affected, got %d", rowsAffected)
	}
}

func TestDeleteIngestionTasksByDocIDs_MultipleIDs(t *testing.T) {
	db := setupTaskTestDB(t)
	orig := DB
	DB = db
	t.Cleanup(func() { DB = orig })

	dao := NewTaskDAO()

	// Insert tasks for multiple documents
	tasks := []*entity.IngestionTask{
		{ID: "itask-1", DocumentID: "doc-1", UserID: "user-1", DatasetID: "ds-1", Status: "pending"},
		{ID: "itask-2", DocumentID: "doc-2", UserID: "user-1", DatasetID: "ds-1", Status: "pending"},
		{ID: "itask-3", DocumentID: "doc-3", UserID: "user-1", DatasetID: "ds-1", Status: "pending"},
		{ID: "itask-4", DocumentID: "keep", UserID: "user-1", DatasetID: "ds-1", Status: "pending"},
	}
	for _, tk := range tasks {
		if err := db.Create(tk).Error; err != nil {
			t.Fatalf("failed to create ingestion task: %v", err)
		}
	}

	// Delete multiple document IDs
	rowsAffected, err := dao.DeleteIngestionTasksByDocIDs([]string{"doc-1", "doc-2", "doc-3"})
	if err != nil {
		t.Fatalf("DeleteIngestionTasksByDocIDs failed: %v", err)
	}
	if rowsAffected != 3 {
		t.Fatalf("expected 3 rows affected, got %d", rowsAffected)
	}

	// Verify only "keep" remains
	var remaining []*entity.IngestionTask
	if err := db.Find(&remaining).Error; err != nil {
		t.Fatalf("failed to find remaining tasks: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != "itask-4" {
		t.Fatalf("expected only itask-4 to remain, got %d tasks", len(remaining))
	}
}
