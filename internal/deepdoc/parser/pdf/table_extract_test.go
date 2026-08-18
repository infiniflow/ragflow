package pdf

import (
	"context"
	"fmt"
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

func (d *orientationScoringDoc) OCRDetect(_ context.Context, img image.Image) ([]pdf.OCRBox, error) {
	regions := 1
	if img.Bounds().Dy() > img.Bounds().Dx() {
		regions = 5
	}
	boxes := make([]pdf.OCRBox, regions)
	for i := range boxes {
		x0 := float64((i + 1) * 10)
		boxes[i] = pdf.OCRBox{
			X0: x0, Y0: 10,
			X1: x0 + 5, Y1: 10,
			X2: x0 + 5, Y2: 30,
			X3: x0, Y3: 30,
		}
	}
	return boxes, nil
}

func (d *orientationScoringDoc) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	return nil, nil
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

// ocrFillingDoc is like orientationScoringDoc but its OCRRecognize returns
// text for any cropped image. It exists so a test can prove Go does NOT
// perform per-cell OCR on empty TSR cells: even though the OCR engine would
// happily fill any cropped cell, the cell must stay empty. This guards the
// alignment target (Python only fills cells from page-level OCR boxes matched
// via construct_table; it never crops individual cells for recognition).
type ocrFillingDoc struct {
	orientationScoringDoc
}

func (d *ocrFillingDoc) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	return []pdf.OCRText{{Text: "OCR-FILL", Confidence: 0.9}}, nil
}

// TestProcessOneTable_NoPerCellOCR is a regression guard for the removal of
// ocrTableCells (per-cell OCR). An empty TSR cell with no overlapping
// page-level OCR box must remain empty regardless of table auto-rotation:
// the former rotated path (bestAngle != 0) and the non-rotated path
// (bestAngle == 0) both used to fill such cells via per-cell OCR.
func TestProcessOneTable_NoPerCellOCR(t *testing.T) {
	doc := &ocrFillingDoc{}
	for _, autoRotate := range []bool{false, true} {
		t.Run(fmt.Sprintf("autoRotate=%v", autoRotate), func(t *testing.T) {
			cfg := pdf.DefaultParserConfig()
			cfg.AutoRotateTables = &autoRotate
			cfg.SkipOCR = false
			p := NewParser(cfg)

			pageImg := image.NewRGBA(image.Rect(0, 0, 320, 220))
			// No page-level OCR box overlaps the cell, so FillCellTextFromBoxes
			// leaves it empty; per-cell OCR must not fill it either.
			boxes := []pdf.TextBox{}
			match := tbl.TableMatch{
				Region: pdf.DLARegion{X0: 10, Y0: 10, X1: 210, Y1: 110, Label: pdf.LayoutTypeTable},
				BoxIdx: []int{},
			}
			builder := &staticTableBuilder{
				cells: []pdf.TSRCell{
					{X0: 10, Y0: 20, X1: 60, Y1: 80, Label: "table row", Text: ""},
				},
			}

			item := p.processOneTable(context.Background(), pageImg, boxes, 0, doc, builder, match, pdf.DlaScale)
			if len(item.Cells) != 1 {
				t.Fatalf("cells = %d, want 1", len(item.Cells))
			}
			if item.Cells[0].Text != "" {
				t.Errorf("empty cell filled by per-cell OCR: %q; Go must align with Python, which skips per-cell OCR", item.Cells[0].Text)
			}
		})
	}
}
