//go:build cgo

package parser

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newDeepDocStubServer serves a minimally valid DeepDoc inference API: a
// healthy /health endpoint and empty DLA/TSR/OCR predictions. It keeps the
// default test tier exercising the full DeepDoc HTTP path (client, parse, and
// JSON/Markdown serialization) without a live inference service.
func newDeepDocStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/predict/dla", "/predict/tsr":
			_, _ = w.Write([]byte(`{"bboxes":[]}`))
		case "/predict/ocr":
			_, _ = w.Write([]byte(`{"output":[]}`))
		default:
			t.Errorf("unexpected DeepDoc request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPDFParser_ParseWithResult_CGODeepDocStubJSON(t *testing.T) {
	srv := newDeepDocStubServer(t)
	t.Setenv("DEEPDOC_URL", srv.URL)
	t.Setenv("OSSDEEPDOC_URL", "")

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()

	res := pdf.ParseWithResult(t.Context(), "Doc1.pdf", data)
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
	positions, ok := res.JSON[0]["_pdf_positions"].([][]any)
	if !ok || len(positions) == 0 {
		t.Fatalf("JSON[0]._pdf_positions = %#v; want non-empty [][]any with normalized positions for fixture text", res.JSON[0]["_pdf_positions"])
	}
}

func TestPDFParser_ParseWithResult_CGODeepDocStubMarkdown(t *testing.T) {
	srv := newDeepDocStubServer(t)
	t.Setenv("DEEPDOC_URL", srv.URL)
	t.Setenv("OSSDEEPDOC_URL", "")

	path := filepath.Join("..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := NewPDFParser()
	pdf.ConfigureFromSetup(map[string]any{"output_format": "markdown"})

	res := pdf.ParseWithResult(t.Context(), "Doc1.pdf", data)
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
