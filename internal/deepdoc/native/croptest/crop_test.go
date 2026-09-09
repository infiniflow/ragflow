//go:build cgo

// Package croptest holds regression tests for the pure image-raster helpers
// in the parent native package that do NOT require the DeepDoc testdata
// (ONNX models / fixtures). The parent package's TestMain unconditionally
// skips the whole test binary when that testdata has not been fetched, so a
// pure-logic test placed there would never run in the default build. Keeping
// these tests in a separate package yields an independent test binary that
// runs under `go test ./...` without any external assets.
package croptest

import (
	"image"
	"image/color"
	"testing"

	"ragflow/internal/deepdoc/native"
)

// fillPattern writes a distinct colour to every pixel so a wrong raster
// offset shows up as a value mismatch, not just as a panic.
func fillPattern(img *image.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((x*3 + y) & 0xff),
				G: uint8((y*5 + 1) & 0xff),
				B: uint8((x + y*2) & 0xff),
				A: 255,
			})
		}
	}
}

// TestFromImageOrigin asserts FromImage handles images whose Bounds().Min is
// not the origin. OCR feeds cropped sub-images (SubImage returns a rectangle
// with a non-zero Min), and indexing the RGBA raster with 0-based coordinates
// while RGBA.PixOffset subtracts Rect.Min yields a negative offset (panic).
func TestFromImageOrigin(t *testing.T) {
	const srcW, srcH = 40, 30

	cases := []struct {
		name string
		rect image.Rectangle
	}{
		{"zero origin", image.Rect(0, 0, 20, 15)},
		{"non-zero origin", image.Rect(7, 5, 27, 20)},
		{"origin offset only in Y", image.Rect(0, 9, 12, 24)},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := image.NewRGBA(image.Rect(0, 0, srcW, srcH))
			fillPattern(src)

			got, err := native.FromImage(src.SubImage(c.rect))
			if err != nil {
				t.Fatalf("FromImage: unexpected error: %v", err)
			}
			if got.W != c.rect.Dx() || got.H != c.rect.Dy() {
				t.Fatalf("size = %dx%d, want %dx%d", got.W, got.H, c.rect.Dx(), c.rect.Dy())
			}
			if want := got.W * got.H * 3; len(got.Pix) != want {
				t.Fatalf("len(Pix) = %d, want %d", len(got.Pix), want)
			}
			for y := 0; y < got.H; y++ {
				for x := 0; x < got.W; x++ {
					want := src.RGBAAt(c.rect.Min.X+x, c.rect.Min.Y+y)
					d := (y*got.W + x) * 3
					if got.Pix[d] != want.R || got.Pix[d+1] != want.G || got.Pix[d+2] != want.B {
						t.Fatalf("pixel (%d,%d) = (%d,%d,%d), want (%d,%d,%d)",
							x, y, got.Pix[d], got.Pix[d+1], got.Pix[d+2], want.R, want.G, want.B)
					}
				}
			}
		})
	}
}
