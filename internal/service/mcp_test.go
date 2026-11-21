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

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

func TestIsValidMCPServerType(t *testing.T) {
	for _, v := range []string{mcpServerTypeSSE, mcpServerTypeStreamableHTTP} {
		if !isValidMCPServerType(v) {
			t.Errorf("expected %q to be a valid MCP server type", v)
		}
	}
	for _, v := range []string{"", "stdio", "http", "SSE"} {
		if isValidMCPServerType(v) {
			t.Errorf("expected %q to be an invalid MCP server type", v)
		}
	}
}

func TestUpdateMCPServerRejectsDuplicatedName(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pushServiceDB(t, testDB)

	const tenantID = "tenant-1"
	for _, srv := range []*entity.MCPServer{
		{ID: "mcp-1", Name: "alpha", TenantID: tenantID, URL: "http://example.com/sse", ServerType: mcpServerTypeSSE},
		{ID: "mcp-2", Name: "beta", TenantID: tenantID, URL: "http://example.com/sse", ServerType: mcpServerTypeSSE},
	} {
		if err := testDB.Create(srv).Error; err != nil {
			t.Fatalf("create mcp server: %v", err)
		}
	}
	addMCPOwnerMembership(t, testDB, tenantID, tenantID)

	s := NewMCPService()
	ctx := serviceTestContext(t, tenantID, tenantID)

	nameReq := func(name string) UpdateMCPServerRequest {
		raw, err := json.Marshal(name)
		if err != nil {
			t.Fatalf("marshal name: %v", err)
		}
		return UpdateMCPServerRequest{"name": raw}
	}

	// Renaming to an existing server name of the same tenant is rejected.
	if _, code, err := s.UpdateMCPServer(ctx, "mcp-1", nameReq("beta")); err == nil || code != common.CodeDataError {
		t.Errorf("expected duplicated name data error, got code=%v err=%v", code, err)
	}

	// Keeping the current name is allowed.
	if _, code, err := s.UpdateMCPServer(ctx, "mcp-1", nameReq("alpha")); err != nil || code != common.CodeSuccess {
		t.Errorf("expected success keeping current name, got code=%v err=%v", code, err)
	}

	// Renaming to a fresh name is allowed.
	updated, code, err := s.UpdateMCPServer(ctx, "mcp-1", nameReq("gamma"))
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("expected success renaming to fresh name, got code=%v err=%v", code, err)
	}
	if updated.Name != "gamma" {
		t.Errorf("expected renamed server name %q, got %q", "gamma", updated.Name)
	}
}

func TestServerInputValidation(t *testing.T) {
	s := &MCPService{}
	ctx := t.Context()

	// Empty URL is rejected before any connection attempt.
	if _, err := s.TestServer(ctx, "id-1", &TestServerRequest{ServerType: mcpServerTypeSSE}); !errors.Is(err, ErrMCPInvalidURL) {
		t.Errorf("expected ErrMCPInvalidURL for empty url, got %v", err)
	}

	// nil body is treated as empty URL.
	if _, err := s.TestServer(ctx, "id-1", nil); !errors.Is(err, ErrMCPInvalidURL) {
		t.Errorf("expected ErrMCPInvalidURL for nil body, got %v", err)
	}

	// Invalid server type is rejected before connecting.
	if _, err := s.TestServer(ctx, "id-1", &TestServerRequest{URL: "http://example.com/sse", ServerType: "stdio"}); !errors.Is(err, ErrMCPInvalidType) {
		t.Errorf("expected ErrMCPInvalidType for bad type, got %v", err)
	}
}

func TestImportServersValidationErrors(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate MCP server: %v", err)
	}
	pushServiceDB(t, db)
	addMCPOwnerMembership(t, db, "tenant-1", "tenant-1")
	s := NewMCPService()

	// Missing url and type produce an in-band error per entry rather than
	// failing the batch.
	configs := map[string]map[string]interface{}{
		"missing-fields": {"foo": "bar"},
		"bad-type":       {"url": "http://example.com", "type": "stdio"},
	}
	ctx := serviceTestContext(t, "tenant-1", "tenant-1")
	results, err := s.ImportServers(ctx, configs, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Success {
			t.Errorf("expected failure result for %q", r.Server)
		}
		if r.Server == "missing-fields" && !strings.Contains(r.Message, "Missing required fields") {
			t.Errorf("unexpected message for missing-fields: %q", r.Message)
		}
		if r.Server == "bad-type" && !strings.Contains(r.Message, "Unsupported MCP server type") {
			t.Errorf("unexpected message for bad-type: %q", r.Message)
		}
	}
}

func TestJSONMapStringValuesRendersScalarVariables(t *testing.T) {
	values := entity.JSONMap{
		"text":    "value",
		"count":   float64(123),
		"ratio":   float64(1.5),
		"enabled": true,
		"missing": nil,
		"object":  map[string]interface{}{"key": "value"},
	}

	got := jsonMapStringValues(values)
	for key, want := range map[string]string{
		"text":    "value",
		"count":   "123",
		"ratio":   "1.5",
		"enabled": "True",
		"missing": "None",
	} {
		if got[key] != want {
			t.Errorf("jsonMapStringValues[%q]=%q, want %q", key, got[key], want)
		}
	}
	if _, ok := got["object"]; ok {
		t.Errorf("compound variable should not be rendered as a header value: %#v", got["object"])
	}
	if headerValues := jsonMapHeaderValues(values); len(headerValues) != 1 || headerValues["text"] != "value" {
		t.Errorf("jsonMapHeaderValues = %#v, want string headers only", headerValues)
	}
}

func TestMCPVariablesForResponseRedactsAuthorizationToken(t *testing.T) {
	stored := entity.JSONMap{"authorization_token": "ragflow-secret", "region": "us"}
	got := MCPVariablesForResponse(stored)
	if got["authorization_token"] != maskedMCPSecret || got["region"] != "us" {
		t.Fatalf("response variables = %#v", got)
	}
	if stored["authorization_token"] != "ragflow-secret" {
		t.Fatalf("stored variables were mutated: %#v", stored)
	}
}

func TestMCPResponseMasksScalarTokensAndHeaders(t *testing.T) {
	for _, token := range []any{"secret", json.Number("123"), float64(123), true, nil} {
		if got := MCPVariablesForResponse(entity.JSONMap{"authorization_token": token}); got["authorization_token"] != maskedMCPSecret {
			t.Errorf("token %v was not masked", token)
		}
	}
	headers := entity.JSONMap{"Authorization": "Bearer secret", "X-Custom": "custom-secret", "Empty": ""}
	got := MCPHeadersForResponse(headers)
	if got["Authorization"] != maskedMCPSecret || got["X-Custom"] != maskedMCPSecret || got["Empty"] != "" || headers["Authorization"] != "Bearer secret" {
		t.Fatalf("header masking mutated or exposed credentials: %#v", got)
	}
}

func TestMCPManagedTokenLifecycle(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	pushServiceDB(t, db)
	if err := db.Create(&entity.Tenant{ID: "tenant-1"}).Error; err != nil {
		t.Fatal(err)
	}
	addMCPOwnerMembership(t, db, "tenant-1", "tenant-1")
	server, captured := newMCPDiscoveryTestServer(t)
	defer server.Close()
	withMCPDiscoveryOverrides(t, server)
	s := NewMCPService()
	ctx := serviceTestContext(t, "tenant-1", "tenant-1")
	created, code, err := s.CreateMCPServer(ctx, CreateMCPServerRequest{
		Name: "managed-auth", URL: server.URL, ServerType: mcpServerTypeStreamableHTTP,
		Headers:   json.RawMessage(`{"Authorization":"Bearer ${authorization_token}"}`),
		Variables: json.RawMessage(`{"authorization_token":"ragflow-secret"}`),
	})
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("create: code=%d err=%v", code, err)
	}
	if created.URL != server.URL || created.Variables["authorization_token"] != maskedMCPSecret {
		t.Fatalf("create response exposed authentication: %#v", created)
	}
	if created.Headers["Authorization"] != maskedMCPSecret {
		t.Fatal("create response exposed headers")
	}
	listed, _, err := s.ListMCPServers(ctx, "", "", nil, "", 0, 0, "create_time", false)
	if err != nil || len(listed.MCPServers) != 1 || listed.MCPServers[0].Variables["authorization_token"] != maskedMCPSecret {
		t.Fatalf("list: result=%#v err=%v", listed, err)
	}
	testReq := &TestServerRequest{URL: server.URL, ServerType: mcpServerTypeStreamableHTTP, Headers: map[string]any(created.Headers), Variables: map[string]any{"authorization_token": maskedMCPSecret}}
	beforeRejected := len(captured())
	testReq.URL = server.URL + "/other"
	if _, err := s.TestServer(ctx, created.ID, testReq); err == nil {
		t.Fatal("preview reused credentials for a different URL")
	}
	testReq.Variables["authorization_token"] = "replacement"
	if _, err := s.TestServer(ctx, created.ID, testReq); err == nil {
		t.Fatal("preview reused masked headers for a different URL")
	}
	testReq.Variables["authorization_token"] = maskedMCPSecret
	testReq.URL = server.URL
	for _, extra := range []UpdateMCPServerRequest{
		{},
		{"variables": json.RawMessage(`{"authorization_token":"********"}`)},
		{"variables": json.RawMessage(`{"authorization_token":"replacement"}`), "headers": json.RawMessage(`{"Authorization":"********"}`)},
	} {
		extra["url"] = json.RawMessage(fmt.Sprintf("%q", server.URL+"/other"))
		if _, _, err := s.UpdateMCPServer(ctx, created.ID, extra); err == nil {
			t.Fatal("update reused credentials for a different URL")
		}
	}
	if len(captured()) != beforeRejected {
		t.Fatal("rejected endpoint changes made outbound requests")
	}
	if _, err := s.TestServer(serviceTestContext(t, "other-tenant", "other-tenant"), created.ID, testReq); err == nil {
		t.Fatal("masked preview resolved another tenant's credential")
	}
	if _, err := s.TestServer(ctx, created.ID, testReq); err != nil {
		t.Fatalf("masked preview: %v", err)
	}
	if testReq.Variables["authorization_token"] != maskedMCPSecret || testReq.Headers["Authorization"] != maskedMCPSecret {
		t.Fatal("preview mutated the client payload")
	}
	updated, code, err := s.UpdateMCPServer(ctx, created.ID, UpdateMCPServerRequest{
		"url":       json.RawMessage(fmt.Sprintf("%q", server.URL)),
		"variables": json.RawMessage(`{"authorization_token":"********"}`),
		"headers":   json.RawMessage(`{"Authorization":"********"}`),
	})
	if err != nil || code != common.CodeSuccess || updated.URL != server.URL || updated.Variables["authorization_token"] != "ragflow-secret" {
		t.Fatalf("masked update: result=%#v code=%d err=%v", updated, code, err)
	}
	stored, _, err := s.GetMCPServer(ctx, created.ID)
	if err != nil || stored.URL != server.URL || stored.Variables["authorization_token"] != "ragflow-secret" {
		t.Fatalf("reload: result=%#v err=%v", stored, err)
	}
	if stored.Headers["Authorization"] != "Bearer ${authorization_token}" {
		t.Fatal("masked update overwrote stored headers")
	}
	for _, headers := range captured() {
		if headers.Get("Authorization") != "Bearer ragflow-secret" {
			t.Fatalf("authentication not preserved: %#v", headers)
		}
	}
	exported, _, err := s.ExportMCPServer(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(exported)
	if err != nil || !strings.Contains(string(payload), "ragflow-secret") {
		t.Fatalf("explicit credential export: payload=%s err=%v", payload, err)
	}
	updated, code, err = s.UpdateMCPServer(ctx, created.ID, UpdateMCPServerRequest{
		"url":       json.RawMessage(fmt.Sprintf("%q", server.URL+"/other")),
		"variables": json.RawMessage(`{"authorization_token":"replacement"}`),
		"headers":   json.RawMessage(`{"Authorization":"Bearer ${authorization_token}"}`),
	})
	if err != nil || code != common.CodeSuccess || updated.URL != server.URL+"/other" {
		t.Fatalf("explicit credentials must allow endpoint changes: code=%d err=%v", code, err)
	}
}

func TestImportServersPreservesExplicitHeadersAndVariables(t *testing.T) {
	for _, tc := range []struct {
		name             string
		headers          map[string]interface{}
		wantHeaderValues map[string]string
	}{
		{
			name: "custom headers",
			headers: map[string]interface{}{
				"User-Agent": "ragflow",
				"X-Token":    "Bearer ${authorization_token}",
				"X-Count":    "count=$count",
			},
			wantHeaderValues: map[string]string{
				"User-Agent": "ragflow",
				"X-Token":    "Bearer secret-token",
				"X-Count":    "count=123",
			},
		},
		{
			name:             "explicit empty headers",
			headers:          map[string]interface{}{},
			wantHeaderValues: map[string]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testDB := setupServiceTestDB(t)
			if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
				t.Fatalf("migrate mcp server: %v", err)
			}
			pushServiceDB(t, testDB)
			addMCPOwnerMembership(t, testDB, "tenant-1", "tenant-1")

			srv, captured := newMCPDiscoveryTestServer(t)
			defer srv.Close()
			withMCPDiscoveryOverrides(t, srv)

			results, err := NewMCPService().ImportServers(serviceTestContext(t, "tenant-1", "tenant-1"), map[string]map[string]interface{}{
				"parallel": {
					"type":                mcpServerTypeStreamableHTTP,
					"url":                 srv.URL,
					"authorization_token": "secret-token",
					"count":               float64(123),
					"headers":             tc.headers,
				},
			}, 2)
			if err != nil {
				t.Fatalf("ImportServers: %v", err)
			}
			if len(results) != 1 || !results[0].Success {
				t.Fatalf("ImportServers results = %#v", results)
			}

			for _, headers := range captured() {
				if got := headers.Get("Authorization"); got != "" {
					t.Errorf("unexpected generated Authorization header %q", got)
				}
				for key, want := range tc.wantHeaderValues {
					if got := headers.Get(key); got != want {
						t.Errorf("%s=%q, want %q", key, got, want)
					}
				}
			}

			var stored entity.MCPServer
			if err := testDB.Where("name = ?", "parallel").First(&stored).Error; err != nil {
				t.Fatalf("load imported server: %v", err)
			}
			if len(stored.Headers) != len(tc.headers) {
				t.Fatalf("stored headers = %#v, want %#v", stored.Headers, tc.headers)
			}
			for key, want := range tc.headers {
				if got := stored.Headers[key]; got != want {
					t.Errorf("stored headers[%q]=%#v, want %#v", key, got, want)
				}
			}
			if got, ok := stored.Variables["authorization_token"].(string); !ok || got != "secret-token" {
				t.Errorf("stored authorization_token = %#v, want original token", stored.Variables["authorization_token"])
			}
			if got, ok := stored.Variables["count"].(float64); !ok || got != 123 {
				t.Errorf("stored count = %#v, want float64(123)", stored.Variables["count"])
			}

			// Export the saved record, decode the wire JSON, and import that
			// configuration again. This is the round-trip that must not grow
			// an implicit authentication header when headers is present.
			exported, err := json.Marshal(newExportMCPServerResponse(&stored))
			if err != nil {
				t.Fatalf("marshal export: %v", err)
			}
			var envelope struct {
				MCPServers map[string]map[string]interface{} `json:"mcpServers"`
			}
			if err := json.Unmarshal(exported, &envelope); err != nil {
				t.Fatalf("decode export: %v", err)
			}
			reimportConfig := envelope.MCPServers["parallel"]
			if reimportConfig == nil {
				t.Fatalf("export did not contain parallel config: %s", exported)
			}
			capturedBefore := len(captured())
			reimported, err := NewMCPService().ImportServers(serviceTestContext(t, "tenant-1", "tenant-1"), map[string]map[string]interface{}{
				"parallel": reimportConfig,
			}, 2)
			if err != nil {
				t.Fatalf("reimport exported config: %v", err)
			}
			if len(reimported) != 1 || !reimported[0].Success {
				t.Fatalf("reimport results = %#v", reimported)
			}
			for _, headers := range captured()[capturedBefore:] {
				if got := headers.Get("Authorization"); got != "" {
					t.Errorf("reimport generated Authorization header %q", got)
				}
				for key, want := range tc.wantHeaderValues {
					if got := headers.Get(key); got != want {
						t.Errorf("reimport %s=%q, want %q", key, got, want)
					}
				}
			}
			var roundTripped entity.MCPServer
			if err := testDB.Where("name = ?", "parallel(1)").First(&roundTripped).Error; err != nil {
				t.Fatalf("load reimported server: %v", err)
			}
			if len(roundTripped.Headers) != len(tc.headers) {
				t.Fatalf("reimported headers = %#v, want %#v", roundTripped.Headers, tc.headers)
			}
		})
	}
}

func newMCPDiscoveryTestServer(t *testing.T) (*httptest.Server, func() []http.Header) {
	t.Helper()
	var mu sync.Mutex
	var captured []http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = append(captured, r.Header.Clone())
		mu.Unlock()

		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var req struct {
			ID     interface{} `json:"id"`
			Method string      `json:"method"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode MCP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "test-session")
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"result":{"capabilities":{}}}`, req.ID)
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%v,"result":{"tools":[{"name":"search"}]}}`, req.ID)
		default:
			t.Errorf("unexpected MCP method %q", req.Method)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	return server, func() []http.Header {
		mu.Lock()
		defer mu.Unlock()
		out := make([]http.Header, len(captured))
		copy(out, captured)
		return out
	}
}

func withMCPDiscoveryOverrides(t *testing.T, server *httptest.Server) {
	t.Helper()
	originalAssert := common.AssertURLSafe
	originalPinned := utility.PinnedHTTPClient
	common.AssertURLSafe = func(string) (string, string, error) {
		return "mcp.test", "127.0.0.1", nil
	}
	utility.PinnedHTTPClient = func(string, string, time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		common.AssertURLSafe = originalAssert
		utility.PinnedHTTPClient = originalPinned
	})
}

func TestListMCPServersAppliesAuthorizationBeforeSQLPagination(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	pushServiceDB(t, db)
	addMCPOwnerMembership(t, db, "owner", "tenant-1")
	active := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{
		ID: "mcp-target-member", UserID: "target-user", TenantID: "tenant-1",
		Role: string(permission.RoleNormal), Status: &active,
	}).Error; err != nil {
		t.Fatalf("create MCP target membership: %v", err)
	}
	for _, server := range []*entity.MCPServer{
		{ID: "mcp-c", UserID: "owner", TenantID: "tenant-1", Name: "charlie", URL: "https://example.test/c", ServerType: mcpServerTypeSSE},
		{ID: "mcp-a", UserID: "owner", TenantID: "tenant-1", Name: "alpha", URL: "https://example.test/a", ServerType: mcpServerTypeSSE},
		{ID: "mcp-b", UserID: "owner", TenantID: "tenant-1", Name: "bravo", URL: "https://example.test/b", ServerType: mcpServerTypeSSE},
		{ID: "mcp-target", UserID: "target-user", TenantID: "tenant-1", Name: "target", URL: "https://example.test/target", ServerType: mcpServerTypeSSE},
	} {
		if err := db.Create(server).Error; err != nil {
			t.Fatalf("create MCP server %q: %v", server.ID, err)
		}
	}

	ctx := serviceTestContext(t, "owner", "tenant-1", "target-user")
	for _, serverID := range []string{"mcp-a", "mcp-b", "mcp-c", "mcp-target"} {
		if err := testsupport.GrantTestResourcePermission(db, "tenant-1", "mcp", serverID, "target-user", "read"); err != nil {
			t.Fatalf("grant selected MCP user access: %v", err)
		}
	}
	listed, code, err := NewMCPService().ListMCPServers(ctx, "tenant-1", "", nil, "", 2, 1, "name", false)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListMCPServers() = (code %d, %v), want success", code, err)
	}
	if listed.Total != 4 || len(listed.MCPServers) != 1 || listed.MCPServers[0].ID != "mcp-b" {
		t.Fatalf("ListMCPServers() = %#v, want total 4 and page [mcp-b]", listed)
	}
	listed, code, err = NewMCPService().ListMCPServers(ctx, "tenant-1", "target-user", nil, "", 0, 0, "name", false)
	if err != nil || code != common.CodeSuccess || listed.Total != 4 || len(listed.MCPServers) != 4 {
		t.Fatalf("ListMCPServers(access_user_id) = (code %d, %#v, %v), want all tenant MCP servers usable by the selected user", code, listed, err)
	}
	if err := db.Create(&entity.UserTenant{
		ID: "mcp-member-tenant-1", UserID: "member", TenantID: "tenant-1", Role: string(permission.RoleNormal), Status: &active,
	}).Error; err != nil {
		t.Fatalf("create MCP member membership: %v", err)
	}
	memberCtx := serviceTestContext(t, "member", "tenant-1")
	for _, serverID := range []string{"mcp-a", "mcp-b", "mcp-c", "mcp-target"} {
		if err := testsupport.GrantTestResourcePermission(db, "tenant-1", "mcp", serverID, "member", "read"); err != nil {
			t.Fatalf("grant MCP member access: %v", err)
		}
	}
	listed, code, err = NewMCPService().ListMCPServers(memberCtx, "tenant-1", "target-user", nil, "", 0, 0, "name", false)
	if err != nil || code != common.CodeSuccess || listed.Total != 4 || len(listed.MCPServers) != 4 {
		t.Fatalf("member ListMCPServers(access_user_id) = (code %d, %#v, %v), want tenant-visible MCP servers the member can use", code, listed, err)
	}
}

func addMCPOwnerMembership(t *testing.T, db *gorm.DB, userID, tenantID string) {
	t.Helper()
	active := string(entity.StatusValid)
	if err := db.Create(&entity.UserTenant{
		ID:        "mcp-owner-" + tenantID,
		UserID:    userID,
		TenantID:  tenantID,
		Role:      "owner",
		InvitedBy: userID,
		Status:    &active,
	}).Error; err != nil {
		t.Fatalf("create MCP owner membership: %v", err)
	}
}

func makeMCPServers(count int) []*entity.MCPServer {
	servers := make([]*entity.MCPServer, 0, count)
	for i := 1; i <= count; i++ {
		servers = append(servers, &entity.MCPServer{ID: fmt.Sprintf("server-%d", i)})
	}
	return servers
}

// sharedMCPTestSetup creates the MCP server, tenant membership, and shared
// agent canvas used by the shared-metadata tests.
func sharedMCPTestSetup(t *testing.T, serverTenant, userID, role string, dsl entity.JSONMap) {
	t.Helper()

	server := &entity.MCPServer{
		ID:         "server",
		Name:       "Project tools",
		UserID:     "owner",
		TenantID:   serverTenant,
		ServerType: mcpServerTypeStreamableHTTP,
		URL:        "https://user:credential@example.test/mcp",
		Variables: entity.JSONMap{
			"authorization_token": "credential",
			"tools": map[string]interface{}{
				"allowed":    map[string]interface{}{"name": "allowed", "inputSchema": map[string]interface{}{"type": "object"}, "secret_extension": "credential"},
				"unselected": map[string]interface{}{"name": "unselected"},
			},
		},
		Headers: entity.JSONMap{"Authorization": "credential"},
	}
	if err := dao.DB.Create(server).Error; err != nil {
		t.Fatalf("create mcp server: %v", err)
	}

	if role != "" {
		for _, rel := range []*entity.UserTenant{
			{ID: "rel-owner", UserID: "owner", TenantID: serverTenant, Role: string(permission.RoleOwner), InvitedBy: serverTenant, Status: sptr("1")},
			{ID: "rel-1", UserID: userID, TenantID: serverTenant, Role: role, InvitedBy: serverTenant, Status: sptr("1")},
		} {
			if rel.UserID == userID && rel.ID == "rel-owner" {
				continue
			}
			if err := dao.DB.Create(rel).Error; err != nil {
				t.Fatalf("create tenant membership: %v", err)
			}
		}
	}

	if dsl != nil {
		canvas := &entity.UserCanvas{
			ID:             "agent-canvas",
			UserID:         serverTenant,
			TenantID:       sptr(serverTenant),
			Permission:     "team",
			CanvasCategory: "agent_canvas",
			Title:          sptr("agent"),
			DSL:            dsl,
		}
		if err := dao.DB.Create(canvas).Error; err != nil {
			t.Fatalf("create agent canvas: %v", err)
		}
	}
}

func sharedMCPAgentDSL(selected []string) entity.JSONMap {
	tools := make(map[string]interface{}, len(selected))
	for _, name := range selected {
		tools[name] = map[string]interface{}{}
	}
	return entity.JSONMap{
		"components": map[string]interface{}{
			"agent": map[string]interface{}{
				"obj": map[string]interface{}{
					"component_name": "Agent",
					"params": map[string]interface{}{
						"mcp": []interface{}{
							map[string]interface{}{
								"mcp_id": "server",
								"tools":  tools,
							},
						},
					},
				},
			},
		},
	}
}

func TestGetSharedMCPServer_ReturnsSelectedToolMetadata(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))

	s := NewMCPService()
	server, code, err := s.GetSharedMCPServer(permission.WithIdentity(t.Context(), "member", "team"), "server")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("expected success, got code=%v err=%v", code, err)
	}
	if server.URL != "" {
		t.Errorf("expected empty shared URL, got %q", server.URL)
	}
	if server.Headers != nil && len(server.Headers) > 0 {
		t.Errorf("expected no headers in shared metadata, got %#v", server.Headers)
	}
	tools, ok := server.Variables["tools"].(map[string]interface{})
	if !ok || len(tools) != 1 {
		t.Fatalf("expected one shared tool, got %#v", server.Variables["tools"])
	}
	if _, ok := tools["allowed"]; !ok {
		t.Errorf("expected selected tool 'allowed', got %#v", tools)
	}
	if _, ok := tools["unselected"]; ok {
		t.Errorf("unselected tool should not be exposed")
	}
	meta, ok := tools["allowed"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tool metadata map, got %#v", tools["allowed"])
	}
	if _, ok := meta["secret_extension"]; ok {
		t.Errorf("secret_extension must not be exposed in shared metadata")
	}
}

func TestGetSharedMCPServer_DeniesNonMember(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "outsider", "", sharedMCPAgentDSL([]string{"allowed"}))

	s := NewMCPService()
	_, code, err := s.GetSharedMCPServer(permission.WithIdentity(t.Context(), "outsider", "team"), "server")
	if code != common.CodeDataError || err == nil {
		t.Fatalf("expected data error for non-member, got code=%v err=%v", code, err)
	}
}

func TestGetSharedMCPServer_DeniesWhenNotReferenced(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "member", "normal", nil)

	s := NewMCPService()
	_, code, err := s.GetSharedMCPServer(permission.WithIdentity(t.Context(), "member", "team"), "server")
	if code != common.CodeDataError || err == nil {
		t.Fatalf("expected data error when not referenced, got code=%v err=%v", code, err)
	}
}

func malformedAgentDSL(obj interface{}) entity.JSONMap {
	return entity.JSONMap{
		"components": map[string]interface{}{
			"agent": map[string]interface{}{
				"obj": obj,
			},
		},
	}
}

func TestGetSharedMCPServer_MalformedDSLIgnored(t *testing.T) {
	malformed := []entity.JSONMap{
		{"components": "not-a-dict"},
		{"components": map[string]interface{}{"agent": "not-a-dict"}},
		malformedAgentDSL("not-a-dict"),
		malformedAgentDSL(map[string]interface{}{
			"component_name": "Agent",
			"params":         "not-a-dict",
		}),
		malformedAgentDSL(map[string]interface{}{
			"component_name": "Agent",
			"params": map[string]interface{}{
				"mcp": "not-a-list",
			},
		}),
		malformedAgentDSL(map[string]interface{}{
			"component_name": "Agent",
			"params": map[string]interface{}{
				"mcp": []interface{}{"not-a-dict"},
			},
		}),
		malformedAgentDSL(map[string]interface{}{
			"component_name": "Agent",
			"params": map[string]interface{}{
				"mcp": []interface{}{
					map[string]interface{}{
						"mcp_id": "server",
						"tools":  "not-a-dict",
					},
				},
			},
		}),
	}
	for _, dsl := range malformed {
		t.Run("", func(t *testing.T) {
			testDB := setupServiceTestDB(t)
			if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
				t.Fatalf("migrate mcp server: %v", err)
			}
			pushServiceDB(t, testDB)

			sharedMCPTestSetup(t, "team", "member", "normal", dsl)

			s := NewMCPService()
			_, code, err := s.GetSharedMCPServer(permission.WithIdentity(t.Context(), "member", "team"), "server")
			if code != common.CodeDataError || err == nil {
				t.Fatalf("expected data error for malformed DSL, got code=%v err=%v", code, err)
			}
		})
	}
}

func TestGetSharedMCPServer_DataflowDoesNotGrantAccess(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))
	if err := dao.DB.Model(&entity.UserCanvas{}).Where("id = ?", "agent-canvas").Update("canvas_category", "dataflow_canvas").Error; err != nil {
		t.Fatalf("update canvas category: %v", err)
	}

	s := NewMCPService()
	_, code, err := s.GetSharedMCPServer(permission.WithIdentity(t.Context(), "member", "team"), "server")
	if code != common.CodeDataError || err == nil {
		t.Fatalf("expected data error for dataflow canvas, got code=%v err=%v", code, err)
	}
}

func TestGetSharedMCPServer_OtherServerReferenceDoesNotGrantAccess(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	dsl := entity.JSONMap{
		"components": map[string]interface{}{
			"agent": map[string]interface{}{
				"obj": map[string]interface{}{
					"component_name": "Agent",
					"params": map[string]interface{}{
						"mcp": []interface{}{
							map[string]interface{}{
								"mcp_id": "other-server",
								"tools":  map[string]interface{}{"allowed": map[string]interface{}{}},
							},
						},
					},
				},
			},
		},
	}
	sharedMCPTestSetup(t, "team", "member", "normal", dsl)

	s := NewMCPService()
	_, code, err := s.GetSharedMCPServer(permission.WithIdentity(t.Context(), "member", "team"), "server")
	if code != common.CodeDataError || err == nil {
		t.Fatalf("expected data error for other server reference, got code=%v err=%v", code, err)
	}
}

func TestGetSharedMCPServer_DoesNotMutateStoredVariables(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))

	var before entity.MCPServer
	if err := dao.DB.First(&before, "id = ?", "server").Error; err != nil {
		t.Fatalf("load server before: %v", err)
	}

	s := NewMCPService()
	if _, _, err := s.GetSharedMCPServer(permission.WithIdentity(t.Context(), "member", "team"), "server"); err != nil {
		t.Fatalf("GetSharedMCPServer: %v", err)
	}

	var after entity.MCPServer
	if err := dao.DB.First(&after, "id = ?", "server").Error; err != nil {
		t.Fatalf("load server after: %v", err)
	}
	beforeJSON, _ := json.Marshal(before.Variables)
	afterJSON, _ := json.Marshal(after.Variables)
	if string(beforeJSON) != string(afterJSON) {
		t.Errorf("stored variables mutated: before=%s after=%s", beforeJSON, afterJSON)
	}
}

func TestListMCPServers_IncludesSharedWhenIDsRequested(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))

	s := NewMCPService()
	resp, code, err := s.ListMCPServers(permission.WithIdentity(t.Context(), "member", "team"), "team", "", []string{"server", "server"}, "", 0, 0, "create_time", true)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("expected success, got code=%v err=%v", code, err)
	}
	if resp.Total != 1 || len(resp.MCPServers) != 1 {
		t.Fatalf("expected one server, got total=%d len=%d", resp.Total, len(resp.MCPServers))
	}
	// The duplicate requested ID must not produce a second row, and the list
	// item must not leak stored credentials.
	if fmt.Sprint(resp.MCPServers[0].Variables) == fmt.Sprint(entity.JSONMap{"authorization_token": "credential"}) {
		t.Error("list item leaked stored MCP credentials")
	}
}

func TestListMCPServers_SharedHonorsPagination(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	pushServiceDB(t, db)
	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))
	ctx := permission.WithIdentity(t.Context(), "member", "team")
	resp, code, err := NewMCPService().ListMCPServers(ctx, "team", "", []string{"server"}, "", 2, 1, "name", false)
	if err != nil || code != common.CodeSuccess || resp.Total != 1 || len(resp.MCPServers) != 0 {
		t.Fatalf("page two: response=%+v code=%d err=%v", resp, code, err)
	}
}

func TestSharedMCPServerManagementRemainsOwnerOnly(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatal(err)
	}
	pushServiceDB(t, db)
	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))
	ctx := permission.WithIdentity(t.Context(), "member", "team")
	s := NewMCPService()
	shared, code, err := s.GetMCPServer(ctx, "server")
	if err != nil || code != common.CodeSuccess || shared.URL != "" || len(shared.Headers) != 0 {
		t.Fatalf("shared detail: server=%+v code=%d err=%v", shared, code, err)
	}
	if _, _, err := s.ExportMCPServer(ctx, "server"); err == nil {
		t.Fatal("shared member could export MCP credentials")
	}
	if _, _, err := s.UpdateMCPServer(ctx, "server", UpdateMCPServerRequest{"name": json.RawMessage(`"changed"`)}); err == nil {
		t.Fatal("shared member could update the MCP registration")
	}
	if _, _, err := s.DeleteMCPServer(ctx, "server"); err == nil {
		t.Fatal("shared member could delete the MCP registration")
	}
	ownerCtx := permission.WithIdentity(t.Context(), "owner", "team")
	owned, _, err := s.GetMCPServer(ownerCtx, "server")
	if err != nil || owned.URL == "" || len(owned.Headers) == 0 {
		t.Fatalf("owner detail: server=%+v err=%v", owned, err)
	}
}

func TestListMCPServers_NoIDDoesNotEnumerateShared(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))

	s := NewMCPService()
	resp, code, err := s.ListMCPServers(permission.WithIdentity(t.Context(), "member", "team"), "team", "", nil, "", 0, 0, "create_time", true)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("expected success, got code=%v err=%v", code, err)
	}
	// The shared-ID expansion only runs for explicitly requested IDs; without
	// them the member sees the tenant-visible server exactly once.
	if resp.Total != 1 || len(resp.MCPServers) != 1 {
		t.Fatalf("expected one tenant-visible server, got total=%d len=%d", resp.Total, len(resp.MCPServers))
	}
}

func TestListMCPServers_SharedHonorsKeyword(t *testing.T) {
	testDB := setupServiceTestDB(t)
	if err := testDB.AutoMigrate(&entity.MCPServer{}); err != nil {
		t.Fatalf("migrate mcp server: %v", err)
	}
	pushServiceDB(t, testDB)

	sharedMCPTestSetup(t, "team", "member", "normal", sharedMCPAgentDSL([]string{"allowed"}))

	s := NewMCPService()
	resp, code, err := s.ListMCPServers(permission.WithIdentity(t.Context(), "member", "team"), "team", "", []string{"server"}, "unmatched", 0, 0, "create_time", true)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("expected success, got code=%v err=%v", code, err)
	}
	if resp.Total != 0 || len(resp.MCPServers) != 0 {
		t.Fatalf("expected keyword filter to exclude shared server, got total=%d len=%d", resp.Total, len(resp.MCPServers))
	}
}
