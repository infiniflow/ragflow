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

// TestClaimAnswerPromptCarriesEvidenceAndCitationRule pins what the answer call
// depends on: the evidence itself, the rule that turns each fact line into a
// resolvable reference, and the two answer rules this corpus needs - pick the
// candidate the question names, and name the inputs of a computation.
func TestClaimAnswerPromptCarriesEvidenceAndCitationRule(t *testing.T) {
	evidence := "- [claim] Acme was founded in 1994 [chunks: c1] ref=0"
	prompt := claimAnswerPrompt(evidence)
	for _, want := range []string{
		evidence, "[ID:n]", "ref=n", "Do not call tools", `"answer"`,
		"the one the question names (last, first, most",
		"derived_from", "A derivation with one input is not a derivation",
		"complete list", `"answerable": false`,
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("claim answer prompt must contain %q, got:\n%s", want, prompt)
		}
	}
}

// TestClaimVerifyPromptTargetsNearMisses: the verifier exists to catch the two
// measured wrong shortcuts, both of which were "right subject, wrong fact". It
// must name that failure, must not become a second answerer, and must carry both
// the page and the candidate.
func TestClaimVerifyPromptTargetsNearMisses(t *testing.T) {
	evidence := "- [claim] The King's Speech was released in 2010 [chunks: c1] ref=0"
	answer := "Colin Firth received his first Academy Award in 2010. [ID:0]"
	prompt := claimVerifyPrompt(evidence, answer)
	for _, want := range []string{
		evidence, answer, "near miss", "release year", "last/latest",
		"Do not rewrite the answer", `"supported": false`, "The page is the only source",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("claim verify prompt must contain %q, got:\n%s", want, prompt)
		}
	}
}

// TestClaimHandleCount backs the floor a derived answer has to clear: distinct
// handles only, so repeating one claim three times cannot pass as a derivation.
func TestClaimHandleCount(t *testing.T) {
	cases := []struct {
		answer string
		want   int
	}{
		{"no handles at all", 0},
		{"1994 [ID:0]", 1},
		{"1994 [ID:0] and 56 years later [ID:0]", 1},
		{"1967 minus 56 = 1911 [ID:1][ID:3]", 2},
		{"[ID:7], [ID:11], [ID:7]", 2},
	}
	for _, c := range cases {
		if got := claimHandleCount(c.answer); got != c.want {
			t.Errorf("claimHandleCount(%q) = %d, want %d", c.answer, got, c.want)
		}
	}
}

// TestParseClaimAnswer: only an explicit answerable answer may produce text, and
// a reply with no answer field is not one - shipping prose is the zero-score
// failure mode. The refusal keeps its named gap for the gap_overlap diagnostic.
func TestParseClaimAnswer(t *testing.T) {
	ok := parseClaimAnswer(`{"answerable": true, "answer": "1994 [ID:0]"}`)
	if !ok.answerable || ok.answer != "1994 [ID:0]" || ok.missing != "" {
		t.Errorf("answer parsed as %+v", ok)
	}
	no := parseClaimAnswer(`{"answerable": false, "missing": "the founding year"}`)
	if no.answerable || no.missing != "the founding year" {
		t.Errorf("refusal parsed as %+v", no)
	}
	narrated := parseClaimAnswer("I need to research who Brian Bergstein is. Let me search for this information.")
	if narrated.answerable || narrated.answer != "" {
		t.Errorf("narrated reply must be treated as not answerable, got %+v", narrated)
	}
	empty := parseClaimAnswer(`{"answerable": true, "answer": "   "}`)
	if empty.answerable {
		t.Errorf("empty answer must be not answerable, got %+v", empty)
	}
	// An answer without the flag is still an answer: treating it as a refusal
	// would hand a usable answer to the loop.
	noFlag := parseClaimAnswer(`{"answer": "1994 [ID:0]"}`)
	if !noFlag.answerable || noFlag.answer != "1994 [ID:0]" {
		t.Errorf("answer without the flag parsed as %+v", noFlag)
	}
	// derived_from as a JSON list is read as-is.
	derived := parseClaimAnswer(`{"answerable": true, "answer": "1994 [ID:0]", "derived_from": ["[ID:0]", "[ID:1]"]}`)
	if got := claimDerivedInputs(derived); len(got) != 2 {
		t.Errorf("derived_from list parsed as %+v", got)
	}
}

// TestClaimDerivedInputs: the prompt asks for a `derived_from:` line inside the
// answer, so the floor has to read both forms - otherwise a derivation written as
// prose would slip past the "at least two inputs" check.
func TestClaimDerivedInputs(t *testing.T) {
	asLine := parseClaimAnswer(`{"answerable": true, "answer": "5 + 6 = 11 [ID:0] [ID:1]\nderived_from: [ID:0], [ID:1]"}`)
	if got := claimDerivedInputs(asLine); len(got) != 2 {
		t.Errorf("derived_from line parsed as %+v", got)
	}
	// The JSON field wins over the line when both are present.
	both := parseClaimAnswer(`{"answerable": true, "answer": "x [ID:0]\nderived_from: [ID:5]", "derived_from": ["[ID:0]", "[ID:1]", "[ID:2]"]}`)
	if got := claimDerivedInputs(both); len(got) != 3 {
		t.Errorf("json field must take precedence, got %+v", got)
	}
	plain := parseClaimAnswer(`{"answerable": true, "answer": "1994 [ID:0]"}`)
	if got := claimDerivedInputs(plain); len(got) != 0 {
		t.Errorf("a direct answer names no inputs, got %+v", got)
	}
	// Repeating one handle is one input, not two.
	repeated := parseClaimAnswer(`{"answerable": true, "answer": "x [ID:0]\nderived_from: [ID:0], [ID:0]"}`)
	if got := claimDistinctHandles(claimDerivedInputs(repeated)); got != 1 {
		t.Errorf("repeated handle counted as %d inputs, want 1", got)
	}
}

// TestParseClaimSupport: a rejection needs an explicit `supported: false`; every
// other reply counts as support, because refusing a good answer hands the
// question to the loop for 300-600k tokens. The unparsed case stays visible
// through parsed=false so that default cannot mask a prompt regression.
func TestParseClaimSupport(t *testing.T) {
	ok := parseClaimSupport(`{"supported": true}`)
	if !ok.supported || !ok.parsed || ok.reason != "" {
		t.Errorf("support parsed as %+v", ok)
	}
	no := parseClaimSupport(`{"supported": false, "reason": "2010 is the release year, not the award year"}`)
	if no.supported || !no.parsed || !strings.Contains(no.reason, "release year") {
		t.Errorf("rejection parsed as %+v", no)
	}
	narrated := parseClaimSupport("This looks right to me.")
	if !narrated.supported || narrated.parsed {
		t.Errorf("unparsed reply must fail open and be flagged, got %+v", narrated)
	}
	missingField := parseClaimSupport(`{"verdict": "ok"}`)
	if !missingField.supported || missingField.parsed {
		t.Errorf("reply without a supported field must fail open, got %+v", missingField)
	}
}
