//go:build integration

package opensearch

import (
	"context"
	"os"
	"testing"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

// Integration tests against a real OpenSearch cluster (the docker one on
// localhost:1201 by default). Run with:
//
//	OS_TEST=1 go test -tags integration ./internal/engine/opensearch/ -run TestOpenSearch -count=1 -v
//
// The cluster is only ever touched through the engine API; every test cleans
// up the indices it creates.

const (
	osTestIndex = "ragflow_os_it_chunk"
	osTestKBID  = "kb-os-it"
	osTestDim   = 768
)

func getOSTestConfig() config.OpenSearchConfig {
	hosts := os.Getenv("OS_HOST")
	if hosts == "" {
		hosts = "http://localhost:1201"
	}
	username := os.Getenv("OS_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("OS_PASSWORD")
	if password == "" {
		password = "infini_rag_flow_OS_01"
	}
	return config.OpenSearchConfig{
		Hosts:    hosts,
		Username: username,
		Password: password,
	}
}

func osTestVector(base float64) []float64 {
	v := make([]float64, osTestDim)
	for i := range v {
		v[i] = base + float64(i%7)*0.01
	}
	return v
}

func osTestChunks() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":                  "c1",
			"doc_id":              "d1",
			"kb_id":               osTestKBID,
			"available_int":       1,
			"content_with_weight": "hello world ragflow vector search test",
			"content_tks":         "hello world ragflow vector search test",
			"content_ltks":        "hello world ragflow vector search test",
			"title_tks":           "doc one",
			"title_ltks":          "doc one",
			"q_768_vec":           osTestVector(0.0),
		},
		{
			"id":                  "c2",
			"doc_id":              "d1",
			"kb_id":               osTestKBID,
			"available_int":       1,
			"content_with_weight": "another document about opensearch",
			"content_tks":         "another document about opensearch",
			"content_ltks":        "another document about opensearch",
			"title_tks":           "doc two",
			"title_ltks":          "doc two",
			"q_768_vec":           osTestVector(10.0),
		},
	}
}

func osTestSearchReq(exprs []interface{}) *types.SearchRequest {
	return &types.SearchRequest{
		IndexNames: []string{osTestIndex},
		KbIDs:      []string{osTestKBID},
		Limit:      10,
		MatchExprs: exprs,
	}
}

// TestOpenSearchChunkLifecycle exercises the chunk path end to end against a
// real cluster: index creation from os_mapping.json, bulk insert, text /
// vector / hybrid search, KNN second-pass scores, update, delete and drop.
func TestOpenSearchChunkLifecycle(t *testing.T) {
	eng, err := NewEngine(t.Context(), getOSTestConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	if err := eng.DropChunkStore(t.Context(), osTestIndex, osTestKBID); err != nil {
		t.Fatalf("DropChunkStore (cleanup): %v", err)
	}
	// t.Context() is cancelled by the time Cleanup runs; use a fresh context so
	// the teardown delete actually reaches the cluster.
	t.Cleanup(func() {
		_ = eng.DropChunkStore(context.Background(), osTestIndex, osTestKBID)
	})

	if err := eng.CreateChunkStore(t.Context(), osTestIndex, osTestKBID, osTestDim, "", "zh"); err != nil {
		t.Fatalf("CreateChunkStore: %v", err)
	}

	if _, err := eng.InsertChunks(t.Context(), osTestChunks(), osTestIndex, osTestKBID, "zh"); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	// Read back: the stored doc must keep the physical field names and the
	// datasetID-scoped kb_id.
	stored, err := eng.GetChunk(t.Context(), osTestIndex, "c1", []string{osTestKBID})
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	sm, ok := stored.(map[string]interface{})
	if !ok {
		t.Fatalf("GetChunk returned %T, want map[string]interface{}", stored)
	}
	if v, ok := sm["content_ltks"].(string); !ok || v != "hello world ragflow vector search test" {
		t.Errorf("stored content_ltks = %#v", sm["content_ltks"])
	}
	if v, ok := sm["kb_id"].(string); !ok || v != osTestKBID {
		t.Errorf("stored kb_id = %#v, want %s", sm["kb_id"], osTestKBID)
	}

	// Pure text search
	textRes, err := eng.Search(t.Context(), osTestSearchReq([]interface{}{
		&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "hello world", TopN: 10},
	}))
	if err != nil {
		t.Fatalf("Search text: %v", err)
	}
	if len(textRes.Chunks) != 1 || textRes.Chunks[0]["id"] != "c1" {
		t.Fatalf("text search: got chunks %v; want 1 chunk c1", textRes.Chunks)
	}

	// Pure vector search: c1 (vector 0) must rank first for a query near it.
	vecRes, err := eng.Search(t.Context(), osTestSearchReq([]interface{}{
		&types.MatchDenseExpr{VectorColumnName: "q_768_vec", EmbeddingData: osTestVector(0.0), TopN: 10},
	}))
	if err != nil {
		t.Fatalf("Search vector: %v", err)
	}
	if len(vecRes.Chunks) != 2 || vecRes.Chunks[0]["id"] != "c1" {
		t.Fatalf("vector search: got chunks %v; want 2 chunks c1 first", vecRes.Chunks)
	}

	// Hybrid search (text + vector + weighted-sum fusion) through the
	// normalization search pipeline.
	hybRes, err := eng.Search(t.Context(), osTestSearchReq([]interface{}{
		&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "hello world", TopN: 10},
		&types.MatchDenseExpr{VectorColumnName: "q_768_vec", EmbeddingData: osTestVector(0.0), TopN: 10},
		&types.FusionExpr{Method: "weighted_sum", FusionParams: map[string]interface{}{"weights": "0.5,0.5"}},
	}))
	if err != nil {
		t.Fatalf("Search hybrid: %v", err)
	}
	if len(hybRes.Chunks) != 2 || hybRes.Chunks[0]["id"] != "c1" {
		t.Fatalf("hybrid search: got chunks %v; want 2 chunks c1 first", hybRes.Chunks)
	}

	// KNN second-pass scores: c1 is the nearest neighbour.
	knnRes, err := eng.KNNScores(t.Context(), []map[string]interface{}{
		{"_id": "c1", "_index": osTestIndex},
		{"_id": "c2", "_index": osTestIndex},
	}, osTestVector(0.0), 2)
	if err != nil {
		t.Fatalf("KNNScores: %v", err)
	}
	scores := eng.GetScores(knnRes)
	if s1, ok := scores["c1"]; !ok || s1 < scores["c2"] {
		t.Fatalf("KNNScores: c1 must outrank c2, got %v", scores)
	}

	// Update + read back
	if err := eng.UpdateChunks(t.Context(), map[string]interface{}{"id": "c1"}, map[string]interface{}{"title_tks": "updated title"}, osTestIndex, osTestKBID); err != nil {
		t.Fatalf("UpdateChunks: %v", err)
	}
	after, err := eng.GetChunk(t.Context(), osTestIndex, "c1", []string{osTestKBID})
	if err != nil {
		t.Fatalf("GetChunk after update: %v", err)
	}
	if v, ok := after.(map[string]interface{})["title_tks"].(string); !ok || v != "updated title" {
		t.Fatalf("updated title_tks = %#v", after.(map[string]interface{})["title_tks"])
	}

	// Delete by kb_id
	deleted, err := eng.DeleteChunks(t.Context(), map[string]interface{}{"kb_id": osTestKBID}, osTestIndex, osTestKBID)
	if err != nil {
		t.Fatalf("DeleteChunks: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("DeleteChunks deleted %d, want 2", deleted)
	}

	exists, err := eng.ChunkStoreExists(t.Context(), osTestIndex, osTestKBID)
	if err != nil {
		t.Fatalf("ChunkStoreExists: %v", err)
	}
	if !exists {
		t.Fatal("ChunkStoreExists = false after chunk deletion")
	}
}

// TestOpenSearchSQL runs a plain SELECT against the real cluster's SQL plugin.
func TestOpenSearchSQL(t *testing.T) {
	eng, err := NewEngine(t.Context(), getOSTestConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	if err := eng.CreateChunkStore(t.Context(), osTestIndex, osTestKBID, osTestDim, "", "zh"); err != nil {
		t.Fatalf("CreateChunkStore: %v", err)
	}
	t.Cleanup(func() {
		_ = eng.DropChunkStore(context.Background(), osTestIndex, osTestKBID)
	})

	if _, err := eng.InsertChunks(t.Context(), osTestChunks(), osTestIndex, osTestKBID, "zh"); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	// The shared caller always passes ES's "json" format; the engine must
	// translate it to the JDBC format OpenSearch's SQL plugin decodes.
	rows, err := eng.RunSQL(t.Context(), osTestIndex, "SELECT id, title_tks FROM "+osTestIndex, []string{osTestKBID}, "json")
	if err != nil {
		t.Fatalf("RunSQL: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("RunSQL returned %d rows, want 2", len(rows))
	}
}
