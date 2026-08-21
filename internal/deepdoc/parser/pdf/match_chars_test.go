//go:build cgo

package pdf

import (
	"testing"

	pdftype "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMatchCharsToBoxes_AreaTieBreak guards the fix for ocr_real RAG分词
// text doubling. When an OCR detector over-segments a line into a full-line
// container box AND a smaller fragment box fully contained in it, a glyph
// that is fully inside both boxes yields an equal overlap ratio (1.0) for
// each. The greedy assignment must prefer the LARGER (container) box on a
// tie, so the fragment cannot steal the glyph and truncate the container.
//
// The previous `>=`-with-last-wins rule let the smaller fragment win, which
// truncated the container (dropped the trailing 么), after which
// DedupSubstringOverlaps could no longer recognise the fragment as a
// substring and NaiveVerticalMerge glued it back on — duplicating the text.
func TestMatchCharsToBoxes_AreaTieBreak(t *testing.T) {
	// Container: full-line box (large area).
	container := ocrDetectBox{
		box: pdftype.TextBox{X0: 10, X1: 500, Top: 560, Bottom: 589},
		x0:  10, y0: 560, x1: 500, y1: 589,
	}
	// Fragment: smaller box fully contained in the container.
	fragment := ocrDetectBox{
		box: pdftype.TextBox{X0: 10, X1: 500, Top: 577, Bottom: 589},
		x0:  10, y0: 577, x1: 500, y1: 589,
	}
	boxes := []ocrDetectBox{container, fragment}

	// Chars: one belongs only to the container (above the fragment's top),
	// and one (么) is fully inside BOTH boxes -> overlap ratio 1.0 for each.
	// Glyph heights (~12pt) are realistic: the container box is ~29pt tall, so
	// the existing char-height filter (>=0.7 height mismatch) does NOT drop the
	// glyph — only the area tie-break decides which box wins.
	chars := []pdftype.TextChar{
		{Text: "在", X0: 20, X1: 32, Top: 565, Bottom: 575},   // container only
		{Text: "么", X0: 100, X1: 112, Top: 577, Bottom: 589}, // inside both
	}

	got := matchCharsToBoxes(boxes, chars)

	if len(got) != 2 {
		t.Fatalf("expected 2 box char groups, got %d", len(got))
	}

	// The trailing glyph must land in the container (box 0), never the
	// fragment (box 1).
	var inContainer, inFragment bool
	for _, c := range got[0] {
		if c.Text == "么" {
			inContainer = true
		}
	}
	for _, c := range got[1] {
		if c.Text == "么" {
			inFragment = true
		}
	}

	if !inContainer {
		t.Errorf("trailing glyph 么 was NOT assigned to the container box; doubling regression risk")
	}
	if inFragment {
		t.Errorf("trailing glyph 么 was assigned to the smaller fragment box; this is exactly the over-segmentation tie-break bug")
	}

	// The container-only char stays in the container.
	var containerOnly bool
	for _, c := range got[0] {
		if c.Text == "在" {
			containerOnly = true
		}
	}
	if !containerOnly {
		t.Errorf("container-only glyph 在 was not assigned to the container box")
	}
}
