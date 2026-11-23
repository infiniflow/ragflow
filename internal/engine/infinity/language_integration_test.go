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

	infinity "github.com/infiniflow/infinity-go-sdk"

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
	search := func(datasetID, language, query string) int {
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames:   []string{baseName},
			KbIDs:        []string{datasetID},
			Language:     language,
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
		legacy              bool
	}{
		{"kb_slovak", "slovak", 1, false},
		// The default analyzer does not fold, which is what makes the Slovak
		// match above the analyzer's doing.
		{"kb_default", "", 0, false},
		{"kb_legacy_slovak", "slovak", 1, true},
	} {
		initialLanguage := tt.language
		if tt.legacy {
			initialLanguage = "English"
		}
		if err := eng.CreateChunkStore(ctx, baseName, tt.datasetID, 2, "naive", initialLanguage); err != nil {
			t.Fatalf("CreateChunkStore(%s): %v", tt.datasetID, err)
		}
		t.Cleanup(func() { _ = eng.DropChunkStore(context.Background(), baseName, tt.datasetID) })
		if tt.legacy {
			makeLegacySlovakIndex(t, eng, baseName, tt.datasetID)
		}
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
		if _, err := eng.InsertChunks(ctx, []map[string]interface{}{chunk}, baseName, tt.datasetID, initialLanguage); err != nil {
			t.Fatalf("InsertChunks(%s): %v", tt.datasetID, err)
		}

		if got := search(tt.datasetID, tt.language, "školy"); got != 1 {
			t.Errorf("%s: accented query returned %d chunks, want 1", tt.datasetID, got)
		}
		if got := search(tt.datasetID, tt.language, "skoly"); got != tt.unaccentedHits {
			t.Errorf("%s: unaccented query returned %d chunks, want %d", tt.datasetID, got, tt.unaccentedHits)
		}
		if tt.legacy {
			for _, step := range []struct {
				language       string
				unaccentedHits int
			}{{"English", 0}, {"English", 0}, {"", 0}, {"Slovak", 1}} {
				if err := eng.EnsureChunkStoreLanguage(ctx, baseName, tt.datasetID, step.language); err != nil {
					t.Fatalf("EnsureChunkStoreLanguage(%s): %v", step.language, err)
				}
				if got := search(tt.datasetID, step.language, "skoly"); got != step.unaccentedHits {
					t.Errorf("%s: unaccented query returned %d chunks, want %d", step.language, got, step.unaccentedHits)
				}
				if got := search(tt.datasetID, step.language, "\u0161koly"); got != 1 {
					t.Errorf("%s: accented query returned %d chunks, want 1", step.language, got)
				}
			}

			tableName := buildChunkTableName(baseName, tt.datasetID)
			oldReadiness := eng.languageIndexReadiness(tableName, "Slovak")
			if !oldReadiness.Load() {
				t.Fatal("expected the old table's Slovak indexes to be cached")
			}
			oldReadiness.Store(false)
			if err := eng.DropChunkStore(ctx, baseName, tt.datasetID); err != nil {
				t.Fatalf("drop old table: %v", err)
			}
			if err := eng.CreateChunkStore(ctx, baseName, tt.datasetID, 2, "naive", "English"); err != nil {
				t.Fatalf("recreate table: %v", err)
			}
			if _, err := eng.InsertChunks(ctx, []map[string]interface{}{chunk}, baseName, tt.datasetID, "English"); err != nil {
				t.Fatalf("insert into recreated table: %v", err)
			}
			// Model a delayed completion that captured the old cache entry before drop.
			oldReadiness.Store(true)
			if got := search(tt.datasetID, "Slovak", "skoly"); got != 1 {
				t.Errorf("recreated table returned %d chunks, want 1 after preparing its Slovak indexes", got)
			}
		}
	}
}

func makeLegacySlovakIndex(t *testing.T, eng *Engine, baseName, datasetID string) {
	t.Helper()
	db, release, err := eng.client.checkoutDatabase(t.Context(), "language integration test")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	table, err := db.GetTable(buildChunkTableName(baseName, datasetID))
	if err != nil {
		t.Fatal(err)
	}

	const field = "content"
	const legacyName = "ft_content_rag_coarse"
	for _, name := range []string{
		legacyName, "ft_content_rag_coarse_default",
		"ft_authors_rag_coarse", "ft_authors_rag_coarse_default",
		"ft_authors_rag_fine", "ft_authors_rag_fine_default",
	} {
		if _, err := table.DropIndex(name, infinity.ConflictTypeError); err != nil {
			t.Fatalf("drop fixture index %s: %v", name, err)
		}
	}
	if _, err := table.DropColumns([]string{"authors"}); err != nil {
		t.Fatalf("drop unrelated column absent in legacy schema: %v", err)
	}
	_, err = table.CreateIndex(legacyName, infinity.NewIndexInfo(field, infinity.IndexTypeFullText,
		map[string]string{"ANALYZER": "rag-coarse-slovak"}), infinity.ConflictTypeError, "")
	if err != nil {
		t.Fatalf("create legacy localized index: %v", err)
	}
}
