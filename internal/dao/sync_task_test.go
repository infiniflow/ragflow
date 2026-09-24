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
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// setupSyncTaskTestDB initializes an in-memory SQLite database for SyncTask DAO tests.
func setupSyncTaskTestDB(t *testing.T) *gorm.DB {
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

	if err := db.AutoMigrate(&entity.SyncLogs{}); err != nil {
		t.Fatalf("failed to migrate SyncLogs: %v", err)
	}
	return db
}

func TestListDatasetSyncTasksKeepsRunningAndLatestPerConnector(t *testing.T) {
	db := setupSyncTaskTestDB(t)
	if err := db.AutoMigrate(&entity.Connector2Kb{}); err != nil {
		t.Fatalf("migrate connector links: %v", err)
	}
	links := []entity.Connector2Kb{
		{ID: "link-a", ConnectorID: "connector-a", KbID: "dataset-1"},
		{ID: "link-a-duplicate", ConnectorID: "connector-a", KbID: "dataset-1"},
		{ID: "link-b", ConnectorID: "connector-b", KbID: "dataset-1"},
		{ID: "link-c", ConnectorID: "connector-c", KbID: "dataset-1"},
		{ID: "link-other", ConnectorID: "connector-unlinked", KbID: "dataset-2"},
	}
	if err := db.Create(&links).Error; err != nil {
		t.Fatalf("create connector links: %v", err)
	}
	task := func(id, connectorID, kbID, taskType, status string, updateTime int64) entity.SyncLogs {
		return entity.SyncLogs{
			ID: id, ConnectorID: connectorID, KbID: kbID, TaskType: taskType, Status: status,
			BaseModel: entity.BaseModel{UpdateTime: &updateTime},
		}
	}
	tasks := []entity.SyncLogs{
		task("a-done-old", "connector-a", "dataset-1", TaskTypeSync, SyncStatusDone, 100),
		task("a-running", "connector-a", "dataset-1", TaskTypeSync, SyncStatusRunning, 200),
		task("a-done-new", "connector-a", "dataset-1", TaskTypeSync, SyncStatusDone, 300),
		task("a-scheduled", "connector-a", "dataset-1", TaskTypeSync, SyncStatusSchedule, 900),
		task("a-prune", "connector-a", "dataset-1", TaskTypePrune, SyncStatusRunning, 800),
		task("b-running-old", "connector-b", "dataset-1", TaskTypeSync, SyncStatusRunning, 150),
		task("b-running", "connector-b", "dataset-1", TaskTypeSync, SyncStatusRunning, 250),
		task("c-tie-a", "connector-c", "dataset-1", TaskTypeSync, SyncStatusFail, 400),
		task("c-tie-z", "connector-c", "dataset-1", TaskTypeSync, SyncStatusFail, 400),
		task("unlinked", "connector-unlinked", "dataset-1", TaskTypeSync, SyncStatusRunning, 700),
		task("other-dataset", "connector-a", "dataset-2", TaskTypeSync, SyncStatusRunning, 1000),
	}
	tasks[1].NewDocsIndexed = 5
	tasks[1].ErrorCount = 2
	tasks[1].ErrorClass = "transient"
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("create sync tasks: %v", err)
	}

	got, err := NewSyncTaskDAO(db).ListDatasetSyncTasks(context.Background(), "dataset-1")
	if err != nil {
		t.Fatalf("list dataset sync tasks: %v", err)
	}
	wantIDs := []string{"c-tie-z", "a-done-new", "b-running", "a-running", "b-running-old"}
	if len(got) != len(wantIDs) {
		t.Fatalf("task count = %d, want %d: %+v", len(got), len(wantIDs), got)
	}
	for i, wantID := range wantIDs {
		if got[i].ID != wantID {
			t.Fatalf("task %d = %q, want %q", i, got[i].ID, wantID)
		}
	}
	if got[3].NewDocsIndexed != 5 || got[3].ErrorCount != 2 || got[3].ErrorClass != "transient" {
		t.Fatalf("running task counters = %+v, want indexed=5 errors=2 class=transient", got[3])
	}
}

func insertRunningSyncTask(t *testing.T, db *gorm.DB, taskID string, errorCount, retryCount int64, errorClass string) {
	t.Helper()
	if err := db.Create(&entity.SyncLogs{
		ID:          taskID,
		ConnectorID: "conn-1",
		KbID:        "kb-1",
		TaskType:    TaskTypeSync,
		Status:      SyncStatusRunning,
		ErrorCount:  errorCount,
		RetryCount:  retryCount,
		ErrorClass:  errorClass,
	}).Error; err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

// TestHandleTransientFailureUsesClassScopedRetryBudget verifies the retry
// comparison uses a counter scoped to the current error class: failures under
// one class must not consume another class's budget, and ErrorCount keeps the
// total failure count for diagnostics.
func TestHandleTransientFailureUsesClassScopedRetryBudget(t *testing.T) {
	db := setupSyncTaskTestDB(t)
	dao := NewSyncTaskDAO(db)
	ctx := context.Background()

	// The task already exhausted the non-transient budget (3 failures), then
	// hits a transient error whose budget (4) must start fresh.
	insertRunningSyncTask(t, db, "task-1", 3, 3, "non_transient")
	attempts, failed, err := dao.HandleTransientFailure(ctx, "task-1", "", "boom", "transient", 4)
	if err != nil {
		t.Fatalf("HandleTransientFailure: %v", err)
	}
	if failed {
		t.Fatalf("failed = true, want false (transient budget must reset)")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != SyncStatusSchedule {
		t.Fatalf("status = %s, want schedule", task.Status)
	}
	if task.ErrorCount != 4 {
		t.Fatalf("error_count = %d, want 4 (diagnostics total)", task.ErrorCount)
	}
	if task.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", task.RetryCount)
	}
	if task.ErrorClass != "transient" {
		t.Fatalf("error_class = %q, want transient", task.ErrorClass)
	}

	// Same class keeps accumulating and fails once the class budget is spent.
	for i := 2; i <= 4; i++ {
		// The worker re-claims the scheduled task before the next attempt.
		if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", SyncStatusRunning).Error; err != nil {
			t.Fatalf("mark running: %v", err)
		}
		attempts, failed, err = dao.HandleTransientFailure(ctx, "task-1", "", "timeout", "transient", 4)
		if err != nil {
			t.Fatalf("HandleTransientFailure: %v", err)
		}
		if attempts != int64(i) {
			t.Fatalf("attempts = %d, want %d", attempts, i)
		}
		if i < 4 && failed {
			t.Fatalf("failed = true on attempt %d, want false", i)
		}
	}
	if !failed {
		t.Fatalf("failed = false, want true after 4 transient attempts")
	}
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != SyncStatusFail {
		t.Fatalf("status = %s, want fail", task.Status)
	}
	if task.ErrorCount != 7 {
		t.Fatalf("error_count = %d, want 7 (total across classes)", task.ErrorCount)
	}
	if task.RetryCount != 4 {
		t.Fatalf("retry_count = %d, want 4", task.RetryCount)
	}
}
