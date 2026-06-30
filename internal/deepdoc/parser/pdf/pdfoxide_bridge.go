//go:build cgo

package parser

import (
	"image"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// pdfoxideEngine adapts pdfoxide.Engine to the pdf.PDFEngine interface.
type pdfoxideEngine struct {
	inner *pdfoxide.Engine
}

// NewEngine returns a pdf.PDFEngine backed by pdf_oxide.
func NewEngine(pdfBytes []byte) (pdf.PDFEngine, error) {
	eng, err := pdfoxide.NewEngine(pdfBytes)
	if err != nil {
		return nil, err
	}
	return &pdfoxideEngine{inner: eng}, nil
}

func (e *pdfoxideEngine) RawData() []byte         { return e.inner.RawData() }
func (e *pdfoxideEngine) PageCount() (int, error) { return e.inner.PageCount() }
func (e *pdfoxideEngine) Close() error            { return e.inner.Close() }

func (e *pdfoxideEngine) Outlines() ([]pdf.Outline, error) {
	ol := pdfium.ExtractOutlines(e.inner.RawData())
	result := make([]pdf.Outline, len(ol))
	for i, o := range ol {
		result[i] = pdf.Outline{Title: o.Title, Level: o.Level, PageNumber: o.PageNumber}
	}
	return result, nil
}

func (e *pdfoxideEngine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	return e.inner.RenderPage(pageNum, dpi)
}

func (e *pdfoxideEngine) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	return e.inner.RenderPageImage(pageNum, dpi)
}

func (e *pdfoxideEngine) ExtractChars(pageNum int) ([]pdf.TextChar, error) {
	chars, err := e.inner.ExtractChars(pageNum)
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
