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
	"ragflow/internal/entity"
	"ragflow/internal/utility"
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

	s := NewMCPService()
	ctx := t.Context()

	nameReq := func(name string) UpdateMCPServerRequest {
		raw, err := json.Marshal(name)
		if err != nil {
			t.Fatalf("marshal name: %v", err)
		}
		return UpdateMCPServerRequest{"name": raw}
	}

	// Renaming to an existing server name of the same tenant is rejected.
	if _, code, err := s.UpdateMCPServer(ctx, tenantID, "mcp-1", nameReq("beta")); err == nil || code != common.CodeDataError {
		t.Errorf("expected duplicated name data error, got code=%v err=%v", code, err)
	}

	// Keeping the current name is allowed.
	if _, code, err := s.UpdateMCPServer(ctx, tenantID, "mcp-1", nameReq("alpha")); err != nil || code != common.CodeSuccess {
		t.Errorf("expected success keeping current name, got code=%v err=%v", code, err)
	}

	// Renaming to a fresh name is allowed.
	updated, code, err := s.UpdateMCPServer(ctx, tenantID, "mcp-1", nameReq("gamma"))
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
	s := &MCPService{}

	// Missing url and type produce an in-band error per entry rather than
	// failing the batch.
	configs := map[string]map[string]interface{}{
		"missing-fields": {"foo": "bar"},
		"bad-type":       {"url": "http://example.com", "type": "stdio"},
	}
	ctx := t.Context()
	results, err := s.ImportServers(ctx, "tenant-1", configs, 1)
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

			srv, captured := newMCPDiscoveryTestServer(t)
			defer srv.Close()
			withMCPDiscoveryOverrides(t, srv)

			results, err := NewMCPService().ImportServers(t.Context(), "tenant-1", map[string]map[string]interface{}{
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
			reimported, err := NewMCPService().ImportServers(t.Context(), "tenant-1", map[string]map[string]interface{}{
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
			if err := testDB.Where("name = ?", "parallel_0").First(&roundTripped).Error; err != nil {
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

func TestPaginateMCPServersNegativeValuesMatchPythonSlice(t *testing.T) {
	servers := makeMCPServers(13)

	got := paginateMCPServers(servers, -1, -2)

	if len(got) != 0 {
		t.Fatalf("expected empty page for negative pagination, got %d servers", len(got))
	}
}

func TestPaginateMCPServersKeepsUnpagedList(t *testing.T) {
	servers := makeMCPServers(3)

	got := paginateMCPServers(servers, 0, 0)

	if len(got) != len(servers) {
		t.Fatalf("expected unpaged list length %d, got %d", len(servers), len(got))
	}
}

func TestPaginateMCPServersPositiveValues(t *testing.T) {
	servers := makeMCPServers(5)

	got := paginateMCPServers(servers, 2, 2)

	if len(got) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(got))
	}
	if got[0].ID != "server-3" || got[1].ID != "server-4" {
		t.Fatalf("expected second page servers, got %q and %q", got[0].ID, got[1].ID)
	}
}

func makeMCPServers(count int) []*entity.MCPServer {
	servers := make([]*entity.MCPServer, 0, count)
	for i := 1; i <= count; i++ {
		servers = append(servers, &entity.MCPServer{ID: fmt.Sprintf("server-%d", i)})
	}
	return servers
}
