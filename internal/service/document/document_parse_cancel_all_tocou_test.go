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

package document

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// TestStopParseDocumentsAll_OutOfScopeStatusNotCancelled exercises the TOCTOU
// guard end-to-end. The caller names COMPLETED in runStatuses, so the list query
// selects the completed document, but RequestStopScoped must refuse to touch a
// terminal task even though it matched the list filter. Only the genuinely
// cancellable (SCHEDULED) task is stopped. No false success, no modification.
func TestStopParseDocumentsAll_OutOfScopeStatusNotCancelled(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.COMPLETED)
	insertTestDocWithFile(t, "doc-2", "file-2", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-2", "user-1", "doc-2", "kb-1", common.SCHEDULED)

	svc := testDocumentService(t)
	ctx := t.Context()
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", []string{common.SCHEDULED, common.RUNNING, common.COMPLETED})
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != 1 {
		t.Fatalf("expected success_count=1 (only SCHEDULED cancelled), got %d", sc)
	}
	task1, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil || task1 == nil {
		t.Fatalf("load doc-1 task: %v", err)
	}
	if task1.Status != common.COMPLETED {
		t.Fatalf("COMPLETED task was modified: status=%s", task1.Status)
	}
	task2, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-2")
	if err != nil || task2 == nil {
		t.Fatalf("load doc-2 task: %v", err)
	}
	if task2.Status != common.STOPPED {
		t.Fatalf("SCHEDULED task not stopped: status=%s", task2.Status)
	}
}

// TestStopParseDocumentsAll_StoppingCountedIdempotent covers a STOPPING task
// that the list query selects but which is already cancelling: it is outside the
// cancellable scope, so RequestStopScoped returns ErrTaskNotInScope, and the loop
// counts it as an idempotent success without touching its status.
func TestStopParseDocumentsAll_StoppingCountedIdempotent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.STOPPING)

	svc := testDocumentService(t)
	ctx := t.Context()
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", []string{common.STOPPING})
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != 1 {
		t.Fatalf("expected success_count=1 (STOPPING idempotent), got %d", sc)
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil || task == nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPING {
		t.Fatalf("STOPPING task should be untouched, got %s", task.Status)
	}
}
