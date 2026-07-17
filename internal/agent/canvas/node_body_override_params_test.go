package canvas

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

type stubComponent struct {
	params map[string]any
}

func (s *stubComponent) Invoke(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}

// TestBuildNodeBody_OverrideParams asserts that a run-level override
// (threaded via ctx, keyed by cpnID) is merged into the component's
// params before the factory is called. Only the entry for the
// component's own cpnID applies. The merge is shallow: a top-level key
// present in the override fully replaces the base entry for that key
// (no inner deep-merge), while base keys absent from the override survive.
func TestBuildNodeBody_OverrideParams(t *testing.T) {
	captured := map[string]any{}
	factory := func(name string, params map[string]any) (runtime.Component, error) {
		cp := make(map[string]any, len(params))
		for k, v := range params {
			cp[k] = v
		}
		captured = cp
		return &stubComponent{params: cp}, nil
	}

	baseParams := map[string]any{
		"pdf": map[string]any{
			"output_format": "one",
			"parse_method":  "naive",
		},
		"doc": map[string]any{
			"output_format": "one",
		},
	}
	// Run-level override is keyed by cpnID. For "cpn-parser": the whole
	// "pdf" entry is replaced (parse_method is dropped), and a new "docx"
	// entry is injected. "doc" is untouched because it is absent from the
	// override. The entry for a different cpnID must not leak in.
	override := map[string]any{
		"cpn-parser": map[string]any{
			"pdf": map[string]any{
				"output_format": "detailed",
			},
			"docx": map[string]any{
				"output_format": "one",
			},
		},
		"cpn-other": map[string]any{
			"pdf": map[string]any{
				"output_format": "should-not-apply",
			},
		},
	}

	ctx := WithComponentFactory(context.Background(), factory)
	ctx = withOverrideParams(ctx, override)

	body, err := buildNodeBody(ctx, "cpn-parser", "Parser", baseParams)
	if err != nil {
		t.Fatalf("buildNodeBody: %v", err)
	}
	if _, err := body(context.Background(), map[string]any{"x": 1}); err != nil {
		t.Fatalf("body: %v", err)
	}

	// doc is untouched (absent from the override).
	if v, _ := captured["doc"].(map[string]any); v == nil || v["output_format"] != "one" {
		t.Errorf("doc should be preserved from base: %#v", captured["doc"])
	}
	// docx injected by the override (was absent in base).
	if v, _ := captured["docx"].(map[string]any); v == nil || v["output_format"] != "one" {
		t.Errorf("docx injection missing: %#v", captured["docx"])
	}
	// pdf: the override fully replaces the base entry, so output_format is
	// overridden and the base-only parse_method is dropped (shallow merge).
	pdf, _ := captured["pdf"].(map[string]any)
	if pdf == nil {
		t.Fatalf("pdf setup missing: %#v", captured)
	}
	if pdf["output_format"] != "detailed" {
		t.Errorf("pdf.output_format not overridden: %#v", pdf)
	}
	if _, ok := pdf["parse_method"]; ok {
		t.Errorf("pdf.parse_method should be dropped by shallow merge: %#v", pdf)
	}
}

func TestBuildNodeBody_OverrideParamsNilIsNoOp(t *testing.T) {
	captured := map[string]any{}
	factory := func(name string, params map[string]any) (runtime.Component, error) {
		cp := make(map[string]any, len(params))
		for k, v := range params {
			cp[k] = v
		}
		captured = cp
		return &stubComponent{params: cp}, nil
	}
	baseParams := map[string]any{"name": "x"}

	ctx := WithComponentFactory(context.Background(), factory)
	body, err := buildNodeBody(ctx, "cpn", "Parser", baseParams)
	if err != nil {
		t.Fatalf("buildNodeBody: %v", err)
	}
	if _, err := body(context.Background(), map[string]any{}); err != nil {
		t.Fatalf("body: %v", err)
	}
	if captured["name"] != "x" {
		t.Errorf("base params should be preserved: %#v", captured)
	}
}
