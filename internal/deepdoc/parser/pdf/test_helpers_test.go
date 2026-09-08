package pdf

import (
	"image"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// cropImageRect crops a rectangular region from an image.
func cropImageRect(img image.Image, x0, y0, x1, y1 int) image.Image {
	b := img.Bounds()
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	out := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			out.Set(x-x0, y-y0, img.At(x, y))
		}
	}
	return out
}

// ── testPageImg: small test image for ocrMergeChars tests ─────────────
// 90×120 px at 216 DPI → 30×40 pt in PDF space after /3.0 scaling.

func testPageImg() image.Image {
	return image.NewRGBA(image.Rect(0, 0, 90, 120))
}

// ── cellTexts: extract text strings from TSRCells ─────────────────────

func cellTexts(cells []pdf.TSRCell) []string {
	t := make([]string, len(cells))
	for i, c := range cells {
		t[i] = c.Text
	}
	return t
}
