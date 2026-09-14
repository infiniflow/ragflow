package pipeline

import (
	"encoding/json"
	"testing"
)

func TestGeneralTemplateUsesGeneralChunker(t *testing.T) {
	dsl, err := LoadBuiltinDSL("general")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL: %v", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal([]byte(dsl), &root); err != nil {
		t.Fatalf("decode general DSL: %v", err)
	}
	components, ok := root["components"].(map[string]interface{})
	if !ok {
		t.Fatal("general DSL has no components")
	}

	const generalID = "GeneralChunker:SixApplesFall"
	if _, ok := components[generalID]; !ok {
		t.Fatalf("general DSL has no %s component", generalID)
	}
	if _, ok := components["TokenChunker:SixApplesFall"]; ok {
		t.Fatal("general DSL still contains the legacy TokenChunker component")
	}
	component := components[generalID].(map[string]interface{})
	obj := component["obj"].(map[string]interface{})
	if got := obj["component_name"]; got != "GeneralChunker" {
		t.Fatalf("component_name = %v, want GeneralChunker", got)
	}
	params := obj["params"].(map[string]interface{})
	if _, ok := params["delimiter_mode"]; ok {
		t.Fatal("GeneralChunker params contain unsupported delimiter_mode")
	}
	delimiters, ok := params["delimiters"].([]interface{})
	if !ok {
		t.Fatalf("delimiters type = %T", params["delimiters"])
	}
	seenSemicolon := false
	for _, delimiter := range delimiters {
		if delimiter == ";" {
			seenSemicolon = true
		}
	}
	if !seenSemicolon {
		t.Fatalf("general delimiters = %#v, want ASCII semicolon", delimiters)
	}
}
