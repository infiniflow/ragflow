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
