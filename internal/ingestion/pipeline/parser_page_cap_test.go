package pipeline

import (
	"reflect"
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

// TestBuildParserPageCapOverride verifies the debug-agnostic page-cap override:
// normal injection, respect-existing-cap, unknown-family no-op, no-parser no-op.
func TestBuildParserPageCapOverride(t *testing.T) {
	familyOf := func(ext string) string {
		if ext == "pdf" {
			return "pdf"
		}
		return ""
	}
	const docType = "pdf"
	dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {}}}}`)

	// 1. normal: enveloped DSL + pdf -> pages cap injected.
	pc := map[string]any{}
	out := BuildParserPageCapOverride(pc, dsl, docType, 2, testParserComponentName, familyOf)
	fam, ok := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
	if !ok {
		t.Fatalf("missing cpnID/family entry: %#v", out)
	}
	if !reflect.DeepEqual(fam["pages"], []any{[]any{1, 2}}) {
		t.Fatalf("pages shape wrong: %#v", fam["pages"])
	}

	// 2. respect an explicit existing cap (fallback, not overwrite).
	pc2 := map[string]any{
		"Parser:Abc": map[string]any{
			"pdf": map[string]any{"pages": []any{[]any{1, 99}}},
		},
	}
	out2 := BuildParserPageCapOverride(pc2, dsl, docType, 2, testParserComponentName, familyOf)
	fam2 := out2["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
	if !reflect.DeepEqual(fam2["pages"], []any{[]any{1, 99}}) {
		t.Fatalf("existing cap must be respected, got %#v", fam2["pages"])
	}

	// 3. unknown docType -> empty family -> no-op.
	out3 := BuildParserPageCapOverride(map[string]any{}, dsl, "xyz", 2, testParserComponentName, familyOf)
	if len(out3) != 0 {
		t.Fatalf("unknown docType should be no-op, got %#v", out3)
	}

	// 4. no Parser component -> no-op.
	dslNoParser := envelopedDSL(`{"Tokenizer:X": {"obj": {"component_name": "Tokenizer", "params": {}}}}`)
	out4 := BuildParserPageCapOverride(map[string]any{}, dslNoParser, docType, 2, testParserComponentName, familyOf)
	if len(out4) != 0 {
		t.Fatalf("no Parser should be no-op, got %#v", out4)
	}
}

// A debug run merges its parser_config over the DSL with override-wins on the
// top-level key, and a parser family is a top-level key: the pages-only entry
// used to replace the authored family wholesale, so a markdown document's
// linked-content switches never reached the Parser (#21032).
func TestBuildParserPageCapOverrideKeepsDSLFamilySettings(t *testing.T) {
	familyOf := func(ext string) string { return ext }
	dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {
		"markdown": {"fetch_links": true, "collect_markdown_links": true, "max_urls": 7, "preprocess": ["main_content"]}
	}}}}`)

	out := BuildParserPageCapOverride(map[string]any{}, dsl, "markdown", 2, testParserComponentName, familyOf)
	fam, ok := out["Parser:Abc"].(map[string]any)["markdown"].(map[string]any)
	if !ok {
		t.Fatalf("missing cpnID/family entry: %#v", out)
	}
	for key, want := range map[string]any{
		"fetch_links": true, "collect_markdown_links": true, "max_urls": float64(7),
	} {
		if !reflect.DeepEqual(fam[key], want) {
			t.Errorf("%s = %#v, want %#v (authored setting lost to the page-cap injection)", key, fam[key], want)
		}
	}
	if !reflect.DeepEqual(fam["pages"], []any{[]any{1, 2}}) {
		t.Errorf("pages = %#v, want the debug cap", fam["pages"])
	}

	// An explicit ParserConfig value still beats the DSL's.
	pc := map[string]any{"Parser:Abc": map[string]any{"markdown": map[string]any{"max_urls": 1}}}
	merged := BuildParserPageCapOverride(pc, dsl, "markdown", 2, testParserComponentName, familyOf)
	famMerged := merged["Parser:Abc"].(map[string]any)["markdown"].(map[string]any)
	if famMerged["max_urls"] != 1 {
		t.Errorf("max_urls = %#v, want the explicit ParserConfig value 1", famMerged["max_urls"])
	}
	if famMerged["fetch_links"] != true {
		t.Errorf("fetch_links = %#v, want the DSL value preserved alongside it", famMerged["fetch_links"])
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
