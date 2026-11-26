//go:build cgo

package pdfium

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct FPDF_DOCUMENT__  { int unused; } *FPDF_DOCUMENT;
typedef struct FPDF_PAGE__      { int unused; } *FPDF_PAGE;
typedef struct FPDF_TEXTPAGE__  { int unused; } *FPDF_TEXTPAGE;

typedef struct FS_RECTF_ {
    float left;
    float top;
    float right;
    float bottom;
} FS_RECTF;

extern void          FPDF_InitLibrary(void);
extern FPDF_DOCUMENT FPDF_LoadMemDocument(const void* data_buf, int size, const char* password);
extern void          FPDF_CloseDocument(FPDF_DOCUMENT document);
extern int           FPDF_GetPageCount(FPDF_DOCUMENT document);
extern FPDF_PAGE     FPDF_LoadPage(FPDF_DOCUMENT document, int page_index);
extern void          FPDF_ClosePage(FPDF_PAGE page);
extern double        FPDF_GetPageWidth(FPDF_PAGE page);
extern double        FPDF_GetPageHeight(FPDF_PAGE page);

extern FPDF_TEXTPAGE FPDFText_LoadPage(FPDF_PAGE page);
extern void          FPDFText_ClosePage(FPDF_TEXTPAGE text_page);
extern int           FPDFText_CountChars(FPDF_TEXTPAGE text_page);
extern unsigned int  FPDFText_GetUnicode(FPDF_TEXTPAGE text_page, int index);
extern int           FPDFText_IsGenerated(FPDF_TEXTPAGE text_page, int index);
extern int           FPDFText_GetLooseCharBox(FPDF_TEXTPAGE text_page, int index, FS_RECTF* rect);
extern int           FPDFText_GetCharOrigin(FPDF_TEXTPAGE text_page, int index,
                                            double* x, double* y);
extern float         FPDFText_GetCharAngle(FPDF_TEXTPAGE text_page, int index);
extern double        FPDFText_GetFontSize(FPDF_TEXTPAGE text_page, int index);
extern unsigned long FPDFText_GetFontInfo(FPDF_TEXTPAGE text_page, int index,
                                          void* buffer, unsigned long buflen,
                                          int* flags);

extern int FPDFPage_GetRotation(FPDF_PAGE page);
extern int FPDFPage_GetCropBox(FPDF_PAGE page, float* left, float* bottom,
                               float* right, float* top);
extern int FPDFPage_GetMediaBox(FPDF_PAGE page, float* left, float* bottom,
                                float* right, float* top);
*/
import "C"

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"unsafe"

	pdfsync "ragflow/internal/deepdoc/parser/pdf/pdfsync"
)

// TextChar is one glyph extracted from a PDF page. ExtractChars returns it in
// PDF points with a top-left origin after page rotation and the CropBox
// translation; pageRawChars returns the intermediate unrotated form.
type TextChar struct {
	X0, X1      float64
	Top, Bottom float64
	Text        string
	FontName    string
	FontSize    float64
	PageNumber  int
}

// Document owns one open PDFium document. It reuses the handle across page
// extractions instead of reparsing the bytes for each call.
type Document struct {
	data   []byte
	buffer unsafe.Pointer
	handle C.FPDF_DOCUMENT
}

// OpenDocument opens a PDF from bytes. The bytes are copied into C memory,
// which PDFium keeps for the lifetime of the document.
func OpenDocument(pdfData []byte) (*Document, error) {
	if len(pdfData) == 0 {
		return nil, errors.New("pdfium: empty pdf data")
	}
	d := &Document{data: pdfData}
	var err error
	pdfsync.With(func() {
		Init()
		d.buffer = C.CBytes(pdfData)
		d.handle = C.FPDF_LoadMemDocument(d.buffer, C.int(len(pdfData)), nil)
		if d.handle == nil {
			C.free(d.buffer)
			d.buffer = nil
			err = errors.New("pdfium: FPDF_LoadMemDocument returned nil")
		}
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// RawData returns the bytes the document was opened from.
func (d *Document) RawData() []byte { return d.data }

// Close releases the PDFium document and its input buffer. It is safe to call
// more than once.
func (d *Document) Close() {
	pdfsync.With(func() {
		if d.handle != nil {
			C.FPDF_CloseDocument(d.handle)
			d.handle = nil
		}
		if d.buffer != nil {
			C.free(d.buffer)
			d.buffer = nil
		}
	})
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() (int, error) {
	var count int
	var err error
	pdfsync.With(func() {
		if d.handle == nil {
			err = errors.New("pdfium: document is closed")
			return
		}
		count = int(C.FPDF_GetPageCount(d.handle))
	})
	return count, err
}

// PageSize returns the post-rotation page dimensions in PDF points.
func (d *Document) PageSize(pageNum int) (float64, float64, error) {
	var width, height float64
	var err error
	pdfsync.With(func() {
		if d.handle == nil {
			err = errors.New("pdfium: document is closed")
			return
		}
		page := C.FPDF_LoadPage(d.handle, C.int(pageNum))
		if page == nil {
			err = fmt.Errorf("pdfium: FPDF_LoadPage(%d) returned nil", pageNum)
			return
		}
		defer C.FPDF_ClosePage(page)
		width, height = float64(C.FPDF_GetPageWidth(page)), float64(C.FPDF_GetPageHeight(page))
	})
	return width, height, err
}

// pageRawChars returns characters in the same coordinate space the previous
// extraction adapter used: x is the PDF user-space left edge, and top / bottom
// are measured from the top of the MediaBox. No rotation or CropBox translation
// is applied; PlainText relies on this stability.
//
// The returned geometry is read from the same open page, so ExtractChars does
// not need a second page load.
func (d *Document) pageRawChars(pageNum int) ([]TextChar, pageGeometry, error) {
	var out []TextChar
	var g pageGeometry
	var err error
	pdfsync.With(func() {
		if d.handle == nil {
			err = errors.New("pdfium: document is closed")
			return
		}
		page := C.FPDF_LoadPage(d.handle, C.int(pageNum))
		if page == nil {
			err = fmt.Errorf("pdfium: FPDF_LoadPage(%d) returned nil", pageNum)
			return
		}
		defer C.FPDF_ClosePage(page)

		g.rotation = int(C.FPDFPage_GetRotation(page))
		var left, bottom, right, top C.float
		if C.FPDFPage_GetMediaBox(page, &left, &bottom, &right, &top) != 0 {
			g.media = [4]float64{float64(left), float64(bottom), float64(right), float64(top)}
		} else {
			width, height := C.FPDF_GetPageWidth(page), C.FPDF_GetPageHeight(page)
			if g.rotation == 1 || g.rotation == 3 {
				width, height = height, width
			}
			g.media = [4]float64{0, 0, float64(width), float64(height)}
		}
		if C.FPDFPage_GetCropBox(page, &left, &bottom, &right, &top) != 0 {
			g.crop = [4]float64{float64(left), float64(bottom), float64(right), float64(top)}
		} else {
			// FPDFPage_GetCropBox fails when the page inherits the default
			// CropBox, which is the MediaBox.
			g.crop = g.media
		}

		textPage := C.FPDFText_LoadPage(page)
		if textPage == nil {
			err = fmt.Errorf("pdfium: FPDFText_LoadPage(%d) returned nil", pageNum)
			return
		}
		defer C.FPDFText_ClosePage(textPage)

		count := int(C.FPDFText_CountChars(textPage))
		if count < 0 {
			err = fmt.Errorf("pdfium: FPDFText_CountChars(%d) returned %d", pageNum, count)
			return
		}
		if count == 0 {
			return
		}

		mediaTop := g.media[3]
		out = make([]TextChar, 0, count)
		for i := 0; i < count; i++ {
			// PDFium synthesises line-break characters the previous adapter
			// never returned. Text layout is reconstructed from glyph geometry,
			// so keeping them would duplicate the newlines PageText adds itself.
			if C.FPDFText_IsGenerated(textPage, C.int(i)) == 1 {
				continue
			}
			var rect C.FS_RECTF
			if C.FPDFText_GetLooseCharBox(textPage, C.int(i), &rect) == 0 {
				continue
			}
			code := uint32(C.FPDFText_GetUnicode(textPage, C.int(i)))
			// An unmapped glyph has no Unicode value. Keep the "#" placeholder
			// the previous adapter used so garbled-page detection still sees it.
			text := "#"
			if code != 0 {
				text = string(rune(code))
			}
			fontSize := float64(C.FPDFText_GetFontSize(textPage, C.int(i)))
			charBoxTop := mediaTop - float64(rect.top)
			charBoxBottom := mediaTop - float64(rect.bottom)
			var originX, originY C.double
			angle := float64(C.FPDFText_GetCharAngle(textPage, C.int(i)))
			if angle == 0 && C.FPDFText_GetCharOrigin(textPage, C.int(i), &originX, &originY) != 0 {
				// The previous adapter reported the typographic cell: the glyph
				// advance horizontally and exactly font-size above the baseline
				// vertically. The loose box supplies the advance; the origin
				// supplies the baseline. Rotated glyphs keep the loose box: their
				// font-size offset is perpendicular to the page axes.
				charBoxTop = mediaTop - (float64(originY) + fontSize)
				charBoxBottom = mediaTop - float64(originY)
			}
			out = append(out, TextChar{
				X0:         float64(rect.left),
				X1:         float64(rect.right),
				Top:        charBoxTop,
				Bottom:     charBoxBottom,
				Text:       text,
				FontName:   charFontName(textPage, i),
				FontSize:   fontSize,
				PageNumber: pageNum,
			})
		}
	})
	return out, g, err
}

// pageGeometry is the page state the character transform needs: /Rotate and the
// CropBox/MediaBox in unrotated user space.
type pageGeometry struct {
	rotation    int
	crop, media [4]float64
}

// ExtractChars returns deduplicated, post-rotation characters in top-left
// coordinates, matching what the PDFEngine contract expects. The rotation and
// CropBox handling deliberately reproduces the previous extraction adapter so
// the engine swap does not move existing character boxes.
func (d *Document) ExtractChars(pageNum int) ([]TextChar, error) {
	chars, geometry, err := d.pageRawChars(pageNum)
	if err != nil {
		return nil, err
	}
	if len(chars) == 0 {
		return nil, nil
	}

	rawW := geometry.media[2] - geometry.media[0]
	rawH := geometry.media[3] - geometry.media[1]
	crop := geometry.crop
	cropH := crop[3] - crop[1]

	needsRotate := geometry.rotation >= 1 && geometry.rotation <= 3
	rotation90 := geometry.rotation == 1
	rotation180 := geometry.rotation == 2

	// CropBox correction: shift the origin when the CropBox differs from the
	// MediaBox. The previous adapter applied this shift twice for unrotated
	// pages (cropDX/DY are copied to rotateCropDX/DY before only the rotated
	// branch clears them), and existing extraction results depend on that
	// placement.
	var cropDX, cropDY float64
	if cropH > 0 && (crop[0] != 0 || crop[1] != 0 ||
		math.Abs(crop[3]-rawH) > 0.5) {
		cropDX = -crop[0]
		cropDY = -(rawH - crop[3])
	}
	rotateCropDX, rotateCropDY := cropDX, cropDY
	if needsRotate && (cropDX != 0 || cropDY != 0) {
		switch {
		case rotation90:
			rotateCropDX = -cropDY
			rotateCropDY = cropDX
		case rotation180:
			rotateCropDX = -cropDX
			rotateCropDY = -cropDY
		default: // 270 clockwise
			rotateCropDX = cropDY
			rotateCropDY = -cropDX
		}
		cropDX, cropDY = 0, 0
	}

	for i := range chars {
		c := &chars[i]
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
			default: // 270 clockwise
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

		c.X0 = x0 + rotateCropDX
		c.X1 = x1 + rotateCropDX
		c.Top = top + rotateCropDY
		c.Bottom = bottom + rotateCropDY
	}
	return dedupeChars(chars, 0.5), nil
}

// PageText returns the page's plain text in reading order (top to bottom,
// then left to right) with spaces inserted for horizontal gaps.
func (d *Document) PageText(pageNum int) (string, error) {
	chars, _, err := d.pageRawChars(pageNum)
	if err != nil {
		return "", err
	}
	if len(chars) == 0 {
		return "", nil
	}
	sorted := make([]TextChar, len(chars))
	copy(sorted, chars)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Top != sorted[j].Top {
			return sorted[i].Top < sorted[j].Top
		}
		return sorted[i].X0 < sorted[j].X0
	})
	var b strings.Builder
	for i, c := range sorted {
		b.WriteString(c.Text)
		if i+1 < len(sorted) {
			next := sorted[i+1]
			if math.Abs(next.Top-c.Top) < 0.5 {
				gap := next.X0 - c.X1
				if gap > (c.X1-c.X0)*0.3 {
					b.WriteByte(' ')
				}
			} else {
				b.WriteByte('\n')
			}
		}
	}
	return b.String(), nil
}

func charFontName(textPage C.FPDF_TEXTPAGE, index int) string {
	n := C.FPDFText_GetFontInfo(textPage, C.int(index), nil, 0, nil)
	if n <= 1 {
		return ""
	}
	buf := make([]byte, int(n))
	if C.FPDFText_GetFontInfo(textPage, C.int(index), unsafe.Pointer(&buf[0]), n, nil) == 0 {
		return ""
	}
	if i := strings.IndexByte(string(buf), 0); i >= 0 {
		buf = buf[:i]
	}
	return string(buf)
}

// dedupeChars drops overlapping duplicates of the same font and size. PDFs
// with fake bold or shadow text paint the same glyph more than once; keeping
// every copy would double the extracted text.
func dedupeChars(chars []TextChar, tolerance float64) []TextChar {
	if len(chars) == 0 {
		return nil
	}
	sorted := make([]TextChar, len(chars))
	copy(sorted, chars)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].X0 < sorted[j].X0 })

	result := make([]TextChar, 0, len(sorted))
	maxCharWidth := 0.0
	for _, ch := range sorted {
		cw := ch.X1 - ch.X0
		if cw > maxCharWidth {
			maxCharWidth = cw
		}
		dup := false
		for i := len(result) - 1; i >= 0; i-- {
			existing := &result[i]
			if ch.X0-existing.X1 > maxCharWidth {
				break
			}
			ox := math.Max(0, math.Min(ch.X1, existing.X1)-math.Max(ch.X0, existing.X0))
			oy := math.Max(0, math.Min(ch.Bottom, existing.Bottom)-math.Max(ch.Top, existing.Top))
			oa := ox * oy
			if oa <= 0 {
				continue
			}
			ca := cw * (ch.Bottom - ch.Top)
			ea := (existing.X1 - existing.X0) * (existing.Bottom - existing.Top)
			maxA := math.Max(ca, ea)
			if maxA <= 0 {
				continue
			}
			sameFont := ch.FontName == existing.FontName
			sameSize := math.Abs(ch.FontSize-existing.FontSize) <= tolerance
			if oa/maxA > 0.5 && sameFont && sameSize {
				dup = true
				break
			}
		}
		if !dup {
			result = append(result, ch)
		}
	}
	return result
}
