//go:build cgo

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPDFParser_ParseWithResult_CGOFixturePlainText(t *testing.T) {
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
