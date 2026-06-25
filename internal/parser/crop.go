package parser

import (
	"encoding/base64"
	"image"
	"image/color"
	"math"
)

// cropSectionImage crops region(s) from rendered page images based on a
// position tag and returns a base64-encoded PNG.  Returns "" if cropping
// is not possible (missing images, out-of-bounds, invalid tag).
//
// Python: pdf_parser.py:1802 RAGFlowPdfParser.crop()
func cropSectionImage(posTag string, decodedImages map[int]image.Image, zoom float64) string {
	if len(decodedImages) == 0 {
		return ""
	}

	positions := ExtractPositions(posTag)
	if len(positions) == 0 {
		return ""
	}

	// Filter valid positions (all pages available).
	var valid []Position
	for _, pos := range positions {
		allValid := true
		for _, pn := range pos.PageNumbers {
			if _, ok := decodedImages[pn]; !ok {
				allValid = false
				break
			}
		}
		if allValid {
			valid = append(valid, pos)
		}
	}
	if len(valid) == 0 {
		return ""
	}

	// Page images are pre-decoded by the caller (Parse).

	// Context padding (Python: 120px above first, 120 below last, 6px gap)
	const contextPad = 120.0
	const gap = 6

	// Compute max width across original positions for full-width edge bands.
	maxWidth := 6.0
	for _, pos := range valid {
		w := pos.Right - pos.Left
		if w > maxWidth {
			maxWidth = w
		}
	}

	// Python-style: insert synthetic context bands at edges.
	// Original positions are all middle entries (narrow width).
	// Synthetic bands are edge entries (full width + semi-transparent overlay).
	first := valid[0]
	last := valid[len(valid)-1]
	firstPageIdx := first.PageNumbers[0]
	lastPageIdx := last.PageNumbers[len(last.PageNumbers)-1]
	lastPageH := float64(decodedImages[lastPageIdx].Bounds().Dy()) / zoom

	// topBand: 120px context above the first content position.
	topBandPos := Position{
		PageNumbers: []int{firstPageIdx},
		Left:        first.Left,
		Right:       first.Right,
		Top:         math.Max(0, first.Top-contextPad),
		Bottom:      math.Max(first.Top-gap, 0),
	}
	// bottomBand: 120px context below the last content position.
	bottomBandPos := Position{
		PageNumbers: []int{lastPageIdx},
		Left:        last.Left,
		Right:       last.Right,
		Top:         math.Min(lastPageH, last.Bottom+gap),
		Bottom:      math.Min(lastPageH, last.Bottom+contextPad),
	}

	// Build entry list: [topBand, original positions..., bottomBand].
	type segment struct {
		img    image.Image
		isEdge bool
	}
	var segments []segment

	allPos := make([]struct {
		pos    Position
		isEdge bool
	}, 0, len(valid)+2)
	allPos = append(allPos, struct {
		pos    Position
		isEdge bool
	}{topBandPos, true})
	for _, pos := range valid {
		allPos = append(allPos, struct {
			pos    Position
			isEdge bool
		}{pos, false})
	}
	allPos = append(allPos, struct {
		pos    Position
		isEdge bool
	}{bottomBandPos, true})

	for _, entry := range allPos {
		pos := entry.pos
		isEdge := entry.isEdge

		top := pos.Top
		bottom := pos.Bottom
		left := pos.Left
		right := pos.Right

		// Width: edge segments are full-width, middle are narrow.
		if !isEdge {
			right = math.Max(left+10, right)
		} else {
			right = left + maxWidth
		}

		pn0 := pos.PageNumbers[0]

		// Accumulate bottom for multi-page positions.
		accumBottom := bottom * zoom
		for _, pn := range pos.PageNumbers[1:] {
			if pn == pn0 {
				continue
			}
			if img, ok := decodedImages[pn]; ok {
				accumBottom += float64(img.Bounds().Dy())
			}
		}

		pageImg, ok := decodedImages[pn0]
		if !ok {
			return ""
		}
		pageH := float64(pageImg.Bounds().Dy())
		bottomClamped := math.Min(accumBottom, pageH)

		// Crop first page of this position.
		cropped := fastCrop(pageImg,
			int(left*zoom), int(top*zoom),
			int(right*zoom), int(bottomClamped))
		if isEdge {
			cropped = applyEdgeOverlay(cropped)
		}
		segments = append(segments, segment{img: cropped, isEdge: isEdge})

		// Subsequent pages (only those different from the first page).
		bottomRemaining := accumBottom - pageH
		for _, pn := range pos.PageNumbers[1:] {
			if pn == pn0 {
				continue
			}
			pageImg2, ok := decodedImages[pn]
			if !ok {
				return ""
			}
			pageH2 := float64(pageImg2.Bounds().Dy())
			bottomClamped2 := math.Min(bottomRemaining, pageH2)
			cropped2 := fastCrop(pageImg2,
				int(left*zoom), 0,
				int(right*zoom), int(bottomClamped2))
			if isEdge {
				cropped2 = applyEdgeOverlay(cropped2)
			}
			segments = append(segments, segment{img: cropped2, isEdge: isEdge})
			bottomRemaining -= bottomClamped2
		}
	}

	if len(segments) == 0 {
		return ""
	}

	// Stitch vertically with gray background and 6px gaps.
	totalH := 0
	maxW := 0
	for _, seg := range segments {
		totalH += seg.img.Bounds().Dy() + gap
		maxW = max(maxW, seg.img.Bounds().Dx())
	}
	stitched := image.NewRGBA(image.Rect(0, 0, maxW, totalH))
	for y := 0; y < totalH; y++ {
		for x := 0; x < maxW; x++ {
			stitched.Set(x, y, color.RGBA{245, 245, 245, 255})
		}
	}
	curY := 0
	for _, seg := range segments {
		srcW := seg.img.Bounds().Dx()
		srcH := seg.img.Bounds().Dy()
		for y := 0; y < srcH; y++ {
			for x := 0; x < srcW; x++ {
				stitched.Set(x, curY+y, seg.img.At(x+seg.img.Bounds().Min.X, y+seg.img.Bounds().Min.Y))
			}
		}
		curY += srcH + gap
	}

	data, err := encodePNG(stitched)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// cropSectionByDLA crops a section using the best-overlapping DLA region.
// It finds a DLA "figure" or "equation" region whose overlap with the section's
// bounding box is maximal, then crops from the page image at 216 DPI using the
// DLA region boundary (plus 3% margin via cropImageRegion).
//
// Returns "" (empty string) if no matching DLA region or page image is found.
// The caller should fall through to cropSectionImage as a fallback.
//
// Python equivalent: cropout() in pdf_parser.py:1144-1148
//
//	louts = [layout for layout in self.page_layout[pn] if layout["type"] == ltype]
//	ii = Recognizer.find_overlapped(b, louts, naive=True)
//	if ii is not None: b = louts[ii]
func cropSectionByDLA(sec Section, dlaDebug []DLAPageRegions, pageImages map[int]image.Image) string {
	if len(sec.Positions) == 0 || len(sec.Positions[0].PageNumbers) == 0 {
		return ""
	}
	pg := sec.Positions[0].PageNumbers[0]
	pos := sec.Positions[0]

	// Find DLA regions for this page.
	var regions []DLARegion
	for _, dp := range dlaDebug {
		if dp.Page == pg {
			regions = dp.Regions
			break
		}
	}
	if len(regions) == 0 {
		return ""
	}

	// Convert section bbox from PDF points (72 DPI) to DLA pixel space (216 DPI).
	scale := dlaDPI / 72.0 // 3.0
	bx := rect{
		x0: pos.Left * scale,
		y0: pos.Top * scale,
		x1: pos.Right * scale,
		y1: pos.Bottom * scale,
	}

	// Find best-overlapping figure or equation DLA region.
	bestIdx := -1
	bestOverlap := 0.0
	for i, r := range regions {
		if r.Label != "figure" && r.Label != "equation" {
			continue
		}
		overlap := rectOverlap(bx, rect{r.X0, r.Y0, r.X1, r.Y1})
		if overlap > bestOverlap {
			bestOverlap = overlap
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return ""
	}

	img, ok := pageImages[pg]
	if !ok {
		return ""
	}
	cropped, err := cropImageRegion(img, regions[bestIdx])
	if err != nil {
		return ""
	}
	data, err := encodePNG(cropped)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// applyEdgeOverlay applies a semi-transparent black overlay to the image,
// matching Python's self.crop edge-segment treatment:
//
//	img.convert("RGBA")
//	overlay = Image.new("RGBA", img.size, (0,0,0,0))
//	overlay.putalpha(128)
//	img = Image.alpha_composite(img, overlay).convert("RGB")
func applyEdgeOverlay(img image.Image) *image.RGBA {
	b := img.Bounds()
	result := image.NewRGBA(b)
	const overlayAlpha = 128 // ~50% opacity black overlay
	factor := 1.0 - float64(overlayAlpha)/255.0
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bb, a := img.At(x+b.Min.X, y+b.Min.Y).RGBA()
			r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(bb>>8), uint8(a>>8)
			result.Set(x, y, color.RGBA{
				R: uint8(float64(r8) * factor),
				G: uint8(float64(g8) * factor),
				B: uint8(float64(b8) * factor),
				A: a8,
			})
		}
	}
	return result
}

// rotateCoordCW returns the clockwise-rotated coordinates of (x, y) for the
// given original dimensions and angle. Only 0/90/180/270 are meaningful;
// other values are passed through unchanged.
func rotateCoordCW(x, y float64, origW, origH int, angle int) (float64, float64) {
	switch angle {
	case 0:
		return x, y
	case 90:
		return float64(origH-1) - y, x
	case 180:
		return float64(origW-1) - x, float64(origH-1) - y
	case 270:
		return y, float64(origW-1) - x
	default:
		return x, y
	}
}

// rotateImageCW rotates an image clockwise. Only 0/90/180/270 supported;
// other values return nil. Matches Python PIL.Image.rotate(-angle, expand=True).
func rotateImageCW(img image.Image, angle int) *image.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	dstW, dstH := w, h
	switch angle {
	case 90, 270:
		dstW, dstH = h, w
	case 0, 180:
		// keep w, h
	default:
		return nil
	}

	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := rotateCoordCW(float64(x), float64(y), w, h, angle)
			dst.Set(int(dx), int(dy), img.At(x+b.Min.X, y+b.Min.Y))
		}
	}
	return dst
}

// mapRotatedPointToOriginal maps a point from rotated image coords back to
// original coords. angle is the clockwise rotation applied. origW, origH
// are the ORIGINAL (pre-rotation) image dimensions.
//
// Python: pdf_parser.py:602 _map_rotated_point()
func mapRotatedPointToOriginal(x, y float64, angle int, origW, origH int) (float64, float64) {
	switch angle {
	case 0:
		return x, y
	case 90:
		// rotateImageCW 90°: (ox,oy) → (origH-1-oy, ox) = (rx,ry).
		// Inverse: ox = ry, oy = origH-1 - rx.
		return y, float64(origH) - 1 - x
	case 180:
		// rotateImageCW 180°: (ox,oy) → (origW-1-ox, origH-1-oy).
		// Inverse: ox = origW-1 - rx, oy = origH-1 - ry.
		return float64(origW) - 1 - x, float64(origH) - 1 - y
	case 270:
		// rotateImageCW 270°: (ox,oy) → (oy, origW-1-ox) = (rx,ry).
		// Inverse: ox = origW-1 - ry, oy = rx.
		return float64(origW) - 1 - y, x
	default:
		return x, y
	}
}
