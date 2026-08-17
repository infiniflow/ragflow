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

// TestWikiReplaceCopiesCandidateVector locks the wiki replace-only merge contract
// (CodeRabbit Major): wikiReplace swaps the page body for the candidate's Markdown
// AND must copy the candidate's embedding too. WriteMerged persists Product.Vector
// without re-embedding, so keeping the existing row's vector would leave a stale
// embedding for the old page body and break KNN similarity searches.
func TestWikiReplaceCopiesCandidateVector(t *testing.T) {
	existing := kccommon.Product{
		ID:       "kb/wiki/alpha",
		DocID:    "kb",
		TenantID: "t1",
		Variant:  kccommon.VariantWiki,
		Content:  "# old markdown",
		Vector:   []float32{1, 0, 0, 0},
		Meta: map[string]any{
			"kind":       "page",
			"slug":       "page/alpha",
			"created_at": "2024-01-01",
		},
	}
	candidate := kccommon.Product{
		ID:       "kb/wiki/alpha",
		DocID:    "kb",
		TenantID: "t1",
		Variant:  kccommon.VariantWiki,
		Content:  "# new markdown",
		Vector:   []float32{0, 1, 0, 0},
		Meta: map[string]any{
			"kind":   "page",
			"slug":   "page/alpha",
			"run_id": "run-9",
		},
	}

	groups := wikiDecideBatch(context.Background(), []MergeGroup{{
		Existing:   existing,
		Candidates: []kccommon.Product{candidate},
	}})

	merged := groups[0].Merged
	if merged.Content != "# new markdown" {
		t.Fatalf("content = %q, want candidate markdown", merged.Content)
	}
	if !groups[0].Duplicate {
		t.Fatalf("wiki candidate must be marked duplicate (replacement)")
	}
	if len(merged.Vector) != len(candidate.Vector) {
		t.Fatalf("merged vector must be copied from the candidate, got %v", merged.Vector)
	}
	for i := range candidate.Vector {
		if merged.Vector[i] != candidate.Vector[i] {
			t.Fatalf("merged vector[%d] = %v, want candidate %v", i, merged.Vector, candidate.Vector)
		}
	}
	// Identity and creation time come from the existing row; content-bearing and
	// page metadata fields come from the candidate.
	if merged.ID != existing.ID || merged.DocID != existing.DocID {
		t.Fatalf("identity must be preserved: id=%q doc_id=%q", merged.ID, merged.DocID)
	}
	if merged.Meta["created_at"] != "2024-01-01" {
		t.Fatalf("creation time must be preserved from the existing row, got %v", merged.Meta["created_at"])
	}
	if merged.Meta["run_id"] != "run-9" {
		t.Fatalf("run_id must come from the candidate, got %v", merged.Meta["run_id"])
	}
}
