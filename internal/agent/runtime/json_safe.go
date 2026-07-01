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

// json_safe.go — Safe JSON marshaling with fallback for non-serializable
// values, mirroring the Python PR #14210 approach.
//
// Agent components may store functools.partial-like objects (function
// closures) or non-copyable client objects in their output slots. These
// values leak into canvas state maps and must not crash json.Marshal.
// This file provides helpers that recursively clean state maps before
// serialization, converting function values and channels to nil.
package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/common"
)

// SafeJSONMarshal marshals v to JSON after recursively replacing
// non-serializable Go values (func, chan) with nil. This mirrors the
// Python PR #14210 _serialize_default / _canvas_json_default fallback
// that converts callables to None.
//
// Use this in preference to json.Marshal when the input is a canvas
// state map (map[string]any) that may contain function closures,
// channels, or other types encoding/json cannot handle.
func SafeJSONMarshal(v any) ([]byte, error) {
	cleaned := cleanForJSON(v)
	return json.Marshal(cleaned)
}

// cleanForJSON recursively replaces non-JSON-serializable values with nil.
func cleanForJSON(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[string]any:
		return cleanMap(val)
	case map[string]map[string]any:
		out := make(map[string]map[string]any, len(val))
		for k, vv := range val {
			out[k] = cleanMap(vv)
		}
		return out
	case []map[string]any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = cleanMap(vv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = cleanForJSON(vv)
		}
		return out
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() {
			return nil
		}
		switch rv.Kind() {
		case reflect.Func, reflect.Chan:
			common.Info("cleanForJSON: converting non-serializable value to nil",
				zap.String("type", rv.Type().String()))
			return nil
		case reflect.Map:
			// Generic map fallback (e.g., map[string]int, etc.).
			// Build as map[string]any because cleaned values may
			// change type (func→nil, typed slice→[]any, etc.) and
			// SetMapIndex on an original-typed map would panic.
			cleaned := make(map[string]any, rv.Len())
			for iter := rv.MapRange(); iter.Next(); {
				key := fmt.Sprint(iter.Key().Interface())
				cleaned[key] = cleanForJSON(iter.Value().Interface())
			}
			return cleaned
		case reflect.Slice, reflect.Array:
			length := rv.Len()
			cleaned := make([]any, length)
			for i := 0; i < length; i++ {
				cleaned[i] = cleanForJSON(rv.Index(i).Interface())
			}
			return cleaned
		case reflect.Ptr:
			if rv.IsNil() {
				return nil
			}
			return cleanForJSON(rv.Elem().Interface())
		case reflect.Interface:
			if rv.IsNil() {
				return nil
			}
			return cleanForJSON(rv.Elem().Interface())
		case reflect.Struct:
			cleaned := make(map[string]any, rv.NumField())
			t := rv.Type()
			for i := 0; i < rv.NumField(); i++ {
				field := t.Field(i)
				if !field.IsExported() {
					continue
				}
				fv := rv.Field(i)
				if !fv.CanInterface() {
					continue
				}
				key := field.Name
				if tag := field.Tag.Get("json"); tag != "" {
					name, _, _ := strings.Cut(tag, ",")
					if name == "-" {
						continue
					}
					if name != "" {
						key = name
					}
				}
				cleaned[key] = cleanForJSON(fv.Interface())
			}
			return cleaned
		default:
			return v
		}
	}
}

func cleanMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = cleanForJSON(v)
	}
	return out
}
