//go:build integration

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

package infinity

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

// TestSlovakChunkStoreMatchesWithAndWithoutDiacritics runs against a real
// Infinity: a chunk store created with the Slovak dataset language gets the
// folding rag-*-slovak analyzers, so an accented word in a chunk matches a query
// typed with or without its diacritics.
func TestSlovakChunkStoreMatchesWithAndWithoutDiacritics(t *testing.T) {
	if err := common.InitLogger("error", common.FileOutput{}, "infinity_language_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	uri := os.Getenv("RAGFLOW_INFINITY_URI")
	if uri == "" {
		uri = "localhost:23817"
	}
	ctx := t.Context()
	eng, err := NewEngine(ctx, config.InfinityConfig{URI: uri, DBName: "ragflow_language_test", PostgresPort: 5432})
	if err != nil {
		t.Fatalf("connect to Infinity at %s: %v", uri, err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	baseName := fmt.Sprintf("ragflow_language_test_%d", time.Now().UnixNano())
	search := func(datasetID, query string) int {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames:   []string{baseName},
			KbIDs:        []string{datasetID},
			Limit:        10,
			SelectFields: []string{"id", "content_with_weight"},
			MatchExprs: []interface{}{&types.MatchTextExpr{
				Fields:       []string{"content_ltks", "content_sm_ltks", "docnm_kwd"},
				MatchingText: query,
				TopN:         10,
			}},
		})
		if err != nil {
			t.Fatalf("Search(%s, %q): %v", datasetID, query, err)
		}
		return len(res.Chunks)
	}

	content := "Deti chodia do školy každý deň."
	for _, tt := range []struct {
		datasetID, language string
		unaccentedHits      int
	}{
		{"kb_slovak", "slovak", 1},
		// The default analyzer does not fold, which is what makes the Slovak
		// match above the analyzer's doing.
		{"kb_default", "", 0},
	} {
		if err := eng.CreateChunkStore(ctx, baseName, tt.datasetID, 2, "naive", tt.language); err != nil {
			t.Fatalf("CreateChunkStore(%s): %v", tt.datasetID, err)
		}
		t.Cleanup(func() { _ = eng.DropChunkStore(context.Background(), baseName, tt.datasetID) })
		chunk := map[string]interface{}{
			"id":                  "chunk-1",
			"doc_id":              "doc-1",
			"kb_id":               tt.datasetID,
			"docnm_kwd":           "škola.txt",
			"content_with_weight": content,
			"content_ltks":        content,
			"content_sm_ltks":     content,
			"available_int":       1,
			"q_2_vec":             []float64{0.1, 0.2},
		}
		if _, err := eng.InsertChunks(ctx, []map[string]interface{}{chunk}, baseName, tt.datasetID, tt.language); err != nil {
			t.Fatalf("InsertChunks(%s): %v", tt.datasetID, err)
		}

		if got := search(tt.datasetID, "školy"); got != 1 {
			t.Errorf("%s: accented query returned %d chunks, want 1", tt.datasetID, got)
		}
		if got := search(tt.datasetID, "skoly"); got != tt.unaccentedHits {
			t.Errorf("%s: unaccented query returned %d chunks, want %d", tt.datasetID, got, tt.unaccentedHits)
		}
	}
}
