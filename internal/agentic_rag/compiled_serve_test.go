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

package agentic_rag

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// sampleHits builds a locate hit set: chunkA has a compiled derivative, chunkB
// does not. The raw snippet body must disappear from compiled modes.
func sampleHits() []snippetHit {
	return []snippetHit{
		{chunk: runtime.RetrievalChunk{
			ID: "chunkA", Content: "raw body for A", DocumentID: "doc1",
			DatasetID: "kb1", DocumentName: "Doc One", Score: 0.9, ChunkIndex: 0, PageNum: 1,
		}, snippet: "raw snippet for A"},
		{chunk: runtime.RetrievalChunk{
			ID: "chunkB", Content: "raw body for B", DocumentID: "doc1",
			DatasetID: "kb1", DocumentName: "Doc One", Score: 0.5, ChunkIndex: 1, PageNum: 2,
		}, snippet: "raw snippet for B"},
	}
}

func sampleProducts() []compiledRow {
	return []compiledRow{
		{id: "p1", kind: "graph", typeName: "person", name: "Harold Peel",
			description: "Former Bradford City player", docID: "doc1", chunkIDs: []string{"chunkA"}},
		{id: "p2", kind: "graph", typeName: "event", name: "1911 Cup Final",
			description: "Bradford won", docID: "doc1", chunkIDs: []string{"chunkA", "chunkB"}},
	}
}

func compiledCtx() context.Context {
	return WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
}

func withFetchFn(t *testing.T, fn func(context.Context, []string, []string) []compiledRow) {
	t.Helper()
	saved := fetchCompiledProductsFn
	fetchCompiledProductsFn = fn
	t.Cleanup(func() { fetchCompiledProductsFn = saved })
}

// TestCompiledServeOffIsByteIdentical: the off path (and the only path when no
// product is found) must serve the raw snippet exactly as today — the branch is a
// no-op, so the rendered bytes are unchanged.
func TestCompiledServeOffIsByteIdentical(t *testing.T) {
	t.Setenv(common.EnvCompiledServe, "off")
	ctx := compiledCtx()
	off := formatLocateResultsXML(ctx, searchBm25ChunksToolName, "q", sampleHits())

	t.Setenv(common.EnvCompiledServe, "only")
	withFetchFn(t, func(context.Context, []string, []string) []compiledRow { return nil })
	onlyNoProduct := formatLocateResultsXML(ctx, searchBm25ChunksToolName, "q", sampleHits())

	if off != onlyNoProduct {
		t.Fatalf("off path and only-without-products must be byte-identical:\noff=%s\nonly=%s", off, onlyNoProduct)
	}
	if !strings.Contains(off, "<match_snippet>") {
		t.Errorf("off path must still serve the raw snippet")
	}
	if strings.Contains(off, "<compiled_products>") {
		t.Errorf("off path must not emit compiled products")
	}
}

// TestCompiledServeOnlyDropsRawText: only mode serves compiled products and the
// chunk metadata handles, but withholds the raw snippet/body.
func TestCompiledServeOnlyDropsRawText(t *testing.T) {
	t.Setenv(common.EnvCompiledServe, "only")
	withFetchFn(t, func(context.Context, []string, []string) []compiledRow { return sampleProducts() })
	out := formatLocateResultsXML(compiledCtx(), searchBm25ChunksToolName, "q", sampleHits())

	if strings.Contains(out, "<match_snippet>") {
		t.Errorf("only mode must not serve the raw snippet:\n%s", out)
	}
	if !strings.Contains(out, "<compiled_products ") {
		t.Errorf("only mode must emit the compiled_products block:\n%s", out)
	}
	if !strings.Contains(out, "<product kind=\"graph\"") {
		t.Errorf("only mode must render compiled product rows:\n%s", out)
	}
	if !strings.Contains(out, "chunk_id=\"chunkA\"") {
		t.Errorf("only mode must keep the chunk handle for deep-read fallback:\n%s", out)
	}
	if !strings.Contains(out, "compiled=\"2\"") {
		t.Errorf("only mode must annotate each hit with its compiled derivative count:\n%s", out)
	}
	if strings.Contains(out, "<deep_read_hint>") {
		t.Errorf("only mode must not emit the deep_read_hint:\n%s", out)
	}
	if strings.Contains(out, "raw body for A") {
		t.Errorf("only mode must not leak the raw chunk body:\n%s", out)
	}
}

// TestCompiledServeFirstKeepsHandlesAndHint: first mode shows compiled products
// plus the chunk handles and the deep_read_hint, but still withholds the raw snippet.
func TestCompiledServeFirstKeepsHandlesAndHint(t *testing.T) {
	t.Setenv(common.EnvCompiledServe, "first")
	withFetchFn(t, func(context.Context, []string, []string) []compiledRow { return sampleProducts() })
	out := formatLocateResultsXML(compiledCtx(), grepChunksToolName, "q", sampleHits())

	if strings.Contains(out, "<match_snippet>") {
		t.Errorf("first mode must not serve the raw snippet:\n%s", out)
	}
	if !strings.Contains(out, "<compiled_products ") {
		t.Errorf("first mode must emit compiled_products:\n%s", out)
	}
	if !strings.Contains(out, "<deep_read_hint>") {
		t.Errorf("first mode must emit the deep_read_hint:\n%s", out)
	}
	if !strings.Contains(out, "number_neighbors=0") {
		t.Errorf("first mode's hint must pin the fallback to the single original chunk (number_neighbors=0):\n%s", out)
	}
	if !strings.Contains(out, "chunk_id=\"chunkA\"") {
		t.Errorf("first mode must keep the chunk handle:\n%s", out)
	}
}

// TestCompiledServeFallsBackWhenNoCompiledRows: when no compiled product is found
// (engine down, or no lineage), the run must passthrough the raw snippet rather
// than hand the model an empty result — the failure philosophy.
func TestCompiledServeFallsBackWhenNoCompiledRows(t *testing.T) {
	t.Setenv(common.EnvCompiledServe, "only")
	withFetchFn(t, func(context.Context, []string, []string) []compiledRow { return nil })
	out := formatLocateResultsXML(compiledCtx(), searchBm25ChunksToolName, "q", sampleHits())

	if !strings.Contains(out, "<match_snippet>") {
		t.Errorf("no-compiled fallback must passthrough the raw snippet:\n%s", out)
	}
	if strings.Contains(out, "<compiled_products>") {
		t.Errorf("no-compiled fallback must not emit an empty compiled block:\n%s", out)
	}
}

// TestCompiledServeAppliesToToolScope: web_search is never in scope (no corpus
// document to derive a product from); the corpus-passage locate tools are.
func TestCompiledServeAppliesToToolScope(t *testing.T) {
	t.Setenv(common.EnvCompiledServe, "only")
	// Tools that must never serve compiled products.
	for _, tool := range []string{
		webSearchToolName,
		listChunksToolName,
		metadataSearchToolName,
		graphExploreToolName,
		graphDocOutlineToolName,
		navigateTreeToolName,
		navigateStructureToolName,
	} {
		if compiledServeAppliesTo(tool) {
			t.Errorf("%s must never be in compiled-serve scope", tool)
		}
	}
	// Every chunk-searching tool is in scope — and so is a tool added later:
	// the scope is an exclusion list, so a new locate tool inherits the arm
	// instead of silently missing it.
	for _, tool := range []string{
		grepChunksToolName,
		searchBm25ChunksToolName,
		searchChunksToolName,
		searchSemanticChunksToolName,
		"some_future_chunk_tool",
	} {
		if !compiledServeAppliesTo(tool) {
			t.Errorf("%s must be in compiled-serve scope", tool)
		}
	}

	t.Setenv(common.EnvCompiledServeScope, "grep_chunks")
	if !compiledServeAppliesTo(grepChunksToolName) {
		t.Errorf("scope override must include grep_chunks")
	}
	if compiledServeAppliesTo(searchBm25ChunksToolName) {
		t.Errorf("scope override must exclude search_bm25_chunks")
	}
}

// TestCompiledServeRequestShape: the lineage query is pure provenance (B design) —
// it filters by source_chunk_ids and compile_kwd, with NO MatchExprs on the
// compiled text, so query-phrasing drift cannot drop a recalled chunk's product.
func TestCompiledServeRequestShape(t *testing.T) {
	f := buildCompiledFilter([]string{"c1", "c2"}, nil, nil)
	if got, _ := f["exists"].(string); got != "compile_kwd" {
		t.Errorf("filter must require compile_kwd presence, got %v", f["exists"])
	}
	if got, _ := f["source_chunk_ids"].([]string); len(got) != 2 {
		t.Errorf("filter must carry the recalled chunk ids, got %v", f["source_chunk_ids"])
	}
	if _, ok := f["compile_kwd"]; ok {
		t.Errorf("without explicit kinds the filter must not add a compile_kwd terms clause")
	}
	if _, ok := f["entity_type_kwd"]; ok {
		t.Errorf("without explicit types the filter must not add an entity_type_kwd terms clause")
	}

	f2 := buildCompiledFilter([]string{"c1"}, []string{"tree", "graph"}, nil)
	if got, _ := f2["compile_kwd"].([]string); len(got) != 2 {
		t.Errorf("explicit kinds must become a compile_kwd terms clause, got %v", f2["compile_kwd"])
	}

	// Claim rows carry compile_kwd=tree just like tree nodes, so TYPES is the only
	// knob that separates them.
	f3 := buildCompiledFilter([]string{"c1"}, []string{"tree"}, []string{"claim"})
	got, _ := f3["entity_type_kwd"].([]string)
	if len(got) != 1 || got[0] != "claim" {
		t.Errorf("explicit types must become an entity_type_kwd terms clause, got %v", f3["entity_type_kwd"])
	}
}
