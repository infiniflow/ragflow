package parser

import (
	"context"
	"image"
	"sort"
	"strings"
)

// OSS model label taxonomies.
// DLA: 8 unique classes (no duplicates — OSS ONNX model output).
var ossDLALabels = []string{
	LayoutTypeTitle, LayoutTypeText, LayoutTypeReference,
	LayoutTypeFigure, DLALabelFigureCaption,
	LayoutTypeTable, DLALabelTableCaption, LayoutTypeEquation,
}

// TSR: 6 structural elements (matches deepdoc/vision/table_structure_recognizer.py).
var ossTSRLabels = []string{
	"table", "table column", "table row",
	"table column header", "table projected row header",
	"table spanning cell",
}

// OssDeepDocService implements TableBuilder and DocAnalyzer for the oss
// DeepDoc service (ONNX models via HTTP).
type OssDeepDocService struct {
	doc DocAnalyzer
}

// NewOssDeepDocService creates a service backed by the oss DeepDoc service.
// If doc is a *DeepDocClient, its DLALabels/TSRLabels are set to the OSS
// taxonomy.
func NewOssDeepDocService(doc DocAnalyzer) *OssDeepDocService {
	if c, ok := doc.(*DeepDocClient); ok {
		c.DLALabels = ossDLALabels
		c.TSRLabels = ossTSRLabels
	}
	return &OssDeepDocService{doc: doc}
}

func (b *OssDeepDocService) Name() string { return "oss-deepdoc" }

func (b *OssDeepDocService) DetectCells(ctx context.Context, cropped image.Image) ([]TSRCell, error) {
	return b.doc.TSR(ctx, cropped)
}

// GroupCells builds a row×column grid from OSS structural cells.
//
// Input: structural cells with labels "table row", "table column",
// "table column header", "table spanning cell".
//
// Algorithm:
//  1. Extract row boundaries from "table row" cells, sort by Y.
//  2. Extract column boundaries from "table column" cells, sort by X.
//  3. Cross-product: grid[r][c].X0/Y0/X1/Y1 = col[c] × row[r].
//  4. Header propagation: rows overlapping the header cell's Y range
//     get Label = "table column header".
//  5. Span injection: for each "table spanning cell", find grid cells
//     whose center falls inside the span bbox.  The top-left cell gets
//     the span label + extended bbox; remaining cells are zeroed (covered).
func (b *OssDeepDocService) GroupCells(cells []TSRCell) [][]TSRCell {
	if len(cells) == 0 {
		return nil
	}

	// 1. Collect and sort structural elements.
	var rows, cols, spans []TSRCell
	var header *TSRCell

	for _, c := range cells {
		switch {
		case strings.HasSuffix(c.Label, "table row"):
			rows = append(rows, c)
		case strings.HasSuffix(c.Label, "table column"):
			cols = append(cols, c)
		case strings.Contains(strings.ToLower(c.Label), "spanning"):
			spans = append(spans, c)
		case strings.HasSuffix(c.Label, "table column header"):
			h := c
			header = &h
		}
	}

	if len(rows) == 0 {
		return nil
	}

	sortYFirstly(rows, 10)
	sortXFirstly(cols, 10)

	// 2. If no column cells, synthesize one wide column from row extents.
	if len(cols) == 0 {
		x0 := rows[0].X0
		x1 := rows[0].X1
		cols = []TSRCell{{X0: x0, Y0: rows[0].Y0, X1: x1, Y1: rows[len(rows)-1].Y1, Label: "table column"}}
	}

	// 3. Cross-product to build grid.
	grid := make([][]TSRCell, len(rows))
	for r := range rows {
		grid[r] = make([]TSRCell, len(cols))
		for c := range cols {
			grid[r][c] = TSRCell{
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
			if rows[ri].Y0 >= header.Y0 && rows[ri].Y1 <= header.Y1 ||
				overlapsY(rows[ri], *header) {
				for cj := range grid[ri] {
					grid[ri][cj].Label = "table column header"
				}
			}
		}
	}

	// 5. Span injection.
	for _, sp := range spans {
		// Find grid cells whose center falls inside the span bbox.
		type cellIdx struct{ r, c int }
		var covered []cellIdx
		for ri := range grid {
			for cj := range grid[ri] {
				cell := grid[ri][cj]
				cx := (cell.X0 + cell.X1) / 2
				cy := (cell.Y0 + cell.Y1) / 2
				if cx >= sp.X0 && cx <= sp.X1 && cy >= sp.Y0 && cy <= sp.Y1 {
					covered = append(covered, cellIdx{ri, cj})
				}
			}
		}
		if len(covered) < 2 {
			continue
		}
		// Sort covered cells: top-left first.
		sort.Slice(covered, func(a, b int) bool {
			if covered[a].r != covered[b].r {
				return covered[a].r < covered[b].r
			}
			return covered[a].c < covered[b].c
		})
		// First cell: extend bbox to span bounds, set label.
		first := covered[0]
		grid[first.r][first.c].X0 = sp.X0
		grid[first.r][first.c].Y0 = sp.Y0
		grid[first.r][first.c].X1 = sp.X1
		grid[first.r][first.c].Y1 = sp.Y1
		grid[first.r][first.c].Label = sp.Label
		// Remaining cells: zeroed (covered).
		for _, idx := range covered[1:] {
			grid[idx.r][idx.c] = TSRCell{}
		}
	}

	return grid
}

// overlapsY reports whether two cells overlap in the Y dimension.
func overlapsY(a, b TSRCell) bool {
	return a.Y0 < b.Y1 && a.Y1 > b.Y0
}
