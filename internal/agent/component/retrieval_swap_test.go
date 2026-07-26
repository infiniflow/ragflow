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

	agenttool "ragflow/internal/agent/tool"
)

// TestRetrieval_DelegatesToRealWrapper pins the Universe A →
// Universe B delegation for `Retrieval` and `SearchMyDataset`:
// with the simple retrieval service installed, an Invoke must
// surface a non-empty `formalized_content` — that's the contract
// the stub-only path no longer satisfies.
func TestRetrieval_DelegatesToRealWrapper(t *testing.T) {
	t.Parallel()

	// Install the synthetic service that returns 3 deterministic
	// chunks per query. Restore the stub on cleanup so the rest of
	// the suite stays unaffected.
	prev := agenttool.GetRetrievalService()
	agenttool.SetSimpleRetrievalService()
	t.Cleanup(func() { agenttool.SetRetrievalService(prev) })

	c, err := New(componentNameRetrieval, map[string]any{
		"kb_ids":               []any{"kb-1", "kb-2"},
		"top_n":                3,
		"similarity_threshold": 0.1,
	})
	if err != nil {
		t.Fatalf("New(Retrieval) errored: %v", err)
	}

	// The component must report the canonical Retrieval name (the
	// wrapper's Name() returns "Retrieval", distinct from the alias
	// "SearchMyDataset" which maps to the same factory).
	if got := c.Name(); got != componentNameRetrieval {
		t.Errorf("Retrieval c.Name() = %q, want %q", got, componentNameRetrieval)
	}

	out, err := c.Invoke(context.Background(), map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Retrieval Invoke errored: %v", err)
	}
	fc, _ := out["formalized_content"].(string)
	if fc == "" {
		t.Fatalf("expected non-empty formalized_content from real wrapper; got empty (out=%v)", out)
	}
	if !strings.Contains(fc, "ragflow") {
		t.Errorf("formalized_content %q does not echo the query", fc)
	}
}

// TestSearchMyDataset_AliasDelegatesToRealWrapper verifies the
// SearchMyDataset → Retrieval alias resolves to the real wrapper
// (not the stub).
func TestSearchMyDataset_AliasDelegatesToRealWrapper(t *testing.T) {
	prev := agenttool.GetRetrievalService()
	agenttool.SetSimpleRetrievalService()
	t.Cleanup(func() { agenttool.SetRetrievalService(prev) })

	c, err := New("SearchMyDataset", nil)
	if err != nil {
		t.Fatalf("New(SearchMyDataset) errored: %v", err)
	}

	// The wrapper's Name() returns the canonical "Retrieval"; the
	// alias lookup by "SearchMyDataset" still resolves.
	if got := c.Name(); got != componentNameRetrieval {
		t.Errorf("SearchMyDataset alias c.Name() = %q, want %q (canonical)", got, componentNameRetrieval)
	}

	out, err := c.Invoke(context.Background(), map[string]any{"query": "kb"})
	if err != nil {
		t.Fatalf("SearchMyDataset Invoke errored: %v", err)
	}
	if fc, _ := out["formalized_content"].(string); fc == "" {
		t.Fatalf("expected non-empty formalized_content from real wrapper; got empty (out=%v)", out)
	}
}

// TestRetrieval_InputsSurfaceMatchesStub guards against accidental
// regression in the Inputs() description surface when swapping from
// the stub to the wrapper. The v1 DSL fixture set uses these keys
// (kb_ids, similarity_threshold, keywords_similarity_weight, top_n,
// top_k, rerank_id, empty_response) for type checking and form
// rendering; removing or renaming one would break the fixture.
func TestRetrieval_InputsSurfaceMatchesStub(t *testing.T) {
	t.Parallel()

	stub, err := NewRetrievalStub(nil)
	if err != nil {
		t.Fatalf("NewRetrievalStub: %v", err)
	}
	inputs := stub.Inputs()
	for _, key := range []string{
		"kb_ids", "similarity_threshold", "keywords_similarity_weight",
		"top_n", "top_k", "rerank_id", "empty_response",
	} {
		if _, ok := inputs[key]; !ok {
			t.Errorf("Inputs() missing key %q (v1 fixture compatibility)", key)
		}
	}
}
