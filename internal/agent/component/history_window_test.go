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
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
)

// TestPrependHistory_EmptyHistory: no history → no prepend.
func TestPrependHistory_EmptyHistory(t *testing.T) {
	current := []schema.Message{{Role: schema.User, Content: "hi"}}
	out := prependHistory(current, nil, 5)
	if len(out) != 1 {
		t.Errorf("expected 1 message, got %d", len(out))
	}
}

// TestPrependHistory_WindowZero: window=0 → no prepend.
func TestPrependHistory_WindowZero(t *testing.T) {
	current := []schema.Message{{Role: schema.User, Content: "hi"}}
	hist := []map[string]any{
		{"role": "user", "content": "older"},
	}
	out := prependHistory(current, hist, 0)
	if len(out) != 1 {
		t.Errorf("expected 1 message, got %d", len(out))
	}
}

// TestPrependHistory_AllWithinWindow: history shorter than window → all kept.
func TestPrependHistory_AllWithinWindow(t *testing.T) {
	current := []schema.Message{{Role: schema.User, Content: "now"}}
	hist := []map[string]any{
		{"role": "user", "content": "turn 1"},
		{"role": "assistant", "content": "reply 1"},
		{"role": "user", "content": "turn 2"},
	}
	out := prependHistory(current, hist, 10)
	if len(out) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(out))
	}
	if out[0].Content != "turn 1" || out[0].Role != "user" {
		t.Errorf("first history entry wrong: %+v", out[0])
	}
	if out[1].Role != "assistant" {
		t.Errorf("second entry should be assistant: %+v", out[1])
	}
	if out[3].Content != "now" {
		t.Errorf("current message should be last: %+v", out[3])
	}
}

// TestPrependHistory_TruncatesToWindow: history longer than window → keep last N.
func TestPrependHistory_TruncatesToWindow(t *testing.T) {
	current := []schema.Message{{Role: schema.User, Content: "now"}}
	hist := []map[string]any{
		{"role": "user", "content": "turn 1"},
		{"role": "assistant", "content": "reply 1"},
		{"role": "user", "content": "turn 2"},
		{"role": "assistant", "content": "reply 2"},
		{"role": "user", "content": "turn 3"},
	}
	out := prependHistory(current, hist, 2)
	if len(out) != 3 {
		t.Fatalf("expected 3 messages (2 history + current), got %d", len(out))
	}
	// Should keep the last 2: turn 2 (user) and turn 3 (user), plus current.
	if out[0].Content != "reply 2" {
		t.Errorf("expected first kept entry to be 'reply 2' (the 4th of 5 with window=2), got %q", out[0].Content)
	}
	if out[1].Content != "turn 3" {
		t.Errorf("expected second kept entry to be 'turn 3', got %q", out[1].Content)
	}
	if out[2].Content != "now" {
		t.Errorf("current should be last: %+v", out[2])
	}
}

// TestPrependHistory_SkipsInvalidEntries: entries missing role or content are skipped.
func TestPrependHistory_SkipsInvalidEntries(t *testing.T) {
	current := []schema.Message{{Role: schema.User, Content: "now"}}
	hist := []map[string]any{
		{"role": "user"},       // missing content
		{"content": "no role"}, // missing role
		{"role": "user", "content": "valid"},
	}
	out := prependHistory(current, hist, 10)
	if len(out) != 2 {
		t.Fatalf("expected 2 (1 valid history + current), got %d", len(out))
	}
	if out[0].Content != "valid" {
		t.Errorf("expected valid entry to be kept; got %+v", out[0])
	}
}

// TestLLM_Invoke_HistoryWindow_PrependsFromState: end-to-end — when a
// canvas state carries history and the LLM is configured with a
// non-zero window size, the prior turns are prepended.
func TestLLM_Invoke_HistoryWindow_PrependsFromState(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	state := runtime.NewCanvasState("rid", "tid")
	state.History = []map[string]any{
		{"role": "user", "content": "earlier 1"},
		{"role": "assistant", "content": "earlier reply 1"},
	}
	window := 5
	c := NewLLMComponent(LLMParam{
		ModelID:                  "echo",
		UserPrompt:               "now",
		MessageHistoryWindowSize: window,
	})
	ctx := runtime.WithState(context.Background(), state)
	_, err := c.Invoke(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.captured == nil {
		t.Fatal("invoker captured no request")
	}
	if len(stub.captured.Messages) != 3 {
		t.Fatalf("expected 3 messages (2 history + 1 current), got %d",
			len(stub.captured.Messages))
	}
	if stub.captured.Messages[0].Content != "earlier 1" {
		t.Errorf("first msg should be 'earlier 1', got %q", stub.captured.Messages[0].Content)
	}
	if stub.captured.Messages[1].Content != "earlier reply 1" {
		t.Errorf("second msg should be 'earlier reply 1', got %q", stub.captured.Messages[1].Content)
	}
	if stub.captured.Messages[2].Content != "now" {
		t.Errorf("last msg should be current 'now', got %q", stub.captured.Messages[2].Content)
	}
}

// TestLLM_Invoke_HistoryWindow_ZeroIsNoop: when window is 0, history is
// not prepended even if present in state.
func TestLLM_Invoke_HistoryWindow_ZeroIsNoop(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	state := runtime.NewCanvasState("rid", "tid")
	state.History = []map[string]any{
		{"role": "user", "content": "should be ignored"},
	}
	c := NewLLMComponent(LLMParam{
		ModelID:    "echo",
		UserPrompt: "now",
		// MessageHistoryWindowSize: 0 (default)
	})
	ctx := runtime.WithState(context.Background(), state)
	_, _ = c.Invoke(ctx, map[string]any{})
	if len(stub.captured.Messages) != 1 {
		t.Errorf("expected only 1 message (no history), got %d", len(stub.captured.Messages))
	}
}
