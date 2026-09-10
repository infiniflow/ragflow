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

package service

import (
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// TestReconcileMemoryTasksPublishesDueWakeups verifies the database is the
// source of recovery work and the broker message carries only durable identity.
func TestReconcileMemoryTasksPublishesDueWakeups(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	pinMemoryNow(t, now)
	db := testutil.SetupTestDB(t, &entity.MemoryTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	future := now.Add(time.Minute)
	for _, task := range []*entity.MemoryTask{
		{TaskID: "due", MemoryID: "memory-1", SourceID: 1, Input: entity.JSONMap{}, State: entity.MemoryTaskStatePending},
		{TaskID: "future", MemoryID: "memory-1", SourceID: 2, Input: entity.JSONMap{}, State: entity.MemoryTaskStatePending, NextRetryAt: &future},
		{TaskID: "completed", MemoryID: "memory-1", SourceID: 3, Input: entity.JSONMap{}, State: entity.MemoryTaskStateCompleted},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create memory task %s: %v", task.TaskID, err)
		}
	}

	publisher := &recordingTaskPublisher{}
	svc := NewMemoryMessageService(nil)
	svc.taskPublisher = publisher
	if err := svc.ReconcileMemoryTasks(t.Context(), 100); err != nil {
		t.Fatalf("ReconcileMemoryTasks: %v", err)
	}
	if publisher.subject != common.TaskSubject || len(publisher.messages) != 1 {
		t.Fatalf("published subject/messages = %q/%d", publisher.subject, len(publisher.messages))
	}
	message := publisher.messages[0]
	if message.TaskID != "due" || message.TaskType != common.TaskTypeMemory {
		t.Fatalf("published wake-up = %+v", message)
	}
}
