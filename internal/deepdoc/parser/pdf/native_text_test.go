package pdf

import (
	"image"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestOCRMergeChars_KeepsUncoveredNativeText pins the fix for the merge path:
// characters no detect box covers are emitted as native line boxes instead of
// being dropped. main dropped them, so figure labels and rotated text vanished.
func TestOCRMergeChars_KeepsUncoveredNativeText(t *testing.T) {
	p := NewParser(pdf.DefaultParserConfig())
	page := image.NewRGBA(image.Rect(0, 0, 300, 100))
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRBoxes: []pdf.OCRBox{{X0: 200, Y0: 60, X1: 260, Y1: 60, X2: 260, Y2: 80, X3: 200, Y3: 80}},
		OCRTexts: []pdf.OCRText{{Text: "Bitmap", Confidence: 0.9}},
	}
	// Two native lines left of every detect box.
	chars := []pdf.TextChar{
		{X0: 10, X1: 18, Top: 10, Bottom: 18, Text: "S", PageNumber: 0},
		{X0: 18, X1: 26, Top: 10, Bottom: 18, Text: "C", PageNumber: 0},
		{X0: 26, X1: 34, Top: 10, Bottom: 18, Text: "L", PageNumber: 0},
		{X0: 10, X1: 18, Top: 30, Bottom: 38, Text: "S", PageNumber: 0},
		{X0: 18, X1: 26, Top: 30, Bottom: 38, Text: "D", PageNumber: 0},
		{X0: 26, X1: 34, Top: 30, Bottom: 38, Text: "A", PageNumber: 0},
	}
	boxes := p.ocrMergeChars(t.Context(), page, chars, mock, 0, 1)
	var texts []string
	for _, b := range boxes {
		texts = append(texts, b.Text)
	}
	joined := strings.Join(texts, "|")
	if !strings.Contains(joined, "SCL") || !strings.Contains(joined, "SDA") || !strings.Contains(joined, "Bitmap") {
		t.Fatalf("boxes = %q, want native SCL/SDA plus OCR Bitmap", joined)
	}
}

// TestOCRMergeChars_AssemblesFormulaInOneBox pins the box granularity: glyphs
// on different baselines inside one detect box are assembled into that box
// (main's behavior) instead of being split into per-line boxes, which used to
// shred formulas into "( − ( ))", "VDD", "VOL", "max".
func TestOCRMergeChars_AssemblesFormulaInOneBox(t *testing.T) {
	p := NewParser(pdf.DefaultParserConfig())
	page := image.NewRGBA(image.Rect(0, 0, 200, 60))
	mock := &MockDocAnalyzer{
		Healthy:  true,
		OCRBoxes: []pdf.OCRBox{{X0: 0, Y0: 0, X1: 150, Y1: 0, X2: 150, Y2: 20, X3: 0, Y3: 20}},
		OCRTexts: []pdf.OCRText{{Text: "OCR", Confidence: 0.9}},
	}
	// Base glyphs plus a raised "max" (smaller, higher baseline) in one box.
	chars := []pdf.TextChar{
		{X0: 0, X1: 10, Top: 2, Bottom: 12, Text: "V", PageNumber: 0},
		{X0: 10, X1: 20, Top: 2, Bottom: 12, Text: "D", PageNumber: 0},
		{X0: 20, X1: 30, Top: 2, Bottom: 12, Text: "D", PageNumber: 0},
		{X0: 30, X1: 40, Top: 4, Bottom: 12, Text: "V", PageNumber: 0},
		{X0: 40, X1: 50, Top: 4, Bottom: 12, Text: "O", PageNumber: 0},
		{X0: 50, X1: 60, Top: 4, Bottom: 12, Text: "L", PageNumber: 0},
		{X0: 60, X1: 65, Top: 0, Bottom: 6, Text: "m", PageNumber: 0},
		{X0: 65, X1: 70, Top: 0, Bottom: 6, Text: "a", PageNumber: 0},
		{X0: 70, X1: 75, Top: 0, Bottom: 6, Text: "x", PageNumber: 0},
	}
	boxes := p.ocrMergeChars(t.Context(), page, chars, mock, 0, 1)
	if len(boxes) != 1 {
		t.Fatalf("boxes = %d, want one assembled formula box: %+v", len(boxes), boxes)
	}
	text := boxes[0].Text
	posVDD := strings.Index(text, "VDD")
	posVOL := strings.Index(text, "VOL")
	posMAX := strings.Index(text, "max")
	if posVDD < 0 || posVOL < 0 || posMAX < 0 || !(posVDD < posVOL && posVOL < posMAX) {
		t.Fatalf("assembled text = %q, want VDD before VOL before max", text)
	}
}
