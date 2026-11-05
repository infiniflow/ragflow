package component

import (
	"context"
	"errors"
	"math"
	"sort"
	"testing"

	"ragflow/internal/dao"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/progress"
	"ragflow/internal/parser/parser"
	"ragflow/internal/utility"
)

func TestVisionEnhancement_ReportsPerFigureFractions(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Parser:vision")

	png := visionTestPNGBase64(t)
	dispatched := parser.ParseResult{
		JSON: parser.MapsToItems([]map[string]any{
			{"text": "1", "image": png, "doc_type_kwd": "image"},
			{"text": "2", "image": png, "doc_type_kwd": "image"},
			{"text": "3", "image": png, "doc_type_kwd": "image"},
			{"text": "4", "image": png, "doc_type_kwd": "image"},
		}),
	}

	win := progress.Window{Base: 0.5, Span: 0.5}
	_, handled, err := maybeDispatchVisionEnhancement(
		ctx, dao.DB, utility.FileTypeXLSX, dispatched,
		map[string]any{"tenant_id": "t1"},
		visionSettings{},
		win,
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}

	fractions := rec.fractions()
	if len(fractions) != 5 {
		t.Fatalf("recorded fractions count = %d, want 5 (4 items + 1 close); got %v", len(fractions), fractions)
	}
	if last := fractions[len(fractions)-1]; last != 1.0 {
		t.Errorf("last fraction = %f, want 1.0", last)
	}

	// Verify the multiset of reported fractions regardless of concurrent arrival order.
	sorted := make([]float64, len(fractions))
	copy(sorted, fractions)
	sort.Float64s(sorted)

	wantFractions := []float64{0.625, 0.75, 0.875, 1.0, 1.0}
	for i, want := range wantFractions {
		if math.Abs(sorted[i]-want) > 1e-6 {
			t.Errorf("sorted fraction[%d] = %f, want %f", i, sorted[i], want)
		}
	}
}

func TestVisionEnhancement_SettledFractionsCountFailedFigures(t *testing.T) {
	// One valid image, one invalid image that fails payload validation before VLM call,
	// and one where the VLM invoker returns error.
	swapVisionGlobals(t, fakeResolver, func(_ context.Context, _ modelModule.ModelDriver, _ string, _ []modelModule.Message, _ *modelModule.APIConfig) (*modelModule.ChatResponse, error) {
		return nil, errors.New("invoker error")
	}, fakePrompt)

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Parser:vision")

	dispatched := parser.ParseResult{
		JSON: parser.MapsToItems([]map[string]any{
			{"text": "valid", "image": visionTestPNGBase64(t), "doc_type_kwd": "image"},
			{"text": "invalid", "image": "not-valid-base64", "doc_type_kwd": "image"},
		}),
	}

	win := progress.Window{Base: 0.2, Span: 0.8}
	_, _, err := maybeDispatchVisionEnhancement(
		ctx, dao.DB, utility.FileTypeXLSX, dispatched,
		map[string]any{"tenant_id": "t1"},
		visionSettings{},
		win,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fractions := rec.fractions()
	// 2 items + 1 close = 3 reports
	if len(fractions) != 3 {
		t.Fatalf("recorded fractions count = %d, want 3; got %v", len(fractions), fractions)
	}
	if last := fractions[len(fractions)-1]; last != 1.0 {
		t.Errorf("last fraction = %f, want 1.0", last)
	}

	sorted := make([]float64, len(fractions))
	copy(sorted, fractions)
	sort.Float64s(sorted)

	wantFractions := []float64{0.6, 1.0, 1.0}
	for i, want := range wantFractions {
		if math.Abs(sorted[i]-want) > 1e-6 {
			t.Errorf("sorted fraction[%d] = %f, want %f", i, sorted[i], want)
		}
	}
}

func TestVisionEnhancement_ZeroWindowReportsNothing(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Parser:vision")

	dispatched := parser.ParseResult{
		JSON: parser.MapsToItems([]map[string]any{
			{"text": "1", "image": visionTestPNGBase64(t), "doc_type_kwd": "image"},
		}),
	}

	// Zero window (default when unpassed or inactive)
	_, handled, err := maybeDispatchVisionEnhancement(
		ctx, dao.DB, utility.FileTypeXLSX, dispatched,
		map[string]any{"tenant_id": "t1"},
		visionSettings{},
		progress.Window{},
	)
	if err != nil {
		t.Fatalf("maybeDispatchVisionEnhancement: %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}

	if len(rec.fractions()) != 0 {
		t.Errorf("expected 0 fractions for zero window, got %v", rec.fractions())
	}
}

func TestVisionEnhancement_CancelledContextReportsNothing(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

	rec := &fractionRecorder{}
	ctx, cancel := context.WithCancel(rec.context(t, "Parser:vision"))
	cancel() // Cancel context before invocation

	png := visionTestPNGBase64(t)
	dispatched := parser.ParseResult{
		JSON: parser.MapsToItems([]map[string]any{
			{"text": "1", "image": png, "doc_type_kwd": "image"},
			{"text": "2", "image": png, "doc_type_kwd": "image"},
		}),
	}

	win := progress.Window{Base: 0.5, Span: 0.5}
	_, _, _ = maybeDispatchVisionEnhancement(
		ctx, dao.DB, utility.FileTypeXLSX, dispatched,
		map[string]any{"tenant_id": "t1"},
		visionSettings{},
		win,
	)

	if len(rec.fractions()) != 0 {
		t.Errorf("cancelled context must not report progress, got: %v", rec.fractions())
	}
}
