//go:build cgo

package infnative

import (
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/common"
	native "ragflow/internal/deepdoc/native"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
)

// These tests need no golden fixtures — they only stat model file names and
// initialize ONNX Runtime (statically linked). Opt the package run in so the
// TestMain fixture guard does not skip them.
func init() {
	testdataFetchAttempted = true
}

// fakeModelDir creates a temp directory containing every file HasModelFiles
// checks, so canServe's model-file half returns true without real weights.
func fakeModelDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range common.DeepDocModelFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("stub"), 0o644); err != nil {
			t.Fatalf("create stub model %s: %v", name, err)
		}
	}
	if !common.HasModelFiles(dir) {
		t.Fatal("fake model dir did not satisfy HasModelFiles")
	}
	return dir
}

// TestServingRequiresRegisteredFactory locks the invariant the startup
// hard-abort gate depends on: ORT initialized + models present is NOT enough —
// the DocAnalyzer factory must also be registered. This is what stops a failed
// execution-provider registration (which returns from Register before the
// factory is installed) from being mistaken for a healthy backend at startup.
func TestServingRequiresRegisteredFactory(t *testing.T) {
	modelDir := fakeModelDir(t)
	if err := native.InitORT(); err != nil {
		t.Skipf("ONNX Runtime not available in this build: %v", err)
	}

	prevDir := registeredModelDir
	prevFactory := deepdoctype.NativeDocAnalyzerFactory
	t.Cleanup(func() {
		registeredModelDir = prevDir
		deepdoctype.SetNativeDocAnalyzerFactory(prevFactory)
	})
	registeredModelDir = modelDir

	// Sanity: the ORT+model half of the check is genuinely ready in this test,
	// so a false Serving() below can only be due to the factory guard.
	if !canServe(modelDir) {
		t.Fatal("precondition failed: canServe(modelDir) is false; test cannot distinguish factory guard")
	}

	// Adversarial: factory NOT registered -> must report not-serving even
	// though ORT and models are ready. Current code returns true here (bug).
	deepdoctype.SetNativeDocAnalyzerFactory(nil)
	if Serving() {
		t.Error("Serving() true with ORT+models ready but factory unregistered; startup would boot a backend with no analyzer")
	}

	// Positive counterpart: with the factory installed, Serving() must be true
	// (guards against an over-broad fix that always returns false).
	deepdoctype.SetNativeDocAnalyzerFactory(func() (deepdoctype.DocAnalyzer, bool) {
		return &NativeAnalyzer{modelDir: modelDir}, true
	})
	if !Serving() {
		t.Error("Serving() false with ORT+models+factory ready")
	}
}

// TestHealthUnaffectedByFactory guards that the instance-level Health check
// keeps its ORT+models-only semantics — the new factory requirement belongs to
// the startup Serving gate, not to per-instance health.
func TestHealthUnaffectedByFactory(t *testing.T) {
	modelDir := fakeModelDir(t)
	if err := native.InitORT(); err != nil {
		t.Skipf("ONNX Runtime not available in this build: %v", err)
	}
	prevFactory := deepdoctype.NativeDocAnalyzerFactory
	deepdoctype.SetNativeDocAnalyzerFactory(nil)
	t.Cleanup(func() { deepdoctype.SetNativeDocAnalyzerFactory(prevFactory) })

	a := &NativeAnalyzer{modelDir: modelDir}
	if !a.Health() {
		t.Error("Health() false with ORT+models ready and factory nil; Health must not depend on the startup factory guard")
	}
}
