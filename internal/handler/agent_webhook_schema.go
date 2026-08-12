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

package handler

// Webhook schema helpers.
//
// These mirror api/apps/restful_apis/agent_api.py:1896-2051 (extract_by_schema,
// default_for_type, auto_cast_value, validate_type) from the Python
// webhook handler. The Go port preserves the Python semantics exactly:
// required fields raise; optional fields get a type-based default;
// type coercion runs before validation.
//
// Edge cases preserved:
//   - "file" type maps to []any (a list of parsed file descriptors).
//   - "array" accepts a JSON-decoded list; "array<inner>" validates each
//     element against `inner`.
//   - "object" must decode to map[string]any.
//   - Empty data sections yield an empty map (NOT nil) so downstream
//     components can index without a nil check.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// defaultForType returns the schema-default value for the given type
// token. Mirrors python's `default_for_type` at agent_api.py:1939-1955.
//
// Special cases:
//   - "file"        → []any{} (a list, the python empty list)
//   - "object"      → map[string]any{} (a dict)
//   - "boolean"     → false
//   - "number"      → 0 (float64 to match python's int/float unification)
//   - "string"      → ""
//   - "array"/"array<...>" → []any{}
//   - "null"        → nil
//   - anything else → nil
func defaultForType(t string) any {
	switch t {
	case "file":
		return []any{}
	case "object":
		return map[string]any{}
	case "boolean":
		return false
	case "number":
		return float64(0)
	case "string":
		return ""
	case "null":
		return nil
	}
	if strings.HasPrefix(t, "array") {
		return []any{}
	}
	return nil
}

// autoCastValue mirrors python's auto_cast_value at agent_api.py:1957-2016.
// It accepts already-decoded values unchanged and converts string values
// to the schema-expected type when possible.
//
// Returns an error when conversion is impossible (e.g. "abc" → number).
func autoCastValue(value any, expectedType string) (any, error) {
	if _, isString := value.(string); !isString {
		// Non-string values are passed through unchanged; the Python
		// reference only converts when the source is a string.
		return value, nil
	}
	v := strings.TrimSpace(value.(string))
	switch expectedType {
	case "boolean":
		switch strings.ToLower(v) {
		case "true", "1":
			return true, nil
		case "false", "0":
			return false, nil
		}
		return nil, fmt.Errorf("cannot convert %q to boolean", value)
	case "number":
		// Python tries int first, then float. In Go strconv.Atoi handles
		// int; for float we fall back to ParseFloat. Negative integers
		// must be supported (the python `v[1:].isdigit()` guard does
		// that).
		if v != "" && (v[0] == '-' && len(v) > 1 || v[0] >= '0' && v[0] <= '9') {
			// Try int first (no fractional part, no exponent).
			if !strings.ContainsAny(v, ".eE") {
				if i, err := strconv.ParseInt(v, 10, 64); err == nil {
					return float64(i), nil
				}
			}
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("cannot convert %q to number", value)
		}
		return f, nil
	case "object":
		var parsed map[string]any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil, fmt.Errorf("cannot convert %q to object", value)
		}
		return parsed, nil
	}
	if strings.HasPrefix(expectedType, "array") {
		var parsed []any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return nil, fmt.Errorf("cannot convert %q to array", value)
		}
		return parsed, nil
	}
	// "string" / "file" / unknown — return as-is (matching the python
	// passthrough branches at the bottom of auto_cast_value).
	return value, nil
}

// validateType mirrors python's validate_type at agent_api.py:2019-2051.
// It returns true when `value` is structurally compatible with type `t`.
// Unknown types pass through (python parity: agent_api.py:2051).
func validateType(value any, t string) bool {
	if t == "" {
		return true
	}
	switch t {
	case "file":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case int, int32, int64, float32, float64:
			return true
		}
		return false
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "null":
		return value == nil
	}
	if strings.HasPrefix(t, "array") {
		list, ok := value.([]any)
		if !ok {
			return false
		}
		// array<string> / array<number> / array<object>: validate each
		// element against the inner type when <...> is present.
		if open := strings.Index(t, "<"); open >= 0 {
			if close := strings.Index(t[open:], ">"); close > 0 {
				inner := t[open+1 : open+close]
				if inner == "" {
					return true
				}
				for _, item := range list {
					if !validateType(item, inner) {
						return false
					}
				}
			}
		}
		return true
	}
	// Unknown type — pass through (python parity at line 2051).
	return true
}

// extractBySchema mirrors python's extract_by_schema at agent_api.py:1896-1936.
//
// Behaviour:
//   - `data` may be nil; treated as empty map.
//   - For each property in schema.properties:
//   - if required and missing → error
//   - if missing (optional) → use defaultForType(prop.type)
//   - if present → autoCastValue then validateType; mismatch raises
//   - returns the cleaned map; nil `data` returns an empty map (NOT nil).
func extractBySchema(data map[string]any, schema map[string]any, name string) (map[string]any, error) {
	if data == nil {
		data = map[string]any{}
	}
	if schema == nil {
		schema = map[string]any{}
	}

	props, _ := schema["properties"].(map[string]any)
	required := stringSlice(schema["required"])

	out := map[string]any{}

	for field, rawSchema := range props {
		fieldSchema, _ := rawSchema.(map[string]any)
		fieldType, _ := fieldSchema["type"].(string)

		// 1. Required field missing → error (python agent_api.py:1913).
		isRequired := contains(required, field)
		if isRequired {
			if _, ok := data[field]; !ok {
				return nil, fmt.Errorf("%s missing required field: %s", name, field)
			}
		}

		// 2. Optional → default value.
		raw, present := data[field]
		if !present {
			out[field] = defaultForType(fieldType)
			continue
		}

		// 3. Auto cast (only matters for string→typed; pass-through otherwise).
		casted, err := autoCastValue(raw, fieldType)
		if err != nil {
			return nil, fmt.Errorf("%s.%s auto-cast failed: %s", name, field, err.Error())
		}

		// 4. Type validation.
		if !validateType(casted, fieldType) {
			return nil, fmt.Errorf(
				"%s.%s type mismatch: expected %s, got %T",
				name, field, fieldType, casted,
			)
		}
		out[field] = casted
	}
	return out, nil
}

// stringSlice accepts either a []string or []any for the schema's
// `required` field. Mirrors the python list-or-tuple-or-set union
// handling at agent_api.py:1788-1798.
func stringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			if str, ok := item.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
