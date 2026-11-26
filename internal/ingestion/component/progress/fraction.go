// Package progress maps ingestion subphases into a component's fraction budget.
package progress

import (
	"context"
	"sync/atomic"

	"ragflow/internal/agent/runtime"
)

// Window is the part of a component's 0..1 budget owned by one phase.
// Its zero value disables reporting.
type Window struct {
	Base, Span float64
	// Phase is the unit tag (e.g. "batch:map") carried into the detail
	// channel. Empty keeps the legacy fraction-only behavior.
	Phase string
}

// WithPhase tags a window so its counter reports discrete positions to the
// detail channel as well as fractions.
func (w Window) WithPhase(phase string) Window {
	w.Phase = phase
	return w
}

// Report maps a phase ratio to the component's framework-bound fraction.
func (w Window) Report(ctx context.Context, ratio float64) {
	if w.Span <= 0 || ctx.Err() != nil {
		return
	}
	ratio = min(max(ratio, 0), 1)
	runtime.ReportComponentFraction(ctx, w.Base+w.Span*ratio)
}

// ReportDetail feeds a phase's discrete position into both progress channels:
// the fraction channel (document.progress) and the detail channel (per-unit
// current/total consumed by the ingestion log and progress snapshot). Guards
// mirror Report: a disabled window, cancelled context, empty unit or zero
// total must never emit a position.
func (w Window) ReportDetail(ctx context.Context, unit string, current, total uint32) {
	if w.Span <= 0 || ctx.Err() != nil || unit == "" || total == 0 {
		return
	}
	ratio := float64(current) / float64(total)
	w.Report(ctx, ratio)
	runtime.ReportProgressDetail(ctx, unit, current, total)
}

// Sub divides a window into total equal parts and returns part index.
func (w Window) Sub(index, total int) Window {
	if total <= 0 {
		return Window{}
	}
	span := w.Span / float64(total)
	return Window{Base: w.Base + float64(index)*span, Span: span, Phase: w.Phase}
}

// ReportPosition reports an absolute (current, total) position. A phased
// window also feeds the detail channel; an unphased one keeps the legacy
// fraction-only behavior.
func (w Window) ReportPosition(ctx context.Context, current, total uint32) {
	if total == 0 {
		return
	}
	if w.Phase != "" {
		w.ReportDetail(ctx, w.Phase, current, total)
		return
	}
	w.Report(ctx, float64(current)/float64(total))
}

// Counter returns a concurrency-safe reporter to call after each completed job.
func (w Window) Counter(ctx context.Context, total int) func() {
	var done atomic.Int64
	return func() {
		if total <= 0 {
			return
		}
		w.ReportPosition(ctx, uint32(done.Add(1)), uint32(total))
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
