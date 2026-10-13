//go:build cgo && integration

package component

import (
	_ "embed"
	"os"
	"strings"
	"testing"

	nativeanalyzer "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

//go:embed testdata/picture_ocr.png
var pictureOCRFixture []byte

const pictureOCRExpectedText = "HELLOWORLD"

// registerNativeOCRModels installs the in-process DeepDoc analyzer backed by the
// real ONNX models. MODEL_DIR is supplied by the CI integration jobs.
func registerNativeOCRModels(t *testing.T) {
	t.Helper()
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		t.Skip("MODEL_DIR required for native OCR integration")
	}
	original := deepdoctype.NativeDocAnalyzerFactory
	t.Cleanup(func() { deepdoctype.NativeDocAnalyzerFactory = original })
	if err := nativeanalyzer.Register(modelDir, nativeanalyzer.DefaultDropScore); err != nil {
		t.Fatal(err)
	}
}

// imageSetupsWithSwitch returns the new contract shape: the image family states
// its OCR choice with ocr_enabled and carries no parse_method at all.
func imageSetupsWithSwitch(ocrEnabled bool) map[string]schema.ParserSetup {
	setups := defaultSetups()
	delete(setups["image"], "parse_method")
	setups["image"]["ocr_enabled"] = ocrEnabled
	return setups
}

func assertNativeOCRTextAndImage(t *testing.T, result parser.ParseResult) {
	t.Helper()
	text, _ := result.JSON[0]["text"].(string)
	if strings.Join(strings.Fields(strings.ToUpper(text)), "") != pictureOCRExpectedText {
		t.Fatalf("native OCR text = %q, want HELLO WORLD: warnings = %v", text, result.Warnings)
	}
	if result.JSON[0]["image"] == "" || result.JSON[0]["doc_type_kwd"] != "image" {
		t.Fatalf("missing image attachment: %+v", result.JSON[0])
	}
}

// TestMaybeDispatchImageNativeOCRWithoutVision runs the real recognizer through
// the legacy parse_method shape, so it also guards the backward-compatible
// reading of canvases saved before the ocr_enabled switch existed.
func TestMaybeDispatchImageNativeOCRWithoutVision(t *testing.T) {
	registerNativeOCRModels(t)
	result, handled, err := maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL,
		"picture_ocr.png", pictureOCRFixture, nil, defaultSetups(), false, visionSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(result.JSON) != 1 {
		t.Fatalf("result = %+v, handled = %v", result, handled)
	}
	assertNativeOCRTextAndImage(t, result)
}

// TestMaybeDispatchImageNativeOCRWithSwitch drives the same real models through
// the ocr_enabled switch that new canvases send. Without it the switch shape
// would only ever be exercised against a fake analyzer.
func TestMaybeDispatchImageNativeOCRWithSwitch(t *testing.T) {
	registerNativeOCRModels(t)
	result, handled, err := maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL,
		"picture_ocr.png", pictureOCRFixture, nil, imageSetupsWithSwitch(true), false, visionSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if !handled || len(result.JSON) != 1 {
		t.Fatalf("result = %+v, handled = %v", result, handled)
	}
	assertNativeOCRTextAndImage(t, result)
}

// TestMaybeDispatchImageNativeSwitchOffRunsNoOCR pins the supersession of #20668
// against the real backend: with the switch off and enhancement off the analyzer
// is never consulted, so the item keeps its image with empty text, and the
// dispatch says so instead of failing silently.
func TestMaybeDispatchImageNativeSwitchOffRunsNoOCR(t *testing.T) {
	registerNativeOCRModels(t)
	result, handled, err := maybeDispatchImage(t.Context(), nil, utility.FileTypeVISUAL,
		"picture_ocr.png", pictureOCRFixture, nil, imageSetupsWithSwitch(false), false, visionSettings{})
	if err != nil || result.Err != nil {
		t.Fatalf("degraded config must not error: err = %v, result.Err = %v", err, result.Err)
	}
	if !handled || len(result.JSON) != 1 {
		t.Fatalf("result = %+v, handled = %v", result, handled)
	}
	if text, _ := result.JSON[0]["text"].(string); text != "" {
		t.Fatalf("OCR must not run with the switch off, got text %q", text)
	}
	if result.JSON[0]["image"] == "" {
		t.Fatalf("image attachment must survive: %+v", result.JSON[0])
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "the Tokenizer will discard this item") {
		t.Fatalf("expected a no-text-source warning, got %v", result.Warnings)
	}
}
