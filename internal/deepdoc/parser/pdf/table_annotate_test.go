package parser

import (
	"context"
	"image"
	"testing"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
)

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
	_ = tbl.AnnotateBoxLayouts(pageBoxes, regions, 1.0, pageImgHeight)

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
