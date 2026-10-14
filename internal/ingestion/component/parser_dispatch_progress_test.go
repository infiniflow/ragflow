// Page-progress wiring tests pin the DeepDOC → component fraction bridge:
// a PDF parse reports done/total pages through the framework-bound fraction
// channel, and every other parser family stays untouched.

package component

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/parser/parser"
)

type fractionObservation struct {
	component string
	fraction  float64
}

// boundFractionContext mirrors the canvas framework: a run-level callback plus
// the per-node binding that supplies attribution.
func boundFractionContext(t *testing.T, component string, seen *[]fractionObservation) context.Context {
	t.Helper()
	ctx := runtime.WithProgressFractionCallback(t.Context(), func(c string, f float64) {
		*seen = append(*seen, fractionObservation{c, f})
	})
	return runtime.BindComponentFraction(ctx, component)
}

func TestAttachPDFPageProgress_ReportsFraction(t *testing.T) {
	var seen []fractionObservation
	ctx := boundFractionContext(t, "Parser:pdf", &seen)

	pdfParser := parser.NewPDFParser()
	attachPDFPageProgress(ctx, pdfParser)
	if pdfParser.OnPageDone == nil {
		t.Fatal("expected OnPageDone to be wired")
	}

	pdfParser.OnPageDone(3, 12)
	pdfParser.OnPageDone(12, 12)

	want := []fractionObservation{{"Parser:pdf", 0.25}, {"Parser:pdf", 1}}
	if len(seen) != len(want) {
		t.Fatalf("observations = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("observation %d = %v, want %v", i, seen[i], want[i])
		}
	}
}

func TestAttachPDFPageProgress_IgnoresUnknownTotal(t *testing.T) {
	var seen []fractionObservation
	ctx := boundFractionContext(t, "Parser:pdf", &seen)

	pdfParser := parser.NewPDFParser()
	attachPDFPageProgress(ctx, pdfParser)
	pdfParser.OnPageDone(1, 0)

	if len(seen) != 0 {
		t.Errorf("expected no fraction for total=0, got %v", seen)
	}
}

func TestAttachPDFPageProgress_LeavesOtherParsersAlone(t *testing.T) {
	var seen []fractionObservation
	ctx := boundFractionContext(t, "Parser:txt", &seen)

	attachPDFPageProgress(ctx, parser.NewTextParser())

	if len(seen) != 0 {
		t.Errorf("expected no fraction from a non-PDF parser, got %v", seen)
	}
}
