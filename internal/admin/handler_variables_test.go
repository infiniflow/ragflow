//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupSetVariableHandler(t *testing.T) (*Handler, *gorm.DB) {
	t.Helper()

	db := testutil.SetupTestDB(t, &entity.SystemSettings{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	return &Handler{service: NewService()}, db
}

type setVariableResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func putVariable(t *testing.T, h *Handler, body string) setVariableResponse {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/admin/variables", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	h.SetVariable(ctx)

	var response setVariableResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, recorder.Body.String())
	}
	return response
}

// TestSetVariableFlattensNativeJSONScalars covers the reported regression: the
// admin console toggles SSO with a JSON boolean, and a string-typed request
// field used to reject it as a missing var_name.
func TestSetVariableFlattensNativeJSONScalars(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name      string
		varName   string
		body      string
		wantValue string
	}{
		{
			name:      "sso enable sends true",
			varName:   "feishu|sso.enabled",
			body:      `{"var_name":"feishu|sso.enabled","var_value":true}`,
			wantValue: "true",
		},
		{
			name:      "sso disable sends false",
			varName:   "feishu|sso.enabled",
			body:      `{"var_name":"feishu|sso.enabled","var_value":false}`,
			wantValue: "false",
		},
		{
			name:      "number keeps its literal",
			varName:   "mail.port",
			body:      `{"var_name":"mail.port","var_value":465}`,
			wantValue: "465",
		},
		{
			name:      "string passes through",
			varName:   "mail.server",
			body:      `{"var_name":"mail.server","var_value":"smtp.example.com"}`,
			wantValue: "smtp.example.com",
		},
		{
			name:      "empty string is a value, not a missing field",
			varName:   "mail.password",
			body:      `{"var_name":"mail.password","var_value":""}`,
			wantValue: "",
		},
		{
			name:      "json object keeps its encoding",
			varName:   "sandbox.self_managed",
			body:      `{"var_name":"sandbox.self_managed","var_value":{"endpoint":"http://sandbox:9385"}}`,
			wantValue: `{"endpoint":"http://sandbox:9385"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, db := setupSetVariableHandler(t)

			response := putVariable(t, handler, tt.body)
			if response.Code != int(common.CodeSuccess) {
				t.Fatalf("code = %d, want %d; message = %s", response.Code, common.CodeSuccess, response.Message)
			}

			var stored entity.SystemSettings
			if err := db.Where("name = ?", tt.varName).First(&stored).Error; err != nil {
				t.Fatalf("load stored setting: %v", err)
			}
			if stored.Value != tt.wantValue {
				t.Fatalf("stored value = %q, want %q", stored.Value, tt.wantValue)
			}
		})
	}
}

// TestSetVariableUpdatesExistingSetting pins the update branch: a bool-typed
// setting must keep accepting the flattened value after the row exists.
func TestSetVariableUpdatesExistingSetting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, db := setupSetVariableHandler(t)

	if response := putVariable(t, handler, `{"var_name":"feishu|sso.enabled","var_value":true}`); response.Code != int(common.CodeSuccess) {
		t.Fatalf("enable: code = %d, message = %s", response.Code, response.Message)
	}
	if response := putVariable(t, handler, `{"var_name":"feishu|sso.enabled","var_value":false}`); response.Code != int(common.CodeSuccess) {
		t.Fatalf("disable: code = %d, message = %s", response.Code, response.Message)
	}

	var stored entity.SystemSettings
	if err := db.Where("name = ?", "feishu|sso.enabled").First(&stored).Error; err != nil {
		t.Fatalf("load stored setting: %v", err)
	}
	if stored.Value != "false" {
		t.Fatalf("stored value = %q, want %q", stored.Value, "false")
	}
}

func TestSetVariableRejectsUnusableRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		body        string
		wantCode    common.ErrorCode
		wantMessage string
	}{
		{
			name:        "missing value",
			body:        `{"var_name":"mail.server"}`,
			wantCode:    common.CodeBadRequest,
			wantMessage: "Var value is required",
		},
		{
			name:        "explicit null value",
			body:        `{"var_name":"mail.server","var_value":null}`,
			wantCode:    common.CodeBadRequest,
			wantMessage: "Var value is required",
		},
		{
			name:        "missing name",
			body:        `{"var_value":"smtp.example.com"}`,
			wantCode:    common.CodeBadRequest,
			wantMessage: "Var name is required",
		},
		{
			name:        "empty name",
			body:        `{"var_name":"","var_value":"smtp.example.com"}`,
			wantCode:    common.CodeBadRequest,
			wantMessage: "Var name is required",
		},
		{
			// A value rejected by ValidateSystemSettingValue surfaces as a
			// server error, which is the pre-existing contract of this endpoint.
			name:        "bool-typed setting rejects non bool",
			body:        `{"var_name":"feishu|sso.enabled","var_value":"yes"}`,
			wantCode:    common.CodeServerError,
			wantMessage: "invalid bool value for feishu|sso.enabled: expected true or false",
		},
		{
			// Falsy scalars are legitimate values, not missing fields.
			name:        "zero is kept, not treated as missing",
			body:        `{"var_name":"mail.port","var_value":0}`,
			wantCode:    common.CodeSuccess,
			wantMessage: "Set variable successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _ := setupSetVariableHandler(t)

			response := putVariable(t, handler, tt.body)
			if response.Code != int(tt.wantCode) {
				t.Fatalf("code = %d, want %d; message = %s", response.Code, tt.wantCode, response.Message)
			}
			if response.Message != tt.wantMessage {
				t.Fatalf("message = %q, want %q", response.Message, tt.wantMessage)
			}
		})
	}
}

func TestSetVariableRejectsMalformedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, _ := setupSetVariableHandler(t)

	response := putVariable(t, handler, `{"var_name":"mail.server","var_value":`)
	if response.Code != int(common.CodeBadRequest) {
		t.Fatalf("code = %d, want %d; message = %s", response.Code, common.CodeBadRequest, response.Message)
	}
	if !strings.HasPrefix(response.Message, "Invalid request body: ") {
		t.Fatalf("message = %q, want it to name the malformed body", response.Message)
	}
}

func TestSetVariableLLMMaxRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		seeded      bool
		body        string
		wantCode    common.ErrorCode
		wantStored  string
		wantMessage string
	}{
		{name: "lower bound", seeded: true, body: `{"var_name":"llm.max_retries","var_value":0}`, wantCode: common.CodeSuccess, wantStored: "0"},
		{name: "upper bound", seeded: true, body: `{"var_name":"llm.max_retries","var_value":10}`, wantCode: common.CodeSuccess, wantStored: "10"},
		{name: "string number", seeded: true, body: `{"var_name":"llm.max_retries","var_value":"5"}`, wantCode: common.CodeSuccess, wantStored: "5"},
		{name: "empty string clears the override", seeded: true, body: `{"var_name":"llm.max_retries","var_value":""}`, wantCode: common.CodeSuccess, wantStored: ""},
		{name: "created when the seed row is missing", body: `{"var_name":"llm.max_retries","var_value":2}`, wantCode: common.CodeSuccess, wantStored: "2"},
		{name: "negative", seeded: true, body: `{"var_name":"llm.max_retries","var_value":-1}`, wantCode: common.CodeServerError, wantMessage: "llm max retries -1 out of range [0, 10]"},
		{name: "above the maximum", seeded: true, body: `{"var_name":"llm.max_retries","var_value":11}`, wantCode: common.CodeServerError, wantMessage: "llm max retries 11 out of range [0, 10]"},
		{name: "fraction", seeded: true, body: `{"var_name":"llm.max_retries","var_value":2.5}`, wantCode: common.CodeServerError, wantMessage: "invalid integer value for llm.max_retries: 2.5"},
		{name: "text", seeded: true, body: `{"var_name":"llm.max_retries","var_value":"many"}`, wantCode: common.CodeServerError, wantMessage: "invalid integer value for llm.max_retries: many"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, db := setupSetVariableHandler(t)
			const previous = "3"
			if tt.seeded {
				seed := entity.SystemSettings{Name: common.SettingLLMMaxRetries, DataType: "integer", Source: "variable", Value: previous}
				if err := db.Create(&seed).Error; err != nil {
					t.Fatal(err)
				}
			}

			response := putVariable(t, handler, tt.body)
			if response.Code != int(tt.wantCode) {
				t.Fatalf("code = %d, want %d; message = %s", response.Code, tt.wantCode, response.Message)
			}
			if tt.wantMessage != "" && response.Message != tt.wantMessage {
				t.Fatalf("message = %q, want %q", response.Message, tt.wantMessage)
			}

			var stored entity.SystemSettings
			err := db.Where("name = ?", common.SettingLLMMaxRetries).First(&stored).Error
			if tt.wantCode != common.CodeSuccess {
				if err != nil || stored.Value != previous {
					t.Fatalf("a rejected value must leave the stored one alone: value = %q, err = %v", stored.Value, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("load stored setting: %v", err)
			}
			if stored.Value != tt.wantStored || stored.DataType != "integer" {
				t.Fatalf("stored = %q (%s), want %q (integer)", stored.Value, stored.DataType, tt.wantStored)
			}
		})
	}
}

// The admin variables endpoints sit behind AuthMiddleware, which only lets
// superusers through; a request without credentials never reaches the handler.
func TestVariablesRequireAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	NewRouter(&Handler{}).Setup(engine)

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1/admin/variables", strings.NewReader(`{"var_name":"llm.max_retries","var_value":1}`)))

		var response setVariableResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("%s: decode response: %v; body = %s", method, err, recorder.Body.String())
		}
		if response.Code != int(common.CodeUnauthorized) {
			t.Errorf("%s: code = %d, want %d", method, response.Code, common.CodeUnauthorized)
		}
	}
}
