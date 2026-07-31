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
	if len(products) != 5 {
		t.Fatalf("products = %d, want 5 (root+A+A1+A2+B)", len(products))
	}
	if products[0].Meta["kind"] != "root" || products[0].ParentID != "" {
		t.Errorf("root product malformed: %+v", products[0].Meta)
	}
	if products[1].ParentID != products[0].ID {
		t.Errorf("A parent link = %q, want root id", products[1].ParentID)
	}
	var aID string
	for _, p := range products {
		if p.Meta["name"] == "A" {
			aID = p.ID
		}
	}
	for _, p := range products {
		if n, _ := p.Meta["name"].(string); n == "A1" || n == "A2" {
			if p.ParentID != aID {
				t.Errorf("%s parent = %q, want A id", n, p.ParentID)
			}
		}
	}
}

func TestSerializeNode_Shape(t *testing.T) {
	root := &utility.Node{ID: "Top \"quoted\"", Children: []*utility.Node{{ID: "child"}}}
	js := serializeNode(root)
	if !strings.HasPrefix(js, `{"id":"Top \"quoted\""`) {
		t.Errorf("serializeNode = %q", js)
	}
}
