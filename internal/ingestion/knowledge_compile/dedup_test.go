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

package knowledge_compile

import (
	"context"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// recordingDeduper is a fake Deduper that records the Decide/DecideBatch calls
// and applies a simple policy: a candidate whose content is "dup" is merged
// into the existing row, otherwise it stays distinct.
type recordingDeduper struct {
	noopDeduper
	decideBatchCalls int
	lastGroups       []MergeGroup
}

func (r *recordingDeduper) DecideBatch(ctx context.Context, groups []MergeGroup) ([]MergeGroup, error) {
	r.decideBatchCalls++
	r.lastGroups = groups
	for gi := range groups {
		existing := groups[gi].Existing
		var distinct []kccommon.Product
		dup := false
		for _, cand := range groups[gi].Candidates {
			if stringOf(cand.Content) == "dup" {
				merged := existing
				merged.Content = "merged-" + existing.Content
				existing = merged
				dup = true
				continue
			}
			c := cand
			c.Merged = true
			c.DocID = existing.DocID
			distinct = append(distinct, c)
		}
		groups[gi].Merged = existing
		groups[gi].Duplicate = dup
		groups[gi].Distinct = distinct
	}
	return groups, nil
}

func stringOf(s any) string {
	if v, ok := s.(string); ok {
		return v
	}
	return ""
}

func TestDeduperDecideBatchFoldsGroups(t *testing.T) {
	d := &recordingDeduper{}
	existing := kccommon.Product{ID: "row-1", DocID: "kb1", Content: "base"}
	groups := []MergeGroup{
		{
			Existing:   existing,
			Candidates: []kccommon.Product{{ID: "a", Content: "dup"}, {ID: "b", Content: "fresh"}},
			Score:      1.0,
		},
	}
	out, err := d.DecideBatch(context.Background(), groups)
	if err != nil {
		t.Fatalf("DecideBatch: %v", err)
	}
	if d.decideBatchCalls != 1 {
		t.Errorf("expected exactly one DecideBatch call, got %d", d.decideBatchCalls)
	}
	g := out[0]
	if !g.Duplicate {
		t.Errorf("group with a dup candidate should report Duplicate=true")
	}
	if g.Merged.Content != "merged-base" {
		t.Errorf("merged content = %q, want merged-base", g.Merged.Content)
	}
	if len(g.Distinct) != 1 || stringOf(g.Distinct[0].Content) != "fresh" {
		t.Errorf("distinct = %v, want one fresh candidate", g.Distinct)
	}
	if g.Distinct[0].DocID != "kb1" || !g.Distinct[0].Merged {
		t.Errorf("distinct candidate should be a new merged row under the KB")
	}
}

// TestSplitWikiGroupsMapsStructureIndexes locks the Critical fix in
// llmDeduper.DecideBatch: structIdx[i] must hold the ORIGINAL position in the
// batch, not the position inside the structure-only slice. When a wiki group
// appears before a structure group, a buggy len(structGroups) mapping would make
// structIdx[0]==0 and fold structure results back into the wiki group instead of
// the structure group at index 1.
func TestSplitWikiGroupsMapsStructureIndexes(t *testing.T) {
	groups := []MergeGroup{
		{Existing: kccommon.Product{ID: "wiki", Variant: kccommon.VariantWiki, Content: "w"}},
		{Existing: kccommon.Product{ID: "struct-0", Variant: kccommon.Variant("structure"), Content: "s0"}},
		{Existing: kccommon.Product{ID: "wiki-2", Variant: kccommon.VariantWiki, Content: "w2"}},
		{Existing: kccommon.Product{ID: "struct-1", Variant: kccommon.Variant("structure"), Content: "s1"}},
	}
	wikiIdx, structIdx, structGroups := splitWikiGroups(groups)

	if len(wikiIdx) != 2 || wikiIdx[0] != 0 || wikiIdx[1] != 2 {
		t.Fatalf("wikiIdx = %v, want [0 2]", wikiIdx)
	}
	if len(structIdx) != 2 {
		t.Fatalf("structIdx = %v, want two structure entries", structIdx)
	}
	// The structure groups appear at original batch indices 1 and 3.
	wantStructIdx := []int{1, 3}
	for i, gi := range structIdx {
		if gi != wantStructIdx[i] {
			t.Fatalf("structIdx[%d] = %d, want %d (original batch index)", i, gi, wantStructIdx[i])
		}
		// groups[structIdx[i]] must be the exact element copied into structGroups[i],
		// so folding back via groups[structIdx[i]] lands on the correct group.
		if groups[gi].Existing.ID != structGroups[i].Existing.ID {
			t.Fatalf("groups[%d] (%s) is not the same element as structGroups[%d] (%s)",
				gi, groups[gi].Existing.ID, i, structGroups[i].Existing.ID)
		}
	}
	// The buggy mapping would have produced structIdx == [0 1], pointing at the
	// wiki groups; assert we did NOT regress to that.
	if structIdx[0] == 0 || structIdx[1] == 1 {
		t.Fatalf("structIdx mapped to the wiki groups' positions: %v", structIdx)
	}
}

func TestNoopDeduperDecideBatchNoMerge(t *testing.T) {
	d := NewNoopDeduper()
	existing := kccommon.Product{ID: "row-1", DocID: "kb1", Content: "base"}
	groups := []MergeGroup{
		{Existing: existing, Candidates: []kccommon.Product{{ID: "a", Content: "x"}, {ID: "b", Content: "y"}}},
	}
	out, err := d.DecideBatch(context.Background(), groups)
	if err != nil {
		t.Fatalf("DecideBatch: %v", err)
	}
	g := out[0]
	if g.Duplicate {
		t.Errorf("noop deduper must never merge")
	}
	if len(g.Distinct) != 2 {
		t.Fatalf("noop deduper should report both candidates distinct, got %d", len(g.Distinct))
	}
	for _, c := range g.Distinct {
		if c.DocID != "kb1" || !c.Merged {
			t.Errorf("distinct candidate should be a new merged row: %+v", c)
		}
	}
}
