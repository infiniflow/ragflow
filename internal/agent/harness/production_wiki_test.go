package harness

import (
	"context"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/agent/component"
	"ragflow/internal/service/wikisearch"
)

// fakeWikiSvcHarness is a deterministic wikisearch.Service double for the
// production runner.
type fakeWikiSvcHarness struct {
	available bool
	// pages are pre-shaped page chunks (map form) returned by QueryPages.
	pages []map[string]interface{}
	// backfill maps chunk id -> evidence content for BackfillChunks.
	backfill   map[string]string
	calls      []string
	queryCount int
	mu         sync.Mutex
}

func (f *fakeWikiSvcHarness) AvailableFor(_ context.Context, _ string, _ []string) bool {
	return f.available
}

func (f *fakeWikiSvcHarness) QueryPages(_ context.Context, _ string, _ []string, query, _ string, _ int) (wikisearch.SearchResult, error) {
	f.mu.Lock()
	f.queryCount++
	f.calls = append(f.calls, query)
	f.mu.Unlock()
	res := wikisearch.SearchResult{Chunks: append([]map[string]interface{}(nil), f.pages...), DocAggs: []map[string]interface{}{}}
	for _, c := range res.Chunks {
		if docID, _ := c["doc_id"].(string); docID != "" {
			res.DocAggs = append(res.DocAggs, map[string]interface{}{"doc_id": docID, "doc_name": c["docnm_kwd"]})
		}
	}
	return res, nil
}

func (f *fakeWikiSvcHarness) seenQueries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// backfill is the fake's per-id evidence map (chunk id -> content).
func (f *fakeWikiSvcHarness) BackfillChunks(_ context.Context, _ string, _ []string, chunkIDs []string) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(chunkIDs))
	for _, id := range chunkIDs {
		content, ok := f.backfill[id]
		if !ok {
			continue
		}
		out = append(out, map[string]interface{}{
			"chunk_id": id, "content_with_weight": content, "doc_id": "d1", "docnm_kwd": "Doc", "dataset_id": "kb1",
		})
	}
	return out, nil
}

// wikiRouteChat returns a route JSON suggesting wiki for the route stage and a
// plain final answer otherwise.
type wikiRouteChat struct{}

func (wikiRouteChat) Invoke(_ context.Context, _ *gorm.DB, req component.ChatInvokeRequest) (*component.ChatInvokeResponse, error) {
	// The route stage carries the route prompt (with "suggests_compilation") as
	// a system message; detect it across any message (system or user).
	isRoute := false
	for _, m := range req.Messages {
		if strings.Contains(m.Content, "suggests_compilation") {
			isRoute = true
			break
		}
	}
	if isRoute {
		return &component.ChatInvokeResponse{Content: `{"question_type":"analytical","requires_decomposition":false,"suggests_compilation":"wiki"}`}, nil
	}
	return &component.ChatInvokeResponse{Content: "final wiki answer"}, nil
}

func installWikiRouteChat(t *testing.T) {
	t.Helper()
	component.SetDefaultChatInvoker(wikiRouteChat{})
	t.Cleanup(func() { component.SetDefaultChatInvoker(nil) })
}

// TestProductionRunner_WikiPreferred_WhenSuggested drives a low-mode run with a
// wiki suggestion and an available wiki service, and asserts the wiki service is
// queried (P5: route suggestion selects the wiki path).
func TestProductionRunner_WikiPreferred_WhenSuggested(t *testing.T) {
	installWikiRouteChat(t)
	hybrid := &fakeInvokableTool{name: "hybrid_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[]}`
	}}
	wikiSvc := &fakeWikiSvcHarness{available: true, pages: []map[string]interface{}{
		{"chunk_id": "wiki/entity/alpha", "content_with_weight": "# Alpha", "doc_id": "kb1", "docnm_kwd": "Alpha", "wiki_slug_kwd": "entity/alpha", "dataset_id": "kb1"},
	}}
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, hybrid, nil)
	runner.wikiSvc = wikiSvc
	res := runner.Run(t.Context(), "What is Alpha?", "", "low", "")
	if res.FinalAnswer != "final wiki answer" {
		t.Errorf("final answer = %q, want wiki chat output", res.FinalAnswer)
	}
	if len(wikiSvc.seenQueries()) == 0 {
		t.Errorf("wiki service was not queried despite a wiki suggestion")
	}
}

// TestProductionRunner_WikiEmpty_FallsBackToHybrid asserts that when the wiki
// service yields nothing, the runner falls back to hybrid search (P5 must not
// discard the general retrieval fallback).
func TestProductionRunner_WikiEmpty_FallsBackToHybrid(t *testing.T) {
	installWikiRouteChat(t)
	hybrid := &fakeInvokableTool{name: "hybrid_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[{"chunk_id":"c1","content_with_weight":"hybrid evidence"}]}`
	}}
	wikiSvc := &fakeWikiSvcHarness{available: true} // no pages
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, hybrid, nil)
	runner.wikiSvc = wikiSvc
	res := runner.Run(t.Context(), "What is Alpha?", "", "low", "")
	if len(wikiSvc.seenQueries()) == 0 {
		t.Errorf("wiki service should have been attempted")
	}
	if !strings.Contains(hybrid.args(), `"query":"What is Alpha?"`) {
		t.Errorf("hybrid fallback was not invoked after empty wiki; args=%s", hybrid.args())
	}
	if res.FinalAnswer == "" {
		t.Errorf("expected a final answer from the hybrid fallback")
	}
}

// TestProductionRunner_NoWikiWithoutSuggestion asserts the wiki service is NOT
// queried when the route does not suggest wiki.
func TestProductionRunner_NoWikiWithoutSuggestion(t *testing.T) {
	// route chat returns no suggestion for a generic route call
	installRouteChat(t) // routeChat returns plain text -> route falls back (no suggestion)
	hybrid := &fakeInvokableTool{name: "hybrid_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[{"chunk_id":"c1","content_with_weight":"evidence"}]}`
	}}
	wikiSvc := &fakeWikiSvcHarness{available: true, pages: []map[string]interface{}{
		{"chunk_id": "wiki/s", "content_with_weight": "c", "doc_id": "kb1", "docnm_kwd": "T", "wiki_slug_kwd": "s", "dataset_id": "kb1"},
	}}
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, hybrid, nil)
	runner.wikiSvc = wikiSvc
	runner.Run(t.Context(), "What is Alpha?", "", "low", "")
	if len(wikiSvc.seenQueries()) != 0 {
		t.Errorf("wiki service must not be queried without a wiki suggestion; calls=%v", wikiSvc.seenQueries())
	}
}

// TestProductionRunner_WebFallback_UnconfiguredIsNoop asserts that with no web
// tool configured the runner never attempts a web call (P8 gate).
func TestProductionRunner_WebFallback_UnconfiguredIsNoop(t *testing.T) {
	webTool := &fakeInvokableTool{name: "web_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[{"chunk_id":"w1","content_with_weight":"web evidence"}]}`
	}}
	hybrid := &fakeInvokableTool{name: "hybrid_search", fn: func(_ context.Context, _ string) string {
		return `{"chunks":[]}`
	}}
	// webTool is NOT set on the runner -> the runner must not invoke it.
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, hybrid, nil)
	// With webTool nil, webFallbackFn returns the hybrid path unchanged, so the
	// web tool is never called.
	fn := runner.webFallbackFn(func(ctx context.Context, q, k string) ([]map[string]interface{}, []map[string]interface{}) {
		return nil, nil
	})
	chunks, _ := fn(t.Context(), "q", "k")
	if len(chunks) != 0 {
		t.Errorf("expected no chunks from the no-web fallback path")
	}
	if webTool.args() != "" {
		t.Errorf("web tool must not be called when unconfigured; args=%s", webTool.args())
	}
}

// TestProductionRunner_WebFallback_TavilyResultsNormalized asserts R2: the web
// fallback consumes the Tavily `{"results":[...]}` envelope (tavily.go contract)
// and normalizes each result into an agent evidence chunk with a doc_id
// reference, not the agent `chunks` shape.
func TestProductionRunner_WebFallback_TavilyResultsNormalized(t *testing.T) {
	webTool := &fakeInvokableTool{name: "web_search", fn: func(_ context.Context, _ string) string {
		return `{"results":[{"title":"Alpha docs","url":"https://example.com/x","content":"web evidence body","source":"example.com"}]}`
	}}
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, nil, nil)
	runner.webTool = webTool
	fn := runner.webFallbackFn(func(ctx context.Context, q, k string) ([]map[string]interface{}, []map[string]interface{}) {
		return nil, nil
	})
	chunks, _ := fn(t.Context(), "q", "k")
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1 normalized Tavily result", len(chunks))
	}
	if chunks[0]["content_with_weight"] != "web evidence body" {
		t.Errorf("content = %v, want web evidence body", chunks[0]["content_with_weight"])
	}
	if chunks[0]["doc_id"] == "" || !strings.Contains(chunks[0]["doc_id"].(string), "https://example.com/x") {
		t.Errorf("doc_id must reference the source url: %v", chunks[0]["doc_id"])
	}
	if chunks[0]["source"] != "web" {
		t.Errorf("source = %v, want web", chunks[0]["source"])
	}
	if webTool.args() == "" {
		t.Errorf("web tool was not invoked when hybrid was empty")
	}
}

// TestModeAllowsWeb asserts R4 gating: only modes whose AvailableTools include
// web_search (high/ultra) allow web fallback; low/medium/unknown do not.
func TestModeAllowsWeb(t *testing.T) {
	if !modeAllowsWeb("high") || !modeAllowsWeb("ultra") {
		t.Errorf("high/ultra must allow web search")
	}
	for _, m := range []string{"low", "medium", "fast", "unknown"} {
		if modeAllowsWeb(m) {
			t.Errorf("mode %q must NOT allow web search", m)
		}
	}
}

// TestProductionRunner_CompiledEvidenceExpansion asserts P7/R3: page hits
// carrying source_chunk_ids get the ORIGINAL chunks fetched by id (via the wiki
// service BackfillChunks) appended after the page results, deduped.
func TestProductionRunner_CompiledEvidenceExpansion(t *testing.T) {
	wikiSvc := &fakeWikiSvcHarness{available: true, backfill: map[string]string{"c1": "raw chunk 1", "c2": "raw chunk 2"}}
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, nil, nil)
	runner.wikiSvc = wikiSvc
	chunks := []map[string]interface{}{
		{
			"chunk_id": "wiki/s", "content_with_weight": "# Page", "doc_id": "d1", "docnm_kwd": "Page", "dataset_id": "kb1",
			"source_chunk_ids": []interface{}{"c1", "c2"},
		},
	}
	merged, aggs := runner.expandCompiledEvidence(t.Context(), chunks, []map[string]interface{}{})
	if len(merged) != 3 {
		t.Fatalf("merged = %d, want 3 (page + 2 backfilled evidence chunks)", len(merged))
	}
	if merged[0]["chunk_id"] != "wiki/s" || merged[1]["chunk_id"] != "c1" || merged[2]["chunk_id"] != "c2" {
		t.Errorf("merge order wrong: %v", merged)
	}
	if merged[1]["content_with_weight"] != "raw chunk 1" {
		t.Errorf("evidence chunk content = %v, want raw chunk 1 (by-id backfill)", merged[1]["content_with_weight"])
	}
	// Doc aggs must include the evidence doc.
	found := false
	for _, d := range aggs {
		if d["doc_id"] == "d1" {
			found = true
		}
	}
	if !found {
		t.Errorf("evidence doc not unioned into doc aggs: %v", aggs)
	}
}

// TestProductionRunner_CompiledEvidence_NoServiceIsNoop asserts P7/R3 degrades
// safely: when the wiki service is unavailable, page hits (even with
// source_chunk_ids) keep the page results unchanged and fabricate nothing.
func TestProductionRunner_CompiledEvidence_NoServiceIsNoop(t *testing.T) {
	runner := newProductionRunnerWithTools(nil, "t1", []string{"kb1"}, nil, nil) // wikiSvc nil
	chunks := []map[string]interface{}{
		{"chunk_id": "wiki/s", "content_with_weight": "# Page", "dataset_id": "kb1", "source_chunk_ids": []interface{}{"c1", "c2"}},
	}
	merged, _ := runner.expandCompiledEvidence(t.Context(), chunks, []map[string]interface{}{})
	if len(merged) != 1 {
		t.Fatalf("merged = %d, want 1 (no service => no fabricated evidence)", len(merged))
	}
	if merged[0]["chunk_id"] != "wiki/s" {
		t.Errorf("page result changed: %v", merged[0])
	}
}
