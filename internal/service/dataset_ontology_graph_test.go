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

package service

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// ontologyTestConfig builds the template config shape parseDeclaredOntology reads.
func ontologyTestConfig(classes, properties []map[string]interface{}) map[string]interface{} {
	fields := make([]interface{}, 0, len(classes))
	for _, c := range classes {
		fields = append(fields, c)
	}
	propFields := make([]interface{}, 0, len(properties))
	for _, p := range properties {
		propFields = append(propFields, p)
	}
	return map[string]interface{}{
		"entity":   map[string]interface{}{"fields": fields},
		"relation": map[string]interface{}{"fields": propFields},
	}
}

// The agent/person split of the shipped template: a datatype attribute declared
// on the superclass has to reach the subclass, and an object property has to stay
// an edge rather than become an attribute.
func TestEffectiveAttributesResolveInheritance(t *testing.T) {
	cfg := ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "agent", "label": "Agent", "description": "a thing that acts"},
			{"type": "person", "label": "Person", "description": "a human", "parent": "agent"},
		},
		[]map[string]interface{}{
			{"type": "alias", "kind": "datatype", "datatype": "list", "domain": "agent", "label": "alias", "description": "other names"},
			{"type": "birth_date", "kind": "datatype", "datatype": "date", "domain": "person", "description": "when born"},
			{"type": "born_in", "kind": "object", "domain": "person", "range": "place", "description": "birthplace"},
		},
	)
	declared := parseDeclaredOntology(cfg)

	// person declares birth_date; alias is inherited from agent. The object
	// property is an edge, so it must not appear here.
	attrs := declared.effectiveAttributes("person", 10, attributeCounts{"person": {"birth_date": 7, "alias": 3}})
	byName := map[string]OntologyAttribute{}
	for _, a := range attrs {
		byName[a.Type] = a
	}
	if len(attrs) != 2 {
		t.Fatalf("person attributes = %v, want birth_date + alias", attrs)
	}
	if got := byName["birth_date"]; got.InheritedFrom != "" || got.Datatype != "date" || got.Assertions != 7 {
		t.Errorf("birth_date = %+v, want own/date/7", got)
	}
	// The inherited attribute reports its declarer, and its count is the count on
	// the class being described (a person's alias lives on the person row), not
	// on the abstract declarer.
	if got := byName["alias"]; got.InheritedFrom != "agent" || got.Datatype != "list" || got.Assertions != 3 {
		t.Errorf("alias = %+v, want inherited_from=agent/list/3", got)
	}
	if _, ok := byName["born_in"]; ok {
		t.Error("an object property became a node attribute")
	}

	// agent has no parent, so its own attribute is not marked inherited.
	agentAttrs := declared.effectiveAttributes("agent", 0, nil)
	if len(agentAttrs) != 1 || agentAttrs[0].Type != "alias" || agentAttrs[0].InheritedFrom != "" {
		t.Fatalf("agent attributes = %+v, want [alias own]", agentAttrs)
	}
}

// `required` is an acceptance rule, so it is judged on the stored rows: the graph
// reports how many instances lack the value. The projection never invents it and
// never drops the row, because a missing value is a fact about the document or
// about the extraction — only the reader can tell which.
func TestEffectiveAttributesReportMissingRequiredValues(t *testing.T) {
	cfg := ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "agent"},
			{"type": "person", "parent": "agent"},
		},
		[]map[string]interface{}{
			{"type": "alias", "kind": "datatype", "datatype": "list", "domain": "agent", "required": true},
			{"type": "birth_date", "kind": "datatype", "datatype": "date", "domain": "person"},
		},
	)
	declared := parseDeclaredOntology(cfg)

	attrs := declared.effectiveAttributes("person", 5, attributeCounts{"person": {"alias": 2, "birth_date": 5}})
	byName := map[string]OntologyAttribute{}
	for _, a := range attrs {
		byName[a.Type] = a
	}
	if got := byName["alias"]; !got.Required || got.Assertions != 2 || got.Missing != 3 {
		t.Errorf("alias = %+v, want required with 2 of 5 carried, 3 missing", got)
	}
	// An attribute nobody declared required reports no gap, even when it is not
	// universally filled: without the declaration there is no threshold to judge.
	if got := byName["birth_date"]; got.Required || got.Missing != 0 {
		t.Errorf("birth_date = %+v, want not required and no gap", got)
	}

	// A tally larger than the instance count — a truncated scan, or rows written
	// before the declaration existed — must not produce a negative gap.
	if got := declared.effectiveAttributes("agent", 1, attributeCounts{"agent": {"alias": 9}}); got[0].Missing != 0 {
		t.Errorf("agent alias missing = %d, want 0 (clamped)", got[0].Missing)
	}
}

func TestBuildOntologyQualitySumsMissingRequiredValues(t *testing.T) {
	graph := &OntologyGraph{Classes: []OntologyClassNode{
		{Type: "person", Declared: true, Entities: 5, Attributes: []OntologyAttribute{
			{Type: "alias", Required: true, Assertions: 2, Missing: 3},
		}},
		{Type: "agent", Declared: true, Entities: 1, Attributes: []OntologyAttribute{
			{Type: "alias", Required: true, Assertions: 1},
		}},
	}}

	quality := buildOntologyQuality(graph, 0, 0, nil, false)
	if quality.RequiredValuesMissing != 3 {
		t.Fatalf("required_values_missing = %d, want 3", quality.RequiredValuesMissing)
	}
}

// A config edited outside the service could hold a parent cycle; the template
// validator rejects it, but the walk must still terminate rather than hang the
// API (it is reachable from a read path).
func TestAncestorChainTerminatesOnCycle(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "a", "parent": "b"},
			{"type": "b", "parent": "a"},
		},
		nil,
	))
	chain := declared.ancestorChain("a")
	if len(chain) != 1 || chain[0] != "b" {
		t.Fatalf("ancestorChain(a) = %v, want [b]", chain)
	}
}

// A diamond must report the shared ancestor once.
func TestAncestorChainDeduplicatesDiamond(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "root"},
			{"type": "left", "parent": "root"},
			{"type": "right", "parent": "root"},
			{"type": "leaf", "parent": "left|right"},
		},
		nil,
	))
	chain := declared.ancestorChain("leaf")
	if len(chain) != 3 {
		t.Fatalf("ancestorChain(leaf) = %v, want left, right, root once each", chain)
	}
	seen := map[string]int{}
	for _, c := range chain {
		seen[c]++
	}
	if seen["root"] != 1 {
		t.Fatalf("shared ancestor reported %d times, want 1 (%v)", seen["root"], chain)
	}
}

func TestBuildOntologyPitfalls(t *testing.T) {
	cfg := ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "person", "parent": "ghost"}, // parent not declared
			{"type": "place"},
			{"type": "concept"}, // no property at all, no instances
			{"type": "event"},   // no property at all, but has instances
		},
		[]map[string]interface{}{
			{"type": "born_in", "kind": "object", "domain": "person", "range": "nowhere"},
			{"type": "located_in", "kind": "object", "domain": "place", "range": "place"},
		},
	)
	declared := parseDeclaredOntology(cfg)
	classCounts := map[string]int{"person": 3, "place": 2, "event": 1}

	found := map[string]OntologyPitfall{}
	for _, p := range buildOntologyPitfalls(declared, classCounts) {
		found[p.Code] = p
	}

	if p, ok := found["dangling_property"]; !ok {
		t.Error("missing dangling_property for range=nowhere")
	} else if p.Severity != "error" || !containsStr(p.Subjects, "born_in") {
		t.Errorf("dangling_property = %+v", p)
	}
	if p, ok := found["dangling_parent"]; !ok {
		t.Error("missing dangling_parent for parent=ghost")
	} else if !strings.Contains(p.Message, "ghost") {
		t.Errorf("dangling_parent message = %q", p.Message)
	}
	// concept and event have no property; only concept also has no instances, so
	// only concept is additionally an orphan.
	if p, ok := found["class_without_property"]; !ok {
		t.Error("missing class_without_property")
	} else if strings.Join(p.Subjects, ",") != "concept,event" {
		t.Errorf("class_without_property subjects = %v, want concept,event", p.Subjects)
	}
	if p, ok := found["orphan_class"]; !ok {
		t.Error("missing orphan_class")
	} else if strings.Join(p.Subjects, ",") != "concept" {
		t.Errorf("orphan_class subjects = %v, want concept", p.Subjects)
	}
}

// A healthy template reports no findings.
func TestBuildOntologyPitfallsClean(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "person"},
			{"type": "place"},
		},
		[]map[string]interface{}{
			{"type": "born_in", "kind": "object", "domain": "person", "range": "place"},
			{"type": "area", "kind": "datatype", "datatype": "float", "domain": "place"},
		},
	))
	if got := buildOntologyPitfalls(declared, map[string]int{"person": 1, "place": 1}); len(got) != 0 {
		t.Fatalf("pitfalls = %+v, want none", got)
	}
}

// The checks added for parity with OntoBricks' detector. One template trips one
// of each kind, so a check that stops firing is visible here rather than in a
// panel nobody has open.
func TestBuildOntologyPitfallsStructureNamingAndSemantics(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "agent"},
			{"type": "person", "parent": "agent"},          // single child of agent
			{"type": "employee", "parent": "person|agent"}, // agent is redundant
			{"type": "thing"},                              // too generic to steer by
			{"type": "place"},
		},
		[]map[string]interface{}{
			// endpoint and its ancestor declared together
			{"type": "born_in", "kind": "object", "domain": "person|agent", "range": "place"},
			// name repeats its range class
			{"type": "has_place", "kind": "object", "domain": "person", "range": "place"},
			// name repeats its domain class
			{"type": "person_alias", "kind": "datatype", "datatype": "string", "domain": "person"},
			// standard vocabulary
			{"type": "label", "kind": "datatype", "datatype": "string", "domain": "place"},
			// datatype that names a declared class: meant to be an object property
			{"type": "area", "kind": "datatype", "datatype": "place", "domain": "agent"},
			// name used by a class as well
			{"type": "place", "kind": "datatype", "datatype": "string", "domain": "agent"},
			// nested name + nested domain: born_in is a narrower has_born_in
			{"type": "has_born_place", "kind": "object", "domain": "person", "range": "place"},
		},
	))
	found := map[string]OntologyPitfall{}
	for _, p := range buildOntologyPitfalls(declared, map[string]int{"person": 1, "place": 1}) {
		found[p.Code] = p
	}

	for code, category := range map[string]string{
		"redundant_parent":              pitfallLogical,
		"single_child_parent":           pitfallStructural,
		"endpoint_ancestor_expansion":   pitfallStructural,
		"property_name_hierarchy":       pitfallStructural,
		"standard_vocabulary_property":  pitfallNaming,
		"range_in_property_name":        pitfallNaming,
		"domain_in_property_name":       pitfallNaming,
		"overly_generic_class":          pitfallSemantic,
		"class_property_name_collision": pitfallSemantic,
		"datatype_is_class":             pitfallSemantic,
	} {
		p, ok := found[code]
		if !ok {
			t.Errorf("missing %s (all: %v)", code, keysOfPitfalls(found))
			continue
		}
		if p.Category != category {
			t.Errorf("%s category = %q, want %q", code, p.Category, category)
		}
		if len(p.Subjects) == 0 || p.Message == "" {
			t.Errorf("%s carries no message/subjects: %+v", code, p)
		}
	}
	if p := found["datatype_is_class"]; !containsStr(p.Subjects, "area declares datatype place, which is a class") {
		t.Errorf("datatype_is_class should name the property and the class: %v", p.Subjects)
	}
}

func keysOfPitfalls(found map[string]OntologyPitfall) []string {
	out := make([]string, 0, len(found))
	for code := range found {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}

// Datatype properties must not become edges: an edge needs a class as its range,
// and a viewer draws the graph from the edge list alone.
func TestMergeOntologyPropertiesKeepsOnlyObjectProperties(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{{"type": "person"}, {"type": "place"}},
		[]map[string]interface{}{
			{"type": "born_in", "kind": "object", "domain": "person", "range": "place", "label": "born in"},
			{"type": "birth_date", "kind": "datatype", "datatype": "date", "domain": "person"},
			{"type": "unspecified", "description": "no endpoints declared"},
		},
	))
	edges := mergeOntologyProperties(declared, map[string]int{ontologyEdgeKey("born_in", "person", "place"): 5})
	if len(edges) != 1 {
		t.Fatalf("edges = %+v, want only born_in", edges)
	}
	if edges[0].Type != "born_in" || edges[0].Label != "born in" || edges[0].Relations != 5 || !edges[0].Declared {
		t.Fatalf("edge = %+v", edges[0])
	}
}

// A name the template declares, observed with an endpoint pair it does not
// declare, is NOT an undeclared property. The writer refuses to declare a name
// the template already has, so the ledger has to tell the two apart and offer
// the edit that can actually be applied — widening the side left out.
func TestMergeOntologyPropertiesSeparatesADeclaredNameFromAnUndeclaredPair(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{{"type": "event"}, {"type": "place"}, {"type": "work"}},
		[]map[string]interface{}{
			{"type": "part_of", "kind": "object", "domain": "event", "range": "place"},
		},
	))
	edges := mergeOntologyProperties(declared, map[string]int{
		ontologyEdgeKey("part_of", "event", "place"): 5,
		ontologyEdgeKey("part_of", "event", "work"):  2,
		ontologyEdgeKey("genre", "work", "place"):    1,
	})
	byKey := map[string]OntologyPropertyEdge{}
	for _, edge := range edges {
		byKey[edge.Type+"|"+edge.Source+"|"+edge.Target] = edge
	}
	if got := byKey["part_of|event|work"]; got.Declared || !got.DeclaredName {
		t.Fatalf("part_of event->work = %+v, want declared=false and declared_name=true", got)
	}
	if got := byKey["genre|work|place"]; got.Declared || got.DeclaredName {
		t.Fatalf("genre work->place = %+v, want a genuinely undeclared property", got)
	}
}

// The counter above the list has to agree with the list: a declared name with an
// undeclared pair is not counted as an undeclared property, or the ledger would
// show a number the reader can never bring down.
func TestBuildOntologyQualityCountsOnlyGenuinelyUndeclaredProperties(t *testing.T) {
	quality := buildOntologyQuality(
		&OntologyGraph{
			Properties: []OntologyPropertyEdge{
				{Type: "part_of", Source: "event", Target: "work", Relations: 2, DeclaredName: true},
				{Type: "genre", Source: "work", Target: "place", Relations: 1},
				{Type: "part_of", Source: "event", Target: "place", Relations: 5, Declared: true},
			},
		},
		0, 0, nil, false,
	)
	if quality.UndeclaredProperties != 1 {
		t.Fatalf("undeclared = %d, want 1 (only genre)", quality.UndeclaredProperties)
	}
}

// A parent the template does not declare is dropped from the edge list: there is
// no node to point at.
func TestMergeOntologyInheritanceSkipsUndeclaredParent(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "agent"},
			{"type": "person", "parent": "agent"},
			{"type": "robot", "parent": "ghost"},
		},
		nil,
	))
	edges := mergeOntologyInheritance(declared)
	if len(edges) != 1 || edges[0].Source != "agent" || edges[0].Target != "person" {
		t.Fatalf("inheritance = %+v, want agent -> person only", edges)
	}
}

// The class merge must carry the declaration's label / parent / attributes and
// the observed instance count, and must still surface an undeclared class the
// data carries (vocabulary drift) with no invented declaration.
func TestMergeOntologyClasses(t *testing.T) {
	declared := parseDeclaredOntology(ontologyTestConfig(
		[]map[string]interface{}{
			{"type": "agent", "label": "Agent"},
			{"type": "person", "label": "Person", "parent": "agent"},
		},
		[]map[string]interface{}{
			{"type": "alias", "kind": "datatype", "datatype": "list", "domain": "agent"},
		},
	))
	nodes := mergeOntologyClasses(declared, map[string]int{"person": 4, "drifted": 1}, attributeCounts{"person": {"alias": 2}})

	byType := map[string]OntologyClassNode{}
	for _, n := range nodes {
		byType[n.Type] = n
	}
	person := byType["person"]
	if person.Label != "Person" || person.Entities != 4 || !person.Declared {
		t.Errorf("person node = %+v", person)
	}
	if len(person.Parent) != 1 || person.Parent[0] != "agent" {
		t.Errorf("person parent = %v, want [agent]", person.Parent)
	}
	if len(person.Attributes) != 1 || person.Attributes[0].InheritedFrom != "agent" || person.Attributes[0].Assertions != 2 {
		t.Errorf("person attributes = %+v", person.Attributes)
	}
	drifted := byType["drifted"]
	if drifted.Declared || drifted.Entities != 1 || len(drifted.Attributes) != 0 {
		t.Errorf("undeclared class node = %+v, want no invented declaration", drifted)
	}
}

// The drill-down window must be legal whatever the caller sends: a negative
// offset and an oversized limit are a paging bug in the view, not a reason to
// ask the engine for a million rows.
func TestOntologyDrilldownPageClamps(t *testing.T) {
	cases := []struct{ offset, limit, wantLimit, wantOffset int }{
		{0, 0, ontologyDrilldownDefaultLimit, 0},
		{-5, 10, 10, 0},
		{10, -1, ontologyDrilldownDefaultLimit, 10},
		{0, 10_000, ontologyDrilldownMaxLimit, 0},
		{30, 50, 50, 30},
	}
	for _, c := range cases {
		limit, offset := ontologyDrilldownPage(c.offset, c.limit)
		if limit != c.wantLimit || offset != c.wantOffset {
			t.Errorf("ontologyDrilldownPage(%d, %d) = (%d, %d), want (%d, %d)",
				c.offset, c.limit, limit, offset, c.wantLimit, c.wantOffset)
		}
	}
}

// The drill-down filter must pin the row kind (or a class page would return
// relations) and must only add the scope keys the caller actually narrowed to.
func TestOntologyInstanceFilter(t *testing.T) {
	base := ontologyInstanceFilter("entity", "", "")
	if len(base) != 1 {
		t.Fatalf("unscoped filter = %v, want only the row kind", base)
	}
	if kinds, _ := base["knowledge_graph_kwd"].([]string); len(kinds) != 1 || kinds[0] != "entity" {
		t.Fatalf("row kind = %v", base["knowledge_graph_kwd"])
	}

	narrowed := ontologyInstanceFilter("relation", "tpl-1", "doc-1")
	if ids, _ := narrowed["compilation_template_ids"].([]string); len(ids) != 1 || ids[0] != "tpl-1" {
		t.Errorf("template scope = %v", narrowed["compilation_template_ids"])
	}
	if ids, _ := narrowed["doc_id"].([]string); len(ids) != 1 || ids[0] != "doc-1" {
		t.Errorf("document scope = %v", narrowed["doc_id"])
	}
}

// A filter narrows to a term when it has one and stays silent when it does not.
// The two are not the same request: "every class" is the absent key, because an
// empty term list is read as "match nothing" by the Elasticsearch translation
// and as "match everything" by Infinity's (see ontologyTermFilter).
func TestOntologyTermFilterNarrowOrNothing(t *testing.T) {
	filter := ontologyInstanceFilter("entity", "tpl-1", "")
	ontologyTermFilter(filter, "entity_type_kwd", " ")
	if _, ok := filter["entity_type_kwd"]; ok {
		t.Fatalf("a blank class must add no clause, got %v", filter)
	}

	ontologyTermFilter(filter, "entity_type_kwd", " person ")
	classes, _ := filter["entity_type_kwd"].([]string)
	if len(classes) != 1 || classes[0] != "person" {
		t.Fatalf("class clause = %v, want the trimmed name", filter["entity_type_kwd"])
	}
}

// An empty class or property name must short-circuit to an empty page rather
// than query the engine for "everything", which is what an empty term filter
// would become. Widening the read is the model-scope inspection's job, and it
// asks for it explicitly with All (compilation_model_inspect.go).
func TestOntologyDrilldownRejectsEmptyName(t *testing.T) {
	svc := &DatasetArtifactService{}
	page, err := svc.GetOntologyClassEntities(context.Background(), OntologyClassEntitiesInput{Class: "  "})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 0 || len(page.Entities) != 0 {
		t.Fatalf("empty class page = %+v, want empty", page)
	}
	rels, err := svc.GetOntologyPropertyRelations(context.Background(), OntologyPropertyRelationsInput{Property: ""})
	if err != nil {
		t.Fatal(err)
	}
	if rels.Total != 0 || len(rels.Relations) != 0 {
		t.Fatalf("empty property page = %+v, want empty", rels)
	}
}

// Regression: the document-level path used to read the template config out of the
// bucket meta, but resolveGraphBucket REBUILDS that meta from
// template_id/name/kind only — so the config was always empty there, and every
// observed class came back "undeclared" with no label, no datatype attribute and
// no inheritance (the dataset-level path was unaffected because it loads the
// config by id). Pinning the drop keeps the next reader from re-introducing the
// dependency.
func TestResolveGraphBucketDropsTheTemplateConfig(t *testing.T) {
	config := map[string]interface{}{
		"kind":   "ontology",
		"entity": map[string]interface{}{"fields": []interface{}{map[string]interface{}{"type": "person"}}},
	}
	meta := map[string]map[string]interface{}{
		"tpl-1": {
			"template_id":   "tpl-1",
			"template_name": "Ontology",
			"kind":          "ontology",
			"config":        config,
		},
	}
	row := map[string]interface{}{
		"compilation_template_ids":      []interface{}{"tpl-1"},
		"compilation_template_kind_kwd": "ontology",
	}

	bucket, _ := resolveGraphBucket(row, meta, "doc-1")
	if bucket["template_id"] != "tpl-1" || bucket["kind"] != "ontology" {
		t.Fatalf("bucket = %v, want the template identity carried through", bucket)
	}
	if _, ok := bucket["config"]; ok {
		t.Fatal("resolveGraphBucket started carrying config; attachDocumentOntology may now read it, " +
			"but it must not RELY on that (the rebuild only promises id/name/kind)")
	}
}

// The ontology attachment is gated on the bucket's kind, and the non-ontology
// branch must not touch the template store or the index at all — it is on the
// path of every document of every other template.
func TestAttachDocumentOntologySkipsNonOntologyBuckets(t *testing.T) {
	svc := &DatasetArtifactService{}
	for _, kind := range []string{"knowledge_graph", "tree", "page_index", ""} {
		bucket := &DocumentStructureGraphTemplate{TemplateID: "tpl-1", Kind: kind}
		// A nil DAO would panic if the config were loaded for these kinds, which
		// is the point: nothing below the gate may run.
		if err := svc.attachDocumentOntology(context.Background(), "t-1", "ds-1", "doc-1", bucket, nil); err != nil {
			t.Fatalf("kind %q: %v", kind, err)
		}
		if bucket.Ontology != nil {
			t.Fatalf("kind %q: ontology attached to a non-ontology bucket", kind)
		}
	}
}

// The engines hand a json column back either decoded or serialized; both must
// count.
func TestAttrValuesAcceptsBothEngineShapes(t *testing.T) {
	decoded := attrValues(map[string]interface{}{"attr": map[string]interface{}{"alias": "A", "birth_date": "1815"}})
	if len(decoded) != 2 {
		t.Fatalf("decoded = %v", decoded)
	}
	serialized := attrValues(map[string]interface{}{"attr": `{"alias":"A"}`})
	if len(serialized) != 1 || serialized["alias"] != "A" {
		t.Fatalf("serialized = %v", serialized)
	}
	if got := attrValues(map[string]interface{}{"attr": ""}); got != nil {
		t.Fatalf("empty = %v, want nil", got)
	}
	if got := attrValues(map[string]interface{}{"attr": "not json"}); got != nil {
		t.Fatalf("malformed = %v, want nil", got)
	}
}

// The count scan has to select the bucket the same way GetDatasetStructure does:
// by the persisted compile kind, not by the template kind column. A row whose two
// kind columns disagree (an older write, or a template whose kind differs from
// the compile kind) would otherwise be counted in the panel and missing from the
// graph it describes.
func TestDatasetBucketFilterUsesCompileKind(t *testing.T) {
	filter := datasetBucketFilter("knowledge_graph", "tpl-1")

	kinds, ok := filter["compile_kwd"].([]string)
	if !ok || len(kinds) != 1 || kinds[0] != "graph" {
		t.Fatalf("compile_kwd = %v, want the canonical graph kind", filter["compile_kwd"])
	}
	if _, present := filter["compilation_template_kind_kwd"]; present {
		t.Fatalf("filter = %#v, the count scan must not select on the template kind", filter)
	}
	scopes, ok := filter["scope_kwd"].([]string)
	if !ok || len(scopes) != 1 || scopes[0] != "dataset" {
		t.Fatalf("scope_kwd = %v, want the dataset scope kept", filter["scope_kwd"])
	}
	templates, ok := filter["compilation_template_ids"].([]string)
	if !ok || len(templates) != 1 || templates[0] != "tpl-1" {
		t.Fatalf("compilation_template_ids = %v, want the template pin kept", filter["compilation_template_ids"])
	}
}
