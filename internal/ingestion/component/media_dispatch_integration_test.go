//go:build cgo && integration

package component

import (
	"os"
	"strings"
	"testing"

	nativeanalyzer "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/utility"
)

func TestMaybeDispatchImageNativeOCRWithoutVision(t *testing.T) {
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		t.Skip("MODEL_DIR required for native OCR integration")
	}
	original := deepdoctype.NativeDocAnalyzerFactory
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = original })
	if err := nativeanalyzer.Register(modelDir, nativeanalyzer.DefaultDropScore); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../deepdoc/native/testdata/page0.png")
	if err != nil {
		t.Fatal(err)
	}
	result, handled, err := maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL,
		"page0.png", data, nil, defaultSetups(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(result.JSON) != 1 {
		t.Fatalf("result = %+v, handled = %v", result, handled)
	}
	text, _ := result.JSON[0]["text"].(string)
	if strings.TrimSpace(text) == "" {
		t.Fatalf("native OCR returned no text: warnings = %v", result.Warnings)
	}
	if result.JSON[0]["image"] == "" || result.JSON[0]["doc_type_kwd"] != "image" {
		t.Fatalf("missing image attachment: %+v", result.JSON[0])
	}
}
