package util

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"math"

	"go.uber.org/zap"

	"ragflow/internal/common"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// CropSectionImage crops region(s) from rendered page images based on a
// position tag and returns a base64-encoded PNG.  Returns "" if cropping
// is not possible (missing images, out-of-bounds, invalid tag).
//
// Python: pdf_parser.py:1802 RAGFlowPdfParser.crop()
func CropSectionImage(posTag string, decodedImages map[int]image.Image, zoom float64) string {
	return encodeCroppedImage(cropSectionImageRaster(posTag, decodedImages, zoom))
}

func cropSectionImageRaster(posTag string, decodedImages map[int]image.Image, zoom float64) image.Image {
	if len(decodedImages) == 0 {
		common.Warn("cropSectionImage: no page images available, skipping image generation")
		return nil
	}

	positions := ExtractPositions(posTag)
	if len(positions) == 0 {
		common.Warn("cropSectionImage: empty position list in tag", zap.String("posTag", posTag[:min(80, len(posTag))]))
		return nil
	}
	return cropSectionPositionsRaster(positions, decodedImages, zoom)
}

type sectionRasterSegment struct {
	page   int
	x0     int
	y0     int
	x1     int
	y1     int
	isEdge bool
}

type sectionRasterPlan struct {
	segments []sectionRasterSegment
	width    int
	height   int
}

const (
	sectionRasterContextPad = 120.0
	sectionRasterGap        = 6
)

func cropSectionPositionsRaster(positions []pdf.Position, decodedImages map[int]image.Image, zoom float64) image.Image {
	plan, ok := buildSectionRasterPlan(positions, decodedImages, zoom)
	if !ok {
		return nil
	}
	return renderSectionRaster(plan, decodedImages)
}

func buildSectionRasterPlan(positions []pdf.Position, decodedImages map[int]image.Image, zoom float64) (*sectionRasterPlan, bool) {
	if len(decodedImages) == 0 || len(positions) == 0 || zoom <= 0 || math.IsNaN(zoom) || math.IsInf(zoom, 0) {
		return nil, false
	}
	valid := make([]pdf.Position, 0, len(positions))
	for _, pos := range positions {
		if len(pos.PageNumbers) == 0 {
			continue
		}
		allValid := true
		for _, pageNum := range pos.PageNumbers {
			if img, ok := decodedImages[pageNum]; !ok || img == nil {
				allValid = false
				break
			}
		}
		if allValid {
			valid = append(valid, pos)
		}
	}
	if len(valid) == 0 {
		return nil, false
	}

	maxWidth := 6.0
	for _, pos := range valid {
		if width := pos.Right - pos.Left; width > maxWidth {
			maxWidth = width
		}
	}

	first := valid[0]
	last := valid[len(valid)-1]
	firstPage := first.PageNumbers[0]
	lastPage := last.PageNumbers[len(last.PageNumbers)-1]
	lastPageHeight := float64(decodedImages[lastPage].Bounds().Dy()) / zoom
	topBand := pdf.Position{
		PageNumbers: []int{firstPage},
		Left:        first.Left,
		Right:       first.Right,
		Top:         math.Max(0, first.Top-sectionRasterContextPad),
		Bottom:      math.Max(first.Top-sectionRasterGap, 0),
	}
	bottomBand := pdf.Position{
		PageNumbers: []int{lastPage},
		Left:        last.Left,
		Right:       last.Right,
		Top:         math.Min(lastPageHeight, last.Bottom+sectionRasterGap),
		Bottom:      math.Min(lastPageHeight, last.Bottom+sectionRasterContextPad),
	}

	type positionEntry struct {
		position pdf.Position
		isEdge   bool
	}
	entries := make([]positionEntry, 0, len(valid)+2)
	entries = append(entries, positionEntry{position: topBand, isEdge: true})
	for _, pos := range valid {
		entries = append(entries, positionEntry{position: pos})
	}
	entries = append(entries, positionEntry{position: bottomBand, isEdge: true})

	plan := &sectionRasterPlan{}
	maxInt := int(^uint(0) >> 1)
	for _, entry := range entries {
		pos := entry.position
		left, right := pos.Left, pos.Right
		if entry.isEdge {
			right = left + maxWidth
		} else {
			right = math.Max(left+10, right)
		}
		firstPage := pos.PageNumbers[0]
		accumBottom := pos.Bottom * zoom
		for _, pageNum := range pos.PageNumbers[1:] {
			if pageNum != firstPage {
				accumBottom += float64(decodedImages[pageNum].Bounds().Dy())
			}
		}

		pageImage := decodedImages[firstPage]
		pageHeight := float64(pageImage.Bounds().Dy())
		bottomClamped := math.Min(accumBottom, pageHeight)
		plan.addCrop(firstPage, int(left*zoom), int(pos.Top*zoom), int(right*zoom), int(bottomClamped), entry.isEdge)

		bottomRemaining := accumBottom - pageHeight
		for _, pageNum := range pos.PageNumbers[1:] {
			if pageNum == firstPage {
				continue
			}
			if bottomRemaining <= 0 {
				break
			}
			pageImage := decodedImages[pageNum]
			bottomClamped := math.Min(bottomRemaining, float64(pageImage.Bounds().Dy()))
			bottomPixels := int(bottomClamped)
			if bottomPixels <= 0 {
				break
			}
			plan.addCrop(pageNum, int(left*zoom), 0, int(right*zoom), bottomPixels, entry.isEdge)
			bottomRemaining -= bottomClamped
		}
	}
	if len(plan.segments) == 0 {
		return nil, false
	}
	for _, segment := range plan.segments {
		width, height := cropSize(decodedImages[segment.page], segment.x0, segment.y0, segment.x1, segment.y1)
		plan.width = max(plan.width, width)
		if height > maxInt-sectionRasterGap || plan.height > maxInt-(height+sectionRasterGap) {
			return nil, false
		}
		plan.height += height + sectionRasterGap
	}
	return plan, plan.width > 0 && plan.height > 0
}

func (p *sectionRasterPlan) addCrop(page, x0, y0, x1, y1 int, isEdge bool) {
	p.segments = append(p.segments, sectionRasterSegment{
		page: page, x0: x0, y0: y0, x1: x1, y1: y1, isEdge: isEdge,
	})
}

func cropSize(img image.Image, x0, y0, x1, y1 int) (int, int) {
	bounds := cropRectBounds(img, x0, y0, x1, y1)
	if bounds.Empty() {
		return 1, 1
	}
	return bounds.Dx(), bounds.Dy()
}

func renderSectionRaster(plan *sectionRasterPlan, decodedImages map[int]image.Image) image.Image {
	segments := make([]image.Image, 0, len(plan.segments))
	for _, segment := range plan.segments {
		cropped := FastCrop(decodedImages[segment.page], segment.x0, segment.y0, segment.x1, segment.y1)
		if segment.isEdge {
			cropped = applyEdgeOverlay(cropped)
		}
		segments = append(segments, cropped)
	}
	stitched := image.NewRGBA(image.Rect(0, 0, plan.width, plan.height))
	for y := 0; y < plan.height; y++ {
		row := stitched.Pix[stitched.PixOffset(0, y):stitched.PixOffset(plan.width, y)]
		for i := 0; i < len(row); i += 4 {
			row[i], row[i+1], row[i+2], row[i+3] = 245, 245, 245, 255
		}
	}

	curY := 0
	for _, segment := range segments {
		srcW, srcH := segment.Bounds().Dx(), segment.Bounds().Dy()
		if rgba, ok := segment.(*image.RGBA); ok {
			srcMinX, srcMinY := segment.Bounds().Min.X, segment.Bounds().Min.Y
			for row := 0; row < srcH; row++ {
				srcStart := rgba.PixOffset(srcMinX, srcMinY+row)
				srcRow := rgba.Pix[srcStart : srcStart+srcW*4]
				dstStart := stitched.PixOffset(0, curY+row)
				copy(stitched.Pix[dstStart:], srcRow)
			}
		} else {
			for y := 0; y < srcH; y++ {
				for x := 0; x < srcW; x++ {
					stitched.Set(x, curY+y, segment.At(x+segment.Bounds().Min.X, y+segment.Bounds().Min.Y))
				}
			}
		}
		curY += srcH + sectionRasterGap
	}
	return stitched
}

func rasterPlanExceedsLimit(plan *sectionRasterPlan, maxPixels int64) bool {
	if plan == nil || plan.width <= 0 || plan.height <= 0 || maxPixels <= 0 {
		return true
	}
	return int64(plan.width) > maxPixels/int64(plan.height)
}

func encodeCroppedImage(img image.Image) string {
	if img == nil {
		return ""
	}
	data, err := EncodePNG(img)
	if err != nil {
		common.Warn("cropSectionImage: PNG encode failed", zap.Error(err))
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

// CropSectionByDLA crops a section using the best-overlapping DLA region,
// mimicking Python's cropout() in deepdoc/parser/pdf_parser.py (around line
// 1307). Unlike the original Go version (which only cropped the first page),
// it now walks every position and every page the section spans, crops each
// page's best DLA region (or the section bbox as a fallback), and
// vertically concatenates the per-page crops — exactly like cropout's
// multi-page branch (Image.new("RGB", (...), (245,245,245)) + paste loop).
//
// Python equivalent (single-page branch):
//
//	louts = [layout for layout in self.page_layout[pn] if layout["type"] == ltype]
//	ii = Recognizer.find_overlapped(b, louts, naive=True)
//	if ii is not None:
//	    b = louts[ii]
//
// find_overlapped ranks candidates by overlapped_area(layout, section,
// ratio=True), i.e. intersection_area / Area(layout). We replicate that exact
// metric with OverlapRatioA(region, bx) — NOT the symmetric OverlapRatioMax —
// so the chosen region matches Python even when the section box is larger than
// the candidate region.
//
// Fallback semantics match cropout too: when a page has no figure/equation DLA
// region (find_overlapped returns None), cropout crops the section's own bbox
// on that page; we do the same via cropDLAPage. We return "" (empty string)
// only when the section has no positions/pages at all or a referenced page
// image is missing — in which case the caller should fall through to
// CropSectionImage for the whole section.
//
// Note: cropout does NOT add the 120px context bands that CropSectionImage
// (Python crop()) inserts; matching cropout, this function returns clean
// per-page crops with no edge bands.
func CropSectionByDLA(sec pdf.Section, dlaRegions []pdf.DLAPageRegions, pageImages map[int]image.Image) string {
	if len(sec.Positions) == 0 {
		return ""
	}
	const gap = 6 // matches cropout's vertical paste gap.

	// Convert section bbox from PDF points (72 DPI) to DLA pixel space (216 DPI).
	scale := pdf.DlaDPI / 72.0 // 3.0

	var crops []image.Image
	for _, pos := range sec.Positions {
		if len(pos.PageNumbers) == 0 {
			continue
		}
		// Spanning height in pixels for this position across its pages,
		// mirroring CropSectionImage's accumBottom computation.
		pn0 := pos.PageNumbers[0]
		accumBottom := pos.Bottom * scale
		for _, pn := range pos.PageNumbers[1:] {
			if pn == pn0 {
				continue
			}
			if img, ok := pageImages[pn]; ok {
				accumBottom += float64(img.Bounds().Dy())
			}
		}

		img0, ok := pageImages[pn0]
		if !ok {
			// First page of this position is missing → skip the whole
			// position (its page order is anchored on pn0). Python's
			// cropout likewise skips out-of-range pages.
			continue
		}
		// First page: crop [Top, min(Bottom, page height)] (clamped).
		firstBottom := math.Min(accumBottom, float64(img0.Bounds().Dy()))
		crops = append(crops, cropDLAPage(img0, dlaRegions, pn0,
			pos.Left*scale, pos.Top*scale, pos.Right*scale, firstBottom))

		// Subsequent pages: each contributes [0, remaining height].
		remaining := accumBottom - float64(img0.Bounds().Dy())
		for _, pn := range pos.PageNumbers[1:] {
			if pn == pn0 {
				continue
			}
			imgN, ok := pageImages[pn]
			if !ok {
				// Missing subsequent page → skip just this page; the
				// caller's CropSectionImage fallback would also drop it.
				continue
			}
			pageH := float64(imgN.Bounds().Dy())
			h := math.Min(remaining, pageH)
			crops = append(crops, cropDLAPage(imgN, dlaRegions, pn,
				pos.Left*scale, 0, pos.Right*scale, h))
			remaining -= pageH
		}
	}
	if len(crops) == 0 {
		return ""
	}
	return stitchVerticalImages(crops, gap)
}

// cropDLAPage crops a single page for a section position. It prefers the
// best-overlapping figure/equation DLA region (cropout's find_overlapped
// branch); if none overlaps, it falls back to the section bbox on that page
// (cropout's ii-is-None branch). left/top/right/bottom are already in DLA
// pixel space (216 DPI).
func cropDLAPage(img image.Image, dlaRegions []pdf.DLAPageRegions,
	pn int, lpx, tpx, rpx, bpx float64,
) image.Image {
	// Find DLA regions for this page.
	var regions []pdf.DLARegion
	for _, dp := range dlaRegions {
		if dp.Page == pn {
			regions = dp.Regions
			break
		}
	}

	bx := Rect{X0: lpx, Y0: tpx, X1: rpx, Y1: bpx}
	bestIdx := -1
	bestOverlap := 0.0
	for i, r := range regions {
		if r.Label != pdf.LayoutTypeFigure && r.Label != pdf.LayoutTypeEquation {
			continue
		}
		// Match Python's find_overlapped metric: intersection_area / Area(region).
		overlap := OverlapRatioA(Rect{r.X0, r.Y0, r.X1, r.Y1}, bx)
		if overlap > bestOverlap {
			bestOverlap = overlap
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		if cropped, err := CropImageRegion(img, regions[bestIdx]); err == nil {
			return cropped
		} else {
			common.Warn("cropSectionByDLA: cropImageRegion failed, falling back to bbox",
				zap.Int("page", pn), zap.Error(err))
		}
	}
	// Fallback: crop the section bbox on this page (FastCrop clamps to bounds).
	return FastCrop(img, int(lpx), int(tpx), int(rpx), int(bpx))
}

// stitchVerticalImages concatenates crops top-to-bottom on a gray (245)
// background with the given pixel gap, matching cropout's
// Image.new("RGB", (...), (245, 245, 245)) + paste loop. Returns a base64 PNG,
// or "" if encoding fails.
func stitchVerticalImages(imgs []image.Image, gap int) string {
	if len(imgs) == 0 {
		return ""
	}
	totalH := 0
	maxW := 0
	for _, im := range imgs {
		b := im.Bounds()
		totalH += b.Dy() + gap
		if w := b.Dx(); w > maxW {
			maxW = w
		}
	}
	totalH -= gap
	stitched := image.NewRGBA(image.Rect(0, 0, maxW, totalH))

	// Fill gray 245 background (BGRA bytes).
	for y := 0; y < totalH; y++ {
		row := stitched.Pix[stitched.PixOffset(0, y):stitched.PixOffset(maxW, y)]
		for i := 0; i < len(row); i += 4 {
			row[i] = 245
			row[i+1] = 245
			row[i+2] = 245
			row[i+3] = 255
		}
	}

	curY := 0
	for _, im := range imgs {
		b := im.Bounds()
		srcW, srcH := b.Dx(), b.Dy()
		if rgba, ok := im.(*image.RGBA); ok {
			for ry := 0; ry < srcH; ry++ {
				srcStart := rgba.PixOffset(b.Min.X, b.Min.Y+ry)
				srcRow := rgba.Pix[srcStart : srcStart+srcW*4]
				dstStart := stitched.PixOffset(0, curY+ry)
				copy(stitched.Pix[dstStart:], srcRow)
			}
		} else {
			for y := 0; y < srcH; y++ {
				for x := 0; x < srcW; x++ {
					stitched.Set(x, curY+y, im.At(x+b.Min.X, y+b.Min.Y))
				}
			}
		}
		curY += srcH + gap
	}

	data, err := EncodePNG(stitched)
	if err != nil {
		common.Warn("cropSectionByDLA: PNG encode failed", zap.Error(err))
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

// RotateImageCW rotates an image clockwise. Only 0/90/180/270 supported;
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

// MapRotatedPointToOriginal maps a point from rotated image coords back to
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

// MapRotatedRectToOriginal maps a rotated-image rectangle back into original
// image coordinates and normalizes the resulting bounds. For 90°/270° rotation,
// mapping only two diagonal corners can invert X/Y bounds; mapping all four
// corners preserves the enclosing rectangle.
func MapRotatedRectToOriginal(x0, y0, x1, y1 float64, angle int, origW, origH int) (float64, float64, float64, float64) {
	points := [][2]float64{
		{x0, y0},
		{x1, y0},
		{x0, y1},
		{x1, y1},
	}
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range points {
		x, y := MapRotatedPointToOriginal(p[0], p[1], angle, origW, origH)
		minX = math.Min(minX, x)
		minY = math.Min(minY, y)
		maxX = math.Max(maxX, x)
		maxY = math.Max(maxY, y)
	}
	return minX, minY, maxX, maxY
}

// TSRRegionMarginPx is the fixed pixel margin added around a DLA table bbox
// before it is sent to TSR. It matches Python's _table_transformer_job, which
// expands the box by MARGIN = 10 PDF points and then scales by ZM
// (10 * DlaScale = 30px at the 216-DPI render). The margin is FIXED, not
// proportional to table size — an earlier Go port mistakenly used w*0.03 /
// h*0.03, which diverged from Python.
const TSRRegionMarginPx = 10.0 * pdf.DlaScale

// CropImageRegion crops a pdf.DLARegion from an image with a fixed margin
// (matching Python's _table_transformer_job: MARGIN=10 points scaled by ZM).
func CropImageRegion(img image.Image, r pdf.DLARegion) (image.Image, error) {
	marginX := TSRRegionMarginPx
	marginY := TSRRegionMarginPx
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

// CropSectionPositions is the tag-free variant of CropSectionImage. It
// crops directly from a typed []pdf.Position — for example decoded from a
// chunk's _pdf_positions matrix — instead of re-parsing a position tag
// string. The page-image map is keyed by zero-indexed page number.
//
// Python: pdf_parser.py:1802 RAGFlowPdfParser.crop()
func CropSectionPositions(positions []pdf.Position, decodedImages map[int]image.Image, zoom float64) string {
	return encodeCroppedImage(CropSectionPositionsRaster(positions, decodedImages, zoom))
}

// CropSectionPositionsRaster crops typed PDF positions and returns the raster
// without encoding it. Callers that need both OCR and a VLM payload can reuse
// this image and encode it once after OCR.
func CropSectionPositionsRaster(positions []pdf.Position, decodedImages map[int]image.Image, zoom float64) image.Image {
	return cropSectionPositionsRaster(positions, decodedImages, zoom)
}

// CropSectionPositionsRasterLimited rejects a section before rendering when
// the planned stitched raster exceeds maxPixels.
func CropSectionPositionsRasterLimited(positions []pdf.Position, decodedImages map[int]image.Image, zoom float64, maxPixels int64) image.Image {
	plan, ok := buildSectionRasterPlan(positions, decodedImages, zoom)
	if !ok || rasterPlanExceedsLimit(plan, maxPixels) {
		return nil
	}
	return renderSectionRaster(plan, decodedImages)
}

// PositionsFromMatrix converts the _pdf_positions / positions matrix form
// (as produced by layout.SectionsToJSON and normalized to 1-based page
// numbers by normalizePDFPageNumber) back into typed []pdf.Position. The
// engine renders 0-based pages, so the leading 1-based page numbers are
// shifted to 0-based — matching Python's crop(), which indexes
// self.page_images[page_number-1].
func PositionsFromMatrix(m [][]any) []pdf.Position {
	var out []pdf.Position
	for _, row := range m {
		if len(row) < 5 {
			continue
		}
		pageNums := matrixPageNumbers(row[0])
		left, lok := matrixFloat(row[1])
		right, rok := matrixFloat(row[2])
		top, tok := matrixFloat(row[3])
		bottom, bok := matrixFloat(row[4])
		if !lok || !rok || !tok || !bok || len(pageNums) == 0 {
			continue
		}
		out = append(out, pdf.Position{
			PageNumbers: pageNums,
			Left:        left,
			Right:       right,
			Top:         top,
			Bottom:      bottom,
		})
	}
	return out
}

// matrixPageNumbers decodes a page-number cell (scalar or list) from the
// JSON matrix and converts it from the 1-based serialization to the
// engine's 0-based page index.
func matrixPageNumbers(raw any) []int {
	switch v := raw.(type) {
	case []any:
		var out []int
		for _, e := range v {
			if n, ok := matrixFloat(e); ok {
				out = append(out, int(n)-1)
			}
		}
		return out
	case float64:
		return []int{int(v) - 1}
	case int:
		return []int{v - 1}
	case int64:
		return []int{int(v) - 1}
	}
	return nil
}

func matrixFloat(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}
