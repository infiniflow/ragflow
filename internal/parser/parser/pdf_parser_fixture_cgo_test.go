//go:build cgo

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	deepdocpdf "ragflow/internal/deepdoc/parser/pdf"
	deepdoctype "ragflow/internal/deepdoc/parser/pdf/type"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// useMockDocAnalyzer installs a test-only MockDocAnalyzer as the in-process
// DeepDoc backend. MockDocAnalyzer is test infrastructure and must never sit in
// the production fallback path; it is injected here through the public factory
// seam (SetNativeDocAnalyzerFactory) so the parse pipeline can be exercised
// without a real DeepDoc service or ONNX Runtime models.
func useMockDocAnalyzer(t *testing.T) {
	t.Helper()
	prev := doctype.NativeDocAnalyzerFactory
	t.Cleanup(func() { doctype.NativeDocAnalyzerFactory = prev })
	doctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		return &deepdocpdf.MockDocAnalyzer{Healthy: true}, true
	})
}

func TestPDFParser_ParseWithResult_CGOFixture(t *testing.T) {
	useMockDocAnalyzer(t)

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "Doc1.pdf", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON is empty; want at least 1 parsed item")
	}
	if got := res.File["page_count"]; got == nil {
		t.Fatal("File.page_count missing")
	}
	if positions, ok := res.JSON[0]["_pdf_positions"].([][]any); ok && len(positions) == 0 {
		t.Fatal("JSON[0]._pdf_positions is empty; want normalized positions for fixture text")
	}
}

func TestPDFParser_ParseWithResult_CGOFixtureMarkdown(t *testing.T) {
	useMockDocAnalyzer(t)

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"output_format": "markdown"})

	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "Doc1.pdf", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "markdown"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if res.Markdown == "" {
		t.Fatal("Markdown is empty; want rendered content")
	}
	if len(res.JSON) != 0 {
		t.Fatalf("JSON len = %d, want 0 for markdown output", len(res.JSON))
	}
}

func TestPDFParser_ParseWithResult_CGOFixturePlainText(t *testing.T) {
	useMockDocAnalyzer(t)

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "plain_text"})
	ctx := t.Context()
	res := pdf.ParseWithResult(ctx, "Doc1.pdf", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON is empty; want page text items")
	}
	if got, _ := res.JSON[0]["text"].(string); strings.TrimSpace(got) == "" {
		t.Fatal("plain_text first page is empty")
	}
}
