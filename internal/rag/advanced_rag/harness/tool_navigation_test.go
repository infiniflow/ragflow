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
)

// ---------------------------------------------------------------------------
// Compiled-structure rendering / normalization
// ---------------------------------------------------------------------------

func TestNormalizeKind(t *testing.T) {
	cases := []struct {
		row  map[string]interface{}
		want string
	}{
		{map[string]interface{}{"compile_kwd": "raptor_graph"}, "raptor"},
		{map[string]interface{}{"compilation_template_kind_kwd": "page_index"}, "timeline"},
		{map[string]interface{}{"compilation_template_kind_kwd": "knowledge_graph"}, "timeline"},
		{map[string]interface{}{"compilation_template_kind_kwd": "mindmap"}, "mindmap"},
		{map[string]interface{}{"compile_kwd": "tree"}, "tree"},
	}
	for _, c := range cases {
		if got := normalizeKind(c.row); got != c.want {
			t.Errorf("normalizeKind(%v) = %q, want %q", c.row, got, c.want)
		}
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// navigate_tree routing
// ---------------------------------------------------------------------------

type stubRouter struct {
	table       [][2]string
	nilResult   bool
	emptyResult bool
	err         error
	gotQuery    string
	gotDocScope []string
	gotTopK     int
}

func (s *stubRouter) Route(_ context.Context, _, _, query string, docScope []string, topK int) ([][2]string, error) {
	s.gotQuery = query
	s.gotDocScope = docScope
	s.gotTopK = topK
	if s.err != nil {
		return nil, s.err
	}
	if s.nilResult {
		return nil, nil
	}
	if s.emptyResult {
		return [][2]string{}, nil
	}
	return s.table, nil
}

func TestNavigateTreeRoutesAndRendersXML(t *testing.T) {
	router := &stubRouter{table: [][2]string{
		{"doc-c", "Culdcept history and design"},
		{"doc-a", "OmiyaSoft company profile"},
		{"doc-b", ""},
	}}
	got := NavigateTree(context.Background(), router, NavTreeInput{
		Query:    "Who created Culdcept?",
		Keywords: "Culdcept, creator",
		KbIDs:    []string{"kb1"},
	})
	if got.EmptyReason != "" {
		t.Fatalf("empty_reason = %q, want a successful route", got.EmptyReason)
	}
	if len(got.DocIDs) != 3 {
		t.Fatalf("doc_ids = %v, want 3", got.DocIDs)
	}
	if len(got.RoutedDocs) != 3 {
		t.Fatalf("routed_docs = %v, want 3", got.RoutedDocs)
	}
	if got.RoutedDocs[0][1] != "Culdcept history and design" {
		t.Errorf("routed summary = %q", got.RoutedDocs[0][1])
	}
	if !strings.Contains(router.gotQuery, "Culdcept, creator") {
		t.Errorf("routing query = %q, want topic + keywords", router.gotQuery)
	}
	if !strings.Contains(got.Text, `count="3"`) {
		t.Errorf("XML missing the count: %s", got.Text)
	}
	if !strings.Contains(got.Text, `doc_id="doc-c"`) {
		t.Errorf("XML missing doc ids: %s", got.Text)
	}
	if !strings.Contains(got.Text, "<summary>Culdcept history and design</summary>") {
		t.Errorf("XML missing the summary: %s", got.Text)
	}
	if !strings.Contains(got.Text, `doc_id="doc-b"/>`) {
		t.Errorf("summary-less doc must be self-closing: %s", got.Text)
	}
	if !strings.HasSuffix(strings.TrimSpace(got.Text), "</tree_navigation>") {
		t.Errorf("XML not closed: %s", got.Text)
	}
}

func TestNavigateTreeDistinguishesNoStructureFromNoDoc(t *testing.T) {
	got := NavigateTree(context.Background(), &stubRouter{nilResult: true}, NavTreeInput{
		Query: "q", KbIDs: []string{"kb1"},
	})
	if got.EmptyReason != ReasonNoStructure {
		t.Errorf("no tree: reason = %q, want %q", got.EmptyReason, ReasonNoStructure)
	}
	if got.HasStructure() {
		t.Error("no tree must report HasStructure=false")
	}
	got = NavigateTree(context.Background(), &stubRouter{emptyResult: true}, NavTreeInput{
		Query: "q", KbIDs: []string{"kb1"},
	})
	if got.EmptyReason != ReasonNoDoc {
		t.Errorf("routed to nothing: reason = %q, want %q", got.EmptyReason, ReasonNoDoc)
	}
	if !got.HasStructure() {
		t.Error("a query-level miss must still report HasStructure=true")
	}
}

func TestNavigateTreeBadArgsAndInfra(t *testing.T) {
	if got := NavigateTree(context.Background(), &stubRouter{}, NavTreeInput{}); got.EmptyReason != ReasonBadArgs {
		t.Errorf("no query: reason = %q, want bad_args", got.EmptyReason)
	}
	if got := NavigateTree(context.Background(), nil, NavTreeInput{Query: "q"}); got.EmptyReason != ReasonInfra {
		t.Errorf("no router: reason = %q, want infra", got.EmptyReason)
	}
	got := NavigateTree(context.Background(), &stubRouter{err: errors.New("ES down")}, NavTreeInput{
		Query: "q", KbIDs: []string{"kb1"},
	})
	if got.EmptyReason != ReasonNoStructure {
		t.Errorf("backend error: reason = %q, want no_structure (no dataset verdict)", got.EmptyReason)
	}
}

func TestNavigateTreeCachesBestPerDocAcrossDatasets(t *testing.T) {
	router := &stubRouter{table: [][2]string{
		{"doc-a", "summary from kb1"},
		{"doc-a", "summary from kb2"},
		{"doc-b", "b"},
	}}
	got := NavigateTree(context.Background(), router, NavTreeInput{
		Query: "q", KbIDs: []string{"kb1", "kb2"},
	})
	if len(got.DocIDs) != 2 {
		t.Fatalf("doc_ids = %v, want 2 (deduped)", got.DocIDs)
	}
}

func TestNavigateTreeCapsAt8(t *testing.T) {
	var table [][2]string
	for i := 0; i < 20; i++ {
		table = append(table, [2]string{string(rune('a'+i%26)) + string(rune('0'+i/26)), "s"})
	}
	got := NavigateTree(context.Background(), &stubRouter{table: table}, NavTreeInput{
		Query: "q", KbIDs: []string{"kb1"},
	})
	if len(got.DocIDs) != navTreeMaxDocs {
		t.Errorf("doc_ids = %d, want capped at %d", len(got.DocIDs), navTreeMaxDocs)
	}
}

func TestNavigateTreeEscapesXML(t *testing.T) {
	router := &stubRouter{table: [][2]string{{"d&<>\"'1", "a < b & c"}}}
	got := NavigateTree(context.Background(), router, NavTreeInput{
		Query: `who "made" <it> & that`, KbIDs: []string{"kb1"},
	})
	if strings.Contains(got.Text, "<it>") {
		t.Errorf("query not escaped: %s", got.Text)
	}
	if !strings.Contains(got.Text, "&lt;it&gt;") {
		t.Errorf("query not escaped: %s", got.Text)
	}
	if strings.Contains(got.Text, `doc_id="d&<>\"'1"`) {
		t.Errorf("doc_id not escaped: %s", got.Text)
	}
}

// ---------------------------------------------------------------------------
// Compiled-structure parsing
// ---------------------------------------------------------------------------

func TestParseCompiledStructureGraphBlob(t *testing.T) {
	rows := []StructureRow{{
		CompileKwd:        "tree",
		KnowledgeGraphKwd: "graph",
		Content:           `{"entities": [{"name": "OmiyaSoft"}, {"name": "Culdcept"}], "relations": [{"src": "OmiyaSoft", "dst": "Culdcept"}]}`,
	}}
	ents, rels := ParseCompiledStructure(rows, []string{"tree"})
	if len(ents) != 2 || len(rels) != 1 {
		t.Fatalf("blob: entities=%d relations=%d, want 2/1", len(ents), len(rels))
	}
}

func TestParseCompiledStructurePerEntityRows(t *testing.T) {
	rows := []StructureRow{
		{CompileKwd: "page_index", KnowledgeGraphKwd: "entity", Content: `{"name": "OmiyaSoft"}`},
		{CompileKwd: "page_index", KnowledgeGraphKwd: "entity", Content: `{"name": "Culdcept"}`},
		{CompileKwd: "page_index", KnowledgeGraphKwd: "relation", Content: `{"src": "OmiyaSoft", "dst": "Culdcept"}`},
	}
	ents, rels := ParseCompiledStructure(rows, []string{"timeline"})
	if len(ents) != 2 || len(rels) != 1 {
		t.Fatalf("per-row: entities=%d relations=%d, want 2/1", len(ents), len(rels))
	}
}

func TestParseCompiledStructureKindFilterAndNormalization(t *testing.T) {
	rows := []StructureRow{
		{CompileKwd: "page_index", KnowledgeGraphKwd: "entity", Content: `{"name": "A"}`},
		{CompileKwd: "knowledge_graph", KnowledgeGraphKwd: "entity", Content: `{"name": "B"}`},
		{CompileKwd: "tree", KnowledgeGraphKwd: "entity", Content: `{"name": "C"}`},
		{CompileKwd: "raptor_graph", KnowledgeGraphKwd: "graph", Content: `{"entities": [{"name": "D"}]}`},
	}
	ents, _ := ParseCompiledStructure(rows, []string{"timeline"})
	if len(ents) != 2 {
		t.Errorf("timeline filter = %d entities, want 2 (page_index + knowledge_graph)", len(ents))
	}
	ents, _ = ParseCompiledStructure(rows, []string{"raptor"})
	if len(ents) != 1 {
		t.Errorf("raptor filter = %d entities, want 1", len(ents))
	}
	ents, _ = ParseCompiledStructure([]StructureRow{
		{CompileKwd: "", TemplateKind: "tree", KnowledgeGraphKwd: "entity", Content: `{"name": "E"}`},
	}, []string{"tree"})
	if len(ents) != 1 {
		t.Errorf("template_kind fallback = %d, want 1", len(ents))
	}
}

func TestParseCompiledStructureSkipsMalformed(t *testing.T) {
	rows := []StructureRow{
		{CompileKwd: "tree", KnowledgeGraphKwd: "entity", Content: "not json"},
		{CompileKwd: "tree", KnowledgeGraphKwd: "entity", Content: `[1,2,3]`},
		{CompileKwd: "tree", KnowledgeGraphKwd: "unknown_shape", Content: `{"name":"X"}`},
		{CompileKwd: "tree", KnowledgeGraphKwd: "entity", Content: `{"name": "ok"}`},
	}
	ents, _ := ParseCompiledStructure(rows, []string{"tree"})
	if len(ents) != 1 {
		t.Errorf("entities = %d, want 1 (malformed rows skipped)", len(ents))
	}
}

func TestParseCompiledStructureEmptyKindsMatchesAll(t *testing.T) {
	rows := []StructureRow{
		{CompileKwd: "tree", KnowledgeGraphKwd: "entity", Content: `{"name": "A"}`},
		{CompileKwd: "other", KnowledgeGraphKwd: "entity", Content: `{"name": "B"}`},
	}
	ents, _ := ParseCompiledStructure(rows, nil)
	if len(ents) != 2 {
		t.Errorf("no filter = %d entities, want 2", len(ents))
	}
}

// ---------------------------------------------------------------------------
// Knowledge-graph exploration
// ---------------------------------------------------------------------------

// TestExploreGraphShortCircuitsOnEmptyInput verifies the documented contract:
// no query text or no bound datasets yields an empty ExploreResult (no engine
// call). The engine-backed happy path needs a live Infinity backend and is not
// exercised in the unit tier.
func TestExploreGraphShortCircuitsOnEmptyInput(t *testing.T) {
	// No query text.
	if res, err := ExploreGraph(context.Background(), SearchDeps{}, "t1", []string{"kb1"}, "", "", nil); err != nil || res.Answer != "" || len(res.Chunks) != 0 {
		t.Errorf("empty query -> (%+v, %v), want empty result", res, err)
	}
	// No bound datasets.
	if res, err := ExploreGraph(context.Background(), SearchDeps{}, "t1", nil, "OmiyaSoft", "", nil); err != nil || res.Answer != "" || len(res.Chunks) != 0 {
		t.Errorf("no datasets -> (%+v, %v), want empty result", res, err)
	}
}

// TestExploreGraphUnconfiguredEngineErrors verifies a missing engine is
// surfaced rather than silently returning empty.
func TestExploreGraphUnconfiguredEngineErrors(t *testing.T) {
	if _, err := ExploreGraph(context.Background(), SearchDeps{}, "t1", []string{"kb1"}, "OmiyaSoft", "", nil); err == nil {
		t.Error("expected an error when the engine is not configured")
	}
}

// stubTenantResolver is a DocTenantResolver test double that assigns a fixed
// (kb, tenant) to every document id it is asked about.
type stubTenantResolver struct {
	owner DocTenant
}

func (s stubTenantResolver) ResolveDocTenants(ctx context.Context, docIDs []string) (map[string]DocTenant, error) {
	out := make(map[string]DocTenant, len(docIDs))
	for _, d := range docIDs {
		out[d] = s.owner
	}
	return out, nil
}

// TestResolveKGScopeGroupsByOwner mirrors Python _kg_scopes: documents are
// grouped by their real owning (kb, tenant) — which may be a KB outside the
// caller's datasetIDs — rather than every bound dataset being searched with
// the whole DocScope.
func TestResolveKGScopeGroupsByOwner(t *testing.T) {
	// No DocScope: one whole-dataset group per bound dataset.
	got := resolveKGScope(SearchDeps{TenantID: "t1"}, nil, []string{"kb1", "kb2"})
	if len(got) != 2 {
		t.Fatalf("no-docScope scopes = %d, want 2", len(got))
	}
	for _, s := range got {
		if len(s.Docs) != 0 {
			t.Errorf("scope %s has Docs=%v, want nil (whole-dataset)", s.KBID, s.Docs)
		}
	}

	// DocScope + resolver that maps every doc to a NEW kb outside datasetIDs.
	got = resolveKGScope(SearchDeps{
		TenantID:          "t1",
		DocTenantResolver: stubTenantResolver{owner: DocTenant{KBID: "otherKB", TenantID: "t9"}},
	}, []string{"d1", "d2"}, []string{"kb1"})
	if len(got) != 1 {
		t.Fatalf("resolver scopes = %d, want 1 (new kb from resolver)", len(got))
	}
	if got[0].KBID != "otherKB" || got[0].TenantID != "t9" {
		t.Errorf("scope = %+v, want KBID=otherKB TenantID=t9", got[0])
	}
	if len(got[0].Docs) != 2 {
		t.Errorf("scope Docs = %v, want [d1 d2]", got[0].Docs)
	}

	// DocScope but NO resolver: fall back to old behaviour (each bound dataset
	// searched with the whole DocScope).
	got = resolveKGScope(SearchDeps{TenantID: "t1"}, []string{"d1"}, []string{"kb1", "kb2"})
	if len(got) != 2 {
		t.Fatalf("no-resolver scopes = %d, want 2", len(got))
	}
	for _, s := range got {
		if len(s.Docs) != 1 || s.Docs[0] != "d1" {
			t.Errorf("scope %s Docs=%v, want [d1]", s.KBID, s.Docs)
		}
	}
}

// TestResolveKGScopeAppliesSessionDocScopeCeiling verifies Python's
// scoped_doc_ids ceiling (exploration.py:_kg_scopes): the session doc_scope restricts
// whatever scope the caller passed before ownership resolution.
func TestResolveKGScopeAppliesSessionDocScopeCeiling(t *testing.T) {
	got := resolveKGScope(SearchDeps{
		TenantID:          "t1",
		DocScope:          []string{"d1", "d2"}, // session ceiling
		DocTenantResolver: stubTenantResolver{owner: DocTenant{KBID: "kb1", TenantID: "t1"}},
	}, []string{"d1", "drop", "d2"}, []string{"kb1"})
	if len(got) != 1 || len(got[0].Docs) != 2 {
		t.Fatalf("after ceiling scopes = %+v, want 1 scope with [d1 d2]", got)
	}
	if got[0].Docs[0] != "d1" || got[0].Docs[1] != "d2" {
		t.Errorf("docs = %v, want [d1 d2] (drop is outside the session scope)", got[0].Docs)
	}
}

// TestResolveKGScopeNonNilEmptyExpandsNothing pins the resolveDocScope "match
// nothing" signal: a non-nil empty scope must yield no search groups at all
// (not a whole-dataset scan).
func TestResolveKGScopeNonNilEmptyExpandsNothing(t *testing.T) {
	if got := resolveKGScope(SearchDeps{TenantID: "t1"}, []string{}, []string{"kb1", "kb2"}); len(got) != 0 {
		t.Fatalf("scopes = %+v, want none for an explicit empty (match nothing) scope", got)
	}
}

// ---------------------------------------------------------------------------
// Knowledge-graph row parsing
// ---------------------------------------------------------------------------

func TestKgParseEntity_NameFallbacks(t *testing.T) {
	// name wins.
	if e, ok := kgParseEntity(map[string]interface{}{"content_with_weight": `{"name":"OmiyaSoft"}`}); !ok || e.Name != "OmiyaSoft" {
		t.Fatalf("name parse = %+v ok=%v", e, ok)
	}
	// term fallback.
	if e, ok := kgParseEntity(map[string]interface{}{"content_with_weight": `{"term":"Culdcept"}`}); !ok || e.Name != "Culdcept" {
		t.Fatalf("term fallback = %+v ok=%v", e, ok)
	}
	// title fallback.
	if e, ok := kgParseEntity(map[string]interface{}{"content_with_weight": `{"title":"Rocket"}`}); !ok || e.Name != "Rocket" {
		t.Fatalf("title fallback = %+v ok=%v", e, ok)
	}
	// aliases parsed + doc_id carried.
	row := map[string]interface{}{
		"content_with_weight": `{"name":"OmiyaSoft","type":"company","description":"dev","aliases":["Omiya","OS"]}`,
		"doc_id":              "d1",
		"source_chunk_ids":    []interface{}{"c1", "c2"},
	}
	e, ok := kgParseEntity(row)
	if !ok || e.Name != "OmiyaSoft" || e.Type != "company" || e.DocID != "d1" {
		t.Fatalf("entity = %+v ok=%v", e, ok)
	}
	if len(e.Aliases) != 2 || e.Aliases[0] != "Omiya" {
		t.Errorf("aliases = %v, want [Omiya OS]", e.Aliases)
	}
	if len(e.SourceChunkIDs) != 2 {
		t.Errorf("source_chunk_ids = %v, want [c1 c2]", e.SourceChunkIDs)
	}
}

func TestKgParseEntityRejectsEmptyName(t *testing.T) {
	cases := []string{
		`{"type":"x"}`,  // no name
		`{"name":""}`,   // empty name
		`{"name":null}`, // null name
		`not json`,      // unparsable
		`[1,2,3]`,       // not an object
	}
	for _, c := range cases {
		if _, ok := kgParseEntity(map[string]interface{}{"content_with_weight": c}); ok {
			t.Errorf("content %q should not parse to an entity", c)
		}
	}
}

func TestKgParseRelationEndpointsRequired(t *testing.T) {
	// Missing from endpoint.
	if _, ok := kgParseRelation(map[string]interface{}{"to_entity_kwd": "B"}); ok {
		t.Error("relation without from endpoint must be rejected")
	}
	// Missing to endpoint.
	if _, ok := kgParseRelation(map[string]interface{}{"from_entity_kwd": "A"}); ok {
		t.Error("relation without to endpoint must be rejected")
	}
	// Valid: type from content payload takes precedence over default.
	row := map[string]interface{}{
		"from_entity_kwd":     "A",
		"to_entity_kwd":       "B",
		"content_with_weight": `{"type":"founded"}`,
		"doc_id":              "d1",
	}
	r, ok := kgParseRelation(row)
	if !ok || r.From != "A" || r.To != "B" || r.Type != "founded" || r.DocID != "d1" {
		t.Fatalf("relation = %+v ok=%v", r, ok)
	}
	// Default type when payload has neither type nor relation.
	r2, ok := kgParseRelation(map[string]interface{}{
		"from_entity_kwd": "A", "to_entity_kwd": "B",
		"content_with_weight": `{}`,
	})
	if !ok || r2.Type != "related" {
		t.Errorf("default relation type = %q, want related", r2.Type)
	}
}

// ---------------------------------------------------------------------------
// endpoint terms (original + lowercased, deduped)
// ---------------------------------------------------------------------------

func TestEndpointTerms(t *testing.T) {
	got := endpointTerms([]string{"OmiyaSoft", "omiya", "", "Culdcept"})
	want := map[string]bool{
		"OmiyaSoft": true, "omiyasoft": true, "omiya": true, "Culdcept": true, "culdcept": true,
	}
	if len(got) != len(want) {
		t.Fatalf("endpointTerms = %v, want %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected term %q", g)
		}
	}
}

// ---------------------------------------------------------------------------
// Evidence collection: relevant entities + relations grouped by doc
// ---------------------------------------------------------------------------

func TestCollectEvidenceIDsByDocAndAlias(t *testing.T) {
	entities := []kgEntity{
		{Name: "OmiyaSoft", Aliases: []string{"Omiya"}, DocID: "d1", SourceChunkIDs: []string{"c1", "c2"}},
		{Name: "Culdcept", DocID: "d2", SourceChunkIDs: []string{"c3"}},
	}
	relations := []kgRelation{
		{From: "OmiyaSoft", To: "Culdcept", DocID: "d1", SourceChunkIDs: []string{"c4"}},
	}
	// Relevant set includes an alias ("Omiya") and the relation target ("Culdcept").
	byDoc := collectEvidenceIDs(entities, relations, []string{"Omiya", "Culdcept"})
	// Doc order is FIRST-SEEN — Python's dict insertion order (d1 before d2).
	if len(byDoc) != 2 || byDoc[0].DocID != "d1" || byDoc[1].DocID != "d2" {
		t.Fatalf("doc order = %v, want [d1 d2]", byDoc)
	}
	if got := byDoc[0].IDs; len(got) != 3 || got[0] != "c1" || got[1] != "c2" || got[2] != "c4" {
		t.Errorf("d1 chunks = %v, want [c1 c2 c4]", got)
	}
	if got := byDoc[1].IDs; len(got) != 1 || got[0] != "c3" {
		t.Errorf("d2 chunks = %v, want [c3]", got)
	}
	// A non-relevant entity must contribute nothing.
	byDoc = collectEvidenceIDs(entities, relations, []string{"nobody"})
	if len(byDoc) != 0 {
		t.Errorf("non-relevant selection = %v, want empty", byDoc)
	}
}

func TestCollectEvidenceIDsDeduplicatesChunks(t *testing.T) {
	entities := []kgEntity{
		{Name: "A", DocID: "d1", SourceChunkIDs: []string{"c1", "c1"}},
	}
	rel := []kgRelation{
		{From: "A", To: "B", DocID: "d1", SourceChunkIDs: []string{"c1"}},
	}
	byDoc := collectEvidenceIDs(entities, rel, []string{"A", "B"})
	if len(byDoc) != 1 || byDoc[0].DocID != "d1" {
		t.Fatalf("byDoc = %v, want a single d1 group", byDoc)
	}
	seen := map[string]int{}
	for _, c := range byDoc[0].IDs {
		seen[c]++
	}
	if seen["c1"] != 1 {
		t.Errorf("chunk c1 counted %d times, want 1 (deduped)", seen["c1"])
	}
}

// ---------------------------------------------------------------------------
// mention_count re-ranking (the dense-seed re-sort)
// ---------------------------------------------------------------------------

func TestMentionCountTypeCoercion(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int
	}{
		{int(5), 5},
		{int64(7), 7},
		{float64(9), 9},
		{float32(3), 3},
		{nil, 0},
		{"x", 0},
	}
	for _, c := range cases {
		if got := mentionCount(map[string]interface{}{"mention_count_int": c.in}); got != c.want {
			t.Errorf("mentionCount(%T(%v)) = %d, want %d", c.in, c.in, got, c.want)
		}
	}
}

func TestTopMentionCountSortsAndCaps(t *testing.T) {
	rows := []map[string]interface{}{
		{"mention_count_int": 1, "name": "a"},
		{"mention_count_int": 10, "name": "b"},
		{"mention_count_int": 5, "name": "c"},
	}
	got := topMentionCount(rows, 2)
	if len(got) != 2 || mentionCount(got[0]) != 10 || mentionCount(got[1]) != 5 {
		t.Fatalf("topMentionCount = %v, want [10 5]", got)
	}
}

// ---------------------------------------------------------------------------
// String-field helpers
// ---------------------------------------------------------------------------

func TestStrSliceField(t *testing.T) {
	if got := strSliceField([]string{"a", "b"}); len(got) != 2 {
		t.Errorf("[]string path lost elements: %v", got)
	}
	if got := strSliceField([]interface{}{"a", 2, "b"}); len(got) != 2 {
		t.Errorf("[]interface{} path must skip non-strings: %v", got)
	}
	if got := strSliceField(42); got != nil {
		t.Errorf("scalar path must yield nil, got %v", got)
	}
}

func TestStrOr(t *testing.T) {
	if strOr("x", "def") != "x" {
		t.Error("non-empty value must win")
	}
	if strOr("", "def") != "def" {
		t.Error("empty value must fall back")
	}
	if strOr(nil, "def") != "def" {
		t.Error("non-string must fall back")
	}
}

// TestBuildTocTreeHierarchyFromRelations guards the navigate_structure relations
// wiring: when relations are loaded (Python passes them into
// _render_toc_drilldown / _build_toc_tree) the tree must expose the parent→child
// hierarchy and a single root, instead of collapsing to isolated roots the way
// an empty/nil relation list does. Mirrors Python _build_toc_tree.
func TestBuildTocTreeHierarchyFromRelations(t *testing.T) {
	nodes := []structureNode{
		{name: "root"},
		{name: "A"},
		{name: "A1"},
		{name: "A2"},
	}
	rels := []structureRel{
		{from: "root", to: "A"},
		{from: "A", to: "A1"},
		{from: "A", to: "A2"},
	}
	byName, children, parents, roots := buildTocTree(nodes, rels)
	if len(byName) != 4 {
		t.Fatalf("byName = %d, want 4", len(byName))
	}
	if len(roots) != 1 || roots[0] != "root" {
		t.Fatalf("roots = %v, want single root \"root\"", roots)
	}
	if parents["A"] != "root" || parents["A1"] != "A" || parents["A2"] != "A" {
		t.Fatalf("parents = %v, want A->root and A1/A2->A", parents)
	}
	if len(children["A"]) != 2 {
		t.Fatalf("children[A] = %v, want [A1 A2]", children["A"])
	}
}

// TestBuildTocTreeNoRelationsYieldsIsolatedRoots documents the pre-fix shape: a
// nil/empty relation list leaves every node without a parent, so each node is its
// own root and no parent→child hierarchy can be descended.
func TestBuildTocTreeNoRelationsYieldsIsolatedRoots(t *testing.T) {
	nodes := []structureNode{
		{name: "root"},
		{name: "A"},
		{name: "A1"},
	}
	_, _, _, roots := buildTocTree(nodes, nil)
	if len(roots) != 3 {
		t.Fatalf("roots without relations = %v, want all 3 isolated", roots)
	}
}

// TestStructureDocSegmentFormat locks the per-doc XML shape navigateStructures
// renders for each routed/requested document, mirroring Python's
// <doc rank doc_id doc_title="" entities relations> element (navigation.py
// _navigate_structure_impl, where doc_title is always empty).
func TestStructureDocSegmentFormat(t *testing.T) {
	nodes := []structureNode{{name: "N1"}, {name: "N2"}}
	rels := []structureRel{{from: "N0", to: "N1"}}
	drill := structureDrillout{outline: "· N1 [chunks: c1]"}

	seg := structureDocSegment("docA", "q", "catalog", 2, nodes, rels, drill)
	want := `  <doc rank="2" doc_id="docA" doc_title="" entities="2" relations="1">`
	if !strings.Contains(seg, want) {
		t.Fatalf("segment missing header %q:\n%s", want, seg)
	}
	if !strings.Contains(seg, "\n    <structure>· N1 [chunks: c1]</structure>") {
		t.Fatalf("segment missing structure outline:\n%s", seg)
	}
	if !strings.Contains(seg, "\n  </doc>") {
		t.Fatalf("segment missing </doc> close:\n%s", seg)
	}
}

// TestEmptyReasonLabel verifies the empty_reason→XML error label mapping used by
// the single- and multi-document renderers.
func TestEmptyReasonLabel(t *testing.T) {
	cases := map[string]string{
		ReasonBadArgs:     "query is required",
		ReasonNoDoc:       "no document located",
		ReasonNoStructure: "no structure",
	}
	for in, want := range cases {
		if got := emptyReasonLabel(in); got != want {
			t.Errorf("emptyReasonLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestVecsEqual(t *testing.T) {
	if !vecsEqual([]float64{1, 2, 3}, []float64{1, 2, 3}) {
		t.Error("identical vectors should be equal")
	}
	if vecsEqual([]float64{1, 2, 3}, []float64{1, 2, 4}) {
		t.Error("differing vectors should not be equal")
	}
	if vecsEqual([]float64{1, 2}, []float64{1, 2, 3}) {
		t.Error("differently-sized vectors should not be equal")
	}
	if vecsEqual(nil, []float64{1}) {
		t.Error("empty vs non-empty should not be equal")
	}
}

func TestHasDistinctNodeVectors(t *testing.T) {
	if hasDistinctNodeVectors([]structureNode{{name: "a"}, {name: "b"}}) {
		t.Error("no vectors at all -> not distinct")
	}
	if hasDistinctNodeVectors([]structureNode{{name: "a", vec: []float64{1, 2}}}) {
		t.Error("a single vector-bearing node -> not distinct")
	}
	// RAPTOR: every node inherits the same blob vector.
	if hasDistinctNodeVectors([]structureNode{
		{name: "a", vec: []float64{1, 2}},
		{name: "b", vec: []float64{1, 2}},
	}) {
		t.Error("all nodes sharing one vector -> not distinct")
	}
	// page_index: per-node vectors differ.
	if !hasDistinctNodeVectors([]structureNode{
		{name: "a", vec: []float64{1, 2}},
		{name: "b", vec: []float64{3, 4}},
	}) {
		t.Error("distinct per-node vectors -> distinct")
	}
	// One vector-bearing among several is treated as not-distinct (Python: <2 distinct).
	if hasDistinctNodeVectors([]structureNode{
		{name: "a", vec: []float64{1, 2}},
		{name: "b"},
	}) {
		t.Error("one vector-bearing among two -> not distinct")
	}
}

func TestNodesCoveringChunks(t *testing.T) {
	nodes := []structureNode{
		{name: "A", sourceChunkIDs: []string{"c1", "c2"}},
		{name: "B", sourceChunkIDs: []string{"c3"}},
		{name: "C"},
	}
	got := nodesCoveringChunks(nodes, []string{"c2", "c9"})
	if len(got) != 1 || got[0] != "A" {
		t.Errorf("covering = %v, want [A]", got)
	}
	if got := nodesCoveringChunks(nodes, nil); got != nil {
		t.Errorf("nil chunk list -> nil, got %v", got)
	}
}

func renderDrillNodes() ([]structureNode, []structureRel) {
	nodes := []structureNode{
		{name: "Root", nodeType: "tree_node", desc: "root entity", sourceChunkIDs: []string{"r1"}},
		{name: "Leaf", nodeType: "tree_node", desc: "leaf entity", sourceChunkIDs: []string{"l1"}},
	}
	rels := []structureRel{{from: "Root", to: "Leaf", relType: "contains"}}
	return nodes, rels
}

func drillLoader(ids []string) []chunkWithText {
	var out []chunkWithText
	for _, id := range ids {
		out = append(out, chunkWithText{id: id, text: "chunk content for " + id})
	}
	return out
}

func TestRenderTocDrilloutLLMSelect(t *testing.T) {
	nodes, rels := renderDrillNodes()
	out := renderTocDrilldown("some query", nil, nodes, rels, drillLoader, nil, []string{"Leaf"})
	if out.selector != "llm_toc" {
		t.Errorf("selector = %q, want llm_toc", out.selector)
	}
	if out.outline == "" {
		t.Fatal("outline should be non-empty for a selected leaf")
	}
	// The selected leaf's parent (Root) must be pulled in as an ancestor.
	if !strings.Contains(out.outline, "Root") {
		t.Errorf("ancestor Root missing from outline:\n%s", out.outline)
	}
}

func TestRenderTocDrilloutChunkRetrieval(t *testing.T) {
	nodes, rels := renderDrillNodes()
	hits := []chunkHit{{id: "r1", score: 0.9}, {id: "l1", score: 0.7}}
	out := renderTocDrilldown("q", nil, nodes, rels, drillLoader, hits, nil)
	if out.selector != "chunk_retrieval" {
		t.Errorf("selector = %q, want chunk_retrieval", out.selector)
	}
	if out.topScore != 0.9 {
		t.Errorf("topScore = %v, want 0.9", out.topScore)
	}
	// Retrieved chunks are surfaced even when they are not a selected node's anchor.
	if !strings.Contains(out.outline, "r1") || !strings.Contains(out.outline, "l1") {
		t.Errorf("retrieved chunks missing from outline:\n%s", out.outline)
	}
}

func TestRenderTocDrilloutBeamFallback(t *testing.T) {
	nodes, rels := renderDrillNodes()
	// No selection and no hits, but a query: beam picks by keyword relevance. The
	// blank query yields no terms, so it must still return a (flat) outline, not panic.
	out := renderTocDrilldown("leaf", nil, nodes, rels, drillLoader, nil, nil)
	if out.outline == "" {
		t.Fatal("beam path produced an empty outline")
	}
	if out.selector != "beam" {
		t.Errorf("selector = %q, want beam", out.selector)
	}
}

func TestParseCompiledStructureCarriesVec(t *testing.T) {
	rows := []StructureRow{{
		CompileKwd:        "tree",
		KnowledgeGraphKwd: "graph",
		Content:           `{"entities": [{"name": "A"}, {"name": "B"}], "relations": []}`,
		Vec:               []float64{1, 2},
	}}
	ents, _ := ParseCompiledStructure(rows, []string{"tree"})
	if len(ents) != 2 {
		t.Fatalf("entities = %d, want 2", len(ents))
	}
	// Both nested entities of the RAPTOR blob inherit its single vector.
	for _, e := range ents {
		v, _ := e["_vec"].([]float64)
		if len(v) != 2 || v[0] != 1 {
			t.Errorf("entity %v did not inherit the blob vector", e["name"])
		}
	}
}

func TestNavigationToolsReportEmptyWhenUncompiled(t *testing.T) {
	// The Go port has no compiled navigation structures, so navigate_tree /
	// navigate_structure report EMPTY/no_structure — the DATASET-level signal
	// that lets the ladder fall through to `global` and (after the strikes)
	// disable the tool, instead of looping on a rung that cannot succeed.
	kb := &Kbinfos{}
	ex := &searchExecutor{deps: SearchDeps{KB: kb}}
	// No database is wired in this unit test, so defaultSeedEncoder's nil-DB
	// guard returns nil and structure navigation falls back to keyword matching
	// instead of panicking.

	oc, _ := ex.Execute(context.Background(), "navigate_tree", map[string]any{"query": "topic"})
	if oc.Status != StatusEmpty || oc.Reason != ReasonNoStructure {
		t.Errorf("navigate_tree: got (%s,%s), want (empty,no_structure)", oc.Status, oc.Reason)
	}
	// A note must accompany it, so the model knows to switch tools.
	if len(oc.Payload) == 0 {
		t.Error("navigate_tree must return an actionable note, not an empty payload")
	}

	oc, _ = ex.Execute(context.Background(), "navigate_structure", map[string]any{"doc_id": "d1", "query": "topic"})
	if oc.Status != StatusEmpty || oc.Reason != ReasonNoStructure {
		t.Errorf("navigate_structure: got (%s,%s), want (empty,no_structure)", oc.Status, oc.Reason)
	}
}

func TestNavigateStructureDocRejectsForeignDocument(t *testing.T) {
	// Python _load_compiled_structure: a doc_id outside the bound datasets
	// yields no structure, so nothing from the other dataset is ever read.
	res, _ := navigateStructures(context.Background(), "t", "q", []string{"foreign-doc"}, "catalog", nil, SearchDeps{
		KbIDs:         []string{"kb"},
		DocIDVerifier: stubVerifier{known: map[string]bool{"mine": true}},
	})

	if res.EmptyReason != ReasonNoStructure {
		t.Fatalf("EmptyReason = %q, want %q", res.EmptyReason, ReasonNoStructure)
	}
	if len(res.DocIDs) != 0 {
		t.Fatalf("DocIDs = %v, want none", res.DocIDs)
	}
}

// TestIndexNameForFallsBackToRagflowPrefix pins the index-name seam fix: Python
// reads search.index_name (navigation.py:_navigate_structure_impl); the RAGFlow default is
// "ragflow_<tenant_id>", so an empty configured name falls back to that, while a
// tenant override is honoured verbatim.

// TestIndexNameForFallsBackToRagflowPrefix pins the index-name seam fix: Python
// reads search.index_name (navigation.py:_navigate_structure_impl); the RAGFlow default is
// "ragflow_<tenant_id>", so an empty configured name falls back to that, while a
// tenant override is honoured verbatim.
func TestIndexNameForFallsBackToRagflowPrefix(t *testing.T) {
	if got := indexNameFor("tenantA", ""); got != "ragflow_tenantA" {
		t.Errorf("empty configured = %q, want ragflow_tenantA", got)
	}
	if got := indexNameFor("tenantA", "custom_idx"); got != "custom_idx" {
		t.Errorf("configured = %q, want custom_idx", got)
	}
}

// TestNavigateStructuresEmptyDocIDsRoutesViaRouter pins the empty-doc_id vector
// routing fix: mirroring Python _navigate_structure_impl(:1012), a direct call
// with no doc_ids routes through the NavTreeRouter first (search_dataset_layers)
// instead of immediately returning MISS. The tool path still passes non-empty
// doc_ids and must NOT consult the router.

// TestNavigateStructuresEmptyDocIDsRoutesViaRouter pins the empty-doc_id vector
// routing fix: mirroring Python _navigate_structure_impl(:1012), a direct call
// with no doc_ids routes through the NavTreeRouter first (search_dataset_layers)
// instead of immediately returning MISS. The tool path still passes non-empty
// doc_ids and must NOT consult the router.
func TestNavigateStructuresEmptyDocIDsRoutesViaRouter(t *testing.T) {
	router := &stubNavRouter{docs: [][2]string{{"d1", "summary"}}}
	// Empty doc_ids + router => router consulted (it then returns ReasonNoDoc
	// because no engine-backed structure surfaces in the test env, identical to
	// the Python post-route empty result).
	if _, _ = navigateStructures(context.Background(), "t", "q", []string{}, "catalog", router, SearchDeps{KbIDs: []string{"kb"}}); !router.called {
		t.Fatal("empty docIDs should route via NavTreeRouter, but router was not called")
	}

	// Non-empty doc_ids => router must NOT be consulted (no double routing).
	router2 := &stubNavRouter{docs: [][2]string{{"d1", "summary"}}}
	navigateStructures(context.Background(), "t", "q", []string{"foreign-doc"}, "catalog", router2, SearchDeps{
		KbIDs:         []string{"kb"},
		DocIDVerifier: stubVerifier{known: map[string]bool{"mine": true}},
	})
	if router2.called {
		t.Error("non-empty docIDs should not consult the router")
	}
}

// TestNavigateTreeRoutesWithinSessionDocScope pins that the nav-tree route runs
// inside the session document scope. Python's _exec_navigate_tree does not thread
// args["doc_scope"], but _navigate_tree_impl routes through _nav_search_titled,
// which ceilings with tools.scoped_doc_ids(None) — i.e. the session doc_scope
// (navigation.py:_nav_search_titled). Dropping it routed over the whole dataset.
func TestNavigateTreeRoutesWithinSessionDocScope(t *testing.T) {
	router := &stubNavRouter{docs: [][2]string{{"d1", "summary"}}}
	NavigateTree(context.Background(), router, NavTreeInput{
		Query:    "topic",
		TenantID: "t1",
		KbIDs:    []string{"kb1"},
		DocScope: []string{"d1", "d2"},
	})
	if len(router.scope) != 2 || router.scope[0] != "d1" || router.scope[1] != "d2" {
		t.Fatalf("router doc scope = %v, want [d1 d2]", router.scope)
	}
}

// TestNavigateToolsRouteWithinSessionDocScope pins the executor end of the same
// contract: both navigation tools hand the router the SESSION scope
// (deps.DocScope), never the tool argument.
