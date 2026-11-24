package pipeline

import (
	"fmt"
	"reflect"
	"testing"
)

func TestBuildParserPageCapOverridePreservesParserSettings(t *testing.T) {
	for _, method := range []string{"deepdoc_cpu_ee", "deepdoc_gpu_ee", "deepdoc_ascend_ee"} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/nested=%t", method, nested), func(t *testing.T) {
				params := fmt.Sprintf(`{"pdf":{"parse_method":%q,"lang":"English","pages":[[1,100000]]}}`, method)
				if nested {
					params = `{"setups":` + params + `}`
				}
				dsl := envelopedDSL(`{"Parser:Abc":{"obj":{"component_name":"Parser","params":` + params + `}}}`)
				familyOf := func(string) string { return "pdf" }
				out := BuildParserPageCapOverride(nil, dsl, "pdf", 2, testParserComponentName, familyOf)
				family := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
				want := map[string]any{"parse_method": method, "lang": "English", "pages": []any{[]any{1, 2}}}
				if !reflect.DeepEqual(family, want) {
					t.Fatalf("without overrides: want %#v, got %#v", want, family)
				}
				for _, explicitPages := range []bool{false, true} {
					requestFamily := map[string]any{"lang": "French"}
					wantPages := []any{[]any{1, 2}}
					if explicitPages {
						wantPages = []any{[]any{3, 4}}
						requestFamily["pages"] = wantPages
					}
					pc := map[string]any{"Parser:Abc": map[string]any{"pdf": requestFamily}}
					out := BuildParserPageCapOverride(pc, dsl, "pdf", 2, testParserComponentName, familyOf)
					family := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)
					want := map[string]any{"parse_method": method, "lang": "French", "pages": wantPages}
					if !reflect.DeepEqual(family, want) {
						t.Fatalf("explicitPages=%t: want %#v, got %#v", explicitPages, want, family)
					}
				}
				override := map[string]any{"Parser:Abc": map[string]any{"pdf": map[string]any{"parse_method": "plain_text"}}}
				out = BuildParserPageCapOverride(override, dsl, "pdf", 2, testParserComponentName, familyOf)
				if got := out["Parser:Abc"].(map[string]any)["pdf"].(map[string]any)["parse_method"]; got != "plain_text" {
					t.Fatalf("explicit parser method overwritten: %v", got)
				}
			})
		}
	}
}

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
