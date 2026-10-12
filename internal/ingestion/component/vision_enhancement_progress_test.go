package component

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/dao"
	modelModule "ragflow/internal/entity/models"
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
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "1", "image": png, "doc_type_kwd": "image"},
			{"text": "2", "image": png, "doc_type_kwd": "image"},
			{"text": "3", "image": png, "doc_type_kwd": "image"},
			{"text": "4", "image": png, "doc_type_kwd": "image"},
		},
	}

	win := fractionWindow{base: 0.5, span: 0.5}
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
	for i, f := range fractions {
		if f < 0.5 || f > 1.0 {
			t.Errorf("fraction %d = %f, outside window [0.5, 1.0]", i, f)
		}
	}
	if last := fractions[len(fractions)-1]; last != 1.0 {
		t.Errorf("last fraction = %f, want 1.0", last)
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
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "valid", "image": visionTestPNGBase64(t), "doc_type_kwd": "image"},
			{"text": "invalid", "image": "not-valid-base64", "doc_type_kwd": "image"},
		},
	}

	win := fractionWindow{base: 0.2, span: 0.8}
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
}

func TestVisionEnhancement_ZeroWindowReportsNothing(t *testing.T) {
	invoker := &visionEnhanceCaptureInvoker{}
	swapVisionGlobals(t, fakeResolver, invoker.invoke, fakePrompt)

	rec := &fractionRecorder{}
	ctx := rec.context(t, "Parser:vision")

	dispatched := parser.ParseResult{
		OutputFormat: "json",
		JSON: []map[string]any{
			{"text": "1", "image": visionTestPNGBase64(t), "doc_type_kwd": "image"},
		},
	}

	// Zero window (default when unpassed or inactive)
	_, handled, err := maybeDispatchVisionEnhancement(
		ctx, dao.DB, utility.FileTypeXLSX, dispatched,
		map[string]any{"tenant_id": "t1"},
		visionSettings{},
		fractionWindow{},
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
