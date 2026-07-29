package layout

import (
	"math"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// hasCJK reports whether s contains a CJK rune. CJK scripts do not separate
// words with spaces, so geometric gaps between their glyphs must not become one.
func hasCJK(s string) bool {
	for _, r := range s {
		if pdf.IsCJK(r) {
			return true
		}
	}
	return false
}

// CharsToBoxes converts raw characters to initial text boxes by grouping
// characters into lines based on vertical overlap.
//
// Python: pdf_parser.__images__ producing self.boxes
func CharsToBoxes(chars []pdf.TextChar, pageNum int, sortByTop bool) []pdf.TextBox {
	if len(chars) == 0 {
		return nil
	}

	lines := GroupCharsToLines(chars, sortByTop)

	// Page-level column gap threshold from ALL inter-char gaps.
	// Falls back to per-line threshold when page has too few gaps.
	threshold := pageXGapThreshold(lines)

	boxes := make([]pdf.TextBox, 0, len(lines))
	for _, line := range lines {
		thr := threshold
		if thr > 100 {
			// No significant column gaps on this page → use per-line threshold.
			thr = perLineXGapThreshold(line)
		}
		subLines := splitLineByXGap(line, thr)
		for _, sub := range subLines {
			box := LineToTextBox(sub)
			box.PageNumber = pageNum
			boxes = append(boxes, box)
		}
	}
	return boxes
}

// perLineXGapThreshold computes a dynamic X-gap threshold for column
// splitting within a single line (fallback when page has few gaps).
func perLineXGapThreshold(chars []pdf.TextChar) float64 {
	if len(chars) <= 1 {
		return 1e9
	}
	var gaps []float64
	for i := 1; i < len(chars); i++ {
		g := chars[i].X0 - chars[i-1].X1
		gaps = append(gaps, g)
	}
	if len(gaps) == 0 {
		return 1e9
	}
	sort.Float64s(gaps)
	medianGap := gaps[len(gaps)/2]
	if medianGap < 6 {
		medianGap = 6
	}
	return medianGap * 2.5
}

// pageXGapThreshold computes a global X-gap column threshold from all
// inter-char gaps across all lines on the page.  95th percentile catches
// column boundaries while excluding word-level gaps.
// Returns a value > 100 when there are too few gaps for reliable p95,
// signalling the caller to fall back to perLineXGapThreshold.
func pageXGapThreshold(lines [][]pdf.TextChar) float64 {
	var allGaps []float64
	for _, line := range lines {
		for i := 1; i < len(line); i++ {
			g := line[i].X0 - line[i-1].X1
			allGaps = append(allGaps, g)
		}
	}
	if len(allGaps) < 10 {
		return 1e9 // too few gaps for reliable p95 → fall back to per-line
	}
	sort.Float64s(allGaps)
	// 95th percentile: only the largest 5% of gaps are column boundaries.
	p95 := allGaps[len(allGaps)*95/100]
	if p95 < 30 {
		p95 = 30 // floor: column gaps are ≥30pt in practice
	}
	return p95
}

// splitLineByXGap splits a character line into sub-lines where X gaps
// meet or exceed the threshold (column boundaries).  Uses >= to match the
// p95 boundary value — a gap exactly at the 95th percentile is a column gap,
// not a word gap.
func splitLineByXGap(chars []pdf.TextChar, threshold float64) [][]pdf.TextChar {
	if len(chars) <= 1 {
		return [][]pdf.TextChar{chars}
	}
	var result [][]pdf.TextChar
	start := 0
	for i := 1; i < len(chars); i++ {
		gap := chars[i].X0 - chars[i-1].X1
		if gap >= threshold {
			result = append(result, chars[start:i])
			start = i
		}
	}
	result = append(result, chars[start:])
	return result
}

// ---- internal helpers ----

// groupCharsToLines groups characters into horizontal lines based on vertical overlap.
func GroupCharsToLines(chars []pdf.TextChar, sortByTop bool) [][]pdf.TextChar {
	if len(chars) == 0 {
		return nil
	}

	key := func(c pdf.TextChar) float64 { return c.Bottom }
	if sortByTop {
		key = func(c pdf.TextChar) float64 { return c.Top }
	}

	// Sort by vertical key (Bottom or Top) then x0 using sort.SliceStable.
	// Guard against NaN: a NaN key sorts after everything else.
	sort.SliceStable(chars, func(i, j int) bool {
		ki, kj := key(chars[i]), key(chars[j])
		if ki != kj && !math.IsNaN(ki) && !math.IsNaN(kj) {
			return ki < kj
		}
		if math.IsNaN(ki) != math.IsNaN(kj) {
			return !math.IsNaN(ki) // non-NaN before NaN
		}
		return chars[i].X0 < chars[j].X0
	})

	var lines [][]pdf.TextChar
	var currentLine []pdf.TextChar

	for _, c := range chars {
		if len(currentLine) == 0 {
			currentLine = append(currentLine, c)
			continue
		}
		if verticalOverlap(currentLine[len(currentLine)-1], c) {
			currentLine = append(currentLine, c)
		} else {
			if len(currentLine) > 0 {
				lines = append(lines, currentLine)
			}
			currentLine = []pdf.TextChar{c}
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}
	return lines
}

// verticalOverlap checks if two characters are on the same horizontal line.
func verticalOverlap(a, b pdf.TextChar) bool {
	mh := math.Max(util.CharHeight(a), util.CharHeight(b))
	if mh <= 0 {
		mh = 1.0
	}
	return math.Abs(a.Top-b.Top) < mh*0.5
}

// lineToTextBox converts a line of characters to a single pdf.TextBox.

func LineToTextBox(chars []pdf.TextChar) pdf.TextBox {
	if len(chars) == 0 {
		return pdf.TextBox{}
	}
	box := pdf.TextBox{
		X0:     chars[0].X0,
		X1:     chars[0].X1,
		Top:    chars[0].Top,
		Bottom: chars[0].Bottom,
	}
	// Recover missing spaces from geometry: many PDFs encode no space glyphs and
	// separate words by positioning alone. For a script that separates words with
	// spaces, a gap wider than a fraction of the mean char width is a word
	// boundary; intra-word kerns fall well below it. CJK is excluded: it does not
	// write inter-word spaces, so a gap between CJK glyphs is ordinary tracking,
	// not a boundary.
	var sumWidth float64
	var nWidth int
	for _, c := range chars {
		if strings.TrimSpace(c.Text) != "" {
			sumWidth += c.X1 - c.X0
			nWidth++
		}
	}
	var spaceGap float64
	if nWidth > 0 {
		spaceGap = (sumWidth / float64(nWidth)) * 0.25
	}
	var textParts []string
	for i, c := range chars {
		if i > 0 && spaceGap > 0 {
			prev := chars[i-1]
			if strings.TrimSpace(prev.Text) != "" && strings.TrimSpace(c.Text) != "" &&
				!hasCJK(prev.Text) && !hasCJK(c.Text) &&
				c.X0-prev.X1 > spaceGap {
				textParts = append(textParts, " ")
			}
		}
		box.X0 = math.Min(box.X0, c.X0)
		box.X1 = math.Max(box.X1, c.X1)
		box.Top = math.Min(box.Top, c.Top)
		box.Bottom = math.Max(box.Bottom, c.Bottom)
		textParts = append(textParts, c.Text)
		if c.LayoutType != "" {
			box.LayoutType = c.LayoutType
		}
		if c.LayoutNo != "" {
			box.LayoutNo = c.LayoutNo
		}
	}
	box.Text = strings.Join(textParts, "")
	return box
}
