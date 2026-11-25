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

//go:build !ee

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/server/local"
	"ragflow/internal/service"
)

// TestDocAnalyzers_RequiresAuth pins that GET /api/v1/system/doc-analyzers
// is served only behind AuthMiddleware. The production router registers it
// under the authenticated v1 group, not apiNoAuth; this test exercises the
// same handler + middleware wiring and asserts an unauthenticated request is
// rejected with 401 before DocAnalyzers runs.
func TestDocAnalyzers_RequiresAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	ah := &AuthHandler{userService: &stubUserTokenResolver{}}
	authorized := engine.Group("")
	authorized.Use(ah.AuthMiddleware())
	v1 := authorized.Group("/api/v1")
	v1.GET("/system/doc-analyzers", NewSystemHandler(nil).DocAnalyzers)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/doc-analyzers", nil)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 without auth, got %d (body %s)", w.Code, w.Body.String())
	}
}

// TestDocAnalyzers_AllowsAuthenticated pins the positive auth path: a request
// carrying a valid access token must pass AuthMiddleware and reach
// DocAnalyzers, which then returns 200 with the probed analyzer list (the
// builtin "deepdoc" entry at minimum). This is the counterpart to
// TestDocAnalyzers_RequiresAuth and closes the coverage gap for the
// authenticated success case.
func TestDocAnalyzers_AllowsAuthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	ah := &AuthHandler{
		userService: &stubUserTokenResolver{
			resolveAccessTokenFn: func(ctx context.Context, authorization string) (*service.AuthIdentity, common.ErrorCode, error) {
				return &service.AuthIdentity{
					User:     &entity.User{ID: "u-regular"},
					TenantID: "tenant-1",
				}, common.CodeSuccess, nil
			},
		},
	}
	authorized := engine.Group("")
	authorized.Use(ah.AuthMiddleware())
	v1 := authorized.Group("/api/v1")
	v1.GET("/system/doc-analyzers", NewSystemHandler(nil).DocAnalyzers)

	// The auth middleware additionally gates on admin availability; mirror
	// the unit-test setup from admin_status_ee_test.go so IsAdminAvailable()
	// reports true in either build.
	local.InitAdminStatus()
	local.SetAdminStatus(local.Available, "unit test")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/doc-analyzers", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200 with valid token, got %d (body %s)", w.Code, w.Body.String())
	}
	var resp struct {
		Code int               `json:"code"`
		Data []DocAnalyzerInfo `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0", resp.Code)
	}
	if _, ok := findAnalyzer(resp.Data, "deepdoc"); !ok {
		t.Fatalf("expected a deepdoc analyzer in the authed response, got %+v", resp.Data)
	}
}
