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

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// JSONFloat64 is a float64 that always marshals with decimal point
type JSONFloat64 float64

func (f JSONFloat64) MarshalJSON() ([]byte, error) {
	// Always output with decimal point (e.g., 0.0 instead of 0)
	return []byte(fmt.Sprintf("%.1f", float64(f))), nil
}

// GetProjectBaseDirectory returns the current working directory.
// If an error occurs while getting the current directory, it returns ".".
//
// Returns:
//   - string: The current working directory path, or "." if an error occurs.
//
// Example:
//
//	baseDir := utility.GetProjectBaseDirectory()
//	configPath := filepath.Join(baseDir, "conf", "config.json")
func GetProjectBaseDirectory() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// StringPtr converts a string to a pointer of string.
// If the input string is empty, it returns nil.
//
// Parameters:
//   - s: The string to convert to a pointer.
//
// Returns:
//   - *string: A pointer to the input string, or nil if the input is empty.
//
// Example:
//
//	name := utility.StringPtr("example")  // returns &"example"
//	empty := utility.StringPtr("")        // returns nil
func StringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ParseInt64 parses a string to int64.
// If parsing fails, it returns 0.
//
// Parameters:
//   - s: The string to parse.
//
// Returns:
//   - int64: The parsed integer value, or 0 if parsing fails.
//
// Example:
//
//	val := utility.ParseInt64("123")   // returns 123
//	val := utility.ParseInt64("abc")   // returns 0
//	val := utility.ParseInt64("")      // returns 0
func ParseInt64(s string) int64 {
	var result int64
	fmt.Sscanf(s, "%d", &result)
	return result
}

// FormatTime formats time for display
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return "N/A (Perpetual)"
	}
	return t.Format("2006-01-02 15:04:05")
}

// FormatTimeToString converts time.Time to string in specified format
func FormatTimeToString(t *time.Time, format string) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(format)
}

// ConvertHexToPositionIntArray converts hex string to position int array (grouped by 5)
func ConvertHexToPositionIntArray(hexStr string) interface{} {
	if hexStr == "" {
		return nil
	}

	parts := strings.Split(hexStr, "_")
	var intVals []int
	for _, part := range parts {
		if part == "" {
			continue
		}
		val, err := strconv.ParseInt(part, 16, 64)
		if err != nil {
			continue
		}
		intVals = append(intVals, int(val))
	}

	if len(intVals) == 0 {
		return nil
	}

	// Group by 5 elements
	var result [][]int
	for i := 0; i < len(intVals); i += 5 {
		end := i + 5
		if end > len(intVals) {
			end = len(intVals)
		}
		result = append(result, intVals[i:end])
	}

	return result
}

// ConvertPositionIntArrayToHex converts position_int list (2D) to hex string
// e.g. [[1,2],[3,4]] -> "0000000100000002_0000000300000004"
func ConvertPositionIntArrayToHex(list []interface{}) string {
	var hexParts []string
	for _, item := range list {
		if inner, ok := item.([]interface{}); ok {
			for _, num := range inner {
				if n, ok := num.(float64); ok {
					hexParts = append(hexParts, fmt.Sprintf("%08x", int64(n)))
				} else if n, ok := num.(int64); ok {
					hexParts = append(hexParts, fmt.Sprintf("%08x", n))
				} else if n, ok := num.(int); ok {
					hexParts = append(hexParts, fmt.Sprintf("%08x", n))
				}
			}
		}
	}
	return strings.Join(hexParts, "_")
}

// ConvertHexToIntArray converts hex string to int array (split by "_")
func ConvertHexToIntArray(hexStr string) interface{} {
	if hexStr == "" {
		return nil
	}

	parts := strings.Split(hexStr, "_")
	var result []int
	for _, part := range parts {
		if part == "" {
			continue
		}
		val, err := strconv.ParseInt(part, 16, 64)
		if err != nil {
			continue
		}
		result = append(result, int(val))
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ConvertIntArrayToHex converts int array to hex string
// e.g. [1, 2] -> "00000001_00000002"
func ConvertIntArrayToHex(list []interface{}) string {
	var hexParts []string
	for _, num := range list {
		if n, ok := num.(float64); ok {
			hexParts = append(hexParts, fmt.Sprintf("%08x", int64(n)))
		} else if n, ok := num.(int64); ok {
			hexParts = append(hexParts, fmt.Sprintf("%08x", n))
		} else if n, ok := num.(int); ok {
			hexParts = append(hexParts, fmt.Sprintf("%08x", n))
		}
	}
	return strings.Join(hexParts, "_")
}

// IsEmpty checks if value is empty (nil, empty array, or empty string)
func IsEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	if arr, ok := v.([]interface{}); ok {
		return len(arr) == 0
	}
	if arr, ok := v.([]string); ok {
		return len(arr) == 0
	}
	if arr, ok := v.([]int); ok {
		return len(arr) == 0
	}
	if strVal, ok := v.(string); ok && strVal == "" {
		return true
	}
	return false
}

// SetFieldArray copies value to dest key, or sets empty array if value is empty
func SetFieldArray(result map[string]interface{}, destKey string, v interface{}) {
	if IsEmpty(v) {
		result[destKey] = []interface{}{}
	} else {
		result[destKey] = v
	}
}

// ToFloat64 converts various types to float64
func ToFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// ConvertToStringSlice converts an interface{} to []string
// e.g. []interface{}{"a", "b", "c"} -> []string{"a", "b", "c"}
// e.g. "hello" -> []string{"hello"}
func ConvertToStringSlice(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			} else {
				result = append(result, fmt.Sprintf("%v", item))
			}
		}
		return result
	case []string:
		return val
	case string:
		return []string{val}
	default:
		return nil
	}
}

// ConvertToString converts an interface{} to space-separated string
// For []interface{}, joins elements with space; for other types, returns string representation
// e.g. []interface{}{"a", "b", "c"} -> "a b c"
// e.g. "hello" -> "hello"
func ConvertToString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			} else {
				parts = append(parts, fmt.Sprintf("%v", item))
			}
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ConvertMapToJSONString converts a map to JSON string for Infinity JSON columns
// If v is a map[string]interface{}, marshals it to JSON string
// If v is nil, returns "{}"
// Otherwise returns v as-is
//
// e.g. map[string]interface{}{"key": "value"}) -> `"{\"key\":\"value\"}"`
func ConvertMapToJSONString(v interface{}) interface{} {
	if v == nil {
		return "{}"
	}
	if m, ok := v.(map[string]interface{}); ok {
		jsonBytes, _ := json.Marshal(m)
		return string(jsonBytes)
	}
	return v
}