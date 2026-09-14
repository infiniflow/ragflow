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

// TestValidateDocumentModifiable_RunStatuses pins the allowed/disallowed run
// statuses for editing a document via UpdateDatasetDocument. RUNNING ("1") and
// SCHEDULE ("5") must be rejected; UNSTART/CANCEL/DONE/FAIL may be edited.
// A nil Run (gorm default "0" UNSTART) is treated as editable.
func TestValidateDocumentModifiable_RunStatuses(t *testing.T) {
	cases := []struct {
		name     string
		run      *string
		wantCode common.ErrorCode
		wantErr  bool
	}{
		{"nil run defaults to unstart", nil, common.CodeSuccess, false},
		{"unstart editable", sptr(string(entity.TaskStatusUnstart)), common.CodeSuccess, false},
		{"cancel editable", sptr(string(entity.TaskStatusCancel)), common.CodeSuccess, false},
		{"done editable", sptr(string(entity.TaskStatusDone)), common.CodeSuccess, false},
		{"fail editable", sptr(string(entity.TaskStatusFail)), common.CodeSuccess, false},
		{"running rejected", sptr(string(entity.TaskStatusRunning)), common.CodeDataError, true},
		{"schedule rejected", sptr(string(entity.TaskStatusSchedule)), common.CodeDataError, true},
	}

	svc := &DocumentService{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := &entity.Document{Run: tc.run}
			code, err := svc.validateDocumentModifiable(doc)
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

// updateDatasetDocumentRejected asserts that editing a document in the given
// run status with the supplied request is rejected by the run-state guard.
func updateDatasetDocumentRejected(t *testing.T, run string, req *UpdateDatasetDocumentRequest, present map[string]bool) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDocWithRun(t, "doc-1", "kb-1", run, 0, 0)

	svc := testDocumentService(t)
	ctx := t.Context()
	_, code, err := svc.UpdateDatasetDocument(ctx, "tenant-1", "kb-1", "doc-1", req, present)
	if err == nil {
		t.Fatalf("expected run-state rejection, got nil error (code=%v)", code)
	}
	if code != common.CodeDataError {
		t.Fatalf("code = %v, want %v", code, common.CodeDataError)
	}
	if !strings.Contains(err.Error(), "cannot be modified") {
		t.Fatalf("err = %q missing sentinel phrase", err.Error())
	}
}

// updateDatasetDocumentAllowed asserts that editing a document in an editable
// run status with the supplied request is NOT blocked by the run-state guard
// (it may still fail later validation, but not with the run-state error).
func updateDatasetDocumentAllowed(t *testing.T, run string, req *UpdateDatasetDocumentRequest, present map[string]bool) {
	t.Helper()
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDocWithRun(t, "doc-1", "kb-1", run, 0, 0)

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
	updateDatasetDocumentRejected(t, string(entity.TaskStatusRunning),
		&UpdateDatasetDocumentRequest{ParserConfig: map[string]any{"chunk_token_num": float64(128)}},
		map[string]bool{"parser_config": true})
}

func TestUpdateDatasetDocumentRejectsRunningChunkMethod(t *testing.T) {
	cm := "naive"
	updateDatasetDocumentRejected(t, string(entity.TaskStatusRunning),
		&UpdateDatasetDocumentRequest{ChunkMethod: &cm},
		map[string]bool{"chunk_method": true})
}

func TestUpdateDatasetDocumentRejectsRunningRename(t *testing.T) {
	name := "renamed.txt"
	updateDatasetDocumentRejected(t, string(entity.TaskStatusRunning),
		&UpdateDatasetDocumentRequest{Name: &name},
		map[string]bool{"name": true})
}

func TestUpdateDatasetDocumentRejectsRunningEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentRejected(t, string(entity.TaskStatusRunning),
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentRejectsScheduledRename(t *testing.T) {
	name := "renamed.txt"
	updateDatasetDocumentRejected(t, string(entity.TaskStatusSchedule),
		&UpdateDatasetDocumentRequest{Name: &name},
		map[string]bool{"name": true})
}

func TestUpdateDatasetDocumentAllowsDoneEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, string(entity.TaskStatusDone),
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentAllowsCancelEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, string(entity.TaskStatusCancel),
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentAllowsFailEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, string(entity.TaskStatusFail),
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

func TestUpdateDatasetDocumentAllowsUnstartEnabled(t *testing.T) {
	enabled := 0
	updateDatasetDocumentAllowed(t, string(entity.TaskStatusUnstart),
		&UpdateDatasetDocumentRequest{Enabled: &enabled},
		map[string]bool{"enabled": true})
}

// TestUpdateDatasetDocumentRunningEmptyPresentNotRejected locks the
// len(present) > 0 gate: an empty PATCH (no fields) on a RUNNING doc must not
// be rejected by the run-state guard (it is a no-op, not an edit).
func TestUpdateDatasetDocumentRunningEmptyPresentNotRejected(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDocWithRun(t, "doc-1", "kb-1", string(entity.TaskStatusRunning), 0, 0)

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
	updateDatasetDocumentRejected(t, string(entity.TaskStatusRunning),
		&UpdateDatasetDocumentRequest{MetaFields: map[string]any{"author": "x"}},
		map[string]bool{"meta_fields": true})
}

// TestUpdateDatasetDocumentAllowsDoneRename proves an editable status is not
// over-restricted: renaming a DONE document succeeds (no run-state error).
func TestUpdateDatasetDocumentAllowsDoneRename(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertNamedTestDoc(t, "doc-1", "kb-1", "orig.txt", 0, 0)

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
