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
	"math"
	"testing"

	"github.com/cloudwego/eino/schema"
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
	if _, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "hi"}); err != nil {
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

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	if _, err := c.Invoke(context.Background(), map[string]any{
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

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	if _, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "hi"}); err != nil {
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

	topP := 0.5
	c := NewAgentComponent(AgentParam{
		ModelID:   "echo",
		TopP:      &topP,
		MaxRounds: 1,
	})
	if _, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "hi"}); err != nil {
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

	c := NewAgentComponent(AgentParam{ModelID: "echo", MaxRounds: 1})
	if _, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "hi",
		"top_p":       0.42,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}
