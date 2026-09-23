package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestBoxOverlapsPositionPage locks the page-aware behavior of the table/box
// overlap check. Table positions and table-layout boxes are both stored in
// page-local coordinates (Y resets near 0 at the top of every page), so a
// position's Y band is shared by the boxes of every page. The check must
// therefore reject a box that overlaps only in X/Y but lives on a different
// page; otherwise a single page-local position wrongly matches the same Y band
// on all pages (the source of the multi-GB replacement cross-product and of
// cross-page content mis-attribution).
func TestBoxOverlapsPositionPage(t *testing.T) {
	// A position on page 5 with a normal X/Y band.
	pos := pdf.Position{PageNumbers: []int{5}, Left: 10, Right: 100, Top: 10, Bottom: 50}

	// Same page + X/Y overlap -> true.
	samePage := pdf.TextBox{PageNumber: 5, X0: 10, X1: 100, Top: 10, Bottom: 50}
	if !boxOverlapsPositionPage(samePage, pos) {
		t.Errorf("same-page X/Y overlap should be true")
	}

	// Different page but identical X/Y band -> false. This is the case the old
	// (page-blind) check got wrong: it would return true and inflate reps.
	crossPage := pdf.TextBox{PageNumber: 6, X0: 10, X1: 100, Top: 10, Bottom: 50}
	if boxOverlapsPositionPage(crossPage, pos) {
		t.Errorf("cross-page X/Y overlap must be false: page-local Y must not match across pages")
	}

	// Disjoint X/Y on a different page -> false (sanity, would be false either way).
	disjoint := pdf.TextBox{PageNumber: 6, X0: 500, X1: 600, Top: 500, Bottom: 600}
	if boxOverlapsPositionPage(disjoint, pos) {
		t.Errorf("disjoint box should be false regardless of page")
	}

	// Missing page info on the position -> fall back to X/Y-only (overlap true).
	noPagePos := pdf.Position{PageNumbers: nil, Left: 10, Right: 100, Top: 10, Bottom: 50}
	if !boxOverlapsPositionPage(samePage, noPagePos) {
		t.Errorf("fallback to X/Y-only expected when position has no page info")
	}
	emptyPagePos := pdf.Position{PageNumbers: []int{}, Left: 10, Right: 100, Top: 10, Bottom: 50}
	if !boxOverlapsPositionPage(samePage, emptyPagePos) {
		t.Errorf("fallback to X/Y-only expected when position PageNumbers is empty")
	}

	// Missing page info on the box (PageNumber == 0) -> fall back to X/Y-only.
	noBoxPage := pdf.TextBox{PageNumber: 0, X0: 10, X1: 100, Top: 10, Bottom: 50}
	if !boxOverlapsPositionPage(noBoxPage, pos) {
		t.Errorf("fallback to X/Y-only expected when box has no page number")
	}
}

// TestBoxOverlapsPositionPage_MergedTable locks that a table merged across
// consecutive pages (MergeTablesAcrossPages appends every spanned page into
// Position.PageNumbers, see table_merge.go) still claims the per-page
// table-layout box that sits on one of its continuation pages. The page check
// must accept the box when its single page is a member of the merged
// position's multi-page PageNumbers set — not reject it just because the
// position is no longer single-page. This is the cross-page-merge counterpart
// of TestBoxOverlapsPositionPage: it guards against the page constraint
// accidentally breaking legitimate cross-page merges while still rejecting
// boxes on pages the merged table does not occupy.
func TestBoxOverlapsPositionPage_MergedTable(t *testing.T) {
	// A table merged across pages 4 and 5. Both positions share the same
	// page-local X/Y band (Y resets near 0 on every page), so the only thing
	// distinguishing them is the page set.
	merged := pdf.Position{PageNumbers: []int{4, 5}, Left: 10, Right: 400, Top: 60, Bottom: 200}

	// Box on the anchor page (4) -> must match.
	anchorBox := pdf.TextBox{PageNumber: 4, X0: 10, X1: 400, Top: 60, Bottom: 200}
	if !boxOverlapsPositionPage(anchorBox, merged) {
		t.Errorf("anchor-page box of a merged table should match")
	}

	// Box on the continuation page (5) -> must still match. This is the case
	// that proves the page constraint uses membership, not equality with a
	// single page, so cross-page merges keep their boxes.
	continuationBox := pdf.TextBox{PageNumber: 5, X0: 10, X1: 400, Top: 60, Bottom: 200}
	if !boxOverlapsPositionPage(continuationBox, merged) {
		t.Errorf("continuation-page box of a merged table should match")
	}

	// Box on a page the merged table does NOT span (6) -> must be rejected,
	// even though the page-local X/Y band is identical. Without membership
	// semantics this would wrongly inflate reps on large documents.
	outsideBox := pdf.TextBox{PageNumber: 6, X0: 10, X1: 400, Top: 60, Bottom: 200}
	if boxOverlapsPositionPage(outsideBox, merged) {
		t.Errorf("box on a non-spanned page must be rejected even for a merged table")
	}
}
