package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestVerticalMergeEnglishNoMerge locks that an ENGLISH page with median
// height 0 does NOT vertically merge adjacent lines. Python clears chars for
// is_english documents so mean_height becomes 0 and _naive_vertical_merge
// skips every pair (gap > 0); Go must mirror that, otherwise 'linexxx' rows
// (eval_two_wide_gutter) concatenate into one giant line.
func TestVerticalMergeEnglishNoMerge(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 100, Bottom: 112, X0: 60, X1: 260},
		{Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 113, Bottom: 125, X0: 60, X1: 260},
		{Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 126, Bottom: 138, X0: 60, X1: 260},
	}
	// English page, median height explicitly 0 -> no vertical merge.
	got := NaiveVerticalMerge(boxes, map[int]float64{0: 0}, map[int]float64{0: 8}, map[int]bool{0: true})
	if len(got) != 3 {
		t.Fatalf("english page with mh=0 must keep 3 separate lines, got %d", len(got))
	}
}

// TestVerticalMergeEnglishNoMergePositiveMedian locks the REAL production path:
// an ENGLISH page whose char-derived median height is POSITIVE (the common case
// for digital English PDFs, where Go keeps embedded chars) must STILL NOT
// vertically merge. The old guard `if mh <= 0 { if pageEnglish ... }` never
// fired for real pages (mh > 0), so English lines were wrongly merged and
// 'linexxx' rows concatenated into one giant line. This reproduces that gap and
// is the regression lock for the fix (mirror Python is_english -> chars=[] ->
// mean_height 0 -> no merge).
func TestVerticalMergeEnglishNoMergePositiveMedian(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 100, Bottom: 112, X0: 60, X1: 260},
		{Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 113, Bottom: 125, X0: 60, X1: 260},
		{Text: "line xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx", PageNumber: 0, Top: 126, Bottom: 138, X0: 60, X1: 260},
	}
	// English page, POSITIVE median height (real char-path case) -> no merge.
	got := NaiveVerticalMerge(boxes, map[int]float64{0: 15}, map[int]float64{0: 8}, map[int]bool{0: true})
	if len(got) != 3 {
		t.Fatalf("english page with positive median height must keep 3 separate lines, got %d", len(got))
	}
}

// TestVerticalMergeNonEnglishKeepsMerge ensures non-English pages still merge
// close adjacent lines (median height > 0), so paragraph assembly is intact.
func TestVerticalMergeNonEnglishKeepsMerge(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "first paragraph line one", PageNumber: 0, Top: 100, Bottom: 115, X0: 60, X1: 520},
		{Text: "second line of the same paragraph", PageNumber: 0, Top: 116, Bottom: 131, X0: 60, X1: 520},
	}
	got := NaiveVerticalMerge(boxes, map[int]float64{0: 15}, map[int]float64{0: 8}, map[int]bool{0: false})
	if len(got) != 1 {
		t.Fatalf("non-english page should merge close lines, got %d boxes", len(got))
	}
}
