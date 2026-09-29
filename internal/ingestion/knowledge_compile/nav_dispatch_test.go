package knowledge_compile

import (
	"reflect"
	"strings"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/service/nav"
)

// TestNavInputFromProducts_TreeAndPageIndex verifies that navigation is fed by
// tree roots and PageIndex entity rows. Other structure kinds do not create
// navigation entries.
func TestNavInputFromProducts_TreeAndPageIndex(t *testing.T) {
	products := []kccommon.Product{
		// tree: root product carries the doc summary + vector.
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantTree,
			Content: "overall doc theme root summary", Vector: []float32{0.1, 0.2},
			Meta: map[string]any{"kind": "root"}},
		// tree: a non-root summary node is NOT a nav input.
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantTree,
			Content: "section body", Meta: map[string]any{"kind": "summary", "level": 0}},
		// PageIndex entity rows fold their descriptions into the doc summary.
		{DocID: "d2", TenantID: "t1", Variant: kccommon.VariantStructure,
			Content: `{"name":"Engine","type":"component","description":"a propulsion device"}`,
			Kind:    "page_index", Meta: map[string]any{"kind": "entity"}},
		{DocID: "d2", TenantID: "t1", Variant: kccommon.VariantStructure,
			Content: `{"name":"Turbine","type":"component","description":"converts flow into rotation"}`,
			Kind:    "page_index", Meta: map[string]any{"kind": "entity"}},
		// A PageIndex relation is not a nav summary line.
		{DocID: "d2", TenantID: "t1", Variant: kccommon.VariantStructure,
			Content: `{"from":"Engine","to":"Turbine","type":"drives"}`,
			Kind:    "page_index", Meta: map[string]any{"kind": "relation", "from": "Engine", "to": "Turbine"}},
		// Graph entities must not create navigation entries.
		{DocID: "d3", TenantID: "t1", Variant: kccommon.VariantStructure,
			Content: `{"name":"Graph entity","description":"must not become navigation"}`,
			Kind:    "knowledge_graph", Meta: map[string]any{"kind": "entity"}},
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
	pageIndexIn, ok := byDoc["d2"]
	if !ok {
		t.Fatal("missing PageIndex nav input for d2")
	}
	if _, found := byDoc["d3"]; found {
		t.Fatal("Graph structure product must not produce navigation input")
	}
	if !strings.Contains(pageIndexIn.Summary, "Engine: a propulsion device") {
		t.Errorf("PageIndex summary = %q, want folded entity descriptions", pageIndexIn.Summary)
	}
	if !strings.Contains(pageIndexIn.Summary, "Turbine: converts flow into rotation") {
		t.Errorf("PageIndex summary = %q, want both entity lines", pageIndexIn.Summary)
	}
	if strings.Contains(pageIndexIn.Summary, "drives") {
		t.Errorf("PageIndex summary must not include relation rows, got %q", pageIndexIn.Summary)
	}
	// The entity rows' vectors are NOT the summary vector: leave Embedd empty so
	// NavService embeds the folded summary text.
	if len(pageIndexIn.Embedd) != 0 {
		t.Errorf("PageIndex embedd should be empty (NavService re-embeds), got %v", pageIndexIn.Embedd)
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
// tree/structure summary is empty (no root / no entity descriptions) is
// skipped, not fed empty.
func TestNavInputFromProducts_EmptySummaryDropped(t *testing.T) {
	products := []kccommon.Product{
		// tree root with empty content -> skip.
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantTree, Meta: map[string]any{"kind": "root"}},
		// PageIndex entity with no description -> skip.
		{DocID: "d2", TenantID: "t1", Variant: kccommon.VariantStructure,
			Kind: "page_index", Content: `{"name":"A","description":""}`, Meta: map[string]any{"kind": "entity"}},
	}
	got := navInputFromProducts("kb1", products)
	if len(got) != 0 {
		t.Fatalf("empty-summary products must be dropped, got %d inputs", len(got))
	}
}

func TestNavInputFromProducts_TreeSummaryWinsRegardlessOfOrder(t *testing.T) {
	products := []kccommon.Product{
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantStructure,
			Content: `{"name":"A","description":"structure summary"}`,
			Kind:    "page_index", Meta: map[string]any{"kind": "entity"}},
		{DocID: "d1", TenantID: "t1", Variant: kccommon.VariantTree,
			Content: "tree summary", Vector: []float32{0.1},
			Meta: map[string]any{"kind": "root"}},
	}

	got := navInputFromProducts("kb1", products)
	if len(got) != 1 {
		t.Fatalf("want one nav input, got %d", len(got))
	}
	if got[0].Summary != "tree summary" {
		t.Fatalf("summary = %q, want tree summary", got[0].Summary)
	}
	if !reflect.DeepEqual(got[0].Embedd, []float32{0.1}) {
		t.Fatalf("embedding = %v, want tree embedding", got[0].Embedd)
	}
}

// TestProductsForVariants_GatesDispatch covers the A0-4 per-variant gate: a doc
// whose event carries only ["wiki"] must NOT have its stale tree/structure
// doc-level products leak into the nav/structure paths. Empty variants (legacy
// events) keep everything.
func TestProductsForVariants_GatesDispatch(t *testing.T) {
	products := []kccommon.Product{
		{DocID: "d1", Variant: kccommon.VariantTree, Meta: map[string]any{"kind": "root"}, Content: "tree summary"},
		{DocID: "d1", Variant: kccommon.VariantStructure, Meta: map[string]any{"kind": "entity"}, Content: `{"name":"A","description":"d"}`},
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

// TestStructureEntityNavLine verifies the per-row fold: whitespace-normalized
// description, "name: desc" shape, empty description -> empty line.
func TestStructureEntityNavLine(t *testing.T) {
	if got := structureEntityNavLine(`{"name":"A","description":"one  thing"}`); got != "A: one thing" {
		t.Errorf("line = %q, want %q", got, "A: one thing")
	}
	if got := structureEntityNavLine(`{"name":"B","description":""}`); got != "" {
		t.Errorf("empty description should yield empty line, got %q", got)
	}
	if got := structureEntityNavLine(`{"description":"no name"}`); got != "no name" {
		t.Errorf("nameless entity line = %q, want %q", got, "no name")
	}
	if got := structureEntityNavLine(`not json`); got != "" {
		t.Errorf("malformed payload should yield empty line, got %q", got)
	}
}
