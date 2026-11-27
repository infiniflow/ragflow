package pipeline

import (
	"testing"
)

// testParserComponentName mirrors component.ComponentNameParser ("Parser").
// The pipeline package deliberately does NOT import component (no reverse
// dependency); callers inject the name, so the test uses the literal here.
const testParserComponentName = "Parser"

// envelopedDSL wraps a components map in the canvas envelope {"dsl": {...}}.
func envelopedDSL(components string) []byte {
	return []byte(`{"dsl": {"components": ` + components + `}}`)
}

// TestExtractParserCpnID verifies the Parser cpnID is discovered from both
// enveloped and raw DSL, and "" is returned when no Parser component exists.
func TestExtractParserCpnID(t *testing.T) {
	// enveloped DSL with a Parser component.
	dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {}}}}`)
	if got := ExtractParserCpnID(dsl, testParserComponentName); got != "Parser:Abc" {
		t.Fatalf("enveloped: want Parser:Abc, got %q", got)
	}

	// no Parser component -> "".
	dslNo := envelopedDSL(`{"Tokenizer:X": {"obj": {"component_name": "Tokenizer", "params": {}}}}`)
	if got := ExtractParserCpnID(dslNo, testParserComponentName); got != "" {
		t.Fatalf("no parser: want empty, got %q", got)
	}

	// raw (non-enveloped) inner DSL is also accepted.
	raw := []byte(`{"components": {"Parser:Z": {"obj": {"component_name": "Parser", "params": {}}}}}`)
	if got := ExtractParserCpnID(raw, testParserComponentName); got != "Parser:Z" {
		t.Fatalf("raw: want Parser:Z, got %q", got)
	}
}

// TestUnwrapCanvasDSL verifies the envelope is stripped and a raw DSL passes
// through unchanged; an empty/invalid payload errors.
func TestUnwrapCanvasDSL(t *testing.T) {
	env := []byte(`{"dsl": {"components": {}}}`)
	inner, err := UnwrapCanvasDSL(env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := inner["components"]; !ok {
		t.Fatalf("envelope not stripped: %#v", inner)
	}

	raw := []byte(`{"components": {}}`)
	if _, err := UnwrapCanvasDSL(raw); err != nil {
		t.Fatalf("raw DSL should pass through, got error: %v", err)
	}

	// an empty-but-valid object passes through (matches UnwrapCanvasDSL: only
	// a truly nil/unparseable DSL errors).
	empty, err := UnwrapCanvasDSL([]byte(`{}`))
	if err != nil {
		t.Fatalf("empty object should not error, got: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty object should yield empty map, got: %#v", empty)
	}

	// invalid JSON errors.
	if _, err := UnwrapCanvasDSL([]byte(`not json`)); err == nil {
		t.Fatalf("invalid JSON should error")
	}
}
