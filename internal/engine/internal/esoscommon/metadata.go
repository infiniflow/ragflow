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

package esoscommon

import (
	"encoding/json"
	"fmt"
)

// DedupeStrings returns the unique values of s, preserving first-seen order.
func DedupeStrings(s []string) []string {
	if len(s) == 0 {
		return s
	}
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// TotalHitsFromSearchResponse extracts the exact total hit count from a search
// response. Returns ok=false when the field is missing or in an unexpected
// shape; callers should treat that as "cannot verify" rather than "no overflow".
func TotalHitsFromSearchResponse(searchResponse map[string]interface{}) (int64, bool) {
	hitsRoot, ok := searchResponse["hits"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	totalRoot, ok := hitsRoot["total"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	// JSON numeric values decode as float64 in Go's default decoder.
	if v, ok := totalRoot["value"].(float64); ok {
		return int64(v), true
	}
	// Some clients / older versions return int64 directly.
	if v, ok := totalRoot["value"].(int64); ok {
		return v, true
	}
	if v, ok := totalRoot["value"].(json.Number); ok {
		n, err := v.Int64()
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

// BuildMetadataIndexName returns the metadata index name for a tenant.
func BuildMetadataIndexName(tenantID string) string {
	return fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
}
