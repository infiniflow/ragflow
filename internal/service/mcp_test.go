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
	"testing"

	"ragflow/internal/entity"
)

func TestIsValidMCPServerType(t *testing.T) {
	valid := []string{entity.MCPServerTypeSSE, entity.MCPServerTypeStreamableHTTP, "sse", "streamable-http"}
	for _, v := range valid {
		if !entity.IsValidMCPServerType(v) {
			t.Errorf("expected %q to be a valid MCP server type", v)
		}
	}
	invalid := []string{"", "stdio", "http", "SSE"}
	for _, v := range invalid {
		if entity.IsValidMCPServerType(v) {
			t.Errorf("expected %q to be an invalid MCP server type", v)
		}
	}
}

func TestServerTestValidation(t *testing.T) {
	s := &MCPService{}

	// Empty URL is rejected before any connection attempt.
	if _, err := s.TestServer("", entity.MCPServerTypeSSE); !errors.Is(err, ErrMCPInvalidURL) {
		t.Errorf("expected ErrMCPInvalidURL for empty url, got %v", err)
	}

	// Invalid server type is rejected.
	if _, err := s.TestServer("http://localhost:1234/sse", "stdio"); !errors.Is(err, ErrMCPInvalidType) {
		t.Errorf("expected ErrMCPInvalidType for bad type, got %v", err)
	}

	// Valid request shape: the live MCP client is not ported yet, so the call
	// surfaces ErrMCPTestUnsupported rather than attempting a connection.
	if _, err := s.TestServer("http://localhost:1234/sse", entity.MCPServerTypeSSE); !errors.Is(err, ErrMCPTestUnsupported) {
		t.Errorf("expected ErrMCPTestUnsupported for valid request, got %v", err)
	}
}
