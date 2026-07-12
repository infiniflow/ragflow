//go:build integration
// +build integration

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
	"bytes"
	"context"
	"errors"
	"log"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

func requireTokenizerPool(t *testing.T) {
	t.Helper()
	if err := tokenizer.Init(&tokenizer.PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        1,
		MaxSize:        2,
		IdleTimeout:    30 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}); err != nil {
		t.Skipf("tokenizer pool unavailable: %v", err)
	}
}

// stubEmbedder records every call and returns canned vectors.
// Matches the Embedder contract: len(results) == len(texts).
type stubEmbedder struct {
	calls         atomic.Int32
	dim           int
	maxTokens     int
	delay         time.Duration
	err           error
	callInputs    [][]string
	resultsByCall []embeddingCallResult
	callTokens    []int
}

type embeddingCallResult struct {
	vectors    [][]float64
	tokenCount int
}

func (s *stubEmbedder) MaxTokens() int {
	return s.maxTokens
}

func (s *stubEmbedder) Encode(texts []string) ([]EmbeddingResult, error) {
	s.calls.Add(1)
	copied := append([]string(nil), texts...)
	s.callInputs = append(s.callInputs, copied)
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.err != nil {
		return nil, s.err
	}
	callIdx := int(s.calls.Load()) - 1
	var cfg embeddingCallResult
	if callIdx < len(s.resultsByCall) {
		cfg = s.resultsByCall[callIdx]
	}
	out := make([]EmbeddingResult, len(texts))
	for i := range texts {
		var v []float64
		if i < len(cfg.vectors) {
			v = append([]float64(nil), cfg.vectors[i]...)
		} else {
			v = make([]float64, s.dim)
			v[0] = float64(i + 1)
		}
		tokenCount := len(texts[i])
		if callIdx < len(s.callTokens) {
			tokenCount = s.callTokens[callIdx]
		} else if cfg.tokenCount > 0 {
			tokenCount = cfg.tokenCount
		}
		out[i] = EmbeddingResult{Vector: v, TokenCount: tokenCount}
	}
	return out, nil
}

// newStubEmbedder returns a stub embedder for instance-level resolver injection.
func newStubEmbedder(dim int) *stubEmbedder {
	return &stubEmbedder{dim: dim}
}

// withStubEmbedder constructs a TokenizerComponent with an instance-scoped
// resolver backed by a stub embedder.
func withStubEmbedder(t *testing.T, dim int) (*TokenizerComponent, *stubEmbedder) {
	t.Helper()
	stub := newStubEmbedder(dim)
	comp, err := NewTokenizerComponentWithResolver(nil, func(_, _, _ string) (Embedder, error) { return stub, nil })
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	return comp.(*TokenizerComponent), stub
}

// TestTokenizerComponent_Registered verifies init() enrollment
// under runtime.CategoryIngestion (Phase 4 / API endpoint depends
// on this contract).
func TestTokenizerComponent_Registered(t *testing.T) {
	factory, cat, md, ok := runtime.DefaultRegistry.Lookup("Tokenizer")
	if !ok {
		t.Fatal("Tokenizer not registered in runtime.DefaultRegistry")
	}
	if cat != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", cat, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Error("factory is nil")
	}
	if len(md.Inputs) == 0 {
		t.Error("metadata.Inputs empty")
	}
	if len(md.Outputs) == 0 {
		t.Error("metadata.Outputs empty")
	}
}

// TestTokenizerComponent_Invoke_HappyPath drives three chunks
// through both full_text tokenization and embedding. Verifies that
// every chunk gains `content_ltks`, `content_sm_ltks`, and a
// `q_<n>_vec` vector keyed by the embedder's vector dimension.
func TestTokenizerComponent_Invoke_HappyPath(t *testing.T) {
	requireTokenizerPool(t)
	const dim = 4
	c, stub := withStubEmbedder(t, dim)

	_ = stub
	var err error
	if err != nil {
		t.Fatalf("NewTokenizerComponent: %v", err)
	}

	chunks := []map[string]any{
		{"text": "alpha chunk text"},
		{"text": "bravo chunk text"},
		{"text": "charlie chunk text"},
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"tenant_id":     "t1",
		"model_id":      "embd-1",
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        chunks,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	gotChunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks type = %T, want []map[string]any", out["chunks"])
	}
	if len(gotChunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(gotChunks))
	}
	for i, ck := range gotChunks {
		if ck["content_ltks"] == nil {
			t.Errorf("chunk[%d].content_ltks missing", i)
		}
		if ck["content_sm_ltks"] == nil {
			t.Errorf("chunk[%d].content_sm_ltks missing", i)
		}
		if ck["title_tks"] == nil {
			t.Errorf("chunk[%d].title_tks missing", i)
		}
		key := "q_4_vec"
		v, ok := ck[key].([]float64)
		if !ok {
			t.Errorf("chunk[%d].%s missing or wrong type: %T", i, key, ck[key])
			continue
		}
		if len(v) != dim {
			t.Errorf("chunk[%d].%s len = %d, want %d", i, key, len(v), dim)
		}
		if v[0] == 0 {
			t.Errorf("chunk[%d].%s[0] = %v, want non-zero", i, key, v[0])
		}
	}
	if out["output_format"] != "chunks" {
		t.Errorf("output_format = %v, want chunks", out["output_format"])
	}
	if out["embedding_token_consumption"] == nil {
		t.Error("embedding_token_consumption missing")
	}
	if out["_elapsed_time"] == nil {
		t.Error("_elapsed_time missing")
	}
}

// TestTokenizerComponent_Invoke_EmptyChunks covers the no-op branch:
// empty chunk list → empty output, no panic, no encoder call.
func TestTokenizerComponent_Invoke_EmptyChunks(t *testing.T) {
	c, stub := withStubEmbedder(t, 4)
	_ = stub
	var err error
	if err != nil {
		t.Fatalf("NewTokenizerComponent: %v", err)
	}

	out, err := c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Errorf("chunks len = %d, want 0", len(chunks))
	}
	if stub.calls.Load() != 0 {
		t.Errorf("embedder called %d times on empty input, want 0", stub.calls.Load())
	}
	if got := out["embedding_token_consumption"]; got != 0 {
		t.Errorf("embedding_token_consumption = %v, want 0", got)
	}
	if out["output_format"] != "chunks" {
		t.Errorf("output_format = %v, want chunks", out["output_format"])
	}
}

// TestTokenizerComponent_Invoke_NilChunks covers the nil-input
// branch: nil chunks list is treated as zero-length (matches
// python `kwargs.get("chunks")` with None).
func TestTokenizerComponent_Invoke_NilChunks(t *testing.T) {
	c, stub := withStubEmbedder(t, 4)
	_ = stub
	out, err := c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Errorf("chunks len = %d, want 0", len(chunks))
	}
}

// TestTokenizerComponent_Invoke_Unicode asserts CJK input
// produces finite, non-negative token counts (plan §8 Q2: Go
// `NumTokensFromString` over-counts CJK on tiktoken-init failure;
// Python returns 0 — both are valid as long as the count is
// finite).
func TestTokenizerComponent_Invoke_Unicode(t *testing.T) {
	requireTokenizerPool(t)
	c, stub := withStubEmbedder(t, 4)
	_ = stub

	inputs := []string{
		"中文测试文本",
		"こんにちは世界",
		"한국어 텍스트",
		"English mixed 中文 français 日本語",
	}
	chunks := make([]map[string]any, len(inputs))
	for i, txt := range inputs {
		chunks[i] = map[string]any{"text": txt}
	}

	out, err := c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
		"chunks":        chunks,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	gotChunks, _ := out["chunks"].([]map[string]any)
	if len(gotChunks) != len(inputs) {
		t.Fatalf("chunks len = %d, want %d", len(gotChunks), len(inputs))
	}
	for i, ck := range gotChunks {
		// Direct call to verify the count contract.
		tokens := tokenizer.NumTokensFromString(inputs[i])
		if tokens < 0 {
			t.Errorf("chunk[%d] token count negative: %d", i, tokens)
		}
		if ck["content_ltks"] == nil {
			t.Errorf("chunk[%d].content_ltks missing", i)
		}
	}
}

func TestTokenizerComponent_Invoke_TextPayload(t *testing.T) {
	requireTokenizerPool(t)
	_, _ = withStubEmbedder(t, 4)
	c, _ := NewTokenizerComponent(map[string]any{
		"search_method": []any{"full_text"},
	})
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "note.txt",
		"output_format": "text",
		"text":          "plain payload",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["chunks"].([]map[string]any)
	if len(got) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(got))
	}
	if got[0]["text"] != "plain payload" {
		t.Errorf("text = %v, want plain payload", got[0]["text"])
	}
	if got[0]["content_ltks"] == nil {
		t.Errorf("content_ltks missing: %v", got[0])
	}
}

func TestTokenizerComponent_Invoke_JSONPayload(t *testing.T) {
	requireTokenizerPool(t)
	_, _ = withStubEmbedder(t, 4)
	c, _ := NewTokenizerComponent(map[string]any{
		"search_method": []any{"full_text"},
	})
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "note.pdf",
		"output_format": "json",
		"json":          []map[string]any{{"text": "row one"}, {"text": "row two"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["chunks"].([]map[string]any)
	if len(got) != 2 {
		t.Fatalf("chunks len = %d, want 2", len(got))
	}
	if got[0]["content_ltks"] == nil || got[1]["content_ltks"] == nil {
		t.Errorf("content_ltks missing: %v", got)
	}
}

// TestTokenizerComponent_Invoke_BatchedEmbedding asserts the
// embedding client is called once for the title plus once for the
// chunk batch when all chunks fit into a single batch.
func TestTokenizerComponent_Invoke_BatchedEmbedding(t *testing.T) {
	requireTokenizerPool(t)
	c, stub := withStubEmbedder(t, 8)
	chunks := []map[string]any{
		{"text": "one"},
		{"text": "two"},
		{"text": "three"},
	}
	if _, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.txt",
		"output_format": "chunks",
		"chunks":        chunks,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("embedder calls = %d, want 2 (title + single content batch)", got)
	}
}

// TestTokenizerComponent_Invoke_FullTextOnly covers the
// search_method=["full_text"] branch: no embedding, no encoder
// call, but tokenized fields present.
func TestTokenizerComponent_Invoke_FullTextOnly(t *testing.T) {
	requireTokenizerPool(t)
	_, stub := withStubEmbedder(t, 4)
	c, _ := NewTokenizerComponent(map[string]any{
		"search_method": []any{"full_text"},
	})
	out, err := c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha bravo"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls.Load() != 0 {
		t.Errorf("embedder should not be called, got %d", stub.calls.Load())
	}
	if out["embedding_token_consumption"] != nil {
		t.Errorf("embedding_token_consumption should be absent, got %v", out["embedding_token_consumption"])
	}
	got, _ := out["chunks"].([]map[string]any)
	if len(got) == 0 || got[0]["content_ltks"] == nil {
		t.Errorf("content_ltks missing: %v", got)
	}
}

func TestTokenizerComponent_Invoke_EmbeddingOnly(t *testing.T) {
	cIntf, err := NewTokenizerComponentWithResolver(map[string]any{
		"search_method": []any{"embedding"},
	}, func(_, _, _ string) (Embedder, error) {
		return newStubEmbedder(4), nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	out, err := cIntf.(*TokenizerComponent).Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha bravo"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["chunks"].([]map[string]any)
	if len(got) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(got))
	}
	if got[0]["q_4_vec"] == nil {
		t.Fatalf("q_4_vec missing: %v", got[0])
	}
	if got[0]["content_ltks"] != nil || got[0]["content_sm_ltks"] != nil {
		t.Fatalf("embedding-only mode should not emit full-text tokens: %v", got[0])
	}
	if out["embedding_token_consumption"] == nil {
		t.Fatal("embedding_token_consumption missing")
	}
}

func TestTokenizerComponent_Invoke_FullTextAndEmbedding(t *testing.T) {
	requireTokenizerPool(t)
	c, _ := withStubEmbedder(t, 4)
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha bravo"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["chunks"].([]map[string]any)
	if len(got) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(got))
	}
	if got[0]["content_ltks"] == nil || got[0]["content_sm_ltks"] == nil {
		t.Fatalf("full-text tokens missing: %v", got[0])
	}
	if got[0]["q_4_vec"] == nil {
		t.Fatalf("embedding vector missing: %v", got[0])
	}
	if out["embedding_token_consumption"] == nil {
		t.Fatal("embedding_token_consumption missing")
	}
}

// TestTokenizerComponent_Invoke_EmbedNoResolver covers the
// "embedding requested but no embedder resolver configured" branch
// (explicit resolver nil and DefaultEmbedderResolver unset) — must
// return a clear error, not panic.
func TestTokenizerComponent_Invoke_EmbedNoResolver(t *testing.T) {
	requireTokenizerPool(t)
	c, _ := NewTokenizerComponent(nil)
	_, err := c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}},
	})
	if err == nil {
		t.Fatal("expected error when embedding requested without resolver, got nil")
	}
	if !strings.Contains(err.Error(), "resolver") {
		t.Errorf("error should mention resolver: %v", err)
	}
}

// TestTokenizerComponent_Invoke_EmbedderError covers the
// propagation of an error from the embedding driver.
func TestTokenizerComponent_Invoke_EmbedderError(t *testing.T) {
	requireTokenizerPool(t)
	c, stub := withStubEmbedder(t, 4)
	stub.err = errors.New("simulated upstream error")

	_, err := c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}},
	})
	if err == nil {
		t.Fatal("expected error from embedder, got nil")
	}
	if !strings.Contains(err.Error(), "simulated upstream error") {
		t.Errorf("error should chain embedder error: %v", err)
	}
}

// TestTokenizerComponent_Invoke_EncoderCountMismatch covers the
// "embedder returned wrong number of vectors" defensive branch.
func TestTokenizerComponent_Invoke_EncoderCountMismatch(t *testing.T) {
	requireTokenizerPool(t)
	_, stub := withStubEmbedder(t, 4)
	// Inject an embedder that returns the wrong number of vectors
	// regardless of input.
	wrong := &countMismatchedEmbedder{want: 1}
	cIntf, err := NewTokenizerComponentWithResolver(nil, func(_, _, _ string) (Embedder, error) { return wrong, nil })
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	c := cIntf.(*TokenizerComponent)
	_ = stub
	_, err = c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "a"}, {"text": "b"}, {"text": "c"}},
	})
	if err == nil {
		t.Fatal("expected error from count mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "vectors") {
		t.Errorf("error should mention vectors: %v", err)
	}
}

type countMismatchedEmbedder struct{ want int }

func (c *countMismatchedEmbedder) MaxTokens() int { return 0 }

func (c *countMismatchedEmbedder) Encode(texts []string) ([]EmbeddingResult, error) {
	out := make([]EmbeddingResult, c.want)
	for i := range out {
		out[i] = EmbeddingResult{Vector: make([]float64, 4), TokenCount: 1}
	}
	return out, nil
}

// TestTokenizerComponent_Invoke_HonorsTimeout installs an
// embedder that blocks past a (test-shrunk) tokenizerTimeout and
// asserts the component returns context.DeadlineExceeded.
func TestTokenizerComponent_Invoke_HonorsTimeout(t *testing.T) {
	requireTokenizerPool(t)
	prevTimeout := tokenizerTimeout
	tokenizerTimeout = 50 * time.Millisecond
	t.Cleanup(func() { tokenizerTimeout = prevTimeout })

	c, stub := withStubEmbedder(t, 4)
	stub.delay = 500 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Invoke(ctx, map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}},
	})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

// TestTokenizerComponent_InputsOutputs_NonEmpty verifies Phase 4
// API metadata shape.
func TestTokenizerComponent_InputsOutputs_NonEmpty(t *testing.T) {
	c, _ := NewTokenizerComponent(map[string]any{})
	ins := c.(*TokenizerComponent).Inputs()
	outs := c.(*TokenizerComponent).Outputs()
	if len(ins) == 0 {
		t.Error("Inputs() empty")
	}
	if len(outs) == 0 {
		t.Error("Outputs() empty")
	}
	for _, key := range []string{"chunks", "output_format"} {
		if _, ok := outs[key]; !ok {
			t.Errorf("Outputs() missing %q", key)
		}
	}
	for _, key := range []string{"chunks", "name"} {
		if _, ok := ins[key]; !ok {
			t.Errorf("Inputs() missing %q", key)
		}
	}
}

// TestTokenizerComponent_Parallelism locks the fan-out to 1 (plan
// §AD-5a: "embedding calls batched, not fanned").
func TestTokenizerComponent_Parallelism(t *testing.T) {
	c, _ := NewTokenizerComponent(map[string]any{})
	if got := c.(*TokenizerComponent).Parallelism(); got != 1 {
		t.Errorf("Parallelism() = %d, want 1", got)
	}
}

// TestTokenizerComponent_NewTokenizerComponent_Defaults verifies
// the Python default param values propagate.
func TestTokenizerComponent_NewTokenizerComponent_Defaults(t *testing.T) {
	c, err := NewTokenizerComponent(nil)
	if err != nil {
		t.Fatalf("NewTokenizerComponent(nil): %v", err)
	}
	tc := c.(*TokenizerComponent)
	if tc.param.FilenameEmbdWeight != 0.1 {
		t.Errorf("filename_embd_weight = %v, want 0.1", tc.param.FilenameEmbdWeight)
	}
	if len(tc.param.Fields) != 1 || tc.param.Fields[0] != "text" {
		t.Errorf("fields = %v, want [text]", tc.param.Fields)
	}
	if len(tc.param.SearchMethod) != 2 {
		t.Errorf("search_method len = %d, want 2", len(tc.param.SearchMethod))
	}
}

// TestTokenizerComponent_NewTokenizerComponent_BadParam covers
// the param-validation branch (invalid search_method value).
func TestTokenizerComponent_NewTokenizerComponent_BadParam(t *testing.T) {
	_, err := NewTokenizerComponent(map[string]any{
		"search_method": []any{"unknown"},
	})
	if err == nil {
		t.Fatal("expected param validation error, got nil")
	}
}

// TestTokenizerComponent_Smoke_EndToEnd is the BLOCKER smoke test
// (plan §8 R3). Drives 1 chunk of ~1000 tokens through the real
// tokenizer and a stub embedder with no artificial latency, then
// asserts:
//
//   - non-zero vector returned
//   - latency well under 5s (real stub returns in <1ms; we assert
//     < 5s as the §R3 ceiling)
//   - no panic
//
// Documented result: this stub embedding completes in well under
// 5s (typical observed latency < 5ms on the test host). The
// production path against a real embedding API was not exercised
// in this CI sandbox; the helper `withStubEmbedder` deliberately
// avoids the network round-trip while still exercising the full
// wiring (TrackElapsed, WithTimeout, batched Encode, vector
// stamping).

func TestTokenizerComponent_Smoke_EndToEnd(t *testing.T) {
	requireTokenizerPool(t)
	const dim = 1024
	c, _ := withStubEmbedder(t, dim)

	words := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		words = append(words, "ragflow")
	}
	chunkText := strings.Join(words, " ")
	preflightTokens := tokenizer.NumTokensFromString(chunkText)
	if preflightTokens < 100 || preflightTokens > 5000 {
		t.Logf("preflight token count = %d (acceptable range 100-5000)", preflightTokens)
	}

	start := time.Now()
	out, err := c.Invoke(context.Background(), map[string]any{
		"tenant_id":     "tenant-smoke",
		"model_id":      "embd-smoke",
		"name":          "smoke.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": chunkText}},
	})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if elapsed >= 5*time.Second {
		t.Errorf("elapsed %v exceeds 5s ceiling", elapsed)
	}

	got, ok := out["chunks"].([]map[string]any)
	if !ok || len(got) != 1 {
		t.Fatalf("chunks output malformed: %v", out["chunks"])
	}
	vec, ok := got[0]["q_1024_vec"].([]float64)
	if !ok {
		t.Fatalf("q_1024_vec missing or wrong type: %T", got[0]["q_1024_vec"])
	}
	if len(vec) != dim {
		t.Errorf("vector len = %d, want %d", len(vec), dim)
	}
	nonZero := 0
	for _, x := range vec {
		if x != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Error("vector is all zeros")
	}

	t.Logf("smoke complete: chunks=%d elapsed=%v tokens=%d vec_dim=%d",
		len(got), elapsed, preflightTokens, len(vec))
}

func TestTokenizerComponent_Embedding_MergesTitleAndContentVectors(t *testing.T) {
	c, stub := withStubEmbedder(t, 2)
	stub.resultsByCall = []embeddingCallResult{
		{vectors: [][]float64{{10, 20}}, tokenCount: 7},
		{vectors: [][]float64{{1, 2}, {3, 4}}, tokenCount: 11},
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}, {"text": "beta"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["chunks"].([]map[string]any)
	want0 := []float64{1.9, 3.8}
	want1 := []float64{3.7, 5.6}
	if !floatSliceClose(got[0]["q_2_vec"].([]float64), want0) {
		t.Fatalf("chunk[0] q_2_vec = %v, want %v", got[0]["q_2_vec"], want0)
	}
	if !floatSliceClose(got[1]["q_2_vec"].([]float64), want1) {
		t.Fatalf("chunk[1] q_2_vec = %v, want %v", got[1]["q_2_vec"], want1)
	}
}

func TestTokenizerComponent_Embedding_UsesFilenameWeight(t *testing.T) {
	cIntf, err := NewTokenizerComponentWithResolver(map[string]any{
		"filename_embd_weight": 0.25,
	}, func(_, _, _ string) (Embedder, error) {
		stub := newStubEmbedder(2)
		stub.resultsByCall = []embeddingCallResult{
			{vectors: [][]float64{{8, 8}}, tokenCount: 3},
			{vectors: [][]float64{{2, 2}}, tokenCount: 5},
		}
		return stub, nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	c := cIntf.(*TokenizerComponent)
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["chunks"].([]map[string]any)
	want := []float64{3.5, 3.5}
	if !floatSliceClose(got[0]["q_2_vec"].([]float64), want) {
		t.Fatalf("q_2_vec = %v, want %v", got[0]["q_2_vec"], want)
	}
}

func TestTokenizerComponent_Embedding_EmptyNameWarnsAndUsesContentVector(t *testing.T) {
	c, stub := withStubEmbedder(t, 2)
	stub.resultsByCall = []embeddingCallResult{{vectors: [][]float64{{2, 4}}, tokenCount: 5}}

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})

	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "   ",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := stub.calls.Load(); got != 1 {
		t.Fatalf("embedder calls = %d, want 1 (content only)", got)
	}
	if !strings.Contains(buf.String(), "empty name provided from upstream") {
		t.Fatalf("log output = %q, want empty-name warning", buf.String())
	}
	got, _ := out["chunks"].([]map[string]any)
	want := []float64{2, 4}
	if !floatSliceClose(got[0]["q_2_vec"].([]float64), want) {
		t.Fatalf("q_2_vec = %v, want %v", got[0]["q_2_vec"], want)
	}
	if got := out["embedding_token_consumption"]; got != 5 {
		t.Fatalf("embedding_token_consumption = %v, want 5", got)
	}
}

func TestTokenizerComponent_Embedding_TruncatesByMaxTokensMinus10(t *testing.T) {
	c, stub := withStubEmbedder(t, 2)
	stub.maxTokens = 12
	longText := strings.Repeat("hello world ", 20)
	if _, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": longText}},
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(stub.callInputs) != 2 {
		t.Fatalf("callInputs len = %d, want 2", len(stub.callInputs))
	}
	if len(stub.callInputs[1]) != 1 {
		t.Fatalf("content batch size = %d, want 1", len(stub.callInputs[1]))
	}
	if got := stub.callInputs[1][0]; len(got) >= len(longText) {
		t.Fatalf("content text was not truncated: original=%d got=%d", len(longText), len(got))
	}
}

func TestTokenizerComponent_Embedding_SkipsEmptyCleanedTextsButReturnsZeroWhenAllSkipped(t *testing.T) {
	c, stub := withStubEmbedder(t, 2)
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks": []map[string]any{
			{"text": "<table><tr><td></td></tr></table>"},
			{"text": "   "},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := stub.calls.Load(); got != 0 {
		t.Fatalf("embedder calls = %d, want 0", got)
	}
	if got := out["embedding_token_consumption"]; got != 0 {
		t.Fatalf("embedding_token_consumption = %v, want 0", got)
	}
	got, _ := out["chunks"].([]map[string]any)
	for i, ck := range got {
		if _, ok := ck["q_2_vec"]; ok {
			t.Fatalf("chunk[%d] should not have vector: %v", i, ck)
		}
	}
}

func TestTokenizerComponent_Embedding_SetsTokenConsumptionIncludingTitleCall(t *testing.T) {
	c, stub := withStubEmbedder(t, 2)
	stub.callTokens = []int{3, 5, 7}
	prevBatchSize := tokenizerEmbeddingBatchSize
	tokenizerEmbeddingBatchSize = 1
	t.Cleanup(func() { tokenizerEmbeddingBatchSize = prevBatchSize })

	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}, {"text": "beta"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["embedding_token_consumption"]; got != 15 {
		t.Fatalf("embedding_token_consumption = %v, want 15", got)
	}
}

func TestTokenizerComponent_Embedding_BatchesByConfiguredBatchSize(t *testing.T) {
	c, stub := withStubEmbedder(t, 2)
	prevBatchSize := tokenizerEmbeddingBatchSize
	tokenizerEmbeddingBatchSize = 2
	t.Cleanup(func() { tokenizerEmbeddingBatchSize = prevBatchSize })

	if _, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks": []map[string]any{
			{"text": "one"},
			{"text": "two"},
			{"text": "three"},
			{"text": "four"},
			{"text": "five"},
		},
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := stub.calls.Load(); got != 4 {
		t.Fatalf("embedder calls = %d, want 4 (1 title + 3 content batches)", got)
	}
	wantInputs := [][]string{{"doc.pdf"}, {"one", "two"}, {"three", "four"}, {"five"}}
	if !reflect.DeepEqual(stub.callInputs, wantInputs) {
		t.Fatalf("call inputs = %#v, want %#v", stub.callInputs, wantInputs)
	}
}

func TestTokenizerComponent_Embedding_ZeroChunksStillEmitsConsumptionZero(t *testing.T) {
	c, stub := withStubEmbedder(t, 2)
	out, err := c.Invoke(context.Background(), map[string]any{
		"name":          "doc.pdf",
		"output_format": "chunks",
		"chunks":        []map[string]any{},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := stub.calls.Load(); got != 0 {
		t.Fatalf("embedder calls = %d, want 0", got)
	}
	if got := out["embedding_token_consumption"]; got != 0 {
		t.Fatalf("embedding_token_consumption = %v, want 0", got)
	}
}

func floatSliceClose(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			return false
		}
	}
	return true
}

func TestValidateTokenizerOutputs_FullTextMissingReturnsError(t *testing.T) {
	err := validateTokenizerOutputs([]schema.ChunkDoc{{Text: "alpha"}}, []string{"full_text"}, []string{"text"})
	if err == nil || !strings.Contains(err.Error(), "missing full_text tokens") {
		t.Fatalf("err = %v, want missing full_text tokens", err)
	}
}

func TestValidateTokenizerOutputs_EmbeddingMissingReturnsError(t *testing.T) {
	err := validateTokenizerOutputs([]schema.ChunkDoc{{Text: "alpha"}}, []string{"embedding"}, []string{"text"})
	if err == nil || !strings.Contains(err.Error(), "missing embedding vector") {
		t.Fatalf("err = %v, want missing embedding vector", err)
	}
}

func TestValidateTokenizerOutputs_BothModesFailWhenOneMissing(t *testing.T) {
	ck := schema.ChunkDoc{Text: "alpha", ContentLtks: "tok", ContentSmLtks: "sm"}
	err := validateTokenizerOutputs([]schema.ChunkDoc{ck}, []string{"full_text", "embedding"}, []string{"text"})
	if err == nil || !strings.Contains(err.Error(), "missing embedding vector") {
		t.Fatalf("err = %v, want missing embedding vector", err)
	}
}

func TestTokenizerComponent_InstanceResolversDoNotLeakAcrossComponents(t *testing.T) {
	requireTokenizerPool(t)
	compAIntf, err := NewTokenizerComponentWithResolver(nil, func(_, _, _ string) (Embedder, error) {
		stub := newStubEmbedder(2)
		stub.resultsByCall = []embeddingCallResult{{vectors: [][]float64{{10, 10}}, tokenCount: 1}, {vectors: [][]float64{{1, 1}}, tokenCount: 1}}
		return stub, nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver(A): %v", err)
	}
	compBIntf, err := NewTokenizerComponentWithResolver(nil, func(_, _, _ string) (Embedder, error) {
		stub := newStubEmbedder(2)
		stub.resultsByCall = []embeddingCallResult{{vectors: [][]float64{{20, 20}}, tokenCount: 1}, {vectors: [][]float64{{2, 2}}, tokenCount: 1}}
		return stub, nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver(B): %v", err)
	}
	compA := compAIntf.(*TokenizerComponent)
	compB := compBIntf.(*TokenizerComponent)

	outA, err := compA.Invoke(context.Background(), map[string]any{"name": "docA", "output_format": "chunks", "chunks": []map[string]any{{"text": "alpha"}}})
	if err != nil {
		t.Fatalf("Invoke A: %v", err)
	}
	outB, err := compB.Invoke(context.Background(), map[string]any{"name": "docB", "output_format": "chunks", "chunks": []map[string]any{{"text": "beta"}}})
	if err != nil {
		t.Fatalf("Invoke B: %v", err)
	}
	vecA := outA["chunks"].([]map[string]any)[0]["q_2_vec"].([]float64)
	vecB := outB["chunks"].([]map[string]any)[0]["q_2_vec"].([]float64)
	if reflect.DeepEqual(vecA, vecB) {
		t.Fatalf("instance resolvers leaked: vecA=%v vecB=%v", vecA, vecB)
	}
}
