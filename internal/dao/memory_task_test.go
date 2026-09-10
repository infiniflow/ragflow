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
	"time"

	"ragflow/internal/entity"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupMemoryTaskTestDB creates an isolated SQLite store for DAO tests.
func setupMemoryTaskTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.Task{}, &entity.MemoryTask{}); err != nil {
		t.Fatalf("auto-migrate memory tasks: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	return db
}

// newMemoryTaskPair returns matching UI and execution records.
func newMemoryTaskPair(taskID string) (*entity.Task, *entity.MemoryTask) {
	progressMsg := ""
	return &entity.Task{
			ID:          taskID,
			DocID:       "memory-1",
			TaskType:    "memory",
			ProgressMsg: &progressMsg,
		}, &entity.MemoryTask{
			TaskID:   taskID,
			MemoryID: "memory-1",
			SourceID: 42,
			Input: entity.JSONMap{
				"user_id":    "user-1",
				"user_input": "remember this",
			},
			State:     entity.MemoryTaskStatePending,
			LastError: "",
		}
}

// TestMemoryTaskDAOCreateWithTaskIsAtomic verifies a failed execution-record
// insert rolls back the generic task insert.
func TestMemoryTaskDAOCreateWithTaskIsAtomic(t *testing.T) {
	db := setupMemoryTaskTestDB(t)
	dao := NewMemoryTaskDAO()

	_, existing := newMemoryTaskPair("task-1")
	if err := db.Create(existing).Error; err != nil {
		t.Fatalf("seed memory task: %v", err)
	}
	task, memoryTask := newMemoryTaskPair("task-1")
	if err := dao.CreateWithTask(t.Context(), db, task, memoryTask); err == nil {
		t.Fatal("CreateWithTask error = nil, want duplicate-key error")
	}

	var taskCount int64
	if err := db.Model(&entity.Task{}).Where("id = ?", task.ID).Count(&taskCount).Error; err != nil {
		t.Fatalf("count generic tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("generic task count = %d, want transaction rollback", taskCount)
	}
}

// TestMemoryTaskDAOClaimRenewAndRetry verifies lease exclusion, renewal, and
// retry-time admission.
func TestMemoryTaskDAOClaimRenewAndRetry(t *testing.T) {
	db := setupMemoryTaskTestDB(t)
	dao := NewMemoryTaskDAO()
	task, memoryTask := newMemoryTaskPair("task-1")
	if err := dao.CreateWithTask(t.Context(), db, task, memoryTask); err != nil {
		t.Fatalf("CreateWithTask: %v", err)
	}

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	claimed, acquired, err := dao.Claim(t.Context(), db, task.ID, "worker-1", now, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("Claim acquired=%v err=%v, want acquired", acquired, err)
	}
	if claimed.AttemptCount != 1 || claimed.LeaseOwner != "worker-1" {
		t.Fatalf("claimed task = %+v, want attempt 1 owned by worker-1", claimed)
	}

	if _, acquired, err = dao.Claim(t.Context(), db, task.ID, "worker-2", now.Add(30*time.Second), time.Minute); err != nil || acquired {
		t.Fatalf("duplicate Claim acquired=%v err=%v, want live lease rejection", acquired, err)
	}
	if renewed, renewErr := dao.RenewLease(t.Context(), db, task.ID, "worker-1", now.Add(30*time.Second), time.Minute); renewErr != nil || !renewed {
		t.Fatalf("RenewLease renewed=%v err=%v, want renewed", renewed, renewErr)
	}

	retryAt := now.Add(2 * time.Minute)
	if scheduled, scheduleErr := dao.ScheduleRetry(t.Context(), db, task.ID, "worker-1", retryAt, "temporary failure"); scheduleErr != nil || !scheduled {
		t.Fatalf("ScheduleRetry scheduled=%v err=%v, want scheduled", scheduled, scheduleErr)
	}
	if _, acquired, err = dao.Claim(t.Context(), db, task.ID, "worker-2", retryAt.Add(-time.Second), time.Minute); err != nil || acquired {
		t.Fatalf("early retry Claim acquired=%v err=%v, want not due", acquired, err)
	}
	claimed, acquired, err = dao.Claim(t.Context(), db, task.ID, "worker-2", retryAt, time.Minute)
	if err != nil || !acquired {
		t.Fatalf("due retry Claim acquired=%v err=%v, want acquired", acquired, err)
	}
	if claimed.AttemptCount != 2 || claimed.LeaseOwner != "worker-2" {
		t.Fatalf("retried task = %+v, want attempt 2 owned by worker-2", claimed)
	}
}

// TestMemoryTaskDAOCheckpointAndComplete verifies the complete durable state
// sequence and final UI progress projection.
func TestMemoryTaskDAOCheckpointAndComplete(t *testing.T) {
	db := setupMemoryTaskTestDB(t)
	dao := NewMemoryTaskDAO()
	task, memoryTask := newMemoryTaskPair("task-1")
	if err := dao.CreateWithTask(t.Context(), db, task, memoryTask); err != nil {
		t.Fatalf("CreateWithTask: %v", err)
	}

	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if _, acquired, err := dao.Claim(t.Context(), db, task.ID, "worker-1", now, time.Minute); err != nil || !acquired {
		t.Fatalf("Claim acquired=%v err=%v, want acquired", acquired, err)
	}
	extraction := entity.JSONSlice{
		map[string]interface{}{"message_id": float64(7), "content": "remembered"},
	}
	if updated, err := dao.PersistExtraction(t.Context(), db, task.ID, "worker-1", now, extraction); err != nil || !updated {
		t.Fatalf("PersistExtraction updated=%v err=%v, want updated", updated, err)
	}
	if updated, err := dao.MarkStored(t.Context(), db, task.ID, "worker-1", now); err != nil || !updated {
		t.Fatalf("MarkStored updated=%v err=%v, want updated", updated, err)
	}
	if completed, err := dao.Complete(t.Context(), db, task.ID, "worker-1", "complete", now); err != nil || !completed {
		t.Fatalf("Complete completed=%v err=%v, want completed", completed, err)
	}

	stored, err := dao.GetByID(t.Context(), db, task.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if stored.State != entity.MemoryTaskStateCompleted || stored.LeaseOwner != "" || stored.LeaseExpiresAt != nil {
		t.Fatalf("completed memory task = %+v", stored)
	}
	var genericTask entity.Task
	if err = db.First(&genericTask, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("load generic task: %v", err)
	}
	if genericTask.Progress != 1 || genericTask.ProgressMsg == nil || *genericTask.ProgressMsg != "complete" {
		t.Fatalf("generic task progress = %v message=%v, want 1/complete", genericTask.Progress, genericTask.ProgressMsg)
	}
}

// TestMemoryTaskDAOCompleteRollsBackWithoutGenericTask verifies completion
// cannot commit only one side of the final transaction.
func TestMemoryTaskDAOCompleteRollsBackWithoutGenericTask(t *testing.T) {
	db := setupMemoryTaskTestDB(t)
	dao := NewMemoryTaskDAO()
	_, memoryTask := newMemoryTaskPair("task-1")
	memoryTask.State = entity.MemoryTaskStateStored
	memoryTask.LeaseOwner = "worker-1"
	expiresAt := time.Date(2026, 9, 10, 12, 1, 0, 0, time.UTC)
	memoryTask.LeaseExpiresAt = &expiresAt
	if err := db.Create(memoryTask).Error; err != nil {
		t.Fatalf("create memory task: %v", err)
	}

	completed, err := dao.Complete(t.Context(), db, memoryTask.TaskID, "worker-1", "complete", expiresAt.Add(-time.Second))
	if err == nil || completed {
		t.Fatalf("Complete completed=%v err=%v, want missing generic task error", completed, err)
	}
	stored, getErr := dao.GetByID(t.Context(), db, memoryTask.TaskID)
	if getErr != nil {
		t.Fatalf("GetByID: %v", getErr)
	}
	if stored.State != entity.MemoryTaskStateStored || stored.LeaseOwner != "worker-1" {
		t.Fatalf("memory task after rollback = %+v, want stored with original lease", stored)
	}
}

// TestMemoryTaskDAOListDueExcludesFutureLeasedAndTerminalTasks verifies the
// reconciler query returns only executable rows.
func TestMemoryTaskDAOListDueExcludesFutureLeasedAndTerminalTasks(t *testing.T) {
	db := setupMemoryTaskTestDB(t)
	dao := NewMemoryTaskDAO()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)

	for _, task := range []*entity.MemoryTask{
		{TaskID: "due", MemoryID: "memory-1", SourceID: 1, Input: entity.JSONMap{}, State: entity.MemoryTaskStatePending, LastError: ""},
		{TaskID: "future", MemoryID: "memory-1", SourceID: 2, Input: entity.JSONMap{}, State: entity.MemoryTaskStatePending, NextRetryAt: &future, LastError: ""},
		{TaskID: "leased", MemoryID: "memory-1", SourceID: 3, Input: entity.JSONMap{}, State: entity.MemoryTaskStateExtracted, LeaseOwner: "worker-1", LeaseExpiresAt: &future, LastError: ""},
		{TaskID: "completed", MemoryID: "memory-1", SourceID: 4, Input: entity.JSONMap{}, State: entity.MemoryTaskStateCompleted, LastError: ""},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.TaskID, err)
		}
	}

	tasks, err := dao.ListDue(t.Context(), db, now, 10)
	if err != nil {
		t.Fatalf("ListDue: %v", err)
	}
	if len(tasks) != 1 || tasks[0].TaskID != "due" {
		t.Fatalf("ListDue = %+v, want only due task", tasks)
	}
}
