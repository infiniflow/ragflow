package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMergeTablesAcrossPages_TopTablesOnConsecutivePagesNotMerged locks the
// ZoomNeXt regression: Table 3 sits at the TOP of page 11 and Table 5 sits at
// the TOP of page 12. Both are complete, top-of-page tables whose page-local Y
// repeats near 0, so the continuation's page-local yDis is NEGATIVE. #18740's
// "negative yDis ⇒ merge" rule wrongly merges them, which (a) gives Table 3 a
// false cross-page span (pages 11-12) and (b) drops Table 5 entirely from the
// output. A genuine cross-page continuation only exists when the ANCHOR table
// reaches near the BOTTOM of its page; a top-of-page anchor is a complete
// table, and the next page's top table is a NEW table.
//
// Geometry mirrors ZoomNeXt: anchor first box bottom ~81 (near the table top),
// but the table's true bottom extent ~198; continuation is at the very top of
// the next page (top 52). Page height 842, median char height 10.
func TestMergeTablesAcrossPages_TopTablesOnConsecutivePagesNotMerged(t *testing.T) {
	anchor := pdf.TableItem{
		Page: 10,
		Positions: []pdf.Position{
			{PageNumbers: []int{10}, Left: 226, Right: 457, Top: 61, Bottom: 81},   // first OCR box, near table top
			{PageNumbers: []int{10}, Left: 117, Right: 473, Top: 189, Bottom: 198}, // true bottom extent of the table
		},
		Cells: []pdf.TSRCell{{Text: "No. Model"}},
	}
	cont := pdf.TableItem{
		Page: 11,
		Positions: []pdf.Position{
			{PageNumbers: []int{11}, Left: 143, Right: 444, Top: 52, Bottom: 61},
		},
		Cells: []pdf.TSRCell{{Text: "Input Scale"}},
	}
	pageH := map[int]float64{10: 842, 11: 842}
	medianH := map[int]float64{10: 10, 11: 10}

	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, cont}, medianH, pageH)
	if len(merged) != 2 {
		t.Fatalf("top-of-page tables on consecutive pages: expected 2 SEPARATE tables (no over-merge), got %d (Table 5 dropped)", len(merged))
	}
	// The anchor must NOT have gained page 11.
	pages := map[int]bool{}
	for _, p := range merged[0].Positions {
		for _, pn := range p.PageNumbers {
			pages[pn] = true
		}
	}
	if pages[11] {
		t.Errorf("anchor wrongly merged across pages; pages=%v (want only page 10)", pages)
	}
}
