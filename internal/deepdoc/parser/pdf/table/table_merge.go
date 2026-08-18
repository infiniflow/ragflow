package table

import (
	"math"
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// MergeTablesAcrossPages merges TableItems on consecutive pages with
// overlapping X and close Y proximity.  Matches Python's
// _extract_table_figure table merge (pdf_parser.py:1061-1080).
func MergeTablesAcrossPages(tables []pdf.TableItem, medianHeights map[int]float64) []pdf.TableItem {
	if len(tables) <= 1 {
		return tables
	}
	// Sort by position for deterministic adjacency.
	type indexed struct {
		idx int
		pg  int
		top float64
	}
	var items []indexed
	for i, tbl := range tables {
		if len(tbl.Positions) == 0 {
			continue
		}
		p := tbl.Positions[0]
		pg := 0
		if len(p.PageNumbers) > 0 {
			pg = p.PageNumbers[0]
		}
		items = append(items, indexed{i, pg, p.Top})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].pg != items[j].pg {
			return items[i].pg < items[j].pg
		}
		return items[i].top < items[j].top
	})

	merged := make([]bool, len(tables))
	var result []pdf.TableItem

	for _, it := range items {
		if merged[it.idx] {
			continue
		}
		anchor := tables[it.idx]
		merged[it.idx] = true
		var contGrids [][][]pdf.TSRCell

		// Python nomerge_lout_no: tables whose box is followed by a
		// caption/title/reference should not be merged cross-page.
		if anchor.NoMerge {
			result = append(result, anchor)
			continue
		}

		anchorPg := it.pg
		anchorBtm := anchor.Positions[0].Bottom

		// Look for consecutive-page continuations.
		for _, jt := range items {
			if merged[jt.idx] || jt.pg <= anchorPg {
				continue
			}
			// Python nomerge_lout_no: skip continuation candidates
			// tagged as no-merge.
			if tables[jt.idx].NoMerge {
				continue
			}
			if jt.pg-anchorPg > 1 {
				break // pages must be consecutive
			}
			if len(tables[jt.idx].Positions) == 0 {
				continue
			}
			bp := tables[jt.idx].Positions[0]
			bpg := 0
			if len(bp.PageNumbers) > 0 {
				bpg = bp.PageNumbers[0]
			}
			if bpg != anchorPg+1 {
				continue
			}
			// Check X overlap.
			ap := anchor.Positions[0]
			if ap.Right < bp.Left || bp.Right < ap.Left {
				continue
			}
			// Check Y proximity: page 1 table top should be close below
			// page 0 table bottom.  Python: y_dis <= mh * 23.
			mh := 10.0
			if medianHeights != nil {
				if h, ok := medianHeights[anchorPg]; ok && h > 0 {
					mh = h
				}
			}
			yDis := (bp.Top + bp.Bottom - anchorBtm - ap.Bottom) / 2
			if yDis > mh*23 {
				continue
			}
			// Merge: combine cells and positions.
			anchor.Cells = append(anchor.Cells, tables[jt.idx].Cells...)
			anchor.Positions = append(anchor.Positions, tables[jt.idx].Positions...)
			contGrids = append(contGrids, tables[jt.idx].Grid)
			if tables[jt.idx].Caption != "" {
				if anchor.Caption != "" {
					anchor.Caption += " "
				}
				anchor.Caption += tables[jt.idx].Caption
			}
			merged[jt.idx] = true
			anchorPg = bpg
			anchorBtm = bp.Bottom
			ap = anchor.Positions[len(anchor.Positions)-1]
		}
		// Rebuild the merged Grid from the per-page grids so ConstructTable
		// emits rows from every merged page, not just the stale anchor
		// (page-0) grid. Only when the anchor already had a Grid (the
		// production path); Grid-less tables fall back to the cells path
		// and must be left untouched to avoid regression.
		//
		// Guard: all merged pages must share the anchor's column count. A
		// jagged cross-page stack (continuation page with a different number
		// of columns) would feed ConstructTable a non-uniform grid, causing
		// CalSpans / CleanupOrphanColumns / RowsToHTML to misalign or
		// silently drop continuation columns and possibly delete a
		// legitimate anchor column. In that case we skip the rebuild and
		// keep the anchor-only Grid — the same safe degrade as the
		// len(anchor.Grid)==0 path (continuation rows dropped, but
		// structurally valid HTML).
		if len(anchor.Grid) > 0 && len(contGrids) > 0 {
			anchorCols := len(anchor.Grid[0])
			uniform := true
			for _, cg := range contGrids {
				if len(cg) == 0 || len(cg[0]) != anchorCols {
					uniform = false
					break
				}
			}
			if uniform {
				allGrids := make([][][]pdf.TSRCell, 0, 1+len(contGrids))
				allGrids = append(allGrids, anchor.Grid)
				allGrids = append(allGrids, contGrids...)
				if rebuilt := stackGrids(allGrids...); len(rebuilt) > 0 {
					anchor.Grid = rebuilt
				}
			}
		}
		result = append(result, anchor)
	}
	// Append unprocessed tables (those with empty Positions) so they
	// are not silently dropped from the output.
	for i := range tables {
		if !merged[i] {
			result = append(result, tables[i])
		}
	}
	return result
}

// stackGrids concatenates per-page grids (each already built correctly by
// processOneTable) into one grid for a cross-page-merged table. Continuation
// pages are shifted in Y so their rows sit strictly below the anchor rows,
// keeping Y-based downstream logic (span detection, ordering) monotonic.
func stackGrids(grids ...[][]pdf.TSRCell) [][]pdf.TSRCell {
	var out [][]pdf.TSRCell
	prevMaxY := 0.0
	for _, g := range grids {
		if len(g) == 0 {
			continue
		}
		minY, maxY := gridYExtent(g)
		if prevMaxY > 0 {
			// Place this page's rows below everything stacked so far, with a
			// gap of at least one row height to avoid false row grouping.
			shift := prevMaxY - minY + math.Max(maxY-minY, 1)
			g = shiftGridY(g, shift)
			maxY += shift
		}
		out = append(out, g...)
		prevMaxY = maxY
	}
	return out
}

// gridYExtent returns the min/max Y0/Y1 across all cells of a grid.
func gridYExtent(g [][]pdf.TSRCell) (minY, maxY float64) {
	first := true
	for _, row := range g {
		for _, c := range row {
			if first {
				minY, maxY = c.Y0, c.Y1
				first = false
				continue
			}
			if c.Y0 < minY {
				minY = c.Y0
			}
			if c.Y1 > maxY {
				maxY = c.Y1
			}
		}
	}
	return minY, maxY
}

// shiftGridY returns a copy of g with every cell's Y0/Y1 shifted by dy.
func shiftGridY(g [][]pdf.TSRCell, dy float64) [][]pdf.TSRCell {
	out := make([][]pdf.TSRCell, len(g))
	for i, row := range g {
		nr := make([]pdf.TSRCell, len(row))
		for j, c := range row {
			nc := c
			nc.Y0 += dy
			nc.Y1 += dy
			nr[j] = nc
		}
		out[i] = nr
	}
	return out
}
