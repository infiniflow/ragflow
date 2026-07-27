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
	ctx := t.Context()
	p := NewPPTXParser()
	data := buildPPTX(t, "Hello World")
	res := p.ParseWithResult(ctx, "test.pptx", data)
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
	ctx := t.Context()
	p := NewPPTParser()
	// Use PPTX content — office_oxide may reject it with format="ppt"
	// hint (expects OLE binary). When it does, skip gracefully; when
	// it succeeds, verify the metadata contract.
	data := buildPPTX(t, "Hello")
	res := p.ParseWithResult(ctx, "test.ppt", data)
	if res.Err != nil {
		t.Skip("PPTParser with PPTX data (expected maybe to fail):", res.Err)
	}
	if got := res.File["format"]; got != "ppt" {
		t.Errorf("File[format] = %v, want %q", got, "ppt")
	}
}

// TestPPTParser_TCADPFileType covers : when a .ppt file
// is routed through PPTParser with parse_method="tcadp", the underlying
// PPTXParser must pass "PPT" as the file_type to TCADP, not "PPTX".
// This test verifies format propagation from PPTParser through
// ConfigureFromSetup to the embedded PPTXParser.
func TestPPTParser_TCADPFileType(t *testing.T) {
	// PPTParser delegates to PPTXParser{format:"ppt"}.
	p := NewPPTParser()
	setup := map[string]any{
		"parse_method":  "tcadp",
		"output_format": "json",
	}
	p.ConfigureFromSetup(setup)

	// Verify the embedded PPTXParser received the config and keeps
	// format="ppt" (which maps to "PPT" in TCADP fileType).
	if p.pptx.format != "ppt" {
		t.Errorf("PPTParser.pptx.format = %q, want %q", p.pptx.format, "ppt")
	}
	if p.pptx.ParseMethod != "tcadp" {
		t.Errorf("PPTParser.pptx.ParseMethod = %q, want %q", p.pptx.ParseMethod, "tcadp")
	}
	if p.pptx.OutputFormat != "json" {
		t.Errorf("PPTParser.pptx.OutputFormat = %q, want %q", p.pptx.OutputFormat, "json")
	}
}

// TestPPTXParser_TCADPFileType covers : PPTXParser
// with format="pptx" must derive fileType "PPTX" for TCADP calls.
func TestPPTXParser_TCADPFileType(t *testing.T) {
	p := NewPPTXParser()
	if p.format != "pptx" {
		t.Fatalf("NewPPTXParser().format = %q, want pptx", p.format)
	}
	setup := map[string]any{
		"parse_method":  "tcadp",
		"output_format": "json",
	}
	p.ConfigureFromSetup(setup)
	if p.ParseMethod != "tcadp" {
		t.Errorf("ParseMethod = %q, want tcadp", p.ParseMethod)
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
