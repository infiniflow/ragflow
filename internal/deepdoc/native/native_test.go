//go:build cgo

package native

// Unit tests for the pure (model-free) pieces of package native.
// These run under the default `go test ./...` (no ONNX Runtime, no models).
// Model-backed end-to-end checks live in native_integration_test.go (build tag
// `integration`) so they are excluded from the default unit run.

import (
	"math"
	"strings"
	"testing"
)

func TestNMS(t *testing.T) {
	boxes := []nmsBox{
		{X0: 0, Y0: 0, X1: 10, Y1: 10, Score: 0.9},
		{X0: 0, Y0: 0, X1: 10, Y1: 10, Score: 0.8},       // duplicate -> suppressed
		{X0: 100, Y0: 100, X1: 110, Y1: 110, Score: 0.7}, // far away -> kept
	}
	keep := nms(boxes, 0.45, true)
	if len(keep) != 2 {
		t.Fatalf("want 2 kept boxes, got %d (%v)", len(keep), keep)
	}
	if keep[0] != 0 || keep[1] != 2 {
		t.Fatalf("want kept indices [0 2], got %v", keep)
	}
}

func TestNMSNoPlusOne(t *testing.T) {
	// Two boxes that barely overlap; without the +1 term IoU < 0.2 -> both kept.
	a := nmsBox{X0: 0, Y0: 0, X1: 10, Y1: 10, Score: 0.9}
	b := nmsBox{X0: 9, Y0: 0, X1: 19, Y1: 10, Score: 0.8}
	keep := nms([]nmsBox{a, b}, 0.2, false)
	if len(keep) != 2 {
		t.Fatalf("want 2 kept boxes (no +1), got %d (%v)", len(keep), keep)
	}
}

func TestOCRRecCTCDecode(t *testing.T) {
	// vocab = 3: ["blank", "a", "b"]. Build a [recSeqLen*recVocab] tensor whose
	// argmax sequence is a, blank, b, blank, blank -> "ab".
	out := make([]float32, recSeqLen*recVocab)
	seq := []int{1, 0, 2, 0, 0}
	for t, idx := range seq {
		out[t*recVocab+idx] = 0.9
	}
	res := ocrRecCTCDecode(out, []string{"blank", "a", "b"})
	if res.Text != "ab" {
		t.Fatalf("want 'ab', got %q", res.Text)
	}
	if math.Abs(float64(res.Score-0.9)) > 1e-6 {
		t.Fatalf("want score 0.9, got %v", res.Score)
	}
}

func TestOCRRecCTCDecodeDedup(t *testing.T) {
	out := make([]float32, recSeqLen*recVocab)
	// a, a, blank, b -> consecutive a's collapse -> "ab"
	for _, t := range []int{0, 1} {
		out[t*recVocab+1] = 0.9
	}
	out[2*recVocab+0] = 0.9 // blank
	out[3*recVocab+2] = 0.9 // b
	res := ocrRecCTCDecode(out, []string{"blank", "a", "b"})
	if res.Text != "ab" {
		t.Fatalf("want 'ab' (deduped), got %q", res.Text)
	}
}

func TestBilinearResize1x1(t *testing.T) {
	// 1x1 red pixel (BGR: 0,0,255) resized to NxN must stay uniform.
	src := []byte{0, 0, 255}
	dst := bilinearResize(src, 1, 1, 5, 5)
	if len(dst) != 5*5*3 {
		t.Fatalf("wrong dst length %d", len(dst))
	}
	for i := 0; i < len(dst); i += 3 {
		if dst[i] != 0 || dst[i+1] != 0 || dst[i+2] != 255 {
			t.Fatalf("resize changed pixel at %d: %v", i, dst[i:i+3])
		}
	}
}

func TestRound(t *testing.T) {
	if round2(1.2345) != 1.23 {
		t.Fatalf("round2(1.2345) = %v", round2(1.2345))
	}
	if round4(1.23456) != 1.2346 {
		t.Fatalf("round4(1.23456) = %v", round4(1.23456))
	}
}

func TestDLAWire(t *testing.T) {
	r := DLAResult{Boxes: []DLABox{{X0: 1.235, Y0: 2.345, X1: 3.0, Y1: 4.0, Score: 0.5, Class: 5}}, W: 1000, H: 1000}
	// within bounds so clamp is a no-op
	want := `{"bboxes":[[1.235,2.345,3,4,0.5,5]]}`
	if got := r.Wire(); got != want {
		t.Fatalf("Wire() = %s, want %s", got, want)
	}
}

func TestDLAWireClamps(t *testing.T) {
	// Boxes outside the image must be clamped into [0,W]/[0,H].
	r := DLAResult{Boxes: []DLABox{
		{X0: -5, Y0: 2.3, X1: 3000, Y1: -1, Score: 0.5, Class: 5},
	}, W: 10, H: 10}
	want := `{"bboxes":[[0,2.3,10,0,0.5,5]]}`
	if got := r.Wire(); got != want {
		t.Fatalf("Wire() = %s, want %s", got, want)
	}
}

func TestTSRWire(t *testing.T) {
	r := TSRResult{Boxes: []TSRBox{{Label: "table column", Score: 0.7, X0: 1.2, X1: 9.8, Top: 3.4, Bottom: 5.6}}, W: 1000, H: 1000}
	// "table column" -> class 1; within bounds so clamp is a no-op.
	want := `{"bboxes":[[1.2,3.4,9.8,5.6,0.7,1]]}`
	if got := r.Wire(); got != want {
		t.Fatalf("Wire() = %s, want %s", got, want)
	}
}

func TestTSRWireClamps(t *testing.T) {
	// Boxes outside the image must be clamped into [0,W]/[0,H].
	r := TSRResult{Boxes: []TSRBox{
		{Label: "table column", Score: 0.7, X0: -5, X1: 9.8, Top: 3.4, Bottom: 5000},
	}, W: 10, H: 10}
	want := `{"bboxes":[[0,3.4,9.8,10,0.7,1]]}`
	if got := r.Wire(); got != want {
		t.Fatalf("Wire() = %s, want %s", got, want)
	}
}

func TestOCRRecWire(t *testing.T) {
	r := OCRRecResult{Text: "hello", Score: 1.0}
	got := r.Wire()
	if !strings.Contains(got, "hello") {
		t.Fatalf("Wire() missing text: %s", got)
	}
	if !strings.Contains(got, `"output"`) {
		t.Fatalf("Wire() missing output key: %s", got)
	}
}

// ---- DBPostProcess (det) geometry unit tests (model-free) ----

func TestConvexHullSquare(t *testing.T) {
	// A square plus an interior point; hull keeps only the 4 corners.
	pts := []pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {5, 5}}
	h := convexHull(pts)
	if len(h) != 4 {
		t.Fatalf("want 4 hull points, got %d: %v", len(h), h)
	}
}

func TestPolygonAreaUnitSquare(t *testing.T) {
	sq := []pt{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	if got := polygonArea(sq); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("unit square area = %v, want 1", got)
	}
}

func TestMinAreaRectGetMiniBoxes(t *testing.T) {
	// A 100x40 rectangle, top-left at (10,20).
	rect := [4]pt{{10, 20}, {110, 20}, {110, 60}, {10, 60}}
	corners, sside := minAreaRect(rect[:])
	if sside < 39.9 || sside > 40.1 {
		t.Fatalf("minSide = %v, want ~40", sside)
	}
	// The canonical ordering starts at top-left (smallest x, smallest y).
	tl := corners[0]
	if tl.X != 10 || tl.Y != 20 {
		t.Fatalf("corner[0] (top-left) = %v, want {10 20}", tl)
	}
}

func TestUnclipExpands(t *testing.T) {
	sq := [4]pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	before := polygonArea(sq[:])
	expanded := unclip(sq, 1.5)
	after := polygonArea(expanded[:])
	if after <= before {
		t.Fatalf("unclip did not expand area: before=%v after=%v", before, after)
	}
}

func TestFillPolyCoversQuad(t *testing.T) {
	// 11x11 mask; fill the quad from (0,0) to (10,10). OpenCV's cv2.fillPoly
	// treats the integer vertices as pixel corners and (with the +0.5
	// fixed-point rounding) fills the full 11x11 cell = 121 pixels, matching
	// cv2.fillPoly bit-for-bit.
	quad := [4]pt{{0, 0}, {10, 0}, {10, 10}, {0, 10}}
	mask := make([]bool, 11*11)
	fillPoly(mask, 11, 11, quad)
	var n int
	for _, b := range mask {
		if b {
			n++
		}
	}
	if n != 121 {
		t.Fatalf("filled %d px, want 121 (cv2.fillPoly)", n)
	}
}

func TestFilterTagDetResDropsTiny(t *testing.T) {
	// One valid wide box and one tiny (sub-3px) box.
	boxes := []DetBox{
		{Pts: [4][2]float32{{10, 10}, {110, 10}, {110, 30}, {10, 30}}},
		{Pts: [4][2]float32{{200, 200}, {201, 200}, {201, 201}, {200, 201}}}, // 1px
	}
	kept := filterTagDetRes(boxes, 300, 300)
	if len(kept) != 1 {
		t.Fatalf("want 1 box kept, got %d", len(kept))
	}
}

func TestDetWireNesting(t *testing.T) {
	r := DetResult{Boxes: []DetBox{
		{Pts: [4][2]float32{{1, 2}, {3, 2}, {3, 4}, {1, 4}}},
	}}
	got := r.Wire()
	// Boxes must live at output[0][0] (page -> batch -> boxes).
	want := `{"output":[[[[[1,2],[3,2],[3,4],[1,4]]]]]}`
	if got != want {
		t.Fatalf("Wire() = %s, want %s", got, want)
	}
}
