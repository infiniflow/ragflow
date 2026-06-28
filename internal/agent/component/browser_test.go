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

package component

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
)

// mockStagehandInvoker captures RunExtract requests and returns a
// canned JSON string. Used by every test in this file; the Browser
// component depends on `DefaultRuntime` which is swapped via
// `SetDefaultStagehandInvoker` and restored at test cleanup.
type mockStagehandInvoker struct {
	mu       sync.Mutex
	requests []RunExtractRequest
	rawJSON  string // canned JSON-string response (e.g. "\"hello\"")
	err      error
}

func (m *mockStagehandInvoker) RunTask(_ context.Context, _ RunTaskRequest) (string, error) {
	return "", errors.New("RunTask not used in browser tests")
}

func (m *mockStagehandInvoker) RunExtract(_ context.Context, req RunExtractRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if m.err != nil {
		return "", m.err
	}
	return m.rawJSON, nil
}

func (m *mockStagehandInvoker) lastRequest(t *testing.T) RunExtractRequest {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		t.Fatal("no RunExtract call recorded")
	}
	return m.requests[len(m.requests)-1]
}

// withMockRuntime swaps DefaultRuntime for a mock for the duration
// of t, restoring the production runtime on cleanup.
func withMockRuntime(t *testing.T, mock StagehandInvoker) {
	t.Helper()
	prev := DefaultRuntime
	SetDefaultStagehandInvoker(mock)

	t.Cleanup(func() {
		SetDefaultStagehandInvoker(prev)
	})
}

// stateWith seeds a CanvasState with a tenant_id and the supplied
// sys map. The state is attached to ctx so the Browser component
// can read it.
func stateWith(t *testing.T, sys map[string]any) context.Context {
	t.Helper()
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys = sys
	return canvas.WithState(context.Background(), state)
}

// TestBrowser_AcceptsPythonSchema: the v1 fixture's param surface
// (llm_id / prompts / max_steps / headless / enable_default_extensions
// / chromium_sandbox / persist_session) parses into
// browserParam without error.
func TestBrowser_AcceptsPythonSchema(t *testing.T) {
	_, err := NewBrowserComponent(map[string]any{
		"llm_id":                    "deepseek-v4-pro@DeepSeek",
		"prompts":                   "search for AI trends",
		"max_steps":                 30,
		"headless":                  true,
		"enable_default_extensions": false,
		"chromium_sandbox":          false,
		"persist_session":           true,
	})
	if err != nil {
		t.Fatalf("NewBrowserComponent: %v", err)
	}
}

// TestBrowser_AcceptsAliases: model_id / prompt are accepted as
// aliases for llm_id / prompts.
func TestBrowser_AcceptsAliases(t *testing.T) {
	c, err := NewBrowserComponent(map[string]any{
		"model_id": "gpt-4o@OpenAI",
		"prompt":   "summarize the page",
	})
	if err != nil {
		t.Fatalf("NewBrowserComponent: %v", err)
	}
	got := c.(*BrowserComponent).param
	if got.LLMID != "gpt-4o@OpenAI" {
		t.Errorf("LLMID: got %q, want %q (from model_id alias)", got.LLMID, "gpt-4o@OpenAI")
	}
	if got.Prompts != "summarize the page" {
		t.Errorf("Prompts: got %q, want %q (from prompt alias)", got.Prompts, "summarize the page")
	}
}

// TestBrowser_LLMIDAndPromptsRequired: both fields are required.
func TestBrowser_LLMIDAndPromptsRequired(t *testing.T) {
	_, err := NewBrowserComponent(map[string]any{"prompts": "hi"})
	if err == nil || !strings.Contains(err.Error(), "llm_id") {
		t.Errorf("expected llm_id required error, got %v", err)
	}
	_, err = NewBrowserComponent(map[string]any{"llm_id": "gpt-4o"})
	if err == nil || !strings.Contains(err.Error(), "prompts") {
		t.Errorf("expected prompts required error, got %v", err)
	}
}

// TestBrowser_ResolvesSysQueryTemplate: {sys.query} in prompts is
// resolved against canvas state before dispatch to the runtime.
func TestBrowser_ResolvesSysQueryTemplate(t *testing.T) {
	mock := &mockStagehandInvoker{rawJSON: `"ok"`}
	withMockRuntime(t, mock)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "deepseek-v4-pro@DeepSeek",
		"prompts": "{sys.query}打开百度，搜索'2026年最新AI技术趋势'",
	})
	ctx := stateWith(t, map[string]any{
		"user_id": "tenant-1",
		"query":   "what's the latest",
	})

	// LLM lookup will fail (no DB seeded in test), so we can't
	// exercise the full Invoke path. Instead, verify the template
	// resolution independently via runtime.ResolveTemplate.
	resolved, err := runtime.ResolveTemplate(c.(*BrowserComponent).param.Prompts, mustState(t, ctx))
	if err != nil {
		t.Fatalf("ResolveTemplate: %v", err)
	}
	if !strings.Contains(resolved, "what's the latest") {
		t.Errorf("resolved prompts should contain sys.query value; got %q", resolved)
	}
	if strings.Contains(resolved, "{sys.query}") {
		t.Errorf("sys.query ref not substituted: %q", resolved)
	}
}

// mustState pulls the CanvasState out of ctx; used by helpers that
// need to test template resolution without going through Invoke.
func mustState(t *testing.T, ctx context.Context) *runtime.CanvasState {
	t.Helper()
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil {
		t.Fatalf("get state from context: %v", err)
	}
	return state
}

// TestBrowser_DispatchesToRuntime: when a valid llm_id and tenant
// are provided, the resolved prompts and llm config are forwarded
// to the runtime.
//
// We override the tenant LLM lookup to return a hard error so the
// test doesn't need a seeded DB; the assertion is that the error
// surfaces with the expected wrapper AND the runtime is not called.
func TestBrowser_DispatchesToRuntime(t *testing.T) {
	mock := &mockStagehandInvoker{rawJSON: `"agent result text"`}
	withMockRuntime(t, mock)

	prevLookup := tenantLLMLookupForTest
	tenantLLMLookupForTest = func(tenantID, modelName, factory string) (string, string, error) {
		return "", "", errors.New("fake: tenant LLM not found")
	}
	t.Cleanup(func() { tenantLLMLookupForTest = prevLookup })

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "deepseek-v4-pro@DeepSeek",
		"prompts": "do something",
	})
	ctx := stateWith(t, map[string]any{"user_id": "tenant-1"})

	_, err := c.Invoke(ctx, nil)
	if err == nil {
		t.Fatal("expected tenant LLM lookup error, got nil")
	}
	if !strings.Contains(err.Error(), "tenant llm lookup") {
		t.Errorf("error %q should mention tenant llm lookup", err.Error())
	}
	if !strings.Contains(err.Error(), "fake: tenant LLM not found") {
		t.Errorf("error %q should include underlying message", err.Error())
	}
	if len(mock.requests) != 0 {
		t.Errorf("runtime should not be called when tenant lookup fails; got %d calls", len(mock.requests))
	}
}

// TestBrowser_MissingTenant: state.Sys["user_id"] is the only
// tenant handle (until the cross-cutting tenant_id fix lands).
// Missing tenant_id must error.
func TestBrowser_MissingTenant(t *testing.T) {
	mock := &mockStagehandInvoker{rawJSON: `"ok"`}
	withMockRuntime(t, mock)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "x",
	})
	// state with no user_id
	state := canvas.NewCanvasState("run-1", "task-1")
	ctx := canvas.WithState(context.Background(), state)

	_, err := c.Invoke(ctx, nil)
	if err == nil || !strings.Contains(err.Error(), "tenant_id") {
		t.Errorf("expected tenant_id error, got %v", err)
	}
}

// TestBrowser_PropagatesRuntimeError: a runtime error surfaces
// wrapped as `Browser: stagehand run: ...`.
//
// We can't easily seed the tenant DAO in a unit test, so this test
// verifies the error-wrapping contract by injecting a mock runtime
// and a no-op DAO bypass via the resolveTenantLLM indirection. For
// v1 we keep the indirection simple: we expose a package-level
// override that tests can swap.
func TestBrowser_PropagatesRuntimeError(t *testing.T) {
	mock := &mockStagehandInvoker{rawJSON: `"x"`, err: errors.New("boom")}
	withMockRuntime(t, mock)

	// Override the tenant LLM lookup so the test doesn't need a
	// real DB.
	prevLookup := tenantLLMLookupForTest
	tenantLLMLookupForTest = func(tenantID, modelName, factory string) (string, string, error) {
		return "sk-test", "https://api.openai.com/v1", nil
	}
	t.Cleanup(func() { tenantLLMLookupForTest = prevLookup })

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "x",
	})
	ctx := stateWith(t, map[string]any{"user_id": "tenant-1"})

	_, err := c.Invoke(ctx, nil)
	if err == nil {
		t.Fatal("expected runtime error, got nil")
	}
	if !strings.Contains(err.Error(), "stagehand extract") {
		t.Errorf("error should mention stagehand extract; got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should include underlying message; got %v", err)
	}
}

// TestBrowser_RunExtractRequestShape verifies the RunExtractRequest
// fields forwarded to the runtime: ModelName, TenantID, APIKey,
// Schema, and the resolved Instruction.
func TestBrowser_RunExtractRequestShape(t *testing.T) {
	mock := &mockStagehandInvoker{rawJSON: `"ok"`}
	withMockRuntime(t, mock)

	prevLookup := tenantLLMLookupForTest
	tenantLLMLookupForTest = func(tenantID, modelName, factory string) (string, string, error) {
		return "sk-test", "https://api.openai.com/v1", nil
	}
	t.Cleanup(func() { tenantLLMLookupForTest = prevLookup })

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "extract the page title",
	})
	ctx := stateWith(t, map[string]any{"user_id": "tenant-1"})

	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	req := mock.lastRequest(t)
	if req.ModelName != "openai/gpt-4o" {
		t.Errorf("ModelName: got %q, want openai/gpt-4o", req.ModelName)
	}
	if req.TenantID != "tenant-1" {
		t.Errorf("TenantID: got %q, want tenant-1", req.TenantID)
	}
	if req.APIKey != "sk-test" {
		t.Errorf("APIKey: got %q, want sk-test", req.APIKey)
	}
	if req.Instruction != "extract the page title" {
		t.Errorf("Instruction: got %q, want 'extract the page title'", req.Instruction)
	}
	if req.Schema == nil {
		t.Fatal("Schema should not be nil")
	}
	if typ, ok := req.Schema["type"]; !ok || typ != "string" {
		t.Errorf("Schema.type: got %v, want 'string'", typ)
	}
}

// TestBrowser_HeadlessPropagates: the param's headless bool is
// forwarded to the runtime verbatim.
func TestBrowser_HeadlessPropagates(t *testing.T) {
	mock := &mockStagehandInvoker{rawJSON: `"ok"`}
	withMockRuntime(t, mock)

	prevLookup := tenantLLMLookupForTest
	tenantLLMLookupForTest = func(tenantID, modelName, factory string) (string, string, error) {
		return "sk-test", "", nil
	}
	t.Cleanup(func() { tenantLLMLookupForTest = prevLookup })

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":   "gpt-4o@OpenAI",
		"prompts":  "x",
		"headless": false,
	})
	ctx := stateWith(t, map[string]any{"user_id": "tenant-1"})

	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	req := mock.lastRequest(t)
	if req.Headless == nil {
		t.Fatal("Headless: got nil, want pointer to false")
	}
	if *req.Headless != false {
		t.Errorf("Headless: got %v, want false", *req.Headless)
	}
}

// TestBrowser_OutputsShape: the output map contains the Python keys
// (content, downloaded_files) and the Go-native compat keys (url,
// status, size, model_id, prompt).
func TestBrowser_OutputsShape(t *testing.T) {
	mock := &mockStagehandInvoker{rawJSON: `"the agent's final message"`}
	withMockRuntime(t, mock)

	prevLookup := tenantLLMLookupForTest
	tenantLLMLookupForTest = func(tenantID, modelName, factory string) (string, string, error) {
		return "sk-test", "", nil
	}
	t.Cleanup(func() { tenantLLMLookupForTest = prevLookup })

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "x",
	})
	ctx := stateWith(t, map[string]any{"user_id": "tenant-1"})

	out, err := c.Invoke(ctx, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "the agent's final message"; got != want {
		t.Errorf("content: got %v, want %v", got, want)
	}
	df, ok := out["downloaded_files"].([]map[string]any)
	if !ok {
		t.Fatalf("downloaded_files: got %T, want []map[string]any", out["downloaded_files"])
	}
	if len(df) != 0 {
		t.Errorf("downloaded_files: got %d entries, want 0 (v1 always empty)", len(df))
	}
	if got, want := out["model_id"], "gpt-4o@OpenAI"; got != want {
		t.Errorf("model_id: got %v, want %v", got, want)
	}
	if got, want := out["prompt"], "x"; got != want {
		t.Errorf("prompt: got %v, want %v", got, want)
	}
}

// TestBrowser_Registered: factory lookup works.
func TestBrowser_Registered(t *testing.T) {
	c, err := New("browser", map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "x",
	})
	if err != nil {
		t.Fatalf("registry lookup: %v", err)
	}
	if c.Name() != "Browser" {
		t.Errorf("Name()=%q, want Browser", c.Name())
	}
}

// TestBrowser_ParamCheck_NegativeMaxSteps: negative max_steps is
// rejected at construction.
func TestBrowser_ParamCheck_NegativeMaxSteps(t *testing.T) {
	_, err := NewBrowserComponent(map[string]any{
		"llm_id":    "gpt-4o@OpenAI",
		"prompts":   "x",
		"max_steps": -1,
	})
	if err == nil || !strings.Contains(err.Error(), "max_steps") {
		t.Errorf("expected max_steps error, got %v", err)
	}
}
