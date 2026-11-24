package progress

import (
	"context"
	"math"
	"sort"
	"sync"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestNestedWindowCountsConcurrentCompletions(t *testing.T) {
	var mu sync.Mutex
	var values []float64
	ctx := runtime.BindComponentFraction(runtime.WithProgressFractionCallback(t.Context(), func(id string, f float64) {
		mu.Lock()
		defer mu.Unlock()
		if id != "Compiler:test" {
			t.Errorf("component = %q", id)
		}
		values = append(values, f)
	}), "Compiler:test")
	// Second of two templates, first of two phases: component budget [0.5,0.75].
	completed := (Window{Span: 1}).Sub(1, 2).Sub(0, 2).Counter(ctx, 4)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(completed)
	}
	wg.Wait()
	sort.Float64s(values)
	want := []float64{0.5625, 0.625, 0.6875, 0.75}
	if len(values) != len(want) {
		t.Fatalf("fractions = %v, want %v", values, want)
	}
	for i, f := range values {
		if math.Abs(f-want[i]) > 1e-9 {
			t.Errorf("fraction[%d] = %v, want %v", i, f, want[i])
		}
	}
}

func TestCancelledAndEmptyWindowsDoNotAdvanceProgress(t *testing.T) {
	ctx := runtime.BindComponentFraction(runtime.WithProgressFractionCallback(t.Context(), func(_ string, f float64) {
		t.Errorf("unexpected fraction: %v", f)
	}), "Compiler:test")
	Window{}.Report(ctx, 1)
	(Window{Span: 1}).Counter(ctx, 0)()
	NewSplit(false, false).Take(true).Report(ctx, 1)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	(Window{Span: 1}).Report(canceled, 1)
	(Window{Span: 1}).Counter(canceled, 1)()
}

type detailCall struct {
	component string
	unit      string
	current   uint32
	total     uint32
}

func TestReportDetailFeedsBothChannels(t *testing.T) {
	var details []detailCall
	var fractions []float64
	ctx := runtime.BindComponentDetail(runtime.WithProgressDetailCallback(
		runtime.BindComponentFraction(runtime.WithProgressFractionCallback(t.Context(), func(_ string, f float64) {
			fractions = append(fractions, f)
		}), "Extractor:test"),
		func(component string, d runtime.ProgressDetail) {
			details = append(details, detailCall{component, d.Unit, d.Current, d.Total})
		}), "Extractor:test")

	w := Window{Span: 1}.Sub(1, 2)
	w.ReportDetail(ctx, "chunk:keywords", 3, 10)

	if len(details) != 1 {
		t.Fatalf("details = %v, want one call", details)
	}
	want := detailCall{"Extractor:test", "chunk:keywords", 3, 10}
	if details[0] != want {
		t.Fatalf("detail = %+v, want %+v", details[0], want)
	}
	if len(fractions) != 1 || math.Abs(fractions[0]-0.65) > 1e-9 {
		t.Fatalf("fractions = %v, want [0.65]", fractions)
	}
}

func TestReportDetailRejectsBadInputs(t *testing.T) {
	var details []detailCall
	ctx := runtime.BindComponentDetail(runtime.WithProgressDetailCallback(t.Context(), func(_ string, d runtime.ProgressDetail) {
		details = append(details, detailCall{"", d.Unit, d.Current, d.Total})
	}), "Extractor:test")

	// Zero total must not divide by zero or emit a bogus position.
	(Window{Span: 1}).ReportDetail(ctx, "chunk:keywords", 0, 0)
	// Disabled window must not emit either.
	Window{}.ReportDetail(ctx, "chunk:keywords", 1, 10)
	// Cancelled context must not emit.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	(Window{Span: 1}).ReportDetail(cancelled, "chunk:keywords", 1, 10)

	if len(details) != 0 {
		t.Fatalf("unexpected details: %v", details)
	}
}

func TestCounterWithPhaseFeedsDetailChannel(t *testing.T) {
	var details []detailCall
	ctx := runtime.BindComponentDetail(runtime.WithProgressDetailCallback(t.Context(), func(_ string, d runtime.ProgressDetail) {
		details = append(details, detailCall{"", d.Unit, d.Current, d.Total})
	}), "Compiler:test")

	completed := (Window{Span: 1}).WithPhase("batch:map").Counter(ctx, 3)
	completed()
	completed()

	if len(details) != 2 {
		t.Fatalf("details = %v, want 2 calls", details)
	}
	if details[0].unit != "batch:map" || details[0].current != 1 || details[0].total != 3 {
		t.Fatalf("detail[0] = %+v", details[0])
	}
	if details[1].current != 2 {
		t.Fatalf("detail[1] = %+v, want current 2", details[1])
	}
}

func TestPhasePropagatesThroughSub(t *testing.T) {
	ctx := runtime.BindComponentDetail(runtime.WithProgressDetailCallback(t.Context(), func(_ string, d runtime.ProgressDetail) {
		if d.Unit != "page:refine" {
			t.Errorf("unit = %q, want page:refine", d.Unit)
		}
	}), "Compiler:test")
	w := (Window{Span: 1}).WithPhase("page:refine").Sub(1, 2)
	w.ReportDetail(ctx, w.Phase, 1, 4)
}

func TestReportPositionBranches(t *testing.T) {
	var details []detailCall
	var fractions []float64
	ctx := runtime.BindComponentDetail(runtime.WithProgressDetailCallback(
		runtime.BindComponentFraction(runtime.WithProgressFractionCallback(t.Context(), func(_ string, f float64) {
			fractions = append(fractions, f)
		}), "C"), func(_ string, d runtime.ProgressDetail) {
			details = append(details, detailCall{"", d.Unit, d.Current, d.Total})
		}), "C")

	(Window{Span: 1}).WithPhase("batch:map").ReportPosition(ctx, 2, 4)
	(Window{Span: 1}).ReportPosition(ctx, 1, 4)
	(Window{Span: 1}).WithPhase("batch:map").ReportPosition(ctx, 1, 0)

	if len(details) != 1 || details[0].current != 2 {
		t.Fatalf("details = %v, want one detail with current 2", details)
	}
	// The phased call also advances the fraction channel; the unphased one
	// contributes 0.25; the zero-total call is dropped entirely.
	if len(fractions) != 2 || math.Abs(fractions[0]-0.5) > 1e-9 || math.Abs(fractions[1]-0.25) > 1e-9 {
		t.Fatalf("fractions = %v, want [0.5 0.25]", fractions)
	}
}
