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
	"regexp"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	enginetypes "ragflow/internal/engine/types"
)

// === Bm25Adapter / search_bm25_chunks ===

// TestBm25Adapter_TextOnlyMatchExpr asserts each query is sent as a standalone
// engine Search whose MatchExprs contain exactly one MatchTextExpr (no dense
// vector, no fusion) with default top_n and the compile/graph filters.
func TestBm25Adapter_TextOnlyMatchExpr(t *testing.T) {
	fe := &grepFakeEngine{}
	adapter := NewBm25Adapter(fe)

	_, err := adapter.SearchBm25(context.Background(), runtime.Bm25Request{
		TenantID:   "t",
		Queries:    []string{"alpha beta", "gamma"},
		DatasetIDs: []string{"kb1"},
	})
	if err != nil {
		t.Fatalf("SearchBm25: %v", err)
	}
	if fe.searchCalls != 2 {
		t.Fatalf("searchCalls = %d, want 2 (one per query)", fe.searchCalls)
	}
	req := fe.lastReq
	if req == nil {
		t.Fatal("no SearchRequest captured")
	}
	if len(req.MatchExprs) != 1 {
		t.Fatalf("MatchExprs has %d elements, want exactly 1 text expr", len(req.MatchExprs))
	}
	mt, ok := req.MatchExprs[0].(*enginetypes.MatchTextExpr)
	if !ok {
		t.Fatalf("MatchExprs[0] is %T, want *MatchTextExpr", req.MatchExprs[0])
	}
	if mt.MatchingText != "gamma" || mt.TopN != 12 {
		t.Errorf("text expr = {%q, TopN %d}, want {gamma, 12}", mt.MatchingText, mt.TopN)
	}
	if req.Limit != 12 {
		t.Errorf("Limit = %d, want default 12", req.Limit)
	}
	if req.Filter["available_int"] != 1 {
		t.Errorf("Filter missing available_int=1: %v", req.Filter)
	}
	if _, ok := req.Filter["must_not"]; !ok {
		t.Error("Filter missing must_not compile_kwd exclusion")
	}
}

// TestBm25Adapter_MergesDedupesSkipsGraph asserts cross-query dedup by chunk
// id, graph-payload skipping, and _score propagation into RetrievalChunk.Score.
func TestBm25Adapter_MergesDedupesSkipsGraph(t *testing.T) {
	base := &grepFakeEngine{
		searchChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "plain prose", "doc_id": "d1", "kb_id": "kb1", "_score": 7.5},
		},
	}
	adapter := NewBm25Adapter(&fakeTwoResultEngine{grepFakeEngine: base})

	chunks, err := adapter.SearchBm25(context.Background(), runtime.Bm25Request{
		TenantID: "t", Queries: []string{"q1", "q2"}, DatasetIDs: []string{"kb1"},
	})
	if err != nil {
		t.Fatalf("SearchBm25: %v", err)
	}
	if len(chunks) != 1 || chunks[0].ID != "c1" {
		t.Fatalf("chunks = %+v, want single c1 after dedup+graph skip", chunks)
	}
	if chunks[0].Score != 7.5 {
		t.Errorf("Score = %v, want 7.5 from first occurrence's _score", chunks[0].Score)
	}
}

// fakeTwoResultEngine returns different results per Search call: the wrapped
// engine's set on the first call, then a duplicate id plus a graph JSON payload
// chunk on the second.
type fakeTwoResultEngine struct {
	*grepFakeEngine
	calls int
}

func (e *fakeTwoResultEngine) Search(ctx context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	e.calls++
	if e.calls == 2 {
		return &enginetypes.SearchResult{Chunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "duplicate prose", "_score": 3.0},
			{"id": "c9", "content_with_weight": `{"head":"A","tail":"B","type":"relation"}`, "_score": 9.9},
		}}, nil
	}
	return e.grepFakeEngine.Search(ctx, req)
}

// TestSearchBm25ChunksTool_ArgsBounds pins argument validation through the
// InvokableRun surface with a stub Bm25Service.
func TestSearchBm25ChunksTool_ArgsBounds(t *testing.T) {
	stub := &bm25StubService{}
	runtime.SetBm25Service(stub)
	defer runtime.SetBm25Service(nil)
	tool := NewSearchBm25ChunksTool("t", []string{"kb1"})

	// Empty queries -> <tool_error> result (the loop keeps running; the
	// model reads the reason and can fix its arguments).
	if out, err := tool.InvokableRun(context.Background(), `{"queries":[""]}`); err != nil || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("empty queries must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
	// More than 5 -> <tool_error> result.
	if out, err := tool.InvokableRun(context.Background(), `{"queries":["a","b","c","d","e","f"]}`); err != nil || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf(">5 queries must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
	// Happy path: top_n clamped and queries passed through; XML emitted with
	// short snippets — never full chunk bodies.
	out, err := tool.InvokableRun(context.Background(), `{"queries":["alpha","beta"],"top_n":999}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `<search_results`) {
		t.Errorf("output missing search_results tag: %.120q", out)
	}
	if !strings.Contains(out, `<match_snippet>`) {
		t.Errorf("output missing match_snippet element: %.200q", out)
	}
	if strings.Contains(out, "<content>") || strings.Contains(out, "content_snippet") {
		t.Errorf("unified contract forbids legacy content elements: %.200q", out)
	}
	if stub.lastReq.TopN != 50 {
		t.Errorf("TopN = %d, want clamped 50", stub.lastReq.TopN)
	}
	if len(stub.lastReq.Queries) != 2 {
		t.Errorf("Queries = %v, want 2", stub.lastReq.Queries)
	}
}

// TestUnifiedSnippetHelpers pins the shared snippet machinery used by all
// three locate tools: whole-span anchoring (earliest start − N .. latest end
// + N), bounded size with ellipses, whitespace collapsing, and no-match
// behavior.
func TestUnifiedSnippetHelpers(t *testing.T) {
	content := strings.Repeat("filler ", 200) +
		"visit the Bronner archive today. More Bronner lore follows. " +
		strings.Repeat("tail ", 200)

	first, last, ok := termMatchSpan([]string{"bronner"}, content)
	if !ok || !strings.Contains(strings.ToLower(content[first:last]), "bronner") {
		t.Fatalf("term span wrong: ok=%v [%d,%d)", ok, first, last)
	}
	// Span must extend to the LAST occurrence's end, not the first.
	lastFirst := strings.Index(strings.ToLower(content), "bronner")
	if last <= lastFirst+len("bronner") {
		t.Fatalf("span end %d did not cover the latest occurrence (first end at %d)", last, lastFirst+len("bronner"))
	}
	snip, truncated := snippetForMatches(content, first, last)
	lower := strings.ToLower(snip)
	for _, want := range []string{"bronner"} {
		if !strings.Contains(lower, want) {
			t.Errorf("snippet lost matched term: %.160q", snip)
		}
	}
	if !truncated {
		t.Error("a 2400-rune chunk must yield a truncated snippet")
	}
	if !strings.HasPrefix(snip, "...") || !strings.HasSuffix(snip, "...") {
		t.Errorf("snippet missing both ellipses: %.80q / %.80q", snip[:8], snip[len(snip)-8:])
	}
	// Bounded by the snippet budget (the match span here is far shorter).
	if n := len([]rune(snip)); n > snippetMaxRunes+len("...")*2 {
		t.Errorf("snippet too long: %d > %d runes", n, snippetMaxRunes)
	}
	if strings.ContainsAny(snip, "\n") {
		t.Error("snippet must be single-line")
	}

	// Regex counterpart shares the span semantics.
	re := regexp.MustCompile(`(?i)bronner`)
	rf, rl, rok := regexMatchSpan(re, content)
	if !rok || rf != first || rl != last {
		t.Fatalf("regex span (%d,%d) diverges from term span (%d,%d)", rf, rl, first, last)
	}

	// No match → not-ok span; inverted bounds are clamped, not dropped.
	if _, _, ok := termMatchSpan([]string{"zzz"}, content); ok {
		t.Fatal("unexpected match for absent term")
	}
	if got, _ := snippetForMatches(content, 10, 5); got == "" {
		t.Error("inverted bounds must still yield a fragment")
	}
}

type bm25StubService struct {
	lastReq runtime.Bm25Request
}

func (s *bm25StubService) SearchBm25(_ context.Context, req runtime.Bm25Request) ([]runtime.RetrievalChunk, error) {
	s.lastReq = req
	return []runtime.RetrievalChunk{{ID: "c1", Content: "text mentioning alpha prominently"}}, nil
}

// fakeQueryTokenizer stands in for the service/nlp QueryBuilder: it "tokenizes"
// the raw query into the word forms the ingestion pipeline would have indexed.
type fakeQueryTokenizer struct{}

func (fakeQueryTokenizer) Question(txt string, _ string, _ float64) (*enginetypes.MatchTextExpr, []string) {
	return &enginetypes.MatchTextExpr{
		Fields:       []string{"content_ltks^2"},
		MatchingText: "tokenized(" + txt + ")",
		TopN:         100,
	}, []string{"tokenized", txt}
}

// TestBm25Adapter_TokenizedMatchExpr: without a query tokenizer the raw query
// text reaches the engine verbatim; with one attached, the engine receives the
// TOKENIZED match expression (same word forms as the ingestion-side index
// tokens) and the tokenizer's own TopN is overridden by the caller's.
func TestBm25Adapter_TokenizedMatchExpr(t *testing.T) {
	fe := &grepFakeEngine{}
	adapter := NewBm25Adapter(fe)
	adapter.SetQueryBuilder(fakeQueryTokenizer{})

	if _, err := adapter.SearchBm25(context.Background(), runtime.Bm25Request{
		TenantID:   "t",
		Queries:    []string{"mineralizer"},
		DatasetIDs: []string{"kb1"},
	}); err != nil {
		t.Fatalf("SearchBm25: %v", err)
	}
	mt, ok := fe.lastReq.MatchExprs[0].(*enginetypes.MatchTextExpr)
	if !ok {
		t.Fatalf("MatchExprs[0] is %T, want *MatchTextExpr", fe.lastReq.MatchExprs[0])
	}
	if mt.MatchingText != "tokenized(mineralizer)" {
		t.Fatalf("MatchingText = %q, want tokenized(mineralizer)", mt.MatchingText)
	}
	if mt.TopN != 12 {
		t.Fatalf("TopN = %d, want caller topN 12 (tokenizer's 100 overridden)", mt.TopN)
	}
}
