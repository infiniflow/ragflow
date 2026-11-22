//go:build cgo

package pdf

import (
	"testing"

	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
)

// uncoveredFixture holds: one covered usable glyph, one uncovered usable
// glyph, one blank glyph and one PUA (unusable) glyph outside every box.
// The uncovered set must contain ONLY the uncovered usable non-blank glyph.
func uncoveredFixture() ([]ocrDetectBox, []pdftype.TextChar) {
	boxes := []ocrDetectBox{{
		box: pdftype.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 20},
		x0:  0, y0: 0, x1: 100, y1: 20,
	}}
	chars := []pdftype.TextChar{
		{Text: "A", X0: 10, X1: 20, Top: 5, Bottom: 15},        // covered
		{Text: "B", X0: 200, X1: 210, Top: 5, Bottom: 15},      // uncovered, usable
		{Text: " ", X0: 220, X1: 230, Top: 5, Bottom: 15},      // uncovered, blank -> excluded
		{Text: "\ue000", X0: 240, X1: 250, Top: 5, Bottom: 15}, // PUA -> excluded
	}
	return boxes, chars
}

// TestUncoveredGlyphFilter guards the filtering difference the single-pass
// reuse must honor: charsOutsideDetectBoxes only emits usable, non-blank
// glyphs, while matchCharsToBoxes scans all chars. Reusing bestIdx<0 without
// the same filter would emit blank/PUA glyphs as native boxes.
func TestUncoveredGlyphFilter(t *testing.T) {
	boxes, chars := uncoveredFixture()
	_, uncovered := matchCharsToBoxes(boxes, chars)
	if len(uncovered) != 1 || uncovered[0].Text != "B" {
		t.Fatalf("uncovered: got %v, want exactly [B]", uncovered)
	}
}

func BenchmarkMatchCharsToBoxesDense(b *testing.B) {
	boxes := make([]ocrDetectBox, 150)
	for i := range boxes {
		x := float64(i * 20)
		boxes[i] = ocrDetectBox{
			box: pdftype.TextBox{X0: x, X1: x + 18, Top: 0, Bottom: 20},
			x0:  x, y0: 0, x1: x + 18, y1: 20,
		}
	}
	chars := make([]pdftype.TextChar, 3000)
	for i := range chars {
		x := float64(i % 3000)
		y := float64(i/3000*10) + 5
		chars[i] = pdftype.TextChar{Text: "字", X0: x, X1: x + 8, Top: y, Bottom: y + 10}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		matchCharsToBoxes(boxes, chars)
	}
}
