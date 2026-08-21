//go:build cgo

package pdf

import (
	"strings"
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

// TestMatchCharsToBoxes_FullyContainedSmallChar guards the fix for the
// ocr_real plugin-daemon "certifi missing" divergence. A small inline code
// span (e.g. "certifi", ~8pt ink height) sits fully inside a tall detect box
// (~36pt, spanning two text lines). The char-height filter
// (parser_ocr.go:263, mirroring pdf_parser.py:798) drops it because
// |8-36|/36 >= 0.7 — but the char is geometrically fully inside the box, so
// it cannot belong to an adjacent line and must be kept. The Python golden
// keeps such glyphs (plugin-daemon box[16] carries "certifi"), so Go must
// too.
//
// Fix: a fully-contained small glyph (overlap ≈ 1.0, ratio < 0.9) is deferred
// and re-kept when the box ALSO carries a non-space normal-height char — the
// box is a real text line with an inline span. Cross-line chars only partially
// overlap a tall box (ratio << 1.0), so they are still filtered — no bleed.
func TestMatchCharsToBoxes_FullyContainedSmallChar(t *testing.T) {
	// Tall detect box (height ~35.7), like plugin-daemon box[16].
	tallBox := ocrDetectBox{
		box: pdftype.TextBox{X0: 70, X1: 763, Top: 492.6, Bottom: 528.3},
		x0:  70, y0: 492.6, x1: 763, y1: 528.3,
	}
	boxes := []ocrDetectBox{tallBox}

	// `在`: body text, ~12pt, fully inside the tall box. Passes the filter
	// regardless (|12-35.7|/35.7 = 0.66 < 0.7) — control case.
	// `c`/`e`: small inline code span, ~8.3pt, ALSO fully inside the tall box.
	// |8.3-35.7|/35.7 = 0.77 >= 0.7 -> dropped by the raw filter, but it is
	// fully contained, so the fix must keep it (mirrors Python).
	chars := []pdftype.TextChar{
		{Text: "在", X0: 100, X1: 112, Top: 500, Bottom: 512},     // h=12, inside
		{Text: "c", X0: 667, X1: 675, Top: 497.4, Bottom: 505.7}, // h=8.3, inside (certifi)
		{Text: "e", X0: 680, X1: 688, Top: 497.4, Bottom: 505.7}, // h=8.3, inside (certifi)
	}

	got := matchCharsToBoxes(boxes, chars)

	if len(got) != 1 {
		t.Fatalf("expected 1 box char group, got %d", len(got))
	}

	gotTexts := make(map[string]bool)
	for _, c := range got[0] {
		gotTexts[c.Text] = true
	}

	if !gotTexts["在"] {
		t.Errorf("body glyph 在 was dropped from the box")
	}
	// Pre-fix: `c` and `e` are dropped by the height filter (regression guard).
	if !gotTexts["c"] {
		t.Errorf("small inline glyph c (fully contained, h=8.3) was dropped; certifi-style text loss regression")
	}
	if !gotTexts["e"] {
		t.Errorf("small inline glyph e (fully contained, h=8.3) was dropped; certifi-style text loss regression")
	}
}

// TestMatchCharsToBoxes_FullyContainedTinyDropped locks the other boundary of
// the containment guard: a FULLY CONTAINED char that is extremely tiny
// (height <= 10% of the box, ratio >= 0.9) must STILL be dropped by the
// height gate — it is baseline noise (e.g. a 1pt glyph in a 40pt box), not
// inline content. This preserves TestOCRMergeChars_HeightGate's contract.
func TestMatchCharsToBoxes_FullyContainedTinyDropped(t *testing.T) {
	box := ocrDetectBox{
		box: pdftype.TextBox{X0: 0, X1: 90, Top: 0, Bottom: 120},
		x0:  0, y0: 0, x1: 90, y1: 120,
	}
	boxes := []ocrDetectBox{box}
	// h=1 vs bh=120 -> ratio 0.992 >= 0.9, fully contained (overlap 1.0).
	chars := []pdftype.TextChar{
		{X0: 5, X1: 10, Top: 40, Bottom: 41, Text: "t"},
	}
	got := matchCharsToBoxes(boxes, chars)
	if len(got) != 1 {
		t.Fatalf("expected 1 box group, got %d", len(got))
	}
	if len(got[0]) != 0 {
		t.Errorf("extremely tiny fully-contained glyph must be dropped by the height gate, got %+v", got[0])
	}
}

// TestMatchCharsToBoxes_DeferredSpaceOnlyNormalDropped locks the boundary that
// kept dell-configuration-services-sd-zh-tw (and the other real-PDF set) from
// regressing: a box whose ONLY normal-height chars are SPACES must NOT absorb
// deferred inline glyphs. The real line text lives in a tighter neighbor box
// there, so the small glyphs are another line's content — leaving the box
// empty lets buildTextBoxes' OCR fallback recognize the true line from the
// image. Without this guard, plugin-daemon's certifi fix added stray glyphs to
// space-only boxes and dropped gridSim from 100% to 95.9% on that PDF.
func TestMatchCharsToBoxes_DeferredSpaceOnlyNormalDropped(t *testing.T) {
	box := ocrDetectBox{
		box: pdftype.TextBox{X0: 0, X1: 90, Top: 0, Bottom: 40},
		x0:  0, y0: 0, x1: 90, y1: 40,
	}
	boxes := []ocrDetectBox{box}
	// Space char: ratio < 0.7 (normal), but carries no content.
	// "x": small glyph, fully contained, ratio in [0.7, 0.9) -> deferred.
	chars := []pdftype.TextChar{
		{X0: 5, X1: 6, Top: 5, Bottom: 20, Text: " "},
		{X0: 10, X1: 16, Top: 6, Bottom: 12, Text: "x"}, // h=6 vs bh=40 -> ratio 0.85
	}
	got := matchCharsToBoxes(boxes, chars)
	if len(got) != 1 {
		t.Fatalf("expected 1 box group, got %d", len(got))
	}
	for _, c := range got[0] {
		if strings.TrimSpace(c.Text) != "" {
			t.Errorf("deferred glyph %q must be dropped when the box's only normal chars are spaces", c.Text)
		}
	}
}
