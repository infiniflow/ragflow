//go:build cgo

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPDFParser_ParseWithResult_CGOFixture(t *testing.T) {
	t.Setenv("DEEPDOC_URL", "")
	t.Setenv("OSSDEEPDOC_URL", "")

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()
	res := pdf.ParseWithResult("Doc1.pdf", data)
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
	t.Setenv("DEEPDOC_URL", "")
	t.Setenv("OSSDEEPDOC_URL", "")

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"output_format": "markdown"})

	res := pdf.ParseWithResult("Doc1.pdf", data)
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
	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"parse_method": "plain_text"})
	res := pdf.ParseWithResult("Doc1.pdf", data)
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
