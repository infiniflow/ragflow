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
	"encoding/json"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/entity/models"
)

// TestLLM_ForwardsTopP verifies that a TopP value on LLMParam reaches
// the ChatInvoker layer.
func TestLLM_ForwardsTopP(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	topP := 0.9
	c := NewLLMComponent(LLMParam{
		ModelID: "echo",
		TopP:    &topP,
	})
	ctx := t.Context()

	if _, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "hi"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls != 1 {
		t.Fatalf("invoker calls=%d, want 1", stub.calls)
	}
	if stub.captured == nil {
		t.Fatalf("invoker captured no request")
	}
	if stub.captured.TopP == nil {
		t.Fatalf("TopP not forwarded; got nil")
	}
	if math.Abs(*stub.captured.TopP-0.9) > 1e-9 {
		t.Errorf("TopP forwarded=%v, want 0.9", *stub.captured.TopP)
	}
}

// TestLLM_TopPFromInputs verifies that an inputs["top_p"] override reaches
// the ChatInvoker via mergeLLMParam.
func TestLLM_TopPFromInputs(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)
	ctx := t.Context()

	c := NewLLMComponent(LLMParam{ModelID: "echo"})

	if _, err := c.Invoke(ctx, nil, map[string]any{
		"user_prompt": "hi",
		"top_p":       0.7,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil || stub.captured.TopP == nil {
		t.Fatalf("TopP not propagated from inputs; captured=%+v", stub.captured)
	}
	if math.Abs(*stub.captured.TopP-0.7) > 1e-9 {
		t.Errorf("TopP forwarded=%v, want 0.7", *stub.captured.TopP)
	}
}

// TestLLM_NoTopPByDefault verifies backward compat — TopP is nil when
// neither LLMParam nor inputs set it.
func TestLLM_NoTopPByDefault(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)
	ctx := t.Context()

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	if _, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "hi"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatalf("invoker captured no request")
	}
	if stub.captured.TopP != nil {
		t.Errorf("TopP unexpectedly set to %v when no input", *stub.captured.TopP)
	}
}

// TestLLMFactory_ParsesTopP verifies that the registered LLM factory
// (registered via init()) populates LLMParam.TopP from the params map.
func TestLLMFactory_ParsesTopP(t *testing.T) {
	c, err := New("LLM", map[string]any{
		"model_id": "echo",
		"top_p":    0.85,
	})
	if err != nil {
		t.Fatalf("New(LLM): %v", err)
	}
	comp, ok := c.(*LLMComponent)
	if !ok {
		t.Fatalf("factory returned %T, want *LLMComponent", c)
	}
	if comp.param.TopP == nil {
		t.Fatalf("TopP not parsed by factory")
	}
	if math.Abs(*comp.param.TopP-0.85) > 1e-9 {
		t.Errorf("TopP parsed=%v, want 0.85", *comp.param.TopP)
	}
}

// TestLLMFactory_DisabledSamplerParams verifies that the LLM factory drops
// sampler values whose *Enabled flag is false, matching Python's
// LLMParam.gen_conf, and keeps them when the flag is true.
func TestLLMFactory_DisabledSamplerParams(t *testing.T) {
	params := map[string]any{
		"model_id":          "echo",
		"temperature":       0.2,
		"top_p":             0.75,
		"max_tokens":        256,
		"presence_penalty":  0.4,
		"frequency_penalty": 0.7,
	}
	flags := []string{"temperatureEnabled", "topPEnabled", "maxTokensEnabled", "presencePenaltyEnabled", "frequencyPenaltyEnabled"}

	build := func(enabled bool) LLMParam {
		t.Helper()
		conf := maps.Clone(params)
		for _, flag := range flags {
			conf[flag] = enabled
		}
		c, err := New("LLM", conf)
		if err != nil {
			t.Fatalf("New(LLM): %v", err)
		}
		return c.(*LLMComponent).param
	}

	p := build(false)
	if p.Temperature != nil {
		t.Errorf("disabled temperature still applied: %v", *p.Temperature)
	}
	if p.TopP != nil {
		t.Errorf("disabled top_p still applied: %v", *p.TopP)
	}
	if p.MaxTokens != nil {
		t.Errorf("disabled max_tokens still applied: %v", *p.MaxTokens)
	}
	if p.PresencePenalty != nil {
		t.Errorf("disabled presence_penalty still applied: %v", *p.PresencePenalty)
	}
	if p.FrequencyPenalty != nil {
		t.Errorf("disabled frequency_penalty still applied: %v", *p.FrequencyPenalty)
	}

	p = build(true)
	if p.Temperature == nil || p.TopP == nil || p.MaxTokens == nil || p.PresencePenalty == nil || p.FrequencyPenalty == nil {
		t.Errorf("enabled sampler params lost: temperature=%v top_p=%v max_tokens=%v presence_penalty=%v frequency_penalty=%v",
			p.Temperature != nil, p.TopP != nil, p.MaxTokens != nil, p.PresencePenalty != nil, p.FrequencyPenalty != nil)
	}
}

// TestLLM_DisabledSamplersStayOffAtRuntime verifies that runtime inputs
// carrying a sampler value without its *Enabled flag do not bring back a value
// the DSL switched off, checked on the request sent to the ChatInvoker.
func TestLLM_DisabledSamplersStayOffAtRuntime(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c, err := New("LLM", map[string]any{
		"model_id":                "echo",
		"temperature":             0.2,
		"temperatureEnabled":      false,
		"top_p":                   0.75,
		"topPEnabled":             false,
		"max_tokens":              256,
		"maxTokensEnabled":        false,
		"presence_penalty":        0.4,
		"presencePenaltyEnabled":  false,
		"frequency_penalty":       0.7,
		"frequencyPenaltyEnabled": false,
	})
	if err != nil {
		t.Fatalf("New(LLM): %v", err)
	}
	if _, err := c.Invoke(t.Context(), nil, map[string]any{
		"user_prompt":       "hi",
		"temperature":       0.9,
		"top_p":             0.5,
		"max_tokens":        128,
		"presence_penalty":  0.3,
		"frequency_penalty": 0.3,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	req := stub.captured
	if req == nil {
		t.Fatalf("invoker captured no request")
	}
	if req.Temperature != nil || req.TopP != nil || req.MaxTokens != nil || req.PresencePenalty != nil || req.FrequencyPenalty != nil {
		t.Errorf("disabled sampler params sent: temperature=%v top_p=%v max_tokens=%v presence_penalty=%v frequency_penalty=%v",
			req.Temperature != nil, req.TopP != nil, req.MaxTokens != nil, req.PresencePenalty != nil, req.FrequencyPenalty != nil)
	}
}

// TestAgent_DisabledSamplersStayOffAtRuntime verifies that runtime inputs
// carrying a sampler value without its *Enabled flag do not bring back a value
// the DSL switched off, checked on the request body sent to the provider.
func TestAgent_DisabledSamplersStayOffAtRuntime(t *testing.T) {
	setupAllowAnyHost(t, true)
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	var mu sync.Mutex
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	c, err := New("Agent", map[string]any{
		"llm_id":             "qwen-flash",
		"driver":             "Tongyi-Qianwen",
		"api_key":            "test-key",
		"base_url":           server.URL,
		"max_rounds":         1,
		"temperature":        0.2,
		"temperatureEnabled": false,
		"top_p":              0.75,
		"topPEnabled":        false,
		"max_tokens":         256,
		"maxTokensEnabled":   false,
	})
	if err != nil {
		t.Fatalf("New(Agent): %v", err)
	}
	if _, err := c.Invoke(t.Context(), nil, map[string]any{
		"user_prompt": "hi",
		"temperature": 0.9,
		"top_p":       0.5,
		"max_tokens":  128,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) == 0 {
		t.Fatalf("provider received no request")
	}
	for _, body := range bodies {
		for _, key := range []string{"temperature", "top_p", "max_tokens"} {
			if v, ok := body[key]; ok {
				t.Errorf("disabled %s sent to provider: %v", key, v)
			}
		}
	}
}

// TestAgentParam_ForwardsTopP verifies AgentParam.TopP reaches
// buildAgentChatModel and produces a ChatConfig with the value set.
//
// The check is indirect: we verify that an Agent component with TopP
// set, when invoked, calls the agent runner with the value preserved
// through mergeAgentParam.
func TestAgentParam_ForwardsTopP(t *testing.T) {
	withAgentRunner(t, func(_ context.Context, p AgentParam) (*schema.Message, error) {
		if p.TopP == nil {
			t.Errorf("TopP nil at runner; mergeAgentParam did not propagate it")
		} else if math.Abs(*p.TopP-0.5) > 1e-9 {
			t.Errorf("TopP at runner=%v, want 0.5", *p.TopP)
		}
		return &schema.Message{Content: "ok"}, nil
	})
	ctx := t.Context()

	topP := 0.5
	c := NewAgentComponent(AgentParam{
		ModelID:   "echo",
		TopP:      &topP,
		MaxRounds: 1,
	})
	if _, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "hi"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// TestAgent_TopPFromInputs verifies mergeAgentParam parses inputs["top_p"].
func TestAgent_TopPFromInputs(t *testing.T) {
	withAgentRunner(t, func(_ context.Context, p AgentParam) (*schema.Message, error) {
		if p.TopP == nil {
			t.Errorf("TopP nil at runner; inputs[top_p] not parsed")
		}
		return &schema.Message{Content: "ok"}, nil
	})
	ctx := t.Context()

	c := NewAgentComponent(AgentParam{ModelID: "echo", MaxRounds: 1})
	if _, err := c.Invoke(ctx, nil, map[string]any{
		"user_prompt": "hi",
		"top_p":       0.42,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}
