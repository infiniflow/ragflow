package pdfparser

import (
	"math"
	"sort"
)

// CharWidth returns the average character width: (x1 - x0) / len(text).
// Returns 0 if text is empty.
//
// Python: pdf_parser.py:107 __char_width()
//
// Example:
//
//	c := TextChar{X0: 50, X1: 58, Text: "A"}
//	w := CharWidth(c)  // (58-50)/1 = 8
func CharWidth(c TextChar) float64 {
	if len(c.Text) == 0 {
		return 0
	}
	return (c.X1 - c.X0) / float64(len(c.Text))
}

// CharHeight returns the character height in PDF points.
//
// Python: pdf_parser.py:110 __height()
//
// Example:
//
//	c := TextChar{Top: 200, Bottom: 212}
//	h := CharHeight(c)  // 212-200 = 12
func CharHeight(c TextChar) float64 {
	return c.Bottom - c.Top
}

// XDis computes the minimum horizontal distance between two characters.
// Used to determine if they belong to the same text line.
//
// Python: pdf_parser.py:113 _x_dis()
//
// Example:
//
//	a := TextChar{X0: 50, X1: 58}
//	b := TextChar{X0: 60, X1: 68}
//	d := XDis(a, b)  // min(|58-60|=2, |50-68|=18, |108-128|/2=10) = 2
func XDis(a, b TextChar) float64 {
	return min(
		math.Abs(a.X1-b.X0),
		min(math.Abs(a.X0-b.X1), math.Abs(a.X0+a.X1-b.X0-b.X1)/2),
	)
}

// YDis computes the vertical distance between two characters' centerlines.
// Positive means b is below a.
//
// Python: pdf_parser.py:116 _y_dis()
//
// Example:
//
//	a := TextChar{Top: 100, Bottom: 112}
//	b := TextChar{Top: 114, Bottom: 126}
//	d := YDis(a, b)  // (114+126-100-112)/2 = 14
func YDis(a, b TextChar) float64 {
	return (b.Top + b.Bottom - a.Top - a.Bottom) / 2
}

// BoxWidth returns the width of a text box.
func BoxWidth(b TextBox) float64 {
	return b.X1 - b.X0
}

// BoxHeight returns the height of a text box.
func BoxHeight(b TextBox) float64 {
	return b.Bottom - b.Top
}

// BoxYDis computes vertical centerline distance between boxes.
// Positive means b2 is below b1.
func BoxYDis(b1, b2 TextBox) float64 {
	return (b2.Top + b2.Bottom - b1.Top - b1.Bottom) / 2
}

// BoxXDis computes horizontal distance between boxes.
func BoxXDis(b1, b2 TextBox) float64 {
	return min(
		math.Abs(b1.X1-b2.X0),
		min(math.Abs(b1.X0-b2.X1), math.Abs(b1.X0+b1.X1-b2.X0-b2.X1)/2),
	)
}

// TextBoxOverlapX returns the horizontal overlap ratio between two boxes.
// Returns 0.0-1.0 where 1.0 means fully overlapped.
//
// Python: pdf_parser.py:964-965 overlap calculation in _naive_vertical_merge
func TextBoxOverlapX(b1, b2 TextBox) float64 {
	overlap := math.Max(0, math.Min(b1.X1, b2.X1)-math.Max(b1.X0, b2.X0))
	// Python: max(1, min(width_a, width_b)) — denominator ≥1 prevents
	// artificially high ratios for very narrow boxes.
	minWidth := math.Max(1, math.Min(b1.X1-b1.X0, b2.X1-b2.X0))
	return overlap / minWidth
}

// SortXByPage sorts boxes by page_number, then x0, then top.
// After sorting, corrects for same-page boxes that have nearly the same x0
// but inverted top ordering (a layout artifact).
//
// Python: pdf_parser.py:178 sort_X_by_page()
func SortXByPage(boxes []TextBox, threshold float64) []TextBox {
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].PageNumber != boxes[j].PageNumber {
			return boxes[i].PageNumber < boxes[j].PageNumber
		}
		if boxes[i].X0 != boxes[j].X0 {
			return boxes[i].X0 < boxes[j].X0
		}
		return boxes[i].Top < boxes[j].Top
	})

	for i := len(boxes) - 1; i >= 1; i-- {
		for j := i - 1; j >= 0; j-- {
			if math.Abs(boxes[j+1].X0-boxes[j].X0) < threshold &&
				boxes[j+1].Top < boxes[j].Top &&
				boxes[j+1].PageNumber == boxes[j].PageNumber {
				boxes[j], boxes[j+1] = boxes[j+1], boxes[j]
			}
		}
	}
	return boxes
}

// MedianCharHeight computes the median character height for a page,
// matching Python's np.median(char height) in __images__ (pdf_parser.py:1552).
// Used as a reference unit for vertical spacing decisions.
func MedianCharHeight(chars []TextChar) float64 {
	if len(chars) == 0 {
		return 10
	}
	heights := make([]float64, len(chars))
	for i, c := range chars {
		heights[i] = CharHeight(c)
	}
	sort.Float64s(heights)
	n := len(heights)
	if n%2 == 0 {
		return (heights[n/2-1] + heights[n/2]) / 2
	}
	return heights[n/2]
}

// MedianCharWidth computes the median character width for a page,
// matching Python's np.median(char width) in __images__ (pdf_parser.py:1553).
func MedianCharWidth(chars []TextChar) float64 {
	if len(chars) == 0 {
		return 5
	}
	widths := make([]float64, len(chars))
	for i, c := range chars {
		widths[i] = CharWidth(c)
	}
	sort.Float64s(widths)
	n := len(widths)
	if n%2 == 0 {
		return (widths[n/2-1] + widths[n/2]) / 2
	}
	return widths[n/2]
}

// MedianHeight computes the median height of a set of text boxes.
// Falls back to 10 if list is empty.
//
// Python: np.median([b["bottom"]-b["top"] for b in bxs]) or 10
// in _naive_vertical_merge:941
func MedianHeight(boxes []TextBox) float64 {
	if len(boxes) == 0 {
		return 10
	}
	heights := make([]float64, len(boxes))
	for i, b := range boxes {
		heights[i] = b.Bottom - b.Top
	}
	sort.Float64s(heights)
	mid := len(heights) / 2
	if len(heights)%2 == 0 && mid > 0 {
		return (heights[mid-1] + heights[mid]) / 2
	}
	return heights[mid]
}
