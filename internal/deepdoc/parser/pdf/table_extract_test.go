package pdf

import (
	"context"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

type orientationScoringDoc struct{}

func (d *orientationScoringDoc) DLA(_ context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}

func (d *orientationScoringDoc) TSR(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}

func (d *orientationScoringDoc) OCRDetect(_ context.Context, img image.Image) ([]pdf.OCRBox, error) {
	// EvaluateTableOrientation now scores each angle by per-line recognition,
	// so it requires detection output (an empty result scores 0). Emit line
	// boxes whose GEOMETRY tracks the image orientation so the warped crop
	// handed to OCRRecognize preserves aspect ratio: a landscape image yields
	// wide boxes, a portrait (rotated) image yields tall boxes. The mock's
	// recognition signal (portrait crop reads as more legible) must survive
	// the warp — fixed-size boxes would always yield a landscape strip and
	// silently disable the orientation signal.
	portrait := img.Bounds().Dy() > img.Bounds().Dx()
	w, h := 100.0, 10.0
	if portrait {
		w, h = 10.0, 100.0
	}
	return []pdf.OCRBox{{
		X0: 0, Y0: 0, X1: w, Y1: 0, X2: w, Y2: h, X3: 0, Y3: h,
	}}, nil
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

	item := p.processOneTable(t.Context(), pageImg, boxes, 0, &orientationScoringDoc{}, builder, match, pdf.DlaScale)
	if len(item.Cells) != 1 {
		t.Fatalf("cells = %d, want 1", len(item.Cells))
	}

	got := item.Cells[0]

	// Auto-rotate must return axis-aligned (non-inverted) bounds — the core
	// "normalize" invariant. Asserting this instead of absolute pixels makes
	// the test immune to TSR crop margin / crop-size changes.
	if got.X0 >= got.X1 || got.Y0 >= got.Y1 {
		t.Fatalf("cell bounds are inverted: (%.0f,%.0f,%.0f,%.0f)", got.X0, got.Y0, got.X1, got.Y1)
	}

	// Rotation is area-preserving: the processed cell keeps the input cell's
	// area (50*60 = 3000) regardless of crop size or rotation angle.
	const inW, inH = 50.0, 60.0
	gotW, gotH := got.X1-got.X0, got.Y1-got.Y0
	if math.Abs(gotW*gotH-inW*inH) > 1e-6 {
		t.Errorf("cell area = %.0f, want %.0f (rotation preserves area)", gotW*gotH, inW*inH)
	}

	// The cell must stay inside the cropped image. Crop bounds are derived
	// from the shared TSRRegionMarginPx constant, so they track margin changes.
	cropX0 := math.Max(0, match.Region.X0-util.TSRRegionMarginPx)
	cropY0 := math.Max(0, match.Region.Y0-util.TSRRegionMarginPx)
	cropX1 := math.Min(float64(pageImg.Bounds().Dx()), match.Region.X1+util.TSRRegionMarginPx)
	cropY1 := math.Min(float64(pageImg.Bounds().Dy()), match.Region.Y1+util.TSRRegionMarginPx)
	if got.X0 < cropX0-1 || got.Y0 < cropY0-1 || got.X1 > cropX1+1 || got.Y1 > cropY1+1 {
		t.Errorf("cell (%.0f,%.0f,%.0f,%.0f) outside crop (%.0f,%.0f,%.0f,%.0f)",
			got.X0, got.Y0, got.X1, got.Y1, cropX0, cropY0, cropX1, cropY1)
	}
}

// TestProcessOneTable_CropOffUsesFixedMargin locks the parity contract that
// the TSR crop offset is a fixed margin (TSRRegionMarginPx = 10pt * DlaScale =
// 30px), not a proportional percentage of the region. processOneTable computes
// cropOffX = max(0, region.X0 - TSRRegionMarginPx); for a region whose X0/Y0
// lie beyond the margin the offsets must equal region.X - 30 (nonzero), which
// is exactly the inverse of CropImageRegion's forward 30px expansion. The
// pre-fix code used w*0.03/h*0.03 here, diverging from Python.
func TestProcessOneTable_CropOffUsesFixedMargin(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)

	pageImg := image.NewRGBA(image.Rect(0, 0, 320, 220))
	boxes := []pdf.TextBox{
		{X0: 10, X1: 60, Top: 10, Bottom: 30, Text: "cell", LayoutType: pdf.LayoutTypeTable},
	}
	// Region beyond TSRRegionMarginPx (30px), with distinct X/Y origins so a
	// regression that uses the wrong origin for CropOffY is caught. Offset
	// must be region.X - 30 (nonzero), not the old proportional w*0.03 and
	// not clamped to 0.
	const regionX0, regionY0 = 100.0, 140.0
	match := tbl.TableMatch{
		Region: pdf.DLARegion{X0: regionX0, Y0: regionY0, X1: 210, Y1: 200, Label: pdf.LayoutTypeTable},
		BoxIdx: []int{0},
	}
	builder := &staticTableBuilder{
		cells: []pdf.TSRCell{
			{X0: 10, Y0: 20, X1: 60, Y1: 80, Label: "table row"},
		},
	}

	item := p.processOneTable(t.Context(), pageImg, boxes, 0, &orientationScoringDoc{}, builder, match, pdf.DlaScale)

	const wantOffX = regionX0 - util.TSRRegionMarginPx // 100 - 30 = 70
	const wantOffY = regionY0 - util.TSRRegionMarginPx // 140 - 30 = 110
	if item.CropOffX != wantOffX {
		t.Errorf("cropOffX = %v, want %v (region.X0 - fixed 30px margin)", item.CropOffX, wantOffX)
	}
	if item.CropOffY != wantOffY {
		t.Errorf("cropOffY = %v, want %v (region.Y0 - fixed 30px margin)", item.CropOffY, wantOffY)
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

			item := p.processOneTable(t.Context(), pageImg, boxes, 0, doc, builder, match, pdf.DlaScale)
			if len(item.Cells) != 1 {
				t.Fatalf("cells = %d, want 1", len(item.Cells))
			}
			if item.Cells[0].Text != "" {
				t.Errorf("empty cell filled by per-cell OCR: %q; Go must align with Python, which skips per-cell OCR", item.Cells[0].Text)
			}
		})
	}
}

// twoRowTableBuilder groups DetectCells' output into one row per cell
// (single column), so a test can control exactly which TSR row band each
// cell occupies.
type twoRowTableBuilder struct {
	cells []pdf.TSRCell
}

func (b *twoRowTableBuilder) Name() string { return "tworow" }

func (b *twoRowTableBuilder) DetectCells(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return append([]pdf.TSRCell(nil), b.cells...), nil
}

func (b *twoRowTableBuilder) GroupCells(cells []pdf.TSRCell) [][]pdf.TSRCell {
	rows := make([][]pdf.TSRCell, len(cells))
	for i, c := range cells {
		rows[i] = []pdf.TSRCell{c}
	}
	return rows
}

// TestProcessOneTable_CollapsesOverlappingBoxesBeforeCellFill verifies that
// two overlapping OCR boxes over the same table region (one text box and a
// nested duplicate detection covering part of the same text, e.g. "Alpha
// Beta" and "Beta") are collapsed before cell assignment, matching Python's
// pipeline order (_naive_vertical_merge runs before construct_table).
// Without the collapse, the two boxes independently pick their own
// best-matching row, spreading the duplicated word across two different
// cells instead of landing once in a single row.
func TestProcessOneTable_CollapsesOverlappingBoxesBeforeCellFill(t *testing.T) {
	cfg := pdf.DefaultParserConfig()
	p := NewParser(cfg)

	pageImg := image.NewRGBA(image.Rect(0, 0, 600, 600))
	// PDF-point space. Box2 ("Beta") is a nested duplicate OCR detection
	// overlapping the tail of box1 ("Alpha Beta"): by itself it best-matches
	// the second TSR row, while box1 alone best-matches the first.
	boxes := []pdf.TextBox{
		{X0: 0, X1: 50, Top: 5, Bottom: 25, Text: "Alpha Beta"},
		{X0: 20, X1: 50, Top: 18.33, Bottom: 28.33, Text: "Beta"},
	}
	match := tbl.TableMatch{
		Region: pdf.DLARegion{X0: 0, Y0: 0, X1: 600, Y1: 600, Label: pdf.LayoutTypeTable},
		BoxIdx: []int{0, 1},
	}
	// TSR cells in crop-pixel space (scale = DlaScale = 3): row0 y=[0,60],
	// row1 y=[60,120], single column x=[0,150].
	builder := &twoRowTableBuilder{
		cells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 150, Y1: 60, Label: "table row"},
			{X0: 0, Y0: 60, X1: 150, Y1: 120, Label: "table row"},
		},
	}

	item := p.processOneTable(t.Context(), pageImg, boxes, 0, &orientationScoringDoc{}, builder, match, pdf.DlaScale)
	// Python's construct_table groups boxes by their R/C labels, producing a
	// row only for an R that carries a box. Both boxes land in the SAME row
	// (they overlap vertically), so the grid is 1x1 — not 2x1.
	if len(item.Grid) != 1 || len(item.Grid[0]) != 1 {
		t.Fatalf("grid shape = %v, want 1x1 (R/C grouping emits a row only for the R that carries the box)", item.Grid)
	}

	row0 := item.Grid[0][0].Text
	if row0 == "" {
		t.Fatalf("row0 = %q, want non-empty: overlapping boxes must merge into a single cell assignment instead of spreading duplicated text across rows", row0)
	}
}

// TestDedupNestedBoxes_IdenticalBBoxKeepsContent locks that two boxes with
// IDENTICAL bbox AND identical text are not BOTH dropped. The containment
// check drops the inner box when its bbox lies fully inside the outer's and
// the outer text contains the inner's — but for identical bboxes each box is
// "inside" the other, so both get marked for removal and the cell content
// vanishes. Only STRICT containment may drop; identical-bbox boxes must keep
// at least one copy.
func TestDedupNestedBoxes_IdenticalBBoxKeepsContent(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 100, X1: 300, Top: 100, Bottom: 120, Text: "Revenue", IsOCR: true},
		{X0: 100, X1: 300, Top: 100, Bottom: 120, Text: "Revenue", IsOCR: true},
	}
	got := dedupNestedBoxes(boxes)
	if len(got) == 0 {
		t.Fatal("identical bbox+text boxes were BOTH dropped — content loss; at least one copy must survive")
	}
	for _, b := range got {
		if strings.TrimSpace(b.Text) != "Revenue" {
			t.Errorf("surviving box text = %q, want %q", b.Text, "Revenue")
		}
	}
}
