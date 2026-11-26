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

package opensearch

import (
	"fmt"
	"testing"
)

// relationExclusionChunk mirrors the writer's cap: 512 ids per must_not terms
// clause.
const relationExclusionTestChunk = 512

// TestBuildBoolQueryExcludesRelationWindowIDs drives the real condition parser
// with the writer's read-id exclusion: every id read so far must appear once
// under bool.must_not, chunked the same way, for exactly one result window and
// past it.
func TestBuildBoolQueryExcludesRelationWindowIDs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		total int
	}{
		{"exactly one result window", 10000},
		{"past the result window", 10400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := make([]string, tc.total)
			for i := range ids {
				ids[i] = fmt.Sprintf("id-%d", i)
			}
			var and []map[string]interface{}
			for start := 0; start < len(ids); start += relationExclusionTestChunk {
				end := min(start+relationExclusionTestChunk, len(ids))
				and = append(and, map[string]interface{}{"must_not": map[string]interface{}{"id": ids[start:end]}})
			}
			query := buildBoolQueryFromConditionRaw(map[string]interface{}{
				"knowledge_graph_kwd": "relation",
				"and":                 and,
			}, nil, false, false)
			if query == nil {
				t.Fatal("query is nil")
			}

			seen := map[string]int{}
			mustNot, maxTerms := 0, 0
			var walk func(v interface{})
			walk = func(v interface{}) {
				switch node := v.(type) {
				case map[string]interface{}:
					if _, ok := node["must_not"]; ok {
						mustNot++
					}
					if terms, ok := node["terms"]; ok {
						if fields, ok := terms.(map[string]interface{}); ok {
							switch list := fields["id"].(type) {
							case []string:
								maxTerms = max(maxTerms, len(list))
								for _, id := range list {
									seen[id]++
								}
							case []interface{}:
								maxTerms = max(maxTerms, len(list))
								for _, id := range list {
									seen[fmt.Sprint(id)]++
								}
							}
						}
					}
					for _, child := range node {
						walk(child)
					}
				case []interface{}:
					for _, child := range node {
						walk(child)
					}
				case []map[string]interface{}:
					for _, child := range node {
						walk(child)
					}
				}
			}
			walk(query)

			if want := (tc.total + relationExclusionTestChunk - 1) / relationExclusionTestChunk; mustNot != want {
				t.Errorf("must_not clauses = %d, want %d", mustNot, want)
			}
			if maxTerms > relationExclusionTestChunk {
				t.Errorf("one terms clause carries %d ids, want at most %d", maxTerms, relationExclusionTestChunk)
			}
			if len(seen) != tc.total {
				t.Errorf("excluded %d distinct ids, want %d", len(seen), tc.total)
			}
			for _, id := range ids {
				if seen[id] != 1 {
					t.Fatalf("id %q excluded %d times, want 1", id, seen[id])
				}
			}
		})
	}
}
