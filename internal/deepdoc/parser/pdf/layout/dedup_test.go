package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestDedupIdenticalText locks that boxes carrying byte-identical text on the
// SAME page with DISJOINT Y bands are collapsed to one. OCR repeatedly detects
// the same text at different Y positions (09_crosspage_paragraph detects each
// paragraph 5-6x per page, all disjoint), and Python's downstream merge
// collapses these; Go must too, or the replay output duplicates paragraphs.
func TestDedupIdenticalText(t *testing.T) {
	long := "paragraph one with enough words to qualify as a real paragraph duplicate text for collapsing"
	boxes := []pdf.TextBox{
		{Text: long, PageNumber: 0, Top: 100, Bottom: 115, X0: 60, X1: 520},
		{Text: long, PageNumber: 0, Top: 300, Bottom: 315, X0: 60, X1: 520}, // disjoint dup -> drop
		{Text: long, PageNumber: 1, Top: 100, Bottom: 115, X0: 60, X1: 520}, // other page -> keep
		{Text: "paragraph two", PageNumber: 0, Top: 200, Bottom: 215},
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 3 {
		t.Fatalf("want 3 boxes (1 disjoint dup dropped), got %d: %+v", len(got), got)
	}
	if got[0].Text != long || got[1].Text != long {
		t.Fatalf("page-0 and page-1 copies must both be kept in order")
	}
	if got[2].Text != "paragraph two" {
		t.Fatalf("want 'paragraph two' third, got %q", got[2].Text)
	}
}

// TestDedupIdenticalText_YOverlap ensures overlapping boxes are kept: two
// columns / adjacent lines on the same page legitimately share text (e.g.
// eval_three_wide has 3 columns at the same Y), so only disjoint duplicates
// are collapsed.
func TestDedupIdenticalText_YOverlap(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "line xxxxxxxxxxxxx", PageNumber: 0, Top: 104, Bottom: 116, X0: 60, X1: 260},
		{Text: "line xxxxxxxxxxxxx", PageNumber: 0, Top: 104, Bottom: 116, X0: 320, X1: 520}, // 2nd column -> keep
		{Text: "line xxxxxxxxxxxxx", PageNumber: 0, Top: 118, Bottom: 130, X0: 60, X1: 260},  // overlapping neighbor -> keep
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 3 {
		t.Fatalf("overlapping same-text boxes must all be kept, got %d", len(got))
	}
}

// TestDedupIdenticalText_WhitespaceSensitive ensures trimming does not merge
// boxes that differ only by trailing spaces into a false duplicate.
func TestDedupIdenticalText_WhitespaceSensitive(t *testing.T) {
	long := "a sufficiently long repeated sentence that qualifies as a paragraph"
	boxes := []pdf.TextBox{
		{Text: long, PageNumber: 0, Top: 10, Bottom: 20, X0: 60, X1: 520},
		{Text: long + " ", PageNumber: 0, Top: 90, Bottom: 100, X0: 60, X1: 520}, // 80pt gap > 4x height
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 1 {
		t.Fatalf("trimmed-equal texts should be treated as duplicates, got %d", len(got))
	}
}

// TestDedupIdenticalText_ShortTextKept locks that SHORT identical texts
// (e.g. the repeated keyword 'Transformer' in 16_dense_cjk) are NOT collapsed —
// short repeated content is real document text, not an OCR paragraph duplicate.
func TestDedupIdenticalText_ShortTextKept(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "Transformer", PageNumber: 0, Top: 100, Bottom: 112, X0: 60, X1: 120},
		{Text: "Transformer", PageNumber: 0, Top: 300, Bottom: 312, X0: 60, X1: 120}, // disjoint Y, short -> keep
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 2 {
		t.Fatalf("short identical text must be kept, got %d boxes", len(got))
	}
}

// TestDedupIdenticalText_AdjacentRepeatsKept locks that identical lines only
// ~1x their height apart (adjacent rows) are NOT collapsed — they are real
// document content (eval_two_narrow_gutter has 'linexxx' rows 44pt apart),
// unlike OCR pseudo-duplicates detected with a large rolling stride (89-136pt).
func TestDedupIdenticalText_AdjacentRepeatsKept(t *testing.T) {
	row := "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxX"
	boxes := []pdf.TextBox{
		{Text: row, PageNumber: 0, Top: 106, Bottom: 121, X0: 60, X1: 260},
		{Text: row, PageNumber: 0, Top: 150, Bottom: 165, X0: 60, X1: 260}, // 44pt gap (< 4x height) -> keep
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 2 {
		t.Fatalf("adjacent identical rows must be kept, got %d boxes", len(got))
	}
}

// TestDedupIdenticalText_StridedPseudoDuplicate collapses identical text far
// apart (>4x height) at the same X — the OCR rolling-stride duplicate
// (eval_single_wide / 09_crosspage_paragraph).
func TestDedupIdenticalText_StridedPseudoDuplicate(t *testing.T) {
	long := "a sufficiently long repeated sentence that qualifies as a paragraph duplicate"
	boxes := []pdf.TextBox{
		{Text: long, PageNumber: 0, Top: 100, Bottom: 115, X0: 60, X1: 520},
		{Text: long, PageNumber: 0, Top: 300, Bottom: 315, X0: 60, X1: 520}, // 200pt gap -> drop
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 1 {
		t.Fatalf("strided pseudo-duplicate must be dropped, got %d boxes", len(got))
	}
}

// TestDedupSubstringOverlaps locks that a box whose text is a CONTIGUOUS
// SUBSTRING of another same-page box, and whose Y band overlaps it, is
// collapsed — OCR detects both a full paragraph and its middle fragment (e.g.
// 01_english_simple box1 y=(105,166) full paragraph + box2 y=(119,132)
// "language models. When a user asks..."), and Python drops the fragment.
func TestDedupSubstringOverlaps(t *testing.T) {
	full := pdf.TextBox{
		Text:       "Retrieval-Augmented Generation (RAG) is a technique that combines information retrieval with large language models. When a user asks a question",
		PageNumber: 0, Top: 100, Bottom: 160, X0: 60, X1: 520,
	}
	frag := pdf.TextBox{
		Text:       "language models. When a user asks a question",
		PageNumber: 0, Top: 115, Bottom: 130, X0: 60, X1: 520, // overlaps the full box
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{full, frag})
	if len(got) != 1 {
		t.Fatalf("overlapping substring fragment must be dropped, got %d boxes", len(got))
	}
	if got[0].Text != full.Text {
		t.Fatalf("the full paragraph must be kept, got %q", got[0].Text)
	}
}

// TestDedupSubstringOverlaps_DisjointYKept ensures a substring box at a
// DISJOINT Y position is kept — a real repeated heading or sentence is legal.
func TestDedupSubstringOverlaps_DisjointYKept(t *testing.T) {
	full := pdf.TextBox{
		Text:       "Conclusion summary of the whole document body text",
		PageNumber: 0, Top: 100, Bottom: 115,
	}
	repeat := pdf.TextBox{
		Text:       "Conclusion summary",
		PageNumber: 0, Top: 300, Bottom: 315, // disjoint Y
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{full, repeat})
	if len(got) != 2 {
		t.Fatalf("disjoint-Y substring must be kept, got %d boxes", len(got))
	}
}

// TestDedupSubstringOverlaps_DifferentColumnKept ensures a substring-like text
// in a DIFFERENT column (disjoint X) is kept — two columns can carry similar
// 'linexxx' fragments at the same Y (eval_two_wide_gutter). Only fragments at
// the same X location (true OCR duplicates) are collapsed.
func TestDedupSubstringOverlaps_DifferentColumnKept(t *testing.T) {
	colA := pdf.TextBox{
		Text:       "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		PageNumber: 0, Top: 100, Bottom: 115, X0: 60, X1: 260,
	}
	colB := pdf.TextBox{
		Text:       "line xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		PageNumber: 0, Top: 100, Bottom: 115, X0: 320, X1: 520, // disjoint X
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{colA, colB})
	if len(got) != 2 {
		t.Fatalf("same-Y different-column substring must be kept, got %d boxes", len(got))
	}
}

// TestDedupSubstringOverlaps_AdjacentLinesKept ensures two 'linexxx' boxes on
// ADJACENT lines (Y only touches at the boundary, overlap << 80%) are kept —
// they are distinct rows, not an OCR fragment of one another.
func TestDedupSubstringOverlaps_AdjacentLinesKept(t *testing.T) {
	a := pdf.TextBox{
		Text:       "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		PageNumber: 0, Top: 100, Bottom: 112, X0: 60, X1: 260,
	}
	b := pdf.TextBox{
		Text:       "line xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		PageNumber: 0, Top: 113, Bottom: 125, X0: 60, X1: 260, // 1pt overlap, 0.1 < 0.8
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{a, b})
	if len(got) != 2 {
		t.Fatalf("adjacent-line substring must be kept, got %d boxes", len(got))
	}
}
