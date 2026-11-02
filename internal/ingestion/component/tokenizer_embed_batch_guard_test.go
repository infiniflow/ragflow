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
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

// emptyingTrimmer emulates an EmbedderTrimmer that returns no text. The seam is
// optional and its signature carries no non-empty guarantee, so this is a
// legitimate implementation; a provider rejects the whole batch when such an
// input reaches it (vLLM: 400 "`inputs` cannot be empty").
type emptyingTrimmer struct{ *stubEmbedder }

func (e emptyingTrimmer) Trim(string) (string, int) { return "", 0 }

// configurableTrimmer is the transparent counterpart: it implements the trimmer
// seam but returns the text unchanged, so trimForEmbedding never needs the
// cl100k BPE table these unit tests must not depend on.
type configurableTrimmer struct{ *stubEmbedder }

func (c configurableTrimmer) Trim(text string) (string, int) { return text, 0 }

// TestEmbedChunksNeverSendsEmptyInput pins that the embedding batch never
// carries an empty string, whatever the trimmer returns. Sending one costs the
// entire document: the provider rejects the request as a whole.
func TestEmbedChunksNeverSendsEmptyInput(t *testing.T) {
	stub := newStubEmbedder(4)
	embdID := "embd-test"
	comp, err := NewTokenizerComponentWithResolver(nil, func(ctx context.Context, _, _ string) (Embedder, string, error) {
		return emptyingTrimmer{stub}, embdID, nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	tc := comp.(*TokenizerComponent)
	tc.param.Fields = []string{"text"}
	chunks := []schema.ChunkDoc{chunkWithID("c1", "text that cannot be trimmed to nothing")}

	if _, _, err := tc.embedChunks(context.Background(), "tenant", "kb", "doc.md", chunks, newMemCacheStore()); err != nil {
		t.Fatalf("embedChunks: %v", err)
	}
	// The chunk has text, so a content call must have happened (the title call
	// comes first). Without this the loop below would pass vacuously if some
	// future change skipped the chunk entirely.
	if len(stub.callInputs) < 2 {
		t.Fatalf("expected a title call and a content call, got %v", stub.callInputs)
	}
	for callIdx, in := range stub.callInputs {
		for _, s := range in {
			if strings.TrimSpace(s) == "" {
				t.Fatalf("call %d embedded an empty input %q; the provider rejects the whole batch (all inputs per call: %v)",
					callIdx, s, stub.callInputs)
			}
		}
	}
}

// TestEmbedChunksKeepsVectorRequirementWhenTrimIsEmpty covers the consistency
// CodeRabbit raised on #21439: validateTokenizerOutputs decides embeddability
// on the pre-trim text, so a chunk that has text must still end up with a
// vector even when the trimmer returns nothing. Otherwise the guard would trade
// the provider's 400 for "missing embedding vector".
func TestEmbedChunksKeepsVectorRequirementWhenTrimIsEmpty(t *testing.T) {
	stub := newStubEmbedder(4)
	embdID := "embd-test"
	comp, err := NewTokenizerComponentWithResolver(nil, func(ctx context.Context, _, _ string) (Embedder, string, error) {
		return emptyingTrimmer{stub}, embdID, nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	tc := comp.(*TokenizerComponent)
	tc.param.Fields = []string{"text"}
	chunks := []schema.ChunkDoc{chunkWithID("c1", "text the pipeline produced")}

	out, _, err := tc.embedChunks(context.Background(), "tenant", "kb", "doc.md", chunks, newMemCacheStore())
	if err != nil {
		t.Fatalf("embedChunks: %v", err)
	}
	if !hasEmbeddingVector(out[0]) {
		t.Fatal("chunk with pre-trim text carries no vector; validateTokenizerOutputs would fail with 'missing embedding vector'")
	}
	if err := validateTokenizerOutputs(out, []string{"embedding"}, tc.param.Fields, "kb"); err != nil {
		t.Fatalf("validateTokenizerOutputs rejected the output: %v", err)
	}
	// Negative control: the assertion above only means something if validation
	// actually demands a vector for this configuration. A chunk with the same
	// text and no vector must be rejected.
	if err := validateTokenizerOutputs([]schema.ChunkDoc{chunkWithID("c9", "text the pipeline produced")}, []string{"embedding"}, tc.param.Fields, "kb"); err == nil {
		t.Fatal("validateTokenizerOutputs accepted a vector-less chunk; the positive assertion above would be vacuous")
	}
}

// TestEmbedChunksMixedBatchKeepsResolvableChunks covers the case the report's
// diagnostic must not hide: some chunks resolve to text and others do not. The
// resolvable ones must still be embedded and keep their vectors, and the whole
// document must not fail because of the others.
func TestEmbedChunksMixedBatchKeepsResolvableChunks(t *testing.T) {
	stub := newStubEmbedder(4)
	embdID := "embd-test"
	comp, err := NewTokenizerComponentWithResolver(nil, func(ctx context.Context, _, _ string) (Embedder, string, error) {
		return configurableTrimmer{stub}, embdID, nil
	})
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	tc := comp.(*TokenizerComponent)
	tc.param.Fields = []string{"text"}
	chunks := []schema.ChunkDoc{
		chunkWithID("c1", "resolvable content"),
		chunkWithID("c2", "<table><tr><td></td></tr></table>"), // empty table: legitimately skipped
	}

	out, _, err := tc.embedChunks(context.Background(), "tenant", "kb", "doc.md", chunks, newMemCacheStore())
	if err != nil {
		t.Fatalf("embedChunks: %v", err)
	}
	if !hasEmbeddingVector(out[0]) {
		t.Error("chunk[0] has text but no vector; a mixed batch must still embed the resolvable chunks")
	}
	if hasEmbeddingVector(out[1]) {
		t.Error("chunk[1] is an empty table and must stay vector-less")
	}
	for _, in := range stub.callInputs {
		for _, s := range in {
			if strings.TrimSpace(s) == "" {
				t.Fatalf("embedded an empty input %q; provider rejects the whole batch", s)
			}
		}
	}
}

// TestEmbedChunksSkipsEveryChunkWhenFieldsResolveToNothing covers the
// configuration that produced the report: `fields` names a chunk field the
// pipeline does not fill, so every chunk resolves to nothing. The component must
// not call the provider at all and the caller keeps its chunks; the warning it
// emits names the configured fields (asserted only indirectly here — this test
// pins the behaviour, not the log line).
func TestEmbedChunksSkipsEveryChunkWhenFieldsResolveToNothing(t *testing.T) {
	comp, stub := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"no_such_field"}
	chunks := []schema.ChunkDoc{
		chunkWithID("c1", "content the pipeline actually produced"),
		chunkWithID("c2", "more content"),
	}

	out, consumed, err := comp.embedChunks(context.Background(), "tenant", "kb", "doc.md", chunks, newMemCacheStore())
	if err != nil {
		t.Fatalf("embedChunks: %v", err)
	}
	if got := stub.calls.Load(); got != 0 {
		t.Fatalf("embedder calls = %d, want 0 when no configured field resolves to text (inputs: %v)", got, stub.callInputs)
	}
	if consumed != 0 {
		t.Fatalf("embedding_token_consumption = %d, want 0", consumed)
	}
	if len(out) != len(chunks) {
		t.Fatalf("returned %d chunks, want %d preserved", len(out), len(chunks))
	}
	for i, ck := range out {
		if hasEmbeddingVector(ck) {
			t.Errorf("chunk[%d] unexpectedly carries a vector", i)
		}
	}
}
