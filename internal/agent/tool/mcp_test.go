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

package tool

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mcpclient "ragflow/internal/utility"
)

// TestMCPToolAdapter_InfoReturnsMCPDescriptor: the eino ToolInfo
// surface matches the underlying MCP tool's name, description, and
// input schema.
func TestMCPToolAdapter_InfoReturnsMCPDescriptor(t *testing.T) {
	mcp := mcpclient.Tool{
		Name:        "search_docs",
		Description: "search internal docs",
		InputSchema: map[string]any{
			"query": map[string]any{"type": "string"},
		},
	}
	a := NewMCPToolAdapter(mcp)
	if a.Name() != "search_docs" {
		t.Errorf("Name=%q, want search_docs", a.Name())
	}
	info, err := a.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "search_docs" {
		t.Errorf("ToolInfo.Name=%q, want search_docs", info.Name)
	}
	if info.Desc != "search internal docs" {
		t.Errorf("ToolInfo.Desc=%q, want 'search internal docs'", info.Desc)
	}
	if info.ParamsOneOf == nil {
		t.Error("expected non-nil ParamsOneOf")
	}
}

// TestMCPToolAdapter_InvokableRunNotYetImplemented: the current
// mcpclient is discovery-only; InvokableRun must return a clear error
// until tools/call lands.
func TestMCPToolAdapter_InvokableRunNotYetImplemented(t *testing.T) {
	a := NewMCPToolAdapter(mcpclient.Tool{Name: "x"})
	out, err := a.InvokableRun(context.Background(), `{"q":"hi"}`)
	if err == nil {
		t.Fatal("expected error from unimplemented tools/call")
	}
	if out != "" {
		t.Errorf("expected empty string result on error, got %q", out)
	}
	if !strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error message should mention 'not yet implemented'; got %v", err)
	}
	if !strings.Contains(err.Error(), "x") {
		t.Errorf("error message should mention tool name 'x'; got %v", err)
	}
}

// TestBuildMCPToolAdapters_Empty: empty input → empty output.
func TestBuildMCPToolAdapters_Empty(t *testing.T) {
	out := BuildMCPToolAdapters(nil)
	if len(out) != 0 {
		t.Errorf("expected empty, got %d", len(out))
	}
}

// TestBuildMCPToolAdapters_Multiple: each MCP tool gets a wrapper.
func TestBuildMCPToolAdapters_Multiple(t *testing.T) {
	tools := []mcpclient.Tool{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	out := BuildMCPToolAdapters(tools)
	if len(out) != 3 {
		t.Fatalf("expected 3 wrappers, got %d", len(out))
	}
	// eino's InvokableTool interface doesn't expose Name directly;
	// the name comes from the ToolInfo returned by Info(ctx). Use the
	// underlying wrapper to assert the name (we cast via the
	// concrete *MCPToolAdapter which DOES expose Name).
	names := make([]string, len(out))
	for i, w := range out {
		adapter, ok := w.(*MCPToolAdapter)
		if !ok {
			t.Fatalf("wrapper[%d] type=%T, want *MCPToolAdapter", i, w)
		}
		names[i] = adapter.Name()
	}
	want := []string{"a", "b", "c"}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("wrapper[%d].Name=%q, want %q", i, n, want[i])
		}
	}
}

// TestMarshalArguments_Empty: empty / {} returns "{}".
func TestMarshalArguments_Empty(t *testing.T) {
	cases := []string{"", "{}", "  "}
	for _, in := range cases {
		// Trim whitespace because eino's einoChatInvoker may pass
		// "  " for tools with no args.
		got, err := marshalArguments(strings.TrimSpace(in))
		if err != nil {
			t.Errorf("marshalArguments(%q): %v", in, err)
		}
		if string(got) != "{}" {
			t.Errorf("marshalArguments(%q)=%q, want {}", in, got)
		}
	}
}

// TestMarshalArguments_InvalidJSON: garbage in → clear error.
func TestMarshalArguments_InvalidJSON(t *testing.T) {
	_, err := marshalArguments("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("error should mention 'not valid JSON'; got %v", err)
	}
}

// TestMarshalArguments_ValidJSON: pass-through.
func TestMarshalArguments_ValidJSON(t *testing.T) {
	got, err := marshalArguments(`{"q":"hi","n":3}`)
	if err != nil {
		t.Fatalf("marshalArguments: %v", err)
	}
	if string(got) != `{"q":"hi","n":3}` {
		t.Errorf("got %q, want pass-through", got)
	}
}

// TestMCPToolAdapter_InvokableRunDispatchesCallTool: with a
// server URL set, InvokableRun dispatches through CallTool
// against a local httptest server. Verifies the eino tool
// envelope (string result) and the session lifecycle
// (initialize → tools/call).
func TestMCPToolAdapter_InvokableRunDispatchesCallTool(t *testing.T) {
	defer mcpLoopbackOverride(t)()
	var sawCall bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "test-session")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
		case "tools/call":
			sawCall = true
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"ok from mcp"}],"isError":false}}`))
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer srv.Close()

	a := NewMCPToolAdapterFull(mcpclient.Tool{Name: "echo"}, srv.URL, nil, 2*time.Second, srv.Client())
	out, err := a.InvokableRun(context.Background(), `{"msg":"hi"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if out != "ok from mcp" {
		t.Errorf("out=%q, want 'ok from mcp'", out)
	}
	if !sawCall {
		t.Errorf("server did not receive a tools/call request")
	}
}

// TestMCPToolAdapter_InvokableRunIsError: a tools/call response
// with isError=true surfaces as a Go error.
func TestMCPToolAdapter_InvokableRunIsError(t *testing.T) {
	defer mcpLoopbackOverride(t)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string `json:"method"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{}}`))
		case "tools/call":
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"bad input"}],"isError":true}}`))
		default:
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer srv.Close()

	a := NewMCPToolAdapterFull(mcpclient.Tool{Name: "echo"}, srv.URL, nil, 2*time.Second, srv.Client())
	_, err := a.InvokableRun(context.Background(), `{}`)
	if err == nil {
		t.Fatalf("expected error for isError response")
	}
	if !strings.Contains(err.Error(), "isError") {
		t.Errorf("error should mention isError, got %v", err)
	}
}

// mcpLoopbackOverride swaps the SSRF guard's resolver for the
// duration of the test so httptest's 127.0.0.1 server is
// accepted. The pattern mirrors the one used by
// utility/mcp_client_test.go's allowLoopbackForTests helper.
func mcpLoopbackOverride(t *testing.T) func() {
	t.Helper()
	orig := mcpclient.LookupHost
	mcpclient.LookupHost = func(_ string) ([]string, error) {
		return []string{"8.8.8.8"}, nil
	}
	return func() { mcpclient.LookupHost = orig }
}
