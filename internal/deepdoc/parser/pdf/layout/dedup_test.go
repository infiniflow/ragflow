package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestDedupIdenticalText locks that boxes carrying TRIM-EQUAL text (identical
// after strings.TrimSpace — trailing whitespace normalized) on the SAME page
// with DISJOINT Y bands are collapsed to one. OCR repeatedly detects the same
// text at different Y positions (09_crosspage_paragraph detects each paragraph
// 5-6x per page, all disjoint), and Python's downstream merge collapses these;
// Go must too, or the replay output duplicates paragraphs.
func TestDedupIdenticalText(t *testing.T) {
	long := "paragraph one with enough words to qualify as a real paragraph duplicate text for collapsing"
	boxes := []pdf.TextBox{
		{Text: long, PageNumber: 0, Top: 100, Bottom: 115, X0: 60, X1: 520, IsOCR: true},
		{Text: long, PageNumber: 0, Top: 300, Bottom: 315, X0: 60, X1: 520, IsOCR: true}, // disjoint dup -> drop
		{Text: long, PageNumber: 1, Top: 100, Bottom: 115, X0: 60, X1: 520, IsOCR: true}, // other page -> keep
		{Text: "paragraph two", PageNumber: 0, Top: 200, Bottom: 215, IsOCR: true},
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
		{Text: "line xxxxxxxxxxxxx", PageNumber: 0, Top: 104, Bottom: 116, X0: 60, X1: 260, IsOCR: true},
		{Text: "line xxxxxxxxxxxxx", PageNumber: 0, Top: 104, Bottom: 116, X0: 320, X1: 520, IsOCR: true}, // 2nd column -> keep
		{Text: "line xxxxxxxxxxxxx", PageNumber: 0, Top: 118, Bottom: 130, X0: 60, X1: 260, IsOCR: true},  // overlapping neighbor -> keep
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
		{Text: long, PageNumber: 0, Top: 10, Bottom: 20, X0: 60, X1: 520, IsOCR: true},
		{Text: long + " ", PageNumber: 0, Top: 90, Bottom: 100, X0: 60, X1: 520, IsOCR: true}, // 80pt gap > 4x height
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
		{Text: "Transformer", PageNumber: 0, Top: 100, Bottom: 112, X0: 60, X1: 120, IsOCR: true},
		{Text: "Transformer", PageNumber: 0, Top: 300, Bottom: 312, X0: 60, X1: 120, IsOCR: true}, // disjoint Y, short -> keep
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
		{Text: row, PageNumber: 0, Top: 106, Bottom: 121, X0: 60, X1: 260, IsOCR: true},
		{Text: row, PageNumber: 0, Top: 150, Bottom: 165, X0: 60, X1: 260, IsOCR: true}, // 44pt gap (< 4x height) -> keep
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
		{Text: long, PageNumber: 0, Top: 100, Bottom: 115, X0: 60, X1: 520, IsOCR: true},
		{Text: long, PageNumber: 0, Top: 300, Bottom: 315, X0: 60, X1: 520, IsOCR: true}, // 200pt gap -> drop
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 1 {
		t.Fatalf("strided pseudo-duplicate must be dropped, got %d boxes", len(got))
	}
}

// TestDedupIdenticalText_CharPathKept locks the key invariant: char-path
// digital-PDF boxes (IsOCR=false) are NEVER de-duplicated, even when they are
// byte-identical, far apart, and in the same column. Dropping them would
// silently lose legitimate repeated content (repeated clauses / headings) that
// Python's char path keeps — a regression the IsOCR scoping must prevent.
// With the scoping removed this test fails (gets 1 instead of 2).
func TestDedupIdenticalText_CharPathKept(t *testing.T) {
	clause := "保密条款：双方应对在合作中知悉的商业秘密承担保密义务直至保密期限届满"
	boxes := []pdf.TextBox{
		{Text: clause, PageNumber: 0, Top: 100, Bottom: 120, X0: 60, X1: 520},   // char path
		{Text: clause, PageNumber: 0, Top: 1000, Bottom: 1020, X0: 60, X1: 520}, // far-apart repeat, same column
	}
	got := DedupIdenticalText(boxes)
	if len(got) != 2 {
		t.Fatalf("char-path identical repeats must be kept, got %d boxes (lost content?)", len(got))
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
		PageNumber: 0, Top: 100, Bottom: 160, X0: 60, X1: 520, IsOCR: true,
	}
	frag := pdf.TextBox{
		Text:       "language models. When a user asks a question",
		PageNumber: 0, Top: 115, Bottom: 130, X0: 60, X1: 520, IsOCR: true, // overlaps the full box
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
		Text: "Conclusion summary of the whole document body text", PageNumber: 0, Top: 100, Bottom: 115, IsOCR: true,
	}
	repeat := pdf.TextBox{
		Text: "Conclusion summary", PageNumber: 0, Top: 300, Bottom: 315, IsOCR: true, // disjoint Y
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
		Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 100, Bottom: 115, X0: 60, X1: 260, IsOCR: true,
	}
	colB := pdf.TextBox{
		Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 100, Bottom: 115, X0: 320, X1: 520, IsOCR: true, // disjoint X
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
		Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 100, Bottom: 112, X0: 60, X1: 260, IsOCR: true,
	}
	b := pdf.TextBox{
		Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 113, Bottom: 125, X0: 60, X1: 260, IsOCR: true, // 1pt overlap, 0.1 < 0.8
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{a, b})
	if len(got) != 2 {
		t.Fatalf("adjacent-line substring must be kept, got %d boxes", len(got))
	}
}

// TestDedupSubstringOverlaps_PartialYOverlapKept locks the Y-containment
// boundary: a substring box that PARTIALLY overlaps the containing box in Y
// (extends below it) is KEPT — it is an adjacent-line fragment, not a contained
// duplicate. Only a fragment FULLY inside the containing box (boxInside) is
// collapsed. With the height-vs-text decoupled guard this case is also kept;
// the test pins the boundary against future over-collapsing.
func TestDedupSubstringOverlaps_PartialYOverlapKept(t *testing.T) {
	full := pdf.TextBox{
		Text: "the quick brown fox jumps over the lazy dog near the river bank", PageNumber: 0, Top: 100, Bottom: 130, X0: 60, X1: 520, IsOCR: true,
	}
	frag := pdf.TextBox{
		Text: "near the river", PageNumber: 0, Top: 120, Bottom: 150, X0: 60, X1: 520, IsOCR: true, // extends below full -> not inside
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{full, frag})
	if len(got) != 2 {
		t.Fatalf("partial-Y-overlap substring must be kept (not fully inside), got %d boxes", len(got))
	}
}

// TestDedupSubstringOverlaps_TallerFragmentKept locks the defensive invariant:
// a PHYSICALLY TALLER box whose SHORT text is a substring of a shorter, contained
// box's longer text is NOT silently dropped. Collapse requires the substring-text
// box to be geometrically INSIDE the text-containing box; here the substring box
// is the taller CONTAINER, so it is kept. This guards the height-vs-text decoupling
// fix (a taller box must not be dropped just because its text is a substring).
func TestDedupSubstringOverlaps_TallerFragmentKept(t *testing.T) {
	tall := pdf.TextBox{
		Text: "X", PageNumber: 0, Top: 100, Bottom: 160, X0: 60, X1: 520, IsOCR: true, // taller container
	}
	wide := pdf.TextBox{
		Text: "prefix X suffix", PageNumber: 0, Top: 115, Bottom: 130, X0: 60, X1: 520, IsOCR: true, // shorter, inside tall, contains "X"
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{tall, wide})
	if len(got) != 2 {
		t.Fatalf("taller substring box must be kept (only contained fragments are dropped), got %d boxes", len(got))
	}
}

// TestDedupSubstringOverlaps_CharPathKept locks that a char-path box whose text
// is a substring of another char-path box is NEVER collapsed — e.g. a repeated
// heading inside another paragraph's text range on a digital PDF. Only OCR
// pseudo-fragments (IsOCR=true) are collapsed.
func TestDedupSubstringOverlaps_CharPathKept(t *testing.T) {
	full := pdf.TextBox{
		Text: "本协议终止后保密条款继续有效双方仍应承担保密义务", PageNumber: 0, Top: 100, Bottom: 120,
	}
	repeat := pdf.TextBox{
		Text: "保密条款继续有效", PageNumber: 0, Top: 110, Bottom: 118, // nested substring, char path
	}
	got := DedupSubstringOverlaps([]pdf.TextBox{full, repeat})
	if len(got) != 2 {
		t.Fatalf("char-path substring must be kept, got %d boxes (lost content?)", len(got))
	}
}
