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
	"testing"
	"time"

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

// TestTokenizerComponent_Invoke_KeywordSplitCJK verifies important_kwd is
// split by the full ASCII+CJK delimiter set, not just ASCII comma. A Chinese
// LLM commonly emits CJK commas/semicolons even when asked for
// "comma-separated"; ASCII-only splitting would leave keywords glued together.
func TestTokenizerComponent_Invoke_KeywordSplitCJK(t *testing.T) {
	requireTokenizerPool(t)
	_, stub := withStubEmbedder(t, 4)
	c, _ := NewTokenizerComponent(map[string]any{
		"search_method": []any{"full_text"},
	})
	out, err := c.Invoke(context.Background(), map[string]any{
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha", "keywords": "kw1，kw2；kw3"}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if stub.calls.Load() != 0 {
		t.Errorf("embedder should not be called in full_text-only mode, got %d", stub.calls.Load())
	}
	got, _ := out["chunks"].([]map[string]any)
	if len(got) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(got))
	}
	kwd, ok := got[0]["important_kwd"].([]string)
	if !ok {
		t.Fatalf("important_kwd should be []string, got %T", got[0]["important_kwd"])
	}
	if len(kwd) != 3 {
		t.Errorf("important_kwd must split CJK delimiters into 3 elements, got %d: %v", len(kwd), kwd)
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

func (c *countMismatchedEmbedder) Encode(ctx context.Context, texts []string) ([]EmbeddingResult, error) {
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
	t.Setenv("COMPONENT_EXEC_TIMEOUT_TOKENIZER", "1")

	c, stub := withStubEmbedder(t, 4)
	stub.delay = 2 * time.Second

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
	requireTokenizerPool(t)
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
	requireTokenizerPool(t)
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
	requireTokenizerPool(t)
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

// Python tokenizer.py:95 passes the raw name to embedding without .strip();
// Go must match — the title embedding must receive the original name, not a
// TrimSpace'd copy. The empty-name guard still uses TrimSpace (mirroring
// Python's `.strip()==""` check at tokenizer.py:200), but the value encoded
// is the raw name.
func TestTokenizerComponent_Embedding_UsesRawNameNotTrimmed(t *testing.T) {
	requireTokenizerPool(t)
	c, stub := withStubEmbedder(t, 2)

	if _, err := c.Invoke(context.Background(), map[string]any{
		"name":          "  report.pdf  ",
		"output_format": "chunks",
		"chunks":        []map[string]any{{"text": "alpha"}},
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(stub.callInputs) < 1 {
		t.Fatalf("callInputs len = %d, want >= 1", len(stub.callInputs))
	}
	// First call is the title embedding; it must receive the raw name with
	// surrounding whitespace preserved, matching Python.
	if got := stub.callInputs[0][0]; got != "  report.pdf  " {
		t.Fatalf("title embedding input = %q, want %q (raw, not trimmed)", got, "  report.pdf  ")
	}
}

func TestTokenizerComponent_Embedding_TruncatesByMaxTokensMinus10(t *testing.T) {
	requireTokenizerPool(t)
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
	requireTokenizerPool(t)
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
	requireTokenizerPool(t)
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
	requireTokenizerPool(t)
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

func TestTokenizeChunks_SymbolOnlyTextFallsBackToRawText(t *testing.T) {
	requireTokenizerPool(t)
	chunks := []schema.ChunkDoc{
		{Text: "·"}, // middle dot · — seen in production chunk[15]
		{Text: ")"},
		{Text: "("},
		{Text: "*"},
	}
	err := tokenizeChunks(chunks, "test", "English")
	if err != nil {
		t.Fatalf("tokenizeChunks: %v", err)
	}
	for i, ck := range chunks {
		t.Logf("chunk[%d]: text=%q content_ltks=%q content_sm_ltks=%q",
			i, ck.Text, ck.ContentLtks, ck.ContentSmLtks)
		// After fix: Tokenize returns empty for symbol-only text,
		// but the fallback sets ContentLtks = raw text.
		if strings.TrimSpace(ck.ContentLtks) == "" {
			t.Errorf("chunk[%d]: expected non-empty ContentLtks (raw text fallback) for %q, got empty",
				i, ck.Text)
		}
	}
}

func TestTokenizeChunks_WhitespaceSummaryShadowsTextBug(t *testing.T) {
	requireTokenizerPool(t)
	chunks := []schema.ChunkDoc{
		{Summary: "   ", Text: "real content here"},
	}
	err := tokenizeChunks(chunks, "test", "English")
	if err != nil {
		t.Fatalf("tokenizeChunks: %v", err)
	}
	// After fix: TrimSpace("   ") is empty, so the Summary branch is skipped.
	// The Text branch is entered and "real content here" is tokenized normally.
	if strings.TrimSpace(chunks[0].ContentLtks) == "" {
		t.Errorf("whitespace Summary should be skipped, Text %q should be tokenized, but ContentLtks is empty",
			chunks[0].Text)
	}
}
