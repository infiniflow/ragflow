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
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
)

// TestAddToolCallMemory_NoToolCalls: when msg has no ToolCalls, the
// function returns ("", nil) — caller skips appending to history.
func TestAddToolCallMemory_NoToolCalls(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	got, err := addToolCallMemory(context.Background(), AgentParam{ModelID: "echo"}, &schema.Message{Content: "no tools"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "" {
		t.Errorf("expected empty for no tool calls, got %q", got)
	}
	if stub.calls != 0 {
		t.Errorf("expected 0 invoker calls (no LLM needed), got %d", stub.calls)
	}
}

// TestAddToolCallMemory_SummarizesAndAppendsToState: end-to-end —
// stub returns a summary; Agent.Invoke appends it to state.History.
func TestAddToolCallMemory_SummarizesAndAppendsToState(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{
		Content: "the assistant searched docs and found 3 results",
		Model:   "echo",
	}}
	withStubInvoker(t, stub)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		// Final message with one tool call.
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID: "1", Type: "function",
				Function: schema.FunctionCall{
					Name:      "search",
					Arguments: `{"q":"what is X"}`,
				},
			}},
		}, nil
	})

	state := runtime.NewCanvasState("rid", "tid")
	c := NewAgentComponent(AgentParam{ModelID: "echo", MaxRounds: 1})
	ctx := runtime.WithState(context.Background(), state)
	_, err := c.Invoke(ctx, map[string]any{"user_prompt": "do it"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls != 1 {
		t.Errorf("expected 1 LLM call (the memory summary), got %d", stub.calls)
	}
	if len(state.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d: %+v", len(state.History), state.History)
	}
	h := state.History[0]
	if h["role"] != "assistant" {
		t.Errorf("history role=%v, want assistant", h["role"])
	}
	if !strings.Contains(h["content"].(string), "searched docs") {
		t.Errorf("history content missing summary: %q", h["content"])
	}
}

// TestAddToolCallMemory_LLMFailure: when the summary LLM fails, the
// history is NOT appended (graceful degradation).
func TestAddToolCallMemory_LLMFailure(t *testing.T) {
	stub := &stubInvoker{err: context.DeadlineExceeded}
	withStubInvoker(t, stub)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID: "1", Type: "function",
				Function: schema.FunctionCall{Name: "search"},
			}},
		}, nil
	})

	state := runtime.NewCanvasState("rid", "tid")
	c := NewAgentComponent(AgentParam{ModelID: "echo", MaxRounds: 1})
	ctx := runtime.WithState(context.Background(), state)
	_, err := c.Invoke(ctx, map[string]any{"user_prompt": "do it"})
	if err != nil {
		t.Fatalf("Invoke should not error when memory summary fails: %v", err)
	}
	if len(state.History) != 0 {
		t.Errorf("expected no history entry on summary failure, got %d", len(state.History))
	}
}
