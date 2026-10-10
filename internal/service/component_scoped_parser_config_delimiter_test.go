package service

import (
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

func TestApplyComponentScopedParserConfig_BridgesFlatDelimiter(t *testing.T) {
	parserConfig := entity.JSONMap{
		"delimiter": "`问：`",
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "llm-default")

	if _, ok := got["delimiter"]; ok {
		t.Fatal("flat delimiter key should be removed")
	}
	chunker, ok := got[generalChunkerNodeID].(map[string]any)
	if !ok {
		t.Fatalf("missing %s node", generalChunkerNodeID)
	}
	if chunker["delimiter"] != "`问：`" {
		t.Fatalf("delimiter = %#v, want `问：`", chunker["delimiter"])
	}
}

func TestApplyComponentScopedParserConfig_BridgesFlatDelimitersArray(t *testing.T) {
	want := []any{"`问：`", "\n"}
	parserConfig := entity.JSONMap{
		"delimiters": want,
		"GeneralChunker:SixApplesFall": map[string]any{
			"chunk_token_size": 256,
		},
	}

	got := ApplyComponentScopedParserConfig(parserConfig, "llm-default")

	if _, ok := got["delimiters"]; ok {
		t.Fatal("flat delimiters key should be removed")
	}
	chunker := got[generalChunkerNodeID].(map[string]any)
	if chunker["chunk_token_size"] != 256 {
		t.Fatalf("chunk_token_size = %#v", chunker["chunk_token_size"])
	}
	if !reflect.DeepEqual(chunker["delimiters"], want) {
		t.Fatalf("delimiters = %#v, want %#v", chunker["delimiters"], want)
	}
}
