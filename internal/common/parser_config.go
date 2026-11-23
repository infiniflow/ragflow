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

package common

// DeepMergeMaps applies override onto base while preserving nested defaults.
func DeepMergeMaps(base, override map[string]interface{}) map[string]interface{} {
	merged := DeepCopyMap(base)
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
			"topn_tags":        3,
		},
		"qa":           nil,
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
		if name == "" || name == ComponentNameFile {
			continue
		}
		params, _ := obj["params"].(map[string]interface{})
		if params == nil {
			continue
		}
		copy_ := DeepCopyMap(params)
		delete(copy_, "outputs")
		result[cid] = copy_
		hasAny = true
	}
	if !hasAny {
		return nil
	}
	return result
}
