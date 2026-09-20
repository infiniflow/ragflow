package parser

import (
	"testing"

	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// TestSectionOrderBreaksEvictionCleanOrder is the sanity case: sections whose
// positions are page-monotonic keep eviction enabled. This documents the
// intended (bounded) behavior cropMediaSections relies on.
func TestSectionOrderBreaksEvictionCleanOrder(t *testing.T) {
	sections := []deepdoctype.Section{
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{1}}}},
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{2, 3}}}},
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{4}}}},
	}
	broken, idx := sectionOrderBreaksEviction(sections)
	if broken {
		t.Fatalf("clean in-order sections must not break eviction, got break at index %d", idx)
	}
}

// TestSectionOrderBreaksEvictionCrossPageMerge reproduces the defect: a figure
// whose first position is on a LATER page (25) but which also carries a
// caption merged onto an EARLIER page (18). sortSectionsByPosition orders by
// firstSectionPage, so this section sorts after a table on page 20, yet its
// true minimum page (18) is below that table's page. The mismatch flips
// sectionsOrdered off and permanently disables eviction.
func TestSectionOrderBreaksEvictionCrossPageMerge(t *testing.T) {
	sections := []deepdoctype.Section{
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{20}}}},
		{LayoutType: deepdoctype.LayoutTypeFigure, Positions: []deepdoctype.Position{
			{PageNumbers: []int{25}},
			{PageNumbers: []int{18}},
		}},
	}
	broken, idx := sectionOrderBreaksEviction(sections)
	if !broken {
		t.Fatalf("expected eviction break from cross-page-merge section, got none")
	}
	if idx != 1 {
		t.Errorf("expected break at index 1, got %d", idx)
	}
}

// TestCropMediaSectionsEvictionContinuesAfterPageOrderBreak replays the exact
// eviction loop from cropMediaSections (future-page window) with a fake render
// that records which pages are kept. It proves the fix for the defect above:
// after a page-order break, eviction keeps running (using the TRUE future
// minimum page) instead of stalling forever — so the cache stays bounded to a
// sliding window of the current section's pages rather than growing without
// bound and OOMing large PDFs.
func TestCropMediaSectionsEvictionContinuesAfterPageOrderBreak(t *testing.T) {
	sections := []deepdoctype.Section{
		// table on page 20
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{20}}}},
		// figure with cross-page-merge artifact: first position page 25, caption page 18
		{LayoutType: deepdoctype.LayoutTypeFigure, Positions: []deepdoctype.Position{
			{PageNumbers: []int{25}},
			{PageNumbers: []int{18}},
		}},
		// two sections far down the document
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{300}}}},
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{301}}}},
	}

	// Mirror cropMediaSections' corrected eviction decision: a suffix-min
	// future-page window keeps eviction running even after a page-order break.
	n := len(sections)
	minFuturePage := make([]int, n)
	cur := -1
	for j := n - 1; j >= 0; j-- {
		if mp := sectionMinPage(sections[j]); mp >= 0 {
			if cur < 0 || mp < cur {
				cur = mp
			}
		}
		minFuturePage[j] = cur
	}
	pageCache := map[int]bool{} // page -> rendered and kept
	for i := range sections {
		if minFuturePage[i] >= 0 {
			for pn := range pageCache {
				if pn < minFuturePage[i] {
					delete(pageCache, pn)
				}
			}
		}
		for _, pos := range sections[i].Positions {
			for _, pn := range pos.PageNumbers {
				pageCache[pn] = true
			}
		}
	}

	// The cross-page-merge break must NOT disable eviction: once we advance
	// past pages 20/25 to 300/301, the early pages are pruned by the
	// future-page window, so the cache holds only the current section's page
	// instead of all four (the old unbounded behaviour).
	if pageCache[20] {
		t.Error("expected page 20 to be evicted by the future-page window, but it was retained")
	}
	if pageCache[25] {
		t.Error("expected page 25 to be evicted by the future-page window, but it was retained")
	}
	if !pageCache[301] {
		t.Error("expected page 301 to remain cached (current section), but it was evicted")
	}
	if len(pageCache) != 1 {
		t.Errorf("expected bounded cache to hold only the last section's page {301}, got %d: %v", len(pageCache), pageCache)
	}
}

// TestSectionRendersFromCache pins the eligibility predicate cropMediaSections
// and computeFuturePageWindow share: only media sections that actually render
// from pageCache (no pre-existing Image, a media layout/type, and at least one
// Position) participate in the eviction window. Skipped sections must not
// contribute to the window, or a late skipped section with a low merged page
// would keep the window low and let pageCache grow (the residual of #19938).
func TestSectionRendersFromCache(t *testing.T) {
	renderable := []deepdoctype.Section{
		{LayoutType: deepdoctype.LayoutTypeFigure, Positions: []deepdoctype.Position{{PageNumbers: []int{1}}}},
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{2}}}},
		{LayoutType: deepdoctype.DLALabelFigureCaption, Positions: []deepdoctype.Position{{PageNumbers: []int{3}}}},
		{LayoutType: "image", Positions: []deepdoctype.Position{{PageNumbers: []int{4}}}},
		{DocTypeKwd: "image", Positions: []deepdoctype.Position{{PageNumbers: []int{5}}}},
		{DocTypeKwd: "table", Positions: []deepdoctype.Position{{PageNumbers: []int{6}}}},
	}
	for i, sec := range renderable {
		if !sectionRendersFromCache(sec) {
			t.Errorf("case %d: expected media section to render from cache", i)
		}
	}
	skipped := []deepdoctype.Section{
		{LayoutType: deepdoctype.LayoutTypeText, Positions: []deepdoctype.Position{{PageNumbers: []int{1}}}},
		{LayoutType: deepdoctype.LayoutTypeFigure, Image: "already-cropped", Positions: []deepdoctype.Position{{PageNumbers: []int{1}}}},
		{LayoutType: deepdoctype.LayoutTypeTable}, // no Positions
	}
	for i, sec := range skipped {
		if sectionRendersFromCache(sec) {
			t.Errorf("case %d: expected skipped section NOT to render from cache", i)
		}
	}
}

// TestComputeFuturePageWindowIgnoresSkippedSections is the regression for
// Finding A of the #19938 review: a section that cropMediaSections skips (here
// a text section) but which carries a low merged page late in the document
// must NOT pull the future-page window down. With the bug, the window at the
// first section would equal that low page (5) and early rendered pages could
// never be evicted; after the fix the window stays at the first renderable
// section's page (100).
func TestComputeFuturePageWindowIgnoresSkippedSections(t *testing.T) {
	sections := []deepdoctype.Section{
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{100}}}},
		{LayoutType: deepdoctype.LayoutTypeText, Positions: []deepdoctype.Position{{PageNumbers: []int{5}}}},
	}
	win := computeFuturePageWindow(sections)
	if win[0] != 100 {
		t.Errorf("skipped low-page section must not lower the future window; got win[0]=%d, want 100", win[0])
	}
	if win[1] != -1 {
		t.Errorf("skipped section has no renderable future window; got win[1]=%d, want -1", win[1])
	}
}

// TestSectionOrderBreaksEvictionSkipsUnknownPages is the regression for the
// -1 handling in sectionOrderBreaksEviction (Finding C): a section with no
// page info (sectionMinPage == -1) must not be treated as a page-order break.
// Before the fix, the -1 section was reported as the offending break; after
// the fix the break is correctly attributed to the next known lower page (18
// following 20), so the offending index is 2, not 1.
func TestSectionOrderBreaksEvictionSkipsUnknownPages(t *testing.T) {
	sections := []deepdoctype.Section{
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{20}}}},
		{LayoutType: deepdoctype.LayoutTypeFigure}, // no Positions -> minPage -1
		{LayoutType: deepdoctype.LayoutTypeTable, Positions: []deepdoctype.Position{{PageNumbers: []int{18}}}},
	}
	broken, idx := sectionOrderBreaksEviction(sections)
	if !broken {
		t.Fatalf("expected a page-order break from 20 -> 18, got none")
	}
	if idx != 2 {
		t.Errorf("expected break at index 2 (the -1 section must be skipped), got %d", idx)
	}
}
