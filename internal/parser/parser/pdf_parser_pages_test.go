package parser

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

// TestConfigureFromSetup_Pages verifies ConfigureFromSetup reads the "pages"
// field from the filetype setup map, normalizes it, and assigns it to
// PDFParser.Pages.
func TestConfigureFromSetup_Pages(t *testing.T) {
	t.Run("reads and normalizes pages", func(t *testing.T) {
		p := &PDFParser{}
		setup := map[string]any{
			"pages": []any{
				[]any{float64(1), float64(3)},
				[]any{float64(8), float64(10)},
			},
		}
		p.ConfigureFromSetup(setup)
		want := [][]int{{1, 3}, {8, 10}}
		if !reflect.DeepEqual(p.Pages, want) {
			t.Errorf("Pages = %v, want %v", p.Pages, want)
		}
	})

	t.Run("overlapping ranges merged", func(t *testing.T) {
		p := &PDFParser{}
		setup := map[string]any{
			"pages": []any{
				[]any{float64(1), float64(200)},
				[]any{float64(111), float64(333)},
			},
		}
		p.ConfigureFromSetup(setup)
		want := [][]int{{1, 333}}
		if !reflect.DeepEqual(p.Pages, want) {
			t.Errorf("Pages = %v, want %v", p.Pages, want)
		}
	})

	t.Run("all invalid -> nil", func(t *testing.T) {
		p := &PDFParser{}
		setup := map[string]any{
			"pages": []any{[]any{float64(3), float64(1)}},
		}
		p.ConfigureFromSetup(setup)
		if p.Pages != nil {
			t.Errorf("Pages = %v, want nil", p.Pages)
		}
	})

	t.Run("missing pages key -> nil", func(t *testing.T) {
		p := &PDFParser{}
		setup := map[string]any{"flatten_media_to_text": true}
		p.ConfigureFromSetup(setup)
		if p.Pages != nil {
			t.Errorf("Pages = %v, want nil", p.Pages)
		}
	})

	// Regression guard: reading pages must not break other fields.
	t.Run("other fields still read (no regression)", func(t *testing.T) {
		p := &PDFParser{}
		setup := map[string]any{
			"flatten_media_to_text": true,
			"parse_method":          "DeepDOC",
			"output_format":         "json",
			"pages": []any{
				[]any{float64(1), float64(100)},
			},
		}
		p.ConfigureFromSetup(setup)
		if !p.FlattenMediaToText {
			t.Error("FlattenMediaToText not read")
		}
		if p.ParseMethod != "DeepDOC" {
			t.Errorf("ParseMethod = %q, want DeepDOC", p.ParseMethod)
		}
		if p.OutputFormat != "json" {
			t.Errorf("OutputFormat = %q, want json", p.OutputFormat)
		}
		if want := [][]int{{1, 100}}; !reflect.DeepEqual(p.Pages, want) {
			t.Errorf("Pages = %v, want %v", p.Pages, want)
		}
	})
}

// TestPDFParser_PlainTextHonorsPages pins that the plain_text method reads only
// the configured page ranges and keeps the original page numbers.
func TestPDFParser_PlainTextHonorsPages(t *testing.T) {
	p := NewPDFParser()
	p.ConfigureFromSetup(map[string]any{
		"parse_method": "plain_text",
		"pages":        []any{[]any{float64(2), float64(3)}},
	})
	res := p.ParseWithResult(t.Context(), "three.pdf", threePagePDF(t))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var pageNumbers []any
	for _, item := range res.JSON {
		pageNumbers = append(pageNumbers, item["page_number"])
	}
	if want := []any{2, 3}; !reflect.DeepEqual(pageNumbers, want) {
		t.Fatalf("page numbers = %v, want %v", pageNumbers, want)
	}
}

// threePagePDF builds a minimal uncompressed PDF with one line of text on each
// of its three pages.
func threePagePDF(t *testing.T) []byte {
	t.Helper()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [4 0 R 6 0 R 8 0 R] /Count 3 >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	for _, text := range []string{"page one", "page two", "page three"} {
		content := fmt.Sprintf("BT /F1 24 Tf 72 700 Td (%s) Tj ET", text)
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", len(objects)+2),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		)
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return buf.Bytes()
}
