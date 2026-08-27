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
	"math"
	"testing"
)

const posSliceEpsilon = 1e-9

func assertMatrixNear(t *testing.T, got json.RawMessage, want [][]float64) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("got nil matrix, want %v", want)
	}
	m := matrixOfRaw(t, got)
	if len(m) != len(want) {
		t.Fatalf("matrix rows: got %d (%v), want %d (%v)", len(m), m, len(want), want)
	}
	for i := range want {
		if len(m[i]) != len(want[i]) {
			t.Fatalf("row %d width: got %d, want %d", i, len(m[i]), len(want[i]))
		}
		for j := range want[i] {
			if math.Abs(m[i][j]-want[i][j]) > posSliceEpsilon {
				t.Fatalf("m[%d][%d] = %v, want %v", i, j, m[i][j], want[i][j])
			}
		}
	}
}

func matrixOfRaw(t *testing.T, raw json.RawMessage) [][]float64 {
	t.Helper()
	var m [][]float64
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return m
}

func TestSlicePositionsByTextRatio_SingleRowHalves(t *testing.T) {
	raw := json.RawMessage(`[[1,10,200,0,100]]`)
	assertMatrixNear(t, slicePositionsByTextRatio(raw, 0, 0.5), [][]float64{{1, 10, 200, 0, 50}})
	assertMatrixNear(t, slicePositionsByTextRatio(raw, 0.5, 1), [][]float64{{1, 10, 200, 50, 100}})
}

func TestSlicePositionsByTextRatio_AdjacentSlicesAbut(t *testing.T) {
	raw := json.RawMessage(`[[1,10,20,50,80]]`)
	first := slicePositionsByTextRatio(raw, 0, 0.4)
	second := slicePositionsByTextRatio(raw, 0.4, 1)
	m1, m2 := matrixOfRaw(t, first), matrixOfRaw(t, second)
	if len(m1) != 1 || len(m2) != 1 {
		t.Fatalf("single-row input must yield single-row slices: %v / %v", m1, m2)
	}
	if math.Abs(m1[0][4]-m2[0][3]) > posSliceEpsilon {
		t.Errorf("adjacent slices must abut: first bottom=%v second top=%v", m1[0][4], m2[0][3])
	}
	if m1[0][3] != 50 || m2[0][4] != 80 {
		t.Errorf("outer bounds changed: %v / %v", m1, m2)
	}
	if m1[0][1] != 10 || m1[0][2] != 20 || m2[0][1] != 10 || m2[0][2] != 20 {
		t.Errorf("left/right must be preserved: %v / %v", m1, m2)
	}
}

func TestSlicePositionsByTextRatio_MultiRowSequentialDistribution(t *testing.T) {
	// Two equal-height rows (page 1 and page 2), total height 20.
	raw := json.RawMessage(`[[1,10,20,0,10],[2,10,20,10,20]]`)
	// First quarter of total height lives entirely in row 1.
	assertMatrixNear(t, slicePositionsByTextRatio(raw, 0, 0.25), [][]float64{{1, 10, 20, 0, 5}})
	// Last quarter lives entirely in row 2.
	assertMatrixNear(t, slicePositionsByTextRatio(raw, 0.75, 1), [][]float64{{2, 10, 20, 15, 20}})
}

func TestSlicePositionsByTextRatio_CrossPagePieceSpansBothRows(t *testing.T) {
	raw := json.RawMessage(`[[1,10,20,0,10],[2,10,20,10,20]]`)
	got := matrixOfRaw(t, slicePositionsByTextRatio(raw, 0.4, 0.6))
	if len(got) != 2 {
		t.Fatalf("a piece spanning the row boundary must emit both tail and head rows, got %v", got)
	}
	if math.Abs(got[0][3]-8) > posSliceEpsilon || math.Abs(got[0][4]-10) > posSliceEpsilon {
		t.Errorf("tail of row 1 wrong: %v", got[0])
	}
	if math.Abs(got[1][3]-10) > posSliceEpsilon || math.Abs(got[1][4]-12) > posSliceEpsilon {
		t.Errorf("head of row 2 wrong: %v", got[1])
	}
}

func TestSlicePositionsByTextRatio_PreservesPageNumbersAndLeftRight(t *testing.T) {
	raw := json.RawMessage(`[[7,3.5,44.25,0,40],[9,3.5,44.25,40,80]]`)
	got := matrixOfRaw(t, slicePositionsByTextRatio(raw, 0.25, 0.75))
	if len(got) == 0 {
		t.Fatal("expected a non-empty sliced matrix")
	}
	if got[0][0] != 7 || got[len(got)-1][0] != 9 {
		t.Errorf("page numbers not preserved per row: %v", got)
	}
	for _, row := range got {
		if row[1] != 3.5 || row[2] != 44.25 {
			t.Errorf("left/right altered: %v", row)
		}
	}
}

func TestSlicePositionsByTextRatio_MalformedRowsReturnNil(t *testing.T) {
	// Any malformed row (short, zero-height) invalidates the whole matrix
	// so callers fall back to the original bbox rather than silently losing
	// a page region.
	for name, raw := range map[string]json.RawMessage{
		"short row":   json.RawMessage(`[[1,10,20],[1,10,20,0,10]]`),
		"zero height": json.RawMessage(`[[1,10,20,30,30],[1,10,20,0,10]]`),
		"mixed":       json.RawMessage(`[[1,10,20],[1,10,20,30,30],[1,10,20,0,10]]`),
	} {
		if got := slicePositionsByTextRatio(raw, 0, 0.5); got != nil {
			t.Errorf("%s: expected nil for malformed matrix, got %s", name, got)
		}
	}
}

func TestSlicePositionsByTextRatio_ZeroTotalReturnsNil(t *testing.T) {
	if got := slicePositionsByTextRatio(json.RawMessage(`[[1,10,20,5,5]]`), 0, 1); got != nil {
		t.Errorf("zero-height matrix must return nil, got %s", got)
	}
}

func TestSlicePositionsByTextRatio_EmptyOrGarbageReturnsNil(t *testing.T) {
	for name, raw := range map[string]json.RawMessage{
		"empty":     {},
		"garbage":   json.RawMessage(`not-json`),
		"not-array": json.RawMessage(`{"page":1}`),
		"null":      json.RawMessage(`null`),
	} {
		if got := slicePositionsByTextRatio(raw, 0, 1); got != nil {
			t.Errorf("%s: expected nil, got %s", name, got)
		}
	}
}

func TestSlicePositionsByTextRatio_InvalidRatiosReturnNil(t *testing.T) {
	raw := json.RawMessage(`[[1,10,20,0,100]]`)
	cases := []struct{ s, e float64 }{{0.5, 0.5}, {0.8, 0.2}, {-1, -2}}
	for _, c := range cases {
		if got := slicePositionsByTextRatio(raw, c.s, c.e); got != nil && c.s >= c.e {
			t.Errorf("start=%v >= end=%v must return nil, got %s", c.s, c.e, got)
		}
	}
	// Clamping still works for slight out-of-range values.
	assertMatrixNear(t, slicePositionsByTextRatio(raw, -0.1, 1.5), [][]float64{{1, 10, 20, 0, 100}})
}

func TestSliceAnyPositions_Float64MatrixRoundTrip(t *testing.T) {
	pos := [][]float64{{1, 0, 100, 0, 100}}
	sliced := sliceAnyPositions(pos, 0, 0.5)
	mat, ok := sliced.([][]float64)
	if !ok {
		t.Fatalf("sliceAnyPositions returned %T, want [][]float64", sliced)
	}
	if len(mat) != 1 || mat[0][3] != 0 || math.Abs(mat[0][4]-50) > posSliceEpsilon {
		t.Errorf("sliced positions = %v, want [[1 0 100 0 50]]", mat)
	}
}

func TestSliceAnyPositions_NilAndUnknownTypesReturnNil(t *testing.T) {
	if got := sliceAnyPositions(nil, 0, 1); got != nil {
		t.Errorf("nil input must return nil, got %v", got)
	}
	if got := sliceAnyPositions("not-a-matrix", 0, 1); got != nil {
		t.Errorf("non-matrix value must return nil, got %v", got)
	}
	// A value with no position-like rows yields no usable slicing either.
	if got := sliceAnyPositions([]any{map[string]any{"page": 1}}, 0, 1); got != nil {
		t.Errorf("malformed matrix must return nil, got %v", got)
	}
}

func TestTotalRunes(t *testing.T) {
	if got := totalRunes([]string{"a", "中中", ""}); got != 3 {
		t.Errorf("totalRunes = %d, want 3", got)
	}
	if got := totalRunes(nil); got != 0 {
		t.Errorf("totalRunes(nil) = %d, want 0", got)
	}
}
