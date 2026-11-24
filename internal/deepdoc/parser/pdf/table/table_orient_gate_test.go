package table

import (
	"context"
	"image"
	"sync/atomic"
	"testing"
	"time"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// gatedRotationDoc implements DocAnalyzer with atomic call counters so gate
// assertions stay race-free when orientation runs in a goroutine.
type gatedRotationDoc struct {
	detectCalls atomic.Int64
	recCalls    atomic.Int64
}

func (d *gatedRotationDoc) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (d *gatedRotationDoc) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (d *gatedRotationDoc) Health() bool { return true }
func (d *gatedRotationDoc) OCRDetect(context.Context, image.Image) ([]pdf.OCRBox, error) {
	d.detectCalls.Add(1)
	return nil, nil
}
func (d *gatedRotationDoc) OCRRecognize(context.Context, image.Image) ([]pdf.OCRText, error) {
	d.recCalls.Add(1)
	return nil, nil
}

func setupTableGate(t *testing.T) {
	t.Helper()
	doctype.SetInferenceLimit(1)
	if got := doctype.InferenceLimit(); got != 1 {
		t.Fatalf("InferenceLimit() = %d, want 1", got)
	}
}

// Adversarial (core): with the only inference slot held, orientation must not
// reach the OCR backend. Pre-fix, table_orient.go called doc.OCRDetect with no
// gate, so the backend ran even while the budget was exhausted.
func TestEvaluateTableOrientationRespectsInferenceGate(t *testing.T) {
	setupTableGate(t)
	ctx := context.Background()
	if err := doctype.AcquireInference(ctx); err != nil {
		t.Fatalf("pre-acquire the only slot: %v", err)
	}

	doc := &gatedRotationDoc{}
	done := make(chan struct{})
	go func() {
		EvaluateTableOrientation(ctx, makeTestTableImage(), doc)
		close(done)
	}()

	select {
	case <-done:
		doctype.ReleaseInference()
		t.Fatalf("orientation returned while the only slot was held; OCRDetect calls=%d (want blocked)", doc.detectCalls.Load())
	case <-time.After(200 * time.Millisecond):
	}
	if got := doc.detectCalls.Load(); got != 0 {
		t.Fatalf("OCRDetect called %d times while gate was full, want 0", got)
	}

	doctype.ReleaseInference()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("orientation did not proceed after the slot was released")
	}
	if got := doc.detectCalls.Load(); got == 0 {
		t.Fatal("OCRDetect never called after the slot was released")
	}
}

// recognizeGateProbeDoc returns one valid line from OCRDetect, then probes the
// gate from inside OCRRecognize. If the wrapper correctly holds a slot across
// the recognize call, the probe (capacity 1) must time out; if the call is
// ungated, the probe succeeds and flags the gap.
type recognizeGateProbeDoc struct {
	probedUngated atomic.Bool
}

func (d *recognizeGateProbeDoc) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (d *recognizeGateProbeDoc) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (d *recognizeGateProbeDoc) Health() bool { return true }
func (d *recognizeGateProbeDoc) OCRDetect(context.Context, image.Image) ([]pdf.OCRBox, error) {
	return []pdf.OCRBox{{X0: 0, Y0: 0, X1: 100, Y1: 0, X2: 100, Y2: 8, X3: 0, Y3: 8}}, nil
}
func (d *recognizeGateProbeDoc) OCRRecognize(context.Context, image.Image) ([]pdf.OCRText, error) {
	probeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := doctype.AcquireInference(probeCtx); err == nil {
		doctype.ReleaseInference()
		d.probedUngated.Store(true)
	}
	return []pdf.OCRText{{Text: "X", Confidence: 0.9}}, nil
}

// Adversarial (second call site): doc.OCRRecognize must also run under the gate.
func TestEvaluateTableOrientationGatesRecognizeCalls(t *testing.T) {
	setupTableGate(t)
	doc := &recognizeGateProbeDoc{}
	EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
	if doc.probedUngated.Load() {
		t.Fatal("OCRRecognize ran without holding an inference slot")
	}
}

// Adversarial (error path): a cancelled context with the gate full must skip
// OCR entirely and yield zero scores, never reaching the backend.
func TestEvaluateTableOrientationCancelledContextSkipsOCR(t *testing.T) {
	setupTableGate(t)
	ctx := context.Background()
	if err := doctype.AcquireInference(ctx); err != nil {
		t.Fatalf("pre-acquire the only slot: %v", err)
	}
	defer doctype.ReleaseInference()

	callCtx, cancel := context.WithCancel(ctx)
	cancel()
	doc := &gatedRotationDoc{}
	_, _, scores := EvaluateTableOrientation(callCtx, makeTestTableImage(), doc)

	if got := doc.detectCalls.Load(); got != 0 {
		t.Fatalf("OCRDetect called %d times on a cancelled request, want 0", got)
	}
	for angle, score := range scores {
		if score != 0 {
			t.Fatalf("cancelled request scored angle %d = %v, want 0", angle, score)
		}
	}
}
