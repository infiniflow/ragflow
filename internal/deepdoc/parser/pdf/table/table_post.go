package table

import (
	"log/slog"
	"math"
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

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

// buildReplacements scans for data-source-attribution boxes to remove and maps
// each table to overlapping table-layout boxes, producing the replacement list.
func buildReplacements(boxes []pdf.TextBox, tables []pdf.TableItem) (map[int]bool, []replacement) {
	removeSet := make(map[int]bool)
	for i := range boxes {
		if boxes[i].LayoutType == pdf.LayoutTypeTable && isDataSourceBox(boxes[i].Text) {
			removeSet[i] = true
		}
	}
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
	return removeSet, reps
}

func ExtractTableAndReplace(boxes []pdf.TextBox, tables []pdf.TableItem) []pdf.TextBox {
	if len(tables) == 0 {
		return boxes
	}
	// Pre-merge nomerge detection: match Python's nomerge_lout_no.
	// Traverse boxes in page order. When a caption/title/reference is
	// found, mark the preceding table group as NoMerge, preventing
	// cross-page merge when a caption ends a table group.
	// Python: if is_caption(c) or layout_type in ["table caption", "title",
	// "figure caption", "reference"]: nomerge_lout_no.append(lst_lout_no)
	MarkNoMergeTables(boxes, tables)

	// Merge same-layoutno tables across consecutive pages (Python _extract_table_figure).
	tables = MergeTablesAcrossPages(tables, nil)

	// Pre-scan: mark data-source-attribution table boxes for removal.
	// Python: if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
	// self.boxes.pop(i); continue — box discarded, no HTML replacement.
	removeSet, replacements := buildReplacements(boxes, tables)

	// Image-only PDFs (0 boxes) may have tables with cells but no
	// overlapping LayoutType=="table" boxes — generate HTML directly.
	if len(replacements) == 0 && len(boxes) == 0 {
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
				out = append(out, pdf.TextBox{
					Text: html, LayoutType: "table", PageNumber: 0,
				})
			}
		}
		return out
	}
	if len(replacements) == 0 {
		// No HTML replacements, but data-source boxes still need removal.
		if len(removeSet) == 0 {
			return boxes
		}
		out := make([]pdf.TextBox, 0, len(boxes)-len(removeSet))
		for i, b := range boxes {
			if !removeSet[i] {
				out = append(out, b)
			}
		}
		return out
	}

	// Distance-based anchor selection (Python's min_rectangle_distance).
	// Find the spatially nearest non-table text box for each table and
	// use that as the anchor, matching insert_table_figures behavior.
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
		} else {
			for _, r := range replacements {
				if r.tableIdx == ti {
					if _, ok := replacedByTable[ti]; !ok || r.boxIdx < replacedByTable[ti] {
						replacedByTable[ti] = r.boxIdx
					}
				}
			}
		}
	}
	for _, r := range replacements {
		removeSet[r.boxIdx] = true
	}

	// Build HTML for each table using post-merge boxes converted to crop space.
	htmlByTable := make(map[int]string)
	for ti := range tables {
		if len(tables[ti].Cells) == 0 {
			continue
		}
		// Convert TSR cells from crop-pixel space to page-global 72 DPI,
		// matching Python's coordinate space.  Text boxes are already in
		// page-global 72 DPI (from ocrMergeChars), so no conversion needed.
		s := tables[ti].Scale
		pageGlobalCells := CellSliceToPageSpace(tables[ti].Cells, tables[ti].CropOffX, tables[ti].CropOffY, s)
		// Collect only table-labelled boxes (Python: filters by layout_type).
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
		htmlByTable[ti] = ConstructTable(pageGlobalCells, tableBoxes, tables[ti].Caption, &tables[ti])
	}

	// Sort anchors by position for stable insertion.
	anchorList := make([]struct{ ti, pos int }, 0, len(replacedByTable))
	for ti, pos := range replacedByTable {
		anchorList = append(anchorList, struct{ ti, pos int }{ti, pos})
	}
	sort.Slice(anchorList, func(i, j int) bool { return anchorList[i].pos < anchorList[j].pos })

	out := make([]pdf.TextBox, 0, len(boxes)-len(removeSet)+len(replacedByTable))
	anchorIdx := 0
	for i, b := range boxes {
		// Insert any HTML boxes whose anchor position is before or at i.
		for anchorIdx < len(anchorList) && anchorList[anchorIdx].pos <= i {
			ti := anchorList[anchorIdx].ti
			html := htmlByTable[ti]
			if html != "" {
				tbl := &tables[ti]
				out = append(out, tableRegionBox(tbl, &b, html))
			}
			anchorIdx++
		}
		if !removeSet[i] {
			out = append(out, b)
		}
	}
	// Remaining anchors after last box.
	for anchorIdx < len(anchorList) {
		ti := anchorList[anchorIdx].ti
		html := htmlByTable[ti]
		if html != "" {
			tbl := &tables[ti]
			last := &boxes[len(boxes)-1]
			out = append(out, tableRegionBox(tbl, last, html))
		}
		anchorIdx++
	}
	return out
}

// consolidateFigures merges figure boxes that share the same LayoutNo
// (i.e., belong to the same DLA figure region) into a single pdf.TextBox.
// Matches Python's _extract_table_figure + insert_table_figures which pops
// individual figure boxes and re-inserts one consolidated figure block
// per DLA region with combined text.
//
// Figure boxes whose text matches the data-source discard pattern
// (r"(数据|资料|图表)*来源[:： ]") are removed entirely — matching Python's
// _extract_table_figure discard behavior (pdf_parser.py:1050-1052).
func ConsolidateFigures(boxes []pdf.TextBox) []pdf.TextBox {
	// Pre-scan: mark data-source-attribution figure boxes for removal.
	// Python: if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
	// self.boxes.pop(i); continue — box discarded.
	removeSet := make(map[int]bool)
	for i, b := range boxes {
		if b.LayoutType == pdf.LayoutTypeFigure && isDataSourceBox(b.Text) {
			removeSet[i] = true
		}
	}

	// Group figure boxes by (page, layoutno).
	type figKey struct {
		page int
		ln   string
	}
	groups := make(map[figKey][]int)
	for i, b := range boxes {
		if b.LayoutType != pdf.LayoutTypeFigure || removeSet[i] {
			continue
		}
		key := figKey{b.PageNumber, b.LayoutNo}
		groups[key] = append(groups[key], i)
	}

	if len(groups) == 0 {
		// Still need to filter out data-source figure boxes.
		if len(removeSet) == 0 {
			return boxes
		}
		out := make([]pdf.TextBox, 0, len(boxes)-len(removeSet))
		for i, b := range boxes {
			if !removeSet[i] {
				out = append(out, b)
			}
		}
		return out
	}

	// Collect indices to remove (all group members except the first).
	for _, indices := range groups {
		if len(indices) <= 1 {
			continue
		}
		// Merge into the first box of the group.
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

	if len(removeSet) == 0 {
		return boxes
	}

	out := make([]pdf.TextBox, 0, len(boxes)-len(removeSet))
	for i, b := range boxes {
		if !removeSet[i] {
			out = append(out, b)
		}
	}
	return out
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
