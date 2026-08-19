package knowledge_compile

import (
	"reflect"
	"strings"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/service/nav"
)

// TestNavInputFromProducts_TreeAndStructure covers B2: nav is fed by BOTH tree
// and structure products (not tree alone). Tree uses the root product's summary;
// structure folds the graph entity descriptions into a summary via pageIndexSummary.
func TestNavInputFromProducts_TreeAndStructure(t *testing.T) {
	products := []kccommon.Product{
		// tree: root product carries the doc summary + vector.
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantTree,
			Content: "overall doc theme root summary", Vector: []float32{0.1, 0.2},
			Meta: map[string]any{"kind": "root"}},
		// tree: a non-root summary node is NOT a nav input.
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantTree,
			Content: "section body", Meta: map[string]any{"kind": "summary", "level": 0}},
		// structure: graph product folds entity descriptions.
		{DocID: "d2", TenantID: "t1", Variant: kccommon.VariantStructure,
			Content: `{"entities":[{"name":"Engine","description":"a propulsion device"}]}`,
			Meta:    map[string]any{"kind": "graph", "compile_kwd": "page_index"}},
	}

	got := navInputFromProducts("kb1", products)
	if len(got) != 2 {
		t.Fatalf("want 2 nav inputs (tree d1 + structure d2), got %d", len(got))
	}
	byDoc := map[string]nav.UpsertDocInput{}
	for _, in := range got {
		byDoc[in.DocID] = in
	}
	treeIn, ok := byDoc["d1"]
	if !ok {
		t.Fatal("missing tree nav input for d1")
	}
	if !strings.Contains(treeIn.Summary, "overall doc theme") {
		t.Errorf("tree summary = %q, want root summary", treeIn.Summary)
	}
	if !reflect.DeepEqual(treeIn.Embedd, []float32{0.1, 0.2}) {
		t.Errorf("tree embedd should be the root vector, got %v", treeIn.Embedd)
	}
	structIn, ok := byDoc["d2"]
	if !ok {
		t.Fatal("missing structure nav input for d2")
	}
	if !strings.Contains(structIn.Summary, "Engine: a propulsion device") {
		t.Errorf("structure summary = %q, want folded entity descriptions", structIn.Summary)
	}
	// Structure graph vector is NOT the summary vector: leave Embedd empty so
	// NavService embeds the folded summary text.
	if len(structIn.Embedd) != 0 {
		t.Errorf("structure embedd should be empty (NavService re-embeds), got %v", structIn.Embedd)
	}
}

// TestNavInputFromProducts_WikiExcluded covers the dispatch boundary: wiki
// products never become nav inputs (they go to the page-specific wiki merge).
func TestNavInputFromProducts_WikiExcluded(t *testing.T) {
	products := []kccommon.Product{
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantWiki,
			Content: "a wiki page", Meta: map[string]any{"kind": "page", "slug": "entity/alpha"}},
	}
	got := navInputFromProducts("kb1", products)
	if len(got) != 0 {
		t.Fatalf("wiki products must not feed nav, got %d inputs", len(got))
	}
}

// TestNavInputFromProducts_EmptySummaryDropped covers the guard: a doc whose
// tree/structure summary is empty (no root / no graph) is skipped, not fed empty.
func TestNavInputFromProducts_EmptySummaryDropped(t *testing.T) {
	products := []kccommon.Product{
		// tree root with empty content -> skip.
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantTree, Meta: map[string]any{"kind": "root"}},
		// structure graph with no entity descriptions -> skip.
		{DocID: "d2", TenantID: "t1", Variant: kccommon.VariantStructure,
			Content: `{"entities":[]}`, Meta: map[string]any{"kind": "graph"}},
	}
	got := navInputFromProducts("kb1", products)
	if len(got) != 0 {
		t.Fatalf("empty-summary products must be dropped, got %d inputs", len(got))
	}
}

// TestProductsForVariants_GatesDispatch covers the A0-4 per-variant gate: a doc
// whose event carries only ["wiki"] must NOT have its stale tree/structure
// doc-level products leak into the nav/structure paths. Empty variants (legacy
// events) keep everything.
func TestProductsForVariants_GatesDispatch(t *testing.T) {
	products := []kccommon.Product{
		{DocID: "d1", Variant: kccommon.VariantTree, Meta: map[string]any{"kind": "root"}, Content: "tree summary"},
		{DocID: "d1", Variant: kccommon.VariantStructure, Meta: map[string]any{"kind": "graph"}, Content: `{"entities":[]}`},
		{DocID: "d1", Variant: kccommon.VariantWiki, Meta: map[string]any{"kind": "page"}, Content: "wiki page"},
	}
	// A wiki-only event must drop the tree/structure products.
	got := productsForVariants(products, []string{string(kccommon.VariantWiki)})
	if len(got) != 1 || got[0].Variant != kccommon.VariantWiki {
		t.Fatalf("wiki-only event: want exactly the wiki product, got %d", len(got))
	}
	// A tree-only event drops structure + wiki.
	got = productsForVariants(products, []string{string(kccommon.VariantTree)})
	if len(got) != 1 || got[0].Variant != kccommon.VariantTree {
		t.Fatalf("tree-only event: want exactly the tree product, got %d", len(got))
	}
	// An empty variant set (legacy event / no variants) keeps everything.
	if got = productsForVariants(products, nil); len(got) != 3 {
		t.Fatalf("empty variants: want all 3 products kept, got %d", len(got))
	}
}

// TestPageIndexSummary_ConcatenatesAndSkipsEmpty verifies pageIndexSummary folds
// entity descriptions and skips empty ones.
func TestPageIndexSummary_ConcatenatesAndSkipsEmpty(t *testing.T) {
	got := pageIndexSummary(`{"entities":[{"name":"A","description":"one  thing"},{"name":"B","description":""}]}`)
	if !strings.Contains(got, "A: one thing") {
		t.Errorf("summary = %q, want entity A", got)
	}
	if strings.Contains(got, "B") {
		t.Errorf("summary should skip empty-description entity, got %q", got)
	}
}
