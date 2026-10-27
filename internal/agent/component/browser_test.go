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
	"ragflow/internal/entity/models"
)

// mockStagehandInvoker captures RunTask requests and returns a
// canned final message. Used by every test in this file; the Browser
// component depends on `DefaultRuntime` which is swapped via
// `SetDefaultStagehandInvoker` and restored at test cleanup.
type mockStagehandInvoker struct {
	mu       sync.Mutex
	requests []RunTaskRequest
	message  string
	err      error
}

func (m *mockStagehandInvoker) RunExtract(_ context.Context, _ RunExtractRequest) (string, error) {
	return "", errors.New("Browser must execute the task before extracting page content")
}

func (m *mockStagehandInvoker) RunTask(_ context.Context, req RunTaskRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = append(m.requests, req)
	if m.err != nil {
		return "", m.err
	}
	return m.message, nil
}

func (m *mockStagehandInvoker) lastRequest(t *testing.T) RunTaskRequest {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.requests) == 0 {
		t.Fatal("no RunTask call recorded")
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
	return canvas.WithState(t.Context(), state)
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
	mock := &mockStagehandInvoker{message: "ok"}
	withMockRuntime(t, mock)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "deepseek-v4-pro@DeepSeek",
		"prompts": "{sys.query}打开百度，搜索'2026年最新AI技术趋势'",
	})
	ctx := stateWith(t, map[string]any{
		"tenant_id": "tenant-1",
		"query":     "what's the latest",
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
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil {
		t.Fatalf("get state from context: %v", err)
	}
	return state
}

// A model resolution error must stop before the browser runtime is invoked.
func TestBrowser_DispatchesToRuntime(t *testing.T) {
	mock := &mockStagehandInvoker{message: "agent result text"}
	withMockRuntime(t, mock)
	factoryErr := errors.New("tenant model unavailable")
	withBrowserModelFactory(t, func(ctx context.Context, tenantID, modelRef string) (*models.ChatModel, error) {
		if tenantID != "tenant-1" || modelRef != "selected-model" {
			t.Errorf("factory scope: tenant=%q model=%q", tenantID, modelRef)
		}
		return nil, factoryErr
	})
	c, _ := NewBrowserComponent(map[string]any{"llm_id": "selected-model", "prompts": "do something"})
	_, err := c.Invoke(stateWith(t, map[string]any{"tenant_id": "tenant-1"}), nil, nil)
	if !errors.Is(err, factoryErr) {
		t.Fatalf("expected factory error, got %v", err)
	}
	if len(mock.requests) != 0 {
		t.Fatal("runtime called after model factory failed")
	}
}

func withBrowserModelFactory(t *testing.T, factory func(context.Context, string, string) (*models.ChatModel, error)) {
	t.Helper()
	previous := browserModelFactory
	SetBrowserModelFactory(factory)
	t.Cleanup(func() { SetBrowserModelFactory(previous) })
}

func withBrowserTestModel(t *testing.T) {
	t.Helper()
	model := models.NewChatModel(nil, new("gpt-4o"), &models.APIConfig{
		ApiKey: new("sk-test"), BaseURL: new("https://api.openai.com/v1"),
	})
	withBrowserModelFactory(t, func(context.Context, string, string) (*models.ChatModel, error) { return model, nil })
}

// TestBrowser_MissingTenant: state.Sys["tenant_id"] is required.
// Missing tenant_id must error.
func TestBrowser_MissingTenant(t *testing.T) {
	mock := &mockStagehandInvoker{message: "ok"}
	withMockRuntime(t, mock)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "x",
	})
	// state with no tenant_id
	state := canvas.NewCanvasState("run-1", "task-1")
	ctx := canvas.WithState(t.Context(), state)

	_, err := c.Invoke(ctx, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "tenant_id") {
		t.Errorf("expected tenant_id error, got %v", err)
	}
}

// TestBrowser_PropagatesRuntimeError verifies runtime errors retain model context.
func TestBrowser_PropagatesRuntimeError(t *testing.T) {
	mock := &mockStagehandInvoker{message: "x", err: errors.New("boom")}
	withMockRuntime(t, mock)

	withBrowserTestModel(t)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "x",
	})
	ctx := stateWith(t, map[string]any{"tenant_id": "tenant-1"})

	_, err := c.Invoke(ctx, nil, nil)
	if err == nil {
		t.Fatal("expected runtime error, got nil")
	}
	if !strings.Contains(err.Error(), "stagehand task") {
		t.Errorf("error should mention stagehand task; got %v", err)
	}
	if !strings.Contains(err.Error(), `model="gpt-4o"`) {
		t.Errorf("error should include model context; got %v", err)
	}
	if !strings.Contains(err.Error(), "base_url=https://api.openai.com/v1") {
		t.Errorf("error should include base_url context; got %v", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error should include underlying message; got %v", err)
	}
}

func TestBrowserBaseURLForLog(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "<provider default>"},
		{name: "blank", in: " \t\n ", want: "<provider default>"},
		{name: "redacts_userinfo", in: "https://user:pass@example.com/v1", want: "https://example.com/v1"},
		{name: "invalid", in: "://bad", want: "<invalid>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := browserBaseURLForLog(tc.in); got != tc.want {
				t.Fatalf("browserBaseURLForLog(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestBrowser_RunTaskRequestShape verifies model settings, task instructions,
// and the step limit are forwarded to the navigation-capable runtime.
func TestBrowser_RunTaskRequestShape(t *testing.T) {
	mock := &mockStagehandInvoker{message: "ok"}
	withMockRuntime(t, mock)

	withBrowserTestModel(t)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":    "gpt-4o@OpenAI",
		"prompts":   "{sys.query}",
		"max_steps": 17,
	})
	instruction := "extract the title of the pr in https://github.com/infiniflow/ragflow/pull/21460"
	ctx := stateWith(t, map[string]any{"tenant_id": "tenant-1", "query": instruction})

	if _, err := c.Invoke(ctx, nil, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	req := mock.lastRequest(t)
	if req.ModelName != "gpt-4o" {
		t.Errorf("ModelName: got %q, want gpt-4o", req.ModelName)
	}
	if req.TenantID != "tenant-1" {
		t.Errorf("TenantID: got %q, want tenant-1", req.TenantID)
	}
	if req.APIKey != "sk-test" {
		t.Errorf("APIKey: got %q, want sk-test", req.APIKey)
	}
	if req.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("BaseURL: got %q", req.BaseURL)
	}
	if req.ChatModel == nil {
		t.Fatal("shared chat model missing from runtime request")
	}
	if req.Instruction != instruction {
		t.Errorf("Instruction: got %q, want %q", req.Instruction, instruction)
	}
	if req.MaxSteps != 17 {
		t.Errorf("MaxSteps: got %d, want 17", req.MaxSteps)
	}
}

// TestBrowser_HeadlessPropagates: the param's headless bool is
// forwarded to the runtime verbatim.
func TestBrowser_HeadlessPropagates(t *testing.T) {
	mock := &mockStagehandInvoker{message: "ok"}
	withMockRuntime(t, mock)

	withBrowserTestModel(t)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":   "gpt-4o@OpenAI",
		"prompts":  "x",
		"headless": false,
	})
	ctx := stateWith(t, map[string]any{"tenant_id": "tenant-1"})

	if _, err := c.Invoke(ctx, nil, nil); err != nil {
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

func TestBrowser_ChromiumSandboxPropagates(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  *bool
		want bool
	}{
		{name: "default_disabled"},
		{name: "disabled", set: new(false)},
		{name: "enabled", set: new(true), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &mockStagehandInvoker{message: "ok"}
			withMockRuntime(t, mock)

			withBrowserTestModel(t)

			conf := map[string]any{"llm_id": "gpt-4o@OpenAI", "prompts": "x"}
			if tc.set != nil {
				conf["chromium_sandbox"] = *tc.set
			}
			c, err := NewBrowserComponent(conf)
			if err != nil {
				t.Fatalf("NewBrowserComponent: %v", err)
			}
			ctx := stateWith(t, map[string]any{"tenant_id": "tenant-1"})
			if _, err := c.Invoke(ctx, nil, nil); err != nil {
				t.Fatalf("Invoke: %v", err)
			}
			if got := mock.lastRequest(t).ChromiumSandbox; got != tc.want {
				t.Errorf("ChromiumSandbox = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestBrowser_OutputsShape: the output map contains the Python keys
// (content, downloaded_files) and the Go-native compat keys (url,
// status, size, model_id, prompt).
func TestBrowser_OutputsShape(t *testing.T) {
	mock := &mockStagehandInvoker{message: "the agent's final message"}
	withMockRuntime(t, mock)

	withBrowserTestModel(t)

	c, _ := NewBrowserComponent(map[string]any{
		"llm_id":  "gpt-4o@OpenAI",
		"prompts": "x",
	})
	ctx := stateWith(t, map[string]any{"tenant_id": "tenant-1"})

	out, err := c.Invoke(ctx, nil, nil)
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

// Provider catalog coverage verifies delegation, without making provider calls.
func TestBrowser_ForwardsConfiguredProviderModels(t *testing.T) {
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatal(err)
	}
	providers := models.GetProviderManager().Providers
	if len(providers) == 0 {
		t.Fatal("provider catalog is empty")
	}
	t.Logf("verifying Browser delegates %d configured provider drivers", len(providers))
	for _, provider := range providers {
		if provider.ModelDriver == nil {
			t.Fatalf("provider %q has no configured driver", provider.Name)
		}
		t.Run(provider.Name, func(t *testing.T) {
			mock := &mockStagehandInvoker{message: "ok"}
			withMockRuntime(t, mock)
			model := models.NewChatModel(provider.ModelDriver, new("configured-browser-model"), nil)
			withBrowserModelFactory(t, func(context.Context, string, string) (*models.ChatModel, error) { return model, nil })
			c, _ := NewBrowserComponent(map[string]any{"llm_id": "configured-model", "prompts": "browse"})
			if _, err := c.Invoke(stateWith(t, map[string]any{"tenant_id": "tenant-1"}), nil, nil); err != nil {
				t.Fatal(err)
			}
			req := mock.lastRequest(t)
			if req.ChatModel != model || req.ChatModel.ModelDriver != provider.ModelDriver || req.ModelName != *model.ModelName {
				t.Errorf("configured provider model was replaced: name=%q model=%p", req.ModelName, req.ChatModel)
			}
		})
	}
}

func TestBrowser_ModelFactoryUnavailable(t *testing.T) {
	for _, factory := range []func(context.Context, string, string) (*models.ChatModel, error){nil, func(context.Context, string, string) (*models.ChatModel, error) { return nil, nil }} {
		withBrowserModelFactory(t, factory)
		c, _ := NewBrowserComponent(map[string]any{"llm_id": "configured-model", "prompts": "browse"})
		if _, err := c.Invoke(stateWith(t, map[string]any{"tenant_id": "tenant-1"}), nil, nil); err == nil {
			t.Fatal("expected unavailable model error")
		}
	}
}
