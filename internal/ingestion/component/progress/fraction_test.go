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
