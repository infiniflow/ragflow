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
	"encoding/json"
	"strings"
	"testing"
)

// fakeStructureReader returns canned compiled-structure rows per document.
type fakeStructureReader struct {
	byDoc map[string][]StructureRow
}

func (f fakeStructureReader) ReadStructure(_ context.Context, _ string, _ string, docID string) ([]StructureRow, error) {
	return f.byDoc[docID], nil
}

func installFakeStructureReader(t *testing.T, r structureReader) {
	t.Helper()
	prev := getStructureReader()
	SetStructureReader(r)
	t.Cleanup(func() { SetStructureReader(prev) })
}

// TestNavigateStructureToolIsRegisteredAndBuildableByTemplate pins the wiring:
// the name must be in the registry and toolsFor must build it with scope injected.
func TestNavigateStructureToolIsRegisteredAndBuildableByTemplate(t *testing.T) {
	if _, ok := toolRegistry()[navigateStructureToolName]; !ok {
		t.Fatalf("%s must be registered so a template can select it", navigateStructureToolName)
	}
	tools := toolsFor(Template{ID: "nav", Tools: []string{navigateStructureToolName}}, "tenant-1", []string{"kb1"})
	if len(tools) != 1 {
		t.Fatalf("toolsFor returned %d tools, want 1", len(tools))
	}
	tool, ok := tools[0].(*NavigateStructureTool)
	if !ok {
		t.Fatalf("toolsFor built %T, want *NavigateStructureTool", tools[0])
	}
	info, err := tool.Info(context.Background())
	if err != nil || info.Name != navigateStructureToolName {
		t.Fatalf("tool info = %+v err=%v", info, err)
	}
	if tool.tenantID != "tenant-1" || len(tool.datasetIDs) != 1 || tool.datasetIDs[0] != "kb1" {
		t.Errorf("scope = %q/%v, want tenant-1/[kb1]", tool.tenantID, tool.datasetIDs)
	}
}

// graphRow is a canned compiled-structure "graph" blob for the drill tests.
func graphRow() StructureRow {
	content, _ := json.Marshal(map[string]any{
		"entities": []map[string]any{
			{"name": "Alpha", "type": "section", "description": "first topic", "source_chunk_ids": []string{"c1", "c2"}},
			{"name": "Beta", "type": "subsection", "description": "second topic", "source_chunk_ids": []string{"c3"}},
		},
		"relations": []map[string]any{
			{"from": "Alpha", "to": "Beta", "type": "contains"},
		},
	})
	return StructureRow{KnowledgeGraphKwd: "graph", Content: string(content)}
}

// TestNavigateStructureToolDrillsStructure pins the payload: a routed document's
// compiled structure renders as a query-focused outline with chunk pointers.
func TestNavigateStructureToolDrillsStructure(t *testing.T) {
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]StructureRow{
		"d1": {graphRow()},
	}})

	tool := NewNavigateStructureTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"first topic","doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	for _, want := range []string{
		`<structure_navigation count="1" query="first topic" kind="">`,
		`<doc rank="1" doc_id="d1"`,
		`<structure>`,
		"Alpha (section)",
		"[chunks: c1,c2]",
		`</structure>`,
		`</structure_navigation>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output must contain %q, got:\n%s", want, out)
		}
	}
}

// TestNavigateStructureToolFlatFallback renders the whole outline when the query
// carries no usable terms (the documented no-keyword fallback).
func TestNavigateStructureToolFlatFallback(t *testing.T) {
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]StructureRow{
		"d1": {graphRow()},
	}})

	tool := NewNavigateStructureTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"...","doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "Alpha (section)") || !strings.Contains(out, "Beta (subsection)") {
		t.Errorf("flat fallback must list every entity, got:\n%s", out)
	}
}

// TestNavigateStructureToolNoDocIsAHintNotAnError: a call without a doc_id is a
// usage-level dead end the model reads and routes first with navigate_tree.
func TestNavigateStructureToolNoDocIsAHintNotAnError(t *testing.T) {
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]StructureRow{}})

	tool := NewNavigateStructureTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"q"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `count="0"`) {
		t.Fatalf("missing doc_id must report count=0, got:\n%s", out)
	}
	if !strings.Contains(out, `error="route first with navigate_tree"`) {
		t.Errorf("missing doc_id must explain the next step, got:\n%s", out)
	}
}

// TestNavigateStructureToolNoStructureIsAHintNotAnError: a document without a
// compiled structure of the requested kind is a dead end, not an abort.
func TestNavigateStructureToolNoStructureIsAHintNotAnError(t *testing.T) {
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]StructureRow{
		"d1": nil, // no structure rows
	}})

	tool := NewNavigateStructureTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"q","doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `count="0"`) {
		t.Fatalf("no structure must report count=0, got:\n%s", out)
	}
	if !strings.Contains(out, `error="no compiled structure for this kind"`) {
		t.Errorf("no structure must explain the absence, got:\n%s", out)
	}
}

// TestNavigateStructureToolRequiresQuery pins the argument contract.
func TestNavigateStructureToolRequiresQuery(t *testing.T) {
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]StructureRow{}})
	tool := NewNavigateStructureTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"   "}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("missing query must produce a <tool_error>, got:\n%s", out)
	}
}

// TestNavigateStructureToolRespectsDatasetScope: an out-of-scope dataset_id is
// rejected before any read.
func TestNavigateStructureToolRespectsDatasetScope(t *testing.T) {
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]StructureRow{}})
	tool := NewNavigateStructureTool("tenant-1", []string{"kb1"})
	out, err := tool.InvokableRun(context.Background(), `{"query":"q","doc_id":"d1","dataset_ids":["rogue"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("out-of-scope dataset_id must produce a <tool_error>, got:\n%s", out)
	}
}

// --- Pure-function tests (no engine, no reader) ---

func TestQueryTermsTokenizes(t *testing.T) {
	got := queryTerms("The 2023 Revenue, by Segment!")
	want := []string{"the", "2023", "revenue", "by", "segment"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("queryTerms = %v, want %v", got, want)
	}
	if len(queryTerms("a  x")) != 0 { // sub-2-char tokens dropped
		t.Errorf("sub-2-char tokens must be dropped")
	}
}

func TestExtractJSONObjectToleratesProse(t *testing.T) {
	v := ExtractJSON("here is the blob: {\"a\": 1, \"b\": [2,3]} trailing text")
	m, ok := v.(map[string]any)
	if !ok || m["a"] != float64(1) {
		t.Errorf("ExtractJSON = %#v ok=%v", v, ok)
	}
	if ExtractJSON("no json here") != nil {
		t.Errorf("ExtractJSON must return nil when no object is present")
	}
}

func TestParseCompiledStructureMergesGraphBlob(t *testing.T) {
	row := graphRow()
	entities, relations := ParseCompiledStructure([]StructureRow{row}, nil)
	if len(entities) != 2 || len(relations) != 1 {
		t.Fatalf("parsed entities=%d relations=%d, want 2/1", len(entities), len(relations))
	}
	if entities[0]["name"] != "Alpha" || relations[0]["from"] != "Alpha" {
		t.Errorf("unexpected parse: %v / %v", entities[0], relations[0])
	}
}

func TestStructureGraphFromRawBuildsNodesAndRels(t *testing.T) {
	entities, relations := ParseCompiledStructure([]StructureRow{graphRow()}, nil)
	nodes, rels := structureGraphFromRaw(entities, relations)
	if len(nodes) != 2 || len(rels) != 1 {
		t.Fatalf("nodes=%d rels=%d, want 2/1", len(nodes), len(rels))
	}
	if nodes[0].name != "Alpha" || len(nodes[0].sourceChunkIDs) != 2 {
		t.Errorf("node = %+v", nodes[0])
	}
	if rels[0].from != "Alpha" || rels[0].to != "Beta" {
		t.Errorf("rel = %+v", rels[0])
	}
}

func TestBuildTocTreeAndOutline(t *testing.T) {
	entities, relations := ParseCompiledStructure([]StructureRow{graphRow()}, nil)
	nodes, rels := structureGraphFromRaw(entities, relations)
	byName, children, parents, roots := buildTocTree(nodes, rels)
	if len(roots) != 1 || roots[0] != "Alpha" {
		t.Errorf("roots = %v, want [Alpha] (Beta is a child of Alpha)", roots)
	}
	if len(children["Alpha"]) != 1 || children["Alpha"][0] != "Beta" {
		t.Errorf("children[Alpha] = %v, want [Beta]", children["Alpha"])
	}
	if parents["Beta"] != "Alpha" {
		t.Errorf("parents[Beta] = %q, want Alpha", parents["Beta"])
	}
	out := renderOutline(nodes, rels)
	if !strings.Contains(out, "Alpha (section)") || !strings.Contains(out, "Beta (subsection)") {
		t.Errorf("renderOutline = %q", out)
	}
	if !strings.Contains(out, "- Alpha -[contains]-> Beta") {
		t.Errorf("relations must render, got:\n%s", out)
	}
	_ = byName
}
