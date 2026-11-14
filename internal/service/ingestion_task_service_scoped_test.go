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
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// TestIngestionTaskServiceRequestStopScopedStopsScheduled covers the in-scope
// path: a SCHEDULED task with SCHEDULED in the allowed set transitions to STOPPED.
func TestIngestionTaskServiceRequestStopScopedStopsScheduled(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	got, err := svc.RequestStopScoped(ctx, "task-1", []string{common.SCHEDULED, common.RUNNING})
	if err != nil {
		t.Fatalf("RequestStopScoped failed: %v", err)
	}
	if got.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", got.Status, common.STOPPED)
	}
}

// TestIngestionTaskServiceRequestStopScopedStopsRunning covers the in-scope
// RUNNING path: it transitions to STOPPING (the worker picks the cancel flag up).
func TestIngestionTaskServiceRequestStopScopedStopsRunning(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.RUNNING)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	got, err := svc.RequestStopScoped(ctx, "task-1", []string{common.SCHEDULED, common.RUNNING})
	if err != nil {
		t.Fatalf("RequestStopScoped failed: %v", err)
	}
	if got.Status != common.STOPPING {
		t.Fatalf("status = %q, want %q", got.Status, common.STOPPING)
	}
}

// TestIngestionTaskServiceRequestStopScopedStopsCreated covers a CREATED task
// that is explicitly within the allowed set.
func TestIngestionTaskServiceRequestStopScopedStopsCreated(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	if _, err := svc.RequestStopScoped(ctx, "task-1", []string{common.CREATED}); err != nil {
		t.Fatalf("RequestStopScoped failed: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", task.Status, common.STOPPED)
	}
}

// TestIngestionTaskServiceRequestStopScopedOutOfScopeUnchanged is the heart of
// the TOCTOU fix: a task whose current status is not in the allowed set must not
// be transitioned. A CREATED task is refused when only SCHEDULED/RUNNING are
// permitted, and the stored status is left untouched.
func TestIngestionTaskServiceRequestStopScopedOutOfScopeUnchanged(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	got, err := svc.RequestStopScoped(ctx, "task-1", []string{common.SCHEDULED, common.RUNNING})
	if !errors.Is(err, ErrTaskNotInScope) {
		t.Fatalf("err = %v, want ErrTaskNotInScope", err)
	}
	if got == nil || got.Status != common.CREATED {
		t.Fatalf("expected loaded task with CREATED status, got %+v", got)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.CREATED {
		t.Fatalf("out-of-scope task was modified: status=%s", task.Status)
	}
}

// TestIngestionTaskServiceRequestStopScopedTerminalUnchanged covers a terminal
// task (COMPLETED) that the caller's list query may have selected: it is outside
// the cancel scope and must be left as-is rather than reported cancelled.
func TestIngestionTaskServiceRequestStopScopedTerminalUnchanged(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.COMPLETED)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	got, err := svc.RequestStopScoped(ctx, "task-1", []string{common.SCHEDULED, common.RUNNING})
	if !errors.Is(err, ErrTaskNotInScope) {
		t.Fatalf("err = %v, want ErrTaskNotInScope", err)
	}
	if got == nil || got.Status != common.COMPLETED {
		t.Fatalf("expected loaded task with COMPLETED status, got %+v", got)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil || task == nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("terminal task was modified: status=%s", task.Status)
	}
}

// TestIngestionTaskServiceRequestStopScopedStoppedReturnsTask verifies the
// idempotent-already-stopped case: a STOPPED task is out of scope but the call
// returns the loaded task (so callers can treat it as an idempotent success).
func TestIngestionTaskServiceRequestStopScopedStoppedReturnsTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.STOPPED)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	got, err := svc.RequestStopScoped(ctx, "task-1", []string{common.SCHEDULED, common.RUNNING})
	if !errors.Is(err, ErrTaskNotInScope) {
		t.Fatalf("err = %v, want ErrTaskNotInScope", err)
	}
	if got == nil || got.Status != common.STOPPED {
		t.Fatalf("expected loaded STOPPED task, got %+v", got)
	}
}

// TestIngestionTaskServiceRequestStopScopedEmptyAllowed permits nothing: a
// non-cancellable requested scope (e.g. only COMPLETED) yields an empty allowed
// set, so every task is out of scope and untouched. This guards the convention
// that an empty allowed set means "cancel nothing", which StopParseDocumentsAll
// relies on when the requested statuses contain no stoppable value.
func TestIngestionTaskServiceRequestStopScopedEmptyAllowed(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	got, err := svc.RequestStopScoped(ctx, "task-1", []string{common.COMPLETED})
	if !errors.Is(err, ErrTaskNotInScope) {
		t.Fatalf("err = %v, want ErrTaskNotInScope", err)
	}
	if got.Status != common.SCHEDULED {
		t.Fatalf("status = %q, want SCHEDULED (untouched)", got.Status)
	}
}

// TestIngestionTaskServiceRequestStopScopedDriftRefusesStop is the end-to-end
// TOCTOU guard: the caller selected the task as SCHEDULED, but it drifted to
// COMPLETED before the stop re-read it. RequestStopScoped re-reads the live
// status and refuses, so no false stop happens.
func TestIngestionTaskServiceRequestStopScopedDriftRefusesStop(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)

	// Simulate the status drifting out of scope after the list/lookup read the
	// caller performed but before RequestStopScoped's own GetTask.
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.COMPLETED).Error; err != nil {
		t.Fatalf("drift task to COMPLETED: %v", err)
	}

	svc := NewIngestionTaskService()
	got, err := svc.RequestStopScoped(t.Context(), "task-1", []string{common.SCHEDULED, common.RUNNING})
	if !errors.Is(err, ErrTaskNotInScope) {
		t.Fatalf("err = %v, want ErrTaskNotInScope (drift must be caught)", err)
	}
	if got == nil || got.Status != common.COMPLETED {
		t.Fatalf("expected loaded COMPLETED task, got %+v", got)
	}
}
