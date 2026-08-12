package layout

import (
	"math"
	"math/rand"
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// AssignColumn groups boxes into columns using the hybrid gap + KMeans
// strategy that beats gap-only column detection on real documents.
//
// Decision per page (mirrors tool-py/diagnose_combined.py):
//  1. Geometric gap (whitespace gutter voting) finds candidate column
//     separators. But "gap >= 2" is NOT blindly trusted:
//     - If the resulting columns are NARROW (max column width <
//     tableMaxColFrac of the page), they are table cells, not text columns:
//     the page is a single reading block -> return 1 directly (and do NOT
//     fall through to the balance gate, which would re-split the table's
//     bimodal x0 into 2).
//     - If gap == 2, the separator is unreliable (it is often a fake gutter
//     from indentation/line-width variation, not a real column). Defer to
//     the balance gate below.
//     - If gap >= 3 with WIDE columns, it is a real multi-column layout:
//     trust it and partition by KMeans(g).
//  2. When gap reports 1 (single column OR a double column whose gutter is
//     bridged by full-width front matter), or gap == 2 was deferred, a forced
//     k=2 KMeans on the BODY x0 decides whether the lines form TWO clusters
//     each holding >= minModeFrac of body lines, separated by >=
//     minSepFrac*width. A balanced split is a real second column; an
//     unbalanced split (the usual KMeans false-split on a single page) is
//     dropped -> stays 1.
//
// Net effect: tables and fake gutters no longer over-split, while the
// double-column pages that gap alone misses are recovered by the balance gate.
func AssignColumn(boxes []pdf.TextBox) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}
	pageGroups, sortedPages := groupBoxesByPage(boxes)
	result := make([]pdf.TextBox, len(boxes))
	copy(result, boxes)
	for _, pg := range sortedPages {
		indices := pageGroups[pg]
		k, cents := detectColumnCount(boxes, indices)
		assignColIDs(boxes, result, indices, k, cents)
	}
	return result
}

// tableMaxColFrac: a column narrower than this fraction of the page width is
// treated as a table cell, not a text column. Above this, the columns are
// wide enough to be real reading columns.
const tableMaxColFrac = 0.22

// maxColumnCount caps how many columns the gap detector may report. Gap
// voting can over-split a single page into many spurious gutters (e.g.
// first-line indentation), so we bound the count to the old detector's best-k
// cap of min(4, n). This prevents catastrophic splits (a single page reported
// as 7+ columns) that the old code could never produce.
const maxColumnCount = 4

// minColLineFrac: a column holding fewer than this fraction of the page's
// lines (or zero lines) is not a real reading column — it is a spurious
// gutter sliver (an indented block, a stray caption, an empty kmeans
// centroid). Drop it so the detector does not over-split.
//
// The threshold is set with margin below the smallest genuine column ratio
// observed on the 70-page labeled corpus: the sparsest real double's minority
// column is ~17.7% of lines, and the only real triple's columns are each
// >=22%. 12% prunes genuine outliers (e.g. a 4-line footnote, 7.3%) without
// touching those.
const minColLineFrac = 0.12

// detectColumnCount returns (columnCount, centroids) for one page.
// columnCount is 1, 2, or up to maxColumnCount; centroids are the k cluster
// means in x0 space (snapshot of the gate decision) and are reused for ColID
// assignment.
func detectColumnCount(boxes []pdf.TextBox, indices []int) (int, []float64) {
	lines := make([]pdf.TextBox, len(indices))
	for i, idx := range indices {
		lines[i] = boxes[idx]
	}
	g := gapColumnCount(lines, 0.04, 0.15, 2.0)
	if g >= 2 {
		_, width := pageExtent(lines)
		if width > 0 {
			widths := gapColumnWidths(lines)
			maxw := 0.0
			for _, w := range widths {
				if w > maxw {
					maxw = w
				}
			}
			if maxw < tableMaxColFrac*width {
				// Narrow columns => table cells, not text columns. The page
				// is one reading block; return 1 and skip the balance gate
				// (which would otherwise re-split the table's x0).
				return 1, nil
			}
		}
		if g > 2 {
			// gap >= 3 with wide columns: a real multi-column layout.
			// Cap the count (maxColumnCount) so spurious gutters cannot
			// split a single page into many columns, then prune empty or
			// too-sparse columns so an indentation-created sliver does not
			// survive as a spurious column.
			k := g
			if k > maxColumnCount {
				k = maxColumnCount
			}
			if k > len(lines) {
				k = len(lines)
			}
			_, w := pageExtent(lines)
			cents := kmeansCentroids(lines, k, w)
			if pk, pc, ok := pruneColumns(lines, cents); ok {
				return pk, pc
			}
			return 1, nil
		}
		// g == 2: unreliable (fake gutter or real 2-col) -> defer to balance.
	}
	if ok, cents, body := balancedBodyK2(lines, 0.30, 0.10); ok {
		// prune on the SAME body the gate clustered, not all lines: full-width
		// titles/abstracts were deliberately excluded from the balance check
		// and must not be re-counted here (they would inflate one column and
		// let prune wrongly collapse a real two-column page to one).
		if pk, pc, ok2 := pruneColumns(body, cents); ok2 {
			return pk, pc
		}
		return 1, nil
	}
	return 1, nil
}

// pruneColumns drops empty (0-line) or too-sparse (< minColLineFrac) columns
// from a k-centroid partition and returns the surviving (k', cents'). A column
// is "real" only if it captures enough of the page's lines. If fewer than 2
// real columns survive, ok is false and the caller should treat the page as a
// single column.
func pruneColumns(lines []pdf.TextBox, cents []float64) (int, []float64, bool) {
	n := len(lines)
	if n == 0 || len(cents) < 2 {
		return len(cents), cents, len(cents) >= 2
	}
	counts := make([]int, len(cents))
	for _, b := range lines {
		best, bestD := 0, math.Abs(b.X0-cents[0])
		for c := 1; c < len(cents); c++ {
			if d := math.Abs(b.X0 - cents[c]); d < bestD {
				bestD, best = d, c
			}
		}
		counts[best]++
	}
	keep := make([]int, 0, len(cents))
	for c := range cents {
		if counts[c] > 0 && float64(counts[c]) >= minColLineFrac*float64(n) {
			keep = append(keep, c)
		}
	}
	if len(keep) < 2 {
		return len(keep), nil, false
	}
	newCents := make([]float64, len(keep))
	for i, c := range keep {
		newCents[i] = cents[c]
	}
	return len(keep), newCents, true
}

// gapColumnWidths returns the width (in page units) of each column found by
// the same gutter voting as gapColumnCount. Used to tell real wide text
// columns apart from narrow table-cell columns.
func gapColumnWidths(lines []pdf.TextBox) []float64 {
	n := len(lines)
	if n == 0 {
		return nil
	}
	minX0, width := pageExtent(lines)
	if width <= 0 {
		return nil
	}
	binPt := 2.0
	nb := int(width/binPt) + 1
	cov := make([]int, nb)
	for _, b := range lines {
		i0 := clampInt(int((b.X0-minX0)/binPt), 0, nb-1)
		i1 := clampInt(int((b.X1-minX0)/binPt), 0, nb-1)
		for i := i0; i <= i1; i++ {
			cov[i]++
		}
	}
	thr := 0.15 * float64(n)
	var widths []float64
	i := 0
	for i < nb {
		if float64(cov[i]) < thr {
			i++
			continue
		}
		j := i
		for j < nb && float64(cov[j]) >= thr {
			j++
		}
		widths = append(widths, float64(j-i)*binPt)
		i = j
	}
	return widths
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// gapColumnCount mirrors column_detectors.gap_column_counts: rasterize the
// [minX0, maxX1] text region into x-bins, count how many lines cover each bin,
// and treat a covered-fraction-below-crossTol run wider than gapMinFrac*width
// as a column-separating gutter.
func gapColumnCount(lines []pdf.TextBox, gapMinFrac, crossTol, binPt float64) int {
	n := len(lines)
	if n == 0 {
		return 1
	}
	minX0, width := pageExtent(lines)
	if width <= 0 {
		return 1
	}
	minGap := gapMinFrac * width
	nb := int(width/binPt) + 1
	cov := make([]int, nb)
	for _, b := range lines {
		i0 := int((b.X0 - minX0) / binPt)
		if i0 < 0 {
			i0 = 0
		}
		i1 := int((b.X1 - minX0) / binPt)
		if i1 > nb-1 {
			i1 = nb - 1
		}
		for i := i0; i <= i1; i++ {
			cov[i]++
		}
	}
	thr := crossTol * float64(n)
	cols := 1
	run := 0.0
	for _, c := range cov {
		if float64(c) < thr {
			run += binPt
		} else {
			if run >= minGap {
				cols++
			}
			run = 0
		}
	}
	if run >= minGap {
		cols++
	}
	return cols
}

// balancedBodyK2 runs a forced k=2 KMeans on the BODY x0 (full-width front
// matter excluded) and reports whether the split is a real two-column: two
// clusters each holding >= minModeFrac of body lines, separated by >=
// minSepFrac*width. Returns the 2 cluster centroids on success, plus the body
// slice it clustered on so the caller's prune step counts the SAME line set
// (otherwise full-width lines re-inflated into one column would let prune
// collapse a balanced two-column page back to one).
func balancedBodyK2(lines []pdf.TextBox, minModeFrac, minSepFrac float64) (bool, []float64, []pdf.TextBox) {
	minX0, width := pageExtent(lines)
	if width <= 0 {
		return false, nil, nil
	}
	body := dropFullWidth(lines, width)
	if len(body) < 4 {
		return false, nil, nil
	}
	x0s := make([]float64, len(body))
	for i, b := range body {
		x0s[i] = b.X0
	}
	indentTol := width * 0.12
	sx := snapX0s(x0s, minX0, indentTol)
	labels, cents := kmeansK2PlusPlus(sx, 42)
	if len(uniqueInts(labels)) < 2 {
		return false, nil, nil
	}
	counts := make(map[int]int, 2)
	for _, l := range labels {
		counts[l]++
	}
	minCount := math.MaxInt32
	for _, c := range counts {
		if c < minCount {
			minCount = c
		}
	}
	if float64(minCount) < minModeFrac*float64(len(body)) {
		return false, nil, nil
	}
	if math.Abs(cents[0]-cents[1]) < minSepFrac*width {
		return false, nil, nil
	}
	return true, cents, body
}

// dropFullWidth removes lines whose width spans >=90% of the page text width
// (titles / abstracts / headings that legitimately bridge a gutter).
func dropFullWidth(lines []pdf.TextBox, width float64) []pdf.TextBox {
	fwThr := 0.9 * width
	out := make([]pdf.TextBox, 0, len(lines))
	for _, b := range lines {
		if b.X1-b.X0 < fwThr {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		// Every line is full-width: there is no narrow body to form a second
		// column. Return nil (not the original lines) so the caller's
		// len(body) < 4 guard treats the page as a single column instead of
		// pushing the whole page through the balance gate, which could
		// mis-split a full-width single column whose x0 happens to be bimodal.
		return nil
	}
	return out
}

// pageExtent returns minX0 (leftmost x0) and the text width (maxX1 - minX0).
func pageExtent(lines []pdf.TextBox) (minX0, width float64) {
	minX0 = math.MaxFloat64
	maxX1 := 0.0
	for _, b := range lines {
		if b.X0 < minX0 {
			minX0 = b.X0
		}
		if b.X1 > maxX1 {
			maxX1 = b.X1
		}
	}
	return minX0, maxX1 - minX0
}

// snapX0s pulls x0 values within indentTol of minX0 back to minX0, so slightly
// indented lines still cluster with the left edge (mirrors _assign_column).
func snapX0s(x0s []float64, minX0, indentTol float64) []float64 {
	out := make([]float64, len(x0s))
	for i, v := range x0s {
		if math.Abs(v-minX0) < indentTol {
			out[i] = minX0
		} else {
			out[i] = v
		}
	}
	return out
}

// kmeansK2PlusPlus is a density-aware k=2 clustering (k-means++ init, single
// Lloyd pass). Unlike util.KMeans1D (even-spaced init, a range partition), the
// first center is a random data point and the second is the farthest point, so
// it respects natural x0 density — required for the balance check to reject a
// single column whose x0 merely has a wide range. Deterministic via seed.
func kmeansK2PlusPlus(x0s []float64, seed int64) ([]int, []float64) {
	n := len(x0s)
	labels := make([]int, n)
	if n == 0 {
		return labels, nil
	}
	rng := rand.New(rand.NewSource(seed))
	first := rng.Intn(n)
	c0 := x0s[first]
	bestJ, bestD := 0, -1.0
	for j, v := range x0s {
		d := (v - c0) * (v - c0)
		if d > bestD {
			bestD, bestJ = d, j
		}
	}
	c1 := x0s[bestJ]
	cents := []float64{c0, c1}
	for iter := 0; iter < 100; iter++ {
		changed := false
		for i, v := range x0s {
			bestC := 0
			if math.Abs(v-c1) < math.Abs(v-c0) {
				bestC = 1
			}
			if labels[i] != bestC {
				changed = true
				labels[i] = bestC
			}
		}
		if !changed {
			break
		}
		sum := [2]float64{}
		cnt := [2]int{}
		for i, v := range x0s {
			sum[labels[i]] += v
			cnt[labels[i]]++
		}
		for c := 0; c < 2; c++ {
			if cnt[c] > 0 {
				cents[c] = sum[c] / float64(cnt[c])
			}
		}
	}
	return labels, cents
}

// kmeansCentroids returns the k cluster centroids from util.KMeans1D on the
// snapped x0s of all lines; used to partition a page when gap reports >=2.
func kmeansCentroids(lines []pdf.TextBox, k int, width float64) []float64 {
	minX0, _ := pageExtent(lines)
	x0s := make([]float64, len(lines))
	for i, b := range lines {
		x0s[i] = b.X0
	}
	sx := snapX0s(x0s, minX0, width*0.12)
	_, cents := util.KMeans1D(sx, k)
	return cents
}

// assignColIDs sets ColID for a page's boxes by nearest centroid, remapped so
// the leftmost centroid becomes column 0.
func assignColIDs(boxes, result []pdf.TextBox, indices []int, k int, cents []float64) {
	if k <= 1 || len(cents) == 0 {
		for _, idx := range indices {
			result[idx].ColID = 0
		}
		return
	}
	order := make([]int, len(cents))
	idxByVal := make([]int, len(cents))
	for i := range cents {
		idxByVal[i] = i
	}
	sort.Slice(idxByVal, func(a, b int) bool { return cents[idxByVal[a]] < cents[idxByVal[b]] })
	for newL, oldL := range idxByVal {
		order[oldL] = newL
	}
	for _, idx := range indices {
		x := boxes[idx].X0
		best, bestD := 0, math.Abs(x-cents[0])
		for c := 1; c < len(cents); c++ {
			if d := math.Abs(x - cents[c]); d < bestD {
				bestD, best = d, c
			}
		}
		result[idx].ColID = order[best]
	}
}

func uniqueInts(xs []int) []int {
	seen := make(map[int]struct{}, len(xs))
	for _, x := range xs {
		seen[x] = struct{}{}
	}
	out := make([]int, 0, len(seen))
	for x := range seen {
		out = append(out, x)
	}
	return out
}
