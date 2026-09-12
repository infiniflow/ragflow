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
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// navScriptedModel answers with canned replies in call order and records every
// message slice handed to Complete (satisfies SessionModel).
type navScriptedModel struct {
	replies []string
	idx     int
	seen    [][]schema.Message
	err     error
}

func (m *navScriptedModel) Complete(_ context.Context, msgs []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.seen = append(m.seen, msgs)
	if len(m.replies) == 0 {
		return &ModelReply{Content: ""}, nil
	}
	r := m.replies[m.idx]
	if m.idx < len(m.replies)-1 {
		m.idx++
	}
	return &ModelReply{Content: r}, nil
}

// navBrowserStub is a NavTreeBrowser over pre-loaded rows, keyed by kbID
// (clusters) and kbID+"\x00"+name (children).
type navBrowserStub struct {
	clusters map[string][]map[string]any
	children map[string][]map[string]any
}

func (b *navBrowserStub) ListNavClusters(_ context.Context, _, kbID string, _ int) ([]map[string]any, error) {
	return b.clusters[kbID], nil
}

func (b *navBrowserStub) ListNavChildren(_ context.Context, _, kbID, name string, _ int) ([]map[string]any, error) {
	return b.children[kbID+"\x00"+name], nil
}

// navRecallStub is a Retriever recording the request and returning canned
// chunks (satisfies harness.Retriever).
type navRecallStub struct {
	chunks []map[string]any
	err    error
	req    RetrieveRequest
}

func (r *navRecallStub) Retrieve(_ context.Context, req RetrieveRequest) ([]map[string]any, error) {
	r.req = req
	return r.chunks, r.err
}

// ---------------------------------------------------------------------------
// navSelectSystem（Python _NAV_SELECT_SYSTEM）
// ---------------------------------------------------------------------------

// TestNavSelectSystemMatchesPython pins the formatted system prompt
// byte-exactly for both nouns the walk uses（Python .format(noun=noun)）.
func TestNavSelectSystemMatchesPython(t *testing.T) {
	wantClusters := `You are routing a question through a dataset's navigation tree.

You are given a QUESTION and a numbered list of clusters, each with a name and a short description.
Choose the clusters most likely to contain information relevant to answering the question.

Rules:
1. Judge only from the names and descriptions shown.
2. Be selective — include an item only if it is plausibly relevant. Include several when several are equally plausible.
3. If none are clearly relevant, return an empty list.
4. Return the bracketed index numbers of the chosen clusters.

Output ONLY JSON, no prose, no code fences:
{"relevant": [<index>, ...]}`
	if got := navSelectSystem("clusters"); got != wantClusters {
		t.Fatalf("clusters system prompt mismatch:\n%q\nwant:\n%q", got, wantClusters)
	}
	if !strings.Contains(navSelectSystem("documents"), `{"relevant": [<index>, ...]}`) {
		t.Fatal("documents system prompt must unescape the doubled braces")
	}
}

// ---------------------------------------------------------------------------
// AskNavSelect（Python _ask_nav_select）
// ---------------------------------------------------------------------------

// TestAskNavSelect_RendersItemsAndPicksIndices pins the numbered-list rendering
// (name strip, description newline flattening, doc_count / tags / entities
// heads) and the index parsing (dedup, out-of-range and non-int indices
// dropped, order follows the model's list).
func TestAskNavSelect_RendersItemsAndPicksIndices(t *testing.T) {
	items := []map[string]any{
		{"name": " History ", "description": "line1\nline2", "doc_count": 3},
		{"name": "Science", "description": "physics", "keywords": []any{"k1", "k2"}, "entities": []any{"E1"}},
		{"description": "no name here"},
	}
	mdl := &navScriptedModel{replies: []string{`{"relevant": [1, 0, 0, 9, -2, "bad"]}`}}

	got := AskNavSelect(context.Background(), mdl, "when was it built", items, "clusters", navMaxClusters)
	if len(got) != 2 {
		t.Fatalf("selected = %d items, want 2", len(got))
	}
	if got[0]["name"] != "Science" || got[1]["name"] != " History " {
		t.Fatalf("selection = %v / %v, want Science then History", got[0]["name"], got[1]["name"])
	}

	// The exact user message Python's renderer produces (:425-439).
	wantUser := "Question:\nwhen was it built\n\nClusters (numbered):\n" +
		"[0] History [3 docs]: line1 line2\n" +
		"[1] Science [tags: k1, k2] [entities: E1]: physics\n" +
		"[2] item-2: no name here" +
		"\n\nOutput JSON:"
	if len(mdl.seen) != 1 || len(mdl.seen[0]) != 2 {
		t.Fatalf("model calls = %d, want one [system, user] call", len(mdl.seen))
	}
	if mdl.seen[0][0].Content != navSelectSystem("clusters") {
		t.Fatalf("system prompt = %q", mdl.seen[0][0].Content)
	}
	if mdl.seen[0][1].Content != wantUser {
		t.Fatalf("user message mismatch:\n%q\nwant:\n%q", mdl.seen[0][1].Content, wantUser)
	}
}

// TestAskNavSelect_CleansReply pins the think-preamble / fence stripping
// （Python :445-446）before the JSON parse.
func TestAskNavSelect_CleansReply(t *testing.T) {
	items := []map[string]any{{"name": "A"}, {"name": "B"}}
	mdl := &navScriptedModel{replies: []string{
		"<think>reasoning</think>```json\n{\"relevant\": [1]}\n```",
	}}
	got := AskNavSelect(context.Background(), mdl, "q", items, "documents", 8)
	if len(got) != 1 || got[0]["name"] != "B" {
		t.Fatalf("selection = %v, want [B]", got)
	}
}

// TestAskNavSelect_FailureBestEffort mirrors Python :448-455: a failed call or
// an unparsable / non-dict reply yields nil — never an error, never a guess.
func TestAskNavSelect_FailureBestEffort(t *testing.T) {
	items := []map[string]any{{"name": "A"}}
	cases := []struct {
		name string
		mdl  *navScriptedModel
	}{
		{"model-error", &navScriptedModel{err: errors.New("down")}},
		{"prose-reply", &navScriptedModel{replies: []string{"I cannot decide."}}},
		{"empty-reply", &navScriptedModel{}},
		{"wrong-shape", &navScriptedModel{replies: []string{`{"relevant": "zero"}`}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AskNavSelect(context.Background(), tc.mdl, "q", items, "clusters", 8); got != nil {
				t.Fatalf("selection = %v, want nil", got)
			}
		})
	}
	// No items / no model: nil without touching the model.
	if got := AskNavSelect(context.Background(), &navScriptedModel{}, "q", nil, "clusters", 8); got != nil {
		t.Fatalf("no items: got %v, want nil", got)
	}
	if got := AskNavSelect(context.Background(), nil, "q", items, "clusters", 8); got != nil {
		t.Fatalf("no model: got %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// CollectNavLeaves / NavClusterNames（Python :470-511）
// ---------------------------------------------------------------------------

// TestCollectNavLeaves_BFSDedupScope pins the walk: doc leaves collected and
// deduped by doc_id, sub-clusters descended breadth-first, blank doc ids and
// out-of-scope docs dropped（Python :483-505）.
func TestCollectNavLeaves_BFSDedupScope(t *testing.T) {
	browser := &navBrowserStub{
		clusters: map[string][]map[string]any{
			"kb1": {{"name": "History", "type": "cluster"}},
		},
		children: map[string][]map[string]any{
			"kb1\x00History": {
				{"type": "doc", "doc_id": "d1", "description": "sum-1"},
				{"type": "cluster", "name": "sub"},
				{"type": "doc", "doc_id": ""}, // blank id dropped
			},
			"kb1\x00sub": {
				{"type": "doc", "doc_id": "d2", "description": "sum-2"},
				{"type": "doc", "doc_id": "d1", "description": "dup"},
				{"type": "other", "doc_id": "d3"}, // unknown type ignored
			},
		},
	}
	leaves := CollectNavLeaves(context.Background(), browser, "t1", []map[string]any{
		{"name": "History", "kb_id": "kb1"},
	}, nil)
	if len(leaves) != 2 {
		t.Fatalf("leaves = %d, want 2", len(leaves))
	}
	if leaves[0]["doc_id"] != "d1" || leaves[1]["doc_id"] != "d2" {
		t.Fatalf("leaf order = %v, %v (BFS: d1 before sub's d2)", leaves[0]["doc_id"], leaves[1]["doc_id"])
	}

	// doc_scope restricts the leaves (Python :481, :499).
	leaves = CollectNavLeaves(context.Background(), browser, "t1", []map[string]any{
		{"name": "History", "kb_id": "kb1"},
	}, []string{"d2"})
	if len(leaves) != 1 || leaves[0]["doc_id"] != "d2" {
		t.Fatalf("scoped leaves = %v, want [d2]", leaves)
	}

	// A cluster without a name never enters the frontier (Python :480).
	if leaves := CollectNavLeaves(context.Background(), browser, "t1", []map[string]any{{"kb_id": "kb1"}}, nil); len(leaves) != 0 {
		t.Fatalf("nameless cluster leaves = %v, want none", leaves)
	}
}

// TestNavClusterNames mirrors Python _nav_cluster_names: names joined, blanks
// dropped, "none" when empty.
func TestNavClusterNames(t *testing.T) {
	got := NavClusterNames([]map[string]any{
		{"name": " History "},
		{"name": ""},
		{"name": "Science"},
	})
	if got != "History, Science" {
		t.Fatalf("names = %q", got)
	}
	if got := NavClusterNames(nil); got != "none" {
		t.Fatalf("empty names = %q, want none", got)
	}
}

// ---------------------------------------------------------------------------
// ContentRecallDocs（Python _content_recall_docs）
// ---------------------------------------------------------------------------

// TestContentRecallDocs_AggregatesMostHitFirst pins the doc aggregation order
// and the request shape（Python :539-553: top_n 40, threshold 0.2, hybrid 0.3
// with an embedder, keyword-only without）.
func TestContentRecallDocs_AggregatesMostHitFirst(t *testing.T) {
	newChunks := func() []map[string]any {
		return []map[string]any{
			{"doc_id": "d1"},
			{"doc_id": "d2"},
			{"doc_id": "d1"},
			{"doc_id": " "}, // blank id dropped
			{"doc_id": "d1"},
		}
	}

	t.Run("no-embedder-keyword-only", func(t *testing.T) {
		stub := &navRecallStub{chunks: newChunks()}
		got := ContentRecallDocs(context.Background(), stub, "q", nil, "t1", []string{"kb1"}, false)
		if len(got) != 2 || got[0] != "d1" || got[1] != "d2" {
			t.Fatalf("docs = %v, want [d1 d2] most-hit-first", got)
		}
		if stub.req.TopN != navRecallTopN {
			t.Fatalf("TopN = %d, want %d", stub.req.TopN, navRecallTopN)
		}
		if stub.req.SimilarityThreshold == nil || *stub.req.SimilarityThreshold != navRecallMinScore {
			t.Fatalf("threshold = %v, want 0.2", stub.req.SimilarityThreshold)
		}
		if !stub.req.DisableVectorLeg || stub.req.VectorSimilarityWeight != nil {
			t.Fatal("no embedder: the dense leg must be disabled entirely (embd_mdl=None)")
		}
	})
	t.Run("embedder-hybrid", func(t *testing.T) {
		stub := &navRecallStub{chunks: newChunks()}
		_ = ContentRecallDocs(context.Background(), stub, "q", []string{"d9"}, "t1", []string{"kb1"}, true)
		if stub.req.DisableVectorLeg {
			t.Fatal("embedder: the dense leg must run")
		}
		if stub.req.VectorSimilarityWeight == nil || *stub.req.VectorSimilarityWeight != navRecallVectorWeight {
			t.Fatalf("vector weight = %v, want 0.3", stub.req.VectorSimilarityWeight)
		}
		if len(stub.req.DocScope) != 1 || stub.req.DocScope[0] != "d9" {
			t.Fatalf("doc scope forwarded = %v, want [d9]", stub.req.DocScope)
		}
	})
	t.Run("guards", func(t *testing.T) {
		stub := &navRecallStub{chunks: newChunks()}
		if got := ContentRecallDocs(context.Background(), stub, "  ", nil, "t1", []string{"kb1"}, true); got != nil {
			t.Fatalf("blank query: %v, want nil", got)
		}
		if got := ContentRecallDocs(context.Background(), stub, "q", nil, "t1", nil, true); got != nil {
			t.Fatalf("no KBs: %v, want nil", got)
		}
		if got := ContentRecallDocs(context.Background(), nil, "q", nil, "t1", []string{"kb1"}, true); got != nil {
			t.Fatalf("no backend: %v, want nil", got)
		}
	})
	t.Run("retrieval-failure", func(t *testing.T) {
		stub := &navRecallStub{err: errors.New("ES down")}
		if got := ContentRecallDocs(context.Background(), stub, "q", nil, "t1", []string{"kb1"}, true); got != nil {
			t.Fatalf("failure: %v, want nil", got)
		}
	})
}

// ---------------------------------------------------------------------------
// NavigateTree wiring: the sweep verdict is FINAL (Python: deliberately NO
// fallback, navigation.py:902-912)
// ---------------------------------------------------------------------------

// TestNavigateTree_QueryMissStaysNoDoc pins the Python semantics: when the sweep
// routes to nothing (structure exists), the verdict is no_doc EVEN when the
// LLM-walk seams are present — navigation.py:902-912 deliberately has no
// fallback here ("the generic retriever reinventing what dataset_nav already
// does ... the most expensive thing this file could do"). The orchestrator, not
// the route call, owns any retrieval fallback.
func TestNavigateTree_QueryMissStaysNoDoc(t *testing.T) {
	router := &stubRouter{emptyResult: true}
	browser := &navBrowserStub{
		clusters: map[string][]map[string]any{
			"kb1": {{"name": "History", "type": "cluster", "description": "historical events"}},
		},
		children: map[string][]map[string]any{
			"kb1\x00History": {{"type": "doc", "doc_id": "d1", "description": "The founding of the colony"}},
		},
	}
	mdl := &navScriptedModel{}
	got := NavigateTree(context.Background(), router, NavTreeInput{
		Query:       "who founded it",
		KbIDs:       []string{"kb1"},
		TenantID:    "t1",
		Model:       mdl,
		Source:      browser,
		Backend:     &navRecallStub{},
		HasEmbedder: true,
	})
	if got.EmptyReason != ReasonNoDoc {
		t.Fatalf("empty_reason = %q, want %q (no LLM-walk fallback)", got.EmptyReason, ReasonNoDoc)
	}
	if len(got.DocIDs) != 0 {
		t.Fatalf("doc_ids = %v, want none", got.DocIDs)
	}
	if len(mdl.seen) != 0 {
		t.Fatalf("model calls = %d, want 0 (the walk must never run from the route path)", len(mdl.seen))
	}
}

// TestNavigateTree_NoTreeStaysNoStructure: a dataset with NO compiled tree is a
// DATASET-level verdict (no_structure), not something the route call rescues by
// content-recalling (navigation.py:902-912).
func TestNavigateTree_NoTreeStaysNoStructure(t *testing.T) {
	router := &stubRouter{nilResult: true}
	browser := &navBrowserStub{clusters: map[string][]map[string]any{}}
	backend := &navRecallStub{chunks: []map[string]any{
		{"doc_id": "d1"},
		{"doc_id": "d2"},
		{"doc_id": "d1"},
	}}
	got := NavigateTree(context.Background(), router, NavTreeInput{
		Query:    "q",
		KbIDs:    []string{"kb1"},
		TenantID: "t1",
		Model:    &navScriptedModel{},
		Source:   browser,
		Backend:  backend,
	})
	if got.EmptyReason != ReasonNoStructure {
		t.Fatalf("empty_reason = %q, want %q (deliberately NO fallback)", got.EmptyReason, ReasonNoStructure)
	}
	if len(got.DocIDs) != 0 {
		t.Fatalf("doc_ids = %v, want none", got.DocIDs)
	}
}

// TestNavigateTree_EmptyVerdictsHold: with the fallback gone, both misses keep
// their original verdicts (no_doc / no_structure).
func TestNavigateTree_EmptyVerdictsHold(t *testing.T) {
	browser := &navBrowserStub{clusters: map[string][]map[string]any{}}
	cases := []struct {
		name   string
		router *stubRouter
		want   string
	}{
		{"no-doc", &stubRouter{emptyResult: true}, ReasonNoDoc},
		{"no-structure", &stubRouter{nilResult: true}, ReasonNoStructure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NavigateTree(context.Background(), tc.router, NavTreeInput{
				Query:    "q",
				KbIDs:    []string{"kb1"},
				TenantID: "t1",
				Model:    &navScriptedModel{},
				Source:   browser,
				Backend:  &navRecallStub{}, // recalls nothing
			})
			if got.EmptyReason != tc.want {
				t.Fatalf("empty_reason = %q, want %q", got.EmptyReason, tc.want)
			}
		})
	}
}
