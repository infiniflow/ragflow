package parser

import (
	"context"
	"image"
	"testing"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
)

func TestAnnotateBoxLayouts_SetsLabel(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
		{X0: 0, X1: 100, Top: 30, Bottom: 50},
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 60, Label: "title"},  // covers box 0 at scale 3
		{X0: 0, Y0: 90, X1: 300, Y1: 150, Label: "text"}, // covers box 1 at scale 3
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
	}
	regions := []pdf.DLARegion{
		{X0: 900, Y0: 900, X1: 1000, Y1: 1000, Label: "far"}, // completely outside
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Errorf("no match: expected empty, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_EmptyRegions(t *testing.T) {
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 20}}
	boxes = annotateBoxLayouts(boxes, nil, 3.0, 0)
	boxes = annotateBoxLayouts(boxes, []pdf.DLARegion{}, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Errorf("empty regions: got %q, want empty", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_PriorityOverMaxArea(t *testing.T) {
	// "table" type checked before "text" in priority order.
	// Even if "text" region has larger overlap, "table" wins if it meets threshold (≥40%).
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 50}}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 50}}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 30 * 3, Y1: 30 * 3, Label: "table"}, // covers ~30% of box
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Errorf("threshold: overlap < 40%% should not match, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_CIDGarbage(t *testing.T) {
	// CID-pattern boxes should be popped entirely (Python: bxs.pop(i)).
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20, Text: "(cid:123)"},
		{X0: 0, X1: 100, Top: 30, Bottom: 50, Text: "normal text"},
	}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
		{X0: 0, X1: 100, Top: 30, Bottom: 50},
	}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
		{X0: 0, X1: 100, Top: 100, Bottom: 120},
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 60, Label: "text"},    // per-type index 0
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
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 50}}
	// Low-confidence footer — should be filtered out.
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 280, Bottom: 290}}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 5, Bottom: 20}}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 50, Bottom: 70}}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 95, Bottom: 100}}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 50, Bottom: 70}}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 150, X1: 300, Y1: 210, Label: "footer", Confidence: 0.9}, // y=50-70 after /3
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if boxes[0].LayoutType != "footer" {
		t.Errorf("footer in middle of page: DLA false positive, keep text, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_ReferenceAlwaysGarbage(t *testing.T) {
	// Reference type is always garbage regardless of position (no keep_feat).
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 50, Bottom: 70}}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 150, X1: 300, Y1: 210, Label: "reference", Confidence: 0.9},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if len(boxes) != 0 {
		t.Errorf("reference: should always be garbage-filtered, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_NonGarbageTypeUnaffected(t *testing.T) {
	// "text" type is NOT a garbage type — should always be assigned.
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 200, Bottom: 220}}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 600, X1: 300, Y1: 660, Label: "text"},
	}
	boxes = annotateBoxLayouts(boxes, regions, 3.0, 300)
	if boxes[0].LayoutType != "text" {
		t.Errorf("non-garbage type: should be assigned, got %q", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_ZeroPageHeightDisablesGarbage(t *testing.T) {
	// pageImgHeight=0 → garbage check disabled → all types assigned.
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 100, Bottom: 120}}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20, Text: "text box"},
	}
	// Two figure regions, one text region
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 150, Y1: 60, Label: "text", Confidence: 0.9},        // matches text box → visited
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},
	}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20},    // overlaps text region 0
		{X0: 0, X1: 100, Top: 200, Bottom: 220}, // overlaps text region 1
		{X0: 200, X1: 300, Top: 0, Bottom: 20},  // overlaps figure region 0 only
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 150, Y1: 60, Label: "text", Confidence: 0.9},     // text-0
		{X0: 0, Y0: 600, X1: 150, Y1: 660, Label: "text", Confidence: 0.9},  // text-1
		{X0: 600, Y0: 0, X1: 900, Y1: 60, Label: "figure", Confidence: 0.9}, // figure-0 (PDF: x0=200, x1=300)
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
	boxes := []pdf.TextBox{
		{X0: 200, X1: 400, Top: 200, Bottom: 400, Text: "cell content"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 0, Y0: 0, X1: 2700, Y1: 2700, Label: "text"},      // full-page, 3x scale
			{X0: 300, Y0: 300, X1: 1500, Y1: 1500, Label: "table"}, // partial, 3x scale
		},
		TSRCells: []pdf.TSRCell{{X0: 200, Y0: 200, X1: 400, Y1: 400, Text: "cell1"}},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

	items := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	if len(items) == 0 {
		t.Error("priority: table should win over text, got 0 tables")
	}
}

func TestExtractTableBoxes_OverlapBelowThresholdNoTable(t *testing.T) {
	// Table region covers <40% of the box's area → matches no box → no table.
	dummyImg := image.NewRGBA(image.Rect(0, 0, 900, 900))
	boxes := []pdf.TextBox{
		{X0: 200, X1: 400, Top: 200, Bottom: 400, Text: "content"},
	}
	// Table region only touches a tiny corner (40*40/3 = 13x13 in PDF space).
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 600, Y0: 600, X1: 720, Y1: 720, Label: "table"}, // tiny corner
		},
		TSRCells: []pdf.TSRCell{},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

	items := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	if len(items) != 0 {
		t.Errorf("threshold: overlap < 40%% should produce 0 tables, got %d", len(items))
	}
}

func TestExtractTableBoxes_FooterGarbageNotTriggerTable(t *testing.T) {
	// Footer at page bottom → garbage-filtered → not kept as footer.
	// Since no other type matches, box remains unannotated.
	dummyImg := image.NewRGBA(image.Rect(0, 0, 900, 900)) // 900/3=300 PDF height
	boxes := []pdf.TextBox{
		{X0: 100, X1: 300, Top: 280, Bottom: 295, Text: "page 1"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 300, Y0: 840, X1: 900, Y1: 885, Label: "footer", Confidence: 0.9}, // y=280-295 in PDF
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

	items := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	// Footer at bottom edge → garbage → no table regions match
	if len(items) != 0 {
		t.Errorf("footer garbage: should not produce tables, got %d", len(items))
	}
}

// ---- helpers ----

func TestCellToPageSpace(t *testing.T) {
	cell := pdf.TSRCell{X0: 100, Y0: 200, X1: 300, Y1: 400, Text: "hello", Label: "table"}
	got := tbl.CellToPageSpace(cell, 15, 25, 3.0)

	// (100+15)/3 = 38.33..., (200+25)/3 = 75
	if got.X0 != 38.333333333333336 || got.Y0 != 75 || got.X1 != 105 || got.Y1 != 141.66666666666666 {
		t.Errorf("cellToPageSpace: got (%f,%f,%f,%f), want (38.33,75,105,141.67)", got.X0, got.Y0, got.X1, got.Y1)
	}
	if got.Text != "hello" || got.Label != "table" {
		t.Error("cellToPageSpace should preserve Text and Label")
	}
}

func TestCellAddOffset(t *testing.T) {
	cell := pdf.TSRCell{X0: 100, Y0: 200, X1: 300, Y1: 400, Text: "hello"}
	got := tbl.CellAddOffset(cell, 15, 25)
	if got.X0 != 115 || got.Y0 != 225 || got.X1 != 315 || got.Y1 != 425 {
		t.Errorf("cellAddOffset: got (%f,%f,%f,%f)", got.X0, got.Y0, got.X1, got.Y1)
	}
	if got.Text != "hello" {
		t.Error("cellAddOffset should preserve Text")
	}
}

func TestBoxToCropSpace(t *testing.T) {
	box := pdf.TextBox{X0: 50, X1: 200, Top: 100, Bottom: 300, Text: "text"}
	got := tbl.BoxToCropSpace(box, 3.0, 10, 20)
	if got.X0 != 140 || got.Top != 280 || got.X1 != 590 || got.Bottom != 880 {
		t.Errorf("boxToCropSpace: got (%f,%f,%f,%f)", got.X0, got.Top, got.X1, got.Bottom)
	}
	if got.Text != "text" {
		t.Error("boxToCropSpace should preserve Text")
	}
}

func TestCopyBoxAnnotations(t *testing.T) {
	src := &pdf.TextBox{R: 1, C: 2, RTop: 10, RBott: 20, H: 3, HTop: 30, HBott: 40,
		HLeft: 50, HRight: 60, CLeft: 70, CRight: 80, SP: 4}
	dst := &pdf.TextBox{}
	tbl.CopyBoxAnnotations(dst, src)
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "will be dropped via reference match"},
		{X0: 0, X1: 100, Top: 60, Bottom: 110, Text: "text box A"},
		{X0: 110, X1: 200, Top: 60, Bottom: 110, Text: "text box B"},
	}

	// Per-page subset (what enrichWithDeepDoc constructs from byPage[pg]).
	indices := []int{0, 1, 2}
	pageBoxes := make([]pdf.TextBox, len(indices))
	for i, idx := range indices {
		pageBoxes[i] = boxes[idx] // value copy
	}

	// DLA regions: one reference (garbage type → matched boxes are dropped
	// unless at page edge), two text regions for the surviving boxes.
	// scale=1.0 so DLA pixel coords == PDF point coords.
	regions := []pdf.DLARegion{
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
		tbl.CopyBoxAnnotations(&boxes[idx], &pageBoxes[i])
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50},
		{X0: 200, X1: 300, Top: 0, Bottom: 50},
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "table"},  // covers box 0 at scale 3
		{X0: 600, Y0: 0, X1: 900, Y1: 150, Label: "text"}, // non-table, ignored
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50},
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "text"},
		{X0: 0, Y0: 0, X1: 300, Y1: 150, Label: "figure"},
	}
	matches := matchTableRegions(boxes, regions, 3.0)
	if len(matches) != 0 {
		t.Errorf("non-table labels: expected 0 matches, got %d", len(matches))
	}
}

func TestMatchTableRegions_MultipleBoxesSameTable(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50},   // box 0
		{X0: 110, X1: 210, Top: 0, Bottom: 50}, // box 1
	}
	regions := []pdf.DLARegion{
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
	var boxes []pdf.TextBox // nil
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 100},
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 90, Y1: 90, Label: "table"}, // 30x30 at scale 3 → 9% overlap
	}
	matches := matchTableRegions(boxes, regions, 3.0)
	if len(matches) != 0 {
		t.Errorf("below threshold: expected 0 matches, got %d", len(matches))
	}
}

func TestCellSliceToPageSpace(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 100, Y0: 200, X1: 300, Y1: 400},
		{X0: 400, Y0: 200, X1: 600, Y1: 400},
	}
	got := tbl.CellSliceToPageSpace(cells, 15, 25, 3)
	if len(got) != 2 {
		t.Fatal("expected 2 cells")
	}
	if got[0].X0 != 38.333333333333336 || got[1].X0 != 138.33333333333334 {
		t.Error("wrong conversion")
	}
}

// MockTableBuilder is a test-only pdf.TableBuilder with a configurable GroupCells.
type MockTableBuilder struct {
	GroupCellsFn func(cells []pdf.TSRCell) [][]pdf.TSRCell
}

func (m *MockTableBuilder) Name() string { return "mock" }
func (m *MockTableBuilder) DetectCells(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (m *MockTableBuilder) GroupCells(cells []pdf.TSRCell) [][]pdf.TSRCell {
	if m.GroupCellsFn != nil {
		return m.GroupCellsFn(cells)
	}
	return nil
}

// ── writeTableAnnotations unit tests ──────────────────────────────────

func TestWriteTableAnnotations_WriteBack(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 100, Top: 10, Bottom: 30, Text: "A", LayoutType: "table"},
		{X0: 110, X1: 200, Top: 10, Bottom: 30, Text: "B", LayoutType: "table"},
		{X0: 10, X1: 100, Top: 35, Bottom: 55, Text: "C", LayoutType: "table"},
	}
	boxIdx := []int{0, 2}
	cells := []pdf.TSRCell{
		{X0: 30, Y0: 30, X1: 300, Y1: 90, Label: "table row"},
		{X0: 30, Y0: 110, X1: 300, Y1: 170, Label: "table row"},
	}
	scale := 3.0

	tb := &MockTableBuilder{GroupCellsFn: func(cells []pdf.TSRCell) [][]pdf.TSRCell {
		return [][]pdf.TSRCell{{cells[0]}, {cells[1]}}
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
	boxes := []pdf.TextBox{
		{X0: 10, X1: 100, Top: 10, Bottom: 50, Text: "X", LayoutType: "table"},
	}
	boxIdx := []int{0}
	cells := []pdf.TSRCell{
		{X0: 30, Y0: 30, X1: 300, Y1: 150, Label: "table row"},
	}
	scale := 3.0

	tb := &MockTableBuilder{GroupCellsFn: func(cells []pdf.TSRCell) [][]pdf.TSRCell {
		return [][]pdf.TSRCell{{cells[0]}}
	}}
	writeTableAnnotations(boxes, boxIdx, cells, scale, 0, 0, tb)

	// After scale-down: RTop / 3 should be in PDF space (~10).
	if boxes[0].RTop == 0 {
		t.Error("RTop should be non-zero after annotation")
	}
}

func TestWriteTableAnnotations_EmptyCells(t *testing.T) {
	boxes := []pdf.TextBox{{X0: 10, X1: 100, Top: 10, Bottom: 50, Text: "X", LayoutType: "table"}}
	boxIdx := []int{0}
	var cells []pdf.TSRCell

	tb := &MockTableBuilder{GroupCellsFn: func(cells []pdf.TSRCell) [][]pdf.TSRCell {
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
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 0, X1: 100, Top: 35, Bottom: 50, LayoutType: "table caption", Text: "表1：标题"},
	}
	tables := []pdf.TableItem{
		{Positions: []pdf.Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},
	}
	tbl.MarkNoMergeTables(boxes, tables)
	if !tables[0].NoMerge {
		t.Error("table followed by caption should be marked NoMerge")
	}
}

func TestMarkNoMergeTables_TitleAfterTable(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 0, X1: 100, Top: 35, Bottom: 50, LayoutType: "title"},
	}
	tables := []pdf.TableItem{
		{Positions: []pdf.Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},
	}
	tbl.MarkNoMergeTables(boxes, tables)
	if !tables[0].NoMerge {
		t.Error("table followed by title should be marked NoMerge")
	}
}

func TestMarkNoMergeTables_NoCaptionAfter(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 0, X1: 100, Top: 35, Bottom: 50, LayoutType: "text"},
		{X0: 0, X1: 100, Top: 55, Bottom: 70, LayoutType: "table"},
	}
	tables := []pdf.TableItem{
		{Positions: []pdf.Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},
		{Positions: []pdf.Position{{Left: 0, Right: 100, Top: 55, Bottom: 70}}},
	}
	tbl.MarkNoMergeTables(boxes, tables)
	if tables[0].NoMerge {
		t.Error("table followed by text should NOT be marked NoMerge")
	}
	if tables[1].NoMerge {
		t.Error("last table should NOT be marked NoMerge")
	}
}

func TestMarkNoMergeTables_StaleLastTableTI(t *testing.T) {
	// Scenario: table box that does NOT overlap any pdf.TableItem.Position
	// should reset lastTableTI. Otherwise the next caption marks the
	// wrong (non-adjacent) table as NoMerge.
	// Box 0: "table", overlaps table[0] → lastTableTI = 0
	// Box 1: "table", no overlap → lastTableTI should reset to -1
	// Box 2: "title" → should be a no-op (no adjacent table)
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, LayoutType: "table"},
		{X0: 500, X1: 600, Top: 100, Bottom: 130, LayoutType: "table"}, // far away, no overlap
		{X0: 0, X1: 100, Top: 140, Bottom: 160, LayoutType: "title"},
	}
	tables := []pdf.TableItem{
		{Positions: []pdf.Position{{Left: 0, Right: 100, Top: 0, Bottom: 30}}},  // table 0
		{Positions: []pdf.Position{{Left: 0, Right: 100, Top: 35, Bottom: 65}}}, // table 1 — box 0 doesn't overlap this either
	}
	tbl.MarkNoMergeTables(boxes, tables)
	// table[0] should NOT be NoMerge: the title follows a non-matching
	// table box, not table[0] directly.
	if tables[0].NoMerge {
		t.Error("stale lastTableTI: table[0] incorrectly marked NoMerge — " +
			"the non-overlapping table box (box 1) should have reset lastTableTI")
	}
}

func TestMarkNoMergeTables_EmptyInputs(t *testing.T) {
	// Should not panic with empty inputs.
	tbl.MarkNoMergeTables(nil, nil)
	tbl.MarkNoMergeTables([]pdf.TextBox{}, []pdf.TableItem{})
}
