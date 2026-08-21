//go:build cgo

package pdf

import (
	"fmt"
	"strings"
)

// gridShape returns a compact STRUCTURAL signature of a grid, ignoring all
// cell text: the row count and, when every row shares the same column count,
// "RxC"; otherwise "Rx[colcounts]" so divergent shapes stay distinguishable.
// Examples: "3x6", "8x7", "2x[2,1]". Two grids with identical shape share the
// same signature even when their cell text differs.
func gridShape(g [][]string) string {
	if len(g) == 0 {
		return "0x0"
	}
	cols := make([]int, len(g))
	allSame := true
	first := len(g[0])
	for i, row := range g {
		cols[i] = len(row)
		if cols[i] != first {
			allSame = false
		}
	}
	if allSame {
		return fmt.Sprintf("%dx%d", len(g), first)
	}
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf("%d", c)
	}
	return fmt.Sprintf("%dx[%s]", len(g), strings.Join(parts, ","))
}

// gridStructureSimilarity reports how closely two grids' ROW/COLUMN structure
// matches, independent of cell text. Unlike gridSim (CharSimilarity over
// joined cell text, which is order- and shape-blind), this catches table
// segmentation divergences: Go splitting a rotated table into 3x6 while Python
// merges it into 8x7 yields gridSim=100% (identical cell text) but
// structureSim<100% (different shape).
//
// Returns (similarity, detail) where detail is "<goShape> vs <pyShape>" for
// logging. Similarity is 100 when both grids are empty or have identical row
// counts and identical per-row column counts; otherwise it is graded by the
// fraction of structural elements that match (1 unit for matching row count +
// 1 unit per row whose column count matches, normalized).
func gridStructureSimilarity(goRows, pyRows [][]string) (float64, string) {
	gs, ps := gridShape(goRows), gridShape(pyRows)
	if gs == ps {
		return 100, gs
	}
	n := len(goRows)
	if len(pyRows) > n {
		n = len(pyRows)
	}
	if n == 0 {
		return 100, gs + " vs " + ps // both empty -> identical
	}
	match := 0
	if len(goRows) == len(pyRows) {
		match++
	}
	compared := len(goRows)
	if len(pyRows) < compared {
		compared = len(pyRows)
	}
	for i := 0; i < compared; i++ {
		if len(goRows[i]) == len(pyRows[i]) {
			match++
		}
	}
	total := 1 + compared
	return float64(match) / float64(total) * 100, gs + " vs " + ps
}
