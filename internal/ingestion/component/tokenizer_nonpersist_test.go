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

// Regression guard for the canvas-debug (dry-run) run contract: a debug run
// carries no KB (kb_id == ""), so the tokenizer must skip embedding rather than
// resolving an embedder. It lives in the canvas-debug branch (not the
// ingestion-fixes branch) so the ingestion-fixes branch stays compilable on its
// own.

package component

import (
	"context"
	"testing"
)

// TestTokenizerComponent_DebugSkipsEmbedding is a regression gate for the
// canvas-debug (dry-run) contract: a debug run has no KB (kb_id == ""), so
// embedding MUST be skipped — there is no embedder (and no embd_id) to resolve
// against, and a debug run must stay side-effect free. The tokenizer must
// return its chunks without error, without invoking the embedder, and without
// attaching embedding vectors. Conversely, when a kb_id is present the embedding
// path must run.
//
// It uses embedding-only search_method so it runs without the C++ RAGAnalyzer
// pool (plain `go test`, no -tags integration), keeping the regression gate
// executable in every environment.
func TestTokenizerComponent_DebugSkipsEmbedding(t *testing.T) {
	stub := newStubEmbedder(4)
	cIntf, err := NewTokenizerComponentWithResolver(
		map[string]any{"search_method": []any{"embedding"}, "fields": []any{"text"}},
		func(ctx context.Context, _, _, _ string) (Embedder, error) { return stub, nil },
	)
	if err != nil {
		t.Fatalf("NewTokenizerComponentWithResolver: %v", err)
	}
	c := cIntf.(*TokenizerComponent)

	run := func(kbID string) (int32, map[string]any) {
		inputs := map[string]any{
			"name":          "doc.pdf",
			"output_format": "chunks",
			"chunks": []map[string]any{
				{"text": "alpha bravo"},
				{"text": "charlie delta"},
			},
		}
		if kbID != "" {
			inputs["kb_id"] = kbID
		}
		out, err := c.Invoke(context.Background(), nil, inputs)
		if err != nil {
			t.Fatalf("Invoke(kb_id=%q): %v", kbID, err)
		}
		return stub.calls.Load(), out
	}

	// Debug (kb_id == ""): embedding must be skipped entirely.
	stub.calls.Store(0)
	debugCalls, debugOut := run("")
	if debugCalls != 0 {
		t.Fatalf("embedder called under debug (kb_id=\"\"): %d calls; embedding must be skipped", debugCalls)
	}
	if got := debugOut["embedding_token_consumption"]; got != nil {
		t.Errorf("embedding_token_consumption = %v under debug; must be omitted when embedding is skipped", got)
	}
	got, ok := debugOut["chunks"].([]map[string]any)
	if !ok || len(got) != 2 {
		t.Fatalf("chunks malformed under debug: %v", debugOut["chunks"])
	}
	for i, ck := range got {
		if _, has := ck["q_4_vec"]; has {
			t.Errorf("chunk[%d] carries q_4_vec under debug; embedding must be skipped", i)
		}
	}

	// With a KB: embedding must run.
	stub.calls.Store(0)
	kbCalls, kbOut := run("kb-1")
	if kbCalls != 2 {
		t.Fatalf("embedder calls = %d with kb_id set; want 2", kbCalls)
	}
	if got := kbOut["embedding_token_consumption"]; got == nil {
		t.Error("embedding_token_consumption missing with kb_id set; embedding accounting must run")
	}
	kbChunks, ok := kbOut["chunks"].([]map[string]any)
	if !ok || len(kbChunks) != 2 {
		t.Fatalf("chunks malformed with kb_id: %v", kbOut["chunks"])
	}
	for i, ck := range kbChunks {
		vec, ok := ck["q_4_vec"].([]float64)
		if !ok || len(vec) != 4 {
			t.Fatalf("chunk[%d] missing q_4_vec with kb_id set: %v", i, ck["q_4_vec"])
		}
	}
}
