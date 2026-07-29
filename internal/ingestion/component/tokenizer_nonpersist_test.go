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

// Regression guard for the non-persist (canvas-debug) run contract. It lives in
// the canvas-debug branch (not the ingestion-fixes branch) because it depends on
// component/globals.WithPersist, which is defined by the debug feature, keeping
// the ingestion-fixes branch compilable on its own.

package component

import (
	"context"
	"testing"

	"ragflow/internal/ingestion/component/globals"
)

// TestTokenizerComponent_EmbeddingRunsWhenNonPersist is a regression gate for
// the non-persist (canvas-debug) run contract: embedding MUST still execute
// even when the run is flagged persist=false. The debug path exists to exercise
// the whole pipeline — including the embedding service's capability — so only
// the *persistent* side effects (MinIO image upload, index insert, pipeline
// log) are gated by persist. A future "optimization" that skips embedding under
// !persist would silently break debug verification of the embedding service;
// this test fails loudly if that ever lands.
//
// It uses embedding-only search_method so it runs without the C++ RAGAnalyzer
// pool (plain `go test`, no -tags integration), keeping the regression gate
// executable in every environment.
func TestTokenizerComponent_EmbeddingRunsWhenNonPersist(t *testing.T) {
	stub := newStubEmbedder(4)
	cIntf, err := NewTokenizerComponentWithResolver(
		map[string]any{"search_method": []any{"embedding"}, "fields": []any{"text"}},
		func(ctx context.Context, _, _, _ string) (Embedder, error) { return stub, nil },
	)
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	c := cIntf.(*TokenizerComponent)

	run := func(ctx context.Context) (int32, map[string]any) {
		out, err := c.Invoke(ctx, nil, map[string]any{
			"name":          "doc.pdf",
			"output_format": "chunks",
			"chunks": []map[string]any{
				{"text": "alpha bravo"},
				{"text": "charlie delta"},
			},
		})
		if err != nil {
			t.Fatalf("Invoke: %v", err)
		}
		return stub.calls.Load(), out
	}

	persistCalls, _ := run(globals.WithPersist(context.Background(), true))
	stub.calls.Store(0)
	nonPersistCalls, nonPersistOut := run(globals.WithPersist(context.Background(), false))

	if nonPersistCalls == 0 {
		t.Fatal("embedder NOT called under persist=false; embedding must run in non-persist (debug) runs")
	}
	// Embedding work must be identical regardless of the persist flag.
	if nonPersistCalls != persistCalls {
		t.Errorf("embedder call count differs by persist flag: non-persist=%d persist=%d; embedding work must not depend on persist",
			nonPersistCalls, persistCalls)
	}
	if got := nonPersistOut["embedding_token_consumption"]; got == nil {
		t.Error("embedding_token_consumption missing under persist=false; embedding accounting must run")
	}

	got, ok := nonPersistOut["chunks"].([]map[string]any)
	if !ok || len(got) != 2 {
		t.Fatalf("chunks malformed under persist=false: %v", nonPersistOut["chunks"])
	}
	for i, ck := range got {
		vec, ok := ck["q_4_vec"].([]float64)
		if !ok || len(vec) != 4 {
			t.Fatalf("chunk[%d] missing q_4_vec under persist=false: %v", i, ck["q_4_vec"])
		}
	}
}
