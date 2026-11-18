//go:build cgo

package pdfoxide

import (
	"context"
	"image"
	"math"
	"sync"

	"ragflow/internal/deepdoc/parser/pdf/pdfium"
)

// Char represents a single character extracted from a PDF page.
type Char struct {
	X0, X1      float64
	Top, Bottom float64
	Text        string
	FontName    string
	FontSize    float64
	PageNumber  int
}

// Engine wraps pdf_oxide to extract chars and render pages.
//
// rawData and doc belong to exactly one document (NewEngine opens one and
// Close releases it), so the per-document lookups below are cached on the
// engine: the byte-level /Rotate and /CropBox scan runs once, and pdfium's
// page sizes are read in one pass instead of one full document reload per
// page.
type Engine struct {
	doc     *Document
	rawData []byte

	mu         sync.Mutex
	directives map[int]pageDirectives
	// pageDims caches the page sizes pdfium has reported so far; dimsAttempted
	// records that the document-wide pass already ran, so a page missing from
	// the map is retried on its own rather than by re-reading every page.
	pageDims      map[int][2]float64
	dimsAttempted bool

	// pageSizesBatch and pageSizeSingle are the pdfium lookups this engine
	// uses. They are fields rather than package variables so a test can
	// simulate a partial batch on one engine without touching global state.
	pageSizesBatch func(ctx context.Context, pdf []byte, pages []int) (map[int][2]float64, int, error)
	pageSizeSingle func(pdf []byte, page int) (float64, float64, error)
}

// NewEngine opens a PDF from bytes and returns an Engine.
func NewEngine(pdfBytes []byte) (*Engine, error) {
	doc, err := OpenBytes(pdfBytes)
	if err != nil {
		return nil, err
	}
	return &Engine{
		doc:            doc,
		rawData:        pdfBytes,
		pageSizesBatch: pdfium.PageSizes,
		pageSizeSingle: pdfium.PageSize,
	}, nil
}

func (e *Engine) RawData() []byte { return e.rawData }

// loadPageDims fills pageDims with every page size pdfium can report from one
// batch pass. The pass runs once: successful pages are cached even when other
// pages fail, so a missing page is retried individually by pageSize instead of
// re-reading the whole document on every call. A failed pass is not retried in
// bulk; only the pages that missed out are looked up again.
//
// The pass has no caller context to honour: ExtractChars carries none, and
// threading one through the PDFEngine interface is a separate change.
func (e *Engine) loadPageDims() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.dimsAttempted {
		return
	}
	e.dimsAttempted = true
	n, err := e.doc.PageCount()
	if err != nil || n <= 0 {
		return
	}
	idxs := make([]int, n)
	for i := range idxs {
		idxs[i] = i
	}
	sizes, _, _ := e.pageSizesBatch(context.Background(), e.rawData, idxs)
	if len(sizes) == 0 {
		return
	}
	if e.pageDims == nil {
		e.pageDims = make(map[int][2]float64, len(sizes))
	}
	for page, size := range sizes {
		e.pageDims[page] = size
	}
}

// pageSize returns a page's post-rotation size. It prefers the document-wide
// batch, then retries that one page directly, and reports false only when both
// fail. The retry result is cached so a page pdfium cannot size costs at most
// one extra lookup per document. Both ExtractChars and PageSize use this helper
// so the fallback rule cannot diverge between them.
func (e *Engine) pageSize(pageNum int) ([2]float64, bool) {
	e.loadPageDims()

	e.mu.Lock()
	d, ok := e.pageDims[pageNum]
	e.mu.Unlock()
	if ok {
		return d, true
	}

	w, h, err := e.pageSizeSingle(e.rawData, pageNum)
	if err != nil {
		return [2]float64{}, false
	}
	d = [2]float64{w, h}
	e.mu.Lock()
	if e.pageDims == nil {
		e.pageDims = make(map[int][2]float64, 1)
	}
	e.pageDims[pageNum] = d
	e.mu.Unlock()
	return d, true
}

// pageDirective returns the cached /Rotate and /CropBox of one page, scanning
// the raw bytes on first use.
func (e *Engine) pageDirective(pageNum int) pageDirectives {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.directives == nil {
		e.directives = scanPageDirectives(e.rawData)
	}
	return e.directives[pageNum]
}

func (e *Engine) ExtractChars(pageNum int) ([]Char, error) {
	chars, err := e.doc.GetDedupePageChars(pageNum, 0.5)
	if err != nil {
		return nil, err
	}

	// pdf_oxide returns characters in the original (unrotated) PDF
	// coordinate space.  Rotate to match pdfium's effective (post-
	// /Rotate) coordinate space used for rendering and DLA/OCR.
	//
	// Rotation detection uses two sources:
	// 1. Byte-scan for explicit /Rotate (finds directly-defined values).
	// 2. Dimension comparison: pdf_oxide raw vs pdfium effective.
	//    If dimensions are swapped, the page has implicit rotation
	//    (inherited /Rotate or ContentBox rotation).
	rawW, rawH, _ := e.doc.PageSize(pageNum)
	effW, effH := rawW, rawH
	if d, ok := e.pageSize(pageNum); ok {
		effW, effH = d[0], d[1]
	}

	dimSwapped := rawW > 0 && rawH > 0 && effW > 0 && effH > 0 &&
		math.Abs(rawW-effH) < 1 && math.Abs(rawH-effW) < 1

	dir := e.pageDirective(pageNum)
	rawRot := dir.rotate

	needsRotate := false
	rotation90 := false
	rotation180 := false

	if dimSwapped {
		needsRotate = true
		if rawRot == 270 {
			rotation90 = false
		} else {
			rotation90 = true
		}
	} else if rawRot == 90 || rawRot == 270 {
		// Explicit /Rotate found but dimension-swap check failed
		// (e.g. CropBox alters effective dimensions).  Trust the
		// explicit /Rotate value.
		needsRotate = true
		rotation90 = (rawRot != 270)
	} else if rawRot == 180 {
		needsRotate = true
		rotation180 = true
	}

	// CropBox correction — shift origin if CropBox differs from MediaBox.
	var cropDX, cropDY float64
	realCrop, hasCrop := dir.cropBox, dir.hasCrop
	if hasCrop {
		cropH := realCrop[3] - realCrop[1]
		oxideCropH := rawH
		if cropH > 0 && (realCrop[0] != 0 || realCrop[1] != 0 ||
			math.Abs(realCrop[3]-oxideCropH) > 0.5) {
			cropDX = -realCrop[0]
			cropDY = -(oxideCropH - realCrop[3])
		}
	}

	// When rotation is applied, the crop shift must be applied AFTER
	// rotation, using the correct axes for the rotated coordinate space.
	rotateCropDX, rotateCropDY := cropDX, cropDY
	if needsRotate && (cropDX != 0 || cropDY != 0) {
		switch {
		case rotation90:
			// rotate(x+cropDX,y+cropDY) = (rawH-(y+cropDY),x+cropDX)
			// = rotate(x,y) + (-cropDY, +cropDX)
			// cropDX=-30,cropDY=-10 => post-rotate shift = (+10,-30)
			rotateCropDX = -cropDY
			rotateCropDY = cropDX
		case rotation180:
			rotateCropDX = -cropDX
			rotateCropDY = -cropDY
		default: // 270 CW
			rotateCropDX = cropDY
			rotateCropDY = -cropDX
		}
		cropDX, cropDY = 0, 0
	}

	result := make([]Char, len(chars))
	for i, c := range chars {
		x0, x1 := c.X0, c.X1
		top, bottom := c.Top, c.Bottom

		x0 += cropDX
		x1 += cropDX
		top += cropDY
		bottom += cropDY

		if needsRotate {
			origX0, origX1 := x0, x1
			origTop, origBottom := top, bottom

			switch {
			case rotation90:
				x0 = rawH - origBottom
				x1 = rawH - origTop
				top = origX0
				bottom = origX1
			case rotation180:
				x0 = rawW - origX1
				x1 = rawW - origX0
				top = rawH - origBottom
				bottom = rawH - origTop
			default: // 270 CW
				x0 = origTop
				x1 = origBottom
				top = rawW - origX1
				bottom = rawW - origX0
			}

			if x0 > x1 {
				x0, x1 = x1, x0
			}
			if top > bottom {
				top, bottom = bottom, top
			}
		}

		// Apply crop correction in the final coordinate space.
		x0 += rotateCropDX
		x1 += rotateCropDX
		top += rotateCropDY
		bottom += rotateCropDY

		result[i] = Char{
			X0: x0, X1: x1, Top: top, Bottom: bottom,
			Text: c.Text, FontName: c.Fontname, FontSize: c.Size,
			PageNumber: pageNum,
		}
	}
	return result, nil
}

// RenderPageImage uses pdfium for page rendering — pdfium correctly
// applies /Rotate so the output matches character coordinates and DLA.
// There is no pdf_oxide fallback because pdf_oxide does not apply
// /Rotate, producing images in a different coordinate space.
func (e *Engine) RenderPageImage(pageNum int, dpi float64) (image.Image, error) {
	return pdfium.RenderPage(e.rawData, pageNum, dpi)
}

func (e *Engine) RenderPage(pageNum int, dpi float64) ([]byte, error) {
	result, err := e.doc.RenderPage(pageNum, dpi)
	if err != nil {
		return nil, err
	}
	return result.Data, nil
}

// PageSize returns the effective page dimensions via pdfium, which
// correctly applies /Rotate.  pdf_oxide's own PageSize returns raw
// (unrotated) dimensions. Sizes come from the document-wide cache, with a
// single-page pdfium retry for a page the batch could not size; only when
// both fail does it fall back to pdf_oxide, exactly as before.
func (e *Engine) PageSize(pageNum int) (float64, float64, error) {
	if d, ok := e.pageSize(pageNum); ok {
		return d[0], d[1], nil
	}
	return e.doc.PageSize(pageNum)
}
func (e *Engine) PageCount() (int, error) { return e.doc.PageCount() }
func (e *Engine) Close() error            { e.doc.Close(); return nil }
