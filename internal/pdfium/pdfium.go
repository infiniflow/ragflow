// Package pdfium renders PDF pages using the system's libpdfium.so
// (bundled with pypdfium2). It exists solely to replace pdf_oxide's
// RenderPageRaw for use cases where image quality matters for downstream
// OCR/DLA — pdf_oxide still handles all text/char/table extraction.
package pdfium

/*
#cgo LDFLAGS: -L/home/shenyushi/cc-workspace/ragflow/.venv/lib/python3.13/site-packages/pypdfium2_raw -lpdfium -lm -lpthread -ldl
#cgo linux LDFLAGS: -Wl,-rpath,/home/shenyushi/cc-workspace/ragflow/.venv/lib/python3.13/site-packages/pypdfium2_raw

#include <stdint.h>
#include <stdlib.h>

typedef struct FPDF_DOCUMENT__  { int unused; } *FPDF_DOCUMENT;
typedef struct FPDF_PAGE__     { int unused; } *FPDF_PAGE;
typedef struct FPDF_BITMAP__   { int unused; } *FPDF_BITMAP;

extern void          FPDF_InitLibrary(void);
extern FPDF_DOCUMENT FPDF_LoadMemDocument(const void* data_buf, int size, const char* password);
extern void          FPDF_CloseDocument(FPDF_DOCUMENT document);
extern int           FPDF_GetPageCount(FPDF_DOCUMENT document);
extern FPDF_PAGE     FPDF_LoadPage(FPDF_DOCUMENT document, int page_index);
extern void          FPDF_ClosePage(FPDF_PAGE page);
extern double        FPDF_GetPageWidth(FPDF_PAGE page);
extern double        FPDF_GetPageHeight(FPDF_PAGE page);
extern FPDF_BITMAP   FPDFBitmap_Create(int width, int height, int alpha);
extern void          FPDFBitmap_Destroy(FPDF_BITMAP bitmap);
extern void          FPDF_RenderPageBitmap(FPDF_BITMAP bitmap, FPDF_PAGE page,
                       int start_x, int start_y, int size_x, int size_y,
                       int rotate, int flags);
extern void*         FPDFBitmap_GetBuffer(FPDF_BITMAP bitmap);
extern int           FPDFBitmap_GetWidth(FPDF_BITMAP bitmap);
extern int           FPDFBitmap_GetHeight(FPDF_BITMAP bitmap);
extern int           FPDFBitmap_GetStride(FPDF_BITMAP bitmap);
*/
import "C"
import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"unsafe"
)

var initOnce sync.Once

// pdfiumMu serializes all pdfium C API access. pdfium is NOT thread-safe —
// concurrent calls to FPDF_LoadPage / FPDF_RenderPageBitmap corrupt the
// global heap, causing SIGSEGV. See TestPdfiumConcurrentSafety.
var pdfiumMu sync.Mutex

// Init initializes the PDFium library. Safe to call multiple times.
func Init() { initOnce.Do(func() { C.FPDF_InitLibrary() }) }

// PageSize returns the page dimensions in PDF points (1/72 inch) as seen
// after rotation.  For a page with /Rotate 90 on A4, this returns ~842×595
// (swapped from the MediaBox 595×842).  The call is cheap — it opens the
// document and page, reads dimensions, then closes.
func PageSize(pdfData []byte, pageIdx int) (width, height float64, err error) {
	Init()
	pdfiumMu.Lock()
	defer pdfiumMu.Unlock()
	_, _, pw, ph, closeAll, err := openPage(pdfData, pageIdx)
	if err != nil {
		return 0, 0, err
	}
	closeAll()
	return pw, ph, nil
}

// RenderPage renders a single page of a PDF to an *image.RGBA at the given DPI.
// pdfData is the raw PDF bytes, pageIdx is 0-based.
func RenderPage(pdfData []byte, pageIdx int, dpi float64) (*image.RGBA, error) {
	Init()
	pdfiumMu.Lock()
	defer pdfiumMu.Unlock()
	_, page, pw, ph, closeAll, err := openPage(pdfData, pageIdx)
	if err != nil {
		return nil, err
	}
	defer closeAll()

	scale := dpi / 72.0
	pxW := int(math.Round(pw * scale))
	pxH := int(math.Round(ph * scale))

	bitmap := C.FPDFBitmap_Create(C.int(pxW), C.int(pxH), 1) // 1 = RGBA
	if bitmap == nil {
		return nil, fmt.Errorf("pdfium: FPDFBitmap_Create(%d,%d) returned nil", pxW, pxH)
	}
	defer C.FPDFBitmap_Destroy(bitmap)

	// Fill with opaque white before rendering, so transparent areas
	// (e.g. outside crop box) are white rather than undefined.
	stride := int(C.FPDFBitmap_GetStride(bitmap))
	buf := C.FPDFBitmap_GetBuffer(bitmap)
	pixels := (*[1 << 30]byte)(unsafe.Pointer(buf))[:pxH*stride : pxH*stride]
	for i := range pixels {
		pixels[i] = 255
	}

	// FPDF_ANNOT (0x01) — render annotations.
	// LCD text AA (0x02) is left off; default text smoothing is sufficient.
	C.FPDF_RenderPageBitmap(bitmap, page, 0, 0, C.int(pxW), C.int(pxH), 0, 0x01)

	// pdfium outputs BGRA; convert to RGBA.
	img := image.NewRGBA(image.Rect(0, 0, pxW, pxH))
	for y := 0; y < pxH; y++ {
		for x := 0; x < pxW; x++ {
			off := y*stride + x*4
			img.SetRGBA(x, y, color.RGBA{
				R: pixels[off+2], // B
				G: pixels[off+1], // G
				B: pixels[off],   // R
				A: 255,
			})
		}
	}
	return img, nil
}

// openPage opens a document and page, returning post-rotation dimensions
// and a cleanup function.  Callers must call closeAll() to free resources.
func openPage(pdfData []byte, pageIdx int) (
	doc C.FPDF_DOCUMENT,
	page C.FPDF_PAGE,
	pw, ph float64,
	closeAll func(),
	err error,
) {
	cData := C.CBytes(pdfData)

	doc = C.FPDF_LoadMemDocument(unsafe.Pointer(cData), C.int(len(pdfData)), nil)
	if doc == nil {
		C.free(cData)
		err = fmt.Errorf("pdfium: FPDF_LoadMemDocument returned nil")
		return
	}

	page = C.FPDF_LoadPage(doc, C.int(pageIdx))
	if page == nil {
		C.FPDF_CloseDocument(doc)
		C.free(cData)
		err = fmt.Errorf("pdfium: FPDF_LoadPage(%d) returned nil", pageIdx)
		return
	}

	pw = float64(C.FPDF_GetPageWidth(page))
	ph = float64(C.FPDF_GetPageHeight(page))
	if pw <= 0 || ph <= 0 {
		C.FPDF_ClosePage(page)
		C.FPDF_CloseDocument(doc)
		C.free(cData)
		err = fmt.Errorf("pdfium: invalid page dimensions %.1fx%.1f", pw, ph)
		return
	}

	closeAll = func() {
		C.FPDF_ClosePage(page)
		C.FPDF_CloseDocument(doc)
		C.free(cData)
	}
	return
}
