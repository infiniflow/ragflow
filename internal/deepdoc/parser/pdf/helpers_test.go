//go:build cgo

package pdf

import (
	"os"
	"path/filepath"
	"testing"

	inf "ragflow/internal/deepdoc/parser/pdf/inference"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// mustConnectInferenceClient returns a InferenceClient for the OSS DeepDoc service.
func mustConnectInferenceClient(t *testing.T) *inf.Client {
	t.Helper()
	url := os.Getenv("OSSDEEPDOC_URL")
	if url == "" {
		url = "http://localhost:9390"
	}
	client, err := inf.NewClient(url)
	if err != nil {
		t.Fatal(err)
	}
	if !client.Health() {
		t.Fatalf("OssDeepDoc not available at %s", url)
	}
	return client
}

// mustOpenEngine opens a PDF from testdata/pdfs/ and returns a pdf.PDFEngine.
func mustOpenEngine(t *testing.T, name string) pdf.PDFEngine {
	t.Helper()
	pdfPath := filepath.Join("testdata", "pdfs", name)
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	eng, err := NewEngine(data)
	if err != nil {
		t.Fatalf("open engine %s: %v", name, err)
	}
	return eng
}

func mustReadPDF(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "pdfs", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}
