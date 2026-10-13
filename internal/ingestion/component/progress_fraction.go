package component

import (
	"context"

	"ragflow/internal/agent/runtime"
)

// fractionWindow is the slice of a component's 0..1 fraction budget owned by one
// sequential phase. Splitting the budget keeps the reported fraction climbing
// across phases: the run-level progress tracker keeps a high-water mark, so a
// second phase restarting from zero would freeze the bar until the component
// exits.
type fractionWindow struct{ base, span float64 }

// report maps a phase completion ratio into the window and forwards it to the
// run-level fraction channel. The zero window reports nothing, which keeps
// disabled phases free of budget arithmetic.
func (w fractionWindow) report(ctx context.Context, ratio float64) {
	if w.span <= 0 {
		return
	}
	ratio = min(max(ratio, 0), 1)
	runtime.ReportComponentFraction(ctx, w.base+w.span*ratio)
}

// fractionSplit hands out one equal window per enabled phase, in phase order.
type fractionSplit struct {
	phases int
	next   int
}

func newFractionSplit(enabled ...bool) *fractionSplit {
	s := &fractionSplit{}
	for _, on := range enabled {
		if on {
			s.phases++
		}
	}
	return s
}

func (s *fractionSplit) take(enabled bool) fractionWindow {
	if !enabled {
		return fractionWindow{}
	}
	span := 1 / float64(s.phases)
	w := fractionWindow{base: float64(s.next) * span, span: span}
	s.next++
	return w
}
