// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// integerParams lists persisted Agent DSL fields consumed as integer counts by
// their component implementations. Decimal-valued component parameters are
// deliberately absent.
var integerParams = map[string]map[string]struct{}{
	"agent": {
		"max_retries":                 {},
		"max_rounds":                  {},
		"max_tokens":                  {},
		"message_history_window_size": {},
		"optimize_history_window":     {},
	},
	"browser": {
		"max_retries":                 {},
		"max_steps":                   {},
		"max_tokens":                  {},
		"message_history_window_size": {},
		"timeout":                     {},
	},
	"categorize": {
		"max_retries":                 {},
		"max_tokens":                  {},
		"message_history_window_size": {},
	},
	"docgenerator":   {"font_size": {}},
	"docsgenerator":  {"font_size": {}},
	"invoke":         {"max_retries": {}},
	"iteration":      {"max_concurrency": {}},
	"listoperations": {"n": {}},
	"llm": {
		"max_retries":                 {},
		"max_tokens":                  {},
		"message_history_window_size": {},
	},
	"loop":              {"maximum_loop_count": {}},
	"parallel":          {"max_concurrency": {}},
	"retrieval":         {"top_k": {}, "top_n": {}},
	"search_my_dataset": {"top_k": {}, "top_n": {}},
	"search_my_dateset": {"top_k": {}, "top_n": {}},
	"searchmydateset":   {"top_k": {}, "top_n": {}},
	"searchmydataset":   {"top_k": {}, "top_n": {}},
}

// ValidateIntegerParameters rejects fractional numeric values in the Agent
// configuration fields whose component contract requires an integer. Agent
// update requests preserve numeric JSON lexemes as json.Number, so 1.0 and
// fractional values are rejected instead of being coerced.
func ValidateIntegerParameters(dsl map[string]any) error {
	return validateIntegerParameters(dsl, "")
}

func validateIntegerParameters(value any, path string) error {
	switch node := value.(type) {
	case map[string]any:
		if err := validateComponentIntegerParameters(node, path); err != nil {
			return err
		}
		for key, child := range node {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if err := validateIntegerParameters(child, childPath); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range node {
			if err := validateIntegerParameters(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateComponentIntegerParameters(node map[string]any, path string) error {
	name, _ := node["component_name"].(string)
	if name == "" {
		name, _ = node["name"].(string)
	}
	params, _ := node["params"].(map[string]any)
	if name == "" || params == nil {
		return nil
	}
	fields := integerParams[strings.ToLower(name)]
	for field := range fields {
		value, ok := params[field]
		if !ok || value == nil || isIntegerNumber(value) {
			continue
		}
		if isNonIntegerNumber(value) {
			return fmt.Errorf("component %q parameter %q must be an integer", componentPath(path, name), field)
		}
	}
	return nil
}

func componentPath(path, name string) string {
	path = strings.TrimSuffix(path, ".obj")
	if path == "" {
		return name
	}
	return path
}

func isIntegerNumber(value any) bool {
	switch n := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case json.Number:
		_, err := n.Int64()
		return err == nil
	case float32:
		return !math.IsNaN(float64(n)) && !math.IsInf(float64(n), 0) && math.Trunc(float64(n)) == float64(n)
	case float64:
		return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Trunc(n) == n
	default:
		return false
	}
}

func isNonIntegerNumber(value any) bool {
	switch n := value.(type) {
	case json.Number:
		_, err := n.Int64()
		return err != nil
	case float32:
		return !math.IsNaN(float64(n)) && !math.IsInf(float64(n), 0) && math.Trunc(float64(n)) != float64(n)
	case float64:
		return !math.IsNaN(n) && !math.IsInf(n, 0) && math.Trunc(n) != n
	default:
		return false
	}
}
