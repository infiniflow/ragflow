//go:build integration

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

// TestInsertChunksUpsertNoDuplicateRows pins the upsert invariant: inserting a
// chunk with an id that already exists must replace the old row, not add a
// second one. Infinity has no id uniqueness, so InsertChunks deletes the old
// row by id before inserting; if that delete silently failed (the old code only
// Warn-and-continued), the insert would leave both the stale and new rows and
// search would return duplicate chunks. This test inserts the same id twice
// with different content and asserts a search returns exactly one chunk whose
// content is the new value.
//
// Runs against a real Infinity (RAGFLOW_INFINITY_URI, default localhost:23817).
func TestInsertChunksUpsertNoDuplicateRows(t *testing.T) {
	if err := common.InitLogger("error", common.FileOutput{}, "infinity_upsert_test"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	uri := os.Getenv("RAGFLOW_INFINITY_URI")
	if uri == "" {
		uri = "localhost:23817"
	}
	ctx := t.Context()
	eng, err := NewEngine(ctx, config.InfinityConfig{URI: uri, DBName: "ragflow_upsert_test", PostgresPort: 5432})
	if err != nil {
		t.Fatalf("connect to Infinity at %s: %v", uri, err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	baseName := fmt.Sprintf("ragflow_upsert_test_%d", time.Now().UnixNano())
	const datasetID = "kb_upsert"
	t.Cleanup(func() { _ = eng.DropChunkStore(context.Background(), baseName, datasetID) })
	if err := eng.CreateChunkStore(ctx, baseName, datasetID, 2, "naive", ""); err != nil {
		t.Fatalf("CreateChunkStore: %v", err)
	}

	chunkFor := func(content string) map[string]interface{} {
		return map[string]interface{}{
			"id":                  "chunk-same-id",
			"doc_id":              "doc-1",
			"kb_id":               datasetID,
			"docnm_kwd":           "upsert.txt",
			"content_with_weight": content,
			"content_ltks":        content,
			"content_sm_tks":      content,
			"available_int":       1,
			"q_2_vec":             []float64{0.1, 0.2},
		}
	}

	// First insert establishes the row.
	if _, err := eng.InsertChunks(ctx, []map[string]interface{}{chunkFor("first version of the content")}, baseName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks #1: %v", err)
	}
	// Second insert with the SAME id must upsert, not append.
	if _, err := eng.InsertChunks(ctx, []map[string]interface{}{chunkFor("second version of the content")}, baseName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks #2: %v", err)
	}

	res, err := eng.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{baseName},
		KbIDs:        []string{datasetID},
		Limit:        10,
		SelectFields: []string{"id", "content_with_weight"},
		MatchExprs: []interface{}{&types.MatchTextExpr{
			Fields:       []string{"content_ltks", "content_sm_tks", "docnm_kwd"},
			MatchingText: "content",
			TopN:         10,
		}},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Chunks) != 1 {
		t.Fatalf("search returned %d chunks for one upserted id, want 1 (no duplicate rows): %+v", len(res.Chunks), res.Chunks)
	}
	got, _ := res.Chunks[0]["content_with_weight"].(string)
	if got != "second version of the content" {
		t.Errorf("upserted content = %q, want the second (newest) value", got)
	}
}
