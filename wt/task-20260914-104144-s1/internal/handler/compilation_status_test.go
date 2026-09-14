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

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	dataset "ragflow/internal/service/dataset"
)

// setupCompilationStatusHandlerDB migrates the minimal schema for the
// GET /datasets/:id/compilation/status handler and pushes it onto dao.DB.
func setupCompilationStatusHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{
		TranslateError: true,
	})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.Knowledgebase{},
		&entity.KnowledgeCompileDataset{},
	); err != nil {
		t.Fatalf("failed to migrate test schema: %v", err)
	}
	origDB := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = origDB })
	return db
}

func insertCompilationStatusHandlerKB(t *testing.T, kbID, ownerID string) {
	t.Helper()
	status := string(entity.StatusValid)
	kb := &entity.Knowledgebase{
		ID:         kbID,
		TenantID:   ownerID,
		Name:       "compile-status-handler-kb",
		EmbdID:     "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:  ownerID,
		Permission: string(entity.TenantPermissionMe),
		Status:     &status,
	}
	if err := dao.DB.Create(kb).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
}

func newCompilationStatusHandlerRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewDatasetsHandler(dataset.NewDatasetService(), nil)
	r := gin.New()
	r.GET("/api/v1/datasets/:dataset_id/compilation/status", func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user-1"})
		h.GetCompilationStatus(c)
	})
	return r
}

type compilationStatusResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

func getCompilationStatus(t *testing.T, r *gin.Engine, datasetID string) (int, compilationStatusResponse) {
	t.Helper()
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/datasets/"+datasetID+"/compilation/status", nil)
	r.ServeHTTP(resp, req)
	var body compilationStatusResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, resp.Body.String())
	}
	return resp.Code, body
}

// TestCompilationStatusHandler_NoRowIdle verifies a dataset with no scheduling
// row returns idle with zero counts.
func TestCompilationStatusHandler_NoRowIdle(t *testing.T) {
	db := setupCompilationStatusHandlerDB(t)
	insertCompilationStatusHandlerKB(t, "kb-status-idle", "user-1")
	_ = db

	status, body := getCompilationStatus(t, newCompilationStatusHandlerRouter(), "kb-status-idle")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if body.Code != int(common.CodeSuccess) {
		t.Fatalf("code=%d message=%q", body.Code, body.Message)
	}
	if body.Data["state"] != entity.DatasetStateIdle {
		t.Fatalf("state=%v want idle", body.Data["state"])
	}
	if n, _ := body.Data["inflight"].(float64); n != 0 {
		t.Fatalf("inflight=%v want 0", body.Data["inflight"])
	}
	if n, _ := body.Data["backlog"].(float64); n != 0 {
		t.Fatalf("backlog=%v want 0", body.Data["backlog"])
	}
}

// TestCompilationStatusHandler_FullOutput locks the JSON contract for a row
// with state, inflight/backlog counts, and error diagnostic.
func TestCompilationStatusHandler_FullOutput(t *testing.T) {
	db := setupCompilationStatusHandlerDB(t)
	insertCompilationStatusHandlerKB(t, "kb-status-full", "user-1")

	row := entity.KnowledgeCompileDataset{
		DatasetID:      "kb-status-full",
		TenantID:       "user-1",
		BacklogDocIDs:  `[{"doc_id":"d2","event_type":"completed","seq":2}]`,
		InflightDocIDs: `[{"doc_id":"d1","event_type":"completed","seq":1}]`,
		State:          entity.DatasetStatePending,
		ErrorMsg:       "merge failed: boom",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("insert scheduling row: %v", err)
	}

	status, body := getCompilationStatus(t, newCompilationStatusHandlerRouter(), "kb-status-full")
	if status != http.StatusOK {
		t.Fatalf("status=%d want 200", status)
	}
	if body.Code != int(common.CodeSuccess) {
		t.Fatalf("code=%d message=%q", body.Code, body.Message)
	}
	if body.Data["state"] != entity.DatasetStatePending {
		t.Fatalf("state=%v want pending", body.Data["state"])
	}
	if n, _ := body.Data["inflight"].(float64); n != 1 {
		t.Fatalf("inflight=%v want 1", body.Data["inflight"])
	}
	if n, _ := body.Data["backlog"].(float64); n != 1 {
		t.Fatalf("backlog=%v want 1", body.Data["backlog"])
	}
	if body.Data["error"] != "merge failed: boom" {
		t.Fatalf("error=%v want %q", body.Data["error"], "merge failed: boom")
	}
}

// TestCompilationStatusHandler_Unauthorized verifies a user who does not own the
// dataset is rejected with a data error (HTTP 200 + non-zero code, matching the
// handler's ErrorWithCode contract).
func TestCompilationStatusHandler_Unauthorized(t *testing.T) {
	db := setupCompilationStatusHandlerDB(t)
	// KB is owned by user-1; the router sets user to user-1, so this test must
	// exercise the case where the KB belongs to a different owner. We insert the
	// KB under a different owner tenant than the request user by re-pointing the
	// KB owner to "other-owner".
	insertCompilationStatusHandlerKB(t, "kb-status-forbidden", "other-owner")
	if err := db.Create(&entity.KnowledgeCompileDataset{
		DatasetID:      "kb-status-forbidden",
		TenantID:       "other-owner",
		BacklogDocIDs:  "[]",
		InflightDocIDs: "[]",
		State:          entity.DatasetStateRunning,
	}).Error; err != nil {
		t.Fatalf("insert scheduling row: %v", err)
	}

	_, body := getCompilationStatus(t, newCompilationStatusHandlerRouter(), "kb-status-forbidden")
	if body.Code != int(common.CodeDataError) {
		t.Fatalf("code=%d want %d", body.Code, common.CodeDataError)
	}
	if body.Message != "no authorization" {
		t.Fatalf("message=%q want %q", body.Message, "no authorization")
	}
}
