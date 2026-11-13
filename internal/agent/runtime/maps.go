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

package runtime

import "encoding/json"

// IntFromMap returns raw[key] as an int, tolerating the numeric types JSON
// decoding and the doc engine produce (float64, float32, int, int64). Shared by
// the retrieval tools so chunk metadata is parsed uniformly.
func IntFromMap(raw map[string]any, key string) int {
	if f, ok := NumberFromMap(raw, key); ok {
		return int(f)
	}
	return 0
}

// StringFromMap returns raw[key].(string) or "" if missing / wrong type.
func StringFromMap(raw map[string]any, key string) string {
	if v, ok := raw[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// FirstStringFromMap returns the first non-empty string among the given keys.
func FirstStringFromMap(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := StringFromMap(raw, key); value != "" {
			return value
		}
	}
	return ""
}

// NumberFromMap returns raw[key].(float64) with a tolerant path for ints, JSON
// numbers and json.Number (which arises when decoding with UseNumber).
func NumberFromMap(raw map[string]any, key string) (float64, bool) {
	v, ok := raw[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		if f, err := x.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
}
