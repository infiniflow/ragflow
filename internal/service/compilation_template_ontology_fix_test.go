package service

import (
	"strings"
	"testing"
)

// ontologyFixConfig is a template with one object property declared on two
// endpoints, the shape the ledger's "widen" edit works against.
func ontologyFixConfig() map[string]any {
	return map[string]any{
		"base_uri": "http://example.org/tv#",
		"entity": map[string]any{
			"description": "classes",
			"fields": []any{
				map[string]any{"type": "event"},
				map[string]any{"type": "work"},
				map[string]any{"type": "place"},
			},
		},
		"relation": map[string]any{
			"description": "properties",
			"fields": []any{
				map[string]any{
					"type": "part_of", "kind": "object",
					"domain": "event", "range": "place|event",
					"label": "Part of",
				},
				map[string]any{"type": "is_a", "kind": "object", "domain": "concept|taxon", "range": "concept"},
			},
		},
	}
}

func relationField(t *testing.T, config map[string]any, property string) map[string]any {
	t.Helper()
	for _, field := range relationFields(config) {
		if stringFieldValue(field, "type") == property {
			return field
		}
	}
	t.Fatalf("field %s is missing from %v", property, config)
	return nil
}

// The ledger's most common row: one property whose declared range did not admit
// a class the data actually uses. The edit is to admit it, in place.
func TestApplyOntologyFixWidensTheDeclaredSide(t *testing.T) {
	original := ontologyFixConfig()
	patched, err := ApplyOntologyFixToConfig(original, OntologyFixRequest{
		Op: ontologyFixWiden, Property: "part_of", Side: "range", Class: "work",
	})
	if err != nil {
		t.Fatalf("widen: %v", err)
	}
	if got := stringFieldValue(relationField(t, patched, "part_of"), "range"); got != "place|event|work" {
		t.Fatalf("range = %q, want place|event|work", got)
	}
	// Other declarations and the rest of the field are untouched.
	if got := stringFieldValue(relationField(t, patched, "part_of"), "label"); got != "Part of" {
		t.Fatalf("label = %q, want it preserved", got)
	}
	if got := stringFieldValue(relationField(t, patched, "is_a"), "domain"); got != "concept|taxon" {
		t.Fatalf("is_a domain = %q, want it untouched", got)
	}
	if got := stringFieldValue(relationField(t, original, "part_of"), "range"); got != "place|event" {
		t.Fatalf("the input config was mutated: range = %q", got)
	}
}

// Confirming the same fix twice must be a no-op rather than a second entry: the
// editor renders these values verbatim, so "place|event|work|work" reads as two
// declarations of the same target.
func TestApplyOntologyFixWidenIsIdempotent(t *testing.T) {
	patched, err := ApplyOntologyFixToConfig(ontologyFixConfig(), OntologyFixRequest{
		Op: ontologyFixWiden, Property: "part_of", Side: "range", Class: "event",
	})
	if err != nil {
		t.Fatalf("widen: %v", err)
	}
	if got := stringFieldValue(relationField(t, patched, "part_of"), "range"); got != "place|event" {
		t.Fatalf("range = %q, want the duplicate refused", got)
	}
}

// A widened domain lands on the domain side only.
func TestApplyOntologyFixWidenDomainSide(t *testing.T) {
	patched, err := ApplyOntologyFixToConfig(ontologyFixConfig(), OntologyFixRequest{
		Op: ontologyFixWiden, Property: "is_a", Side: "domain", Class: "work",
	})
	if err != nil {
		t.Fatalf("widen: %v", err)
	}
	if got := stringFieldValue(relationField(t, patched, "is_a"), "domain"); got != "concept|taxon|work" {
		t.Fatalf("domain = %q", got)
	}
}

// The other ledger row: a property the data carries that the template never
// declared. Only the endpoints the reader knows are written.
func TestApplyOntologyFixDeclaresAnObservedProperty(t *testing.T) {
	original := ontologyFixConfig()
	patched, err := ApplyOntologyFixToConfig(original, OntologyFixRequest{
		Op: ontologyFixDeclare, Property: "genre", Domain: "work", Range: "concept",
	})
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	declared := relationField(t, patched, "genre")
	if stringFieldValue(declared, "kind") != "object" {
		t.Fatalf("kind = %q, want object when both endpoints are classes", stringFieldValue(declared, "kind"))
	}
	if stringFieldValue(declared, "domain") != "work" || stringFieldValue(declared, "range") != "concept" {
		t.Fatalf("declared endpoints = %v", declared)
	}
	if len(relationFields(original)) != 2 {
		t.Fatalf("the input config gained a field: %d", len(relationFields(original)))
	}
	if len(relationFields(patched)) != 3 {
		t.Fatalf("patched fields = %d, want 3", len(relationFields(patched)))
	}
}

// A datatype declaration carries kind: datatype instead of a class range.
func TestApplyOntologyFixDeclaresDatatypeProperty(t *testing.T) {
	patched, err := ApplyOntologyFixToConfig(ontologyFixConfig(), OntologyFixRequest{
		Op: ontologyFixDeclare, Property: "release_year", Domain: "work", Datatype: "int",
	})
	if err != nil {
		t.Fatalf("declare: %v", err)
	}
	declared := relationField(t, patched, "release_year")
	if stringFieldValue(declared, "kind") != "datatype" || stringFieldValue(declared, "datatype") != "int" {
		t.Fatalf("declared = %v", declared)
	}
	if _, ok := declared["range"]; ok {
		t.Fatalf("a datatype property must not carry a class range: %v", declared)
	}
}

// A reader selects several lines and applies them together: every edit lands,
// and a later edit sees what an earlier one changed.
func TestApplyOntologyFixesToConfigAppliesEveryEdit(t *testing.T) {
	patched, err := ApplyOntologyFixesToConfig(ontologyFixConfig(), []OntologyFixRequest{
		{Op: ontologyFixWiden, Property: "part_of", Side: "range", Class: "work"},
		{Op: ontologyFixWiden, Property: "part_of", Side: "range", Class: "season"},
		{Op: ontologyFixDeclare, Property: "genre", Domain: "work", Range: "concept"},
	})
	if err != nil {
		t.Fatalf("batch: %v", err)
	}
	if got := stringFieldValue(relationField(t, patched, "part_of"), "range"); got != "place|event|work|season" {
		t.Fatalf("range = %q, want both selected classes appended in order", got)
	}
	if len(relationFields(patched)) != 3 {
		t.Fatalf("fields = %d, want the declared property added too", len(relationFields(patched)))
	}
}

// All-or-nothing: one edit the template cannot take fails the batch, and the
// config the caller handed in is untouched, so nothing half-applied can be
// saved and then compiled against.
func TestApplyOntologyFixesToConfigIsAllOrNothing(t *testing.T) {
	original := ontologyFixConfig()
	patched, err := ApplyOntologyFixesToConfig(original, []OntologyFixRequest{
		{Op: ontologyFixDeclare, Property: "genre", Domain: "work", Range: "concept"},
		{Op: ontologyFixWiden, Property: "invented", Side: "range", Class: "work"},
	})
	if err == nil {
		t.Fatal("expected the batch to fail on the second edit")
	}
	patched = nil
	_ = patched
	if len(relationFields(original)) != 2 {
		t.Fatalf("the input config gained %d field(s) from a failed batch", len(relationFields(original))-2)
	}
	if !strings.Contains(err.Error(), "edit 2") || !strings.Contains(err.Error(), "invented") {
		t.Fatalf("err = %v, want it to name the offending edit", err)
	}
}

// An empty selection is a client bug, not a silent no-op write.
func TestApplyOntologyFixesToConfigRefusesEmptySelection(t *testing.T) {
	if _, err := ApplyOntologyFixesToConfig(ontologyFixConfig(), nil); err == nil {
		t.Fatal("expected an error for an empty selection")
	}
}

// Every refusal the ledger can run into, so a client cannot talk the template
// into a rewrite: unknown operation, unknown property, missing class, and a
// property that is already declared.
func TestApplyOntologyFixRefusesBadEdits(t *testing.T) {
	cases := map[string]OntologyFixRequest{
		"unsupported op":        {Op: "rewrite", Property: "part_of"},
		"no property":           {Op: ontologyFixWiden, Side: "range", Class: "work"},
		"no side":               {Op: ontologyFixWiden, Property: "part_of", Class: "work"},
		"bad side":              {Op: ontologyFixWiden, Property: "part_of", Side: "both", Class: "work"},
		"no class":              {Op: ontologyFixWiden, Property: "part_of", Side: "range"},
		"undeclared property":   {Op: ontologyFixWiden, Property: "invented", Side: "range", Class: "work"},
		"declare with no edges": {Op: ontologyFixDeclare, Property: "genre"},
		"declare twice":         {Op: ontologyFixDeclare, Property: "part_of", Domain: "event"},
	}
	for name, fix := range cases {
		if _, err := ApplyOntologyFixToConfig(ontologyFixConfig(), fix); err == nil {
			t.Errorf("%s: expected an error, got none", name)
		}
	}
}

// A config without a relation section cannot be edited by widening; the message
// says so rather than panicking on a nil section.
func TestApplyOntologyFixWithoutRelationSection(t *testing.T) {
	_, err := ApplyOntologyFixToConfig(map[string]any{"entity": map[string]any{"fields": []any{}}},
		OntologyFixRequest{Op: ontologyFixWiden, Property: "part_of", Side: "range", Class: "work"})
	if err == nil || !strings.Contains(err.Error(), "relation section") {
		t.Fatalf("err = %v, want a relation-section complaint", err)
	}
}
