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
)

type fakeLangfuseService struct {
	setFn    func(tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error)
	getFn    func(tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error)
	deleteFn func(tenantID string) (bool, common.ErrorCode, string, error)
}

func (f fakeLangfuseService) SetAPIKey(tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
	if f.setFn == nil {
		return nil, common.CodeServerError, errors.New("unexpected SetAPIKey call")
	}
	return f.setFn(tenantID, secretKey, publicKey, host)
}

func (f fakeLangfuseService) GetAPIKey(tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
	if f.getFn == nil {
		return nil, common.CodeServerError, "", errors.New("unexpected GetAPIKey call")
	}
	return f.getFn(tenantID)
}

func (f fakeLangfuseService) DeleteAPIKey(tenantID string) (bool, common.ErrorCode, string, error) {
	if f.deleteFn == nil {
		return false, common.CodeServerError, "", errors.New("unexpected DeleteAPIKey call")
	}
	return f.deleteFn(tenantID)
}

func serveLangfuse(method, target, body string, h func(c *gin.Context)) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Handle(method, target, func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-1"})
		h(c)
	})

	resp := httptest.NewRecorder()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(resp, req)
	return resp
}

func decode(t *testing.T, resp *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var payload map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, resp.Body.String())
	}
	return payload
}

func TestLangfuseHandler_SetAPIKey_Success(t *testing.T) {
	var gotTenant, gotSecret, gotPublic, gotHost string
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			gotTenant, gotSecret, gotPublic, gotHost = tenantID, secretKey, publicKey, host
			return &entity.TenantLangfuse{TenantID: tenantID, SecretKey: secretKey, PublicKey: publicKey, Host: host}, common.CodeSuccess, nil
		},
	}}

	body := `{"secret_key":"sk","public_key":"pk","host":"https://a.langfuse.com"}`
	resp := serveLangfuse(http.MethodPost, "/api/v1/langfuse/api-key", body, h.SetAPIKey)

	if gotTenant != "tenant-1" || gotSecret != "sk" || gotPublic != "pk" || gotHost != "https://a.langfuse.com" {
		t.Fatalf("service args: tenant=%q secret=%q public=%q host=%q", gotTenant, gotSecret, gotPublic, gotHost)
	}
	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %v", payload["data"])
	}
	if data["secret_key"] != "sk" || data["public_key"] != "pk" || data["host"] != "https://a.langfuse.com" || data["tenant_id"] != "tenant-1" {
		t.Fatalf("unexpected data: %v", data)
	}
}

func TestLangfuseHandler_SetAPIKey_ServiceError(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			return nil, common.CodeDataError, errors.New("Invalid Langfuse keys")
		},
	}}

	body := `{"secret_key":"sk","public_key":"pk","host":"host"}`
	resp := serveLangfuse(http.MethodPost, "/api/v1/langfuse/api-key", body, h.SetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeDataError) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Invalid Langfuse keys" {
		t.Fatalf("message=%v", payload["message"])
	}
}

func TestLangfuseHandler_SetAPIKey_BindFailureStopsEarly(t *testing.T) {
	called := false
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		setFn: func(tenantID, secretKey, publicKey, host string) (*entity.TenantLangfuse, common.ErrorCode, error) {
			called = true
			return nil, common.CodeSuccess, nil
		},
	}}

	resp := serveLangfuse(http.MethodPost, "/api/v1/langfuse/api-key", `{not-json`, h.SetAPIKey)

	if called {
		t.Fatal("service should not be called when binding fails")
	}
	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeDataError) {
		t.Fatalf("payload=%v", payload)
	}
}

func TestLangfuseHandler_GetAPIKey_Success(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			return &entity.LangfuseInfoResponse{
				TenantID: tenantID, Host: "host", SecretKey: "sk", PublicKey: "pk",
				ProjectID: "proj-1", ProjectName: "My Project",
			}, common.CodeSuccess, "success", nil
		},
	}}

	resp := serveLangfuse(http.MethodGet, "/api/v1/langfuse/api-key", "", h.GetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) || payload["message"] != "success" {
		t.Fatalf("payload=%v", payload)
	}
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %v", payload["data"])
	}
	if data["project_id"] != "proj-1" || data["project_name"] != "My Project" {
		t.Fatalf("unexpected data: %v", data)
	}
}

func TestLangfuseHandler_GetAPIKey_NoRecord(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			return nil, common.CodeSuccess, "Have not record any Langfuse keys.", nil
		},
	}}

	resp := serveLangfuse(http.MethodGet, "/api/v1/langfuse/api-key", "", h.GetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Have not record any Langfuse keys." {
		t.Fatalf("message=%v", payload["message"])
	}
	if payload["data"] != nil {
		t.Fatalf("expected nil data, got %v", payload["data"])
	}
}

func TestLangfuseHandler_GetAPIKey_Unauthorized(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		getFn: func(tenantID string) (*entity.LangfuseInfoResponse, common.ErrorCode, string, error) {
			return nil, common.CodeDataError, "Invalid Langfuse keys loaded", errors.New("unauthorized")
		},
	}}

	resp := serveLangfuse(http.MethodGet, "/api/v1/langfuse/api-key", "", h.GetAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeDataError) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Invalid Langfuse keys loaded" {
		t.Fatalf("message=%v", payload["message"])
	}
}

func TestLangfuseHandler_DeleteAPIKey_Success(t *testing.T) {
	var gotTenant string
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		deleteFn: func(tenantID string) (bool, common.ErrorCode, string, error) {
			gotTenant = tenantID
			return true, common.CodeSuccess, "", nil
		},
	}}

	resp := serveLangfuse(http.MethodDelete, "/api/v1/langfuse/api-key", "", h.DeleteAPIKey)

	if gotTenant != "tenant-1" {
		t.Fatalf("tenant=%q", gotTenant)
	}
	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["data"] != true {
		t.Fatalf("expected data true, got %v", payload["data"])
	}
}

func TestLangfuseHandler_DeleteAPIKey_NoRecord(t *testing.T) {
	h := &LangfuseHandler{langfuseService: fakeLangfuseService{
		deleteFn: func(tenantID string) (bool, common.ErrorCode, string, error) {
			return false, common.CodeSuccess, "Have not record any Langfuse keys.", nil
		},
	}}

	resp := serveLangfuse(http.MethodDelete, "/api/v1/langfuse/api-key", "", h.DeleteAPIKey)

	payload := decode(t, resp)
	if payload["code"] != float64(common.CodeSuccess) {
		t.Fatalf("payload=%v", payload)
	}
	if payload["message"] != "Have not record any Langfuse keys." {
		t.Fatalf("message=%v", payload["message"])
	}
	if payload["data"] != nil {
		t.Fatalf("expected nil data, got %v", payload["data"])
	}
}
