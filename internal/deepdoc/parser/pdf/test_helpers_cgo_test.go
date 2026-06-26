//go:build cgo

package parser

import (
	"os"
	"path/filepath"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	inf "ragflow/internal/deepdoc/parser/pdf/inference"
)

// ── Shared CGO test helpers ──────────────────────────────────────────────────
// These helpers were previously duplicated across multiple test files with
// different build tags (integration, manual). Consolidating them into one file
// with the //go:build cgo tag makes them available to all cgo-tagged tests.

// mustConnectDeepDoc returns a InferenceClient; fatals if unavailable.
func mustConnectDeepDoc(t *testing.T) *inf.InferenceClient {
	t.Helper()
	url := os.Getenv("DEEPDOC_URL")
	if url == "" {
		url = "http://localhost:9390"
	}
	client, err := inf.NewInferenceClient(url)
	if err != nil {
		t.Fatal(err)
	}
	if !client.Health() {
		t.Fatalf("DeepDoc not available at %s", url)
	}
	return client
}

// mustConnectOssDeepDoc returns a InferenceClient pointed at the OSS service;
// skips the test if the service reports a non-OSS model type.
func mustConnectOssDeepDoc(t *testing.T) *inf.InferenceClient {
	t.Helper()
	url := os.Getenv("OSSDEEPDOC_URL")
	if url == "" {
		url = "http://localhost:9390"
	}
	client, err := inf.NewInferenceClient(url)
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
