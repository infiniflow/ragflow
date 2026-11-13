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

	parserComponent, ok := components["Parser:HipSignsRhyme"].(map[string]interface{})
	if !ok {
		t.Fatal("general DSL has no Parser:HipSignsRhyme component")
	}
	parserObj, ok := parserComponent["obj"].(map[string]interface{})
	if !ok {
		t.Fatal("general parser has no object definition")
	}
	parserParams, ok := parserObj["params"].(map[string]interface{})
	if !ok {
		t.Fatal("general parser has no params")
	}
	if _, ok := parserParams["outputs"]; ok {
		t.Fatal("general parser retains obsolete html/markdown/text output declarations")
	}
	if _, ok := parserParams["spreadsheet"].(map[string]interface{}); !ok {
		t.Fatal("general parser has no spreadsheet setup")
	}
	image, ok := parserParams["image"].(map[string]interface{})
	if !ok {
		t.Fatal("general parser has no image setup")
	}
	if got := image["ocr_enabled"]; got != true {
		t.Fatalf("general image ocr_enabled = %v, want true", got)
	}
	email, ok := parserParams["email"].(map[string]interface{})
	if !ok {
		t.Fatal("general parser has no email setup")
	}
	if _, ok := email["fields"]; !ok {
		t.Fatal("general email setup has no fields")
	}
}

func TestGeneralTemplateParserGraphUsesJSONContract(t *testing.T) {
	dsl, err := LoadBuiltinDSL("general")
	if err != nil {
		t.Fatalf("LoadBuiltinDSL: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(dsl), &root); err != nil {
		t.Fatalf("decode general DSL: %v", err)
	}
	graph, ok := root["graph"].(map[string]interface{})
	if !ok {
		t.Fatal("general DSL has no graph")
	}
	nodes, ok := graph["nodes"].([]interface{})
	if !ok {
		t.Fatal("general graph has no nodes")
	}
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]interface{})
		if !ok || node["id"] != "Parser:HipSignsRhyme" {
			continue
		}
		data, ok := node["data"].(map[string]interface{})
		if !ok {
			t.Fatal("general parser graph node has no data")
		}
		form, ok := data["form"].(map[string]interface{})
		if !ok {
			t.Fatal("general parser graph node has no form")
		}
		if _, ok := form["outputs"]; ok {
			t.Fatal("general parser graph form retains obsolete output declarations")
		}
		setups, ok := form["setups"].([]interface{})
		if !ok {
			t.Fatal("general parser graph form has no setups")
		}
		seen := map[string]bool{}
		for _, rawSetup := range setups {
			setup, ok := rawSetup.(map[string]interface{})
			if !ok {
				continue
			}
			format, _ := setup["fileFormat"].(string)
			seen[format] = true
		}
		for _, want := range []string{"spreadsheet", "image", "email"} {
			if !seen[want] {
				t.Fatalf("general parser graph form has no %s setup", want)
			}
		}
		return
	}
	t.Fatal("general graph has no Parser:HipSignsRhyme node")
}
