//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"testing"

	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine"
)

// stubRetrievalService returns a fixed set of child chunks so the harness
// retireval path can be exercised without a live search backend. lastReq, when
// set, receives the request the harness handed down.
type stubRetrievalService struct {
	chunks  []runtime.RetrievalChunk
	lastReq *runtime.RetrievalRequest
}

func (s stubRetrievalService) Search(_ context.Context, _ *gorm.DB, req runtime.RetrievalRequest) ([]runtime.RetrievalChunk, error) {
	if s.lastReq != nil {
		*s.lastReq = req
	}
	return s.chunks, nil
}

// stubDocEngine implements engine.DocEngine by only serving GetChunk; the rest
// of the (large) interface is promoted from a nil field and never called by
// this test.
type stubDocEngine struct {
	engine.DocEngine
	parents map[string]map[string]any
}

func (s stubDocEngine) GetChunk(_ context.Context, _, chunkID string, _ []string) (interface{}, error) {
	if p, ok := s.parents[chunkID]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("parent %s not found", chunkID)
}

// TestRuntimeRetrieverPreservesUnsetControls pins the presence semantics of the
// retrieval controls: an omitted threshold/weight must reach the retrieval
// service as nil so it keeps its own default, while an explicit zero is a real
// override. Taking the address of a zero value used to force "no threshold
// floor + the vector leg at full weight" onto every caller that omitted them.
func TestRuntimeRetrieverPreservesUnsetControls(t *testing.T) {
	prev := runtime.GetRetrievalService()
	var got runtime.RetrievalRequest
	runtime.SetRetrievalService(stubRetrievalService{lastReq: &got})
	t.Cleanup(func() { runtime.SetRetrievalService(prev) })

	r := &RuntimeRetriever{}
	if _, err := r.Retrieve(context.Background(), RetrieveRequest{Query: "q", DatasetIDs: []string{"kb-1"}}); err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if got.SimilarityThreshold != nil || got.KeywordsSimilarityWeight != nil {
		t.Errorf("omitted controls = %v / %v, want nil so the service keeps its defaults",
			got.SimilarityThreshold, got.KeywordsSimilarityWeight)
	}

	threshold, weight := 0.35, 0.3
	if _, err := r.Retrieve(context.Background(), RetrieveRequest{
		Query:                    "q",
		SimilarityThreshold:      &threshold,
		KeywordsSimilarityWeight: &weight,
	}); err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if got.SimilarityThreshold == nil || *got.SimilarityThreshold != threshold {
		t.Errorf("SimilarityThreshold = %v, want %v", got.SimilarityThreshold, threshold)
	}
	if got.KeywordsSimilarityWeight == nil || *got.KeywordsSimilarityWeight != weight {
		t.Errorf("KeywordsSimilarityWeight = %v, want %v", got.KeywordsSimilarityWeight, weight)
	}
}

// TestChunkAggRetrieveLeavesControlsUnset pins the caller side: the chunk-agg
// retriever has no threshold/weight of its own, so it must omit both rather
// than pass zero overrides.
func TestChunkAggRetrieveLeavesControlsUnset(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{{"chunk_id": "c1"}}}

	chunks, err := chunkAggRetrieveFrom(r)(context.Background(), "t1", "kb-1", "q", nil, 5, 0.9)
	if err != nil {
		t.Fatalf("chunkAggRetrieveFrom: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want the retrieved chunk", len(chunks))
	}
	req := r.lastReq(t)
	if req.SimilarityThreshold != nil || req.KeywordsSimilarityWeight != nil {
		t.Errorf("controls = %v / %v, want nil (zero is a valid value, not an unset marker)",
			req.SimilarityThreshold, req.KeywordsSimilarityWeight)
	}
}

// TestChunkAggRetrievePropagatesError pins the adapter half of the router
// contract: a retrieval failure must reach the router as an error, not as an
// empty chunk list (which the router would report as a successful empty route).
func TestChunkAggRetrievePropagatesError(t *testing.T) {
	wantErr := errors.New("retrieval down")
	r := &stubRetriever{err: wantErr}

	chunks, err := chunkAggRetrieveFrom(r)(context.Background(), "t1", "kb-1", "q", nil, 5, 0.9)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if chunks != nil {
		t.Errorf("chunks = %v, want nil alongside the error", chunks)
	}
}

// TestRuntimeRetrieverPromotesChildrenToParent verifies that
// RuntimeRetriever.Retrieve threads child chunks through retrieval_by_children:
// two child fragments sharing a mom_id collapse into the single parent chunk,
// mirroring Python settings.retriever.retrieval_by_children.
func TestRuntimeRetrieverPromotesChildrenToParent(t *testing.T) {
	prev := runtime.GetRetrievalService()
	runtime.SetRetrievalService(stubRetrievalService{chunks: []runtime.RetrievalChunk{
		{ID: "child-1", Content: "frag one", DatasetID: "kb-1", MomID: "parent-1", Score: 0.6},
		{ID: "child-2", Content: "frag two", DatasetID: "kb-1", MomID: "parent-1", Score: 0.8},
		{ID: "top-1", Content: "standalone", DatasetID: "kb-1", Score: 0.9},
	}})
	t.Cleanup(func() { runtime.SetRetrievalService(prev) })

	de := stubDocEngine{parents: map[string]map[string]any{
		"parent-1": {
			"chunk_id":            "parent-1",
			"content_with_weight": "the full parent chunk text",
			"doc_id":              "doc-1",
			"docnm_kwd":           "doc.pdf",
			"kb_id":               "kb-1",
			"doc_type_kwd":        "pdf",
		},
	}}

	deps := SearchDeps{
		Backend:   &RuntimeRetriever{},
		DocEngine: de,
		TenantID:  "tenant-1",
	}
	out, aggs := HybridSearch(context.Background(), deps, SearchParams{
		Question: "q",
		KbIDs:    []string{"kb-1"},
	})
	_ = aggs
	if len(out) == 0 {
		t.Fatal("HybridSearch returned no chunks")
	}

	// parent-1 (from child-1 + child-2) + top-1 = 2 chunks.
	if len(out) != 2 {
		t.Fatalf("expected 2 aggregated chunks, got %d: %v", len(out), out)
	}
	var parent, top map[string]any
	for _, c := range out {
		if c["chunk_id"] == "parent-1" {
			parent = c
		}
		if c["chunk_id"] == "top-1" {
			top = c
		}
	}
	if parent == nil {
		t.Fatalf("parent chunk missing from result: %v", out)
	}
	if top == nil {
		t.Fatalf("top-level chunk missing from result: %v", out)
	}
	// similarity is the average of the two child scores (0.6, 0.8) -> 0.7.
	if sim, ok := parent["similarity"].(float64); !ok || sim != 0.7 {
		t.Fatalf("expected parent similarity 0.7, got %v", parent["similarity"])
	}
	if pw, _ := parent["content_with_weight"].(string); pw != "the full parent chunk text" {
		t.Fatalf("expected parent content_with_weight, got %v", parent["content_with_weight"])
	}
	if v, ok := parent["mom_id"]; ok && v != "" {
		t.Fatalf("aggregated parent should no longer carry a mom_id, got %v", v)
	}
}

// TestHybridSearchSkipsWhenNoChildren verifies that chunks without a mom_id
// pass through unchanged and retrieval_by_children is a no-op for them.
func TestHybridSearchSkipsWhenNoChildren(t *testing.T) {
	prev := runtime.GetRetrievalService()
	runtime.SetRetrievalService(stubRetrievalService{chunks: []runtime.RetrievalChunk{
		{ID: "top-1", Content: "a", DatasetID: "kb-1", Score: 0.5},
	}})
	t.Cleanup(func() { runtime.SetRetrievalService(prev) })

	deps := SearchDeps{Backend: &RuntimeRetriever{}, TenantID: "tenant-1"}
	out, _ := HybridSearch(context.Background(), deps, SearchParams{Question: "q", KbIDs: []string{"kb-1"}})
	if len(out) != 1 || out[0]["chunk_id"] != "top-1" {
		t.Fatalf("expected unchanged single chunk, got %v", out)
	}
}

// TestBM25SearchDoesPromoteChildren verifies that bm25_search (like every other
// entry point) runs retrieval_by_children: a child fragment is lifted to its
// parent chunk. This mirrors Python search.py:bm25_search, which routes
// through _normalize (which calls retrieval_by_children).
func TestBM25SearchDoesPromoteChildren(t *testing.T) {
	prev := runtime.GetRetrievalService()
	runtime.SetRetrievalService(stubRetrievalService{chunks: []runtime.RetrievalChunk{
		{ID: "child-1", Content: "frag", DatasetID: "kb-1", MomID: "parent-1", Score: 0.6},
	}})
	t.Cleanup(func() { runtime.SetRetrievalService(prev) })

	// DocEngine is set, so BM25Search promotes the child to its parent.
	deps := SearchDeps{Backend: &RuntimeRetriever{}, DocEngine: stubDocEngine{parents: map[string]map[string]any{
		"parent-1": {"chunk_id": "parent-1", "content_with_weight": "the full parent chunk text", "doc_id": "doc-1"},
	}}, TenantID: "tenant-1"}
	out, _ := BM25Search(context.Background(), deps, SearchParams{Question: "q", KbIDs: []string{"kb-1"}})
	if len(out) != 1 || out[0]["chunk_id"] != "parent-1" {
		t.Fatalf("bm25_search must promote the child to its parent, got %v", out)
	}
}

// TestExecuteLogsFunctionToolLine covers the Python tool_decorator.py:tool_call_async line
// "[Function tool] Running the {name} tool with: {args}", which Python's
// _SCOPED_PREFIXES forwarded into the think block.
func TestExecuteLogsFunctionToolLine(t *testing.T) {
	var buf bytes.Buffer
	deps := SearchDeps{Logger: log.New(&buf, "", 0)}
	ex := NewSearchExecutor(deps, RunRequest{})

	// "calculate" is a local tool; no backend is configured, so the call may
	// fail. The think-log line must be emitted regardless of the outcome.
	_, _ = ex.Execute(context.Background(), "calculate", map[string]any{"expr": "1+1"})

	out := buf.String()
	if !strings.Contains(out, "[Function tool] Running the calculate tool with:") {
		t.Fatalf("missing [Function tool] line, got:\n%s", out)
	}
	if !strings.Contains(out, `"expr":"1+1"`) {
		t.Errorf("args not rendered, got:\n%s", out)
	}
}

// TestExecuteNoLoggerIsSafe guards the nil-Logger path (Logger is optional).
func TestExecuteNoLoggerIsSafe(t *testing.T) {
	ex := NewSearchExecutor(SearchDeps{}, RunRequest{})
	if _, err := ex.Execute(context.Background(), "calculate", map[string]any{"expr": "2+2"}); err == nil {
		t.Log("call succeeded; nil logger must not panic")
	}
}

// TestSearchChunksAlwaysUsesCompiled pins fix #2: Python's search_chunks tool
// ALWAYS enables compiled-structure expansion (action_session.py:execute_tool passes
// use_compiled=True); retrieve never does. The Go executor must mirror that and
// NOT gate it on RunRequest.UseCompiled (which controls the L1 direct retrieve).
func TestSearchChunksAlwaysUsesCompiled(t *testing.T) {
	// search_chunks must expand (independent of req.UseCompiled).
	deps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	deps.Expand = &stubExpander{}
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})
	if _, err := ex.Execute(context.Background(), "search_chunks", map[string]any{"query": "q"}); err != nil {
		t.Fatalf("search_chunks: %v", err)
	}
	if deps.Expand.(*stubExpander).calls != 1 {
		t.Errorf("search_chunks compiled expansion calls = %d, want 1", deps.Expand.(*stubExpander).calls)
	}

	// retrieve must NOT expand.
	deps2, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	deps2.Expand = &stubExpander{}
	ex2 := NewSearchExecutor(deps2, RunRequest{DatasetIDs: []string{"kb1"}})
	if _, err := ex2.Execute(context.Background(), "retrieve", map[string]any{"query": "q"}); err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if deps2.Expand.(*stubExpander).calls != 0 {
		t.Errorf("retrieve compiled expansion calls = %d, want 0", deps2.Expand.(*stubExpander).calls)
	}

	// An explicit req.UseCompiled=false must NOT disable it for search_chunks.
	deps3, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	deps3.Expand = &stubExpander{}
	exOff := NewSearchExecutor(deps3, RunRequest{DatasetIDs: []string{"kb1"}, UseCompiled: false})
	if _, err := exOff.Execute(context.Background(), "search_chunks", map[string]any{"query": "q"}); err != nil {
		t.Fatalf("search_chunks (UseCompiled=false): %v", err)
	}
	if deps3.Expand.(*stubExpander).calls != 1 {
		t.Errorf("search_chunks with req.UseCompiled=false must still expand, calls = %d, want 1", deps3.Expand.(*stubExpander).calls)
	}
}

func TestRenderToolArgs(t *testing.T) {
	if got := renderToolArgs(nil); got != "{}" {
		t.Errorf("renderToolArgs(nil) = %q, want {}", got)
	}
	if got := renderToolArgs(map[string]any{}); got != "{}" {
		t.Errorf("renderToolArgs(map{}) = %q, want {}", got)
	}
	got := renderToolArgs(map[string]any{"q": "hi", "n": 3})
	if !strings.Contains(got, `"q":"hi"`) || !strings.Contains(got, `"n":3`) {
		t.Errorf("renderToolArgs = %q, want both keys", got)
	}
	// Unmarshalable values must degrade to "{}", not blow up the log line.
	if got := renderToolArgs(map[string]any{"f": func() {}}); got != "{}" {
		t.Errorf("renderToolArgs(func value) = %q, want {}", got)
	}
}

// TestSearchNilKBIsSeeded pins the Python _seed_evidence semantics
// (action_session.py:_admit_evidence): a nil deps.KB is not a caller mistake — the
// executor creates the (empty) pool on first use, so the search RUNS and its
// outcome is decided by what it admitted (OK here), never a forced MISS. The
// executor must also not panic on the nil pool.
func TestSearchNilKBIsSeeded(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	deps.KB = nil
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})
	oc, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("nil KB must not error: %v", err)
	}
	if oc.Status != StatusOK {
		t.Errorf("nil KB retrieve status = %s, want %s", oc.Status, StatusOK)
	}
	if len(oc.Payload) != 1 {
		t.Errorf("nil KB retrieve payload = %v, want 1 passage", oc.Payload)
	}
}

// TestListChunksNilKBIsMiss pins the Python semantics: a nil deps.KB is seeded
// by _seed_evidence, and with no doc-store reader wired the read yields nothing
// — _search_outcome([]) is MISS/no_doc (there is no "graceful OK empty" branch in
// Python). It must never error or panic.
func TestListChunksNilKBIsMiss(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.KB = nil
	ex := &searchExecutor{deps: deps, req: RunRequest{DatasetIDs: []string{"kb1"}}}
	oc, err := ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a"})
	if err != nil {
		t.Fatalf("nil KB list_chunks must not error: %v", err)
	}
	if oc.Status != StatusMiss || oc.Reason != ReasonNoDoc {
		t.Errorf("nil KB list_chunks = (%s,%s), want (miss,no_doc)", oc.Status, oc.Reason)
	}
	if len(oc.Payload) != 0 {
		t.Errorf("nil KB list_chunks payload = %v, want empty", oc.Payload)
	}
}

// TestListChunksDeepReadsDocStore pins the Python-parity deep read: list_chunks
// reads the document off the chunk store (deps.DocChunks, the same reader
// fetch_full_document uses) even when NONE of its chunks are in the evidence
// pool yet, and admits them into the shared pool the model can cite.
func TestListChunksDeepReadsDocStore(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.DocChunks = docChunksFor("doc-a", 2) // only doc-a is readable
	ex := &searchExecutor{deps: deps, req: RunRequest{DatasetIDs: []string{"kb1"}, MaxLength: 8192}}

	oc, err := ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a"})
	if err != nil {
		t.Fatalf("list_chunks: %v", err)
	}
	if oc.Status != StatusOK {
		t.Errorf("status = %s, want %s", oc.Status, StatusOK)
	}
	if len(oc.Payload) != 2 {
		t.Fatalf("payload = %d, want 2 (only doc-a's chunks)", len(oc.Payload))
	}
	// Deep-read admitted into the shared evidence pool.
	if len(kb.Chunks) != 2 {
		t.Errorf("kb.Chunks = %d, want 2 admitted", len(kb.Chunks))
	}
	if len(oc.EvidenceIDs) != 2 {
		t.Errorf("evidenceIDs = %d, want 2", len(oc.EvidenceIDs))
	}
	// Passage shape mirrors _admit_evidence(include_doc_id=False): id + content.
	for _, p := range oc.Payload {
		m, ok := p.(map[string]any)
		if !ok || m["id"] == nil || m["content"] == nil {
			t.Fatalf("passage missing id/content: %#v", p)
		}
		if _, hasDoc := m["doc_id"]; hasDoc {
			t.Errorf("list_chunks passage must not carry doc_id: %#v", m)
		}
	}
}

// TestListChunksCapsDeepReadAndOutput pins the Python parity: the deep read may
// fetch up to listChunksMaxDeep (80) chunks, but _exec_list_chunks admits only
// the first listChunksMaxOut (30) into the shared pool and shows the model the
// same 30 — so the pool holds 30, not 80.
func TestListChunksCapsDeepReadAndOutput(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.DocChunks = docChunksFor("doc-a", 100)
	ex := &searchExecutor{deps: deps, req: RunRequest{DatasetIDs: []string{"kb1"}, MaxLength: 1 << 20}}

	oc, err := ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a"})
	if err != nil {
		t.Fatalf("list_chunks: %v", err)
	}
	if len(kb.Chunks) != listChunksMaxOut {
		t.Errorf("kb.Chunks = %d, want admitted cap %d (Python _exec_list_chunks [:30])", len(kb.Chunks), listChunksMaxOut)
	}
	if len(oc.Payload) != listChunksMaxOut {
		t.Errorf("payload = %d, want output cap %d", len(oc.Payload), listChunksMaxOut)
	}
}

// TestEvidencePoolCapStopsAdmitting pins the PR's _EVIDENCE_POOL_CAP early-stop:
// once the shared pool reaches the cap, a search admits no further chunk, so its
// outcome collapses to MISS (nothing new was admitted) — a saturated session
// stops bloating the pool beyond what the SCA view can read.
func TestEvidencePoolCapStopsAdmitting(t *testing.T) {
	pre := make([]map[string]any, 0, evidencePoolCap)
	for i := 0; i < evidencePoolCap; i++ {
		pre = append(pre, map[string]any{"chunk_id": fmt.Sprintf("pre-%d", i), "content": "old"})
	}
	deps, kb := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	kb.Chunks = pre
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})
	oc, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if oc.Status != StatusMiss {
		t.Errorf("status = %s, want %s (a full pool admits nothing)", oc.Status, StatusMiss)
	}
	if len(kb.Chunks) != evidencePoolCap {
		t.Errorf("kb.Chunks = %d, want the pool to stay at the cap %d", len(kb.Chunks), evidencePoolCap)
	}
	// Dropping one chunk frees a slot and admission resumes.
	kb.Chunks = kb.Chunks[:evidencePoolCap-1]
	oc, err = ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("retrieve after freeing a slot: %v", err)
	}
	if oc.Status != StatusOK {
		t.Errorf("status after freeing a slot = %s, want %s", oc.Status, StatusOK)
	}
}

// TestWebSearchAdmitsToPool pins the web_search parity fix: web results merge
// into the SAME shared evidence pool as corpus hits (Python _exec_web_search →
// _admit_evidence), so downstream formalize/compose can cite them, and the
// outcome is REDUNDANT/OK/MISS like the corpus tools.
func TestWebSearchAdmitsToPool(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.WebSearch = stubWebSearch{results: []string{"web answer one", "web answer two", "web answer three"}}
	oc, err := WebSearchTool(context.Background(), deps, map[string]any{"": []any{"q1", "q2"}})
	if err != nil {
		t.Fatalf("web_search: %v", err)
	}
	if oc.Status != StatusOK {
		t.Errorf("status = %s, want %s", oc.Status, StatusOK)
	}
	if len(oc.Payload) != 3 {
		t.Errorf("payload = %d, want 3", len(oc.Payload))
	}
	if len(kb.Chunks) != 3 {
		t.Errorf("kb.Chunks = %d, want 3 admitted to pool", len(kb.Chunks))
	}
	if len(oc.EvidenceIDs) != 3 {
		t.Errorf("evidenceIDs = %d, want 3", len(oc.EvidenceIDs))
	}
	for _, p := range oc.Payload {
		m, ok := p.(map[string]any)
		if !ok || m["id"] == nil || m["content"] == nil {
			t.Fatalf("passage missing id/content: %#v", p)
		}
	}
}

// TestWebSearchDedupsAcrossQueries pins the Python parity: a passage that both
// queries return is admitted/shown only once (Python dedups by chunk_id across
// the two queries via a shared `seen` set).
func TestWebSearchDedupsAcrossQueries(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.WebSearch = stubWebSearch{results: []string{"dup passage", "unique one", "dup passage"}}
	oc, err := WebSearchTool(context.Background(), deps, map[string]any{"": []any{"q1", "q2"}})
	if err != nil {
		t.Fatalf("web_search: %v", err)
	}
	if len(oc.Payload) != 2 {
		t.Errorf("payload = %d, want 2 (duplicate collapsed)", len(oc.Payload))
	}
	if len(kb.Chunks) != 2 {
		t.Errorf("kb.Chunks = %d, want 2 admitted (duplicate not double-merged)", len(kb.Chunks))
	}
}

// TestListChunksSkipsEmptyChunkID pins Python parity (L3): a deep-read chunk
// with no chunk_id is skipped — neither admitted to the pool nor shown to the
// model (Python _exec_list_chunks: `if not cid: continue`).
func TestListChunksSkipsEmptyChunkID(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.DocChunks = docChunksWithIDs("doc-a", []string{"", "c1"}) // first has empty chunk_id
	ex := &searchExecutor{deps: deps, req: RunRequest{DatasetIDs: []string{"kb1"}, MaxLength: 8192}}

	oc, err := ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a"})
	if err != nil {
		t.Fatalf("list_chunks: %v", err)
	}
	if len(kb.Chunks) != 1 {
		t.Errorf("kb.Chunks = %d, want 1 (empty-id chunk skipped)", len(kb.Chunks))
	}
	if len(oc.Payload) != 1 {
		t.Fatalf("payload = %d, want 1", len(oc.Payload))
	}
	if m, ok := oc.Payload[0].(map[string]any); !ok || m["id"] == "" {
		t.Errorf("payload passage must not have empty id: %#v", oc.Payload[0])
	}
}

// docChunksFor returns a DocChunkLister that serves n chunks for the given doc id
// (filtering by doc id, like the production chunk-store reader).
func docChunksFor(docID string, n int) DocChunkLister {
	return docChunksStub{docID: docID, n: n}
}

type docChunksStub struct {
	docID string
	n     int
}

func (s docChunksStub) DocChunks(_ context.Context, req DocChunksRequest) ([]map[string]any, error) {
	if req.DocID != s.docID {
		return nil, nil
	}
	out := make([]map[string]any, 0, s.n)
	for i := 0; i < s.n; i++ {
		out = append(out, map[string]any{
			"chunk_id": fmt.Sprintf("c%d", i),
			"doc_id":   s.docID,
			"content":  "x",
		})
	}
	return out, nil
}

// docChunksWithIDs serves one chunk per id (empty strings stay empty), used to
// exercise the empty-chunk_id skip.
func docChunksWithIDs(docID string, ids []string) DocChunkLister {
	return docChunksIDsStub{docID: docID, ids: ids}
}

type docChunksIDsStub struct {
	docID string
	ids   []string
}

func (s docChunksIDsStub) DocChunks(_ context.Context, req DocChunksRequest) ([]map[string]any, error) {
	if req.DocID != s.docID {
		return nil, nil
	}
	out := make([]map[string]any, 0, len(s.ids))
	for _, id := range s.ids {
		out = append(out, map[string]any{
			"chunk_id": id,
			"doc_id":   s.docID,
			"content":  "x",
		})
	}
	return out, nil
}

type stubWebSearch struct{ results []string }

func (s stubWebSearch) Search(_ context.Context, _ []string) ([]string, error) {
	return s.results, nil
}

// TestSearchRedundantReturnsFullPayload pins fix #5: when every hit is already
// in the evidence pool the result is StatusRedundant — but Python _search_outcome
// still returns the FULL passages so the model sees what it already has. The Go
// port must not drop the payload to empty (which hid the evidence).
func TestSearchRedundantReturnsFullPayload(t *testing.T) {
	chunk := map[string]any{"content": "already known passage", "chunk_id": "c1"}
	deps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{chunk}})
	// Pre-seed the pool with the same chunk so the merge adds nothing new.
	deps.KB.Chunks = append(deps.KB.Chunks, chunk)
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})
	oc, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if oc.Status != StatusRedundant {
		t.Fatalf("status = %s, want %s", oc.Status, StatusRedundant)
	}
	if len(oc.Payload) != 1 {
		t.Fatalf("REDUNDANT payload = %v, want 1 full passage", oc.Payload)
	}
	p, ok := oc.Payload[0].(map[string]any)
	if !ok || p["content"] != "already known passage" {
		t.Errorf("REDUNDANT payload[0] = %#v, want the known passage", oc.Payload[0])
	}
}

func TestNavigateToolsRouteWithinSessionDocScope(t *testing.T) {
	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"navigate_tree", map[string]any{"query": "topic"}},
		{"navigate_structure", map[string]any{"query": "topic", "kind": "catalog"}},
	} {
		router := &stubNavRouter{docs: [][2]string{{"d1", "summary"}}}
		ex := &searchExecutor{deps: SearchDeps{
			NavRouter: router,
			DocScope:  []string{"sess1"},
			KbIDs:     []string{"kb1"},
			TenantID:  "t1",
		}}
		if _, err := ex.Execute(context.Background(), tool.name, tool.args); err != nil {
			t.Fatalf("%s: %v", tool.name, err)
		}
		if len(router.scope) != 1 || router.scope[0] != "sess1" {
			t.Errorf("%s: router doc scope = %v, want [sess1] (session ceiling)", tool.name, router.scope)
		}
	}
}

// TestDocInDatasetsRejectsForeignDocumentViaSubsetVerifier reproduces the
// production DocIDLookup.KnownDocIDs shape: a foreign doc is reported via an
// EMPTY known map (not a false entry). docInDatasets must still reject it
// (mirrors Python _resolve_doc_tenant returning None), not fail open.

// End-to-end wiring tests: one Run call, with a stubbed retriever and a
// scripted model, must move evidence from the backend into Kbinfos and (when a
// canvas state is attached) into the citation store.

// corpusRetriever answers every query from a fixed corpus.
type corpusRetriever struct{ calls []string }

func (c *corpusRetriever) Retrieve(_ context.Context, req RetrieveRequest) ([]map[string]any, error) {
	c.calls = append(c.calls, req.Query)
	return []map[string]any{{
		"chunk_id":   "c1",
		"content":    "Culdcept was created by OmiyaSoft and released in 1999.",
		"doc_id":     "doc-culdcept",
		"doc_name":   "Culdcept History",
		"dataset_id": "kb1",
	}}, nil
}

func TestSearchExecutorReportsMissAndRedundancy(t *testing.T) {
	kb := &Kbinfos{}
	sd := SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, KB: kb}
	ex := &searchExecutor{deps: sd, req: RunRequest{}}

	// First call: OK with new evidence.
	oc, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q1"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if oc.Status != StatusOK {
		t.Fatalf("first call status = %s, want ok", oc.Status)
	}
	if len(oc.EvidenceIDs) != 1 {
		t.Errorf("evidence ids = %v, want 1", oc.EvidenceIDs)
	}

	// Same query again: every hit is already in the pool -> REDUNDANT, not ok.
	oc, _ = ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q1"})
	if oc.Status != StatusRedundant {
		t.Errorf("repeat call status = %s, want redundant", oc.Status)
	}

	// Missing query -> MISS/no_doc, NOT a bad-args error: Python
	// _arg_query_list yields [] and _search_outcome([]) is a miss
	// (action_session.py:_search_outcome).
	oc, _ = ex.Execute(context.Background(), "retrieve", map[string]any{})
	if oc.Status != StatusMiss || oc.Reason != ReasonNoDoc {
		t.Errorf("missing query: got (%s,%s), want (miss,no_doc)", oc.Status, oc.Reason)
	}

	// A genuinely unwired tool -> miss (valid tool, nothing reached), never an
	// error, so the model falls back instead of stalling.
	oc, _ = ex.Execute(context.Background(), "wiki_query", map[string]any{"query": "topic"})
	if oc.Status != StatusMiss {
		t.Errorf("unwired tool status = %s, want miss", oc.Status)
	}
}

// TestSearchRecordsDocAggsForReferences: the search tools are the only writer of
// kbinfos["doc_aggs"], which the chat pipeline turns into the answer's
// document/reference list.
func TestSearchRecordsDocAggsForReferences(t *testing.T) {
	kb := &Kbinfos{}
	ex := &searchExecutor{deps: SearchDeps{Backend: &corpusRetriever{}, KbIDs: []string{"kb1"}, KB: kb}, req: RunRequest{}}

	if _, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "Culdcept"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(kb.DocAggs) != 1 {
		t.Fatalf("kb.DocAggs = %v, want the searched document recorded", kb.DocAggs)
	}
	if got := kb.DocAggs[0]["doc_id"]; got != "doc-culdcept" {
		t.Errorf("doc_agg doc_id = %v, want doc-culdcept", got)
	}

	// A REDUNDANT repeat (nothing new admitted) must not duplicate the agg.
	if _, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "Culdcept"}); err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if len(kb.DocAggs) != 1 {
		t.Errorf("kb.DocAggs = %v, want 1 (deduped by doc_id)", kb.DocAggs)
	}
}

// fakeWikiRetriever returns a fixed compiled wiki page.
type fakeWikiRetriever struct{}

func (f *fakeWikiRetriever) SearchWiki(_ context.Context, question string, keywords []string, topN int) ([]WikiPage, error) {
	return []WikiPage{{
		ChunkID: "w1", DocID: "wdoc1", DocName: "Synthesis", Title: "Culdcept Overview",
		Content: "Culdcept is a board game by OmiyaSoft.", Score: 0.9,
	}}, nil
}

func TestWikiQueryWiredReturnsPages(t *testing.T) {
	// When a WikiRetriever is wired, wiki_query returns the parsed page payload
	// in the same shape Python's action layer consumes.
	ex := &searchExecutor{deps: SearchDeps{WikiRetriever: &fakeWikiRetriever{}}, req: RunRequest{}}

	oc, err := ex.Execute(context.Background(), "wiki_query", map[string]any{"query": "Culdcept overview"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if oc.Status != StatusOK {
		t.Fatalf("status = %s, want ok", oc.Status)
	}
	if n, _ := toInt(oc.Metrics["n_hits"]); n != 1 {
		t.Errorf("n_hits = %v, want 1", oc.Metrics["n_hits"])
	}
	hit, ok := oc.Payload[0].(map[string]any)
	if !ok {
		t.Fatalf("payload[0] not a map: %T", oc.Payload[0])
	}
	for _, key := range []string{"chunk_id", "doc_id", "docnm_kwd", "title", "content", "query"} {
		if _, present := hit[key]; !present {
			t.Errorf("payload missing key %q", key)
		}
	}
	if hit["content"] != "Culdcept is a board game by OmiyaSoft." {
		t.Errorf("content = %v", hit["content"])
	}
}

func TestWikiQuerySchemaIsUnplugged(t *testing.T) {
	// wiki_query is ported as an UNPLUGGED extension seam: the handler exists
	// (TestWikiQueryWiredReturnsPages), but it is NOT part of any mode's active
	// tool set (removed from allTools) and no SearchWiki backend is wired in
	// production, so it must never appear in the advertised surface — mirroring
	// Python, which keeps tools/exploration.py::wiki_query but never registers it
	// in the action session.
	for _, mode := range []string{"medium", "high", "ultra"} {
		ts := &Toolset{ThinkingMode: mode}
		if spec, ok := findSpec(ts.ActiveToolSpecs(), "wiki_query"); ok {
			t.Errorf("wiki_query surfaced in mode %q: %+v", mode, spec)
		}
	}
}

// findSpec is a test helper mirroring ActiveToolSpecs lookup.
func findSpec(specs []ToolSpec, name string) (ToolSpec, bool) {
	for _, s := range specs {
		if s.Function.Name == name {
			return s, true
		}
	}
	return ToolSpec{}, false
}

func TestListChunksReadsFromEvidencePool(t *testing.T) {
	// list_chunks deep-reads a document. With no doc-store reader wired it scans
	// the accumulated evidence pool; an unknown/missing doc_id is a query-level
	// MISS (mirroring Python list_chunks("") → _search_outcome), never a dataset
	// EMPTY and never a hard error.
	kb := &Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "doc_id": "doc-a", "content": "first"},
		{"chunk_id": "c2", "doc_id": "doc-b", "content": "other"},
		{"chunk_id": "c3", "doc_id": "doc-a", "content": "second"},
	}}
	ex := &searchExecutor{deps: SearchDeps{KB: kb}}

	oc, _ := ex.Execute(context.Background(), "list_chunks", map[string]any{"doc_id": "doc-a"})
	// Already in evidence → REDUNDANT (nothing new admitted), but the passages
	// are still returned (mirrors Python _search_outcome).
	if oc.Status != StatusRedundant {
		t.Fatalf("status = %s, want redundant", oc.Status)
	}
	if len(oc.Payload) != 2 {
		t.Errorf("payload = %d, want 2 chunks of doc-a", len(oc.Payload))
	}
	// Evidence ids are the CHUNK ids — Python _admit_evidence does
	// `ids.append(cid)`, not a pool position.
	if len(oc.EvidenceIDs) != 2 || oc.EvidenceIDs[0] != "c1" || oc.EvidenceIDs[1] != "c3" {
		t.Errorf("evidence ids = %v, want [c1 c3]", oc.EvidenceIDs)
	}

	oc, _ = ex.Execute(context.Background(), "list_chunks", map[string]any{"doc_id": "doc-zz"})
	if oc.Status != StatusMiss {
		t.Errorf("unknown doc status = %s, want miss", oc.Status)
	}
	// Missing doc_id → MISS (Python list_chunks("") → empty → _search_outcome).
	oc, _ = ex.Execute(context.Background(), "list_chunks", map[string]any{})
	if oc.Status != StatusMiss {
		t.Errorf("no doc_id: got %s, want miss", oc.Status)
	}
}

func TestToolQueriesAcceptsBothShapes(t *testing.T) {
	// Models emit `query` as a string OR a list, per the advertised schema.
	if got := toolQueries(map[string]any{"query": "single"}); len(got) != 1 || got[0] != "single" {
		t.Errorf("string form = %v", got)
	}
	if got := toolQueries(map[string]any{"query": []any{"a", "", "b"}}); len(got) != 2 {
		t.Errorf("list form = %v, want blanks dropped", got)
	}
	if got := toolQueries(map[string]any{"query": []string{"a", "b"}}); len(got) != 2 {
		t.Errorf("[]string form = %v", got)
	}
	if got := toolQueries(map[string]any{}); got != nil {
		t.Errorf("absent query = %v, want nil", got)
	}
	// The `q` alias some models emit.
	if got := toolQueries(map[string]any{"q": "alias"}); len(got) != 1 || got[0] != "alias" {
		t.Errorf("q alias = %v", got)
	}
}

func TestPublishReferencesSkipsWithoutCanvasState(t *testing.T) {
	// No canvas state attached: publishing is a silent no-op (best-effort),
	// which is what happens in unit tests and non-canvas callers.
	kb := &Kbinfos{Chunks: []map[string]any{{"content": "x", "doc_id": "d1"}}}
	PublishReferences(context.Background(), kb) // must not panic
}

func TestPassageFromChunkTruncatesContent(t *testing.T) {
	long := strings.Repeat("word ", 500)
	p := passageFromChunk(map[string]any{
		"chunk_id": "c1", "doc_id": "d1", "docnm_kwd": "Title", "content": long,
	})
	content, _ := p["content"].(string)
	// Non-table chunk is cut to 1200 code points (Python _admit_evidence's
	// `_ct[:1200]`), a plain slice — so NO trailing ellipsis is added.
	if n := len([]rune(content)); n != 1200 {
		t.Errorf("content length = %d runes, want 1200", n)
	}
	if strings.HasSuffix(content, "...") {
		t.Errorf("Python _ct[:1200] adds no ellipsis marker: %q", content)
	}
	// Keys mirror Python _admit_evidence exactly: {"id","content","doc_id"} — the
	// id must live under "id", which is what the drill merge reads back.
	if p["doc_id"] != "d1" || p["id"] != "c1" {
		t.Errorf("passage = %v", p)
	}
}
