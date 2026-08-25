//go:build cgo && integration

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/deepdoc/parser/pdf/inference"
)

// defaultDeepDocURL is where a locally started OSS DeepDoc inference server
// listens (deepdoc/server/deepdoc_server.py). It is a test convenience only:
// production code requires DEEPDOC_URL to be set explicitly, since no single
// default is correct in both source builds and Docker (http://deepdoc:9390).
const defaultDeepDocURL = "http://localhost:9390"

// requireDeepDocServer skips the test when no healthy DeepDoc inference
// service is reachable at DEEPDOC_URL (default http://localhost:9390).
func requireDeepDocServer(t *testing.T) {
	t.Helper()
	baseURL := strings.TrimSpace(common.GetEnv(common.EnvDeepDocURL))
	if baseURL == "" {
		baseURL = defaultDeepDocURL
	}
	client, err := inference.NewClient(baseURL)
	if err != nil || !client.Health() {
		t.Skipf("DeepDoc inference service unavailable at %s; start deepdoc/server/deepdoc_server.py to run this test", baseURL)
	}
}

func TestPDFParser_ParseWithResult_CGOFixture(t *testing.T) {
	requireDeepDocServer(t)

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
	requireDeepDocServer(t)

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
