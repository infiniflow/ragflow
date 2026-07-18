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

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
)

// TestMessage_ResolveTemplate asserts the canonical {{sys.x}} substitution
// flow: a state with sys.query="world" and a template "hello {{sys.query}}"
// must resolve to "hello world".
func TestMessage_ResolveTemplate(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-1", "task-1")
	state.Sys["query"] = "world"
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"text":   "hello {{sys.query}}",
		"stream": false,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["content"].(string)
	if got != "hello world" {
		t.Errorf("content: got %q, want %q", got, "hello world")
	}
	if _, ok := out["streamed_chunks"]; ok {
		t.Errorf("streamed_chunks must not be present, got %v", out["streamed_chunks"])
	}
	if downloads, ok := out["downloads"].([]DownloadInfo); !ok || len(downloads) != 0 {
		t.Errorf("downloads = %#v, want an empty []DownloadInfo", out["downloads"])
	}
}

func TestMessage_ResolveListReferenceAsJSON(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-list", "task-list")
	state.SetVar("list_0", "result", []any{"user: 1"})
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"text":   "{{list_0@result}}",
		"stream": false,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != `["user: 1"]` {
		t.Fatalf("content = %q, want JSON list", got)
	}
}

// TestMessage_Stream confirms the Stream() contract: the returned
// channel receives the resolved content and then closes. The outer
// SSE handler owns the [DONE] terminator.
func TestMessage_Stream(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-2", "task-2")
	state.Sys["query"] = "alice"
	ctx := withStateForTest(context.Background(), state)

	ch, err := c.Stream(ctx, map[string]any{
		"text":   "hi",
		"stream": true,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 content chunk, got %d: %+v", len(got), got)
	}
	if got[0]["content"] != "hi" {
		t.Errorf("chunk[0].content=%q, want 'hi'", got[0]["content"])
	}
	if _, ok := got[0]["done"]; ok {
		t.Errorf("component stream must not emit done marker: %+v", got[0])
	}
}

// TestMessage_NoTemplate verifies the no-ref passthrough: a plain text
// without any {{...}} reference is returned verbatim.
func TestMessage_NoTemplate(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-3", "task-3")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"text": "no refs here", "stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "no refs here" {
		t.Errorf("content: got %q, want %q", got, "no refs here")
	}
}

func TestMessage_RuntimeContentInput(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-4", "task-4")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"content": "from upstream", "stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "from upstream" {
		t.Errorf("content: got %q, want %q", got, "from upstream")
	}
}

func TestMessage_EmitsAgentMessage(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-emit", "task-emit")
	ctx := withStateForTest(context.Background(), state)
	var emitted []string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(contentDelta, thinkingDelta string) {
		if contentDelta != "" {
			emitted = append(emitted, contentDelta)
		}
		if thinkingDelta != "" {
			t.Fatalf("unexpected thinking delta: %q", thinkingDelta)
		}
	})

	out, err := c.Invoke(ctx, map[string]any{"content": "visible message"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "visible message" {
		t.Fatalf("content: got %q, want visible message", got)
	}
	if len(emitted) != 1 || emitted[0] != "visible message" {
		t.Fatalf("emitted = %#v, want [visible message]", emitted)
	}
	if !runtime.AgentMessageEventsEmitted(ctx) {
		t.Fatal("AgentMessageEventsEmitted = false, want true")
	}
}

func TestMessage_EmitsDirectCanvasMessage(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-direct", "task-direct")
	ctx := withStateForTest(context.Background(), state)
	var direct []string
	var agent []string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(contentDelta, thinkingDelta string) {
		if contentDelta != "" {
			agent = append(agent, contentDelta)
		}
	})
	ctx = runtime.WithCanvasMessageEmitter(ctx, func(content string) {
		direct = append(direct, content)
	})

	out, err := c.Invoke(ctx, map[string]any{"content": "direct message"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "direct message" {
		t.Fatalf("content: got %q, want direct message", got)
	}
	if len(direct) != 1 || direct[0] != "direct message" {
		t.Fatalf("direct = %#v, want [direct message]", direct)
	}
	if len(agent) != 0 {
		t.Fatalf("agent fallback should not run when direct emitter exists: %#v", agent)
	}
	if !runtime.AgentMessageEventsEmitted(ctx) {
		t.Fatal("AgentMessageEventsEmitted = false, want true")
	}
}

func TestMessage_FormalizedContentFallback(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-5", "task-5")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"formalized_content": "retrieved answer",
		"_created_time":      "2026-07-15T00:00:00Z",
		"_elapsed_time":      0.01,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "retrieved answer" {
		t.Errorf("content: got %q, want %q", got, "retrieved answer")
	}
}

func TestMessage_SingleStringFallback(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-6", "task-6")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"value":         "single upstream text",
		"_elapsed_time": 0.01,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "single upstream text" {
		t.Errorf("content: got %q, want %q", got, "single upstream text")
	}
}

// withStateForTest is a thin alias for canvas.WithState kept for
// readability at the test call sites (also defined in begin_test.go).
// Same package → same symbol; defining it twice is a compile error,
// so the canonical declaration lives in begin_test.go.
