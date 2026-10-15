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
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// The trial's answer is only as good as the identity it compares on: a
// different engine id or embedding between two runs of the same input must not
// read as a change.
func TestTrialAssertionsReduceToKnowledgeIdentity(t *testing.T) {
	products := []kccommon.Product{
		{ID: "row-1", Meta: map[string]any{"kind": "entity", "name": "Alpha", "entity_type": "person"}},
		// The same entity, stored under a different engine id and vector: it is
		// the same assertion twice, so it is counted once.
		{ID: "row-2", Meta: map[string]any{"kind": "entity", "name": "Alpha", "entity_type": "person"}},
		{ID: "row-3", Meta: map[string]any{"kind": "relation", "from": "Alpha", "to": "Beta", "relation_type": "born_in"}},
		// A container row renders the assertions above it; counting it would
		// double every change.
		{ID: "row-4", Meta: map[string]any{"kind": "graph", "title": "Alpha"}},
		// A row with no name is not an assertion anybody can review.
		{ID: "row-5", Meta: map[string]any{"kind": "entity"}},
	}

	assertions := trialAssertions(products)
	if len(assertions) != 2 {
		t.Fatalf("assertions = %#v, want the entity and the relation", assertions)
	}
	if assertions[0].Kind != "entity" || assertions[0].Name != "Alpha" || assertions[0].Type != "person" {
		t.Fatalf("assertion[0] = %#v, want the person entity", assertions[0])
	}
	if assertions[1].Kind != "relation" || assertions[1].Name != "Alpha -> Beta" || assertions[1].Type != "born_in" {
		t.Fatalf("assertion[1] = %#v, want the born_in relation", assertions[1])
	}
}

// A namesake of a different type is a different assertion: the model declaring
// Alpha as a person and as a place is exactly the kind of change a publisher
// has to see.
func TestTrialAssertionsKeepTypesApart(t *testing.T) {
	products := []kccommon.Product{
		{Meta: map[string]any{"kind": "entity", "name": "Alpha", "entity_type": "person"}},
		{Meta: map[string]any{"kind": "entity", "name": "Alpha", "entity_type": "place"}},
	}
	if assertions := trialAssertions(products); len(assertions) != 2 {
		t.Fatalf("assertions = %#v, want one per declared type", assertions)
	}
}

func TestDiffTrialAssertionsReportsBothDirections(t *testing.T) {
	current := []ModelTrialAssertion{
		{Kind: "entity", Type: "person", Name: "Alpha", Key: "entity|person|Alpha"},
		{Kind: "entity", Type: "place", Name: "Gamma", Key: "entity|place|Gamma"},
	}
	draft := []ModelTrialAssertion{
		{Kind: "entity", Type: "person", Name: "Alpha", Key: "entity|person|Alpha"},
		{Kind: "entity", Type: "place", Name: "Beta", Key: "entity|place|Beta"},
	}

	added, removed, unchanged := diffTrialAssertions(current, draft)
	if unchanged != 1 {
		t.Fatalf("unchanged = %d, want the entity both runs agree on", unchanged)
	}
	if len(added) != 1 || added[0].Name != "Beta" {
		t.Fatalf("added = %#v, want the entity the draft introduces", added)
	}
	if len(removed) != 1 || removed[0].Name != "Gamma" {
		t.Fatalf("removed = %#v, want the entity the draft drops", removed)
	}
}

// An unchanged model must produce an empty diff, which is what tells a
// publisher that the model they are about to publish changes nothing.
func TestDiffTrialAssertionsIsEmptyForIdenticalRuns(t *testing.T) {
	products := []kccommon.Product{
		{Meta: map[string]any{"kind": "entity", "name": "Alpha", "entity_type": "person"}},
		{Meta: map[string]any{"kind": "relation", "from": "Alpha", "to": "Beta", "relation_type": "born_in"}},
	}
	assertions := trialAssertions(products)
	added, removed, unchanged := diffTrialAssertions(assertions, trialAssertions(products))
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("diff = (+%#v, -%#v), want no change for the same knowledge", added, removed)
	}
	if unchanged != len(assertions) {
		t.Fatalf("unchanged = %d, want %d", unchanged, len(assertions))
	}
}
