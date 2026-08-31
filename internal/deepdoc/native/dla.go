//go:build cgo

package native

// dla.go — DLA (layout detection) recognizer.
//
// Ports deepdoc/vision/layout_recognizer.py LayoutRecognizer4YOLOv10 and
// deepdoc/server/adapters/dla_adapter.py. Self-contained: owns its
// preprocessing, inference, postprocessing, and wire encoding.

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"sort"
	"strings"
)

const dlaInputSize = 1024
const dlaMaxBoxes = 300

// DLABox is one detected layout region in the wire format.
type DLABox struct {
	X0, Y0, X1, Y1 float32
	Score          float32
	Class          int
}

// DLAResult is the full DLA output.
// W/H are the source image dimensions, used to clamp boxes into bounds (mirrors
// dla_adapter.py, which clamps every coordinate to [0, width]/[0, height]).
type DLAResult struct {
	Boxes []DLABox
	W, H  int
}

var (
	// yoloDlaLabels mirrors LayoutRecognizer4YOLOv10.labels (10 classes). It
	// must stay element-for-element identical to doctype.DefaultDLALabels
	// (same order, same duplicate indices 4/7/9) — that list is the wire
	// contract the in-process detector serialises through. The two live in
	// separate modules so they cannot share one constant; keep them in sync by
	// hand. Case here is irrelevant: dlaPostprocess lowercases each entry
	// before looking it up in dlaClassMap.
	yoloDlaLabels = []string{
		"title", "Text", "Reference", "Figure", "Figure caption",
		"Table", "Table caption", "Table caption", "Equation", "Figure caption",
	}
	// dlaClassMap mirrors dla_adapter.DLA_CLASS_MAP.
	dlaClassMap = map[string]int{
		"title": 0, "text": 1, "reference": 2, "figure": 3, "figure caption": 4,
		"table": 5, "table caption": 6, "equation": 8,
	}
)

// RunDLA runs layout detection on a page image.
func RunDLA(ctx context.Context, modelDir string, img *Image) (DLAResult, error) {
	blob, sf := dlaPreprocess(img)
	// 0 → all cores, matching deepdoc's Python onnxruntime for bit-stable
	// parity (no contour extraction in the DLA Run path).
	sess, release, err := getModelSession(filepath.Join(modelDir, "layout.onnx"), "images",
		[]int64{1, 3, dlaInputSize, dlaInputSize}, "output0",
		[]int64{1, dlaMaxBoxes, 6}, 0)
	if err != nil {
		return DLAResult{}, err
	}
	defer release()

	out, err := sess.Run(ctx, blob)
	if err != nil {
		return DLAResult{}, err
	}
	res := dlaPostprocess(out, sf)
	res.W, res.H = img.W, img.H
	return res, nil
}

// dlaGeom computes the letterbox geometry (mirrors ref_dla.py): the resize
// target (newW,newH) and the symmetric padding (dw,dh) that centers the
// resized image in the dlaInputSize canvas.
func dlaGeom(img *Image) (newW, newH int, dw, dh float64) {
	r := math.Min(float64(dlaInputSize)/float64(img.H), float64(dlaInputSize)/float64(img.W))
	newW = int(math.Round(float64(img.W) * r))
	newH = int(math.Round(float64(img.H) * r))
	dw = (float64(dlaInputSize) - float64(newW)) / 2.0
	dh = (float64(dlaInputSize) - float64(newH)) / 2.0
	return
}

// dlaLetterbox places the already-resized BGR raster (newH*newW*3, row-major)
// into the dlaInputSize canvas with 114-filled borders and returns the CHW
// float blob (/255) the YOLOv10 layout model consumes. Only the resize source
// differs from the production Python reference (Go bilinearResize vs cv2).
func dlaLetterbox(resized []byte, newW, newH int, dw, dh float64) []float32 {
	top := int(math.Round(dh - 0.1))
	left := int(math.Round(dw - 0.1))

	blob := make([]float32, 3*dlaInputSize*dlaInputSize)
	for y := 0; y < dlaInputSize; y++ {
		for x := 0; x < dlaInputSize; x++ {
			var cr, cg, cb float32 = 114, 114, 114
			inY, inX := y-top, x-left
			if inY >= 0 && inY < newH && inX >= 0 && inX < newW {
				o := (inY*newW + inX) * 3
				cb = float32(resized[o])
				cg = float32(resized[o+1])
				cr = float32(resized[o+2])
			}
			// CHW; model expects BGR, so channel 0 = blue, 2 = red.
			blob[0*dlaInputSize*dlaInputSize+y*dlaInputSize+x] = cb / 255.0
			blob[1*dlaInputSize*dlaInputSize+y*dlaInputSize+x] = cg / 255.0
			blob[2*dlaInputSize*dlaInputSize+y*dlaInputSize+x] = cr / 255.0
		}
	}
	return blob
}

// dlaScaleFactor builds the [W/newW, H/newH, dw, dh] mapping (mirrors
// ref_dla.py scale_factor) used by dlaPostprocess to map model coords back to
// source pixels.
func dlaScaleFactor(img *Image, newW, newH int, dw, dh float64) [4]float32 {
	return [4]float32{
		float32(float64(img.W) / float64(newW)),
		float32(float64(img.H) / float64(newH)),
		float32(dw), float32(dh),
	}
}

func dlaPostprocess(out []float32, sf [4]float32) DLAResult {
	const scoreThr = 0.08
	type cand struct {
		nmsBox
		cls int
	}
	cands := make([]cand, 0, dlaMaxBoxes)
	for i := 0; i < dlaMaxBoxes; i++ {
		base := i * 6
		score := out[base+4]
		if score <= scoreThr {
			continue
		}
		// Truncate toward zero, matching deepdoc LayoutRecognizer4YOLOv10.postprocess
		// (boxes[:, -1].astype(int)). The prior int(x+0.5) rounding shifted
		// class-channel values in [2.5, 2.999] from class 2 to class 3.
		cls := int(out[base+5])
		cands = append(cands, cand{
			nmsBox: nmsBox{
				X0:    (out[base+0] - sf[2]) * sf[0],
				Y0:    (out[base+1] - sf[3]) * sf[1],
				X1:    (out[base+2] - sf[2]) * sf[0],
				Y1:    (out[base+3] - sf[3]) * sf[1],
				Score: score,
			},
			cls: cls,
		})
	}

	byClass := map[int][]int{}
	for i, c := range cands {
		byClass[c.cls] = append(byClass[c.cls], i)
	}
	res := DLAResult{}
	for cls, idxs := range byClass {
		sub := make([]nmsBox, len(idxs))
		for k, i := range idxs {
			sub[k] = cands[i].nmsBox
		}
		for _, keep := range nms(sub, 0.45, true) {
			res.Boxes = append(res.Boxes, DLABox{
				X0: round2(sub[keep].X0), Y0: round2(sub[keep].Y0),
				X1: round2(sub[keep].X1), Y1: round2(sub[keep].Y1),
				Score: round4(sub[keep].Score), Class: cls,
			})
		}
	}
	// Re-map class ids through the OSS label->Go index map.
	mapped := res.Boxes[:0]
	for _, b := range res.Boxes {
		// Guard the raw YOLO class index before the slice lookup: the model
		// output column is the integer class id, but an out-of-range or
		// negative value would panic on yoloDlaLabels[b.Class] and take the
		// server process down. Drop the box instead, mirroring the bestCls
		// bounds guard in tsr.go.
		if b.Class < 0 || b.Class >= len(yoloDlaLabels) {
			continue
		}
		label := yoloDlaLabels[b.Class]
		goCls, ok := dlaClassMap[strings.ToLower(label)]
		if !ok {
			continue
		}
		b.Class = goCls
		mapped = append(mapped, b)
	}
	res.Boxes = mapped
	// Deterministic ordering: dlaPostprocess iterates a class->index map, whose
	// iteration order is unspecified in Go. Sort so identical detections always
	// serialize identically (e.g. for stable Wire() across runs / session reuse).
	sort.Slice(res.Boxes, func(i, j int) bool {
		a, b := res.Boxes[i], res.Boxes[j]
		if a.Class != b.Class {
			return a.Class < b.Class
		}
		if a.X0 != b.X0 {
			return a.X0 < b.X0
		}
		if a.Y0 != b.Y0 {
			return a.Y0 < b.Y0
		}
		if a.X1 != b.X1 {
			return a.X1 < b.X1
		}
		if a.Y1 != b.Y1 {
			return a.Y1 < b.Y1
		}
		return a.Score < b.Score
	})
	return res
}

// Wire encodes the result in the exact format the Go DocAnalyzer consumes:
// {"bboxes": [[x0,y0,x1,y1,score,class_id], ...]}.
func (r DLAResult) Wire() string {
	rows := make([][]float32, 0, len(r.Boxes))
	w, h := float32(r.W), float32(r.H)
	for _, b := range r.Boxes {
		// Clamp into image bounds (mirrors dla_adapter.py).
		x0 := minf(maxf(b.X0, 0), w)
		y0 := minf(maxf(b.Y0, 0), h)
		x1 := minf(maxf(b.X1, 0), w)
		y1 := minf(maxf(b.Y1, 0), h)
		rows = append(rows, []float32{x0, y0, x1, y1, b.Score, float32(b.Class)})
	}
	b, _ := json.Marshal(map[string]any{"bboxes": rows})
	return string(b)
}

func round2(v float32) float32 { return float32(math.Round(float64(v)*100) / 100) }
func round4(v float32) float32 { return float32(math.Round(float64(v)*10000) / 10000) }
