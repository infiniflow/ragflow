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

func FillCellTextFromBoxes(cells []pdf.TSRCell, boxes []pdf.TextBox) {
	slog.Debug("fillCellTextFromBoxes", "cells", len(cells), "boxes", len(boxes))
	if len(cells) > 0 && len(boxes) > 0 {
		c0 := cells[0]
		slog.Debug("fillCellTextFromBoxes cell[0]", "x0", c0.X0, "y0", c0.Y0, "x1", c0.X1, "y1", c0.Y1)
		b0 := boxes[0]
		slog.Debug("fillCellTextFromBoxes box[0]", "x0", b0.X0, "y0", b0.Top, "x1", b0.X1, "y1", b0.Bottom, "text_len", len(b0.Text))
	}
	matched, filled := 0, 0
	for ci := range cells {
		var matches []string
		for _, b := range boxes {
			if IsCaptionBox(b.Text, b.LayoutType) {
				continue
			}
			if BoxMatchesCell(cells[ci], b, cells[ci].Text == "") {
				matched++
				t := strings.TrimSpace(b.Text)
				if t != "" {
					matches = append(matches, t)
				}
			}
		}
		if len(matches) > 0 {
			cells[ci].Text = strings.Join(matches, " ")
			filled++
		}
	}
	slog.Debug("fillCellTextFromBoxes done", "cell_box_matches", matched, "cells_filled", filled)
}

// boxMatchesCell reports whether a text box's text should be assigned
// to a TSR cell.  When the cell already has text (from TSR), the box
// must be mostly inside the cell (≥85% of box area).  When the cell
// is empty, any overlap suffices — matching Python's _table_transformer_job
// which fills cells from overlapping PDF boxes with thr=0.3.
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
	// "图表" pattern could be either — check if isCaptionBox matches.
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
	words := strings.Fields(text)
	for _, w := range words {
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

// headerSetWithBlockType returns rows that should be header rows, using both
// TSR cell labels AND block-type classification.  Matches Python's
// construct_table header detection (table_structure_recognizer.py:370-384).
func HeaderSetWithBlockType(rows [][]pdf.TSRCell) map[int]bool {
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
	for ri, row := range rows {
		cnt, h := 0, 0
		for _, cell := range row {
			t := strings.TrimSpace(cell.Text)
			if t == "" {
				continue
			}
			cnt++
			bt := BlockType(t)
			// Python: if max_type == "Nu" and cell btype == "Nu" → skip
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
	// Fallback: if block-type found no headers, check for model-agnostic
	// "header" substring in cell labels (works across different TSR models).
	if len(hdrs) == 0 {
		for ri, row := range rows {
			for _, cell := range row {
				if strings.Contains(cell.Label, "header") || strings.Contains(cell.Label, "Header") {
					hdrs[ri] = true
					break
				}
			}
		}
	}
	return hdrs
}
