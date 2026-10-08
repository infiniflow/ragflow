package knowledge_compiler

import (
	"encoding/json"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

func TestProductsToChunkDocsCanonicalKinds(t *testing.T) {
	for _, test := range []struct {
		kind    string
		variant common.Variant
		want    string
	}{
		{"knowledge_graph", common.VariantStructure, "graph"},
		{"graph", common.VariantStructure, "graph"},
		{"mind_map", common.VariantMindmap, "mind_map"},
		{"mindmap", common.VariantMindmap, "mind_map"},
		{"page_index", common.VariantStructure, "page_index"},
		{"tree", common.VariantTree, "tree"},
		{"timeline", common.VariantStructure, "timeline"},
		{"wiki", common.VariantWiki, "wiki"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			docs, err := productsToChunkDocs([]common.Product{{ID: "row", Kind: test.kind, Variant: test.variant, Content: "summary",
				Meta: map[string]any{"compile_kwd": "list", "kind": "entity", "page_type": "concept"}}})
			if err != nil {
				t.Fatal(err)
			}
			row := docs[0].ToMap()
			if row["compile_kwd"] != test.want {
				t.Fatalf("compile_kwd = %v, want %s", row["compile_kwd"], test.want)
			}
			for _, field := range []string{"compilation_template_kind_kwd", "page_type_kwd"} {
				if _, exists := row[field]; exists {
					t.Fatalf("retired field %s was written", field)
				}
			}
			if test.variant == common.VariantStructure {
				var extra map[string]string
				if err := json.Unmarshal([]byte(row["extra"].(string)), &extra); err != nil {
					t.Fatal(err)
				}
				if extra["compile_type"] != "list" {
					t.Fatalf("compile type = %v", extra)
				}
			}
			if test.variant == common.VariantWiki && (row["type_kwd"] != "wiki_entity" || row["entity_type_kwd"] != "concept") {
				t.Fatalf("wiki role/category = %v/%v", row["type_kwd"], row["entity_type_kwd"])
			}
		})
	}
}
