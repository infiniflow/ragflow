//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

//go:build cgo

package native

// nms.go — generic axis-aligned bounding-box nms.
//
// Shared by DLA (IoU 0.45, +1) and TSR (IoU 0.2, no +1). The +1 term matches
// deepdoc/vision/operators.py's nms; some PP-Det paths omit it.

import (
	"sort"
)

// nmsBox is an xyxy rectangle with an associated score.
type nmsBox struct {
	X0, Y0, X1, Y1 float32
	Score          float32
}

// nms returns the indices (into the input slice) of the kept boxes.
func nms(boxes []nmsBox, iouThr float32, plusOne bool) []int {
	order := make([]int, len(boxes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return boxes[order[i]].Score > boxes[order[j]].Score
	})

	areas := make([]float32, len(boxes))
	for i, b := range boxes {
		areas[i] = (b.Y1 - b.Y0) * (b.X1 - b.X0)
	}

	var keep []int
	for len(order) > 0 {
		i := order[0]
		keep = append(keep, i)
		if len(order) == 1 {
			break
		}
		xx1 := make([]float32, len(order)-1)
		yy1 := make([]float32, len(order)-1)
		xx2 := make([]float32, len(order)-1)
		yy2 := make([]float32, len(order)-1)
		for k, j := range order[1:] {
			xx1[k] = max(boxes[i].X0, boxes[j].X0)
			yy1[k] = max(boxes[i].Y0, boxes[j].Y0)
			xx2[k] = min(boxes[i].X1, boxes[j].X1)
			yy2[k] = min(boxes[i].Y1, boxes[j].Y1)
		}
		w := subMax0(xx2, xx1)
		h := subMax0(yy2, yy1)
		if plusOne {
			for k := range w {
				w[k] += 1
				h[k] += 1
			}
		}
		overlaps := mul(w, h)
		var idx []int
		for k := range overlaps {
			denom := areas[i] + areas[order[k+1]] - overlaps[k]
			iou := float32(0)
			if denom > 0 {
				iou = overlaps[k] / denom
			}
			if iou <= iouThr {
				idx = append(idx, k)
			}
		}
		next := make([]int, 0, len(idx))
		for _, k := range idx {
			next = append(next, order[k+1])
		}
		order = next
	}
	return keep
}

func subMax0(a, b []float32) []float32 {
	r := make([]float32, len(a))
	for i := range a {
		v := a[i] - b[i]
		if v < 0 {
			v = 0
		}
		r[i] = v
	}
	return r
}

func mul(a, b []float32) []float32 {
	r := make([]float32, len(a))
	for i := range a {
		r[i] = a[i] * b[i]
	}
	return r
}
