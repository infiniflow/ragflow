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
	if !strings.Contains(out, "<note>") {
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

// TestGraphExploreToolEmptyGraphIsAHintNotAnError: a dataset without a compiled
// graph is a dataset-level dead end the model must read and move on from.
func TestGraphExploreToolEmptyGraphIsAHintNotAnError(t *testing.T) {
	installFakeKgService(t, &fakeKgService{})

	tool := NewGraphExploreTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(kgExploreContext(t), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if strings.Contains(out, "<subgraph>") {
		t.Errorf("an empty walk must not render a subgraph, got:\n%s", out)
	}
	if !strings.Contains(out, "NO compiled knowledge graph") {
		t.Errorf("empty walk must explain the graph is unavailable, got:\n%s", out)
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
	tools := toolsFor(Template{ID: "kg", Tools: []string{graphExploreToolName}}, "tenant-1", []string{"kb1"})
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
