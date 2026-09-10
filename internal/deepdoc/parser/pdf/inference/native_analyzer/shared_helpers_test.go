//go:build cgo

package infnative

import (
	"os"
	"testing"

	"ragflow/internal/deepdoc/native"
)

// deepdocNativeRequired reports whether a missing ONNX Runtime / model
// prerequisite must fail loud instead of skipping. Set DEEPDOC_NATIVE_REQUIRED=1
// in CI so a missing prerequisite is caught rather than painted green.
func deepdocNativeRequired() bool {
	return os.Getenv("DEEPDOC_NATIVE_REQUIRED") == "1"
}

func labelKey(labels []string, label string) int {
	for i, l := range labels {
		if l == label {
			return i
		}
	}
	return -1
}

// analyzerWithModels builds a NativeAnalyzer after ensuring ONNX Runtime is
// initialized. MODEL_DIR is always required, and ONNX Runtime is resolved
// statically via dlopen(NULL). If the binary was not built with static ORT,
// InitORT() fails and the test skips rather than fails. InitORT is idempotent
// so it composes with the other analyzer tests.
//
// It is defined in this cgo-tagged file (not the integration-tagged suite) so
// the manual-tier raster-alignment tests can reuse it without the integration
// tag.
func analyzerWithModels(t *testing.T) *NativeAnalyzer {
	t.Helper()
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		if deepdocNativeRequired() {
			t.Fatalf("MODEL_DIR must be set: the in-process DeepDoc backend is required (DEEPDOC_NATIVE_REQUIRED=1)")
		}
		t.Skip("MODEL_DIR required (in-process backend integration)")
	}
	if err := native.InitORT(); err != nil {
		if deepdocNativeRequired() {
			t.Fatalf("ONNX Runtime not statically linked but required (DEEPDOC_NATIVE_REQUIRED=1): %v", err)
		}
		t.Skipf("ORT not available (not statically linked): %v", err)
	}
	a, err := NewAnalyzer(modelDir, DefaultDropScore)
	if err != nil {
		t.Fatalf("NewAnalyzer: %v", err)
	}
	return a
}
