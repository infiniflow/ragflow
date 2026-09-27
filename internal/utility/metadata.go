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

		// Merge with existing value, mirroring Python
		// common.metadata_utils.update_metadata_to exactly:
		//   - both lists: extend + dedupe
		//   - target is list, incoming is scalar: append + dedupe
		//   - target is scalar (or any non-list): overwrite with the
		//     incoming value (the stored/merged-in side wins), NOT a list.
		targetList, targetIsList := existing.([]string)
		normList, normIsList := normVal.([]string)
		if targetIsList {
			if normIsList {
				target[k] = dedupeStrings(append(targetList, normList...))
			} else if s, ok := normVal.(string); ok {
				target[k] = dedupeStrings(append(targetList, s))
			}
			continue
		}
		// target is a scalar: overwrite with the incoming value.
		target[k] = normVal
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
