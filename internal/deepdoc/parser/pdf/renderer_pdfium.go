//go:build cgo

package pdf

import (
	"image"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// pdfiumRender uses the pdfium C library for higher-quality rasterisation
// (AA, hinting) which is essential for downstream OCR/DLA accuracy on
// scanned or low-quality PDFs.
func pdfiumRender(engine pdf.PDFEngine, pageNum int) (image.Image, error) {
	raw := engine.RawData()
	if raw == nil {
		// PythonCharEngine and mocks don't carry PDF bytes —
		// fall back to the engine's own RenderPageImage.
		return fallbackRender(engine, pageNum)
	}
	// Guard against typed nil: (*image.RGBA)(nil) wrapped as non-nil interface
	// would panic on downstream .Bounds() / .At() calls.
	img, err := pdfium.RenderPage(raw, pageNum, 216)
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, ErrNoPDFData
	}
	return img, nil
}

func init() {
	renderFn = pdfiumRender
}
