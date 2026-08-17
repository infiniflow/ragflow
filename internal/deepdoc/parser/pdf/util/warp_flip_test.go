package util

import (
	"image"
	"image/color"
	"testing"
)

// TestWarpCropAxisAlignedEarlyReturnsFastCrop pins the performance contract:
// an axis-parallel quad must take the cheap FastCrop path (direct Pix copy),
// not a full bicubic perspective warp. We use a sub-pixel axis-aligned quad so
// that, WITHOUT the early-return, WarpCrop resamples at sub-pixel positions
// and diverges from the integer-rounded FastCrop bbox (MSE > 0). WITH the
// early-return the two are pixel-identical (MSE == 0).
func TestWarpCropAxisAlignedEarlyReturnsFastCrop(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0, 255})
		}
	}
	// Axis-parallel quad with sub-pixel offset (still axis-aligned).
	pts := [4]Pt{{20.5, 30.5}, {120.5, 30.5}, {120.5, 130.5}, {20.5, 130.5}}

	got := WarpCrop(src, pts)
	// The early-return path rounds the corners with int() truncation and calls
	// FastCrop on the integer bbox, exactly as a caller would.
	want := FastCrop(src, 20, 30, 120, 130)

	if got.Bounds() != want.Bounds() {
		t.Fatalf("size mismatch: got=%v want=%v", got.Bounds(), want.Bounds())
	}
	mse := imageMSE(got, want)
	if mse > 1e-6 {
		t.Errorf("axis-aligned WarpCrop is not pixel-identical to FastCrop (early-return path missing?); MSE=%.6f", mse)
	}
}

// TestRotateIfVertical pins the vertical-text "layer 2" flip: a crop taller
// than wide by >= 1.5x is rotated 90° clockwise (matching Python deepdoc's
// get_rotate_crop_image heuristic); a wide or near-square crop is returned
// unchanged.
func TestRotateIfVertical(t *testing.T) {
	// Tall image (w=20, h=100): should be rotated CW 90° to 100x20.
	tall := image.NewRGBA(image.Rect(0, 0, 20, 100))
	tall.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})     // TL red
	tall.SetRGBA(19, 0, color.RGBA{0, 255, 0, 255})    // TR green
	tall.SetRGBA(0, 99, color.RGBA{0, 0, 255, 255})    // BL blue
	tall.SetRGBA(19, 99, color.RGBA{255, 255, 0, 255}) // BR yellow

	got := RotateIfVertical(tall)
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 20 {
		t.Fatalf("rotated size = %v, want 100x20", got.Bounds())
	}
	// CW 90° mapping (x,y) -> dst(h-1-y, x):
	//   TL(0,0)  -> dst(99,0) = right-top
	//   BL(0,99) -> dst(0,0)  = left-top
	if got.RGBAAt(99, 0) != tall.RGBAAt(0, 0) {
		t.Errorf("TL did not map to right-top after CW 90°")
	}
	if got.RGBAAt(0, 0) != tall.RGBAAt(0, 99) {
		t.Errorf("BL did not map to left-top after CW 90°")
	}

	// Wide image (w=100, h=20): not rotated.
	wide := image.NewRGBA(image.Rect(0, 0, 100, 20))
	wide.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	gotWide := RotateIfVertical(wide)
	if gotWide.Bounds() != wide.Bounds() {
		t.Errorf("wide image should be unchanged: got %v want %v", gotWide.Bounds(), wide.Bounds())
	}
	if !sameRGBA(gotWide, wide) {
		t.Errorf("wide image content changed after RotateIfVertical")
	}

	// Boundary: h/w == 1.5 flips; just below does not.
	flip := image.NewRGBA(image.Rect(0, 0, 100, 150)) // 150/100 = 1.5
	if RotateIfVertical(flip).Bounds().Dx() != 150 {
		t.Errorf("h/w==1.5 should flip (100x150 -> 150x100)")
	}
	noflip := image.NewRGBA(image.Rect(0, 0, 100, 149)) // 1.49
	if RotateIfVertical(noflip).Bounds().Dx() != 100 {
		t.Errorf("h/w<1.5 should not flip (still 100 wide)")
	}
}

// TestWarpCropForOCRVerticalFlip locks that WarpCropForOCR bundles the
// perspective de-skew (layer 1) with the vertical-text flip (layer 2), so a
// tall detection quad is fed to recognition rotated to horizontal, while a
// wide quad is left as-is (and, being axis-aligned, equals the old FastCrop).
func TestWarpCropForOCRVerticalFlip(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 0, 255})
		}
	}

	// Vertical text quad: tall thin box (width 20, height 100).
	tallQuad := [4]Pt{{40, 40}, {60, 40}, {60, 140}, {40, 140}}
	tall := WarpCropForOCR(src, tallQuad)
	if tall.Bounds().Dx() <= tall.Bounds().Dy() {
		t.Errorf("vertical quad should be flipped to wider-than-tall: got %v", tall.Bounds())
	}

	// Horizontal text quad: wide box (width 100, height 20). No flip.
	wideQuad := [4]Pt{{40, 40}, {140, 40}, {140, 60}, {40, 60}}
	wide := WarpCropForOCR(src, wideQuad)
	if wide.Bounds().Dx() < wide.Bounds().Dy() {
		t.Errorf("horizontal quad must not be flipped: got %v", wide.Bounds())
	}
	// For a wide, axis-aligned quad, WarpCropForOCR == WarpCrop (no flip,
	// early-return == FastCrop).
	if !sameRGBA(wide, WarpCrop(src, wideQuad)) {
		t.Errorf("horizontal WarpCropForOCR diverged from WarpCrop")
	}
}

func sameRGBA(a, b *image.RGBA) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	return imageMSE(a, b) == 0
}
