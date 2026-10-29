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
