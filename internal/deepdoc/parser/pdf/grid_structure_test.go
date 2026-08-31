//go:build cgo

package pdf

import (
	"math"
	"testing"
)

// mkgrid builds an r×c grid of empty cells (cell text is irrelevant to the
// structural metric under test).
func mkgrid(r, c int) [][]string {
	g := make([][]string, r)
	for i := range g {
		g[i] = make([]string, c)
	}
	return g
}

// TestGridStructureSimilarity verifies gridStructureSimilarity is SHAPE-AWARE:
// it catches table-segmentation divergences (e.g. Go splitting a rotated table
// into 3x6 while Python merges it into 8x7) even when both grids contain the
// identical cell text — which gridSim (CharSimilarity over joined text) cannot
// see. This is the regression guard behind surfacing table_rotation_test.pdf
// as INTENTIONAL instead of mislabeling it html-divergent.
func TestGridStructureSimilarity(t *testing.T) {
	mkTextGrid := func(rows ...[]string) [][]string {
		return rows
	}
	cases := []struct {
		name       string
		goRows     [][]string
		pyRows     [][]string
		wantSim    float64
		wantDetail string
	}{
		{
			name:    "identical 3x6",
			goRows:  mkgrid(3, 6),
			pyRows:  mkgrid(3, 6),
			wantSim: 100, wantDetail: "3x6",
		},
		{
			name:       "rotated 3x6 vs merged 8x7",
			goRows:     mkgrid(3, 6),
			pyRows:     mkgrid(8, 7),
			wantSim:    0,
			wantDetail: "3x6 vs 8x7",
		},
		{
			name:    "both empty",
			goRows:  nil,
			pyRows:  nil,
			wantSim: 100, wantDetail: "0x0",
		},
		{
			name:       "row count differs 5x3 vs 8x7",
			goRows:     mkgrid(5, 3),
			pyRows:     mkgrid(8, 7),
			wantSim:    0,
			wantDetail: "5x3 vs 8x7",
		},
		{
			name:       "uneven column counts",
			goRows:     mkTextGrid([]string{"a", "b"}, []string{"c"}),
			pyRows:     mkTextGrid([]string{"a", "b"}, []string{"c", "d"}),
			wantSim:    66.67,
			wantDetail: "2x[2,1] vs 2x2",
		},
		{
			// Shape matches -> 100 even though cell TEXT differs. Confirms the
			// metric is purely structural, not a content re-test.
			name:       "same shape, different text",
			goRows:     mkTextGrid([]string{"x", "y"}, []string{"z", "w"}),
			pyRows:     mkTextGrid([]string{"p", "q"}, []string{"r", "s"}),
			wantSim:    100,
			wantDetail: "2x2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sim, detail := gridStructureSimilarity(tc.goRows, tc.pyRows)
			if math.Abs(sim-tc.wantSim) > 0.01 {
				t.Errorf("sim = %.2f, want %.2f", sim, tc.wantSim)
			}
			if detail != tc.wantDetail {
				t.Errorf("detail = %q, want %q", detail, tc.wantDetail)
			}
		})
	}
}
