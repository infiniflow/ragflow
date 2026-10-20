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

// Citation-grounding tests. The Agent's post-stream grounding
// call reads chunks from state.Retrieval and makes a second LLM
// call to insert [ID:N] tags. The tests inject canned agentRunner
// + ChatInvoker to verify the call shape and the resulting
// outputs["content"] / outputs["grounding_status"].

package component

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
)

// groundingTestInvoker records the chat request and returns a
// canned response. The LLM call shape (system + user messages)
// is the only thing the grounding path cares about, so we record
// the request verbatim and return a fixed string.
type groundingTestInvoker struct {
	mu        sync.Mutex
	lastReq   ChatInvokeRequest
	responses []string
	calls     int
}

type groundingStreamingInvoker struct {
	groundingTestInvoker
	streamCalls int
}

// groundingPartialFailureInvoker streams the first half of the response and
// then returns an error, simulating a mid-stream failure during citation
// grounding.
type groundingPartialFailureInvoker struct {
	groundingTestInvoker
}

func (g *groundingPartialFailureInvoker) Stream(ctx context.Context, db *gorm.DB, req ChatInvokeRequest, onDelta func(string, bool) error) (*ChatInvokeResponse, error) {
	resp, err := g.Invoke(ctx, db, req)
	if err != nil {
		return nil, err
	}
	mid := len(resp.Content) / 2
	if err := onDelta(resp.Content[:mid], false); err != nil {
		return nil, err
	}
	return nil, errors.New("simulated mid-stream grounding failure")
}

func (g *groundingStreamingInvoker) Stream(ctx context.Context, db *gorm.DB, req ChatInvokeRequest, onDelta func(string, bool) error) (*ChatInvokeResponse, error) {
	g.streamCalls++
	resp, err := g.Invoke(ctx, db, req)
	if err != nil {
		return nil, err
	}
	mid := len(resp.Content) / 2
	for _, delta := range []string{resp.Content[:mid], resp.Content[mid:]} {
		if err := onDelta(delta, false); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (g *groundingTestInvoker) Invoke(_ context.Context, _ *gorm.DB, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lastReq = req
	g.calls++
	if len(g.responses) == 0 {
		return &ChatInvokeResponse{Content: "grounded"}, nil
	}
	idx := g.calls - 1
	if idx >= len(g.responses) {
		idx = len(g.responses) - 1
	}
	return &ChatInvokeResponse{Content: g.responses[idx]}, nil
}

// TestGrounding_Applied: Cite=true + state has chunks → second
// LLM call is made and the grounded content replaces the original.
func TestGrounding_Applied(t *testing.T) {
	inv := &groundingStreamingInvoker{groundingTestInvoker: groundingTestInvoker{responses: []string{"grounded answer [ID:0]"}}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "original answer"}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{
		{"id": "0", "content": "the source content"},
	})
	ctx := runtime.WithState(t.Context(), state)
	var streamed string
	var deltas int
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) {
		streamed += content
		deltas++
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "grounded answer [ID:0]"; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	if got := out["grounding_status"]; got != "applied" {
		t.Errorf("grounding_status=%v, want 'applied'", got)
	}
	if inv.calls != 1 {
		t.Errorf("expected 1 chat call, got %d", inv.calls)
	}
	// Grounding is buffered, so the only emitted delta is the final
	// grounded answer.
	if inv.streamCalls != 1 || deltas != 1 || streamed != "grounded answer [ID:0]" {
		t.Errorf("stream calls=%d, deltas=%d, streamed=%q, want 1 / 1 / grounded answer [ID:0]", inv.streamCalls, deltas, streamed)
	}
	// System message should contain the citation prompt + sources block.
	if got := inv.lastReq.Messages[0].Role; got != schema.System {
		t.Errorf("first message role=%v, want System", got)
	}
	if !contains(inv.lastReq.Messages[0].Content, "ID: 0") {
		t.Errorf("system prompt missing source block: %q", inv.lastReq.Messages[0].Content)
	}
}

// TestGrounding_NoChunks: Cite=true but state has no chunks → no
// grounding call, status reflects "no_chunks".
func TestGrounding_NoChunks(t *testing.T) {
	inv := &groundingTestInvoker{}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "answer"}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	// No SetRetrievalChunks — state has no chunks recorded.
	ctx := runtime.WithState(t.Context(), state)

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "answer"; got != want {
		t.Errorf("content=%v, want %v (no grounding should be applied)", got, want)
	}
	if got := out["grounding_status"]; got != "no_chunks" {
		t.Errorf("grounding_status=%v, want 'no_chunks'", got)
	}
	if inv.calls != 0 {
		t.Errorf("expected 0 chat calls, got %d", inv.calls)
	}
}

// TestGrounding_CiteFalse: Cite=false → no grounding call, no
// grounding_status key.
func TestGrounding_CiteFalse(t *testing.T) {
	inv := &groundingTestInvoker{}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "answer"}, nil
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: false})
	out, err := c.Invoke(t.Context(), nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if _, ok := out["grounding_status"]; ok {
		t.Errorf("grounding_status should be absent when Cite=false, got %v", out["grounding_status"])
	}
	if inv.calls != 0 {
		t.Errorf("expected 0 chat calls, got %d", inv.calls)
	}
}

// TestGrounding_LLMError: grounding LLM call fails → original
// content is preserved, status reflects the error.
func TestGrounding_LLMError(t *testing.T) {
	errInv := &errInvoker{err: errors.New("llm down")}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(errInv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "original"}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "x"}})
	ctx := runtime.WithState(t.Context(), state)

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "original"; got != want {
		t.Errorf("content=%v, want %v (original preserved on grounding failure)", got, want)
	}
	got, _ := out["grounding_status"].(string)
	if got == "" || got[:6] != "error:" {
		t.Errorf("grounding_status=%q, want 'error: ...'", got)
	}
}

// TestGrounding_EmptyContent: grounding LLM returns empty
// content → original is preserved.
func TestGrounding_EmptyContent(t *testing.T) {
	inv := &groundingTestInvoker{responses: []string{""}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "original"}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "x"}})
	ctx := runtime.WithState(t.Context(), state)

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "original"; got != want {
		t.Errorf("content=%v, want %v (empty grounding should preserve original)", got, want)
	}
}

func TestGrounding_HeldContentEmittedOnce(t *testing.T) {
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(&groundingStreamingInvoker{groundingTestInvoker: groundingTestInvoker{responses: []string{"[ID:0] answer"}}})
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{
			Role:    schema.Assistant,
			Content: "answer without citations",
		}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "x"}})
	ctx := runtime.WithState(t.Context(), state)
	var contentEvents []string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) {
		if content != "" {
			contentEvents = append(contentEvents, content)
		}
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "[ID:0] answer"; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	joined := strings.Join(contentEvents, "")
	if joined != "[ID:0] answer" {
		t.Errorf("content events=%v, joined=%q, want [ID:0] answer", contentEvents, joined)
	}
}

func TestGrounding_PartialStreamFailure_FallsBack(t *testing.T) {
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(&groundingPartialFailureInvoker{})
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "original"}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "x"}})
	ctx := runtime.WithState(t.Context(), state)
	var emitted []string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) {
		if content != "" {
			emitted = append(emitted, content)
		}
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["content"]; got != "original" {
		t.Errorf("content=%v, want original answer on grounding failure", got)
	}
	got, _ := out["grounding_status"].(string)
	if !strings.HasPrefix(got, "error:") {
		t.Errorf("grounding_status=%q, want 'error: ...'", got)
	}
	if len(emitted) != 1 || emitted[0] != "original" {
		t.Errorf("emitted=%v, want exactly one emission of the original answer", emitted)
	}
}

// TestGrounding_BatchedChunks: a chunk set larger than one batch budget
// is grounded through multiple sequential calls, each feeding its output
// to the next.
func TestGrounding_BatchedChunks(t *testing.T) {
	inv := &groundingStreamingInvoker{groundingTestInvoker: groundingTestInvoker{responses: []string{"batch1", "batch2", "batch3"}}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "original"}, nil
	})

	// 30 chunks x 3000 chars exceeds the 40k batch budget => >=3 batches.
	var chunks []map[string]any
	for i := 0; i < 30; i++ {
		chunks = append(chunks, map[string]any{"id": fmt.Sprintf("%d", i), "content": strings.Repeat("x", 3100)})
	}
	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks(chunks)
	ctx := runtime.WithState(t.Context(), state)

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if inv.calls < 2 {
		t.Fatalf("expected multiple grounding calls, got %d", inv.calls)
	}
	if got := out["content"]; got != "batch3" {
		t.Errorf("content=%v, want last batch output 'batch3'", got)
	}
	if got := out["grounding_status"]; got != "applied" {
		t.Errorf("grounding_status=%v, want 'applied'", got)
	}
}

// TestCitationGroundingHasSubstantiveText: marker-only strings are rejected;
// real answer text (with or without markers) is accepted.
func TestCitationGroundingHasSubstantiveText(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"[ID:0]", false},
		{"[ID:0] [ID:1]", false},
		{"[ID: 0]\n[ID:1]", false},
		{"[ID:simple-0]", false},
		{"[ID:ck-1]", false},
		{"[ID:simple-0] [ID:ck-1]", false},
		{"[ID: simple-0]", false},
		{"[2026]", true},
		{"  [2026]  ", true},
		{"The reimbursement request requires manager approval.", true},
		{"The reimbursement request requires manager approval. [ID:0]", true},
		{"The reimbursement request requires manager approval. [ID:simple-0]", true},
		{"[ID:oops more [link] end", true},
		{"答案[ID:0]", true},
		{"答案[ID:ck-1]", true},
	}
	for _, tc := range cases {
		if got := citationGroundingHasSubstantiveText(tc.in); got != tc.want {
			t.Errorf("citationGroundingHasSubstantiveText(%q)=%v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestGrounding_MarkerOnlyFallsBack: non-streaming grounding that returns
// only [ID:…] markers must keep the first-pass answer (#20613).
func TestGrounding_MarkerOnlyFallsBack(t *testing.T) {
	inv := &groundingTestInvoker{responses: []string{"[ID:0] [ID:1]"}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "The reimbursement request requires manager approval."}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "policy text"}})
	ctx := runtime.WithState(t.Context(), state)

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := "The reimbursement request requires manager approval."
	if got, _ := out["content"].(string); got != want {
		t.Errorf("content=%q, want %q (marker-only grounding must not replace original)", got, want)
	}
	if got := out["grounding_status"]; got == "applied" {
		t.Errorf("grounding_status=%v, want not applied for marker-only response", got)
	}
	if inv.calls != 1 {
		t.Errorf("expected 1 grounding call, got %d", inv.calls)
	}
}

// TestGrounding_MarkerOnlyStreamFallsBack: grounding deltas are buffered,
// so a marker-only stream never reaches the client and the original answer
// is emitted instead (#20613).
func TestGrounding_MarkerOnlyStreamFallsBack(t *testing.T) {
	inv := &groundingStreamingInvoker{groundingTestInvoker: groundingTestInvoker{responses: []string{"[ID:0] [ID:1]"}}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "The reimbursement request requires manager approval."}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "policy text"}})
	ctx := runtime.WithState(t.Context(), state)
	var streamed string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) {
		streamed += content
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := "The reimbursement request requires manager approval."
	if got, _ := out["content"].(string); got != want {
		t.Errorf("content=%q, want %q (marker-only grounding must not replace original)", got, want)
	}
	if got := out["grounding_status"]; got == "applied" {
		t.Errorf("grounding_status=%v, want not applied for marker-only stream", got)
	}
	if streamed != want {
		t.Errorf("streamed=%q, want the buffered original answer %q", streamed, want)
	}
	if inv.streamCalls != 1 {
		t.Errorf("expected 1 stream call, got %d", inv.streamCalls)
	}
}

// TestGrounding_NonNumericMarkerOnlyFallsBack: retrieval chunk IDs may be
// non-numeric (e.g. simple-0). Marker-only grounding with those IDs must still
// keep the first-pass answer (#20685 review).
func TestGrounding_NonNumericMarkerOnlyFallsBack(t *testing.T) {
	inv := &groundingTestInvoker{responses: []string{"[ID:simple-0]"}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "The reimbursement request requires manager approval."}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "simple-0", "content": "policy text"}})
	ctx := runtime.WithState(t.Context(), state)

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := "The reimbursement request requires manager approval."
	if got, _ := out["content"].(string); got != want {
		t.Errorf("content=%q, want %q (non-numeric marker-only grounding must not replace original)", got, want)
	}
	if got := out["grounding_status"]; got == "applied" {
		t.Errorf("grounding_status=%v, want not applied for marker-only response", got)
	}
	if inv.calls != 1 {
		t.Errorf("expected 1 grounding call, got %d", inv.calls)
	}
}

// TestGrounding_NonNumericMarkerOnlyStreamFallsBack: streamed marker-only
// responses with non-numeric IDs fall back like numeric ones.
func TestGrounding_NonNumericMarkerOnlyStreamFallsBack(t *testing.T) {
	inv := &groundingStreamingInvoker{groundingTestInvoker: groundingTestInvoker{responses: []string{"[ID:simple-0] [ID:ck-1]"}}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "The reimbursement request requires manager approval."}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{
		{"id": "simple-0", "content": "policy text"},
		{"id": "ck-1", "content": "more policy"},
	})
	ctx := runtime.WithState(t.Context(), state)
	var streamed string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) {
		streamed += content
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := "The reimbursement request requires manager approval."
	if got, _ := out["content"].(string); got != want {
		t.Errorf("content=%q, want %q (non-numeric marker-only grounding must not replace original)", got, want)
	}
	if got := out["grounding_status"]; got == "applied" {
		t.Errorf("grounding_status=%v, want not applied for non-numeric marker-only stream", got)
	}
	if streamed != want {
		t.Errorf("streamed=%q, want the buffered original answer %q", streamed, want)
	}
	if inv.streamCalls != 1 {
		t.Errorf("expected 1 stream call, got %d", inv.streamCalls)
	}
}

// TestGrounding_MarkersWithTextStillApplied: a grounded answer that keeps
// substantive text plus markers is still accepted.
func TestGrounding_MarkersWithTextStillApplied(t *testing.T) {
	inv := &groundingTestInvoker{responses: []string{"The reimbursement request requires manager approval. [ID:0]"}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "The reimbursement request requires manager approval."}, nil
	})

	state := canvas.NewCanvasState("r1", "t1")
	state.SetRetrievalChunks([]map[string]any{{"id": "0", "content": "policy text"}})
	ctx := runtime.WithState(t.Context(), state)

	c := NewAgentComponent(AgentParam{ModelID: "stub", Cite: true})
	out, err := c.Invoke(ctx, nil, map[string]any{"user_prompt": "q"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := "The reimbursement request requires manager approval. [ID:0]"
	if got, _ := out["content"].(string); got != want {
		t.Errorf("content=%q, want %q", got, want)
	}
	if got := out["grounding_status"]; got != "applied" {
		t.Errorf("grounding_status=%v, want applied", got)
	}
}

type errInvoker struct {
	err error
}

func (e *errInvoker) Invoke(_ context.Context, _ *gorm.DB, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
	return nil, e.err
}

// contains is a tiny strings.Contains alias kept local to avoid an
// extra import in this single-use case.
func contains(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
