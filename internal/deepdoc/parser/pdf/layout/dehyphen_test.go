package layout

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// mkHyphenBox builds a TextBox with the given text on a single line at the given
// top, with an approximate 10pt height.
func mkHyphenBox(text string, top, colID, page int) pdf.TextBox {
	return pdf.TextBox{
		Text:       text,
		Top:        float64(top),
		Bottom:     float64(top) + 10,
		X0:         100,
		X1:         100 + float64(len(text)*5),
		ColID:      colID,
		PageNumber: page,
	}
}

func TestDehyphenateFFFD(t *testing.T) {
	// "pre" + U+FFFD at line end, "cipitation" continues on next line.
	in := []pdf.TextBox{
		mkHyphenBox("pre\ufffd", 100, 1, 1),
		mkHyphenBox("cipitation", 112, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 box, got %d: %+v", len(out), out)
	}
	if out[0].Text != "precipitation" {
		t.Errorf("expected \"precipitation\", got %q", out[0].Text)
	}
}

func TestDehyphenateSoftHyphen(t *testing.T) {
	in := []pdf.TextBox{
		mkHyphenBox("sepa\u00ad", 100, 1, 1),
		mkHyphenBox("ration", 112, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 || out[0].Text != "separation" {
		t.Errorf("expected rejoined \"separation\", got %+v", out)
	}
}

func TestDehyphenateASCIIHyphen(t *testing.T) {
	in := []pdf.TextBox{
		mkHyphenBox("re-", 100, 1, 1),
		mkHyphenBox("covery", 112, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 || out[0].Text != "recovery" {
		t.Errorf("expected rejoined \"recovery\", got %+v", out)
	}
}

func TestDehyphenateNoMergeUppercaseContinuation(t *testing.T) {
	// Line ends with '-' but the next line starts with a capital → new
	// sentence / dash, not a hyphenated word. Must NOT merge.
	in := []pdf.TextBox{
		mkHyphenBox("word-", 100, 1, 1),
		mkHyphenBox("Word", 112, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 boxes (no merge), got %d: %+v", len(out), out)
	}
	if out[0].Text != "word-" || out[1].Text != "Word" {
		t.Errorf("boxes changed unexpectedly: %q / %q", out[0].Text, out[1].Text)
	}
}

func TestDehyphenateNoMergeDifferentColumn(t *testing.T) {
	in := []pdf.TextBox{
		mkHyphenBox("pre\ufffd", 100, 1, 1),
		mkHyphenBox("cipitation", 112, 2, 1), // different column
	}
	out := Dehyphenate(in)
	if len(out) != 2 {
		t.Errorf("expected 2 boxes (different columns), got %d", len(out))
	}
}

func TestDehyphenateNoMergeDifferentPage(t *testing.T) {
	in := []pdf.TextBox{
		mkHyphenBox("pre\ufffd", 100, 1, 1),
		mkHyphenBox("cipitation", 112, 1, 2), // different page
	}
	out := Dehyphenate(in)
	if len(out) != 2 {
		t.Errorf("expected 2 boxes (different pages), got %d", len(out))
	}
}

func TestDehyphenateNoMergeNonAdjacentLines(t *testing.T) {
	// Marker present and lowercase continuation, but lines are far apart.
	in := []pdf.TextBox{
		mkHyphenBox("pre\ufffd", 100, 1, 1),
		mkHyphenBox("cipitation", 300, 1, 1), // huge vertical gap
	}
	out := Dehyphenate(in)
	if len(out) != 2 {
		t.Errorf("expected 2 boxes (non-adjacent lines), got %d", len(out))
	}
}

func TestDehyphenateCleanIntraWordGarbage(t *testing.T) {
	// A replacement char embedded between two letters inside one box.
	in := []pdf.TextBox{
		mkHyphenBox("a\ufffdb", 100, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 || out[0].Text != "ab" {
		t.Errorf("expected \"ab\", got %q", out[0].Text)
	}
}

func TestDehyphenatePreservesRealHyphen(t *testing.T) {
	// A literal hyphen inside a single word ("lithium-ion") must survive.
	in := []pdf.TextBox{
		mkHyphenBox("lithium-ion", 100, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 || out[0].Text != "lithium-ion" {
		t.Errorf("expected \"lithium-ion\" preserved, got %q", out[0].Text)
	}
}

func TestDehyphenateWellKnownPreserved(t *testing.T) {
	// "well-known" has a real in-word hyphen with no following space.
	in := []pdf.TextBox{
		mkHyphenBox("well-known", 100, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 || out[0].Text != "well-known" {
		t.Errorf("expected \"well-known\" preserved, got %q", out[0].Text)
	}
}

func TestDehyphenateMidBoxFFFDWithSpace(t *testing.T) {
	// Real-PDF shape: FFFD mid-box followed by the line-break space, then the
	// continuation word (e.g. "devel� inorganic").
	in := []pdf.TextBox{
		mkHyphenBox("devel\ufffd inorganic", 100, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 || out[0].Text != "develinorganic" {
		t.Errorf("expected \"develinorganic\", got %q", out[0].Text)
	}
}

func TestDehyphenateLineBreakASCIIHyphenWithSpace(t *testing.T) {
	// "re-" at a line break followed by the line-break space and "covery".
	in := []pdf.TextBox{
		mkHyphenBox("re- covery", 100, 1, 1),
	}
	out := Dehyphenate(in)
	if len(out) != 1 || out[0].Text != "recovery" {
		t.Errorf("expected \"recovery\", got %q", out[0].Text)
	}
}

func TestDehyphenateWholePageAssembly(t *testing.T) {
	// Two independent hyphenated breaks plus an unrelated line.
	in := []pdf.TextBox{
		mkHyphenBox("pre\ufffd", 100, 1, 1),
		mkHyphenBox("cipitation", 112, 1, 1),
		mkHyphenBox("sepa\ufffd", 130, 1, 1),
		mkHyphenBox("ration", 142, 1, 1),
		mkHyphenBox("lithium-ion", 160, 1, 1),
	}
	out := Dehyphenate(in)
	var sb strings.Builder
	for _, b := range out {
		sb.WriteString(b.Text)
		sb.WriteString(" ")
	}
	got := sb.String()
	for _, want := range []string{"precipitation", "separation", "lithium-ion"} {
		if !strings.Contains(got, want) {
			t.Errorf("assembled text %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "\ufffd") {
		t.Errorf("assembled text still contains U+FFFD: %q", got)
	}
}
