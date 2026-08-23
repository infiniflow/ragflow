package layout

import (
	"math"
	"math/rand"
	"sort"
	"unicode/utf8"

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

	// Document-wide column majority, mirroring Python's _assign_column
	// (pdf_parser.py:942 global_cols): per-page detectors (gap voting / balance
	// gate / 2D rescue) can mis-read indent-heavy single-column pages — e.g.
	// 刑法's TOC and code-like pages, whose lines share a full-width right edge
	// but start at staggered x0 (footnote 79, body 111, indents 144/176/270) —
	// as 2-3 columns, which scrambles the reading order. Python sidesteps this
	// by re-clustering EVERY page with the majority column count. Mirror that
	// for the single-column majority: if most pages read as one column, the
	// whole document reads as one column (ColID 0), so a few indent-staggered
	// pages cannot break the flow. A multi-column majority keeps the per-page
	// detector result (those documents are genuinely multi-column throughout).
	// Deterministic majority matching Python's _assign_column global_cols
	// (Counter(page_cols.values()).most_common(1)): the most frequent per-page
	// column count wins; on a tie the FIRST page's count wins (Python keeps
	// dict insertion order). Iterate in page order so the result never depends
	// on Go map iteration order.
	colCount := map[int]int{}
	var firstK []int
	for _, pg := range sortedPages {
		k, _ := detectColumnCount(boxes, pageGroups[pg])
		if colCount[k] == 0 {
			firstK = append(firstK, k)
		}
		colCount[k]++
	}
	modeK, modeN := firstK[0], colCount[firstK[0]]
	for _, k := range firstK {
		if colCount[k] > modeN {
			modeK, modeN = k, colCount[k]
		}
	}
	if modeK == 1 {
		for i := range result {
			result[i].ColID = 0
		}
		return result
	}

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

// maxPageExtent caps the X span a single line may plausibly occupy. A line
// wider than this is treated as a malformed coordinate (mirrors pdf-inspector's
// MAX_PAGE_EXTENT=14400 guard) and excluded from the column projection so it
// cannot balloon the page extent and collapse multi-column detection.
const maxPageExtent = 14400.0

// maxTrimFraction is the largest fraction of lines robustPageExtent may discard
// as outliers. If more than this fraction is anomalous, the page is trusted
// as-is: the "anomalies" are the norm, not noise.
const maxTrimFraction = 0.10

// maxBins caps the histogram allocation in gapColumnCount/detectColumnCount2D so
// a malformed (ballooned) page extent cannot trigger an OOM-scale allocation
// (mirrors pdf-inspector's bin cap). When the extent is huge, the bin is
// widened so the projection still resolves real gutters.
const maxBins = 65536

// gapMinFrac: a horizontal run of low coverage counts as a column gap only if
// it is at least this fraction of the page width. Reused by both gapColumnCount
// and the 2D both-sides gutter rescue so the two detectors agree on what a
// "real" gutter width is.
const gapMinFrac = 0.04

// binPt: x-binning resolution (points) for the 1D gap histogram and the 2D
// gutter scan. Sharing it keeps the gap and gutter detectors aligned.
const binPt = 2.0

// detectColumnCount returns (columnCount, centroids) for one page.
// columnCount is 1, 2, or up to maxColumnCount; centroids are the k cluster
// means in x0 space (snapshot of the gate decision) and are reused for ColID
// assignment.
func detectColumnCount(boxes []pdf.TextBox, indices []int) (int, []float64) {
	lines := make([]pdf.TextBox, len(indices))
	for i, idx := range indices {
		lines[i] = boxes[idx]
	}
	g := gapColumnCount(lines, gapMinFrac, 0.15, binPt)
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
	// 2D rescue: a clean vertical gutter the 1D projection masks via bridging
	// rows (full-width front matter + in-body headings/captions). Recovers
	// title-bridged doubles the balance gate correctly rejects (sparse
	// minority). Runs only after both gap>=2 and the balance gate fail, so it
	// never touches already-correct pages.
	if k, cents := detectColumnCount2D(lines); k >= 2 {
		return k, cents
	}
	// L3: median-width-ratio complement (PR #10475). Fires only after gap,
	// balance, and the 2D valley rescue all returned 1, so it never touches
	// the pages they already handle. Targets "gutter-less" doubles/triples.
	if k, cents := detectColumnCountMedian(lines); k >= 2 {
		return k, cents
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
	// A1: image/equation placeholders must not feed the projection — a figure
	// spanning the gutter would otherwise fill the gap and mask a real column
	// boundary.
	lines = textProjectionLines(lines)
	if len(lines) == 0 {
		return 1
	}
	// A2: robust page extent discards malformed/outlier lines so a single bad
	// box cannot balloon the width and collapse detection to one column.
	minX0, width := robustPageExtent(lines)
	if width <= 0 {
		return 1
	}
	minGap := gapMinFrac * width
	// Cap the bin count so a ballooned extent cannot allocate an OOM-scale
	// histogram. When the extent is huge, widen the bin so the projection still
	// resolves real gutters.
	effBin := binPt
	if width/float64(maxBins) > effBin {
		effBin = width / float64(maxBins)
	}
	nb := int(width/effBin) + 1
	if nb < 1 {
		nb = 1
	}
	cov := make([]int, nb)
	for _, b := range lines {
		i0 := int((b.X0 - minX0) / effBin)
		if i0 < 0 {
			i0 = 0
		}
		i1 := int((b.X1 - minX0) / effBin)
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
			run += effBin
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

// bridgingFrac: lines wider than this fraction of the page text width are
// treated as bridging elements — full-width front matter (already dropped by
// dropFullWidth at 0.9) plus partially-wide in-body headings/captions that
// span the gutter. Dropping them before the valley scan is what exposes the
// clean gutter of a title-bridged double column. 0.60 is the sweet spot
// measured on the 70-page corpus: lower (0.50) leaves too few real column
// lines on single pages and keeps enough bridging width to still hide some
// gutters; higher (0.65) lets the sparse bridging lines that hide the target
// gutters survive.
const bridgingFrac = 0.60

// medianFullWidthFrac is the full-width threshold for the L3 median-width
// detector. It is lower than dropFullWidth's 0.9 because the median path
// buckets by normalized center x and only needs to exclude lines that would
// otherwise dominate every bucket; lines between 0.8 and 0.9 width are rare
// and keeping them out of the buckets avoids a single wide line skewing cents.
const medianFullWidthFrac = 0.80

// medianColCap: max column count the median-width-ratio signal (L3, from PR
// #10475's page_w/median_w) may assign. Capped at 3 so a raw_cols estimate of
// 4 (common on 3-column pages) does not over-shoot, and well under
// maxColumnCount.
const medianColCap = 3

// shortLineFrac: L3 requires at least one line spanning >= this fraction of
// the page width. A real multi-column page has lines that span a column
// (~page_w/N); a single page of uniformly short lines also has a small median
// width, but no near-full-width line — this guard filters those false doubles.
const shortLineFrac = 0.45

// detectColumnCount2D is a rescue detector for title-bridged double columns:
// pages whose two body columns are separated by a clean gutter that the 1D x0
// projection loses once full-width front matter (and in-body bridging
// headings/captions) spans it. It runs only after gap>=2 and the balance gate
// both fail, so it never touches already-correct pages.
//
// Method (faithful to tool-py/column_detectors.gap_glyph_body_column_counts,
// extended with bridging removal): project the BODY — full-width lines dropped
// by dropFullWidth, then any still-wide bridging line dropped at
// bridgingFrac*width — onto the x-axis with glyph-count-per-bin weighting
// (each covered bin receives the line's full rune count, so a wide line
// contributes proportionally more), and look
// for INTERIOR valleys (low-ink runs bounded by high ink on both sides, wider
// than gapMinFrac*width, and not at the page edge). A single clean gutter
// splits the page into two real columns.
//
// The rescue ACCEPTS only exactly one interior valley (k=2). Zero valleys
// means no clean gutter (keep single). Two or more valleys means either a
// multi-column layout (already handled by the gap path) or a single page with
// a vertical blank band (figure/equation) — both are rejected so the rescue
// never over-splits a single column into 3+. The both-sides prune gate
// (pruneColumns, minColLineFrac) is the final guard: a spurious second block
// with too few lines is dropped.
func detectColumnCount2D(lines []pdf.TextBox) (int, []float64) {
	// A1: strip figure/equation boxes (they span the gutter and would fill the
	// projection, hiding a real column boundary). A2: robust extent so a
	// malformed box cannot balloon the page width / histogram allocation.
	projLines := textProjectionLines(lines)
	minX0, width := robustPageExtent(projLines)
	if width <= 0 {
		return 0, nil
	}
	body := dropFullWidth(projLines, width)
	body = dropWide(body, width, bridgingFrac)
	if len(body) < 4 {
		return 0, nil
	}
	// Cap the bin count so a ballooned extent cannot allocate an OOM-scale
	// histogram. When the extent is huge, widen the bin so the projection still
	// resolves real gutters.
	effBin := binPt
	if width/float64(maxBins) > effBin {
		effBin = width / float64(maxBins)
	}
	nb := int(width/effBin) + 1
	proj := make([]int, nb)
	for _, b := range body {
		w := utf8.RuneCountInString(b.Text)
		if w <= 0 {
			w = 1
		}
		i0 := clampInt(int((b.X0-minX0)/effBin), 0, nb-1)
		i1 := clampInt(int((b.X1-minX0)/effBin), 0, nb-1)
		for i := i0; i <= i1; i++ {
			proj[i] += w
		}
	}
	pk := 0
	for _, p := range proj {
		if p > pk {
			pk = p
		}
	}
	if pk == 0 {
		return 0, nil
	}
	// A gutter is a run of bins whose glyph-weight is below valleyFrac of the
	// page peak. Measured on the 70-page corpus this relative threshold (the
	// tool-py reference value) is what actually recovers title-bridged doubles:
	// their gutter is clean (≈0 glyphs) and the minority column still carries
	// enough ink to sit above valleyFrac*peak and bound the gutter. A minority
	// column below ~30% of peak ink (e.g. a very sparse 6-line column) merges
	// with the gutter and is NOT recovered — that is a real limitation, not a
	// bug; such pages fall back to the confidence-labeling track (issue #18079).
	const valleyFrac = 0.30
	minGap := gapMinFrac * width
	edge := int(0.05 * width / effBin)
	if edge < 0 {
		edge = 0
	}
	// Find interior valleys; accept ONLY a single clean gutter (k=2). Zero
	// valleys means no clean gutter (keep single). Two or more valleys means
	// either a multi-column layout (handled by the gap path) or a single page
	// with a vertical blank band (figure/equation) — both are rejected so the
	// rescue never over-splits a single column into 3+.
	var valleyC float64
	count := 0
	i := 0
	for i < nb {
		if float64(proj[i]) < valleyFrac*float64(pk) {
			j := i
			for j < nb && float64(proj[j]) < valleyFrac*float64(pk) {
				j++
			}
			runW := float64(j-i) * effBin
			isInterior := i > edge && j-1 < nb-1-edge
			if runW >= minGap && isInterior {
				count++
				valleyC = minX0 + float64(i+j)*effBin/2
			}
			i = j
		} else {
			i++
		}
	}
	if count != 1 {
		return 0, nil
	}
	// Split the body at the single valley into left/right blocks; each
	// centroid is the mean X0 of its lines. Classify by b.X0 (not the center)
	// so the split agrees with pruneColumns' X0-based assignment — lines whose
	// center and X0 fall on opposite sides of the valley would otherwise be
	// counted differently by the two steps.
	var leftSum, rightSum float64
	lc, rc := 0, 0
	for _, b := range body {
		if b.X0 < valleyC {
			leftSum += b.X0
			lc++
		} else {
			rightSum += b.X0
			rc++
		}
	}
	if lc == 0 || rc == 0 {
		return 0, nil
	}
	cents := []float64{leftSum / float64(lc), rightSum / float64(rc)}
	if pk2, pc, ok := pruneColumns(body, cents); ok {
		return pk2, pc
	}
	return 0, nil
}

// dropWide removes lines whose width spans >= frac of the page text width.
// Used by detectColumnCount2D to strip in-body bridging headings/captions
// (partially-wide lines that span the gutter but are not full-width front
// matter) before the valley scan. Returns nil if every line is wide so the
// caller treats the page as single rather than pushing an empty body through.
func dropWide(lines []pdf.TextBox, width, frac float64) []pdf.TextBox {
	if frac >= 1 {
		return lines
	}
	thr := frac * width
	out := make([]pdf.TextBox, 0, len(lines))
	for _, b := range lines {
		if b.X1-b.X0 < thr {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// medianWidth returns the median box width on the page. Used by the L3
// median-width-ratio column signal.
func medianWidth(lines []pdf.TextBox) float64 {
	if len(lines) == 0 {
		return 1.0
	}
	ws := make([]float64, len(lines))
	for i, b := range lines {
		ws[i] = b.X1 - b.X0
		if ws[i] < 1 {
			ws[i] = 1
		}
	}
	sort.Float64s(ws)
	n := len(ws)
	if n%2 == 1 {
		return ws[n/2]
	}
	return (ws[n/2-1] + ws[n/2]) / 2.0
}

// maxWidth returns the widest box on the page.
func maxWidth(lines []pdf.TextBox) float64 {
	m := 0.0
	for _, b := range lines {
		if w := b.X1 - b.X0; w > m {
			m = w
		}
	}
	return m
}

// detectColumnCountMedian is the L3 complementary signal, inspired by PR
// #10475's _assign_column (page_w / median_line_width). It fires only after
// gap, the balance gate, and the 2D valley rescue have ALL returned a single
// column, so it never touches the pages they already handle correctly.
//
// It targets "gutter-less" doubles/triples: pages whose two (or three) body
// columns are separated by a gutter so narrow/bridged that the x-projection
// has no clean ink dip — so the geometric detectors miss them, yet each line
// is only ~page_w/N wide, giving raw_cols = page_w/median_w >= 2.
//
// Two gates keep it safe (measured on the 70-page corpus):
//   - raw_cols > maxColumnCount (4): a huge ratio means table cells, not text
//     columns (narrow cells yield a tiny median width) -> skip.
//   - no line spanning >= shortLineFrac*page_w: the page is one column of
//     uniformly short lines whose small median width is not a real multi-column
//     signal -> skip.
//
// When it fires, columns are assigned by normalized center-x bucketing
// (matching PR #10475's col_id assignment); the bucket means become centroids.
func detectColumnCountMedian(lines []pdf.TextBox) (int, []float64) {
	minX0, width := pageExtent(lines)
	if width <= 0 {
		return 0, nil
	}
	mw := medianWidth(lines)
	if mw < 1 {
		mw = 1
	}
	raw := int(width / mw)
	if raw < 2 {
		return 0, nil
	}
	if raw > maxColumnCount {
		// Table-like (narrow cells): not a text layout.
		return 0, nil
	}
	if maxWidth(lines) < shortLineFrac*width {
		// Uniformly short lines, not real columns.
		return 0, nil
	}
	k := raw
	if k > medianColCap {
		k = medianColCap
	}
	if k < 2 {
		k = 2
	}
	// Bucket non-full-width lines by normalized center x, mirroring PR #10475.
	// Collect the SAME non-full-width lines into body so the prune step counts
	// the line set that produced the centroids — otherwise full-width
	// titles/abstracts (which the bucket loop skips) would be re-counted by
	// pruneColumns, inflate one column, and let a real multi-column page
	// collapse to one. This is the same discipline balancedBodyK2 and
	// detectColumnCount2D already follow.
	fwThr := medianFullWidthFrac * width
	body := make([]pdf.TextBox, 0, len(lines))
	buckets := make([][]float64, k)
	for _, b := range lines {
		if b.X1-b.X0 >= fwThr {
			continue
		}
		body = append(body, b)
		cx := 0.5 * (b.X0 + b.X1)
		norm := (cx - minX0) / width
		if norm < 0 {
			norm = 0
		}
		if norm > 0.999999 {
			norm = 0.999999
		}
		bkt := int(norm * float64(k))
		if bkt > k-1 {
			bkt = k - 1
		}
		buckets[bkt] = append(buckets[bkt], b.X0)
	}
	cents := make([]float64, 0, k)
	for _, bx := range buckets {
		if len(bx) == 0 {
			continue
		}
		var s float64
		for _, x := range bx {
			s += x
		}
		cents = append(cents, s/float64(len(bx)))
	}
	if len(cents) < 2 {
		return 0, nil
	}
	// Require each column to hold enough lines (the same guard used by the
	// rest of the detector) so a sparse side column does not become a false
	// split.
	if pk, pc, ok := pruneColumns(body, cents); ok {
		return pk, pc
	}
	return 0, nil
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

// textProjectionLines returns the lines that should feed the column
// projection. Image and equation placeholders are excluded because a figure
// that spans the gutter would otherwise fill the gutter's gap and mask a real
// column boundary (mirrors pdf-inspector stripping image placeholders). Table
// boxes are intentionally kept: the existing tableMaxColFrac gate already
// handles narrow table columns, and dropping them here would widen the blast
// radius unnecessarily.
func textProjectionLines(lines []pdf.TextBox) []pdf.TextBox {
	out := make([]pdf.TextBox, 0, len(lines))
	for _, b := range lines {
		switch b.LayoutType {
		case pdf.LayoutTypeFigure, pdf.LayoutTypeEquation:
			continue
		default:
			out = append(out, b)
		}
	}
	return out
}

// robustPageExtent returns the X extent (minX0, width) of the text body,
// discarding a small number of outlier lines (malformed coordinates / stray
// boxes) that would otherwise balloon the extent and collapse multi-column
// detection. Normal pages return exactly pageExtent's result.
func robustPageExtent(lines []pdf.TextBox) (minX0, width float64) {
	if len(lines) == 0 {
		return 0, 0
	}
	// Drop implausibly wide (malformed) lines outright: a single box with an
	// absurd X1 (e.g. 1e6) would otherwise set the page width to 1e6.
	work := make([]pdf.TextBox, 0, len(lines))
	dropped := 0
	for _, b := range lines {
		if b.X1-b.X0 > maxPageExtent {
			dropped++
			continue
		}
		work = append(work, b)
	}
	if len(work) == 0 || float64(dropped)/float64(len(lines)) >= maxTrimFraction {
		// Everything malformed, or too many dropped: the outliers are the
		// norm. Fall back to the raw extent so the page is never altered.
		return pageExtent(lines)
	}
	// Cluster lines by left edge; discard clusters separated from the main
	// body by more than a full page width, provided they are a minority.
	return clusteredExtent(work)
}

// clusteredExtent keeps the largest cluster of lines (by count) and returns its
// extent, but only when the discarded minority is below maxTrimFraction;
// otherwise it returns the raw extent of all lines. This catches outliers whose
// width alone is plausible (e.g. a tiny box placed at an absurd X coordinate).
func clusteredExtent(lines []pdf.TextBox) (minX0, width float64) {
	if len(lines) <= 1 {
		return pageExtent(lines)
	}
	xs := make([]float64, len(lines))
	for i, b := range lines {
		xs[i] = b.X0
	}
	sort.Float64s(xs)
	// Split into clusters at gaps larger than a full page.
	clusters := [][]float64{{xs[0]}}
	for i := 1; i < len(xs); i++ {
		if xs[i]-xs[i-1] > maxPageExtent {
			clusters = append(clusters, []float64{xs[i]})
		} else {
			clusters[len(clusters)-1] = append(clusters[len(clusters)-1], xs[i])
		}
	}
	if len(clusters) == 1 {
		return pageExtent(lines)
	}
	best := 0
	for i := 1; i < len(clusters); i++ {
		if len(clusters[i]) > len(clusters[best]) {
			best = i
		}
	}
	dropped := len(lines) - len(clusters[best])
	if float64(dropped)/float64(len(lines)) >= maxTrimFraction {
		return pageExtent(lines)
	}
	lo, hi := clusters[best][0], clusters[best][0]
	for _, x := range clusters[best] {
		if x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	return lo, hi - lo
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
