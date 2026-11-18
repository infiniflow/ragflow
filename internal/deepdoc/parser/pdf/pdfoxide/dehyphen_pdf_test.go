//go:build manual

package pdfoxide

import (
	"os"
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/layout"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestDehyphenateRealPDF is a regression test against the Elsevier article
// that emits U+FFFD at every line-break hyphen (pre-cipitation, sepa-ration,
// re-covery, ...). It is manual-tier and gated on RAGFLOW_TEST_PDF so it
// never runs in CI and does not require committing the 1.5 MB PDF.
//
// Run locally with:
//
//	RAGFLOW_TEST_PDF=~/Downloads/1-s2.0-S0956053X25001680-main.pdf \
//	  bash build.sh --test-manual ./internal/deepdoc/parser/pdf/pdfoxide/ -run TestDehyphenateRealPDF -v
func TestDehyphenateRealPDF(t *testing.T) {
	path := os.Getenv("RAGFLOW_TEST_PDF")
	if path == "" {
		t.Skip("set RAGFLOW_TEST_PDF to the Elsevier PDF to run this regression test")
	}

	doc, err := Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer doc.Close()

	chars, err := doc.GetPageChars(0)
	if err != nil {
		t.Fatalf("get page chars: %v", err)
	}

	// Convert the extractor's local char type into the layout TextChar type.
	tchars := make([]pdf.TextChar, len(chars))
	for i, c := range chars {
		tchars[i] = pdf.TextChar{
			X0:         c.X0,
			X1:         c.X1,
			Top:        c.Top,
			Bottom:     c.Bottom,
			Text:       c.Text,
			PageNumber: c.PageNumber,
		}
	}

	boxes := layout.CharsToBoxes(tchars, 1, false)
	boxes = layout.Dehyphenate(boxes)

	var sb strings.Builder
	for _, b := range boxes {
		sb.WriteString(b.Text)
		sb.WriteString("\n")
	}
	text := sb.String()

	if strings.Contains(text, "\ufffd") {
		t.Errorf("output still contains U+FFFD replacement char:\n%s", text)
	}

	// The three words the user reported must be rejoined, not broken.
	for _, want := range []string{"precipitation", "separation", "recovery"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected rejoined word %q in output, got:\n%s", want, text)
		}
	}

	// And the broken forms must be gone.
	for _, bad := range []string{"pre- cipitation", "pre\u00ad", "sepa\u00ad", "re\u00ad"} {
		if strings.Contains(text, bad) {
			t.Errorf("broken hyphenation not cleaned, found %q in:\n%s", bad, text)
		}
	}
}
