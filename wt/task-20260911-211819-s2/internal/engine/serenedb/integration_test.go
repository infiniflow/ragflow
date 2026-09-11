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

// This end-to-end test drives the engine against a real SereneDB. It is
// skipped unless SERENEDB_TEST_DSN points at a live instance (>= 26.07.4), so
// the default test run stays pure and CI-safe. To run it:
//
//	docker run -d --name serenedb-gotest -p 127.0.0.1:7899:7890 \
//	  -e POSTGRES_PASSWORD=gotest serenedb/serenedb:26.07.4
//	SERENEDB_TEST_DSN='host=127.0.0.1 port=7899 user=postgres password=gotest dbname=postgres sslmode=disable' \
//	  go test -run Integration -v ./internal/engine/serenedb/
package serenedb

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

// logOnce initializes the shared logger the engine's Search path expects. In
// production the server does this at startup; a bare `go test` does not.
var logOnce sync.Once

func liveEngine(t *testing.T) *serenedbEngine {
	t.Helper()
	dsn := os.Getenv("SERENEDB_TEST_DSN")
	if dsn == "" {
		t.Skip("SERENEDB_TEST_DSN not set; skipping live SereneDB integration test")
	}
	logOnce.Do(func() {
		_ = common.InitLogger("info", common.FileOutput{Path: filepath.Join(t.TempDir(), "serenedb-it.log")}, "serenedb-it")
	})
	t.Setenv("SERENEDB_DSN", dsn)
	e, err := NewEngine(config.SereneDBConfig{})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	return e
}

// waitForFulltext polls until the async inverted index has caught up with the
// last write (SereneDB refreshes the index ~1s after insert, like ES).
func waitForFulltext(t *testing.T, e *serenedbEngine, req *types.SearchRequest) *types.SearchResult {
	t.Helper()
	ctx := t.Context()
	deadline := time.Now().Add(15 * time.Second)
	for {
		res, err := e.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search: %v", err)
		}
		if len(res.Chunks) > 0 || time.Now().After(deadline) {
			return res
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func chunkIDs(res *types.SearchResult) []string {
	ids := make([]string, 0, len(res.Chunks))
	for _, c := range res.Chunks {
		if id, ok := c["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestIntegrationChunkLifecycle(t *testing.T) {
	e := liveEngine(t)
	defer e.Close()
	ctx := t.Context()

	const base = "ragflow_gotest"
	const kb = "kb1"
	// Start clean and always tear down the throwaway table.
	_ = e.DropChunkStore(ctx, base, kb)
	defer func() { _ = e.DropChunkStore(ctx, base, kb) }()

	if err := e.CreateChunkStore(ctx, base, kb, 4, ""); err != nil {
		t.Fatalf("CreateChunkStore: %v", err)
	}
	if ok, _ := e.ChunkStoreExists(ctx, base, kb); !ok {
		t.Fatal("ChunkStoreExists = false after create")
	}

	chunks := []map[string]interface{}{
		{"id": "a", "doc_id": "d1", "kb_id": kb, "content_ltks": "alpha beta", "content_with_weight": "alpha beta", "q_4_vec": []float64{1, 0, 0, 0}, "important_kwd": []interface{}{"alpha"}},
		{"id": "b", "doc_id": "d1", "kb_id": kb, "content_ltks": "gamma delta", "content_with_weight": "gamma delta", "q_4_vec": []float64{0, 1, 0, 0}, "important_kwd": []interface{}{"gamma"}},
		{"id": "c", "doc_id": "d2", "kb_id": kb, "content_ltks": "alpha gamma", "content_with_weight": "alpha gamma", "q_4_vec": []float64{0.9, 0.1, 0, 0}, "important_kwd": []interface{}{"alpha", "gamma"}},
	}
	if _, err := e.InsertChunks(ctx, chunks, base, kb); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	req := func(exprs []interface{}, filter map[string]interface{}) *types.SearchRequest {
		return &types.SearchRequest{
			IndexNames: []string{base}, KbIDs: []string{kb},
			Limit: 10, MatchExprs: exprs, Filter: filter,
		}
	}

	t.Run("fulltext", func(t *testing.T) {
		res := waitForFulltext(t, e, req([]interface{}{
			&types.MatchTextExpr{MatchingText: "alpha", TopN: 10},
		}, nil))
		ids := chunkIDs(res)
		if !contains(ids, "a") || !contains(ids, "c") {
			t.Fatalf("fulltext 'alpha' should match a and c, got %v", ids)
		}
		if contains(ids, "b") {
			t.Fatalf("fulltext 'alpha' should not match b, got %v", ids)
		}
		for _, ch := range res.Chunks {
			if _, ok := ch["_score"].(float64); !ok {
				t.Errorf("chunk %v missing float _score", ch["id"])
			}
		}
	})

	t.Run("vector", func(t *testing.T) {
		res := waitForFulltext(t, e, req([]interface{}{
			&types.MatchDenseExpr{VectorColumnName: "q_4_vec", EmbeddingData: []float64{1, 0, 0, 0}, TopN: 10},
		}, nil))
		ids := chunkIDs(res)
		if len(ids) == 0 || ids[0] != "a" {
			t.Fatalf("vector query [1,0,0,0] should rank 'a' first, got %v", ids)
		}
	})

	t.Run("fusion", func(t *testing.T) {
		res := waitForFulltext(t, e, req([]interface{}{
			&types.MatchTextExpr{MatchingText: "alpha", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_4_vec", EmbeddingData: []float64{1, 0, 0, 0}, TopN: 10},
			&types.FusionExpr{Method: "weighted_sum", FusionParams: map[string]interface{}{"weights": "0.3,0.7"}},
		}, nil))
		ids := chunkIDs(res)
		if len(ids) == 0 || ids[0] != "a" {
			t.Fatalf("fusion(alpha, [1,0,0,0]) should rank 'a' first, got %v", ids)
		}
	})

	t.Run("filter_only", func(t *testing.T) {
		res, err := e.Search(ctx, req(nil, map[string]interface{}{"doc_id": "d2"}))
		if err != nil {
			t.Fatalf("filter search: %v", err)
		}
		ids := chunkIDs(res)
		if len(ids) != 1 || ids[0] != "c" {
			t.Fatalf("filter doc_id=d2 should return only c, got %v", ids)
		}
	})

	t.Run("get_and_scores", func(t *testing.T) {
		got, err := e.GetChunk(ctx, base, "a", []string{kb})
		if err != nil || got == nil {
			t.Fatalf("GetChunk(a) = %v, %v", got, err)
		}
		m := got.(map[string]interface{})
		if m["content_ltks"] != "alpha beta" {
			t.Errorf("GetChunk content = %v", m["content_ltks"])
		}
		// important_kwd is a native array column.
		if arr, ok := m["important_kwd"].([]string); !ok || len(arr) == 0 || arr[0] != "alpha" {
			t.Errorf("important_kwd not decoded as array: %v", m["important_kwd"])
		}
		res := waitForFulltext(t, e, req([]interface{}{
			&types.MatchTextExpr{MatchingText: "alpha", TopN: 10},
		}, nil))
		knn, _ := e.KNNScores(ctx, res.Chunks, nil, 10)
		scores := e.GetScores(knn)
		if _, ok := scores["a"]; !ok {
			t.Errorf("GetScores missing 'a': %v", scores)
		}
	})

	t.Run("update_and_delete", func(t *testing.T) {
		if err := e.UpdateChunks(ctx,
			map[string]interface{}{"id": "b"},
			map[string]interface{}{"add": map[string]interface{}{"tag_kwd": "x"}},
			base, kb); err != nil {
			t.Fatalf("UpdateChunks add: %v", err)
		}
		n, err := e.DeleteChunks(ctx, map[string]interface{}{"id": "b"}, base, kb)
		if err != nil || n != 1 {
			t.Fatalf("DeleteChunks(b) = %d, %v (want 1)", n, err)
		}
	})
}

func TestIntegrationMetadata(t *testing.T) {
	e := liveEngine(t)
	defer e.Close()
	ctx := t.Context()

	const tenant = "gotest_tenant"
	_ = e.DropMetadataStore(ctx, tenant)
	defer func() { _ = e.DropMetadataStore(ctx, tenant) }()

	if err := e.CreateMetadataStore(ctx, tenant); err != nil {
		t.Fatalf("CreateMetadataStore: %v", err)
	}
	if _, err := e.InsertMetadata(ctx, []map[string]interface{}{
		{"id": "doc1", "kb_id": "kb1", "meta_fields": map[string]interface{}{"author": "ann", "year": float64(2026)}},
	}, tenant); err != nil {
		t.Fatalf("InsertMetadata: %v", err)
	}

	// Merge update preserves untouched keys.
	if err := e.UpdateMetadata(ctx, "doc1", "kb1", map[string]interface{}{"author": "bob"}, tenant); err != nil {
		t.Fatalf("UpdateMetadata: %v", err)
	}
	res, err := e.SearchMetadata(ctx, &types.SearchMetadataRequest{TenantID: tenant, Limit: 10})
	if err != nil {
		t.Fatalf("SearchMetadata: %v", err)
	}
	if res.Total != 1 || len(res.MetadataRecords) != 1 {
		t.Fatalf("SearchMetadata total=%d records=%d", res.Total, len(res.MetadataRecords))
	}
	mf, ok := res.MetadataRecords[0]["meta_fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("meta_fields not decoded to map: %v", res.MetadataRecords[0]["meta_fields"])
	}
	if mf["author"] != "bob" {
		t.Errorf("merge should set author=bob, got %v", mf["author"])
	}
	if _, ok := mf["year"]; !ok {
		t.Errorf("merge should preserve year, got %v", mf)
	}

	if err := e.DeleteMetadataKeys(ctx, "doc1", "kb1", []string{"year"}, tenant); err != nil {
		t.Fatalf("DeleteMetadataKeys: %v", err)
	}
	after, _ := e.loadMetaFields(ctx, buildMetadataTableName(tenant), "doc1", "kb1")
	if _, ok := after["year"]; ok {
		t.Errorf("year should be removed, got %v", after)
	}
}
