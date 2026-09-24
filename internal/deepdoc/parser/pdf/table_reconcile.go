package pdf

import (
	"math"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
)

type pageTableCandidate struct {
	item   pdf.TableItem
	boxIdx []int
	region pdf.DLARegion
}

type sourcedTableRow struct {
	cells  []pdf.TSRCell
	boxIDs map[int]bool
	top    float64
	bottom float64
}

// Reconcile overlapping DLA crops before page merging, while their OCR box
// identities and TSR row geometry are still available.
func reconcileContainedPageTables(candidates []pageTableCandidate, boxes []pdf.TextBox) []pageTableCandidate {
	if len(candidates) < 2 {
		return candidates
	}
	order := make([]int, len(candidates))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return len(candidates[order[i]].boxIdx) > len(candidates[order[j]].boxIdx)
	})
	dropped := make([]bool, len(candidates))
	for _, parent := range order {
		if dropped[parent] {
			continue
		}
		for _, child := range order {
			if parent == child || dropped[child] || !sameTableHorizontalSpan(candidates[parent].region, candidates[child].region) ||
				!strictlyContainsBoxIDs(candidates[parent].boxIdx, candidates[child].boxIdx) {
				continue
			}
			columnsMatch := compatibleTableColumns(candidates[parent].item, candidates[child].item)
			if !columnsMatch && !coarserShortCandidate(candidates[parent].item, candidates[child].item) {
				continue
			}
			if mergeContainedCandidateRows(&candidates[parent].item, candidates[child].item, columnsMatch) {
				dropped[child] = true
			}
		}
	}
	out := make([]pageTableCandidate, 0, len(candidates))
	for i, candidate := range candidates {
		if !dropped[i] {
			out = append(out, candidate)
		}
	}
	return removeSharedPageRows(out, boxes)
}

func coarserShortCandidate(parent, child pdf.TableItem) bool {
	const maxCoarseFragmentRows = 3
	const minParentColumns = 4
	if len(child.Grid) == 0 || len(child.Grid) > maxCoarseFragmentRows || len(parent.Grid) == 0 {
		return false
	}
	columnCount := func(item pdf.TableItem) int {
		count := 0
		for _, cell := range item.Cells {
			if strings.HasSuffix(cell.Label, "table column") {
				count++
			}
		}
		return count
	}
	return columnCount(parent) >= minParentColumns && columnCount(parent) > columnCount(child) && len(child.Grid[0]) >= 2
}

func removeSharedPageRows(candidates []pageTableCandidate, boxes []pdf.TextBox) []pageTableCandidate {
	order := make([]int, len(candidates))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return candidates[order[i]].region.Y0 < candidates[order[j]].region.Y0
	})
	for earlier := 0; earlier < len(order); earlier++ {
		owner := &candidates[order[earlier]]
		for later := earlier + 1; later < len(order); later++ {
			duplicate := &candidates[order[later]]
			if !sameTableHorizontalSpan(owner.region, duplicate.region) || !boxIDSlicesOverlap(owner.boxIdx, duplicate.boxIdx) ||
				!compatibleTableColumns(owner.item, duplicate.item) {
				continue
			}
			ownerRows := sourceTableRows(owner.item, owner.boxIdx, boxes)
			duplicateRows := sourceTableRows(duplicate.item, duplicate.boxIdx, boxes)
			removed := make(map[int]bool)
			keptRows := make([][]pdf.TSRCell, 0, len(duplicateRows))
			for _, row := range duplicateRows {
				shared := false
				if len(row.boxIDs) > 0 && rowHasText(row.cells) {
					for _, ownerRow := range ownerRows {
						if boxIDsEqual(ownerRow.boxIDs, row.boxIDs) && rowCoversOCRText(ownerRow, boxes) &&
							normalizedGridRowText(ownerRow.cells) == normalizedGridRowText(row.cells) {
							shared = true
							break
						}
					}
				}
				if shared {
					for id := range row.boxIDs {
						removed[id] = true
					}
				} else {
					keptRows = append(keptRows, row.cells)
				}
			}
			if len(removed) == 0 {
				continue
			}
			duplicate.item.Grid = keptRows
			keptIDs := make([]int, 0, len(duplicate.boxIdx))
			keptPositions := make([]pdf.Position, 0, len(duplicate.item.Positions))
			for i, id := range duplicate.boxIdx {
				if !removed[id] {
					keptIDs = append(keptIDs, id)
					if i < len(duplicate.item.Positions) {
						keptPositions = append(keptPositions, duplicate.item.Positions[i])
					}
				}
			}
			duplicate.boxIdx = keptIDs
			duplicate.item.Positions = keptPositions
		}
	}
	out := make([]pageTableCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if len(candidate.item.Grid) > 0 || len(candidate.boxIdx) > 0 {
			out = append(out, candidate)
		}
	}
	return out
}

func boxIDSlicesOverlap(a, b []int) bool {
	seen := make(map[int]bool, len(a))
	for _, id := range a {
		seen[id] = true
	}
	for _, id := range b {
		if seen[id] {
			return true
		}
	}
	return false
}

func compatibleTableColumns(a, b pdf.TableItem) bool {
	const duplicateColumnTolerance = 5.0
	const singleColumnTolerance = 10.0
	const columnCenterTolerance = 8.0
	const minMatchingColumnFraction = 0.7
	columns := func(item pdf.TableItem) []pdf.TSRCell {
		if item.Scale <= 0 {
			return nil
		}
		var out []pdf.TSRCell
		for _, cell := range item.Cells {
			if strings.HasSuffix(cell.Label, "table column") {
				out = append(out, pdf.TSRCell{
					X0: (cell.X0 + item.CropOffX) / item.Scale,
					X1: (cell.X1 + item.CropOffX) / item.Scale,
				})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].X0 < out[j].X0 })
		unique := out[:0]
		for _, column := range out {
			if len(unique) > 0 && math.Abs(column.X0-unique[len(unique)-1].X0) <= duplicateColumnTolerance &&
				math.Abs(column.X1-unique[len(unique)-1].X1) <= duplicateColumnTolerance {
				continue
			}
			unique = append(unique, column)
		}
		return unique
	}
	left, right := columns(a), columns(b)
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	if len(left) == 1 || len(right) == 1 {
		return len(left) == 1 && len(right) == 1 &&
			math.Abs(left[0].X0-right[0].X0) <= singleColumnTolerance && math.Abs(left[0].X1-right[0].X1) <= singleColumnTolerance
	}
	matched := 0
	for i, j := 0, 0; i < len(left) && j < len(right); {
		leftCenter := (left[i].X0 + left[i].X1) / 2
		rightCenter := (right[j].X0 + right[j].X1) / 2
		if math.Abs(leftCenter-rightCenter) <= columnCenterTolerance {
			matched++
			i++
			j++
		} else if leftCenter < rightCenter {
			i++
		} else {
			j++
		}
	}
	return matched >= 2 && float64(matched) >= minMatchingColumnFraction*float64(min(len(left), len(right)))
}

func sameTableHorizontalSpan(a, b pdf.DLARegion) bool {
	if a.X1 <= a.X0 || b.X1 <= b.X0 {
		return false
	}
	margin := 2 * util.TSRRegionMarginPx
	return math.Abs(a.X0-b.X0) <= margin && math.Abs(a.X1-b.X1) <= margin
}

func strictlyContainsBoxIDs(parent, child []int) bool {
	if len(child) == 0 || len(child) >= len(parent) {
		return false
	}
	ids := make(map[int]bool, len(parent))
	for _, id := range parent {
		ids[id] = true
	}
	for _, id := range child {
		if !ids[id] {
			return false
		}
	}
	return true
}

func mergeContainedCandidateRows(parent *pdf.TableItem, child pdf.TableItem, matchingColumns bool) bool {
	if len(parent.Grid) == 0 || len(child.Grid) == 0 {
		return false
	}
	childTop, childBottom := math.Inf(1), math.Inf(-1)
	for _, row := range child.Grid {
		top, bottom := gridRowPageBounds(row, child)
		childTop = math.Min(childTop, top)
		childBottom = math.Max(childBottom, bottom)
	}
	if childTop >= childBottom {
		return false
	}
	var selected []int
	for i, row := range parent.Grid {
		top, bottom := gridRowPageBounds(row, *parent)
		midpoint := (top + bottom) / 2
		if top < bottom && midpoint >= childTop && midpoint <= childBottom {
			selected = append(selected, i)
		}
	}
	if len(selected) > 0 && sameRowCharacters(parent.Grid, selected, child.Grid) {
		if !matchingColumns {
			return childRowsCoveredByParent(*parent, child, false)
		}
		if len(child.Grid) > len(selected) {
			if !sameOrderedRowCharacters(parent.Grid, selected, child.Grid) {
				return false
			}
			merged := make([][]pdf.TSRCell, 0, len(parent.Grid)-len(selected)+len(child.Grid))
			first := selected[0]
			skip := make(map[int]bool, len(selected))
			for _, i := range selected {
				skip[i] = true
			}
			for i, row := range parent.Grid {
				if i == first {
					for _, childRow := range child.Grid {
						merged = append(merged, rowInCrop(childRow, child, *parent))
					}
				}
				if !skip[i] {
					merged = append(merged, row)
				}
			}
			parent.Grid = merged
		}
		return true
	}
	return childRowsCoveredByParent(*parent, child, matchingColumns)
}

func sameOrderedRowCharacters(parent [][]pdf.TSRCell, selected []int, child [][]pdf.TSRCell) bool {
	childPrefixes := make([]map[rune]int, len(child))
	childCounts := make(map[rune]int)
	for i, row := range child {
		for _, r := range normalizedGridRowText(row) {
			childCounts[r]++
		}
		prefix := make(map[rune]int, len(childCounts))
		for r, count := range childCounts {
			prefix[r] = count
		}
		childPrefixes[i] = prefix
	}
	parentCounts := make(map[rune]int)
	lastMatch := -1
	for _, i := range selected {
		for _, r := range normalizedGridRowText(parent[i]) {
			parentCounts[r]++
		}
		matched := false
		for j := lastMatch + 1; j < len(childPrefixes); j++ {
			if runeCountsEqual(parentCounts, childPrefixes[j]) {
				lastMatch = j
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return lastMatch == len(child)-1
}

func runeCountsEqual(a, b map[rune]int) bool {
	if len(a) != len(b) {
		return false
	}
	for r, count := range a {
		if b[r] != count {
			return false
		}
	}
	return true
}

func childRowsCoveredByParent(parent, child pdf.TableItem, allowSubstring bool) bool {
	used := make(map[int]bool)
	for _, childRow := range child.Grid {
		childText := normalizedGridRowText(childRow)
		childTop, childBottom := gridRowPageBounds(childRow, child)
		if childText == "" || childTop >= childBottom {
			return false
		}
		matched := false
		for i, parentRow := range parent.Grid {
			if used[i] {
				continue
			}
			parentTop, parentBottom := gridRowPageBounds(parentRow, parent)
			if math.Min(childBottom, parentBottom) <= math.Max(childTop, parentTop) {
				continue
			}
			parentText := normalizedGridRowText(parentRow)
			if childText == parentText || allowSubstring && strings.Contains(parentText, childText) {
				used[i] = true
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func sameRowCharacters(parent [][]pdf.TSRCell, selected []int, child [][]pdf.TSRCell) bool {
	counts := make(map[rune]int)
	for _, i := range selected {
		for _, c := range normalizedGridRowText(parent[i]) {
			counts[c]++
		}
	}
	for _, row := range child {
		for _, c := range normalizedGridRowText(row) {
			counts[c]--
		}
	}
	if len(counts) == 0 {
		return false
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

func sourceTableRows(item pdf.TableItem, boxIDs []int, boxes []pdf.TextBox) []sourcedTableRow {
	const minBoxHeightOverlap = 0.3
	rows := make([]sourcedTableRow, len(item.Grid))
	for i, cells := range item.Grid {
		rows[i] = sourcedTableRow{cells: cells, boxIDs: make(map[int]bool)}
		rows[i].top, rows[i].bottom = gridRowPageBounds(cells, item)
	}
	for _, id := range boxIDs {
		if id < 0 || id >= len(boxes) {
			continue
		}
		box := boxes[id]
		bestRow, bestOverlap := -1, 0.0
		for i, row := range rows {
			overlap := math.Min(box.Bottom, row.bottom) - math.Max(box.Top, row.top)
			if overlap > bestOverlap {
				bestRow, bestOverlap = i, overlap
			}
		}
		if bestRow >= 0 && bestOverlap >= (box.Bottom-box.Top)*minBoxHeightOverlap {
			rows[bestRow].boxIDs[id] = true
		}
	}
	return rows
}

func gridRowPageBounds(row []pdf.TSRCell, item pdf.TableItem) (float64, float64) {
	if item.Scale <= 0 {
		return math.Inf(1), math.Inf(-1)
	}
	top, bottom := math.Inf(1), math.Inf(-1)
	for _, cell := range row {
		if cell.Y1 <= cell.Y0 {
			continue
		}
		top = math.Min(top, (cell.Y0+item.CropOffY)/item.Scale)
		bottom = math.Max(bottom, (cell.Y1+item.CropOffY)/item.Scale)
	}
	return top, bottom
}

func rowInCrop(row []pdf.TSRCell, from, to pdf.TableItem) []pdf.TSRCell {
	if from.Scale <= 0 || to.Scale <= 0 {
		return row
	}
	out := make([]pdf.TSRCell, len(row))
	for i, cell := range row {
		out[i] = cell
		out[i].X0 = (cell.X0+from.CropOffX)/from.Scale*to.Scale - to.CropOffX
		out[i].X1 = (cell.X1+from.CropOffX)/from.Scale*to.Scale - to.CropOffX
		out[i].Y0 = (cell.Y0+from.CropOffY)/from.Scale*to.Scale - to.CropOffY
		out[i].Y1 = (cell.Y1+from.CropOffY)/from.Scale*to.Scale - to.CropOffY
	}
	return out
}

func rowCoversOCRText(row sourcedTableRow, boxes []pdf.TextBox) bool {
	text := normalizedGridRowText(row.cells)
	if text == "" || len(row.boxIDs) == 0 {
		return false
	}
	for id := range row.boxIDs {
		ocrText := strings.Join(strings.Fields(boxes[id].Text), "")
		if ocrText != "" && !strings.Contains(text, ocrText) {
			return false
		}
	}
	return true
}

func normalizedGridRowText(row []pdf.TSRCell) string {
	var text strings.Builder
	for _, cell := range row {
		text.WriteString(strings.Join(strings.Fields(cell.Text), ""))
	}
	return text.String()
}

func rowHasText(row []pdf.TSRCell) bool { return normalizedGridRowText(row) != "" }

func boxIDsContained(parent, child map[int]bool) bool {
	if len(child) == 0 || len(child) > len(parent) {
		return false
	}
	for id := range child {
		if !parent[id] {
			return false
		}
	}
	return true
}

func boxIDsEqual(a, b map[int]bool) bool { return len(a) == len(b) && boxIDsContained(a, b) }
