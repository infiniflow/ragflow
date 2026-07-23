package canvas

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

func TestCompile_OverrideParams(t *testing.T) {
	captured := map[string]map[string]any{} // component_name -> params received by factory
	factory := func(name string, params map[string]any) (runtime.Component, error) {
		cp := make(map[string]any, len(params))
		for k, v := range params {
			cp[k] = v
		}
		captured[name] = cp
		return &stubComponent{params: cp}, nil
	}

	dsl := &Canvas{
		Components: map[string]CanvasComponent{
			"parser_0": {
				Obj: CanvasComponentObj{
					ComponentName: "Parser",
					Params: map[string]any{
						"pdf": map[string]any{
							"output_format": "one",
							"parse_method":  "naive",
						},
						"doc": map[string]any{
							"output_format": "one",
						},
					},
				},
				Upstream:   []string{},
				Downstream: []string{"sink_0"},
			},
			"sink_0": {
				Obj: CanvasComponentObj{
					ComponentName: "Sink",
					Params:        map[string]any{"name": "sink"},
				},
				Upstream:   []string{"parser_0"},
				Downstream: []string{},
			},
		},
		Path: []string{"parser_0", "sink_0"},
	}

	override := map[string]any{
		"parser_0": map[string]any{
			"pdf": map[string]any{
				"output_format": "detailed",
			},
			"docx": map[string]any{
				"output_format": "one",
			},
		},
	}

	ctx := WithComponentFactory(context.Background(), factory)
	if _, err := Compile(ctx, dsl, WithOverrideParams(override)); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	got := captured["Parser"]
	if got == nil {
		t.Fatalf("Parser factory not called")
	}
	// doc preserved from base.
	if v, _ := got["doc"].(map[string]any); v == nil || v["output_format"] != "one" {
		t.Errorf("doc should be preserved from base: %#v", got["doc"])
	}
	// docx injected by the override (was absent in base).
	if v, _ := got["docx"].(map[string]any); v == nil || v["output_format"] != "one" {
		t.Errorf("docx injection missing: %#v", got["docx"])
	}
	// pdf: override fully replaces the base entry (shallow merge).
	pdf, _ := got["pdf"].(map[string]any)
	if pdf == nil {
		t.Fatalf("pdf setup missing: %#v", got)
	}
	if pdf["output_format"] != "detailed" {
		t.Errorf("pdf.output_format not overridden: %#v", pdf)
	}
	if _, ok := pdf["parse_method"]; ok {
		t.Errorf("pdf.parse_method should be dropped by shallow merge: %#v", pdf)
	}

	// sink_0: absent from the override → no file-format keys injected.
	if _, ok := captured["Sink"]["pdf"]; ok {
		t.Errorf("Sink should not have pdf injected: %#v", captured["Sink"])
	}
}
