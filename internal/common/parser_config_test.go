package common

import (
	"testing"
)

func TestDeepMergeMaps_Nil(t *testing.T) {
	result := DeepMergeMaps(nil, nil)
	if result == nil {
		t.Fatal("expected non-nil for nil inputs")
	}
}

func TestDeepMergeMaps_Override(t *testing.T) {
	base := map[string]interface{}{
		"a": 1,
		"b": "hello",
	}
	override := map[string]interface{}{
		"a": 2,
		"c": "world",
	}
	result := DeepMergeMaps(base, override)
	if result["a"] != 2 {
		t.Fatalf("expected a=2, got %v", result["a"])
	}
	if result["b"] != "hello" {
		t.Fatalf("expected b=hello, got %v", result["b"])
	}
	if result["c"] != "world" {
		t.Fatalf("expected c=world, got %v", result["c"])
	}
}

func TestDeepMergeMaps_Nested(t *testing.T) {
	base := map[string]interface{}{
		"parser": map[string]interface{}{
			"lang":  "en",
			"model": "gpt4",
		},
	}
	override := map[string]interface{}{
		"parser": map[string]interface{}{
			"lang": "zh",
		},
	}
	result := DeepMergeMaps(base, override)
	parser, _ := result["parser"].(map[string]interface{})
	if parser["lang"] != "zh" {
		t.Fatalf("expected lang=zh, got %v", parser["lang"])
	}
	if parser["model"] != "gpt4" {
		t.Fatalf("expected model=gpt4, got %v", parser["model"])
	}
}

func TestGetParserConfig_Naive(t *testing.T) {
	result := GetParserConfig("naive", nil)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["chunk_token_num"] != 512 {
		t.Fatalf("expected chunk_token_num=512, got %v", result["chunk_token_num"])
	}
	if result["layout_recognize"] != "DeepDOC" {
		t.Fatalf("expected layout_recognize=DeepDOC, got %v", result["layout_recognize"])
	}
}

func TestGetParserConfig_QA(t *testing.T) {
	result := GetParserConfig("qa", nil)
	if result == nil {
		t.Fatal("expected non-nil")
	}
}

func TestGetParserConfig_Override(t *testing.T) {
	override := map[string]interface{}{
		"chunk_token_num": 256,
	}
	result := GetParserConfig("naive", override)
	if result["chunk_token_num"] != 256 {
		t.Fatalf("expected chunk_token_num=256, got %v", result["chunk_token_num"])
	}
	if result["layout_recognize"] != "DeepDOC" {
		t.Fatalf("expected layout_recognize preserved, got %v", result["layout_recognize"])
	}
}

func TestExtractPipelineDefaults_Nil(t *testing.T) {
	if result := ExtractPipelineDefaults(nil); result != nil {
		t.Fatalf("expected nil for nil input, got %v", result)
	}
}

func TestExtractPipelineDefaults_Basic(t *testing.T) {
	dsl := map[string]interface{}{
		"components": map[string]interface{}{
			"Parser:abc": map[string]interface{}{
				"obj": map[string]interface{}{
					"component_name": "Parser",
					"params": map[string]interface{}{
						"pdf": map[string]interface{}{
							"parse_method": "DeepDOC",
							"lang":         "en",
						},
					},
				},
			},
		},
	}
	result := ExtractPipelineDefaults(dsl)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	params, ok := result["Parser:abc"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Parser:abc in result")
	}
	pdf, _ := params["pdf"].(map[string]interface{})
	if pdf["parse_method"] != "DeepDOC" {
		t.Fatalf("expected parse_method=DeepDOC, got %v", pdf["parse_method"])
	}
}

func TestExtractPipelineDefaults_SkipsFile(t *testing.T) {
	dsl := map[string]interface{}{
		"components": map[string]interface{}{
			"File:xyz": map[string]interface{}{
				"obj": map[string]interface{}{
					"component_name": "File",
					"params": map[string]interface{}{
						"path": "/tmp/test",
					},
				},
			},
		},
	}
	result := ExtractPipelineDefaults(dsl)
	if result != nil {
		t.Fatal("expected nil when only File component exists")
	}
}

func TestExtractPipelineDefaults_StripsOutputs(t *testing.T) {
	dsl := map[string]interface{}{
		"components": map[string]interface{}{
			"Tokenizer:abc": map[string]interface{}{
				"obj": map[string]interface{}{
					"component_name": "Tokenizer",
					"params": map[string]interface{}{
						"fields":        "text",
						"search_method": []interface{}{"embedding"},
						"outputs":       map[string]interface{}{"chunks": "data"},
					},
				},
			},
		},
	}
	result := ExtractPipelineDefaults(dsl)
	params := result["Tokenizer:abc"].(map[string]interface{})
	if _, ok := params["outputs"]; ok {
		t.Fatal("expected outputs to be stripped")
	}
	if params["fields"] != "text" {
		t.Fatalf("expected fields=text, got %v", params["fields"])
	}
}

func TestExtractPipelineDefaults_WithDSLWrapper(t *testing.T) {
	dsl := map[string]interface{}{
		"dsl": map[string]interface{}{
			"components": map[string]interface{}{
				"Extractor:xyz": map[string]interface{}{
					"obj": map[string]interface{}{
						"component_name": "Extractor",
						"params": map[string]interface{}{
							"auto_keywords":  float64(5),
							"auto_questions": float64(3),
						},
					},
				},
			},
		},
	}
	result := ExtractPipelineDefaults(dsl)
	raw := result["Extractor:xyz"].(map[string]interface{})
	if raw["auto_keywords"] != float64(5) {
		t.Fatalf("expected auto_keywords=5, got %v", raw["auto_keywords"])
	}
	if raw["auto_questions"] != float64(3) {
		t.Fatalf("expected auto_questions=3, got %v", raw["auto_questions"])
	}
}

func TestExtractPipelineDefaults_Delimiters(t *testing.T) {
	dsl := map[string]interface{}{
		"components": map[string]interface{}{
			"TokenChunker:abc": map[string]interface{}{
				"obj": map[string]interface{}{
					"component_name": "TokenChunker",
					"params": map[string]interface{}{
						"delimiters": []interface{}{"\n", ".", " "},
					},
				},
			},
		},
	}
	result := ExtractPipelineDefaults(dsl)
	raw := result["TokenChunker:abc"].(map[string]interface{})
	delims, _ := raw["delimiters"].([]interface{})
	if len(delims) != 3 {
		t.Fatalf("expected 3 delimiters, got %v", delims)
	}
}
