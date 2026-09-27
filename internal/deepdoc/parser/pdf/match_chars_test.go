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

// TestMatchCharsToBoxes_PartialOverlapHeightFiltered locks that a char which
// only PARTIALLY overlaps its assigned box (overlap < 0.95) is STILL dropped
// by the height gate when its height differs from the box by >=70% — it is an
// adjacent-line glyph poking into the box, not inline content. The deferred
// inline-glyph path requires full containment (bestOverlap >= 0.95), so a
// partial overlap must never be re-kept there.
func TestMatchCharsToBoxes_PartialOverlapHeightFiltered(t *testing.T) {
	box := ocrDetectBox{
		box: pdftype.TextBox{X0: 0, X1: 90, Top: 0, Bottom: 40},
		x0:  0, y0: 0, x1: 90, y1: 40,
	}
	boxes := []ocrDetectBox{box}
	// h=5 vs bh=40 -> ratio 0.875 >= 0.7; the char spans top=-2..3, so only
	// 3 of its 5pt vertically overlap the box (top 0..3) -> overlap 0.6 < 0.95,
	// i.e. NOT fully contained -> deferred path must not absorb it.
	chars := []pdftype.TextChar{
		{X0: 5, X1: 12, Top: -2, Bottom: 3, Text: "x"},
	}
	got := matchCharsToBoxes(boxes, chars)
	if len(got) != 1 {
		t.Fatalf("expected 1 box group, got %d", len(got))
	}
	if len(got[0]) != 0 {
		t.Errorf("partial-overlap height-mismatched glyph must be dropped by the height gate, got %+v", got[0])
	}
}

// TestMatchCharsToBoxes_FullyContainedSmallOvershoot locks the 刑法 footnote
// ① case: a small glyph (h=8.1) whose top rises ~0.6pt above the tall box's
// edge (overlap ratio 0.92, NOT >= 0.95) is still an inline glyph of that box
// — detection noise, not an adjacent line. The deferred inline-glyph path must
// absorb a small top/bottom overshoot (bestOverlap >= 0.90), not just a
// pixel-perfect 0.95.
func TestMatchCharsToBoxes_FullyContainedSmallOvershoot(t *testing.T) {
	box := ocrDetectBox{
		box: pdftype.TextBox{X0: 70, X1: 763, Top: 494, Bottom: 528.7},
		x0:  70, y0: 494, x1: 763, y1: 528.7,
	}
	boxes := []ocrDetectBox{box}
	chars := []pdftype.TextChar{
		{Text: "在", X0: 100, X1: 112, Top: 500, Bottom: 512},       // normal, h=12
		{Text: "①", X0: 100, X1: 108, Top: 493.35, Bottom: 501.45}, // h=8.1, top 0.65pt above box -> overlap ~0.92
	}
	got := matchCharsToBoxes(boxes, chars)
	if len(got) != 1 {
		t.Fatalf("expected 1 box group, got %d", len(got))
	}
	found := false
	for _, c := range got[0] {
		if c.Text == "①" {
			found = true
		}
	}
	if !found {
		t.Errorf("small glyph with 0.92 overlap (0.6pt top overshoot) must be kept as inline content, got %+v", got[0])
	}
}

// TestBoxIsCoveredLeftFragment locks the fix for 刑法's 妨妨 duplicate. The
// OCR detector over-segments a single TOC line into a narrow LEFT box plus the
// real text box; the char layer assigns the glyphs to the real box, leaving
// the left box char-less. OCR-filling that left box re-reads the overlapping
// glyph and duplicates it (妨妨). Such a char-less left box must be detected
// as covered by its same-line right neighbor and left empty (dropped by the
// trailing filter) instead of OCR-filled.
//
// Geometry (刑法 p4): fragment A x0=220.7 x1=239.3 (h~10), container B
// x0=234.7 x1=456.3 (h~12). A is a left overhang: B starts inside A's x-span
// (234.7 in (220.7,239.3]) and A ends before B (239.3 <= 456.3); same line
// (Y-overlap ~1.0); A is much narrower (18.6 << 221.6). B carries the 妨 glyph.
func TestBoxIsCoveredLeftFragment(t *testing.T) {
	frag := ocrDetectBox{
		box: pdftype.TextBox{X0: 220.7, X1: 239.3, Top: 2814.7, Bottom: 2825.0},
		x0:  220.7, y0: 2814.7, x1: 239.3, y1: 2825.0,
	}
	cont := ocrDetectBox{
		box: pdftype.TextBox{X0: 234.7, X1: 456.3, Top: 2813.7, Bottom: 2826.0},
		x0:  234.7, y0: 2813.7, x1: 456.3, y1: 2826.0,
	}

	t.Run("covered left fragment is detected", func(t *testing.T) {
		boxes := []ocrDetectBox{frag, cont}
		// frag's stray char was deferred-then-dropped by the height gate, so
		// its assembled text is empty (selfText == ""); cont carries the 妨
		// glyph (char layer resolved it here).
		boxChars := [][]pdftype.TextChar{
			{{Text: "妨", X0: 237.5, X1: 253.5, Top: 285.1, Bottom: 301.1}},
			{{Text: "妨", X0: 237.5, X1: 253.5, Top: 285.1, Bottom: 301.1}},
		}
		if !boxIsCoveredLeftFragment(boxes, boxChars, 0, "") {
			t.Errorf("left-overhang fragment with empty assembled text must be reported as covered")
		}
		// The container itself carries usable text -> must NOT be reported.
		if boxIsCoveredLeftFragment(boxes, boxChars, 1, "妨害对公司、企业的管理秩序罪") {
			t.Errorf("a box that carries its own text must not be a fragment")
		}
	})

	t.Run("char-less box with no right neighbor is not a fragment", func(t *testing.T) {
		// A lone char-less box (legit OCR target, e.g. a font-encoded caption)
		// with no same-line right neighbor must still be OCR-filled.
		alone := ocrDetectBox{
			box: pdftype.TextBox{X0: 100, X1: 300, Top: 500, Bottom: 520},
			x0:  100, y0: 500, x1: 300, y1: 520,
		}
		boxes := []ocrDetectBox{alone}
		boxChars := [][]pdftype.TextChar{{}}
		if boxIsCoveredLeftFragment(boxes, boxChars, 0, "") {
			t.Errorf("char-less box without a same-line right neighbor must NOT be treated as a fragment")
		}
	})

	t.Run("different-line right neighbor is not a fragment", func(t *testing.T) {
		// Neighbor is on a different line (low Y overlap) -> not covered.
		otherLine := ocrDetectBox{
			box: pdftype.TextBox{X0: 234.7, X1: 456.3, Top: 2900, Bottom: 2920},
			x0:  234.7, y0: 2900, x1: 456.3, y1: 2920,
		}
		boxes := []ocrDetectBox{frag, otherLine}
		boxChars := [][]pdftype.TextChar{
			{},
			{{Text: "妨", X0: 237.5, X1: 253.5, Top: 2905, Bottom: 2918}},
		}
		if boxIsCoveredLeftFragment(boxes, boxChars, 0, "") {
			t.Errorf("a right neighbor on a different line must not cover the fragment")
		}
	})
}
