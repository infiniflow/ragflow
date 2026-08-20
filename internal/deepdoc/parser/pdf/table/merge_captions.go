package table

import (
	"html"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func MergeCaptions(sections []pdf.Section, figures []pdf.Section) []pdf.Section {
	captions := make([]int, 0, 4)
	for i, s := range sections {
		captionType := CaptionKind(s)
		if captionType == "" {
			continue
		}
		target := findNearestParent(i, s, sections, figures, captionType)
		if target >= 0 {
			// Emit the caption inside the target table's HTML as a <caption>
			// element (matching Python's __html_table) and drop the standalone
			// caption section. Retaining the caption text closes the previous
			// content-loss go_bug table-html-emission-format; it is NOT a
			// table-assembly change (cell content/structure are untouched).
			injectCaption(&sections[target], s.Text)
			captions = append(captions, i)
			continue
		}
		// No merge target. A FIGURE caption is kept as its own section: a pure
		// image figure has no text section (BoxesToSections skips empty figure
		// boxes), so removing it would drop caption text that Python keeps
		// (07_mixed_content 'Figure 1/2'). A TABLE caption without a table
		// section is a DLA mislabel (e.g. rotate_270's rotated text labeled
		// "table") — keep the historical removal so rotated-page text is not
		// duplicated.
		if captionType != pdf.LayoutTypeFigure {
			captions = append(captions, i)
		}
	}
	// Remove caption sections in reverse order.
	n := len(sections)
	out := make([]pdf.Section, 0, n-len(captions))
	capSet := make(map[int]bool, len(captions))
	for _, idx := range captions {
		capSet[idx] = true
	}
	for i, s := range sections {
		if !capSet[i] {
			out = append(out, s)
		}
	}
	return out
}

// findNearestParent finds the nearest figure (for figure caption) or
// table (for table caption) section by position proximity.
// captionType is "table" or "figure" (from captionKind).
// Returns the index in `sections` (for tables) or a virtual index mapping
// to `figures` (negative offset for figures).
func findNearestParent(captionIdx int, caption pdf.Section, sections []pdf.Section, figures []pdf.Section, captionType string) int {
	find := func(targets []pdf.Section, skipIdx int) (int, float64) {
		bestIdx := -1
		bestDist := 1e9
		for i, t := range targets {
			if i == skipIdx {
				continue // don't match caption to itself
			}
			if len(t.Positions) == 0 || len(caption.Positions) == 0 {
				continue
			}
			tp := t.Positions[0]
			cp := caption.Positions[0]
			// Squared Euclidean distance (Python _extract_table_figure:1196).
			// Caption is typically below. Use center-point distance.
			cx := (tp.Left + tp.Right) / 2
			cy := (tp.Top + tp.Bottom) / 2
			ccx := (cp.Left + cp.Right) / 2
			ccy := (cp.Top + cp.Bottom) / 2
			dist := (cx-ccx)*(cx-ccx) + (cy-ccy)*(cy-ccy)
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		return bestIdx, bestDist
	}

	const maxCaptionGap = 40000.0 // PDF points (~7cm) — beyond this, don't attach.
	if captionType == pdf.LayoutTypeFigure && len(figures) > 0 {
		idx, dist := find(figures, -1) // figures don't contain the caption itself
		if idx >= 0 && dist < maxCaptionGap {
			// Match by position coordinates, not PositionTag strings.
			f := figures[idx]
			for i, s := range sections {
				if s.LayoutType != pdf.LayoutTypeFigure || len(s.Positions) == 0 || len(f.Positions) == 0 {
					continue
				}
				sp, fp := s.Positions[0], f.Positions[0]
				if sp.Left == fp.Left && sp.Right == fp.Right &&
					sp.Top == fp.Top && sp.Bottom == fp.Bottom {
					return i
				}
			}
		}
	}
	if captionType == pdf.LayoutTypeTable {
		idx, dist := findTables(sections, caption)
		if idx >= 0 && dist < maxCaptionGap {
			return idx
		}
	}
	return -1
}

// findTables returns the nearest section whose LayoutType is table, by
// center-point distance from the caption. Restricting to table sections (not
// all sections) prevents a caption sitting in the left margin — far from the
// table's horizontal center — from matching a nearer non-table section (e.g.
// another caption) and being wrongly dropped.
func findTables(sections []pdf.Section, caption pdf.Section) (int, float64) {
	bestIdx := -1
	bestDist := 1e9
	if len(caption.Positions) == 0 {
		return bestIdx, bestDist
	}
	cp := caption.Positions[0]
	ccx := (cp.Left + cp.Right) / 2
	ccy := (cp.Top + cp.Bottom) / 2
	for i, t := range sections {
		if t.LayoutType != pdf.LayoutTypeTable || len(t.Positions) == 0 {
			continue
		}
		tp := t.Positions[0]
		cx := (tp.Left + tp.Right) / 2
		cy := (tp.Top + tp.Bottom) / 2
		dist := (cx-ccx)*(cx-ccx) + (cy-ccy)*(cy-ccy)
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	return bestIdx, bestDist
}

// injectCaption inserts a <caption> element immediately after the table's
// opening <table> tag (matching Python's __html_table), retaining the caption
// text inside the table HTML instead of losing it as a dropped standalone
// section. If the table has no <table> tag it is prepended so the text is at
// least preserved.
func injectCaption(table *pdf.Section, caption string) {
	text := strings.TrimSpace(caption)
	if text == "" {
		return
	}
	escaped := "<caption>" + html.EscapeString(text) + "</caption>"
	if table.Text == "" {
		table.Text = escaped
		return
	}
	const open = "<table>"
	if idx := strings.Index(table.Text, open); idx >= 0 {
		at := idx + len(open)
		table.Text = table.Text[:at] + escaped + table.Text[at:]
		return
	}
	table.Text = escaped + table.Text
}
