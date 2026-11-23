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

package agentic_rag

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

// sharedScopeCtx pins the dataset → tenant resolution the way agentic_rag.Run
// does for one turn: kb-a lives in tenant-a (a team-shared dataset), kb-b in the
// conversation's own tenant-b.
func sharedScopeCtx() context.Context {
	return WithDatasetTenants(context.Background(), map[string]string{
		"kb-a": "tenant-a",
		"kb-b": "tenant-b",
	})
}

// TestIndexNamesForCoversEveryDatasetTenant is the regression guard for the
// team-shared-dataset bug: the corpus index set must come from the datasets, so
// a dataset shared from tenant-a is searched in tenant-a's index rather than in
// the conversation's index (which holds none of its chunks).
func TestIndexNamesForCoversEveryDatasetTenant(t *testing.T) {
	got := IndexNamesFor(sharedScopeCtx(), []string{"kb-a", "kb-b", "kb-a"})
	want := []string{"ragflow_tenant-a", "ragflow_tenant-b"}
	if len(got) != len(want) {
		t.Fatalf("IndexNamesFor = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("IndexNamesFor = %v, want %v", got, want)
		}
	}
}

// TestIndexNamesForWithoutResolutionIsEmpty asserts an unresolved scope can only
// ever widen to nothing: the engines reject an empty index list, so a missing
// resolution cannot turn into an unscoped read.
func TestIndexNamesForWithoutResolutionIsEmpty(t *testing.T) {
	if got := IndexNamesFor(context.Background(), []string{"kb-a"}); len(got) != 0 {
		t.Fatalf("IndexNamesFor without resolution = %v, want no index names", got)
	}
	// An empty resolution is never pinned: pinning it would freeze one failed
	// lookup into "this turn has no corpus", with no chance to recover.
	if _, pinned := datasetTenantsFromContext(WithDatasetTenants(context.Background(), nil)); pinned {
		t.Fatal("an empty dataset→tenant resolution must not be pinned")
	}
}

// TestGrepAndBm25SearchEveryDatasetTenant asserts both lexical legs address the
// index of each bound dataset's own tenant.
func TestGrepAndBm25SearchEveryDatasetTenant(t *testing.T) {
	fe := &grepFakeEngine{}
	ctx := sharedScopeCtx()

	if _, err := NewGrepAdapter(fe).Grep(ctx, runtime.GrepRequest{
		Pattern:    "alpha",
		DatasetIDs: []string{"kb-a", "kb-b"},
	}); err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if fe.lastRegexpReq == nil {
		t.Fatal("grep sent no regexp request")
	}
	if names := fe.lastRegexpReq.IndexNames; len(names) != 2 || names[0] != "ragflow_tenant-a" || names[1] != "ragflow_tenant-b" {
		t.Fatalf("grep IndexNames = %v, want both dataset tenants", names)
	}

	if _, err := NewBm25Adapter(fe).SearchBm25(ctx, runtime.Bm25Request{
		Queries:    []string{"alpha"},
		DatasetIDs: []string{"kb-a", "kb-b"},
	}); err != nil {
		t.Fatalf("SearchBm25: %v", err)
	}
	if fe.lastReq == nil {
		t.Fatal("bm25 sent no search request")
	}
	if names := fe.lastReq.IndexNames; len(names) != 2 || names[0] != "ragflow_tenant-a" || names[1] != "ragflow_tenant-b" {
		t.Fatalf("bm25 IndexNames = %v, want both dataset tenants", names)
	}
}

// TestDeepReadSearchesEveryDatasetTenant covers list_chunks: the deep read is a
// doc-scoped read, and it must still address the shared dataset's index.
func TestDeepReadSearchesEveryDatasetTenant(t *testing.T) {
	fe := &grepFakeEngine{regexpChunks: []map[string]interface{}{
		{"id": "c1", "doc_id": "d1", "kb_id": "kb-a", "content_with_weight": "shared text", "chunk_order_int": 0},
	}}
	if _, err := NewGrepAdapter(fe).ListByDocIDs(sharedScopeCtx(), runtime.GrepRequest{
		DatasetIDs: []string{"kb-a", "kb-b"},
		DocScope:   []string{"d1"},
	}); err != nil {
		t.Fatalf("ListByDocIDs: %v", err)
	}
	if fe.lastRegexpReq == nil {
		t.Fatal("deep read sent no regexp request")
	}
	names := fe.lastRegexpReq.IndexNames
	if len(names) != 2 || names[0] != "ragflow_tenant-a" || names[1] != "ragflow_tenant-b" {
		t.Fatalf("deep read IndexNames = %v, want both dataset tenants", names)
	}
}

// TestCompiledProbeSearchesEveryDatasetTenant covers the compiled-knowledge
// probe: it decides whether the navigation tools are offered at all, so a probe
// scoped to the conversation's tenant would hide a shared dataset's compiled
// products.
func TestCompiledProbeSearchesEveryDatasetTenant(t *testing.T) {
	fe := &grepFakeEngine{}

	if datasetsHaveCompiledKnowledge(sharedScopeCtx(), fe, []string{"kb-a", "kb-b"}) {
		t.Fatal("probe reported compiled knowledge for an empty result set")
	}

	if fe.lastReq == nil {
		t.Fatal("compiled-knowledge probe sent no search request")
	}
	names := fe.lastReq.IndexNames
	if len(names) != 2 || names[0] != "ragflow_tenant-a" || names[1] != "ragflow_tenant-b" {
		t.Fatalf("compiled-knowledge probe IndexNames = %v, want both dataset tenants", names)
	}
}
