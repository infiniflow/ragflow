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

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ragflow/internal/common"

	"github.com/gin-gonic/gin"
)

func TestUserAgentsRouteRequiresEE(t *testing.T) {
	engine := gin.New()
	RegisterEERouter(engine.Group("/api/v1/admin"), NewRouter(&Handler{}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/user%40example.com/agents", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("HTTP status = %d, want %d; body = %s", response.Code, http.StatusNotImplemented, response.Body.String())
	}

	var result struct {
		Code    common.ErrorCode `json:"code"`
		Data    json.RawMessage  `json:"data"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != common.CodeNotImplemented || string(result.Data) != "null" || result.Message != "listing user agents is only available in the enterprise edition" {
		t.Fatalf("response = %s, want enterprise edition error with null data", response.Body.String())
	}
}

func TestUserDatasetsRouteRequiresEE(t *testing.T) {
	engine := gin.New()
	RegisterEERouter(engine.Group("/api/v1/admin"), NewRouter(&Handler{}))

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users/user%40example.com/datasets", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("HTTP status = %d, want %d; body = %s", response.Code, http.StatusNotImplemented, response.Body.String())
	}

	var result struct {
		Code    common.ErrorCode `json:"code"`
		Data    json.RawMessage  `json:"data"`
		Message string           `json:"message"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != common.CodeNotImplemented || string(result.Data) != "null" || result.Message != "listing user datasets is only available in the enterprise edition" {
		t.Fatalf("response = %s, want enterprise edition error with null data", response.Body.String())
	}
}
