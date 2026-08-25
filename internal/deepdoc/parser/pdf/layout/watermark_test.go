package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// makeBox is a small constructor so the per-case tables read like the
// per-glyph reproductions that actually triggered #18145: one box per
// rotated watermark glyph, positioned in disjoint X/Y locations across
// the same page.
func makeBox(page int, x0, x1, top, bot float64, text string) pdf.TextBox {
	return pdf.TextBox{
		X0:         x0,
		X1:         x1,
		Top:        top,
		Bottom:     bot,
		Text:       text,
		PageNumber: page,
	}
}

func TestFilterWatermarkBoxes_DropsRepeatedSingleChars(t *testing.T) {
	// 5× "Q" and 5× "1" on page 0 (≥ watermarkMinOccurrences) — these
	// are the rotated glyphs from the tiled watermark in #18145.
	var boxes []pdf.TextBox
	for i := 0; i < 5; i++ {
		y := float64(100 + i*20)
		boxes = append(boxes,
			makeBox(0, 50, 58, y, y+12, "Q"),
			makeBox(0, 80, 88, y, y+12, "1"),
		)
	}
	out := FilterWatermarkBoxes(boxes)
	for _, b := range out {
		if b.Text == "Q" || b.Text == "1" {
			t.Errorf("watermark glyph %q survived (page=%d)", b.Text, b.PageNumber)
		}
	}
	if len(out) != 0 {
		t.Errorf("expected all watermark glyphs dropped, got %d", len(out))
	}
}

func TestFilterWatermarkBoxes_KeepsLegitimateShortContent(t *testing.T) {
	// 1× "Q", 1× "1", 1× "A" on page 0 — all legitimate single-char uses
	// (list markers, table column labels). Each appears below the
	// threshold so NOTHING should be dropped.
	boxes := []pdf.TextBox{
		makeBox(0, 50, 58, 100, 112, "Q"),
		makeBox(0, 50, 58, 130, 142, "1"),
		makeBox(0, 50, 58, 160, 172, "A"),
	}
	out := FilterWatermarkBoxes(boxes)
	if len(out) != len(boxes) {
		t.Errorf("expected %d boxes kept, got %d", len(boxes), len(out))
	}
}

func TestFilterWatermarkBoxes_KeepsCJK(t *testing.T) {
	// 6× "姓" on page 0 — a real form-field label appearing repeatedly
	// in a Chinese resume. CJK glyphs are ≥3 UTF-8 bytes per rune, so
	// isSingleAsciiAlnum rejects them outright. Every box survives.
	var boxes []pdf.TextBox
	for i := 0; i < 6; i++ {
		boxes = append(boxes,
			makeBox(0, 50, 58, float64(100+i*20), float64(112+i*20), "姓"),
		)
	}
	out := FilterWatermarkBoxes(boxes)
	if len(out) != len(boxes) {
		t.Errorf("CJK boxes (%d) lost — watermark filter must not touch CJK", len(boxes)-len(out))
	}
}

func TestFilterWatermarkBoxes_KeepsRepeatedMultiCharTokens(t *testing.T) {
	// "ABC123" is a realistic repeated SKU / batch-code / part-number
	// pattern. Even when it appears 8× per page, the multi-char length
	// keeps it out of the watermark filter, so the data is not lost.
	var boxes []pdf.TextBox
	for i := 0; i < 8; i++ {
		boxes = append(boxes,
			makeBox(0, float64(50+i*30), float64(80+i*30), 100, 112, "ABC123"),
		)
	}
	out := FilterWatermarkBoxes(boxes)
	if len(out) != len(boxes) {
		t.Errorf("multi-char SKU '%s' should survive verbatim, lost %d", "ABC123", len(boxes)-len(out))
	}
}

func TestFilterWatermarkBoxes_PerPageScopeIsolatesByPageNumber(t *testing.T) {
	// 4× "Q" on page 0 (≥ threshold → watermark), 2× "Q" on page 1
	// (legitimate repetition below threshold).
	var boxes []pdf.TextBox
	for i := 0; i < 4; i++ {
		boxes = append(boxes, makeBox(0, float64(50+i*30), 58, 100, 112, "Q"))
	}
	for i := 0; i < 2; i++ {
		boxes = append(boxes, makeBox(1, float64(50+i*30), 58, 100, 112, "Q"))
	}
	out := FilterWatermarkBoxes(boxes)
	page0 := 0
	page1 := 0
	for _, b := range out {
		switch b.PageNumber {
		case 0:
			page0++
		case 1:
			page1++
		}
	}
	if page0 != 0 {
		t.Errorf("page 0 still has %d 'Q' boxes after filter", page0)
	}
	if page1 != 2 {
		t.Errorf("page 1 'Q' boxes lost: want 2, got %d", page1)
	}
}

func TestFilterWatermarkBoxes_HandlesEmptyAndNil(t *testing.T) {
	if got := FilterWatermarkBoxes(nil); len(got) != 0 {
		t.Errorf("nil input should give empty output, got %d", len(got))
	}
	if got := FilterWatermarkBoxes([]pdf.TextBox{}); len(got) != 0 {
		t.Errorf("empty input should give empty output, got %d", len(got))
	}
}

func TestFilterWatermarkBoxes_NoFalsePositiveOnMixed(t *testing.T) {
	// A page with one "Q", one "1", one "ABC123", one " ", one "," — no
	// single ASCII-alnum token hits the threshold, nothing dropped.
	boxes := []pdf.TextBox{
		makeBox(0, 50, 58, 100, 112, "Q"),
		makeBox(0, 50, 58, 130, 142, "1"),
		makeBox(0, 50, 100, 160, 172, "ABC123"),
		makeBox(0, 50, 58, 190, 202, " "),
		makeBox(0, 50, 58, 220, 232, ","),
	}
	out := FilterWatermarkBoxes(boxes)
	if len(out) != len(boxes) {
		t.Errorf("expected %d boxes kept, got %d (lost %d)", len(boxes), len(out), len(boxes)-len(out))
	}
}

// 4 raw "Q" boxes reach the watermark threshold and must drop; the
// 3 padded variants ("Q ", " Q", " Q ") are not single-character
// candidates (length > 1) and must survive.
func TestFilterWatermarkBoxes_WhitespacePaddedNotCollapsedWithBareSingleChar(t *testing.T) {
	boxes := []pdf.TextBox{
		makeBox(0, 50, 58, 100, 112, "Q"),   // raw → candidate
		makeBox(0, 50, 58, 130, 142, "Q "),  // padded → NOT a candidate under no-TrimSpace
		makeBox(0, 50, 58, 160, 172, " Q"),  // padded
		makeBox(0, 50, 58, 190, 202, " Q "), // padded
		makeBox(0, 50, 58, 220, 232, "Q"),   // raw → candidate
		makeBox(0, 50, 58, 250, 262, "Q"),   // raw → candidate
		makeBox(0, 50, 58, 280, 292, "Q"),   // raw → candidate
	}
	if got := len(boxes); got != 7 {
		t.Fatalf("sanity: want 7 boxes, got %d", got)
	}
	out := FilterWatermarkBoxes(boxes)
	rawQ, padded := 0, 0
	for _, b := range out {
		switch b.Text {
		case "Q":
			rawQ++
		case "Q ", " Q", " Q ":
			padded++
		}
	}
	if rawQ != 0 {
		t.Errorf("expected all raw 'Q' boxes dropped (4 promoted, ≥ threshold), %d survived", rawQ)
	}
	if padded != 3 {
		t.Errorf("expected all 3 whitespace-padded boxes kept, %d survived", padded)
	}
	if len(out) != 3 {
		t.Errorf("expected 3 surviving boxes, got %d", len(out))
	}
}

func TestIsSingleAsciiAlnum(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"A", true}, {"z", true}, {"0", true}, {"9", true},
		{"", false}, {"AA", false}, {"a1", false},
		{" ", false}, {",", false}, {"中", false},
		{"\n", false}, {"\x00", false},
	}
	for _, c := range cases {
		if got := isSingleAsciiAlnum(c.in); got != c.want {
			t.Errorf("isSingleAsciiAlnum(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
