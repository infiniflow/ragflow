package table

import (
	"math"
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// MergeTablesAcrossPages merges TableItems on consecutive pages with
// overlapping X and close Y proximity.  Matches Python's
// _extract_table_figure table merge (pdf_parser.py:1061-1080).
//
// pageHeights maps each 0-based page number to its PDF-point page height. It
// is required to measure the cross-page Y gap in a page-absolute frame: the
// continuation table's page-local Top must be offset by the anchor page's
// height, otherwise two tables whose page-local Y merely repeats every page
// look adjacent and get wrongly merged.
func MergeTablesAcrossPages(tables []pdf.TableItem, medianHeights, pageHeights map[int]float64) []pdf.TableItem {
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
			// page-local yDis (Y resets to 0 on each page). A genuine
			// cross-page continuation sits at the TOP of the next page, which
			// in page-local coordinates is "above" the anchor, so yDis is
			// NEGATIVE — merge it as-is. Two separate tables that merely
			// repeat their page-local Y every page have a POSITIVE yDis; only
			// then shift into the page-absolute frame (by the anchor page
			// height) so the over-merge is rejected. This restates the
			// icbccs-crosspage-table-overmerge guard from #18688 without also
			// rejecting legitimate continuations.
			yDis := (bp.Top + bp.Bottom - anchorBtm - ap.Bottom) / 2
			if yDis >= 0 {
				if anchorPageH, ok := pageHeights[anchorPg]; ok && anchorPageH > 0 {
					yDis += anchorPageH
				}
				if yDis > mh*23 {
					continue
				}
			} else {
				// A NEGATIVE page-local yDis means the continuation sits at the
				// TOP of the next page (in page-local coordinates it is "above"
				// the anchor). A genuine cross-page split is cut off at the page
				// boundary, so its anchor must END NEAR THE BOTTOM of its page.
				// Two independent tables that merely both start near the top of
				// consecutive pages (e.g. ZoomNeXt's R3→R5) also produce a
				// negative yDis but their anchor ends high on its page — merging
				// them wrongly collapses the second table into the first and
				// silently drops it. Reject the merge unless the anchor bottom is
				// within mh*23 of the page bottom (the same proximity used for
				// the Y gate), so only real page-boundary continuations merge.
				if anchorPageH, ok := pageHeights[anchorPg]; ok && anchorPageH > 0 {
					if maxBottomOnPage(anchor.Positions, anchorPg) < anchorPageH-mh*23 {
						continue
					}
				}
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
		// The anchor and continuation pages form ONE logical table, but TSR
		// can detect a slightly different number of columns per page (or even
		// per row within a page). A non-uniform grid must NOT cause the
		// continuation rows to be dropped — doing so silently deletes an
		// entire continuation page from the output.
		//
		// We stack the unpadded per-page grids first, so the zero-coordinate
		// padding cells never enter the Y-shift math in stackGrids /
		// gridYExtent, then align the rebuilt grid to a shared column model:
		// the maximum column count seen across all rows of all grids, padding
		// shorter rows by index. Column i of a continuation page maps to
		// column i of the anchor because they are the same logical column of
		// one cross-page table, so padding keeps the grid uniform
		// (CalSpans / CleanupOrphanColumns / RowsToHTML never see a jagged
		// grid) while preserving every row.
		if len(anchor.Grid) > 0 && len(contGrids) > 0 {
			allGrids := append([][][]pdf.TSRCell{anchor.Grid}, contGrids...)
			uniCols := 0
			for _, g := range allGrids {
				for _, row := range g {
					if len(row) > uniCols {
						uniCols = len(row)
					}
				}
			}
			keep := true
			for _, g := range allGrids {
				if len(g) == 0 {
					// Degenerate grid with no rows: degrade to anchor-only so
					// we don't build a malformed grid.
					keep = false
					break
				}
			}
			if keep {
				// Stack the unpadded grids first so the padded zero-coordinate
				// cells stay out of the Y-shift calculation, then align the
				// rebuilt grid to the shared column model.
				if rebuilt := stackGrids(allGrids...); len(rebuilt) > 0 {
					anchor.Grid = padGridCols(rebuilt, uniCols)
				}
			}
			// Re-run the post-GroupCells cleanup that processOneTable would
			// otherwise have applied per-page: stackGrids rebuilds the grid
			// from raw (un-cleaned) per-page cells, so the empty / orphan
			// cleanup done inside ConstructTable never runs on the merged
			// grid. Without it, an extra "table row" detected next to a
			// "table projected row header" on a cross-page continuation
			// page (e.g. 13_crosspage_table.pdf page 2 y0=885) leaks into
			// the merged grid as a row of empty cells, inflating
			// item.Grid and breaking gridSim against Python's box.R
			// grouping which never produces such a row. See
			// table_construct.go dropAllEmptyRows for the matching
			// per-page fix.
			if len(anchor.Grid) > 0 && HasText(anchor.Grid) {
				anchor.Grid = DropAllEmptyRows(anchor.Grid)
				anchor.Grid = CleanupOrphanColumns(anchor.Grid)
				anchor.Grid = CleanupOrphanRows(anchor.Grid)
				anchor.Rows = RowsToStrings(anchor.Grid)
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

// maxBottomOnPage returns the largest Bottom among the table's positions that
// carry page number pg. Page-local Y resets to 0 at each page top, so a
// multi-page anchor's positions across different pages are not directly
// comparable; this isolates the anchor's extent on the specific page it is
// being tested against for a cross-page continuation.
func maxBottomOnPage(positions []pdf.Position, pg int) float64 {
	var mb float64
	for _, p := range positions {
		onPage := false
		for _, pn := range p.PageNumbers {
			if pn == pg {
				onPage = true
				break
			}
		}
		if !onPage {
			continue
		}
		if p.Bottom > mb {
			mb = p.Bottom
		}
	}
	return mb
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

// padGridCols returns a copy of grid with every row extended to width uniCols
// by appending zero-valued cells. Grids shorter than uniCols keep their
// existing cells at the same column indices; column i of a continuation page
// maps to column i of the anchor because they are the same logical column of
// one cross-page table. Rows are never added or removed, so no content is
// lost when per-page (or per-row) column counts differ.
func padGridCols(grid [][]pdf.TSRCell, uniCols int) [][]pdf.TSRCell {
	if uniCols <= 0 {
		return grid
	}
	out := make([][]pdf.TSRCell, len(grid))
	for i, row := range grid {
		if len(row) >= uniCols {
			out[i] = row
			continue
		}
		nr := make([]pdf.TSRCell, uniCols)
		copy(nr, row)
		out[i] = nr
	}
	return out
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
