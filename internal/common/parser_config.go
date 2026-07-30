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
			if current, ok := compMap["llm_id"].(string); !ok || current == "" {
				compMap["llm_id"] = llmID
				updated = true
			}
		}
	}
	return updated
}

// InjectExtractorEnableMetadata enables auto-metadata (enable_metadata) extraction
// on every Extractor node when the dataset has enable_metadata on and a
// non-empty field set (metadata and/or built_in_metadata). The dataset-level
// enable_metadata flag is authoritative (mirrors Python task_executor.py:519,
// which reads parser_config directly and never consults a per-node flag): a
// shipped DSL that defaults enable_metadata to 0 is still turned on. Only a
// node the user already turned ON (enable_metadata truthy) is left untouched,
// so an explicit per-node enablement keeps its own config. The field schema is
// taken from parserConfig["metadata"] and parserConfig["built_in_metadata"]
// (combined). Returns whether any entry was updated.
func InjectExtractorEnableMetadata(parserConfig map[string]interface{}) bool {
	if parserConfig == nil {
		return false
	}
	if !isTruthy(parserConfig["enable_metadata"]) {
		return false
	}
	fields := metadataFieldDefs(parserConfig)
	if len(fields) == 0 {
		return false
	}
	updated := false
	for cid, raw := range parserConfig {
		compMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cidLower := strings.ToLower(cid)
		if !strings.HasPrefix(cidLower, "extractor:") && !strings.HasPrefix(cidLower, "extractor_") {
			continue
		}
		// The dataset-level enable_metadata flag is authoritative (mirrors
		// Python task_executor.py:519, which reads parser_config directly and
		// never consults a per-node flag). Only a node the user already turned
		// ON (truthy) is left alone; a shipped DSL that defaults the field to
		// 0 must still be enabled by the dataset flag, otherwise auto-metadata
		// could never turn on for any of the built-in pipelines.
		if isTruthy(compMap["enable_metadata"]) {
			continue
		}
		compMap["enable_metadata"] = 1
		compMap["metadata"] = fields
		updated = true
	}
	return updated
}

// metadataFieldDefs combines parserConfig["metadata"] and
// parserConfig["built_in_metadata"] into the field list injected as
// metadata (each entry keeps key/type/description/enum, mirroring the
// stored shape from dataset/helpers.go normalizeMetadataConfigFields).
//
// It returns []any (i.e. []interface{}) rather than []map[string]interface{}
// because the injected value is handed to NewExtractorComponent, which reads
// params["metadata"].([]any). A []map[string]interface{} value would fail that
// type assertion (Go slice types are not covariant) and the field schema would
// be silently dropped, so auto-metadata never reached ExtractorParam.Metadata.
func metadataFieldDefs(parserConfig map[string]interface{}) []any {
	var out []any
	for _, key := range []string{"metadata", "built_in_metadata"} {
		raw, ok := parserConfig[key].([]interface{})
		if !ok {
			continue
		}
		for _, item := range raw {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			k, _ := m["key"].(string)
			if strings.TrimSpace(k) == "" {
				continue
			}
			out = append(out, m)
		}
	}
	return out
}

// isTruthy reports whether a parserConfig flag (e.g. enable_metadata) is on.
// It tolerates bool, numeric >0 and the strings "true"/"1" so storage
// representation differences don't silently disable the feature.
func isTruthy(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true" || t == "1" || t == "True" || t == "TRUE"
	case float64:
		return t > 0
	case int:
		return t > 0
	case int64:
		return t > 0
	}
	return false
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
