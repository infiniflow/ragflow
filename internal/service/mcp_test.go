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
	"strings"
	"testing"

	"ragflow/internal/entity"
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

func TestServerInputValidation(t *testing.T) {
	s := &MCPService{}

	// Empty URL is rejected before any connection attempt.
	if _, err := s.TestServer("id-1", &TestServerRequest{ServerType: mcpServerTypeSSE}); !errors.Is(err, ErrMCPInvalidURL) {
		t.Errorf("expected ErrMCPInvalidURL for empty url, got %v", err)
	}

	// nil body is treated as empty URL.
	if _, err := s.TestServer("id-1", nil); !errors.Is(err, ErrMCPInvalidURL) {
		t.Errorf("expected ErrMCPInvalidURL for nil body, got %v", err)
	}

	// Invalid server type is rejected before connecting.
	if _, err := s.TestServer("id-1", &TestServerRequest{URL: "http://example.com/sse", ServerType: "stdio"}); !errors.Is(err, ErrMCPInvalidType) {
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
	results, err := s.ImportServers("tenant-1", configs, 1)
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

func TestNewExportMCPServerResponseMatchesPythonDownloadShape(t *testing.T) {
	response := newExportMCPServerResponse(&entity.MCPServer{
		Name:       "weather",
		URL:        "https://example.com/mcp",
		ServerType: mcpServerTypeStreamableHTTP,
		Variables: entity.JSONMap{
			"authorization_token": "secret-token",
			"tools": map[string]interface{}{
				"forecast": map[string]interface{}{"name": "forecast"},
			},
		},
	})

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var decoded map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	server := decoded["mcpServers"]["weather"]
	if server["type"] != mcpServerTypeStreamableHTTP {
		t.Fatalf("type = %v, want %s", server["type"], mcpServerTypeStreamableHTTP)
	}
	if server["url"] != "https://example.com/mcp" {
		t.Fatalf("url = %v", server["url"])
	}
	if server["name"] != "weather" {
		t.Fatalf("name = %v", server["name"])
	}
	if server["authorization_token"] != "secret-token" {
		t.Fatalf("authorization_token = %v", server["authorization_token"])
	}
	tools, ok := server["tools"].(map[string]interface{})
	if !ok || tools["forecast"] == nil {
		t.Fatalf("tools = %#v, want forecast tool", server["tools"])
	}
}

func TestNewExportMCPServerResponseDefaultsMissingVariablesLikePython(t *testing.T) {
	response := newExportMCPServerResponse(&entity.MCPServer{
		Name:       "empty-vars",
		URL:        "https://example.com/mcp",
		ServerType: mcpServerTypeSSE,
	})

	server := response.MCPServers["empty-vars"]
	if server.AuthorizationToken != "" {
		t.Fatalf("authorization_token = %#v, want empty string", server.AuthorizationToken)
	}
	tools, ok := server.Tools.(map[string]interface{})
	if !ok {
		t.Fatalf("tools type = %T, want map[string]interface{}", server.Tools)
	}
	if len(tools) != 0 {
		t.Fatalf("tools = %#v, want empty map", tools)
	}
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
