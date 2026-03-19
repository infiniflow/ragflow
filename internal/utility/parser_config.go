//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package utility

// CloneMap deep-copies a map[string]interface{} value.
func CloneMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return nil
	}

	cloned := make(map[string]interface{}, len(source))
	for key, value := range source {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

// DeepMergeMaps recursively merges override into base.
func DeepMergeMaps(base, override map[string]interface{}) map[string]interface{} {
	merged := CloneMap(base)
	if merged == nil {
		merged = make(map[string]interface{})
	}

	for key, value := range override {
		existingValue, exists := merged[key]
		overrideMap, overrideIsMap := value.(map[string]interface{})
		existingMap, existingIsMap := existingValue.(map[string]interface{})
		if exists && overrideIsMap && existingIsMap {
			merged[key] = DeepMergeMaps(existingMap, overrideMap)
			continue
		}
		merged[key] = cloneValue(value)
	}
	return merged
}

// RemapKeys renames keys based on aliases while keeping unspecified keys unchanged.
func RemapKeys(source map[string]interface{}, aliases map[string]string) map[string]interface{} {
	remapped := make(map[string]interface{}, len(source))
	for key, value := range source {
		if mappedKey, ok := aliases[key]; ok {
			remapped[mappedKey] = value
			continue
		}
		remapped[key] = value
	}
	return remapped
}

// GetParserConfig builds the persisted parser configuration for a chunk method.
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

	merged := CloneMap(baseDefaults)
	defaultConfig, hasDefault := defaultConfigs[chunkMethod]
	if hasDefault {
		merged = DeepMergeMaps(merged, defaultConfig)
	}
	if parserConfig != nil {
		merged = DeepMergeMaps(merged, parserConfig)
	}
	return merged
}

func cloneValue(value interface{}) interface{} {
	switch typedValue := value.(type) {
	case map[string]interface{}:
		return CloneMap(typedValue)
	case []interface{}:
		cloned := make([]interface{}, len(typedValue))
		for idx, item := range typedValue {
			cloned[idx] = cloneValue(item)
		}
		return cloned
	default:
		return typedValue
	}
}
