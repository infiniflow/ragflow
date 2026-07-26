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

// UpdateMetadataTo merges metadata into an existing metadata map.
// Only string and []string values are accepted. Existing keys are preserved
// (not overwritten). List values are merged and deduplicated.
// Mirrors Python: common.metadata_utils.update_metadata_to()
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
		normVal := normalizeMetaValue(v)
		if normVal == nil {
			continue
		}

		existing, exists := target[k]
		if !exists {
			target[k] = normVal
			continue
		}

		// Merge with existing value
		existStr, existIsStr := existing.(string)
		newStr, newIsStr := normVal.(string)
		existList, existIsList := existing.([]string)
		newList, newIsList := normVal.([]string)

		if existIsStr && newIsStr {
			// Both strings: convert to list, append
			target[k] = dedupeStrings(append([]string{existStr}, newStr))
		} else if existIsStr && newIsList {
			target[k] = dedupeStrings(append([]string{existStr}, newList...))
		} else if existIsList && newIsStr {
			target[k] = dedupeStrings(append(existList, newStr))
		} else if existIsList && newIsList {
			target[k] = dedupeStrings(append(existList, newList...))
		}
	}

	return target
}

// normalizeMetaValue normalizes a metadata value.
// Returns a string, []string, or nil if the value is not acceptable.
func normalizeMetaValue(v any) any {
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return val
	case []string:
		filtered := make([]string, 0, len(val))
		for _, s := range val {
			if s != "" {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return nil
		}
		return dedupeStrings(filtered)
	case []any:
		filtered := make([]string, 0, len(val))
		for _, elem := range val {
			if s, ok := elem.(string); ok && s != "" {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return nil
		}
		return dedupeStrings(filtered)
	default:
		return nil
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
