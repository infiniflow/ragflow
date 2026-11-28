//go:build cgo

package pdfium

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct FPDF_DOCUMENT__    { int unused; } *FPDF_DOCUMENT;
typedef struct FPDF_PAGE__        { int unused; } *FPDF_PAGE;
typedef struct FPDF_TEXTPAGE__    { int unused; } *FPDF_TEXTPAGE;
typedef struct FPDF_PAGEOBJECT__  { int unused; } *FPDF_PAGEOBJECT;
typedef struct FPDF_PATHSEGMENT__ { int unused; } *FPDF_PATHSEGMENT;

typedef struct FS_MATRIX_ {
    float a;
    float b;
    float c;
    float d;
    float e;
    float f;
} FS_MATRIX;

typedef struct FS_RECTF_ {
    float left;
    float top;
    float right;
    float bottom;
} FS_RECTF;

enum {
    VECTOR_PAGEOBJ_PATH = 2,
    VECTOR_PAGEOBJ_FORM = 5,

    VECTOR_SEGMENT_LINETO   = 0,
    VECTOR_SEGMENT_BEZIERTO = 1,
    VECTOR_SEGMENT_MOVETO   = 2,

    VECTOR_FILLMODE_NONE = 0,
};

extern FPDF_PAGE     FPDF_LoadPage(FPDF_DOCUMENT document, int page_index);
extern void          FPDF_ClosePage(FPDF_PAGE page);
extern double        FPDF_GetPageWidth(FPDF_PAGE page);
extern double        FPDF_GetPageHeight(FPDF_PAGE page);
extern int           FPDFPage_GetRotation(FPDF_PAGE page);
extern int           FPDFPage_GetCropBox(FPDF_PAGE page, float* left, float* bottom,
                                         float* right, float* top);
extern int           FPDFPage_GetMediaBox(FPDF_PAGE page, float* left, float* bottom,
                                          float* right, float* top);

extern FPDF_TEXTPAGE FPDFText_LoadPage(FPDF_PAGE page);
extern void          FPDFText_ClosePage(FPDF_TEXTPAGE text_page);
extern int           FPDFText_CountChars(FPDF_TEXTPAGE text_page);
extern unsigned int  FPDFText_GetUnicode(FPDF_TEXTPAGE text_page, int index);
extern int           FPDFText_IsGenerated(FPDF_TEXTPAGE text_page, int index);
extern int           FPDFText_GetLooseCharBox(FPDF_TEXTPAGE text_page, int index, FS_RECTF* rect);
extern float         FPDFText_GetCharAngle(FPDF_TEXTPAGE text_page, int index);
extern double        FPDFText_GetFontSize(FPDF_TEXTPAGE text_page, int index);

extern int              FPDFPage_CountObjects(FPDF_PAGE page);
extern FPDF_PAGEOBJECT  FPDFPage_GetObject(FPDF_PAGE page, int index);
extern int              FPDFPageObj_GetType(FPDF_PAGEOBJECT page_object);
extern int              FPDFPageObj_GetBounds(FPDF_PAGEOBJECT page_object,
                                              float* left, float* bottom,
                                              float* right, float* top);
extern int              FPDFPageObj_GetMatrix(FPDF_PAGEOBJECT page_object, FS_MATRIX* matrix);
extern int              FPDFPageObj_GetStrokeWidth(FPDF_PAGEOBJECT page_object, float* width);
extern int              FPDFPath_GetDrawMode(FPDF_PAGEOBJECT path, int* fillmode, int* stroke);
extern int              FPDFPath_CountSegments(FPDF_PAGEOBJECT path);
extern FPDF_PATHSEGMENT FPDFPath_GetPathSegment(FPDF_PAGEOBJECT path, int index);
extern int              FPDFPathSegment_GetPoint(FPDF_PATHSEGMENT segment, float* x, float* y);
extern int              FPDFPathSegment_GetType(FPDF_PATHSEGMENT segment);
extern int              FPDFPathSegment_GetClose(FPDF_PATHSEGMENT segment);

extern int              FPDFFormObj_CountObjects(FPDF_PAGEOBJECT form_object);
extern FPDF_PAGEOBJECT  FPDFFormObj_GetObject(FPDF_PAGEOBJECT form_object, unsigned long index);
*/
import "C"

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	pdfsync "ragflow/internal/deepdoc/parser/pdf/pdfsync"
)

// PageGeometry holds post-rotation display page dimensions in PDF points.
type PageGeometry struct {
	Width  float64
	Height float64
}

// Point is a 2D coordinate in top-left display space (PDF points).
type Point struct {
	X float64
	Y float64
}

// WordBox is a contiguous text run in top-left display coordinates.
type WordBox struct {
	Text            string
	X0              float64
	X1              float64
	Top             float64
	Bottom          float64
	RotationDegrees float64
	FontSize        float64
}

// PathSegmentKind identifies a vector path operation.
type PathSegmentKind int

const (
	PathSegmentMoveTo PathSegmentKind = iota
	PathSegmentLineTo
	PathSegmentBezierTo
)

// PathSegment is one transformed vertex/command in a vector path.
type PathSegment struct {
	Kind   PathSegmentKind
	Point  Point
	Closed bool
}

// PathBox describes one visible vector path in top-left display coordinates,
// carrying both its bounding box and its transformed vector segments.
type PathBox struct {
	X0             float64
	X1             float64
	Top            float64
	Bottom         float64
	HasStroke      bool
	HasFill        bool
	OperationCount int
	StrokeWidth    float64
	Segments       []PathSegment
}

type pdfAffine struct {
	a, b, c, d, e, f float64
}

func identityPDFAffine() pdfAffine {
	return pdfAffine{a: 1, d: 1}
}

func (m pdfAffine) apply(x, y float64) (float64, float64) {
	return m.a*x + m.c*y + m.e, m.b*x + m.d*y + m.f
}

func (m pdfAffine) compose(child pdfAffine) pdfAffine {
	return pdfAffine{
		a: m.a*child.a + m.c*child.b,
		b: m.b*child.a + m.d*child.b,
		c: m.a*child.c + m.c*child.d,
		d: m.b*child.c + m.d*child.d,
		e: m.a*child.e + m.c*child.f + m.e,
		f: m.b*child.e + m.d*child.f + m.f,
	}
}

func (m pdfAffine) scale() float64 {
	det := math.Abs(m.a*m.d - m.b*m.c)
	if det <= 0 {
		return 1
	}
	return math.Sqrt(det)
}

type displayTransform struct {
	crop     [4]float64 // [left, bottom, right, top] in unrotated user space
	rotation int        // 0, 1 (90 CW), 2 (180), 3 (270 CW)
	width    float64    // post-rotation display width
	height   float64    // post-rotation display height
}

func readDisplayTransform(page C.FPDF_PAGE) displayTransform {
	rot := int(C.FPDFPage_GetRotation(page)) & 3
	var left, bottom, right, top C.float
	var media [4]float64
	if C.FPDFPage_GetMediaBox(page, &left, &bottom, &right, &top) != 0 {
		media = [4]float64{float64(left), float64(bottom), float64(right), float64(top)}
	} else {
		w, h := float64(C.FPDF_GetPageWidth(page)), float64(C.FPDF_GetPageHeight(page))
		if rot == 1 || rot == 3 {
			w, h = h, w
		}
		media = [4]float64{0, 0, w, h}
	}
	crop := media
	if C.FPDFPage_GetCropBox(page, &left, &bottom, &right, &top) != 0 {
		crop = [4]float64{float64(left), float64(bottom), float64(right), float64(top)}
	}
	cw := crop[2] - crop[0]
	ch := crop[3] - crop[1]
	w, h := cw, ch
	if rot == 1 || rot == 3 {
		w, h = ch, cw
	}
	return displayTransform{
		crop:     crop,
		rotation: rot,
		width:    w,
		height:   h,
	}
}

// point maps an unrotated PDF user-space point (Y up) to post-rotation
// CropBox-relative top-left display coordinates (Y down).
func (t displayTransform) point(ux, uy float64) Point {
	cw := t.crop[2] - t.crop[0]
	ch := t.crop[3] - t.crop[1]
	rx := ux - t.crop[0]
	ry := t.crop[3] - uy
	switch t.rotation {
	case 1: // 90 CW
		return Point{X: ch - ry, Y: rx}
	case 2: // 180
		return Point{X: cw - rx, Y: ch - ry}
	case 3: // 270 CW
		return Point{X: ry, Y: cw - rx}
	default:
		return Point{X: rx, Y: ry}
	}
}

func (t displayTransform) rect(left, bottom, right, top float64) (x0, y0, x1, y1 float64) {
	corners := [4]Point{
		t.point(left, bottom),
		t.point(right, bottom),
		t.point(right, top),
		t.point(left, top),
	}
	x0, y0 = corners[0].X, corners[0].Y
	x1, y1 = corners[0].X, corners[0].Y
	for _, p := range corners[1:] {
		if p.X < x0 {
			x0 = p.X
		}
		if p.X > x1 {
			x1 = p.X
		}
		if p.Y < y0 {
			y0 = p.Y
		}
		if p.Y > y1 {
			y1 = p.Y
		}
	}
	return x0, y0, x1, y1
}

func (t displayTransform) angleDegrees(rad float64) float64 {
	deg := rad*180/math.Pi - float64(t.rotation)*90
	deg = math.Mod(deg, 360)
	if deg > 180 {
		deg -= 360
	}
	if deg < -180 {
		deg += 360
	}
	return deg
}

// ExtractPageVectorLayout reads page geometry, grouped text words (with display
// rotation and font size), and visible vector paths (with transformed segments)
// from a single loaded PDFium page.
func (d *Document) ExtractPageVectorLayout(pageIdx int) (PageGeometry, []WordBox, []PathBox, error) {
	var geom PageGeometry
	var words []WordBox
	var paths []PathBox
	var err error
	pdfsync.With(func() {
		if d.handle == nil {
			err = errors.New("pdfium: document is closed")
			return
		}
		page := C.FPDF_LoadPage(d.handle, C.int(pageIdx))
		if page == nil {
			err = fmt.Errorf("pdfium: FPDF_LoadPage(%d) returned nil", pageIdx)
			return
		}
		defer C.FPDF_ClosePage(page)

		xf := readDisplayTransform(page)
		geom = PageGeometry{Width: xf.width, Height: xf.height}
		words, err = extractPageWordsLocked(page, xf)
		if err != nil {
			return
		}
		paths = extractPagePathsLocked(page, xf)
	})
	return geom, words, paths, err
}

type wordGlyph struct {
	r        rune
	x0, x1   float64
	top, bot float64
	rotDeg   float64
	fontSize float64
}

func extractPageWordsLocked(page C.FPDF_PAGE, xf displayTransform) ([]WordBox, error) {
	textPage := C.FPDFText_LoadPage(page)
	if textPage == nil {
		return nil, errors.New("pdfium: FPDFText_LoadPage returned nil")
	}
	defer C.FPDFText_ClosePage(textPage)

	count := int(C.FPDFText_CountChars(textPage))
	if count < 0 {
		return nil, fmt.Errorf("pdfium: FPDFText_CountChars returned %d", count)
	}
	if count == 0 {
		return nil, nil
	}

	var words []WordBox
	var cur []wordGlyph
	flush := func() {
		if len(cur) == 0 {
			return
		}
		var b strings.Builder
		x0, top := cur[0].x0, cur[0].top
		x1, bot := cur[0].x1, cur[0].bot
		fontSum := 0.0
		for _, g := range cur {
			b.WriteRune(g.r)
			if g.x0 < x0 {
				x0 = g.x0
			}
			if g.x1 > x1 {
				x1 = g.x1
			}
			if g.top < top {
				top = g.top
			}
			if g.bot > bot {
				bot = g.bot
			}
			fontSum += g.fontSize
		}
		text := strings.TrimSpace(b.String())
		if text != "" {
			words = append(words, WordBox{
				Text:            text,
				X0:              x0,
				X1:              x1,
				Top:             top,
				Bottom:          bot,
				RotationDegrees: cur[0].rotDeg,
				FontSize:        fontSum / float64(len(cur)),
			})
		}
		cur = cur[:0]
	}

	for i := 0; i < count; i++ {
		if C.FPDFText_IsGenerated(textPage, C.int(i)) == 1 {
			flush()
			continue
		}
		code := uint32(C.FPDFText_GetUnicode(textPage, C.int(i)))
		if code == 0 {
			flush()
			continue
		}
		r := rune(code)
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			flush()
			continue
		}
		var rect C.FS_RECTF
		if C.FPDFText_GetLooseCharBox(textPage, C.int(i), &rect) == 0 {
			continue
		}
		x0, top, x1, bot := xf.rect(float64(rect.left), float64(rect.bottom), float64(rect.right), float64(rect.top))
		if x1 <= x0 || bot <= top {
			continue
		}
		g := wordGlyph{
			r:        r,
			x0:       x0,
			x1:       x1,
			top:      top,
			bot:      bot,
			rotDeg:   xf.angleDegrees(float64(C.FPDFText_GetCharAngle(textPage, C.int(i)))),
			fontSize: float64(C.FPDFText_GetFontSize(textPage, C.int(i))),
		}
		if len(cur) > 0 && !sameWordGlyph(cur[len(cur)-1], g) {
			flush()
		}
		cur = append(cur, g)
	}
	flush()
	return words, nil
}

func sameWordGlyph(prev, next wordGlyph) bool {
	rotDiff := math.Abs(prev.rotDeg - next.rotDeg)
	if rotDiff > 180 {
		rotDiff = 360 - rotDiff
	}
	if rotDiff > 15 {
		return false
	}
	if prev.fontSize > 0 && next.fontSize > 0 {
		ratio := prev.fontSize / next.fontSize
		if ratio < 0.75 || ratio > 1.33 {
			return false
		}
	}
	fs := math.Max(prev.fontSize, next.fontSize)
	if fs <= 0 {
		fs = math.Max(prev.bot-prev.top, prev.x1-prev.x0)
	}
	gapLimit := math.Max(2.0, fs*0.45)

	absRot := math.Abs(prev.rotDeg)
	vertical := absRot >= 45 && absRot <= 135
	if vertical {
		overlapX := math.Min(prev.x1, next.x1) - math.Max(prev.x0, next.x0)
		minW := math.Min(prev.x1-prev.x0, next.x1-next.x0)
		if minW <= 0 || overlapX < minW*0.35 {
			return false
		}
		gapY := math.Max(next.top-prev.bot, prev.top-next.bot)
		return gapY <= gapLimit
	}
	overlapY := math.Min(prev.bot, next.bot) - math.Max(prev.top, next.top)
	minH := math.Min(prev.bot-prev.top, next.bot-next.top)
	if minH <= 0 || overlapY < minH*0.35 {
		return false
	}
	gapX := math.Max(next.x0-prev.x1, prev.x0-next.x1)
	return gapX <= gapLimit
}

func extractPagePathsLocked(page C.FPDF_PAGE, xf displayTransform) []PathBox {
	count := int(C.FPDFPage_CountObjects(page))
	if count <= 0 {
		return nil
	}
	var out []PathBox
	root := identityPDFAffine()
	for i := 0; i < count; i++ {
		obj := C.FPDFPage_GetObject(page, C.int(i))
		if obj == nil {
			continue
		}
		collectPageObjectPaths(obj, root, xf, 0, &out)
	}
	return out
}

func readObjectMatrix(obj C.FPDF_PAGEOBJECT) pdfAffine {
	var m C.FS_MATRIX
	if C.FPDFPageObj_GetMatrix(obj, &m) == 0 {
		return identityPDFAffine()
	}
	return pdfAffine{
		a: float64(m.a), b: float64(m.b),
		c: float64(m.c), d: float64(m.d),
		e: float64(m.e), f: float64(m.f),
	}
}

func collectPageObjectPaths(obj C.FPDF_PAGEOBJECT, parent pdfAffine, xf displayTransform, depth int, out *[]PathBox) {
	if obj == nil || depth > 16 {
		return
	}
	switch int(C.FPDFPageObj_GetType(obj)) {
	case C.VECTOR_PAGEOBJ_FORM:
		formMatrix := parent.compose(readObjectMatrix(obj))
		n := int(C.FPDFFormObj_CountObjects(obj))
		for i := 0; i < n; i++ {
			child := C.FPDFFormObj_GetObject(obj, C.ulong(i))
			collectPageObjectPaths(child, formMatrix, xf, depth+1, out)
		}
	case C.VECTOR_PAGEOBJ_PATH:
		if path, ok := extractPathObject(obj, parent, xf); ok {
			*out = append(*out, path)
		}
	}
}

func extractPathObject(obj C.FPDF_PAGEOBJECT, parent pdfAffine, xf displayTransform) (PathBox, bool) {
	var fillMode C.int
	var stroke C.int
	if C.FPDFPath_GetDrawMode(obj, &fillMode, &stroke) == 0 {
		return PathBox{}, false
	}
	hasFill := int(fillMode) != C.VECTOR_FILLMODE_NONE
	hasStroke := stroke != 0
	if !hasFill && !hasStroke {
		// Invisible path (typically a clipping region).
		return PathBox{}, false
	}

	mat := parent.compose(readObjectMatrix(obj))
	var strokeW C.float
	strokeWidth := 0.0
	if hasStroke && C.FPDFPageObj_GetStrokeWidth(obj, &strokeW) != 0 {
		strokeWidth = float64(strokeW) * mat.scale()
	}

	segCount := int(C.FPDFPath_CountSegments(obj))
	if segCount <= 0 {
		var l, b, r, t C.float
		if C.FPDFPageObj_GetBounds(obj, &l, &b, &r, &t) == 0 {
			return PathBox{}, false
		}
		pts := [4][2]float64{
			{float64(l), float64(b)},
			{float64(r), float64(b)},
			{float64(r), float64(t)},
			{float64(l), float64(t)},
		}
		x0, top := math.Inf(1), math.Inf(1)
		x1, bot := math.Inf(-1), math.Inf(-1)
		for _, p := range pts {
			ux, uy := parent.apply(p[0], p[1])
			dp := xf.point(ux, uy)
			x0 = math.Min(x0, dp.X)
			x1 = math.Max(x1, dp.X)
			top = math.Min(top, dp.Y)
			bot = math.Max(bot, dp.Y)
		}
		return PathBox{
			X0: x0, X1: x1, Top: top, Bottom: bot,
			HasStroke: hasStroke, HasFill: hasFill, StrokeWidth: strokeWidth,
		}, true
	}

	segments := make([]PathSegment, 0, segCount)
	x0, top := math.Inf(1), math.Inf(1)
	x1, bot := math.Inf(-1), math.Inf(-1)
	opCount := 0
	bezierRun := 0
	for i := 0; i < segCount; i++ {
		seg := C.FPDFPath_GetPathSegment(obj, C.int(i))
		if seg == nil {
			continue
		}
		var px, py C.float
		if C.FPDFPathSegment_GetPoint(seg, &px, &py) == 0 {
			continue
		}
		ux, uy := mat.apply(float64(px), float64(py))
		dp := xf.point(ux, uy)
		x0 = math.Min(x0, dp.X)
		x1 = math.Max(x1, dp.X)
		top = math.Min(top, dp.Y)
		bot = math.Max(bot, dp.Y)

		var kind PathSegmentKind
		switch int(C.FPDFPathSegment_GetType(seg)) {
		case C.VECTOR_SEGMENT_MOVETO:
			kind = PathSegmentMoveTo
			bezierRun = 0
			opCount++
		case C.VECTOR_SEGMENT_BEZIERTO:
			kind = PathSegmentBezierTo
			bezierRun++
			if bezierRun == 3 {
				opCount++
				bezierRun = 0
			}
		default:
			kind = PathSegmentLineTo
			bezierRun = 0
			opCount++
		}
		closed := C.FPDFPathSegment_GetClose(seg) != 0
		segments = append(segments, PathSegment{
			Kind:   kind,
			Point:  dp,
			Closed: closed,
		})
	}
	if len(segments) == 0 {
		return PathBox{}, false
	}
	return PathBox{
		X0:             x0,
		X1:             x1,
		Top:            top,
		Bottom:         bot,
		HasStroke:      hasStroke,
		HasFill:        hasFill,
		OperationCount: opCount,
		StrokeWidth:    strokeWidth,
		Segments:       segments,
	}, true
}
