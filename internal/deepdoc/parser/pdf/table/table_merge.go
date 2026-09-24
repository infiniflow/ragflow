package table

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"ragflow/internal/common"
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
				anchor.Caption = pickMergedCaption(anchor.Caption, tables[jt.idx].Caption)
			}
			merged[jt.idx] = true
			anchorPg = bpg
			anchorBtm = bp.Bottom
			ap = anchor.Positions[len(anchor.Positions)-1]
		}
		// Rebuild the merged Grid from the per-page grids so ConstructTable
		// emits rows from every merged page, not just the stale anchor
		// (page-0) grid.
		rebuildMergedGrid(&anchor, contGrids)
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

// rebuildMergedGrid rebuilds anchor.Grid from its own per-page grid plus the
// continuation pages' grids (contGrids). Only when the anchor already had a
// Grid (the production path); Grid-less tables are handled by ConstructTable.
//
// The anchor and continuation pages form ONE logical table, but TSR
// can detect a slightly different number of columns per page (or even
// per row within a page). A non-uniform grid must NOT cause the
// continuation rows to be dropped — doing so silently deletes an
// entire continuation page from the output.
//
// Stack the unpadded per-page grids first, so the zero-coordinate
// padding cells never enter the Y-shift math in stackGrids /
// gridYExtent, then align the rebuilt grid to a shared column model.
// When every page's grid detected the same number of columns, index
// i is the same logical column on every page and short rows are
// padded by index. When TSR missed a separator on some pages only
// (e.g. a materials price list where pages 5/11 detect 16 columns
// but the rest detect 15 because one vertical line went
// undetected), index padding shifts every cell after the missed
// column one position left and prices land under the wrong headers;
// there the cells are re-mapped onto the widest grid's columns by X
// overlap instead. Both paths keep the grid uniform (CalSpans /
// CleanupOrphanColumns / RowsToHTML never see a jagged grid) while
// preserving every row.
func rebuildMergedGrid(anchor *pdf.TableItem, contGrids [][][]pdf.TSRCell) {
	if len(anchor.Grid) == 0 || len(contGrids) == 0 {
		return
	}
	allGrids := append([][][]pdf.TSRCell{anchor.Grid}, contGrids...)
	uniCols := 0
	for _, g := range allGrids {
		if w := gridMaxWidth(g); w > uniCols {
			uniCols = w
		}
	}
	keep := true
	for _, g := range allGrids {
		if len(g) == 0 {
			// Degenerate grid with no rows: drop the whole merged grid so
			// ConstructTable rebuilds from page-numbered boxes, then cells
			// if boxes are unavailable. Keeping the anchor grid would omit
			// the continuation page.
			keep = false
			break
		}
	}
	if !keep {
		anchor.Grid = nil
		anchor.Rows = nil
	} else {
		// Stack the unpadded grids first so the padded zero-coordinate
		// cells stay out of the Y-shift calculation, then align the
		// rebuilt grid to the shared column model.
		if rebuilt := stackGrids(allGrids...); len(rebuilt) > 0 {
			if gridsHaveUniformWidth(allGrids) {
				anchor.Grid = padGridCols(rebuilt, uniCols)
			} else if cols := canonicalColumns(widestGrid(allGrids)); len(cols) >= 2 {
				common.Debug("rebuildMergedGrid: per-page column counts differ, aligning by X",
					zap.Int("maxCols", uniCols), zap.Int("canonicalCols", len(cols)), zap.Int("rows", len(rebuilt)))
				anchor.Grid = alignGridColsByX(rebuilt, cols)
			} else {
				anchor.Grid = padGridCols(rebuilt, uniCols)
			}
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
		orphanGap := maxOrphanMergeGapPoints
		if anchor.Scale > 0 {
			orphanGap *= anchor.Scale
		}
		anchor.Grid = DropAllEmptyRows(anchor.Grid)
		anchor.Grid = cleanupOrphanColumns(anchor.Grid, orphanGap)
		anchor.Grid = cleanupOrphanRows(anchor.Grid, orphanGap)
		anchor.Rows = RowsToStrings(anchor.Grid)
	}
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
	for gi, g := range grids {
		if len(g) == 0 {
			continue
		}
		if len(out) > 0 && len(g) > 1 && isRepeatedHeader(out[0], g[0]) {
			common.Debug("stackGrids: stripped repeated header row", zap.Int("grid", gi), zap.Int("cells", len(g[0])))
			g = g[1:]
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

// isRepeatedHeader requires matching text and explicit header labels on both
// pages before removing a continuation row. Text alone can match a data row.
func isRepeatedHeader(headerRow []pdf.TSRCell, candidateRow []pdf.TSRCell) bool {
	if len(headerRow) == 0 || len(candidateRow) == 0 {
		return false
	}
	headerTexts := make(map[string]bool)
	headerLabelTexts := make(map[string]bool)
	for _, c := range headerRow {
		t := strings.TrimSpace(c.Text)
		if t != "" {
			headerTexts[t] = true
			if isHeaderLabel(c.Label) {
				headerLabelTexts[t] = true
			}
		}
	}
	if len(headerLabelTexts) < 2 {
		return false
	}
	matches := 0
	exactMatches := 0
	candidateNonEmpty := 0
	for _, c := range candidateRow {
		t := strings.TrimSpace(c.Text)
		if t == "" {
			continue
		}
		candidateNonEmpty++
		if headerTexts[t] {
			matches++
			if isHeaderLabel(c.Label) && headerLabelTexts[t] {
				exactMatches++
			}
		} else {
			for ht := range headerTexts {
				// Rune count, not bytes: len() is >=3 for every single CJK
				// glyph, so a byte guard would let one-character CJK headers
				// substring-match any data cell containing that character and
				// strip real data rows as "repeated headers".
				if utf8.RuneCountInString(ht) > 1 && strings.Contains(t, ht) {
					matches++
					break
				}
			}
		}
	}
	if candidateNonEmpty == 0 {
		return false
	}
	return exactMatches >= 2 && float64(matches)/float64(candidateNonEmpty) >= 0.5
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
// existing cells at the same column indices — valid only when every page's
// grid detected the same columns (see rebuildMergedGrid for the
// mismatched-column path). Rows are never added or removed, so no content is
// lost when per-row column counts differ.
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

// gridsHaveUniformWidth reports whether every ROW of every grid has the same
// number of columns — the case where index-based column alignment is correct.
// Production page grids are TSR-row cross products so their rows are naturally
// equal-width; the per-row check (rather than a per-page max) keeps the
// guarantee honest if a grid ever arrives jagged: a row narrower than its
// page's max means a locally missed separator, whose cells must go through
// X-based alignment instead of index padding.
func gridsHaveUniformWidth(grids [][][]pdf.TSRCell) bool {
	w := -1
	for _, g := range grids {
		for _, row := range g {
			if w < 0 {
				w = len(row)
			} else if len(row) != w {
				return false
			}
		}
	}
	return true
}

// widestGrid returns the grid with the longest row — its columns are the best
// candidate for the shared column model (TSR can only miss separators, so the
// page with the most columns detected the most structure).
func widestGrid(grids [][][]pdf.TSRCell) [][]pdf.TSRCell {
	var best [][]pdf.TSRCell
	bestW := 0
	for _, g := range grids {
		if w := gridMaxWidth(g); w > bestW {
			best, bestW = g, w
		}
	}
	return best
}

func gridMaxWidth(g [][]pdf.TSRCell) int {
	w := 0
	for _, row := range g {
		if len(row) > w {
			w = len(row)
		}
	}
	return w
}

// canonicalColumns derives the column X intervals of a grid by clustering its
// cell X intervals. Spanning cells are excluded because their bbox is widened
// across the region they cover, not a single column. Returns nil when fewer
// than two columns can be derived.
func canonicalColumns(g [][]pdf.TSRCell) [][2]float64 {
	var ivs [][2]float64
	for _, row := range g {
		for _, c := range row {
			if c.X1 <= c.X0 || strings.Contains(c.Label, "spanning") {
				continue
			}
			ivs = append(ivs, [2]float64{c.X0, c.X1})
		}
	}
	if len(ivs) == 0 {
		return nil
	}
	sort.Slice(ivs, func(i, j int) bool {
		if ivs[i][0] != ivs[j][0] {
			return ivs[i][0] < ivs[j][0]
		}
		return ivs[i][1] < ivs[j][1]
	})
	cols := [][2]float64{ivs[0]}
	for _, iv := range ivs[1:] {
		last := &cols[len(cols)-1]
		inter := math.Min(last[1], iv[1]) - math.Max(last[0], iv[0])
		minW := math.Min(last[1]-last[0], iv[1]-iv[0])
		if inter >= minW/2 {
			// Same column (grid cross-product rows repeat each column's
			// interval with sub-pixel differences): union them.
			if iv[0] < last[0] {
				last[0] = iv[0]
			}
			if iv[1] > last[1] {
				last[1] = iv[1]
			}
			continue
		}
		cols = append(cols, iv)
	}
	if len(cols) < 2 {
		return nil
	}
	return cols
}

// alignGridColsByX re-assigns every cell of the stacked grid to the canonical
// column with the largest X overlap (nearest column center when there is no
// overlap), so continuation pages whose TSR missed a separator still land
// their values under the anchor's correct headers. Rows are never added or
// dropped; cells colliding in one (row, column) are merged instead of lost.
// Degenerate cells (zero-coordinate padding / covered placeholders) are
// dropped — coverage is recomputed downstream by CalSpans/MarkCoveredCells.
func alignGridColsByX(grid [][]pdf.TSRCell, cols [][2]float64) [][]pdf.TSRCell {
	out := make([][]pdf.TSRCell, 0, len(grid))
	for _, row := range grid {
		nr := make([]pdf.TSRCell, len(cols))
		used := make([]bool, len(cols))
		for _, c := range row {
			if c.X1 <= c.X0 {
				continue
			}
			ci := bestOverlapColumn(cols, c.X0, c.X1)
			if !used[ci] {
				used[ci] = true
				nr[ci] = c
				continue
			}
			p := &nr[ci]
			if c.Text != "" && !strings.Contains(p.Text, c.Text) {
				if p.Text == "" {
					p.Text = c.Text
				} else {
					p.Text += " " + c.Text
				}
			}
			if c.Label != "" && !strings.Contains(p.Label, c.Label) {
				p.Label += " " + c.Label
			}
			if c.X0 < p.X0 {
				p.X0 = c.X0
			}
			if c.X1 > p.X1 {
				p.X1 = c.X1
			}
			if c.Y0 < p.Y0 {
				p.Y0 = c.Y0
			}
			if c.Y1 > p.Y1 {
				p.Y1 = c.Y1
			}
			if c.Score > p.Score {
				p.Score = c.Score
			}
		}
		out = append(out, nr)
	}
	return out
}

func bestOverlapColumn(cols [][2]float64, x0, x1 float64) int {
	best, bestInter := 0, 0.0
	for i, col := range cols {
		inter := math.Min(col[1], x1) - math.Max(col[0], x0)
		if inter > bestInter {
			best, bestInter = i, inter
		}
	}
	if bestInter > 0 {
		return best
	}
	ctr := (x0 + x1) / 2
	bestD := math.Inf(1)
	for i, col := range cols {
		if d := math.Abs((col[0]+col[1])/2 - ctr); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}
