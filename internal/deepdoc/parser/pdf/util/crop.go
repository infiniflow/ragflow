package util

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"math"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// CropSectionImage crops region(s) from rendered page images based on a
// position tag and returns a base64-encoded PNG.  Returns "" if cropping
// is not possible (missing images, out-of-bounds, invalid tag).
//
// Python: pdf_parser.py:1802 RAGFlowPdfParser.crop()
func CropSectionImage(posTag string, decodedImages map[int]image.Image, zoom float64) string {
	if len(decodedImages) == 0 {
		slog.Warn("cropSectionImage: no page images available, skipping image generation")
		return ""
	}

	positions := ExtractPositions(posTag)
	if len(positions) == 0 {
		slog.Warn("cropSectionImage: empty position list in tag", "posTag", posTag[:min(80, len(posTag))])
		return ""
	}

	// Filter valid positions (all pages available).
	var valid []pdf.Position
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
		slog.Warn("cropSectionImage: no valid positions after filtering, skipping crop")
		return ""
	}

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
	topBandPos := pdf.Position{
		PageNumbers: []int{firstPageIdx},
		Left:        first.Left,
		Right:       first.Right,
		Top:         math.Max(0, first.Top-contextPad),
		Bottom:      math.Max(first.Top-gap, 0),
	}
	// bottomBand: 120px context below the last content position.
	bottomBandPos := pdf.Position{
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
		pos    pdf.Position
		isEdge bool
	}, 0, len(valid)+2)
	allPos = append(allPos, struct {
		pos    pdf.Position
		isEdge bool
	}{topBandPos, true})
	for _, pos := range valid {
		allPos = append(allPos, struct {
			pos    pdf.Position
			isEdge bool
		}{pos, false})
	}
	allPos = append(allPos, struct {
		pos    pdf.Position
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
			slog.Warn("cropSectionImage: page image not found", "page", pn0)
			return ""
		}
		pageH := float64(pageImg.Bounds().Dy())
		bottomClamped := math.Min(accumBottom, pageH)

		// Crop first page of this position.
		cropped := FastCrop(pageImg,
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
				slog.Warn("cropSectionImage: page image not found for subsequent page", "page", pn)
				return ""
			}
			pageH2 := float64(pageImg2.Bounds().Dy())
			bottomClamped2 := math.Min(bottomRemaining, pageH2)
			cropped2 := FastCrop(pageImg2,
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

	// Fill background using direct Pix slice write (matching fastCrop pattern).
	// Gray 245,245,245,255 as BGRA bytes.
	for y := 0; y < totalH; y++ {
		row := stitched.Pix[stitched.PixOffset(0, y):stitched.PixOffset(maxW, y)]
		for i := 0; i < len(row); i += 4 {
			row[i] = 245   // B
			row[i+1] = 245 // G
			row[i+2] = 245 // R
			row[i+3] = 255 // A
		}
	}

	curY := 0
	for _, seg := range segments {
		srcW := seg.img.Bounds().Dx()
		srcH := seg.img.Bounds().Dy()
		if rgba, ok := seg.img.(*image.RGBA); ok {
			// Fast path: direct Pix slice copy (matching fastCrop in geometry.go).
			srcMinX := seg.img.Bounds().Min.X
			srcMinY := seg.img.Bounds().Min.Y
			for ry := 0; ry < srcH; ry++ {
				srcStart := rgba.PixOffset(srcMinX, srcMinY+ry)
				srcRow := rgba.Pix[srcStart : srcStart+srcW*4]
				dstStart := stitched.PixOffset(0, curY+ry)
				copy(stitched.Pix[dstStart:], srcRow)
			}
		} else {
			// Fallback: pixel-by-pixel for non-RGBA images (e.g. edge overlays).
			for y := 0; y < srcH; y++ {
				for x := 0; x < srcW; x++ {
					stitched.Set(x, curY+y, seg.img.At(x+seg.img.Bounds().Min.X, y+seg.img.Bounds().Min.Y))
				}
			}
		}
		curY += srcH + gap
	}

	data, err := EncodePNG(stitched)
	if err != nil {
		slog.Warn("cropSectionImage: PNG encode failed", "err", err)
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
func CropSectionByDLA(sec pdf.Section, dlaDebug []pdf.DLAPageRegions, pageImages map[int]image.Image) string {
	if len(sec.Positions) == 0 || len(sec.Positions[0].PageNumbers) == 0 {
		return ""
	}
	pg := sec.Positions[0].PageNumbers[0]
	pos := sec.Positions[0]

	// Find DLA regions for this page.
	var regions []pdf.DLARegion
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
	scale := pdf.DlaDPI / 72.0 // 3.0
	bx := Rect{
		X0: pos.Left * scale,
		Y0: pos.Top * scale,
		X1: pos.Right * scale,
		Y1: pos.Bottom * scale,
	}

	// Find best-overlapping figure or equation DLA region.
	bestIdx := -1
	bestOverlap := 0.0
	for i, r := range regions {
		if r.Label != pdf.LayoutTypeFigure && r.Label != pdf.LayoutTypeEquation {
			continue
		}
		overlap := RectOverlap(bx, Rect{r.X0, r.Y0, r.X1, r.Y1})
		if overlap > bestOverlap {
			bestOverlap = overlap
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		slog.Warn("cropSectionByDLA: no matching layout region found", "page", pg)
		return ""
	}

	img, ok := pageImages[pg]
	if !ok {
		return ""
	}
	cropped, err := CropImageRegion(img, regions[bestIdx])
	if err != nil {
		slog.Warn("cropSectionByDLA: cropImageRegion failed", "page", pg, "err", err)
		return ""
	}
	data, err := EncodePNG(cropped)
	if err != nil {
		slog.Warn("cropSectionByDLA: PNG encode failed", "err", err)
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
func RotateImageCW(img image.Image, angle int) *image.RGBA {
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
func MapRotatedPointToOriginal(x, y float64, angle int, origW, origH int) (float64, float64) {
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

// CropImageRegion crops a pdf.DLARegion from an image with a 3% margin
// (matching Python's _table_transformer_job: w*0.03, h*0.03).
func CropImageRegion(img image.Image, r pdf.DLARegion) (image.Image, error) {
	w := r.X1 - r.X0
	h := r.Y1 - r.Y0
	marginX := w * 0.03
	marginY := h * 0.03
	maxX := float64(img.Bounds().Dx())
	maxY := float64(img.Bounds().Dy())
	x0 := int(math.Max(0, r.X0-marginX))
	y0 := int(math.Max(0, r.Y0-marginY))
	x1 := int(math.Min(maxX, r.X1+marginX))
	y1 := int(math.Min(maxY, r.Y1+marginY))
	// Python PIL.Image.crop() raises ValueError when right < left or
	// bottom < top.  We return an error instead of silently falling back
	// to the full-page image — the caller skips this table gracefully.
	if x0 >= x1 || y0 >= y1 {
		return nil, fmt.Errorf("crop: invalid region x0=%d y0=%d x1=%d y1=%d (DLA raw: %.1f,%.1f,%.1f,%.1f)",
			x0, y0, x1, y1, r.X0, r.Y0, r.X1, r.Y1)
	}
	cropped := FastCrop(img, x0, y0, x1, y1)
	return cropped, nil
}
