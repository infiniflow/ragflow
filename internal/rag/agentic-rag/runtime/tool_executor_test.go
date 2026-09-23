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

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"

	"gorm.io/gorm"
)

// stubRetrievalService returns a fixed set of child chunks so the runtime
// retireval path can be exercised without a live search backend. lastReq, when
// set, receives the request the runtime handed down.
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

// stubDocEngine serves the document-scoped Search used by retrieval_by_children;
// the rest of the (large) interface is promoted from a nil field and never
// called by this test.
type stubDocEngine struct {
	engine.DocEngine
	parents map[string]map[string]any
}

func (s stubDocEngine) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	parentID, _ := req.Filter["id"].(string)
	docID, _ := req.Filter["doc_id"].(string)
	if p, ok := s.parents[parentID]; ok && p["doc_id"] == docID {
		return &enginetypes.SearchResult{Chunks: []map[string]interface{}{p}}, nil
	}
	return &enginetypes.SearchResult{}, nil
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

	threshold, keywordsWeight := 0.35, 0.7
	if _, err := r.Retrieve(context.Background(), RetrieveRequest{
		Query:                    "q",
		SimilarityThreshold:      &threshold,
		KeywordsSimilarityWeight: &keywordsWeight,
	}); err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if got.SimilarityThreshold == nil || *got.SimilarityThreshold != threshold {
		t.Errorf("SimilarityThreshold = %v, want %v", got.SimilarityThreshold, threshold)
	}
	if got.KeywordsSimilarityWeight == nil || *got.KeywordsSimilarityWeight != keywordsWeight {
		t.Errorf("KeywordsSimilarityWeight = %v, want %v", got.KeywordsSimilarityWeight, keywordsWeight)
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
// two child fragments sharing a mom_id collapse into the single parent chunk.
func TestRuntimeRetrieverPromotesChildrenToParent(t *testing.T) {
	prev := runtime.GetRetrievalService()
	runtime.SetRetrievalService(stubRetrievalService{chunks: []runtime.RetrievalChunk{
		{ID: "child-1", Content: "frag one", DocumentID: "doc-1", DatasetID: "kb-1", MomID: "parent-1", Score: 0.6},
		{ID: "child-2", Content: "frag two", DocumentID: "doc-1", DatasetID: "kb-1", MomID: "parent-1", Score: 0.8},
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

// TestBM25SearchDoesPromoteChildren verifies that bm25_search (like every other entry
// point) runs retrieval_by_children: a child fragment is lifted to its parent chunk, because
// the normaliser calls retrieval_by_children.
func TestBM25SearchDoesPromoteChildren(t *testing.T) {
	prev := runtime.GetRetrievalService()
	runtime.SetRetrievalService(stubRetrievalService{chunks: []runtime.RetrievalChunk{
		{ID: "child-1", Content: "frag", DocumentID: "doc-1", DatasetID: "kb-1", MomID: "parent-1", Score: 0.6},
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

// TestExecuteLogsFunctionToolLine covers the "[Function tool] Running the {name} tool
// with: {args}" line, one of the namespaces forwarded into the think block.
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

// TestSearchChunksAlwaysUsesCompiled pins fix #2: the search_chunks tool ALWAYS enables
// compiled-structure expansion; retrieve never does. The executor must NOT gate it on
// RunRequest.UseCompiled (which controls the L1 direct retrieve).
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
	if got := RenderToolArgs(nil); got != "{}" {
		t.Errorf("RenderToolArgs(nil) = %q, want {}", got)
	}
	if got := RenderToolArgs(map[string]any{}); got != "{}" {
		t.Errorf("RenderToolArgs(map{}) = %q, want {}", got)
	}
	got := RenderToolArgs(map[string]any{"q": "hi", "n": 3})
	if !strings.Contains(got, `"q":"hi"`) || !strings.Contains(got, `"n":3`) {
		t.Errorf("RenderToolArgs = %q, want both keys", got)
	}
	// Unmarshalable values must degrade to "{}", not blow up the log line.
	if got := RenderToolArgs(map[string]any{"f": func() {}}); got != "{}" {
		t.Errorf("RenderToolArgs(func value) = %q, want {}", got)
	}

	// A long value is CAPPED: the slot-research driver passes a paragraph of
	// evidence as its "query", and the uncapped rendering repeated the same 200
	// characters on every line of its round.
	long := strings.Repeat("曹", 200)
	got = RenderToolArgs(map[string]any{"query": []any{long}})
	if strings.Contains(got, long) || !strings.Contains(got, "…") {
		t.Errorf("long args = %q, want the value capped with an ellipsis", got)
	}
	if n := utf8.RuneCountInString(got); n > thinkLabelMaxRunes+16 {
		t.Errorf("rendered args = %d runes, want one capped value", n)
	}
	// Capping must not produce invalid UTF-8 (it cuts on rune boundaries)...
	if !utf8.ValidString(got) {
		t.Errorf("capped args are not valid UTF-8: %q", got)
	}
	// ...nor invalid JSON: the capped value still parses.
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Errorf("capped args must stay valid JSON: %v", err)
	}
	// Nested shapes are capped too (a tool argument can be an object).
	if got := RenderToolArgs(map[string]any{"scope": map[string]any{"q": long}}); !strings.Contains(got, "…") {
		t.Errorf("nested args = %q, want the nested value capped", got)
	}
	// A list of short queries is left alone: capping caps VALUES, it does not
	// truncate the call.
	if got := RenderToolArgs(map[string]any{"query": []any{"a", "b"}}); got != `{"query":["a","b"]}` {
		t.Errorf("short args = %q, want them untouched", got)
	}
}

// TestToolCallLine pins the think block's call step: the tool and its query, and
// nothing else. The argument object is a developer's record — the document id, the
// kind switch, the nav hint and the empty scope list are all detail a reader would
// have to parse — and a call that carries no query names only the tool instead of
// trailing an empty "with:" or falling back to an id.
func TestToolCallLine(t *testing.T) {
	const docID = "95a7aee3f11143e69dc9fa5b7bad3a14"
	cases := []struct {
		name string
		tool string
		args map[string]any
		want string
	}{
		// The shape the reader actually meets: a document-scoped call that also
		// carries the query it was made for. Only the query survives.
		{"query-with-plumbing", "navigate_structure",
			map[string]any{"doc_id": docID, "kind": "catalog", "query": "曹操历史地位"},
			`Running the navigate_structure tool with "曹操历史地位".`},
		{"query-list", "retrieve", map[string]any{"nav_hint": "", "query": []any{"曹操的逝世日期", "曹操是谁"}},
			`Running the retrieve tool with "曹操的逝世日期", "曹操是谁".`},
		{"question", "calculate", map[string]any{"question": "how many people"},
			`Running the calculate tool with "how many people".`},
		{"doc-only", "summarize_document", map[string]any{"doc_id": docID},
			"Running the summarize_document tool."},
		{"no-args", "summarize_document", nil, "Running the summarize_document tool."},
	}
	for _, tc := range cases {
		if got := ToolCallLine(tc.tool, tc.args); got != tc.want {
			t.Errorf("%s: ToolCallLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestCountOfPluralizes pins the two rules the trace needs: a regular noun takes
// "s", and a consonant+y noun takes "ies". Appending a bare "s" is what printed
// "3 first-hop querys" and "3 targeted querys".
func TestCountOfPluralizes(t *testing.T) {
	cases := []struct {
		n    int
		noun string
		want string
	}{
		{1, "query", "1 query"},
		{3, "query", "3 queries"},
		{2, "first-hop query", "2 first-hop queries"},
		{2, "targeted query", "2 targeted queries"},
		{2, "passage", "2 passages"},
		{2, "new passage", "2 new passages"},
		{1, "sub-question", "1 sub-question"},
		{2, "sub-question", "2 sub-questions"},
		{2, "evidence block", "2 evidence blocks"},
		{2, "key", "2 keys"},
		{0, "gathered passage", "0 gathered passages"},
	}
	for _, tc := range cases {
		if got := CountOf(tc.n, tc.noun); got != tc.want {
			t.Errorf("CountOf(%d, %q) = %q, want %q", tc.n, tc.noun, got, tc.want)
		}
	}
}

// TestExecuteNarratesToolOutcome pins the RESULT half of the "[Function tool]"
// think-block narration: every call reports what came back (status, result
// count, source documents), not only that it ran. Without it the trace shows a
// tool was invoked but never whether it found anything.
//
// It also pins the STRUCTURED twin of both halves: a client rendering steps
// gets the tool name, arguments, status, result/document counts and evidence
// anchors without parsing the sentences.
func TestExecuteNarratesToolOutcome(t *testing.T) {
	var buf bytes.Buffer
	deps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c1", "doc_id": "d1", "content": "hit one"},
		{"chunk_id": "c2", "doc_id": "d1", "content": "hit two"},
		{"chunk_id": "c3", "doc_id": "d2", "content": "hit three"},
	}})
	deps.Logger = log.New(&buf, "", 0)
	var events []ThinkEvent
	ctx := WithSteps(context.Background(), StepReporter{Events: func(ev ThinkEvent) { events = append(events, ev) }})
	// search_chunks never narrows (no keywords), so the stub's hits reach the
	// outcome verbatim.
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})

	if _, err := ex.Execute(ctx, "search_chunks", map[string]any{"query": "q"}); err != nil {
		t.Fatalf("search_chunks: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"[Function tool] Running the search_chunks tool with: ",
		`[Function tool] The search_chunks tool returned 3 results from 2 documents for "q".`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("think-log narration missing %q:\n%s", want, out)
		}
	}

	// The tool's two halves PLUS the search legs underneath it: the legs report
	// their own steps (which leg ran, what it searched, what it found), so the
	// think block answers "what did it actually search".
	var call, result ThinkEvent
	var legLines []string
	for _, ev := range events {
		switch ev.Kind {
		case ThinkKindToolCall:
			call = ev
		case ThinkKindToolResult:
			result = ev
		case thinkKindStage:
			legLines = append(legLines, ev.Summary)
		}
	}
	legs := strings.Join(legLines, "\n")
	for _, want := range []string{
		// The leg's two steps as the READER gets them: sentences in the family every
		// leg shares (the method is named inside the sentence, because the
		// "[Hybrid search]" prefix is a developer's label), counts in words, no ids —
		// and no "the knowledge base", which every leg has in common.
		`[Hybrid search] Searching by meaning and keyword for "q".`,
		`[Hybrid search] Found 3 passages in 2 documents for "q".`,
	} {
		if !strings.Contains(legs, want) {
			t.Errorf("search-leg step missing %q; legs:\n%s", want, legs)
		}
	}
	// The log keeps Python's own wording (search.py:122): it names the corpus, not
	// the method, and ends without a period.
	if !strings.Contains(out, `[Hybrid search] Searching the knowledge base for "q"`) {
		t.Errorf("the log must keep its searching line:\n%s", out)
	}
	// ...and the log form must not be what the block got: the two sentences differ
	// by design (the log names the corpus, the block names the method, and only the
	// block ends in a period).
	if strings.Contains(legs, `Searching the knowledge base for`) {
		t.Errorf("the think block got the log line instead of the sentence:\n%s", legs)
	}
	// The per-document breakdown the sentence is derived from stays in the
	// developer log, keyed by id: that is the half a developer greps for.
	if !strings.Contains(out, `"q" -> 3 chunk(s): d1:2chunk(14chars); d2:1chunk(9chars)`) {
		t.Errorf("the log must keep the per-document breakdown:\n%s", out)
	}
	if strings.Contains(legs, "d1:2chunk") {
		t.Errorf("document ids must not reach the think block; legs:\n%s", legs)
	}
	if call.Kind != ThinkKindToolCall || call.Tool != "search_chunks" || call.Stage != "Function tool" {
		t.Errorf("call event = %#v", call)
	}
	// The call step names the tool and its query — nothing else — while the log and
	// the event's Args keep the argument object the model emitted.
	if want := `[Function tool] Running the search_chunks tool with "q".`; call.Summary != want {
		t.Errorf("call summary = %q, want %q", call.Summary, want)
	}
	if !strings.Contains(out, `[Function tool] Running the search_chunks tool with: {"query":"q"}`) {
		t.Errorf("the log must keep the argument object:\n%s", out)
	}
	if !strings.Contains(call.Args, `"query":"q"`) {
		t.Errorf("call args = %q, want the rendered arguments", call.Args)
	}

	if result.Kind != ThinkKindToolResult || result.Status != StatusOK {
		t.Errorf("result event = %#v, want an ok result", result)
	}
	if result.Results != 3 || result.Documents != 2 {
		t.Errorf("result counts = %d/%d, want 3 results from 2 documents", result.Results, result.Documents)
	}
	// The evidence anchors are the admitted chunk ids — what the sentence's
	// "3 results" points at.
	if len(result.Sources) != 3 {
		t.Errorf("sources = %#v, want the 3 admitted chunk ids", result.Sources)
	}
	// The result carries the call's arguments too, and its sentence names them:
	// with concurrent calls interleaving their lines, that is what pairs a result
	// back to the call that produced it.
	if result.Args != call.Args {
		t.Errorf("result args = %q, want the call's arguments %q", result.Args, call.Args)
	}
	if !strings.Contains(result.Summary, `for "q"`) {
		t.Errorf("result summary = %q, want it to name the query", result.Summary)
	}
	// The structured counts must agree with the sentence shown to the user.
	if !strings.Contains(result.Summary, "3 results from 2 documents") {
		t.Errorf("result summary = %q, want it to carry the same counts", result.Summary)
	}
}

// TestExecuteHidesDocumentIDsFromThink pins the split the two projections exist
// for: a 32-hex document id is exact for a developer and meaningless for a reader,
// so it belongs in the developer log and in the machine-readable Args — and not in
// the think block, neither in the call's arguments nor in the outcome sentence.
//
// The call targets a tool name this deployment has no binding for on purpose: it
// exercises the whole path (call line, outcome line, label fallback) without
// needing a retriever.
func TestExecuteHidesDocumentIDsFromThink(t *testing.T) {
	const docID = "95a7aee3f11143e69dc9fa5b7bad3a14"
	var logBuf, think strings.Builder
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.Logger = log.New(&logBuf, "", 0)
	var events []ThinkEvent
	ctx := WithSteps(context.Background(), StepReporter{
		Text:   func(line string) { think.WriteString(line) },
		Events: func(ev ThinkEvent) { events = append(events, ev) },
	})
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})

	if _, err := ex.Execute(ctx, "time_travel", map[string]any{"doc_id": docID}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// A doc-scoped call has no query, so the call step names only the tool: no
	// argument object, and therefore no id.
	if !strings.Contains(think.String(), "[Function tool] Running the time_travel tool.") {
		t.Errorf("think block should show the call without arguments:\n%s", think.String())
	}
	if strings.Contains(think.String(), docID) {
		t.Errorf("the think block must not carry a document id:\n%s", think.String())
	}
	// A document-scoped call has no human label: it goes unlabelled rather than
	// naming a document the reader cannot resolve.
	if !strings.Contains(think.String(),
		"[Function tool] The time_travel tool is not wired in this deployment, so nothing ran.") {
		t.Errorf("think block missing the outcome sentence:\n%s", think.String())
	}

	// The structured summaries follow the same rule as the text (a client renders
	// them), while the event's Args keep the exact call: that is what a client
	// correlates a step on.
	for _, ev := range events {
		if strings.Contains(ev.Summary, docID) {
			t.Errorf("event summary %q carries the document id", ev.Summary)
		}
	}
	if last := events[len(events)-1]; !strings.Contains(last.Args, docID) {
		t.Errorf("event Args = %q, want the document id kept", last.Args)
	}

	// The developer log keeps the id on both halves of the call.
	if !strings.Contains(logBuf.String(), `"doc_id":"`+docID+`"`) {
		t.Errorf("the log's call line must keep the document id:\n%s", logBuf.String())
	}
	if !strings.Contains(logBuf.String(), "so nothing ran for document "+docID) {
		t.Errorf("the log's outcome line must name the document by id:\n%s", logBuf.String())
	}
}

// TestExecuteNarratesEmptyOutcome pins the other end: a tool that reached
// nothing says so, instead of leaving that indistinguishable from a hit.
func TestExecuteNarratesEmptyOutcome(t *testing.T) {
	var buf bytes.Buffer
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.Logger = log.New(&buf, "", 0)
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})

	if _, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q"}); err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if want := `[Function tool] The retrieve tool matched nothing for "q".`; !strings.Contains(buf.String(), want) {
		t.Errorf("think-log narration missing %q:\n%s", want, buf.String())
	}
}

// TestChunkAggLegFitsTheCandidateCountToItsPool pins the nav-tree chunk-agg
// leg's retrieval parameters against the retrieval service's own invariant
// (`page * page_size <= rerank_candidates_count`, nlp/retrieval.go:126 =
// rag/nlp/search.py:745).
//
// Python passes the pool as page_size AND raises rerank_candidates_count to the
// same pool for this leg (dataset_api_service.py:4106-4129, plus the
// must_not={"exists":"compile_kwd"} of _NAV_CHUNK_AGG_EXCLUDE_COMPILED=True).
// Go dropped both, so page_size 256 met the default candidate count 64 and every
// nav-tree descent failed with "rerank_candidates_count(64) must be greater than
// or equal to page(1) * page_size(256)" — reported as an infra tool failure, and
// the navigation ladder silently degraded to plain retrieval on every round.
func TestChunkAggLegFitsTheCandidateCountToItsPool(t *testing.T) {
	const pool = 256 // nav chunkAggPool / Python _NAV_CHUNK_AGG_POOL
	r := &stubRetriever{chunks: []map[string]any{{"chunk_id": "c1"}}}
	retrieve := chunkAggRetrieveFrom(r)

	chunks, err := retrieve(context.Background(), "tenant1", "kb1", "q", []string{"d1"}, pool, 0.3)
	if err != nil {
		t.Fatalf("chunkAggRetrieveFrom: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want the backend's hit", len(chunks))
	}
	req := r.lastReq(t)
	if req.TopN != pool {
		t.Errorf("TopN = %d, want %d (page_size IS the pool)", req.TopN, pool)
	}
	if req.RerankCandidatesCount < req.TopN {
		t.Errorf("RerankCandidatesCount = %d < page_size %d: the retrieval service rejects this combination",
			req.RerankCandidatesCount, req.TopN)
	}
	if !req.ExcludeCompiled {
		t.Error("ExcludeCompiled = false: this leg aggregates ORIGINAL chunks per document, so compiled rows must be filtered out")
	}
	if len(req.DocScope) != 1 || req.DocScope[0] != "d1" {
		t.Errorf("DocScope = %v, want the caller's document scope", req.DocScope)
	}
}

// TestRenderToolOutcomeCoversEveryStatus pins one written sentence per status,
// so an unmapped status can never silently fall back to a bare result count.
//
// Two properties are pinned alongside the status wording, because both are what
// makes the sentence usable: it names the call's query (parallel calls interleave
// their lines, so an unlabelled result cannot be paired with its call) and it
// never spells the machine-readable reason into prose.
//
// The label is the ONLY input that differs between the two audiences (the think
// sentence passes ArgsLabel, the developer line docIDLabel), so this test passes
// labels rather than arguments: the wording above is written once and cannot
// drift between them. TestExecuteHidesDocumentIDsFromThink pins the split itself.
func TestRenderToolOutcomeCoversEveryStatus(t *testing.T) {
	cases := []struct {
		name  string
		tool  string
		label string
		oc    ToolOutcome
		want  string
	}{
		{"ok-no-docs", "calculate", "", ToolOutcome{Status: StatusOK, Payload: []any{map[string]any{"id": "c1"}}},
			"The calculate tool returned 1 result."},
		{"ok-with-docs", "retrieve", ` for "曹操是谁"`, ToolOutcome{Status: StatusOK, Payload: []any{
			map[string]any{"doc_id": "d1"}, map[string]any{"doc_id": "d2"}}},
			`The retrieve tool returned 2 results from 2 documents for "曹操是谁".`},
		{"ok-multi-query", "retrieve", ` for "曹操的逝世日期", "曹操是谁"`, ToolOutcome{Status: StatusOK,
			Payload: []any{map[string]any{"doc_id": "d1"}}},
			`The retrieve tool returned 1 result from 1 document for "曹操的逝世日期", "曹操是谁".`},
		// navigate_tree reports routing, not passages: its single payload entry
		// names the documents it routed to as doc_ids.
		{"ok-routed-docs", "navigate_tree", ` for "曹操是谁"`, ToolOutcome{Status: StatusOK, Payload: []any{
			map[string]any{"kind": "navigate_tree", "doc_ids": []string{"d1", "d2", "d3"}}}},
			`The navigate_tree tool returned 1 result from 3 documents for "曹操是谁".`},
		// A document-scoped call has NO label a reader can use: the id names
		// nothing they know, so the sentence goes unlabelled while the developer
		// copy carries " for document d9" (docIDLabel — pinned by TestQuoteQueries).
		{"ok-doc-scoped", "list_chunks", "", ToolOutcome{Status: StatusOK,
			Payload: []any{map[string]any{"id": "c1"}}},
			"The list_chunks tool returned 1 result."},
		// Singular "1 result" must not be followed by "all of them".
		{"redundant", "retrieve", ` for "q"`, ToolOutcome{Status: statusRedundant,
			Payload: []any{map[string]any{"id": "c1"}}},
			`The retrieve tool returned 1 result for "q", already in the evidence pool.`},
		{"miss", "retrieve", ` for "q"`, ToolOutcome{Status: StatusMiss, Reason: ReasonNoDoc},
			`The retrieve tool matched nothing for "q".`},
		// Without a query argument the sentence still has to name what was
		// matched against, or it ends on a dangling preposition.
		{"miss-unlabelled", "retrieve", "", ToolOutcome{Status: StatusMiss, Reason: ReasonNoDoc},
			"The retrieve tool matched nothing for this query."},
		// A hallucinated tool name is not a failure of this deployment's tools.
		{"unwired", "time_travel", ` for "q"`, ToolOutcome{Status: StatusMiss, Reason: ReasonUnwired},
			`The time_travel tool is not wired in this deployment, so nothing ran for "q".`},
		{"empty-no-structure", "navigate_structure", ` for "曹操是谁"`,
			ToolOutcome{Status: StatusEmpty, Reason: reasonNoStructure},
			`The navigate_structure tool has no compiled structure to read for "曹操是谁".`},
		{"poor", "calculate", ` for "how many people"`, ToolOutcome{Status: statusPoor, Reason: ReasonNoDoc},
			`The calculate tool produced a result too weak to use for "how many people".`},
		// The producer's own diagnostic is the actionable half of a failure; the
		// reason token stays in the event.
		{"error-with-cause", "navigate_tree", "", ToolOutcome{Status: StatusError, Reason: reasonInfra,
			Diagnostic: "nav-tree descent failed for kb=kb1"},
			"The navigate_tree tool could not run: nav-tree descent failed for kb=kb1."},
	}
	for _, tc := range cases {
		if got := renderToolOutcome(tc.tool, tc.label, tc.oc, nil); got != tc.want {
			t.Errorf("%s: renderToolOutcome = %q, want %q", tc.name, got, tc.want)
		}
	}

	got := renderToolOutcome("retrieve", ` for "q"`, ToolOutcome{Status: StatusOK}, errors.New("backend down"))
	if !strings.Contains(got, "backend down") || !strings.Contains(got, `for "q"`) {
		t.Errorf("a transport error must be reported against its call: %q", got)
	}
}

// TestQuoteQueries pins the query label's two shapes and its refusal to label an
// argument that holds no text (a fall-through, not a blank label).
func TestQuoteQueries(t *testing.T) {
	cases := []struct {
		name string
		raw  any
		want string
	}{
		{"string", "曹操是谁", `"曹操是谁"`},
		{"string-list", []string{"a", "b"}, `"a", "b"`},
		{"any-list", []any{"a", 3, "b"}, `"a", "b"`},
		{"blank", "   ", ""},
		{"blank-list", []any{"", "  "}, ""},
		{"wrong-type", 42, ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := quoteQueries(tc.raw); got != tc.want {
			t.Errorf("%s: quoteQueries = %q, want %q", tc.name, got, tc.want)
		}
	}
	// A blank query does NOT fall back to the document id on the human side: the
	// id names nothing a reader knows. The developer copy is what carries it.
	if got := ArgsLabel(map[string]any{"query": "  ", "doc_id": "d1"}); got != "" {
		t.Errorf("ArgsLabel = %q, want no label for a blank query", got)
	}
	if got := docIDLabel(map[string]any{"query": "  ", "doc_id": "d1"}); got != " for document d1" {
		t.Errorf("docIDLabel = %q, want the doc_id fallback for the developer log", got)
	}
	if got := docIDLabel(map[string]any{"doc_id": "  "}); got != "" {
		t.Errorf("docIDLabel = %q, want no label for a blank id", got)
	}
	if got := ArgsLabel(map[string]any{"nav_hint": "x"}); got != "" {
		t.Errorf("ArgsLabel = %q, want no label", got)
	}

	// A paragraph-long "query" (the slot-research driver searches slot evidence)
	// is capped, and capped on a rune boundary: the label rides the result
	// sentence, so an uncapped one repeated the same 200 characters on every line
	// of the round.
	label := quoteQueries(strings.Repeat("曹", 200))
	if !strings.Contains(label, "…") {
		t.Errorf("quoteQueries(long) = %q, want it capped", label)
	}
	if n := utf8.RuneCountInString(label); n > thinkLabelMaxRunes+4 {
		t.Errorf("quoteQueries(long) = %d runes, want a capped label", n)
	}
	if !utf8.ValidString(label) {
		t.Errorf("quoteQueries(long) = %q, not valid UTF-8", label)
	}
}

// TestSearchNilKBIsSeeded pins the seed-on-demand semantics: a nil deps.KB is not a caller
// mistake — the executor creates the (empty) pool on first use, so the search RUNS and its
// outcome is decided by what it admitted (OK here), never a forced MISS. The executor must
// also not panic on the nil pool.
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

// TestListChunksNilKBIsMiss pins the semantics: a nil deps.KB is seeded on first use, and
// with no doc-store reader wired the read yields nothing, which is MISS/no_doc (there is no
// "graceful OK empty" branch). It must never error or panic.
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

// TestListChunksDeepReadsDocStore pins the deep read: list_chunks reads the document off the
// chunk store (deps.DocChunks, the same reader fetch_full_document uses) even when NONE of
// its chunks are in the evidence pool yet, and admits them into the shared pool the model can
// cite.
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

// tableChunksFor serves one table chunk for docID, so a test can pin how THIS path renders a
// table (the shape contract above must not cost the shared renderer).
func tableChunksFor(docID, raw string) DocChunkLister {
	return tableChunksStub{docID: docID, raw: raw}
}

type tableChunksStub struct {
	docID string
	raw   string
}

func (s tableChunksStub) DocChunks(_ context.Context, req DocChunksRequest) ([]map[string]any, error) {
	if req.DocID != s.docID || req.Offset > 0 {
		return nil, nil
	}
	return []map[string]any{{
		"chunk_id": "t0",
		"doc_id":   s.docID,
		"content":  s.raw,
	}}, nil
}

// TestListChunksTablePassageIsRendered pins the SHARED renderer on the list_chunks path: a table
// chunk must reach the model as the rendered field view — one JSON object per row, see renderTables —
// not as raw <table> markup. This path used to build its own passage dict, which is how an
// 8275-code-point standings table reached the model as raw HTML and then, cut to its first ~800 code
// points, read as "only the top four finishers". What the assertion is really about is the ROW SET
// surviving the render (hence the last row); the notation is the renderer's business.
func TestListChunksTablePassageIsRendered(t *testing.T) {
	const raw = `<table><thead><tr><th>Rank</th><th>Player</th><th>Total</th></tr></thead>` +
		`<tbody><tr><td>1</td><td>Kelly Kulick</td><td>587</td></tr>` +
		`<tr><td>19</td><td>Mariana Ayala</td><td>501</td></tr></tbody></table>`
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.DocChunks = tableChunksFor("doc-t", raw)
	ex := &searchExecutor{deps: deps, req: RunRequest{DatasetIDs: []string{"kb1"}, MaxLength: 8192}}

	oc, err := ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-t"})
	if err != nil {
		t.Fatalf("list_chunks: %v", err)
	}
	if len(oc.Payload) != 1 {
		t.Fatalf("payload = %d, want 1", len(oc.Payload))
	}
	content, _ := oc.Payload[0].(map[string]any)["content"].(string)
	if strings.Contains(content, "<table") {
		t.Errorf("table passage still carries raw markup:\n%s", content)
	}
	if !strings.Contains(content, `"Player": "Mariana Ayala"`) {
		t.Errorf("table passage is not the rendered field view carrying its late rows:\n%s", content)
	}
}

// TestListChunksPagesThroughADocument pins the paging reader: one call returns ONE PAGE
// (listChunksMaxOut chunks) and its note says whether the document continues, so a long
// document is read by advancing the offset rather than by widening the page.
//
// This replaced a pair of caps: the reader fetched up to listChunksMaxDeep (80) passages and
// admitted the first listChunksMaxOut (30), and the model was told nothing about the other
// fifty — a document was either short enough to fit one page or silently truncated.
func TestListChunksPagesThroughADocument(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.DocChunks = docChunksFor("doc-a", 100)
	ex := &searchExecutor{deps: deps, req: RunRequest{DatasetIDs: []string{"kb1"}, MaxLength: 1 << 20}}

	// Page 1: one page admitted, and the note says where the document continues.
	oc, err := ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a"})
	if err != nil {
		t.Fatalf("list_chunks: %v", err)
	}
	if len(oc.Payload) != listChunksMaxOut {
		t.Fatalf("payload = %d, want one page of %d", len(oc.Payload), listChunksMaxOut)
	}
	if len(kb.Chunks) != listChunksMaxOut {
		t.Errorf("kb.Chunks = %d, want %d admitted", len(kb.Chunks), listChunksMaxOut)
	}
	if !strings.Contains(oc.Note, "continues") || !strings.Contains(oc.Note, "offset=30") {
		t.Errorf("note = %q, want the page to say the document continues at offset 30", oc.Note)
	}

	// Page 2 starts where page 1 stopped, and the pool holds both pages.
	oc, err = ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a", "offset": float64(30)})
	if err != nil {
		t.Fatalf("list_chunks (page 2): %v", err)
	}
	if len(oc.Payload) != listChunksMaxOut {
		t.Errorf("page 2 payload = %d, want %d", len(oc.Payload), listChunksMaxOut)
	}
	if len(kb.Chunks) != 2*listChunksMaxOut {
		t.Errorf("kb.Chunks = %d, want %d (both pages)", len(kb.Chunks), 2*listChunksMaxOut)
	}

	// Page 3 fills in the middle, so paging is the only way to reach the end.
	oc, err = ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a", "offset": float64(60)})
	if err != nil {
		t.Fatalf("list_chunks (page 3): %v", err)
	}
	if len(kb.Chunks) != 3*listChunksMaxOut {
		t.Errorf("kb.Chunks = %d, want %d (three pages)", len(kb.Chunks), 3*listChunksMaxOut)
	}

	// The last page is short, and the note says the document ends here.
	oc, err = ex.listChunks(context.Background(), map[string]any{"doc_id": "doc-a", "offset": float64(90)})
	if err != nil {
		t.Fatalf("list_chunks (last page): %v", err)
	}
	if len(oc.Payload) != 10 {
		t.Errorf("last page payload = %d, want the remaining 10", len(oc.Payload))
	}
	if !strings.Contains(oc.Note, "END") {
		t.Errorf("note = %q, want the last page to say the document ends here", oc.Note)
	}
	if len(kb.Chunks) != 100 {
		t.Errorf("kb.Chunks = %d, want 100 (the whole document, once paged through)", len(kb.Chunks))
	}

	// Every page also went into the READ ledger: the run can now tell a passage it READ from one it
	// was only shown as a search snippet (see Kbinfos.NoteChunksRead). Four pages, 100 distinct
	// chunks, and the last page is not the last one the document has — this one ended.
	progress := kb.ReadProgress()
	if len(progress) != 1 {
		t.Fatalf("ReadProgress = %v, want one line for doc-a", progress)
	}
	line := progress[0]
	for _, want := range []string{"doc-a", "4 page(s)", "100 chunk(s) read", "offset 90"} {
		if !strings.Contains(line, want) {
			t.Errorf("ReadProgress line = %q, want it to carry %q", line, want)
		}
	}
	if kb.WasRead(payloadChunkID(oc.Payload[0].(map[string]any))) != true {
		t.Error("the last page's first passage was not recorded as READ")
	}
}

// TestLargePoolStillAdmitsNewEvidence pins that the pool has NO ceiling at the tool
// boundary: a search over an already-large pool still admits the chunk it found.
//
// The cap this replaces early-stopped admission at a hard number, and what it refused fell
// on the MODEL's evidence rather than on storage — the passage that would have reached an
// enumeration's last members, which are named late in the run (see the note on Kbinfos).
// Identity is the only thing the pool still refuses, which the second half asserts.
func TestLargePoolStillAdmitsNewEvidence(t *testing.T) {
	pre := make([]map[string]any, 0, 400)
	for i := 0; i < 400; i++ {
		pre = append(pre, map[string]any{"chunk_id": fmt.Sprintf("pre-%d", i), "content": "old"})
	}
	deps, kb := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	kb.Chunks = pre
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})

	oc, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if oc.Status != StatusOK {
		t.Errorf("status = %s, want %s (a large pool does not refuse evidence)", oc.Status, StatusOK)
	}
	if len(kb.Chunks) != len(pre)+1 {
		t.Errorf("kb.Chunks = %d, want %d (the hit was admitted)", len(kb.Chunks), len(pre)+1)
	}
	// The same search again: the chunk is already pooled, so nothing new is admitted.
	oc, err = ex.Execute(context.Background(), "retrieve", map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("retrieve (second): %v", err)
	}
	if oc.Status != statusRedundant && oc.Status != StatusMiss {
		t.Errorf("status = %s, want REDUNDANT/MISS on the repeated search", oc.Status)
	}
	if len(kb.Chunks) != len(pre)+1 {
		t.Errorf("kb.Chunks = %d, want the pool to stay at %d", len(kb.Chunks), len(pre)+1)
	}
}

// TestProbeWindowLandsOnALargePool pins the INTENT the cap exemption used to serve, now
// held by the pool itself: on a large pool a probe's per-name window is admitted, because
// that window is the batch's RESULT rather than one more passage. Drop it and "this name was
// found here" becomes "nothing new", which the model reads as "not a member".
//
// The cap that needed an exemption to achieve this is gone (see the note on Kbinfos), so the
// window now lands for the ordinary reason: nothing is refused for want of room.
func TestProbeWindowLandsOnALargePool(t *testing.T) {
	pre := make([]map[string]any, 0, 400)
	for i := 0; i < 400; i++ {
		pre = append(pre, map[string]any{"chunk_id": fmt.Sprintf("pre-%d", i), "content": "already pooled prose"})
	}
	deps, kb := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c1", "content": "荀正 被关公一刀斩于马下"},
	}})
	kb.Chunks = pre
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})

	oc, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "车胄|荀正|管亥"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if oc.Status != StatusOK {
		t.Errorf("status = %s, want %s (the probe window answered an unanswered name)", oc.Status, StatusOK)
	}
	if len(kb.Chunks) != len(pre)+1 {
		t.Errorf("kb.Chunks = %d, want %d (the probe's window was admitted)", len(kb.Chunks), len(pre)+1)
	}
	if len(oc.Payload) != 1 {
		t.Errorf("payload = %d, want the probe's window", len(oc.Payload))
	}

	// The same probe again: the pool now carries the window, so the cap is back
	// in charge and the outcome is REDUNDANT, not another seat.
	oc, err = ex.Execute(context.Background(), "retrieve", map[string]any{"query": "车胄|荀正|管亥"})
	if err != nil {
		t.Fatalf("retrieve (second): %v", err)
	}
	if len(kb.Chunks) != len(pre)+1 {
		t.Errorf("kb.Chunks = %d, want the pool to stay at %d", len(kb.Chunks), len(pre)+1)
	}
	if oc.Status != StatusMiss && oc.Status != statusRedundant {
		t.Errorf("status = %s, want MISS/REDUNDANT on the repeated probe", oc.Status)
	}
}

// TestWebSearchAdmitsToPool pins the web_search parity fix: web results merge into the SAME
// shared evidence pool as corpus hits, so downstream formalize/compose can cite them, and
// the outcome is REDUNDANT/OK/MISS like the corpus tools.
func TestWebSearchAdmitsToPool(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.WebSearch = stubWebSearch{results: []string{"web answer one", "web answer two", "web answer three"}}
	// The query list is read from args["query"].
	oc, err := webSearchTool(context.Background(), deps, map[string]any{"query": []any{"q1", "q2"}})
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

// TestWebSearchDedupsAcrossQueries pins the dedup: a passage that both queries return is
// admitted/shown only once (dedup by chunk_id across the queries via a shared `seen` set).
func TestWebSearchDedupsAcrossQueries(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{})
	deps.WebSearch = stubWebSearch{results: []string{"dup passage", "unique one", "dup passage"}}
	// Same args["query"] contract as above.
	oc, err := webSearchTool(context.Background(), deps, map[string]any{"query": []any{"q1", "q2"}})
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

// TestListChunksSkipsEmptyChunkID pins the empty-id skip: a deep-read chunk with no
// chunk_id is skipped — neither admitted to the pool nor shown to the model.
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
	// The lister contract is page-ordered and paged ("up to Limit chunks starting at
	// Offset; a short page means the document is exhausted"), so the stub honours both: a
	// stub that returned everything whatever the request asked for could not tell a paging
	// reader from one that ignores the page.
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset >= s.n {
		return nil, nil
	}
	end := s.n
	if req.Limit > 0 && offset+req.Limit < end {
		end = offset + req.Limit
	}
	out := make([]map[string]any, 0, end-offset)
	for i := offset; i < end; i++ {
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

// TestSearchRedundantReturnsFullPayload pins fix #5: when every hit is already in the
// evidence pool the result is statusRedundant — but the FULL passages are still returned so
// the model sees what it already has. Dropping the payload to empty hid the evidence.
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
	if oc.Status != statusRedundant {
		t.Fatalf("status = %s, want %s", oc.Status, statusRedundant)
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
// (an unresolvable owner), not fail open.

// End-to-end wiring tests: one Run call, with a stubbed retriever and a
// scripted model, must move evidence from the backend into Kbinfos and (when a
// canvas state is attached) into the citation store.

// corpusRetriever answers every query from a fixed corpus.
type corpusRetriever struct {
	mu    sync.Mutex
	calls []string
}

func (c *corpusRetriever) Retrieve(_ context.Context, req RetrieveRequest) ([]map[string]any, error) {
	// Locked: the search legs run concurrently (see runSearch), so the retriever contract is
	// concurrent and a fixture that appends without a lock is a race.
	c.mu.Lock()
	c.calls = append(c.calls, req.Query)
	c.mu.Unlock()
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
	if oc.Status != statusRedundant {
		t.Errorf("repeat call status = %s, want redundant", oc.Status)
	}

	// Missing query -> MISS/no_doc, NOT a bad-args error: an empty query list is simply a
	// miss.
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

func (f *fakeWikiRetriever) SearchWiki(_ context.Context, question string, keywords []string, topN int) ([]wikiPage, error) {
	return []wikiPage{{
		ChunkID: "w1", DocID: "wdoc1", DocName: "Synthesis", Title: "Culdcept Overview",
		Content: "Culdcept is a board game by OmiyaSoft.", Score: 0.9,
	}}, nil
}

func TestWikiQueryWiredReturnsPages(t *testing.T) {
	// When a WikiRetriever is wired, wiki_query returns the parsed page payload in the shape
	// the action layer consumes.
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
	// wiki_query is an UNPLUGGED extension seam: the handler exists
	// (TestWikiQueryWiredReturnsPages), but it is NOT part of any mode's active tool set
	// (removed from allTools) and no SearchWiki backend is wired in production, so it must
	// never appear in the advertised surface.
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
	// list_chunks deep-reads a document. With no doc-store reader wired it scans the
	// accumulated evidence pool; an unknown/missing doc_id is a query-level MISS, never a
	// dataset EMPTY and never a hard error.
	kb := &Kbinfos{Chunks: []map[string]any{
		{"chunk_id": "c1", "doc_id": "doc-a", "content": "first"},
		{"chunk_id": "c2", "doc_id": "doc-b", "content": "other"},
		{"chunk_id": "c3", "doc_id": "doc-a", "content": "second"},
	}}
	ex := &searchExecutor{deps: SearchDeps{KB: kb}}

	oc, _ := ex.Execute(context.Background(), "list_chunks", map[string]any{"doc_id": "doc-a"})
	// Already in evidence → REDUNDANT (nothing new admitted), but the passages
	// are still returned.
	if oc.Status != statusRedundant {
		t.Fatalf("status = %s, want redundant", oc.Status)
	}
	if len(oc.Payload) != 2 {
		t.Errorf("payload = %d, want 2 chunks of doc-a", len(oc.Payload))
	}
	// Evidence ids are the CHUNK ids, not pool positions.
	if len(oc.EvidenceIDs) != 2 || oc.EvidenceIDs[0] != "c1" || oc.EvidenceIDs[1] != "c3" {
		t.Errorf("evidence ids = %v, want [c1 c3]", oc.EvidenceIDs)
	}

	oc, _ = ex.Execute(context.Background(), "list_chunks", map[string]any{"doc_id": "doc-zz"})
	if oc.Status != StatusMiss {
		t.Errorf("unknown doc status = %s, want miss", oc.Status)
	}
	// Missing doc_id → MISS.
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
	// Non-table chunk is cut to 1200 code points as a plain slice — so NO trailing ellipsis
	// is added.
	if n := len([]rune(content)); n != 1200 {
		t.Errorf("content length = %d runes, want 1200", n)
	}
	if strings.HasSuffix(content, "...") {
		t.Errorf("the 1200-code-point cut adds no ellipsis marker: %q", content)
	}
	// Keys: exactly: {"id","content","doc_id"} — the
	// id must live under "id", which is what the drill merge reads back.
	if p["doc_id"] != "d1" || p["id"] != "c1" {
		t.Errorf("passage = %v", p)
	}
}

// TestMetadataSearchToolBadArgsAndKeyHints pins the two "the model should change what it
// asked for" outcomes: a call without filters, and a filter key the dataset does not
// carry. Both are MISS/bad_args — a query-level miss, never a claim about the tool — and
// the key case names the fields the dataset really has.
func TestMetadataSearchToolBadArgsAndKeyHints(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{
		pushdownOK: true,
		metas:      common.MetaData{"author": {"Alice": {"d1"}}},
	}
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})
	ctx := context.Background()

	out, err := exec.Execute(ctx, "metadata_search", map[string]any{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusMiss || out.Reason != ReasonBadArgs {
		t.Errorf("status/reason = %s/%s, want miss/bad_args for a call without filters", out.Status, out.Reason)
	}
	if note := toolNote(out); !strings.Contains(note, "search_chunks") {
		t.Errorf("note = %q, want the fallback tools named", note)
	}

	out, err = exec.Execute(ctx, "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "title", "op": "contains", "value": "New York"}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusMiss || out.Reason != ReasonBadArgs {
		t.Errorf("status/reason = %s/%s, want miss/bad_args for a missing key", out.Status, out.Reason)
	}
	if note := toolNote(out); !strings.Contains(note, "author") {
		t.Errorf("note = %q, want the dataset's REAL keys listed", note)
	}
}

// TestMetadataSearchToolInfraWhenIndexUnreadable pins the semantic split: a metadata-index
// failure is ERROR/infra and must NOT be phrased as "this dataset has no metadata" — the
// model would then give up on a filter that a working index would have honoured.
func TestMetadataSearchToolInfraWhenIndexUnreadable(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{flattenErr: errors.New("es down")}
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})

	out, err := exec.Execute(context.Background(), "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "title", "op": "contains", "value": "New York"}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusError || out.Reason != reasonInfra {
		t.Errorf("status/reason = %s/%s, want error/infra", out.Status, out.Reason)
	}
	if note := toolNote(out); !strings.Contains(note, "NOT a statement about the dataset") {
		t.Errorf("note = %q, want an explicit infra framing", note)
	}
}

// TestMetadataSearchToolReturnsMatchedDocIDsOnly pins the selector contract: the tool
// resolves the matching documents and returns their doc_ids — it runs NO retrieval, admits
// nothing to the shared pool and returns no passages. The ids are the whole output, because
// that is what lets the model spend one filter on several follow-up tools.
//
// The result also carries the applied filter and each matched document's metadata values —
// context the model reads, never an argument to hand to another tool (the ids are that
// argument). Values are drawn from the dataset's DECLARED fields, so annotation noise the
// dataset never declared cannot leak into every result.
func TestMetadataSearchToolReturnsMatchedDocIDsOnly(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		{"id": "c1", "doc_id": "d1", "content": "Culdcept was released in 1999 by OmiyaSoft."},
	}}
	deps, kb := newTestSearchDeps(r)
	deps.MetadataResolver = &stubMetadataResolver{
		pushdownOK:  true,
		pushdownIDs: []string{"d1", "d2"},
		metas:       common.MetaData{"title": {"Culdcept": {"d1", "d2"}}},
		docMeta: map[string]map[string]any{
			"d1": {
				"title":       "Culdcept",
				"file_name":   "culdcept.pdf",
				"update_time": "2026-09-20 13:55:03",
				"verdict":     "incorrect", // observed-only noise: must not be carried
			},
			"d2": {"title": []any{"Culdcept", "Culdcept II"}},
		},
	}
	deps.DeclaredMetadata = &stubDeclaredMetadata{defs: []common.MetadataFieldDef{
		{Key: "title", Type: "string"},
		{Key: "file_name", Type: "string"},
		{Key: "update_time", Type: "time"},
	}}
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})

	out, err := exec.Execute(context.Background(), "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "title", "op": "contains", "value": "Culdcept"}},
		"logic":   "and",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusOK {
		t.Fatalf("status = %s (reason %s), want ok", out.Status, out.Reason)
	}
	// A selector retrieves nothing: the retriever must not be called at all.
	if len(r.requests) != 0 {
		t.Errorf("retriever calls = %d, want 0 (metadata_search must not search)", len(r.requests))
	}
	if len(kb.Chunks) != 0 {
		t.Errorf("pool chunks = %d, want none admitted by a selector", len(kb.Chunks))
	}
	if len(out.EvidenceIDs) != 0 {
		t.Errorf("evidence ids = %v, want none", out.EvidenceIDs)
	}
	if len(out.Payload) != 1 {
		t.Fatalf("payload = %v, want one doc_ids object", out.Payload)
	}
	m, ok := out.Payload[0].(map[string]any)
	if !ok {
		t.Fatalf("payload[0] = %#v, want a map", out.Payload[0])
	}
	if m["kind"] != "metadata_search" {
		t.Errorf("payload kind = %v, want metadata_search", m["kind"])
	}
	ids, _ := m["doc_ids"].([]string)
	if len(ids) != 2 || ids[0] != "d1" || ids[1] != "d2" {
		t.Errorf("doc_ids = %v, want the matched documents", m["doc_ids"])
	}
	if docs, _ := out.Metrics["docs"].(int); docs != 2 {
		t.Errorf("metrics docs = %v, want 2", out.Metrics["docs"])
	}

	// The applied filter travels with the result.
	applied, _ := m["filters"].([]map[string]any)
	if len(applied) != 1 || applied[0]["key"] != "title" || applied[0]["op"] != "contains" {
		t.Errorf("payload filters = %v, want the applied condition", m["filters"])
	}

	// Per-document context: declared fields only, lists joined, noise excluded.
	docsList, _ := m["documents"].([]any)
	if len(docsList) != 2 {
		t.Fatalf("payload documents = %v, want one entry per matched document", m["documents"])
	}
	first, _ := docsList[0].(map[string]any)
	if first["doc_id"] != "d1" {
		t.Errorf("documents[0].doc_id = %v, want d1", first["doc_id"])
	}
	fields, _ := first["metadata"].(map[string]any)
	if fields["file_name"] != "culdcept.pdf" || fields["update_time"] != "2026-09-20 13:55:03" {
		t.Errorf("documents[0].metadata = %v, want the document's declared values", fields)
	}
	if _, leaked := fields["verdict"]; leaked {
		t.Errorf("documents[0].metadata carries an undeclared observed key: %v", fields)
	}
	second, _ := docsList[1].(map[string]any)
	fields2, _ := second["metadata"].(map[string]any)
	if fields2["title"] != "Culdcept, Culdcept II" {
		t.Errorf("documents[1].metadata[title] = %v, want the list joined with \", \"", fields2["title"])
	}
}

// TestMetadataSearchToolContextReadFailureKeepsTheSelection pins the best-effort contract of
// the context block: the documents were already resolved, so a failed metadata read must
// still return them (with the filter) rather than turning the call into an error.
func TestMetadataSearchToolContextReadFailureKeepsTheSelection(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{
		pushdownOK:  true,
		pushdownIDs: []string{"d1"},
		metas:       common.MetaData{"title": {"Culdcept": {"d1"}}},
		docMetaErr:  errors.New("metadata index down"),
	}
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})

	out, err := exec.Execute(context.Background(), "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "title", "op": "contains", "value": "Culdcept"}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusOK {
		t.Fatalf("status = %s (reason %s), want ok — the ids resolved, only the context is missing", out.Status, out.Reason)
	}
	m, _ := out.Payload[0].(map[string]any)
	if got := payloadDocIDs(out.Payload); len(got) != 1 || got[0] != "d1" {
		t.Errorf("doc_ids = %v, want the matched document", got)
	}
	if _, present := m["documents"]; present {
		t.Errorf("payload documents = %v, want the block omitted when nothing could be read", m["documents"])
	}
}

// TestMetadataSearchToolAcceptsCatalogFieldKey pins the non-title path end to end at the
// executor: a field the dataset really carries is accepted and the push-down resolves the
// documents. Before the catalog the schema never let a model emit this filter, so the
// executor's field-agnosticism was unreachable in practice.
func TestMetadataSearchToolAcceptsCatalogFieldKey(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	resolver := &stubMetadataResolver{
		pushdownOK:  true,
		pushdownIDs: []string{"d1"},
		metas:       common.MetaData{"author": {"Alice": {"d1"}}},
	}
	deps.MetadataResolver = resolver
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})

	out, err := exec.Execute(context.Background(), "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "author", "op": "contains", "value": "Alice"}},
		"logic":   "and",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusOK {
		t.Fatalf("status = %s (reason %s), want ok — a field the dataset carries must not be rejected", out.Status, out.Reason)
	}
	if len(resolver.gotFilters) != 1 || resolver.gotFilters[0]["key"] != "author" {
		t.Errorf("resolver filters = %v, want the author condition", resolver.gotFilters)
	}
	if got := payloadDocIDs(out.Payload); len(got) != 1 || got[0] != "d1" {
		t.Errorf("doc_ids = %v, want the author-matched document", got)
	}
}

// TestMetadataSearchToolWithoutMetadataIsMissNotError pins the metadata-free outcome: a
// call against a dataset whose metadata index carries NOTHING is a query-level bad_args
// miss with the fallback named — never an ERROR, which would tell the model the
// infrastructure failed and it should give up on the whole line of search.
func TestMetadataSearchToolWithoutMetadataIsMissNotError(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{pushdownOK: true, metas: common.MetaData{}}
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})

	out, err := exec.Execute(context.Background(), "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "title", "op": "contains", "value": "New York"}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status == StatusError {
		t.Fatalf("status = error (reason %s); a metadata-free dataset is NOT an infrastructure failure", out.Reason)
	}
	if out.Status != StatusMiss || out.Reason != ReasonBadArgs {
		t.Errorf("status/reason = %s/%s, want miss/bad_args", out.Status, out.Reason)
	}
	note := toolNote(out)
	if !strings.Contains(note, "NONE") {
		t.Errorf("note = %q, want the explicit no-metadata hint", note)
	}
	if !strings.Contains(note, "search_chunks") {
		t.Errorf("note = %q, want the fallback tools named", note)
	}
}

// TestMetadataSearchToolAcceptsDeclaredOnlyField pins the consistency the declarative source
// forces: a field the dataset DECLARES but has not indexed yet is advertised by the schema, so
// the executor — which validates against the same union — must accept it rather than answer
// "no such key" to the very filter it offered.
func TestMetadataSearchToolAcceptsDeclaredOnlyField(t *testing.T) {
	r := &stubRetriever{chunks: []map[string]any{
		{"id": "c1", "doc_id": "d1", "content": "A report about towers."},
	}}
	deps, _ := newTestSearchDeps(r)
	deps.MetadataResolver = &stubMetadataResolver{pushdownOK: true, pushdownIDs: []string{"d1"}, metas: common.MetaData{}}
	deps.DeclaredMetadata = &stubDeclaredMetadata{defs: []common.MetadataFieldDef{
		{Key: "doc_type", Description: "kind of document", Enum: []string{"report"}},
	}}
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})

	out, err := exec.Execute(context.Background(), "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "doc_type", "op": "=", "value": "report"}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusOK {
		t.Fatalf("status = %s (reason %s); a declared field must be accepted, not answered with bad_args", out.Status, out.Reason)
	}
	if got := payloadDocIDs(out.Payload); len(got) != 1 || got[0] != "d1" {
		t.Errorf("doc_ids = %v, want the declared-field match", got)
	}
}

// TestMetadataSearchToolRejectsSystemKey pins the blacklist at the enforcement point: a key
// the catalog refuses to advertise (benchmark labels, connector fields) is ALSO refused when
// the model names it anyway, so the leak cannot be reached by guessing.
func TestMetadataSearchToolRejectsSystemKey(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{})
	deps.MetadataResolver = &stubMetadataResolver{pushdownOK: true, metas: common.MetaData{
		"title":       {"Culdcept": {"d1"}},
		"question_id": {"444": {"d1"}},
	}}
	exec := NewSearchExecutor(deps, RunRequest{ThinkingMode: "high"})

	out, err := exec.Execute(context.Background(), "metadata_search", map[string]any{
		"filters": []any{map[string]any{"key": "question_id", "op": "=", "value": "444"}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Status != StatusMiss || out.Reason != ReasonBadArgs {
		t.Fatalf("status/reason = %s/%s, want miss/bad_args for a blacklisted key", out.Status, out.Reason)
	}
	note := toolNote(out)
	if !strings.Contains(note, "Available: title") {
		t.Errorf("note = %q, want the offered fields only (no question_id)", note)
	}
	if strings.Contains(note, "444") {
		t.Errorf("note = %q, must not echo a blacklisted key's value", note)
	}
}

// TestNormalizeMetadataValue pins the value coercion: a list where one keyword is expected
// collapses to its first non-empty element (one call = one keyword), 'in' keeps its list,
// and an all-empty list becomes no value at all.
func TestNormalizeMetadataValue(t *testing.T) {
	cases := []struct {
		value any
		op    string
		want  any
	}{
		{[]any{"New York", "Boston"}, "contains", "New York"},
		{[]any{"", nil, "Boston"}, "contains", "Boston"},
		{[]any{"", nil}, "contains", nil},
		{[]any{"a", "b"}, "in", []any{"a", "b"}},
		{"New York", "contains", "New York"},
	}
	for _, c := range cases {
		if got := NormalizeMetadataValue(c.value, c.op); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("NormalizeMetadataValue(%v, %q) = %v, want %v", c.value, c.op, got, c.want)
		}
	}
}

// TestMetadataSearchSecondFilterRuns pins the ABSENCE of the old one-shot guard: the tool is a
// document-set SELECTOR, so a question that spans two conditions (two days, two authors) needs
// one call per condition, and the second filter must reach the executor instead of being nudged
// away. (Identical calls are still answered from the tool cache — only a NEW filter re-runs.)
func TestMetadataSearchSecondFilterRuns(t *testing.T) {
	exec := &ladderExec{emptyFor: map[string]bool{}}
	dayFilter := func(day string) map[string]any {
		return map[string]any{"filters": []any{map[string]any{
			"key": "update_time", "op": "start with", "value": day,
		}}}
	}
	st := &sessionState{
		Tools:        &Toolset{Exec: exec, ThinkingMode: "high"},
		DeadlineLeft: 60,
		ToolCache:    NewToolCache(),
		PendingCalls: []ToolCall{{ID: "call-1", Name: "metadata_search", Args: dayFilter("2026-09-20")}},
	}
	if err := st.toolNode(context.Background()); err != nil {
		t.Fatalf("toolNode: %v", err)
	}
	st.PendingCalls = []ToolCall{{ID: "call-2", Name: "metadata_search", Args: dayFilter("2026-09-21")}}
	if err := st.toolNode(context.Background()); err != nil {
		t.Fatalf("toolNode (second turn): %v", err)
	}
	if len(exec.calls) != 2 {
		t.Fatalf("calls = %v, want both filters executed", exec.calls)
	}
	if last := st.Messages[len(st.Messages)-1]; strings.Contains(last.Content, "ONE-SHOT") {
		t.Errorf("the second filter was nudged away instead of executed: %q", last.Content)
	}
}

// toolNote reads the note off a tool outcome whose payload is a single note object.
func toolNote(out ToolOutcome) string {
	if len(out.Payload) == 0 {
		return ""
	}
	m, ok := out.Payload[0].(map[string]any)
	if !ok {
		return ""
	}
	note, _ := m["note"].(string)
	return note
}

// TestPassageFromChunkRendersTablesAsLines pins the action-session table shape: the model
// sees the rendered view (one "key: value" line per row, a fraction of the tokens) and never
// the raw <table>/<td> markup. The chunk in the shared pool stays raw for citation.
func TestPassageFromChunkRendersTablesAsLines(t *testing.T) {
	table := "<table><tr><th>Rank</th><th>Rider</th><th>Points</th></tr>" +
		"<tr><td>19</td><td>Danilo</td><td>62</td></tr>" +
		"<tr><td>20</td><td>Erik</td><td>61</td></tr></table>"
	p := passageFromChunk(map[string]any{
		"chunk_id": "c1", "doc_id": "d1", "content": table,
	})
	content, _ := p["content"].(string)
	if strings.Contains(content, "<td>") || strings.Contains(content, "<table") {
		t.Errorf("the model-visible passage still carries raw HTML: %q", content)
	}
	for _, want := range []string{`"Rank": "19"`, `"Rider": "Danilo"`, `"Points": "62"`, `"Rank": "20"`} {
		if !strings.Contains(content, want) {
			t.Errorf("passage lost %q: %q", want, content)
		}
	}
	if p["doc_id"] != "d1" || p["id"] != "c1" {
		t.Errorf("passage = %v", p)
	}
}

// seatRetriever answers a search by its query string, normalized to the DISTINCT
// tokens it carries: the engine receives "question keywords" (a seat search sends
// the same term twice), so a fixture must not depend on how that string is
// assembled.
type seatRetriever struct {
	mu      sync.Mutex
	byQuery map[string][]map[string]any
	calls   []string
}

func (s *seatRetriever) Retrieve(_ context.Context, req RetrieveRequest) ([]map[string]any, error) {
	// Locked: the seat pass runs beside the query legs, so calls arrive concurrently.
	s.mu.Lock()
	s.calls = append(s.calls, req.Query)
	s.mu.Unlock()
	seen := map[string]bool{}
	var tokens []string
	for _, tok := range strings.Fields(req.Query) {
		if seen[tok] {
			continue
		}
		seen[tok] = true
		tokens = append(tokens, tok)
	}
	return s.byQuery[strings.Join(tokens, " ")], nil
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestSearchOutcomeCarriesTheReachLine pins the delivery: the per-term reach the
// grep leg computes reaches the MODEL through ToolOutcome.Note (rendered behind
// the payload by the session tool node), not only the log.
//
// This is what makes a batch probe iterable: "华雄|荀正|管亥" either answers all
// three or says which of them nothing reached, and the next call can be aimed.
func TestSearchOutcomeCarriesTheReachLine(t *testing.T) {
	deps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c1", "content": "云长手起一刀，斩华雄于马下"},
	}})
	ex := NewSearchExecutor(deps, RunRequest{DatasetIDs: []string{"kb1"}})

	oc, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "华雄|荀正|管亥"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if oc.Note == "" {
		t.Fatal("Note is empty: the reach of a batch probe must reach the model")
	}
	for _, want := range []string{"华雄(1)", "荀正", "管亥", "NOT reached by this query"} {
		if !strings.Contains(oc.Note, want) {
			t.Errorf("Note %q missing %q", oc.Note, want)
		}
	}

	// A query with nothing to report on carries no note at all.
	plain, err := ex.Execute(context.Background(), "retrieve", map[string]any{"query": "云长"})
	if err != nil {
		t.Fatalf("retrieve (plain): %v", err)
	}
	if strings.HasPrefix(plain.Note, "[reach]") && !strings.Contains(plain.Note, "carry:") {
		t.Errorf("Note = %q, want no half-formed reach line", plain.Note)
	}
}

// TestSetDirectionWidensTheQueryBudget pins the recall rule a SET/COUNT direction
// buys, and it is deliberately written with NEUTRAL queries: nothing here depends
// on a corpus, a language, or a name.
//
// A direction assembling a SET asks the corpus about one facet per query, so a
// dropped query is a member nobody searched rather than a spared repeat. The
// executor therefore runs more of them once the direction has declared itself
// (Kbinfos.MarkSetDirection — set by the same gate that hands the model the set
// method), and when it still cannot run them all it SAYS so in the tool result,
// because a silent cut is invisible in the passages.
//
// Measured (2026-09-16, medium mode): every call asked 3-5 queries against caps of
// 2 (search_chunks) / 3 (retrieve), nine calls were cut, and the members the run
// then failed to record had been named only in the dropped ones.
func TestSetDirectionWidensTheQueryBudget(t *testing.T) {
	queries := []string{"term one", "term two", "term three", "term four"}

	// A value direction keeps the small cap — and reports the cut.
	valueDeps, _ := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	value := &stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}}
	valueDeps.Backend = value
	valueEx := NewSearchExecutor(valueDeps, RunRequest{DatasetIDs: []string{"kb1"}})
	oc, err := valueEx.Execute(context.Background(), "retrieve", map[string]any{"query": queries})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if !strings.Contains(oc.Note, "only 3 of this call's 4 queries were searched") {
		t.Errorf("Note = %q, want it to name the dropped query", oc.Note)
	}

	// The same call on a SET direction runs every query and reports no cut.
	setDeps, setKB := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}})
	dropped := &stubRetriever{chunks: []map[string]any{{"content": "hit", "chunk_id": "c1"}}}
	setDeps.Backend = dropped
	setKB.MarkSetDirection()
	if !setKB.IsSetDirection() {
		t.Fatal("MarkSetDirection did not declare the direction")
	}
	setEx := NewSearchExecutor(setDeps, RunRequest{DatasetIDs: []string{"kb1"}})
	oc2, err := setEx.Execute(context.Background(), "retrieve", map[string]any{"query": queries})
	if err != nil {
		t.Fatalf("retrieve (set): %v", err)
	}
	if strings.Contains(oc2.Note, "queries were searched") {
		t.Errorf("Note = %q, want no cut on a set direction", oc2.Note)
	}
	if len(dropped.requests) <= len(value.requests) {
		t.Errorf("set direction searched %d backend request(s), value direction %d: the set direction must search MORE of the caller's own queries",
			len(dropped.requests), len(value.requests))
	}

	// A nil pool is not a set direction, and asking is safe.
	var noPool *Kbinfos
	if noPool.IsSetDirection() {
		t.Error("a nil pool must not report a set direction")
	}
	noPool.MarkSetDirection()
}

// TestChunksToMapsCarriesReferenceFields pins the fields the answer reference
// card and the shared citation readers expect on an agentic chunk:
// doc_type_kwd (the image/table marker) and the content_with_weight spelling of
// the body. Without them an image chunk reached the response as an untyped,
// bodyless reference while the naive path rendered it fine.
func TestChunksToMapsCarriesReferenceFields(t *testing.T) {
	got := chunksToMaps([]runtime.RetrievalChunk{{
		ID:      "c1",
		Content: "wolf",
		ImageID: "kb-doc",
		DocType: "image",
	}})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0]["doc_type_kwd"] != "image" {
		t.Errorf("doc_type_kwd = %v, want image", got[0]["doc_type_kwd"])
	}
	if got[0]["content_with_weight"] != "wolf" || got[0]["content"] != "wolf" {
		t.Errorf("content fields = %v/%v, want wolf/wolf",
			got[0]["content_with_weight"], got[0]["content"])
	}
	if got[0]["image_id"] != "kb-doc" {
		t.Errorf("image_id = %v, want kb-doc", got[0]["image_id"])
	}
}
