// Package progress maps ingestion subphases into a component's fraction budget.
package progress

import (
	"context"
	"sync/atomic"

	"ragflow/internal/agent/runtime"
)

// Window is the part of a component's 0..1 budget owned by one phase.
// Its zero value disables reporting.
type Window struct{ Base, Span float64 }

// Report maps a phase ratio to the component's framework-bound fraction.
func (w Window) Report(ctx context.Context, ratio float64) {
	if w.Span <= 0 || ctx.Err() != nil {
		return
	}
	ratio = min(max(ratio, 0), 1)
	runtime.ReportComponentFraction(ctx, w.Base+w.Span*ratio)
}

// Sub divides a window into total equal parts and returns part index.
func (w Window) Sub(index, total int) Window {
	if total <= 0 {
		return Window{}
	}
	span := w.Span / float64(total)
	return Window{Base: w.Base + float64(index)*span, Span: span}
}

// Counter returns a concurrency-safe reporter to call after each completed job.
func (w Window) Counter(ctx context.Context, total int) func() {
	var done atomic.Int64
	return func() {
		if total > 0 {
			w.Report(ctx, float64(done.Add(1))/float64(total))
		}
	}
}

// split hands out one equal window per enabled sequential phase.
type split struct {
	phases int
	next   int
}

func NewSplit(enabled ...bool) *split {
	s := &split{}
	for _, on := range enabled {
		if on {
			s.phases++
		}
	}
	return s
}

func (s *split) Take(enabled bool) Window {
	if !enabled {
		return Window{}
	}
	w := (Window{Span: 1}).Sub(s.next, s.phases)
	s.next++
	return w
}
