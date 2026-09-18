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
