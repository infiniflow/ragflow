package pdf

import (
	"image"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"reflect"
)

// renderFn is the active page-rendering function.  It defaults to
// fallbackRender (pure Go, engine-provided RenderPageImage).  When
// pdfium is available (*_cgo build), renderer_pdfium.go replaces it
// with pdfiumRender via its init().
var renderFn = fallbackRender

// renderPageToImage renders a page at 216 DPI for downstream DLA/TSR/OCR.
func RenderPageToImage(engine pdf.PDFEngine, pageNum int) (image.Image, error) {
	return renderFn(engine, pageNum)
}

// fallbackRender uses the engine's own RenderPageImage (no C dependency).
func fallbackRender(engine pdf.PDFEngine, pageNum int) (image.Image, error) {
	img, err := engine.RenderPageImage(pageNum, pdf.DlaDPI)
	if err != nil {
		return nil, err
	}
	// Guard against typed-nil (e.g. (*image.RGBA)(nil) returned as non-nil
	// interface).  The plain img==nil check misses that case.
	if img == nil {
		return nil, ErrNoPDFData
	}
	if rv := reflect.ValueOf(img); rv.Kind() == reflect.Ptr && rv.IsNil() {
		return nil, ErrNoPDFData
	}
	return img, nil
}

// ErrNoPDFData is returned when the engine has no raw PDF bytes to render.
var ErrNoPDFData = &pdfError{"engine has no raw PDF data"}

type pdfError struct{ msg string }

func (e *pdfError) Error() string { return e.msg }
