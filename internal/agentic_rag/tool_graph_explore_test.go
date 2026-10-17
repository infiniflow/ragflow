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
	"errors"
	"strings"
	"testing"

	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

// fakeKgService records the request and returns a canned walk result.
type fakeKgService struct {
	gotReq KgExploreRequest
	result KgExploreResult
	err    error
}

func (f *fakeKgService) Explore(_ context.Context, req KgExploreRequest) (KgExploreResult, error) {
	f.gotReq = req
	return f.result, f.err
}

// DocOutline completes KgService. This fake only ever backs the explore tool's
// tests, so it carries the fake's canned error (nil by default) and returns no
// skeleton.
func (f *fakeKgService) DocOutline(context.Context, KgDocOutlineRequest) (KgExploreResult, error) {
	return KgExploreResult{}, f.err
}

func installFakeKgService(t *testing.T, svc KgService) {
	t.Helper()
	previous := GetKgService()
	SetKgService(svc)
	t.Cleanup(func() { SetKgService(previous) })
}

func kgExploreContext(t *testing.T) context.Context {
	t.Helper()
	return WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
}

// TestGraphExploreToolRendersSubgraphAndPassages pins the payload contract: the
// subgraph (entities + relations) plus source passages, with each passage
// carrying the [ID:n] handle the answer cites.
func TestGraphExploreToolRendersSubgraphAndPassages(t *testing.T) {
	fake := &fakeKgService{result: KgExploreResult{
		Entities: []KgEntity{
			{Name: "Marie Curie", Type: "person", Description: "physicist"},
			{Name: "Radium Institute", Type: "org"},
		},
		Relations: []KgRelation{{From: "Marie Curie", To: "Radium Institute", Type: "part_of"}},
		Passages: []KgPassage{
			{ChunkID: "c1", DocID: "d1", DocName: "Curie.md", DatasetID: "kb1", Content: "Curie founded the institute."},
		},
	}}
	installFakeKgService(t, fake)

	tool := NewGraphExploreTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(kgExploreContext(t), `{"query":"what connects Curie to the Radium Institute"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	for _, want := range []string{
		`<entity name="Marie Curie" type="person"`,
		`<relation from="Marie Curie" to="Radium Institute" type="part_of"/>`,
		`<passage chunk_id="c1" doc_id="d1" doc_name="Curie.md" dataset_id="kb1" ref="0">`,
		`<content>Curie founded the institute.</content>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output must contain %q, got:\n%s", want, out)
		}
	}
	if fake.gotReq.TenantID != "tenant-1" || len(fake.gotReq.DatasetIDs) != 1 || fake.gotReq.DatasetIDs[0] != "kb1" {
		t.Errorf("walk scope = %+v, want tenant-1 / [kb1]", fake.gotReq)
	}
	if fake.gotReq.Query != "what connects Curie to the Radium Institute" {
		t.Errorf("walk query = %q", fake.gotReq.Query)
	}
}

// TestGraphExploreToolWithoutBindingsDoesNotCallTheWalk: an empty bound scope
// must short-circuit - the tool never reads outside the conversation's scope.
func TestGraphExploreToolWithoutBindingsDoesNotCallTheWalk(t *testing.T) {
	fake := &fakeKgService{}
	installFakeKgService(t, fake)

	tool := NewGraphExploreTool("tenant-1", nil)
	out, err := tool.InvokableRun(kgExploreContext(t), `{"query":"anything"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "<note") {
		t.Errorf("empty scope must render the unavailable note, got:\n%s", out)
	}
	if fake.gotReq.Query != "" {
		t.Errorf("the walk must not run with no bound dataset, got %+v", fake.gotReq)
	}
}

// TestGraphExploreToolRejectsOutOfScopeDataset: a model-supplied dataset id must
// stay inside the conversation's bound scope. The failure becomes a
// model-readable <tool_error>, not a Go error that would abort the ReAct loop.
func TestGraphExploreToolRejectsOutOfScopeDataset(t *testing.T) {
	installFakeKgService(t, &fakeKgService{})

	tool := NewGraphExploreTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(kgExploreContext(t), `{"query":"q","dataset_ids":["kb2"]}`)
	if err != nil {
		t.Fatalf("out-of-scope dataset must not abort the loop: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("expected a <tool_error> result, got:\n%s", out)
	}
}

// TestGraphExploreToolNoCompiledGraphDisables: a DATASET-level absence
// (GraphAbsent) is a dead end the model must read, and it disables the tool for
// the conversation so later rounds stop spending calls on it.
func TestGraphExploreToolNoCompiledGraphDisables(t *testing.T) {
	installFakeKgService(t, &fakeKgService{result: KgExploreResult{GraphAbsent: true}})

	tool := NewGraphExploreTool("tenant-1", []string{"kb1"})
	ctx := WithConversationKey(kgExploreContext(t), "test-graph-absent")
	out, err := tool.InvokableRun(ctx, `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if strings.Contains(out, "<subgraph>") {
		t.Errorf("an empty walk must not render a subgraph, got:\n%s", out)
	}
	if !strings.Contains(out, `error="no compiled knowledge graph"`) {
		t.Errorf("dataset-level absence must be marked in the note, got:\n%s", out)
	}
	if navToolEnabled(ctx, graphExploreToolName) {
		t.Error("a proven dataset-level absence must disable graph_explore for the conversation")
	}
}

// TestGraphExploreToolQueryMissDoesNotDisable: an empty walk WITHOUT the
// dataset-level marker is a query-level miss - a better-phrased query may still
// hit, so it must not disable the tool and must tell the model to rephrase.
func TestGraphExploreToolQueryMissDoesNotDisable(t *testing.T) {
	installFakeKgService(t, &fakeKgService{result: KgExploreResult{}})

	tool := NewGraphExploreTool("tenant-1", []string{"kb1"})
	ctx := WithConversationKey(kgExploreContext(t), "test-graph-query-miss")
	out, err := tool.InvokableRun(ctx, `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if strings.Contains(out, `error="no compiled knowledge graph"`) {
		t.Errorf("a query miss must not claim the dataset has no graph, got:\n%s", out)
	}
	if !strings.Contains(out, "matched THIS query") {
		t.Errorf("a query miss must tell the model to rephrase, got:\n%s", out)
	}
	if !navToolEnabled(ctx, graphExploreToolName) {
		t.Error("a query-level miss must NOT disable graph_explore")
	}
}

// TestGraphExploreToolRequiresQuery pins the argument contract.
func TestGraphExploreToolRequiresQuery(t *testing.T) {
	installFakeKgService(t, &fakeKgService{})

	tool := NewGraphExploreTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(kgExploreContext(t), `{"query":"   "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("missing query must produce a <tool_error>, got:\n%s", out)
	}
}

// TestGraphExploreToolPropagatesWalkFailure: an infrastructure failure surfaces
// as a tool error (the loop continues) rather than a silent empty graph.
func TestGraphExploreToolPropagatesWalkFailure(t *testing.T) {
	installFakeKgService(t, &fakeKgService{err: errors.New("engine down")})

	tool := NewGraphExploreTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(kgExploreContext(t), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) || !strings.Contains(out, "engine down") {
		t.Errorf("walk failure must surface as a tool error, got:\n%s", out)
	}
}

// TestGraphExploreToolIsRegisteredAndBuildableByTemplate pins the wiring a
// template needs: the name must be in the registry, and toolsFor must turn it
// into the graph tool with the session scope injected.
func TestGraphExploreToolIsRegisteredAndBuildableByTemplate(t *testing.T) {
	if _, ok := toolRegistry()[graphExploreToolName]; !ok {
		t.Fatalf("%s must be registered so a template can select it", graphExploreToolName)
	}
	tools := toolsFor(context.Background(), Template{ID: "kg", Tools: []string{graphExploreToolName}}, "tenant-1", []string{"kb1"}, nil)
	if len(tools) != 1 {
		t.Fatalf("toolsFor returned %d tools, want 1", len(tools))
	}
	tool, ok := tools[0].(*GraphExploreTool)
	if !ok {
		t.Fatalf("toolsFor built %T, want *GraphExploreTool", tools[0])
	}
	info, err := tool.Info(context.Background())
	if err != nil || info.Name != graphExploreToolName {
		t.Fatalf("tool info = %+v err=%v", info, err)
	}
	if tool.tenantID != "tenant-1" || len(tool.datasetIDs) != 1 || tool.datasetIDs[0] != "kb1" {
		t.Errorf("scope = %q/%v, want tenant-1/[kb1]", tool.tenantID, tool.datasetIDs)
	}
}

func TestKgParseEntityReadsPayloadAndRowFields(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"name":"吕布","type":"person","description":"武将","aliases":["吕奉先"]}`,
		"source_chunk_ids":    []interface{}{"c1", "c2"},
		"doc_id":              "d1",
	}
	e, ok := kgParseEntity(row)
	if !ok {
		t.Fatal("entity row must parse")
	}
	if e.Name != "吕布" || e.Type != "person" || e.Description != "武将" || e.DocID != "d1" {
		t.Errorf("entity = %+v", e)
	}
	if len(e.Aliases) != 1 || e.Aliases[0] != "吕奉先" {
		t.Errorf("aliases = %v", e.Aliases)
	}
	if len(e.SourceChunkIDs) != 2 {
		t.Errorf("source chunks = %v", e.SourceChunkIDs)
	}
	if _, ok := kgParseEntity(map[string]interface{}{"content_with_weight": "{}"}); ok {
		t.Error("a nameless row must be dropped")
	}
}

func TestKgParseRelationRequiresBothEndpoints(t *testing.T) {
	row := map[string]interface{}{
		"id":                  "r1",
		"from_entity_kwd":     "A",
		"to_entity_kwd":       "B",
		"content_with_weight": `{"type":"owns"}`,
		"source_chunk_ids":    []string{"c9"},
		"doc_id":              "d2",
	}
	r, ok := kgParseRelation(row)
	if !ok || r.From != "A" || r.To != "B" || r.Type != "owns" || r.ID != "r1" || r.DocID != "d2" {
		t.Fatalf("relation = %+v ok=%v", r, ok)
	}
	if _, ok := kgParseRelation(map[string]interface{}{"from_entity_kwd": "A"}); ok {
		t.Error("a half-open edge must be dropped")
	}
}

func TestKgEndpointTermsKeepsOriginalAndLowercase(t *testing.T) {
	got := kgEndpointTerms([]string{" Marie Curie ", "marie curie", ""})
	want := map[string]bool{"Marie Curie": true, "marie curie": true}
	if len(got) != len(want) {
		t.Fatalf("terms = %v, want one original + one lowercase form", got)
	}
	for _, term := range got {
		if !want[term] {
			t.Errorf("unexpected term %q", term)
		}
	}
}

func TestKgTopMentionCountRanksAndCaps(t *testing.T) {
	rows := []map[string]interface{}{
		{"name_kwd": "low", "mention_count_int": 1},
		{"name_kwd": "high", "mention_count_int": 9},
		{"name_kwd": "mid", "mention_count_int": 4},
	}
	got := kgTopMentionCount(rows, 2)
	if len(got) != 2 || got[0]["name_kwd"] != "high" || got[1]["name_kwd"] != "mid" {
		t.Fatalf("ranked rows = %v, want high, mid", got)
	}
}

// kgProbeFakeEngine answers the existence probe (a single "id" select field with
// no match expression) from entityRows, and returns nothing for the walk's seed
// / hop searches - which is exactly the "graph exists, this query seeded
// nothing" shape the absence detection must not mistake for an empty dataset.
type kgProbeFakeEngine struct {
	engine.DocEngine
	entityRows  []map[string]interface{}
	probeErr    error
	probeCalled bool
}

func (e *kgProbeFakeEngine) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	if len(req.SelectFields) == 1 && req.SelectFields[0] == "id" {
		e.probeCalled = true
		if e.probeErr != nil {
			return nil, e.probeErr
		}
		return &enginetypes.SearchResult{Chunks: e.entityRows}, nil
	}
	return &enginetypes.SearchResult{}, nil
}

// TestKgAdapterExploreMarksAbsenceOnlyWithoutEntityRows pins the disable trigger:
// an empty walk over a scope that holds NO entity row is a dataset-level absence
// (GraphAbsent), while an empty walk over a scope that DOES hold one is a
// query-level miss and must stay non-absent.
func TestKgAdapterExploreMarksAbsenceOnlyWithoutEntityRows(t *testing.T) {
	req := KgExploreRequest{TenantID: "t1", DatasetIDs: []string{"kb1"}, Query: "who links A to B"}

	absentEng := &kgProbeFakeEngine{}
	got, err := NewKgAdapter(absentEng).Explore(context.Background(), req)
	if err != nil {
		t.Fatalf("Explore: %v", err)
	}
	if !absentEng.probeCalled {
		t.Error("an empty walk must probe for entity rows before deciding absence")
	}
	if !got.GraphAbsent {
		t.Error("no entity rows in scope must set GraphAbsent")
	}

	presentEng := &kgProbeFakeEngine{entityRows: []map[string]interface{}{{"id": "e1"}}}
	got, err = NewKgAdapter(presentEng).Explore(context.Background(), req)
	if err != nil {
		t.Fatalf("Explore: %v", err)
	}
	if got.GraphAbsent {
		t.Error("entity rows exist: an empty walk is a query miss, not a dataset absence")
	}
}

// TestKgAdapterExploreDocScopedEmptyIsNotAbsence: a walk the model scoped to
// specific documents proves nothing about the dataset as a whole, so an empty
// result must not be reported as a dataset-level absence - that would disable
// graph_explore for the entire conversation on one scoped miss.
func TestKgAdapterExploreDocScopedEmptyIsNotAbsence(t *testing.T) {
	eng := &kgProbeFakeEngine{}
	got, err := NewKgAdapter(eng).Explore(context.Background(), KgExploreRequest{
		TenantID: "t1", DatasetIDs: []string{"kb1"}, DocScope: []string{"d1"}, Query: "q",
	})
	if err != nil {
		t.Fatalf("Explore: %v", err)
	}
	if got.GraphAbsent {
		t.Error("a doc-scoped empty walk must not report a dataset-level absence")
	}
	if eng.probeCalled {
		t.Error("a doc-scoped walk must not probe: its scope cannot answer a dataset-level question")
	}
}

// TestKgAdapterExploreProbeFailureIsAnError: a failed existence probe must be
// surfaced, not swallowed into a false "no graph" verdict that would disable the
// tool for the whole conversation.
func TestKgAdapterExploreProbeFailureIsAnError(t *testing.T) {
	eng := &kgProbeFakeEngine{probeErr: errors.New("engine down")}
	_, err := NewKgAdapter(eng).Explore(context.Background(), KgExploreRequest{
		TenantID: "t1", DatasetIDs: []string{"kb1"}, Query: "q",
	})
	if err == nil || !strings.Contains(err.Error(), "engine down") {
		t.Fatalf("probe failure must surface as an error, got %v", err)
	}
}

// kgOutlineFakeEngine serves the per-document panorama read and the
// entity/relation fallback from separate row sets, so a test can hand a
// document only one of them.
type kgOutlineFakeEngine struct {
	engine.DocEngine
	graphRows    []map[string]interface{}
	entityRows   []map[string]interface{}
	relationRows []map[string]interface{}
	searchErr    error
	reads        []string
}

func (e *kgOutlineFakeEngine) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	kind, _ := req.Filter["knowledge_graph_kwd"].(string)
	e.reads = append(e.reads, kind)
	if e.searchErr != nil {
		return nil, e.searchErr
	}
	var rows []map[string]interface{}
	switch kind {
	case "graph":
		rows = e.graphRows
	case "entity":
		rows = e.entityRows
	case "relation":
		rows = e.relationRows
	}
	// A real engine applies the doc_id term filter, and the outline relies on it:
	// the fallback read is scoped to the requested documents, while the dataset
	// probe is unscoped. Without this, a fixture row for another document would
	// answer a document-scoped read.
	if docs, ok := req.Filter["doc_id"].([]string); ok && len(docs) > 0 {
		keep := make(map[string]bool, len(docs))
		for _, d := range docs {
			keep[d] = true
		}
		scoped := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			if id, _ := row["doc_id"].(string); keep[id] {
				scoped = append(scoped, row)
			}
		}
		rows = scoped
	}
	return &enginetypes.SearchResult{Chunks: rows}, nil
}

// TestKgAdapterDocOutlineFallsBackToPerDocumentRows pins the gap fix: a document
// the compiler wrote no `graph` panorama for still carries its own entity and
// relation rows, so the outline is built from those instead of coming back
// empty - which the agent reads as "this document has no compiled knowledge".
func TestKgAdapterDocOutlineFallsBackToPerDocumentRows(t *testing.T) {
	eng := &kgOutlineFakeEngine{
		entityRows: []map[string]interface{}{{
			"doc_id": "d1", "mention_count_int": 7,
			"content_with_weight": `{"name":"Marie Curie","type":"person"}`,
		}},
		relationRows: []map[string]interface{}{{
			"doc_id": "d1", "from_entity_kwd": "marie curie", "to_entity_kwd": "radium",
			"content_with_weight": `{"type":"discovered"}`,
		}},
	}
	got, err := NewKgAdapter(eng).DocOutline(context.Background(), KgDocOutlineRequest{
		TenantID: "t1", DatasetIDs: []string{"kb1"}, DocIDs: []string{"d1"},
	})
	if err != nil {
		t.Fatalf("DocOutline: %v", err)
	}
	if got.GraphAbsent {
		t.Error("entity rows exist for the document: a dataset-level absence must not be reported")
	}
	if len(got.Entities) != 1 || got.Entities[0].Name != "Marie Curie" || got.Entities[0].MentionCount != 7 {
		t.Errorf("entities = %+v, want the fallback entity row", got.Entities)
	}
	if len(got.Relations) != 1 || got.Relations[0].Type != "discovered" {
		t.Errorf("relations = %+v, want the fallback relation row", got.Relations)
	}
	if len(eng.reads) == 0 || eng.reads[0] != "graph" {
		t.Errorf("reads = %v, want the panorama read first", eng.reads)
	}
}

// TestKgAdapterDocOutlinePrefersThePanoramaRows: when the document HAS a
// panorama row the fallback must not run, so a document is never read twice.
func TestKgAdapterDocOutlinePrefersThePanoramaRows(t *testing.T) {
	eng := &kgOutlineFakeEngine{
		graphRows: []map[string]interface{}{{
			"doc_id": "d1",
			"content_with_weight": `{"entities":[{"name":"Hub","type":"org"}],` +
				`"relations":[{"from":"Hub","to":"Leaf","type":"owns"}]}`,
		}},
		entityRows: []map[string]interface{}{{
			"doc_id": "d1", "content_with_weight": `{"name":"Fallback","type":"other"}`,
		}},
	}
	got, err := NewKgAdapter(eng).DocOutline(context.Background(), KgDocOutlineRequest{
		TenantID: "t1", DatasetIDs: []string{"kb1"}, DocIDs: []string{"d1"},
	})
	if err != nil {
		t.Fatalf("DocOutline: %v", err)
	}
	if len(got.Entities) != 1 || got.Entities[0].Name != "Hub" {
		t.Errorf("entities = %+v, want only the panorama entity", got.Entities)
	}
	for _, kind := range eng.reads {
		if kind == "entity" || kind == "relation" {
			t.Errorf("reads = %v: the fallback ran for a document that has a panorama", eng.reads)
			break
		}
	}
}

// TestKgAdapterDocOutlineScopedMissIsNotDatasetAbsence pins the contract the
// tool's self-disable rides on: GraphAbsent is a dataset-level fact, so a
// document that yielded no row - an unprocessed document, or a BM25 hit whose
// document was never compiled - must come back as a plain empty result. Were it
// reported as an absence, one such document would turn graph_doc_outline off for
// the rest of the conversation (markNavToolUnavailable), and the renderer's
// "these documents carry no compiled graph row" note could never be reached.
func TestKgAdapterDocOutlineScopedMissIsNotDatasetAbsence(t *testing.T) {
	eng := &kgOutlineFakeEngine{
		entityRows: []map[string]interface{}{{
			"doc_id": "other-doc", "content_with_weight": `{"name":"Someone else","type":"person"}`,
		}},
	}
	got, err := NewKgAdapter(eng).DocOutline(context.Background(), KgDocOutlineRequest{
		TenantID: "t1", DatasetIDs: []string{"kb1"}, DocIDs: []string{"d-never-compiled"},
	})
	if err != nil {
		t.Fatalf("DocOutline: %v", err)
	}
	if got.GraphAbsent {
		t.Error("a document-scoped miss must not report a dataset-level absence: the tool would disable itself")
	}
	if len(got.Entities) != 0 || len(got.Relations) != 0 {
		t.Errorf("entities/relations = %d/%d, want the requested document's empty payload", len(got.Entities), len(got.Relations))
	}
}

// TestKgAdapterDocOutlineAbsentOnlyWhenNothingIsCompiled is the other half: the
// absence IS reported when every scoped read came back empty AND the dataset
// holds no compiled row at all, which is the only case that may disable the tool.
func TestKgAdapterDocOutlineAbsentOnlyWhenNothingIsCompiled(t *testing.T) {
	got, err := NewKgAdapter(&kgOutlineFakeEngine{}).DocOutline(context.Background(), KgDocOutlineRequest{
		TenantID: "t1", DatasetIDs: []string{"kb1"}, DocIDs: []string{"d1"},
	})
	if err != nil {
		t.Fatalf("DocOutline: %v", err)
	}
	if !got.GraphAbsent {
		t.Error("an uncompiled dataset must report the dataset-level absence")
	}
}

// TestKgAdapterDocOutlineProbeFailureIsNotAbsence: a read failure looks like "no
// rows" to this code path, so it must be settled by the probe, not assumed. A
// failing probe has to surface as an error - a transient engine failure may
// never masquerade as "this dataset has no compiled knowledge", because that
// answer disables the tool for the conversation.
func TestKgAdapterDocOutlineProbeFailureIsNotAbsence(t *testing.T) {
	eng := &kgOutlineFakeEngine{searchErr: errors.New("engine down")}
	got, err := NewKgAdapter(eng).DocOutline(context.Background(), KgDocOutlineRequest{
		TenantID: "t1", DatasetIDs: []string{"kb1"}, DocIDs: []string{"d1"},
	})
	if err == nil {
		t.Fatalf("probe failure must surface as an error, got GraphAbsent=%v", got.GraphAbsent)
	}
	if !strings.Contains(err.Error(), "engine down") {
		t.Errorf("error = %v, want the probe failure", err)
	}
}
