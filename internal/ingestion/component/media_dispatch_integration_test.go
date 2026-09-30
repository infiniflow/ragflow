//go:build cgo && integration

package component

import (
	_ "embed"
	"os"
	"strings"
	"testing"

	nativeanalyzer "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/utility"
)

//go:embed testdata/picture_ocr.png
var pictureOCRFixture []byte

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
	result, handled, err := maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL,
		"picture_ocr.png", pictureOCRFixture, nil, defaultSetups(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(result.JSON) != 1 {
		t.Fatalf("result = %+v, handled = %v", result, handled)
	}
	text, _ := result.JSON[0]["text"].(string)
	if strings.Join(strings.Fields(strings.ToUpper(text)), "") != "HELLOWORLD" {
		t.Fatalf("native OCR text = %q, want HELLO WORLD: warnings = %v", text, result.Warnings)
	}
	if result.JSON[0]["image"] == "" || result.JSON[0]["doc_type_kwd"] != "image" {
		t.Fatalf("missing image attachment: %+v", result.JSON[0])
	}
}
