package pdf

import (
	"image"
	"math"
	"strings"
	"testing"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestProcessPageBoxesPreservesNativeTextOutsideDetection(t *testing.T) {
	chars := []pdf.TextChar{
		{Text: "SCL", X0: 20, X1: 38, Top: 30, Bottom: 38},
		{Text: "SDA", X0: 20, X1: 38, Top: 70, Bottom: 78},
	}
	analyzer := &MockDocAnalyzer{Healthy: true, OCRBoxes: []pdf.OCRBox{
		{X0: 60, Y0: 210, X1: 114, Y1: 210, X2: 114, Y2: 234, X3: 60, Y3: 234},
	}}
	boxes, _, usedOCR := NewParser(pdf.DefaultParserConfig()).processPageBoxes(
		t.Context(), image.NewRGBA(image.Rect(0, 0, 900, 900)), chars, 0, nil, false, analyzer, 3)
	var text strings.Builder
	for _, box := range boxes {
		text.WriteString(box.Text)
	}
	if got := text.String(); got != "SCLSDA" {
		t.Fatalf("native text = %q, want SCLSDA even when DET only finds SDA", got)
	}
	if usedOCR {
		t.Fatal("reliable native characters should not be produced by OCR")
	}
}

func TestEnrichFigureBoundsUseRenderScale(t *testing.T) {
	analyzer := &MockDocAnalyzer{Healthy: true, DLARegions: []pdf.DLARegion{
		{X0: 90, Y0: 180, X1: 810, Y1: 540, Label: "figure", Confidence: 0.95},
	}}
	boxes := []pdf.TextBox{{Text: "SDA", X0: 5, X1: 25, Top: 30, Bottom: 35}}
	p := NewParser(pdf.DefaultParserConfig())
	annotated, _, _ := p.enrichOnePageWithDeepDoc(t.Context(), image.NewRGBA(image.Rect(0, 0, 900, 900)), boxes, 4, nil, analyzer, NewTableBuilderFor(analyzer), 9)
	if len(annotated) != 2 || annotated[0].X0 != 5 || annotated[0].X1 != 25 {
		t.Fatalf("annotation changed native text geometry: %+v", annotated)
	}
	figures := tbl.ConsolidateFigures(annotated)
	if len(figures) != 1 || figures[0].PageNumber != 4 || figures[0].X0 != 10 || figures[0].X1 != 90 || figures[0].Top != 20 || figures[0].Bottom != 60 {
		t.Fatalf("figure did not retain retry-render scale or page: %+v", figures)
	}
}

func TestParsePreservesFigureWithoutText(t *testing.T) {
	engine := &MockEngine{NumPages: 4, RenderW: 900, RenderH: 900}
	analyzer := &MockDocAnalyzer{Healthy: true, DLARegions: []pdf.DLARegion{
		{X0: 90, Y0: 180, X1: 810, Y1: 540, Label: "figure", Confidence: 0.95},
	}}
	cfg := pdf.DefaultParserConfig()
	cfg.Pages = [][]int{{4, 4}}
	result, err := NewParser(cfg).ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	figures := result.Figures()
	if len(figures) != 1 || figures[0].Text != "" {
		t.Fatalf("textless figure lost or fabricated text: %+v", result.Sections)
	}
	pos := figures[0].Positions[0]
	if len(pos.PageNumbers) != 1 || pos.PageNumbers[0] != 3 || pos.Left != 30 || pos.Right != 270 || pos.Top != 60 || pos.Bottom != 180 {
		t.Fatalf("textless figure geometry lost: %+v", pos)
	}
}

func TestProcessPageBoxesRejectsInvalidNativeCharacters(t *testing.T) {
	chars := []pdf.TextChar{
		{Text: "Native", X0: 10, X1: 50, Top: 10, Bottom: 20},
		{Text: "bad", X0: math.NaN(), X1: 60, Top: 30, Bottom: 40},
		{Text: "zero", X0: 60, X1: 60, Top: 30, Bottom: 40},
		{Text: "\ue000", X0: 70, X1: 80, Top: 30, Bottom: 40},
	}
	boxes, _, _ := NewParser(pdf.DefaultParserConfig()).processPageBoxes(t.Context(), image.NewRGBA(image.Rect(0, 0, 300, 300)), chars, 0, nil, false, nil, 3)
	if len(boxes) != 1 || boxes[0].Text != "Native" {
		t.Fatalf("invalid native glyphs leaked into output: %+v", boxes)
	}
}

func TestProcessPageBoxesRecognizesUnusableNativeLayer(t *testing.T) {
	chars := []pdf.TextChar{{Text: "\ue000", X0: 10, X1: 20, Top: 10, Bottom: 20}}
	analyzer := &MockDocAnalyzer{Healthy: true, OCRBoxes: []pdf.OCRBox{
		{X0: 30, Y0: 30, X1: 180, Y1: 30, X2: 180, Y2: 60, X3: 30, Y3: 60},
	}, OCRTexts: []pdf.OCRText{{Text: "Readable text", Confidence: 0.95}}}
	boxes, _, usedOCR := NewParser(pdf.DefaultParserConfig()).processPageBoxes(t.Context(), image.NewRGBA(image.Rect(0, 0, 300, 300)), chars, 0, nil, false, analyzer, 3)
	if !usedOCR || len(boxes) != 1 || boxes[0].Text != "Readable text" {
		t.Fatalf("unusable text layer did not use OCR: %+v", boxes)
	}
}

func TestParseFigureUsesFullLayoutBounds(t *testing.T) {
	for _, label := range []string{"", "SDA", "数据来源：测试"} {
		name := "caption_only"
		if label != "" {
			name = label
		}
		t.Run(name, func(t *testing.T) {
			chars := []pdf.TextChar{{Text: "图 3. 时序图", X0: 150, X1: 230, Top: 210, Bottom: 220, PageNumber: 3}}
			if label != "" {
				chars = append(chars, pdf.TextChar{Text: label, X0: 110, X1: 125, Top: 180, Bottom: 188, PageNumber: 3})
			}
			engine := &MockEngine{NumPages: 4, RenderW: 1200, RenderH: 1200, Chars: map[int][]pdf.TextChar{3: chars}}
			analyzer := &MockDocAnalyzer{Healthy: true, DLARegions: []pdf.DLARegion{
				{X0: 300, Y0: 300, X1: 900, Y1: 600, Label: "figure", Confidence: 0.96},
				{X0: 450, Y0: 630, X1: 690, Y1: 660, Label: "figure caption", Confidence: 0.9},
			}}
			cfg := pdf.DefaultParserConfig()
			cfg.Pages = [][]int{{4, 4}}
			result, err := NewParser(cfg).ParseRaw(t.Context(), engine, analyzer)
			if err != nil {
				t.Fatal(err)
			}
			figures := result.Figures()
			if len(figures) != 1 {
				t.Fatalf("want one complete figure, got %+v", result.Sections)
			}
			figure := figures[0]
			if !strings.Contains(figure.Text, "图 3. 时序图") {
				t.Fatalf("caption missing: %+v", figure)
			}
			pos := figure.Positions[0]
			if len(pos.PageNumbers) != 1 || pos.PageNumbers[0] != 3 || pos.Left != 100 || pos.Right != 300 || pos.Top != 100 || pos.Bottom != 200 {
				t.Fatalf("figure position = %+v, want page 3 and full region [100,300,100,200]", pos)
			}
		})
	}
}

func TestParseMixedPageRecognizesRegionWithoutNativeText(t *testing.T) {
	testMixedPageRegion(t, 20, 30)
}

func TestParseMixedPageRecognizesRegionTouchingNativeText(t *testing.T) {
	// The native line overlaps the image's left edge by only half a point.
	// It is not the image's text layer and must not suppress region OCR.
	testMixedPageRegion(t, 100, 110)
}

func testMixedPageRegion(t *testing.T, top, bottom float64) {
	t.Helper()
	engine := &MockEngine{RenderW: 1200, RenderH: 1200, Chars: map[int][]pdf.TextChar{0: {
		{Text: "Native text", X0: 10, X1: 100.5, Top: top, Bottom: bottom},
	}}}
	analyzer := &MockDocAnalyzer{Healthy: true, DLARegions: []pdf.DLARegion{
		{X0: 15, Y0: top*3 - 15, X1: 305, Y1: bottom*3 + 15, Label: "text", Confidence: 0.9},
		{X0: 300, Y0: 300, X1: 600, Y1: 450, Label: "figure", Confidence: 0.9},
	}, OCRBoxes: []pdf.OCRBox{
		{X0: 15, Y0: 15, X1: 150, Y1: 15, X2: 150, Y2: 45, X3: 15, Y3: 45},
	}, OCRTexts: []pdf.OCRText{{Text: "Bitmap label", Confidence: 0.95}}}
	result, err := NewParser(pdf.DefaultParserConfig()).ParseRaw(t.Context(), engine, analyzer)
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, section := range result.Sections {
		text.WriteString(section.Text)
	}
	if !strings.Contains(text.String(), "Native text") || !strings.Contains(text.String(), "Bitmap label") {
		t.Fatalf("mixed page must retain native and OCR content: %q", text.String())
	}
	if strings.Count(text.String(), "Native text") != 1 {
		t.Fatal("native text duplicated")
	}
	figures := result.Figures()
	if len(figures) != 1 || figures[0].Positions[0].Left != 100 || figures[0].Positions[0].Top != 100 {
		t.Fatalf("bitmap figure lost its page-space position: %+v", figures)
	}
}
