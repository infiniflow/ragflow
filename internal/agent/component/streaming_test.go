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

// TestLLM_Stream_HappyPath: Stream emits content + done chunks, channel
// closes.
func TestLLM_Stream_HappyPath(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "hello", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(context.Background(), map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 chunks (content + done), got %d: %+v", len(got), got)
	}
	// First chunk: content with empty thinking
	if got[0]["content"] != "hello" {
		t.Errorf("chunk[0].content=%v, want hello", got[0]["content"])
	}
	if got[0]["thinking"] != "" {
		t.Errorf("chunk[0].thinking=%v, want empty", got[0]["thinking"])
	}
	// Second chunk: done
	if got[1]["done"] != true {
		t.Errorf("chunk[1].done=%v, want true", got[1]["done"])
	}
	if got[1]["model"] != "echo" {
		t.Errorf("chunk[1].model=%v, want echo", got[1]["model"])
	}
}

// TestLLM_Stream_Error: when Invoke returns an error, Stream emits an
// error chunk and closes.
func TestLLM_Stream_Error(t *testing.T) {
	stub := &stubInvoker{err: context.DeadlineExceeded}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(context.Background(), map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 error chunk, got %d: %+v", len(got), got)
	}
	if _, ok := got[0]["error"]; !ok {
		t.Errorf("error chunk missing 'error' key; got: %+v", got[0])
	}
}

// TestLLM_Stream_RespectsCancellation: pre-cancelled context causes
// Stream to exit before producing chunks.
func TestLLM_Stream_RespectsCancellation(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "hi", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	ch, err := c.Stream(ctx, map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	// No chunks should be received (cancel was already triggered).
	if len(got) != 0 {
		t.Errorf("expected 0 chunks after pre-cancel, got %d: %+v", len(got), got)
	}
}

// TestLLM_Stream_BufferDoesNotBlock: the channel buffer is large
// enough that the producer doesn't block on a slow consumer for the
// small N of chunks this v1 emits. (A real streaming integration
// would need to handle backpressure more carefully; for v1 the
// buffer of 16 is way more than the 1-2 chunks per call.)
func TestLLM_Stream_BufferDoesNotBlock(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "ok", Model: "echo"}}
	withStubInvoker(t, stub)

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(context.Background(), map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	// Drain with a timeout so the test fails loudly if the producer
	// deadlocks. Each chunk should arrive within milliseconds.
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not drain within 2s — likely deadlock")
	}
}
