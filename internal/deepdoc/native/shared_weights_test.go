//go:build cgo && integration

package native

// Integration tests for the per-model weight-sharing integration point. They
// require MODEL_DIR (gated by skipIfNoModels) and a statically linked ONNX
// Runtime resolved via dlopen(NULL), so they self-skip wherever the DeepDoc
// model snapshot or the static ORT archives are absent. When CI bakes both and
// sets DEEPDOC_NATIVE_REQUIRED=1 they run as a hard gate instead of a skip.
//
// Run with:
//   MODEL_DIR=... go test -tags cgo,integration -run TestWeightSharing ./internal/deepdoc/native/...

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// These integration tests need only MODEL_DIR (the .ort weights), not the
// golden fixtures, so opt the package run in without the fetch_testdata tag
// (which would try to download fixtures we don't use here).
func init() {
	testdataFetchAttempted = true
}

// deepdocModels lists every .ort the in-process backend loads, with the
// fixed-shape I/O signature each recognizer uses. The rec.ort width here is
// arbitrary — extraction does not run the graph, so only the rank matters.
var deepdocModels = []struct {
	name    string
	inName  string
	inShape []int64
	outName string
}{
	{"layout.ort", "images", []int64{1, 3, dlaInputSize, dlaInputSize}, "output0"},
	{"tsr.ort", "images", []int64{1, 3, tsrInputSize, tsrInputSize}, "output0"},
	{"det.ort", "x", []int64{1, 3, 736, 736}, "sigmoid_0.tmp_0"},
	{"rec.ort", "x", []int64{1, 3, 48, 320}, "softmax_11.tmp_0"},
}

// TestWeightSharingExtractsAndCaches proves the integration point for every
// model: each model's constant initializers are extracted exactly once per
// modelPath and cached, and a repeat lookup — or any NewSession/recSession for
// that model — reuses the same shared weight set instead of deserializing a
// fresh copy.
func TestWeightSharingExtractsAndCaches(t *testing.T) {
	skipIfNoModels(t)

	// Force a clean cache so the extraction count is deterministic.
	weightMu.Lock()
	weightCache = map[string]*weightSet{}
	weightMu.Unlock()

	for _, m := range deepdocModels {
		modelPath := filepath.Join(os.Getenv("MODEL_DIR"), m.name)

		ws1, err := sharedWeights(modelPath, m.inName, m.inShape, m.outName)
		if err != nil {
			t.Fatalf("sharedWeights(%s) first call: %v", m.name, err)
		}
		if len(ws1.vals) == 0 {
			t.Fatalf("sharedWeights(%s) extracted 0 initializers; expected > 0", m.name)
		}

		ws2, err := sharedWeights(modelPath, m.inName, m.inShape, m.outName)
		if err != nil {
			t.Fatalf("sharedWeights(%s) second call: %v", m.name, err)
		}
		if ws2 != ws1 {
			t.Fatalf("sharedWeights(%s) not cached: second call returned %p, first %p",
				m.name, ws2, ws1)
		}

		weightMu.Lock()
		cached := weightCache[modelPath]
		weightMu.Unlock()
		if cached != ws1 {
			t.Fatalf("weightCache[%s] = %p, expected the cached set %p", m.name, cached, ws1)
		}
	}
}

// TestWeightSharingProducesIdenticalInference proves that injecting the shared
// weight buffers into a fixed-shape (layout.ort) session yields numerically
// identical inference to a session that deserializes its own private copy of
// the weights. Sharing changes the memory footprint, not the arithmetic; if
// extraction copied the weights wrong, the two outputs diverge.
func TestWeightSharingProducesIdenticalInference(t *testing.T) {
	skipIfNoModels(t)

	m := deepdocModels[0] // layout.ort
	modelPath := filepath.Join(os.Getenv("MODEL_DIR"), m.name)
	inShape := m.inShape

	// Non-shared: ORT deserializes its own private copy of the constants.
	alone, err := newRawSession(modelPath, m.inName, inShape, m.outName, nil)
	if err != nil {
		t.Fatalf("newRawSession (non-shared): %v", err)
	}
	defer alone.Destroy()

	// Shared: weights extracted once and injected via AddInitializer.
	shared, err := NewSession(modelPath, m.inName, inShape, m.outName)
	if err != nil {
		t.Fatalf("NewSession (shared): %v", err)
	}
	defer shared.Destroy()

	ctx := context.Background()
	input := make([]float32, prod(inShape))
	outAlone, err := alone.Run(ctx, input)
	if err != nil {
		t.Fatalf("Run (non-shared): %v", err)
	}
	outShared, err := shared.Run(ctx, input)
	if err != nil {
		t.Fatalf("Run (shared): %v", err)
	}
	if !reflect.DeepEqual(outAlone, outShared) {
		t.Fatalf("shared weights changed inference output (%d vs %d elems diverge)",
			len(outAlone), len(outShared))
	}
}

// TestWeightSharingRecProducesIdenticalInference covers the dynamic-width
// (OCR-rec) injection point: newRecSession shares the same weight set as the
// one used by the pooled recSession.
func TestWeightSharingRecProducesIdenticalInference(t *testing.T) {
	skipIfNoModels(t)

	m := deepdocModels[3] // rec.ort
	modelPath := filepath.Join(os.Getenv("MODEL_DIR"), m.name)
	inShape := m.inShape

	weights, err := sharedWeights(modelPath, m.inName, inShape, m.outName)
	if err != nil {
		t.Fatalf("sharedWeights(rec.ort): %v", err)
	}

	alone, err := newRecSession(modelPath, m.inName, inShape, m.outName, nil)
	if err != nil {
		t.Fatalf("newRecSession (non-shared): %v", err)
	}
	defer alone.Destroy()

	shared, err := newRecSession(modelPath, m.inName, inShape, m.outName, weights)
	if err != nil {
		t.Fatalf("newRecSession (shared): %v", err)
	}
	defer shared.Destroy()

	ctx := context.Background()
	input := make([]float32, prod(inShape))
	outAlone, err := alone.Run(ctx, input)
	if err != nil {
		t.Fatalf("Run (non-shared rec): %v", err)
	}
	outShared, err := shared.Run(ctx, input)
	if err != nil {
		t.Fatalf("Run (shared rec): %v", err)
	}
	if !reflect.DeepEqual(outAlone, outShared) {
		t.Fatalf("shared rec weights changed inference output (%d vs %d elems diverge)",
			len(outAlone), len(outShared))
	}
}
