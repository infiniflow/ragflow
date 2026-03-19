package common

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
func GetParserConfig(chunkMethod string, parserConfig map[string]interface{}) map[string]interface{} {
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
			"raptor": map[string]interface{}{
				"use_raptor":  true,
				"prompt":      "Please summarize the following paragraphs. Be careful with the numbers, do not make things up. Paragraphs as following:\n      {cluster_content}\nThe above is the content you need to summarize.",
				"max_token":   256,
				"threshold":   0.1,
				"max_cluster": 64,
				"random_seed": 0,
			},
			"graphrag": map[string]interface{}{
				"use_graphrag": true,
				"entity_types": []interface{}{"organization", "person", "geo", "event", "category"},
				"method":       "light",
			},
		},
		"qa": {
			"raptor":   map[string]interface{}{"use_raptor": false},
			"graphrag": map[string]interface{}{"use_graphrag": false},
		},
		"resume": nil,
		"manual": {
			"raptor":   map[string]interface{}{"use_raptor": false},
			"graphrag": map[string]interface{}{"use_graphrag": false},
		},
		"paper": {
			"raptor":   map[string]interface{}{"use_raptor": false},
			"graphrag": map[string]interface{}{"use_graphrag": false},
		},
		"book": {
			"raptor":   map[string]interface{}{"use_raptor": false},
			"graphrag": map[string]interface{}{"use_graphrag": false},
		},
		"laws": {
			"raptor":   map[string]interface{}{"use_raptor": false},
			"graphrag": map[string]interface{}{"use_graphrag": false},
		},
		"presentation": {
			"raptor":   map[string]interface{}{"use_raptor": false},
			"graphrag": map[string]interface{}{"use_graphrag": false},
		},
		"knowledge_graph": {
			"chunk_token_num": 8192,
			"delimiter":       "\\n",
			"entity_types":    []interface{}{"organization", "person", "location", "event", "time"},
			"raptor":          map[string]interface{}{"use_raptor": false},
			"graphrag":        map[string]interface{}{"use_graphrag": false},
		},
	}

	merged := DeepMergeMaps(baseDefaults, defaultConfigs[chunkMethod])
	return DeepMergeMaps(merged, parserConfig)
}
