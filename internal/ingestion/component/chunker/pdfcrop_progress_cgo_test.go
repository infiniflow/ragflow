//go:build cgo

package chunker

import (
	"context"
	"math"
	"sort"
	"sync"
	"testing"

	agentruntime "ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

type chunkFractionRecorder struct {
	mu        sync.Mutex
	fractions []float64
}

func (r *chunkFractionRecorder) context(t *testing.T, component string) context.Context {
	t.Helper()
	ctx := agentruntime.WithProgressFractionCallback(t.Context(), func(c string, f float64) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.fractions = append(r.fractions, f)
	})
	return agentruntime.BindComponentFraction(ctx, component)
}

func (r *chunkFractionRecorder) getFractions() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]float64, len(r.fractions))
	copy(out, r.fractions)
	return out
}

func TestCropImageChunks_ReportsFractions(t *testing.T) {
	rec := &chunkFractionRecorder{}
	ctx := rec.context(t, "Chunker:pdf")

	chunks := []schema.ChunkDoc{
		{
			Text:      "content 1",
			Positions: jsonPositions(t, []float64{1, 0, 100, 0, 100}),
			CKType:    "text",
		},
		{
			Text:      "content 2",
			Positions: jsonPositions(t, []float64{1, 100, 200, 100, 200}),
			CKType:    "text",
		},
		{
			Text:      "content 3",
			Positions: jsonPositions(t, []float64{1, 200, 300, 200, 300}),
			CKType:    "text",
		},
	}

	out := cropImageChunks(ctx, mockCropEngine{}, chunks)
	if len(out) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(out))
	}

	fractions := rec.getFractions()
	// 3 tasks settled + 1 terminal 1.0 = 4 reports
	if len(fractions) != 4 {
		t.Fatalf("expected 4 reports, got %d (%v)", len(fractions), fractions)
	}
	if last := fractions[len(fractions)-1]; last != 1.0 {
		t.Errorf("last fraction = %f, want 1.0", last)
	}

	// Verify the multiset of reported fractions regardless of concurrent arrival order.
	sorted := make([]float64, len(fractions))
	copy(sorted, fractions)
	sort.Float64s(sorted)

	wantFractions := []float64{1.0 / 3.0, 2.0 / 3.0, 1.0, 1.0}
	for i, want := range wantFractions {
		if math.Abs(sorted[i]-want) > 1e-6 {
			t.Errorf("sorted fraction[%d] = %f, want %f", i, sorted[i], want)
		}
	}
}

func TestCropImageChunks_ZeroTasksReportsNothing(t *testing.T) {
	rec := &chunkFractionRecorder{}
	ctx := rec.context(t, "Chunker:pdf")

	chunks := []schema.ChunkDoc{
		{
			Text:   "no positions",
			CKType: "text",
		},
	}

	out := cropImageChunks(ctx, mockCropEngine{}, chunks)
	if len(out) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(out))
	}

	fractions := rec.getFractions()
	if len(fractions) != 0 {
		t.Errorf("expected 0 reports for 0 crop tasks, got %v", fractions)
	}
}

func TestCropImageChunks_CancelledContextDoesNotReport(t *testing.T) {
	rec := &chunkFractionRecorder{}
	ctx, cancel := context.WithCancel(rec.context(t, "Chunker:pdf"))
	cancel() // Cancel context before invocation

	chunks := []schema.ChunkDoc{
		{
			Text:      "content 1",
			Positions: jsonPositions(t, []float64{1, 0, 100, 0, 100}),
			CKType:    "text",
		},
		{
			Text:      "content 2",
			Positions: jsonPositions(t, []float64{1, 100, 200, 100, 200}),
			CKType:    "text",
		},
	}

	_ = cropImageChunks(ctx, mockCropEngine{}, chunks)
	fractions := rec.getFractions()
	if len(fractions) != 0 {
		t.Errorf("cancelled context must not report progress, got: %v", fractions)
	}
}
