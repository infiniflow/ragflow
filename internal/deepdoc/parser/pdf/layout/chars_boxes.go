package layout

import (
	"math"
	"regexp"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

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
// asciiWordPattern matches strings composed entirely of ASCII word
// characters. Python uses re.match (prefix match) — the stricter
// full-string match here is equivalent in practice because each
// pdf.TextChar.Text is a single rune, so prevText+currText ≤ 2 chars.
// Python: pdf_parser.py:1528 re.match(r"[0-9a-zA-Z,.:;!%]+", ...)
var asciiWordPattern = regexp.MustCompile(`^[0-9a-zA-Z,.:;!%]+$`)

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
	var textParts []string
	for i, c := range chars {
		// Insert space between adjacent ASCII words with a visible gap.
		// Python: pdf_parser.py:1524-1532 __img_ocr space insertion.
		if i > 0 {
			prev := chars[i-1]
			prevText := strings.TrimSpace(prev.Text)
			currText := strings.TrimSpace(c.Text)
			if prevText != "" && currText != "" {
				gap := c.X0 - prev.X1
				minWidth := math.Min(c.X1-c.X0, prev.X1-prev.X0)
				if gap >= minWidth/2 &&
					asciiWordPattern.MatchString(prevText+currText) {
					textParts = append(textParts, " ")
				}
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
