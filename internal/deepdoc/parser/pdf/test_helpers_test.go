package pdf

import (
	"image"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

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
