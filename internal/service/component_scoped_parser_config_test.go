package service

import (
	"encoding/json"
	"testing"
)

func TestFlatParserConfigOverrides(t *testing.T) {
	dsl, err := json.Marshal(map[string]any{"components": map[string]any{
		"TokenChunker:One": map[string]any{"obj": map[string]any{"component_name": "TokenChunker", "params": map[string]any{"chunk_token_size": 512, "delimiters": []any{"\n"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	got := FlatParserConfigOverrides(dsl, map[string]interface{}{"chunk_token_num": float64(1024), "delimiter": "X"})
	chunker := got["TokenChunker:One"].(map[string]interface{})
	if chunker["chunk_token_size"] != float64(1024) || chunker["delimiters"].([]interface{})[0] != "X" {
		t.Fatalf("got %#v", chunker)
	}
}
