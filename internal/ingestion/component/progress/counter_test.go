//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package progress

import (
	"context"
	"sync"
	"testing"
	"time"

	agentruntime "ragflow/internal/agent/runtime"
)

type detailReport struct {
	current uint32
	total   uint32
}

func detailContext(t *testing.T, reports *[]detailReport) context.Context {
	t.Helper()
	ctx := agentruntime.WithProgressDetailCallback(t.Context(), func(_ string, d agentruntime.ProgressDetail) {
		*reports = append(*reports, detailReport{d.Current, d.Total})
	})
	return agentruntime.BindComponentDetail(ctx, "test-component")
}

func TestCounterEmitsInitialAndMonotonicPositions(t *testing.T) {
	var reports []detailReport
	ctx := detailContext(t, &reports)

	c := NewCounter(ctx, "image:crop", 3, Window{Span: 1})
	c.Inc()
	c.Inc()
	c.Inc()
	c.Finish()

	want := []detailReport{{0, 3}, {1, 3}, {2, 3}, {3, 3}}
	if len(reports) != len(want) {
		t.Fatalf("reports = %v, want %v", reports, want)
	}
	for i := range want {
		if reports[i] != want[i] {
			t.Errorf("report[%d] = %v, want %v", i, reports[i], want[i])
		}
	}
}

func TestCounterIgnoresStaleAndOutOfRangeReports(t *testing.T) {
	var reports []detailReport
	ctx := detailContext(t, &reports)

	c := NewCounter(ctx, "unit", 5, Window{Span: 1})
	c.Report(3, 5)
	c.Report(2, 5)  // stale — must not regress
	c.Report(99, 5) // above total — must clamp to 5
	c.Report(4, 5)  // stale after the clamp — correctly dropped

	want := []detailReport{{0, 5}, {3, 5}, {5, 5}}
	for i := range want {
		if i >= len(reports) || reports[i] != want[i] {
			t.Fatalf("reports = %v, want %v", reports, want)
		}
	}
}

func TestCounterZeroTotalReportsNothing(t *testing.T) {
	var reports []detailReport
	ctx := detailContext(t, &reports)

	c := NewCounter(ctx, "unit", 0, Window{Span: 1})
	c.Inc()
	c.Finish()
	c.Report(1, 0)

	if len(reports) != 0 {
		t.Fatalf("zero-total counter must stay silent, got %v", reports)
	}
}

func TestCounterFinishIsIdempotent(t *testing.T) {
	var reports []detailReport
	ctx := detailContext(t, &reports)

	c := NewCounter(ctx, "unit", 2, Window{Span: 1})
	c.Finish()
	c.Finish()
	c.Report(1, 2) // must be ignored after finish

	want := []detailReport{{0, 2}, {2, 2}}
	if len(reports) != len(want) {
		t.Fatalf("reports = %v, want %v", reports, want)
	}
	for i := range want {
		if reports[i] != want[i] {
			t.Errorf("report[%d] = %v, want %v", i, reports[i], want[i])
		}
	}
}

// Coupled-field adversarial: with a fraction callback installed, both channels
// must receive the same position; without one, detail must still be emitted.
func TestCounterSyncsFractionAndDetail(t *testing.T) {
	var reports []detailReport
	var fractions []float64
	ctx := agentruntime.WithProgressDetailCallback(t.Context(), func(_ string, d agentruntime.ProgressDetail) {
		reports = append(reports, detailReport{d.Current, d.Total})
	})
	ctx = agentruntime.WithProgressFractionCallback(ctx, func(_ string, f float64) {
		fractions = append(fractions, f)
	})
	ctx = agentruntime.BindComponentDetail(ctx, "test-component")
	ctx = agentruntime.BindComponentFraction(ctx, "test-component")

	// The counter reuses Window for fraction emission: Base/Span shape the
	// fraction while the detail channel keeps the raw current/total.
	c := NewCounter(ctx, "chunk:embed", 2, Window{Base: 0.5, Span: 0.5})
	c.Inc()
	c.Finish()

	// The initial 0/N detail has no fraction (document progress is monotonic).
	if len(reports) != len(fractions)+1 {
		t.Fatalf("detail reports %v and fractions %v must differ only by the initial 0/N", reports, fractions)
	}
	if reports[0].current != 0 {
		t.Fatalf("first detail = %v, want 0/N", reports[0])
	}
	for i, r := range reports[1:] {
		want := 0.5 + 0.5*(float64(r.current)/float64(r.total))
		if fractions[i] != want {
			t.Errorf("fraction[%d] = %v, want %v", i, fractions[i], want)
		}
	}
}

func TestCounterWithNoSinkDoesNotPanic(t *testing.T) {
	c := NewCounter(context.Background(), "unit", 2, Window{})
	c.Inc()
	c.Finish()
}

// The first report establishes the total; later callbacks must not be able to
// change it (a backend may report an inconsistent total mid-run).
func TestCounterLocksTotalAfterFirstReport(t *testing.T) {
	var reports []detailReport
	ctx := detailContext(t, &reports)

	c := NewCounter(ctx, "unit", 0, Window{Span: 1})
	c.Report(1, 10)
	c.Report(2, 20) // inconsistent total — must stay 10

	if len(reports) != 2 {
		t.Fatalf("reports = %v, want 2", reports)
	}
	if reports[1] != (detailReport{2, 10}) {
		t.Errorf("second report = %v, want {2 10}", reports[1])
	}
}

// Adversarial: under concurrent Inc() calls, positions must reach the sink in
// increasing order. The old implementation released its mutex before emit, so
// a slower first emit could land after a faster second one.
func TestCounterEmitsInOrderUnderConcurrency(t *testing.T) {
	var mu sync.Mutex
	var reports []detailReport
	firstEmitStarted := make(chan struct{})
	secondEmitted := make(chan struct{})
	releaseFirst := make(chan struct{})
	ctx := agentruntime.WithProgressDetailCallback(t.Context(), func(_ string, d agentruntime.ProgressDetail) {
		switch d.Current {
		case 1:
			close(firstEmitStarted)
			<-releaseFirst
		case 2:
			close(secondEmitted)
		}
		mu.Lock()
		reports = append(reports, detailReport{d.Current, d.Total})
		mu.Unlock()
	})
	ctx = agentruntime.BindComponentDetail(ctx, "test-component")

	c := NewCounter(ctx, "unit", 2, Window{Span: 1})
	doneFirst := make(chan struct{})
	doneSecond := make(chan struct{})
	go func() { c.Inc(); close(doneFirst) }()
	<-firstEmitStarted
	go func() { c.Inc(); close(doneSecond) }()

	// With emit outside the lock the second report lands while the first is
	// blocked. With emit inside the lock the second Inc cannot progress.
	select {
	case <-secondEmitted:
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	<-doneFirst
	<-doneSecond

	mu.Lock()
	defer mu.Unlock()
	if len(reports) != 3 {
		t.Fatalf("reports = %v, want 3 (initial 0/N + two increments)", reports)
	}
	if reports[0].current != 0 || reports[1].current != 1 || reports[2].current != 2 {
		t.Errorf("reports = %v, want positions 0, 1, 2 in order", reports)
	}
}

// The Window reuse keeps its cancellation guard: a cancelled run emits no
// fraction, while the initial detail position is still established at boot.
func TestCounterWindowCancellationGuard(t *testing.T) {
	var reports []detailReport
	var fractions []float64
	ctx, cancel := context.WithCancel(t.Context())
	ctx = agentruntime.WithProgressDetailCallback(ctx, func(_ string, d agentruntime.ProgressDetail) {
		reports = append(reports, detailReport{d.Current, d.Total})
	})
	ctx = agentruntime.WithProgressFractionCallback(ctx, func(_ string, f float64) {
		fractions = append(fractions, f)
	})
	ctx = agentruntime.BindComponentDetail(ctx, "test-component")
	ctx = agentruntime.BindComponentFraction(ctx, "test-component")

	w := Window{Span: 1}
	cancel()
	c := NewCounter(ctx, "unit", 3, w)
	c.Inc()
	c.Finish()

	if len(fractions) != 0 {
		t.Errorf("cancelled window must emit no fractions, got %v", fractions)
	}
	if len(reports) != 1 {
		t.Errorf("initial detail must still be emitted, got %v", reports)
	}
}
