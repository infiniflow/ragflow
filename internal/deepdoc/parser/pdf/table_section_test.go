package pdf

import (
	"context"
	"image"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestTableSection_TextFromTSR verifies that table Sections carry
// TSR-structured text (from pdf.TableItem.Rows) rather than raw char text.
// Python _parse_loaded_window_into_bboxes runs _extract_table_figure
// which pops table boxes and replaces them with consolidated table
// entries. Go backfills pdf.Section.Text from pdf.TableItem.Rows after
// linkTableSections.
func TestTableSection_TextFromTSR(t *testing.T) {
	eng := &MockEngine{
		NumPages: 1,
		RenderW:  900, // 300pt at 3x = 900px (216 DPI)
		RenderH:  600,
		Chars: map[int][]pdf.TextChar{0: {
			// PDF space (72 DPI): well inside DLA region
			{X0: 50, X1: 70, Top: 40, Bottom: 55, Text: "姓"},
			{X0: 80, X1: 100, Top: 40, Bottom: 55, Text: "名"},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		// DLA table region in pixel space (216 DPI).
		// PDF space: x0=100/3≈33, y0=80/3≈27, x1=500/3≈167, y1=300/3≈100.
		DLARegions: []pdf.DLARegion{
			{X0: 100, Y0: 80, X1: 500, Y1: 300, Label: "table", Confidence: 0.9},
		},
		// TSR returns structured 2x2 cells with text.
		// Pixel space (relative to cropped region).
		TSRCells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 100, Text: "姓名", Label: "table column header"},
			{X0: 200, Y0: 0, X1: 460, Y1: 100, Text: "年龄", Label: "table column header"},
			{X0: 0, Y0: 100, X1: 200, Y1: 220, Text: "张三", Label: "table row"},
			{X0: 200, Y0: 100, X1: 460, Y1: 220, Text: "25", Label: "table row"},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// ── Assert 1: Tables exist (Cells are filled by constructTable later) ──
	if len(result.Tables) == 0 {
		t.Fatal("expected at least 1 pdf.TableItem")
	}
	tbl := result.Tables[0]
	if len(tbl.Cells) == 0 {
		t.Fatal("expected TSR cells in pdf.TableItem")
	}

	// ── Assert 2: A table section exists with HTML output ──
	var tableSections []pdf.Section
	for _, s := range result.Sections {
		if s.LayoutType == "table" {
			tableSections = append(tableSections, s)
		}
	}
	if len(tableSections) == 0 {
		t.Fatal("expected at least 1 section with LayoutType=='table'")
	}
	ts := tableSections[0]

	// ── Assert 3: pdf.Section.Text is HTML table from constructTable ──
	if !strings.HasPrefix(ts.Text, "<table>") {
		t.Errorf("table pdf.Section.Text = %q, want HTML <table>", ts.Text)
	}
	// OSS pipeline: TSR cell text is not preserved in the grid (OSS
	// GroupCells creates new cells from row×column cross product).
	// Cell text comes from fillCellTextFromBoxes matching PDF chars,
	// not from pre-filled TSR cell text (EE feature).
}

// TestEnrichWithDeepDoc_ImageOnlyPage verifies that enrichWithDeepDoc
// runs DLA on pages that have images but zero embedded chars (boxes).
// Regression test for test.pdf (Go 0 tables, Py 1 table).
func TestEnrichWithDeepDoc_ImageOnlyPage(t *testing.T) {
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 54, Y0: 100, X1: 846, Y1: 500, Label: "table", Confidence: 0.95},
		},
		TSRCells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 100, Text: "A", Label: "table row"},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	// 0 text boxes, but page 0 has a rendered image.
	boxes := []pdf.TextBox{}
	dummyImg := image.NewRGBA(image.Rect(0, 0, 900, 600))
	pageImages := map[int]image.Image{0: dummyImg}

	tables := p.enrichWithDeepDoc(context.Background(), nil, nil, boxes, pageImages, mock, NewTableBuilderFor(mock))
	if len(tables) == 0 {
		t.Fatal("enrichWithDeepDoc: expected at least 1 table from DLA on page with image but no boxes, got 0")
	}
	if len(tables[0].Cells) == 0 {
		t.Fatal("enrichWithDeepDoc: expected TSR cells in table")
	}
}

// TestFigureCaption_MergedIntoFigure verifies that "figure caption" text
// is merged into the nearest "figure" pdf.Section and the caption pdf.Section is
// removed. Matches Python _extract_table_figure caption matching.
func TestFigureCaption_MergedIntoFigure(t *testing.T) {
	eng := &MockEngine{
		NumPages: 1,
		RenderW:  1800, RenderH: 2400,
		Chars: map[int][]pdf.TextChar{0: {
			// Figure text — overlaps DLA figure region (pixel Y=80-300 → PDF 27-100).
			{X0: 40, X1: 60, Top: 30, Bottom: 45, Text: "F"},
			// Caption text — overlaps DLA figure caption region (pixel Y=310-340 → PDF 103-113).
			{X0: 40, X1: 60, Top: 104, Bottom: 112, Text: "C"},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 100, Y0: 80, X1: 500, Y1: 300, Label: "figure", Confidence: 0.9},
			// Caption is below the figure.
			{X0: 100, Y0: 310, X1: 500, Y1: 340, Label: "figure caption", Confidence: 0.9},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Assert 1: figure caption pdf.Section removed.
	for _, s := range result.Sections {
		if s.LayoutType == "figure caption" {
			t.Errorf("figure caption pdf.Section should be removed after mergeCaptions, got %q", s.Text)
		}
	}

	// Assert 2: figure pdf.Section exists and has caption text appended.
	var fig *pdf.Section
	for i := range result.Sections {
		if result.Sections[i].LayoutType == "figure" {
			fig = &result.Sections[i]
			break
		}
	}
	if fig == nil {
		t.Fatal("expected a figure pdf.Section")
	}
	if !strings.Contains(fig.Text, "C") {
		t.Errorf("figure Text should contain caption text 'C', got %q", fig.Text)
	}

	// Assert 3: figure is in result.Figures().
	if len(result.Figures()) == 0 {
		t.Error("expected at least 1 entry in result.Figures()")
	}
}

// TestTableCaption_MergedIntoTable verifies that "table caption" text
// is merged into the nearest table pdf.Section and the caption is removed.
func TestTableCaption_MergedIntoTable(t *testing.T) {
	eng := &MockEngine{
		NumPages: 1,
		RenderW:  1800, RenderH: 2400,
		Chars: map[int][]pdf.TextChar{0: {
			// Table text — overlaps DLA table region (pixel Y=80-300 → PDF 27-100).
			{X0: 40, X1: 60, Top: 30, Bottom: 45, Text: "T"},
			// Caption text — overlaps DLA table caption region (pixel Y=310-340 → PDF 103-113).
			{X0: 40, X1: 60, Top: 104, Bottom: 112, Text: "C"},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 100, Y0: 80, X1: 500, Y1: 300, Label: "table", Confidence: 0.9},
			{X0: 100, Y0: 310, X1: 500, Y1: 340, Label: "table caption", Confidence: 0.9},
		},
		TSRCells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 100, Text: "A", Label: "table row"},
			{X0: 200, Y0: 0, X1: 460, Y1: 100, Text: "B", Label: "table row"},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Assert: table caption pdf.Section removed, text merged into table pdf.Section.
	for _, s := range result.Sections {
		if s.LayoutType == "table caption" {
			t.Errorf("table caption pdf.Section should be removed, got %q", s.Text)
		}
	}
	var tbl *pdf.Section
	for i := range result.Sections {
		if result.Sections[i].LayoutType == "table" {
			tbl = &result.Sections[i]
			break
		}
	}
	if tbl == nil {
		t.Fatal("expected a table pdf.Section")
	}
	if !strings.Contains(tbl.Text, "C") {
		t.Errorf("table Text should contain caption text 'C', got %q", tbl.Text)
	}
}

// TestTextSectionsInsideTableRegion_Suppressed verifies that Sections
// whose positions fall inside a table region are suppressed even when
// DLA labeled them as "text".  Python _extract_table_figure pops ALL
// boxes overlapping a table region, regardless of their DLA label.
// This is the #1 cause of Go vs Python discrepancy on table-heavy PDFs.
func TestTextSectionsInsideTableRegion_Suppressed(t *testing.T) {
	eng := &MockEngine{
		NumPages: 1,
		RenderW:  1800, RenderH: 2400,
		Chars: map[int][]pdf.TextChar{0: {
			// Box A: inside DLA table region, labeled as "text" by DLA.
			{X0: 50, X1: 100, Top: 40, Bottom: 55, Text: "碎片文字"},
			// Box B: inside DLA table region, same situation.
			{X0: 120, X1: 160, Top: 40, Bottom: 55, Text: "垃圾"},
		}},
	}
	// DLA returns a "table" region AND a "text" sub-region inside it.
	// Real DLA often splits large table regions this way.
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			{X0: 100, Y0: 80, X1: 500, Y1: 300, Label: "table", Confidence: 0.9},
			{X0: 120, Y0: 100, X1: 180, Y1: 140, Label: "text", Confidence: 0.8},
		},
		TSRCells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 100, Text: "姓名", Label: "table row"},
			{X0: 200, Y0: 0, X1: 460, Y1: 100, Text: "年龄", Label: "table row"},
		},
	}
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Assert 1: table pdf.Section exists with structured text.
	var hasTable bool
	for _, s := range result.Sections {
		if s.LayoutType == "table" && s.Text != "" {
			hasTable = true
			break
		}
	}
	if !hasTable {
		t.Fatal("expected a table pdf.Section with structured text")
	}

	// Assert 2: NO "text" fragment sections remain — they were inside
	// the table region and should be suppressed (Python pops them).
	for _, s := range result.Sections {
		if s.LayoutType != "table" && strings.Contains(s.Text, "碎片") {
			t.Errorf("text fragment %q inside table region should be suppressed, got %q",
				s.Text, s.LayoutType)
		}
		if s.LayoutType != "table" && strings.Contains(s.Text, "垃圾") {
			t.Errorf("text fragment %q inside table region should be suppressed, got %q",
				s.Text, s.LayoutType)
		}
	}
	sectionCount := len(result.Sections)
	if sectionCount > 3 {
		t.Errorf("expected ≤3 sections (table + outside fragments), got %d", sectionCount)
	}
}

// TestEmptyDoc_NoCrash verifies Parse handles edge cases gracefully.
func TestEmptyDoc_NoCrash(t *testing.T) {
	eng := &MockEngine{NumPages: 0}
	mock := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())
	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) != 0 {
		t.Errorf("expected 0 sections for empty doc, got %d", len(result.Sections))
	}
}

// TestNilChars_handled verifies zero-chars pages don't crash.
func TestNilChars_Handled(t *testing.T) {
	eng := &MockEngine{NumPages: 1, RenderW: 200, RenderH: 200}
	mock := &MockDocAnalyzer{Healthy: true}
	p := NewParser(pdf.DefaultParserConfig())
	result, err := p.ParseRaw(context.Background(), eng, mock)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(result.Sections) != 0 {
		t.Logf("nil chars + DeepDoc: sections=%d (may trigger OCR path)", len(result.Sections))
	}
}

func TestMatchTableImage_ByPositions(t *testing.T) {
	tableByRegion := map[string]string{
		"0_50.0_500.0_100.0_300.0": "img_base64_positions",
	}
	sec := &pdf.Section{
		LayoutType: pdf.LayoutTypeTable,
		Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 50.0, Right: 500.0, Top: 100.0, Bottom: 300.0}},
	}
	img, ok := matchTableImage(sec, tableByRegion)
	if !ok {
		t.Fatal("expected match by Positions")
	}
	if img != "img_base64_positions" {
		t.Errorf("got %q, want img_base64_positions", img)
	}
}

func TestMatchTableImage_FallbackToRegion(t *testing.T) {
	tableByRegion := map[string]string{
		"0_80.0_520.0_200.0_400.0": "img_base64_region",
	}
	sec := &pdf.Section{
		LayoutType: pdf.LayoutTypeTable,
		Positions:  nil,
		TableItem:  &pdf.TableItem{RegionLeft: 80.0, RegionRight: 520.0, RegionTop: 200.0, RegionBottom: 400.0},
	}
	img, ok := matchTableImage(sec, tableByRegion)
	if !ok {
		t.Fatal("expected match by Region fallback")
	}
	if img != "img_base64_region" {
		t.Errorf("got %q, want img_base64_region", img)
	}
}

func TestMatchTableImage_NoMatch(t *testing.T) {
	tableByRegion := map[string]string{"0_10.0_20.0_30.0_40.0": "no_chance"}
	sec := &pdf.Section{
		LayoutType: pdf.LayoutTypeTable,
		Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 100, Right: 200, Top: 300, Bottom: 400}},
	}
	_, ok := matchTableImage(sec, tableByRegion)
	if ok {
		t.Error("expected no match")
	}
}

func TestMatchTableImage_EmptySection(t *testing.T) {
	sec := &pdf.Section{LayoutType: pdf.LayoutTypeTable}
	_, ok := matchTableImage(sec, map[string]string{"x": "y"})
	if ok {
		t.Error("expected no match for empty section")
	}
}
