//go:build cgo

package native

// tsr.go — Table Structure Recognition recognizer.
//
// Ports the PP-Det style path in deepdoc/vision/recognizer.py (base
// Recognizer.preprocess/postprocess, "scale_factor" branch off) plus the
// column/row alignment in deepdoc/vision/table_structure_recognizer.py and the
// wire mapping in deepdoc/server/adapters/tsr_adapter.py.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

const tsrInputSize = 640
const tsrCandidates = 8400

// tsrLabels mirrors TableStructureRecognizer.labels (also tsr_adapter.TSR_CLASS_MAP keys).
var tsrLabels = []string{
	"table", "table column", "table row",
	"table column header", "table projected row header", "table spanning cell",
}

// TSRBox is one structural element (in original pixel coordinates).
type TSRBox struct {
	Label               string
	Score               float32
	X0, X1, Top, Bottom float32
}

// TSRResult is the aligned set of structural elements for one table region.
// W/H are the source image dimensions, used to clamp boxes into bounds (mirrors
// tsr_adapter.py, which clamps every coordinate to [0, width]/[0, height]).
type TSRResult struct {
	Boxes []TSRBox
	W, H  int
}

// RunTSR runs table-structure recognition on a cropped table image.
func RunTSR(ctx context.Context, modelDir string, img *Image) (TSRResult, error) {
	blob, sf := tsrPreprocess(img)
	// 0 → all cores, matching deepdoc's Python onnxruntime for bit-stable
	// parity (no contour extraction in the TSR Run path).
	sess, release, err := getModelSession(filepath.Join(modelDir, "tsr.onnx"), "images",
		[]int64{1, 3, tsrInputSize, tsrInputSize}, "output0",
		[]int64{1, 11, tsrCandidates}, 0)
	if err != nil {
		return TSRResult{}, err
	}
	defer release()

	out, err := sess.Run(ctx, blob)
	if err != nil {
		return TSRResult{}, err
	}
	res := tsrPostprocess(out, sf)
	res.W, res.H = img.W, img.H
	return res, nil
}

// tsrBlob assembles the CHW float blob (/255) the TSR model consumes from an
// already-resized BGR raster (tsrInputSize*tsrInputSize*3, row-major). Only
// the resize source differs (Go bilinearResize vs cv2 in the production
// Python reference).
func tsrBlob(resized []byte) []float32 {
	blob := make([]float32, 3*tsrInputSize*tsrInputSize)
	for y := 0; y < tsrInputSize; y++ {
		for x := 0; x < tsrInputSize; x++ {
			o := (y*tsrInputSize + x) * 3
			blob[0*tsrInputSize*tsrInputSize+y*tsrInputSize+x] = float32(resized[o]) / 255.0
			blob[1*tsrInputSize*tsrInputSize+y*tsrInputSize+x] = float32(resized[o+1]) / 255.0
			blob[2*tsrInputSize*tsrInputSize+y*tsrInputSize+x] = float32(resized[o+2]) / 255.0
		}
	}
	return blob
}

// tsrScaleFactor builds the [W/640, H/640] mapping (mirrors ref_tsr.py sf).
func tsrScaleFactor(img *Image) [2]float32 {
	return [2]float32{float32(img.W) / tsrInputSize, float32(img.H) / tsrInputSize}
}

func tsrPostprocess(out []float32, sf [2]float32) TSRResult {
	const scoreThr = 0.2
	type cand struct {
		nmsBox
		cls int
	}
	cands := make([]cand, 0, tsrCandidates)
	for a := 0; a < tsrCandidates; a++ {
		// Model output is [1, 11, 8400] (feature-major / channels-first), so
		// flat index for feature c, anchor a is c*8400 + a.
		// Class scores live in features 4..10; pick the max.
		best, bestCls := float32(-1), 0
		for c := 4; c < 11; c++ {
			v := out[c*tsrCandidates+a]
			if v > best {
				best = v
				bestCls = c - 4
			}
		}
		if best <= scoreThr {
			continue
		}
		if bestCls >= len(tsrLabels) {
			continue
		}
		// Model emits [x, y, w, h] (center-based) in the 640-input space.
		// Scale back to original pixels, then convert to xyxy (mirrors
		// recognizer.py postprocess: multiply by scale_factor, then xywh2xyxy).
		cx := out[0*tsrCandidates+a] * sf[0]
		cy := out[1*tsrCandidates+a] * sf[1]
		hw := out[2*tsrCandidates+a] * sf[0] * 0.5
		hh := out[3*tsrCandidates+a] * sf[1] * 0.5
		cands = append(cands, cand{
			nmsBox: nmsBox{
				X0:    cx - hw,
				Y0:    cy - hh,
				X1:    cx + hw,
				Y1:    cy + hh,
				Score: best,
			},
			cls: bestCls,
		})
	}

	byClass := map[int][]int{}
	for i, c := range cands {
		byClass[c.cls] = append(byClass[c.cls], i)
	}
	boxes := make([]TSRBox, 0, len(cands))
	for cls, idxs := range byClass {
		sub := make([]nmsBox, len(idxs))
		for k, i := range idxs {
			sub[k] = cands[i].nmsBox
		}
		for _, keep := range nms(sub, 0.2, false) {
			b := sub[keep]
			boxes = append(boxes, TSRBox{
				Label: tsrLabels[cls],
				Score: round4(b.Score),
				X0:    round2(b.X0), X1: round2(b.X1),
				Top: round2(b.Y0), Bottom: round2(b.Y1),
			})
		}
	}

	alignTSR(boxes)
	// Deterministic ordering: tsrPostprocess iterates a class->index map, whose
	// iteration order is unspecified in Go. Sort so identical detections always
	// serialize identically (e.g. for stable Wire() across runs / session reuse).
	sort.Slice(boxes, func(i, j int) bool {
		a, b := boxes[i], boxes[j]
		ca, cb := tsrClassMap[a.Label], tsrClassMap[b.Label]
		if ca != cb {
			return ca < cb
		}
		if a.X0 != b.X0 {
			return a.X0 < b.X0
		}
		if a.Top != b.Top {
			return a.Top < b.Top
		}
		if a.X1 != b.X1 {
			return a.X1 < b.X1
		}
		if a.Bottom != b.Bottom {
			return a.Bottom < b.Bottom
		}
		return a.Score < b.Score
	})
	return TSRResult{Boxes: boxes}
}

// alignTSR pulls row/header boxes to the table's horizontal extremes and
// column boxes to its vertical extremes, matching deepdoc
// TableStructureRecognizer.__call__: when there are more than 4 boxes of a
// kind it aligns to the mean (rows/headers) or median (columns) of the edges,
// otherwise to the plain min/max extremes. The adjustment is one-sided — a box
// edge is only pulled in to the bound when it exceeds it, never pushed out.
func alignTSR(boxes []TSRBox) {
	var leftVals, rightVals, topVals, botVals []float32
	for _, b := range boxes {
		if strings.Contains(b.Label, "row") || strings.Contains(b.Label, "header") {
			leftVals = append(leftVals, b.X0)
			rightVals = append(rightVals, b.X1)
		}
		if b.Label == "table column" {
			topVals = append(topVals, b.Top)
			botVals = append(botVals, b.Bottom)
		}
	}
	if len(leftVals) == 0 {
		return
	}
	// Rows/headers: mean when >4 boxes, else min (left) / max (right).
	left := meanOf(leftVals)
	if len(leftVals) <= 4 {
		left = minOf(leftVals)
	}
	right := meanOf(rightVals)
	if len(rightVals) <= 4 {
		right = maxOf(rightVals)
	}
	for i := range boxes {
		if strings.Contains(boxes[i].Label, "row") || strings.Contains(boxes[i].Label, "header") {
			if boxes[i].X0 > left {
				boxes[i].X0 = left
			}
			if boxes[i].X1 < right {
				boxes[i].X1 = right
			}
		}
	}
	if len(topVals) == 0 {
		return
	}
	// Columns: median when >4 boxes, else min (top) / max (bottom).
	top := medianOf(topVals)
	if len(topVals) <= 4 {
		top = minOf(topVals)
	}
	bot := medianOf(botVals)
	if len(botVals) <= 4 {
		bot = maxOf(botVals)
	}
	for i := range boxes {
		if boxes[i].Label == "table column" {
			if boxes[i].Top > top {
				boxes[i].Top = top
			}
			if boxes[i].Bottom < bot {
				boxes[i].Bottom = bot
			}
		}
	}
}

func meanOf(v []float32) float32 {
	var s float32
	for _, x := range v {
		s += x
	}
	return s / float32(len(v))
}

func medianOf(v []float32) float32 {
	s := make([]float32, len(v))
	copy(s, v)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

var tsrClassMap = map[string]int{
	"table": 0, "table column": 1, "table row": 2,
	"table column header": 3, "table projected row header": 4, "table spanning cell": 5,
}

// Wire emits the Go DocAnalyzer TSR format:
// {"bboxes": [[x0,y0,x1,y1,score,class_id], ...]}.
func (r TSRResult) Wire() string {
	rows := make([][]float32, 0, len(r.Boxes))
	w, h := float32(r.W), float32(r.H)
	for _, b := range r.Boxes {
		cls, ok := tsrClassMap[b.Label]
		if !ok {
			continue
		}
		// Clamp into image bounds (mirrors tsr_adapter.py).
		x0 := minf(maxf(b.X0, 0), w)
		x1 := minf(maxf(b.X1, 0), w)
		top := minf(maxf(b.Top, 0), h)
		bot := minf(maxf(b.Bottom, 0), h)
		rows = append(rows, []float32{x0, top, x1, bot, b.Score, float32(cls)})
	}
	out, _ := json.Marshal(map[string]any{"bboxes": rows})
	return string(out)
}

func minOf(v []float32) float32 {
	m := v[0]
	for _, x := range v[1:] {
		if x < m {
			m = x
		}
	}
	return m
}

func maxOf(v []float32) float32 {
	m := v[0]
	for _, x := range v[1:] {
		if x > m {
			m = x
		}
	}
	return m
}
