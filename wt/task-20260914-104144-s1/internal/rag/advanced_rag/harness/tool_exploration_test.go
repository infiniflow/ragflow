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

package harness

import (
	"context"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// kgIndexCapturingEngine records the index names of every search it serves.
type kgIndexCapturingEngine struct {
	engine.DocEngine
	indexNames []string
}

func (e *kgIndexCapturingEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.indexNames = append(e.indexNames, req.IndexNames...)
	return &types.SearchResult{}, nil
}

// TestKGSearchUsesConfiguredIndexName pins that the knowledge-graph walk searches
// the SAME index the rest of the request reads: ExploreGraph loads its evidence
// with indexNameFor(deps.TenantID, deps.IndexName), so a tenant that overrides
// the index name must have it honoured by the KG rows too. Hardcoding
// ragflow_<tenantID> made the walk query one index and return passages loaded
// from another. Empty keeps the default, matching Python's search.index_name.
func TestKGSearchUsesConfiguredIndexName(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name      string
		indexName string
		want      string
	}{
		{"configured override", "custom_idx", "custom_idx"},
		{"default", "", "ragflow_tenantA"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, call := range []struct {
				name string
				fn   func(de engine.DocEngine)
			}{
				{"kgSearchRaw", func(de engine.DocEngine) {
					kgSearchRaw(ctx, de, "tenantA", "kb1", nil, "entity", "dataset", nil, nil, "", 5, tc.indexName)
				}},
				{"kgSearch", func(de engine.DocEngine) {
					kgSearch(ctx, de, "tenantA", "kb1", nil, "entity", "q", 5, "dataset", nil, "", 0, tc.indexName)
				}},
				{"kgSeedSearch", func(de engine.DocEngine) {
					kgSeedSearch(ctx, de, "tenantA", "kb1", nil, "q", "dataset", nil, tc.indexName)
				}},
			} {
				t.Run(call.name, func(t *testing.T) {
					de := &kgIndexCapturingEngine{}
					call.fn(de)
					if len(de.indexNames) == 0 {
						t.Fatal("no search was issued")
					}
					for _, got := range de.indexNames {
						if got != tc.want {
							t.Errorf("index = %q, want %q", got, tc.want)
						}
					}
				})
			}
		})
	}
}
