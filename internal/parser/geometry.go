package parser

import (
	"image"
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

// ── Rectangular interface and overlap helpers ──────────────────────────

// Rectangular is any 2D axis-aligned rectangle that can report its bounds.
type Rectangular interface {
	Bounds() (x0, y0, x1, y1 float64)
}

// Area returns the area of a Rectangular. Returns 0 for degenerate rects.
func Area(r Rectangular) float64 {
	x0, y0, x1, y1 := r.Bounds()
	if x1 <= x0 || y1 <= y0 {
		return 0
	}
	return (x1 - x0) * (y1 - y0)
}

// rectOverlapInter returns the intersection area of two axis-aligned rectangles.
// Returns 0 when the rectangles do not overlap or either is degenerate.
func rectOverlapInter(x0a, y0a, x1a, y1a, x0b, y0b, x1b, y1b float64) float64 {
	x0 := max(x0a, x0b)
	y0 := max(y0a, y0b)
	x1 := min(x1a, x1b)
	y1 := min(y1a, y1b)
	if x0 >= x1 || y0 >= y1 {
		return 0
	}
	return (x1 - x0) * (y1 - y0)
}

// OverlapInter returns the raw intersection area of two rectangles.
func OverlapInter(a, b Rectangular) float64 {
	ax0, ay0, ax1, ay1 := a.Bounds()
	bx0, by0, bx1, by1 := b.Bounds()
	return rectOverlapInter(ax0, ay0, ax1, ay1, bx0, by0, bx1, by1)
}

// OverlapRatio returns intersection(a,b) / Area(denom).
// Returns 0 when denom has zero area or there is no intersection.
func OverlapRatio(a, b, denom Rectangular) float64 {
	inter := OverlapInter(a, b)
	if inter <= 0 {
		return 0
	}
	d := Area(denom)
	if d <= 0 {
		return 0
	}
	return inter / d
}

// OverlapRatioA returns intersection(a,b) / Area(a).
func OverlapRatioA(a, b Rectangular) float64 {
	return OverlapRatio(a, b, a)
}

// OverlapRatioMax returns intersection(a,b) / max(Area(a), Area(b)).
func OverlapRatioMax(a, b Rectangular) float64 {
	inter := OverlapInter(a, b)
	if inter <= 0 {
		return 0
	}
	d := max(Area(a), Area(b))
	if d <= 0 {
		return 0
	}
	return inter / d
}

// OverlapX returns the horizontal (X-axis only) overlap ratio between two rectangles.
// Ratio = overlap_width / max(1, min(width(a), width(b))).
//
// Python: pdf_parser.py:964-965 overlap calculation in _naive_vertical_merge
func OverlapX(a, b Rectangular) float64 {
	ax0, _, ax1, _ := a.Bounds()
	bx0, _, bx1, _ := b.Bounds()
	overlap := math.Max(0, math.Min(ax1, bx1)-math.Max(ax0, bx0))
	wA := ax1 - ax0
	wB := bx1 - bx0
	minWidth := math.Max(1, math.Min(wA, wB))
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
	heights := make([]float64, len(chars))
	for i, c := range chars {
		heights[i] = CharHeight(c)
	}
	return medianFloat64(heights, 10)
}

// MedianCharWidth computes the median character width for a page,
// matching Python's np.median(char width) in __images__ (pdf_parser.py:1553).
func MedianCharWidth(chars []TextChar) float64 {
	widths := make([]float64, len(chars))
	for i, c := range chars {
		widths[i] = CharWidth(c)
	}
	return medianFloat64(widths, 5)
}

// MedianHeight computes the median height of a set of text boxes.
// Falls back to 10 if list is empty.
//
// Python: np.median([b["bottom"]-b["top"] for b in bxs]) or 10
// in _naive_vertical_merge:941
func MedianHeight(boxes []TextBox) float64 {
	heights := make([]float64, len(boxes))
	for i, b := range boxes {
		heights[i] = b.Bottom - b.Top
	}
	return medianFloat64(heights, 10)
}

// medianFloat64 returns the median of vals, or fallback if empty.
func medianFloat64(vals []float64, fallback float64) float64 {
	if len(vals) == 0 {
		return fallback
	}
	sort.Float64s(vals)
	n := len(vals)
	if n%2 == 0 {
		return (vals[n/2-1] + vals[n/2]) / 2
	}
	return vals[n/2]
}

// rect is a lightweight rectangle for overlap calculations.
// Coordinates are in whatever space the caller uses (pixel or PDF points).
type rect struct{ x0, y0, x1, y1 float64 }

func (r rect) Bounds() (float64, float64, float64, float64) { return r.x0, r.y0, r.x1, r.y1 }

// rectOverlap returns the overlap ratio between two rects.
// Ratio = area(intersection) / max(area(a), area(b)).
// Returns 0 when there is no overlap.
func rectOverlap(a, b rect) float64 {
	return OverlapRatioMax(a, b)
}

// fastCrop copies a rectangular region from src to a new *image.RGBA.
// Uses direct Pix slice copy for *image.RGBA sources (zero allocation per row);
// falls back to pixel-by-pixel for other image types.
func fastCrop(src image.Image, x0, y0, x1, y1 int) *image.RGBA {
	// Clamp to source bounds
	b := src.Bounds()
	if x0 < b.Min.X { x0 = b.Min.X }
	if y0 < b.Min.Y { y0 = b.Min.Y }
	if x1 > b.Max.X { x1 = b.Max.X }
	if y1 > b.Max.Y { y1 = b.Max.Y }
	if x0 >= x1 || y0 >= y1 {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	w, h := x1-x0, y1-y0
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if rgba, ok := src.(*image.RGBA); ok {
		for y := y0; y < y1; y++ {
			srcRow := rgba.Pix[rgba.PixOffset(x0, y):rgba.PixOffset(x1, y)]
			dstRow := dst.Pix[dst.PixOffset(0, y-y0):]
			copy(dstRow, srcRow)
		}
		
	} else {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				dst.Set(x-x0, y-y0, src.At(x, y))
			}
		}
	}
	return dst
}
