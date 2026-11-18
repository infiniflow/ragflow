//go:build cgo

package native

// geometry.go — shared geometric primitives.
//
// Only the pieces that are genuinely common live here (bilinear resize with
// the same center-aligned mapping as cv2.INTER_LINEAR). Per-task transforms
// (letterbox padding, aspect-ratio stretch, recognition padding) are kept in
// each recognizer so responsibilities don't overlap.

import "math"

// bilinearResize resizes an HxWx3 image (channel order is caller's concern;
// the function resizes each channel independently) to outW x outH using the
// same coordinate mapping as cv2.INTER_LINEAR: dst = (src + 0.5) * scale - 0.5.
func bilinearResize(src []byte, sw, sh, outW, outH int) []byte {
	dst := make([]byte, outW*outH*3)
	if sw == 0 || sh == 0 {
		return dst
	}
	scaleX := float64(sw) / float64(outW)
	scaleY := float64(sh) / float64(outH)
	for y := 0; y < outH; y++ {
		sy := (float64(y)+0.5)*scaleY - 0.5
		y0, y1, wy0, wy1 := lerp(sy, sh)
		for x := 0; x < outW; x++ {
			sx := (float64(x)+0.5)*scaleX - 0.5
			x0, x1, wx0, wx1 := lerp(sx, sw)
			for c := 0; c < 3; c++ {
				v0 := float64(src[(y0*sw+x0)*3+c])
				v1 := float64(src[(y0*sw+x1)*3+c])
				v2 := float64(src[(y1*sw+x0)*3+c])
				v3 := float64(src[(y1*sw+x1)*3+c])
				val := wy0*(wx0*v0+wx1*v1) + wy1*(wx0*v2+wx1*v3)
				if val < 0 {
					val = 0
				} else if val > 255 {
					val = 255
				}
				dst[(y*outW+x)*3+c] = byte(val + 0.5)
			}
		}
	}
	return dst
}

func lerp(s float64, max int) (i0, i1 int, w0, w1 float64) {
	i0 = int(math.Floor(s))
	w1 = s - float64(i0)
	w0 = 1 - w1
	i1 = i0 + 1
	if i0 < 0 {
		i0 = 0
	}
	if i1 < 0 {
		i1 = 0
	}
	if i0 >= max {
		i0 = max - 1
	}
	if i1 >= max {
		i1 = max - 1
	}
	return
}
