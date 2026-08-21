// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration

package elasticsearch

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// TestSearchByRegexp_MatchAndMiss exercises the ES-native regexp pushdown over a
// real Elasticsearch instance. It verifies:
//   - a matching regex returns the expected chunk(s);
//   - a non-matching regex returns zero chunks;
//   - the case-insensitive flag behaves as documented;
//   - the result maps preserve the chunk content field.
//
// Requires a running Elasticsearch; set ES_TEST=1 to run.
func TestSearchByRegexp_MatchAndMiss(t *testing.T) {
	if common.GetEnv(common.EnvESTest) != "1" {
		t.Skip("Skipping ES integration test; set ES_TEST=1 to run")
	}

	ctx := context.Background()
	engine, err := NewEngine(ctx, getESTestConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	baseName := "ragflow_search_regexp_test"
	datasetID := "kb-regexp-1"

	chunks := []map[string]interface{}{
		{
			"doc_id":              "doc-1",
			"id":                  "regexp-chunk-1",
			"kb_id":               datasetID,
			"docnm_kwd":           "alpha.md",
			"content_with_weight": "the Stardust engine accelerates inference",
			"available_int":       1,
		},
		{
			"doc_id":              "doc-1",
			"id":                  "regexp-chunk-2",
			"kb_id":               datasetID,
			"docnm_kwd":           "beta.md",
			"content_with_weight": "skyvault persists checkpoint state",
			"available_int":       1,
		},
		{
			"doc_id":              "doc-1",
			"id":                  "regexp-chunk-3",
			"kb_id":               datasetID,
			"docnm_kwd":           "gamma.md",
			"content_with_weight": "stardust benchmark results",
			"available_int":       0, // not available; must be filtered out
		},
	}
	if _, err := engine.InsertChunks(ctx, chunks, baseName, datasetID); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	t.Run("match", func(t *testing.T) {
		res, err := engine.SearchByRegexp(ctx, &types.RegexpSearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{datasetID},
			Pattern:    "stardust",
			Limit:      10,
			Filter:     map[string]interface{}{"available_int": 1},
		})
		if err != nil {
			t.Fatalf("SearchByRegexp: %v", err)
		}
		if len(res.Chunks) != 1 {
			t.Fatalf("got %d chunks, want 1", len(res.Chunks))
		}
		content := contentField(t, res.Chunks[0])
		if !strings.Contains(content, "Stardust") {
			t.Errorf("chunk content = %q, want a match on 'stardust'", content)
		}
	})

	t.Run("miss", func(t *testing.T) {
		res, err := engine.SearchByRegexp(ctx, &types.RegexpSearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{datasetID},
			Pattern:    "definitely_not_present_term",
			Limit:      10,
			Filter:     map[string]interface{}{"available_int": 1},
		})
		if err != nil {
			t.Fatalf("SearchByRegexp: %v", err)
		}
		if len(res.Chunks) != 0 {
			t.Errorf("got %d chunks, want 0 for a non-matching pattern", len(res.Chunks))
		}
	})

	t.Run("case_insensitive", func(t *testing.T) {
		res, err := engine.SearchByRegexp(ctx, &types.RegexpSearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{datasetID},
			Pattern:    "SKYVAULT", // uppercase pattern must still match lowercase content
			Limit:      10,
			Filter:     map[string]interface{}{"available_int": 1},
		})
		if err != nil {
			t.Fatalf("SearchByRegexp: %v", err)
		}
		if len(res.Chunks) != 1 {
			t.Errorf("case-insensitive match failed: got %d chunks, want 1", len(res.Chunks))
		}
	})

	t.Run("alternation", func(t *testing.T) {
		res, err := engine.SearchByRegexp(ctx, &types.RegexpSearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{datasetID},
			Pattern:    "stardust|skyvault",
			Limit:      10,
			Filter:     map[string]interface{}{"available_int": 1},
		})
		if err != nil {
			t.Fatalf("SearchByRegexp: %v", err)
		}
		if len(res.Chunks) != 2 {
			t.Errorf("alternation match failed: got %d chunks, want 2", len(res.Chunks))
		}
	})
}

// contentField extracts the stored chunk content, preferring content_with_weight
// and falling back to content.
func contentField(t *testing.T, raw map[string]interface{}) string {
	t.Helper()
	if v, ok := raw["content_with_weight"].(string); ok && v != "" {
		return v
	}
	if v, ok := raw["content"].(string); ok {
		return v
	}
	t.Fatalf("chunk result missing content field: %#v", raw)
	return ""
}
