package service

import "testing"

// A request for an abstract class must reach the instances typed with its
// descendants: this expansion is the whole point of the declared hierarchy, and
// without it `agent` matches nothing while `person` holds the data.
func TestOntologyStructureFilterExpandsDeclaredDescendants(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "agent"},
			{"type": "person", "parent": "agent"},
			{"type": "organization", "parent": "agent"},
			{"type": "place"},
		},
		nil,
	))

	filter := OntologyStructureFilter(declared, OntologyStructureConstraints{
		EntityTypes: []string{"agent"},
	})

	got, ok := filter["entity_type_kwd"].([]string)
	if !ok {
		t.Fatalf("entity_type_kwd = %#v, want a string slice", filter["entity_type_kwd"])
	}
	assertSameStrings(t, got, []string{"agent", "person", "organization"})
	if kinds, _ := filter["knowledge_graph_kwd"].([]string); len(kinds) != 1 || kinds[0] != "entity" {
		t.Fatalf("knowledge_graph_kwd = %#v, want [entity]", filter["knowledge_graph_kwd"])
	}
}

// A constraint on a property or an endpoint means compiled assertions; without
// pinning the row kind, `prop_kwd` would also match hypernode rows, which carry
// a leaf attribute in the same column.
//
// A request that names BOTH a class and a property has to become two branches,
// not one map: entity_type_kwd lives on entity rows and prop_kwd on assertion
// rows, so a single AND-ed map matched nothing at all (measured: `no_match` plus
// a second, unnarrowed search). Each branch carries its own kind pin.
func TestOntologyStructureFilterPinsRowKinds(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{{"type": "person"}, {"type": "organization"}},
		[]map[string]interface{}{{"type": "member_of", "kind": "object", "domain": "person", "range": "organization"}},
	))

	filter := OntologyStructureFilter(declared, OntologyStructureConstraints{
		EntityTypes: []string{"person"},
		Properties:  []string{"member_of"},
	})

	if _, flat := filter["entity_type_kwd"]; flat {
		t.Fatalf("filter = %#v, want the class clause inside its own branch", filter)
	}
	branches, ok := filter["or"].([]interface{})
	if !ok || len(branches) != 2 {
		t.Fatalf("filter = %#v, want two OR-ed row-kind branches", filter)
	}
	entity, ok := branches[0].(map[string]interface{})
	if !ok {
		t.Fatalf("entity branch = %#v", branches[0])
	}
	if kinds, _ := entity["knowledge_graph_kwd"].([]string); len(kinds) != 1 || kinds[0] != "entity" {
		t.Fatalf("entity branch kind pin = %#v, want [entity]", entity["knowledge_graph_kwd"])
	}
	if classes, _ := entity["entity_type_kwd"].([]string); len(classes) != 1 || classes[0] != "person" {
		t.Fatalf("entity_type_kwd = %#v, want [person]", entity["entity_type_kwd"])
	}
	if _, leaked := entity["prop_kwd"]; leaked {
		t.Fatal("the property clause leaked into the entity branch; the two can never match one row")
	}

	assertions, ok := branches[1].(map[string]interface{})
	if !ok {
		t.Fatalf("assertion branch = %#v", branches[1])
	}
	if kinds, _ := assertions["knowledge_graph_kwd"].([]string); len(kinds) != 1 || kinds[0] != "relation" {
		t.Fatalf("assertion branch kind pin = %#v, want [relation]", assertions["knowledge_graph_kwd"])
	}
	if props, _ := assertions["prop_kwd"].([]string); len(props) != 1 || props[0] != "member_of" {
		t.Fatalf("prop_kwd = %#v, want [member_of]", assertions["prop_kwd"])
	}
}

// A constraint set that is guaranteed to match nothing is relaxed one clause at a
// time, endpoint classes first, and the ladder never repeats a step.
func TestOntologyConstraintLadderRelaxesOneClauseAtATime(t *testing.T) {
	steps := ontologyConstraintLadder(OntologyStructureConstraints{
		EntityTypes: []string{"person"},
		Properties:  []string{"member_of"},
		FromTypes:   []string{"person"},
		ToTypes:     []string{"organization"},
	})
	if len(steps) != 4 {
		t.Fatalf("steps = %#v, want full, no-endpoints, class-only, property-only", steps)
	}
	if ontologyConstraintsDiffer(steps[0], OntologyStructureConstraints{
		EntityTypes: []string{"person"}, Properties: []string{"member_of"},
		FromTypes: []string{"person"}, ToTypes: []string{"organization"},
	}) {
		t.Fatal("the first step must be the request as asked")
	}
	if ontologyConstraintsDiffer(steps[1], OntologyStructureConstraints{
		EntityTypes: []string{"person"}, Properties: []string{"member_of"},
	}) {
		t.Fatalf("second step = %#v, want the endpoint clauses dropped", steps[1])
	}
	if ontologyConstraintsDiffer(steps[2], OntologyStructureConstraints{EntityTypes: []string{"person"}}) {
		t.Fatalf("third step = %#v, want the class half on its own", steps[2])
	}
	if ontologyConstraintsDiffer(steps[3], OntologyStructureConstraints{Properties: []string{"member_of"}}) {
		t.Fatalf("fourth step = %#v, want the property half on its own", steps[3])
	}
}

// Endpoint constraints expand the same way the subject side does, and they are
// assertion constraints, not entity ones.
func TestOntologyStructureFilterExpandsEndpointSides(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "agent"},
			{"type": "person", "parent": "agent"},
		},
		nil,
	))

	filter := OntologyStructureFilter(declared, OntologyStructureConstraints{
		FromTypes: []string{"agent"},
		ToTypes:   []string{"agent"},
	})

	assertSameStrings(t, filter["from_type_kwd"].([]string), []string{"agent", "person"})
	assertSameStrings(t, filter["to_type_kwd"].([]string), []string{"agent", "person"})
	if kinds, _ := filter["knowledge_graph_kwd"].([]string); len(kinds) != 1 || kinds[0] != "relation" {
		t.Fatalf("knowledge_graph_kwd = %#v, want [relation]", filter["knowledge_graph_kwd"])
	}
}

// A class the template does not declare is kept: the caller named it, and a
// filter that matches nothing is more honest than one that silently searches for
// something else.
func TestOntologyStructureFilterKeepsUnknownClass(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{{"type": "person"}},
		nil,
	))

	filter := OntologyStructureFilter(declared, OntologyStructureConstraints{
		EntityTypes: []string{"ghost"},
	})

	assertSameStrings(t, filter["entity_type_kwd"].([]string), []string{"ghost"})
}

// A cyclic parent chain is a template defect the declaration validator rejects,
// but this walk must not hang on a config edited outside the service.
func TestExpandDeclaredClassesTerminatesOnCycle(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "a", "parent": "b"},
			{"type": "b", "parent": "a"},
		},
		nil,
	))

	got := expandDeclaredClasses([]string{"a"}, declaredChildren(declared))
	assertSameStrings(t, got, []string{"a", "b"})
}

// No constraints must leave retrieval exactly as it was.
func TestOntologyStructureFilterIgnoresNothing(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{{"type": "person"}},
		nil,
	))

	if filter := OntologyStructureFilter(declared, OntologyStructureConstraints{}); filter != nil {
		t.Fatalf("filter = %#v, want nil for an empty request", filter)
	}
	if !(OntologyStructureConstraints{}).IsZero() {
		t.Fatal("an empty constraint set must report itself as zero")
	}
}

func assertSameStrings(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
