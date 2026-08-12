package table

import (
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
			// For table sections, prepend caption before the HTML table
			// (matching Python's _extract_table_figure caption->construct_table).
			if sections[target].LayoutType == pdf.LayoutTypeTable && sections[target].Text != "" {
				sections[target].Text = s.Text + sections[target].Text
			} else if sections[target].Text != "" {
				sections[target].Text += " " + s.Text
			} else {
				sections[target].Text = s.Text
			}
		}
		captions = append(captions, i)
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
		idx, dist := find(sections, captionIdx)
		if idx >= 0 && dist < maxCaptionGap && sections[idx].LayoutType == pdf.LayoutTypeTable {
			return idx
		}
	}
	return -1
}
