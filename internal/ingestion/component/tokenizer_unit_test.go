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
	"encoding/json"
	"os"
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
// maxTokens defaults to 2048 so truncateForEmbedding truncates to the first
// 2048 tokens; tests that exercise truncation set maxTokens explicitly.
func newStubEmbedder(dim int) *stubEmbedder {
	return &stubEmbedder{dim: dim, maxTokens: 2048}
}

// withStubEmbedder constructs a TokenizerComponent with an instance-scoped
// stub embedder resolver. The component uses the default search_method
// (["full_text","embedding"]); callers that need a different mode construct
// the component directly via NewTokenizerComponent(NewTokenizerComponentWithResolver).
func withStubEmbedder(t *testing.T, dim int) (*TokenizerComponent, *stubEmbedder) {
	t.Helper()
	stub := newStubEmbedder(dim)
	comp, err := NewTokenizerComponentWithResolver(nil, func(ctx context.Context, _, _, _ string) (Embedder, error) { return stub, nil })
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

	out, err := c.Invoke(context.Background(), nil, map[string]any{
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
	out, err := c.Invoke(context.Background(), nil, map[string]any{
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
	}, func(ctx context.Context, _, _, _ string) (Embedder, error) {
		return newStubEmbedder(4), nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	out, err := cIntf.(*TokenizerComponent).Invoke(context.Background(), nil, map[string]any{
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
	out, err := c.Invoke(context.Background(), nil, map[string]any{
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

// TestChunkDocsToMaps_PreservesPDFPositions is the pool-free unit test for
// Tokenizer-(T)1: the tokenizer emits chunks via schema.ChunkDocsToMaps
// (ChunkDoc.ToMap), which must carry the raw `positions` / `_pdf_positions`
// through untouched so the downstream executor stage
// (internal/ingestion/task processChunkPositions → AddPositions) can convert
// them into position_int / page_num_int / top_int exactly once. This does NOT
// require the C++ analyzer pool, so it runs under plain `go test`.
func TestChunkDocsToMaps_PreservesPDFPositions(t *testing.T) {
	pos := json.RawMessage(`[[1,10,20,30,40],[2,15,25,35,45]]`)
	chunks := []schema.ChunkDoc{
		{Text: "PDF paragraph", DocType: "text", CKType: "text",
			Positions: pos, PDFPositions: pos},
	}
	maps := schema.ChunkDocsToMaps(chunks)

	got, ok := maps[0]["positions"].([][]float64)
	if !ok || len(got) != 2 {
		t.Fatalf("positions not preserved through tokenizer output mapping: %#v", maps[0]["positions"])
	}
	if _, ok := maps[0]["_pdf_positions"].([][]float64); !ok {
		t.Errorf("_pdf_positions not preserved through tokenizer output mapping: %#v", maps[0]["_pdf_positions"])
	}
	// Sanity: page numbers are still raw 1-indexed, i.e. not yet converted
	// to page_num_int (the executor owns that step).
	if int(got[0][0]) != 1 {
		t.Errorf("positions page already converted; want raw 1-indexed page 1, got %v", got[0][0])
	}
}

// TestIsPhantomChunk verifies that zero-value ChunkDocs (no Text, no
// Image, no ContentWithWeight, no Summary) are identified as phantom,
// while any one of those fields being present keeps the chunk.
func TestIsPhantomChunk(t *testing.T) {
	if !isPhantomChunk(schema.ChunkDoc{}) {
		t.Error("empty ChunkDoc must be phantom")
	}
	if !isPhantomChunk(schema.ChunkDoc{Text: ""}) {
		t.Error("ChunkDoc with empty Text only must be phantom")
	}
	if isPhantomChunk(schema.ChunkDoc{Text: "hello"}) {
		t.Error("ChunkDoc with Text must not be phantom")
	}
	if isPhantomChunk(schema.ChunkDoc{Image: "data:image/png;base64,abc"}) {
		t.Error("ChunkDoc with Image must not be phantom")
	}
	if isPhantomChunk(schema.ChunkDoc{ContentWithWeight: "weight"}) {
		t.Error("ChunkDoc with ContentWithWeight must not be phantom")
	}
	if isPhantomChunk(schema.ChunkDoc{Summary: "a summary"}) {
		t.Error("ChunkDoc with Summary must not be phantom")
	}
}

// TestTruncateForEmbedding_SmallMaxTokens covers Tokenizer Diff-14. For any
// positive maxTokens, truncateForEmbedding keeps the first maxTokens tokens
// (non-empty) and the result is strictly shorter than the input.
//
// The unconfigured case (maxTokens <= 0) is covered separately by
// TestTruncateForEmbedding_UnconfiguredClampsToDefault: rather than mirroring
// Python's `truncate` (which returns "" for max_len <= 0 and would make the
// embeddings API reject the batch), Go clamps the limit to a safe default so
// every path still truncates instead of passing the full text through.
func TestTruncateForEmbedding_SmallMaxTokens(t *testing.T) {
	// Long enough to produce well over 50 tokens, so 5/10/50 are all clearly
	// below the total and truncation is observable.
	long := strings.Repeat("a", 2000)
	if got := truncateForEmbedding(long, 5); got == "" {
		t.Error("truncateForEmbedding(maxTokens=5) returned empty, want first 5 tokens")
	}
	if got := truncateForEmbedding(long, 10); got == "" {
		t.Error("truncateForEmbedding(maxTokens=10) returned empty, want first 10 tokens")
	}
	// Normal path: maxTokens > 10 should truncate (not return empty).
	if got := truncateForEmbedding(long, 50); got == "" {
		t.Error("truncateForEmbedding(maxTokens=50) returned empty, want truncated text")
	}
	if got := truncateForEmbedding(long, 50); len(got) >= len(long) {
		t.Errorf("truncateForEmbedding(maxTokens=50) len = %d, want strictly shorter than %d", len(got), len(long))
	}
}

// TestTruncateForEmbedding_UnconfiguredClampsToDefault is the regression gate
// for the root cause behind embedding truncation: an embedder that reports no
// token limit (maxTokens <= 0) must NOT be passed through verbatim. Before the
// central clamp, the generic model_service branch left maxTokens = 0, which
// silently disabled truncation for the whole generic path. The fix clamps
// maxTokens <= 0 to defaultEmbeddingTokenLimit (8192) inside
// truncateForEmbedding itself, so every caller — Builtin and generic alike —
// keeps truncation active.
//
// For a clearly-over-limit input and maxTokens = 0, the result must be non-empty
// AND strictly shorter than the input: proof that it was truncated to the
// default, not returned verbatim.
func TestTruncateForEmbedding_UnconfiguredClampsToDefault(t *testing.T) {
	// ~10000+ CL100K tokens, comfortably above the 8192 default so truncation
	// is observable.
	long := strings.Repeat("hello world ", 5000)
	got := truncateForEmbedding(long, 0)
	if got == "" {
		t.Fatal("truncateForEmbedding(maxTokens=0) returned empty; clamp must keep truncation active")
	}
	if len(got) >= len(long) {
		t.Errorf("truncateForEmbedding(maxTokens=0) len = %d, want strictly shorter than %d; clamp to default must truncate",
			len(got), len(long))
	}
}

// TestEmbeddingBatchSizeEnvVar covers Tokenizer Omission-3: the batch size
// must be configurable via TOKENIZER_EMBEDDING_BATCH_SIZE env var, matching
// Python's configurable settings.EMBEDDING_BATCH_SIZE.
func TestEmbeddingBatchSizeEnvVar(t *testing.T) {
	if got := embeddingBatchSize(); got != 16 {
		t.Errorf("embeddingBatchSize() default = %d, want 16", got)
	}
	os.Setenv("TOKENIZER_EMBEDDING_BATCH_SIZE", "32")
	t.Cleanup(func() { os.Unsetenv("TOKENIZER_EMBEDDING_BATCH_SIZE") })
	if got := embeddingBatchSize(); got != 32 {
		t.Errorf("embeddingBatchSize() after env = %d, want 32", got)
	}
	// Invalid value falls back to default.
	os.Setenv("TOKENIZER_EMBEDDING_BATCH_SIZE", "bad")
	if got := embeddingBatchSize(); got != 16 {
		t.Errorf("embeddingBatchSize() invalid env = %d, want 16", got)
	}
}

// TestChunkOrderInt_EmbeddingOnly covers Tokenizer Diff-8: chunk_order_int
// must be set even when search_method does not include "full_text" (i.e.
// embedding-only path). tokenizeChunks previously only set it for the
// full_text branch.
func TestChunkOrderInt_EmbeddingOnly(t *testing.T) {
	stub := newStubEmbedder(3)
	comp, err := NewTokenizerComponentWithResolver(
		map[string]any{"search_method": []string{"embedding"}, "fields": []string{"text"}},
		func(ctx context.Context, _, _, _ string) (Embedder, error) { return stub, nil },
	)
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	inputs := map[string]any{
		"name":          "doc.pdf",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "first chunk", "doc_type_kwd": "text"},
			{"text": "second chunk", "doc_type_kwd": "text"},
		},
	}
	out, err := comp.Invoke(context.Background(), nil, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}
	for i, ck := range chunks {
		coi, ok := ck["chunk_order_int"]
		if !ok {
			t.Errorf("chunk %d: chunk_order_int missing (embedding-only path must set it)", i)
		}
		if coi == nil {
			t.Errorf("chunk %d: chunk_order_int is nil", i)
		}
	}
}

// TestChunksFromTokenizerUpstream_FiltersPhantomChunks covers Tokenizer
// Omission-2 at the pipeline level: when upstream input contains a
// zero-value ChunkDoc (no Text, no Image, no ContentWithWeight), it must
// be silently dropped from the output before tokenization and embedding.
// This mirrors Python's `if not text and not d.get("image"): continue`
// in tokenizer.py:80-82.
func TestChunksFromTokenizerUpstream_FiltersPhantomChunks(t *testing.T) {
	// JSON path: three items, the middle one is a phantom.
	items := []map[string]any{
		{"text": "valid chunk", "doc_type_kwd": "text"},
		{}, // phantom: no text, no image, no content_with_weight
		{"text": "another valid", "doc_type_kwd": "text"},
	}
	// Use embedding-only mode to avoid CGo tokenizer dependency.
	stub := newStubEmbedder(3)
	comp, err := NewTokenizerComponentWithResolver(
		map[string]any{"search_method": []string{"embedding"}, "fields": []string{"text"}},
		func(ctx context.Context, _, _, _ string) (Embedder, error) { return stub, nil },
	)
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	out, err := comp.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.pdf",
		"output_format": "json",
		"json":          items,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := out["chunks"].([]map[string]any)
	// Must drop the phantom — only 2 valid chunks remain.
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks (phantom filtered), got %d", len(chunks))
	}
	// Verify the surviving chunks are the valid ones.
	if chunks[0]["text"] != "valid chunk" {
		t.Errorf("chunk 0 text = %q, want %q", chunks[0]["text"], "valid chunk")
	}
	if chunks[1]["text"] != "another valid" {
		t.Errorf("chunk 1 text = %q, want %q", chunks[1]["text"], "another valid")
	}
}
