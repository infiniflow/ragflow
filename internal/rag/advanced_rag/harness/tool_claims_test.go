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

package harness

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// recordingClaimEngine implements engine.DocEngine by serving Search from a
// fixed row set and recording every request; the rest of the interface is
// promoted from a nil field and never called by these tests.
type recordingClaimEngine struct {
	engine.DocEngine
	requests []*types.SearchRequest
	rows     []map[string]interface{}
}

func (e *recordingClaimEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.requests = append(e.requests, req)
	return &types.SearchResult{Chunks: e.rows}, nil
}

// claimRow builds one claim store row in the shape the compilers write.
func claimRow(docID, name, description, quote string, similarity float64, chunkIDs []string) map[string]interface{} {
	payload := map[string]any{"name": name, "type": "claim"}
	if description != "" {
		payload["description"] = description
	}
	if quote != "" {
		payload["evidence"] = []any{map[string]any{"quote": quote}}
	}
	if chunkIDs != nil {
		payload["source_chunk_ids"] = chunkIDs
	}
	raw, _ := json.Marshal(payload)
	return map[string]interface{}{
		"content_with_weight": string(raw),
		"source_chunk_ids":    chunkIDs,
		"doc_id":              docID,
		"similarity":          similarity,
	}
}

func resetClaimCaches() {
	claimRecallMu.Lock()
	claimRecallCache = map[string]claimCacheEntry{}
	claimRecallMu.Unlock()
	docClaimMu.Lock()
	docClaimCache = map[string]docClaimCacheEntry{}
	docClaimMu.Unlock()
	compilationMu.Lock()
	compilationCache = map[string]compilationProbe{}
	compilationMu.Unlock()
}

// TestRecallDocClaimHitsFiltersAndMaps pins the store contract and the output
// mapping of the per-document claim leg (Python navigation.py:_recall_claim_hits):
// doc-scope + claim-row filters, sorted compile_kwd when kinds are given, and
// hits shaped as {chunk_id, score, rank, name, description-or-name, evidence}.
func TestRecallDocClaimHitsFiltersAndMaps(t *testing.T) {
	resetClaimCaches()
	de := &recordingClaimEngine{rows: []map[string]interface{}{
		claimRow("doc-1", "The tower opened in 1889", "", "The tower opened in 1889.", 0.9, []string{"c1", "c2"}),
	}}
	deps := SearchDeps{TenantID: "tenant-1", IndexName: "idx", DocEngine: de, KbIDs: []string{"kb-1"}}

	hits := RecallDocClaimHits(context.Background(), deps, "When did the tower open?", "doc-1", []string{"tree", "raptor"}, nil, 8)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	h := hits[0]
	if h.ChunkID != "c1" {
		t.Errorf("ChunkID = %q, want the first source chunk c1", h.ChunkID)
	}
	if h.Score != 0.9 || h.Rank != 1 {
		t.Errorf("Score/Rank = %v/%d, want 0.9/1", h.Score, h.Rank)
	}
	if h.Name != "The tower opened in 1889" {
		t.Errorf("Name = %q", h.Name)
	}
	// Python: description = h["description"] or h["name"].
	if h.Description != h.Name {
		t.Errorf("Description = %q, want the name fallback", h.Description)
	}
	wantEv := []map[string]any{{"quote": "The tower opened in 1889."}}
	if !reflect.DeepEqual(h.Evidence, wantEv) {
		t.Errorf("Evidence = %v, want %v", h.Evidence, wantEv)
	}

	// Store contract: one BM25 leg (no qvec), doc-scope claim filters, sorted
	// compile_kwd, the wide field list, limit floored at 32.
	if len(de.requests) != 1 {
		t.Fatalf("store calls = %d, want 1 (BM25 leg only)", len(de.requests))
	}
	req := de.requests[0]
	wantFilter := map[string]interface{}{
		"doc_id":          []string{"doc-1"},
		"entity_type_kwd": []string{"claim"},
		"scope_kwd":       []string{"doc"},
		"compile_kwd":     []string{"raptor", "tree"},
	}
	if !reflect.DeepEqual(req.Filter, wantFilter) {
		t.Errorf("Filter = %v, want %v", req.Filter, wantFilter)
	}
	for _, f := range []string{"content_with_weight", "source_chunk_ids", "entity_type_kwd", "compile_kwd"} {
		if !sliceContains(req.SelectFields, f) {
			t.Errorf("SelectFields missing %q: %v", f, req.SelectFields)
		}
	}
	if req.Limit != claimRecallLegFloor {
		t.Errorf("Limit = %d, want the %d floor", req.Limit, claimRecallLegFloor)
	}
	if len(req.MatchExprs) != 1 {
		t.Fatalf("MatchExprs = %d, want the single BM25 leg", len(req.MatchExprs))
	}
	text, ok := req.MatchExprs[0].(*types.MatchTextExpr)
	if !ok {
		t.Fatalf("MatchExprs[0] is %T, want *types.MatchTextExpr", req.MatchExprs[0])
	}
	if text.MatchingText != "When did the tower open?" {
		t.Errorf("BM25 leg text = %q, want the raw query", text.MatchingText)
	}
}

// TestRecallDocClaimHitsDenseLeg pins the KNN leg: a query vector turns into
// one MatchDenseExpr over q_<dim>_vec ahead of the BM25 leg.
func TestRecallDocClaimHitsDenseLeg(t *testing.T) {
	resetClaimCaches()
	de := &recordingClaimEngine{}
	deps := SearchDeps{TenantID: "tenant-1", DocEngine: de}
	qvec := []float64{0.1, 0.2, 0.3}
	RecallDocClaimHits(context.Background(), deps, "q", "doc-1", nil, qvec, 8)
	if len(de.requests) != 2 {
		t.Fatalf("store calls = %d, want 2 (KNN + BM25 legs)", len(de.requests))
	}
	dense, ok := de.requests[0].MatchExprs[0].(*types.MatchDenseExpr)
	if !ok {
		t.Fatalf("first leg is %T, want *types.MatchDenseExpr", de.requests[0].MatchExprs[0])
	}
	if dense.VectorColumnName != "q_3_vec" {
		t.Errorf("vector column = %q, want q_3_vec", dense.VectorColumnName)
	}
	if !reflect.DeepEqual(dense.EmbeddingData, qvec) {
		t.Errorf("embedding data = %v, want %v", dense.EmbeddingData, qvec)
	}
	// No kinds: no compile_kwd filter, row types default to claim.
	if _, ok := de.requests[0].Filter["compile_kwd"]; ok {
		t.Errorf("compile_kwd filter present without kinds: %v", de.requests[0].Filter)
	}
	if !reflect.DeepEqual(de.requests[0].Filter["entity_type_kwd"], []string{"claim"}) {
		t.Errorf("entity_type_kwd = %v, want [claim]", de.requests[0].Filter["entity_type_kwd"])
	}
}

// TestRecallDocClaimHitsMemo pins the (doc_id, query) memo: the drill re-issues
// navigate_structure, so the same pair must not cost two more store
// round-trips, while a different query or doc must miss. Empty results are NOT
// cached (Python returns before the cache write).
func TestRecallDocClaimHitsMemo(t *testing.T) {
	resetClaimCaches()
	de := &recordingClaimEngine{rows: []map[string]interface{}{
		claimRow("doc-1", "claim a", "", "", 0.5, []string{"c1"}),
	}}
	deps := SearchDeps{TenantID: "tenant-1", DocEngine: de}
	ctx := context.Background()

	RecallDocClaimHits(ctx, deps, "same query", "doc-1", nil, nil, 8)
	RecallDocClaimHits(ctx, deps, "Same Query ", "doc-1", nil, nil, 8)
	if len(de.requests) != 1 {
		t.Fatalf("store calls after repeat = %d, want 1 (memo hit)", len(de.requests))
	}
	RecallDocClaimHits(ctx, deps, "other query", "doc-1", nil, nil, 8)
	if len(de.requests) != 2 {
		t.Fatalf("store calls after new query = %d, want 2", len(de.requests))
	}
	RecallDocClaimHits(ctx, deps, "same query", "doc-2", nil, nil, 8)
	if len(de.requests) != 3 {
		t.Fatalf("store calls after new doc = %d, want 3", len(de.requests))
	}

	empty := &recordingClaimEngine{}
	emptyDeps := SearchDeps{TenantID: "tenant-1", DocEngine: empty}
	RecallDocClaimHits(ctx, emptyDeps, "nothing here", "doc-9", nil, nil, 8)
	RecallDocClaimHits(ctx, emptyDeps, "nothing here", "doc-9", nil, nil, 8)
	if len(empty.requests) != 2 {
		t.Fatalf("empty result must not be memoized; store calls = %d, want 2", len(empty.requests))
	}
}

// TestClaimAggRouterClaimsDecide pins the claim-first routing order: documents
// ranked by the damped claim DocScore with the best claim similarity as the
// reported tie-break, nav summaries attached, and the chunk_agg fallback NEVER
// consulted while the claim leg hits.
func TestClaimAggRouterClaimsDecide(t *testing.T) {
	resetClaimCaches()
	de := &recordingClaimEngine{rows: []map[string]interface{}{
		// doc-a: one strong claim -> DocScore 0.9/sqrt(2) ≈ 0.636.
		claimRow("doc-a", "claim a", "", "quote a", 0.9, []string{"c1"}),
		// doc-b: two weaker claims -> DocScore 0.9/sqrt(3) ≈ 0.520.
		claimRow("doc-b", "claim b1", "", "", 0.5, []string{"c2"}),
		claimRow("doc-b", "claim b2", "", "", 0.4, []string{"c3"}),
	}}
	fallback := &fallbackRouter{}
	summaries := func(_ context.Context, _, _ string, docIDs []string) map[string]string {
		out := map[string]string{}
		for _, d := range docIDs {
			out[d] = "summary of " + d
		}
		return out
	}
	r := &ClaimAggRouter{
		Deps:      SearchDeps{TenantID: "tenant-1", DocEngine: de},
		Fallback:  fallback,
		Summarize: summaries,
	}
	routed, err := r.Route(context.Background(), "tenant-1", "kb-1", "tower height", nil, 8)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if fallback.called {
		t.Fatal("chunk_agg fallback must not run while the claim leg hits")
	}
	if len(routed) != 2 {
		t.Fatalf("routed = %v, want 2 docs", routed)
	}
	if routed[0][0] != "doc-a" || routed[1][0] != "doc-b" {
		t.Errorf("order = %v, want [doc-a doc-b] (damped DocScore)", routed)
	}
	if routed[0][1] != "summary of doc-a" {
		t.Errorf("summary = %q, want the nav_doc summary", routed[0][1])
	}
}

// TestClaimAggRouterDocScope pins the doc_scope filter: out-of-scope documents
// never surface, and a scope that empties the claim leg falls back to the
// chunk leg (Python filters the store condition, so a scoped miss is an empty
// leg, not a routing verdict).
func TestClaimAggRouterDocScope(t *testing.T) {
	resetClaimCaches()
	de := &recordingClaimEngine{rows: []map[string]interface{}{
		claimRow("doc-a", "claim a", "", "", 0.9, []string{"c1"}),
		claimRow("doc-b", "claim b", "", "", 0.8, []string{"c2"}),
	}}
	fallback := &fallbackRouter{next: [][2]string{{"doc-fallback", ""}}}
	r := &ClaimAggRouter{
		Deps:     SearchDeps{TenantID: "tenant-1", DocEngine: de},
		Fallback: fallback,
	}
	routed, err := r.Route(context.Background(), "tenant-1", "kb-1", "q", []string{"doc-b"}, 8)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(routed) != 1 || routed[0][0] != "doc-b" {
		t.Errorf("routed = %v, want only doc-b", routed)
	}
	if fallback.called {
		t.Fatal("fallback must not run when the scoped claim leg still hits")
	}

	fallback2 := &fallbackRouter{next: [][2]string{{"doc-fallback", ""}}}
	r2 := &ClaimAggRouter{Deps: SearchDeps{TenantID: "tenant-1", DocEngine: de}, Fallback: fallback2}
	routed2, err := r2.Route(context.Background(), "tenant-1", "kb-1", "q", []string{"doc-elsewhere"}, 8)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !fallback2.called || len(routed2) != 1 || routed2[0][0] != "doc-fallback" {
		t.Errorf("emptied scoped leg must fall back, got routed=%v called=%v", routed2, fallback2.called)
	}
}

// TestClaimAggRouterFallsBackOnEmpty pins the empty-leg contract: no claim
// rows (a raptor-less or pre-claim dataset) is a legitimate empty leg, so the
// chunk_agg fallback decides.
func TestClaimAggRouterFallsBackOnEmpty(t *testing.T) {
	resetClaimCaches()
	de := &recordingClaimEngine{}
	fallback := &fallbackRouter{next: [][2]string{{"doc-chunk", "chunk summary"}}}
	r := &ClaimAggRouter{
		Deps:     SearchDeps{TenantID: "tenant-1", DocEngine: de},
		Fallback: fallback,
	}
	routed, err := r.Route(context.Background(), "tenant-1", "kb-1", "q", nil, 8)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if !fallback.called {
		t.Fatal("empty claim leg must consult the chunk_agg fallback")
	}
	if len(routed) != 1 || routed[0][0] != "doc-chunk" {
		t.Errorf("routed = %v, want the fallback's result", routed)
	}
}

// fallbackRouter stands in for the chunk_agg router.
type fallbackRouter struct {
	called bool
	next   [][2]string
}

func (r *fallbackRouter) Route(_ context.Context, _, _, _ string, _ []string, _ int) ([][2]string, error) {
	r.called = true
	return r.next, nil
}

// TestPublishClaimHits pins the evidence-pool publish (Python
// _publish_claim_hits): one pseudo chunk per claim under the SAME
// "claim_"+md5(doc:name) id the session prefetch writes, verbatim evidence
// capped at ClaimEvidenceChars, the claim's own chunk pointer as
// source_chunk_ids, deduped against the live pool.
func TestPublishClaimHits(t *testing.T) {
	kb := &Kbinfos{}
	deps := SearchDeps{KB: kb}
	hits := []DocClaimHit{
		{ChunkID: "c1", Rank: 2, Name: "Claim A", Description: "Claim A", Evidence: []map[string]any{{"quote": strings.Repeat("q", 2000)}}},
		{ChunkID: "", Rank: 3, Name: "Claim B", Description: "a different description"},
	}
	if got := publishClaimHits(deps, hits, "doc-1"); got != 2 {
		t.Fatalf("published = %d, want 2", got)
	}
	if got := publishClaimHits(deps, hits, "doc-1"); got != 0 {
		t.Fatalf("re-publish must dedup to 0, got %d", got)
	}
	if len(kb.Chunks) != 2 {
		t.Fatalf("pool = %d entries, want 2", len(kb.Chunks))
	}
	a := kb.Chunks[0]
	if a["chunk_id"] != claimHitID(&ClaimHit{DocID: "doc-1", Name: "Claim A"}) {
		t.Errorf("chunk_id = %v, want the shared claim id scheme", a["chunk_id"])
	}
	content, _ := a["content_with_weight"].(string)
	if !strings.HasPrefix(content, "[claim #2] Claim A") {
		t.Errorf("content = %q, want the [claim #rank] prefix", content)
	}
	if !strings.Contains(content, "\nEvidence (verbatim): \"") {
		t.Errorf("content missing the verbatim evidence line:\n%s", content)
	}
	if len(content) > 1200 {
		t.Errorf("content length = %d, want the 1200 cap", len(content))
	}
	if !reflect.DeepEqual(a["source_chunk_ids"], []string{"c1"}) {
		t.Errorf("source_chunk_ids = %v, want [c1]", a["source_chunk_ids"])
	}
	// Rank-3 claim with no chunk pointer and no quote: empty source list,
	// description rendered when it differs from the name.
	b := kb.Chunks[1]
	if !reflect.DeepEqual(b["source_chunk_ids"], []string{}) {
		t.Errorf("source_chunk_ids = %v, want []", b["source_chunk_ids"])
	}
	if !strings.Contains(b["content_with_weight"].(string), "Claim B — a different description") {
		t.Errorf("content = %q, want the description dash form", b["content_with_weight"])
	}
}

// TestLoadChunksForIDs pins the directed source-chunk fetch (Python
// _load_chunks_for_ids): an id-filtered store read that maps rows back to
// pool-shaped chunks.
func TestLoadChunksForIDs(t *testing.T) {
	de := &recordingClaimEngine{rows: []map[string]interface{}{
		{"id": "c1", "content_with_weight": "full text one", "doc_id": "doc-1", "docnm_kwd": "a.pdf"},
	}}
	deps := SearchDeps{TenantID: "tenant-1", IndexName: "idx", DocEngine: de, KbIDs: []string{"kb-1"}}
	out := LoadChunksForIDs(context.Background(), deps, []string{"c1", " "})
	if len(out) != 1 {
		t.Fatalf("fetched = %d, want 1", len(out))
	}
	if out[0]["chunk_id"] != "c1" || out[0]["content_with_weight"] != "full text one" {
		t.Errorf("fetched row = %v", out[0])
	}
	if len(de.requests) != 1 || !reflect.DeepEqual(de.requests[0].Filter["id"], []string{"c1"}) {
		t.Errorf("request = %+v, want one id-filtered read for [c1]", de.requests[0])
	}
	if LoadChunksForIDs(context.Background(), deps, nil) != nil {
		t.Error("empty ids must fetch nothing")
	}
}
