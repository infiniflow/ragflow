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
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func TestNormalizeModelInspectScope(t *testing.T) {
	cases := map[string]ModelInspectScope{
		"":         ModelInspectScopeDataset,
		"doc":      ModelInspectScopeDoc,
		" DOC ":    ModelInspectScopeDoc,
		"dataset":  ModelInspectScopeDataset,
		"corpus":   ModelInspectScopeCorpus,
		"all":      ModelInspectScopeCorpus,
		"nonsense": ModelInspectScopeDataset,
	}
	for in, want := range cases {
		if got := NormalizeModelInspectScope(in); got != want {
			t.Errorf("NormalizeModelInspectScope(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestModelRefsForListsBothWaysIn covers the resolution the panel depends on: a
// template in a group is reached by naming the group or by naming the template,
// and both have to be looked up. modelRefOf answers the narrower question ("which
// reference does this model prefer"), which is not reachability.
func TestModelRefsForListsBothWaysIn(t *testing.T) {
	groupID := "grp1"
	inGroup := modelRefsFor(&entity.CompilationTemplate{ID: "tpl1", GroupID: &groupID})
	want := []common.ModelRef{
		{Kind: common.ModelRefGroup, ID: "grp1"},
		{Kind: common.ModelRefTemplate, ID: "tpl1"},
	}
	if len(inGroup) != len(want) {
		t.Fatalf("refs = %+v, want %+v", inGroup, want)
	}
	for i := range want {
		if inGroup[i] != want[i] {
			t.Fatalf("ref %d = %+v, want %+v (whole: %+v)", i, inGroup[i], want[i], inGroup)
		}
	}

	// A blank group id is not a reference: looking one up would match nothing and
	// report a dataset gap that does not exist.
	blank := "  "
	bare := modelRefsFor(&entity.CompilationTemplate{ID: "tpl2", GroupID: &blank})
	if len(bare) != 1 || bare[0].Kind != common.ModelRefTemplate || bare[0].ID != "tpl2" {
		t.Fatalf("refs = %+v, want only the template", bare)
	}
}

// TestInspectDocumentIDOnlyNarrowsDocScope pins the scope/dimension pairing: a
// document id under dataset scope would silently narrow the fan-out to rows that
// happen to carry it, which is not what "dataset scope" means.
func TestInspectDocumentIDOnlyNarrowsDocScope(t *testing.T) {
	if got := inspectDocumentID(ModelInspectScopeDoc, " d1 "); got != "d1" {
		t.Errorf("doc scope document id = %q, want d1", got)
	}
	for _, scope := range []ModelInspectScope{ModelInspectScopeDataset, ModelInspectScopeCorpus} {
		if got := inspectDocumentID(scope, "d1"); got != "" {
			t.Errorf("%s scope document id = %q, want none", scope, got)
		}
	}
}

// TestInspectSelectDatasetsCapsAndReportsWhatItLeftOut covers the cap: a model
// compiled by more datasets than one query reads must say which it did not look
// at, or an empty panel reads as a defect in the model.
func TestInspectSelectDatasetsCapsAndReportsWhatItLeftOut(t *testing.T) {
	bound := make([]*dao.ModelImpactDataset, 0, InspectDatasetsMax+3)
	for i := 0; i < InspectDatasetsMax+3; i++ {
		bound = append(bound, &dao.ModelImpactDataset{ID: fmt.Sprintf("kb%02d", i), Name: "kb", DocNum: 2})
	}
	picked, skipped := inspectSelectDatasets(bound, ModelInspectScopeCorpus, nil)
	if len(picked) != InspectDatasetsMax {
		t.Fatalf("picked = %d datasets, want the cap %d", len(picked), InspectDatasetsMax)
	}
	if len(skipped) != 3 {
		t.Fatalf("skipped = %v, want the 3 over the cap", skipped)
	}
	if picked[0].ID != "kb00" || picked[InspectDatasetsMax-1].ID != fmt.Sprintf("kb%02d", InspectDatasetsMax-1) {
		t.Fatalf("picked the wrong datasets: first %q last %q", picked[0].ID, picked[InspectDatasetsMax-1].ID)
	}

	// An explicit list narrows the read to it (and stays under the cap), so a
	// reader can inspect two libraries without the other eighteen in the way.
	picked, skipped = inspectSelectDatasets(bound, ModelInspectScopeDataset, []string{" kb01 ", "", "kb02"})
	if len(picked) != 2 || picked[0].ID != "kb01" || picked[1].ID != "kb02" {
		t.Fatalf("explicit list picked %+v", picked)
	}
	if len(skipped) != 0 {
		t.Fatalf("explicit list skipped %v, want none", skipped)
	}

	// Unknown ids are not invented: the list only narrows what the index knows.
	picked, _ = inspectSelectDatasets(bound, ModelInspectScopeDataset, []string{"nope"})
	if len(picked) != 0 {
		t.Fatalf("picked %+v for an unknown dataset", picked)
	}
}

// TestInspectRowOrderPrefersMentionsThenName keeps paging deterministic across
// datasets: without the name and dataset tie-breakers the same query could
// return two different pages for the same window.
func TestInspectRowOrderPrefersMentionsThenName(t *testing.T) {
	rows := []StructureGraphNode{
		{"name": "beta", "mention_count": 1, "dataset_id": "kb1"},
		{"name": "alpha", "mention_count": 5, "dataset_id": "kb2"},
		{"name": "alpha", "mention_count": 5, "dataset_id": "kb1"},
		{"name": "gamma", "mention_count": 5, "dataset_id": "kb1"},
	}
	sortInspectEntities(rows)
	want := []string{"alpha/kb1", "alpha/kb2", "gamma/kb1", "beta/kb1"}
	for i, w := range want {
		got := inspectRowString(rows[i], "name") + "/" + inspectRowString(rows[i], "dataset_id")
		if got != w {
			t.Fatalf("row %d = %s, want %s (whole order: %v)", i, got, w, rows)
		}
	}
	// A JSON-round-tripped mention count arrives as float64; the order must not
	// depend on which numeric type the engine handed back.
	floats := []StructureGraphNode{
		{"name": "beta", "mention_count": float64(2), "dataset_id": "kb1"},
		{"name": "alpha", "mention_count": float64(9), "dataset_id": "kb1"},
	}
	sortInspectEntities(floats)
	if floats[0]["name"] != "alpha" {
		t.Fatalf("float mention counts were not ranked: %v", floats)
	}
}

// TestInspectNotePageCountsRowsNotEngineMatches pins what "in scope" means: a
// dataset-scope query matches a document's rows AND the dataset's folded row for
// the same name, so the engine's match total counts rows the reader never sees.
// The window filling up is the signal that there may be more.
func TestInspectNotePageCountsRowsNotEngineMatches(t *testing.T) {
	result := &ModelInspectResult{}
	inspectNotePage(result, 3, 50)
	if result.Total != 3 || result.Truncated {
		t.Fatalf("partial page: total = %d truncated = %v, want 3/false", result.Total, result.Truncated)
	}
	inspectNotePage(result, 50, 50)
	if result.Total != 53 {
		t.Fatalf("total = %d, want the returned rows summed (53)", result.Total)
	}
	if !result.Truncated {
		t.Fatal("a full window must report that more may exist")
	}
}

// TestInspectAnnotateRowStampsTheDataset is what lets two same-named instances
// in two libraries stay two instances rather than one merged-looking row.
func TestInspectAnnotateRowStampsTheDataset(t *testing.T) {
	row := StructureGraphNode{"name": "Acme"}
	inspectAnnotateRow(row, ModelInspectDataset{ID: "kb1", Name: "Contracts"})
	if row["dataset_id"] != "kb1" || row["dataset_name"] != "Contracts" {
		t.Fatalf("row = %v, want the source dataset stamped", row)
	}
}
