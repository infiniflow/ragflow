// TOC removal by page geometry.
//
// Table-of-contents pages are detected at line granularity (before
// NaiveVerticalMerge collapses the lines into blocks): a page whose lines
// form a right-aligned, monotonically increasing page-number column is a TOC
// page. This keying on the page-number column is deliberately independent of
// the leader character used by the typesetter — dots, ellipses, middots,
// wide spaces, or none at all — and of whether DeepDoc extracted each entry
// as one line or split the title and its page number into separate columns
// (a layout this repo observed on real book PDFs).
//
// Deletion policy:
//   - A page whose entries dominate the text AND whose every other line is a
//     short, isolated run (cover title, watermark, page marker, TOC heading)
//     is a dedicated TOC page: every box on the page is dropped.
//   - Otherwise (mixed page) only the entry boxes are dropped, plus a
//     wrapped-entry title line sitting directly above a bare page number and
//     a standalone TOC heading above the column. Prose, tables and figures
//     on the page are kept.
package layout

import (
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/type"
)

// tocEntryShapePattern matches the trailing "page number" of a candidate
// entry line: 1-4 digits with an optional roman-numeral fragment, e.g.
// ".20", "6", "..8", "……39 II", "第27章 ……37". The leading text may be a
// title or nothing at all (a bare page reference on its own line).
var tocEntryShapePattern = regexp.MustCompile(`(\d{1,4})([IVXLCivxlc]{0,5})$`)

// tocLeaderChars are the characters that may make up a bare page reference's
// leader: whitespace, dots, middots, ellipses and CJK separators. If a
// candidate line is ONLY leaders followed by the page number it is a
// number-only reference (the wrapped-entry shape); anything else is a
// titled entry.
var tocLeaderChars = " \t.·…⋯‥，。、"

const (
	// tocRightEdgeTolerance is the maximum spread (PDF points) allowed
	// among the right edges of one page-number column.
	tocRightEdgeTolerance = 3.0
	// tocMinClusterSize is the minimum number of aligned page numbers that
	// proves a column is a page-number column, not prose with one or two
	// stray numbers.
	tocMinClusterSize = 3
	// tocShortRunMaxRunes bounds a continuous run of non-entry lines. Only
	// runs of two or more chained lines are length-checked (a single
	// isolated line, however long — a URL watermark — is not prose).
	// Body paragraphs chain past the cap; TOC heading lines and titles stay
	// under it.
	tocShortRunMaxRunes = 50
	// tocGapFactor scales the per-page median box height into the vertical
	// gap below which two lines count as one continuous run.
	tocGapFactor = 1.5
	// tocPartnerX0Tolerance is how far a wrapped-entry title line may sit
	// from the bare page-number line it completes (PDF points).
	tocPartnerX0Tolerance = 10.0
	// tocHeadingGapFactor scales the gap between a standalone TOC heading
	// and the topmost entry of its column.
	tocHeadingGapFactor = 2.0
)

// FilterTOCBoxes drops table-of-contents boxes from the stream. It must run
// on line-shaped boxes (TextMerge output, before FinalReadingOrderMerge /
// NaiveVerticalMerge) so the per-line right-edge alignment is still visible.
// medianHeights is the per-page median box height used for vertical
// contiguity decisions; a missing/zero value falls back to 10pt. The input
// slice is not mutated; the returned slice preserves the input order.
func FilterTOCBoxes(boxes []pdf.TextBox, medianHeights map[int]float64) []pdf.TextBox {
	if len(boxes) == 0 {
		return boxes
	}
	pageGroups, sortedPages := groupBoxesByPage(boxes)
	drop := make([]bool, len(boxes))

	for _, pg := range sortedPages {
		pageIdx := pageGroups[pg]
		mh := medianHeights[pg]
		if mh <= 0 {
			mh = 10
		}

		eligible := eligibleTOCBoxes(boxes, pageIdx)
		if len(eligible) < tocMinClusterSize {
			continue
		}
		candidates := tocCandidates(boxes, eligible)

		// Cluster candidates by right edge: each cluster is one page-number
		// column. Two-column TOCs produce two independent clusters.
		var entries []int
		for _, cluster := range clusterByRightEdge(boxes, candidates) {
			if qualifyingTOCColumn(boxes, cluster) {
				for _, c := range cluster {
					entries = append(entries, c.idx)
				}
			}
		}
		if len(entries) == 0 {
			continue
		}

		if dedicatedTOCPage(boxes, pageIdx, eligible, entries, mh) {
			for _, i := range pageIdx {
				drop[i] = true
			}
			continue
		}

		// Mixed page: drop the entries, wrapped-title partners and a TOC
		// heading; keep everything else.
		for _, i := range entries {
			drop[i] = true
		}
		for _, i := range tocWrappedTitlePartners(boxes, eligible, entries, mh) {
			drop[i] = true
		}
		if h := tocHeadingBox(boxes, eligible, entries, mh); h >= 0 {
			drop[h] = true
		}
	}

	out := make([]pdf.TextBox, 0, len(boxes))
	for i, b := range boxes {
		if drop[i] {
			continue
		}
		out = append(out, b)
	}
	return out
}

// eligibleTOCBoxes returns the indices of page boxes that are real text
// candidates: tables, figures and equations are excluded because their rows
// often end in right-aligned numbers but are not TOC entries.
func eligibleTOCBoxes(boxes []pdf.TextBox, pageIdx []int) []int {
	out := make([]int, 0, len(pageIdx))
	for _, i := range pageIdx {
		switch boxes[i].LayoutType {
		case pdf.LayoutTypeTable, pdf.LayoutTypeFigure, pdf.LayoutTypeEquation:
			continue
		}
		if strings.TrimSpace(boxes[i].Text) == "" {
			continue
		}
		out = append(out, i)
	}
	return out
}

// tocCandidate is an eligible line whose text ends in a page-number shape.
type tocCandidate struct {
	idx        int
	right      float64
	top        float64
	num        int
	numberOnly bool
	runes      int
}

// tocCandidates extracts the entry-shaped lines (trailing 1-4 digit page
// number, optional roman fragment). A line is numberOnly when everything
// before the page number is just leader characters — the wrapped-entry
// shape whose title lives on the line above.
func tocCandidates(boxes []pdf.TextBox, eligible []int) []tocCandidate {
	out := make([]tocCandidate, 0, len(eligible))
	for _, i := range eligible {
		text := strings.TrimSpace(boxes[i].Text)
		m := tocEntryShapePattern.FindStringSubmatchIndex(text)
		if m == nil {
			continue
		}
		num := 0
		for _, r := range text[m[2]:m[3]] {
			num = num*10 + int(r-'0')
		}
		prefix := strings.Trim(text[:m[2]], tocLeaderChars)
		out = append(out, tocCandidate{
			idx:        i,
			right:      boxes[i].X1,
			top:        boxes[i].Top,
			num:        num,
			numberOnly: prefix == "",
			runes:      utf8.RuneCountInString(text),
		})
	}
	return out
}

// clusterByRightEdge greedily groups candidates whose right edges fall
// within tocRightEdgeTolerance. The first member anchors the cluster.
func clusterByRightEdge(boxes []pdf.TextBox, candidates []tocCandidate) [][]tocCandidate {
	sorted := append([]tocCandidate(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].right < sorted[j].right
	})
	var clusters [][]tocCandidate
	for _, c := range sorted {
		if len(clusters) == 0 || c.right-clusters[len(clusters)-1][0].right > tocRightEdgeTolerance {
			clusters = append(clusters, []tocCandidate{c})
			continue
		}
		clusters[len(clusters)-1] = append(clusters[len(clusters)-1], c)
	}
	return clusters
}

// qualifyingTOCColumn reports whether a right-edge cluster is a genuine
// page-number column: at least tocMinClusterSize members whose page numbers
// run in page order. Page numbers must be mostly non-decreasing — sub-entries
// may repeat a page, and the occasional out-of-order entry (a misplaced
// section, an appendix) is tolerated. A price list or statistics table, whose
// numbers do not run in page order at all, still fails the allowance.
func qualifyingTOCColumn(boxes []pdf.TextBox, cluster []tocCandidate) bool {
	if len(cluster) < tocMinClusterSize {
		return false
	}
	sorted := append([]tocCandidate(nil), cluster...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].top < sorted[j].top })
	descents := 0
	for i := 1; i < len(sorted); i++ {
		if sorted[i].num < sorted[i-1].num {
			descents++
		}
	}
	return descents <= 1+len(sorted)/4
}

// dedicatedTOCPage reports whether the page is a pure TOC page and
// whole-page deletion is safe. The gate is content-nature, not a text-share
// ratio: a real TOC page (especially a column-split one) can have most of its
// text in title lines that carry no page number, so an entry-text ratio
// rejects it. The only thing that must be protected is prose, and every
// non-entry line of a pure TOC page is a short, isolated run — a cover
// title, watermark, page marker or TOC heading. Any continuous body
// paragraph fails the short-run check even when each of its lines is
// shorter than tocShortRunMaxRunes. A page that carries structured content
// (a table, figure or equation box) is never whole-page deleted: the
// structured box is kept and only the TOC entries are removed.
func dedicatedTOCPage(boxes []pdf.TextBox, pageIdx, eligible, entries []int, mh float64) bool {
	if pageHasStructuredContent(boxes, pageIdx) {
		return false
	}
	return allNonEntryRunsShort(boxes, eligible, entries, mh)
}

// pageHasStructuredContent reports whether any box on the page is a table,
// figure or equation. Structured boxes are excluded from the TOC entry
// analysis entirely, so their presence must also veto whole-page deletion —
// otherwise a DLA-detected table on the page would be dropped with the TOC.
func pageHasStructuredContent(boxes []pdf.TextBox, pageIdx []int) bool {
	for _, i := range pageIdx {
		switch boxes[i].LayoutType {
		case pdf.LayoutTypeTable, pdf.LayoutTypeFigure, pdf.LayoutTypeEquation:
			return true
		}
	}
	return false
}

// allNonEntryRunsShort splits the non-entry lines into vertical runs (lines
// in the same column with a gap below tocGapFactor×median height chain) and
// requires every CONTINUOUS run to stay under tocShortRunMaxRunes. A single
// isolated line is exempt from the length cap: a long one-liner (a URL
// watermark, a cover title) is not prose. Only a run of two or more chained
// lines — a body paragraph, whose every single line is short — must pass the
// cap, and any paragraph fails it even when each line is shorter than
// tocShortRunMaxRunes.
func allNonEntryRunsShort(boxes []pdf.TextBox, eligible, entries []int, mh float64) bool {
	entrySet := make(map[int]bool, len(entries))
	for _, i := range entries {
		entrySet[i] = true
	}
	nonEntry := make([]int, 0, len(eligible))
	for _, i := range eligible {
		if !entrySet[i] {
			nonEntry = append(nonEntry, i)
		}
	}
	sort.Slice(nonEntry, func(i, j int) bool {
		a, b := nonEntry[i], nonEntry[j]
		if boxes[a].PageNumber != boxes[b].PageNumber {
			return boxes[a].PageNumber < boxes[b].PageNumber
		}
		return boxes[a].Top < boxes[b].Top
	})

	// Run state is tracked per column: boxes are visited in global
	// top-to-bottom order, so the lines of two interleaved columns are
	// mixed in the iteration. A single global run would reset one column's
	// accumulation every time the other column's line sorts between two of
	// its lines, letting a real paragraph slip under the cap.
	runs := make(map[int]*tocRunState, len(nonEntry))
	for _, i := range nonEntry {
		col := boxes[i].ColID
		st := runs[col]
		if st == nil {
			st = &tocRunState{}
			runs[col] = st
		}
		runes := utf8.RuneCountInString(strings.TrimSpace(boxes[i].Text))
		if st.seen && boxes[i].Top-st.bottom < tocGapFactor*mh {
			st.runes += runes
			st.lines++
		} else {
			st.runes = runes
			st.lines = 1
		}
		st.bottom = boxes[i].Bottom
		st.seen = true
		if st.lines >= 2 && st.runes > tocShortRunMaxRunes {
			return false
		}
	}
	return true
}

// tocRunState tracks the vertical run of one column: consecutive lines
// whose gap stays below tocGapFactor×median height.
type tocRunState struct {
	runes  int
	lines  int
	bottom float64
	seen   bool
}

// tocWrappedTitlePartners returns the non-entry line sitting directly above
// a number-only page reference, when it is close enough and left-aligned —
// the wrapped-entry title line ("朴素贝叶斯分类" above "……39").
func tocWrappedTitlePartners(boxes []pdf.TextBox, eligible, entries []int, mh float64) []int {
	numberOnly := make(map[int]bool)
	for _, i := range entries {
		text := strings.TrimSpace(boxes[i].Text)
		m := tocEntryShapePattern.FindStringSubmatchIndex(text)
		if m == nil {
			continue
		}
		if strings.Trim(text[:m[2]], tocLeaderChars) == "" {
			numberOnly[i] = true
		}
	}
	entrySet := make(map[int]bool, len(entries))
	for _, i := range entries {
		entrySet[i] = true
	}
	var partners []int
	for ref := range numberOnly {
		best, bestBottom := -1, -1.0
		for _, i := range eligible {
			if entrySet[i] {
				continue
			}
			if boxes[i].Bottom > boxes[ref].Top {
				continue // not above
			}
			gap := boxes[ref].Top - boxes[i].Bottom
			if gap >= tocGapFactor*mh {
				continue
			}
			if absF(boxes[i].X0-boxes[ref].X0) > tocPartnerX0Tolerance {
				continue
			}
			if best < 0 || boxes[i].Bottom > bestBottom {
				best, bestBottom = i, boxes[i].Bottom
			}
		}
		if best >= 0 {
			partners = append(partners, best)
		}
	}
	return partners
}

// tocHeadingBox returns the standalone TOC heading line ("目录"/"contents"…)
// sitting within tocHeadingGapFactor×median height above the column's
// topmost entry, if any.
func tocHeadingBox(boxes []pdf.TextBox, eligible, entries []int, mh float64) int {
	topEntry := 0
	for _, i := range entries {
		if topEntry == 0 || boxes[i].Top < boxes[topEntry].Top {
			topEntry = i
		}
	}
	for _, i := range eligible {
		if !doctype.IsTOCTitleHeading(boxes[i].Text) {
			continue
		}
		if boxes[i].Bottom > boxes[topEntry].Top {
			continue
		}
		if boxes[topEntry].Top-boxes[i].Bottom <= tocHeadingGapFactor*mh {
			return i
		}
	}
	return -1
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
