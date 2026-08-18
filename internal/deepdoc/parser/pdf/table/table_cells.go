package table

import (
	"log/slog"
	"math"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
	"regexp"
	"sort"
	"strings"
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
	slog.Debug("fillCellTextFromBoxes", "cells", len(cells), "boxes", len(boxes))
	if len(cells) == 0 || len(boxes) == 0 {
		return
	}

	// Group cells into row bands by their top coordinate. The grid is a TSR
	// row×column cross-product, so every cell in a row shares the same Y band.
	// A row band spans the full table width (union of its cells), matching
	// Python's full-width "table row" components.
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
	for i := range cells {
		c := &cells[i]
		if c.X1 <= c.X0 || c.Y1 <= c.Y0 {
			continue // degenerate / span-covered cell: not a fill target
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
	// Stable ordering: rows top-to-bottom, cells left-to-right (matches
	// Python's first-wins tie-breaking in find_overlapped_with_threshold /
	// find_horizontally_tightest_fit).
	sort.Slice(rows, func(i, j int) bool { return rows[i].y0 < rows[j].y0 })
	for ri := range rows {
		sort.Slice(rows[ri].cells, func(a, b int) bool {
			return cells[rows[ri].cells[a]].X0 < cells[rows[ri].cells[b]].X0
		})
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
		// 1. Best row by vertical-overlap ratio (>= 0.3), tie-broken by _ov.
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
			ov2 := 0.0
			if a := util.Area(&strip); a > 0 {
				ov2 = inter / a
			}
			// Skip unless strictly better than the current best, mirroring
			// Python's (ov, _ov) tuple ordering in find_overlapped_with_threshold.
			if !(ov > bestOv || (ov == bestOv && ov2 > bestOv2)) {
				continue
			}
			bestR, bestOv, bestOv2 = ri, ov, ov2
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

// isCaptionBox checks if a text box is a table/figure caption,
// matching Python is_caption().  Captions should not enter table cells.
var reCaption = regexp.MustCompile(`^[图表]+[ 0-9:：]{2,}|(?i)Fig\.?\s*\d+|(?i)Figure\s+\d+|(?i)Table\s+\d+`)

func IsCaptionBox(text string, layoutType string) bool {
	if strings.Contains(layoutType, "caption") {
		return true
	}
	return reCaption.MatchString(strings.TrimSpace(text))
}

// reTableCaptionText matches text patterns that indicate a table caption
// (as opposed to a figure caption). Python is_caption uses the same set.
var reTableCaptionText = regexp.MustCompile(`^表|(?i)Table\s+\d+`)

// reFigureCaptionText matches text patterns that indicate a figure caption.
var reFigureCaptionText = regexp.MustCompile(`^图|(?i)Fig\.?\s*\d+|(?i)Figure\s+\d+`)

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

// HeaderSetWithBlockType returns the set of rows that are header rows, combining
// THREE additive signals to match Python's construct_table header detection
// (table_structure_recognizer.py:336-348):
//
//  1. Geometric: a box overlapping the header region by ≥0.3 has box.H>0
//     (AnnotateTableBoxes sets it against grid[0], the first grid row). A row is
//     a header when more than half of its columns have such a box — the same
//     column-majority Python applies (per-column any(a.get("H") for a in arr),
//     then per-row h/cnt > 0.5). Go hardcodes the header region to grid[0], so
//     this matches Python for the common header-on-row-0 case but is an
//     approximation, not a 1:1 port.
//  2. blockType (Python: max_type == "Nu" and btype != "Nu"): only contributes
//     for numeric-dominant tables, then per-row majority.
//  3. TSR label (Go-only model-agnostic fallback; Python has no exact
//     equivalent): a cell whose Label contains "header" counts, then per-row
//     majority.
//
// A row is a header if ANY signal flags it (no mutually-exclusive gate).
//
// Python divergence: Python stops at the first row that fails the majority (a
// contiguous header prefix); Go scores each row independently and adds signals,
// so a later row that independently clears the majority is still flagged.
//
// boxes may be nil (e.g. the test-only cell-grouping path); the geometric signal
// is then skipped and only blockType + label are consulted.
func HeaderSetWithBlockType(rows [][]pdf.TSRCell, boxes []pdf.TextBox) map[int]bool {
	// Compute dominant block type across all cells.
	typeCounts := make(map[string]int)
	for _, row := range rows {
		for _, cell := range row {
			t := strings.TrimSpace(cell.Text)
			if t != "" {
				typeCounts[BlockType(t)]++
			}
		}
	}
	maxType := ""
	maxCount := 0
	for t, c := range typeCounts {
		if c > maxCount {
			maxType = t
			maxCount = c
		}
	}

	hdrs := make(map[int]bool)

	// Signal 2: blockType (numeric-dominant tables only).
	for ri, row := range rows {
		cnt, h := 0, 0
		for _, cell := range row {
			t := strings.TrimSpace(cell.Text)
			if t == "" {
				continue
			}
			cnt++
			bt := BlockType(t)
			// Python: max_type == "Nu" and cell btype == "Nu" → skip
			if maxType == "Nu" && bt == "Nu" {
				continue
			}
			// Python: max_type == "Nu" and cell btype != "Nu" → header
			if maxType == "Nu" && bt != "Nu" {
				h++
			}
		}
		if cnt > 0 && float64(h)/float64(cnt) > 0.5 {
			hdrs[ri] = true
		}
	}

	// Signal 3: TSR label "header" (additive, with 0.5 majority — fixes
	// over-detection where a single mislabeled cell promoted a whole data row).
	for ri, row := range rows {
		cnt, h := 0, 0
		for _, cell := range row {
			t := strings.TrimSpace(cell.Text)
			if t == "" {
				continue
			}
			cnt++
			if strings.Contains(cell.Label, "header") || strings.Contains(cell.Label, "Header") {
				h++
			}
		}
		if cnt > 0 && float64(h)/float64(cnt) > 0.5 {
			hdrs[ri] = true
		}
	}

	// Signal 1: geometric H from boxes (additive, with the same column-majority
	// Python uses). For each row, count how many of its columns have at least one
	// box overlapping the header region (box.H > 0); mark the row when that
	// fraction exceeds 0.5. This mirrors Python's per-column
	// any(a.get("H") for a in arr) followed by its per-row h/cnt > 0.5 — distinct
	// from the old BoxHeaderSet any-box rule, which flagged a whole row whenever a
	// single stray box overlapped the header region (over-promotion).
	if len(boxes) > 0 {
		// colHit[row] = set of columns whose cell has an H>0 box.
		colHit := make(map[int]map[int]bool)
		for i := range boxes {
			b := boxes[i]
			if b.H <= 0 || b.R < 0 || b.R >= len(rows) {
				continue
			}
			if b.C < 0 || b.C >= len(rows[b.R]) {
				continue
			}
			if colHit[b.R] == nil {
				colHit[b.R] = make(map[int]bool)
			}
			colHit[b.R][b.C] = true
		}
		for ri, row := range rows {
			n := len(row)
			if n == 0 {
				continue
			}
			if float64(len(colHit[ri]))/float64(n) > 0.5 {
				hdrs[ri] = true
			}
		}
	}

	return hdrs
}
