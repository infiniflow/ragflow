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

// TestModelRefsForListsBothWaysIn covers the resolution a cross-dataset read
// depends on: a template in a group is reached by naming the group or by naming
// the template, and both have to be looked up. modelRefOf answers the narrower
// question ("which reference does this model prefer"), which is not
// reachability.
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

// TestInspectSelectDatasetsCapsAndReportsWhatItLeftOut covers the cap: a model
// compiled by more datasets than one query reads must say which it did not look
// at, or an empty result reads as a defect in the model.
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
	// reader can read two libraries without the other eighteen in the way.
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
