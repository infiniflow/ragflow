//go:build cgo

package native

// det.go — OCR text detection (DB) geometry path.
//
// Ports deepdoc/vision/ocr.py TextDetector and deepdoc/vision/postprocess.py
// DBPostProcess (box_type="quad"). The shared entry point, types, the
// true round-offset unclip, and the wire format live in det_core.go.
//
// Inference is near-bit-exact with the Python service (same ONNX Runtime
// build): the raw pred map matches to mean|Δ|≈1.3e-3, with the few >0.1
// pixels confined to high-contrast text edges (bilinear-resize interpolation
// differences between Go's bilinearResize and cv2.resize, not a channel/shift
// bug). Verified stage-by-stage via TestDumpStages + cmp_stages.py +
// diff_stages.py.
//
// The DB geometry — Moore-neighbour (Suzuki-Abe style) contour following,
// rotating-calipers minAreaRect, and a scanline fillPoly for box_score_fast —
// is reimplemented in Go. On mp_physics_p5 Go yields 21 == 21 final boxes. The
// Go det pred map matches the live TextDetector to mean|Δ|≈4e-5 (the earlier
// ~3e-3 gap was a swapped R/B channel order in normalizeCHW, since fixed:
// detPreprocess feeds RGB bytes with RGB-order stats, matching deepdoc, which
// normalizes the RGB image directly). fillPoly is bit-exact
// (TestFillPolyAlignsCV2). This is the only det build.

import (
	"encoding/json"
	"math"
	"os"
)

// detPreprocess mirrors TextDetector's pre_process_list:
// DetResizeForTest(limit_side_len=960, limit_type="max") ->
// NormalizeImage(scale=1/255, mean, std, order="hwc") -> ToCHWImage.
// Returns the CHW float32 blob plus the resized and source dimensions.
func detPreprocess(img *Image) (blob []float32, resizeH, resizeW, srcH, srcW int) {
	srcH, srcW = img.H, img.W
	h, w := srcH, srcW

	ratio := 1.0
	if math.Max(float64(h), float64(w)) > detLimitSideLen {
		ratio = float64(detLimitSideLen) / math.Max(float64(h), float64(w))
	}
	resizeH = int(math.Round(float64(h) * ratio))
	resizeW = int(math.Round(float64(w) * ratio))
	resizeH = int(math.Max(float64(round32(resizeH)), 32))
	resizeW = int(math.Max(float64(round32(resizeW)), 32))

	// deepdoc's TextDetector normalizes the original RGB image directly
	// (RGB-order mean/std, channel 0 of the CHW blob = R), so we feed RGB
	// bytes here — NOT ToBGR. Swapping to BGR while keeping RGB-order stats
	// was the source of a ~3e-3 pred-map divergence that box_score_fast then
	// amplified into score-crossing orphans.
	rgb := img.Pix
	resized := bilinearResize(rgb, w, h, resizeW, resizeH)
	return normalizeCHW(resized, resizeH, resizeW), resizeH, resizeW, srcH, srcW
}

// dbPostProcess mirrors DBPostProcess.boxes_from_bitmap + TextDetector.filter_tag_det_res.
func dbPostProcess(pred []float32, h, w, srcH, srcW int) []DetBox {
	// Binary segmentation mask.
	seg := make([]bool, h*w)
	for i, v := range pred {
		seg[i] = v > detThresh
	}
	if os.Getenv("DLA_DUMP_STAGES") != "" {
		bits := make([]int, len(seg))
		for i, b := range seg {
			if b {
				bits[i] = 1
			}
		}
		if b, err := json.Marshal(map[string]any{"h": h, "w": w, "seg": bits}); err == nil {
			_ = os.WriteFile("/tmp/go_seg.json", b, 0o644)
		}
	}

	// Contour extraction via findContours: Moore-neighbour (Suzuki-Abe style)
	// border following. It returns one boundary point set per 8-connected
	// foreground component, in integer coords (no +0.5 centre offset), so the
	// shared convexHull/minAreaRect/boxScoreFast downstream aligns with the
	// Python oracle. The remaining ~3/5 IoU box-membership orphans versus the
	// goldens are contour-boundary geometry, not pred/score/grouping.
	comps := findContours(seg, w, h, detMaxCandidates)

	if os.Getenv("DLA_DUMP_STAGES") != "" {
		// Each component's full foreground pixel set (resized coords, +0.5
		// center offset) — for a direct cv2.minAreaRect comparison against
		// Python's contour pixel sets, to localize whether the det divergence
		// is the component SET (grouping) or Go's minAreaRect algorithm.
		psets := make([][][2]float64, 0, len(comps))
		for _, c := range comps {
			s := make([][2]float64, 0, len(c))
			for _, p := range c {
				s = append(s, [2]float64{p.X, p.Y})
			}
			psets = append(psets, s)
		}
		if b, err := json.Marshal(map[string]any{"w": w, "h": h, "comps": psets}); err == nil {
			_ = os.WriteFile("/tmp/go_comps.json", b, 0o644)
		}
	}

	boxes := make([]DetBox, 0, len(comps))
	for _, comp := range comps {
		hull := convexHull(comp)
		if len(hull) < 3 {
			continue
		}
		// Pre-unclip min-area rect + side check.
		pts, sside := minAreaRect(hull)
		if sside < detMinSize {
			continue
		}
		dlaRecordPreUnclip(pts)
		score := boxScoreFast(pred, w, h, pts)
		// unclip (expand) then re-rect.
		expanded := unclip(pts, detUnclipRatio)
		pts2, sside2 := minAreaRect(expanded[:])
		if sside2 < detMinSize+2 {
			continue
		}
		// Scale back to source coordinates (dest = source dims here).
		var q [4][2]float32
		for i := 0; i < 4; i++ {
			qx := clampf(math.Round(float64(pts2[i].X)/float64(w)*float64(srcW)), 0, float64(srcW))
			qy := clampf(math.Round(float64(pts2[i].Y)/float64(h)*float64(srcH)), 0, float64(srcH))
			q[i] = [2]float32{float32(qx), float32(qy)}
		}
		// Diagnostic: record the candidate (post-geometry, pre-score-filter)
		// quad + its pre-unclip score so the divergence between Go and cv2 can
		// be classified as geometry/grouping vs score-threshold. Gated by
		// DLA_DUMP_CANDIDATES; harmless otherwise.
		dlaRecordCandidate(q, pts, score)
		if detBoxThresh > score {
			continue
		}
		boxes = append(boxes, DetBox{Pts: q, Score: score})
	}

	// filter_tag_det_res: clockwise order + integer clip + drop tiny boxes.
	dlaFlushPreUnclip()
	dlaFlushCandidates()
	return filterTagDetRes(boxes, srcH, srcW)
}

// findContours extracts foreground contours via Moore-neighbour (Suzuki-Abe
// style) border following, mirroring cv2.findContours(RETR_LIST). It returns
// one point set per contour — the boundary pixels — in cv2's coordinate
// convention (integer pixel indices, no +0.5 centre offset), so the shared
// convexHull/minAreaRect/boxScoreFast downstream matches the Python oracle.
//
// RETR_LIST => a flat list; holes are returned as separate contours (they are
// later dropped by the 0.5 score filter). On mp_physics_p5 this reproduces the
// cv2 component set closely enough that the final boxes match the live
// TextDetector 21 == 21; across all fixtures the IoU box-membership gap vs
// the regenerated goldens is 3/5. The remaining orphans are contour-tracer
// geometry (the hand-rolled border follower vs cv2's), not pred/score/grouping
// — the Go det pred map matches the live TextDetector to mean|Δ|≈4e-5 since
// normalizeCHW was fixed to feed RGB bytes with RGB-order stats. The
// thresholded seg map matches to 0.129% (seg diff 634 px), and fillPoly is
// bit-exact (TestFillPolyAlignsCV2). This is the only det build.
func findContours(seg []bool, w, h, maxComps int) [][]pt {
	// Pad with a 1px background border (OpenCV processes with one).
	W, H := w+2, h+2
	m := make([]int, W*H)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if seg[y*w+x] {
				m[(y+1)*W+(x+1)] = 1
			}
		}
	}
	visited := make([]int, len(m))
	copy(visited, m)
	// 8 neighbours in clockwise order starting from "up".
	NB := [8][2]int{{-1, 0}, {-1, 1}, {0, 1}, {1, 1}, {1, 0}, {1, -1}, {0, -1}, {-1, -1}}

	// nextClockwise returns the first foreground neighbour of (curR,curC) when
	// scanning clockwise starting just after the backtrack direction b.
	nextClockwise := func(b [2]int, curR, curC int) ([2]int, int, int) {
		bi := 7
		for k := 0; k < 8; k++ {
			if NB[k] == b {
				bi = k
				break
			}
		}
		for step := 0; step < 8; step++ {
			k := (bi + 1 + step) % 8
			nr, nc := curR+NB[k][0], curC+NB[k][1]
			if nr >= 0 && nr < H && nc >= 0 && nc < W && m[nr*W+nc] == 1 {
				return NB[k], nr, nc
			}
		}
		return b, -1, -1
	}

	var contours [][]pt
	nbd := 2
	for r := 1; r < H-1; r++ {
		for c := 1; c < W-1; c++ {
			if visited[r*W+c] != 1 {
				continue
			}
			isOuter := visited[r*W+(c-1)] == 0
			isHole := !isOuter && visited[(r-1)*W+c] == 0 && visited[r*W+(c+1)] == 0
			if !isOuter && !isHole {
				continue
			}
			startR, startC := r, c
			var back [2]int
			if isOuter {
				back = [2]int{0, -1}
			} else {
				back = [2]int{-1, 0}
			}
			curR, curC := r, c
			var contour []pt
			first := true
			for {
				bdir, nr, nc := nextClockwise(back, curR, curC)
				if nc < 0 {
					break
				}
				contour = append(contour, pt{X: float64(nc - 1), Y: float64(nr - 1)})
				if visited[nr*W+nc] == 1 {
					visited[nr*W+nc] = nbd
				}
				if !first && nr == startR && nc == startC {
					break
				}
				first = false
				back = [2]int{-bdir[0], -bdir[1]}
				curR, curC = nr, nc
				if len(contour) > W*H {
					break
				}
			}
			if len(contour) >= 3 {
				contours = append(contours, contour)
				nbd++
			}
			visited[r*W+c] = nbd
		}
	}
	if maxComps > 0 && len(contours) > maxComps {
		contours = contours[:maxComps]
	}
	return contours
}

// convexHull is defined in det_core.go (shared by both builds): a generic
// geometry helper used by the pure-Go dbPostProcess.

// minAreaRect is defined in det_core.go (shared by both builds): the pure-Go
// float-precision rotating-calipers port of cv2.minAreaRect.
