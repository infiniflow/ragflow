package util

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

// makeTestPageImage creates a solid-color RGBA PNG and returns the encoded bytes.
func makeTestPageImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	return img
}

func TestCropSectionImage_SinglePage(t *testing.T) {
	pageImages := map[int]image.Image{
		0: makeTestPageImage(200, 300, color.RGBA{255, 0, 0, 255}),
	}
	posTag := FormatPositionTag(0, 10, 100, 20, 150)
	b64 := CropSectionImage(posTag, pageImages, 1)

	if b64 == "" {
		t.Fatal("expected non-empty base64 image")
	}

	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	img := decodePNG(t, decoded)

	bounds := img.Bounds()
	if bounds.Dx() != 90 {
		t.Errorf("width: got %d, want 90", bounds.Dx())
	}
	if bounds.Dy() != 276 {
		t.Errorf("height: got %d, want 276", bounds.Dy())
	}
}

func TestCropSectionImage_EmptyImages(t *testing.T) {
	posTag := FormatPositionTag(0, 10, 100, 20, 150)

	if b64 := CropSectionImage(posTag, nil, 1); b64 != "" {
		t.Error("nil pageImages should return empty string")
	}
	if b64 := CropSectionImage(posTag, map[int]image.Image{}, 1); b64 != "" {
		t.Error("empty pageImages should return empty string")
	}
}

func TestCropSectionImage_OutOfBounds(t *testing.T) {
	pageImages := map[int]image.Image{
		0: makeTestPageImage(200, 300, color.RGBA{255, 0, 0, 255}),
	}
	posTag := FormatPositionTag(5, 10, 100, 20, 150)
	if b64 := CropSectionImage(posTag, pageImages, 1); b64 != "" {
		t.Error("out-of-bounds page should return empty string")
	}
}

func TestCropSectionImage_InvalidTag(t *testing.T) {
	pageImages := map[int]image.Image{
		0: makeTestPageImage(200, 300, color.RGBA{255, 0, 0, 255}),
	}
	if b64 := CropSectionImage("invalid", pageImages, 1); b64 != "" {
		t.Error("invalid position tag should return empty string")
	}
	if b64 := CropSectionImage("", pageImages, 1); b64 != "" {
		t.Error("empty position tag should return empty string")
	}
}

func TestCropSectionImage_ContextPadding(t *testing.T) {
	pageImages := map[int]image.Image{
		0: makeTestPageImage(200, 800, color.RGBA{255, 0, 0, 255}),
	}
	posTag := FormatPositionTag(0, 20, 120, 300, 400)
	b64 := CropSectionImage(posTag, pageImages, 1)
	if b64 == "" {
		t.Fatal("expected non-empty result")
	}
	decoded, _ := base64.StdEncoding.DecodeString(b64)
	img := decodePNG(t, decoded)
	bounds := img.Bounds()
	if bounds.Dy() != 346 {
		t.Errorf("height with context: got %d, want 346", bounds.Dy())
	}
}

func TestCropSectionImage_ZoomScaling(t *testing.T) {
	pageImages := map[int]image.Image{
		0: makeTestPageImage(400, 600, color.RGBA{255, 0, 0, 255}),
	}
	posTag := FormatPositionTag(0, 10, 100, 20, 150)
	b64 := CropSectionImage(posTag, pageImages, 2)
	if b64 == "" {
		t.Fatal("expected non-empty result")
	}
	decoded, _ := base64.StdEncoding.DecodeString(b64)
	img := decodePNG(t, decoded)
	bounds := img.Bounds()
	if bounds.Dx() != 180 {
		t.Errorf("width at zoom 2: got %d, want 180", bounds.Dx())
	}
}

func TestRotateImageCW(t *testing.T) {
	// Create a 3x2 image with known colors: (0,0)=red, (1,0)=green, (2,0)=blue,
	//                                    (0,1)=white, (1,1)=black, (2,1)=gray
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	r, g, b, w, bl, gr := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 0, 255}, color.RGBA{128, 128, 128, 255}
	img.Set(0, 0, r)
	img.Set(1, 0, g)
	img.Set(2, 0, b)
	img.Set(0, 1, w)
	img.Set(1, 1, bl)
	img.Set(2, 1, gr)

	t.Run("0 degrees", func(t *testing.T) {
		rot := RotateImageCW(img, 0)
		if rot == nil {
			t.Fatal("nil result")
		}
		if rot.Bounds().Dx() != 3 || rot.Bounds().Dy() != 2 {
			t.Errorf("size: got %dx%d, want 3x2", rot.Bounds().Dx(), rot.Bounds().Dy())
		}
		if !colorEqual(rot.At(0, 0), r) || !colorEqual(rot.At(2, 1), gr) {
			t.Error("pixels shifted for 0° rotation")
		}
	})
	t.Run("90 degrees", func(t *testing.T) {
		rot := RotateImageCW(img, 90)
		if rot == nil {
			t.Fatal("nil result")
		}
		if rot.Bounds().Dx() != 2 || rot.Bounds().Dy() != 3 {
			t.Errorf("size: got %dx%d, want 2x3", rot.Bounds().Dx(), rot.Bounds().Dy())
		}
		// 90° CW: (0,0) of dst = (h-1-y, x) = (1, 0) = original (0,1)=white
		if !colorEqual(rot.At(0, 0), w) {
			t.Error("90° CW top-left should be original (0,1)=white")
		}
		// 90° CW: (1, 2) of dst = (h-1-y, x) = (1-1-2=-2...) → wait
		// (x=1, y=2): dst_x = h-1-y = 2-1-2 = -1? No. h=2, dst_x = 2-1-y = 1-y.
		// For y=2: dst_x = 1-2 = -1. That's wrong.
		// Actually 90° CW maps (orig_x, orig_y) → (h-1-orig_y, orig_x).
		// So original (2,1)=gray → dst (2-1-1=0, 2) = (0,2)
		if !colorEqual(rot.At(0, 2), gr) {
			t.Error("90° CW: original (2,1)=gray should be at (0,2)")
		}
		// Original (0,0)=red → dst (2-1-0=1, 0) = (1,0)
		if !colorEqual(rot.At(1, 0), r) {
			t.Error("90° CW: original (0,0)=red should be at (1,0)")
		}
	})
	t.Run("180 degrees", func(t *testing.T) {
		rot := RotateImageCW(img, 180)
		if rot == nil {
			t.Fatal("nil result")
		}
		if rot.Bounds().Dx() != 3 || rot.Bounds().Dy() != 2 {
			t.Errorf("size: got %dx%d, want 3x2", rot.Bounds().Dx(), rot.Bounds().Dy())
		}
		if !colorEqual(rot.At(0, 0), gr) {
			t.Error("180°: (0,0) should be original (2,1)=gray")
		}
		if !colorEqual(rot.At(2, 1), r) {
			t.Error("180°: (2,1) should be original (0,0)=red")
		}
	})
	t.Run("270 degrees", func(t *testing.T) {
		rot := RotateImageCW(img, 270)
		if rot == nil {
			t.Fatal("nil result")
		}
		if rot.Bounds().Dx() != 2 || rot.Bounds().Dy() != 3 {
			t.Errorf("size: got %dx%d, want 2x3", rot.Bounds().Dx(), rot.Bounds().Dy())
		}
	})
	t.Run("invalid angle", func(t *testing.T) {
		if RotateImageCW(img, 45) != nil {
			t.Error("expected nil for invalid angle")
		}
	})
}

func TestMapRotatedPointToOriginal_RoundTrip(t *testing.T) {
	// Verify that forward (rotateImageCW) → inverse (mapRotatedPointToOriginal)
	// recovers the original coordinates for all rotation angles.
	origW, origH := 200, 100
	for _, angle := range []int{0, 90, 180, 270} {
		for _, ox := range []float64{0, 50, 199} {
			for _, oy := range []float64{0, 30, 99} {
				rx, ry := rotateCoordCW(ox, oy, origW, origH, angle)
				gotX, gotY := MapRotatedPointToOriginal(rx, ry, angle, origW, origH)
				if math.Abs(gotX-ox) > 0.01 || math.Abs(gotY-oy) > 0.01 {
					t.Errorf("angle=%d orig(%.0f,%.0f) → rot(%.0f,%.0f) → got(%.1f,%.1f)",
						angle, ox, oy, rx, ry, gotX, gotY)
				}
			}
		}
	}
}

func TestMapRotatedPointToOriginal(t *testing.T) {
	// Verify alignment with Python's _map_rotated_point formulas.
	// Original 200x100; rotW,rotH swap for 90/270.
	tests := []struct {
		angle        int
		rx, ry       float64
		origW, origH int
		wantX, wantY float64
	}{
		{0, 50, 30, 200, 100, 50, 30},
		{90, 50, 30, 200, 100, 30, 49},   // rotH=100: forward (100-1-oy,ox)
		{180, 50, 30, 200, 100, 149, 69}, // (199-50, 99-30)
		{270, 50, 30, 200, 100, 169, 50}, // rotW=200: inverse (199-30,50)
	}
	for _, tt := range tests {
		gotX, gotY := MapRotatedPointToOriginal(tt.rx, tt.ry, tt.angle, tt.origW, tt.origH)
		if math.Abs(gotX-tt.wantX) > 0.01 || math.Abs(gotY-tt.wantY) > 0.01 {
			t.Errorf("angle=%d (%f,%f) got(%f,%f) want(%f,%f)",
				tt.angle, tt.rx, tt.ry, gotX, gotY, tt.wantX, tt.wantY)
		}
	}
}

func colorEqual(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// TestCropSectionImage_MultiPage verifies the bottomRemaining fix for 3+ page
// positions where page heights differ. Regression test for Bug #3.
func TestCropSectionImage_MultiPage(t *testing.T) {
	// Page 0: tall (2000px), Page 1: short (800px), Page 2: short (800px)
	// Content spans all 3 pages. The old bug subtracted full pageH2 from
	// bottomRemaining instead of the actual clamped value, causing negative
	// y1 on the last page → 1×1 placeholder crop.
	pageImages := map[int]image.Image{
		0: makeTestPageImage(100, 2000, color.RGBA{200, 0, 0, 255}),
		1: makeTestPageImage(100, 800, color.RGBA{0, 200, 0, 255}),
		2: makeTestPageImage(100, 800, color.RGBA{0, 0, 200, 255}),
	}
	// pdf.Position spans pages 0-2, bottom reaches into page 2.
	posTag := "@@1-3\t0.0\t100.0\t0.0\t500.0##"
	b64 := CropSectionImage(posTag, pageImages, 1)
	if b64 == "" {
		t.Fatal("expected non-empty result for multi-page position")
	}
	// Decode and check height: content 500pt + bottom on page 1 clamped
	// to 800 → page 1 crop 0-800, page 2 crop 0-200. Total with 2x6px gaps
	// should be ~2000 + 200 + 12 = 2212.
	decoded, _ := base64.StdEncoding.DecodeString(b64)
	img := decodePNG(t, decoded)
	h := img.Bounds().Dy()
	// Without the fix, page 2 gets negative y1 → 1x1 output (~100 + gap).
	// With fix, proper crop from all 3 pages.
	if h < 500 {
		t.Errorf("multi-page height too small: got %d, want >= 500 (bug: bottomRemaining over-subtraction)", h)
	}
	t.Logf("multi-page stitch height: %d", h)
}

// TestCropSectionImage_LargePageSpan verifies 2-page case was not broken.
func TestCropSectionImage_LargePageSpan(t *testing.T) {
	pageImages := map[int]image.Image{
		0: makeTestPageImage(100, 800, color.RGBA{200, 0, 0, 255}),
		1: makeTestPageImage(100, 600, color.RGBA{0, 200, 0, 255}),
	}
	posTag := "@@1-2\t0.0\t100.0\t0.0\t900.0##"
	b64 := CropSectionImage(posTag, pageImages, 1)
	if b64 == "" {
		t.Fatal("expected non-empty result")
	}
	decoded, _ := base64.StdEncoding.DecodeString(b64)
	img := decodePNG(t, decoded)
	if img.Bounds().Dy() < 500 {
		t.Errorf("2-page height too small: %d", img.Bounds().Dy())
	}
}

// TestCropSectionByDLA tests that figure sections get cropped using the
// best-overlapping DLA region instead of the text-box PositionTag.
func TestCropSectionByDLA(t *testing.T) {
	// Create a test page image (216 DPI scale = 3x PDF points).
	// The image is 300x450 px, which is 100x150 in PDF points at scale 3.
	pageImages := map[int]image.Image{
		0: makeTestPageImage(300, 450, color.RGBA{255, 0, 0, 255}),
	}

	// DLA regions in pixel space (216 DPI).
	// Figure region at (30, 60, 270, 420) — a large area covering most of the image.
	// Text region at (10, 400, 100, 440) — a small text box near the bottom.
	dlaDebug := []pdf.DLAPageRegions{{
		Page: 0,
		Regions: []pdf.DLARegion{
			{X0: 10, Y0: 400, X1: 100, Y1: 440, Label: "text"},
			{X0: 30, Y0: 60, X1: 270, Y1: 420, Label: "figure"},
			{X0: 5, Y0: 5, X1: 290, Y1: 55, Label: "title"},
		},
	}}

	// pdf.Section with a text-box-sized bbox (PDF points, 72 DPI).
	// In pixel space at scale 3: (60, 1200, 150, 1320) → (20, 400, 50, 440).
	// This overlaps with the "figure" DLA region.
	sec := pdf.Section{
		Positions: []pdf.Position{{
			PageNumbers: []int{0},
			Left:        20, Right: 50,
			Top: 400 / 3.0, Bottom: 440 / 3.0,
		}},
		LayoutType: "figure",
	}

	result := CropSectionByDLA(sec, dlaDebug, pageImages)
	if result == "" {
		t.Fatal("expected non-empty result for figure overlapping DLA region")
	}

	// Decode and verify.
	decoded, _ := base64.StdEncoding.DecodeString(result)
	img := decodePNG(t, decoded)
	// The DLA figure region is (30,60)-(270,420) with 3% margin.
	// Expected: ~(30-7.2, 60-10.8)-(270+7.2, 420+10.8) ≈ (22.8, 49.2)-(277.2, 430.8)
	// width ≈ 254px, height ≈ 381px
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	t.Logf("cropSectionByDLA result: %dx%d", w, h)
	if w < 200 || h < 300 {
		t.Errorf("unexpected crop size %dx%d, want >= 200x300 (DLA region based)", w, h)
	}
}

// TestCropSectionByDLA_NoMatch returns empty when no DLA region overlaps.
func TestCropSectionByDLA_NoMatch(t *testing.T) {
	pageImages := map[int]image.Image{
		0: makeTestPageImage(300, 450, color.RGBA{255, 0, 0, 255}),
	}
	dlaDebug := []pdf.DLAPageRegions{{
		Page: 0,
		Regions: []pdf.DLARegion{
			{X0: 10, Y0: 10, X1: 100, Y1: 50, Label: "title"},
			{X0: 10, Y0: 60, X1: 100, Y1: 100, Label: "text"},
		},
	}}
	// pdf.Section whose bbox doesn't overlap any figure/equation DLA region.
	sec := pdf.Section{
		Positions: []pdf.Position{{
			PageNumbers: []int{0},
			Left:        20, Right: 50, Top: 20, Bottom: 50,
		}},
		LayoutType: "figure",
	}
	result := CropSectionByDLA(sec, dlaDebug, pageImages)
	if result != "" {
		t.Errorf("expected empty result when no figure/equation DLA region found, got length %d", len(result))
	}
}

// TestCropSectionByDLA_EmptyInputs returns empty for edge cases.
func TestCropSectionByDLA_EmptyInputs(t *testing.T) {
	// Empty positions.
	if got := CropSectionByDLA(pdf.Section{}, nil, nil); got != "" {
		t.Error("expected empty for empty positions")
	}
	// Empty page numbers.
	sec := pdf.Section{Positions: []pdf.Position{{PageNumbers: nil}}}
	if got := CropSectionByDLA(sec, nil, nil); got != "" {
		t.Error("expected empty for empty page numbers")
	}
}
func TestCropImageRegion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 300))

	t.Run("normal crop", func(t *testing.T) {
		r := pdf.DLARegion{X0: 10, Y0: 20, X1: 100, Y1: 150}
		cropped, err := CropImageRegion(img, r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 3% proportional margin: 90×3%≈3px, 130×3%≈4px → 95×137
		if cropped.Bounds().Dx() != 95 || cropped.Bounds().Dy() != 137 {
			t.Errorf("size %v, want 95x137", cropped.Bounds())
		}
	})

	t.Run("x0 >= x1 returns error", func(t *testing.T) {
		// 3% proportional margin on each side: if the gap is too small after margin expansion, x0 ≥ x1 triggers error.
		r := pdf.DLARegion{X0: 110, Y0: 20, X1: 50, Y1: 150}
		_, err := CropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for x0 >= x1, got nil")
		}
	})

	t.Run("y0 >= y1 returns error", func(t *testing.T) {
		r := pdf.DLARegion{X0: 10, Y0: 150, X1: 100, Y1: 20}
		_, err := CropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for y0 >= y1, got nil")
		}
	})

	t.Run("region fully outside image bounds", func(t *testing.T) {
		// Clamped to image bounds → zero-width/height → error.
		r := pdf.DLARegion{X0: 300, Y0: 400, X1: 500, Y1: 600}
		_, err := CropImageRegion(img, r)
		if err == nil {
			t.Fatal("expected error for region outside image bounds")
		}
	})
}
