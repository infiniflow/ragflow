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
)

// TestMessage_ResolveTemplate asserts the canonical {{sys.x}} substitution
// flow: a state with sys.query="world" and a template "hello {{sys.query}}"
// must resolve to "hello world". streamed_chunks must NOT be set when
// stream=false.
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
		t.Errorf("streamed_chunks must not be present when stream=false, got %v", out["streamed_chunks"])
	}
}

// TestMessage_Stream confirms the Stream() contract: the returned channel
// receives exactly one payload (the resolved content + streamed_chunks=1)
// and then closes. Phase 2.5 may split into multiple chunks; P0 ships one.
func TestMessage_Stream(t *testing.T) {
	c, _ := NewMessageComponent(nil)
	state := canvas.NewCanvasState("run-2", "task-2")
	state.Sys["query"] = "alice"
	ctx := withStateForTest(context.Background(), state)

	ch, err := c.Stream(ctx, map[string]any{
		"text":   "hi {{sys.query}}",
		"stream": true,
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	ev, ok := <-ch
	if !ok {
		t.Fatal("expected at least one chunk on the channel, got none (channel closed early)")
	}
	// Channel must close after the single chunk.
	if _, open := <-ch; open {
		t.Error("Stream channel should close after one chunk; got additional payload")
	}
	if got, _ := ev["content"].(string); got != "hi alice" {
		t.Errorf("chunk[content]: got %q, want %q", got, "hi alice")
	}
	if n, _ := ev["streamed_chunks"].(int); n != 1 {
		t.Errorf("chunk[streamed_chunks]: got %v, want 1", ev["streamed_chunks"])
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

// withStateForTest is a thin alias for canvas.WithState kept for
// readability at the test call sites (also defined in begin_test.go).
// Same package → same symbol; defining it twice is a compile error,
// so the canonical declaration lives in begin_test.go.
