//go:build cgo

package parser

import (
	"image"

	"ragflow/internal/deepdoc/parser/pdf/pdfoxide"
)

// pdfoxideEngine adapts pdfoxide.Engine to the PDFEngine interface.
type pdfoxideEngine struct {
	inner *pdfoxide.Engine
}

// NewEngine returns a PDFEngine backed by pdf_oxide.
func NewEngine(pdfBytes []byte) (PDFEngine, error) {
	eng, err := pdfoxide.NewEngine(pdfBytes)
	if err != nil {
		return nil, err
	}
	return &pdfoxideEngine{inner: eng}, nil
}

func (e *pdfoxideEngine) RawData() []byte         { return e.inner.RawData() }
func (e *pdfoxideEngine) PageCount() (int, error) { return e.inner.PageCount() }
func (e *pdfoxideEngine) Close() error            { return e.inner.Close() }

func (e *pdfoxideEngine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	return e.inner.RenderPage(pageNum, dpi)
}

func (e *pdfoxideEngine) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	return e.inner.RenderPageImage(pageNum, dpi)
}

func (e *pdfoxideEngine) ExtractChars(pageNum int) ([]TextChar, error) {
	chars, err := e.inner.ExtractChars(pageNum)
	if err != nil {
		return nil, err
	}
	result := make([]TextChar, len(chars))
	for i, c := range chars {
		result[i] = TextChar{
			X0: c.X0, X1: c.X1, Top: c.Top, Bottom: c.Bottom,
			Text: c.Text, FontName: c.FontName, FontSize: c.FontSize,
			PageNumber: c.PageNumber,
		}
	}
	return result, nil
}
