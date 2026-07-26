package pipeline

import (
	"encoding/json"
	"fmt"
)

// ComponentParamsSchema describes the full set of user-configurable parameters
// for one component in a DSL. The frontend uses this to render editable forms;
// the API uses it for validation.
type ComponentParamsSchema struct {
	// CpnID is the component instance identifier in the DSL, e.g. "Parser:HipSignsRhyme".
	CpnID string `json:"cpn_id"`
	// ComponentName is the logical component type, e.g. "Parser", "Tokenizer".
	ComponentName string `json:"component_name"`
	// ParamsDefaults holds the params map from the DSL with "outputs" excluded.
	// Keys are param names (e.g. "setups", "fields", "chunk_token_size");
	// values are their DSL-baked defaults.
	ParamsDefaults map[string]any `json:"params_defaults"`
}

// ExtractAllComponentParams extracts the params schema for every component
// present in the DSL JSON. It excludes the internal "outputs" key from each
// component's params — those are wire-format definitions, not user-configurable.
func ExtractAllComponentParams(dslJSON []byte) ([]ComponentParamsSchema, error) {
	var dsl struct {
		Components map[string]struct {
			Obj struct {
				ComponentName string         `json:"component_name"`
				Params        map[string]any `json:"params"`
			} `json:"obj"`
		} `json:"components"`
	}
	if err := json.Unmarshal(dslJSON, &dsl); err != nil {
		return nil, fmt.Errorf("ExtractAllComponentParams: decode DSL: %w", err)
	}
	if dsl.Components == nil {
		return nil, fmt.Errorf("ExtractAllComponentParams: DSL has no components")
	}

	out := make([]ComponentParamsSchema, 0, len(dsl.Components))
	for cpnID, comp := range dsl.Components {
		params := make(map[string]any, len(comp.Obj.Params))
		for k, v := range comp.Obj.Params {
			if k == "outputs" {
				continue
			}
			params[k] = deepCopyValue(v)
		}
		out = append(out, ComponentParamsSchema{
			CpnID:          cpnID,
			ComponentName:  comp.Obj.ComponentName,
			ParamsDefaults: params,
		})
	}
	return out, nil
}

// deepCopyValue returns a deep copy of a value that might be a nested map or slice.
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(val))
		for mk, mv := range val {
			cp[mk] = deepCopyValue(mv)
		}
		return cp
	case []any:
		cp := make([]any, len(val))
		for i, item := range val {
			cp[i] = deepCopyValue(item)
		}
		return cp
	default:
		return v
	}
}

// deepCopyMapStr deep-copies a map[string]any so the caller can safely mutate the result.
func deepCopyMapStr(src map[string]any) map[string]any {
	cp := make(map[string]any, len(src))
	for k, v := range src {
		cp[k] = deepCopyValue(v)
	}
	return cp
}

// ComponentParamsDefaults extracts component params from DSL JSON and returns
// them in the component_params storage format {cpnID: {param: value}}.
// Values are deep-copied from the DSL so callers can safely mutate the result.
func ComponentParamsDefaults(dslJSON []byte) (map[string]map[string]any, error) {
	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		return nil, err
	}
	out := make(map[string]map[string]any, len(schemas))
	for _, s := range schemas {
		out[s.CpnID] = deepCopyMapStr(s.ParamsDefaults)
	}
	return out, nil
}
