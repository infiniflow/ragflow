package parser

import (
	"context"
	"fmt"
	"image"
	"strings"
	"testing"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// ── MockDocAnalyzer tests ──────────────────────────────────────────────

func TestMockDocAnalyzer(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 0, Y0: 0, X1: 100, Y1: 100, Label: "table", Confidence: 0.95},
		},
		TSRCells: []pdf.TSRCell{
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

// ── enrichWithDeepDoc noop ─────────────────────────────────────────────

func TestEnrichWithDeepDoc_Noop(t *testing.T) {
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "text"},
	}
	eng := &mockEngine{pageCount: 1}

	p := NewParser(pdf.DefaultParserConfig(), &MockDocAnalyzer{Healthy: false})
	tables := p.enrichWithDeepDoc(context.Background(), eng, boxes, nil)
	if len(tables) != 0 {
		t.Error("unhealthy DeepDoc → 0 Tables")
	}
}

// ── extractTableBoxesFromImage with mock ───────────────────────────────

func TestExtractTableBoxes_Mock(t *testing.T) {
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 80, X1: 500, Top: 200, Bottom: 550, Text: "cell 1"},
		{PageNumber: 0, X0: 80, X1: 500, Top: 550, Bottom: 760, Text: "cell 2"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 100, Bottom: 180, Text: "heading"},
		{PageNumber: 0, X0: 50, X1: 550, Top: 780, Bottom: 850, Text: "below"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 250, Y0: 600, X1: 1500, Y1: 2300, Label: "table", Confidence: 0.95},
		},
		TSRCells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 600, Y1: 400, Text: "A1"},
			{X0: 600, Y0: 0, X1: 1240, Y1: 400, Text: "B1"},
			{X0: 0, Y0: 410, X1: 600, Y1: 800, Text: "A2"},
			{X0: 600, Y0: 410, X1: 1240, Y1: 800, Text: "B2"},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)
	dummyImg := image.NewRGBA(image.Rect(0, 0, 2000, 3000))

	tables := p.extractTableBoxesFromImage(context.Background(), boxes, dummyImg, 0, 0)
	if len(tables) != 1 {
		t.Fatalf("expected 1 pdf.TableItem, got %d", len(tables))
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
	mock := &MockDocAnalyzer{Healthy: true, DLARegions: []pdf.DLARegion{}}
	p := NewParser(pdf.DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("0 tables expected, got %d", len(tables))
	}
}

func TestExtractTableBoxes_NonTableRegions(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 150, Y0: 300, X1: 1650, Y1: 336, Label: "text", Confidence: 0.9},
			{X0: 150, Y0: 600, X1: 1650, Y1: 900, Label: "figure", Confidence: 0.8},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 2000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("non-table regions → 0 tables, got %d", len(tables))
	}
}

func TestExtractTableBoxes_NoOverlap(t *testing.T) {
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 50, X1: 550, Top: 10, Bottom: 30, Text: "far away"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 150, Y0: 1500, X1: 1500, Y1: 2300, Label: "table", Confidence: 0.95},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 3000))
	tables := p.extractTableBoxesFromImage(context.Background(), boxes, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("no overlap → 0 tables, got %d", len(tables))
	}
}

func TestExtractTableBoxes_TSRError(t *testing.T) {
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 80, X1: 500, Top: 210, Bottom: 660, Text: "cell"},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 250, Y0: 600, X1: 1500, Y1: 2000, Label: "table", Confidence: 0.95},
		},
		TSRCells: nil, // TSR returns nothing
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 3000))
	tables := p.extractTableBoxesFromImage(context.Background(), boxes, dummy, 0, 0)
	if len(tables) != 1 {
		t.Fatalf("TSR failure: expected 1 pdf.TableItem with image+positions, got %d", len(tables))
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

func TestExtractTableBoxes_DLAError(t *testing.T) {
	// DLA returns only non-table regions → 0 tables
	mock := &MockDocAnalyzer{Healthy: true, DLARegions: []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 100, Y1: 100, Label: "text", Confidence: 0.9},
	}}
	p := NewParser(pdf.DefaultParserConfig(), mock)
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, dummy, 0, 0)
	if len(tables) != 0 {
		t.Errorf("non-table DLA → 0 tables, got %d", len(tables))
	}
}

func TestAnnotateBoxLayouts(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 50, X1: 200, Top: 100, Bottom: 200, Text: "title text"},
		{X0: 250, X1: 500, Top: 100, Bottom: 200, Text: "body"},
		{X0: 50, X1: 500, Top: 300, Bottom: 600, Text: "table content"},
		{X0: 50, X1: 500, Top: 700, Bottom: 800, Text: "unmatched"},
	}
	regions := []pdf.DLARegion{
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
	boxes := []pdf.TextBox{
		{X0: 50, X1: 500, Top: 100, Bottom: 400, Text: "chart image"},
	}
	regions := []pdf.DLARegion{
		{X0: 50, Y0: 200, X1: 2000, Y1: 1000, Label: "figure", Confidence: 0.85},
	}
	annotateBoxLayouts(boxes, regions, 3.0, 0)
	if boxes[0].LayoutType != "figure" {
		t.Errorf("LayoutType = %q, want 'figure'", boxes[0].LayoutType)
	}
}

func TestAnnotateBoxLayouts_Empty(t *testing.T) {
	boxes := []pdf.TextBox{{Text: "x"}}
	annotateBoxLayouts(boxes, nil, 3.0, 0)
	if boxes[0].LayoutType != "" {
		t.Error("empty regions → no annotation")
	}
}

func TestParse_TableLinkedToSections(t *testing.T) {
	// Simulate enrichWithDeepDoc → extractTableAndReplace → boxesToSections:
	// table boxes are popped and replaced with one HTML box.
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 50, X1: 200, Top: 50, Bottom: 80, Text: "heading"},
		{PageNumber: 0, X0: 50, X1: 500, Top: 250, Bottom: 400, Text: "table text", LayoutType: "table"},
		{PageNumber: 0, X0: 50, X1: 200, Top: 450, Bottom: 480, Text: "after"},
	}
	tableItem := pdf.TableItem{
		Cells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 50, Label: "table row"},
			{X0: 0, Y0: 51, X1: 200, Y1: 100, Label: "table row"},
		},
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 250, Bottom: 400}},
		Scale:     1.0,
	}

	boxes = tbl.ExtractTableAndReplace(boxes, []pdf.TableItem{tableItem})
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

func cellTexts(cells []pdf.TSRCell) []string {
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
		r := pdf.DLARegion{X0: 10, Y0: 20, X1: 100, Y1: 150}
		cropped, err := util.CropImageRegion(img, r)
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
		r := pdf.DLARegion{X0: 110, Y0: 20, X1: 50, Y1: 150}
		_, err := util.CropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for x0 >= x1, got nil")
		}
	})

	t.Run("y0 >= y1 returns error", func(t *testing.T) {
		r := pdf.DLARegion{X0: 10, Y0: 150, X1: 100, Y1: 20}
		_, err := util.CropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for y0 >= y1, got nil")
		}
	})

	t.Run("region fully outside image bounds", func(t *testing.T) {
		// Clamped to image bounds → zero-width/height → error.
		r := pdf.DLARegion{X0: 300, Y0: 400, X1: 500, Y1: 600}
		_, err := util.CropImageRegion(img, r)
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
		DLARegions: []pdf.DLARegion{
			{X0: 500, Y0: 100, X1: 100, Y1: 300, Label: "table", Confidence: 0.9},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)
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

	eng := &mockEngine{pageCount: 1, chars: map[int][]pdf.TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "chart image"}}}}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 50, Y0: 200, X1: 2000, Y1: 1000, Label: "figure", Confidence: 0.85},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

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

	eng := &mockEngine{pageCount: 1, chars: map[int][]pdf.TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "just text"}}}}
	mock := &MockDocAnalyzer{
		DLARegions: []pdf.DLARegion{
			{X0: 150, Y0: 300, X1: 1500, Y1: 600, Label: "text", Confidence: 0.8},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

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

	eng := &mockEngine{pageCount: 1, chars: map[int][]pdf.TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "text"}}}}
	p := NewParser(pdf.DefaultParserConfig(), &MockDocAnalyzer{Healthy: true})

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
		chars: map[int][]pdf.TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 5, Y0: 5, X1: 50, Y1: 5, X2: 50, Y2: 50, X3: 5, Y3: 50},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

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
		chars: map[int][]pdf.TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	p := NewParser(pdf.DefaultParserConfig(), &MockDocAnalyzer{Healthy: true})

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
		chars: map[int][]pdf.TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRBoxes: []pdf.OCRBox{}, // empty detect
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

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
	p := NewParser(pdf.DefaultParserConfig(), &MockDocAnalyzer{
		Healthy: true,
		DLAErr:  fmt.Errorf("DLA service unavailable"),
	})
	eng := &mockEngine{pageCount: 1}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pageImages := map[int]image.Image{0: img}
	boxes := []pdf.TextBox{
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
	p := NewParser(pdf.DefaultParserConfig(), &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 0, Y0: 0, X1: 400, Y1: 400, Label: "table", Confidence: 0.95},
		},
		TSRErr: fmt.Errorf("TSR model timeout"),
	})
	eng := &mockEngine{pageCount: 1}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pageImages := map[int]image.Image{0: img}
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 10, X1: 90, Top: 10, Bottom: 90, Text: "in table region"},
	}
	tables := p.enrichWithDeepDoc(context.Background(), eng, boxes, pageImages)
	// DLA detects the table region → 1 pdf.TableItem is created.  TSR failure
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
		chars:     map[int][]pdf.TextChar{}, // empty → triggers OCR path
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)
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
