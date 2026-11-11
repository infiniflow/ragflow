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

package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/handler"
	"ragflow/internal/openapi"

	"github.com/gin-gonic/gin"
)

type dummyReq struct {
	Name string `json:"name"`
}

type dummyResp struct {
	Status string `json:"status"`
}

func TestRouterRegistersDocsAndOpenAPIEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	r := &Router{authHandler: handler.NewAuthHandler()}
	r.Setup(engine)

	// 1. Verify GET /docs returns 200 and Swagger UI HTML
	for _, docPath := range []string{"/docs", "/docs/", "/api/v1/docs", "/api/v1/docs/"} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, docPath, nil)
		engine.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d; want 200", docPath, resp.Code)
		}
		if !strings.Contains(resp.Header().Get("Content-Type"), "text/html") {
			t.Errorf("GET %s expected text/html Content-Type, got %s", docPath, resp.Header().Get("Content-Type"))
		}
		if !strings.Contains(resp.Body.String(), "SwaggerUIBundle") {
			t.Errorf("GET %s missing SwaggerUIBundle in response body", docPath)
		}
	}

	// 2. Verify GET /openapi.json returns valid OpenAPI 3.0 document with migrated typed contracts
	for _, specPath := range []string{"/openapi.json", "/api/v1/openapi.json"} {
		resp := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, specPath, nil)
		engine.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("GET %s status=%d; want 200", specPath, resp.Code)
		}
		if !strings.Contains(resp.Header().Get("Content-Type"), "application/json") {
			t.Errorf("GET %s expected application/json Content-Type, got %s", specPath, resp.Header().Get("Content-Type"))
		}

		var doc openapi.Document
		if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
			t.Fatalf("GET %s returned invalid json: %v", specPath, err)
		}
		if doc.OpenAPI != "3.0.3" {
			t.Errorf("GET %s expected openapi 3.0.3, got %s", specPath, doc.OpenAPI)
		}

		// Verify the migrated real simple endpoint
		docItem := doc.Paths["/api/v1/documents/{id}"]
		if docItem == nil || docItem.Get == nil {
			t.Errorf("GET %s expected /api/v1/documents/{id} in openapi paths", specPath)
		} else {
			if len(docItem.Get.Parameters) == 0 || docItem.Get.Parameters[0].Name != "id" {
				t.Errorf("expected path parameter 'id' for /api/v1/documents/{id}")
			}
		}

		// Verify the migrated real complex multipart endpoint
		agentItem := doc.Paths["/api/v1/agents/{canvas_id}/upload"]
		if agentItem == nil || agentItem.Post == nil {
			t.Errorf("GET %s expected /api/v1/agents/{canvas_id}/upload in openapi paths", specPath)
		} else {
			if agentItem.Post.RequestBody == nil || agentItem.Post.RequestBody.Content["multipart/form-data"] == nil {
				t.Errorf("expected multipart/form-data request body for agent upload")
			}
		}
	}

	// 3. Register a typed route and verify it is published to OpenAPI
	RegisterTypedRoute(
		r,
		engine,
		http.MethodPost,
		"/api/v1/new-typed-endpoint",
		"New typed endpoint",
		func(ctx context.Context, req *dummyReq) (*dummyResp, error) {
			return &dummyResp{Status: "ok"}, nil
		},
	)

	resp := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/openapi.json", nil)
	engine.ServeHTTP(resp, req)

	var doc openapi.Document
	if err := json.Unmarshal(resp.Body.Bytes(), &doc); err != nil {
		t.Fatalf("failed to unmarshal openapi doc: %v", err)
	}
	if _, ok := doc.Paths["/api/v1/new-typed-endpoint"]; !ok {
		t.Fatalf("expected /api/v1/new-typed-endpoint in openapi paths after typed registration")
	}
}
