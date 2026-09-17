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
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// TestValidateDocumentModifiable_TaskStatuses pins the allowed/disallowed task
// statuses for editing a document via UpdateDatasetDocument.
// CREATED, SCHEDULED, RUNNING, and STOPPING must be rejected;
// no task, COMPLETED, STOPPED, and FAILED may be edited.
func TestValidateDocumentModifiable_TaskStatuses(t *testing.T) {
	cases := []struct {
		name       string
		taskStatus *string
		wantCode   common.ErrorCode
		wantErr    bool
	}{
		{"no task defaults to unstart", nil, common.CodeSuccess, false},
		{"created rejected", sptr(common.CREATED), common.CodeDataError, true},
		{"scheduled rejected", sptr(common.SCHEDULED), common.CodeDataError, true},
		{"running rejected", sptr(common.RUNNING), common.CodeDataError, true},
		{"stopping rejected", sptr(common.STOPPING), common.CodeDataError, true},
		{"completed editable", sptr(common.COMPLETED), common.CodeSuccess, false},
		{"stopped editable", sptr(common.STOPPED), common.CodeSuccess, false},
		{"failed editable", sptr(common.FAILED), common.CodeSuccess, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertTestDoc(t, "doc-1", "kb-1", 0, 0)
			if tc.taskStatus != nil {
				insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", *tc.taskStatus)
			}
			svc := testDocumentService(t)
			ctx := t.Context()
			doc := &entity.Document{ID: "doc-1"}
			code, err := svc.validateDocumentModifiable(ctx, doc)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if code != tc.wantCode {
					t.Fatalf("code = %v, want %v", code, tc.wantCode)
				}
				if !strings.Contains(err.Error(), "cannot be modified") {
					t.Fatalf("err = %q missing sentinel phrase", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if code != tc.wantCode {
				t.Fatalf("code = %v, want %v", code, tc.wantCode)
			}
		})
	}
}

// updateDatasetDocumentRejected asserts that editing a document with the given
// task status is rejected by the modifiable guard.
func updateDatasetDocumentRejected(t *testing.T, status string, req *UpdateDatasetDocumentRequest, present map[string]bool) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", status)

	svc := testDocumentService(t)
	ctx := t.Context()
	_, code, err := svc.UpdateDatasetDocument(ctx, "tenant-1", "kb-1", "doc-1", req, present)
	if err == nil {
		t.Fatalf("expected task-state rejection, got nil error (code=%v)", code)
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %v, want %v", code, common.CodeDataError)
	}
	if !strings.Contains(err.Error(), "cannot be modified") {
		t.Fatalf("err = %q missing sentinel phrase", err.Error())
	}
}

// updateDatasetDocumentAllowed asserts that editing a document with an editable
// task status is NOT blocked by the modifiable guard.
func updateDatasetDocumentAllowed(t *testing.T, status string, req *UpdateDatasetDocumentRequest, present map[string]bool) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	if status != "" {
		insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", status)
	}

	svc := testDocumentService(t)
	ctx := t.Context()
	_, code, err := svc.UpdateDatasetDocument(ctx, "tenant-1", "kb-1", "doc-1", req, present)
	if err != nil && strings.Contains(err.Error(), "cannot be modified") {
		t.Fatalf("editable doc was wrongly rejected: %v", err)
	}
	if err != nil {
		t.Logf("non-run-state error (acceptable): code=%v err=%v", code, err)
	}
}

func TestUpdateDatasetDocumentRejectsRunningParserConfig(t *testing.T) {
	updateDatasetDocumentRejected(t, common.RUNNING,
		&UpdateDatasetDocumentRequest{ParserConfig: map[string]any{"chunk_token_num": float64(128)}},
		map[string]bool{"parser_config": true})
}

func TestUpdateDatasetDocumentRejectsRunningChunkMethod(t *testing.T) {
	cm := "naive"
	updateDatasetDocumentRejected(t, common.RUNNING,
		&UpdateDatasetDocumentRequest{ChunkMethod: &cm},
		map[string]bool{"chunk_method": true})
}

func TestUpdateDatasetDocumentRejectsRunningRename(t *testing.T) {
	name := "renamed.txt"
	updateDatasetDocumentRejected(t, common.RUNNING,
		&UpdateDatasetDocumentRequest{Name: &name},
		map[string]bool{"name": true})
}

func TestUpdateDatasetDocumentRejectsRunningEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentRejected(t, common.RUNNING,
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentRejectsScheduledRename(t *testing.T) {
	name := "renamed.txt"
	updateDatasetDocumentRejected(t, common.SCHEDULED,
		&UpdateDatasetDocumentRequest{Name: &name},
		map[string]bool{"name": true})
}

func TestUpdateDatasetDocumentAllowsDoneEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, common.COMPLETED,
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentAllowsCancelEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, common.STOPPED,
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentAllowsFailEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, common.FAILED,
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentAllowsUnstartEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, "",
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

// TestUpdateDatasetDocumentRunningEmptyPresentNotRejected locks the
// len(present) > 0 gate: an empty PATCH (no fields) on a RUNNING doc must not
// be rejected by the modifiable guard (it is a no-op, not an edit).
func TestUpdateDatasetDocumentRunningEmptyPresentNotRejected(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.RUNNING)

	svc := testDocumentService(t)
	ctx := t.Context()
	_, code, err := svc.UpdateDatasetDocument(ctx, "tenant-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{}, map[string]bool{})
	if err != nil {
		t.Fatalf("empty PATCH on RUNNING doc should not be rejected: code=%v err=%v", code, err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want %v", code, common.CodeSuccess)
	}
}

// TestUpdateDatasetDocumentRejectsRunningMetaFields pins that every editable
// field is blocked while RUNNING, including meta_fields.
func TestUpdateDatasetDocumentRejectsRunningMetaFields(t *testing.T) {
	updateDatasetDocumentRejected(t, common.RUNNING,
		&UpdateDatasetDocumentRequest{MetaFields: map[string]any{"author": "x"}},
		map[string]bool{"meta_fields": true})
}

// TestUpdateDatasetDocumentAllowsDoneRename proves an editable status is not
// over-restricted: renaming a DONE document succeeds.
func TestUpdateDatasetDocumentAllowsDoneRename(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "orig.txt", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.COMPLETED)

	name := "renamed.txt"
	svc := testDocumentService(t)
	ctx := t.Context()
	_, code, err := svc.UpdateDatasetDocument(ctx, "tenant-1", "kb-1", "doc-1",
		&UpdateDatasetDocumentRequest{Name: &name},
		map[string]bool{"name": true})
	if err != nil && strings.Contains(err.Error(), "cannot be modified") {
		t.Fatalf("editable DONE doc was wrongly rejected: %v", err)
	}
	if err != nil {
		t.Fatalf("unexpected non-run-state error: code=%v err=%v", code, err)
	}
}
