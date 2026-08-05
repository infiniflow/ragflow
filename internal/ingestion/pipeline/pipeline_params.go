package pipeline

import (
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"

	"go.uber.org/zap"
)

// componentNameParser is the DSL component_name of the Parser component.
// Unlike scalar-param components, its params map is a dynamic per-filetype
// setups table keyed by file family (pdf, docx, slides, ...).
const componentNameParser = "Parser"

// parserFileFamilies is the canonical set of file-family keys the Parser
// component accepts in its setups table. Built-in DSL templates only bake
// the subset of families each pipeline enables by default, but users can
// enable any family from the dataset settings UI, so family keys are
// validated against this set instead of the DSL defaults.
var parserFileFamilies = func() map[string]struct{} {
	allowed := schema.ParserParam{}.Defaults().AllowedOutputFormat
	out := make(map[string]struct{}, len(allowed))
	for family := range allowed {
		out[family] = struct{}{}
	}
	return out
}()

// CleanComponentParams filters rawConfig against the DSL schema given by dslJSON.
// Keys containing ':' are treated as component IDs; they are kept only when both
// the cpnID AND the param name exist in the DSL schema. Keys without ':' (legacy
// flat fields such as chunk_token_num, image_context_size) are dropped with a
// warning — they do not belong in the new component-params world.
//
// Parser components are the exception to the param-name rule: their params
// map is a per-filetype setups table, so any canonical file-family key is
// accepted even when the DSL template does not bake a default for it (e.g.
// "slides" in the general template).
func CleanComponentParams(dslJSON []byte, rawConfig map[string]interface{}) map[string]interface{} {
	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		common.Warn("CleanComponentParams: failed to extract DSL schema, returning input as-is",
			zap.Error(err))
		return rawConfig
	}

	type componentSchema struct {
		isParser  bool
		paramKeys map[string]struct{}
	}
	validCPNs := make(map[string]componentSchema, len(schemas))
	for _, s := range schemas {
		keys := make(map[string]struct{}, len(s.ParamsDefaults))
		for k := range s.ParamsDefaults {
			keys[k] = struct{}{}
		}
		validCPNs[s.CpnID] = componentSchema{
			isParser:  s.ComponentName == componentNameParser,
			paramKeys: keys,
		}
	}

	result := make(map[string]interface{}, len(rawConfig))
	for key, val := range rawConfig {
		if !strings.Contains(key, ":") {
			common.Warn("CleanComponentParams: dropping legacy flat field",
				zap.String("key", key))
			continue
		}
		cpn, ok := validCPNs[key]
		if !ok {
			common.Warn("CleanComponentParams: dropping unknown cpnID",
				zap.String("cpnID", key))
			continue
		}
		params, ok := val.(map[string]any)
		if !ok {
			continue
		}
		cleaned := make(map[string]any, len(params))
		for pk, pv := range params {
			if _, ok := cpn.paramKeys[pk]; ok {
				cleaned[pk] = pv
				continue
			}
			if cpn.isParser {
				if _, ok := parserFileFamilies[pk]; ok {
					cleaned[pk] = pv
					continue
				}
			}
			common.Warn("CleanComponentParams: dropping unknown param",
				zap.String("cpnID", key), zap.String("param", pk))
		}
		if len(cleaned) > 0 {
			result[key] = cleaned
		}
	}
	return result
}

// BuildParserConfig builds the final parser_config by starting from the DSL
// defaults for every component, then overlaying the cleaned incoming overrides.
// This ensures all components from the current pipeline are present while
// stripping stale params from other pipelines.
//
// Parser components follow different merge semantics: their per-filetype
// setups table is user-driven, so when the incoming config carries a Parser
// entry, the incoming file-family set is authoritative — families the user
// removed (e.g. pdf) are not resurrected from the DSL defaults, and families
// the template does not bake (e.g. slides) are kept.
func BuildParserConfig(dslJSON []byte, rawConfig map[string]interface{}) entity.JSONMap {
	cleaned := CleanComponentParams(dslJSON, rawConfig)
	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		common.Warn("BuildParserConfig: failed to extract DSL defaults, using cleaned only",
			zap.Error(err))
		return entity.JSONMap(cleaned)
	}
	result := make(entity.JSONMap, len(schemas))
	for _, s := range schemas {
		base := deepCopyMapStr(s.ParamsDefaults)
		over, hasOver := cleaned[s.CpnID].(map[string]any)
		if s.ComponentName == componentNameParser && hasOver {
			result[s.CpnID] = buildParserComponentConfig(base, over)
			continue
		}
		if hasOver {
			for k, v := range over {
				base[k] = v
			}
		}
		result[s.CpnID] = base
	}
	return result
}

// buildParserComponentConfig merges incoming overrides into a Parser
// component's per-filetype setups table. The incoming family set is
// authoritative: each kept family's setup is overlaid on its DSL default
// base when one exists so template-baked fields survive, families absent
// from the incoming config are dropped, and families without a DSL default
// are taken as-is. Non-family params (if any) come from the DSL defaults
// plus incoming overrides.
func buildParserComponentConfig(defaults map[string]any, over map[string]any) map[string]interface{} {
	base := make(map[string]interface{}, len(defaults)+len(over))
	for k, v := range defaults {
		if _, isFamily := parserFileFamilies[k]; !isFamily {
			base[k] = v
		}
	}
	for k, v := range over {
		if _, isFamily := parserFileFamilies[k]; isFamily {
			if def, ok := defaults[k].(map[string]any); ok {
				if vm, ok := v.(map[string]any); ok {
					merged := make(map[string]interface{}, len(def)+len(vm))
					for dk, dv := range def {
						merged[dk] = dv
					}
					for dk, dv := range vm {
						merged[dk] = dv
					}
					base[k] = merged
					continue
				}
			}
		}
		base[k] = v
	}
	return base
}

// ResolveComponentParamsDefaults takes DSL JSON bytes and returns the
// component params defaults as an entity.JSONMap {cpnID: {param: value}}.
// This is a pure function — callers must load the DSL themselves.
func ResolveComponentParamsDefaults(dslJSON []byte) (entity.JSONMap, error) {
	cp, err := ComponentParamsDefaults(dslJSON)
	if err != nil {
		return nil, err
	}
	out := make(entity.JSONMap, len(cp))
	for k, v := range cp {
		out[k] = v
	}
	return out, nil
}

// ResolveComponentParamsDefaultsFromIDs loads the DSL for the target pipeline
// (builtin via parserID or custom canvas via pipelineID) and returns the
// component params defaults. It is a convenience wrapper around
// LoadPipelineDSL + ResolveComponentParamsDefaults; use the two-step form
// when you already have the DSL bytes.
//
// Deprecated: prefer loading the DSL yourself and calling
// ResolveComponentParamsDefaults directly, to keep DAO dependencies
// out of the pipeline package.
func ResolveComponentParamsDefaultsFromIDs(parserID string, pipelineID *string) (entity.JSONMap, error) {
	isCanvas := pipelineID != nil && strings.TrimSpace(*pipelineID) != ""
	var dslJSON []byte
	var err error
	if isCanvas {
		return nil, fmt.Errorf("ResolveComponentParamsDefaultsFromIDs: cannot load canvas DSL without DAO; " +
			"load the DSL via service.LoadPipelineDSL first")
	}
	registry, regErr := DefaultRegistry()
	if regErr != nil {
		return nil, fmt.Errorf("builtin registry: %w", regErr)
	}
	if !registry.IsValid(parserID) {
		return nil, fmt.Errorf("unknown builtin parser_id: %q", parserID)
	}
	dslStr, dslErr := LoadBuiltinDSL(parserID)
	if dslErr != nil {
		return nil, fmt.Errorf("load builtin DSL: %w", dslErr)
	}
	dslJSON = []byte(dslStr)
	cp, err := ComponentParamsDefaults(dslJSON)
	if err != nil {
		return nil, err
	}
	out := make(entity.JSONMap, len(cp))
	for k, v := range cp {
		out[k] = v
	}
	return out, nil
}
