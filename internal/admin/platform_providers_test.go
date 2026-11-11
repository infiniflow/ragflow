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

package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

func TestCreatePlatformProviderPreservesFirstCreator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testutil.SetupTestDB(t, &entity.SystemSettings{}, &entity.Tenant{}, &entity.User{}, &entity.UserTenant{}, &entity.TenantModelProvider{})
	t.Cleanup(testutil.ReplaceDBForTest(t, db))
	if dao.GetModelProviderManager().FindProvider("OpenAI") == nil {
		t.Fatal("missing OpenAI provider catalog")
	}
	h := &Handler{}
	for _, creator := range []string{"admin-a", "admin-b"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Set("user_id", creator)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/providers", strings.NewReader(`{"provider_name":"openai"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		h.CreatePlatformProvider(ctx)
		var body struct {
			Code common.ErrorCode `json:"code"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != common.CodeSuccess {
			t.Fatalf("create %s: %s, %v", creator, recorder.Body.String(), err)
		}
	}
	var providers []entity.TenantModelProvider
	if err := db.Where("tenant_id = ?", entity.PlatformTenantID).Find(&providers).Error; err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].UserID != "admin-a" || providers[0].ProviderName != "OpenAI" {
		t.Fatalf("providers = %+v", providers)
	}
}

func TestPlatformManagementRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	NewRouter(&Handler{}).Setup(engine)
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/admin/providers"},
		{http.MethodPut, "/api/v1/admin/providers"},
		{http.MethodGet, "/api/v1/admin/providers/catalog"},
		{http.MethodPost, "/api/v1/admin/providers/p/connection"},
		{http.MethodPost, "/api/v1/admin/providers/p/models/discover"},
		{http.MethodGet, "/api/v1/admin/providers/p/instances"},
		{http.MethodPost, "/api/v1/admin/providers/p/instances"},
		{http.MethodGet, "/api/v1/admin/providers/p/instances/i"},
		{http.MethodPut, "/api/v1/admin/providers/p/instances/i"},
		{http.MethodGet, "/api/v1/admin/providers/p/instances/i/models"},
		{http.MethodPost, "/api/v1/admin/providers/p/instances/i/models"},
		{http.MethodPatch, "/api/v1/admin/providers/p/instances/i/models/m"},
		{http.MethodGet, "/api/v1/admin/models/default"},
		{http.MethodPatch, "/api/v1/admin/models/default"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, nil))
			var body struct {
				Code common.ErrorCode `json:"code"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Code != common.CodeUnauthorized {
				t.Fatalf("unauthenticated response = %s, %v", recorder.Body.String(), err)
			}
		})
	}
}

func TestPlatformDiscoveryRejectsEchoedCredentials(t *testing.T) {
	for _, key := range []string{"platform-secret", `{"api_key":"platform-secret"}`} {
		if !platformDiscoveryContainsCredential([]map[string]string{{"name": "echo platform-secret"}}, key) {
			t.Fatal("credential was not detected")
		}
		if platformDiscoveryContainsCredential([]map[string]string{{"name": "model"}}, key) {
			t.Fatal("safe model was rejected")
		}
	}
}

func TestPlatformDiscoveryAcceptsShortKeyAndPublicConfig(t *testing.T) {
	items := []map[string]string{{"name": "model"}}
	for _, key := range []string{"a", `{"api_key":"secret-long-value","auth_mode":"model"}`} {
		if platformDiscoveryContainsCredential(items, key) {
			t.Fatalf("false positive for %s", key)
		}
	}
	if !platformDiscoveryContainsCredential([]map[string]string{{"name": "prefix-abc123-suffix"}}, "abc123") {
		t.Fatal("embedded credential not detected")
	}
	if !platformDiscoveryContainsCredential([]map[string]string{{"name": "prefixabc123suffix"}}, "abc123") {
		t.Fatal("embedded credential without delimiters not detected")
	}
	if !platformDiscoveryContainsCredential([]map[string]string{{"name": "secret"}}, `{"aws_sk":"secret"}`) {
		t.Fatal("structured credential field not detected")
	}
	if !platformDiscoveryContainsCredential([]map[string]string{{"name": "Bearer a"}}, "a") {
		t.Fatal("bearer credential not detected")
	}
}

func TestPlatformDiscoveryChecksPlainJSONLiteralCredentials(t *testing.T) {
	for _, key := range []string{"1234567890", "true"} {
		if !platformDiscoveryContainsCredential([]map[string]string{{"name": "echo-" + key}}, key) {
			t.Fatalf("credential %q was not detected", key)
		}
	}
}
