package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// columnCountOf returns the number of distinct columns reported by AssignColumn
// for the given page boxes (mirrors the harness's ColID tally).
func columnCountOf(boxes []pdf.TextBox) int {
	res := AssignColumn(boxes)
	k := 1
	for _, b := range res {
		if b.ColID+1 > k {
			k = b.ColID + 1
		}
	}
	return k
}

// addBox appends a text box spanning [x0,x1] at the next vertical slot.
func addBox(boxes *[]pdf.TextBox, x0, x1 float64) {
	top := float64(len(*boxes)) * 12
	*boxes = append(*boxes, pdf.TextBox{
		PageNumber: 0, X0: x0, X1: x1, Top: top, Bottom: top + 10,
		LayoutType: pdf.LayoutTypeText, Text: "b",
	})
}

// TestGapColumnWidthsCapsHistogram proves the OOM guard: an absurd page extent
// must not allocate one histogram bin per point. The call is expected to return
// quickly with a bounded bin count instead of trying a multi-GB allocation.
func TestGapColumnWidthsCapsHistogram(t *testing.T) {
	// The shared bin helper must widen the bin instead of allocating one bin
	// per point for an absurd extent.
	effBin, nb := columnHistogramBins(1e9, binPt)
	if nb > maxBins+1 {
		t.Fatalf("bin count %d exceeds cap %d", nb, maxBins)
	}
	if effBin <= binPt {
		t.Fatalf("bin width %v was not widened for a 1e9 extent", effBin)
	}
	// Normal page extents keep the configured resolution.
	if eff, n := columnHistogramBins(600, binPt); eff != binPt || n != 301 {
		t.Fatalf("normal extent bins = (%v, %d), want (%v, 301)", eff, n, binPt)
	}

	var lines []pdf.TextBox
	for i := 0; i < 20; i++ {
		lines = append(lines, pdf.TextBox{X0: 100, X1: 300})
	}
	for i := 0; i < 20; i++ {
		lines = append(lines, pdf.TextBox{X0: 400, X1: 600})
	}
	lines = append(lines, pdf.TextBox{X0: 100, X1: 1e9}) // malformed extent

	widths := gapColumnWidths(lines)
	if len(widths) == 0 {
		t.Fatal("gapColumnWidths returned no widths for a two-column page")
	}
	for _, w := range widths {
		if w <= 0 || w > 1e9 {
			t.Fatalf("implausible column width %v", w)
		}
	}
}

// TestTextProjectionLinesStripsFigures proves A1: image and equation
// placeholders must not feed the column projection (a figure spanning the
// gutter would otherwise mask a real column boundary). Text and table boxes
// are kept.
func TestTextProjectionLinesStripsFigures(t *testing.T) {
	lines := []pdf.TextBox{
		{LayoutType: pdf.LayoutTypeText, X0: 100, X1: 300},
		{LayoutType: pdf.LayoutTypeFigure, X0: 50, X1: 700},
		{LayoutType: pdf.LayoutTypeEquation, X0: 50, X1: 700},
		{LayoutType: pdf.LayoutTypeTable, X0: 100, X1: 300},
		{LayoutType: pdf.LayoutTypeText, X0: 400, X1: 600},
		{LayoutType: "", X0: 100, X1: 300}, // empty type is treated as text
	}
	out := textProjectionLines(lines)
	if len(out) != 4 {
		t.Fatalf("textProjectionLines kept %d lines, want 4 (text+table+text+empty), dropped %d",
			len(out), len(lines)-len(out))
	}
	for _, b := range out {
		if b.LayoutType == pdf.LayoutTypeFigure || b.LayoutType == pdf.LayoutTypeEquation {
			t.Errorf("figure/equation box leaked into projection: %+v", b)
		}
	}
}

// TestRobustPageExtentDropsOutlier proves A2: a single malformed box with an
// absurd width must not inflate the page extent; the extent of the real text
// body must be returned instead. Uses a realistic page (many real lines) so the
// lone outlier is a true minority (< maxTrimFraction).
func TestRobustPageExtentDropsOutlier(t *testing.T) {
	var lines []pdf.TextBox
	for i := 0; i < 20; i++ {
		lines = append(lines, pdf.TextBox{X0: 100, X1: 300}) // left column
	}
	for i := 0; i < 20; i++ {
		lines = append(lines, pdf.TextBox{X0: 400, X1: 600}) // right column
	}
	lines = append(lines, pdf.TextBox{X0: 100, X1: 1e6}) // malformed outlier
	minX0, width := robustPageExtent(lines)
	wantMin, wantWidth := 100.0, 500.0
	if minX0 != wantMin || width != wantWidth {
		t.Fatalf("robustPageExtent = (%.0f, %.0f), want (%.0f, %.0f) (outlier must be dropped)",
			minX0, width, wantMin, wantWidth)
	}
}

// TestRobustPageExtentNotTrimsRealFarColumns proves the A2 guard: two real
// columns placed far apart (but within a sane page span) must NOT be trimmed
// to one. The gap between them is large but below maxPageExtent, so no split.
func TestRobustPageExtentNotTrimsRealFarColumns(t *testing.T) {
	lines := []pdf.TextBox{
		{X0: 100, X1: 300},   // left column
		{X0: 2000, X1: 2200}, // right column, far but < maxPageExtent away
	}
	minX0, width := robustPageExtent(lines)
	// Extent must span BOTH columns: 100 .. 2200.
	if minX0 != 100.0 || width != 2100.0 {
		t.Fatalf("robustPageExtent = (%.0f, %.0f), want (100, 2100) (real far columns kept)",
			minX0, width)
	}
}

// TestSyntheticOutlierBoxKeepsColumns proves A2 end-to-end: a clean double
// column plus a malformed box must still be detected as 2 columns (the outlier
// must not collapse detection to 1 via a ballooned extent).
func TestSyntheticOutlierBoxKeepsColumns(t *testing.T) {
	var boxes []pdf.TextBox
	for i := 0; i < 14; i++ {
		addBox(&boxes, 100, 350) // left column
	}
	for i := 0; i < 6; i++ {
		addBox(&boxes, 450, 700) // right column
	}
	// Malformed box: normal left edge but absurd right edge.
	boxes = append(boxes, pdf.TextBox{
		PageNumber: 0, X0: 100, X1: 1e6, Top: 9999, Bottom: 10009,
		LayoutType: pdf.LayoutTypeText, Text: "bad",
	})
	k := gapColumnCount(boxes, 0.04, 0.15, 2.0)
	if k != 2 {
		t.Fatalf("outlier box collapsed detection to %d column(s); want 2", k)
	}
}

// TestSyntheticFigureSpanningGutter proves A1 end-to-end on the 1D gap path: a
// full-width figure that bridges the gutter must not fill the projection and
// mask the two real text columns.
func TestSyntheticFigureSpanningGutter(t *testing.T) {
	var boxes []pdf.TextBox
	for i := 0; i < 14; i++ {
		addBox(&boxes, 100, 350) // left column
	}
	for i := 0; i < 6; i++ {
		addBox(&boxes, 450, 700) // right column
	}
	// Full-width figure spanning the gutter.
	boxes = append(boxes, pdf.TextBox{
		PageNumber: 0, X0: 100, X1: 700, Top: 5000, Bottom: 5100,
		LayoutType: pdf.LayoutTypeFigure, Text: "fig",
	})
	k := gapColumnCount(boxes, 0.04, 0.15, 2.0)
	if k != 2 {
		t.Fatalf("figure spanning gutter collapsed detection to %d column(s); want 2", k)
	}
}

// TestDetectColumnCount2D_IgnoresFigureSpanningGutter proves A1 extends to the
// 2D rescue projection: a figure that spans the gutter must not fill the
// projection and hide a real two-column valley. Without A1 the figure masks
// the gutter and detectColumnCount2D returns 0 (miss); with A1 it returns 2.
func TestDetectColumnCount2D_IgnoresFigureSpanningGutter(t *testing.T) {
	var boxes []pdf.TextBox
	for i := 0; i < 14; i++ {
		addBox(&boxes, 100, 340) // left column
	}
	for i := 0; i < 6; i++ {
		addBox(&boxes, 460, 700) // right column
	}
	// Figure spanning the gutter: wide enough to survive dropWide (>=0.6*width
	// would drop it, so keep it just under) but not full-width. It fills the
	// projection across the real column boundary and hides the valley pre-A1.
	boxes = append(boxes, pdf.TextBox{
		PageNumber: 0, X0: 100, X1: 455, Top: 5000, Bottom: 5100,
		LayoutType: pdf.LayoutTypeFigure, Text: "fig",
	})
	k, _ := detectColumnCount2D(boxes)
	if k != 2 {
		t.Fatalf("figure spanning gutter hid the 2D valley: detectColumnCount2D=%d, want 2", k)
	}
}
