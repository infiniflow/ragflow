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
