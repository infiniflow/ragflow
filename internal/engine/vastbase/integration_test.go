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

package vastbase

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

// Run against a live Vastbase G100 instance:
//
//	RAGFLOW_TEST_VASTBASE_HOST=... go test -tags integration ./internal/engine/vastbase/
//
// DBCOMPATIBILITY selects the server mode under test — the ParadeDB bm25 path
// (B) or the GIN to_tsvector path (PG). B is the default: it is the only mode
// the Python connector verified end to end.
func vbITEnvOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

// TestVastbaseStorageRoundTrip exercises the full storage surface against a
// live Vastbase instance: DDL, writes, searches, and cleanup.
func TestVastbaseStorageRoundTrip(t *testing.T) {
	host := vbITEnvOr("RAGFLOW_TEST_VASTBASE_HOST", "")
	if host == "" {
		t.Skip("RAGFLOW_TEST_VASTBASE_HOST is not set")
	}
	port, err := strconv.Atoi(vbITEnvOr("RAGFLOW_TEST_VASTBASE_PORT", "5432"))
	if err != nil {
		t.Fatal(err)
	}
	compatibility := vbITEnvOr("RAGFLOW_TEST_VASTBASE_DBCOMPATIBILITY", "B")
	engine, err := NewEngine(config.VastbaseConfig{
		Host: host, Port: port,
		User:            vbITEnvOr("RAGFLOW_TEST_VASTBASE_USER", "ragflow"),
		Password:        vbITEnvOr("RAGFLOW_TEST_VASTBASE_PASSWORD", ""),
		DBName:          vbITEnvOr("RAGFLOW_TEST_VASTBASE_DBNAME", "ragflow"),
		DBCompatibility: compatibility,
		SSLMode:         vbITEnvOr("RAGFLOW_TEST_VASTBASE_SSL_MODE", "disable"),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("ping vastbase (dbcompatibility %s): %v", compatibility, err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	table := "vb_it_" + suffix
	dataset := "kb-" + suffix
	defer engine.DropChunkStore(ctx, table, "")

	// --- Chunk store: DDL, dynamic columns, storage-format round trip ---
	if err := engine.CreateChunkStore(ctx, table, dataset, 3, "naive"); err != nil {
		t.Fatal(err)
	}
	exists, err := engine.ChunkStoreExists(ctx, table, dataset)
	if err != nil || !exists {
		t.Fatalf("chunk store exists = %v err = %v", exists, err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{
			"id": "c1", "kb_id": dataset, "doc_id": "doc-1",
			"content_with_weight": "hello vastbase world", "content_ltks": "hello vastbase world",
			"important_kwd": []interface{}{"hello", "vastbase"},
			"metadata":      map[string]interface{}{"_group_id": "g1"},
			"position_int":  []interface{}{[]interface{}{int64(1), 2, 3, 4, 5}},
			"page_num_int":  []interface{}{int64(1)},
			"custom_field":  "dynamic column value",
			"q_3_vec":       []interface{}{0.1, 0.2, 0.3},
		},
		{
			"id": "c2", "kb_id": dataset, "doc_id": "doc-1",
			"content_with_weight": "second chunk", "content_ltks": "second chunk",
			"available_int": 1, "q_3_vec": []interface{}{0.9, 0.8, 0.7},
		},
	}, table, dataset); err != nil {
		t.Fatal(err)
	}
	row, err := engine.GetChunk(ctx, table, "c1", []string{dataset})
	if err != nil {
		t.Fatal(err)
	}
	chunk := row.(map[string]interface{})
	if !reflect.DeepEqual(chunk["important_kwd"], []string{"hello", "vastbase"}) {
		t.Fatalf("keyword round trip = %#v", chunk["important_kwd"])
	}
	positions, ok := chunk["position_int"].([][]int64)
	if !ok || len(positions) != 1 || len(positions[0]) != 5 || positions[0][0] != 1 {
		t.Fatalf("position_int regroup = %#v", chunk["position_int"])
	}
	if vector, ok := chunk["q_3_vec"].([]float64); !ok || len(vector) != 3 {
		t.Fatalf("vector round trip = %#v", chunk["q_3_vec"])
	}
	if chunk["custom_field"] != "dynamic column value" {
		t.Fatalf("dynamic column round trip = %#v", chunk["custom_field"])
	}
	if groupID, _ := chunk["metadata"].(map[string]interface{}); groupID["_group_id"] != "g1" {
		t.Fatalf("metadata JSON round trip = %#v", chunk["metadata"])
	}

	// --- Four retrieval paths ---
	textResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id", "content_ltks"},
		MatchExprs: []interface{}{&types.MatchTextExpr{
			Fields: []string{"content_ltks"}, MatchingText: "vastbase", TopN: 10,
		}},
	})
	if err != nil {
		t.Fatalf("fulltext search: %v", err)
	}
	if len(textResult.Chunks) != 1 || textResult.Chunks[0]["id"] != "c1" {
		t.Fatalf("fulltext hits = %#v", textResult.Chunks)
	}

	vectorResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"},
		MatchExprs: []interface{}{&types.MatchDenseExpr{
			VectorColumnName: "q_3_vec", EmbeddingData: []float64{0.1, 0.2, 0.3},
			EmbeddingDataType: "float", TopN: 10,
		}},
	})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(vectorResult.Chunks) != 2 || vectorResult.Chunks[0]["id"] != "c1" {
		t.Fatalf("knn order = %#v", vectorResult.Chunks)
	}

	fusionResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "vastbase", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_3_vec", EmbeddingData: []float64{0.1, 0.2, 0.3},
				EmbeddingDataType: "float", TopN: 10},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10,
				FusionParams: map[string]interface{}{"weights": "0.5,0.5"}},
		},
	})
	if err != nil {
		t.Fatalf("fusion search: %v", err)
	}
	if len(fusionResult.Chunks) != 2 || fusionResult.Chunks[0]["id"] != "c1" {
		t.Fatalf("fusion order = %#v", fusionResult.Chunks)
	}

	filterResult, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"}, Filter: map[string]interface{}{"doc_id": "doc-1"},
	})
	if err != nil {
		t.Fatalf("filter search: %v", err)
	}
	if filterResult.Total != 2 || len(filterResult.Chunks) != 2 {
		t.Fatalf("filter total = %d chunks = %#v", filterResult.Total, filterResult.Chunks)
	}

	// --- Update + pagerank + delete ---
	if err := engine.UpdateChunks(ctx, map[string]interface{}{"id": "c2"},
		map[string]interface{}{"important_kwd": []interface{}{"tagged"}}, table, dataset); err != nil {
		t.Fatal(err)
	}
	if err := engine.AdjustChunkPagerank(ctx, table, "c1", dataset, 1.5, 0, 10); err != nil {
		t.Fatal(err)
	}
	row, err = engine.GetChunk(ctx, table, "c1", []string{dataset})
	if err != nil {
		t.Fatal(err)
	}
	if pagerank, _ := numberToFloat(row.(map[string]interface{})["pagerank_fea"]); pagerank != 1.5 {
		t.Fatalf("pagerank after adjust = %#v", row.(map[string]interface{})["pagerank_fea"])
	}
	deleted, err := engine.DeleteChunks(ctx, map[string]interface{}{"id": "c2"}, table, dataset)
	if err != nil || deleted != 1 {
		t.Fatalf("delete = %d err = %v", deleted, err)
	}

	// --- Memory store: tsquery fulltext + fusion ---
	memoryTable := "memory_vb_it_" + suffix
	memoryID := "mem-" + suffix
	defer engine.DropChunkStore(ctx, memoryTable, "")
	if err := engine.CreateChunkStore(ctx, memoryTable, memoryID, 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{"id": memoryID + "_1", "content": "remember the vastbase meeting", "status": true,
			"content_embed": []interface{}{0.25, 0.5}},
	}, memoryTable, memoryID); err != nil {
		t.Fatal(err)
	}
	memoryText, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{memoryTable}, KbIDs: []string{memoryID}, Limit: 10,
		SelectFields: []string{"content", "status"},
		MatchExprs:   []interface{}{&types.MatchTextExpr{MatchingText: "vastbase meeting"}},
	})
	if err != nil {
		t.Fatalf("memory fulltext: %v", err)
	}
	if len(memoryText.Chunks) != 1 || memoryText.Chunks[0]["content"] != "remember the vastbase meeting" {
		t.Fatalf("memory fulltext hits = %#v", memoryText.Chunks)
	}
	memoryFusion, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{memoryTable}, KbIDs: []string{memoryID}, Limit: 10,
		SelectFields: []string{"content"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "vastbase", TopN: 10},
			&types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.25, 0.5},
				EmbeddingDataType: "float", TopN: 10},
			&types.FusionExpr{Method: "weighted_sum", TopN: 10,
				FusionParams: map[string]interface{}{"weights": "0.5,0.5"}},
		},
	})
	if err != nil {
		t.Fatalf("memory fusion: %v", err)
	}
	if len(memoryFusion.Chunks) != 1 {
		t.Fatalf("memory fusion hits = %#v", memoryFusion.Chunks)
	}

	// --- Metadata store CRUD ---
	tenant := "it" + suffix
	defer engine.DropMetadataStore(ctx, tenant)
	if err := engine.CreateMetadataStore(ctx, tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertMetadata(ctx, []map[string]interface{}{
		{"id": "md-1", "kb_id": dataset, "doc_id": "doc-1",
			"meta_fields": map[string]interface{}{"year": 2026, "author": "it"}},
	}, tenant); err != nil {
		t.Fatal(err)
	}
	metaResult, err := engine.SearchMetadata(ctx, &types.SearchMetadataRequest{
		TenantID: tenant, Limit: 10, Filter: map[string]interface{}{"kb_id": dataset},
	})
	if err != nil {
		t.Fatalf("metadata search: %v", err)
	}
	if metaResult.Total != 1 || len(metaResult.MetadataRecords) != 1 {
		t.Fatalf("metadata search result = %#v", metaResult)
	}
	if err := engine.UpdateMetadata(ctx, "md-1", dataset,
		map[string]interface{}{"year": 2026, "author": "updated"}, tenant); err != nil {
		t.Fatal(err)
	}
	metaResult, err = engine.SearchMetadata(ctx, &types.SearchMetadataRequest{
		TenantID: tenant, Limit: 10, Filter: map[string]interface{}{"doc_id": "doc-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := metaResult.MetadataRecords[0]["meta_fields"].(map[string]interface{})
	if fields["author"] != "updated" {
		t.Fatalf("metadata update = %#v", metaResult.MetadataRecords[0])
	}
	if _, err := engine.DeleteMetadata(ctx, map[string]interface{}{"id": "md-1"}, tenant); err != nil {
		t.Fatal(err)
	}

	// --- Drop deletes dataset rows but keeps the shared table ---
	remaining, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{"id": "c3", "kb_id": dataset, "content_ltks": "third"},
	}, table, dataset)
	if err != nil {
		t.Fatal(err)
	}
	_ = remaining
	if err := engine.DropChunkStore(ctx, table, dataset); err != nil {
		t.Fatal(err)
	}
	filterResult, err = engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, Limit: 10,
		SelectFields: []string{"id"},
		MatchExprs:   []interface{}{&types.MatchTextExpr{Fields: []string{"content_ltks"}, MatchingText: "third"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filterResult.Chunks) != 0 {
		t.Fatalf("rows survived dataset drop: %#v", filterResult.Chunks)
	}
}
