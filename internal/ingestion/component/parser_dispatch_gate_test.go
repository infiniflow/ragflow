package component

import (
	"errors"
	"testing"

	"ragflow/internal/parser/parser"
)

// TestBuildParserOutputsUsesProduced pins that buildParserOutputs emits the
// structured json output only for a Produced() dispatch result, and otherwise
// takes the raw-text fallback path. This replaces the old
// `Err == nil && OutputFormat != ""` gate, so the boundary behaviour must stay
// identical for the production producers (which all set JSON/File on success).
func TestBuildParserOutputsUsesProduced(t *testing.T) {
	// Produced result (JSON present) => structured output emitted.
	produced := parser.ParseResult{JSON: []parser.DocItem{parser.NewTextJSONItem("hi")}}
	out := buildParserOutputs(t.Context(), produced, "f.pdf", nil, "")
	if out["output_format"] != "json" {
		t.Errorf("produced result must set output_format=json, got %v", out["output_format"])
	}
	if _, ok := out["json"]; !ok {
		t.Error("produced result must emit json")
	}

	// Not produced: success with neither JSON nor File => raw fallback path.
	notProduced := parser.ParseResult{}
	outFallback := buildParserOutputs(t.Context(), notProduced, "f.pdf", []byte("raw page"), "")
	if _, ok := outFallback["json"]; !ok {
		t.Fatal("fallback must still emit json items")
	}
	items, _ := outFallback["json"].([]parser.DocItem)
	if len(items) != 1 {
		t.Fatalf("fallback must emit one raw text item, got %d", len(items))
	}
	if text, _ := items[0].ToMap()["text"].(string); text != "raw page" {
		t.Errorf("fallback text item mismatch: %q", text)
	}

	// Error result => also takes the fallback path (never emits structured JSON).
	errored := parser.ParseResult{Err: errors.New("boom")}
	outErr := buildParserOutputs(t.Context(), errored, "f.pdf", []byte("raw"), "")
	itemsErr, _ := outErr["json"].([]parser.DocItem)
	if len(itemsErr) != 1 {
		t.Fatalf("errored result must fall back to one raw text item, got %d", len(itemsErr))
	}
}
