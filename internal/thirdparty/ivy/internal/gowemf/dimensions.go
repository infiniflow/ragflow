// Copied from github.com/mgilbir/gowemf, file dimensions.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

// SizeInPoints reports the recorded physical image size, in 1/72-inch points.
// EMF uses its .01-mm frame; placeable WMF uses logical units per inch. Standard
// WMF has no reliable physical size and returns ErrUnsupported. Inverted bounds
// retain their meaning in Header but dimensions here are positive magnitudes.
func (h Header) SizeInPoints() (width, height float64, err error) {
	var x, y int64
	var factor float64
	if h.Format == EMF && h.EMF != nil {
		r := h.EMF.Frame
		x = int64(r.Right) - int64(r.Left)
		y = int64(r.Bottom) - int64(r.Top)
		factor = 72 / 2540.0
	} else if h.Format == WMF && h.Placeable != nil && h.Placeable.UnitsPerInch != 0 {
		r := h.Placeable.Bounds
		x = int64(r.Right) - int64(r.Left)
		y = int64(r.Bottom) - int64(r.Top)
		factor = 72 / float64(h.Placeable.UnitsPerInch)
	} else {
		return 0, 0, failure(0, "physical metafile size", ErrUnsupported)
	}
	if x < 0 {
		x = -x
	}
	if y < 0 {
		y = -y
	}
	if x == 0 || y == 0 {
		return 0, 0, malformed(0, "empty physical frame")
	}
	return float64(x) * factor, float64(y) * factor, nil
}
