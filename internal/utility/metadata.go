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

import "encoding/json"

// UpdateMetadataTo merges metadata into an existing metadata map.
// String lists are merged and deduplicated. Incoming scalars overwrite
// existing values; objects and structured lists are added only for missing keys.
func UpdateMetadataTo(target map[string]any, meta any) map[string]any {
	if target == nil {
		return nil
	}
	if meta == nil {
		return target
	}

	metaMap, ok := meta.(map[string]any)
	if !ok {
		return target
	}

	for k, v := range metaMap {
		normVal, accepted := normalizeMetaValue(v)
		if !accepted {
			continue
		}

		existing, exists := target[k]
		if !exists {
			target[k] = normVal
			continue
		}

		switch normVal.(type) {
		case nil, string, []string, bool, json.Number,
			int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
			float32, float64:
		default:
			// Structured values never replace an existing key.
			continue
		}
		targetList, targetIsList := metadataStringList(existing)
		normList, normIsList := normVal.([]string)
		_, normIsString := normVal.(string)
		if normIsList || normIsString {
			if _, structuredList := existing.([]any); structuredList && !targetIsList {
				continue
			}
		}
		if targetIsList && (normIsList || normIsString) {
			if normIsList {
				target[k] = dedupeStrings(append(targetList, normList...))
			} else if s, ok := normVal.(string); ok {
				target[k] = dedupeStrings(append(targetList, s))
			}
			continue
		}
		// Incoming scalars, including false, zero and null, win.
		target[k] = normVal
	}

	return target
}

// normalizeMetaValue filters empty strings without changing other value types.
// The acceptance flag distinguishes a retained null from a skipped value.
func normalizeMetaValue(v any) (any, bool) {
	if s, ok := v.(string); ok {
		return s, s != ""
	}
	if list, ok := metadataStringList(v); ok {
		filtered := make([]string, 0, len(list))
		for _, s := range list {
			if s != "" {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return nil, false
		}
		return dedupeStrings(filtered), true
	}
	return v, true
}

// metadataStringList recognizes both typed and JSON-decoded string lists.
func metadataStringList(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		return list, true
	case []any:
		strings := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			strings = append(strings, s)
		}
		return strings, true
	default:
		return nil, false
	}
}

// dedupeStrings removes duplicates while preserving order.
func dedupeStrings(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, s := range input {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
