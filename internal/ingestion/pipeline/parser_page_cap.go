package pipeline

import (
	"encoding/json"
	"fmt"
	"maps"
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
	return extractParserComponent(dsl, parserComponentName).CpnID
}

func extractParserComponent(dsl []byte, parserComponentName string) ComponentParamsSchema {
	inner, err := UnwrapCanvasDSL(dsl)
	if err != nil {
		return ComponentParamsSchema{}
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return ComponentParamsSchema{}
	}
	schemas, err := ExtractAllComponentParams(innerJSON)
	if err != nil {
		return ComponentParamsSchema{}
	}
	for _, s := range schemas {
		if s.ComponentName == parserComponentName {
			return s
		}
	}
	return ComponentParamsSchema{}
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
// already present under cpnID+family is respected. Family settings from the
// DSL are retained, with explicit parserConfig settings taking precedence.
// The injected shape is []any{[]any{1, capPages}}: a
// JSON-decoded list of 1-indexed inclusive ranges, the exact form the
// deepdoc/pdf parser's NormalizePDFPages consumes via
// ParserConfig[cpnID][family]["pages"].
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
	parser := extractParserComponent(dsl, parserComponentName)
	parserCpnID := parser.CpnID
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
	famEntry, _ := cpnEntry[family].(map[string]any)
	defaults, _ := parser.ParamsDefaults[family].(map[string]any)
	merged := maps.Clone(defaults)
	if merged == nil {
		merged = map[string]any{}
	}
	maps.Copy(merged, famEntry)
	// Canvas overrides replace the whole family, so retain its parser settings.
	if _, hasPages := famEntry["pages"]; !hasPages {
		merged["pages"] = []any{[]any{1, capPages}}
	}
	cpnEntry[family] = merged
	return parserConfig
}
