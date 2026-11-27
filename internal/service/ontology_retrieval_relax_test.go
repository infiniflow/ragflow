package service

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/service/nlp"
)

// relationRow is a compiled assertion row: it carries the property and the
// endpoint classes, and no entity_type_kwd — which is exactly why a class and a
// property can never be matched by one AND-ed clause.
func relationRow(prop, from, to string) map[string]interface{} {
	return map[string]interface{}{
		"type_kwd":        "relation",
		"prop_kwd":        prop,
		"from_entity_kwd": from,
		"to_entity_kwd":   to,
		"from_type_kwd":   "person",
		"to_type_kwd":     "organization",
		"name_kwd":        prop,
	}
}

// A constraint set whose full form matches nothing is relaxed one clause at a
// time. The rows that survive are still narrowed, so they are still structured
// evidence — `constraint_partial`, not `no_match`.
func TestOntologyABoxRetrievalRelaxesOneClauseAndKeepsTheRest(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1", Name: "Guild model", Version: 3}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1},
	)

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"person"}
	req.Properties = []string{"leads"}
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 2 {
		t.Fatalf("engine calls = %d, want the full set then the class half", svc.searcher.callCount())
	}
	if !result.StructureApplied {
		t.Fatal("structure_applied = false, want the relaxed set reported as applied")
	}
	if result.DegradeCode != OntologyABoxDegradeConstraintPartial {
		t.Fatalf("degrade = %q, want %q", result.DegradeCode, OntologyABoxDegradeConstraintPartial)
	}
	if !strings.Contains(result.DegradeReason, "classes=person") {
		t.Fatalf("reason = %q, want the surviving constraint named", result.DegradeReason)
	}
	// The echo is the filter that actually ran: the class clause only.
	if _, ok := result.Filter["entity_type_kwd"]; !ok {
		t.Fatalf("filter echo = %#v, want the class clause", result.Filter)
	}
	if _, leaked := result.Filter["prop_kwd"]; leaked {
		t.Fatalf("filter echo = %#v, want the dropped clause absent", result.Filter)
	}
	if result.Digest.EvidenceLevel != agentic_rag.OntologyEvidenceStructured {
		t.Fatalf("evidence level = %q, want structured", result.Digest.EvidenceLevel)
	}
}

// When no rung of the ladder matches, the search falls back to the kind pin and
// says so: the rows are not narrowed, and `no_match` is what tells the caller
// that the model's coverage — not the corpus — came up short.
func TestOntologyABoxRetrievalFallsBackWhenNothingNarrows(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1", Name: "Guild model", Version: 3}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{}, Total: 0},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{relationRow("leads", "ada", "guild")}, Total: 1},
	)

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"person"}
	req.Properties = []string{"leads"}
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 4 {
		t.Fatalf("engine calls = %d, want full, class half, property half, unnarrowed", svc.searcher.callCount())
	}
	if result.StructureApplied {
		t.Fatal("structure_applied = true, want false after every rung missed")
	}
	if result.DegradeCode != OntologyABoxDegradeNoMatch {
		t.Fatalf("degrade = %q, want %q", result.DegradeCode, OntologyABoxDegradeNoMatch)
	}
	if result.Filter != nil {
		t.Fatalf("filter echo = %#v, want nil for an unnarrowed search", result.Filter)
	}
	last := svc.searcher.calls[len(svc.searcher.calls)-1]
	if _, narrowed := last.Filter["entity_type_kwd"]; narrowed {
		t.Fatalf("last filter = %#v, want the kind pin only", last.Filter)
	}
}

// The class-directed leg fires only when the ranked page shows no entity of a
// requested class: a page whose ranking already surfaced one does not pay for a
// second retrieval.
func TestOntologyABoxRetrievalAsksForEntitiesOnlyWhenThePageHasNone(t *testing.T) {
	svc := newOntologyTestService(guildScope(),
		&fakeOntologyModel{declared: guildOntology(), ref: OntologyModelRef{ID: "tpl-1", Name: "Guild model", Version: 3}},
		nil,
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{relationRow("leads", "ada", "guild")}, Total: 1},
		&nlp.RetrievalResult{Chunks: []map[string]interface{}{entityRow()}, Total: 1},
	)

	req := validOntologyABoxRequest()
	req.EntityTypes = []string{"person"}
	req.TopK = 4
	result, err := svc.svc.Retrieve(context.Background(), req)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if svc.searcher.callCount() != 2 {
		t.Fatalf("engine calls = %d, want the narrowed search then the entity leg", svc.searcher.callCount())
	}
	leg := svc.searcher.calls[1]
	branch, _ := leg.Filter["entity_type_kwd"].([]string)
	if !containsStringValue(branch, "person") {
		t.Fatalf("entity leg filter = %#v, want the requested class", leg.Filter)
	}
	if kinds, _ := leg.Filter["knowledge_graph_kwd"].([]string); len(kinds) != 1 || kinds[0] != "entity" {
		t.Fatalf("entity leg kind pin = %#v, want [entity]", leg.Filter["knowledge_graph_kwd"])
	}
	if leg.PageSize >= req.TopK {
		t.Fatalf("entity leg page size = %d, want a reserved slice of top_k=%d", leg.PageSize, req.TopK)
	}
	if len(result.Chunks) != 2 {
		t.Fatalf("rows = %d, want the assertion and the entity row", len(result.Chunks))
	}
	if result.Chunks[1]["entity_type_kwd"] != "person" {
		t.Fatalf("rows = %#v, want the entity row merged in", result.Chunks)
	}
}

// The merged page reserves room for the directed rows: the ranking cannot push
// them out of a full page, because a full page of assertions is exactly the case
// the leg exists for.
func TestMergeOntologyChunkPagesReservesRoomForDirectedRows(t *testing.T) {
	primary := []map[string]interface{}{{"id": "a"}, {"id": "b"}, {"id": "c"}}
	extra := []map[string]interface{}{{"id": "x"}}
	merged := mergeOntologyChunkPages(primary, extra, 3)
	if len(merged) != 3 {
		t.Fatalf("merged = %#v, want three rows", merged)
	}
	if merged[2]["id"] != "x" {
		t.Fatalf("merged = %#v, want the directed row kept", merged)
	}
	// A row already on the page is not duplicated by the leg.
	merged = mergeOntologyChunkPages(primary, []map[string]interface{}{{"id": "b"}}, 4)
	seen := map[interface{}]int{}
	for _, row := range merged {
		seen[row["id"]]++
	}
	if seen["b"] != 1 {
		t.Fatalf("merged = %#v, want id b once", merged)
	}
}
