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
	"errors"
	"strings"
	"testing"
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
