package pipeline

import (
	"encoding/json"
	"fmt"

	"ragflow/internal/utility"
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
// pages" and does not count as an explicit range — the cap still applies. The
// all-pages sentinel carried by the shipped templates and the web UI defaults
// ([[1, 100000]]) means the same, so a single gap-free [1, N] range with
// N >= allPagesUpperBound does not count as explicit either; only a genuinely
// narrowed range suppresses the cap.
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
	// The same explicitness predicate as the DSL branch below: null/empty
	// pages and the all-pages sentinel mean "parse all pages", so the cap
	// remains a fallback for them.
	if cpnEntry, ok := parserConfig[parserCpnID].(map[string]any); ok {
		if famEntry, ok := cpnEntry[family].(map[string]any); ok {
			if hasExplicitPages(famEntry["pages"]) {
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

// allPagesUpperBound is the smallest upper bound of a "pages" range treated as
// the all-pages sentinel. The general template and the web UI's page-range
// fields default to [[1, 100000]] (template/ingestion_pipeline_general.json,
// web chunk-method-dialog and agent parser-form), matching Python's
// MAXIMUM_PAGE_NUMBER, and the parser clamps oversized upper bounds to the
// document's actual page count — so a gap-free [1, N] with N >=
// allPagesUpperBound does not actually restrict parsing.
const allPagesUpperBound = 100000

// hasExplicitPages reports whether a raw "pages" value carries a range that
// actually restricts parsing. Missing, null, empty-list, and uninterpretable
// values do not count (the parser degrades malformed values to "parse all
// pages" at parse time, so the cap is not withheld for them here either), and
// neither does the all-pages sentinel — a single gap-free [1, N] range with
// N >= allPagesUpperBound, the shipped template/UI default. The cap remains a
// fallback for all of them; only a genuinely narrowed range counts as
// explicit.
func hasExplicitPages(raw any) bool {
	ranges, err := utility.NormalizePDFPages(raw)
	if err != nil || len(ranges) == 0 {
		return false
	}
	return !(len(ranges) == 1 && ranges[0][0] == 1 && ranges[0][1] >= allPagesUpperBound)
}
