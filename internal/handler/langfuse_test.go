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
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// ── fake service ─────────────────────────────────────────────────────────────

type fakeLangfuseService struct {
	setData    map[string]interface{}
	setErr     error
	getData    map[string]interface{}
	getErr     error
	deleted    bool
	deleteErr  error
}

func (f *fakeLangfuseService) SetAPIKey(_ string, _ *service.SetAPIKeyRequest) (map[string]interface{}, error) {
	return f.setData, f.setErr
}

func (f *fakeLangfuseService) GetAPIKey(_ string) (map[string]interface{}, error) {
	return f.getData, f.getErr
}

func (f *fakeLangfuseService) DeleteAPIKey(_ string) (bool, error) {
	return f.deleted, f.deleteErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newLangfuseCtx(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	c.Request = req
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	return c, w
}

func newLangfuseHandler(fake langfuseServiceIface) *LangfuseHandler {
	return &LangfuseHandler{langfuseService: fake}
}

func decodeLangfuseResp(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode response: %v\nbody: %s", err, w.Body.String())
	}
	return out
}

// ── SetAPIKey tests ───────────────────────────────────────────────────────────

func TestLangfuseSetAPIKey_BadJSON(t *testing.T) {
	h := newLangfuseHandler(&fakeLangfuseService{})
	c, w := newLangfuseCtx(http.MethodPost, "/api/v1/langfuse/api-key", "{bad json")

	h.SetAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeDataError {
		t.Errorf("expected CodeDataError (%d), got %d", common.CodeDataError, code)
	}
}

func TestLangfuseSetAPIKey_MissingRequiredFields(t *testing.T) {
	h := newLangfuseHandler(&fakeLangfuseService{})
	// Omit all required fields — ShouldBindJSON will fail.
	c, w := newLangfuseCtx(http.MethodPost, "/api/v1/langfuse/api-key", "{}")

	h.SetAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeDataError {
		t.Errorf("expected CodeDataError (%d), got %d", common.CodeDataError, code)
	}
}

func TestLangfuseSetAPIKey_ServiceError(t *testing.T) {
	fake := &fakeLangfuseService{setErr: errors.New("invalid keys")}
	h := newLangfuseHandler(fake)
	body := `{"secret_key":"sk","public_key":"pk","host":"https://h.example.com"}`
	c, w := newLangfuseCtx(http.MethodPost, "/api/v1/langfuse/api-key", body)

	h.SetAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeDataError {
		t.Errorf("expected CodeDataError (%d), got %d", common.CodeDataError, code)
	}
}

func TestLangfuseSetAPIKey_Success(t *testing.T) {
	returnData := map[string]interface{}{"tenant_id": "user-1", "public_key": "pk", "host": "https://h.example.com"}
	fake := &fakeLangfuseService{setData: returnData}
	h := newLangfuseHandler(fake)
	body := `{"secret_key":"sk","public_key":"pk","host":"https://h.example.com"}`
	c, w := newLangfuseCtx(http.MethodPost, "/api/v1/langfuse/api-key", body)

	h.SetAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeSuccess {
		t.Errorf("expected CodeSuccess (%d), got %d", common.CodeSuccess, code)
	}
	if resp["data"] == nil {
		t.Error("expected non-nil data in success response")
	}
}

// ── GetAPIKey tests ───────────────────────────────────────────────────────────

func TestLangfuseGetAPIKey_NotFound(t *testing.T) {
	// Service returns (nil, nil) → "Have not record" message.
	fake := &fakeLangfuseService{getData: nil, getErr: nil}
	h := newLangfuseHandler(fake)
	c, w := newLangfuseCtx(http.MethodGet, "/api/v1/langfuse/api-key", "")

	h.GetAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeSuccess {
		t.Errorf("expected CodeSuccess (%d), got %d", common.CodeSuccess, code)
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "Have not record") {
		t.Errorf("expected 'Have not record' in message, got %q", msg)
	}
}

func TestLangfuseGetAPIKey_ServiceError(t *testing.T) {
	fake := &fakeLangfuseService{getErr: errors.New("db error")}
	h := newLangfuseHandler(fake)
	c, w := newLangfuseCtx(http.MethodGet, "/api/v1/langfuse/api-key", "")

	h.GetAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeDataError {
		t.Errorf("expected CodeDataError (%d), got %d", common.CodeDataError, code)
	}
}

func TestLangfuseGetAPIKey_Success(t *testing.T) {
	returnData := map[string]interface{}{
		"tenant_id":    "user-1",
		"public_key":   "pk",
		"host":         "https://h.example.com",
		"project_name": "my-project",
	}
	fake := &fakeLangfuseService{getData: returnData}
	h := newLangfuseHandler(fake)
	c, w := newLangfuseCtx(http.MethodGet, "/api/v1/langfuse/api-key", "")

	h.GetAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeSuccess {
		t.Errorf("expected CodeSuccess (%d), got %d", common.CodeSuccess, code)
	}
	if resp["data"] == nil {
		t.Error("expected non-nil data in success response")
	}
}

// ── DeleteAPIKey tests ────────────────────────────────────────────────────────

func TestLangfuseDeleteAPIKey_NotFound(t *testing.T) {
	// Service returns (false, nil) → "Have not record" message.
	fake := &fakeLangfuseService{deleted: false, deleteErr: nil}
	h := newLangfuseHandler(fake)
	c, w := newLangfuseCtx(http.MethodDelete, "/api/v1/langfuse/api-key", "")

	h.DeleteAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeSuccess {
		t.Errorf("expected CodeSuccess (%d), got %d", common.CodeSuccess, code)
	}
	msg, _ := resp["message"].(string)
	if !strings.Contains(msg, "Have not record") {
		t.Errorf("expected 'Have not record' in message, got %q", msg)
	}
}

func TestLangfuseDeleteAPIKey_ServiceError(t *testing.T) {
	fake := &fakeLangfuseService{deleteErr: errors.New("db error")}
	h := newLangfuseHandler(fake)
	c, w := newLangfuseCtx(http.MethodDelete, "/api/v1/langfuse/api-key", "")

	h.DeleteAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeDataError {
		t.Errorf("expected CodeDataError (%d), got %d", common.CodeDataError, code)
	}
}

func TestLangfuseDeleteAPIKey_Success(t *testing.T) {
	fake := &fakeLangfuseService{deleted: true, deleteErr: nil}
	h := newLangfuseHandler(fake)
	c, w := newLangfuseCtx(http.MethodDelete, "/api/v1/langfuse/api-key", "")

	h.DeleteAPIKey(c)

	resp := decodeLangfuseResp(t, w)
	if code := int(resp["code"].(float64)); code != common.CodeSuccess {
		t.Errorf("expected CodeSuccess (%d), got %d", common.CodeSuccess, code)
	}
	data, ok := resp["data"].(bool)
	if !ok || !data {
		t.Errorf("expected data=true in success response, got %v", resp["data"])
	}
}
