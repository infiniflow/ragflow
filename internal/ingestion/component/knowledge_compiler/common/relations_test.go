package common

import "testing"

func TestFilterOrphanRelations(t *testing.T) {
	products := []Product{
		{Variant: VariantStructure, TemplateID: "graph", Meta: map[string]any{"kind": "entity", "name": "A"}},
		{Variant: VariantStructure, TemplateID: "graph", Meta: map[string]any{"kind": "entity", "name": "B"}},
		{Variant: VariantStructure, TemplateID: "graph", Meta: map[string]any{"kind": "relation", "from": " A ", "to": "B"}},
		{Variant: VariantStructure, TemplateID: "graph", Meta: map[string]any{"kind": "relation", "from": "A", "to": "Missing"}},
	}

	filtered := FilterOrphanRelations(products)
	if len(filtered) != 3 {
		t.Fatalf("got %d products, want 3", len(filtered))
	}
	if filtered[2].Meta["to"] != "B" {
		t.Fatalf("valid relation was not preserved: %+v", filtered[2])
	}
}

func TestFilterOrphanRelationsScopesTemplates(t *testing.T) {
	products := []Product{
		{Variant: VariantStructure, TemplateID: "graph", Meta: map[string]any{"kind": "entity", "name": "A"}},
		{Variant: VariantStructure, TemplateID: "timeline", Meta: map[string]any{"kind": "relation", "from": "A", "to": "B"}},
	}
	if got := FilterOrphanRelations(products); len(got) != 1 {
		t.Fatalf("relation satisfied by another template: got %d products, want 1", len(got))
	}
}
