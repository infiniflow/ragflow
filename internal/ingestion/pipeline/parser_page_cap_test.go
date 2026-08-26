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

// TestParserComponentParams verifies the Parser cpnID and params are
// discovered from both enveloped and raw DSL, and ("", nil) is returned when
// no Parser component exists.
func TestParserComponentParams(t *testing.T) {
	// enveloped DSL with a Parser component carrying params.
	dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {"pdf": {"parse_method": "DeepDOC"}}}}}`)
	cpnID, params := parserComponentParams(dsl, testParserComponentName)
	if cpnID != "Parser:Abc" {
		t.Fatalf("enveloped: want Parser:Abc, got %q", cpnID)
	}
	if pdf, ok := params["pdf"].(map[string]any); !ok || pdf["parse_method"] != "DeepDOC" {
		t.Fatalf("params not extracted: %#v", params)
	}

	// no Parser component -> ("", nil).
	dslNo := envelopedDSL(`{"Tokenizer:X": {"obj": {"component_name": "Tokenizer", "params": {}}}}`)
	if cpnID, params := parserComponentParams(dslNo, testParserComponentName); cpnID != "" || params != nil {
		t.Fatalf("no parser: want (\"\", nil), got (%q, %#v)", cpnID, params)
	}

	// raw (non-enveloped) inner DSL is also accepted.
	raw := []byte(`{"components": {"Parser:Z": {"obj": {"component_name": "Parser", "params": {}}}}}`)
	if cpnID, _ := parserComponentParams(raw, testParserComponentName); cpnID != "Parser:Z" {
		t.Fatalf("raw: want Parser:Z, got %q", cpnID)
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

// TestBuildParserPageCapOverrideRespectsDSLPages is the regression test for
// the canvas page-range bug: a dataflow dry-run carries an empty
// parserConfig, and the canvas saves the user's explicit page range on the
// Parser component's own DSL params (params[family]["pages"]). The injected
// cap must not replace that explicit range via the override-wins merge.
func TestBuildParserPageCapOverrideRespectsDSLPages(t *testing.T) {
	familyOf := func(ext string) string {
		if ext == "pdf" {
			return "pdf"
		}
		return ""
	}
	const docType = "pdf"

	t.Run("explicit DSL pages -> cap not injected", func(t *testing.T) {
		// The default canvas setup: pages [[1, 100000]] covers the whole
		// document, so the debug cap must leave it untouched.
		dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {"pdf": {"parse_method": "DeepDOC", "pages": [[1, 100000]]}}}}}`)
		out := BuildParserPageCapOverride(map[string]any{}, dsl, docType, 2, testParserComponentName, familyOf)
		if len(out) != 0 {
			t.Fatalf("explicit DSL pages must be respected, got override %#v", out)
		}
	})

	t.Run("empty DSL pages -> cap injected, family settings preserved", func(t *testing.T) {
		// pages [] means "parse all pages" — not an explicit range, so the
		// cap still applies, but the DSL family settings must survive the
		// override merge (it replaces the whole family entry).
		dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {"pdf": {"parse_method": "Plain Text", "remove_header_footer": true, "pages": []}}}}}`)
		out := BuildParserPageCapOverride(map[string]any{}, dsl, docType, 2, testParserComponentName, familyOf)
		fam, ok := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
		if !ok {
			t.Fatalf("missing cpnID/family entry: %#v", out)
		}
		if !reflect.DeepEqual(fam["pages"], []any{[]any{1, 2}}) {
			t.Fatalf("pages cap shape wrong: %#v", fam["pages"])
		}
		if fam["parse_method"] != "Plain Text" {
			t.Fatalf("parse_method must survive the cap injection: %#v", fam)
		}
		if fam["remove_header_footer"] != true {
			t.Fatalf("remove_header_footer must survive the cap injection: %#v", fam)
		}
	})

	t.Run("no pages key in DSL -> cap injected, family settings preserved", func(t *testing.T) {
		dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {"pdf": {"parse_method": "DeepDOC"}}}}}`)
		out := BuildParserPageCapOverride(map[string]any{}, dsl, docType, 2, testParserComponentName, familyOf)
		fam, ok := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
		if !ok {
			t.Fatalf("missing cpnID/family entry: %#v", out)
		}
		if !reflect.DeepEqual(fam["pages"], []any{[]any{1, 2}}) {
			t.Fatalf("pages cap shape wrong: %#v", fam["pages"])
		}
		if fam["parse_method"] != "DeepDOC" {
			t.Fatalf("parse_method must survive the cap injection: %#v", fam)
		}
	})

	t.Run("explicit DSL pages for another family do not block the cap", func(t *testing.T) {
		// pages configured under a family other than the document's own must
		// not disable the cap for this document's family.
		dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {"docx": {"pages": [[1, 50]]}, "pdf": {"parse_method": "DeepDOC"}}}}}`)
		out := BuildParserPageCapOverride(map[string]any{}, dsl, docType, 2, testParserComponentName, familyOf)
		fam, ok := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
		if !ok {
			t.Fatalf("missing cpnID/family entry: %#v", out)
		}
		if !reflect.DeepEqual(fam["pages"], []any{[]any{1, 2}}) {
			t.Fatalf("pages cap shape wrong: %#v", fam["pages"])
		}
	})

	t.Run("parserConfig keys win over seeded DSL keys", func(t *testing.T) {
		dsl := envelopedDSL(`{"Parser:Abc": {"obj": {"component_name": "Parser", "params": {"pdf": {"parse_method": "DeepDOC"}}}}}`)
		pc := map[string]any{
			"Parser:Abc": map[string]any{
				"pdf": map[string]any{"parse_method": "Plain Text"},
			},
		}
		out := BuildParserPageCapOverride(pc, dsl, docType, 2, testParserComponentName, familyOf)
		fam := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
		if fam["parse_method"] != "Plain Text" {
			t.Fatalf("existing parserConfig value must win over the seeded DSL value: %#v", fam)
		}
		if !reflect.DeepEqual(fam["pages"], []any{[]any{1, 2}}) {
			t.Fatalf("pages cap shape wrong: %#v", fam["pages"])
		}
	})
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
