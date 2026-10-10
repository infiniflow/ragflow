package pipeline

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBuildParserConfigPreservesGlobalVisionSettings(t *testing.T) {
	const id = "Parser:HipSignsRhyme"
	for _, enabled := range []bool{true, false} {
		for _, model := range []string{"model-B", ""} {
			want := map[string]any{"enable_vision_enhancement": enabled, "vlm": map[string]any{"llm_id": model}}
			params := BuildParserConfig(generalDSL(t), map[string]any{id: want})[id].(map[string]any)
			for key, value := range want {
				if !reflect.DeepEqual(params[key], value) {
					t.Fatalf("existing canvas saved %s = %v, want %v", key, params[key], value)
				}
			}
		}
	}
}

func TestBuiltinParserVisionSettings(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.Refs()) == 0 {
		t.Fatal("no builtin templates")
	}
	for _, ref := range registry.Refs() {
		t.Run(ref, func(t *testing.T) {
			template, ok := registry.Get(ref)
			if !ok {
				t.Fatal("missing builtin template")
			}
			dsl, err := json.Marshal(template.DSL)
			if err != nil {
				t.Fatal(err)
			}
			id := ExtractParserCpnID(dsl, "Parser")
			if id == "" {
				t.Fatal("missing Parser")
			}
			defaults, err := ComponentParamsDefaults(dsl)
			if err != nil {
				t.Fatal(err)
			}
			checkDefaults := func(params map[string]any) {
				t.Helper()
				if params["enable_vision_enhancement"] != false || !reflect.DeepEqual(params["vlm"], map[string]any{"llm_id": ""}) {
					t.Errorf("default vision settings = %v / %v", params["enable_vision_enhancement"], params["vlm"])
				}
			}
			checkDefaults(defaults[id])
			parserForms := 0
			graph := template.DSL["graph"].(map[string]any)
			for _, raw := range graph["nodes"].([]any) {
				data := raw.(map[string]any)["data"].(map[string]any)
				if data["label"] == "Parser" {
					checkDefaults(data["form"].(map[string]any))
					parserForms++
				}
			}
			if parserForms == 0 {
				t.Fatal("missing Parser graph form")
			}
			for _, enabled := range []bool{true, false} {
				want := map[string]any{"enable_vision_enhancement": enabled, "vlm": map[string]any{"llm_id": "model-B"}}
				params := BuildParserConfig(dsl, map[string]any{id: want})[id].(map[string]any)
				for key, value := range want {
					if !reflect.DeepEqual(params[key], value) {
						t.Fatalf("saved %s = %v, want %v", key, params[key], value)
					}
				}
			}
		})
	}
}
