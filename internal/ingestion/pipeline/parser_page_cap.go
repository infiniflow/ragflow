package pipeline

import (
	"encoding/json"
	"fmt"
)

// UnwrapCanvasDSL decodes a raw pipeline DSL and strips the optional canvas
// envelope {"dsl": {...}}, returning the inner components-carrying map. A raw
// (non-enveloped) DSL is returned unchanged. It is the []byte entry point used
// by helpers that need the inner DSL (parserComponentParams,
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

// parserComponentParams returns the cpnID and the params map of the first
// component whose component_name equals parserComponentName, or ("", nil)
// when no such component exists or the DSL cannot be read. dsl may be
// enveloped; it is unwrapped first.
func parserComponentParams(dsl []byte, parserComponentName string) (string, map[string]any) {
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
// The cap is a FALLBACK only. It is not injected when an explicit "pages"
// range is already configured for the family in either of the two places the
// runtime reads it from:
//
//   - parserConfig[cpnID][family]["pages"] — the run-level override map
//     (production ParserConfig), and
//   - the Parser component's own DSL params, params[family]["pages"] — where
//     the canvas page-range field is saved. A canvas dry-run carries an
//     empty parserConfig, so without this second check the injected cap
//     would silently replace the user's explicit range through the
//     override-wins merge and only the first capPages pages would parse.
//
// A "pages" value that is missing, null, or an empty list means "parse all
// pages" and does not count as an explicit range — the cap still applies.
//
// When no Parser component is found or the family is empty, the call is a
// no-op and parserConfig is returned unchanged. When the cap IS injected, the
// family entry is seeded from the component's DSL family params (with "pages"
// set to the cap) because the runtime override merge replaces the whole
// family entry, not just the "pages" key. The injected shape is
// []any{[]any{1, capPages}}: a JSON-decoded list of 1-indexed inclusive
// ranges, the exact form the deepdoc/pdf parser's NormalizePDFPages consumes
// via ParserConfig[cpnID][family]["pages"].
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
	parserCpnID, parserParams := parserComponentParams(dsl, parserComponentName)
	if parserCpnID == "" {
		return parserConfig
	}
	family := familyOf(docType)
	if family == "" {
		return parserConfig
	}
	dslFam, _ := parserParams[family].(map[string]any)
	// Respect an explicit page range already present under cpnID + family.
	if cpnEntry, ok := parserConfig[parserCpnID].(map[string]any); ok {
		if famEntry, ok := cpnEntry[family].(map[string]any); ok {
			if _, has := famEntry["pages"]; has {
				return parserConfig
			}
		}
	}
	// Respect an explicit page range configured on the Parser component's own
	// DSL params (the canvas page-range field).
	if hasExplicitPages(dslFam["pages"]) {
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
	// Seed the family entry from the component's DSL params so the injected
	// override does not drop the settings the DSL already carries. Keys
	// already present in parserConfig win; "pages" is overwritten with the
	// cap below regardless.
	for k, v := range dslFam {
		if _, exists := famEntry[k]; !exists {
			famEntry[k] = deepCopyValue(v)
		}
	}
	famEntry["pages"] = []any{[]any{1, capPages}}
	return parserConfig
}

// hasExplicitPages reports whether a raw "pages" value carries at least one
// configured range. Missing, null, and empty-list values mean "parse all
// pages" and do not count as explicit — the cap remains a fallback for them.
func hasExplicitPages(raw any) bool {
	list, ok := raw.([]any)
	return ok && len(list) > 0
}
