package service

import (
	"reflect"
	"testing"
)

// TestApplyReviewStatusFilter_UnreviewedIsNegative pins the one filter shape the
// queue depends on: "unreviewed" must be expressed as "not accepted and not
// rejected", because that is the only form that treats a missing column (rows
// compiled before review existed), an empty one (Infinity's default) and an
// explicit reset as the same state. A positive term on "" would silently drop the
// rows whose column is absent.
func TestApplyReviewStatusFilter_UnreviewedIsNegative(t *testing.T) {
	filter := map[string]interface{}{}
	applyReviewStatusFilter(filter, AssertionReviewUnreviewed)
	if _, ok := filter["review_status_kwd"]; ok {
		t.Errorf("unreviewed must not be a positive term: %v", filter)
	}
	exclusion, ok := filter["must_not"].(map[string]interface{})
	if !ok {
		t.Fatalf("unreviewed must exclude the decided states: %v", filter)
	}
	if got := stringSliceValue(exclusion["review_status_kwd"]); !reflect.DeepEqual(got, []string{"accepted", "rejected"}) {
		t.Errorf("exclusion = %v, want [accepted rejected]", got)
	}

	accepted := map[string]interface{}{}
	applyReviewStatusFilter(accepted, AssertionReviewAccepted)
	if got := stringSliceValue(accepted["review_status_kwd"]); !reflect.DeepEqual(got, []string{"accepted"}) {
		t.Errorf("accepted filter = %v", accepted)
	}

	all := map[string]interface{}{}
	applyReviewStatusFilter(all, AssertionReviewAll)
	if len(all) != 0 {
		t.Errorf("all must not narrow anything: %v", all)
	}
}

func TestNormalizeAssertionReviewStatusAndDecision(t *testing.T) {
	for raw, want := range map[string]AssertionReviewStatus{
		"":             AssertionReviewUnreviewed,
		"UNREVIEWED":   AssertionReviewUnreviewed,
		"nonsense":     AssertionReviewUnreviewed,
		"accepted":     AssertionReviewAccepted,
		"rejected":     AssertionReviewRejected,
		"all":          AssertionReviewAll,
		"any":          AssertionReviewAll,
		"  accepted  ": AssertionReviewAccepted,
	} {
		if got := NormalizeAssertionReviewStatus(raw); got != want {
			t.Errorf("NormalizeAssertionReviewStatus(%q) = %q, want %q", raw, got, want)
		}
	}
	for raw, want := range map[string]AssertionReviewStatus{
		"accept":  AssertionReviewAccepted,
		"approve": AssertionReviewAccepted,
		"reject":  AssertionReviewRejected,
		"reset":   AssertionReviewUnreviewed,
		"clear":   AssertionReviewUnreviewed,
		"":        "",
		"delete":  "",
	} {
		if got := NormalizeAssertionReviewDecision(raw); got != want {
			t.Errorf("NormalizeAssertionReviewDecision(%q) = %q, want %q", raw, got, want)
		}
	}
}

// TestReviewKindFilter_DoesNotMutateBase guards the fan-out: the same base filter
// feeds the entity and the relation query, so pinning one kind must not leak into
// the other.
func TestReviewKindFilter_DoesNotMutateBase(t *testing.T) {
	base := map[string]interface{}{"must_not": map[string]interface{}{"review_status_kwd": []string{"accepted", "rejected"}}}
	entity := reviewKindFilter(base, "entity")
	relation := reviewKindFilter(base, "relation")
	if _, ok := base["knowledge_graph_kwd"]; ok {
		t.Fatalf("base filter was mutated: %v", base)
	}
	if got := stringSliceValue(entity["knowledge_graph_kwd"]); !reflect.DeepEqual(got, []string{"entity"}) {
		t.Errorf("entity kind = %v", got)
	}
	if got := stringSliceValue(relation["knowledge_graph_kwd"]); !reflect.DeepEqual(got, []string{"relation"}) {
		t.Errorf("relation kind = %v", got)
	}
}

// TestMarkRelationConflicts_OnlyWhenTargetsDisagree covers the mechanical conflict
// signal: one property of one subject pointing at two different objects. A single
// target is not a conflict, and the flag never replaces a row.
func TestMarkRelationConflicts_OnlyWhenTargetsDisagree(t *testing.T) {
	agreeing := []StructureGraphRelation{
		{"from": "Engine", "property": "located_in", "to": "Plant A"},
		{"from": "Engine", "property": "located_in", "to": "plant a"},
	}
	markRelationConflicts(agreeing)
	for _, edge := range agreeing {
		if edge["conflict"] == true {
			t.Errorf("same target in two spellings is not a conflict: %v", edge)
		}
	}

	conflicting := []StructureGraphRelation{
		{"from": "Engine", "property": "located_in", "to": "Plant A"},
		{"from": "engine", "property": "located_in", "to": "Plant B"},
		{"from": "Engine", "property": "owned_by", "to": "Plant A"},
	}
	markRelationConflicts(conflicting)
	if conflicting[0]["conflict"] != true || conflicting[1]["conflict"] != true {
		t.Errorf("disagreeing targets must both be flagged: %v", conflicting)
	}
	if conflicting[2]["conflict"] == true {
		t.Errorf("a different property is not the same disagreement: %v", conflicting[2])
	}
	if got := stringSliceValue(conflicting[0]["conflict_targets"]); !reflect.DeepEqual(got, []string{"Plant A", "Plant B"}) {
		t.Errorf("conflict targets = %v, want both spelled targets", got)
	}
}

// TestMarkRelationConflicts_FallsBackToThePayloadType covers rows whose property
// column is absent: the assertion is still the same assertion, so the payload's
// type keeps it comparable instead of silently dropping it from conflict
// detection.
func TestMarkRelationConflicts_FallsBackToThePayloadType(t *testing.T) {
	edges := []StructureGraphRelation{
		{"from": "A", "type": "causes", "to": "B"},
		{"from": "a", "type": "causes", "to": "C"},
	}
	markRelationConflicts(edges)
	if edges[0]["conflict"] != true || edges[1]["conflict"] != true {
		t.Errorf("payload type must identify the property: %v", edges)
	}
}

// TestMarkEntityClassConflicts covers the homonym signal: one name claimed by two
// classes. The instances stay separate rows (the fold keys on the class); the flag
// only tells the reader they disagree.
func TestMarkEntityClassConflicts(t *testing.T) {
	entities := []StructureGraphNode{
		{"name": "Apple", "type": "company"},
		{"name": "apple", "type": "fruit"},
		{"name": "Banana", "type": "fruit"},
	}
	markEntityClassConflicts(entities)
	if entities[0]["conflict"] != true || entities[1]["conflict"] != true {
		t.Errorf("homonyms must both be flagged: %v", entities)
	}
	if entities[2]["conflict"] == true {
		t.Errorf("one class is not a conflict: %v", entities[2])
	}
	if got := stringSliceValue(entities[0]["conflict_types"]); !reflect.DeepEqual(got, []string{"company", "fruit"}) {
		t.Errorf("conflict types = %v", got)
	}
}

// TestSortReviewItems_ConflictsThenSubstance covers the reading order: what needs
// a decision first, then the assertions with more behind them, with a stable key
// so paging does not reshuffle rows.
func TestSortReviewItems_ConflictsThenSubstance(t *testing.T) {
	items := []ModelAssertionReviewItem{
		{ID: "c", Kind: "entity", Name: "Faint", MentionCount: 2},
		{ID: "a", Kind: "entity", Name: "Disputed", MentionCount: 1, Conflict: true},
		{ID: "b", Kind: "entity", Name: "Substantial", MentionCount: 9},
	}
	sortReviewItems(items)
	if items[0].ID != "a" {
		t.Errorf("conflicts must come first, even against a higher mention count: %v", items)
	}
	if items[1].ID != "b" || items[2].ID != "c" {
		t.Errorf("unflagged rows must follow by substance: %v", items)
	}

	ties := []ModelAssertionReviewItem{
		{ID: "z", Kind: "entity", Name: "B", MentionCount: 1},
		{ID: "y", Kind: "entity", Name: "A", MentionCount: 1},
	}
	sortReviewItems(ties)
	if ties[0].Name != "A" || ties[1].Name != "B" {
		t.Errorf("equal rows must have a stable order: %v", ties)
	}
}

// TestReviewItemFromEntity_CarriesIdentityAndDecision locks the fields the review
// UI needs to act: the row id (a decision writes one row), the dataset it belongs
// to, the scope (doc vs dataset rows are reviewed separately) and the recorded
// decision.
func TestReviewItemFromEntity_CarriesIdentityAndDecision(t *testing.T) {
	node := StructureGraphNode{
		"id": "row-1", "dataset_id": "kb-1", "dataset_name": "Manuals", "scope": "dataset",
		"name": "Engine", "type": "component", "description": "a part", "mention_count": 3,
		"version": 7, "source_chunk_ids": []string{"c1"}, "document_ids": []string{"d1"},
		"review_status": "accepted", "review_by": "u1", "review_at": float64(1700000000000),
		"review_note": "checked", "conflict": true, "conflict_types": []string{"component", "pump"},
		"attributes":   map[string]interface{}{"birth_date": "1990-01-01"},
		"evidence":     []map[string]interface{}{{"quote": "the engine", "chunk_id": "c1"}},
		"unused_field": "ignored",
	}
	item := reviewItemFromEntity(node)
	if item.ID != "row-1" || item.DatasetID != "kb-1" || item.DatasetName != "Manuals" {
		t.Fatalf("row identity was lost: %+v", item)
	}
	if item.Kind != "entity" || item.Scope != "dataset" {
		t.Errorf("kind/scope = %q/%q", item.Kind, item.Scope)
	}
	if item.MentionCount != 3 || item.Version != 7 {
		t.Errorf("counts = %+v", item)
	}
	if item.ReviewStatus != "accepted" || item.ReviewBy != "u1" || item.ReviewAt != 1700000000000 || item.ReviewNote != "checked" {
		t.Errorf("decision was lost: %+v", item)
	}
	if !item.Conflict || !reflect.DeepEqual(item.ConflictTypes, []string{"component", "pump"}) {
		t.Errorf("conflict was lost: %+v", item)
	}
	if len(item.Evidence) != 1 || len(item.SourceChunkIDs) != 1 || len(item.DocumentIDs) != 1 {
		t.Errorf("evidence/ids were lost: %+v", item)
	}
	if item.Attributes["birth_date"] != "1990-01-01" {
		t.Errorf("attributes were lost: %+v", item.Attributes)
	}
}

// TestReviewItemFromRelation_CarriesGraphPlacement covers the relation side: an
// assertion must report the classes that place it on the ontology graph, otherwise
// the reviewer cannot tell a domain/range mistake from a wrong object.
func TestReviewItemFromRelation_CarriesGraphPlacement(t *testing.T) {
	edge := StructureGraphRelation{
		"id": "row-2", "dataset_id": "kb-1", "scope": "doc",
		"from": "Engine", "to": "Plant A", "property": "located_in",
		"source_type": "component", "target_type": "site", "type": "located_in",
		"conflict": true, "conflict_targets": []string{"Plant A", "Plant B"},
	}
	item := reviewItemFromRelation(edge)
	if item.Kind != "relation" || item.Scope != "doc" {
		t.Errorf("kind/scope = %q/%q", item.Kind, item.Scope)
	}
	if item.Property != "located_in" || item.SourceType != "component" || item.TargetType != "site" {
		t.Errorf("graph placement was lost: %+v", item)
	}
	if !reflect.DeepEqual(item.ConflictTargets, []string{"Plant A", "Plant B"}) {
		t.Errorf("conflict targets were lost: %+v", item)
	}
}
