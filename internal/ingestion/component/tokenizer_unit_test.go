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

// Unit tests for the Tokenizer component that do NOT depend on the C++ RAG
// Analyzer pool. These run under plain `go test` (no -tags integration).
// Pool-dependent tests live in tokenizer_test.go (//go:build integration).

package component

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

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

func (s *stubEmbedder) Encode(ctx context.Context, texts []string) ([]EmbeddingResult, error) {
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
// stub embedder resolver. The component uses the default search_method
// (["full_text","embedding"]); callers that need a different mode construct
// the component directly via NewTokenizerComponent(NewTokenizerComponentWithResolver).
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

// TestTokenizerComponent_Invoke_EmptyChunks covers the no-op branch:
// empty chunk list -> empty output, no panic, no encoder call.
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

// TestTokenizerComponent_Embedding_ZeroChunksStillEmitsConsumptionZero uses an
// empty chunk list, so tokenizeChunks is a no-op and the C++ pool is not needed.
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

func TestValidateTokenizerOutputs_SymbolOnlyContentLtksIsEmptyFails(t *testing.T) {
	// Simulates a chunk whose Text is a symbol/punctuation character that
	// the C++ RAGAnalyzer tokenizer cannot produce tokens for (e.g. "·", ")", "(").
	// After tokenizeChunks runs, ContentLtks and ContentSmLtks remain empty,
	// and validateTokenizerOutputs must detect this as a failure.
	ck := schema.ChunkDoc{
		Text:          ")",
		ContentLtks:   "",
		ContentSmLtks: "",
	}
	err := validateTokenizerOutputs([]schema.ChunkDoc{ck}, []string{"full_text"}, []string{"text"})
	if err == nil || !strings.Contains(err.Error(), "missing full_text tokens") {
		t.Fatalf("err = %v, want missing full_text tokens", err)
	}
}
