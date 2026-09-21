package pdf

import (
	"testing"

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
