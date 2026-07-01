package pdf

import (
	"image"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// MockEngine is a minimal pdf.PDFEngine stub for unit/integration tests.
type MockEngine struct {
	Chars    map[int][]pdf.TextChar
	NumPages int
	RenderW  int
	RenderH  int
}

func (m *MockEngine) ExtractChars(pg int) ([]pdf.TextChar, error) {
	return m.Chars[pg], nil
}
func (m *MockEngine) RenderPage(pg int, dpi float64) ([]byte, error) {
	return nil, ErrNoPDFData
}
func (m *MockEngine) RenderPageImage(pg int, dpi float64) (image.Image, error) {
	w, h := m.RenderW, m.RenderH
	if w <= 0 {
		w = 100
	}
	if h <= 0 {
		h = 100
	}
	return image.NewRGBA(image.Rect(0, 0, w, h)), nil
}
func (m *MockEngine) PageCount() (int, error) {
	if m.NumPages <= 0 {
		return 1, nil
	}
	return m.NumPages, nil
}
func (m *MockEngine) RawData() []byte                  { return nil }
func (m *MockEngine) Close() error                     { return nil }
func (m *MockEngine) Outlines() ([]pdf.Outline, error) { return nil, nil }
