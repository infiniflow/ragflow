package table

import (
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
)

// ── TSR cell grouping ──────────────────────────────────────────────────

// GroupTSRCellsToRows groups TSR cells into rows by Y proximity.
// This is the basic fallback grouping used when model-specific grouping
// (e.g. EE label-aware grouping) is not applicable.
func GroupTSRCellsToRows(cells []pdf.TSRCell) [][]pdf.TSRCell {
	if len(cells) == 0 {
		return nil
	}
	if len(cells) == 1 {
		return [][]pdf.TSRCell{{cells[0]}}
	}
	heights := make([]float64, len(cells))
	for i, c := range cells {
		heights[i] = c.Y1 - c.Y0
	}
	sort.Float64s(heights)
	medianH := heights[len(heights)/2]
	if medianH <= 0 {
		medianH = 10
	}
	rowThreshold := medianH * 0.5

	sort.Slice(cells, func(i, j int) bool {
		if math.Abs(cells[i].Y0-cells[j].Y0) < rowThreshold {
			return cells[i].X0 < cells[j].X0
		}
		return cells[i].Y0 < cells[j].Y0
	})

	var rows [][]pdf.TSRCell
	var curRow []pdf.TSRCell
	curY := 0.0
	for _, c := range cells {
		if len(curRow) == 0 {
			curRow = append(curRow, c)
			curY = c.Y0
			continue
		}
		if c.Y0-curY > rowThreshold {
			rows = append(rows, curRow)
			curRow = []pdf.TSRCell{c}
			curY = c.Y0
		} else {
			curRow = append(curRow, c)
		}
	}
	if len(curRow) > 0 {
		rows = append(rows, curRow)
	}
	for _, row := range rows {
		sort.Slice(row, func(i, j int) bool { return row[i].X0 < row[j].X0 })
	}
	return rows
}

// ── cell text filling ──────────────────────────────────────────────────

// FillCellTextFromBoxes assigns PDF text boxes to TSR grid cells, mirroring
// Python's construct_table box→cell assignment (pdf_parser.py +
// table_structure_recognizer.py):
//
//  1. For each box, pick the single BEST row by vertical-overlap ratio
//     inter(box,rowStrip)/area(box) >= 0.3, tie-broken by inter/rowArea
//     (Python find_overlapped_with_threshold on the full-width row strip).
//  2. Within that row, pick the TIGHTEST column by horizontal edge/center
//     distance, requiring vertical overlap (Python find_horizontally_tightest_fit,
//     NO threshold). The box lands in exactly ONE cell (R,C).
//  3. Multiple boxes mapped to the same (R,C) are concatenated (Python joins
//     them in construct_table).
//
// This replaces the old many-to-many 2-D cell-overlap filter
// (inter(box,cell)/area(box) >= 0.3 on every cross-product cell), which
// duplicated a straddling box into two cells (#1), dropped boxes whose 2-D
// cell overlap was < 0.3 even though their row-vertical overlap was >= 0.3
// (#2), and never applied the inter/cellArea tie-break (#3). All three are
// go_bug in testdata/parity/known_diffs.json.
//
// The Go-only 0.85 guard (BoxMatchesCell) is retained for PRE-FILLED cells
// only: if a cell already carries text (e.g. per-cell OCR in the rotated
// path), a detected box overrides it only when it sits almost entirely inside
// the cell (>= 0.85). Empty cells accept any box the row/column selection
// picked, matching Python. See go_intentional rule
// table-cell-fill-filled-threshold-0.85.
func FillCellTextFromBoxes(cells []pdf.TSRCell, boxes []pdf.TextBox) {
	FillCellTextFromBoxesWithRows(cells, boxes, nil)
}

// FillCellTextFromBoxesWithRows is FillCellTextFromBoxes with the per-row
// strip X range taken from the original TSR "table row" component bboxes
// instead of the grid column union.
//
// Python matches a box to a row via find_overlapped_with_threshold(box,
// rows) where rows are the raw TSR row components (gather(".* (row|header)")
// in pdf_parser.py:602) and overlapped_area divides by the row's own bbox.
// Go's grid is a TSR-row × column cross-product; its cells always span the
// full column union, so the old strip X range was wider than the row
// component's true bbox. When TSR emits a row whose line does not cover a
// table edge (e.g. 13_crosspage_table.pdf page 2 row 44: x0=106.9 vs the
// grid's x0=90.8), a col-0 box straddling that row and the one above it
// can pass the 0.3 threshold against the full-width strip while Python
// rejects it against the true bbox — so Go assigned the box one row lower
// than Python.
//
// rowStrips are the TSR "table row" components (data rows only — header /
// projrowheader components are excluded by the caller). Each is matched to a
// grid row band by Y coordinate via matchRowStrip, so the override applies to
// every data row regardless of whether the table has a header. Rows with no
// matching TSR row component (e.g. header rows) keep the grid-union strip X.
func FillCellTextFromBoxesWithRows(cells []pdf.TSRCell, boxes []pdf.TextBox, rowStrips []pdf.TSRCell) {
	slog.Debug("fillCellTextFromBoxes", "cells", len(cells), "boxes", len(boxes))
	if len(cells) == 0 || len(boxes) == 0 {
		return
	}

	// Group cells into row bands by their top coordinate. The grid is a TSR
	// row×column cross-product, so every cell in a row shares the same Y band.
	// A row band spans the full table width (union of its cells), matching
	// Python's full-width "table row" components.
	//
	// Spanning cells are folded into the nearest band by Y0 instead of opening
	// their own band. GroupCells extends a span cell's bbox across the covered
	// region, so its Y0 differs from the sibling cells' Y0; a separate band
	// would span several TSR rows and the top-containment rule would swallow
	// every covered box into it, emptying the real rows (11→6 in dell/
	// screenshot real-PDF parity). Python assigns boxes to rows by TSR row
	// component overlap only — spanning cells never define a row.
	type rowBand struct {
		y0, y1  float64
		stripX0 float64
		stripX1 float64
		cells   []int // indices into `cells`
	}
	var rows []rowBand
	// Group cells into the same row band by exact top coordinate. A TSR
	// cross-product grid (GroupCells) assigns every cell in a row the SAME
	// Y0 value, and distinct rows differ by at least a row height, so a tiny
	// epsilon is enough and never merges two real rows.
	const yTol = 1e-6
	// Pass 1: build bands from non-spanning cells only.
	for i := range cells {
		c := &cells[i]
		if c.X1 <= c.X0 || c.Y1 <= c.Y0 {
			continue // degenerate / span-covered cell: not a fill target
		}
		if strings.Contains(c.Label, "spanning") {
			continue
		}
		rb := &rowBand{}
		found := false
		for ri := range rows {
			if math.Abs(rows[ri].y0-c.Y0) <= yTol {
				rb = &rows[ri]
				found = true
				break
			}
		}
		if !found {
			rows = append(rows, rowBand{
				y0: c.Y0, y1: c.Y1, stripX0: c.X0, stripX1: c.X1,
			})
			rb = &rows[len(rows)-1]
		}
		if c.X0 < rb.stripX0 {
			rb.stripX0 = c.X0
		}
		if c.X1 > rb.stripX1 {
			rb.stripX1 = c.X1
		}
		if c.Y1 > rb.y1 {
			rb.y1 = c.Y1
		}
		rb.cells = append(rb.cells, i)
	}
	// Pass 2: fold spanning cells into the nearest band by Y0. GroupCells
	// extends a span cell's bbox across the covered region, so its Y0 differs
	// from its own row's other cells; a separate band would span several TSR
	// rows and the top-containment rule would swallow every covered box into
	// it, emptying the real rows (dell/screenshot 11→6). Python assigns boxes
	// to rows by TSR row component overlap only — spanning cells never define
	// a row. The span cell stays a fill target in that band (its covered
	// boxes' R/C labels still point at the span cell's column), but it must
	// NOT extend the band's Y/X geometry — its bbox already covers the span.
	for i := range cells {
		c := &cells[i]
		if c.X1 <= c.X0 || c.Y1 <= c.Y0 {
			continue
		}
		if !strings.Contains(c.Label, "spanning") {
			continue
		}
		best, bestD := -1, math.Inf(1)
		for ri := range rows {
			if d := math.Abs(rows[ri].y0 - c.Y0); d < bestD {
				best, bestD = ri, d
			}
		}
		if best < 0 {
			continue
		}
		rows[best].cells = append(rows[best].cells, i)
	}
	// Stable ordering: rows top-to-bottom, cells left-to-right (matches
	// Python's first-wins tie-breaking in find_overlapped_with_threshold /
	// find_horizontally_tightest_fit).
	sort.Slice(rows, func(i, j int) bool { return rows[i].y0 < rows[j].y0 })
	for ri := range rows {
		sort.Slice(rows[ri].cells, func(a, b int) bool {
			return cells[rows[ri].cells[a]].X0 < cells[rows[ri].cells[b]].X0
		})
	}
	// Replace each grid row's strip X with the matching TSR "table row"
	// component's own bbox X (see FillCellTextFromBoxesWithRows doc). Matching
	// is by Y band, not positional index, so the override applies to every
	// data row independently of whether the table has a header: header /
	// projrowheader components are simply absent from rowStrips and fall back
	// to the grid-union strip X, which is correct for header rows. This
	// replaces the former all-or-nothing `len(rowStrips) == len(rows)` guard,
	// which silently disabled the override for any table that had a header.
	for ri := range rows {
		if sx0, sx1, ok := matchRowStrip(rowStrips, rows[ri].y0); ok {
			rows[ri].stripX0 = sx0
			rows[ri].stripX1 = sx1
		}
	}

	// Accumulate box text per target cell so multiple boxes in one cell join.
	cellText := make([]string, len(cells))
	cellFilled := make([]bool, len(cells))
	matched := 0

	for bi := range boxes {
		b := boxes[bi]
		if IsCaptionBox(b.Text, b.LayoutType) {
			continue
		}
		boxArea := util.Area(&b)
		if boxArea <= 0 {
			continue
		}
		// 1. Best row: prefer the row the box BEGINS in.
		//
		// Python's construct_table groups boxes into rows by each box's TSR
		// row label (b["R"]) — the row the box STARTS in — not by spatial
		// overlap (pdf_parser.py construct_table:176-192). When a label or
		// value box spans two adjacent data rows (e.g. a Month cell merged
		// across a pair of rows, or a Margin cell repeated across a seam),
		// Go must place it in the UPPER row to match Python. Without this,
		// FillCellTextFromBoxes picked the MAX-overlap row (usually the
		// lower one) and the doubled text landed one row too low, which is
		// the 13_crosspage_table.pdf divergence (tracked as go_bug
		// table-crosspage-merge-seam-duplication, whose root cause is this
		// row-selection rule, not the cross-page merge itself).
		//
		// We therefore prefer the topmost row band whose Y range CONTAINS the
		// box's TOP edge. Top-containment is a stronger, non-spurious signal
		// than the 0.3 area ratio (a box whose top sits inside a band clearly
		// belongs to that row even when its 2D overlap ratio is below 0.3, as
		// happens for a narrow Month box whose width gives a small area
		// ratio). Only when no band contains the top edge do we fall back to
		// the original max-overlap (ov, ov2) rule, so single-row boxes and
		// genuinely lower-row boxes are unchanged.
		topR := -1
		for ri := range rows {
			rb := &rows[ri]
			if b.Top >= rb.y0 && b.Top <= rb.y1 {
				topR = ri
				break
			}
		}
		bestR := -1
		bestOv, bestOv2 := 0.3, 0.0
		for ri := range rows {
			rb := &rows[ri]
			if math.Min(b.Bottom, rb.y1)-math.Max(b.Top, rb.y0) <= 0 {
				continue // no vertical overlap
			}
			strip := pdf.TSRCell{X0: rb.stripX0, Y0: rb.y0, X1: rb.stripX1, Y1: rb.y1}
			inter := util.OverlapInter(&strip, &b)
			ov := inter / boxArea
			if ov < 0.3 {
				continue
			}
			ov2 := 0.0
			if a := util.Area(&strip); a > 0 {
				ov2 = inter / a
			}
			// Fallback: keep the max-overlap (ov, ov2) best, mirroring
			// Python's find_overlapped_with_threshold tuple ordering.
			if ov > bestOv || (ov == bestOv && ov2 > bestOv2) {
				bestR, bestOv, bestOv2 = ri, ov, ov2
			}
		}
		if topR >= 0 {
			bestR = topR
		}
		if bestR < 0 {
			continue
		}
		// 2. Tightest column within the matched row (no threshold).
		rb := &rows[bestR]
		bestC := -1
		bestDis := 1e9
		for _, ci := range rb.cells {
			c := &cells[ci]
			if math.Min(b.Bottom, c.Y1)-math.Max(b.Top, c.Y0) <= 0 {
				continue
			}
			if dis := tightestColumnDistance(&b, c); dis < bestDis {
				bestDis, bestC = dis, ci
			}
		}
		if bestC < 0 {
			continue
		}
		// 3. Assign, preserving the 0.85 guard for pre-filled cells only.
		target := &cells[bestC]
		if target.Text != "" && !BoxMatchesCell(*target, b, false) {
			continue
		}
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		if cellFilled[bestC] {
			cellText[bestC] += " " + t
		} else {
			cellText[bestC] = t
			cellFilled[bestC] = true
		}
		matched++
	}

	for i := range cells {
		if cellFilled[i] {
			cells[i].Text = cellText[i]
		}
	}
	slog.Debug("fillCellTextFromBoxes done", "box_cell_matches", matched, "cells_filled", matched)
}

// tightestColumnDistance mirrors Python's find_horizontally_tightest_fit
// distance metric: the minimum of the left-edge gap, right-edge gap, and
// half the center gap. Smaller means the box sits tighter against the cell.
func tightestColumnDistance(b *pdf.TextBox, c *pdf.TSRCell) float64 {
	dis := math.Min(math.Abs(b.X0-c.X0), math.Abs(b.X1-c.X1))
	if center := math.Abs((b.X0+b.X1)-(c.X0+c.X1)) / 2; center < dis {
		dis = center
	}
	return dis
}

// BoxMatchesCell reports whether a text box's text may be assigned to a
// TSR cell. The threshold is two-stage:
//   - empty cell:        inter/boxArea >= 0.3  — matches Python's
//     find_overlapped_with_threshold default (thr=0.3), which fills cells from
//     overlapping PDF boxes uniformly.
//   - cell already has text: inter/boxArea >= 0.85 — Go-only guard, NOT in
//     Python. In the rotated-table path (table_extract.go) ocrTableCells
//     pre-fills cells with per-cell OCR text; the 0.85 bar stops a
//     weakly-overlapping detected box from corrupting/overriding that OCR
//     result. Python has no per-cell OCR at this stage, so it never raises the
//     threshold. This is a deliberate go_intentional divergence: it can drop a
//     legitimate secondary text fragment (a box overlapping 30-85%) that
//     Python would keep.
//
// FillCellTextFromBoxes uses only the 0.85 branch (cellIsEmpty=false) as the
// guard for PRE-FILLED cells; for empty cells it relies on the row/column
// selection (which already enforces >= 0.3 vertical overlap, matching Python).
// BoxMatchesCell remains the canonical "does this box match this exact cell"
// primitive and is directly unit-tested.
func BoxMatchesCell(cell pdf.TSRCell, box pdf.TextBox, cellIsEmpty bool) bool {
	inter := util.OverlapInter(&cell, &box)
	boxArea := util.Area(&box)
	if boxArea <= 0 {
		return false
	}
	if cellIsEmpty {
		return inter/boxArea >= 0.3 // Python's find_overlapped_with_threshold default
	}
	return inter/boxArea >= 0.85
}

// matchRowStrip finds the TSR "table row" component whose Y0 matches the grid
// row band y0 within yTol, returning its X range. A header / projrowheader
// component (not collected into rowStrips by the caller) returns ok=false, so
// that row keeps the grid-union strip X — correct for header rows. Matching by
// Y (not positional index) lets the strip-X override apply to every data row
// even when the table also has a header, instead of the former
// all-or-nothing `len(rowStrips) == len(rows)` guard that silently disabled
// the override for any header-bearing table.
func matchRowStrip(rowStrips []pdf.TSRCell, y0 float64) (float64, float64, bool) {
	const yTol = 1e-6
	for _, rs := range rowStrips {
		if math.Abs(rs.Y0-y0) <= yTol {
			return rs.X0, rs.X1, true
		}
	}
	return 0, 0, false
}

// isCaptionBox checks if a text box is a table/figure caption,
// matching Python is_caption().  Captions should not enter table cells.
// reCaption backs IsCaptionBox and must mirror Python's start-anchored
// is_caption: only a line BEGINNING with a caption marker counts. The English
// alternatives are start-anchored with ^ for the same reason as
// reTableCaptionText/reFigureCaptionText.
var reCaption = regexp.MustCompile(`^[图表]+[ 0-9:：]{2,}|(?i)^Fig\.?\s*\d+|(?i)^Figure\s+\d+|(?i)^Table\s+\d+`)

func IsCaptionBox(text string, layoutType string) bool {
	if strings.Contains(layoutType, "caption") {
		return true
	}
	return reCaption.MatchString(strings.TrimSpace(text))
}

// reTableCaptionText matches text patterns that indicate a table caption
// (as opposed to a figure caption). Python is_caption uses re.match, which is
// start-anchored, so a body paragraph that merely MENTIONS "Table N"
// mid-sentence is NOT a caption. The English alternatives below are therefore
// start-anchored with ^: an unanchored alternative wrongly classifies such a
// paragraph as a caption and MergeCaptions then drops it (go_bug
// table-text-interleaved-paragraph-dropped).
var reTableCaptionText = regexp.MustCompile(`^表|(?i)^Table\s+\d+`)

// reFigureCaptionText matches text patterns that indicate a figure caption.
var reFigureCaptionText = regexp.MustCompile(`^图|(?i)^Fig\.?\s*\d+|(?i)^Figure\s+\d+`)

// captionKind returns "table" if the section is a table caption,
// "figure" if a figure caption, or "" if not a caption.
// Matches Python's is_caption check: text patterns OR layout_type containing "caption".
func CaptionKind(s pdf.Section) string {
	lt := s.LayoutType
	if lt == pdf.DLALabelTableCaption || (strings.Contains(lt, "caption") && reTableCaptionText.MatchString(strings.TrimSpace(s.Text))) {
		return pdf.LayoutTypeTable
	}
	if lt == pdf.DLALabelFigureCaption || strings.Contains(lt, "caption") {
		return pdf.LayoutTypeFigure
	}
	// DLA may label captions as "text" or other types — check text patterns.
	t := strings.TrimSpace(s.Text)
	if reTableCaptionText.MatchString(t) {
		return pdf.LayoutTypeTable
	}
	if reFigureCaptionText.MatchString(t) {
		return pdf.LayoutTypeFigure
	}
	// The chart/figure pattern is ambiguous (matches both) — fall back to isCaptionBox.
	if IsCaptionBox(t, "") {
		return pdf.LayoutTypeTable
	}
	return ""
}

// ── blockType: cell content classification (Python: TableStructureRecognizer.blockType) ──

// Compiled once at package init.
var blockTypePatterns = []struct {
	re   *regexp.Regexp
	kind string
}{
	// Dt (date) patterns — Python blockType lines 161-168.
	{regexp.MustCompile(`^(20|19)[0-9]{2}[年/-][0-9]{1,2}[月/-][0-9]{1,2}日*$`), "Dt"},
	{regexp.MustCompile(`^(20|19)[0-9]{2}年$`), "Dt"},
	{regexp.MustCompile(`^(20|19)[0-9]{2}[年-][0-9]{1,2}月*$`), "Dt"},
	{regexp.MustCompile(`^[0-9]{1,2}[月-][0-9]{1,2}日*$`), "Dt"},
	{regexp.MustCompile(`^第*[一二三四1-4]季度$`), "Dt"},
	{regexp.MustCompile(`^(20|19)[0-9]{2}年*[一二三四1-4]季度$`), "Dt"},
	{regexp.MustCompile(`^(20|19)[0-9]{2}[ABCDE]$`), "Dt"},
	// Nu (numeric) — Python blockType line 169.
	{regexp.MustCompile(`^[0-9.,+%/ -]+$`), "Nu"},
	// Ca (categorical) — Python blockType line 170.
	{regexp.MustCompile(`^[0-9A-Z/\._~-]+$`), "Ca"},
	// En (English) — Python blockType line 171.
	{regexp.MustCompile(`^[A-Z]*[a-z' -]+$`), "En"},
	// NE (named entity — mixed alphanumeric) — Python blockType line 172.
	{regexp.MustCompile(`^[0-9.,+-]+[0-9A-Za-z/$￥%<>（）()' -]+$`), "NE"},
	// Sg (single character) — Python blockType line 173.
	{regexp.MustCompile(`^.{1}$`), "Sg"},
}

// blockType classifies cell text into one of 9+1 types, matching Python's
// TableStructureRecognizer.blockType.  Types: Dt (date), Nu (numeric),
// Ca (categorical), En (English), NE (named entity), Sg (single char),
// Tx (short text), Lx (long text), Nr (person name), Ot (other).
func BlockType(text string) string {
	t := strings.TrimSpace(text)
	for _, p := range blockTypePatterns {
		if p.re.MatchString(t) {
			return p.kind
		}
	}
	// Token-based classification: >3 tokens, <12 → Tx, >=12 → Lx.
	// Uses simple token counting (whitespace split + individual CJK chars).
	tkn := simpleTokenCount(t)
	if tkn > 3 {
		if tkn < 12 {
			return "Tx"
		}
		return "Lx"
	}
	// Single token with POS tag "nr" → "Nr" (requires tokenizer — not available).
	// Default: "Ot" (other).
	return "Ot"
}

// simpleTokenCount estimates token count: splits on whitespace and counts
// CJK characters individually (each CJK char ≈ one token in Chinese).
func simpleTokenCount(text string) int {
	count := 0
	for _, r := range text {
		if pdf.IsCJK(r) {
			count++
		} else if r == ' ' || r == '\t' {
			// whitespace tokenizes boundaries already counted via words
		}
	}
	// Also count space-separated words.
	words := strings.FieldsSeq(text)
	for w := range words {
		if !containsCJK(w) {
			count++
		}
	}
	return count
}

func containsCJK(s string) bool {
	for _, r := range s {
		if pdf.IsCJK(r) {
			return true
		}
	}
	return false
}

// HeaderSetWithBlockType returns the set of rows that are header rows, matching
// Python's construct_table header detection (table_structure_recognizer.py:336-348).
// Python scores every row independently (no early stop) and, per column, counts
// the cell toward the header when it has a geometric H OR (for numeric-dominant
// tables) the cell is non-numeric; a numeric cell in a numeric-dominant table is
// skipped entirely (it neither helps nor hurts).
//
// Go folds Python's geometric (box.H>0) and blockType signals into ONE per-cell
// pass with that exact predicate, so a row is a header when more than half of its
// columns (including skipped numeric ones, matching Python's h/cnt) satisfy it.
// box.H is set by AnnotateTableBoxes against the header-labeled cells (Python
// t_recognizer.py: gather(r".*header$")), not grid[0].
//
// A THIRD, additive signal is Go-only: a TSR cell whose Label contains "header"
// (Python has no exact equivalent) also promotes a row via the same >0.5
// row-majority. A row is a header if ANY signal flags it.
//
// boxes may be nil (e.g. the test-only cell-grouping path); the geometric signal
// is then skipped and only blockType + label are consulted.
func HeaderSetWithBlockType(rows [][]pdf.TSRCell, boxes []pdf.TextBox) map[int]bool {
	// Compute dominant block type across all cells (Python: max_type, derived
	// from box btype with first-seen tie-breaking). Track first-seen order so
	// the tie rule matches Python's Counter+max (first max wins on ties).
	typeCounts := make(map[string]int)
	order := []string{}
	seen := make(map[string]bool)
	for _, row := range rows {
		for _, cell := range row {
			if t := strings.TrimSpace(cell.Text); t != "" {
				bt := BlockType(t)
				if !seen[bt] {
					seen[bt] = true
					order = append(order, bt)
				}
				typeCounts[bt]++
			}
		}
	}
	maxType := ""
	maxCount := -1
	for _, t := range order {
		if typeCounts[t] > maxCount {
			maxType, maxCount = t, typeCounts[t]
		}
	}

	// Geometric H per cell, from boxes overlapping the header region (box.H > 0).
	// colHit[row][col] = true when that column's cell has such a box.
	//
	// The boxes' R/C may have been assigned against the PRE-cleanup grid while
	// `rows` here is POST-cleanup (CleanupOrphanColumns/Rows in ConstructTable
	// removes empty rows/columns). So we re-derive (row, column) from geometry
	// instead of trusting the stale R/C — matching Python, which keys the
	// geometric H off the box itself and never re-indexes by a stale coordinate.
	colHit := make(map[int]map[int]bool)
	for i := range boxes {
		b := boxes[i]
		if b.H <= 0 {
			continue
		}
		bestRi, bestCi, bestOv := -1, -1, 0.0
		for ri, row := range rows {
			for ci, cell := range row {
				if tsrBoxOverlap(b, cell) {
					continue // no overlap with this cell
				}
				if ov := util.OverlapInter(&b, &cell); ov > bestOv {
					bestOv, bestRi, bestCi = ov, ri, ci
				}
			}
		}
		if bestRi >= 0 {
			// Geometry resolved the cell against the (post-cleanup) rows.
			if colHit[bestRi] == nil {
				colHit[bestRi] = make(map[int]bool)
			}
			colHit[bestRi][bestCi] = true
		} else if b.R >= 0 && b.R < len(rows) && b.C >= 0 && b.C < len(rows[b.R]) {
			// No geometric overlap (e.g. boxes supplied without coordinates):
			// fall back to the box's own R/C when it is in range, so callers
			// that pre-assigned a correct R/C still work.
			if colHit[b.R] == nil {
				colHit[b.R] = make(map[int]bool)
			}
			colHit[b.R][b.C] = true
		}
	}

	hdrs := make(map[int]bool)

	// Signals 1+2 folded (Python: construct_table, 336-348). Numeric-dominant
	// table: a numeric cell is skipped (continue); otherwise the cell counts when
	// it has H OR is non-numeric. Non-numeric table: the predicate reduces to
	// any(H) — the geometric signal alone.
	if maxType == "Nu" {
		for ri, row := range rows {
			cnt, h := 0, 0
			for ci, cell := range row {
				t := strings.TrimSpace(cell.Text)
				if t == "" {
					continue
				}
				cnt++
				bt := BlockType(t)
				if bt == "Nu" {
					continue // numeric cell in a numeric table: ignored
				}
				if colHit[ri][ci] || bt != "Nu" {
					h++
				}
			}
			if cnt > 0 && float64(h)/float64(cnt) > 0.5 {
				hdrs[ri] = true
			}
		}
	} else {
		for ri, row := range rows {
			cnt, h := 0, 0
			for ci, cell := range row {
				if strings.TrimSpace(cell.Text) == "" {
					continue
				}
				cnt++
				if colHit[ri][ci] {
					h++
				}
			}
			if cnt > 0 && float64(h)/float64(cnt) > 0.5 {
				hdrs[ri] = true
			}
		}
	}

	// Signal 3: TSR label "header" (additive Go-only fallback; Python has no
	// exact equivalent), with the same >0.5 row-majority.
	for ri, row := range rows {
		cnt, h := 0, 0
		for _, cell := range row {
			t := strings.TrimSpace(cell.Text)
			if t == "" {
				continue
			}
			cnt++
			if isHeaderLabel(cell.Label) {
				h++
			}
		}
		if cnt > 0 && float64(h)/float64(cnt) > 0.5 {
			hdrs[ri] = true
		}
	}

	return hdrs
}
