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

import "testing"

func TestParseCompetencyQuestions(t *testing.T) {
	config := map[string]interface{}{
		"competency": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{
					"type":     "CQ-1",
					"question": "未结合同的供应商是谁？",
					"elements": " class:contract | relation:contract->vendor | ?class:person ",
				},
				map[string]interface{}{
					// No id: the question text is the only handle the author gave,
					// and dropping it would silently lose the question.
					"question": "谁批准了这份合同？",
					"elements": "relation:contract->approver",
				},
				map[string]interface{}{
					// Unknown kinds are dropped rather than reported: a typo must
					// not make every question look unanswerable.
					"type":     "CQ-3",
					"question": "坏的写法",
					"elements": "kitten:fluffy",
				},
				map[string]interface{}{},
			},
		},
	}

	questions := parseCompetencyQuestions(config)
	if len(questions) != 3 {
		t.Fatalf("expected 3 questions, got %d", len(questions))
	}
	if questions[0].ID != "CQ-1" {
		t.Errorf("expected CQ-1, got %q", questions[0].ID)
	}
	if len(questions[0].Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(questions[0].Elements))
	}
	if questions[0].Elements[2].Required {
		t.Error("a ?-prefixed element must be optional")
	}
	if questions[0].Elements[0].Kind != competencyClass ||
		questions[0].Elements[0].Name != "contract" {
		t.Errorf("unexpected first element: %+v", questions[0].Elements[0])
	}
	if questions[1].ID != "谁批准了这份合同？" {
		t.Errorf("a question without an id must fall back to its text, got %q", questions[1].ID)
	}
	if len(questions[2].Elements) != 0 {
		t.Errorf("an unknown element kind must be dropped, got %+v", questions[2].Elements)
	}
}

func TestBuildCompetencyCoverageSeparatesMissingFromUndeclared(t *testing.T) {
	graph := &OntologyGraph{
		Classes: []OntologyClassNode{
			{
				Type:     "contract",
				Entities: 4,
				Declared: true,
				Attributes: []OntologyAttribute{
					{Type: "amount", Assertions: 3},
					{Type: "signed_on", Assertions: 0},
				},
			},
			// Declared but the corpus produced nothing: the question would return
			// an empty answer, so it must not count as satisfied.
			{Type: "vendor", Entities: 0, Declared: true},
		},
		Properties: []OntologyPropertyEdge{
			{Type: "supplies", Source: "contract", Target: "vendor", Relations: 0, Declared: true},
		},
	}
	questions := []CompetencyQuestion{
		{
			ID: "CQ-1",
			Elements: []CompetencyElement{
				{Kind: competencyClass, Name: "contract", Required: true},
				{Kind: competencyProperty, Name: "contract.amount", Required: true},
				{Kind: competencyProperty, Name: "contract.signed_on", Required: true},
				{Kind: competencyRelation, Name: "contract->vendor", Required: true},
				{Kind: competencyClass, Name: "ghost", Required: true},
				{Kind: competencyClass, Name: "person", Required: false},
			},
		},
	}

	coverage, required, satisfied := buildCompetencyCoverage(questions, graph)
	if required != 5 || satisfied != 2 {
		t.Fatalf("expected 2/5 required elements, got %d/%d", satisfied, required)
	}
	if coverage[0].Answerable {
		t.Error("a question with unsatisfied required elements must not be answerable")
	}
	reasons := map[string]string{}
	for _, element := range coverage[0].Elements {
		reasons[element.Name] = element.Reason
	}
	if reasons["contract"] != "" {
		t.Errorf("a class with instances must have no reason, got %q", reasons["contract"])
	}
	if reasons["contract->vendor"] != "not_extracted" {
		t.Errorf("a declared but unasserted relation must be not_extracted, got %q", reasons["contract->vendor"])
	}
	if reasons["ghost"] != "undeclared" {
		t.Errorf("a class the model never declares must be undeclared, got %q", reasons["ghost"])
	}
	if _, ok := reasons["person"]; !ok {
		t.Error("an optional element must still be reported")
	}
}

func TestBuildOntologyDriftSortsTheTwoDirections(t *testing.T) {
	graph := &OntologyGraph{
		Classes: []OntologyClassNode{
			{Type: "invoice", Entities: 12, Declared: false},
			{Type: "shipment", Entities: 3, Declared: false},
			{Type: "contract", Entities: 0, Declared: true},
			{Type: "vendor", Entities: 5, Declared: true},
		},
		Properties: []OntologyPropertyEdge{
			{Type: "supplies", Source: "vendor", Target: "contract", Relations: 0, Declared: true},
			{Type: "signs", Source: "vendor", Target: "contract", Relations: 2, Declared: true},
		},
		TotalEntities:  20,
		TotalRelations: 2,
		Quality:        OntologyQuality{DroppedRelations: 1, UntypedEntities: 4},
	}

	drift := buildOntologyDrift(graph)
	if len(drift.UndeclaredClasses) != 2 {
		t.Fatalf("expected 2 undeclared classes, got %d", len(drift.UndeclaredClasses))
	}
	if drift.UndeclaredClasses[0].Type != "invoice" {
		t.Errorf("undeclared classes must be ordered by instance count, got %q first", drift.UndeclaredClasses[0].Type)
	}
	if len(drift.UnusedClasses) != 1 || drift.UnusedClasses[0].Type != "contract" {
		t.Errorf("expected only contract declared with zero instances, got %+v", drift.UnusedClasses)
	}
	if len(drift.UnusedProperties) != 1 || drift.UnusedProperties[0].Type != "supplies" {
		t.Errorf("expected only supplies to have no assertions, got %+v", drift.UnusedProperties)
	}
	if drift.DroppedRelations != 1 || drift.UntypedEntities != 4 {
		t.Errorf("quality counters must travel with the drift: %+v", drift)
	}
	if len(drift.Classes) != 2 {
		t.Errorf("the declared ledger carries only declared classes, got %d", len(drift.Classes))
	}
}

func TestBuildOntologyDriftComputesFillRate(t *testing.T) {
	graph := &OntologyGraph{Classes: []OntologyClassNode{
		{
			Type:     "contract",
			Entities: 4,
			Declared: true,
			Attributes: []OntologyAttribute{
				{Type: "amount", Assertions: 2, Required: true, Missing: 2},
			},
		},
	}}
	drift := buildOntologyDrift(graph)
	field := drift.Classes[0].Fields[0]
	if field.FillRate != 0.5 {
		t.Errorf("expected 0.5 fill rate, got %v", field.FillRate)
	}
	if !field.Required || field.Missing != 2 {
		t.Errorf("the acceptance rule must be carried: %+v", field)
	}
}

func TestBuildCompetencyDeclarationIssues(t *testing.T) {
	config := map[string]interface{}{
		"entity": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{"type": "contract", "label": "合同"},
				map[string]interface{}{"type": "vendor", "label": "供应商"},
			},
		},
		"relation": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{"type": "supplies", "domain": "contract", "range": "vendor"},
			},
		},
		"competency": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{
					"type":     "CQ-1",
					"question": "谁供货？",
					"elements": "class:contract | relation:contract->vendor | class:ghost",
				},
			},
		},
	}

	issues := buildCompetencyDeclarationIssues(config)
	if len(issues) != 1 {
		t.Fatalf("expected exactly the undeclared element to be reported, got %+v", issues)
	}
	if issues[0].Code != competencyIssueCodeUnresolved || issues[0].Severity != ModelIssueBlocker {
		t.Errorf("unexpected issue: %+v", issues[0])
	}
	if issues[0].Subjects[0] != "CQ-1" || issues[0].Subjects[1] != "ghost" {
		t.Errorf("the finding must point at the question and the element: %+v", issues[0].Subjects)
	}
}

func TestCompetencyElementDeclaredAcceptsDeclaredRelation(t *testing.T) {
	declared := parseDeclaredOntology(map[string]interface{}{
		"entity": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{"type": "contract"},
				map[string]interface{}{"type": "vendor"},
			},
		},
		"relation": map[string]interface{}{
			"fields": []interface{}{
				map[string]interface{}{"type": "supplies", "domain": "contract", "range": "vendor"},
			},
		},
	})
	if !competencyElementDeclared(CompetencyElement{Kind: competencyRelation, Name: "contract->vendor"}, declared) {
		t.Error("a declared relation must satisfy the element")
	}
	// The reverse direction is a different relation, not the same one.
	if competencyElementDeclared(CompetencyElement{Kind: competencyRelation, Name: "vendor->contract"}, declared) {
		t.Error("a relation declared in one direction must not satisfy the other")
	}
}

func TestCoveragePercentRoundsAndBounds(t *testing.T) {
	cases := map[float64]string{0: "0%", 0.604: "60%", 1: "100%", 1.4: "100%"}
	for ratio, want := range cases {
		if got := coveragePercent(ratio); got != want {
			t.Errorf("coveragePercent(%v) = %q, want %q", ratio, got, want)
		}
	}
}

func TestOntologyInsightPayloadCoverageDefaultsToComplete(t *testing.T) {
	// A model with no competency questions is not "0% covered": there is nothing
	// it promised, so the gate must not block it.
	if got := (OntologyInsightPayload{}).Coverage(); got != 1 {
		t.Errorf("expected 1 for an empty requirement set, got %v", got)
	}
}
