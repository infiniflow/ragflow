package structure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func relRow(id, from, to string) common.Product {
	return common.Product{
		ID:      id,
		Content: payloadJSON(map[string]any{"type": "linked", "source": from, "target": to}),
		Meta:    map[string]any{"kind": "relation", "from": from, "to": to, "source_chunk_ids": []string{"c1"}},
	}
}

func entRow(id, name string) common.Product {
	return common.Product{
		ID:      id,
		Content: payloadJSON(map[string]any{"type": "letter", "name": name}),
		Meta:    map[string]any{"kind": "entity", "name": name, "source_chunk_ids": []string{"c1"}},
	}
}

func TestChainDetectViolations(t *testing.T) {
	// Clean chain: no issues.
	clean := []chainEdge{{"A", "B"}, {"B", "C"}, {"C", "D"}}
	if got := chainDetectViolations(clean); len(got) != 0 {
		t.Fatalf("clean chain flagged: %v", got)
	}

	bad := []chainEdge{
		{"A", "A"}, // self-loop
		{"A", "B"}, // fan-out partner + cycle member
		{"A", "C"}, // fan-out partner
		{"B", "A"}, // cycle A→B→A
		{"X", "C"},
		{"Y", "C"}, // fan-in to C (together with A→C)
	}
	got := chainDetectViolations(bad)
	hasIssue := func(e chainEdge, substr string) bool {
		for _, iss := range got[e] {
			if strings.Contains(iss, substr) {
				return true
			}
		}
		return false
	}
	if !hasIssue(chainEdge{"A", "A"}, "self-loop") {
		t.Errorf("self-loop not detected: %v", got)
	}
	if !hasIssue(chainEdge{"A", "B"}, "fan-out") || !hasIssue(chainEdge{"A", "C"}, "fan-out") {
		t.Errorf("fan-out not detected on both A edges: %v", got)
	}
	if !hasIssue(chainEdge{"X", "C"}, "fan-in") || !hasIssue(chainEdge{"Y", "C"}, "fan-in") {
		t.Errorf("fan-in not detected on both C edges: %v", got)
	}
	if !hasIssue(chainEdge{"A", "B"}, "cycle") || !hasIssue(chainEdge{"B", "A"}, "cycle") {
		t.Errorf("cycle A↔B not detected: %v", got)
	}
	// Edges uninvolved in any issue must be absent.
	if _, ok := got[chainEdge{"A", "C"}]; ok && !hasIssue(chainEdge{"A", "C"}, "fan-out") {
		t.Errorf("unexpected issue content for A→C: %v", got)
	}
}

func TestChainSCCs(t *testing.T) {
	edges := []chainEdge{{"A", "B"}, {"B", "C"}, {"C", "A"}, {"C", "D"}, {"D", "E"}}
	sccs := chainSCCs(edges)
	if len(sccs) != 1 {
		t.Fatalf("expected exactly one SCC of size>=2, got %d: %v", len(sccs), sccs)
	}
	comp := sccs[0]
	if len(comp) != 3 || !comp["A"] || !comp["B"] || !comp["C"] {
		t.Fatalf("SCC = %v, want {A,B,C}", comp)
	}
}

// keepFirstChainChat keeps the Alpha→Beta edge whenever asked to correct.
type keepFirstChainChat struct{ calls int }

func (m *keepFirstChainChat) Chat(_ context.Context, req common.ChatRequest) (*common.ChatResponse, error) {
	m.calls++
	return &common.ChatResponse{Content: `{"keep":[{"from":"Alpha","to":"Beta"}]}`}, nil
}

type errChainChat struct{}

func (errChainChat) Chat(_ context.Context, _ common.ChatRequest) (*common.ChatResponse, error) {
	return nil, errors.New("llm down")
}

func TestValidateAndCorrectChainFanOut(t *testing.T) {
	rows := []common.Product{
		entRow("e-a", "Alpha"), entRow("e-b", "Beta"), entRow("e-g", "Gamma"),
		relRow("r-ab", "Alpha", "Beta"),
		relRow("r-ag", "Alpha", "Gamma"), // fan-out partner to be dropped
	}
	chat := &keepFirstChainChat{}
	deps := common.Deps{Chat: chat, Embed: hashEmbedder{dim: 8}}
	out := validateAndCorrectChain(context.Background(), deps, "llm1", rows, map[string]string{"c1": "Alpha links Beta and Gamma"}, Type("timeline"))

	var sawAB, sawAG bool
	for _, row := range out {
		if row.ID == "r-ab" {
			sawAB = true
		}
		if row.ID == "r-ag" {
			sawAG = true
		}
	}
	if !sawAB || sawAG {
		t.Fatalf("correction must keep Alpha→Beta and drop Alpha→Gamma: got %v", rowIDs(out))
	}
	if chat.calls != 1 {
		t.Fatalf("expected exactly 1 correction call (single batch, no combined conflicts), got %d", chat.calls)
	}
}

func TestValidateAndCorrectChainFailOpen(t *testing.T) {
	rows := []common.Product{
		relRow("r-ab", "Alpha", "Beta"),
		relRow("r-ag", "Alpha", "Gamma"),
	}
	deps := common.Deps{Chat: errChainChat{}, Embed: hashEmbedder{dim: 8}}
	out := validateAndCorrectChain(context.Background(), deps, "llm1", rows, nil, TypeList)
	if len(out) != len(rows) {
		t.Fatalf("fail-open: LLM failure must return the input untouched (%d vs %d)", len(out), len(rows))
	}

	// Non-chain kinds skip validation entirely (no LLM call possible).
	deps2 := common.Deps{Chat: errChainChat{}}
	out2 := validateAndCorrectChain(context.Background(), deps2, "llm1", rows, nil, TypeHypergraph)
	if len(out2) != len(rows) {
		t.Fatalf("non-chain kind must pass through untouched")
	}
}

func TestDropIsolatedTimelineEntities(t *testing.T) {
	rows := []common.Product{
		entRow("e-a", "Alpha"), entRow("e-b", "Beta"), entRow("e-o", "Orphan"),
		relRow("r-ab", "Alpha", "Beta"),
	}
	out := dropIsolatedTimelineEntities(rows)
	if len(out) != 3 {
		t.Fatalf("expected 3 survivors (2 entities + 1 relation), got %d", len(out))
	}
	for _, row := range out {
		if row.ID == "e-o" {
			t.Fatalf("orphan timeline entity must be dropped")
		}
	}
}

// TestStructureRunTimelineKind exercises the full timeline pipeline end to
// end: extraction → LLM dedup → chain correction (fan-out dropped) →
// orphan-entity cleanup (the dropped relation's target becomes isolated).
func TestStructureRunTimelineKind(t *testing.T) {
	deps := common.Deps{Chat: &graphChat{}, Embed: hashEmbedder{dim: 16}, TenantID: "t1", DatasetID: "d1"}
	param := testParam()
	inputs := common.Inputs{
		DocID: "doc1",
		Chunks: []common.Chunk{
			{ID: "c1", Text: "Alpha is a Beta."},
			{ID: "c2", Text: "Alpha also links Gamma."},
		},
		VariantSpecific: map[string]any{"parser_config": map[string]any{
			"kind": "timeline",
			"entity": map[string]any{"fields": []any{
				map[string]any{"type": "letter", "description": "a single letter entity"},
			}},
			"relation": map[string]any{"fields": []any{
				map[string]any{"type": "linked", "description": "a link between letters"},
			}},
		}},
	}
	out, err := Run(context.Background(), deps, param, inputs)
	if err != nil {
		t.Fatalf("Run timeline: %v", err)
	}
	var entities, relations int
	for _, p := range out.Products {
		switch p.Meta["kind"] {
		case "entity":
			entities++
			if p.Meta["name"] == "Gamma" {
				t.Errorf("Gamma must be dropped: its only relation was corrected away, leaving it isolated")
			}
		case "relation":
			relations++
			if p.Meta["to"] == "Gamma" {
				t.Errorf("fan-out relation Alpha→Gamma must be dropped by chain correction")
			}
		}
	}
	if entities != 2 || relations != 1 {
		t.Fatalf("timeline survivors = %d entities + %d relations, want 2+1 (Alpha, Beta, Alpha→Beta)", entities, relations)
	}
}

func rowIDs(rows []common.Product) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}
