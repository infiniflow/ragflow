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

package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	dataset "ragflow/internal/service/dataset"
)

var errFakeCancelAll = errors.New("fake cancel all error")

func newCancelAllHandler(fake *fakeDocumentService) *DocumentHandler {
	return &DocumentHandler{
		documentService: fake,
		datasetService:  dataset.NewDatasetService(),
	}
}

// cancelAllHandlerWithAccess wires the handler with the permission DB so that
// CheckAccess (OperationRun) passes, mirroring TestStopParseDocumentsHandler_Success.
func cancelAllHandlerWithAccess(t *testing.T, fake *fakeDocumentService) (*DocumentHandler, func()) {
	t.Helper()
	db := setupHandlerAccessDB(t)
	orig := dao.DB
	dao.DB = db
	cleanup := func() { dao.DB = orig }
	return newCancelAllHandler(fake), cleanup
}

func TestStopParseDocumentsHandler_CancelAll_CallsServiceWithDefaultStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeDocumentService{}
	h, cleanup := cancelAllHandlerWithAccess(t, fake)
	defer cleanup()

	c, w := setupHandlerAccessContext("POST", "/api/v1/datasets/ds-1/documents/stop", `{"cancel_all":true}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !fake.cancelAllCalled {
		t.Fatal("expected StopParseDocumentsAll to be called")
	}
	if fake.cancelAllDataset != "ds-1" {
		t.Fatalf("cancelAllDataset = %q, want ds-1", fake.cancelAllDataset)
	}
	if len(fake.cancelAllRunStatuses) != 0 {
		t.Fatalf("expected empty run_statuses (default applied server-side), got %v", fake.cancelAllRunStatuses)
	}
}

func TestStopParseDocumentsHandler_CancelAll_ForwardsRunStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeDocumentService{}
	h, cleanup := cancelAllHandlerWithAccess(t, fake)
	defer cleanup()

	body := `{"cancel_all":true,"run_statuses":["CREATED","RUNNING"]}`
	c, w := setupHandlerAccessContext("POST", "/api/v1/datasets/ds-1/documents/stop", body)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !fake.cancelAllCalled {
		t.Fatal("expected StopParseDocumentsAll to be called")
	}
	if !equalStringSlice(fake.cancelAllRunStatuses, []string{"CREATED", "RUNNING"}) {
		t.Fatalf("cancelAllRunStatuses = %v, want [CREATED RUNNING]", fake.cancelAllRunStatuses)
	}
}

func TestStopParseDocumentsHandler_CancelAll_InvalidRunStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeDocumentService{}
	h, cleanup := cancelAllHandlerWithAccess(t, fake)
	defer cleanup()

	body := `{"cancel_all":true,"run_statuses":["FOO"]}`
	c, w := setupHandlerAccessContext("POST", "/api/v1/datasets/ds-1/documents/stop", body)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 envelope", w.Code)
	}
	var resp struct {
		Code    common.ErrorCode `json:"code"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != common.CodeArgumentError {
		t.Fatalf("code = %v, want %v: %s", resp.Code, common.CodeArgumentError, w.Body.String())
	}
	if fake.cancelAllCalled {
		t.Fatal("StopParseDocumentsAll must not be called on invalid run_status")
	}
}

func TestStopParseDocumentsHandler_CancelAll_MutuallyExclusive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeDocumentService{}
	h, cleanup := cancelAllHandlerWithAccess(t, fake)
	defer cleanup()

	body := `{"cancel_all":true,"document_ids":["doc-1"]}`
	c, w := setupHandlerAccessContext("POST", "/api/v1/datasets/ds-1/documents/stop", body)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 envelope", w.Code)
	}
	var resp struct {
		Code common.ErrorCode `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != common.CodeArgumentError {
		t.Fatalf("code = %v, want %v: %s", resp.Code, common.CodeArgumentError, w.Body.String())
	}
	if fake.cancelAllCalled {
		t.Fatal("StopParseDocumentsAll must not be called when both cancel_all and document_ids are set")
	}
}

// TestStopParseDocumentsHandler_CancelAll_EmptyDocumentIDsArrayRejected is the
// adversarial case for the cancel_all / document_ids mutual exclusion. An explicit
// empty JSON array ("document_ids": []) unmarshals to a non-nil empty slice, which
// len() == 0 does NOT detect. The old guard `len(req.DocumentIDs) > 0` therefore
// let `{"cancel_all": true, "document_ids": []}` slip past the 400 and trigger a
// full-dataset cancel. The guard must reject any explicit document_ids (nil or []),
// so this request must return 400 and never call StopParseDocumentsAll.
func TestStopParseDocumentsHandler_CancelAll_EmptyDocumentIDsArrayRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeDocumentService{}
	h, cleanup := cancelAllHandlerWithAccess(t, fake)
	defer cleanup()

	body := `{"cancel_all":true,"document_ids":[]}`
	c, w := setupHandlerAccessContext("POST", "/api/v1/datasets/ds-1/documents/stop", body)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 envelope", w.Code)
	}
	var resp struct {
		Code common.ErrorCode `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != common.CodeArgumentError {
		t.Fatalf("code = %v, want %v (explicit empty document_ids must be rejected): %s", resp.Code, common.CodeArgumentError, w.Body.String())
	}
	if fake.cancelAllCalled {
		t.Fatal("StopParseDocumentsAll must not be called when cancel_all is set with an explicit (even empty) document_ids array")
	}
}

// TestStopParseDocumentsHandler_CancelAll_OmittedDocumentIDsOk guards against an
// over-strict fix. Omitting document_ids entirely (nil slice) is the normal
// cancel_all usage and must still proceed to StopParseDocumentsAll.
func TestStopParseDocumentsHandler_CancelAll_OmittedDocumentIDsOk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeDocumentService{}
	h, cleanup := cancelAllHandlerWithAccess(t, fake)
	defer cleanup()

	body := `{"cancel_all":true}`
	c, w := setupHandlerAccessContext("POST", "/api/v1/datasets/ds-1/documents/stop", body)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if !fake.cancelAllCalled {
		t.Fatal("expected StopParseDocumentsAll to be called when document_ids is omitted")
	}
}

func TestStopParseDocumentsHandler_CancelAll_ServiceError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fake := &fakeDocumentService{stopAllErr: errFakeCancelAll}
	h, cleanup := cancelAllHandlerWithAccess(t, fake)
	defer cleanup()

	c, w := setupHandlerAccessContext("POST", "/api/v1/datasets/ds-1/documents/stop", `{"cancel_all":true}`)
	c.Params = gin.Params{{Key: "dataset_id", Value: "ds-1"}}
	h.StopParseDocuments(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 envelope", w.Code)
	}
	var resp struct {
		Code common.ErrorCode `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code == common.CodeSuccess {
		t.Fatal("expected error code when service fails")
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
