package table

import (
	"log/slog"
	"math"
	"sort"

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
	estimatedCap := len(boxes) - len(removeSet)
	if estimatedCap < 0 {
		estimatedCap = 0
	}
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
				if boxOverlapsPosition(boxes[i], tp) {
					tableBoxes = append(tableBoxes, boxes[i])
					break
				}
			}
		}
		slog.Debug("extractTableAndReplace constructTable", "table", ti, "cells", len(pageGlobalCells), "boxes", len(tableBoxes))
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

// MarkNoMergeTables traverses boxes in page order. When a caption, title, or
// reference immediately follows a table, the preceding table is marked NoMerge
// to prevent cross-page merge. Matches Python's nomerge_lout_no.
func MarkNoMergeTables(boxes []pdf.TextBox, tables []pdf.TableItem) {
	var lastTableTI int = -1
	for i := range boxes {
		lt := boxes[i].LayoutType
		if lt == pdf.LayoutTypeTable {
			matched := false
			for ti := range tables {
				for _, tp := range tables[ti].Positions {
					if boxOverlapsPosition(boxes[i], tp) {
						lastTableTI = ti
						matched = true
						break
					}
				}
			}
			if !matched {
				lastTableTI = -1
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
func buildReplacementsAfterMerge(boxes []pdf.TextBox, tables []pdf.TableItem, removeSet map[int]bool) []replacement {
	var reps []replacement
	for ti := range tables {
		for i := range boxes {
			if boxes[i].LayoutType != pdf.LayoutTypeTable || removeSet[i] {
				continue
			}
			for _, tp := range tables[ti].Positions {
				if boxOverlapsPosition(boxes[i], tp) {
					reps = append(reps, replacement{tableIdx: ti, boxIdx: i})
					break
				}
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
	tables = MergeTablesAcrossPages(tables, nil)

	// Build replacements AFTER merge so tableIdx refers to the merged slice.
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
	for _, r := range replacements {
		removeSet[r.boxIdx] = true
	}
	anchors := findTableAnchorsWithReplacements(boxes, tables, replacements)
	htmls := buildTableHTMLs(boxes, tables)
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

// boxOverlapsPosition checks if a pdf.TextBox overlaps a pdf.Position with margin.
func boxOverlapsPosition(box pdf.TextBox, pos pdf.Position) bool {
	const margin = 2.0
	return box.X0 <= pos.Right+margin && box.X1 >= pos.Left-margin &&
		box.Top <= pos.Bottom+margin && box.Bottom >= pos.Top-margin
}

// rowsToHTML converts grouped TSR cell rows to an HTML table string.
// spanInfo maps (row,col) → (colspan, rowspan) for spanning cells;
// covered marks cells hidden by a span. Both may be nil.
