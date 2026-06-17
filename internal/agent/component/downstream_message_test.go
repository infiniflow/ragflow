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
	"time"
)

// TestLLM_Stream_ExposesThinkingAndContentForDownstreamMessage is a
// regression guard for the Downstream Message detect path.
//
// Python's `_invoke_async` returns a `functools.partial` callable
// when it detects a Message component downstream, deferring
// streaming evaluation until the consumer actually pulls from
// the channel. The Go port exposes the streaming surface via the
// goroutine + channel + select pattern; this test pins the
// contract that any LLM `Stream()` consumer (Message component
// or otherwise) can rely on:
//
//  1. The stream emits a chunk with key "content" and key "thinking".
//  2. The stream eventually closes (no leaked goroutines).
//  3. The consumer can read at its own pace — backpressure is bounded
//     by the 16-element channel buffer.
//
// The actual "detect Message downstream" decision is a canvas-scheduler
// concern (it would look at the DAG children of an LLM node). That
// introspection lives in `internal/agent/canvas/` rather than this
// component package. For v1, every LLM Stream() is the same shape
// regardless of downstream topology.
func TestLLM_Stream_ExposesThinkingAndContentForDownstreamMessage(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "streamed answer", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(context.Background(), map[string]any{"user_prompt": "go"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	// Simulate a slow downstream consumer (the Message component
	// template-rendering path) that reads one chunk at a time.
	got := []map[string]any{}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case chunk, open := <-ch:
			if !open {
				if len(got) != 2 {
					t.Fatalf("expected 2 chunks (content + done), got %d: %+v", len(got), got)
				}
				if got[0]["content"] != "streamed answer" {
					t.Errorf("content=%v, want 'streamed answer'", got[0]["content"])
				}
				if got[0]["thinking"] != "" {
					t.Errorf("thinking=%v, want empty (no think chain in v1)", got[0]["thinking"])
				}
				if got[1]["done"] != true {
					t.Errorf("done=%v, want true", got[1]["done"])
				}
				return
			}
			got = append(got, chunk)
		case <-deadline:
			t.Fatal("Stream did not close within 2s")
		}
	}
}
