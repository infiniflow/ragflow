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

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
)

func TestJSONInternalErrorRedactsRawError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/files?tenant_id=secret-tenant", nil)

	jsonInternalError(c, errors.New("postgres password=secret host=10.0.0.1 table=tenant"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}

	if got := body["message"]; got != common.CodeServerError.Message() {
		t.Fatalf("message=%v, want %q", got, common.CodeServerError.Message())
	}
	if got := body["code"]; got != float64(common.CodeServerError) {
		t.Fatalf("code=%v, want %d", got, common.CodeServerError)
	}
	if strings.Contains(recorder.Body.String(), "password=secret") || strings.Contains(recorder.Body.String(), "10.0.0.1") {
		t.Fatalf("response leaked raw error details: %s", recorder.Body.String())
	}
}
