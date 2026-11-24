// Page-progress wiring tests pin the DeepDOC → component fraction bridge:
// a PDF parse reports done/total pages through the framework-bound fraction
// channel, and every other parser family stays untouched.

package component

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
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
	attachPDFPageProgress(ctx, pdfParser, progress.Window{Base: 0, Span: 1})
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
	attachPDFPageProgress(ctx, pdfParser, progress.Window{Base: 0, Span: 1})
	pdfParser.OnPageDone(1, 0)

	if len(seen) != 0 {
		t.Errorf("expected no fraction for total=0, got %v", seen)
	}
}

func TestAttachPDFPageProgress_LeavesOtherParsersAlone(t *testing.T) {
	var seen []fractionObservation
	ctx := boundFractionContext(t, "Parser:txt", &seen)

	attachPDFPageProgress(ctx, parser.NewTextParser(), progress.Window{Base: 0, Span: 1})

	if len(seen) != 0 {
		t.Errorf("expected no fraction from a non-PDF parser, got %v", seen)
	}
}

func TestAttachPDFPageProgress_ZeroWindowSilent(t *testing.T) {
	var seen []fractionObservation
	ctx := boundFractionContext(t, "Parser:pdf", &seen)

	pdfParser := parser.NewPDFParser()
	attachPDFPageProgress(ctx, pdfParser, progress.Window{})
	if pdfParser.OnPageDone != nil {
		t.Fatal("expected OnPageDone to remain nil for zero window")
	}
	if len(seen) != 0 {
		t.Errorf("expected no observations for zero window, got %v", seen)
	}
}

func TestAttachPDFPageProgress_ReportsFractionInWindow(t *testing.T) {
	var seen []fractionObservation
	ctx := boundFractionContext(t, "Parser:pdf", &seen)

	pdfParser := parser.NewPDFParser()
	attachPDFPageProgress(ctx, pdfParser, progress.Window{Base: 0.1, Span: 0.4})
	if pdfParser.OnPageDone == nil {
		t.Fatal("expected OnPageDone to be wired")
	}

	pdfParser.OnPageDone(3, 12)  // 0.25 -> 0.1 + 0.4*0.25 = 0.20
	pdfParser.OnPageDone(12, 12) // 1.0  -> 0.1 + 0.4*1.0  = 0.50

	want := []fractionObservation{{"Parser:pdf", 0.2}, {"Parser:pdf", 0.5}}
	if len(seen) != len(want) {
		t.Fatalf("observations = %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Errorf("observation %d = %v, want %v", i, seen[i], want[i])
		}
	}
}

func TestPDFPageProgressSupported(t *testing.T) {
	tests := []struct {
		fileType utility.FileType
		method   string
		want     bool
	}{
		{utility.FileTypePDF, "", true},
		{utility.FileTypePDF, "deepdoc", true},
		{utility.FileTypePDF, "mineru", false},
		{utility.FileTypePDF, "paddleocr", false},
		{utility.FileTypePDF, "docling", false},
		{utility.FileTypePDF, "plain_text", false},
		{utility.FileTypeDOCX, "deepdoc", false},
		{utility.FileTypeMarkdown, "", false},
	}
	for _, tc := range tests {
		setups := map[string]schema.ParserSetup{
			"pdf": {"parse_method": tc.method},
		}
		if got := pdfPageProgressSupported(tc.fileType, setups); got != tc.want {
			t.Errorf("pdfPageProgressSupported(%s, method=%s) = %v, want %v", tc.fileType, tc.method, got, tc.want)
		}
	}
}

// TestPDFPageProgressSupported_RegisteredAnalyzer reports the RED regression
// in an edition-neutral way: any parse method backed by a registered external
// DeepDoc analyzer runs the per-page worker loop and must report page progress,
// while whole-document engines and unregistered methods must not.
func TestPDFPageProgressSupported_RegisteredAnalyzer(t *testing.T) {
	const probe = "probe_progress_analyzer"
	// Registration is process-local to this test binary and the method name is
	// unique, so the throwaway entry cannot collide with other cases.
	parser.RegisterAnalyzerFactory(probe, func(string) (deepdoctype.DocAnalyzer, error) { return nil, nil })

	tests := []struct {
		name   string
		method string
		want   bool
	}{
		{"registered analyzer", probe, true},
		{"registered analyzer normalized", "  PROBE_PROGRESS_ANALYZER  ", true},
		{"whole-document mineru stays off", "mineru", false},
		{"unregistered method stays off", "never_registered_progress_analyzer", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setups := map[string]schema.ParserSetup{
				"pdf": {"parse_method": tc.method},
			}
			if got := pdfPageProgressSupported(utility.FileTypePDF, setups); got != tc.want {
				t.Errorf("pdfPageProgressSupported(pdf, method=%q) = %v, want %v", tc.method, got, tc.want)
			}
		})
	}
}

// detailObservation captures a structured progress detail report.
type detailObservation struct {
	component string
	unit      string
	current   uint32
	total     uint32
}

// boundDetailContext mirrors the canvas framework for both fraction and detail
// channels, so OnPageDone can be asserted on both.
func boundDetailContext(t *testing.T, component string, fracs *[]fractionObservation, details *[]detailObservation) context.Context {
	t.Helper()
	ctx := runtime.WithProgressFractionCallback(t.Context(), func(c string, f float64) {
		*fracs = append(*fracs, fractionObservation{c, f})
	})
	ctx = runtime.WithProgressDetailCallback(ctx, func(c string, d runtime.ProgressDetail) {
		*details = append(*details, detailObservation{c, d.Unit, d.Current, d.Total})
	})
	ctx = runtime.BindComponentFraction(ctx, component)
	return runtime.BindComponentDetail(ctx, component)
}

// TestAttachPDFPageProgress_ReportsPageDetail is the adversarial fixture for
// review finding ③: each completed page must emit a structured page detail
// (unit=page, current=done, total=total), not just a 0..1 fraction. Without the
// detail report this test sees none and fails.
func TestAttachPDFPageProgress_ReportsPageDetail(t *testing.T) {
	var fracs []fractionObservation
	var details []detailObservation
	ctx := boundDetailContext(t, "Parser:pdf", &fracs, &details)

	pdfParser := parser.NewPDFParser()
	attachPDFPageProgress(ctx, pdfParser, progress.Window{Base: 0, Span: 1})
	pdfParser.OnPageDone(3, 12)
	pdfParser.OnPageDone(12, 12)

	want := []detailObservation{
		{"Parser:pdf", "page", 3, 12},
		{"Parser:pdf", "page", 12, 12},
	}
	if len(details) != len(want) {
		t.Fatalf("page details = %v, want %v", details, want)
	}
	for i := range want {
		if details[i] != want[i] {
			t.Errorf("detail %d = %v, want %v", i, details[i], want[i])
		}
	}
}

// TestAttachPDFPageProgress_NoDetailForUnknownTotal is the adversarial edge:
// total=0 must produce neither fraction nor detail (no division, no bogus row).
func TestAttachPDFPageProgress_NoDetailForUnknownTotal(t *testing.T) {
	var fracs []fractionObservation
	var details []detailObservation
	ctx := boundDetailContext(t, "Parser:pdf", &fracs, &details)

	pdfParser := parser.NewPDFParser()
	attachPDFPageProgress(ctx, pdfParser, progress.Window{Base: 0, Span: 1})
	pdfParser.OnPageDone(1, 0)

	if len(details) != 0 || len(fracs) != 0 {
		t.Errorf("total=0 should emit nothing, got details=%v fracs=%v", details, fracs)
	}
}
