package table

import (
	"context"
	"image"
	"sort"
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
//  4. Header propagation: rows overlapping the header cell's Y range
//     get Label = "table column header".
//  5. Span injection: for each "table spanning cell", find grid cells
//     whose center falls inside the span bbox.  The top-left cell gets
//     the span label + extended bbox; remaining cells are zeroed (covered).
func (b *DeepDocTableBuilder) GroupCells(cells []pdf.TSRCell) [][]pdf.TSRCell {
	if len(cells) == 0 {
		return nil
	}

	// 1. Collect and sort structural elements.
	var rows, cols, spans []pdf.TSRCell
	var header *pdf.TSRCell

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

	SortYFirstly(rows, 10)
	SortXFirstly(cols, 10)

	// Python's _table_transformer_job de-duplicates the structure lines before
	// grouping: rows via gather (layouts_cleanup far=5 thr=0.6), columns via
	// layouts_cleanup far=5 thr=0.5. Overlapping duplicate row/column lines are
	// collapsed (higher detection score wins), so the cross-product grid rows
	// match the rows Python's per-char R/C view counts.
	rows = layoutCleanup(rows, nil, 5, 0.6)
	cols = layoutCleanup(cols, nil, 5, 0.5)
	if len(rows) == 0 {
		return nil
	}

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
		// A span box that covers only a single grid cell (e.g. a TSR
		// spanning cell whose box only reaches one row's center because the
		// model's box edge missed the neighbour row) is still a real TSR
		// span and must be labelled, not silently dropped. A 1-cell span
		// renders as rowspan=1/colspan=1 (harmless) but keeps the TSR
		// recognition visible downstream. Only skip when the box overlaps
		// no grid cell at all (genuine coordinate mismatch).
		if len(covered) == 0 {
			continue
		}
		sort.Slice(covered, func(a, b int) bool {
			if covered[a].r != covered[b].r {
				return covered[a].r < covered[b].r
			}
			return covered[a].c < covered[b].c
		})
		first := covered[0]
		grid[first.r][first.c].X0 = sp.X0
		grid[first.r][first.c].Y0 = sp.Y0
		grid[first.r][first.c].X1 = sp.X1
		grid[first.r][first.c].Y1 = sp.Y1
		grid[first.r][first.c].Label = sp.Label
		// The remaining covered cells KEEP their row×column bbox. Python's
		// construct_table assigns every box to a cell by its TSR row/column
		// label (R/C) regardless of spanning cells, then __cal_spans folds the
		// covered cells' text into the span origin cell at render time. If Go
		// zeroed these bboxes here, FillCellTextFromBoxesWithRows would skip
		// them, their text would never reach the span cell, and orphan-row/
		// column cleanup would drop the covered rows/columns — the
		// dell/prodeploy/screenshot real-PDF parity gap (11→6 rows).
	}

	return grid
}

// overlapsY reports whether two cells overlap in the Y dimension.
func overlapsY(a, b pdf.TSRCell) bool {
	return a.Y0 < b.Y1 && a.Y1 > b.Y0
}
