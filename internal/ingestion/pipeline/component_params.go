package pipeline

import (
	"encoding/json"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
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

	extractorCount := 0
	for cpnID, comp := range dsl.Components {
		if cpnID != common.ComponentNameFile && !common.IsComponentKey(cpnID) {
			return nil, fmt.Errorf("ExtractAllComponentParams: component ID %q must be <name>:<instance>; re-save the pipeline with canonical component IDs", cpnID)
		}
		if common.IsExtractorComponent(cpnID, comp.Obj.ComponentName) {
			extractorCount++
		}
	}
	if extractorCount > 1 {
		return nil, fmt.Errorf("ExtractAllComponentParams: at most 1 Extractor component is allowed, found %d", extractorCount)
	}

	out := make([]ComponentParamsSchema, 0, len(dsl.Components))
	for cpnID, comp := range dsl.Components {
		params := make(map[string]any, len(comp.Obj.Params))
		for k, v := range comp.Obj.Params {
			if k == "outputs" {
				continue
			}
			params[k] = common.DeepCopyValue(v)
		}
		// Canvases saved by the -era frontend nest the Parser's
		// per-family setups under a "setups" key; lift them so the extracted
		// defaults (and the parser_config built from them) stay flat.
		if common.IsParserComponent(cpnID, comp.Obj.ComponentName) {
			params = schema.FlattenLegacyParserSetups(params)
		}
		out = append(out, ComponentParamsSchema{
			CpnID:          cpnID,
			ComponentName:  comp.Obj.ComponentName,
			ParamsDefaults: params,
		})
	}
	return out, nil
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
		out[s.CpnID] = common.DeepCopyValue(s.ParamsDefaults).(map[string]any)
	}
	return out, nil
}
