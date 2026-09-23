package table

import (
	"math"
	"sort"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// FilterBoxesByRemoveSet filters boxes by index set
// removeSet: key is index to remove, value=true means remove
func FilterBoxesByRemoveSet(boxes []pdf.TextBox, removeSet map[int]bool) []pdf.TextBox {
	if len(removeSet) == 0 {
		return boxes
	}
	if len(boxes) == 0 {
		return boxes
	}
	// Pre-allocate: estimate final size to avoid resizing
	// Use max to prevent negative capacity when len(removeSet) > len(boxes)
	estimatedCap := max(len(boxes)-len(removeSet), 0)
	out := make([]pdf.TextBox, 0, estimatedCap)
	for i, b := range boxes {
		if !removeSet[i] {
			out = append(out, b)
		}
	}
	return out
}

// createTableBoxFromItem creates HTML-containing TextBox from TableItem
func createTableBoxFromItem(tbl *pdf.TableItem, html string) pdf.TextBox {
	pg := 0
	if len(tbl.Positions) > 0 && len(tbl.Positions[0].PageNumbers) > 0 {
		pg = tbl.Positions[0].PageNumbers[0]
	}
	x0, x1, top, bottom := tbl.RegionLeft, tbl.RegionRight, tbl.RegionTop, tbl.RegionBottom
	if x0 == 0 && x1 == 0 && top == 0 && bottom == 0 && len(tbl.Positions) > 0 {
		p := tbl.Positions[0]
		x0, x1, top, bottom = p.Left, p.Right, p.Top, p.Bottom
	}
	return pdf.TextBox{
		X0:         x0,
		X1:         x1,
		Top:        top,
		Bottom:     bottom,
		Text:       html,
		PageNumber: pg,
		Pages:      mergedTablePages(tbl),
		LayoutType: pdf.LayoutTypeTable,
	}
}

// handleImageOnlyPDFs handles cases with no boxes but tables (Image-only PDF)
func handleImageOnlyPDFs(tables []pdf.TableItem) []pdf.TextBox {
	var out []pdf.TextBox
	for ti := range tables {
		if len(tables[ti].Cells) == 0 {
			continue
		}
		s := tables[ti].Scale
		pageGlobalCells := CellSliceToPageSpace(tables[ti].Cells, tables[ti].CropOffX, tables[ti].CropOffY, s)
		var tableBoxes []pdf.TextBox
		html := ConstructTable(pageGlobalCells, tableBoxes, tables[ti].Caption, &tables[ti])
		if html != "" {
			out = append(out, createTableBoxFromItem(&tables[ti], html))
		}
	}
	return out
}

// findTableAnchors finds the best insertion position for each table by finding
// the spatially nearest non-table text box. Returns a list of (tableIndex, position)
// pairs sorted by position.
func findTableAnchors(boxes []pdf.TextBox, tables []pdf.TableItem) []struct{ ti, pos int } {
	replacedByTable := make(map[int]int)

	for ti := range tables {
		if len(tables[ti].Cells) == 0 {
			continue
		}
		tbl := &tables[ti]
		tblLeft, tblRight := tbl.RegionLeft, tbl.RegionRight
		tblTop, tblBottom := tbl.RegionTop, tbl.RegionBottom
		tblPg := 0
		if len(tbl.Positions) > 0 {
			p := tbl.Positions[0]
			if len(p.PageNumbers) > 0 {
				tblPg = p.PageNumbers[0]
			}
			if tblLeft == 0 && tblRight == 0 && tblTop == 0 && tblBottom == 0 {
				tblLeft, tblRight = p.Left, p.Right
				tblTop, tblBottom = p.Top, p.Bottom
			}
		}
		bestDist := math.MaxFloat64
		bestIdx := -1
		for i, b := range boxes {
			if b.LayoutType == pdf.LayoutTypeTable || b.LayoutType == pdf.LayoutTypeFigure {
				continue
			}
			if b.PageNumber != tblPg {
				continue
			}
			dist := minRectangleDistance(
				b.X0, b.X1, b.Top, b.Bottom,
				tblLeft, tblRight, tblTop, tblBottom,
			)
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			if boxes[bestIdx].Bottom < tblTop {
				bestIdx++
			}
			replacedByTable[ti] = bestIdx
		}
	}

	// Build the anchor list and sort by position
	anchorList := make([]struct{ ti, pos int }, 0, len(replacedByTable))
	for ti, pos := range replacedByTable {
		anchorList = append(anchorList, struct{ ti, pos int }{ti, pos})
	}
	sort.Slice(anchorList, func(i, j int) bool { return anchorList[i].pos < anchorList[j].pos })
	return anchorList
}

// buildTableHTMLs constructs HTML for each table, converting cells to page space first.
// Returns a map from table index to HTML string.
func buildTableHTMLs(boxes []pdf.TextBox, tables []pdf.TableItem) map[int]string {
	htmls := make(map[int]string)
	for ti := range tables {
		if len(tables[ti].Cells) == 0 {
			continue
		}
		// Convert TSR cells from crop-pixel space to page-global 72 DPI
		s := tables[ti].Scale
		pageGlobalCells := CellSliceToPageSpace(tables[ti].Cells, tables[ti].CropOffX, tables[ti].CropOffY, s)
		// Collect only table-labelled boxes
		var tableBoxes []pdf.TextBox
		for i := range boxes {
			if boxes[i].LayoutType != pdf.LayoutTypeTable {
				continue
			}
			for _, tp := range tables[ti].Positions {
				if boxOverlapsPositionPage(boxes[i], tp) {
					tableBoxes = append(tableBoxes, boxes[i])
					break
				}
			}
		}
		common.Debug("extractTableAndReplace constructTable",
			zap.Int("table", ti), zap.Int("cells", len(pageGlobalCells)), zap.Int("boxes", len(tableBoxes)))
		htmls[ti] = ConstructTable(pageGlobalCells, tableBoxes, tables[ti].Caption, &tables[ti])
	}
	return htmls
}

// insertTableBoxes filters out boxes in removeSet and inserts table HTML boxes at anchor positions.
func insertTableBoxes(boxes []pdf.TextBox, tables []pdf.TableItem, removeSet map[int]bool,
	anchors []struct{ ti, pos int }, htmls map[int]string) []pdf.TextBox {

	out := make([]pdf.TextBox, 0, len(boxes)-len(removeSet)+len(anchors))
	anchorIdx := 0
	for i, b := range boxes {
		// Insert any HTML boxes whose anchor position is before or at i
		for anchorIdx < len(anchors) && anchors[anchorIdx].pos <= i {
			ti := anchors[anchorIdx].ti
			if html, ok := htmls[ti]; ok && html != "" {
				tbl := &tables[ti]
				out = append(out, tableRegionBox(tbl, &b, html))
			}
			anchorIdx++
		}
		if !removeSet[i] {
			out = append(out, b)
		}
	}
	// Insert remaining anchors after last box
	for anchorIdx < len(anchors) {
		ti := anchors[anchorIdx].ti
		if html, ok := htmls[ti]; ok && html != "" {
			tbl := &tables[ti]
			last := &boxes[len(boxes)-1]
			out = append(out, tableRegionBox(tbl, last, html))
		}
		anchorIdx++
	}
	return out
}

// extractTableAndReplace pops table boxes and replaces them with consolidated
// HTML boxes (one per table).  This matches Python's _extract_table_figure which
// pops all boxes inside a table DLA region and inserts a single HTML box.
//
// Table boxes whose text matches the data-source discard pattern
// (r"(数据|资料|图表)*来源[:： ]") are removed entirely without replacement —
// matching Python's _extract_table_figure discard behavior.

// pagePosition pairs a table index with one of its Position indices. It is used
// to build a per-page index so callers only test a box against table positions
// on the same page instead of the full cross product.
type pagePosition struct {
	tableIdx int
	posIdx   int
}

// indexTablePositions buckets each table Position by the pages it occupies. A
// Position with no PageNumbers is page-agnostic — boxOverlapsPositionPage falls
// back to an X/Y-only test for it — so it is returned separately as noPage and
// must be tested against every box. all returns every (table, position) pair and
// is used as the fallback when a box itself carries no page metadata.
//
// Correctness note: a box on page P can only match a Position whose PageNumbers
// contains P (boxOverlapsPositionPage enforces this). A cross-page table whose
// positions sit on pages 5, 6, 7 is therefore placed in byPage[5], [6] and [7];
// a box on page 6 only sees the table's page-6 position. This is exactly
// equivalent to the unindexed cross product and never drops a valid match.
func indexTablePositions(tables []pdf.TableItem) (byPage map[int][]pagePosition, noPage []pagePosition, all []pagePosition) {
	byPage = make(map[int][]pagePosition, len(tables))
	for ti := range tables {
		for pi := range tables[ti].Positions {
			pos := tables[ti].Positions[pi]
			pp := pagePosition{tableIdx: ti, posIdx: pi}
			all = append(all, pp)
			if len(pos.PageNumbers) == 0 {
				noPage = append(noPage, pp)
				continue
			}
			for _, p := range pos.PageNumbers {
				byPage[p] = append(byPage[p], pp)
			}
		}
	}
	return byPage, noPage, all
}

// MarkNoMergeTables traverses boxes in page order. When a caption, title, or
// reference immediately follows a table, the preceding table is marked NoMerge
// to prevent cross-page merge. Matches Python's nomerge_lout_no.
func MarkNoMergeTables(boxes []pdf.TextBox, tables []pdf.TableItem) {
	byPage, noPage, all := indexTablePositions(tables)
	var lastTableTI int = -1
	for i := range boxes {
		lt := boxes[i].LayoutType
		if lt == pdf.LayoutTypeTable {
			// Restrict candidates to positions on this box's page, plus the
			// page-agnostic positions. A box without page metadata (HasPageNumber
			// false) falls back to the full set (boxOverlapsPositionPage skips
			// the page check for it). Note we key on HasPageNumber, not on
			// PageNumber == 0: page numbers are 0-based, so the legitimate first
			// page carries PageNumber == 0 and must still be scoped to its page.
			// The original cross-product keeps the highest-indexed table a box
			// overlaps as lastTableTI; candidates are ordered by ascending table
			// index, so assigning lastTableTI on every match leaves the highest
			// index in place — matching the original semantics. seen avoids
			// re-testing a table whose positions span several slots on the page.
			var cands []pagePosition
			if !boxes[i].HasPageNumber {
				cands = all
			} else {
				cands = append(cands, byPage[boxes[i].PageNumber]...)
				cands = append(cands, noPage...)
			}
			lastTableTI = -1
			seen := make(map[int]bool)
			for _, c := range cands {
				if seen[c.tableIdx] {
					continue
				}
				if boxOverlapsPositionPage(boxes[i], tables[c.tableIdx].Positions[c.posIdx]) {
					seen[c.tableIdx] = true
					lastTableTI = c.tableIdx
				}
			}
			continue
		}
		if lastTableTI >= 0 && (lt == pdf.LayoutTypeTitle || lt == pdf.DLALabelTableCaption || lt == pdf.DLALabelFigureCaption || lt == pdf.LayoutTypeReference || IsCaptionBox(boxes[i].Text, lt)) {
			tables[lastTableTI].NoMerge = true
		}
	}
}

// boxes must be post-TextMerge + post-VerticalMerge.  pdf.TableItem.Cells are in
// crop pixel space; boxes are in PDF point space — conversion via Scale/CropOff.
// replacement pairs a table index with the box index it replaces.
type replacement struct {
	tableIdx int
	boxIdx   int
}

// buildRemoveSet scans for data-source-attribution boxes to remove.
// Does NOT depend on table indices — safe to call before MergeTablesAcrossPages.
func buildRemoveSet(boxes []pdf.TextBox) map[int]bool {
	removeSet := make(map[int]bool)
	for i := range boxes {
		if boxes[i].LayoutType == pdf.LayoutTypeTable && isDataSourceBox(boxes[i].Text) {
			removeSet[i] = true
		}
	}
	return removeSet
}

// buildReplacementsAfterMerge maps each table to overlapping table-layout boxes,
// producing the replacement list. Must be called AFTER MergeTablesAcrossPages so
// that tableIdx in each replacement refers to the correct merged-table slot.
//
// The match for a box on page P is restricted to table positions on page P (plus
// any page-agnostic positions), via the per-page index built by
// indexTablePositions. This turns the O(tables*boxes*positions) cross product
// into a page-local scan; it is equivalent to the unindexed version because
// boxOverlapsPositionPage already requires a box and position to share a page.
func buildReplacementsAfterMerge(boxes []pdf.TextBox, tables []pdf.TableItem, removeSet map[int]bool) []replacement {
	byPage, noPage, all := indexTablePositions(tables)
	var reps []replacement
	for i := range boxes {
		if boxes[i].LayoutType != pdf.LayoutTypeTable || removeSet[i] {
			continue
		}
		var cands []pagePosition
		if !boxes[i].HasPageNumber {
			cands = all
		} else {
			cands = append(cands, byPage[boxes[i].PageNumber]...)
			cands = append(cands, noPage...)
		}
		// Emit one replacement per (table, box) overlap pair. A box can overlap
		// several tables on its page (plus the page-agnostic positions), and the
		// original cross-product implementation added a replacement for each such
		// table — so we must not stop at the first match. seen de-duplicates by
		// table: a table that spans several positions on the page is still a
		// single replacement for this box.
		seen := make(map[int]bool)
		for _, c := range cands {
			if seen[c.tableIdx] {
				continue
			}
			if boxOverlapsPositionPage(boxes[i], tables[c.tableIdx].Positions[c.posIdx]) {
				seen[c.tableIdx] = true
				reps = append(reps, replacement{tableIdx: c.tableIdx, boxIdx: i})
			}
		}
	}
	return reps
}

// buildReplacements scans for data-source-attribution boxes to remove and maps
// each table to overlapping table-layout boxes, producing the replacement list.
// Deprecated: pre-merge variant kept for compatibility; prefer calling
// buildRemoveSet + MergeTablesAcrossPages + buildReplacementsAfterMerge.
func buildReplacements(boxes []pdf.TextBox, tables []pdf.TableItem) (map[int]bool, []replacement) {
	removeSet := buildRemoveSet(boxes)
	reps := buildReplacementsAfterMerge(boxes, tables, removeSet)
	return removeSet, reps
}

func ExtractTableAndReplace(boxes []pdf.TextBox, tables []pdf.TableItem) []pdf.TextBox {
	removeSet := buildRemoveSet(boxes)
	if len(tables) == 0 {
		return FilterBoxesByRemoveSet(boxes, removeSet)
	}

	MarkNoMergeTables(boxes, tables)

	// Do NOT re-run MergeTablesAcrossPages here. The caller (Parser.buildLayout,
	// parser.go:540) already merged tables with page-absolute Y coordinates and
	// the real per-page medianHeights. Re-merging with nil page metadata would
	// fall back to the legacy page-local formula and re-merge pairs that the
	// page-absolute gate correctly rejected (icbccs pages 4-5 over-merge),
	// silently undoing the fix. Build replacements against the already-merged
	// slice so tableIdx refers to the correct (merged) table slot.
	replacements := buildReplacementsAfterMerge(boxes, tables, removeSet)

	if len(replacements) == 0 && len(boxes) == 0 {
		return handleImageOnlyPDFs(tables)
	}
	if len(replacements) == 0 {
		return FilterBoxesByRemoveSet(boxes, removeSet)
	}

	return processTablesWithReplacements(boxes, tables, removeSet, replacements)
}

// buildAndSortAnchors creates and sorts anchor list
func buildAndSortAnchors(anchors map[int]int) []struct{ ti, pos int } {
	result := make([]struct{ ti, pos int }, 0, len(anchors))
	for ti, pos := range anchors {
		result = append(result, struct{ ti, pos int }{ti: ti, pos: pos})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].pos < result[j].pos })
	return result
}

// processTablesWithReplacements handles normal flow with replacements
func processTablesWithReplacements(
	boxes []pdf.TextBox,
	tables []pdf.TableItem,
	removeSet map[int]bool,
	replacements []replacement,
) []pdf.TextBox {
	anchors := findTableAnchorsWithReplacements(boxes, tables, replacements)
	htmls := buildTableHTMLs(boxes, tables)
	// A box is removed only when it is actually replaced by a table HTML box.
	// A DLA-tagged region that produced no table HTML (no cells, or a
	// degenerate table whose ConstructTable returned "") keeps its original
	// text as prose instead of being silently dropped. Marking per replacement
	// (rather than un-marking empties) also avoids duplicating content when one
	// box is covered by both an empty and a real table.
	for _, r := range replacements {
		if html, ok := htmls[r.tableIdx]; ok && html != "" {
			removeSet[r.boxIdx] = true
		}
	}
	anchorList := buildAndSortAnchors(anchors)
	return insertTableBoxes(boxes, tables, removeSet, anchorList, htmls)
}

// findTableAnchorsWithReplacements is like findTableAnchors but falls back to
// replacement positions when no text box anchor is found.
func findTableAnchorsWithReplacements(boxes []pdf.TextBox, tables []pdf.TableItem,
	replacements []replacement) map[int]int {

	// First get anchors from findTableAnchors
	anchorList := findTableAnchors(boxes, tables)
	result := make(map[int]int, len(anchorList))
	for _, a := range anchorList {
		result[a.ti] = a.pos
	}

	// Fill in any missing tables using replacements
	for ti := range tables {
		if _, has := result[ti]; has {
			continue
		}
		// Find the earliest replacement for this table
		for _, r := range replacements {
			if r.tableIdx == ti {
				if _, ok := result[ti]; !ok || r.boxIdx < result[ti] {
					result[ti] = r.boxIdx
				}
			}
		}
	}
	return result
}

// figKey groups figure boxes by page and layout number
type figKey struct {
	page int
	ln   string
}

// markDataSourceBoxesForRemoval marks data source attribution figure boxes for removal
func markDataSourceBoxesForRemoval(boxes []pdf.TextBox) map[int]bool {
	removeSet := make(map[int]bool)
	for i, b := range boxes {
		if b.LayoutType == pdf.LayoutTypeFigure && isDataSourceBox(b.Text) {
			removeSet[i] = true
		}
	}
	return removeSet
}

// groupFigureBoxes groups figure boxes by (page, layoutno)
func groupFigureBoxes(boxes []pdf.TextBox, removeSet map[int]bool) map[figKey][]int {
	groups := make(map[figKey][]int)
	for i, b := range boxes {
		if b.LayoutType != pdf.LayoutTypeFigure || removeSet[i] {
			continue
		}
		key := figKey{b.PageNumber, b.LayoutNo}
		groups[key] = append(groups[key], i)
	}
	return groups
}

// mergeFigureGroups merges figure boxes within groups
func mergeFigureGroups(boxes []pdf.TextBox, groups map[figKey][]int, removeSet map[int]bool) {
	for _, indices := range groups {
		if len(indices) <= 1 {
			continue
		}
		anchor := indices[0]
		for _, idx := range indices[1:] {
			b := boxes[idx]
			boxes[anchor].Text += "\n" + b.Text
			boxes[anchor].X0 = math.Min(boxes[anchor].X0, b.X0)
			boxes[anchor].X1 = math.Max(boxes[anchor].X1, b.X1)
			boxes[anchor].Top = math.Min(boxes[anchor].Top, b.Top)
			boxes[anchor].Bottom = math.Max(boxes[anchor].Bottom, b.Bottom)
			removeSet[idx] = true
		}
	}
}

// ConsolidateFigures merges figure boxes that share the same LayoutNo
// (i.e., belong to the same DLA figure region) into a single pdf.TextBox.
// Matches Python's _extract_table_figure + insert_table_figures which pops
// individual figure boxes and re-inserts one consolidated figure block
// per DLA region with combined text.
//
// Figure boxes whose text matches the data-source discard pattern
// (r"(数据|资料|图表)*来源[:： ]") are removed entirely — matching Python's
// _extract_table_figure discard behavior (pdf_parser.py:1050-1052).
func ConsolidateFigures(boxes []pdf.TextBox) []pdf.TextBox {
	removeSet := markDataSourceBoxesForRemoval(boxes)
	groups := groupFigureBoxes(boxes, removeSet)

	if len(groups) > 0 {
		mergeFigureGroups(boxes, groups, removeSet)
	}

	return FilterBoxesByRemoveSet(boxes, removeSet)
}

// boxOverlapsPositionPage reports whether a pdf.TextBox overlaps a
// pdf.Position, additionally requiring the box to live on a page the position
// spans. Table positions and table-layout boxes are both stored in page-local
// coordinates (Y resets to ~0 at the top of every page), so a position's Y
// band is shared by the boxes of every page. Without the page constraint a
// single page-local position matches the same Y band on all pages, which (a)
// inflates the table/box replacement cross-product into a multi-GB reps slice
// and (b) makes a table wrongly claim boxes that live on other pages. When page
// metadata is missing on either side (an empty Position.PageNumbers, or a box
// whose HasPageNumber is false) we fall back to the X/Y-only check so legacy
// call paths keep working. HasPageNumber is used instead of `box.PageNumber !=
// 0` because page numbers are 0-based: the legitimate first page has
// PageNumber == 0 and must NOT be treated as "missing".
func boxOverlapsPositionPage(box pdf.TextBox, pos pdf.Position) bool {
	if len(pos.PageNumbers) > 0 && box.HasPageNumber {
		onSamePage := false
		for _, p := range pos.PageNumbers {
			if p == box.PageNumber {
				onSamePage = true
				break
			}
		}
		if !onSamePage {
			return false
		}
	}
	const margin = 2.0
	return box.X0 <= pos.Right+margin && box.X1 >= pos.Left-margin &&
		box.Top <= pos.Bottom+margin && box.Bottom >= pos.Top-margin
}

// rowsToHTML converts grouped TSR cell rows to an HTML table string.
// spanInfo maps (row,col) → (colspan, rowspan) for spanning cells;
// covered marks cells hidden by a span. Both may be nil.
