//go:build cgo

package parser

import (
	"strings"
	"testing"
)

func TestPDFParser_ParseWithResult_CGOInvalidPDF(t *testing.T) {
	t.Setenv("DEEPDOC_URL", "")
	t.Setenv("OSSDEEPDOC_URL", "")

	pdf := NewPDFParser()
	res := pdf.ParseWithResult("bad.pdf", []byte("not a valid pdf"))
	if res.Err == nil {
		t.Fatal("want parse error for invalid PDF bytes, got nil")
	}
}

func TestPDFParser_ParseWithResult_CGOEmpty(t *testing.T) {
	pdf := NewPDFParser()
	res := pdf.ParseWithResult("empty.pdf", nil)
	if res.Err != nil {
		t.Fatalf("empty input: want nil err, got %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if got, want := res.File["name"], "empty.pdf"; got != want {
		t.Fatalf("File.name = %v, want %v", got, want)
	}
	if got := res.File["page_count"]; got != 0 {
		t.Fatalf("File.page_count = %v, want 0", got)
	}
	outline, ok := res.File["outline"].([]map[string]any)
	if !ok {
		t.Fatalf("File.outline type = %T, want []map[string]any", res.File["outline"])
	}
	if len(outline) != 0 {
		t.Fatalf("File.outline len = %d, want 0", len(outline))
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1", len(res.JSON))
	}
	if text, _ := res.JSON[0]["text"].(string); strings.TrimSpace(text) != "" {
		t.Fatalf("JSON[0].text = %q, want empty", text)
	}
}
