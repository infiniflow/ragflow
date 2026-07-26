package pipeline

import (
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"

	"go.uber.org/zap"
)

// CleanComponentParams filters rawConfig against the DSL schema given by dslJSON.
// Keys containing ':' are treated as component IDs; they are kept only when both
// the cpnID AND the param name exist in the DSL schema. Keys without ':' (legacy
// flat fields such as chunk_token_num, image_context_size) are dropped with a
// warning — they do not belong in the new component-params world.
func CleanComponentParams(dslJSON []byte, rawConfig map[string]interface{}) map[string]interface{} {
	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		common.Warn("CleanComponentParams: failed to extract DSL schema, returning input as-is",
			zap.Error(err))
		return rawConfig
	}

	validCPNs := make(map[string]map[string]struct{}, len(schemas))
	for _, s := range schemas {
		keys := make(map[string]struct{}, len(s.ParamsDefaults))
		for k := range s.ParamsDefaults {
			keys[k] = struct{}{}
		}
		validCPNs[s.CpnID] = keys
	}

	result := make(map[string]interface{}, len(rawConfig))
	for key, val := range rawConfig {
		if !strings.Contains(key, ":") {
			common.Warn("CleanComponentParams: dropping legacy flat field",
				zap.String("key", key))
			continue
		}
		validKeys, ok := validCPNs[key]
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
			if _, ok := validKeys[pk]; ok {
				cleaned[pk] = pv
			} else {
				common.Warn("CleanComponentParams: dropping unknown param",
					zap.String("cpnID", key), zap.String("param", pk))
			}
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
func BuildParserConfig(dslJSON []byte, rawConfig map[string]interface{}) entity.JSONMap {
	cleaned := CleanComponentParams(dslJSON, rawConfig)
	defaults, err := ComponentParamsDefaults(dslJSON)
	if err != nil {
		common.Warn("BuildParserConfig: failed to extract DSL defaults, using cleaned only",
			zap.Error(err))
		return entity.JSONMap(cleaned)
	}
	result := make(entity.JSONMap, len(defaults))
	for cpnID, params := range defaults {
		base := make(map[string]interface{}, len(params))
		for k, v := range params {
			base[k] = v
		}
		if over, ok := cleaned[cpnID]; ok {
			if om, ok := over.(map[string]any); ok {
				for k, v := range om {
					base[k] = v
				}
			}
		}
		result[cpnID] = base
	}
	return result
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
