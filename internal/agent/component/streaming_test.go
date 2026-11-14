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
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"

	"gorm.io/gorm"
)

// TestLLM_Stream_HappyPath: Stream emits content + done chunks, channel
// closes.
func TestLLM_Stream_HappyPath(t *testing.T) {
	stub := &stubInvoker{resp: &ChatInvokeResponse{Content: "hello", Model: "echo"}}
	withStubInvoker(t, stub)
	ctx := t.Context()

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(ctx, nil, map[string]any{"user_prompt": "hi"})
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
	ctx := t.Context()

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(ctx, nil, map[string]any{"user_prompt": "hi"})
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
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel

	ch, err := c.Stream(ctx, nil, map[string]any{"user_prompt": "hi"})
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
	ctx := t.Context()

	c := NewLLMComponent(LLMParam{ModelID: "echo"})
	ch, err := c.Stream(ctx, nil, map[string]any{"user_prompt": "hi"})
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

type stubStreamingInvoker struct {
	calls  int32
	deltas []string
	thinks []string
	resp   *ChatInvokeResponse
	err    error
}

func (s *stubStreamingInvoker) Invoke(_ context.Context, _ *gorm.DB, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

func (s *stubStreamingInvoker) Stream(_ context.Context, _ *gorm.DB, _ ChatInvokeRequest, onDelta func(delta string, isThink bool) error) (*ChatInvokeResponse, error) {
	atomic.AddInt32(&s.calls, 1)
	if s.err != nil {
		return nil, s.err
	}
	for _, th := range s.thinks {
		if err := onDelta(th, true); err != nil {
			return nil, err
		}
	}
	for _, d := range s.deltas {
		if err := onDelta(d, false); err != nil {
			return nil, err
		}
	}
	return s.resp, nil
}

func TestLLM_Stream_RealStreamingDeltas(t *testing.T) {
	stub := &stubStreamingInvoker{
		thinks: []string{"think1", "think2"},
		deltas: []string{"hello ", "world"},
		resp:   &ChatInvokeResponse{Content: "hello world", Thinking: "think1think2", Model: "stream-model"},
	}
	withStubInvoker(t, stub)
	ctx := t.Context()

	c := NewLLMComponent(LLMParam{ModelID: "stream-model"})
	ch, err := c.Stream(ctx, nil, map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var got []map[string]any
	for chunk := range ch {
		got = append(got, chunk)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 chunks (2 thinks + 2 deltas + done), got %d: %+v", len(got), got)
	}
	if got[0]["thinking"] != "think1" || got[1]["thinking"] != "think2" {
		t.Errorf("thinking deltas mismatch: %+v", got[:2])
	}
	if got[2]["content"] != "hello " || got[3]["content"] != "world" {
		t.Errorf("content deltas mismatch: %+v", got[2:4])
	}
	if got[4]["done"] != true || got[4]["model"] != "stream-model" {
		t.Errorf("done chunk mismatch: %+v", got[4])
	}
}

func TestLLM_Invoke_DeferredStream(t *testing.T) {
	stub := &stubStreamingInvoker{
		thinks: []string{"thinking "},
		deltas: []string{"hello ", "world"},
		resp:   &ChatInvokeResponse{Content: "hello world", Thinking: "thinking ", Model: "m"},
	}
	withStubInvoker(t, stub)
	ctx := runtime.WithComponentExecutionOptions(t.Context(), runtime.ComponentExecutionOptions{
		DeferLLMToMessage: true,
	})

	c := NewLLMComponent(LLMParam{ModelID: "m"})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	deferred, ok := out["content"].(*runtime.DeferredStream)
	if !ok || deferred == nil {
		t.Fatalf("expected *runtime.DeferredStream, got %T (%+v)", out["content"], out["content"])
	}

	var contentDeltas, thinkingDeltas []string
	sink := func(contentDelta, thinkingDelta string) {
		if contentDelta != "" {
			contentDeltas = append(contentDeltas, contentDelta)
		}
		if thinkingDelta != "" {
			thinkingDeltas = append(thinkingDeltas, thinkingDelta)
		}
	}

	result, err := deferred.Open(t.Context(), sink)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(thinkingDeltas) != 1 || thinkingDeltas[0] != "thinking " {
		t.Errorf("thinking deltas = %+v, want ['thinking ']", thinkingDeltas)
	}
	if len(contentDeltas) != 2 || contentDeltas[0] != "hello " || contentDeltas[1] != "world" {
		t.Errorf("content deltas = %+v, want ['hello ', 'world']", contentDeltas)
	}
	if result["content"] != "hello world" {
		t.Errorf("result content = %v, want hello world", result["content"])
	}

	// Verify idempotency: subsequent Open calls return cached result without re-invoking the model.
	result2, err2 := deferred.Open(t.Context(), nil)
	if err2 != nil {
		t.Fatalf("second Open: %v", err2)
	}
	if result2["content"] != "hello world" {
		t.Errorf("second Open result content = %v, want hello world", result2["content"])
	}
	if stub.calls != 1 {
		t.Errorf("stub.calls = %d, want 1 (must be memoized via sync.Once)", stub.calls)
	}
}

func TestLLM_Invoke_EagerWhenNotDeferred(t *testing.T) {
	stub := &stubStreamingInvoker{
		deltas: []string{"hello ", "world"},
		resp:   &ChatInvokeResponse{Content: "hello world", Model: "m"},
	}
	withStubInvoker(t, stub)
	ctx := t.Context() // DeferLLMToMessage is false

	c := NewLLMComponent(LLMParam{ModelID: "m"})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "hi"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	content, ok := out["content"].(string)
	if !ok || content != "hello world" {
		t.Fatalf("expected content string 'hello world', got %T (%+v)", out["content"], out["content"])
	}
	if runtime.IsDeferredStream(out["content"]) {
		t.Fatal("expected non-deferred output")
	}
}

func TestGenerateStub_Invoke_DeferredStream(t *testing.T) {
	stub := &stubStreamingInvoker{
		deltas: []string{"gen ", "output"},
		resp:   &ChatInvokeResponse{Content: "gen output", Model: "m"},
	}
	withStubInvoker(t, stub)
	ctx := runtime.WithComponentExecutionOptions(t.Context(), runtime.ComponentExecutionOptions{
		DeferLLMToMessage: true,
	})

	g, err := NewGenerateStub(map[string]any{"prompt": "hi", "llm_id": "m"})
	if err != nil {
		t.Fatalf("NewGenerateStub: %v", err)
	}
	out, err := g.Invoke(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	deferred, ok := out["content"].(*runtime.DeferredStream)
	if !ok || deferred == nil {
		t.Fatalf("expected *runtime.DeferredStream, got %T (%+v)", out["content"], out["content"])
	}

	var contentDeltas []string
	sink := func(contentDelta, _ string) {
		if contentDelta != "" {
			contentDeltas = append(contentDeltas, contentDelta)
		}
	}
	result, err := deferred.Open(t.Context(), sink)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(contentDeltas) != 2 || contentDeltas[0] != "gen " || contentDeltas[1] != "output" {
		t.Errorf("content deltas = %+v, want ['gen ', 'output']", contentDeltas)
	}
	if result["content"] != "gen output" {
		t.Errorf("result content = %v, want gen output", result["content"])
	}
}
