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

package utility

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseCallResult_TextBlocksConcatenated: text content
// blocks are concatenated into Result.Text with a newline
// separator (matches the "multiple text blocks → one
// human-readable result" convention used by the Python
// implementation).
func TestParseCallResult_TextBlocksConcatenated(t *testing.T) {
	raw := json.RawMessage(`{
		"content": [
			{"type": "text", "text": "first"},
			{"type": "text", "text": "second"}
		]
	}`)
	res, err := parseCallResult(raw)
	if err != nil {
		t.Fatalf("parseCallResult: %v", err)
	}
	if res.Text != "first\nsecond" {
		t.Errorf("Text=%q, want 'first\\nsecond'", res.Text)
	}
	if res.IsError {
		t.Errorf("IsError should be false")
	}
	if len(res.Content) != 2 {
		t.Errorf("Content len=%d, want 2", len(res.Content))
	}
}

// TestParseCallResult_IsErrorFlag: the isError flag is surfaced.
func TestParseCallResult_IsErrorFlag(t *testing.T) {
	raw := json.RawMessage(`{
		"content": [{"type": "text", "text": "tool said no"}],
		"isError": true
	}`)
	res, err := parseCallResult(raw)
	if err != nil {
		t.Fatalf("parseCallResult: %v", err)
	}
	if !res.IsError {
		t.Errorf("IsError should be true")
	}
	if res.Text != "tool said no" {
		t.Errorf("Text=%q, want 'tool said no'", res.Text)
	}
}

// TestParseCallResult_NonTextSkipped: non-text content blocks
// (image / audio / resource) are kept in Content but not
// concatenated into Text. This keeps the contract narrow
// while preserving the full envelope.
func TestParseCallResult_NonTextSkipped(t *testing.T) {
	raw := json.RawMessage(`{
		"content": [
			{"type": "text", "text": "see image"},
			{"type": "image", "data": "...", "mimeType": "image/png"}
		]
	}`)
	res, err := parseCallResult(raw)
	if err != nil {
		t.Fatalf("parseCallResult: %v", err)
	}
	if res.Text != "see image" {
		t.Errorf("Text=%q, want 'see image'", res.Text)
	}
	if len(res.Content) != 2 {
		t.Errorf("Content len=%d, want 2", len(res.Content))
	}
}

// TestParseCallResult_Empty: empty / null result returns an
// empty CallResult with no error.
func TestParseCallResult_Empty(t *testing.T) {
	res, err := parseCallResult(nil)
	if err != nil {
		t.Fatalf("parseCallResult(nil): %v", err)
	}
	if res.Text != "" || res.IsError || len(res.Content) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}

// TestCallTool_StreamableHTTP: drive the full session
// (initialize → notifications/initialized → tools/call) against
// a local httptest server. Verifies the request shape, the
// session id propagation, and the response parsing.
func TestCallTool_StreamableHTTP(t *testing.T) {
	defer allowLoopbackForTests(t)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req jsonRPCRequest
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		// First call (initialize) returns a session id.
		if req.Method == "initialize" {
			w.Header().Set(sessionHeader, "test-session-42")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2025-03-26"}}`))
			return
		}
		// tools/call returns the canned result.
		if req.Method == "tools/call" {
			_, _ = w.Write([]byte(`{
				"jsonrpc":"2.0","id":2,
				"result":{"content":[{"type":"text","text":"hello from mcp"}],"isError":false}
			}`))
			return
		}
		// notifications/initialized + others: 202 with no body.
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	res, err := CallTool(context.Background(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "echo",
		Arguments:  json.RawMessage(`{"msg":"hi"}`),
		HTTPClient: srv.Client(),
		Timeout:    srv.Client().Timeout,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.Text != "hello from mcp" {
		t.Errorf("Text=%q, want 'hello from mcp'", res.Text)
	}
	if res.IsError {
		t.Errorf("IsError should be false")
	}
}

// TestCallTool_ServerError: a JSON-RPC error response surfaces
// as a Go error so callers can react (ReAct loop will route
// it as a tool failure).
func TestCallTool_ServerError(t *testing.T) {
	defer allowLoopbackForTests(t)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req jsonRPCRequest
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "initialize" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
			return
		}
		if req.Method == "tools/call" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"error":{"code":-32601,"message":"method not found"}}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	_, err := CallTool(context.Background(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "missing",
		Arguments:  json.RawMessage(`{}`),
		HTTPClient: srv.Client(),
		Timeout:    srv.Client().Timeout,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tools/call") {
		t.Errorf("error should reference tools/call method, got %q", err.Error())
	}
}

// TestCallTool_MissingURL: an empty URL is rejected up front
// before any network I/O.
func TestCallTool_MissingURL(t *testing.T) {
	_, err := CallTool(context.Background(), CallOptions{ToolName: "x"})
	if err == nil {
		t.Fatalf("expected error for empty URL")
	}
	if !strings.Contains(err.Error(), "Invalid url") {
		t.Errorf("got %v, want URL error", err)
	}
}

// TestCallTool_MissingToolName: an empty tool name is rejected
// up front.
func TestCallTool_MissingToolName(t *testing.T) {
	_, err := CallTool(context.Background(), CallOptions{URL: "http://localhost:0"})
	if err == nil {
		t.Fatalf("expected error for empty tool name")
	}
	if !strings.Contains(err.Error(), "tool name") {
		t.Errorf("got %v, want tool-name error", err)
	}
}

// TestCallTool_InvalidArgumentsJSON: non-JSON arguments surface
// a clear error before hitting the network.
func TestCallTool_InvalidArgumentsJSON(t *testing.T) {
	defer allowLoopbackForTests(t)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
	}))
	defer srv.Close()
	_, err := CallTool(context.Background(), CallOptions{
		URL:        srv.URL,
		ServerType: TransportStreamableHTTP,
		ToolName:   "x",
		Arguments:  json.RawMessage(`{not json}`),
		HTTPClient: srv.Client(),
		Timeout:    srv.Client().Timeout,
	})
	if err == nil {
		t.Fatalf("expected error for invalid arguments JSON")
	}
	// The session initialize call still goes out, so the
	// error message references the post-initialize path. The
	// important property is "non-nil error".
	if !strings.Contains(err.Error(), "json") && !strings.Contains(err.Error(), "JSON") {
		t.Errorf("error should mention JSON, got %v", err)
	}
}
