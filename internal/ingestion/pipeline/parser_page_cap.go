package pipeline

import (
	"encoding/json"
	"fmt"
)

// UnwrapCanvasDSL decodes a raw pipeline DSL and strips the optional canvas
// envelope {"dsl": {...}}, returning the inner components-carrying map. A raw
// (non-enveloped) DSL is returned unchanged. It is the []byte entry point used
// by helpers that need the inner DSL.
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
	inner, err := UnwrapCanvasDSL(dsl)
	if err != nil {
		return ""
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return ""
	}
	schemas, err := ExtractAllComponentParams(innerJSON)
	if err != nil {
		return ""
	}
	for _, s := range schemas {
		if s.ComponentName == parserComponentName {
			return s.CpnID
		}
	}
	return ""
}
