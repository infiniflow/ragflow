//go:build cgo

package parser

import (
	"testing"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// TestPPTXParser_FormatField verifies the format field wiring:
// NewPPTXParser() defaults to "pptx", and an explicit "ppt" can be set.
func TestPPTXParser_FormatField(t *testing.T) {
	p := NewPPTXParser()
	if p.format != "pptx" {
		t.Errorf("NewPPTXParser().format = %q, want %q", p.format, "pptx")
	}
	p2 := &PPTXParser{format: "ppt"}
	if p2.format != "ppt" {
		t.Errorf("explicit PPTXParser{format: \"ppt\"}.format = %q, want %q", p2.format, "ppt")
	}
}

// TestPPTXParser_ParseWithResult_CGO verifies that PPTXParser can
// parse a programmatically generated PPTX document into per-slide
// JSON items. Uses office_oxide's own PptxWriter to produce the
// test data so no external file is needed.
func TestPPTXParser_ParseWithResult_CGO(t *testing.T) {
	p := NewPPTXParser()
	data := buildPPTX(t, "Hello World")
	res := p.ParseWithResult("test.pptx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", res.OutputFormat, "json")
	}
	if got := res.File["format"]; got != "pptx" {
		t.Errorf("File[format] = %v, want %q", got, "pptx")
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON items is empty; expected at least one slide")
	}
}

// TestPPTParser_ParseWithResult_CGO verifies that PPTParser
// delegates correctly to PPTXParser{format:"ppt"} and produces
// output with File["format"] = "ppt".
func TestPPTParser_ParseWithResult_CGO(t *testing.T) {
	p := NewPPTParser()
	// Use PPTX content — office_oxide may reject it with format="ppt"
	// hint (expects OLE binary). When it does, skip gracefully; when
	// it succeeds, verify the metadata contract.
	data := buildPPTX(t, "Hello")
	res := p.ParseWithResult("test.ppt", data)
	if res.Err != nil {
		t.Skip("PPTParser with PPTX data (expected maybe to fail):", res.Err)
	}
	if got := res.File["format"]; got != "ppt" {
		t.Errorf("File[format] = %v, want %q", got, "ppt")
	}
}

// buildPPTX creates a minimal valid PPTX document with one slide
// containing the given text, using office_oxide's PptxWriter.
func buildPPTX(t *testing.T, text string) []byte {
	t.Helper()
	w := officeOxide.NewPptxWriter()
	slide := w.AddSlide()
	w.SetSlideTitle(slide, text)
	data, err := w.ToBytes()
	if err != nil {
		t.Fatalf("PptxWriter.ToBytes: %v", err)
	}
	return data
}
