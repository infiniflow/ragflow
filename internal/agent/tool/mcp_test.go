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
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	mcpclient "ragflow/internal/utility"
)

// TestMCPToolAdapter_InfoReturnsMCPDescriptor: the eino ToolInfo
// surface matches the underlying MCP tool's name, description, and
// input schema. The input schema is fed in the real wire shape the
// MCP client produces (full JSON Schema object) so the advertised
// parameters come from "properties" — never the schema's top-level
// keys ("type"/"properties"/"required").
func TestMCPToolAdapter_InfoReturnsMCPDescriptor(t *testing.T) {
	mcp := mcpclient.Tool{
		Name:        "search_docs",
		Description: "search internal docs",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "the search query",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "max results",
				},
			},
			"required": []any{"query"},
		},
	}
	a := NewMCPToolAdapter(mcp)
	if a.Name() != "search_docs" {
		t.Errorf("Name=%q, want search_docs", a.Name())
	}
	info, err := a.Info(t.Context())
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
		t.Fatal("expected non-nil ParamsOneOf")
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	if js.Type != "object" {
		t.Errorf("schema type=%q, want object", js.Type)
	}
	if js.Properties == nil {
		t.Fatal("expected non-nil properties")
	}
	if js.Properties.Len() != 2 {
		t.Fatalf("expected 2 params, got %d", js.Properties.Len())
	}
	for _, leaked := range []string{"type", "properties", "required"} {
		if _, ok := js.Properties.Get(leaked); ok {
			t.Errorf("schema top-level key %q leaked into advertised params", leaked)
		}
	}

	query, ok := js.Properties.Get("query")
	if !ok {
		t.Fatal("expected advertised param 'query'")
	}
	if query.Type != "string" {
		t.Errorf("query.Type=%q, want string", query.Type)
	}
	if query.Description != "the search query" {
		t.Errorf("query.Description=%q, want 'the search query'", query.Description)
	}
	if !slices.Contains(js.Required, "query") {
		t.Errorf("query should be required per inputSchema.required; required=%v", js.Required)
	}

	limit, ok := js.Properties.Get("limit")
	if !ok {
		t.Fatal("expected advertised param 'limit'")
	}
	if limit.Type != "integer" {
		t.Errorf("limit.Type=%q, want integer", limit.Type)
	}
	if slices.Contains(js.Required, "limit") {
		t.Errorf("limit should not be required; required=%v", js.Required)
	}
}

// TestMCPToolAdapter_InfoWithoutPropertiesFallsBackToFreeForm: a tool
// whose schema has no "properties" map advertises no params (eino falls
// back to free-form args) and does not leak schema top-level keys.
func TestMCPToolAdapter_InfoWithoutPropertiesFallsBackToFreeForm(t *testing.T) {
	mcp := mcpclient.Tool{
		Name:        "no_schema",
		Description: "tool without property definitions",
		InputSchema: map[string]any{"type": "object"},
	}
	a := NewMCPToolAdapter(mcp)
	info, err := a.Info(t.Context())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("expected non-nil ParamsOneOf")
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	if js.Properties != nil && js.Properties.Len() != 0 {
		t.Errorf("expected no advertised params, got %d", js.Properties.Len())
	}
}

// TestMCPToolAdapter_InfoWithoutInputSchema: an MCP tool with a missing or
// empty inputSchema takes no parameters. ParamsOneOf must stay nil — a
// non-nil empty jsonschema.Schema would serialize to `true`, which OpenAI
// tool calls would read as `"parameters": true` and reject.
func TestMCPToolAdapter_InfoWithoutInputSchema(t *testing.T) {
	for name, inputSchema := range map[string]map[string]any{
		"missing": nil,
		"empty":   {},
	} {
		t.Run(name, func(t *testing.T) {
			a := NewMCPToolAdapter(mcpclient.Tool{Name: "no_params", InputSchema: inputSchema})
			info, err := a.Info(t.Context())
			if err != nil {
				t.Fatalf("Info: %v", err)
			}
			if info.ParamsOneOf != nil {
				t.Fatalf("expected nil ParamsOneOf for %s inputSchema, got non-nil", name)
			}
			raw, err := json.Marshal(info)
			if err != nil {
				t.Fatalf("Marshal ToolInfo: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("Unmarshal ToolInfo %s: %v", raw, err)
			}
			for _, leaked := range []string{"has_params_one_of", "json_schema"} {
				if _, ok := got[leaked]; ok {
					t.Fatalf("empty inputSchema must not advertise a parameters schema (key %q); got %s", leaked, raw)
				}
			}
		})
	}
}

// TestMCPToolAdapter_InfoPreservesRichJSONSchema: richer JSON Schema
// keywords (enum, default, array items, nested objects) survive the
// round-trip through eino's JSON Schema channel — the flat params form
// would have dropped them.
func TestMCPToolAdapter_InfoPreservesRichJSONSchema(t *testing.T) {
	mcp := mcpclient.Tool{
		Name:        "rich",
		Description: "tool with a rich input schema",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"mode": map[string]any{
					"type":    "string",
					"enum":    []any{"fast", "slow"},
					"default": "fast",
				},
				"tags": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"filter": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"year": map[string]any{"type": "integer"},
					},
				},
			},
		},
	}
	a := NewMCPToolAdapter(mcp)
	info, err := a.Info(t.Context())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	js, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}

	mode, ok := js.Properties.Get("mode")
	if !ok {
		t.Fatal("expected param 'mode'")
	}
	if len(mode.Enum) != 2 || mode.Enum[0] != "fast" || mode.Enum[1] != "slow" {
		t.Errorf("mode.Enum=%v, want [fast slow]", mode.Enum)
	}
	if mode.Default != "fast" {
		t.Errorf("mode.Default=%v, want 'fast'", mode.Default)
	}

	tags, ok := js.Properties.Get("tags")
	if !ok {
		t.Fatal("expected param 'tags'")
	}
	if tags.Type != "array" {
		t.Errorf("tags.Type=%q, want array", tags.Type)
	}
	if tags.Items == nil || tags.Items.Type != "string" {
		t.Errorf("tags.Items=%+v, want element type string", tags.Items)
	}

	filter, ok := js.Properties.Get("filter")
	if !ok {
		t.Fatal("expected param 'filter'")
	}
	if filter.Type != "object" {
		t.Errorf("filter.Type=%q, want object", filter.Type)
	}
	if filter.Properties == nil || filter.Properties.Len() != 1 {
		t.Fatalf("filter.Properties.Len()=%d, want 1", filter.Properties.Len())
	}
	if _, ok := filter.Properties.Get("year"); !ok {
		t.Error("expected nested param 'year'")
	}
}

// TestMCPToolAdapter_InvokableRunNotYetImplemented: the current
// mcpclient is discovery-only; InvokableRun must return a clear error
// until tools/call lands.
func TestMCPToolAdapter_InvokableRunNotYetImplemented(t *testing.T) {
	ctx := t.Context()
	a := NewMCPToolAdapter(mcpclient.Tool{Name: "x"})
	out, err := a.InvokableRun(ctx, `{"q":"hi"}`)
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
	ctx := t.Context()
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
	out, err := a.InvokableRun(ctx, `{"msg":"hi"}`)
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
	ctx := t.Context()
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
	_, err := a.InvokableRun(ctx, `{}`)
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
