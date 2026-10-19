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
	"sort"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// fakeStructureReader returns canned compiled-structure rows per document.
type fakeStructureReader struct {
	byDoc map[string][]structureRow
}

func (f fakeStructureReader) ReadStructure(_ context.Context, _ string, _ string, docID string) ([]structureRow, error) {
	return f.byDoc[docID], nil
}

func installFakeStructureReader(t *testing.T, r structureReader) {
	t.Helper()
	prev := getStructureReader()
	setStructureReader(r)
	t.Cleanup(func() { setStructureReader(prev) })
}

// fakeClaimStructureReader is a structureReader that also carries a claim leg:
// structure rows and claim hits are canned separately, so a test can prove
// which strategy actually rendered the outline.
type fakeClaimStructureReader struct {
	fakeStructureReader
	claims map[string][]claimHit
	// gotQVec captures the vector the tool handed to the claim leg.
	gotQVec *[]float64
}

func (f fakeClaimStructureReader) ReadClaims(_ context.Context, _ string, _ string, docIDs, _ []string, _ string, qvec []float64, limit int) ([]claimHit, error) {
	if f.gotQVec != nil {
		*f.gotQVec = qvec
	}
	// nil/empty docIDs is the corpus-wide read: every canned document, in stable
	// order so a test asserts on a deterministic page.
	requested := docIDs
	if len(requested) == 0 {
		for docID := range f.claims {
			requested = append(requested, docID)
		}
		sort.Strings(requested)
	}
	var hits []claimHit
	for _, docID := range requested {
		for _, h := range f.claims[docID] {
			if h.docID == "" {
				h.docID = docID
			}
			hits = append(hits, h)
		}
	}
	// Spread a cap across the requested documents rather than truncating in
	// document order: the ranked reader interleaves documents, so truncating by
	// document would model a reader this codebase does not have.
	if limit > 0 && len(hits) > limit {
		var byDoc [][]claimHit
		for _, docID := range requested {
			var group []claimHit
			for _, h := range hits {
				if h.docID == docID {
					group = append(group, h)
				}
			}
			if len(group) > 0 {
				byDoc = append(byDoc, group)
			}
		}
		var spread []claimHit
		for round := 0; len(spread) < limit; round++ {
			progressed := false
			for _, group := range byDoc {
				if round < len(group) {
					spread = append(spread, group[round])
					progressed = true
					if len(spread) == limit {
						break
					}
				}
			}
			if !progressed {
				break
			}
		}
		hits = spread
	}
	return hits, nil
}

func claimRowJSON(name, desc, quote, chunkID string) structureRow {
	content, _ := json.Marshal(map[string]any{
		"type":             "claim",
		"name":             name,
		"description":      desc,
		"source_chunk_ids": []string{chunkID},
		"evidence":         []map[string]any{{"quote": quote, "chunk_id": chunkID}},
	})
	return structureRow{Content: string(content)}
}

// TestNavigateStructureToolIsRegisteredAndBuildableByTemplate pins the wiring:
// the name must be in the registry and toolsFor must build it with scope injected.
func TestNavigateStructureToolIsRegisteredAndBuildableByTemplate(t *testing.T) {
	if _, ok := toolRegistry()[navigateStructureToolName]; !ok {
		t.Fatalf("%s must be registered so a template can select it", navigateStructureToolName)
	}
	tools := toolsFor(context.Background(), Template{ID: "nav", Tools: []string{navigateStructureToolName}}, "tenant-1", []string{"kb1"}, nil)
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
func graphRow() structureRow {
	content, _ := json.Marshal(map[string]any{
		"entities": []map[string]any{
			{"name": "Alpha", "type": "section", "description": "first topic", "source_chunk_ids": []string{"c1", "c2"}},
			{"name": "Beta", "type": "subsection", "description": "second topic", "source_chunk_ids": []string{"c3"}},
		},
		"relations": []map[string]any{
			{"from": "Alpha", "to": "Beta", "type": "contains"},
		},
	})
	return structureRow{KnowledgeGraphKwd: "graph", Content: string(content)}
}

// TestNavigateStructureToolDrillsStructure pins the payload: a routed document's
// compiled structure renders as a query-focused outline with chunk pointers.
func TestNavigateStructureToolDrillsStructure(t *testing.T) {
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]structureRow{
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
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]structureRow{
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
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]structureRow{}})

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
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]structureRow{
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
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]structureRow{}})
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
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]structureRow{}})
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
	v := firstJSONValue("here is the blob: {\"a\": 1, \"b\": [2,3]} trailing text")
	m, ok := v.(map[string]any)
	if !ok || m["a"] != float64(1) {
		t.Errorf("firstJSONValue = %#v ok=%v", v, ok)
	}
	if firstJSONValue("no json here") != nil {
		t.Errorf("firstJSONValue must return nil when no object is present")
	}
}

// TestExtractJSONHandlesEscapedQuotes: a description carrying an escaped quote
// (and braces) must not corrupt the brace depth and silently drop the row.
func TestExtractJSONHandlesEscapedQuotes(t *testing.T) {
	v := firstJSONValue(`{"a": "say \"hi\" {ok}", "b": 1}`)
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("escaped quotes must still parse, got %#v", v)
	}
	if m["a"] != `say "hi" {ok}` || m["b"] != float64(1) {
		t.Errorf("parsed = %#v", m)
	}
}

func TestParseCompiledStructureMergesGraphBlob(t *testing.T) {
	row := graphRow()
	entities, relations := parseCompiledStructure([]structureRow{row}, nil)
	if len(entities) != 2 || len(relations) != 1 {
		t.Fatalf("parsed entities=%d relations=%d, want 2/1", len(entities), len(relations))
	}
	if entities[0]["name"] != "Alpha" || relations[0]["from"] != "Alpha" {
		t.Errorf("unexpected parse: %v / %v", entities[0], relations[0])
	}
}

// TestParseCompiledStructureNormalizesRequestedKind pins the filter symmetry: a
// caller asking for kind "graph" must still match a "graph" row, even though
// rowKind normalizes that row shape to "timeline" (and a plain string compare
// would drop every row before parsing).
func TestParseCompiledStructureNormalizesRequestedKind(t *testing.T) {
	entities, relations := parseCompiledStructure([]structureRow{graphRow()}, []string{"graph"})
	if len(entities) != 2 || len(relations) != 1 {
		t.Fatalf("kind=graph must not filter out the graph row: entities=%d relations=%d", len(entities), len(relations))
	}
	if entities, relations := parseCompiledStructure([]structureRow{graphRow()}, []string{"mind_map"}); len(entities) != 0 || len(relations) != 0 {
		t.Fatalf("an unrelated kind must still filter the row out: %d/%d", len(entities), len(relations))
	}
}

func TestStructureGraphFromRawBuildsNodesAndRels(t *testing.T) {
	entities, relations := parseCompiledStructure([]structureRow{graphRow()}, nil)
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
	entities, relations := parseCompiledStructure([]structureRow{graphRow()}, nil)
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

// --- Claim/evidence leg ---

// TestNavigateStructureClaimLegReplacesDrill pins claim-first: a claim hit takes
// over the selection, so the tree is NOT descended even when structure rows
// exist for the same document.
func TestNavigateStructureClaimLegReplacesDrill(t *testing.T) {
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{
			"d1": {graphRow()},
		}},
		claims: map[string][]claimHit{
			"d1": {{
				chunkID:     "c9",
				name:        "Paris is the capital of France",
				description: "capital city",
				quote:       "Paris is the capital of France.",
				score:       0.8,
			}},
		},
	})

	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"capital of France","doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	for _, want := range []string{
		`<structure_navigation count="1" query="capital of France" kind="">`,
		`<doc rank="1" doc_id="d1"`,
		// The outline is escaped as a whole, so the quote's own quotes ship as
		// &quot; (the same escaping the harness applies).
		`- [claim] Paris is the capital of France | Evidence: &quot;Paris is the capital of France.&quot; [chunks: c9]`,
		`</structure>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output must contain %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Alpha (section)") {
		t.Errorf("a claim hit must replace the drill, so no structure node may render, got:\n%s", out)
	}
}

// TestNavigateStructureRendersEveryClaim: the tool renders the claims it recalled,
// with no self-declared sufficiency verdict - whether a claim set is enough to
// answer from is decided by the pipeline's pre-loop recall, not by a marker the
// model has to interpret (and which could never agree with that gate's
// document-diversity rule).
func TestNavigateStructureRendersEveryClaim(t *testing.T) {
	hits := []claimHit{
		{chunkID: "c1", name: "claim one", quote: "q1"},
		{chunkID: "c2", name: "claim two", quote: "q2"},
		{chunkID: "c3", name: "claim three", quote: "q3"},
	}
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims:              map[string][]claimHit{"d1": hits},
	})

	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"claims","doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got := strings.Count(out, "- [claim] "); got != len(hits) {
		t.Errorf("rendered %d claim lines, want %d:\n%s", got, len(hits), out)
	}
}

// TestNavigateStructureClaimOnlyDocumentIsNotNoStructure: a document whose
// compiler wrote claims but no tree nodes still renders — counting only entities
// would report "no compiled structure" and throw the evidence away.
func TestNavigateStructureClaimOnlyDocumentIsNotNoStructure(t *testing.T) {
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{"d1": {
			{chunkID: "c1", name: "only claim", quote: "only quote"},
		}},
	})

	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"only","doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `count="1"`) {
		t.Fatalf("a claim-only document must still render, got:\n%s", out)
	}
	if strings.Contains(out, "no compiled structure") {
		t.Errorf("claim-only document must not report no_structure, got:\n%s", out)
	}
}

// TestNavigateStructureClaimLegReceivesQueryVector: the tool must hand the nav
// service's query vector to the claim leg (the dense half of hybrid recall), and
// must pass nothing when no service is installed.
func TestNavigateStructureClaimLegReceivesQueryVector(t *testing.T) {
	var got []float64
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims:              map[string][]claimHit{"d1": {{chunkID: "c1", name: "n", quote: "q"}}},
		gotQVec:             &got,
	})
	installFakeNavService(t, fakeNavService{
		encode: func(context.Context, string, string) []float64 { return []float64{0.25, 0.5} },
	})

	if _, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"q","doc_id":"d1"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if len(got) != 2 || got[0] != 0.25 {
		t.Errorf("claim leg received qvec %v, want the service's vector", got)
	}

	installFakeNavService(t, nil)
	if _, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"q","doc_id":"d1"}`); err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("without a nav service the claim leg must run lexical-only, got qvec %v", got)
	}
}

// TestClaimQueryVectorSwallowsPanic: a misconfigured embedder degrades recall to
// the lexical leg; it must never take the tool call down.
func TestClaimQueryVectorSwallowsPanic(t *testing.T) {
	installFakeNavService(t, fakeNavService{
		encode: func(context.Context, string, string) []float64 { panic("boom") },
	})
	if vec := claimQueryVector(context.Background(), "tenant-1", "q"); vec != nil {
		t.Errorf("a panicking encoder must yield no vector, got %v", vec)
	}
}

func TestParseClaimHitReadsEvidenceAndPointer(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": claimRowJSON("Paris is the capital", "capital city", "Paris is the capital of France.", "c9").Content,
		"_score":              float64(0.7),
	}
	hit, ok := parseClaimHit(row)
	if !ok {
		t.Fatalf("a well-formed claim row must parse")
	}
	if hit.chunkID != "c9" || hit.name != "Paris is the capital" || hit.quote != "Paris is the capital of France." {
		t.Errorf("hit = %+v", hit)
	}
	if hit.score != 0.7 {
		t.Errorf("score = %v, want 0.7", hit.score)
	}

	// No chunk pointer: the model could not open it, so it is not a hit.
	orphan, _ := json.Marshal(map[string]any{"name": "orphan claim", "description": "d"})
	if _, ok := parseClaimHit(map[string]interface{}{"content_with_weight": string(orphan)}); ok {
		t.Errorf("a claim without source_chunk_ids must be dropped")
	}
	// Column form (engines return the id list as a column).
	col := map[string]interface{}{
		"content_with_weight": `{"name":"col claim"}`,
		"source_chunk_ids":    []interface{}{"c7"},
	}
	if hit, ok := parseClaimHit(col); !ok || hit.chunkID != "c7" {
		t.Errorf("column source_chunk_ids must be honored, got %+v ok=%v", hit, ok)
	}
}

// TestRRFFuseMergesLegsByRank: RRF consumes ranks only, so a hit found by both
// legs outranks one found by a single leg, and the fused list is capped.
func TestRRFFuseMergesLegsByRank(t *testing.T) {
	dense := []claimHit{{name: "both", chunkID: "c1", score: 0.9}, {name: "only dense", chunkID: "c2", score: 0.8}}
	text := []claimHit{{name: "only text", chunkID: "c3", score: 11}, {name: "both", chunkID: "c1", score: 3}}

	fused := rrfFuse(navClaimRRFK, 0, dense, text)
	if len(fused) != 3 {
		t.Fatalf("fused %d hits, want 3: %+v", len(fused), fused)
	}
	if fused[0].name != "both" {
		t.Errorf("the hit found by BOTH legs must rank first, got %+v", fused)
	}
	// A fused hit keeps the best raw score across the legs that found it; the
	// legs' scales are incomparable, which is exactly why fusion uses ranks.
	if fused[0].score != 3 {
		t.Errorf("a fused hit keeps its best raw score, got %v", fused[0].score)
	}
	if len(fused) != 3 || fused[1].name != "only text" || fused[2].name != "only dense" {
		t.Errorf("single-leg hits must follow the double-leg hit, got %+v", fused)
	}
	if capped := rrfFuse(navClaimRRFK, 2, dense, text); len(capped) != 2 {
		t.Errorf("limit must cap the fused list, got %d", len(capped))
	}
}

// TestClaimRecallKeepsDistinctClaimsFromOneChunk: several facts commonly come
// from the same passage, so dedupe must be by claim, not by chunk id.
func TestClaimRecallKeepsDistinctClaimsFromOneChunk(t *testing.T) {
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{"d1": {
			{chunkID: "c1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
			{chunkID: "c1", name: "Acme is based in Zurich", quote: "the Zurich-based firm"},
		}},
	})
	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"Acme","doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got := strings.Count(out, "- [claim] "); got != 2 {
		t.Errorf("both claims from the shared chunk must render, got %d:\n%s", got, out)
	}
}

// TestClaimOutlineLineShape pins the claim line the model reads.
func TestClaimOutlineLineShape(t *testing.T) {
	got := claimOutlineLine(context.Background(), claimHit{name: "Acme was founded in 1994", quote: "Acme, founded in 1994, ...", chunkID: "c1"})
	want := `- [claim] Acme was founded in 1994 | Evidence: "Acme, founded in 1994, ..." [chunks: c1]`
	if got != want {
		t.Errorf("claim line =\n%s\nwant\n%s", got, want)
	}
	// No evidence: the statement and pointer still render.
	got = claimOutlineLine(context.Background(), claimHit{name: "bare claim", chunkID: "c2"})
	if got != "- [claim] bare claim [chunks: c2]" {
		t.Errorf("bare claim line = %q", got)
	}
}

// TestClaimOutlineLineCarriesHandle: a claim is answer material, so its line
// must publish the passage handle the answer is expected to cite. Without it the
// reference payload cannot resolve a claim-backed answer.
func TestClaimOutlineLineCarriesHandle(t *testing.T) {
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	got := claimOutlineLine(ctx, claimHit{name: "Acme was founded in 1994", chunkID: "c1"})
	if !strings.HasSuffix(got, " [chunks: c1] ref=0") {
		t.Fatalf("claim line must carry the passage handle, got %q", got)
	}
	// The same chunk keeps its first handle.
	if again := claimOutlineLine(ctx, claimHit{name: "Acme is in Zurich", chunkID: "c1"}); !strings.HasSuffix(again, "ref=0") {
		t.Errorf("a repeated chunk must keep its handle, got %q", again)
	}
	// A different chunk takes the next one.
	if next := claimOutlineLine(ctx, claimHit{name: "Acme employs 900", chunkID: "c2"}); !strings.HasSuffix(next, "ref=1") {
		t.Errorf("a new chunk must take the next handle, got %q", next)
	}
}

// --- Query-only route (the tool must be usable in one call) ---

func installFakeStructureRoute(t *testing.T, docs [][2]string) {
	t.Helper()
	prev := navStructureRoute
	navStructureRoute = func(context.Context, string, string, string, int) ([][2]string, error) {
		return docs, nil
	}
	t.Cleanup(func() { navStructureRoute = prev })
}

// TestNavigateStructureQueryOnlyRoutesAndClaims: without a doc_id the tool routes
// the query itself and returns compiled claim evidence - this is what makes the
// tool callable in one step, without a prior navigate_tree.
func TestNavigateStructureQueryOnlyRoutesAndClaims(t *testing.T) {
	installFakeStructureRoute(t, [][2]string{{"d1", "Alpha"}})
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{"d1": {{
			chunkID: "c1",
			name:    "Acme was founded in 1994",
			quote:   "Acme, founded in 1994",
		}}},
	})

	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"when was Acme founded"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	for _, want := range []string{
		`count="1"`,
		`doc_id="d1"`,
		`- [claim] Acme was founded in 1994 | Evidence: &quot;Acme, founded in 1994&quot; [chunks: c1]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output must contain %q, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "route first with navigate_tree") {
		t.Errorf("a routed query must not ask for navigate_tree, got:\n%s", out)
	}
}

// TestNavigateStructureQueryOnlyNoClaimsIsAHint: routing that reaches documents
// with no claims says so instead of pretending the corpus has no structure.
func TestNavigateStructureQueryOnlyNoClaimsIsAHint(t *testing.T) {
	installFakeStructureRoute(t, [][2]string{{"d1", "Alpha"}})
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims:              map[string][]claimHit{},
	})

	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"nothing here"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `count="0"`) || !strings.Contains(out, "no claim hits") {
		t.Errorf("expected a claim-miss hint, got:\n%s", out)
	}
}

// TestNavigateStructureQueryOnlyNoRouteKeepsTheOldHint: when routing finds nothing
// at all, the tool still tells the model to route with navigate_tree.
func TestNavigateStructureQueryOnlyNoRouteKeepsTheOldHint(t *testing.T) {
	installFakeStructureRoute(t, nil)
	installFakeStructureReader(t, fakeStructureReader{byDoc: map[string][]structureRow{}})

	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"nothing"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "route first with navigate_tree") {
		t.Errorf("expected the routing hint, got:\n%s", out)
	}
}

// TestRecallCompiledClaimsSufficient: enough distinct claims, from more than one
// document, make the turn answerable without reading a single raw chunk - and
// the evidence they carry is rendered WITH the handles the answer must cite.
func TestRecallCompiledClaimsSufficient(t *testing.T) {
	stubClaimProbe(t, true)
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{
			"d1": {
				{chunkID: "c1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
				{chunkID: "c1", name: "Acme is based in Zurich", quote: "the Zurich-based firm"},
			},
			"d2": {
				{chunkID: "c2", name: "Acme employs 900 people", quote: "a workforce of 900"},
			},
		},
	})
	msgs := []*schema.Message{{Role: schema.User, Content: "when was Acme founded"}}
	ctx := WithEvidenceRegistry(context.Background(), NewEvidenceRegistry())
	pf := recallCompiledClaims(ctx, "tenant-1", []string{"kb1"}, msgs)

	if !pf.sufficient {
		t.Fatalf("3 claims over 2 documents must be sufficient, got %d claims over %v", len(pf.claims), pf.docs)
	}
	if len(pf.claims) != 3 || len(pf.docs) != 2 {
		t.Fatalf("recall = %d claims over %v, want 3 over 2 documents", len(pf.claims), pf.docs)
	}
	if !strings.Contains(pf.evidence, `doc_id="d1"`) || !strings.Contains(pf.evidence, `doc_id="d2"`) ||
		!strings.Contains(pf.evidence, "- [claim] Acme was founded in 1994") {
		t.Errorf("evidence must carry both docs and their claims, got:\n%s", pf.evidence)
	}
	for _, want := range []string{"ref=0", "ref=1"} {
		if !strings.Contains(pf.evidence, want) {
			t.Errorf("evidence must publish %q so the answer can cite it, got:\n%s", want, pf.evidence)
		}
	}
}

// TestRecallCompiledClaimsSingleDocumentIsNotSufficient: one document's claims
// answer one hop at most. This is also the exact shape a broken document ranking
// produces for every question, so the gate must refuse it.
func TestRecallCompiledClaimsSingleDocumentIsNotSufficient(t *testing.T) {
	stubClaimProbe(t, true)
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{"d1": {
			{chunkID: "c1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"},
			{chunkID: "c1", name: "Acme is based in Zurich", quote: "the Zurich-based firm"},
			{chunkID: "c2", name: "Acme employs 900 people", quote: "a workforce of 900"},
		}},
	})
	msgs := []*schema.Message{{Role: schema.User, Content: "when was Acme founded"}}
	pf := recallCompiledClaims(context.Background(), "tenant-1", []string{"kb1"}, msgs)
	if pf.sufficient {
		t.Fatalf("claims from one document must not terminate the turn, got %d claims over %v", len(pf.claims), pf.docs)
	}
	if pf.evidence != "" {
		t.Errorf("an insufficient recall must not render evidence, got:\n%s", pf.evidence)
	}
}

// TestRecallCompiledClaimsEmptyDocumentScopeSkipsRecall: a non-nil EMPTY scope
// means the turn allows no documents. The reader treats an empty docIDs slice as
// "the whole dataset", so the recall must stop before the read instead of
// widening the turn's scope to everything.
func TestRecallCompiledClaimsEmptyDocumentScopeSkipsRecall(t *testing.T) {
	stubClaimProbe(t, true)
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{
			"d1": {{chunkID: "c1", name: "Acme was founded in 1994", quote: "Acme, founded in 1994"}},
			"d2": {{chunkID: "c2", name: "Acme employs 900 people", quote: "a workforce of 900"}},
		},
	})
	msgs := []*schema.Message{{Role: schema.User, Content: "when was Acme founded"}}
	ctx := withDocumentScope(context.Background(), []string{})
	pf := recallCompiledClaims(ctx, "tenant-1", []string{"kb1"}, msgs)
	if len(pf.claims) != 0 || pf.sufficient || pf.evidence != "" {
		t.Fatalf("an empty document scope must skip the recall, got %d claims (sufficient=%v, evidence=%q)",
			len(pf.claims), pf.sufficient, pf.evidence)
	}
}

// TestRecallCompiledClaimsInsufficientIsDropped: a recall below the sufficiency
// count must not reach the model - neither as evidence nor as published handles
// for passages it never saw.
func TestRecallCompiledClaimsInsufficientIsDropped(t *testing.T) {
	stubClaimProbe(t, true)
	installFakeStructureRoute(t, [][2]string{{"d1", "Alpha"}})
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{"d1": {{
			chunkID: "c1",
			name:    "Acme was founded in 1994",
			quote:   "Acme, founded in 1994",
		}}},
	})
	msgs := []*schema.Message{{Role: schema.User, Content: "when was Acme founded"}}
	registry := NewEvidenceRegistry()
	pf := recallCompiledClaims(WithEvidenceRegistry(context.Background(), registry), "tenant-1", []string{"kb1"}, msgs)
	if pf.sufficient {
		t.Fatalf("1 claim must not be sufficient (%d required)", navClaimSufficientHits)
	}
	if pf.evidence != "" {
		t.Errorf("an insufficient recall must not render evidence, got:\n%s", pf.evidence)
	}
	if got := len(registry.IDs()); got != 0 {
		t.Errorf("a dropped recall must not publish handles, got %d", got)
	}
}

// TestRecallCompiledClaimsSkipsEmptyHits: nothing matched, so nothing is
// offered - the loop runs exactly as it would without the recall.
func TestRecallCompiledClaimsSkipsEmptyHits(t *testing.T) {
	stubClaimProbe(t, true)
	installFakeStructureRoute(t, [][2]string{{"d1", "Alpha"}})
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims:              map[string][]claimHit{},
	})
	msgs := []*schema.Message{{Role: schema.User, Content: "nothing here"}}
	pf := recallCompiledClaims(context.Background(), "tenant-1", []string{"kb1"}, msgs)
	if pf.sufficient || pf.evidence != "" || len(pf.claims) != 0 {
		t.Errorf("an empty recall must yield an empty verdict, got %+v", pf)
	}
}

// stubClaimProbe drives the claim-prefetch gate without an engine.
func stubClaimProbe(t *testing.T, present bool) {
	t.Helper()
	prev := claimRowsPresent
	claimRowsPresent = func(context.Context, string, []string) bool { return present }
	t.Cleanup(func() { claimRowsPresent = prev })
}

// claimsPerDoc builds n distinct claims for one document.
func claimsPerDoc(docID string, n int) []claimHit {
	out := make([]claimHit, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, claimHit{
			chunkID: docID + "-c" + string(rune('a'+i)),
			name:    docID + " claim " + string(rune('a'+i)),
			quote:   "quote",
		})
	}
	return out
}

// TestRecallCapsClaims: the recall block is the whole prompt of the answer call,
// so it honours its cap - and the cap is spread across documents, not spent on
// the first one.
func TestRecallCapsClaims(t *testing.T) {
	stubClaimProbe(t, true)
	docs := []string{"d1", "d2", "d3", "d4"}
	claims := map[string][]claimHit{}
	for _, d := range docs {
		claims[d] = claimsPerDoc(d, 8)
	}
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims:              claims,
	})
	msgs := []*schema.Message{{Role: schema.User, Content: "acme"}}
	pf := recallCompiledClaims(context.Background(), "tenant-1", []string{"kb1"}, msgs)
	if got := strings.Count(pf.evidence, "- [claim] "); got != navClaimPrefetchTopN {
		t.Errorf("recall rendered %d claims, want the cap %d:\n%s", got, navClaimPrefetchTopN, pf.evidence)
	}
	for _, docID := range docs {
		if !strings.Contains(pf.evidence, `doc_id="`+docID+`"`) {
			t.Errorf("the cap must be spread across documents, %s missing:\n%s", docID, pf.evidence)
		}
	}
}

// TestClaimAnswerPromptCarriesEvidenceAndCitationRule pins what the
// sufficient-claims answer depends on: the evidence itself, the rule that turns
// each fact line into a resolvable reference, and the refusal path - a model that
// narrates instead of answering must have an explicit way to say so, because the
// caller falls back to the loop on it.
func TestClaimAnswerPromptCarriesEvidenceAndCitationRule(t *testing.T) {
	evidence := "- [claim] Acme was founded in 1994 [chunks: c1] ref=0"
	prompt := claimAnswerPrompt(evidence)
	for _, want := range []string{evidence, "[ID:n]", "ref=n", "Do not call tools", `"answerable"`} {
		if !strings.Contains(prompt, want) {
			t.Errorf("claim answer prompt must contain %q, got:\n%s", want, prompt)
		}
	}
}

// TestParseClaimAnswer: the verdict decides control flow, so both the refusal and
// the malformed-reply cases must fall back rather than ship prose.
func TestParseClaimAnswer(t *testing.T) {
	ok := parseClaimAnswer(`{"answerable": true, "answer": "1994 [ID:0]"}`)
	if !ok.answerable || ok.answer != "1994 [ID:0]" {
		t.Errorf("answerable verdict parsed as %+v", ok)
	}
	no := parseClaimAnswer(`{"answerable": false, "missing": "the founding year"}`)
	if no.answerable || no.missing != "the founding year" {
		t.Errorf("refusal parsed as %+v", no)
	}
	// A model that narrates instead of answering must NOT be shipped as the
	// answer: that is the zero-score failure mode.
	narrated := parseClaimAnswer("I need to research who Brian Bergstein is. Let me search for this information.")
	if narrated.answerable || narrated.answer != "" {
		t.Errorf("narrated reply must be treated as not answerable, got %+v", narrated)
	}
	// "answerable" without an answer is not an answer.
	empty := parseClaimAnswer(`{"answerable": true, "answer": "   "}`)
	if empty.answerable {
		t.Errorf("empty answer must be not answerable, got %+v", empty)
	}
}

// TestQueryOnlyToolCallIsUncapped: a model-invoked call is not capped - the model
// asked for it and its result lands in the history once.
func TestQueryOnlyToolCallIsUncapped(t *testing.T) {
	installFakeStructureRoute(t, [][2]string{{"d1", "A"}, {"d2", "B"}})
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		claims: map[string][]claimHit{
			"d1": claimsPerDoc("d1", 5),
			"d2": claimsPerDoc("d2", 5),
		},
	})
	out, err := NewNavigateStructureTool("tenant-1", []string{"kb1"}).
		InvokableRun(context.Background(), `{"query":"acme"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if got := strings.Count(out, "- [claim] "); got != 10 {
		t.Errorf("a model-invoked call must not be capped, got %d claims:\n%s", got, out)
	}
}

// TestRecallSkippedWithoutClaimRows: a compiled but claim-less dataset (a tree
// or timeline) must not pay for a routing call and two retrieval legs.
func TestRecallSkippedWithoutClaimRows(t *testing.T) {
	stubClaimProbe(t, false)
	installFakeStructureRoute(t, [][2]string{{"d1", "A"}})
	installFakeStructureReader(t, fakeClaimStructureReader{
		fakeStructureReader: fakeStructureReader{byDoc: map[string][]structureRow{}},
		// Claims exist in the fake store on purpose: the gate must win, otherwise
		// this test would pass for the wrong reason.
		claims: map[string][]claimHit{"d1": {{
			chunkID: "c1",
			name:    "Acme was founded in 1994",
			quote:   "Acme, founded in 1994",
		}}},
	})
	msgs := []*schema.Message{{Role: schema.User, Content: "when was Acme founded"}}
	pf := recallCompiledClaims(context.Background(), "tenant-1", []string{"kb1"}, msgs)
	if pf.sufficient || pf.evidence != "" || len(pf.claims) != 0 {
		t.Errorf("no claim rows must disable the recall, got %+v", pf)
	}
}
