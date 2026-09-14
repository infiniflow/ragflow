package pipeline

import (
	"fmt"
	"reflect"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"

	"go.uber.org/zap"
)

// llmRuntimeParamKeys contains generic LLM hyper-parameters and UI switches
// common to LLM-based components. They are emitted by the shared LLM settings
// popover (LlmSettingFieldItems) on top of each operator's DSL-declared params.
var llmRuntimeParamKeys = map[string]struct{}{
	"llm_id":                  {},
	"temperature":             {},
	"top_p":                   {},
	"max_tokens":              {},
	"presence_penalty":        {},
	"frequency_penalty":       {},
	"parameter":               {},
	"thinking":                {},
	"outputs":                 {},
	"temperatureEnabled":      {},
	"topPEnabled":             {},
	"presencePenaltyEnabled":  {},
	"frequencyPenaltyEnabled": {},
	"maxTokensEnabled":        {},
}

// extractorValidParamKeys is dynamically derived from schema.ExtractorParam + LLM hyper-parameters.
var extractorValidParamKeys = func() map[string]struct{} {
	keys := extractJSONTags(schema.ExtractorParam{})
	for k := range llmRuntimeParamKeys {
		keys[k] = struct{}{}
	}
	return keys
}()

// extractJSONTags returns all top-level json tag names from a struct.
func extractJSONTags(v any) map[string]struct{} {
	tags := make(map[string]struct{})
	t := reflect.TypeOf(v)
	if t == nil {
		return tags
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return tags
	}
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name != "" {
			tags[name] = struct{}{}
		}
	}
	return tags
}

// isExtractorComponent returns true if the component name or component ID
// indicates an Extractor component.
func isExtractorComponent(cpnID, componentName string) bool {
	lowerName := strings.ToLower(componentName)
	lowerID := strings.ToLower(cpnID)
	return lowerName == "extractor" ||
		strings.HasPrefix(lowerID, "extractor:") ||
		strings.HasPrefix(lowerID, "extractor_")
}

// isCompilerComponent returns true if the component name or component ID
// indicates a Compiler component.
func isCompilerComponent(cpnID, componentName string) bool {
	lowerName := strings.ToLower(componentName)
	lowerID := strings.ToLower(cpnID)
	return lowerName == "compiler" ||
		strings.HasPrefix(lowerID, "compiler:") ||
		strings.HasPrefix(lowerID, "compiler_")
}

// getComponentParamWhitelist returns the dynamic parameter whitelist for an
// LLM-backed component type: keys accepted on top of the DSL-declared params.
// Extractor gets its schema-derived feature groups; Compiler shares the generic
// LLM runtime keys only.
func getComponentParamWhitelist(cpnID, componentName string) (map[string]struct{}, bool) {
	if isExtractorComponent(cpnID, componentName) {
		return extractorValidParamKeys, true
	}
	if isCompilerComponent(cpnID, componentName) {
		return llmRuntimeParamKeys, true
	}
	return nil, false
}

// CleanComponentParams filters rawConfig against the DSL schema given by dslJSON.
// Keys containing ':' are treated as component IDs; they are kept only when both
// the cpnID AND the param name exist in the DSL schema or the component's dynamic
// parameter whitelist (Extractor modular features; shared LLM runtime keys for
// LLM-backed components such as Compiler). Keys without ':' (legacy
// flat fields) are dropped with a warning.
func CleanComponentParams(dslJSON []byte, rawConfig map[string]interface{}) map[string]interface{} {
	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		common.Warn("CleanComponentParams: failed to extract DSL schema, returning input as-is",
			zap.Error(err))
		return rawConfig
	}

	type dslComponent struct {
		name       string
		validParam map[string]struct{}
	}
	validCPNs := make(map[string]dslComponent, len(schemas))
	for _, s := range schemas {
		keys := make(map[string]struct{}, len(s.ParamsDefaults))
		for k := range s.ParamsDefaults {
			keys[k] = struct{}{}
		}
		validCPNs[s.CpnID] = dslComponent{name: s.ComponentName, validParam: keys}
	}

	result := make(map[string]interface{}, len(rawConfig))
	for key, val := range rawConfig {
		if !strings.Contains(key, ":") {
			common.Warn("CleanComponentParams: dropping legacy flat field",
				zap.String("key", key))
			continue
		}
		entry, ok := validCPNs[key]
		if !ok {
			common.Warn("CleanComponentParams: dropping unknown cpnID",
				zap.String("cpnID", key))
			continue
		}
		params, ok := val.(map[string]any)
		if !ok {
			continue
		}
		dynamicWhitelist, hasDynamic := getComponentParamWhitelist(key, entry.name)
		if isExtractorComponent(key, entry.name) {
			params = NormalizeExtractorParams(params)
		}
		cleaned := make(map[string]any, len(params))
		for pk, pv := range params {
			if _, ok := entry.validParam[pk]; ok {
				cleaned[pk] = pv
				continue
			}
			if hasDynamic {
				if _, ok := dynamicWhitelist[pk]; ok {
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

// NormalizeExtractorParams normalizes a raw Extractor parameters map into the canonical modular format.
func NormalizeExtractorParams(raw map[string]any) map[string]any {
	if raw == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(raw))
	for k, v := range raw {
		out[k] = v
	}

	// 1. Keywords
	if kwRaw, ok := out["keywords"].(map[string]any); ok {
		kw := make(map[string]any, len(kwRaw))
		for k, v := range kwRaw {
			kw[k] = v
		}
		out["keywords"] = kw
	}

	// 2. Questions
	if qRaw, ok := out["questions"].(map[string]any); ok {
		q := make(map[string]any, len(qRaw))
		for k, v := range qRaw {
			q[k] = v
		}
		out["questions"] = q
	}

	// 3. Tags
	if tagRaw, ok := out["tags"].(map[string]any); ok {
		tag := make(map[string]any, len(tagRaw))
		for k, v := range tagRaw {
			tag[k] = v
		}
		out["tags"] = tag
	}

	// 4. Summary
	if sumRaw, ok := out["summary"].(map[string]any); ok {
		sum := make(map[string]any, len(sumRaw))
		for k, v := range sumRaw {
			sum[k] = v
		}
		out["summary"] = sum
	}

	// 5. Metadata
	if metaRaw, ok := out["metadata"].(map[string]any); ok {
		meta := make(map[string]any, len(metaRaw))
		for k, v := range metaRaw {
			meta[k] = v
		}
		out["metadata"] = meta
	}

	return out
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
