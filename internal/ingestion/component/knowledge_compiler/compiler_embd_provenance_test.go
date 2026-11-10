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

package knowledge_compiler

import (
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestCompilerContentEmbeddingVariant(t *testing.T) {
	if !compilerContentEmbeddingVariant(common.VariantMindmap) {
		t.Error("mindmap embeds Content; expected eligible")
	}
	if !compilerContentEmbeddingVariant(common.VariantWiki) {
		t.Error("wiki embeds Content; expected eligible")
	}
	if compilerContentEmbeddingVariant(common.VariantStructure) {
		t.Error("structure embeds payloadDescription, not Content; must be ineligible")
	}
	if compilerContentEmbeddingVariant(common.VariantTree) {
		t.Error("tree graph embeds payloadDescription; VariantTree omitted fail-closed")
	}
}

// TestProductsToChunkDocs_CompilerEmbdIDByVariant checks that productsToChunkDocs
// forwards `_compiler_embd_id` when present on mindmap/wiki Meta and leaves
// structure/tree products unmarked when Meta has no such key.
func TestProductsToChunkDocs_CompilerEmbdIDByVariant(t *testing.T) {
	const embdID = "text-embedding@Builtin"
	products := []common.Product{
		{
			ID: "mm-1", Variant: common.VariantMindmap, Content: "mindmap node",
			Vector: []float32{0.1, 0.2, 0.3},
			Meta:   map[string]any{"_compiler_embd_id": embdID, "kind": "entity"},
		},
		{
			ID: "wiki-1", Variant: common.VariantWiki, Content: "wiki page body",
			Vector: []float32{0.4, 0.5, 0.6},
			Meta:   map[string]any{"_compiler_embd_id": embdID, "kind": "page"},
		},
		{
			ID: "struct-1", Variant: common.VariantStructure, Content: `{"name":"A"}`,
			Vector: []float32{0.7, 0.8, 0.9},
			Meta:   map[string]any{"kind": "entity"}, // no provenance marker
		},
		{
			ID: "tree-1", Variant: common.VariantTree, Content: `{"name":"B"}`,
			Vector: []float32{0.2, 0.3, 0.4},
			Meta:   map[string]any{"kind": "entity"}, // no provenance marker
		},
	}

	docs, err := productsToChunkDocs(products, "English")
	if err != nil {
		t.Fatalf("productsToChunkDocs: %v", err)
	}
	if len(docs) != 4 {
		t.Fatalf("docs len = %d, want 4", len(docs))
	}

	if got, ok := docs[0].GetExtraString("_compiler_embd_id"); !ok || got != embdID {
		t.Errorf("mindmap missing marker: ok=%v got=%q", ok, got)
	}
	if got, ok := docs[1].GetExtraString("_compiler_embd_id"); !ok || got != embdID {
		t.Errorf("wiki missing marker: ok=%v got=%q", ok, got)
	}
	if _, ok := docs[2].GetExtraString("_compiler_embd_id"); ok {
		t.Error("structure product must not carry _compiler_embd_id")
	}
	if _, ok := docs[3].GetExtraString("_compiler_embd_id"); ok {
		t.Error("tree product must not carry _compiler_embd_id")
	}
}

// TestStampCompilerEmbdID exercises the production stamping helper used by
// Invoke: only mindmap/wiki products with a non-empty embdID and non-empty
// vector receive `_compiler_embd_id`; other Meta keys are preserved.
func TestStampCompilerEmbdID(t *testing.T) {
	cases := []struct {
		name     string
		variant  common.Variant
		embdID   string
		vector   []float32
		wantMark bool
	}{
		{"mindmap ok", common.VariantMindmap, "m1", []float32{1}, true},
		{"wiki ok", common.VariantWiki, "m1", []float32{1}, true},
		{"structure blocked", common.VariantStructure, "m1", []float32{1}, false},
		{"tree blocked", common.VariantTree, "m1", []float32{1}, false},
		{"mindmap empty embd", common.VariantMindmap, "", []float32{1}, false},
		{"mindmap empty vec", common.VariantMindmap, "m1", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			products := []common.Product{{
				ID:     "p1",
				Meta:   map[string]any{"kind": "entity", "keep": "yes"},
				Vector: tc.vector,
			}}
			stampCompilerEmbdID(products, tc.variant, tc.embdID)
			got, has := products[0].Meta["_compiler_embd_id"].(string)
			if has != tc.wantMark {
				t.Fatalf("has marker = %v, want %v (got=%q)", has, tc.wantMark, got)
			}
			if tc.wantMark && got != tc.embdID {
				t.Fatalf("marker = %q, want %q", got, tc.embdID)
			}
			if products[0].Meta["keep"] != "yes" || products[0].Meta["kind"] != "entity" {
				t.Fatalf("other Meta keys not preserved: %#v", products[0].Meta)
			}
		})
	}
}
