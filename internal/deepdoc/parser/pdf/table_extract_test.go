package pdf

import (
	"context"
	"image"
	"testing"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

type orientationScoringDoc struct{}

func (d *orientationScoringDoc) DLA(_ context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}

func (d *orientationScoringDoc) TSR(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}

func (d *orientationScoringDoc) OCRDetect(_ context.Context, _ image.Image) ([]pdf.OCRBox, error) {
	// EvaluateTableOrientation now scores by OCRRecognize confidence, so
	// detection output is unused by this test. Return empty.
	return nil, nil
}

func (d *orientationScoringDoc) OCRRecognize(_ context.Context, img image.Image) ([]pdf.OCRText, error) {
	// Encode the orientation signal via recognition confidence: a portrait
	// (rotated) crop reads as more legible text, so it should score higher.
	// This mirrors the region-count-vs-orientation intent the mock previously
	// expressed through OCRDetect.
	regions := 1
	conf := 0.1
	if img.Bounds().Dy() > img.Bounds().Dx() {
		regions = 5
		conf = 0.9
	}
	texts := make([]pdf.OCRText, regions)
	for i := range texts {
		texts[i] = pdf.OCRText{Text: "cell", Confidence: conf}
	}
	return texts, nil
}

func (d *orientationScoringDoc) Health() bool { return true }

type staticTableBuilder struct {
	cells []pdf.TSRCell
}

func (b *staticTableBuilder) Name() string { return "static" }

func (b *staticTableBuilder) DetectCells(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return append([]pdf.TSRCell(nil), b.cells...), nil
}

func (b *staticTableBuilder) GroupCells(cells []pdf.TSRCell) [][]pdf.TSRCell {
	if len(cells) == 0 {
		return nil
	}
	return [][]pdf.TSRCell{{cells[0]}}
}

func TestProcessOneTable_AutoRotateNormalizesCellBounds(t *testing.T) {
	autoRotate := true
	cfg := pdf.DefaultParserConfig()
	cfg.AutoRotateTables = &autoRotate
	cfg.SkipOCR = true
	p := NewParser(cfg)

	pageImg := image.NewRGBA(image.Rect(0, 0, 320, 220))
	boxes := []pdf.TextBox{
		{X0: 10, X1: 60, Top: 10, Bottom: 30, Text: "cell", LayoutType: pdf.LayoutTypeTable},
	}
	match := tbl.TableMatch{
		Region: pdf.DLARegion{X0: 10, Y0: 10, X1: 210, Y1: 110, Label: pdf.LayoutTypeTable},
		BoxIdx: []int{0},
	}
	builder := &staticTableBuilder{
		cells: []pdf.TSRCell{
			{X0: 10, Y0: 20, X1: 60, Y1: 80, Label: "table row"},
		},
	}

	item := p.processOneTable(context.Background(), pageImg, boxes, 0, &orientationScoringDoc{}, builder, match, pdf.DlaScale)
	if len(item.Cells) != 1 {
		t.Fatalf("cells = %d, want 1", len(item.Cells))
	}

	got := item.Cells[0]
	if got.X0 != 20 || got.Y0 != 45 || got.X1 != 80 || got.Y1 != 95 {
		t.Errorf("cell bounds = (%.0f,%.0f,%.0f,%.0f), want (20,45,80,95)",
			got.X0, got.Y0, got.X1, got.Y1)
	}
	if got.X0 > got.X1 || got.Y0 > got.Y1 {
		t.Fatalf("cell bounds are inverted: (%.0f,%.0f,%.0f,%.0f)", got.X0, got.Y0, got.X1, got.Y1)
	}
}
