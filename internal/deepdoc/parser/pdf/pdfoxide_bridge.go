//go:build cgo

package pdf

import (
	"image"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// pdfoxideEngine adapts pdfoxide.Engine to the pdf.PDFEngine interface.
type PDFOxideEngine struct {
	Inner *pdfoxide.Engine
}

// NewEngine returns a pdf.PDFEngine backed by pdf_oxide.
func NewEngine(pdfBytes []byte) (pdf.PDFEngine, error) {
	eng, err := pdfoxide.NewEngine(pdfBytes)
	if err != nil {
		return nil, err
	}
	return &PDFOxideEngine{Inner: eng}, nil
}

func (e *PDFOxideEngine) RawData() []byte         { return e.Inner.RawData() }
func (e *PDFOxideEngine) PageCount() (int, error) { return e.Inner.PageCount() }
func (e *PDFOxideEngine) Close() error            { return e.Inner.Close() }

func (e *PDFOxideEngine) Outlines() ([]pdf.Outline, error) {
	ol := pdfium.ExtractOutlines(e.Inner.RawData())
	result := make([]pdf.Outline, len(ol))
	for i, o := range ol {
		result[i] = pdf.Outline{Title: o.Title, Level: o.Level, PageNumber: o.PageNumber}
	}
	return result, nil
}

func (e *PDFOxideEngine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	return e.Inner.RenderPage(pageNum, dpi)
}

func (e *PDFOxideEngine) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	return e.Inner.RenderPageImage(pageNum, dpi)
}

func (e *PDFOxideEngine) ExtractChars(pageNum int) ([]pdf.TextChar, error) {
	chars, err := e.Inner.ExtractChars(pageNum)
	if err != nil {
		return nil, err
	}
	result := make([]pdf.TextChar, len(chars))
	for i, c := range chars {
		result[i] = pdf.TextChar{
			X0: c.X0, X1: c.X1, Top: c.Top, Bottom: c.Bottom,
			Text: c.Text, FontName: c.FontName, FontSize: c.FontSize,
			PageNumber: c.PageNumber,
		}
	}
	return result, nil
}
