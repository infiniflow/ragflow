package parser

import (
	"image"
)

// renderFn is the active page-rendering function.  It defaults to
// fallbackRender (pure Go, engine-provided RenderPageImage).  When
// pdfium is available (*_cgo build), renderer_pdfium.go replaces it
// with pdfiumRender via its init().
var renderFn = fallbackRender

// renderPageToImage renders a page at 216 DPI for downstream DLA/TSR/OCR.
func renderPageToImage(engine PDFEngine, pageNum int) (image.Image, error) {
	return renderFn(engine, pageNum)
}

// fallbackRender uses the engine's own RenderPageImage (no C dependency).
func fallbackRender(engine PDFEngine, pageNum int) (image.Image, error) {
	img, err := engine.RenderPageImage(pageNum, dlaDPI)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, ErrNoPDFData
	}
	return img, nil
}

// ErrNoPDFData is returned when the engine has no raw PDF bytes to render.
var ErrNoPDFData = &pdfError{"engine has no raw PDF data"}

type pdfError struct{ msg string }

func (e *pdfError) Error() string { return e.msg }
