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
	"ragflow/internal/entity"
	"testing"
)

func TestNewExportMCPServerResponseMatchesPythonDownloadShape(t *testing.T) {
	response := newExportMCPServerResponse(&entity.MCPServer{
		Name:       "weather",
		URL:        "https://example.com/mcp",
		ServerType: "streamable-http",
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
	if server["type"] != "streamable-http" {
		t.Fatalf("type = %v, want %s", server["type"], "streamable-http")
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
		ServerType: "sse",
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

func TestExportPreservesCustomHeadersAndNameVariable(t *testing.T) {
	input := &entity.MCPServer{Name: "server-label", URL: "https://example.com/mcp", ServerType: "streamable-http", Headers: entity.JSONMap{"User-Agent": "ragflow", "X-Token": "${custom}", "X-Name": "${name}"}, Variables: entity.JSONMap{"custom": "secret", "name": "header-identity"}}
	payload, err := json.Marshal(newExportMCPServerResponse(input))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]map[string]map[string]interface{}
	if err = json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	entry := decoded["mcpServers"]["server-label"]
	if entry["headers"] == nil || entry["custom"] != "secret" || entry["name"] != "header-identity" {
		t.Fatalf("lost connection configuration: %#v", entry)
	}
	if input.Variables["name"] != "header-identity" {
		t.Fatal("export mutated the source")
	}
}
