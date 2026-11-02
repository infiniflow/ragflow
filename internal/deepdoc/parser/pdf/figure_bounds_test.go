package pdf

import (
	"image"
	"testing"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

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
