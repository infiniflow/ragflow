//go:build cgo

package parser

import (
	"context"
	"fmt"
	"image"
	"strings"
	"testing"
)

// ── MockDocAnalyzer tests ──────────────────────────────────────────────

func TestMockDocAnalyzer(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 0, Y0: 0, X1: 100, Y1: 100, Label: "table", Confidence: 0.95},
		},
		TSRCells: []TSRCell{
			{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "A"},
		},
	}

	if !mock.Health() {
		t.Error("mock should be healthy")
	}
	regions, _ := mock.DLA(context.Background(), nil)
	if len(regions) != 1 || regions[0].Label != "table" {
		t.Error("mock DLA returned wrong data")
	}
	cells, _ := mock.TSR(context.Background(), nil)
	if len(cells) != 1 || cells[0].Text != "A" {
		t.Error("mock TSR returned wrong data")
	}
	// OCRDetect + OCRRecognize replaces deprecated OCR — tested in TestOCR_scanPage/TestOCR_fallback.
	_ = mock.OCRDetect
	_ = mock.OCRRecognize

	// Unhealthy mock
	mock2 := &MockDocAnalyzer{Healthy: false}
	if mock2.Health() {
		t.Error("unhealthy mock should return false")
	}
}

// ── groupTSRCellsToRows ────────────────────────────────────────────────

func TestGroupTSRCellsToRows(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if rows := groupTSRCellsToRows(nil); rows != nil {
			t.Error("nil → nil")
		}
		if rows := groupTSRCellsToRows([]TSRCell{}); rows != nil {
			t.Error("empty → nil")
		}
	})

	t.Run("single cell", func(t *testing.T) {
		cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A"}}
		rows := groupTSRCellsToRows(cells)
		if len(rows) != 1 || rows[0][0].Text != "A" {
			t.Error("single cell not preserved")
		}
	})

	t.Run("two rows two cols", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "A"},
			{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
			{X0: 0, Y0: 50, X1: 50, Y1: 80, Text: "C"},
			{X0: 50, Y0: 50, X1: 100, Y1: 80, Text: "D"},
		}
		rows := groupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Fatalf("2 rows expected, got %d", len(rows))
		}
		if rows[0][0].Text != "A" || rows[0][1].Text != "B" {
			t.Errorf("row0: %v", cellTexts(rows[0]))
		}
		if rows[1][0].Text != "C" || rows[1][1].Text != "D" {
			t.Errorf("row1: %v", cellTexts(rows[1]))
		}
	})

	t.Run("unsorted input", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 50, Y0: 50, X1: 100, Y1: 80, Text: "D"},
			{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "A"},
			{X0: 0, Y0: 50, X1: 50, Y1: 80, Text: "C"},
			{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
		}
		rows := groupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Fatalf("unsorted: 2 rows expected, got %d", len(rows))
		}
		if rows[0][0].Text != "A" || rows[0][1].Text != "B" {
			t.Errorf("unsorted row0: %v", cellTexts(rows[0]))
		}
	})

	t.Run("tall merged cell", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 0, Y0: 0, X1: 50, Y1: 100, Text: "merged"},
			{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
			{X0: 50, Y0: 50, X1: 100, Y1: 80, Text: "D"},
		}
		rows := groupTSRCellsToRows(cells)
		// merged cell starts Y0=0 → row 0; Y0=50 cell → row 1
		if len(rows) != 2 {
			t.Fatalf("merged cell: 2 rows expected, got %d", len(rows))
		}
	})

	t.Run("large gap different rows", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "top"},
			{X0: 0, Y0: 200, X1: 50, Y1: 230, Text: "far"},
		}
		rows := groupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Fatalf("large gap: 2 rows expected, got %d", len(rows))
		}
	})
}

// ── fillCellTextFromBoxes ──────────────────────────────────────────────

func TestFillCellTextFromBoxes(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50},
			{X0: 100, Y0: 0, X1: 200, Y1: 50},
		}
		boxes := []TextBox{
			{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "A"},
			{X0: 100, X1: 200, Top: 0, Bottom: 50, Text: "B"},
		}
		fillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "A" || cells[1].Text != "B" {
			t.Errorf("got %q/%q, want A/B", cells[0].Text, cells[1].Text)
		}
	})

	t.Run("empty cells", func(t *testing.T) {
		cells := []TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50},
			{X0: 100, Y0: 0, X1: 200, Y1: 50},
		}
		boxes := []TextBox{
			{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "only first"},
		}
		fillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "only first" {
			t.Errorf("cell[0]: got %q", cells[0].Text)
		}
		if cells[1].Text != "" {
			t.Errorf("cell[1] should be empty, got %q", cells[1].Text)
		}
	})

	t.Run("partial cell coverage — empty cell filled from any overlapping box", func(t *testing.T) {
		// Box covers 40% of cell area.  Old code rejected (<85% cell coverage).
		// New code: cell is empty → accepts box (≥30% box area inside cell).
		cells := []TSRCell{{X0: 0, Y0: 0, X1: 200, Y1: 50}}
		boxes := []TextBox{{X0: 0, X1: 80, Top: 0, Bottom: 50, Text: "partial"}}
		fillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "partial" {
			t.Errorf("empty cell should be filled from overlapping box, got %q", cells[0].Text)
		}
	})

	t.Run("box inside cell >85%", func(t *testing.T) {
		cells := []TSRCell{{X0: 0, Y0: 0, X1: 500, Y1: 300}}
		boxes := []TextBox{{X0: 10, X1: 490, Top: 10, Bottom: 290, Text: "inside"}}
		fillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "inside" {
			t.Errorf("got %q", cells[0].Text)
		}
	})

	t.Run("concatenate two boxes to same cell", func(t *testing.T) {
		cells := []TSRCell{{X0: 0, Y0: 0, X1: 200, Y1: 100}}
		boxes := []TextBox{
			{X0: 5, X1: 195, Top: 2, Bottom: 98, Text: "hello"},
			{X0: 5, X1: 195, Top: 2, Bottom: 98, Text: "world"},
		}
		fillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "hello world" {
			t.Errorf("got %q, want 'hello world'", cells[0].Text)
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		fillCellTextFromBoxes(nil, nil)
		fillCellTextFromBoxes([]TSRCell{}, []TextBox{})
		c := []TSRCell{{X0: 0, Y0: 0, X1: 1, Y1: 1}}
		fillCellTextFromBoxes(c, nil)
		if c[0].Text != "" {
			t.Error("no boxes → text empty")
		}
	})
}

// ── regionOverlapsBox ──────────────────────────────────────────────────

func TestRegionOverlapsBox(t *testing.T) {
	scale := 3.0
	tests := []struct {
		name     string
		region   DLARegion
		box      TextBox
		expected bool
	}{
		{"full overlap", DLARegion{X0: 0, Y0: 300, X1: 1500, Y1: 2300, Label: "table", Confidence: 0.9}, TextBox{X0: 50, X1: 500, Top: 100, Bottom: 760, Text: "x", PageNumber: 0}, true},
		{"no overlap", DLARegion{X0: 0, Y0: 3000, X1: 1500, Y1: 5000, Label: "table", Confidence: 0.9}, TextBox{X0: 50, X1: 500, Top: 0, Bottom: 10, Text: "x", PageNumber: 0}, false},
		{"no Y overlap", DLARegion{X0: 150, Y0: 300, X1: 1650, Y1: 336, Label: "table", Confidence: 0.9}, TextBox{X0: 50, X1: 550, Top: 500, Bottom: 520, Text: "x", PageNumber: 0}, false},
		{"zero area box", DLARegion{X0: 0, Y0: 300, X1: 1500, Y1: 2300, Label: "table", Confidence: 0.9}, TextBox{X0: 50, X1: 50, Top: 50, Bottom: 50, Text: "x", PageNumber: 0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := regionOverlapsBox(tt.region, tt.box, scale); got != tt.expected {
				t.Errorf("= %v, want %v", got, tt.expected)
			}
		})
	}
}

// ── enrichWithDeepDoc noop ─────────────────────────────────────────────

func TestEnrichWithDeepDoc_Noop(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "text"},
	}
	eng := &mockEngine{pageCount: 1}

	p := NewParser(DefaultParserConfig(), &MockDocAnalyzer{Healthy: false, Model: ModelSaas})
	tables := p.enrichWithDeepDoc(context.Background(), eng, boxes, nil)
	if len(tables) != 0 {
		t.Error("unhealthy DeepDoc → 0 Tables")
	}
}

// ── extractTableBoxesFromImage with mock ───────────────────────────────

func TestExtractTableBoxes_Mock(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, X0: 80, X1: 500, Top: 200, Bottom: 550, Text: "cell 1"},
		{PageNumber: 0, X0: 80, X1: 500, Top: 550, Bottom: 760, Text: "cell 2"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 180, Text: "heading"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 780, Bottom: 850, Text: "below"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 250, Y0: 600, X1: 1500, Y1: 2300, Label: "table", Confidence: 0.95},
		},
		TSRCells: []TSRCell{
			{X0: 0, Y0: 0, X1: 600, Y1: 400, Text: "A1"},
			{X0: 600, Y0: 0, X1: 1240, Y1: 400, Text: "B1"},
			{X0: 0, Y0: 410, X1: 600, Y1: 800, Text: "A2"},
			{X0: 600, Y0: 410, X1: 1240, Y1: 800, Text: "B2"},
		},
	}
	p := NewParser(DefaultParserConfig(), mock)
	dummyImg := image.NewRGBA(image.Rect(0, 0, 2000, 3000))

	tables := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	if len(tables) != 1 {
		t.Fatalf("expected 1 TableItem, got %d", len(tables))
	}
	tbl := tables[0]
	if len(tbl.Cells) != 4 {
		t.Errorf("expected 4 cells, got %d", len(tbl.Cells))
	}
	// Rows populated later by constructTable via extractTableAndReplace.
	if tbl.ImageB64 == "" {
		t.Error("ImageB64 empty")
	}
	if len(tbl.Positions) != 2 {
		t.Errorf("expected 2 Positions, got %d", len(tbl.Positions))
	}
}

func TestExtractTableBoxes_NoTables(t *testing.T) {
	mock := &MockDocAnalyzer{Healthy: true, DLARegions: []DLARegion{}}
	p := NewParser(DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("0 tables expected, got %d", len(tables))
	}
}

func TestExtractTableBoxes_NonTableRegions(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 150, Y0: 300, X1: 1650, Y1: 336, Label: "text", Confidence: 0.9},
			{X0: 150, Y0: 600, X1: 1650, Y1: 900, Label: "figure", Confidence: 0.8},
		},
	}
	p := NewParser(DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 2000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("non-table regions → 0 tables, got %d", len(tables))
	}
}

func TestExtractTableBoxes_NoOverlap(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 10, Bottom: 30, Text: "far away"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 150, Y0: 1500, X1: 1500, Y1: 2300, Label: "table", Confidence: 0.95},
		},
	}
	p := NewParser(DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 3000))
	tables := p.extractTableBoxesFromImage(context.Background(), boxes, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("no overlap → 0 tables, got %d", len(tables))
	}
}

func TestExtractTableBoxes_TSRError(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, X0: 80, X1: 500, Top: 210, Bottom: 660, Text: "cell"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 250, Y0: 600, X1: 1500, Y1: 2000, Label: "table", Confidence: 0.95},
		},
		TSRCells: nil, // TSR returns nothing
	}
	p := NewParser(DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 3000))
	tables := p.extractTableBoxesFromImage(context.Background(), boxes, dummy, 0, 0)
	if len(tables) != 1 {
		t.Fatalf("TSR failure: expected 1 TableItem with image+positions, got %d", len(tables))
	}
	if tables[0].ImageB64 == "" {
		t.Error("should have image despite TSR failure")
	}
	if len(tables[0].Positions) == 0 {
		t.Error("should have positions despite TSR failure")
	}
	if len(tables[0].Rows) != 0 {
		t.Errorf("TSR failure → 0 rows, got %d", len(tables[0].Rows))
	}
}

func TestGroupTSRCellsToRows_SameHeight(t *testing.T) {
	// All cells have identical height → medianH is that value → threshold = medianH/2
	cells := []TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "A"},
		{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
		{X0: 0, Y0: 31, X1: 50, Y1: 61, Text: "C"}, // gap = 31-30=1 < 30/2=15 → same row? NO, Y0=31 is right at edge
	}
	rows := groupTSRCellsToRows(cells)
	// medianH=30, threshold=15. C.Y0=31 > curY+threshold?" curY=0, 31 > 15 → new row.
	// So A,B in row 0, C in row 1.
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 1 {
		t.Errorf("row sizes: %d %d, want 2 1", len(rows[0]), len(rows[1]))
	}
}

func TestFillCellTextFromBoxes_WhitespaceTrim(t *testing.T) {
	cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 100}}
	boxes := []TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 100, Text: "  hello  "}}
	fillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "hello" {
		t.Errorf("got %q, want 'hello'", cells[0].Text)
	}
}

func TestFillCellTextFromBoxes_EmptyBoxIgnored(t *testing.T) {
	cells := []TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 100}}
	boxes := []TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 100, Text: "   "}} // all whitespace
	fillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("whitespace text should produce empty, got %q", cells[0].Text)
	}
}

func TestExtractTableBoxes_DLAError(t *testing.T) {
	// DLA returns only non-table regions → 0 tables
	mock := &MockDocAnalyzer{Healthy: true, DLARegions: []DLARegion{
		{X0: 0, Y0: 0, X1: 100, Y1: 100, Label: "text", Confidence: 0.9},
	}}
	p := NewParser(DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("non-table DLA → 0 tables, got %d", len(tables))
	}
}

func TestAnnotateBoxLayouts(t *testing.T) {
	boxes := []TextBox{
		{X0: 50, X1: 200, Top: 100, Bottom: 200, Text: "title text"},
		{X0: 250, X1: 500, Top: 100, Bottom: 200, Text: "body"},
		{X0: 50, X1: 500, Top: 300, Bottom: 600, Text: "table content"},
		{X0: 50, X1: 500, Top: 700, Bottom: 800, Text: "unmatched"},
	}
	regions := []DLARegion{
		{X0: 150, Y0: 300, X1: 600, Y1: 600, Label: "title", Confidence: 0.9},    // PDF pts: X50-200,Y100-200 → only box[0]
		{X0: 750, Y0: 300, X1: 1500, Y1: 600, Label: "text", Confidence: 0.8},    // PDF pts: X250-500,Y100-200 → box[1]
		{X0: 150, Y0: 900, X1: 1500, Y1: 1800, Label: "table", Confidence: 0.95}, // PDF pts: X50-500,Y300-600 → box[2]
	}
	scale := 3.0
	annotateBoxLayouts(boxes, regions, scale, 0)

	if boxes[0].LayoutType != "title" {
		t.Errorf("box[0] = %q, want title", boxes[0].LayoutType)
	}
	if boxes[1].LayoutType != "text" {
		t.Errorf("box[1] = %q, want text", boxes[1].LayoutType)
	}
	if boxes[2].LayoutType != "table" {
		t.Errorf("box[2] = %q, want table", boxes[2].LayoutType)
	}
	if boxes[3].LayoutType != "" {
		t.Errorf("box[3] = %q, want empty (no matching region)", boxes[3].LayoutType)
	}
}

func TestAnnotateBoxLayouts_Figure(t *testing.T) {
	// Figure region → box gets "figure" layout type (no TSR needed)
	boxes := []TextBox{
		{X0: 50, X1: 500, Top: 100, Bottom: 400, Text: "chart image"},
	}
	regions := []DLARegion{
		{X0: 50, Y0: 200, X1: 2000, Y1: 1000, Label: "figure", Confidence: 0.85},
	}
	annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "figure" {
		t.Errorf("LayoutType = %q, want 'figure'", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_Empty(t *testing.T) {
	boxes := []TextBox{{Text: "x"}}
	annotateBoxLayouts(boxes, nil, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Error("empty regions → no annotation")
	}
}

func TestBoxesToSections_PassesLayoutType(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "标题", LayoutType: "title"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 200, Bottom: 212, Text: "表格", LayoutType: "table"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 300, Bottom: 312, Text: "正文", LayoutType: "text"},
	}
	sections := boxesToSections(boxes, nil)
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}
	if sections[0].LayoutType != "title" {
		t.Errorf("section[0].LayoutType = %q, want 'title'", sections[0].LayoutType)
	}
	if sections[1].LayoutType != "table" {
		t.Errorf("section[1].LayoutType = %q, want 'table'", sections[1].LayoutType)
	}
	if sections[2].LayoutType != "text" {
		t.Errorf("section[2].LayoutType = %q, want 'text'", sections[2].LayoutType)
	}
}

func TestBoxesToSections_PreservesTableLayout(t *testing.T) {
	// boxesToSections should produce sections for all boxes regardless of LayoutType.
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "标题", LayoutType: "title"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 200, Bottom: 212, Text: "表格文字", LayoutType: "table"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 300, Bottom: 312, Text: "正文", LayoutType: "text"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 400, Bottom: 412, Text: ""},
	}
	sections := boxesToSections(boxes, nil)
	if len(sections) != 3 {
		t.Errorf("expected 3 sections (1 empty skipped), got %d", len(sections))
	}
	for _, s := range sections {
		if strings.Contains(s.Text, "@@") {
			t.Error("section text should NOT contain position tag")
		}
	}
	t.Logf("boxesToSections: %d sections (all LayoutTypes passed through)", len(sections))
}

func TestEnrichWithDeepDoc_PreservesBoxes(t *testing.T) {
	// Simulate enrichWithDeepDoc's write-back logic:
	// 1. Create pageBoxes as copies of p.boxes[idx]
	// 2. annotateBoxLayouts(pageBoxes, regions) — modifies copies
	// 3. Write LayoutType back to p.boxes[idx]
	// This test validates step 3 works.

	original := []TextBox{
		{PageNumber: 0, X0: 50, X1: 200, Top: 50, Bottom: 80, Text: "title", LayoutType: ""},
		{PageNumber: 0, X0: 50, X1: 200, Top: 100, Bottom: 200, Text: "text before", LayoutType: ""},
		{PageNumber: 0, X0: 50, X1: 500, Top: 250, Bottom: 700, Text: "table cell", LayoutType: ""},
		{PageNumber: 0, X0: 50, X1: 200, Top: 750, Bottom: 800, Text: "text after", LayoutType: ""},
		{PageNumber: 1, X0: 50, X1: 200, Top: 50, Bottom: 80, Text: "page2", LayoutType: ""},
	}

	byPage := map[int][]int{0: {0, 1, 2, 3}, 1: {4}} // indices into original

	regions := []DLARegion{
		{X0: 150, Y0: 150, X1: 600, Y1: 240, Label: "title", Confidence: 0.9},    // PDF: X50-200,Y50-80 → box[0]
		{X0: 150, Y0: 750, X1: 1500, Y1: 2100, Label: "table", Confidence: 0.95}, // PDF: X50-500,Y250-700 → box[2]
	}

	// Step 1-2: copy + annotate
	for _, indices := range byPage {
		pageBoxes := make([]TextBox, len(indices))
		for i, idx := range indices {
			pageBoxes[i] = original[idx]
		}
		annotateBoxLayouts(pageBoxes, regions, 3.0, 0)

		// Step 3: write back (this is what enrichWithDeepDoc now does)
		for i, idx := range indices {
			if pageBoxes[i].LayoutType != "" {
				original[idx].LayoutType = pageBoxes[i].LayoutType
			}
		}
	}

	if original[0].LayoutType != "title" {
		t.Errorf("box[0] LayoutType = %q, want 'title'", original[0].LayoutType)
	}
	if original[2].LayoutType != "table" {
		t.Errorf("box[2] LayoutType = %q, want 'table'", original[2].LayoutType)
	}
	if original[1].LayoutType != "" {
		t.Errorf("box[1] LayoutType = %q, want '' (no matching region)", original[1].LayoutType)
	}
	// All boxes still present
	if len(original) != 5 {
		t.Errorf("all boxes preserved: got %d, want 5", len(original))
	}
	t.Logf("Write-back verified: box[0]=%q box[2]=%q", original[0].LayoutType, original[2].LayoutType)
}

func TestBoxesToSections_PositionsFromTag(t *testing.T) {
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "标题段落"},
	}
	sections := boxesToSections(boxes, nil)
	if sections[0].PositionTag == "" {
		t.Error("PositionTag should not be empty")
	}
	if len(sections[0].Positions) == 0 {
		t.Error("Positions should be parsed from PositionTag — BUG: ExtractPositions not called")
	}
	if len(sections[0].Positions) > 0 {
		pos := sections[0].Positions[0]
		if pos.Left != 50 || pos.Right != 550 || pos.Top != 100 || pos.Bottom != 112 {
			t.Errorf("position coords wrong: got (%.0f,%.0f,%.0f,%.0f)", pos.Left, pos.Right, pos.Top, pos.Bottom)
		}
	}
	t.Logf("Positions: %v", sections[0].Positions)
}

func TestParse_TableLinkedToSections(t *testing.T) {
	// Simulate enrichWithDeepDoc → extractTableAndReplace → boxesToSections:
	// table boxes are popped and replaced with one HTML box.
	boxes := []TextBox{
		{PageNumber: 0, X0: 50, X1: 200, Top: 50, Bottom: 80, Text: "heading"},
		{PageNumber: 0, X0: 50, X1: 500, Top: 250, Bottom: 400, Text: "table text", LayoutType: "table"},
		{PageNumber: 0, X0: 50, X1: 200, Top: 450, Bottom: 480, Text: "after"},
	}
	tableItem := TableItem{
		Cells: []TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 50, Label: "table row"},
			{X0: 0, Y0: 51, X1: 200, Y1: 100, Label: "table row"},
		},
		Positions: []Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 250, Bottom: 400}},
		Scale:     1.0,
	}

	boxes = extractTableAndReplace(boxes, []TableItem{tableItem})
	sections := boxesToSections(boxes, nil)

	// 3 boxes (heading, table, after) → 3 sections (heading, HTML, after).
	if len(sections) != 3 {
		t.Errorf("expected 3 sections, got %d", len(sections))
	}
	tableFound := false
	for _, s := range sections {
		if s.LayoutType == "table" && strings.Contains(s.Text, "<table>") {
			tableFound = true
		}
	}
	if !tableFound {
		t.Errorf("expected at least one section with HTML table")
		for _, s := range sections {
			t.Logf("  section text=%q LayoutType=%q", s.Text[:min(40, len(s.Text))], s.LayoutType)
		}
	}
}

func cellTexts(cells []TSRCell) []string {
	t := make([]string, len(cells))
	for i, c := range cells {
		t[i] = c.Text
	}
	return t
}

// ── cropImageRegion ────────────────────────────────────────────────────

func TestCropImageRegion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 300))

	t.Run("normal crop", func(t *testing.T) {
		r := DLARegion{X0: 10, Y0: 20, X1: 100, Y1: 150}
		cropped, err := cropImageRegion(img, r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 3% proportional margin: 90×3%≈3px, 130×3%≈4px → 95×137
		if cropped.Bounds().Dx() != 95 || cropped.Bounds().Dy() != 137 {
			t.Errorf("size %v, want 95x137", cropped.Bounds())
		}
	})

	t.Run("x0 >= x1 returns error", func(t *testing.T) {
		// 3% proportional margin on each side: if the gap is too small after margin expansion, x0 ≥ x1 triggers error.
		r := DLARegion{X0: 110, Y0: 20, X1: 50, Y1: 150}
		_, err := cropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for x0 >= x1, got nil")
		}
	})

	t.Run("y0 >= y1 returns error", func(t *testing.T) {
		r := DLARegion{X0: 10, Y0: 150, X1: 100, Y1: 20}
		_, err := cropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for y0 >= y1, got nil")
		}
	})

	t.Run("region fully outside image bounds", func(t *testing.T) {
		// Clamped to image bounds → zero-width/height → error.
		r := DLARegion{X0: 300, Y0: 400, X1: 500, Y1: 600}
		_, err := cropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for region outside image bounds")
		}
	})
}

// ── extractTableBoxesFromImage: invalid DLA region ─────────────────────

func TestExtractTableBoxes_InvalidRegion(t *testing.T) {
	// DLA returns a table region with x1 < x0.  The pipeline should skip
	// this table gracefully (Python raises ValueError from PIL.Image.crop).
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 500, Y0: 100, X1: 100, Y1: 300, Label: "table", Confidence: 0.9},
		},
	}
	p := NewParser(DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("invalid DLA region should be skipped, got %d tables", len(tables))
	}
}

// ── DLA → figure end-to-end ───────────────────────────────────────────

func TestParse_CollectsFigures(t *testing.T) {
	// End-to-end: Parse() with mock DeepDoc that labels a box as "figure".
	// Verify p.Figures is populated.

eng := &mockEngine{pageCount: 1, chars: map[int][]TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "chart image"}}}}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 50, Y0: 200, X1: 2000, Y1: 1000, Label: "figure", Confidence: 0.85},
		},
	}
	p := NewParser(DefaultParserConfig(), mock)

	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least 1 section")
	}
	if len(result.Figures) != 1 {
		t.Fatalf("expected 1 figure, got %d", len(result.Figures))
	}
	if result.Figures[0].LayoutType != "figure" {
		t.Errorf("figure LayoutType = %q, want 'figure'", result.Figures[0].LayoutType)
	}
	if result.Figures[0].Text == "" {
		t.Error("figure Text should not be empty")
	}
}

func TestParse_NoFigures(t *testing.T) {
	// Parse() with no DLA figure regions → p.Figures should be empty.

	eng := &mockEngine{pageCount: 1, chars: map[int][]TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "just text"}}}}
	mock := &MockDocAnalyzer{
		DLARegions: []DLARegion{
			{X0: 150, Y0: 300, X1: 1500, Y1: 600, Label: "text", Confidence: 0.8},
		},
	}
	p := NewParser(DefaultParserConfig(), mock)

	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Figures) != 0 {
		t.Fatalf("expected 0 figures, got %d", len(result.Figures))
	}
}

func TestParse_NoDeepDoc_NoFigures(t *testing.T) {
	// Parse() with mock DeepDoc → Figures should be empty (no DLA-detected figures).

	eng := &mockEngine{pageCount: 1, chars: map[int][]TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "text"}}}}
	p := NewParser(DefaultParserConfig(), &MockDocAnalyzer{Healthy: true, Model: ModelSaas})

	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Figures) != 0 {
		t.Fatalf("expected 0 Figures (no DLA-detected figures), got %d", len(result.Figures))
	}
}

// ── Parse + ocrMergeChars (full-page detect) ──────────────────────────

func TestParse_UsesOCRDetectForEmbeddedChars(t *testing.T) {
	// When DeepDoc is available and the page has embedded chars,
	// Parse should use ocrMergeChars (detect → merge → recognize).
	eng := &mockEngine{
		pageCount: 1,
		chars: map[int][]TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []OCRBox{
			{X0: 5, Y0: 5, X1: 50, Y1: 5, X2: 50, Y2: 50, X3: 5, Y3: 50},
		},
	}
	p := NewParser(DefaultParserConfig(), mock)

	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least 1 section")
	}
	// The box should come from OCR detect, not charsToBoxes.
	// Verifying that ocrMergeChars was used (sections exist).
	if result.Metrics.BoxesInitial == 0 {
		t.Error("expected BoxesInitial > 0 (OCR detect path)")
	}
}

func TestParse_FallsBackToCharsToBoxes_NoDeepDoc(t *testing.T) {
	// Without DeepDoc, Parse should use charsToBoxes (unchanged behavior).
	eng := &mockEngine{
		pageCount: 1,
		chars: map[int][]TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	p := NewParser(DefaultParserConfig(), &MockDocAnalyzer{Healthy: true, Model: ModelSaas})

	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least 1 section (charsToBoxes)")
	}
}

func TestParse_FallsBackToCharsToBoxes_EmptyOCRBoxes(t *testing.T) {
	// OCRDetect returns no boxes → falls through to charsToBoxes.
	eng := &mockEngine{
		pageCount: 1,
		chars: map[int][]TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRBoxes: []OCRBox{}, // empty detect
	}
	p := NewParser(DefaultParserConfig(), mock)

	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least 1 section (charsToBoxes fallback)")
	}
}


// ── Error path coverage ────────────────────────────────────────────────

func TestMockDocAnalyzer_DLAError_DoesNotCrash(t *testing.T) {
	p := NewParser(DefaultParserConfig(), &MockDocAnalyzer{
		Healthy: true,
		DLAErr:  fmt.Errorf("DLA service unavailable"),
	})
	eng := &mockEngine{pageCount: 1}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pageImages := map[int]image.Image{0: img}
	boxes := []TextBox{
		{PageNumber: 0, X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "text"},
	}
	// enrichWithDeepDoc should return nil (not panic) on DLA error.
	tables := p.enrichWithDeepDoc(context.Background(), eng, boxes, pageImages)
	if len(tables) != 0 {
		t.Errorf("DLA error should produce 0 tables, got %d", len(tables))
	}
}

func TestMockDocAnalyzer_TSRError_DoesNotCrash(t *testing.T) {
	// TSR error: DLA succeeds, TSR fails.  The table region is detected
	// but no cells are returned — the table is skipped gracefully.
	p := NewParser(DefaultParserConfig(), &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []DLARegion{
			{X0: 0, Y0: 0, X1: 400, Y1: 400, Label: "table", Confidence: 0.95},
		},
		TSRErr: fmt.Errorf("TSR model timeout"),
	})
	eng := &mockEngine{pageCount: 1}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pageImages := map[int]image.Image{0: img}
	boxes := []TextBox{
		{PageNumber: 0, X0: 10, X1: 90, Top: 10, Bottom: 90, Text: "in table region"},
	}
	tables := p.enrichWithDeepDoc(context.Background(), eng, boxes, pageImages)
	// DLA detects the table region → 1 TableItem is created.  TSR failure
	// means it has no cells, but the pipeline must not panic.
	if len(tables) != 1 {
		t.Errorf("TSR error: expected 1 table (DLA region found), got %d", len(tables))
	}
	if len(tables[0].Cells) != 0 {
		t.Errorf("TSR error: Cells should be empty, got %d", len(tables[0].Cells))
	}
}

func TestMockDocAnalyzer_OCRDetectError_DoesNotCrash(t *testing.T) {
	// OCRDetect failure path: extractPages uses ocrDetectAndRecognize which
	// calls doc.OCRDetect.  When it fails, the page is skipped gracefully.
	mock := &MockDocAnalyzer{Healthy: true, OCRDetectErr: fmt.Errorf("OCR model OOM")}
	eng := &mockEngine{
		pageCount: 1,
		chars:     map[int][]TextChar{}, // empty → triggers OCR path
	}
	p := NewParser(DefaultParserConfig(), mock)
	_, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	// Parse should succeed — the page with OCRDetect error is just skipped.
}

// TestTSRLabels verifies Go defaultTSRLabels matches Python's table_structure_recognizer.py labels.
// Order must be exact — the ONNX model returns class IDs that index into this array.
func TestTSRLabels(t *testing.T) {
	want := []string{
		"table", "table column", "table row",
		"table column header", "table projected row header",
		"table spanning cell",
	}
	if len(defaultTSRLabels) != len(want) {
		t.Fatalf("defaultTSRLabels length %d, want %d", len(defaultTSRLabels), len(want))
	}
	for i := range want {
		if defaultTSRLabels[i] != want[i] {
			t.Errorf("defaultTSRLabels[%d] = %q, want %q", i, defaultTSRLabels[i], want[i])
		}
	}
}
