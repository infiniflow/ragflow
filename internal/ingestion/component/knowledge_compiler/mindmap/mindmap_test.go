package mindmap

import (
	"strings"
	"testing"

	"ragflow/internal/utility"
)

func TestRenderPrompt(t *testing.T) {
	got := renderPrompt("THE TEXT")
	if !strings.Contains(got, "-TEXT-\nTHE TEXT") {
		t.Fatalf("input_text not substituted: %q", got)
	}
	if strings.Contains(got, "{input_text}") {
		t.Fatalf("placeholder survived: %q", got)
	}
	// The verbatim prompt body must be present.
	if !strings.Contains(got, "Generate a title for user's 'TEXT'。") {
		t.Fatalf("prompt body drifted")
	}
}

func TestPackSections_Budget(t *testing.T) {
	// budget = max(4096*0.8, 4096-512) = 3584; with the len-based fake
	// tokenizer each section of 2000 tokens packs one per batch.
	tok := fakeTok{}
	sections := []string{strings.Repeat("a", 8000), strings.Repeat("b", 8000), "short"}
	got := packSections(sections, tok)
	if len(got) != 2 {
		t.Fatalf("batches = %d, want 2 (2000+2000 > 3584 splits)", len(got))
	}
	// An oversized section is never split: it forms its own batch.
	big := packSections([]string{strings.Repeat("x", 20000)}, tok)
	if len(big) != 1 {
		t.Fatalf("oversized section must stay whole: %d batches", len(big))
	}
}

type fakeTok struct{}

func (fakeTok) NumTokens(s string) int { return len(s) / 4 }

func TestTreeToProducts_ParentLinks(t *testing.T) {
	root := &utility.Node{ID: "root", Children: []*utility.Node{
		{ID: "A", Children: []*utility.Node{{ID: "A1"}, {ID: "A2"}}},
		{ID: "B"},
	}}
	products := treeToProducts("t1", "d1", root)
	// 5 entities (root, A, A1, A2, B) + 4 relations (root→A, A→A1, A→A2, root→B).
	if len(products) != 9 {
		t.Fatalf("products = %d, want 9 (5 entities + 4 relations)", len(products))
	}
	entCount, relCount := 0, 0
	fromTo := map[string]bool{}
	for _, p := range products {
		kind, _ := p.Meta["kind"].(string)
		switch kind {
		case "entity":
			entCount++
			if p.Meta["entity_type"] != "mindmap" {
				t.Errorf("entity %v type = %v, want mindmap", p.Meta["name"], p.Meta["entity_type"])
			}
			if p.Meta["compile_kwd"] != "mindmap" {
				t.Errorf("entity %v compile_kwd = %v, want mindmap", p.Meta["name"], p.Meta["compile_kwd"])
			}
		case "relation":
			relCount++
			from, _ := p.Meta["from"].(string)
			to, _ := p.Meta["to"].(string)
			fromTo[from+"->"+to] = true
			if p.Meta["relation_type"] != "related" {
				t.Errorf("relation %v->%v type = %v, want related", from, to, p.Meta["relation_type"])
			}
		default:
			t.Errorf("unexpected kind %q", kind)
		}
	}
	if entCount != 5 || relCount != 4 {
		t.Errorf("entities=%d relations=%d, want 5/4", entCount, relCount)
	}
	for _, edge := range []string{"root->A", "A->A1", "A->A2", "root->B"} {
		if !fromTo[edge] {
			t.Errorf("missing relation %s", edge)
		}
	}
}

func TestTreeToProducts_EmptyAndNil(t *testing.T) {
	if got := treeToProducts("t1", "d1", nil); len(got) != 0 {
		t.Errorf("nil root produced %d products", len(got))
	}
	if got := treeToProducts("t1", "d1", &utility.Node{ID: ""}); len(got) != 0 {
		t.Errorf("empty root id produced %d products", len(got))
	}
}
