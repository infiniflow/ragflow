// Fraction instrumentation tests pin the component-side progress signal:
// the extractor's phase windows stay monotonic across sequential phases,
// awaitFutures reports every settled job, and the tokenizer's embedding
// batches advance the fraction (cache hits counted as finished work).

package component

import (
	"context"
	"sync"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/utility"
)

// fractionRecorder is a concurrency-safe fraction sink, since the extractor
// reports from several goroutines at once.
type fractionRecorder struct {
	mu   sync.Mutex
	seen []fractionObservation
}

func (r *fractionRecorder) context(t *testing.T, component string) context.Context {
	t.Helper()
	ctx := runtime.WithProgressFractionCallback(t.Context(), func(c string, f float64) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.seen = append(r.seen, fractionObservation{c, f})
	})
	return runtime.BindComponentFraction(ctx, component)
}

func (r *fractionRecorder) fractions() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]float64, 0, len(r.seen))
	for _, obs := range r.seen {
		out = append(out, obs.fraction)
	}
	return out
}

func TestFractionSplit_HandsOutMonotonicWindows(t *testing.T) {
	split := newFractionSplit(true, false, true)

	first := split.take(true)
	skipped := split.take(false)
	second := split.take(true)

	if first.base != 0 || first.span != 0.5 {
		t.Errorf("first window = %+v, want base 0 span 0.5", first)
	}
	if skipped.base != 0 || skipped.span != 0 {
		t.Errorf("disabled window = %+v, want the zero window", skipped)
	}
	if second.base != 0.5 || second.span != 0.5 {
		t.Errorf("second window = %+v, want base 0.5 span 0.5", second)
	}

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Extractor:abc")
	first.report(ctx, 0.5)
	skipped.report(ctx, 1)
	second.report(ctx, 1)

	got := rec.fractions()
	want := []float64{0.25, 1}
	if len(got) != len(want) {
		t.Fatalf("fractions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fraction %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestFractionWindow_ClampsRatio(t *testing.T) {
	rec := &fractionRecorder{}
	ctx := rec.context(t, "Extractor:abc")
	w := fractionWindow{base: 0.5, span: 0.5}

	w.report(ctx, -1)
	w.report(ctx, 2)

	got := rec.fractions()
	if len(got) != 2 || got[0] != 0.5 || got[1] != 1 {
		t.Errorf("fractions = %v, want [0.5 1]", got)
	}
}

func TestAwaitFutures_ReportsEverySettledJob(t *testing.T) {
	rec := &fractionRecorder{}
	ctx := rec.context(t, "Extractor:abc")

	futs := make([]utility.WorkerPoolFuture[extractorJob, struct{}], 0, 4)
	for i := 0; i < 4; i++ {
		f, err := extractorPool.Submit(ctx, func() error { return nil })
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		futs = append(futs, f)
	}

	w := fractionWindow{base: 0.5, span: 0.5}
	if err := awaitFutures(ctx, futs, w); err != nil {
		t.Fatalf("awaitFutures: %v", err)
	}

	got := rec.fractions()
	if len(got) != 4 {
		t.Fatalf("expected 4 reports, got %d: %v", len(got), got)
	}
	highest := got[0]
	for i, f := range got {
		if f < w.base || f > w.base+w.span {
			t.Errorf("report %d = %v, outside window [%v %v]", i, f, w.base, w.base+w.span)
		}
		highest = max(highest, f)
	}
	// Goroutines report concurrently, so only the highest value proves the
	// phase reached the end of its window.
	if highest != w.base+w.span {
		t.Errorf("highest report = %v, want %v", highest, w.base+w.span)
	}
}

func TestAwaitFutures_NoJobsNoReports(t *testing.T) {
	rec := &fractionRecorder{}
	ctx := rec.context(t, "Extractor:abc")

	if err := awaitFutures(ctx, nil, fractionWindow{span: 1}); err != nil {
		t.Fatalf("awaitFutures: %v", err)
	}
	if got := rec.fractions(); len(got) != 0 {
		t.Errorf("expected no reports for an empty phase, got %v", got)
	}
}

func TestEmbedChunks_ReportsBatchFractions(t *testing.T) {
	t.Setenv("TOKENIZER_EMBEDDING_BATCH_SIZE", "2")

	comp, stub := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Tokenizer:abc")
	chunks := []schema.ChunkDoc{
		chunkWithID("c1", "alpha"),
		chunkWithID("c2", "beta"),
		chunkWithID("c3", "gamma"),
		chunkWithID("c4", "delta"),
	}

	if _, _, err := comp.embedChunks(ctx, "tenant", "kb", "", chunks, newMemCacheStore()); err != nil {
		t.Fatalf("embedChunks: %v", err)
	}
	if stub.calls.Load() != 2 {
		t.Fatalf("expected 2 embedding batches, got %d", stub.calls.Load())
	}

	got := rec.fractions()
	want := []float64{0.5, 1}
	if len(got) != len(want) {
		t.Fatalf("fractions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fraction %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestEmbedChunks_CountsCacheHitsAsDone(t *testing.T) {
	t.Setenv("TOKENIZER_EMBEDDING_BATCH_SIZE", "1")

	comp, _ := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}

	warm := []schema.ChunkDoc{chunkWithID("c1", "alpha"), chunkWithID("c2", "beta")}
	store := newMemCacheStore()
	if _, _, err := comp.embedChunks(t.Context(), "tenant", "kb", "", warm, store); err != nil {
		t.Fatalf("warm embedChunks: %v", err)
	}

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Tokenizer:abc")
	chunks := append(append([]schema.ChunkDoc(nil), warm...),
		chunkWithID("c3", "gamma"), chunkWithID("c4", "delta"))

	if _, _, err := comp.embedChunks(ctx, "tenant", "kb", "", chunks, store); err != nil {
		t.Fatalf("embedChunks: %v", err)
	}

	// Two of the four chunks come from the cache, so the first freshly embedded
	// chunk already completes 3/4 of the work.
	got := rec.fractions()
	want := []float64{0.75, 1}
	if len(got) != len(want) {
		t.Fatalf("fractions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fraction %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestEmbedChunks_AllCacheHitsReportsCompletion covers the run where every
// content embedding is a cache hit: the batch loop never executes, so the
// phase has to report its completion separately or it stays silent.
func TestEmbedChunks_AllCacheHitsReportsCompletion(t *testing.T) {
	t.Setenv("TOKENIZER_EMBEDDING_BATCH_SIZE", "1")

	comp, _ := withStubEmbedder(t, 4)
	comp.param.Fields = []string{"text"}

	store := newMemCacheStore()
	chunks := []schema.ChunkDoc{chunkWithID("c1", "alpha"), chunkWithID("c2", "beta")}
	if _, _, err := comp.embedChunks(t.Context(), "tenant", "kb", "", chunks, store); err != nil {
		t.Fatalf("warm embedChunks: %v", err)
	}

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Tokenizer:abc")
	if _, _, err := comp.embedChunks(ctx, "tenant", "kb", "", chunks, store); err != nil {
		t.Fatalf("embedChunks: %v", err)
	}

	got := rec.fractions()
	want := []float64{1}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("fractions = %v, want %v (all-cached work must report completion)", got, want)
	}
}
