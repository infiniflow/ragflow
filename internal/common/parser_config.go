package common

import "strings"

// InjectExtractorLLMID finds all Extractor component entries (keys prefixed
// with "extractor:" or "extractor_") in parserConfig and sets their llm_id
// to the given value. Returns whether any entry was updated.
func InjectExtractorLLMID(parserConfig map[string]interface{}, llmID string) bool {
	if parserConfig == nil || llmID == "" {
		return false
	}
	updated := false
	for cid, raw := range parserConfig {
		compMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cidLower := strings.ToLower(cid)
		if strings.HasPrefix(cidLower, "extractor:") || strings.HasPrefix(cidLower, "extractor_") {
			compMap["llm_id"] = llmID
			updated = true
		}
	}
	return updated
}

// deepCopyMap duplicates a JSON-like map so later merges do not mutate shared defaults.
func deepCopyMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}

	cloned := make(map[string]interface{}, len(source))
	for key, value := range source {
		cloned[key] = deepCopyValue(value)
	}
	return cloned
}

// deepCopyValue recursively copies nested maps and slices inside parser_config values.
func deepCopyValue(value interface{}) interface{} {
	switch typedValue := value.(type) {
	case map[string]interface{}:
		return deepCopyMap(typedValue)
	case []interface{}:
		cloned := make([]interface{}, len(typedValue))
		for idx, item := range typedValue {
			cloned[idx] = deepCopyValue(item)
		}
		return cloned
	default:
		return typedValue
	}
}

// DeepMergeMaps applies override onto base while preserving nested defaults such as raptor/graphrag.
func DeepMergeMaps(base, override map[string]interface{}) map[string]interface{} {
	merged := deepCopyMap(base)
	if merged == nil {
		merged = make(map[string]interface{})
	}
	if override == nil {
		return merged
	}

	for key, value := range override {
		overrideMap, overrideIsMap := value.(map[string]interface{})
		existingMap, existingIsMap := merged[key].(map[string]interface{})
		if overrideIsMap && existingIsMap {
			merged[key] = DeepMergeMaps(existingMap, overrideMap)
			continue
		}
		merged[key] = deepCopyValue(value)
	}
	return merged
}

// GetParserConfig builds the final parser_config stored on a dataset:
// base defaults -> chunk-method defaults -> caller overrides.
func GetParserConfig(parserID string, parserConfig map[string]interface{}) map[string]interface{} {
	baseDefaults := map[string]interface{}{
		"table_context_size": 0,
		"image_context_size": 0,
	}

	defaultConfigs := map[string]map[string]interface{}{
		"naive": {
			"layout_recognize": "DeepDOC",
			"chunk_token_num":  512,
			"delimiter":        "\n",
			"auto_keywords":    0,
			"auto_questions":   0,
			"html4excel":       false,
			"topn_tags":        3,
		},
		"qa":           nil,
		"resume":       nil,
		"manual":       nil,
		"paper":        nil,
		"book":         nil,
		"laws":         nil,
		"presentation": nil,
	}

	merged := DeepMergeMaps(baseDefaults, defaultConfigs[parserID])
	return DeepMergeMaps(merged, parserConfig)
}

func ExtractPipelineDefaults(dsl map[string]interface{}) map[string]interface{} {
	if dsl == nil {
		return nil
	}
	if inner, ok := dsl["dsl"].(map[string]interface{}); ok {
		dsl = inner
	}
	components, _ := dsl["components"].(map[string]interface{})
	if components == nil {
		return nil
	}

	result := make(map[string]interface{})
	hasAny := false
	for cid, compVal := range components {
		compMap, ok := compVal.(map[string]interface{})
		if !ok {
			continue
		}
		obj, _ := compMap["obj"].(map[string]interface{})
		if obj == nil {
			continue
		}
		name, _ := obj["component_name"].(string)
		if name == "" || name == "File" {
			continue
		}
		params, _ := obj["params"].(map[string]interface{})
		if params == nil {
			continue
		}
		copy_ := deepCopyMap(params)
		delete(copy_, "outputs")
		result[cid] = copy_
		hasAny = true
	}
	if !hasAny {
		return nil
	}
	return result
}
