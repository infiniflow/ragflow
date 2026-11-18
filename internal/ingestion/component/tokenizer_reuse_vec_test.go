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
	"encoding/json"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

// mustMarshal marshals v to json.RawMessage, panicking on error.
func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// makeChunkWithExtra creates a ChunkDoc with the given extra key-value pairs.
func makeChunkWithExtra(text string, extra map[string]any) schema.ChunkDoc {
	ck := schema.ChunkDoc{Text: text}
	if len(extra) > 0 {
		ck.Extra = make(map[string]json.RawMessage)
		for k, v := range extra {
			ck.Extra[k] = mustMarshal(v)
		}
	}
	return ck
}

func strPtr(s string) *string { return &s }

// TestExtractCompilerVector_ValidProvenance verifies that vectors are only
// extracted when the chunk carries both a q_<dim>_vec and a _compiler_embd_id
// marker (#22134).
func TestExtractCompilerVector_ValidProvenance(t *testing.T) {
	embdID := "model-abc-123"
	vec := []float64{0.1, 0.2, 0.3}

	ck := makeChunkWithExtra("test", map[string]any{
		"id":                "chunk-1",
		"q_3_vec":           vec,
		"_compiler_embd_id": embdID,
	})
	gotVec, gotID := extractCompilerVector(ck)
	if gotID != embdID {
		t.Errorf("expected embd_id %q, got %q", embdID, gotID)
	}
	if len(gotVec) != len(vec) {
		t.Errorf("expected vec length %d, got %d", len(vec), len(gotVec))
	}
	for i := range vec {
		if gotVec[i] != vec[i] {
			t.Errorf("vec[%d] = %f, want %f", i, gotVec[i], vec[i])
		}
	}
}

// TestExtractCompilerVector_NoProvenance verifies that chunks without
// _compiler_embd_id marker return nil even if they have a q_*_vec.
func TestExtractCompilerVector_NoProvenance(t *testing.T) {
	ck := makeChunkWithExtra("test", map[string]any{
		"id":      "chunk-1",
		"q_3_vec": []float64{0.1, 0.2, 0.3},
	})
	gotVec, gotID := extractCompilerVector(ck)
	if gotVec != nil || gotID != "" {
		t.Errorf("expected (nil, \"\"), got (%v, %q)", gotVec, gotID)
	}
}

// TestExtractCompilerVector_MismatchedDimension verifies that vectors whose
// length does not match the key's declared dimension are rejected.
func TestExtractCompilerVector_MismatchedDimension(t *testing.T) {
	ck := makeChunkWithExtra("test", map[string]any{
		"id":                "chunk-1",
		"q_5_vec":           []float64{0.1, 0.2, 0.3}, // dim=5 but len=3
		"_compiler_embd_id": "model-abc",
	})
	gotVec, gotID := extractCompilerVector(ck)
	if gotVec != nil || gotID != "" {
		t.Errorf("expected (nil, \"\") for mismatched dimension, got (%v, %q)", gotVec, gotID)
	}
}

// TestExtractCompilerVector_MultipleVectors verifies that chunks with multiple
// q_*_vec keys are rejected as ambiguous.
func TestExtractCompilerVector_MultipleVectors(t *testing.T) {
	ck := makeChunkWithExtra("test", map[string]any{
		"id":                "chunk-1",
		"q_3_vec":           []float64{0.1, 0.2, 0.3},
		"q_4_vec":           []float64{0.1, 0.2, 0.3, 0.4},
		"_compiler_embd_id": "model-abc",
	})
	gotVec, gotID := extractCompilerVector(ck)
	if gotVec != nil || gotID != "" {
		t.Errorf("expected (nil, \"\") for multiple vectors (ambiguous), got (%v, %q)", gotVec, gotID)
	}
}

// TestExtractCompilerVector_ZeroVector verifies that all-zero vectors are
// rejected.
func TestExtractCompilerVector_ZeroVector(t *testing.T) {
	ck := makeChunkWithExtra("test", map[string]any{
		"id":                "chunk-1",
		"q_3_vec":           []float64{0, 0, 0},
		"_compiler_embd_id": "model-abc",
	})
	gotVec, gotID := extractCompilerVector(ck)
	if gotVec != nil || gotID != "" {
		t.Errorf("expected (nil, \"\") for zero vector, got (%v, %q)", gotVec, gotID)
	}
}

// TestExtractCompilerVector_NilExtra verifies that chunks with nil Extra
// are handled gracefully (no panic).
func TestExtractCompilerVector_NilExtra(t *testing.T) {
	ck := schema.ChunkDoc{Text: "test"}
	gotVec, gotID := extractCompilerVector(ck)
	if gotVec != nil || gotID != "" {
		t.Errorf("expected (nil, \"\") for nil Extra, got (%v, %q)", gotVec, gotID)
	}
}

// TestCanReuseCompilerVector_FieldsMustBeText verifies that reuse is only
// allowed when Fields is exactly ["text"].
func TestCanReuseCompilerVector_FieldsMustBeText(t *testing.T) {
	ck := schema.ChunkDoc{Text: "test"}

	if !canReuseCompilerVector([]string{"text"}, ck) {
		t.Error("expected reuse allowed for Fields=[\"text\"]")
	}
	if canReuseCompilerVector([]string{"questions"}, ck) {
		t.Error("expected reuse disallowed for Fields=[\"questions\"]")
	}
	if canReuseCompilerVector([]string{"text", "questions"}, ck) {
		t.Error("expected reuse disallowed for Fields=[\"text\", \"questions\"]")
	}
	if canReuseCompilerVector([]string{}, ck) {
		t.Error("expected reuse disallowed for empty Fields")
	}
}

// TestCanReuseCompilerVector_NoContextWindows verifies that reuse is
// disallowed when the chunk has ContextAbove or ContextBelow.
func TestCanReuseCompilerVector_NoContextWindows(t *testing.T) {
	ck := schema.ChunkDoc{Text: "test"}
	if !canReuseCompilerVector([]string{"text"}, ck) {
		t.Error("expected reuse allowed when no context")
	}

	ck.ContextAbove = "previous paragraph"
	if canReuseCompilerVector([]string{"text"}, ck) {
		t.Error("expected reuse disallowed when ContextAbove present")
	}

	ck = schema.ChunkDoc{Text: "test", ContextBelow: "next paragraph"}
	if canReuseCompilerVector([]string{"text"}, ck) {
		t.Error("expected reuse disallowed when ContextBelow present")
	}
}

// TestCanReuseCompilerVector_NilExtra verifies that canReuseCompilerVector
// handles chunks with nil Extra gracefully.
func TestCanReuseCompilerVector_NilExtra(t *testing.T) {
	ck := schema.ChunkDoc{Text: "test"}
	if !canReuseCompilerVector([]string{"text"}, ck) {
		t.Error("expected reuse allowed for simple chunk with nil Extra")
	}
}

// TestClearCompilerProvenance verifies that the _compiler_embd_id marker is
// removed after title weighting, preventing re-reuse of weighted vectors.
func TestClearCompilerProvenance(t *testing.T) {
	ck := makeChunkWithExtra("test", map[string]any{
		"id":                "chunk-1",
		"q_3_vec":           []float64{0.1, 0.2, 0.3},
		"_compiler_embd_id": "model-abc",
	})
	if _, ok := ck.Extra["_compiler_embd_id"]; !ok {
		t.Fatal("marker should exist before clear")
	}
	clearCompilerProvenance(ck)
	if _, ok := ck.Extra["_compiler_embd_id"]; ok {
		t.Error("marker should be removed after clear")
	}
	if _, ok := ck.Extra["q_3_vec"]; !ok {
		t.Error("vector should not be removed")
	}
}

// TestTokenizerReusesCompilerVector tests that the Tokenizer reuses vectors
// from Compiler when the embedding model matches and asserts the merged
// title-weighted values (#22134).
func TestTokenizerReusesCompilerVector(t *testing.T) {
	const embdID = "test-embedding-model"
	const dim = 3
	const titleWeight = 0.1

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text"}
	comp.param.FilenameEmbdWeight = titleWeight

	titleVec := []float64{1.0, 2.0, 3.0}
	freshVec := []float64{9.0, 8.0, 7.0}
	stub.resultsByCall = []embeddingCallResult{
		{vectors: [][]float64{titleVec}}, // title encode
		{vectors: [][]float64{freshVec}}, // fresh chunk content
	}

	compilerVec := []float64{0.5, 0.6, 0.7}
	chunks := []schema.ChunkDoc{
		makeChunkWithExtra("compiler chunk", map[string]any{
			"id":                "chunk-compiler",
			"compile_kwd":       "mindmap_entity",
			"q_3_vec":           compilerVec,
			"_compiler_embd_id": embdID,
		}),
		makeChunkWithExtra("fresh chunk", map[string]any{
			"id": "chunk-fresh",
		}),
	}

	result, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}

	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (title + fresh chunk), got %d", got)
	}

	wantMerged, err := mergeEmbeddingVectors(titleVec, compilerVec, titleWeight)
	if err != nil {
		t.Fatalf("mergeEmbeddingVectors: %v", err)
	}
	gotMerged := vecOf(t, result[0], "q_3_vec")
	if len(gotMerged) != len(wantMerged) {
		t.Fatalf("merged len = %d, want %d", len(gotMerged), len(wantMerged))
	}
	for i := range wantMerged {
		if gotMerged[i] != wantMerged[i] {
			t.Fatalf("merged[%d] = %v, want %v (full=%v)", i, gotMerged[i], wantMerged[i], gotMerged)
		}
	}
	// Swapping compilerVec for zeros would change the merge; the assertion
	// above fails if reuse silently dropped the content contribution.
	zeroMerged, _ := mergeEmbeddingVectors(titleVec, []float64{0, 0, 0}, titleWeight)
	if gotMerged[0] == zeroMerged[0] && gotMerged[1] == zeroMerged[1] && gotMerged[2] == zeroMerged[2] {
		t.Fatal("merged vector equals title-only/zero-content merge; reuse content was ignored")
	}

	if _, ok := result[0].Extra["_compiler_embd_id"]; ok {
		t.Error("_compiler_embd_id should be cleared after processing")
	}
}

// TestTokenizerSkipsReuseOnModelMismatch verifies that vectors are NOT reused
// when the embedding model ID does not match (#22134).
func TestTokenizerSkipsReuseOnModelMismatch(t *testing.T) {
	const currentEmbdID = "current-model"
	const compilerEmbdID = "different-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(currentEmbdID))
	comp.param.Fields = []string{"text"}
	comp.param.FilenameEmbdWeight = 0.1

	chunks := []schema.ChunkDoc{
		makeChunkWithExtra("compiler chunk", map[string]any{
			"id":                "chunk-compiler",
			"compile_kwd":       "mindmap_entity",
			"q_3_vec":           []float64{0.5, 0.6, 0.7},
			"_compiler_embd_id": compilerEmbdID,
		}),
	}

	_, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (title + content), got %d; model mismatch should prevent reuse", got)
	}
}

// TestTokenizerMixedChunksPartialReuse verifies that a mix of Compiler and
// fresh chunks results in partial reuse.
func TestTokenizerMixedChunksPartialReuse(t *testing.T) {
	const embdID = "test-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text"}

	chunks := []schema.ChunkDoc{
		makeChunkWithExtra("compiler chunk 1", map[string]any{
			"id":                "c1",
			"q_3_vec":           []float64{0.1, 0.2, 0.3},
			"_compiler_embd_id": embdID,
		}),
		makeChunkWithExtra("fresh chunk", map[string]any{
			"id": "c2",
		}),
		makeChunkWithExtra("compiler chunk 2", map[string]any{
			"id":                "c3",
			"q_3_vec":           []float64{0.4, 0.5, 0.6},
			"_compiler_embd_id": embdID,
		}),
	}

	result, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (title + 1 fresh), got %d", got)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 result chunks, got %d", len(result))
	}
	for i, ck := range result {
		if _, ok := ck.Extra["_compiler_embd_id"]; ok {
			t.Errorf("chunk %d still has _compiler_embd_id", i)
		}
	}
}

// TestTokenizerSkipsReuseOnNonTextFields verifies that vectors are NOT reused
// when the Tokenizer is configured with fields other than ["text"].
func TestTokenizerSkipsReuseOnNonTextFields(t *testing.T) {
	const embdID = "test-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text", "questions"}
	comp.param.FilenameEmbdWeight = 0.1

	ck := makeChunkWithExtra("compiler chunk", map[string]any{
		"id":                "c1",
		"q_3_vec":           []float64{0.1, 0.2, 0.3},
		"_compiler_embd_id": embdID,
	})
	ck.Questions = "What is this?"
	chunks := []schema.ChunkDoc{ck}

	_, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (no reuse for non-text fields), got %d", got)
	}
}

// TestTokenizerSkipsReuseOnContextWindows verifies that vectors are NOT reused
// when the chunk has ContextAbove or ContextBelow.
func TestTokenizerSkipsReuseOnContextWindows(t *testing.T) {
	const embdID = "test-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text"}
	comp.param.FilenameEmbdWeight = 0.1

	ck := makeChunkWithExtra("compiler chunk", map[string]any{
		"id":                "c1",
		"q_3_vec":           []float64{0.1, 0.2, 0.3},
		"_compiler_embd_id": embdID,
	})
	ck.ContextAbove = "previous paragraph"
	chunks := []schema.ChunkDoc{ck}

	_, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (no reuse with ContextAbove), got %d", got)
	}
}

// TestTokenizerSkipsReuseOnTableTags verifies that table-structure tags, which
// cleanEmbedText strips, prevent reuse because the encoded text would differ
// from ck.Text.
func TestTokenizerSkipsReuseOnTableTags(t *testing.T) {
	const embdID = "test-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text"}
	comp.param.FilenameEmbdWeight = 0.1

	text := "hello<table></table>world"
	chunks := []schema.ChunkDoc{
		makeChunkWithExtra(text, map[string]any{
			"id":                "c1",
			"q_3_vec":           []float64{0.1, 0.2, 0.3},
			"_compiler_embd_id": embdID,
		}),
	}

	_, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (no reuse with table tags), got %d", got)
	}
}

// TestTokenizerSkipsReuseOnLeadingTrailingWhitespace verifies that TrimSpace
// in cleanEmbedText prevents reuse when Text has surrounding whitespace.
func TestTokenizerSkipsReuseOnLeadingTrailingWhitespace(t *testing.T) {
	const embdID = "test-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text"}
	comp.param.FilenameEmbdWeight = 0.1

	chunks := []schema.ChunkDoc{
		makeChunkWithExtra("  padded text  ", map[string]any{
			"id":                "c1",
			"q_3_vec":           []float64{0.1, 0.2, 0.3},
			"_compiler_embd_id": embdID,
		}),
	}

	_, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (no reuse with whitespace), got %d", got)
	}
}

// TestTokenizerSkipsReuseOnTokenTruncation verifies that token-limit truncation
// prevents reuse because trunc would differ from ck.Text.
func TestTokenizerSkipsReuseOnTokenTruncation(t *testing.T) {
	const embdID = "test-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text"}
	comp.param.FilenameEmbdWeight = 0.1
	stub.maxTokens = 1 // force truncateForEmbedding to keep a single token

	longText := strings.Repeat("word ", 64) + "tail"
	chunks := []schema.ChunkDoc{
		makeChunkWithExtra(longText, map[string]any{
			"id":                "c1",
			"q_3_vec":           []float64{0.1, 0.2, 0.3},
			"_compiler_embd_id": embdID,
		}),
	}

	_, _, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("embedChunks error: %v", err)
	}
	if got := stub.calls.Load(); got != 2 {
		t.Errorf("expected 2 encode calls (no reuse when truncated), got %d", got)
	}
}

// TestTokenizerRepeatedApplication verifies that running Tokenizer twice does
// not re-reuse vectors (title weighting applied only once).
func TestTokenizerRepeatedApplication(t *testing.T) {
	const embdID = "test-model"
	const dim = 3

	comp, stub := withStubEmbedderEmbdID(t, dim, strPtr(embdID))
	comp.param.Fields = []string{"text"}
	comp.param.FilenameEmbdWeight = 0.1

	chunks := []schema.ChunkDoc{
		makeChunkWithExtra("compiler chunk", map[string]any{
			"id":                "c1",
			"q_3_vec":           []float64{0.1, 0.2, 0.3},
			"_compiler_embd_id": embdID,
		}),
	}

	ctx := context.Background()
	result1, _, err := comp.embedChunks(ctx, "tenant", "kb", "doc.pdf", chunks, nil)
	if err != nil {
		t.Fatalf("first embedChunks error: %v", err)
	}
	firstCallCount := stub.calls.Load()

	result2, _, err := comp.embedChunks(ctx, "tenant", "kb", "doc.pdf", result1, nil)
	if err != nil {
		t.Fatalf("second embedChunks error: %v", err)
	}
	secondPassCalls := stub.calls.Load() - firstCallCount
	if secondPassCalls != 2 {
		t.Errorf("second pass expected 2 encode calls, got %d", secondPassCalls)
	}
	if len(result2) != 1 {
		t.Fatalf("expected 1 result chunk, got %d", len(result2))
	}
}

// TestTokenizerInvokeFullTextOnlyClearsCompilerProvenance verifies that a
// full_text-only Invoke (no embedding) still strips `_compiler_embd_id` from
// output chunks so the marker never leaks downstream (#22134).
func TestTokenizerInvokeFullTextOnlyClearsCompilerProvenance(t *testing.T) {
	tokenizer.SetEngineType("infinity")
	defer tokenizer.SetEngineType("")

	comp, err := NewTokenizerComponent(map[string]any{
		"search_method": []any{"full_text"},
		"fields":        []any{"text"},
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponent: %v", err)
	}

	out, err := comp.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.pdf",
		"kb_id":         "kb-1",
		"output_format": "chunks",
		"chunks": []map[string]any{{
			"id":                "c1",
			"text":              "compiler chunk",
			"q_3_vec":           []float64{0.1, 0.2, 0.3},
			"_compiler_embd_id": "model-abc",
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunksRaw, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok {
		t.Fatalf("chunks type = %T", out["chunks"])
	}
	if len(chunksRaw) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunksRaw))
	}
	if _, exists := chunksRaw[0]["_compiler_embd_id"]; exists {
		t.Error("full_text-only output still has _compiler_embd_id")
	}
}
