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

import (
	"strconv"
	"strings"
)

// MetaCondition represents a single parsed filter condition.
type MetaCondition struct {
	Operator string      // "=", "≠", ">", "<", "≥", "≤", "contains", "not contains", "in", "not in", "start with", "end with", "empty", "not empty"
	Key      string      // metadata field name
	Value    interface{} // comparison value
}

// ConvertConditions converts API-format conditions into internal format.
// Python equivalent: common/metadata_utils.py::convert_conditions()
func ConvertConditions(metadataCondition map[string]interface{}) []MetaCondition {
	if metadataCondition == nil {
		return nil
	}
	opMapping := map[string]string{
		"is":     "=",
		"not is": "≠",
		">=":     "≥",
		"<=":     "≤",
		"!=":     "≠",
	}

	rawConditions, ok := metadataCondition["conditions"].([]interface{})
	if !ok {
		return nil
	}

	var result []MetaCondition
	for _, raw := range rawConditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := cond["name"].(string)
		op, _ := cond["comparison_operator"].(string)
		if mapped, exists := opMapping[op]; exists {
			op = mapped
		}
		value := cond["value"]
		result = append(result, MetaCondition{
			Operator: op,
			Key:      name,
			Value:    value,
		})
	}
	return result
}

// MetaFilter applies filter conditions against metadata and returns matching doc IDs.
// Python equivalent: common/metadata_utils.py::meta_filter()
// metas: map[fieldName]map[fieldValue][]docID
// filters: output of ConvertConditions
// logic: "and" | "or" (defaults to "and")
func MetaFilter(metas map[string]map[string][]string, filters []MetaCondition, logic string) []string {
	if logic == "" {
		logic = "and"
	}

	var docIDs *map[string]struct{}

	for _, f := range filters {
		v2docs, ok := metas[f.Key]
		if !ok {
			// Key not found: no match
			if logic == "and" {
				return []string{}
			}
			continue
		}

		matched := filterOut(v2docs, f.Operator, f.Value)

		if docIDs == nil {
			s := make(map[string]struct{}, len(matched))
			for _, id := range matched {
				s[id] = struct{}{}
			}
			docIDs = &s
		} else {
			if logic == "and" {
				s := make(map[string]struct{})
				for _, id := range matched {
					if _, exists := (*docIDs)[id]; exists {
						s[id] = struct{}{}
					}
				}
				docIDs = &s
				if len(*docIDs) == 0 {
					return []string{}
				}
			} else {
				for _, id := range matched {
					(*docIDs)[id] = struct{}{}
				}
			}
		}
	}

	if docIDs == nil {
		return []string{}
	}
	result := make([]string, 0, len(*docIDs))
	for id := range *docIDs {
		result = append(result, id)
	}
	return result
}

// filterOut returns matching doc IDs for a single (value → matchedDocs) map and operator.
func filterOut(v2docs map[string][]string, operator string, value interface{}) []string {
	var ids []string
	for input, docids := range v2docs {
		if matchValue(input, operator, value) {
			ids = append(ids, docids...)
		}
	}
	return ids
}

// matchValue checks if a single metadata value matches the operator+value.
func matchValue(input string, operator string, value interface{}) bool {
	switch operator {
	case "empty":
		return input == ""
	case "not empty":
		return input != ""
	}

	valStr := toString(value)
	if valStr == "" && value != nil {
		valStr = ""
	}

	switch operator {
	case "contains":
		return strings.Contains(input, valStr)
	case "not contains":
		return !strings.Contains(input, valStr)
	case "start with":
		return strings.HasPrefix(strings.ToLower(input), strings.ToLower(valStr))
	case "end with":
		return strings.HasSuffix(strings.ToLower(input), strings.ToLower(valStr))
	case "in":
		if list, ok := value.([]interface{}); ok {
			for _, item := range list {
				if toString(item) == input {
					return true
				}
			}
		}
		return false
	case "not in":
		if list, ok := value.([]interface{}); ok {
			for _, item := range list {
				if toString(item) == input {
					return false
				}
			}
		}
		return true
	}

	// Comparison operators: =, ≠, >, <, ≥, ≤
	return compareValues(input, valStr, operator)
}

// compareValues handles numeric/date/string comparison.
// Matches Python's logic: try numeric → try date → fall back to string.
func compareValues(a, b, operator string) bool {
	// Try numeric comparison
	af, errA := strconv.ParseFloat(a, 64)
	bf, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		return compareFloat(af, bf, operator)
	}

	// Try date comparison (YYYY-MM-DD format)
	if isDate(a) && isDate(b) {
		return compareString(a, b, operator)
	}

	// Fall back to case-insensitive string comparison
	return compareString(strings.ToLower(a), strings.ToLower(b), operator)
}

func compareFloat(a, b float64, operator string) bool {
	switch operator {
	case "=":
		return a == b
	case "≠":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case "≥":
		return a >= b
	case "≤":
		return a <= b
	}
	return false
}

func compareString(a, b string, operator string) bool {
	switch operator {
	case "=":
		return a == b
	case "≠":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case "≥":
		return a >= b
	case "≤":
		return a <= b
	}
	return false
}

// isDate checks if a string is in YYYY-MM-DD format.
func isDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	if s[4] != '-' || s[7] != '-' {
		return false
	}
	for i := 0; i < 10; i++ {
		if i == 4 || i == 7 {
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// toString converts a value to string for comparison.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		if s {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}
