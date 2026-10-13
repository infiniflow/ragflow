package util

import (
	"image"
	"math"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// CropTextSectionPositions preserves whole rasterized lines around estimated
// text boundaries. Source positions stay unchanged; only preview pixels expand.
func CropTextSectionPositions(positions []pdf.Position, pages map[int]image.Image, zoom float64) string {
	plan, ok := buildSectionRasterPlan(positions, pages, zoom)
	if !ok {
		return ""
	}
	// Twelve PDF points cover nearby glyph edges at the render's scale, while
	// bounding searches on dense graphics or pages with no clear whitespace.
	radius := int(math.Ceil(12 * zoom))
	for i := range plan.segments {
		s := &plan.segments[i]
		if s.isEdge {
			continue
		}
		s.y0, _ = snapTextBoundary(pages[s.page], s.x0, s.x1, s.y0, -1, radius)
		s.y1, _ = snapTextBoundary(pages[s.page], s.x0, s.x1, s.y1, 1, radius)
	}
	// Expanding two cuts inside the same line may make their body crops
	// overlap. Render that line once within a preview.
	segments := make([]sectionRasterSegment, 0, len(plan.segments))
	columnTolerance := int(math.Ceil(2 * zoom))
	for _, s := range plan.segments {
		if len(segments) > 0 && !s.isEdge {
			prev := &segments[len(segments)-1]
			if !prev.isEdge && prev.page == s.page && sameTextColumn(*prev, s, columnTolerance) &&
				s.y0 <= prev.y1 && s.y1 >= prev.y0 {
				prev.x0, prev.x1 = min(prev.x0, s.x0), max(prev.x1, s.x1)
				prev.y0 = min(prev.y0, s.y0)
				prev.y1 = max(prev.y1, s.y1)
				continue
			}
		}
		segments = append(segments, s)
	}
	plan.segments = segments
	// Context meets the corrected body instead of skipping a fixed strip
	// through a neighbouring line. Its outer edges also retain whole lines.
	if len(segments) >= 3 {
		top, first := &segments[0], segments[1]
		last, bottom := segments[len(segments)-2], &segments[len(segments)-1]
		if y, ok := snapTextBoundary(pages[top.page], top.x0, top.x1, first.y0, -1, radius); ok {
			top.y1 = y
		}
		top.y0, _ = snapTextBoundary(pages[top.page], top.x0, top.x1, top.y0, -1, radius)
		if y, ok := snapTextBoundary(pages[bottom.page], bottom.x0, bottom.x1, last.y1, 1, radius); ok {
			bottom.y0 = y
		}
		bottom.y1, _ = snapTextBoundary(pages[bottom.page], bottom.x0, bottom.x1, bottom.y1, 1, radius)
	}
	if !measureSectionRasterPlan(plan, pages) {
		return ""
	}
	return encodeCroppedImage(renderSectionRaster(plan, pages))
}

// sameTextColumn allows small raster-boundary drift, while requiring horizontal
// overlap and nearly matching sides so independent columns stay separate.
func sameTextColumn(a, b sectionRasterSegment, tolerance int) bool {
	return a.x0 < b.x1 && b.x0 < a.x1 &&
		max(a.x0, b.x0)-min(a.x0, b.x0) <= tolerance &&
		max(a.x1, b.x1)-min(a.x1, b.x1) <= tolerance
}

// snapTextBoundary moves a cut outward to a nearby pair of clear rows. It
// leaves safe, out-of-page, and unresolvable boundaries alone. The boolean
// reports whether the result is a safe cut, including either page edge.
func snapTextBoundary(img image.Image, x0, x1, y, direction, radius int) (int, bool) {
	b := img.Bounds()
	x0, x1 = max(x0, b.Min.X), min(x1, b.Max.X)
	if x0 >= x1 || y <= b.Min.Y || y >= b.Max.Y {
		return y, x0 < x1 && (y == b.Min.Y || y == b.Max.Y)
	}
	blank := func(row int) bool {
		if row < b.Min.Y || row >= b.Max.Y {
			return true
		}
		for x := x0; x < x1; x++ {
			r, g, blue, a := img.At(x, row).RGBA()
			// Composite transparency over the white PDF page background.
			if r+65535-a < 245*257 || g+65535-a < 245*257 || blue+65535-a < 245*257 {
				return false
			}
		}
		return true
	}
	if blank(y-1) || blank(y) {
		return y, true
	}
	for distance := 1; distance <= radius; distance++ {
		candidate := y + direction*distance
		if candidate < b.Min.Y || candidate > b.Max.Y {
			break
		}
		if candidate == b.Min.Y || candidate == b.Max.Y {
			return candidate, true
		}
		if blank(candidate-1) && blank(candidate) {
			return candidate, true
		}
	}
	return y, false
}
