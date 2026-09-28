package chunker

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/parser/parser"
)

type generalTextPythonGolden struct {
	Input  string         `json:"input"`
	Params map[string]any `json:"params"`
	Chunks []struct {
		Text string `json:"text"`
	} `json:"chunks"`
}

// TestGeneralChunkerTextPythonParity verifies the post-refactor boundary:
// Python's built-in general path parses and chunks text in one operation,
// whereas Go sends the full normalized TextParser result to GeneralChunker.
// The golden is captured from rag.app.naive.chunk and locks the resulting
// chunk boundaries after removing Python's representation-only leading newline.
func TestGeneralChunkerTextPythonParity(t *testing.T) {
	raw, err := os.ReadFile("testdata/general/text_default.python.golden.json")
	if err != nil {
		t.Fatalf("read Python golden: %v", err)
	}
	var golden generalTextPythonGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode Python golden: %v", err)
	}

	parsed := parser.NewTextParser().ParseWithResult(
		context.Background(), "general.txt", []byte(golden.Input),
	)
	if parsed.Err != nil {
		t.Fatalf("TextParser.ParseWithResult: %v", parsed.Err)
	}
	component, err := NewGeneralChunker(golden.Params)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "general.txt",
		"file_type":     "txt",
		"output_format": "json",
		"json":          parsed.JSON,
	})
	if err != nil {
		t.Fatalf("GeneralChunker.Invoke: %v", err)
	}

	got := outputTexts(t, out)
	want := make([]string, 0, len(golden.Chunks))
	for _, chunk := range golden.Chunks {
		want = append(want, strings.TrimPrefix(chunk.Text, "\n"))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GeneralChunker text chunks = %#v, want Python %v", got, want)
	}
}
