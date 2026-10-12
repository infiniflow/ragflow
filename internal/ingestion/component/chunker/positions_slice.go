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

package chunker

import (
	"encoding/json"
	"ragflow/internal/common"
	"unicode/utf8"
)

// slicePositionsByTextRatio proportionally slices a PDF position matrix
// (JSON array of [page, left, right, top, bottom] rows) by vertical ratio.
//
// Each row is [page, left, right, top, bottom]. When a chunk's text is split
// into sub-pieces, the original bbox is sliced vertically so each sub-piece's
// screenshot crops only its own region instead of the whole paragraph.
//
// startRatio and endRatio are in [0,1] of the total vertical span. For a
// single-row matrix the slice is a simple vertical interpolation. For a
// multi-row matrix the total height is the sum of row heights; the slice is
// distributed sequentially across rows so a piece spanning a page boundary
// correctly covers the tail of one row and the head of the next.
func slicePositionsByTextRatio(raw json.RawMessage, startRatio, endRatio float64) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	if startRatio < 0 {
		startRatio = 0
	}
	if endRatio > 1 {
		endRatio = 1
	}
	if startRatio >= endRatio {
		return nil
	}
	var matrix [][]any
	if err := json.Unmarshal(raw, &matrix); err != nil || len(matrix) == 0 {
		return nil
	}
	// Compute height per row. Any malformed row (short, non-numeric,
	// or non-positive height) invalidates the whole matrix so callers
	// can fall back to the original bbox rather than silently losing a
	// page region.
	heights := make([]float64, len(matrix))
	total := 0.0
	for i, row := range matrix {
		if len(row) < 5 {
			return nil
		}
		top, ok1 := common.ToFloat(row[3])
		bottom, ok2 := common.ToFloat(row[4])
		if !ok1 || !ok2 || bottom <= top {
			return nil
		}
		h := bottom - top
		heights[i] = h
		total += h
	}
	if total <= 0 {
		return nil
	}
	targetStart := startRatio * total
	targetEnd := endRatio * total
	var out [][]any
	cum := 0.0
	for i, row := range matrix {
		h := heights[i]
		if h <= 0 {
			continue
		}
		segStart := cum
		segEnd := cum + h
		cum += h
		interStart := segStart
		if targetStart > segStart {
			interStart = targetStart
		}
		interEnd := segEnd
		if targetEnd < segEnd {
			interEnd = targetEnd
		}
		if interStart >= interEnd {
			continue
		}
		localStartRatio := (interStart - segStart) / h
		localEndRatio := (interEnd - segStart) / h
		oldTop, _ := common.ToFloat(row[3])
		oldBottom, _ := common.ToFloat(row[4])
		oldH := oldBottom - oldTop
		newTop := oldTop + localStartRatio*oldH
		newBottom := oldTop + localEndRatio*oldH
		newRow := make([]any, len(row))
		copy(newRow, row)
		newRow[3] = newTop
		newRow[4] = newBottom
		out = append(out, newRow)
	}
	if len(out) == 0 {
		return nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

// slicePositionsByCounts slices both position matrices by per-piece visible
// counts so adjacent pieces abut instead of each carrying the full source
// box. counts[i] is the visible-text weight of piece i; parser tags and
// synthetic glue must already be excluded by the caller (see removeTag).
// A piece whose slice is empty (zero-weight piece or malformed matrix)
// falls back to the source matrix so it keeps its preview instead of none.
// When the total weight is not positive every piece falls back the same way.
// Shared by the Token path (chunkFromItem/splitOversizedText) and the General
// path (sliceGeneralSplitPieces) so the three stay mirrored.
func slicePositionsByCounts(pdfSrc, posSrc json.RawMessage, counts []int) ([]json.RawMessage, []json.RawMessage) {
	pdfOut := make([]json.RawMessage, len(counts))
	posOut := make([]json.RawMessage, len(counts))
	total := 0
	for _, c := range counts {
		total += c
	}
	if total <= 0 {
		for i := range counts {
			if len(pdfSrc) > 0 {
				pdfOut[i] = pdfSrc
			}
			if len(posSrc) > 0 {
				posOut[i] = posSrc
			}
		}
		return pdfOut, posOut
	}
	cum := 0
	for i, c := range counts {
		startRatio := float64(cum) / float64(total)
		endRatio := float64(cum+c) / float64(total)
		pdfOut[i] = slicePositionsByTextRatio(pdfSrc, startRatio, endRatio)
		posOut[i] = slicePositionsByTextRatio(posSrc, startRatio, endRatio)
		if len(pdfOut[i]) == 0 && len(pdfSrc) > 0 {
			pdfOut[i] = pdfSrc
		}
		if len(posOut[i]) == 0 && len(posSrc) > 0 {
			posOut[i] = posSrc
		}
		cum += c
	}
	return pdfOut, posOut
}

// sliceAnyPositions is the map-value variant used by the title chunker
// (GroupTitle/HierarchyTitle) where positions are stored as any
// (typically [][]float64). It round-trips through JSON so the same
// vertical slicing logic applies regardless of the in-memory representation.
func sliceAnyPositions(pos any, startRatio, endRatio float64) any {
	if pos == nil {
		return nil
	}
	b, err := json.Marshal(pos)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return nil
	}
	sliced := slicePositionsByTextRatio(json.RawMessage(b), startRatio, endRatio)
	if len(sliced) == 0 {
		return nil
	}
	// Prefer restoring as [][]float64 when possible (the title chunker's
	// native type). Fall back to a generic decoded value.
	var mat [][]float64
	if err := json.Unmarshal(sliced, &mat); err == nil {
		return mat
	}
	var generic any
	if err := json.Unmarshal(sliced, &generic); err == nil {
		return generic
	}
	return nil
}

// totalRunes returns the sum of rune counts for the given strings.
func totalRunes(strs []string) int {
	n := 0
	for _, s := range strs {
		n += utf8.RuneCountInString(s)
	}
	return n
}
