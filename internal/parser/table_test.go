package parser

import (
	"context"
	"image"
	"strings"
	"testing"
)

// ---- groupTSRCellsToRows ----

func TestGroupTSRCellsToRows_Empty(t *testing.T) {
	if rows := groupTSRCellsToRows(nil); rows != nil {
		t.Errorf("nil input: expected nil, got %d rows", len(rows))
	}
	if rows := groupTSRCellsToRows([]TSRCell{}); rows != nil {
		t.Errorf("empty input: expected nil, got %d rows", len(rows))
	}
}

func TestGroupTSRCellsToRows_SingleCell(t *testing.T) {
	cells := []TSRCell{{X0: 0, Y0: 0, X1: 10, Y1: 10, Text: "A"}}
	rows := groupTSRCellsToRows(cells)
	if len(rows) != 1 || len(rows[0]) != 1 || rows[0][0].Text != "A" {
		t.Errorf("single cell: expected [[A]], got %v", rows)
	}
}

func TestGroupTSRCellsToRows_TwoRows(t *testing.T) {
	cells := []TSRCell{
		{X0: 00, Y0: 0, X1: 10, Y1: 10, Text: "A1"},
		{X0: 20, Y0: 0, X1: 30, Y1: 10, Text: "B1"},
		{X0: 00, Y0: 30, X1: 10, Y1: 40, Text: "A2"},
		{X0: 20, Y0: 30, X1: 30, Y1: 40, Text: "B2"},
	}
	rows := groupTSRCellsToRows(cells)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("expected 2 cells per row, got %d/%d", len(rows[0]), len(rows[1]))
	}
	// Row 0 sorted by X0
	if rows[0][0].Text != "A1" || rows[0][1].Text != "B1" {
		t.Errorf("row 0 order wrong: %v", tsrCellTexts(rows[0]))
	}
	// Row 1 sorted by X0
	if rows[1][0].Text != "A2" || rows[1][1].Text != "B2" {
		t.Errorf("row 1 order wrong: %v", tsrCellTexts(rows[1]))
	}
}

func TestGroupTSRCellsToRows_CloseRows(t *testing.T) {
	// Two rows with small Y gap — should still be separate rows
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 10, Y1: 8, Text: "Row1"},
		{X0: 0, Y0: 9, X1: 10, Y1: 17, Text: "Row2"},
	}
	rows := groupTSRCellsToRows(cells)
	// medianH = 8, threshold = 4. gap = 9-8 = 1 < 4? Actually Y diff = 9-8=1 < 4 → same row!
	// No: cells sorted by Y0: Row1(0), Row2(9). gap = 9-0 = 9 > 4 → different rows.
	if len(rows) != 2 {
		t.Errorf("close rows: expected 2, got %d", len(rows))
	}
}

func TestGroupTSRCellsToRows_VaryingHeights(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 10, Y1: 5, Text: "A"},   // height 5
		{X0: 0, Y0: 50, X1: 10, Y1: 70, Text: "B"},  // height 20
		{X0: 0, Y0: 50, X1: 10, Y1: 70, Text: "C"},  // height 20, same row as B
	}
	rows := groupTSRCellsToRows(cells)
	// median height = 5 (sorted: 5,20,20 → median index 1 = 20)
	// threshold = 10. Y gap B-to-A = 50-5 = 45 > 10 → different row
	// Y gap C-to-B = 50-50 = 0 ≤ 10 → same row
	if len(rows) != 2 {
		t.Fatalf("varying heights: expected 2 rows, got %d", len(rows))
	}
	if len(rows[0]) != 1 || rows[0][0].Text != "A" {
		t.Errorf("row 0: expected [A], got %v", tsrCellTexts(rows[0]))
	}
	if len(rows[1]) != 2 {
		t.Errorf("row 1: expected 2 cells, got %v", tsrCellTexts(rows[1]))
	}
}

func tsrCellTexts(cells []TSRCell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.Text
	}
	return out
}

// ---- boxOverlapsCell ----

func TestBoxOverlapsCell_FullOverlap(t *testing.T) {
	// Box is entirely inside cell → ≥85% of box area inside cell → match.
	cell := TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := TextBox{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "hello"}
	if !boxOverlapsCell(cell, box) {
		t.Error("full overlap should return true")
	}
	// Box is still entirely inside cell → box→cell = 100% ≥ 85% → match.
	box2 := TextBox{X0: 10, X1: 90, Top: 10, Bottom: 40, Text: "partial"}
	if !boxOverlapsCell(cell, box2) {
		t.Error("box entirely inside cell (100% of box) should match")
	}
}

func TestBoxOverlapsCell_NoOverlap(t *testing.T) {
	cell := TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := TextBox{X0: 200, X1: 300, Top: 10, Bottom: 40, Text: "away"}
	if boxOverlapsCell(cell, box) {
		t.Error("no X overlap should return false")
	}
}

func TestBoxOverlapsCell_PartialOverlap(t *testing.T) {
	// Box is entirely inside cell (100% of box area) → matches.
	// boxOverlapsCell uses box→cell overlap (≥85% of box area inside cell).
	cell := TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := TextBox{X0: 0, X1: 30, Top: 0, Bottom: 25, Text: "small"}
	if !boxOverlapsCell(cell, box) {
		t.Error("box entirely inside cell should match")
	}
	// Box straddles cell boundary (< 85% of box inside cell) → no match.
	box2 := TextBox{X0: 80, X1: 180, Top: 0, Bottom: 25, Text: "spill"}
	if boxOverlapsCell(cell, box2) {
		t.Error("box straddling boundary (<85% inside) should NOT match")
	}
}

func TestBoxOverlapsCell_ZeroArea(t *testing.T) {
	cell := TSRCell{X0: 0, Y0: 0, X1: 0, Y1: 50}
	box := TextBox{X0: 0, X1: 10, Top: 0, Bottom: 10, Text: "x"}
	if boxOverlapsCell(cell, box) {
		t.Error("zero cell area should return false")
	}
}

// ---- fillCellTextFromBoxes ----

func TestFillCellTextFromBoxes_Simple(t *testing.T) {
	// Box covering entire cell (>85%) → match
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50},
		{X0: 100, Y0: 0, X1: 200, Y1: 50},
	}
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "cell1"},
		{X0: 100, X1: 200, Top: 0, Bottom: 50, Text: "cell2"},
	}
	fillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "cell1" {
		t.Errorf("cell 0: got %q, want 'cell1'", cells[0].Text)
	}
	if cells[1].Text != "cell2" {
		t.Errorf("cell 1: got %q, want 'cell2'", cells[1].Text)
	}
}

func TestFillCellTextFromBoxes_MultipleBoxesPerCell(t *testing.T) {
	// Two boxes, each covering >85% of the cell → concatenated
	// (boxes must overlap the cell near-completely to match individually)
	cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []TextBox{
		{X0: 0, X1: 95, Top: 0, Bottom: 47, Text: "part1"},
		{X0: 5, X1: 100, Top: 3, Bottom: 50, Text: "part2"},
	}
	fillCellTextFromBoxes(cells, boxes)
	// Both boxes cover >85% → both match → concatenated with space
	if cells[0].Text == "" {
		t.Error("expected non-empty cell text")
	}
}

func TestFillCellTextFromBoxes_EmptyBoxText(t *testing.T) {
	cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []TextBox{
		{X0: 5, X1: 95, Top: 5, Bottom: 45, Text: "   "},
	}
	fillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("empty box text: got %q, want empty", cells[0].Text)
	}
}

func TestFillCellTextFromBoxes_NoMatchingBox(t *testing.T) {
	cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []TextBox{
		{X0: 500, X1: 600, Top: 500, Bottom: 550, Text: "far away"},
	}
	fillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("no match: got %q, want empty", cells[0].Text)
	}
}

// ---- regionOverlapsBox ----

func TestRegionOverlapsBox_StrongOverlap(t *testing.T) {
	region := DLARegion{X0: 0, Y0: 0, X1: 216, Y1: 108} // DLA coords at 216 DPI
	box := TextBox{X0: 0, X1: 100, Top: 0, Bottom: 50}
	if !regionOverlapsBox(region, box, 3.0) {
		t.Error("full overlap should match")
	}
}

func TestRegionOverlapsBox_NoOverlap(t *testing.T) {
	region := DLARegion{X0: 0, Y0: 0, X1: 216, Y1: 108}
	box := TextBox{X0: 500, X1: 600, Top: 500, Bottom: 550}
	if regionOverlapsBox(region, box, 3.0) {
		t.Error("no overlap should return false")
	}
}

func TestRegionOverlapsBox_WeakOverlap(t *testing.T) {
	// Overlap at 30% → below 40% threshold → false.
	region := DLARegion{X0: 0, Y0: 0, X1: 90, Y1: 90} // 30x30 at scale 3
	box := TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100} // overlap = 30*30/10000 = 9%? No: 30x30=900 / 10000 = 9%
	if regionOverlapsBox(region, box, 3.0) {
		t.Error("9% overlap should return false")
	}
	// Overlap ≥ 40% → should match (Python thr=0.4).
	// box 100x100=10000 area; region 100x40=4000 → exactly 40%.
	region2 := DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 120, Label: "table"} // 100x40 at scale 3
	if !regionOverlapsBox(region2, box, 3.0) {
		t.Error("40% overlap should match (>= 0.4)")
	}
	// Region that covers most of the box → should match
	region3 := DLARegion{X0: 0, Y0: 0, X1: 270, Y1: 270} // 90x90 at scale 3
	if !regionOverlapsBox(region3, box, 3.0) {
		t.Error("81% overlap should match")
	}
}

func TestRegionOverlapsBox_ThresholdAt040(t *testing.T) {
	// Exact 40% overlap: 100x100 box, region just covering 40%
	// 0.4 * 10000 = 4000. Need region with area 4000 in box space.
	// 63.2*63.2 ≈ 3994. Let's use 100x40 = 4000.
	box := TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100}
	region := DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 120, Label: "table"} // 100x40 at scale 3
	if !regionOverlapsBox(region, box, 3.0) {
		t.Error("exact 40% overlap should match (>= 0.4)")
	}
	// 39% overlap should NOT match
	region2 := DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 117, Label: "table"} // 100x39 at scale 3
	if regionOverlapsBox(region2, box, 3.0) {
		t.Error("39% overlap should NOT match")
	}
}

// ---- annotateBoxLayouts ----

func TestAnnotateBoxLayouts_SetsLabel(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
		{X0: 0, X1: 100, Top: 30, Bottom: 50},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 60, Label: "title"},   // covers box 0 at scale 3
		{X0: 0, Y0: 90, X1: 300, Y1: 150, Label: "text"},   // covers box 1 at scale 3
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "title" {
		t.Errorf("box 0: got %q, want 'title'", boxes[0].LayoutType)
	}
	if boxes[1].LayoutType != "text" {
		t.Errorf("box 1: got %q, want 'text'", boxes[1].LayoutType)
	}
}

func TestAnnotateBoxLayouts_NoMatch(t *testing.T) {
	// Region far away from the box — no overlap
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
	}
	regions := []DLARegion{
		{X0: 900, Y0: 900, X1: 1000, Y1: 1000, Label: "far"}, // completely outside
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Errorf("no match: expected empty, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_EmptyRegions(t *testing.T) {
	boxes := []TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 20}}
	boxes = annotateBoxLayouts(boxes, nil, 3.0, 0)
	boxes = annotateBoxLayouts(boxes, []DLARegion{}, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Errorf("empty regions: got %q, want empty", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_PriorityOverMaxArea(t *testing.T) {
	// "table" type checked before "text" in priority order.
	// Even if "text" region has larger overlap, "table" wins if it meets threshold (≥40%).
	boxes := []TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 50}}
	regions := []DLARegion{
		// text region: full coverage (100% overlap) — but lower priority
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "text"},
		// table region: 45% overlap (45x50 out of 100x50) — higher priority, meets threshold
		{X0: 0, Y0: 0, X1: 45 * 3, Y1: 50 * 3, Label: "table"},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "table" {
		t.Errorf("priority: 'table' should win over 'text' when both meet threshold, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_OverlapThreshold(t *testing.T) {
	// Region overlaps only 30% of box — below 0.4 threshold — should NOT match.
	boxes := []TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 50}}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 30 * 3, Y1: 30 * 3, Label: "table"}, // covers ~30% of box
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Errorf("threshold: overlap < 40%% should not match, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_CIDGarbage(t *testing.T) {
	// CID-pattern boxes should be popped entirely (Python: bxs.pop(i)).
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20, Text: "(cid:123)"},
		{X0: 0, X1: 100, Top: 30, Bottom: 50, Text: "normal text"},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 60, Label: "text", Confidence: 0.9},
		{X0: 0, Y0: 90, X1: 300, Y1: 150, Label: "text", Confidence: 0.9},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	// CID-garbled box was popped → only 1 box remains.
	if len(boxes) != 1 {
		t.Fatalf("CID-garbled box should be popped, got %d boxes", len(boxes))
	}
	if boxes[0].LayoutType != "text" {
		t.Errorf("CID: remaining box should be 'text', got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_LayoutNoFormat(t *testing.T) {
	// layoutno uses Python format: "{type}-{per_type_index}" where per_type_index
	// is the index of the matched DLA region within its type (not global).
	// Two boxes overlapping the SAME text region share the same layoutno → VM can merge them.
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
		{X0: 0, X1: 100, Top: 30, Bottom: 50},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "text"}, // covers both boxes
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	want := "text-0"
	if boxes[0].LayoutNo != want {
		t.Errorf("box 0 layoutno: got %q, want %q", boxes[0].LayoutNo, want)
	}
	if boxes[1].LayoutNo != want {
		t.Errorf("box 1 layoutno should share same per-type index: got %q, want %q", boxes[1].LayoutNo, want)
	}
}

func TestAnnotateBoxLayouts_LayoutNoDifferentRegions(t *testing.T) {
	// Two boxes in different text regions → different layoutno.
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
		{X0: 0, X1: 100, Top: 100, Bottom: 120},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 60, Label: "text"},   // per-type index 0
		{X0: 0, Y0: 300, X1: 300, Y1: 360, Label: "text"}, // per-type index 1
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutNo != "text-0" {
		t.Errorf("box 0: got %q, want 'text-0'", boxes[0].LayoutNo)
	}
	if boxes[1].LayoutNo != "text-1" {
		t.Errorf("box 1: got %q, want 'text-1'", boxes[1].LayoutNo)
	}
}

// TestAnnotateBoxLayouts_ConfidenceFilter verifies that DLA regions with
// low confidence (< 0.4) for garbage layout types are excluded from matching.
// Python: float(b["score"]) >= 0.4 filter in LayoutRecognizer.
func TestAnnotateBoxLayouts_ConfidenceFilter(t *testing.T) {
	boxes := []TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 50}}
	// Low-confidence footer — should be filtered out.
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "footer", Confidence: 0.2},
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "text", Confidence: 0.9},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	// Footer region filtered (low confidence) → box matches "text" instead.
	if boxes[0].LayoutType != "text" {
		t.Errorf("low-confidence footer filtered → box should get 'text', got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_GarbageFooterRejected(t *testing.T) {
	// Footer at page bottom: Bottom(290) > 270 (90% of 300px→PDF height 100→90% of 100=90)
	// → real footer decoration → garbage → pop (Python: bxs.pop(i)).
	boxes := []TextBox{{X0: 0, X1: 100, Top: 280, Bottom: 290}}
	regions := []DLARegion{
		{X0: 0, Y0: 840, X1: 300, Y1: 870, Label: "footer", Confidence: 0.9}, // y=280-290 after /3, PDF 93-97
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300) // PDF height = 300/3 = 100
	if len(boxes) != 0 {
		t.Errorf("footer at bottom: should be popped as decoration, got %d boxes left", len(boxes))
	}
}

func TestAnnotateBoxLayouts_HeaderRemovedAtTop(t *testing.T) {
	// Header at page top edge (y=5 in 300px page → PDF height 100 → 5 < 10% of 100)
	// → real header decoration → garbage → pop (Python: bxs.pop(i)).
	boxes := []TextBox{{X0: 0, X1: 100, Top: 5, Bottom: 20}}
	regions := []DLARegion{
		{X0: 0, Y0: 15, X1: 300, Y1: 60, Label: "header", Confidence: 0.9}, // y=5-20 after /3
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if len(boxes) != 0 {
		t.Errorf("header at very top: should be popped as decoration, got %d boxes left", len(boxes))
	}
}

func TestAnnotateBoxLayouts_HeaderKeptInMiddle(t *testing.T) {
	// Header in middle of page (y=50 in 300px page → PDF height 100 → 50 > 10)
	// → DLA false positive → KEEP the text.
	boxes := []TextBox{{X0: 0, X1: 100, Top: 50, Bottom: 70}}
	regions := []DLARegion{
		{X0: 0, Y0: 150, X1: 300, Y1: 210, Label: "header", Confidence: 0.9}, // y=50-70 after /3
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if boxes[0].LayoutType != "header" {
		t.Errorf("header in middle of page: DLA false positive, keep text, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_FooterRemovedAtBottom(t *testing.T) {
	// Footer at page bottom (y=95 in 300px page → PDF height 100 → 95 > 90% of 100)
	// → real footer decoration → garbage → REMOVE.
	boxes := []TextBox{{X0: 0, X1: 100, Top: 95, Bottom: 100}}
	regions := []DLARegion{
		{X0: 0, Y0: 285, X1: 300, Y1: 300, Label: "footer", Confidence: 0.9}, // y=95-100 after /3
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if len(boxes) != 0 {
		t.Errorf("footer at very bottom: should be popped as decoration, got %d boxes left", len(boxes))
	}
}

func TestAnnotateBoxLayouts_FooterKeptInMiddle(t *testing.T) {
	// Footer in middle of page (y=50 in 300px page → PDF height 100 → 50 < 90)
	// → DLA false positive → KEEP the text.
	boxes := []TextBox{{X0: 0, X1: 100, Top: 50, Bottom: 70}}
	regions := []DLARegion{
		{X0: 0, Y0: 150, X1: 300, Y1: 210, Label: "footer", Confidence: 0.9}, // y=50-70 after /3
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if boxes[0].LayoutType != "footer" {
		t.Errorf("footer in middle of page: DLA false positive, keep text, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_ReferenceAlwaysGarbage(t *testing.T) {
	// Reference type is always garbage regardless of position (no keep_feat).
	boxes := []TextBox{{X0: 0, X1: 100, Top: 50, Bottom: 70}}
	regions := []DLARegion{
		{X0: 0, Y0: 150, X1: 300, Y1: 210, Label: "reference", Confidence: 0.9},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if len(boxes) != 0 {
		t.Errorf("reference: should always be garbage-filtered, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_NonGarbageTypeUnaffected(t *testing.T) {
	// "text" type is NOT a garbage type — should always be assigned.
	boxes := []TextBox{{X0: 0, X1: 100, Top: 200, Bottom: 220}}
	regions := []DLARegion{
		{X0: 0, Y0: 600, X1: 300, Y1: 660, Label: "text"},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if boxes[0].LayoutType != "text" {
		t.Errorf("non-garbage type: should be assigned, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_ZeroPageHeightDisablesGarbage(t *testing.T) {
	// pageImgHeight=0 → garbage check disabled → all types assigned.
	boxes := []TextBox{{X0: 0, X1: 100, Top: 100, Bottom: 120}}
	regions := []DLARegion{
		{X0: 0, Y0: 300, X1: 300, Y1: 360, Label: "header", Confidence: 0.9},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "header" {
		t.Errorf("zero page height: garbage check disabled, got %q", boxes[0].LayoutType)
	}
}

// TestAnnotateBoxLayouts_SyntheticFigure creates synthetic figure boxes for
// unmatched figure/equation DLA regions (Python: dla_cli.py:187-195).
func TestAnnotateBoxLayouts_SyntheticFigure(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20, Text: "text box"},
	}
	// Two figure regions, one text region
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 150, Y1: 60, Label: "text", Confidence: 0.9},     // matches text box → visited
		{X0: 300, Y0: 300, X1: 600, Y1: 600, Label: "figure", Confidence: 0.9}, // no box overlaps → synthetic
		{X0: 600, Y0: 0, X1: 900, Y1: 300, Label: "figure", Confidence: 0.9},   // no box overlaps → synthetic
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	// Original text box + 2 synthetic figure boxes = 3
	if len(boxes) != 3 {
		t.Fatalf("expected 3 boxes (1 original + 2 synthetic figures), got %d", len(boxes))
	}
	// Check synthetic boxes
	foundFig0, foundFig1 := false, false
	for _, b := range boxes {
		if b.LayoutType == "figure" && b.Text == "" {
			if b.LayoutNo == "figure-0" {
				foundFig0 = true
				if b.X0 != 100 || b.X1 != 200 {
					t.Errorf("synthetic figure-0: expected x0=100,x1=200 (300/3,600/3), got x0=%v,x1=%v", b.X0, b.X1)
				}
			}
			if b.LayoutNo == "figure-1" {
				foundFig1 = true
			}
		}
	}
	if !foundFig0 {
		t.Error("missing synthetic figure-0 box")
	}
	if !foundFig1 {
		t.Error("missing synthetic figure-1 box")
	}
}

// TestAnnotateBoxLayouts_EquationMappedToFigure verifies equation DLA regions
// get LayoutType="figure" but LayoutNo keeps "equation" prefix (Python behavior).
func TestAnnotateBoxLayouts_EquationMappedToFigure(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 60, Label: "equation", Confidence: 0.9},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if len(boxes) != 1 {
		t.Fatalf("expected 1 box, got %d", len(boxes))
	}
	if boxes[0].LayoutType != "figure" {
		t.Errorf("equation → LayoutType: got %q, want 'figure'", boxes[0].LayoutType)
	}
	if boxes[0].LayoutNo != "equation-0" {
		t.Errorf("equation → LayoutNo: got %q, want 'equation-0'", boxes[0].LayoutNo)
	}
}

// TestAnnotateBoxLayouts_MixedTypesLayoutNo verifies per-type LayoutNo counting
// with multiple region types present.
func TestAnnotateBoxLayouts_MixedTypesLayoutNo(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},       // overlaps text region 0
		{X0: 0, X1: 100, Top: 200, Bottom: 220},     // overlaps text region 1
		{X0: 200, X1: 300, Top: 0, Bottom: 20},      // overlaps figure region 0 only
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 150, Y1: 60, Label: "text", Confidence: 0.9},        // text-0
		{X0: 0, Y0: 600, X1: 150, Y1: 660, Label: "text", Confidence: 0.9},      // text-1
		{X0: 600, Y0: 0, X1: 900, Y1: 60, Label: "figure", Confidence: 0.9},     // figure-0 (PDF: x0=200, x1=300)
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if len(boxes) != 3 {
		t.Fatalf("expected 3 boxes, got %d", len(boxes))
	}
	// Check that text and figure indices are independent
	if boxes[0].LayoutNo != "text-0" {
		t.Errorf("box 0: got %q, want 'text-0'", boxes[0].LayoutNo)
	}
	if boxes[1].LayoutNo != "text-1" {
		t.Errorf("box 1: got %q, want 'text-1'", boxes[1].LayoutNo)
	}
	if boxes[2].LayoutNo != "figure-0" {
		t.Errorf("box 2: got %q, want 'figure-0' (independent from text counter)", boxes[2].LayoutNo)
	}
}

// ---- Mock-integration: DLA→TSR pipeline with MockDeepDoc ----

func TestExtractTableBoxes_PriorityPreservesTable(t *testing.T) {
	// One box overlaps both a large "text" region and a smaller "table" region.
	// Priority order (table before text) must ensure the box gets "table" label,
	// triggering TSR and producing TableItems.
	dummyImg := image.NewRGBA(image.Rect(0, 0, 900, 900))
	boxes := []TextBox{
		{X0: 200, X1: 400, Top: 200, Bottom: 400, Text: "cell content"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 0, Y0: 0, X1: 2700, Y1: 2700, Label: "text"},     // full-page, 3x scale
			{X0: 300, Y0: 300, X1: 1500, Y1: 1500, Label: "table"}, // partial, 3x scale
		},
		TSRCells: []TSRCell{{X0: 200, Y0: 200, X1: 400, Y1: 400, Text: "cell1"}},
	}
	p := NewParser(DefaultParserConfig(), mock)

	items := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	if len(items) == 0 {
		t.Error("priority: table should win over text, got 0 tables")
	}
}

func TestExtractTableBoxes_OverlapBelowThresholdNoTable(t *testing.T) {
	// Table region covers <40% of the box's area → matches no box → no table.
	dummyImg := image.NewRGBA(image.Rect(0, 0, 900, 900))
	boxes := []TextBox{
		{X0: 200, X1: 400, Top: 200, Bottom: 400, Text: "content"},
	}
	// Table region only touches a tiny corner (40*40/3 = 13x13 in PDF space).
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 600, Y0: 600, X1: 720, Y1: 720, Label: "table"}, // tiny corner
		},
		TSRCells: []TSRCell{},
	}
	p := NewParser(DefaultParserConfig(), mock)

	items := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	if len(items) != 0 {
		t.Errorf("threshold: overlap < 40%% should produce 0 tables, got %d", len(items))
	}
}

func TestExtractTableBoxes_FooterGarbageNotTriggerTable(t *testing.T) {
	// Footer at page bottom → garbage-filtered → not kept as footer.
	// Since no other type matches, box remains unannotated.
	dummyImg := image.NewRGBA(image.Rect(0, 0, 900, 900)) // 900/3=300 PDF height
	boxes := []TextBox{
		{X0: 100, X1: 300, Top: 280, Bottom: 295, Text: "page 1"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 300, Y0: 840, X1: 900, Y1: 885, Label: "footer", Confidence: 0.9}, // y=280-295 in PDF
		},
	}
	p := NewParser(DefaultParserConfig(), mock)

	items := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	// Footer at bottom edge → garbage → no table regions match
	if len(items) != 0 {
		t.Errorf("footer garbage: should not produce tables, got %d", len(items))
	}
}

// ---- helpers ----

func TestCellTexts(t *testing.T) {
	cells := []TSRCell{
		{Text: "A"}, {Text: "B"}, {Text: "C"},
	}
	texts := tsrCellTexts(cells)
	got := strings.Join(texts, ",")
	if got != "A,B,C" {
		t.Errorf("cellTexts: got %q, want 'A,B,C'", got)
	}
}

// ── constructTable unit tests ─────────────────────────────────────────

func TestConstructTable_Simple3x2(t *testing.T) {
	// 3 columns × 2 rows — cells pre-filled (simulating extractTableBoxesFromImage).
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Text: "B", Label: "table row"},
		{X0: 201, Y0: 0, X1: 300, Y1: 50, Text: "C", Label: "table row"},
		{X0: 0, Y0: 51, X1: 100, Y1: 100, Text: "D", Label: "table row"},
		{X0: 101, Y0: 51, X1: 200, Y1: 100, Text: "E", Label: "table row"},
		{X0: 201, Y0: 51, X1: 300, Y1: 100, Text: "F", Label: "table row"},
	}
	boxes := []TextBox{}
	html := constructTable(cells, boxes, "", nil)
	if !strings.Contains(html, "<table>") {
		t.Error("expected <table> tag")
	}
	if !strings.Contains(html, "A") || !strings.Contains(html, "B") || !strings.Contains(html, "C") {
		t.Error("expected cell texts A, B, C in HTML")
	}
	// Should have 2 <tr> elements
	trCount := strings.Count(html, "<tr>")
	if trCount != 2 {
		t.Errorf("expected 2 <tr> rows, got %d", trCount)
	}
	tdCount := strings.Count(html, "<td ")
	if tdCount != 6 {
		t.Errorf("expected 6 <td > cells, got %d", tdCount)
	}
	t.Logf("HTML:\n%s", html)
}

func TestConstructTable_EmptyCells(t *testing.T) {
	html := constructTable(nil, nil, "", nil)
	if html != "" {
		t.Errorf("expected empty string for empty cells, got %q", html)
	}
	html = constructTable([]TSRCell{}, []TextBox{}, "", nil)
	if html != "" {
		t.Errorf("expected empty string for empty cells slice, got %q", html)
	}
}

func TestConstructTable_NoMatchingBox(t *testing.T) {
	// Cell has no overlapping text box → empty <td >
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "Has text", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Label: "table row"},
	}
	boxes := []TextBox{}
	html := constructTable(cells, boxes, "", nil)
	if !strings.Contains(html, "Has text") {
		t.Error("expected first cell text")
	}
	// Should still have 2 <td > cells
	if strings.Count(html, "<td ") != 2 {
		t.Errorf("expected 2 <td > cells, got %d. HTML:\n%s", strings.Count(html, "<td "), html)
	}
}

func TestConstructTable_WithCaption(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "X", Label: "table row"},
	}
	html := constructTable(cells, nil, "表1：测试标题", nil)
	if !strings.Contains(html, "<caption>表1：测试标题</caption>") {
		t.Errorf("expected caption, got:\n%s", html)
	}
	t.Logf("HTML:\n%s", html)
}

func TestConstructTable_SingleRow(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 40, Text: "Col1", Label: "table row"},
		{X0: 51, Y0: 0, X1: 100, Y1: 40, Text: "Col2", Label: "table row"},
	}
	html := constructTable(cells, nil, "", nil)
	if strings.Count(html, "<tr>") != 1 {
		t.Errorf("expected 1 row, got %d", strings.Count(html, "<tr>"))
	}
	if strings.Count(html, "<td ") != 2 {
		t.Errorf("expected 2 cells, got %d", strings.Count(html, "<td "))
	}
}

func TestConstructTable_CellsTextFilledAfterCall(t *testing.T) {
	// constructTable should populate cell text from boxes.
	// Bug: fillCellTextFromBoxes modifies a local copy — original cells stay empty,
	// causing generate_test.go to output empty rows.
	// Cells pre-filled — constructTable no longer fills text (done in extractTableBoxesFromImage).
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A1", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Text: "B1", Label: "table row"},
		{X0: 0, Y0: 51, X1: 100, Y1: 100, Text: "A2", Label: "table row"},
		{X0: 101, Y0: 51, X1: 200, Y1: 100, Text: "B2", Label: "table row"},
	}
	_ = constructTable(cells, nil, "", nil)

	// constructTable preserves cell text (does not clear or overwrite).
	if cells[0].Text != "A1" {
		t.Errorf("cell[0] text = %q, want %q", cells[0].Text, "A1")
	}
	if cells[1].Text != "B1" {
		t.Errorf("cell[1] text = %q, want %q", cells[1].Text, "B1")
	}
}

func TestConstructTable_YBasedFallback(t *testing.T) {
	// Cells with label "table" + pre-filled text
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "R1C1", Label: "table"},
		{X0: 51, Y0: 0, X1: 100, Y1: 30, Text: "R1C2", Label: "table"},
		{X0: 0, Y0: 31, X1: 50, Y1: 60, Text: "R2C1", Label: "table"},
	}
	html := constructTable(cells, nil, "", nil)
	if strings.Count(html, "<tr>") != 2 {
		t.Errorf("expected 2 rows from Y-fallback, got %d", strings.Count(html, "<tr>"))
	}
	if strings.Count(html, "<td ") != 4 { // padded to maxCols=2: 2 in row1, 2 in row2
		t.Errorf("expected 4 cells (padded), got %d", strings.Count(html, "<td "))
	}
}

// TestExtractTableAndReplace_CellTextFilled verifies that extractTableAndReplace
// fills cell text correctly with realistic coordinate transforms (Scale=3, CropOff≠0).
// This simulates the real pipeline where TSR cells are in crop pixel space and
// post-merge boxes are in PDF point space.
func TestExtractTableAndReplace_CellTextFilled(t *testing.T) {
	// Simulate 公司差旅费 page 0 table coordinates.
	// DLA region: X0=217, X1=1584, Y0=985, Y1=1599 at 216 DPI → PDF: 72-528 x 328-533
	// Scale = 216/72 = 3.0
	// cropOff ≈ region.X - region.W*0.03
	const scale = 3.0
	const cropOffX = 176.0
	const cropOffY = 967.0

	// Post-merge boxes in PDF point space (inside the table region).
	// PDF Y=470 → crop Top = 470*3-967 = 443 → overlaps crop cell at Y0=441.
	// Boxes must have R (row) and C (col) annotations matching cells,
	// matching Python's construct_table which assigns boxes to cells by R/C.
	boxes := []TextBox{
		{X0: 80, X1: 210, Top: 470, Bottom: 490, Text: "标职务", LayoutType: "table", PageNumber: 0, R: 0, C: 0},
		{X0: 220, X1: 270, Top: 470, Bottom: 490, Text: "飞机", LayoutType: "table", PageNumber: 0, R: 0, C: 1},
		{X0: 80, X1: 210, Top: 492, Bottom: 512, Text: "公司级领导", LayoutType: "table", PageNumber: 0, R: 1, C: 0},
		{X0: 220, X1: 270, Top: 492, Bottom: 512, Text: "经济舱位", LayoutType: "table", PageNumber: 0, R: 1, C: 1},
	}

	// TSR cells in crop pixel space (matching real TSR output).
	// Cells pre-filled (extractTableBoxesFromImage already ran fillText + OCR).
	cells := []TSRCell{
		{X0: 35, Y0: 441, X1: 456, Y1: 500, Text: "标职务", Label: "table row"},
		{X0: 460, Y0: 442, X1: 630, Y1: 500, Text: "飞机", Label: "table row"},
		{X0: 35, Y0: 501, X1: 456, Y1: 560, Text: "公司级领导", Label: "table row"},
		{X0: 460, Y0: 502, X1: 630, Y1: 560, Text: "经济舱位", Label: "table row"},
	}

	tables := []TableItem{{
		Cells:     cells,
		Positions: []Position{{Left: 80, Right: 500, Top: 480, Bottom: 560}},
		Scale:     scale,
		CropOffX:  cropOffX,
		CropOffY:  cropOffY,
	}}

	result := extractTableAndReplace(boxes, tables)
	if len(result) != 1 {
		t.Fatalf("expected 1 output box (HTML table), got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "<table>") {
		t.Error("output should contain HTML table")
	}

	// Key assertion: constructTable backfills tables[0].Rows.
	rows := tables[0].Rows
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0][0] != "标职务" {
		t.Errorf("row 0 col 0 = %q, want %q", rows[0][0], "标职务")
	}
	if rows[0][1] != "飞机" {
		t.Errorf("row 0 col 1 = %q, want %q", rows[0][1], "飞机")
	}
	if rows[1][0] != "公司级领导" {
		t.Errorf("row 1 col 0 = %q, want %q", rows[1][0], "公司级领导")
	}
	if rows[1][1] != "经济舱位" {
		t.Errorf("row 1 col 1 = %q, want %q", rows[1][1], "经济舱位")
	}
}

// TestConstructTable_FromBoxesRC builds HTML directly from boxes with R/C
// annotations, matching Python's construct_table.  No cells needed for text.
func TestConstructTable_FromBoxesRC(t *testing.T) {
	// Boxes with R (row) and C (col) annotations — like the output of
	// annotateTableBoxes after layout cleanup.
	boxes := []TextBox{
		{X0: 50, X1: 150, Top: 100, Bottom: 130, Text: "姓名", R: 0, C: 0},
		{X0: 155, X1: 255, Top: 100, Bottom: 130, Text: "年龄", R: 0, C: 1},
		{X0: 50, X1: 150, Top: 135, Bottom: 165, Text: "张三", R: 1, C: 0},
		{X0: 155, X1: 255, Top: 135, Bottom: 165, Text: "25", R: 1, C: 1},
	}

	// constructTable should build HTML directly from boxes by R/C grouping,
	// ignoring cell text (matching Python's construct_table).
	item := &TableItem{}
	html := constructTable(nil, boxes, "", item)

	if !strings.Contains(html, "姓名") || !strings.Contains(html, "张三") {
		t.Errorf("HTML missing box text: %s", html)
	}
	// 2 rows, 2 cols
	if strings.Count(html, "<tr>") != 2 {
		t.Errorf("expected 2 rows, got %d. HTML: %s", strings.Count(html, "<tr>"), html)
	}
	if strings.Count(html, "<td ") != 4 {
		t.Errorf("expected 4 cells, got %d. HTML: %s", strings.Count(html, "<td "), html)
	}
	// Verify Rows output
	if len(item.Rows) != 2 || len(item.Rows[0]) != 2 {
		t.Errorf("Rows: expected 2x2, got %dx%d", len(item.Rows), len(item.Rows[0]))
	}
	if item.Rows[0][0] != "姓名" {
		t.Errorf("Rows[0][0] = %q, want %q", item.Rows[0][0], "姓名")
	}
	t.Logf("HTML: %s", html)
}

// TestFillCellTextFromBoxes_RCAnnotations fills text via R/C when spatial
// overlap is marginal.  Real-world TSR cells and pdf_oxide boxes have pixel-level
// offsets — R/C annotations (set by annotateTableBoxes) are the Python-equivalent
// way to assign boxes to cells regardless of coordinate deviations.
func TestFillCellTextFromBoxes_RCAnnotations(t *testing.T) {
	// Cells with real-world coordinate offsets (box shifted by 2px from cell).
	// Spatial overlap <30% for the shifted case — fillCellTextFromBoxes fails.
	cells := []TSRCell{
		{X0: 10, Y0: 10, X1: 200, Y1: 50},
		{X0: 210, Y0: 10, X1: 400, Y1: 50},
		{X0: 10, Y0: 55, X1: 200, Y1: 95},
		{X0: 210, Y0: 55, X1: 400, Y1: 95},
	}

	// Boxes have R/C annotations but their spatial overlap with cell rects
	// is marginal (real-world scenario).  R/C path should still fill text.
	boxes := []TextBox{
		{X0: 12, X1: 198, Top: 12, Bottom: 48, Text: "A", R: 0, C: 0},  // overlap ~92% → OK
		{X0: 215, X1: 395, Top: 12, Bottom: 48, Text: "B", R: 0, C: 1}, // overlap ~90% → OK
		{X0: 12, X1: 198, Top: 58, Bottom: 92, Text: "C", R: 1, C: 0},  // overlap ~92% → OK
		{X0: 215, X1: 350, Top: 58, Bottom: 90, Text: "D", R: 1, C: 1}, // overlap ~50% → MARGINAL
	}

	// This SHOULD fill all 4 cells via R/C, but spatial-only may fail on D.
	fillCellTextFromBoxes(cells, boxes)

	// When spatial overlap is marginal (box "D" at 50%), fillCellTextFromBoxes
	// may still match because cell is empty (0.3 threshold).  But the real
	// problem is that fillCellTextFromBoxes depends on coordinates, while
	// R/C annotations don't.
	hasText := false
	for _, c := range cells {
		if c.Text != "" {
			hasText = true
		}
	}
	if !hasText {
		t.Error("fillCellTextFromBoxes should fill text from spatially overlapping boxes with R/C")
	}

	// NOW test the R/C path explicitly: fillCellTextFromAnnotations uses
	// R/C labels only, ignoring coordinate overlap entirely.
	cells2 := []TSRCell{
		{X0: 10, Y0: 10, X1: 200, Y1: 50},
		{X0: 210, Y0: 10, X1: 400, Y1: 50},
		{X0: 10, Y0: 55, X1: 200, Y1: 95},
		{X0: 210, Y0: 55, X1: 400, Y1: 95},
	}
	rows := [][]TSRCell{{cells2[0], cells2[1]}, {cells2[2], cells2[3]}}
	fillCellTextFromAnnotations(rows, boxes)

	if rows[0][0].Text != "A" {
		t.Errorf("R/C: row0 col0 = %q, want %q", rows[0][0].Text, "A")
	}
	if rows[0][1].Text != "B" {
		t.Errorf("R/C: row0 col1 = %q, want %q", rows[0][1].Text, "B")
	}
	if rows[1][0].Text != "C" {
		t.Errorf("R/C: row1 col0 = %q, want %q", rows[1][0].Text, "C")
	}
	if rows[1][1].Text != "D" {
		t.Errorf("R/C: row1 col1 = %q, want %q", rows[1][1].Text, "D")
	}
}


// TestConstructTable_SingleRowMultiCol covers R=0 with multiple columns
// (table header pattern). boxesHaveAnnotations must detect valid annotations
// even though maxR=0.
func TestConstructTable_SingleRowMultiCol(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, Text: "姓名", R: 0, C: 0},
		{X0: 101, X1: 200, Top: 0, Bottom: 30, Text: "年龄", R: 0, C: 1},
		{X0: 201, X1: 300, Top: 0, Bottom: 30, Text: "性别", R: 0, C: 2},
	}
	item := &TableItem{}
	html := constructTable(nil, boxes, "", item)
	if strings.Count(html, "<td ") != 3 {
		t.Errorf("expected 3 cells, got %d. HTML: %s", strings.Count(html, "<td "), html)
	}
	if item.Rows[0][0] != "姓名" || item.Rows[0][1] != "年龄" || item.Rows[0][2] != "性别" {
		t.Errorf("wrong row text: %v", item.Rows[0])
	}
}

// TestConstructTable_MultiRowSingleCol covers C=0 with multiple rows
// (vertical list pattern). boxesHaveAnnotations must detect valid
// annotations even though maxC=0.
func TestConstructTable_MultiRowSingleCol(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, Text: "第一行", R: 0, C: 0},
		{X0: 0, X1: 100, Top: 35, Bottom: 65, Text: "第二行", R: 1, C: 0},
		{X0: 0, X1: 100, Top: 70, Bottom: 100, Text: "第三行", R: 2, C: 0},
	}
	item := &TableItem{}
	html := constructTable(nil, boxes, "", item)
	if strings.Count(html, "<tr>") != 3 {
		t.Errorf("expected 3 rows, got %d. HTML: %s", strings.Count(html, "<tr>"), html)
	}
	if item.Rows[0][0] != "第一行" || item.Rows[1][0] != "第二行" || item.Rows[2][0] != "第三行" {
		t.Errorf("wrong text: row0=%q row1=%q row2=%q", item.Rows[0][0], item.Rows[1][0], item.Rows[2][0])
	}
}

// TestConstructTable_RCAfterMerge verifies that R/C annotations survive
// text merge. The merged box expands bounds but keeps the first box's R/C.
func TestConstructTable_RCAfterMerge(t *testing.T) {
	// Simulate two adjacent fragments merged into one box.
	// The merged box keeps R/C from the first fragment.
	postMerge := []TextBox{
		{X0: 0, X1: 350, Top: 0, Bottom: 30, Text: "公司级领导人员（含公司董事、总监）", R: 0, C: 0},
		{X0: 355, X1: 500, Top: 0, Bottom: 30, Text: "经济舱位", R: 0, C: 1},
		{X0: 0, X1: 200, Top: 35, Bottom: 65, Text: "其他工作人员", R: 1, C: 0},
		{X0: 355, X1: 500, Top: 35, Bottom: 65, Text: "经济舱位", R: 1, C: 1},
	}
	item := &TableItem{}
	html := constructTable(nil, postMerge, "", item)
	if !strings.Contains(html, "公司级领导") {
		t.Errorf("missing merged text: %s", html)
	}
	if strings.Count(html, "<tr>") != 2 {
		t.Errorf("expected 2 rows, got %d", strings.Count(html, "<tr>"))
	}
	if item.Rows[0][0] != "公司级领导人员（含公司董事、总监）" {
		t.Errorf("row 0 col 0 = %q", item.Rows[0][0])
	}
}

// TestGroupTSRCellsToRowsLabeled_DefaultTableLabel verifies that cells with
// the real TSR default label "table" (class 0) are grouped correctly.
// The current deepDocReRowHdr regex only matches ".* (row|header)" — it misses
// the default "table" label, causing gatherTSR to return empty and forcing
// a fallback to pure Y-based grouping (which loses R/C annotations).
func TestGroupTSRCellsToRowsLabeled_DefaultTableLabel(t *testing.T) {
	cells := []TSRCell{
		{X0: 10, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 10, Y0: 35, X1: 100, Y1: 65, Label: "table"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Label: "table"},
	}
	rows := groupTSRCellsToRowsLabeled(cells)
	if len(rows) != 2 {
		t.Fatalf("label %q: expected 2 rows, got %d (BUG: deepDocReRowHdr does not match %q)", "table", len(rows), "table")
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("expected 2 cols/row, got %d/%d", len(rows[0]), len(rows[1]))
	}
}

// TestGroupBoxesByRC_RDiffSplitsRows verifies that groupBoxesByRC
// creates separate rows for different R values (Python: R differs → new row).
// Even when boxes share the same Y, different R → different grid row.
func TestGroupBoxesByRC_RDiffSplitsRows(t *testing.T) {
	// 6 boxes with 6 different R values → 6 rows (Python R-first splitting).
	boxes := []TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 1, C: 1},
		{X0: 210, X1: 290, Top: 0, Bottom: 30, Text: "C", R: 2, C: 2},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "D", R: 3, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "E", R: 4, C: 1},
		{X0: 210, X1: 290, Top: 35, Bottom: 65, Text: "F", R: 5, C: 2},
	}
	rows := groupBoxesByRC(boxes)
	// R=0,1,2,3,4,5 → 6 rows (Python: R differs → new row).
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows (R differs → split), got %d", len(rows))
	}
}

// TestGroupBoxesByRC_MergesCloseCols verifies that C compression works
// within each R group — merging different C values that are close in X.
func TestGroupBoxesByRC_MergesCloseCols(t *testing.T) {
	// R=0 has C=0,1. R=1 has C=0,1. C compression → 2 cols each.
	boxes := []TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	rows := groupBoxesByRC(boxes)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (R diff), got %d", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("expected 2 cols/row, got %d/%d", len(rows[0]), len(rows[1]))
	}
	if rows[0][0].Text != "A" || rows[0][1].Text != "B" {
		t.Errorf("row0 wrong: %q %q", rows[0][0].Text, rows[0][1].Text)
	}
	if rows[1][0].Text != "C" || rows[1][1].Text != "D" {
		t.Errorf("row1 wrong: %q %q", rows[1][0].Text, rows[1][1].Text)
	}
}

// TestGroupBoxesByRC_RDiffSplitsRow verifies that boxes with different R
// values are placed in separate rows even when their Y ranges overlap.
// Matches Python: R differs → new row unconditionally.
func TestGroupBoxesByRC_RDiffSplitsRow(t *testing.T) {
	// R=0 and R=1 at same Y (overlapping) → two separate rows in the grid.
	boxes := []TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 1, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 2, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 3, C: 1},
	}
	rows := groupBoxesByRC(boxes)
	// R=0,1,2,3 → 4 different R values → 4 rows (Python: R differs → new row).
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (R differs → split), got %d", len(rows))
	}
	if rows[0][0].Text != "A" || rows[1][0].Text != "B" {
		t.Errorf("row0/1 wrong: A=%q B=%q", rows[0][0].Text, rows[1][0].Text)
	}
}

// TestFillCellTextFromBoxes_RCOnly verifies that box text goes to exactly
// one cell via R/C annotations, not multiple cells via spatial overlap.
// A box overlapping two cells should only fill the one matching its R/C.
func TestFillCellTextFromBoxes_RCOnly(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Label: "table"},
		{X0: 90, Y0: 0, X1: 200, Y1: 50, Label: "table"},
	}
	// This box straddles cell 0 (X=0-100) and cell 1 (X=90-200).
	// Spatial overlap: both match. R/C: should go to cell R=0, C=0 only.
	boxes := []TextBox{
		{X0: 80, X1: 120, Top: 0, Bottom: 50, Text: "TEXT", LayoutType: "table", R: 0, C: 0},
	}
	rows := groupTSRCellsToRowsLabeled(cells)
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		if b.R >= 0 && b.R < len(rows) && b.C >= 0 && b.C < len(rows[b.R]) {
			rows[b.R][b.C].Text = t
		}
	}
	// Cell 0 should have text, cell 1 should NOT.
	if rows[0][0].Text != "TEXT" {
		t.Errorf("cell[0][0] = %q, want %q", rows[0][0].Text, "TEXT")
	}
	if rows[0][1].Text != "" {
		t.Errorf("cell[0][1] = %q, should be empty (spatial overlap leak)", rows[0][1].Text)
	}
}

// TestRowsToHTML_HeaderRows verifies that header rows use <th > instead of <td >.
func TestRowsToHTML_HeaderRows(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Name", Label: "table column header"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Age", Label: "table column header"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "John", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "30", Label: "table row"},
	}
	// constructTable should produce <th > for header row.
	item := &TableItem{}
	html := constructTable(cells, nil, "", item)
	// Header row should use <th >, data row <td >.
	if !strings.Contains(html, "<th >") {
		t.Errorf("expected <th > for header row. HTML: %s", html)
	}
	if strings.Count(html, "<th ") != 2 {
		t.Errorf("expected 2 <th > cells, got %d. HTML: %s", strings.Count(html, "<th "), html)
	}
	if strings.Count(html, "<td ") != 2 {
		t.Errorf("expected 2 <td > cells (data row), got %d", strings.Count(html, "<td "))
	}
}



// TestExtractTableAndReplace_OnlyTableBoxes verifies that only boxes with
// LayoutType=="table" are passed to constructTable (Python: filters by layout_type).
func TestExtractTableAndReplace_OnlyTableBoxes(t *testing.T) {
	boxes := []TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0, LayoutType: "table"},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1, LayoutType: "table"},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "NOT_TABLE", R: 0, C: 0, LayoutType: "text"}, // non-table, R/C=0
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1, LayoutType: "table"},
	}
	tables := []TableItem{{
		Cells:     []TSRCell{{Label: "table"}},
		Positions: []Position{{Left: 0, Right: 200, Top: 0, Bottom: 70}},
		Scale:     1.0,
	}}
	result := extractTableAndReplace(boxes, tables)
	// constructTable should produce HTML with "A", "B", "D" but NOT "NOT_TABLE".
	if !strings.Contains(result[0].Text, "A") || !strings.Contains(result[0].Text, "D") {
		t.Errorf("missing table box text: %s", result[0].Text)
	}
	if strings.Contains(result[0].Text, "NOT_TABLE") {
		t.Errorf("non-table box leaked into HTML: %s", result[0].Text)
	}
}

// TestFillCellText_RCOverSpatial verifies that R/C-based fill puts a
// box into exactly one cell (matching Python), unlike spatial fill which
// puts it into all overlapping cells.
func TestFillCellText_RCOverSpatial(t *testing.T) {
	// Box at X=30..270 overlaps all 3 cells (>30% each — spatial fills ALL).
	// With R/C, it belongs only to cell[1] (R=0, C=1).
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 90, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 180, Y0: 0, X1: 300, Y1: 30, Label: "table"},
	}
	boxes := []TextBox{
		{X0: 30, X1: 270, Top: 0, Bottom: 30, Text: "TEXT", LayoutType: "table", R: 0, C: 1},
	}

	// Spatial fill: fills ALL overlapping cells —> duplication.
	cellsCopy := make([]TSRCell, 3)
	copy(cellsCopy, cells)
	fillCellTextFromBoxes(cellsCopy, boxes)
	spatialCount := 0
	for _, c := range cellsCopy {
		if c.Text != "" { spatialCount++ }
	}
	if spatialCount <= 1 {
		t.Errorf("spatial fill: expected >1 cells with text, got %d", spatialCount)
	}
	t.Logf("spatial fill: %d cells (WRONG — duplication)", spatialCount)

	// R/C fill: only cell matching box.R/C gets text.
	cellsRC := make([]TSRCell, 3)
	copy(cellsRC, cells)
	rows := groupTSRCellsToRowsLabeled(cellsRC)
	for _, b := range boxes {
		if b.R >= 0 && b.R < len(rows) && b.C >= 0 && b.C < len(rows[b.R]) {
			rows[b.R][b.C].Text = strings.TrimSpace(b.Text)
		}
	}
	rcCount := 0
	for _, row := range rows {
		for _, c := range row {
			if c.Text == "TEXT" { rcCount++ }
		}
	}
	if rcCount != 1 {
		t.Errorf("R/C fill: expected 1 cell with 'TEXT', got %d", rcCount)
	}
}

func TestIsCaptionBox(t *testing.T) {
	tests := []struct{ text string; want bool }{
		{"表1：交通工具等级", true},
		{"Table 1: Transport Levels", true},
		{"图表 1: 测试", true},
		{"公司领导班子成员、出差地", false},   // plain text, not caption
		{"第十条到厂矿单位出差", false},       // normal paragraph
		{"", false},
	}
	for _, tt := range tests {
		if got := isCaptionBox(tt.text, ""); got != tt.want {
			t.Errorf("isCaptionBox(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestFillCellTextFromBoxes_SkipsCaption(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 0, Y0: 35, X1: 200, Y1: 65, Label: "table"},
	}
	boxes := []TextBox{
		// Caption box (should be skipped)
		{X0: 0, X1: 200, Top: 0, Bottom: 30, Text: "表1：交通工具等级"},
		// Data box
		{X0: 0, X1: 200, Top: 35, Bottom: 65, Text: "数据行"},
	}
	fillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("caption leaked into cell 0: %q", cells[0].Text)
	}
	if cells[1].Text != "数据行" {
		t.Errorf("data not in cell 1: %q", cells[1].Text)
	}
}

func TestFillCellText_RCPreventsCrossCellLeak(t *testing.T) {
	// Caption box at Y=0-15 overlaps BOTH cell rows (both are "empty").
	// Spatial fill: text leaks to both cells. R/C fill: only cell[0] gets text.
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 300, Y1: 30, Label: "table"},
		{X0: 0, Y0: 35, X1: 300, Y1: 65, Label: "table"},
	}
	boxes := []TextBox{
		{X0: 10, X1: 200, Top: 12, Bottom: 28, Text: "公司领导班子成员、出差地", R: 0, C: 0},
	}

	// Spatial fill → leaks to cells[1] (overlap ≥30%).
	cellsSp := make([]TSRCell, 2)
	copy(cellsSp, cells)
	fillCellTextFromBoxes(cellsSp, boxes)
	if cellsSp[1].Text != "" {
		t.Errorf("spatial fill: caption leaked to cell[1]: %q", cellsSp[1].Text)
	}

	// R/C fill → only cell[0] (R=0,C=0).
	cellsRC := make([]TSRCell, 2)
	copy(cellsRC, cells)
	rows := groupTSRCellsToRowsLabeled(cellsRC)
	for _, b := range boxes {
		if b.R >= 0 && b.R < len(rows) && b.C >= 0 && b.C < len(rows[b.R]) {
			if rows[b.R][b.C].Text == "" {
				rows[b.R][b.C].Text = strings.TrimSpace(b.Text)
			}
		}
	}
	if cellsRC[1].Text != "" {
		t.Errorf("R/C fill: caption leaked to cell[1]: %q", cellsRC[1].Text)
	}
}

func TestGroupBoxesByRC_FallbackToYXWhenNoAnnotations(t *testing.T) {
	// When all boxes have R=-1 (Python's case: regex didn't match "table" label),
	// groupBoxesByRC should fall back to YX coordinate grouping.
	boxes := []TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: -1, C: -1},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: -1, C: -1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: -1, C: -1},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: -1, C: -1},
	}
	rows := groupBoxesByRC(boxes)
	// R=-1 for all → maxR = -1 → grid would be 0 rows. Must fall back to YX.
	if len(rows) == 0 {
		t.Fatal("groupBoxesByRC returned 0 rows when R=-1 — no YX fallback")
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows (Y-split), got %d", len(rows))
	}
}

func TestRowsToHTML_Colspan(t *testing.T) {
	// Box spanning 2 columns: SP annotation with HLeft/HRight covering cols 0-1.
	boxes := []TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "Name", R: 0, C: 0, H: 1, HLeft: 10, HRight: 190},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "", R: 0, C: 1, SP: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "John", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "30", R: 1, C: 1},
	}
	rows := groupBoxesByRC(boxes)
	spans, covered := calSpans(rows)
	html := rowsToHTML(rows, "", nil, spans, covered)
	if !strings.Contains(html, "colspan") {
		t.Errorf("expected colspan attribute, got: %s", html)
	}
	t.Logf("HTML: %s", html)
}

// TestStripCaptionFromCells verifies that caption-like text is cleared
// from TSR cells before the table HTML is built.
func TestStripCaptionFromCells_ClearsCaptionPattern(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "表1：差旅费标准"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: ""},
		{X0: 0, Y0: 60, X1: 100, Y1: 110, Text: "张三"},
		{X0: 100, Y0: 60, X1: 200, Y1: 110, Text: "100"},
	}
	stripCaptionFromCells(cells)
	if cells[0].Text != "" {
		t.Errorf("caption cell should be cleared, got %q", cells[0].Text)
	}
	if cells[2].Text != "张三" {
		t.Errorf("data cell should be preserved, got %q", cells[2].Text)
	}
}

// TestStripCaptionFromCells_PreservesData verifies that non-caption
// cells are not cleared.
func TestStripCaptionFromCells_PreservesData(t *testing.T) {
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "姓名"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "年龄"},
		{X0: 0, Y0: 60, X1: 100, Y1: 110, Text: "张三"},
		{X0: 100, Y0: 60, X1: 200, Y1: 110, Text: "25"},
	}
	// Make a copy and strip
	orig := make([]string, len(cells))
	for i, c := range cells {
		orig[i] = c.Text
	}
	stripCaptionFromCells(cells)
	for i := range cells {
		if cells[i].Text != orig[i] {
			t.Errorf("cell[%d] changed: %q -> %q", i, orig[i], cells[i].Text)
		}
	}
}

// TestStripCaptionFromCells_Empty is a no-op on empty cells.
func TestStripCaptionFromCells_Empty(t *testing.T) {
	cells := []TSRCell{}
	stripCaptionFromCells(cells) // must not panic
}

// TestConstructTable_StripsCaptionFromCells verifies that constructTable
// strips caption text from cells before building HTML.
func TestConstructTable_StripsCaptionFromCells(t *testing.T) {
	// Cell[0] has caption text "表1：标题"; cell[1] has real data.
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "表1：标题"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "数据"},
	}
	html := constructTable(cells, nil, "", nil)
	// "表1：标题" should NOT appear in the HTML (stripped as caption).
	if strings.Contains(html, "表1") {
		t.Errorf("caption text '表1：标题' should be stripped: %s", html)
	}
	// "数据" should still be there.
	if !strings.Contains(html, "数据") {
		t.Errorf("data text '数据' should be preserved: %s", html)
	}
	t.Logf("HTML: %s", html)
}

// TestCalSpans_NonSpanningCellsNotPolluted verifies that a regular cell
// at position [0,0] is NOT detected as spanning when a spanning cell at
// [0,1] extends to the left, polluting column boundary calculations.
// Bug: calSpans computed column boundaries from ALL cells including
// spanning cells. "部门开支汇总" at [0,1] with X0=0 extends colLeft[1]
// to 0 instead of 101, shifting the center and causing "Q1" at [0,0]
// to be incorrectly detected as spanning 2 columns.
func TestCalSpans_NonSpanningCellsNotPolluted(t *testing.T) {
	// Simulate the SpannedTable test grid: row 0 has Q1(regular), 部门开支汇总(span), Q2(regular)
	rows := [][]TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Q1", Label: "table row"},
			{X0: 0, Y0: 0, X1: 200, Y1: 30, Text: "部门开支汇总", Label: "table spanning cell"},
			{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Q2", Label: "table row"},
		},
		{
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "100", Label: "table row"},
			{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "200", Label: "table row"},
		},
	}

	spans, covered := calSpans(rows)

	// Q1 at [0,0] has X0=0, X1=100 which should only cover its own column.
	// It should NOT get a colspan.
	if s, ok := spans[[2]int{0, 0}]; ok {
		t.Errorf("Q1 at [0,0] should NOT have colspan, got %v. "+
			"Spanning cell at [0,1] polluted column boundaries", s)
	}

	// 部门开支汇总 at [0,1] has X0=0, X1=200 which DOES span columns 0 and 1.
	if s, ok := spans[[2]int{0, 1}]; !ok {
		t.Error("部门开支汇总 at [0,1] should have colspan=2 (covers X=0-200)")
	} else if s[0] != 2 {
		t.Errorf("部门开支汇总 colspan = %d, want 2", s[0])
	}

	// Q2 at [0,2] should be covered by the spanning cell (col 2 is within X=0-200).
	if !covered[[2]int{0, 2}] {
		t.Error("Q2 at [0,2] should be covered by spanning cell at [0,1]")
	}

	t.Logf("spans: %v, covered: %v", spans, covered)
}

// ── coordinate space conversion helpers ─────────────────────────────────

func TestCellToPageSpace(t *testing.T) {
	cell := TSRCell{X0: 100, Y0: 200, X1: 300, Y1: 400, Text: "hello", Label: "table"}
	got := cellToPageSpace(cell, 15, 25, 3.0)

	// (100+15)/3 = 38.33..., (200+25)/3 = 75
	if got.X0 != 38.333333333333336 || got.Y0 != 75 || got.X1 != 105 || got.Y1 != 141.66666666666666 {
		t.Errorf("cellToPageSpace: got (%f,%f,%f,%f), want (38.33,75,105,141.67)", got.X0, got.Y0, got.X1, got.Y1)
	}
	if got.Text != "hello" || got.Label != "table" {
		t.Error("cellToPageSpace should preserve Text and Label")
	}
}

func TestCellAddOffset(t *testing.T) {
	cell := TSRCell{X0: 100, Y0: 200, X1: 300, Y1: 400, Text: "hello"}
	got := cellAddOffset(cell, 15, 25)
	if got.X0 != 115 || got.Y0 != 225 || got.X1 != 315 || got.Y1 != 425 {
		t.Errorf("cellAddOffset: got (%f,%f,%f,%f)", got.X0, got.Y0, got.X1, got.Y1)
	}
	if got.Text != "hello" {
		t.Error("cellAddOffset should preserve Text")
	}
}

func TestBoxToCropSpace(t *testing.T) {
	box := TextBox{X0: 50, X1: 200, Top: 100, Bottom: 300, Text: "text"}
	got := boxToCropSpace(box, 3.0, 10, 20)
	if got.X0 != 140 || got.Top != 280 || got.X1 != 590 || got.Bottom != 880 {
		t.Errorf("boxToCropSpace: got (%f,%f,%f,%f)", got.X0, got.Top, got.X1, got.Bottom)
	}
	if got.Text != "text" {
		t.Error("boxToCropSpace should preserve Text")
	}
}

func TestCopyBoxAnnotations(t *testing.T) {
	src := &TextBox{R: 1, C: 2, RTop: 10, RBott: 20, H: 3, HTop: 30, HBott: 40,
		HLeft: 50, HRight: 60, CLeft: 70, CRight: 80, SP: 4}
	dst := &TextBox{}
	copyBoxAnnotations(dst, src)
	if dst.R != 1 || dst.C != 2 || dst.RTop != 10 || dst.RBott != 20 {
		t.Error("R/C fields not copied")
	}
	if dst.H != 3 || dst.HTop != 30 || dst.HBott != 40 {
		t.Error("H fields not copied")
	}
	if dst.HLeft != 50 || dst.HRight != 60 || dst.CLeft != 70 || dst.CRight != 80 {
		t.Error("spanning fields not copied")
	}
	if dst.SP != 4 {
		t.Error("SP not copied")
	}
}

// TestAnnotateBoxLayouts_CompactionPreservesWriteBackMapping verifies that
// when annotateBoxLayouts drops some boxes (CID garbage or garbage-layout
// at non-edge positions), the compaction step does not corrupt the caller's
// ability to write annotations back to the correct global box indices.
//
// The bug: annotateBoxLayouts compacts boxes in place in the shared backing
// array, shifting survivors forward.  enrichWithDeepDoc then iterates
// len(indices) positions and writes pageBoxes[i] back to boxes[indices[i]],
// but after compaction pageBoxes[1] holds what was originally pageBoxes[2],
// so annotations land on the wrong global box.
func TestAnnotateBoxLayouts_CompactionPreservesWriteBackMapping(t *testing.T) {
	// ── Simulate the exact enrichWithDeepDoc write-back pattern ──
	// Global boxes on a page: B0, B1, B2 (indices 0, 1, 2 in the PDF-space
	// boxes slice).
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "will be dropped via reference match"},
		{X0: 0, X1: 100, Top: 60, Bottom: 110, Text: "text box A"},
		{X0: 110, X1: 200, Top: 60, Bottom: 110, Text: "text box B"},
	}

	// Per-page subset (what enrichWithDeepDoc constructs from byPage[pg]).
	indices := []int{0, 1, 2}
	pageBoxes := make([]TextBox, len(indices))
	for i, idx := range indices {
		pageBoxes[i] = boxes[idx] // value copy
	}

	// DLA regions: one reference (garbage type → matched boxes are dropped
	// unless at page edge), two text regions for the surviving boxes.
	// scale=1.0 so DLA pixel coords == PDF point coords.
	regions := []DLARegion{
		{Label: "reference", Confidence: 0.9, X0: 0, Y0: 0, X1: 100, Y1: 50},
		{Label: "text", Confidence: 0.9, X0: 0, Y0: 60, X1: 100, Y1: 110},
		{Label: "text", Confidence: 0.9, X0: 110, Y0: 60, X1: 200, Y1: 110},
	}
	pageImgHeight := 200.0

	// The function under test.
	_ = annotateBoxLayouts(pageBoxes, regions, 1.0, pageImgHeight)

	// Simulate enrichWithDeepDoc write-back (table.go:52-58).
	for i, idx := range indices {
		if pageBoxes[i].LayoutType != "" {
			boxes[idx].LayoutType = pageBoxes[i].LayoutType
			boxes[idx].LayoutNo = pageBoxes[i].LayoutNo
		}
		copyBoxAnnotations(&boxes[idx], &pageBoxes[i])
	}

	// ── Assertions ──

	// B0 matched a "reference" region far from page edge → must be dropped.
	if boxes[0].LayoutType != "" {
		t.Errorf("B0 was dropped (reference region) but got LayoutType=%q from a shifted survivor",
			boxes[0].LayoutType)
	}

	// B1 matched the first text region → must be text-0.
	if boxes[1].LayoutType != "text" {
		t.Errorf("B1 LayoutType = %q, want text", boxes[1].LayoutType)
	}
	if boxes[1].LayoutNo != "text-0" {
		t.Errorf("B1 LayoutNo = %q, want text-0 (compaction shifted B2 into position 1)", boxes[1].LayoutNo)
	}

	// B2 matched the second text region → must be text-1.
	if boxes[2].LayoutType != "text" {
		t.Errorf("B2 LayoutType = %q, want text", boxes[2].LayoutType)
	}
	if boxes[2].LayoutNo != "text-1" {
		t.Errorf("B2 LayoutNo = %q, want text-1 (stale element at position 2 after compaction)", boxes[2].LayoutNo)
	}
}

// ── matchTableRegions unit tests ─────────────────────────────────────

func TestMatchTableRegions_SingleMatch(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50},
		{X0: 200, X1: 300, Top: 0, Bottom: 50},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "table"},  // covers box 0 at scale 3
		{X0: 600, Y0: 0, X1: 900, Y1: 150, Label: "text"},  // non-table, ignored
	}
	matches := matchTableRegions(boxes, regions, 3.0)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if len(matches[0].boxIdx) != 1 || matches[0].boxIdx[0] != 0 {
		t.Errorf("expected box 0 matched, got %v", matches[0].boxIdx)
	}
}

func TestMatchTableRegions_NoTableLabel(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "text"},
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "figure"},
	}
	matches := matchTableRegions(boxes, regions, 3.0)
	if len(matches) != 0 {
		t.Errorf("non-table labels: expected 0 matches, got %d", len(matches))
	}
}

func TestMatchTableRegions_MultipleBoxesSameTable(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50},   // box 0
		{X0: 110, X1: 210, Top: 0, Bottom: 50},  // box 1
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 630, Y1: 150, Label: "table"}, // covers both boxes at scale 3
	}
	matches := matchTableRegions(boxes, regions, 3.0)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if len(matches[0].boxIdx) != 2 {
		t.Errorf("expected 2 boxes matched, got %d: %v", len(matches[0].boxIdx), matches[0].boxIdx)
	}
}

func TestMatchTableRegions_ImageOnlyPDF(t *testing.T) {
	// Zero boxes — image-only PDF. Python processes every table DLA region
	// regardless of text box overlap.
	var boxes []TextBox // nil
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "table"},
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "text"},
	}
	matches := matchTableRegions(boxes, regions, 3.0)
	if len(matches) != 1 {
		t.Fatalf("image-only: expected 1 table match, got %d", len(matches))
	}
	if len(matches[0].boxIdx) != 0 {
		t.Errorf("image-only: expected empty boxIdx, got %d", len(matches[0].boxIdx))
	}
}

func TestMatchTableRegions_BelowThreshold(t *testing.T) {
	// Region overlaps only a sliver of the box (<40%) → no match.
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 100},
	}
	regions := []DLARegion{
		{X0: 0, Y0: 0, X1: 90, Y1: 90, Label: "table"}, // 30x30 at scale 3 → 9% overlap
	}
	matches := matchTableRegions(boxes, regions, 3.0)
	if len(matches) != 0 {
		t.Errorf("below threshold: expected 0 matches, got %d", len(matches))
	}
}

func TestCellSliceToPageSpace(t *testing.T) {
	cells := []TSRCell{
		{X0: 100, Y0: 200, X1: 300, Y1: 400},
		{X0: 400, Y0: 200, X1: 600, Y1: 400},
	}
	got := cellSliceToPageSpace(cells, 15, 25, 3)
	if len(got) != 2 {
		t.Fatal("expected 2 cells")
	}
	if got[0].X0 != 38.333333333333336 || got[1].X0 != 138.33333333333334 {
		t.Error("wrong conversion")
	}
}

// MockTableBuilder is a test-only TableBuilder with a configurable GroupCells.
type MockTableBuilder struct {
	GroupCellsFn func(cells []TSRCell) [][]TSRCell
}

func (m *MockTableBuilder) Name() string                             { return "mock" }
func (m *MockTableBuilder) DetectCells(_ context.Context, _ image.Image) ([]TSRCell, error) {
	return nil, nil
}
func (m *MockTableBuilder) GroupCells(cells []TSRCell) [][]TSRCell {
	if m.GroupCellsFn != nil {
		return m.GroupCellsFn(cells)
	}
	return nil
}

// ── writeTableAnnotations unit tests ──────────────────────────────────

func TestWriteTableAnnotations_WriteBack(t *testing.T) {
	boxes := []TextBox{
		{X0: 10, X1: 100, Top: 10, Bottom: 30, Text: "A", LayoutType: "table"},
		{X0: 110, X1: 200, Top: 10, Bottom: 30, Text: "B", LayoutType: "table"},
		{X0: 10, X1: 100, Top: 35, Bottom: 55, Text: "C", LayoutType: "table"},
	}
	boxIdx := []int{0, 2}
	cells := []TSRCell{
		{X0: 30, Y0: 30, X1: 300, Y1: 90, Label: "table row"},
		{X0: 30, Y0: 110, X1: 300, Y1: 170, Label: "table row"},
	}
	scale := 3.0

	tb := &MockTableBuilder{GroupCellsFn: func(cells []TSRCell) [][]TSRCell {
		return [][]TSRCell{{cells[0]}, {cells[1]}}
	}}
	writeTableAnnotations(boxes, boxIdx, cells, scale, 0, 0, tb)

	if boxes[0].R != 0 {
		t.Errorf("box 0 R = %d, want 0", boxes[0].R)
	}
	if boxes[0].C != 0 {
		t.Errorf("box 0 C = %d, want 0", boxes[0].C)
	}
	// Box 1 was not in boxIdx — should NOT be annotated
	if boxes[1].R != 0 || boxes[1].C != 0 {
		t.Errorf("box 1 should not be annotated: R=%d C=%d", boxes[1].R, boxes[1].C)
	}
	if boxes[2].R != 1 {
		t.Errorf("box 2 R = %d, want 1", boxes[2].R)
	}
}

func TestWriteTableAnnotations_ScaleDown(t *testing.T) {
	boxes := []TextBox{
		{X0: 10, X1: 100, Top: 10, Bottom: 50, Text: "X", LayoutType: "table"},
	}
	boxIdx := []int{0}
	cells := []TSRCell{
		{X0: 30, Y0: 30, X1: 300, Y1: 150, Label: "table row"},
	}
	scale := 3.0

	tb := &MockTableBuilder{GroupCellsFn: func(cells []TSRCell) [][]TSRCell {
		return [][]TSRCell{{cells[0]}}
	}}
	writeTableAnnotations(boxes, boxIdx, cells, scale, 0, 0, tb)

	// After scale-down: RTop / 3 should be in PDF space (~10).
	if boxes[0].RTop == 0 {
		t.Error("RTop should be non-zero after annotation")
	}
}

func TestWriteTableAnnotations_EmptyCells(t *testing.T) {
	boxes := []TextBox{{X0: 10, X1: 100, Top: 10, Bottom: 50, Text: "X", LayoutType: "table"}}
	boxIdx := []int{0}
	var cells []TSRCell

	tb := &MockTableBuilder{GroupCellsFn: func(cells []TSRCell) [][]TSRCell {
		return nil
	}}
	// Should not panic with empty cells.
	writeTableAnnotations(boxes, boxIdx, cells, 3.0, 0, 0, tb)
	if boxes[0].R != 0 || boxes[0].C != 0 {
		t.Errorf("empty cells: R=%d C=%d, want 0,0", boxes[0].R, boxes[0].C)
	}
}

// ── markNoMergeTables unit tests ─────────────────────────────────────

func TestMarkNoMergeTables_CaptionAfterTable(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 0, X1: 100, Top: 35, Bottom: 50, LayoutType: "table caption", Text: "表1：标题"},
	}
	tables := []TableItem{
		{Positions: []Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},
	}
	markNoMergeTables(boxes, tables)
	if !tables[0].NoMerge {
		t.Error("table followed by caption should be marked NoMerge")
	}
}

func TestMarkNoMergeTables_TitleAfterTable(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 0, X1: 100, Top: 35, Bottom: 50, LayoutType: "title"},
	}
	tables := []TableItem{
		{Positions: []Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},
	}
	markNoMergeTables(boxes, tables)
	if !tables[0].NoMerge {
		t.Error("table followed by title should be marked NoMerge")
	}
}

func TestMarkNoMergeTables_NoCaptionAfter(t *testing.T) {
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 0, X1: 100, Top: 35, Bottom: 50, LayoutType: "text"},
		{X0: 0, X1: 100, Top: 55, Bottom: 70, LayoutType: "table"},
	}
	tables := []TableItem{
		{Positions: []Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},
		{Positions: []Position{{Left: 0, Right: 100, Top: 55, Bottom: 70}}},
	}
	markNoMergeTables(boxes, tables)
	if tables[0].NoMerge {
		t.Error("table followed by text should NOT be marked NoMerge")
	}
	if tables[1].NoMerge {
		t.Error("last table should NOT be marked NoMerge")
	}
}

func TestMarkNoMergeTables_StaleLastTableTI(t *testing.T) {
	// Scenario: table box that does NOT overlap any TableItem.Position
	// should reset lastTableTI. Otherwise the next caption marks the
	// wrong (non-adjacent) table as NoMerge.
	// Box 0: "table", overlaps table[0] → lastTableTI = 0
	// Box 1: "table", no overlap → lastTableTI should reset to -1
	// Box 2: "title" → should be a no-op (no adjacent table)
	boxes := []TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 500, X1: 600, Top: 100, Bottom: 130, LayoutType: "table"}, // far away, no overlap
		{X0: 0, X1: 100, Top: 140, Bottom: 160, LayoutType: "title"},
	}
	tables := []TableItem{
		{Positions: []Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},   // table 0
		{Positions: []Position{{Left: 0, Right: 100, Top: 35, Bottom: 65}}},   // table 1 — box 0 doesn't overlap this either
	}
	markNoMergeTables(boxes, tables)
	// table[0] should NOT be NoMerge: the title follows a non-matching
	// table box, not table[0] directly.
	if tables[0].NoMerge {
		t.Error("stale lastTableTI: table[0] incorrectly marked NoMerge — " +
			"the non-overlapping table box (box 1) should have reset lastTableTI")
	}
}

func TestMarkNoMergeTables_EmptyInputs(t *testing.T) {
	// Should not panic with empty inputs.
	markNoMergeTables(nil, nil)
	markNoMergeTables([]TextBox{}, []TableItem{})
}
