//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package canvas

import (
	"context"
	"testing"

	"ragflow/internal/agent/runtime"
)

// TestCompile_OverrideParams exercises the full canvas-level wiring:
//
//	canvas.Compile(ctx, dsl, WithSetupOverrides(override))
//
// threads the cpnID-keyed override through ctx into
// BuildWorkflow → buildNodeBody → applyOverrideParams → mergeSetups, so
// each component's factory receives its own merged params["setups"]. Only
// the entry for a component's own cpnID applies; components absent from the
// override map keep their base params (no spurious "setups" injected).
func TestCompile_OverrideParams(t *testing.T) {
	captured := map[string]map[string]any{} // component_name -> params received by factory
	factory := func(name string, params map[string]any) (runtime.Component, error) {
		// Deep-shallow copy so later mutations don't hide what the
		// factory actually received.
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
						"setups": map[string]any{
							"pdf": map[string]any{
								"output_format": "one",
								"parse_method":  "naive",
							},
							"doc": map[string]any{
								"output_format": "one",
							},
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

	// Override is keyed by cpnID. Only "parser_0" is present, so its
	// "pdf" entry is fully replaced (parse_method dropped) and "docx" is
	// injected; "doc" survives from the base. "sink_0" is absent and must
	// keep its base params untouched.
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
	if _, err := Compile(ctx, dsl, WithSetupOverrides(override)); err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// parser_0: override merged into params["setups"] before the factory.
	got, ok := captured["Parser"]["setups"].(map[string]any)
	if !ok {
		t.Fatalf("Parser factory did not receive a setups map: %#v", captured["Parser"])
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

	// sink_0: absent from the override → no setups key injected.
	if _, ok := captured["Sink"]["setups"]; ok {
		t.Errorf("Sink should not receive a setups key: %#v", captured["Sink"])
	}
}
