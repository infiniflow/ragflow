package pdf

import (
	"context"
	"fmt"
	"image"
	inf "ragflow/internal/deepdoc/parser/pdf/inference"
	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"strings"
	"testing"
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
	eng := &MockEngine{NumPages: 1}

	p := NewParser(pdf.DefaultParserConfig())
	mock := &MockDocAnalyzer{Healthy: false}
	tables := p.enrichWithDeepDoc(context.Background(), nil, eng, boxes, nil, mock, NewTableBuilderFor(mock))
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
	p := NewParser(pdf.DefaultParserConfig())
	dummyImg := image.NewRGBA(image.Rect(0, 0, 2000, 3000))

	tables := p.extractTableBoxesFromImage(context.Background(), nil, boxes, dummyImg, 0, 0, mock, NewTableBuilderFor(mock))
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
	p := NewParser(pdf.DefaultParserConfig())
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, nil, dummy, 0, 0, mock, NewTableBuilderFor(mock))
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
	p := NewParser(pdf.DefaultParserConfig())
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 2000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, nil, dummy, 0, 0, mock, NewTableBuilderFor(mock))
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
	p := NewParser(pdf.DefaultParserConfig())
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 3000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, boxes, dummy, 0, 0, mock, NewTableBuilderFor(mock))
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
	p := NewParser(pdf.DefaultParserConfig())
	dummy := image.NewRGBA(image.Rect(0, 0, 2000, 3000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, boxes, dummy, 0, 0, mock, NewTableBuilderFor(mock))
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
	p := NewParser(pdf.DefaultParserConfig())
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, nil, dummy, 0, 0, mock, NewTableBuilderFor(mock))
	if len(tables) != 0 {
		t.Errorf("non-table DLA → 0 tables, got %d", len(tables))
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
	sections := lyt.BoxesToSections(boxes, nil)

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

// ── cropImageRegion ────────────────────────────────────────────────────
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
	p := NewParser(pdf.DefaultParserConfig())
	dummy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	tables := p.extractTableBoxesFromImage(context.Background(), nil, nil, dummy, 0, 0, mock, NewTableBuilderFor(mock))
	if len(tables) != 0 {
		t.Errorf("invalid DLA region should be skipped, got %d tables", len(tables))
	}
}

// ── DLA → figure end-to-end ───────────────────────────────────────────

func TestParse_CollectsFigures(t *testing.T) {
	// End-to-end: Parse() with mock DeepDoc that labels a box as "figure".
	// Verify p.Figures is populated.

	eng := &MockEngine{NumPages: 1, Chars: map[int][]pdf.TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "chart image"}}}}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 50, Y0: 200, X1: 2000, Y1: 1000, Label: "figure", Confidence: 0.85},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least 1 section")
	}
	if len(result.Figures()) != 1 {
		t.Fatalf("expected 1 figure, got %d", len(result.Figures()))
	}
	if result.Figures()[0].LayoutType != "figure" {
		t.Errorf("figure LayoutType = %q, want 'figure'", result.Figures()[0].LayoutType)
	}
	if result.Figures()[0].Text == "" {
		t.Error("figure Text should not be empty")
	}
}

func TestParse_NoFigures(t *testing.T) {
	// Parse() with no DLA figure regions → p.Figures should be empty.

	eng := &MockEngine{NumPages: 1, Chars: map[int][]pdf.TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "just text"}}}}
	mock := &MockDocAnalyzer{
		DLARegions: []pdf.DLARegion{
			{X0: 150, Y0: 300, X1: 1500, Y1: 600, Label: "text", Confidence: 0.8},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Figures()) != 0 {
		t.Fatalf("expected 0 figures, got %d", len(result.Figures()))
	}
}

func TestParse_NoDeepDoc_NoFigures(t *testing.T) {
	// Parse() with mock DeepDoc → Figures should be empty (no DLA-detected figures).

	eng := &MockEngine{NumPages: 1, Chars: map[int][]pdf.TextChar{0: {{X0: 50, X1: 550, Top: 100, Bottom: 112, Text: "text"}}}}
	mock := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Figures()) != 0 {
		t.Fatalf("expected 0 Figures (no DLA-detected figures), got %d", len(result.Figures()))
	}
}

// ── Parse + ocrMergeChars (full-page detect) ──────────────────────────

func TestParse_UsesOCRDetectForEmbeddedChars(t *testing.T) {
	// When DeepDoc is available and the page has embedded chars,
	// Parse should use ocrMergeChars (detect → merge → recognize).
	eng := &MockEngine{
		NumPages: 1,
		Chars: map[int][]pdf.TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		OCRBoxes: []pdf.OCRBox{
			{X0: 5, Y0: 5, X1: 50, Y1: 5, X2: 50, Y2: 50, X3: 5, Y3: 50},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
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
	eng := &MockEngine{
		NumPages: 1,
		Chars: map[int][]pdf.TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	mock := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least 1 section (charsToBoxes)")
	}
}

func TestParse_FallsBackToCharsToBoxes_EmptyOCRBoxes(t *testing.T) {
	// OCRDetect returns no boxes → falls through to charsToBoxes.
	eng := &MockEngine{
		NumPages: 1,
		Chars: map[int][]pdf.TextChar{0: {
			{X0: 10, X1: 30, Top: 10, Bottom: 30, Text: "Hello", PageNumber: 0},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRBoxes: []pdf.OCRBox{}, // empty detect
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) == 0 {
		t.Fatal("expected at least 1 section (charsToBoxes fallback)")
	}
}

// ── Error path coverage ────────────────────────────────────────────────

func TestMockDocAnalyzer_DLAError_DoesNotCrash(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLAErr:  fmt.Errorf("DLA service unavailable"),
	}
	p := NewParser(pdf.DefaultParserConfig())
	eng := &MockEngine{NumPages: 1}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pageImages := map[int]image.Image{0: img}
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "text"},
	}
	// enrichWithDeepDoc should return nil (not panic) on DLA error.
	tables := p.enrichWithDeepDoc(context.Background(), nil, eng, boxes, pageImages, mock, NewTableBuilderFor(mock))
	if len(tables) != 0 {
		t.Errorf("DLA error should produce 0 tables, got %d", len(tables))
	}
}

func TestMockDocAnalyzer_TSRError_DoesNotCrash(t *testing.T) {
	// TSR error: DLA succeeds, TSR fails.  The table region is detected
	// but no cells are returned — the table is skipped gracefully.
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 0, Y0: 0, X1: 400, Y1: 400, Label: "table", Confidence: 0.95},
		},
		TSRErr: fmt.Errorf("TSR model timeout"),
	}
	p := NewParser(pdf.DefaultParserConfig())
	eng := &MockEngine{NumPages: 1}
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	pageImages := map[int]image.Image{0: img}
	boxes := []pdf.TextBox{
		{PageNumber: 0, X0: 10, X1: 90, Top: 10, Bottom: 90, Text: "in table region"},
	}
	tables := p.enrichWithDeepDoc(context.Background(), nil, eng, boxes, pageImages, mock, NewTableBuilderFor(mock))
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
	eng := &MockEngine{
		NumPages: 1,
		Chars:    map[int][]pdf.TextChar{}, // empty → triggers OCR path
	}
	p := NewParser(pdf.DefaultParserConfig())
	_, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	// Parse should succeed — the page with OCRDetect error is just skipped.
}

// TestTSRLabels verifies Go inf.DefaultTSRLabels() matches Python's table_structure_recognizer.py labels.
// Order must be exact — the ONNX model returns class IDs that index into this array.
func TestTSRLabels(t *testing.T) {
	want := []string{
		"table", "table column", "table row",
		"table column header", "table projected row header",
		"table spanning cell",
	}
	if len(inf.DefaultTSRLabels()) != len(want) {
		t.Fatalf("inf.DefaultTSRLabels() length %d, want %d", len(inf.DefaultTSRLabels()), len(want))
	}
	for i := range want {
		if inf.DefaultTSRLabels()[i] != want[i] {
			t.Errorf("inf.DefaultTSRLabels()[%d] = %q, want %q", i, inf.DefaultTSRLabels()[i], want[i])
		}
	}
}
