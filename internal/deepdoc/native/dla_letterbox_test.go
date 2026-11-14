//go:build cgo

package native

import "testing"

// TestDlaLetterboxChannelOrder verifies the DLA preprocessing feeds the layout
// model pixels in RGB order (channel 0 = red), matching the Python reference
// deepdoc pipeline. The Python LayoutRecognizer letterbox applies
// cv2.cvtColor(BGR2RGB) before the HWC->CHW transpose, so the YOLOv10 layout
// model is trained and inferred on RGB input. The Go image decoder yields a BGR
// raster (img.ToBGR), therefore the channel swap to RGB must happen inside
// dlaLetterbox.
//
// Regression guard: feeding BGR (channel 0 = blue) instead of RGB silently
// flips the colour channels. Grayscale pages are byte-identical so detection is
// unaffected, but a coloured region (e.g. a figure) sees swapped channels and
// the model's confidence for that box collapses -- the box can then fall under
// the 0.4 encode gate and vanish from the output.
func TestDlaLetterboxChannelOrder(t *testing.T) {
	const (
		blue  = 30
		green = 20
		red   = 10
	)
	// resized is BGR row-major, exactly as produced by
	// img.ToBGR() followed by bilinearResize.
	resized := []byte{blue, green, red}
	blob := dlaLetterbox(resized, 1, 1, 0, 0)

	// CHW layout: offset for channel c at (y,x) is c*W*H + y*W + x.
	at := func(c int) float32 {
		return blob[c*dlaInputSize*dlaInputSize+0*dlaInputSize+0]
	}
	if got := at(0); got != float32(red)/255.0 {
		t.Fatalf("channel 0 (must be RED) = %v, want %v -- RGB order required; Python ref does BGR2RGB", got, float32(red)/255.0)
	}
	if got := at(1); got != float32(green)/255.0 {
		t.Fatalf("channel 1 (must be GREEN) = %v, want %v", got, float32(green)/255.0)
	}
	if got := at(2); got != float32(blue)/255.0 {
		t.Fatalf("channel 2 (must be BLUE) = %v, want %v", got, float32(blue)/255.0)
	}
}
