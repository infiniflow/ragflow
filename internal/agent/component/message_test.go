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

func TestMessage_NormalTemplateEmitsOnlyRenderedMessage(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-normal-template", "task-normal-template")
	state.Sys["query"] = "world"
	ctx := withStateForTest(context.Background(), state)
	var direct []string
	ctx = runtime.WithCanvasMessageEmitter(ctx, func(content string) {
		direct = append(direct, content)
	})

	out, err := c.Invoke(ctx, map[string]any{"text": "hello {{sys.query}}"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "hello world" {
		t.Fatalf("content: got %q, want %q", got, "hello world")
	}
	if len(direct) != 1 || direct[0] != "hello world" {
		t.Fatalf("direct = %#v, want one rendered message", direct)
	}
}

// TestMessage_SkipsEmissionWhenAgentAlreadyStreamed verifies that the
// Message component does not re-emit content when an upstream Agent
// component already streamed its answer. This prevents the double-reply
// bug (Agent → Message): the Agent's model stream reaches the runtime before
// the Message node, and the Message node must not send the same content again.
func TestMessage_SkipsEmissionWhenAgentAlreadyStreamed(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-skip", "task-skip")
	ctx := withStateForTest(context.Background(), state)
	var emitted []string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(contentDelta, thinkingDelta string) {
		if contentDelta != "" {
			emitted = append(emitted, contentDelta)
		}
	})
	var direct []string
	ctx = runtime.WithCanvasMessageEmitter(ctx, func(content string) {
		direct = append(direct, content)
	})

	// Simulate the Agent having already streamed its answer in deltas.
	runtime.EmitAgentMessage(ctx, "agent ", "")
	runtime.EmitAgentMessage(ctx, "streamed this", "")
	emitted = nil // clear setup emission so we only capture Message's output

	out, err := c.Invoke(ctx, map[string]any{"content": "agent streamed this"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	// Content is still set in outputs for state persistence.
	if got, _ := out["content"].(string); got != "agent streamed this" {
		t.Fatalf("content: got %q, want %q", got, "agent streamed this")
	}
	// But no additional SSE emission should occur.
	if len(emitted) != 0 {
		t.Fatalf("emitted = %#v, want empty (Agent already streamed)", emitted)
	}
	if len(direct) != 0 {
		t.Fatalf("direct = %#v, want empty (Agent already streamed)", direct)
	}
}

func TestMessage_EmitsContentDifferentFromAgentStream(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-distinct", "task-distinct")
	ctx := withStateForTest(context.Background(), state)
	var emitted []string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(contentDelta, thinkingDelta string) {
		if contentDelta != "" {
			emitted = append(emitted, contentDelta)
		}
	})
	var direct []string
	ctx = runtime.WithCanvasMessageEmitter(ctx, func(content string) {
		direct = append(direct, content)
	})

	runtime.EmitAgentMessage(ctx, "draft answer", "")
	emitted = nil

	out, err := c.Invoke(ctx, map[string]any{"content": "final answer"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "final answer" {
		t.Fatalf("content: got %q, want %q", got, "final answer")
	}
	if len(emitted) != 0 {
		t.Fatalf("agent fallback emitted = %#v, want empty", emitted)
	}
	if len(direct) != 1 || direct[0] != "final answer" {
		t.Fatalf("direct = %#v, want [final answer]", direct)
	}
}

func TestMessage_ConsumesDeferredAgentStream(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-deferred", "task-deferred")
	state.SetVar("agent_0", "content", &runtime.DeferredStream{
		Open: func(_ context.Context, sink runtime.AgentDeltaSink) (map[string]any, error) {
			sink("hello ", "")
			sink("world", "")
			return map[string]any{"content": "hello world"}, nil
		},
	})
	ctx := withStateForTest(context.Background(), state)
	var emitted []string
	ctx = runtime.WithCanvasMessageEmitter(ctx, func(content string) {
		if content != "" {
			emitted = append(emitted, content)
		}
	})

	out, err := c.Invoke(ctx, map[string]any{"text": "answer: {{agent_0@content}}"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "answer: hello world" {
		t.Fatalf("content: got %q, want %q", got, "answer: hello world")
	}
	if got := strings.Join(emitted, ""); got != "answer: hello world" {
		t.Fatalf("emitted: got %q, want %q (%#v)", got, "answer: hello world", emitted)
	}
}

func TestMessage_DeferredStreamThinkingEvents(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-deferred-thinking", "task-deferred-thinking")
	state.SetVar("agent_0", "content", &runtime.DeferredStream{
		Open: func(_ context.Context, sink runtime.AgentDeltaSink) (map[string]any, error) {
			sink("", "plan")
			sink("answer", "")
			return map[string]any{"content": "answer"}, nil
		},
	})
	ctx := withStateForTest(context.Background(), state)
	var events []string
	ctx = runtime.WithCanvasMessageEventEmitter(ctx, func(content string, startToThink, endToThink bool) {
		switch {
		case startToThink:
			events = append(events, "start")
		case endToThink:
			events = append(events, "end")
		default:
			events = append(events, content)
		}
	})

	if _, err := c.Invoke(ctx, map[string]any{"text": "{{agent_0@content}}"}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := strings.Join(events, "|"), "start|plan|end|answer"; got != want {
		t.Fatalf("events=%q, want %q", got, want)
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
