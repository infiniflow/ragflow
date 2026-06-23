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

// TestMessage_Stream confirms the Stream() contract: the returned
// channel receives exactly one payload (the resolved content +
// streamed_chunks=1) and then closes.
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
	// Chunked streaming. "hi" has no sentence boundary, so we
	// expect exactly one content chunk plus the done marker.
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks (content + done), got %d: %+v", len(got), got)
	}
	if got[0]["content"] != "hi" {
		t.Errorf("chunk[0].content=%q, want 'hi'", got[0]["content"])
	}
	if got[1]["done"] != true {
		t.Errorf("chunk[1].done=%v, want true", got[1]["done"])
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
