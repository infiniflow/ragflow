package inference

import (
	"image"
	"math"
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// A 4096px threshold keeps common 300-DPI document pages on one request.
// Larger images would be reduced below 0.234x by the detector's 960px input;
// 2880px tiles retain about 0.33x scale, with a 10% overlap for edge text.
const (
	ocrTileSize         = 2880
	ocrTileOverlap      = 288
	ocrTilingThreshold  = 4096
	ocrDuplicateOverlap = 0.5
)

type ocrBoxBounds struct {
	left, top, right, bottom float64
}

type ocrGridCell struct {
	x, y int
}

func ocrTileStarts(length int) []int {
	if length <= ocrTileSize {
		return []int{0}
	}
	step := ocrTileSize - ocrTileOverlap
	starts := make([]int, 0, (length+step-1)/step)
	for start := 0; start <= length-ocrTileSize; start += step {
		starts = append(starts, start)
	}
	finalStart := length - ocrTileSize
	if starts[len(starts)-1] != finalStart {
		starts = append(starts, finalStart)
	}
	return starts
}

func detectTiledOCR(img image.Image, detectTile func(image.Image) ([]pdf.OCRBox, error)) ([]pdf.OCRBox, error) {
	bounds := img.Bounds()
	rowStarts := ocrTileStarts(bounds.Dy())
	columnStarts := ocrTileStarts(bounds.Dx())
	boxes := make([]pdf.OCRBox, 0)
	sources := make([]int, 0)
	tileIndex := 0
	for _, top := range rowStarts {
		for _, left := range columnStarts {
			tile := util.FastCrop(
				img,
				bounds.Min.X+left,
				bounds.Min.Y+top,
				bounds.Min.X+min(left+ocrTileSize, bounds.Dx()),
				bounds.Min.Y+min(top+ocrTileSize, bounds.Dy()),
			)
			tileBoxes, err := detectTile(tile)
			if err != nil {
				return nil, err
			}
			for _, box := range tileBoxes {
				boxes = append(boxes, translateOCRBox(box, float64(left), float64(top)))
				sources = append(sources, tileIndex)
			}
			tileIndex++
		}
	}
	return deduplicateOCRBoxes(boxes, sources), nil
}

func translateOCRBox(box pdf.OCRBox, left, top float64) pdf.OCRBox {
	box.X0 += left
	box.X1 += left
	box.X2 += left
	box.X3 += left
	box.Y0 += top
	box.Y1 += top
	box.Y2 += top
	box.Y3 += top
	return box
}

func boundsForOCRBox(box pdf.OCRBox) ocrBoxBounds {
	return ocrBoxBounds{
		left:   min(box.X0, box.X1, box.X2, box.X3),
		top:    min(box.Y0, box.Y1, box.Y2, box.Y3),
		right:  max(box.X0, box.X1, box.X2, box.X3),
		bottom: max(box.Y0, box.Y1, box.Y2, box.Y3),
	}
}

func (b ocrBoxBounds) area() float64 {
	return max(0, b.right-b.left) * max(0, b.bottom-b.top)
}

func (b ocrBoxBounds) cells() []ocrGridCell {
	left := int(math.Floor(b.left / ocrTileOverlap))
	top := int(math.Floor(b.top / ocrTileOverlap))
	right := int(math.Floor(b.right / ocrTileOverlap))
	bottom := int(math.Floor(b.bottom / ocrTileOverlap))
	cells := make([]ocrGridCell, 0, (right-left+1)*(bottom-top+1))
	for x := left; x <= right; x++ {
		for y := top; y <= bottom; y++ {
			cells = append(cells, ocrGridCell{x: x, y: y})
		}
	}
	return cells
}

func intersectionOverSmaller(a, b ocrBoxBounds) float64 {
	left := max(a.left, b.left)
	top := max(a.top, b.top)
	right := min(a.right, b.right)
	bottom := min(a.bottom, b.bottom)
	intersection := max(0, right-left) * max(0, bottom-top)
	smallerArea := min(a.area(), b.area())
	if smallerArea == 0 {
		return 0
	}
	return intersection / smallerArea
}

func axisAlignedOCRBox(bounds ocrBoxBounds) pdf.OCRBox {
	return pdf.OCRBox{
		X0: bounds.left, Y0: bounds.top,
		X1: bounds.right, Y1: bounds.top,
		X2: bounds.right, Y2: bounds.bottom,
		X3: bounds.left, Y3: bounds.bottom,
	}
}

func deduplicateOCRBoxes(boxes []pdf.OCRBox, sources []int) []pdf.OCRBox {
	if len(boxes) == 0 {
		return nil
	}
	if len(sources) != len(boxes) {
		return boxes
	}

	bounds := make([]ocrBoxBounds, len(boxes))
	order := make([]int, len(boxes))
	for i, box := range boxes {
		bounds[i] = boundsForOCRBox(box)
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return bounds[order[i]].area() > bounds[order[j]].area()
	})

	spatialIndex := make(map[ocrGridCell][]int)
	keptBoxes := make([]pdf.OCRBox, 0, len(boxes))
	keptBounds := make([]ocrBoxBounds, 0, len(boxes))
	keptSources := make([]map[int]struct{}, 0, len(boxes))
	active := make([]bool, 0, len(boxes))

	for _, candidateIndex := range order {
		candidateBounds := bounds[candidateIndex]
		nearby := make(map[int]struct{})
		for _, cell := range candidateBounds.cells() {
			for _, index := range spatialIndex[cell] {
				if active[index] {
					nearby[index] = struct{}{}
				}
			}
		}

		duplicates := make([]int, 0)
		for index := range nearby {
			if _, sameSource := keptSources[index][sources[candidateIndex]]; sameSource {
				continue
			}
			if intersectionOverSmaller(candidateBounds, keptBounds[index]) >= ocrDuplicateOverlap {
				duplicates = append(duplicates, index)
			}
		}
		if len(duplicates) > 0 {
			sort.Ints(duplicates)
			duplicateIndex := duplicates[0]
			mergedBounds := candidateBounds
			mergedSources := map[int]struct{}{sources[candidateIndex]: {}}
			for _, index := range duplicates {
				mergedBounds.left = min(mergedBounds.left, keptBounds[index].left)
				mergedBounds.top = min(mergedBounds.top, keptBounds[index].top)
				mergedBounds.right = max(mergedBounds.right, keptBounds[index].right)
				mergedBounds.bottom = max(mergedBounds.bottom, keptBounds[index].bottom)
				for source := range keptSources[index] {
					mergedSources[source] = struct{}{}
				}
				if index != duplicateIndex {
					active[index] = false
				}
			}
			if mergedBounds != keptBounds[duplicateIndex] {
				keptBoxes[duplicateIndex] = axisAlignedOCRBox(mergedBounds)
			}
			keptBounds[duplicateIndex] = mergedBounds
			keptSources[duplicateIndex] = mergedSources
			for _, cell := range mergedBounds.cells() {
				alreadyIndexed := false
				for _, index := range spatialIndex[cell] {
					if index == duplicateIndex {
						alreadyIndexed = true
						break
					}
				}
				if !alreadyIndexed {
					spatialIndex[cell] = append(spatialIndex[cell], duplicateIndex)
				}
			}
			continue
		}

		keptIndex := len(keptBoxes)
		keptBoxes = append(keptBoxes, boxes[candidateIndex])
		keptBounds = append(keptBounds, candidateBounds)
		keptSources = append(keptSources, map[int]struct{}{sources[candidateIndex]: {}})
		active = append(active, true)
		for _, cell := range candidateBounds.cells() {
			spatialIndex[cell] = append(spatialIndex[cell], keptIndex)
		}
	}

	result := make([]pdf.OCRBox, 0, len(keptBoxes))
	for index, box := range keptBoxes {
		if active[index] {
			result = append(result, box)
		}
	}
	return result
}
