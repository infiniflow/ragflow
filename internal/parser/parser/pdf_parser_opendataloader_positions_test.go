package parser

import (
	"math"
	"testing"
)

// OpenDataLoader reports [left, bottom, right, top] in PDF points (origin
// bottom-left); RAGFlow stores [page, left, right, top, bottom] in screen space
// (origin top-left). These tests pin the conversion that keeps citations
// highlightable (issue #21216).
func TestOpenDataLoaderItems_ScreenSpacePositions(t *testing.T) {
	root := map[string]any{
		"type":         "table of contents",
		"page number":  1.0,
		"bounding box": []any{72.0, 650.5, 540.0, 700.2},
		"children": []any{
			map[string]any{
				"type":        "paragraph",
				"content":     "body",
				"page number": 2.0,
				"bbox":        []any{100.0, 50.0, 200.0, 80.0},
			},
		},
	}
	sizes := map[int][2]float64{0: {612, 792}, 1: {612, 792}}
	items := openDataLoaderItemsWithGeometry(root, sizes)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (unknown type without text is skipped): %v", len(items), items)
	}
	got := items[0]["_pdf_positions"].([][]any)
	want := []any{2.0, 100.0, 200.0, 712.0, 742.0}
	if len(got) != 1 || len(got[0]) != 5 {
		t.Fatalf("positions = %#v, want one 5-column row", got)
	}
	for i := range want {
		if math.Abs(got[0][i].(float64)-want[i].(float64)) > 1e-6 {
			t.Fatalf("positions = %#v, want %#v", got, want)
		}
	}
	legacy, ok := items[0]["positions"].([][]any)
	if !ok || len(legacy) != 1 || len(legacy[0]) != 5 {
		t.Fatalf("positions alias = %#v, want the same one-row matrix", items[0]["positions"])
	}
	for i := range want {
		if math.Abs(legacy[0][i].(float64)-want[i].(float64)) > 1e-6 {
			t.Fatalf("positions alias = %#v, want %#v", legacy, want)
		}
	}
}

func TestOpenDataLoaderItems_AllElementTypesCarryPositions(t *testing.T) {
	root := map[string]any{
		"type": "document", "page number": 1.0, "bounding box": []any{0.0, 0.0, 612.0, 792.0},
		"children": []any{
			map[string]any{"type": "heading", "content": "Title", "page number": 1.0, "bounding box": []any{72.0, 700.0, 400.0, 730.0}},
			map[string]any{"type": "table", "html": "<table><tr><td>a</td></tr></table>", "page number": 1.0, "bounding box": []any{72.0, 400.0, 500.0, 600.0}},
			map[string]any{"type": "image", "page number": 1.0, "bounding box": []any{72.0, 100.0, 300.0, 300.0}},
			map[string]any{"type": "formula", "content": "a+b", "page number": 1.0, "bounding box": []any{72.0, 50.0, 200.0, 80.0}},
		},
	}
	sizes := map[int][2]float64{0: {612, 792}}
	items := openDataLoaderItemsWithGeometry(root, sizes)
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4: %v", len(items), items)
	}
	for i, item := range items {
		rows, ok := item["_pdf_positions"].([][]any)
		if !ok || len(rows) != 1 || rows[0][0] != 1.0 {
			t.Errorf("item %d (%v) positions = %#v, want one page-1 row", i, item["layout"], item["_pdf_positions"])
		}
	}
}

func TestOpenDataLoaderItems_NoGeometryNoPositions(t *testing.T) {
	root := map[string]any{"type": "paragraph", "content": "body", "page number": 1.0, "bounding box": []any{72.0, 650.5, 540.0, 700.2}}
	if items := openDataLoaderItemsWithGeometry(root, nil); len(items) != 1 || items[0]["_pdf_positions"] != nil {
		t.Fatalf("items = %v, want one item without positions when page geometry is unavailable", items)
	} else if items[0]["page_number"] != 1 {
		t.Errorf("page_number = %#v, want 1 (page identity survives without geometry)", items[0]["page_number"])
	}
	// A page the geometry map does not cover behaves the same way.
	sizes := map[int][2]float64{2: {612, 792}}
	if items := openDataLoaderItemsWithGeometry(root, sizes); len(items) != 1 || items[0]["_pdf_positions"] != nil {
		t.Fatalf("items = %v, want no positions for an uncovered page", items)
	}
}

func TestOpenDataLoaderItems_BadBoxesAreDropped(t *testing.T) {
	for _, el := range []map[string]any{
		{"type": "paragraph", "content": "no page", "bounding box": []any{1.0, 2.0, 3.0, 4.0}},
		{"type": "paragraph", "content": "no box", "page number": 1.0},
		{"type": "paragraph", "content": "short box", "page number": 1.0, "bounding box": []any{1.0, 2.0}},
		{"type": "paragraph", "content": "zero width", "page number": 1.0, "bounding box": []any{5.0, 2.0, 5.0, 4.0}},
		{"type": "paragraph", "content": "inverted", "page number": 1.0, "bounding box": []any{200.0, 300.0, 100.0, 100.0}},
		{"type": "paragraph", "content": "page zero", "page number": 0.0, "bounding box": []any{1.0, 2.0, 3.0, 4.0}},
	} {
		items := openDataLoaderItemsWithGeometry(el, map[int][2]float64{0: {612, 792}})
		if len(items) != 1 {
			t.Fatalf("%v: items = %d, want 1", el["content"], len(items))
		}
		if items[0]["_pdf_positions"] != nil {
			t.Errorf("%v: positions = %#v, want none", el["content"], items[0]["_pdf_positions"])
		}
	}
}

// TestOpenDataLoaderItems_BoxClampedToPage keeps an off-page box from
// producing a highlight outside the page rectangle.
func TestOpenDataLoaderItems_BoxClampedToPage(t *testing.T) {
	el := map[string]any{
		"type": "paragraph", "content": "bleed", "page number": 1.0,
		"bounding box": []any{-10.0, -10.0, 700.0, 900.0},
	}
	items := openDataLoaderItemsWithGeometry(el, map[int][2]float64{0: {612, 792}})
	rows, ok := items[0]["_pdf_positions"].([][]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("positions = %#v, want one row", items[0]["_pdf_positions"])
	}
	want := []float64{0, 612, 0, 792}
	for i, v := range want {
		if got, _ := numericAny(rows[0][i+1]); math.Abs(got-v) > 1e-6 {
			t.Fatalf("positions = %#v, want the page rectangle %v", rows, want)
		}
	}
}

// TestOpenDataLoaderReportedPageCount pins the geometry-free fallback: the
// root's total wins; otherwise the HIGHEST page number, not the number of
// distinct pages (a sparse document would otherwise undercount); otherwise 1.
func TestOpenDataLoaderReportedPageCount(t *testing.T) {
	cases := []struct {
		name string
		root any
		want int
	}{
		{"root total", map[string]any{"number of pages": 7, "children": []any{
			map[string]any{"type": "paragraph", "content": "a", "page number": 1.0}}}, 7},
		{"highest page", map[string]any{"children": []any{
			map[string]any{"type": "paragraph", "content": "a", "page number": 3.0},
			map[string]any{"type": "paragraph", "content": "b", "page number": 1.0}}}, 3},
		{"no page keys", map[string]any{"type": "paragraph", "content": "a"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := openDataLoaderReportedPageCount(c.root); got != c.want {
				t.Errorf("page count = %d, want %d", got, c.want)
			}
		})
	}
}
