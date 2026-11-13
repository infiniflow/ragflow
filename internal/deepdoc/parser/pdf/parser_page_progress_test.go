package pdf

import (
	"image"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestParseRaw_OnPageDone_ReportsEveryPage verifies the page callback fires
// once per collected page with a monotonically increasing done count and the
// total number of submitted pages.
func TestParseRaw_OnPageDone_ReportsEveryPage(t *testing.T) {
	eng := makePageTaggedEngine(6)

	type observation struct{ done, total int }
	var seen []observation
	cfg := pdf.DefaultParserConfig()
	cfg.OnPageDone = func(done, total int) {
		seen = append(seen, observation{done, total})
	}
	p := NewParser(cfg)

	if _, err := p.ParseRaw(t.Context(), eng, &MockDocAnalyzer{Healthy: true}); err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}

	if len(seen) != 6 {
		t.Fatalf("expected 6 callbacks, got %d: %v", len(seen), seen)
	}
	for i, obs := range seen {
		if obs.done != i+1 {
			t.Errorf("callback %d: done = %d, want %d", i, obs.done, i+1)
		}
		if obs.total != 6 {
			t.Errorf("callback %d: total = %d, want 6", i, obs.total)
		}
	}
}

// TestParseRaw_OnPageDone_TotalIsSubmittedPages verifies the reported total
// reflects the page-range restriction rather than the document page count, so
// a caller computing done/total reaches 1.0 when the run finishes.
func TestParseRaw_OnPageDone_TotalIsSubmittedPages(t *testing.T) {
	eng := makePageTaggedEngine(10)

	var totals []int
	cfg := pdf.DefaultParserConfig()
	cfg.Pages = [][]int{{1, 3}}
	cfg.OnPageDone = func(_, total int) {
		totals = append(totals, total)
	}
	p := NewParser(cfg)

	if _, err := p.ParseRaw(t.Context(), eng, &MockDocAnalyzer{Healthy: true}); err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}

	if len(totals) != 3 {
		t.Fatalf("expected 3 callbacks, got %d: %v", len(totals), totals)
	}
	for i, total := range totals {
		if total != 3 {
			t.Errorf("callback %d: total = %d, want 3", i, total)
		}
	}
}

// TestParseRaw_OnPageDone_NilIsNoop is the regression guard: an unset callback
// must not change parsing.
func TestParseRaw_OnPageDone_NilIsNoop(t *testing.T) {
	eng := makePageTaggedEngine(4)
	p := NewParser(pdf.DefaultParserConfig())

	result, err := p.ParseRaw(t.Context(), eng, &MockDocAnalyzer{Healthy: true})
	if err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if len(result.PageHeight) != 4 {
		t.Errorf("expected 4 parsed pages, got %d", len(result.PageHeight))
	}
}

// TestParseRaw_OnPageDone_FiresBeforeSubmissionDrains is the reporting-lag
// regression guard. It parses more pages than the worker pool can ever hold
// (max 12 workers + 4x queue = 60), so a callback driven by the collection
// loop could not run until the whole document had been submitted — every
// non-first page here blocks its worker until the first page has been
// reported, which a collector-side callback only reaches after they give up.
func TestParseRaw_OnPageDone_FiresBeforeSubmissionDrains(t *testing.T) {
	const numPages = 256

	firstDone := make(chan struct{})
	giveUp := make(chan struct{})
	var firstOnce, timeoutOnce sync.Once
	var late atomic.Bool

	eng := makePageTaggedEngine(numPages)
	eng.RenderPageImageFunc = func(pg int, _ float64) (image.Image, error) {
		if pg != 0 {
			select {
			case <-firstDone:
			case <-giveUp:
			case <-time.After(5 * time.Second):
				late.Store(true)
				timeoutOnce.Do(func() { close(giveUp) })
			}
		}
		return image.NewRGBA(image.Rect(0, 0, eng.RenderW, eng.RenderH)), nil
	}

	cfg := pdf.DefaultParserConfig()
	cfg.OnPageDone = func(done, _ int) {
		if done == 1 {
			firstOnce.Do(func() { close(firstDone) })
		}
	}
	p := NewParser(cfg)

	if _, err := p.ParseRaw(t.Context(), eng, &MockDocAnalyzer{Healthy: true}); err != nil {
		t.Fatalf("ParseRaw: %v", err)
	}
	if late.Load() {
		t.Error("first page completion was reported only after other pages gave up waiting: OnPageDone is not firing when the page finishes")
	}
}
