//go:build cgo

package pdf

import (
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/deepdoc/native"
	infnative "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// mustConnectInProcessAnalyzer builds the in-process NativeAnalyzer (real ONNX
// inference). It replaces the former external Python service client: the
// production path serves DeepDoc entirely in-process, so regression tests
// should exercise the same backend. The test is skipped (not failed) when
// MODEL_DIR is unset (ONNX Runtime is statically linked and resolved via
// dlopen(NULL)); native.InitORT is idempotent across tests.
func mustConnectInProcessAnalyzer(t *testing.T) deepdoctype.DocAnalyzer {
	t.Helper()
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		t.Skip("MODEL_DIR required (in-process backend integration)")
	}
	if err := native.InitORT(); err != nil {
		t.Fatalf("InitORT: %v", err)
	}
	a, err := infnative.NewAnalyzer(modelDir, infnative.DefaultDropScore)
	if err != nil {
		t.Fatalf("NewAnalyzer: %v", err)
	}
	return a
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
