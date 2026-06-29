package parser

import (
	"image"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── mockEngine: minimal pdf.PDFEngine stub for unit tests ─────────────

type mockEngine struct {
	chars     map[int][]pdf.TextChar
	pageCount int
	renderW   int
	renderH   int
}

func (m *mockEngine) ExtractChars(pg int) ([]pdf.TextChar, error) {
	return m.chars[pg], nil
}
func (m *mockEngine) RenderPage(pg int, dpi float64) ([]byte, error) {
	w, h := m.renderW, m.renderH
	if w <= 0 {
		w = 595
	}
	if h <= 0 {
		h = 842
	}
	return nil, nil
}
func (m *mockEngine) RenderPageImage(pg int, dpi float64) (image.Image, error) {
	w, h := m.renderW, m.renderH
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 100
	}
	return image.NewRGBA(image.Rect(0, 0, w, h)), nil
}
func (m *mockEngine) PageCount() (int, error) {
	if m.pageCount <= 0 {
		return 1, nil
	}
	return m.pageCount, nil
}
func (m *mockEngine) RawData() []byte                  { return nil }
func (m *mockEngine) Close() error                     { return nil }
func (m *mockEngine) Outlines() ([]pdf.Outline, error) { return nil, nil }

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
