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

// insertTestDocWithFile links a document to a file so it appears in the
// document list query used by StopParseDocumentsAll (which joins file2document+file).
func insertTestDocWithFile(t *testing.T, docID, fileID, kbID string) {
	t.Helper()
	insertTestDoc(t, docID, kbID, 10, 5)
	insertTestFile(t, fileID, "", fileID+".txt", nil)
	insertTestFile2Document(t, "f2d-"+docID, fileID, docID)
}

func stopAllSuccessCount(t *testing.T, result map[string]interface{}) int {
	t.Helper()
	sc, ok := result["success_count"].(int)
	if !ok {
		t.Fatalf("success_count not found or wrong type: %v", result)
	}
	return sc
}

func TestStopParseDocumentsAll_DefaultSelectsActiveOnly(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)
	insertTestDocWithFile(t, "doc-2", "file-2", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-2", "user-1", "doc-2", "kb-1", common.SCHEDULED)
	insertTestDocWithFile(t, "doc-3", "file-3", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-3", "user-1", "doc-3", "kb-1", common.RUNNING)
	insertTestDocWithFile(t, "doc-4", "file-4", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-4", "user-1", "doc-4", "kb-1", common.COMPLETED)
	insertTestDocWithFile(t, "doc-5", "file-5", "kb-1") // no task → UNSTART

	svc := testDocumentService(t)
	ctx := t.Context()
	// runStatuses nil → default {SCHEDULED, RUNNING}
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", nil)
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}

	if sc := stopAllSuccessCount(t, result); sc != 2 {
		t.Fatalf("expected success_count=2 (SCHEDULED+RUNNING), got %d", sc)
	}

	// doc-2 (SCHEDULED) and doc-3 (RUNNING) are cancelled; others untouched.
	// RUNNING transitions to STOPPING (transitional), SCHEDULED to STOPPED.
	for docID, want := range map[string]string{
		"doc-1": common.CREATED,   // not in default set
		"doc-2": common.STOPPED,   // SCHEDULED -> STOPPED
		"doc-3": common.STOPPING,  // RUNNING -> STOPPING (transitional)
		"doc-4": common.COMPLETED, // not in default set
	} {
		task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, docID)
		if err != nil {
			t.Fatalf("load task for %s: %v", docID, err)
		}
		if task == nil {
			t.Fatalf("no task for %s", docID)
		}
		if task.Status != want {
			t.Fatalf("doc %s task status = %s, want %s", docID, task.Status, want)
		}
	}
}

func TestStopParseDocumentsAll_ExplicitIncludesUnstartNoop(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)
	insertTestDocWithFile(t, "doc-2", "file-2", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-2", "user-1", "doc-2", "kb-1", common.SCHEDULED)
	insertTestDocWithFile(t, "doc-3", "file-3", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-3", "user-1", "doc-3", "kb-1", common.RUNNING)
	insertTestDocWithFile(t, "doc-4", "file-4", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-4", "user-1", "doc-4", "kb-1", common.COMPLETED)
	insertTestDocWithFile(t, "doc-5", "file-5", "kb-1") // no task → UNSTART

	svc := testDocumentService(t)
	ctx := t.Context()
	runStatuses := []string{common.CREATED, common.SCHEDULED, common.RUNNING, common.UNSTART}
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", runStatuses)
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}

	// doc-1/2/3 cancelled; doc-5 has no ingestion task, so it is reported as a
	// failure rather than a no-op success.
	if sc := stopAllSuccessCount(t, result); sc != 3 {
		t.Fatalf("expected success_count=3 (active cancelled), got %d", sc)
	}
	errs, ok := result["errors"].([]string)
	if !ok || len(errs) != 1 {
		t.Fatalf("expected exactly 1 error for doc-5 (no task), got %v", result["errors"])
	}
	// COMPLETED doc-4 must remain untouched.
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-4")
	if err != nil || task == nil {
		t.Fatalf("load doc-4 task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("doc-4 status = %s, want COMPLETED (must not be touched)", task.Status)
	}
	// doc-5 has no task; ensure still no task (no-op, nothing created).
	none, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-5")
	if err != nil {
		t.Fatalf("load doc-5 task: %v", err)
	}
	if none != nil {
		t.Fatalf("doc-5 should have no task, got %+v", none)
	}
}

func TestStopParseDocumentsAll_StoppedIdempotent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.STOPPED)

	svc := testDocumentService(t)
	ctx := t.Context()
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", []string{common.STOPPED})
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != 1 {
		t.Fatalf("expected success_count=1 (idempotent stopped), got %d", sc)
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("expected STOPPED unchanged, got %s", task.Status)
	}
}

func TestStopParseDocumentsAll_EmptyMatch(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0) // no docs

	svc := testDocumentService(t)
	ctx := t.Context()
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", nil)
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != 0 {
		t.Fatalf("expected success_count=0 for empty match, got %d", sc)
	}
	if _, hasErrs := result["errors"]; hasErrs {
		t.Fatalf("expected no errors for empty match, got %v", result["errors"])
	}
}

func TestStopParseDocumentsAll_TerminalNotSelected(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 10, 5)
	insertTestDocWithFile(t, "doc-1", "file-1", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.COMPLETED)
	insertTestDocWithFile(t, "doc-2", "file-2", "kb-1")
	insertTestIngestionTaskWithStatus(t, "task-2", "user-1", "doc-2", "kb-1", common.FAILED)

	svc := testDocumentService(t)
	ctx := t.Context()
	// Default set is {SCHEDULED, RUNNING}; terminal docs must be excluded.
	result, err := svc.StopParseDocumentsAll(ctx, "kb-1", nil)
	if err != nil {
		t.Fatalf("StopParseDocumentsAll failed: %v", err)
	}
	if sc := stopAllSuccessCount(t, result); sc != 0 {
		t.Fatalf("expected success_count=0 (terminal excluded), got %d", sc)
	}
	for _, docID := range []string{"doc-1", "doc-2"} {
		task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, docID)
		if err != nil || task == nil {
			t.Fatalf("load %s task: %v", docID, err)
		}
		if task.Status == common.STOPPED {
			t.Fatalf("terminal doc %s must not be stopped, got %s", docID, task.Status)
		}
	}
}
