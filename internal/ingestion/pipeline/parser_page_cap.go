package pipeline

import (
	"encoding/json"
	"fmt"
)

// UnwrapCanvasDSL decodes a raw pipeline DSL and strips the optional canvas
// envelope {"dsl": {...}}, returning the inner components-carrying map. A raw
// (non-enveloped) DSL is returned unchanged. It is the []byte entry point used
// by helpers that need the inner DSL (ExtractParserCpnID,
// BuildParserPageCapOverride) so envelope-handling lives in exactly one place.
func UnwrapCanvasDSL(raw []byte) (map[string]any, error) {
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("UnwrapCanvasDSL: decode: %w", err)
	}
	if top == nil {
		return nil, errNilDSL
	}
	if env, ok := top["dsl"].(map[string]any); ok && len(env) > 0 {
		return env, nil
	}
	return top, nil
}

// ExtractParserCpnID returns the cpnID of the first component whose
// component_name equals parserComponentName, or "" when no such component
// exists or the DSL cannot be read. dsl may be enveloped; it is unwrapped
// first.
func ExtractParserCpnID(dsl []byte, parserComponentName string) string {
	cpnID, _ := extractParserComponent(dsl, parserComponentName)
	return cpnID
}

// extractParserComponent returns the Parser component's cpnID together with its
// DSL params, or "" / nil when the DSL holds no Parser or cannot be read.
func extractParserComponent(dsl []byte, parserComponentName string) (string, map[string]any) {
	inner, err := UnwrapCanvasDSL(dsl)
	if err != nil {
		return "", nil
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return "", nil
	}
	schemas, err := ExtractAllComponentParams(innerJSON)
	if err != nil {
		return "", nil
	}
	for _, s := range schemas {
		if s.ComponentName == parserComponentName {
			return s.CpnID, s.ParamsDefaults
		}
	}
	return "", nil
}

// BuildParserPageCapOverride returns parserConfig with a page cap injected for
// the Parser component, keyed by its cpnID and the document's filetype family.
//
// It is debug-agnostic: capPages and familyOf are supplied by the caller, so
// the function carries no debug-specific semantics and is reusable for any
// page-cap scenario. docType is the uploaded file extension (e.g. "pdf");
// familyOf maps it to a parser setup family (callers pass
// component.ParserFileFamily); parserComponentName is the Parser component
// name (callers pass component.ComponentNameParser) — kept as a parameter so
// the pipeline package does not import component.
//
// When no Parser component is found or the family is empty, the call is a
// no-op and parserConfig is returned unchanged. An explicit "pages" cap
// already present under cpnID+family is respected and left untouched — the
// cap is only a fallback. The injected shape is []any{[]any{1, capPages}}: a
// JSON-decoded list of 1-indexed inclusive ranges, the exact form the
// deepdoc/pdf parser's NormalizePDFPages consumes via
// ParserConfig[cpnID][family]["pages"].
//
// The injected family map is seeded from the DSL's own settings for that family
// in both paths: the runtime merges this map over the DSL params with
// override-wins on the top-level key, and a parser family IS a top-level key, so
// an entry carrying only the cap would replace the authored family wholesale.
func BuildParserPageCapOverride(
	parserConfig map[string]any,
	dsl []byte,
	docType string,
	capPages int,
	parserComponentName string,
	familyOf func(string) string,
) map[string]any {
	if parserConfig == nil {
		parserConfig = map[string]any{}
	}
	parserCpnID, parserParams := extractParserComponent(dsl, parserComponentName)
	if parserCpnID == "" {
		return parserConfig
	}
	family := familyOf(docType)
	if family == "" {
		return parserConfig
	}
	cpnEntry, ok := parserConfig[parserCpnID].(map[string]any)
	if !ok {
		cpnEntry = map[string]any{}
		parserConfig[parserCpnID] = cpnEntry
	}
	famEntry, ok := cpnEntry[family].(map[string]any)
	if !ok {
		famEntry = map[string]any{}
		cpnEntry[family] = famEntry
	}
	// An explicit page cap supplied by the caller wins; the injected cap is only
	// a fallback. Read before the carry-forward below so the DSL's own "pages"
	// cannot be mistaken for a caller-supplied cap.
	_, explicitPages := famEntry["pages"]
	// ExtractAllComponentParams already lifted any legacy "setups" nesting, so
	// the authored setup sits at the params top level here.
	dslFamily, _ := parserParams[family].(map[string]any)
	for key, value := range dslFamily {
		if _, exists := famEntry[key]; !exists {
			famEntry[key] = value
		}
	}
	if !explicitPages {
		famEntry["pages"] = []any{[]any{1, capPages}}
	}
	return parserConfig
}
