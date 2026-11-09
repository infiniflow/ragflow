package table

import (
	"context"
	"image"
	"math"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// DeepDocTableBuilder implements pdf.TableBuilder for the DeepDoc
// table structure recognition service. Label injection is handled by the
// NewTableBuilderFor factory.
type DeepDocTableBuilder struct {
	doc pdf.DocAnalyzer
}

// NewDeepDocTableBuilder creates a TableBuilder. Labels must be set on the
// underlying client by the caller (see deepdoc.go NewTableBuilderFor).
func NewDeepDocTableBuilder(doc pdf.DocAnalyzer) *DeepDocTableBuilder {
	return &DeepDocTableBuilder{doc: doc}
}
func (b *DeepDocTableBuilder) Name() string { return "deepdoc" }
func (b *DeepDocTableBuilder) DetectCells(ctx context.Context, cropped image.Image) ([]pdf.TSRCell, error) {
	return b.doc.TSR(ctx, cropped)
}

// GroupCells builds a row×column grid from structural cells.
//
// Input: structural cells with labels "table row", "table column",
// "table column header", "table spanning cell".
//
// Algorithm:
//  1. Extract row boundaries from "table row" cells, sort by Y.
//  2. Extract column boundaries from "table column" cells, sort by X.
//  3. Cross-product: grid[r][c].X0/Y0/X1/Y1 = col[c] × row[r].
//  4. Header propagation: rows substantially covered by the header cell get
//     Label = "table column header".
//  5. Span injection: for each "table spanning cell", find the first grid cell
//     whose center falls inside the span bbox. That cell gets the span label
//     and the span's full bbox; the covered cells keep their own geometry.
func (b *DeepDocTableBuilder) GroupCells(cells []pdf.TSRCell) [][]pdf.TSRCell {
	if len(cells) == 0 {
		return nil
	}

	// 1. Collect and sort structural elements.
	var cols, spans []pdf.TSRCell
	var header *pdf.TSRCell

	for _, c := range cells {
		switch {
		case strings.HasSuffix(c.Label, "table column"):
			cols = append(cols, c)
		case strings.Contains(strings.ToLower(c.Label), "spanning"):
			spans = append(spans, c)
		case strings.HasSuffix(c.Label, "table column header"):
			h := c
			header = &h
		}
	}

	rows := GridRowBands(cells)
	if len(rows) == 0 {
		return nil
	}

	SortXFirstly(cols, 10)

	// De-duplicate overlapping column lines, matching Python's layouts_cleanup.
	cols = layoutCleanup(cols, nil, 5, 0.5)

	// 2. If no column cells, synthesize one wide column from row extents.
	if len(cols) == 0 {
		x0 := rows[0].X0
		x1 := rows[0].X1
		cols = []pdf.TSRCell{{X0: x0, Y0: rows[0].Y0, X1: x1, Y1: rows[len(rows)-1].Y1, Label: "table column"}}
	}

	// 3. Cross-product to build grid.
	grid := make([][]pdf.TSRCell, len(rows))
	for r := range rows {
		grid[r] = make([]pdf.TSRCell, len(cols))
		for c := range cols {
			grid[r][c] = pdf.TSRCell{
				X0: cols[c].X0,
				Y0: rows[r].Y0,
				X1: cols[c].X1,
				Y1: rows[r].Y1,
			}
		}
	}

	// 4. Header propagation.
	if header != nil {
		for ri := range rows {
			// Require substantial coverage: a header component that merely
			// touches the next row must not mark it as a header.
			if rowOverlapRatio(rows[ri], *header) >= 0.5 {
				for cj := range grid[ri] {
					grid[ri][cj].Label = "table column header"
				}
			}
		}
	}

	// 5. Span injection.
	//
	// Find the top-left grid cell whose centre lies inside the span bbox and
	// extend it to the span's full extent; CalSpans later derives the covered
	// rectangle from it. Covered cells keep their own bbox so their text can
	// still be folded into the span origin at render time.
	for _, sp := range spans {
		firstR, firstC := -1, -1
		for ri := range grid {
			// Use raw structure lines for the centre: an earlier span may have
			// widened a grid cell and shifted its centre outside this span.
			cy := (rows[ri].Y0 + rows[ri].Y1) / 2
			if cy < sp.Y0 || cy > sp.Y1 {
				continue
			}
			for cj := range grid[ri] {
				cx := (cols[cj].X0 + cols[cj].X1) / 2
				if cx >= sp.X0 && cx <= sp.X1 {
					if firstR == -1 || ri < firstR || (ri == firstR && cj < firstC) {
						firstR, firstC = ri, cj
					}
				}
			}
		}
		// Keep 1-cell spans: they render harmlessly and preserve TSR
		// recognition. Skip only when no grid cell centre falls inside.
		if firstR == -1 {
			continue
		}
		grid[firstR][firstC].X0 = sp.X0
		grid[firstR][firstC].Y0 = sp.Y0
		grid[firstR][firstC].X1 = sp.X1
		grid[firstR][firstC].Y1 = sp.Y1
		grid[firstR][firstC].Label = sp.Label
	}

	return grid
}

// GridRowBands returns the TSR "table row" lines used to build the grid.
// The header component is handled separately by GroupCells.
func GridRowBands(cells []pdf.TSRCell) []pdf.TSRCell {
	return rowBands(cells, func(label string) bool {
		return strings.HasSuffix(strings.TrimSpace(strings.ToLower(label)), "table row")
	})
}

// AnnotationRowBands returns the TSR row and header lines used for R
// annotation, matching Python's gather(r".* (row|header)").
func AnnotationRowBands(cells []pdf.TSRCell) []pdf.TSRCell {
	return rowBands(cells, func(label string) bool {
		label = strings.TrimSpace(strings.ToLower(label))
		return strings.HasSuffix(label, " row") || strings.HasSuffix(label, " header")
	})
}

func rowBands(cells []pdf.TSRCell, match func(string) bool) []pdf.TSRCell {
	var rows []pdf.TSRCell
	for _, c := range cells {
		if match(c.Label) {
			rows = append(rows, c)
		}
	}
	SortYFirstly(rows, 10)
	return layoutCleanup(rows, nil, 5, 0.6)
}

// rowOverlapRatio reports the fraction of row's height covered by header.
func rowOverlapRatio(row, header pdf.TSRCell) float64 {
	h := row.Y1 - row.Y0
	if h <= 0 {
		return 0
	}
	top := math.Max(row.Y0, header.Y0)
	btm := math.Min(row.Y1, header.Y1)
	if btm <= top {
		return 0
	}
	return (btm - top) / h
}

// FinalizeGrid returns annotGrid unchanged. The DeepDoc builder folds OCR
// text into the grid inline during processOneTable's box-RC grouping, so
// there is no deferred work to perform here. An EE V/H builder that defers
// the OCR overlay overrides this with the real overlay.
func (b *DeepDocTableBuilder) FinalizeGrid(annotGrid [][]pdf.TSRCell, boxes []pdf.TextBox) [][]pdf.TSRCell {
	return annotGrid
}

// AttachSpanInfo is a no-op for the DeepDoc builder: its TableItem carries
// no builder-specific span geometry. An EE V/H builder overrides this to
// attach V/H span info.
func (b *DeepDocTableBuilder) AttachSpanInfo(item *pdf.TableItem) {}
