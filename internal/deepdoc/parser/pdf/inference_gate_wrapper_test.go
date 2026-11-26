package pdf

import (
	"context"
	"image"
	"sync/atomic"
	"testing"
	"time"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	doctype "ragflow/internal/deepdoc/parser/type"
)

// gatingDocAnalyzer is a DocAnalyzer that records how many times its inference
// methods actually reached the backend. It is healthy so wrappers do not
// short-circuit before acquiring the gate.
type gatingDocAnalyzer struct {
	dlaCalls atomic.Int64
}

func (a *gatingDocAnalyzer) DLA(ctx context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	a.dlaCalls.Add(1)
	return nil, nil
}
func (a *gatingDocAnalyzer) TSR(ctx context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (a *gatingDocAnalyzer) OCRDetect(ctx context.Context, _ image.Image) ([]pdf.OCRBox, error) {
	return nil, nil
}
func (a *gatingDocAnalyzer) OCRRecognize(ctx context.Context, _ image.Image) ([]pdf.OCRText, error) {
	return nil, nil
}
func (a *gatingDocAnalyzer) Health() bool { return true }

// setupGate installs the process inference budget once for this test binary.
// SetInferenceLimit is intentionally once-only (production registers it at
// boot); both tests in this file require capacity 1, so a single registration
// is shared. The doctype package's own tests run in a separate binary.
func setupGate(t *testing.T, capacity int) {
	t.Helper()
	doctype.SetInferenceLimit(capacity)
	if got := doctype.InferenceLimit(); got != capacity {
		t.Fatalf("InferenceLimit() = %d, want %d", got, capacity)
	}
}

// Adversarial (core regression): inferDLA must acquire the process inference
// gate BEFORE calling the backend. Pre-fix, the wrapper called doc.DLA() with
// no gate, so with the only slot held the backend was still invoked — this
// test fails on that behaviour and is the falsifiable assertion for the fix.
func TestInferDLARespectsInferenceGate(t *testing.T) {
	setupGate(t, 1)
	ctx := context.Background()

	if err := doctype.AcquireInference(ctx); err != nil {
		t.Fatalf("pre-acquire the only slot: %v", err)
	}

	analyzer := &gatingDocAnalyzer{}
	p := &Parser{}
	done := make(chan struct{})
	go func() {
		_, _ = p.inferDLA(ctx, analyzer, image.NewRGBA(image.Rect(0, 0, 1, 1)))
		close(done)
	}()

	select {
	case <-done:
		doctype.ReleaseInference()
		t.Fatalf("inferDLA returned while the only inference slot was held; backend DLA calls=%d (want blocked)", analyzer.dlaCalls.Load())
	case <-time.After(200 * time.Millisecond):
		// blocked on the gate as expected
	}
	if got := analyzer.dlaCalls.Load(); got != 0 {
		t.Fatalf("backend DLA called %d times while gate was full, want 0", got)
	}

	doctype.ReleaseInference()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inferDLA did not proceed after the slot was released")
	}
	if got := analyzer.dlaCalls.Load(); got != 1 {
		t.Fatalf("backend DLA calls after release = %d, want 1", got)
	}
}

// Adversarial (error path): a waiting wrapper must abort on ctx cancellation
// and must never reach the backend, so cancelled pages free their slot.
func TestInferDLAAbortsOnContextCancellation(t *testing.T) {
	setupGate(t, 1)
	ctx := context.Background()
	if err := doctype.AcquireInference(ctx); err != nil {
		t.Fatalf("pre-acquire the only slot: %v", err)
	}
	defer doctype.ReleaseInference()

	callCtx, cancel := context.WithCancel(context.Background())
	analyzer := &gatingDocAnalyzer{}
	p := &Parser{}
	done := make(chan struct{})
	go func() {
		_, _ = p.inferDLA(callCtx, analyzer, image.NewRGBA(image.Rect(0, 0, 1, 1)))
		close(done)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("inferDLA did not abort after ctx cancellation")
	}
	if got := analyzer.dlaCalls.Load(); got != 0 {
		t.Fatalf("backend DLA called %d times on a cancelled request, want 0", got)
	}
}
