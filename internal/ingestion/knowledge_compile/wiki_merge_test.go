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
	"strings"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// TestWikiMergePreservesCandidateVector verifies that page evidence merging
// retains both Markdown blocks and uses the candidate embedding for the merged
// body.
func TestWikiMergePreservesCandidateVector(t *testing.T) {
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

	groups := wikiMergeBatch(context.Background(), []MergeGroup{{
		Existing:   existing,
		Candidates: []kccommon.Product{candidate},
	}})

	merged := groups[0].Merged
	if !strings.Contains(merged.Content, "# old markdown") || !strings.Contains(merged.Content, "# new markdown") {
		t.Fatalf("content = %q, want both evidence blocks", merged.Content)
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
}

func TestWikiEntityMergeRetainsBothEvidenceBodies(t *testing.T) {
	existing := kccommon.Product{
		ID: "page-1", DocID: "kb", Variant: kccommon.VariantWiki,
		Content: "# Alpha\n\nold evidence",
		Meta:    map[string]any{"kind": "page", "slug": "entity/alpha", "page_type": "entity", "source_doc_ids": []string{"doc-1"}},
	}
	incoming := kccommon.Product{
		ID: "doc-page", DocID: "doc-2", Variant: kccommon.VariantWiki,
		Content: "# Alpha\n\nnew evidence",
		Meta:    map[string]any{"kind": "page", "slug": "entity/alpha", "page_type": "entity", "source_doc_ids": []string{"doc-2"}},
	}
	merged := wikiEntityMerge(existing, incoming)
	if merged.Content == existing.Content || merged.Content == incoming.Content {
		t.Fatalf("entity merge discarded evidence: %q", merged.Content)
	}
	if len(metaStringSliceAny(merged.Meta, "source_doc_ids")) != 2 {
		t.Fatalf("source docs = %#v, want both documents", merged.Meta["source_doc_ids"])
	}
	if merged.ID != existing.ID || merged.DocID != existing.DocID {
		t.Fatalf("entity identity changed: %#v", merged)
	}
}

func TestSplitMarkdownBlocksKeepsFencedCodeTogether(t *testing.T) {
	blocks := splitMarkdownBlocks("before\n\n```go\nline one\n\nline two\n```\n\nafter")
	if len(blocks) != 3 {
		t.Fatalf("blocks=%d, want 3: %#v", len(blocks), blocks)
	}
	if !strings.Contains(blocks[1], "line one\n\nline two") {
		t.Fatalf("fenced block was split: %q", blocks[1])
	}
}
