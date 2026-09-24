package pipeline

import (
	"fmt"
	"reflect"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	parserchunk "ragflow/internal/parser/chunk"

	"go.uber.org/zap"
)

// llmRuntimeParamKeys contains generic LLM hyper-parameters and UI switches common to LLM-based components.
var llmRuntimeParamKeys = map[string]struct{}{
	"llm_id":                  {},
	"temperature":             {},
	"top_p":                   {},
	"max_tokens":              {},
	"presence_penalty":        {},
	"frequency_penalty":       {},
	"outputs":                 {},
	"parameter":               {},
	"thinking":                {},
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

// componentParamSchemaKeys maps a DSL component_name (lower-cased) to the
// param keys its param schema declares. CleanComponentParams unions these with
// the keys a template already bakes, so a param the component can read is
// never dropped just because a template — or a canvas snapshot saved before
// the param existed — does not carry it.
//
// Components whose accepted keys are defined by the DSL itself (Parser file
// families) or whose param struct is not part of the schema package
// (GeneralChunker, QAChunker) stay out of this table; Extractor and Compiler
// keep their cpnID-prefixed dynamic whitelists.
// manualChunkerParamKeys is TitleChunkerParam's key set minus
// include_heading_content (#20139). ManualChunker reuses TitleChunkerParam
// wholesale, but include_heading_content only has a toggle point in the
// hierarchy path (hierarchy.go's leaf-only emission rule), and ManualChunker
// is permanently pinned to method="group" (manual.go:NewManualChunker), so it
// can never reach that code — the key would silently persist with zero
// effect. method, levels, hierarchy, root_chunk_as_heading and
// chunk_token_cap all have real effect for ManualChunker (method's value is
// fixed but still consumed; chunk_token_cap enforces the same cap
// GroupTitleChunker does, as of #20139), so they stay.
var manualChunkerParamKeys = func() map[string]struct{} {
	keys := extractJSONTags(schema.TitleChunkerParam{})
	delete(keys, "include_heading_content")
	return keys
}()

var componentParamSchemaKeys = map[string]map[string]struct{}{
	"titlechunker":  extractJSONTags(schema.TitleChunkerParam{}),
	"manualchunker": manualChunkerParamKeys,
	"tokenchunker":  extractJSONTags(schema.TokenChunkerParam{}),
	"tokenizer":     extractJSONTags(schema.TokenizerParam{}),
}

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

// isCompilerComponent returns true if the component ID indicates a Compiler
// component, whose LLM-backed runtime parameters follow the same surface as
// the Extractor's.
func isCompilerComponent(cpnID string) bool {
	lowerID := strings.ToLower(cpnID)
	return strings.HasPrefix(lowerID, "compiler:") ||
		strings.HasPrefix(lowerID, "compiler_")
}

// getComponentParamWhitelist returns the dynamic parameter whitelist for a component type.
func getComponentParamWhitelist(cpnID string) (map[string]struct{}, bool) {
	if isExtractorComponent(cpnID, "") {
		return extractorValidParamKeys, true
	}
	if isCompilerComponent(cpnID) {
		return llmRuntimeParamKeys, true
	}
	return nil, false
}

// CleanComponentParams filters rawConfig against the DSL schema given by dslJSON.
// Keys containing ':' are treated as component IDs; they are kept only when both
// the cpnID AND the param name exist in one of the accepted-key sources: the DSL
// schema (the params the template bakes), the schema-derived per-component table
// (componentParamSchemaKeys), or the component's dynamic parameter schema (e.g.
// Extractor modular features). Keys without ':' (legacy flat fields) are dropped
// with a warning.
func CleanComponentParams(dslJSON []byte, rawConfig map[string]interface{}) map[string]interface{} {
	schemas, err := ExtractAllComponentParams(dslJSON)
	if err != nil {
		common.Warn("CleanComponentParams: failed to extract DSL schema, returning input as-is",
			zap.Error(err))
		return rawConfig
	}

	validCPNs := make(map[string]map[string]struct{}, len(schemas))
	componentNames := make(map[string]string, len(schemas))
	for _, s := range schemas {
		keys := make(map[string]struct{}, len(s.ParamsDefaults))
		for k := range s.ParamsDefaults {
			keys[k] = struct{}{}
		}
		if s.ComponentName == "GeneralChunker" {
			keys["delimiters"] = struct{}{}
		}
		if IsChunkerComponent(s.CpnID) {
			keys["enable_children"] = struct{}{}
		}
		for k := range componentParamSchemaKeys[strings.ToLower(s.ComponentName)] {
			keys[k] = struct{}{}
		}
		validCPNs[s.CpnID] = keys
		componentNames[s.CpnID] = s.ComponentName
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
		if componentNames[key] == "GeneralChunker" {
			params = normalizeGeneralComponentParams(params)
		}
		dynamicWhitelist, hasDynamic := getComponentParamWhitelist(key)
		if isExtractorComponent(key, "") {
			params = NormalizeExtractorParams(params)
		}
		cleaned := make(map[string]any, len(params))
		for pk, pv := range params {
			if _, ok := validKeys[pk]; ok {
				cleaned[pk] = pv
				continue
			}
			// Extractor and dynamic components: check reflection-driven whitelist
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

func normalizeGeneralComponentParams(params map[string]any) map[string]any {
	var normalized map[string]any
	clone := func() map[string]any {
		if normalized == nil {
			normalized = make(map[string]any, len(params)+1)
			for key, value := range params {
				normalized[key] = value
			}
		}
		return normalized
	}
	if _, hasCanonical := params["delimiters"]; !hasCanonical {
		if value, ok := params["delimiter"].(string); ok {
			normalizedParams := clone()
			normalizedParams["delimiters"] = parserchunk.ParseDelimiterField(value)
			delete(normalizedParams, "delimiter")
		}
	} else if value, ok := params["delimiters"].(string); ok {
		clone()["delimiters"] = parserchunk.ParseDelimiterField(value)
	}
	if value, ok := params["children_delimiters"].(string); ok {
		clone()["children_delimiters"] = parserchunk.ParseDelimiterField(value)
	}
	if normalized == nil {
		return params
	}
	return normalized
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

// ApplyParentChildChunkerConfig derives runtime children_delimiters from the
// top-level parent_child setting unless the request explicitly configures the
// chunker itself.
func ApplyParentChildChunkerConfig(componentConfig entity.JSONMap, rawConfig map[string]interface{}) {
	parentChild, ok := rawConfig["parent_child"].(map[string]interface{})
	if !ok {
		return
	}
	componentConfig["parent_child"] = parentChild
	useParentChild, _ := parentChild["use_parent_child"].(bool)
	childrenDelimiters := []string{}
	if useParentChild {
		if delimiter, ok := parentChild["children_delimiter"].(string); ok {
			childrenDelimiters = parserchunk.ParseDelimiterField(delimiter)
		}
	}

	for componentID, value := range componentConfig {
		if !IsChunkerComponent(componentID) {
			continue
		}
		if requested, ok := rawConfig[componentID].(map[string]interface{}); ok {
			if _, provided := requested["children_delimiters"]; provided {
				continue
			}
		}
		params, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		params["children_delimiters"] = childrenDelimiters
	}
}

// IsChunkerComponent reports whether a pipeline component can split parent
// chunks into children.
func IsChunkerComponent(componentID string) bool {
	lowerID := strings.ToLower(componentID)
	return strings.HasPrefix(lowerID, "generalchunker:") || strings.HasPrefix(lowerID, "tokenchunker:")
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
