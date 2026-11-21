package service

import (
	"context"
	"errors"
	"testing"
)

// fakeTraversalScope stands in for the ownership check.
type fakeTraversalScope struct {
	tenants []string
	err     error
	calls   int
}

func (f *fakeTraversalScope) ResolveTenants(_ context.Context, _ string, _ []string) ([]string, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.tenants, nil
}

// fakeTraversalRows answers each direction probe from a fixed row set, which is
// what the walk would get from the engine: one page per direction per dataset.
type fakeTraversalRows struct {
	rows    []map[string]interface{}
	filters []map[string]interface{}
	err     error
}

func (f *fakeTraversalRows) read(
	_ context.Context, _, _ string, filter map[string]interface{}, limit int,
) ([]map[string]interface{}, error) {
	f.filters = append(f.filters, filter)
	if f.err != nil {
		return nil, f.err
	}
	want := map[string]bool{}
	for _, key := range []string{"from_entity_kwd", "to_entity_kwd"} {
		for _, name := range stringSliceOf(filter[key]) {
			want[key+"|"+name] = true
		}
	}
	out := make([]map[string]interface{}, 0, len(f.rows))
	for _, row := range f.rows {
		from := firstStringValue(row["from_entity_kwd"])
		to := firstStringValue(row["to_entity_kwd"])
		if want["from_entity_kwd|"+from] || want["to_entity_kwd|"+to] {
			out = append(out, row)
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func stringSliceOf(raw interface{}) []string {
	switch v := raw.(type) {
	case []string:
		return v
	case string:
		return []string{v}
	}
	return nil
}

// traversalOntology declares the two-hop chain the tests walk: a paper uses a
// model, and a model has a variable.
func traversalOntology() declaredOntology {
	return parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "paper"},
			{"type": "model"},
			{"type": "variable"},
		},
		[]map[string]interface{}{
			{"type": "uses", "kind": "object", "domain": "paper", "range": "model"},
			{"type": "has_variable", "kind": "object", "domain": "model", "range": "variable"},
		},
	))
}

func newTraversalService(declared declaredOntology, reader ontologyTraversalRowReader) (*OntologyTraversalService, *fakeTraversalScope) {
	scope := &fakeTraversalScope{tenants: []string{"tenant-1"}}
	return &OntologyTraversalService{
		scopeResolver: scope,
		structureLoader: func(context.Context, string, string, int) (declaredOntology, OntologyModelRef, error) {
			return declared, OntologyModelRef{ID: "tpl-1", Name: "Papers", Version: 2}, nil
		},
		rowReader: reader,
	}, scope
}

func assertionRow(prop, from, fromType, to, toType string) map[string]interface{} {
	return map[string]interface{}{
		"prop_kwd":         prop,
		"from_entity_kwd":  from,
		"from_type_kwd":    fromType,
		"to_entity_kwd":    to,
		"to_type_kwd":      toType,
		"source_chunk_ids": []interface{}{"chunk-1"},
	}
}

func traversalRequest(starts ...string) *OntologyTraversalRequest {
	return &OntologyTraversalRequest{
		UserID:             "user-1",
		DatasetIDs:         []string{"kb-1"},
		OntologyTemplateID: "tpl-1",
		Starts:             starts,
	}
}

// The whole point of the walk: the second hop starts from an entity the question
// never named, so no similarity ranking would ever have reached it.
func TestOntologyTraversalReachesTheUnnamedMiddleEntity(t *testing.T) {
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("uses", "Paper A", "paper", "Model M", "model"),
		assertionRow("has_variable", "Model M", "model", "Variable V", "variable"),
	}}
	svc, _ := newTraversalService(traversalOntology(), rows.read)

	req := traversalRequest("Paper A")
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(result.Hops) != 2 {
		t.Fatalf("hops = %#v, want two hops", result.Hops)
	}
	if result.Hops[0].Matched == 0 || result.Hops[1].Matched == 0 {
		t.Fatalf("hops = %#v, want both hops to follow an assertion", result.Hops)
	}
	if result.Hops[1].Frontier != 1 {
		t.Fatalf("second frontier = %d, want the model discovered by hop 1", result.Hops[1].Frontier)
	}
	found := false
	for _, path := range result.Paths {
		if len(path.Nodes) == 3 && path.Nodes[0] == "Paper A" &&
			path.Nodes[1] == "Model M" && path.Nodes[2] == "Variable V" {
			found = true
			if len(path.Steps) != 2 || path.Steps[0].Property != "uses" || path.Steps[1].Property != "has_variable" {
				t.Fatalf("steps = %#v, want uses then has_variable", path.Steps)
			}
			if len(path.Steps[0].SourceChunkIDs) != 1 {
				t.Fatalf("step = %#v, want the source chunks carried", path.Steps[0])
			}
		}
	}
	if !found {
		t.Fatalf("paths = %#v, want paper -> model -> variable", result.Paths)
	}
	if result.Model.ID != "tpl-1" || result.Model.Version != 2 {
		t.Fatalf("model = %#v, want the declaration the walk used", result.Model)
	}
}

// A declared range that the assertion contradicts is refused, and the refusal is
// counted: an empty answer caused by a wrong declaration is not the same fact as
// an empty answer caused by a silent corpus.
func TestOntologyTraversalRefusesContradictedEndpoints(t *testing.T) {
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("has_variable", "Model M", "model", "Dataset D", "dataset"),
	}}
	svc, _ := newTraversalService(traversalOntology(), rows.read)

	req := traversalRequest("Model M")
	req.Hops = 1
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(result.Paths) != 0 {
		t.Fatalf("paths = %#v, want none: the to side is not a declared variable", result.Paths)
	}
	if result.Hops[0].RejectedByDeclaration != 1 {
		t.Fatalf("hop = %#v, want the contradiction counted", result.Hops[0])
	}
}

// A subclass satisfies a rule written against its parent, which is what makes the
// declared hierarchy worth writing down.
func TestOntologyTraversalAcceptsDeclaredSubclasses(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "paper"},
			{"type": "model"},
			{"type": "ocean_model", "parent": "model"},
		},
		[]map[string]interface{}{
			{"type": "uses", "kind": "object", "domain": "paper", "range": "ocean_model"},
		},
	))
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("uses", "Paper A", "paper", "Model M", "ocean_model"),
	}}
	svc, _ := newTraversalService(declared, rows.read)

	req := traversalRequest("Paper A")
	req.Hops = 1
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if result.Hops[0].Matched != 1 || len(result.Paths) == 0 {
		t.Fatalf("result = %#v, want the declared subclass accepted", result)
	}
}

// An undeclared property is drift, and a path must not be built out of it. The
// walk refuses it and counts the refusal rather than quietly following a relation
// the model never declared.
func TestOntologyTraversalRefusesUndeclaredProperties(t *testing.T) {
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("mentions", "Paper A", "paper", "Model M", "model"),
	}}
	svc, _ := newTraversalService(traversalOntology(), rows.read)

	req := traversalRequest("Paper A")
	req.Hops = 1
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(result.Paths) != 0 {
		t.Fatalf("paths = %#v, want none for an undeclared property", result.Paths)
	}
	if result.Hops[0].RejectedByDeclaration != 1 {
		t.Fatalf("hop = %#v, want the drift counted", result.Hops[0])
	}
}

// The caller's own restriction is a separate refusal from the model's, because
// the two call for different fixes: widen the caller's request, or fix the
// declaration.
func TestOntologyTraversalCountsAllowListRefusalsSeparately(t *testing.T) {
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("uses", "Paper A", "paper", "Model M", "model"),
		assertionRow("has_variable", "Model M", "model", "Variable V", "variable"),
	}}
	svc, _ := newTraversalService(traversalOntology(), rows.read)

	req := traversalRequest("Paper A")
	req.Hops = 2
	req.AllowProperties = []string{"has_variable"}
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if result.Hops[0].Matched != 0 || result.Hops[0].RejectedByAllowList == 0 {
		t.Fatalf("hop = %#v, want the property restriction to refuse the first hop", result.Hops[0])
	}
	if len(result.Paths) != 0 {
		t.Fatalf("paths = %#v, want none", result.Paths)
	}
}

// A cyclic chain must terminate, and the edge that closes the loop still belongs
// to the path that walks it once.
func TestOntologyTraversalTerminatesOnCycles(t *testing.T) {
	// A symmetric declared property, so the loop is refused by the visited set
	// rather than by the endpoint check: this is the case that would otherwise
	// expand the same two entities until the hop budget ran out.
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{{"type": "paper"}},
		[]map[string]interface{}{
			{"type": "related", "kind": "object", "domain": "paper", "range": "paper"},
		},
	))
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("related", "Paper A", "paper", "Paper B", "paper"),
		assertionRow("related", "Paper B", "paper", "Paper A", "paper"),
	}}
	svc, _ := newTraversalService(declared, rows.read)

	req := traversalRequest("Paper A")
	req.Hops = 3
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(result.Hops) != 2 {
		t.Fatalf("hops = %#v, want the walk to stop once the frontier empties", result.Hops)
	}
	if result.Hops[1].Matched == 0 {
		t.Fatalf("hops = %#v, want the loop-closing assertion still followed", result.Hops)
	}
	for _, path := range result.Paths {
		seen := map[string]bool{}
		for _, node := range path.Nodes {
			if seen[node] {
				t.Fatalf("path = %#v, want no repeated node", path.Nodes)
			}
			seen[node] = true
		}
	}
}

// An unreadable model degrades the walk instead of refusing it: the assertions
// are still there, only the pruning is gone.
func TestOntologyTraversalDegradesWithoutAModel(t *testing.T) {
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("mentions", "Paper A", "paper", "Model M", "model"),
	}}
	svc := &OntologyTraversalService{
		scopeResolver: &fakeTraversalScope{tenants: []string{"tenant-1"}},
		structureLoader: func(context.Context, string, string, int) (declaredOntology, OntologyModelRef, error) {
			return declaredOntology{}, OntologyModelRef{}, errors.New("model not found")
		},
		rowReader: rows.read,
	}

	req := traversalRequest("Paper A")
	req.Hops = 1
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if result.DegradeCode != OntologyABoxDegradeNoModel || result.DegradeReason == "" {
		t.Fatalf("result = %#v, want a no_model degrade", result)
	}
	if result.Hops[0].Matched != 1 {
		t.Fatalf("hop = %#v, want the row followed without a model to check it", result.Hops[0])
	}
}

func TestOntologyTraversalValidatesItsInput(t *testing.T) {
	svc, _ := newTraversalService(traversalOntology(), (&fakeTraversalRows{}).read)

	for name, mutate := range map[string]func(*OntologyTraversalRequest){
		"no caller":   func(r *OntologyTraversalRequest) { r.UserID = "" },
		"no datasets": func(r *OntologyTraversalRequest) { r.DatasetIDs = nil },
		"no start":    func(r *OntologyTraversalRequest) { r.Starts = nil },
		"blank start": func(r *OntologyTraversalRequest) { r.Starts = []string{"  "} },
	} {
		req := traversalRequest("Paper A")
		mutate(req)
		if _, err := svc.Traverse(context.Background(), req); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
	}
}

// Hops are clamped rather than refused: a caller asking for ten gets the cap and
// the truncation flag, so the budget is visible in the result.
func TestOntologyTraversalClampsHops(t *testing.T) {
	rows := &fakeTraversalRows{rows: []map[string]interface{}{
		assertionRow("uses", "Paper A", "paper", "Model M", "model"),
		assertionRow("has_variable", "Model M", "model", "Variable V", "variable"),
		assertionRow("has_variable", "Variable V", "variable", "Variable W", "variable"),
		assertionRow("has_variable", "Variable W", "variable", "Variable X", "variable"),
	}}
	svc, _ := newTraversalService(traversalOntology(), rows.read)

	req := traversalRequest("Paper A")
	req.Hops = 10
	result, err := svc.Traverse(context.Background(), req)
	if err != nil {
		t.Fatalf("traverse: %v", err)
	}
	if len(result.Hops) > OntologyTraversalMaxHops {
		t.Fatalf("hops = %d, want at most the cap", len(result.Hops))
	}
}
