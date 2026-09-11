package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Synthetic fixtures regression test.
//
// This file is the CI-runnable guardrail for the column detector. Unlike
// TestAssignColumnCombined_Labeled (which needs the gitignored 70-page label
// sheet + 343MB corpus and is therefore skipped in CI), these fixtures are
// hand-built []pdf.TextBox geometries committed to the repo, so they run on
// every `go test ./...` with no external data.
//
// Green assertions pin behavior that must NOT regress. The title-bridged
// double is a skipped TODO that captures the #18079 acceptance target: a 2D
// spatial column detector should split it into 2, but the current x0-based
// balance gate rejects it (sparse minority + x0 overlap).

// columnCount runs AssignColumn on one page of boxes and returns the number of
// distinct columns (max ColID + 1).
func columnCount(boxes []pdf.TextBox) int {
	in := make([]pdf.TextBox, len(boxes))
	copy(in, boxes)
	for i := range in {
		in[i].PageNumber = 0
	}
	res := AssignColumn(in)
	k := 1
	for _, b := range res {
		if b.ColID+1 > k {
			k = b.ColID + 1
		}
	}
	return k
}

// stackedColumn builds n text boxes in a vertical column at [x0,x1], starting
// at top0 with line spacing dy.
func stackedColumn(x0, x1, n int, top0, dy float64) []pdf.TextBox {
	boxes := make([]pdf.TextBox, n)
	for i := 0; i < n; i++ {
		top := top0 + float64(i)*dy
		boxes[i] = pdf.TextBox{X0: float64(x0), X1: float64(x1), Top: top, Bottom: top + 8}
	}
	return boxes
}

func concat(dst, src []pdf.TextBox) []pdf.TextBox { return append(dst, src...) }

// A single reading column must stay single (no over-split).
func TestSyntheticSingleColumn(t *testing.T) {
	boxes := stackedColumn(50, 240, 10, 10, 10)
	if got := columnCount(boxes); got != 1 {
		t.Errorf("single column: got %d, want 1", got)
	}
}

// A balanced two-column page must be recovered by the balance gate.
func TestSyntheticBalancedDouble(t *testing.T) {
	boxes := concat(stackedColumn(50, 240, 10, 10, 10), stackedColumn(270, 460, 10, 10, 10))
	if got := columnCount(boxes); got != 2 {
		t.Errorf("balanced double: got %d, want 2", got)
	}
}

// Narrow side-by-side columns are a table, not text columns: returned as 1.
func TestSyntheticTableNarrowColumns(t *testing.T) {
	var boxes []pdf.TextBox
	for _, c := range [][2]int{{50, 130}, {200, 280}, {350, 430}} {
		boxes = concat(boxes, stackedColumn(c[0], c[1], 5, 10, 10))
	}
	if got := columnCount(boxes); got != 1 {
		t.Errorf("narrow table: got %d, want 1", got)
	}
}

// A real left column plus a sparse right column: the balance gate must reject
// the sparse column (minority < 30%), keeping the page single. Guards the
// sparse-column prune (minColLineFrac) against regression.
func TestSyntheticSparseSecondColumn(t *testing.T) {
	boxes := concat(stackedColumn(50, 240, 30, 10, 10), stackedColumn(300, 460, 3, 10, 20))
	if got := columnCount(boxes); got != 1 {
		t.Errorf("sparse second column: got %d, want 1", got)
	}
}

// TestSyntheticTitleBridgedDouble models a title-bridged double column: a
// CLEAN vertical gutter plus a full-width title block at the very top that
// bridges the gutter. The two body columns do NOT overlap in x0.
//
// The heading bridge is NOT a discriminant signal. What recovers the page is:
// (1) dropFullWidth drops the full-width title, and (2) detectColumnCount2D
// drops the still-wide bridging line at bridgingFrac*width, exposing the clean
// gutter; an interior-valley scan then finds exactly one gutter -> 2 columns.
//
// Geometry here: title [50,450] at top (bridges gutter only at the top);
// left body column [50,240] (30 lines); right body column [260,450] (12 lines)
// — a clean 20-unit gutter at 240–260 with no x0 overlap.
//
// Right-column line count is set to 12 on purpose:
//   - body = 30 left + 12 right = 42; 12/42 = 0.286 < 0.30 (minModeFrac) so
//     the 1D balance gate correctly rejects it (minority too small), and
//     crossTol=0.15 collapses the gutter in 1D projection -> reports 1 until
//     the 2D rescue lands.
//   - The right column carries 12/30 = 0.40 of the page peak glyph ink, i.e.
//     above valleyFrac*peak (0.30), so it BOUNDS the gutter and the valley
//     scan recovers it -> 2 columns. NOTE: this is the real capability of the
//     rescue — it needs the minority column above ~30% of peak ink. A truly
//     sparse column (e.g. 6 lines = 0.20 of peak) merges with the gutter and
//     is NOT recovered; those fall back to the confidence-labeling track.
//   - 12 >= 0.12*42 = 5.04, so the both-sides prune gate (minColLineFrac)
//     accepts it. TestSyntheticSparseSecondColumn (right=3, 3 < 0.12*33) stays
//     1, so the recover/stay-1 split is carried by right-column count.
func TestSyntheticTitleBridgedDouble(t *testing.T) {
	boxes := concat(
		concat(
			stackedColumn(50, 450, 3, 10, 10),  // full-width title (bridges gutter only at top)
			stackedColumn(50, 240, 30, 40, 10), // left body column [50,240]
		),
		stackedColumn(260, 450, 12, 40, 20), // right body column [260,450]: 12 lines, clean gutter 240–260
	)
	if got := columnCount(boxes); got != 2 {
		t.Errorf("title-bridged double: got %d, want 2", got)
	}
}

// TestSyntheticMedianWidthDouble locks the L3 signal (PR #10475's
// page_w/median_w): a gutter-less double whose two columns are ADJACENT in x0
// (no whitespace gutter), so there is no clean ink dip for gap/balance/2D-
// rescue to find, yet each line is only ~half the page wide (raw_cols =
// page_w/median_w = 2).
//
// Geometry: left body column [50,250] (30 lines), right body column [250,450]
// (12 lines) — adjacent at x=250, so the x-projection is one continuous ink
// band with no >= gapMinFrac run -> gap=1. Balance rejects it (right is
// 12/42 = 0.286 < minModeFrac 0.30). The 2D rescue also fails (no clean
// interior valley). Only the median-width ratio (raw_cols=2, a line spans 200
// >= 0.45*400=180) recovers it -> 2 columns.
//
// This pins the #18079 acceptance target that the x0-based detectors alone
// cannot reach: a real double with no geometric gutter.
func TestSyntheticMedianWidthDouble(t *testing.T) {
	boxes := concat(
		stackedColumn(50, 250, 30, 10, 10),  // left body column [50,250]
		stackedColumn(250, 450, 12, 10, 20), // right body column [250,450]: adjacent, no gutter
	)
	if got := columnCount(boxes); got != 2 {
		t.Errorf("median-width gutter-less double: got %d, want 2", got)
	}
}

// TestSyntheticMedianWidthDoubleWithTitle locks the L3 median detector's prune
// discipline: it must prune on the SAME non-full-width lines that produced the
// centroids, not the full line set. A gutter-less double with a full-width
// title exercises the path where, before the fix, the title (counted by
// pruneColumns into the left column) inflated n and collapsed the sparse right
// column to a single column. After the fix the detector recovers 2.
//
// Geometry: left [50,250] (29 lines), right [250,450] (4 lines) — adjacent,
// no gutter; plus one full-width title [50,450]. Without the title the right
// column is 4/33 = 0.121 >= minColLineFrac (0.12) -> 2; with the title counted
// in, 4/34 = 0.118 < 0.12 -> 1 (the pre-fix bug).
func TestSyntheticMedianWidthDoubleWithTitle(t *testing.T) {
	boxes := concat(
		concat(
			stackedColumn(50, 250, 29, 10, 10), // left body column [50,250]
			stackedColumn(250, 450, 4, 10, 20), // right body column [250,450]: adjacent, no gutter
		),
		stackedColumn(50, 450, 1, 10, 10), // full-width title [50,450]
	)
	if got := columnCount(boxes); got != 2 {
		t.Errorf("median-width gutter-less double with title: got %d, want 2", got)
	}
}
