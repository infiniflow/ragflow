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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
)

func TestDifyRetrievalHealthReturnsTrueEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/api/v1/dify/retrieval/health", NewDifyHandler().RetrievalHealth)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dify/retrieval/health", nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, resp.Body.String())
	}
	if code, _ := body["code"].(float64); int(code) != int(common.CodeSuccess) {
		t.Errorf("code=%v want=%d body=%v", body["code"], common.CodeSuccess, body)
	}
	if msg, _ := body["message"].(string); msg != "success" {
		t.Errorf("message=%q want=%q", msg, "success")
	}
	if got, ok := body["data"].(bool); !ok || got != true {
		t.Errorf("data=%v want=true body=%v", body["data"], body)
	}
}

func TestDifyRetrievalHealthDoesNotRequireAuth(t *testing.T) {
	// Public probe: must succeed even with no user attached to the context.
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.GET("/api/v1/dify/retrieval/health", NewDifyHandler().RetrievalHealth)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dify/retrieval/health", nil)
	r.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}

	var body map[string]interface{}
	_ = json.Unmarshal(resp.Body.Bytes(), &body)
	if code, _ := body["code"].(float64); int(code) != int(common.CodeSuccess) {
		t.Errorf("unauthenticated probe should succeed, got code=%v", body["code"])
	}
}
