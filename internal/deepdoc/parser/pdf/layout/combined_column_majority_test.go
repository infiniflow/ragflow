package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// xingfaPage1Boxes reproduces 刑法.pdf page 1 detect boxes: an indent-heavy
// single-column page whose staggered x0 (footnote 80, body 111-119, indents
// 144/176/270) trips the per-page gap detector into 2 columns.
var xingfaPage1Boxes = [][4]float64{ // x0, top, x1, bottom
	{111.7, 955.0, 490.0, 1348.0},
	{113.3, 985.0, 489.0, 997.3},
	{119.7, 1012.0, 489.0, 1027.0},
	{113.3, 1043.0, 487.3, 1055.0},
	{113.3, 1071.0, 490.0, 1085.7},
	{115.3, 1100.7, 488.0, 1113.0},
	{112.7, 1128.7, 490.0, 1143.7},
	{111.7, 1156.0, 489.0, 1171.0},
	{113.3, 1187.7, 488.0, 1200.0},
	{112.7, 1216.7, 487.3, 1228.7},
	{111.7, 1243.7, 489.0, 1258.7},
	{119.7, 1273.7, 488.0, 1288.3},
	{113.3, 1302.3, 489.0, 1317.3},
	{111.7, 1329.7, 341.7, 1348.0},
	{270.3, 1389.3, 334.0, 1405.0},
	{113.3, 1418.3, 205.7, 1433.0},
	{144.3, 1447.3, 441.3, 1462.0},
	{144.3, 1476.0, 236.7, 1491.0},
	{176.3, 1505.0, 347.3, 1520.0},
	{79.7, 1547.3, 521.0, 1583.0},
	{80.7, 1568.3, 324.3, 1583.0},
}

func makeBoxes(coords [][4]float64, page int) []pdf.TextBox {
	out := make([]pdf.TextBox, len(coords))
	for i, r := range coords {
		out[i] = pdf.TextBox{
			X0: r[0], Top: r[1], X1: r[2], Bottom: r[3],
			PageNumber: page, LayoutType: pdf.LayoutTypeText,
		}
	}
	return out
}

// TestAssignColumnSingleColumnMajority locks the document-wide column-majority
// rule (mirroring Python's _assign_column global_cols): when most pages read
// as a single column, EVERY page is assigned ColID 0 — even an indent-heavy
// page whose staggered x0 would trip the per-page gap detector into 2 columns
// (刑法's TOC page). Without the majority rule that page scrambles the reading
// order.
func TestAssignColumnSingleColumnMajority(t *testing.T) {
	var boxes []pdf.TextBox
	boxes = append(boxes, makeBoxes(xingfaPage1Boxes, 0)...) // indent page -> per-page k>=2
	for pg := 1; pg <= 2; pg++ {
		for i := 0; i < 8; i++ {
			boxes = append(boxes, pdf.TextBox{
				X0: 100, Top: float64(50 + 14*i), X1: 400, Bottom: float64(62 + 14*i),
				PageNumber: pg, LayoutType: pdf.LayoutTypeText,
			})
		}
	}
	out := AssignColumn(boxes)
	for i, b := range out {
		if b.ColID != 0 {
			t.Fatalf("box %d (page %d) ColID=%d, want 0 (single-column majority)", i, b.PageNumber, b.ColID)
		}
	}
}

// TestAssignColumnTieFirstPageWins locks the majority tie-break: with one
// two-column page and one single-column page the count ties, and Python's
// Counter.most_common keeps the first-seen column count (the two-column page
// appears first) — so the two-column split is kept.
func TestAssignColumnTieFirstPageWins(t *testing.T) {
	var boxes []pdf.TextBox
	// page 0: genuine two columns
	for i := 0; i < 4; i++ {
		top := float64(50 + 30*i)
		boxes = append(boxes,
			pdf.TextBox{X0: 50, Top: top, X1: 250, Bottom: top + 20, PageNumber: 0, LayoutType: pdf.LayoutTypeText},
			pdf.TextBox{X0: 300, Top: top, X1: 500, Bottom: top + 20, PageNumber: 0, LayoutType: pdf.LayoutTypeText},
		)
	}
	// page 1: single column
	for i := 0; i < 4; i++ {
		top := float64(50 + 14*i)
		boxes = append(boxes, pdf.TextBox{X0: 100, Top: top, X1: 400, Bottom: top + 12, PageNumber: 1, LayoutType: pdf.LayoutTypeText})
	}
	out := AssignColumn(boxes)
	colSeen := map[int]bool{}
	for _, b := range out {
		if b.PageNumber == 0 {
			colSeen[b.ColID] = true
		}
	}
	if len(colSeen) < 2 {
		t.Fatalf("tie must keep the first-seen two-column split, got page0 ColIDs %v", colSeen)
	}
}

// TestAssignColumnMultiColumnMajorityKeepsSplit locks that a document whose
// majority of pages are genuinely two-column keeps the per-page split: the
// majority rule must NOT collapse a real multi-column document to one.
func TestAssignColumnMultiColumnMajorityKeepsSplit(t *testing.T) {
	// Two genuine two-column pages: left column x0=50, right column x0=300.
	var boxes []pdf.TextBox
	for pg := 0; pg < 3; pg++ {
		for i := 0; i < 5; i++ {
			top := float64(50 + 30*i)
			boxes = append(boxes,
				pdf.TextBox{X0: 50, Top: top, X1: 250, Bottom: top + 20, PageNumber: pg, LayoutType: pdf.LayoutTypeText},
				pdf.TextBox{X0: 300, Top: top, X1: 500, Bottom: top + 20, PageNumber: pg, LayoutType: pdf.LayoutTypeText},
			)
		}
	}
	out := AssignColumn(boxes)
	colSeen := map[int]bool{}
	for _, b := range out {
		colSeen[b.ColID] = true
	}
	if len(colSeen) < 2 {
		t.Fatalf("multi-column majority document must keep two columns, got ColIDs %v", colSeen)
	}
}
