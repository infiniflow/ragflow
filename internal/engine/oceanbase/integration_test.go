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

package oceanbase

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

func TestLegacyStorageRoundTrip(t *testing.T) {
	host := os.Getenv("RAGFLOW_TEST_OCEANBASE_HOST")
	if host == "" {
		t.Skip("RAGFLOW_TEST_OCEANBASE_HOST is not set")
	}
	port, err := strconv.Atoi(envOr("RAGFLOW_TEST_OCEANBASE_PORT", "2881"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := NewEngine(envOr("RAGFLOW_TEST_OCEANBASE_ENGINE_TYPE", "oceanbase"), config.OceanBaseConnectionConfig{
		DBName: envOr("RAGFLOW_TEST_OCEANBASE_DBNAME", "test"),
		User:   envOr("RAGFLOW_TEST_OCEANBASE_USER", "root@test"), Password: os.Getenv("RAGFLOW_TEST_OCEANBASE_PASSWORD"),
		Host: host, Port: port, MaxConnections: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	tableName := "ragflow_go_compat_" + suffix
	datasetID := "kb-" + suffix
	defer cleanupChunkStore(t, engine, tableName, "")

	if err := engine.CreateChunkStore(ctx, tableName, datasetID, 2, "naive"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{{
		"id": "chunk-1", "kb_id": datasetID, "doc_id": "doc-1", "content_with_weight": "hello oceanbase",
		"content_ltks": "hello oceanbase", "important_kwd": []string{"hello"},
		"metadata":     map[string]interface{}{"_group_id": "group-1", "custom": "json-value"},
		"custom_field": "kept-in-extra", "q_2_vec": []float64{0.25, 0.5},
	}}, tableName, datasetID); err != nil {
		t.Fatal(err)
	}
	row, err := engine.GetChunk(ctx, tableName, "chunk-1", []string{datasetID})
	if err != nil {
		t.Fatal(err)
	}
	chunk := row.(map[string]interface{})
	if chunk["group_id"] != "group-1" {
		t.Fatalf("legacy metadata denormalization failed: %#v", chunk)
	}

	result, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{tableName}, KbIDs: []string{datasetID}, Limit: 10,
		SelectFields: []string{"id", "metadata", "extra", "q_2_vec"},
		MatchExprs: []interface{}{&types.MatchDenseExpr{
			VectorColumnName: "q_2_vec", EmbeddingData: []float64{0.25, 0.5},
			EmbeddingDataType: "float", TopN: 10, ExtraOptions: map[string]interface{}{"similarity": 0.1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 1 || result.Chunks[0]["id"] != "chunk-1" {
		t.Fatalf("vector round trip returned %#v", result)
	}

	pythonChunkID := "python-chunk-1"
	_, err = engine.db.ExecContext(ctx, fmt.Sprintf(
		"REPLACE INTO %s (id, kb_id, doc_id, content_with_weight, content_ltks, important_kwd, metadata, extra, q_2_vec) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		quoteIdentifier(tableName),
	), pythonChunkID, datasetID, "python-doc-1", "python formatted chunk", "python formatted chunk",
		`["python","legacy"]`, `{"_group_id":"python-group","custom":"python-json"}`,
		`{"python_extra":"preserved"}`, `[0.75,0.25]`)
	if err != nil {
		t.Fatal(err)
	}
	pythonRow, err := engine.GetChunk(ctx, tableName, pythonChunkID, []string{datasetID})
	if err != nil {
		t.Fatal(err)
	}
	pythonChunk := pythonRow.(map[string]interface{})
	if !reflect.DeepEqual(pythonChunk["important_kwd"], []interface{}{"python", "legacy"}) {
		t.Fatalf("Python ARRAY decoding failed: %#v", pythonChunk["important_kwd"])
	}
	if metadata, ok := pythonChunk["metadata"].(map[string]interface{}); !ok || metadata["custom"] != "python-json" {
		t.Fatalf("Python JSON decoding failed: %#v", pythonChunk["metadata"])
	}
	if vector, ok := floatSlice(pythonChunk["q_2_vec"]); !ok || !reflect.DeepEqual(vector, []float64{0.75, 0.25}) {
		t.Fatalf("Python VECTOR decoding failed: %#v", pythonChunk["q_2_vec"])
	}

	assertStoredJSON(ctx, t, engine, tableName, "important_kwd", "chunk-1", []interface{}{"hello"})
	assertStoredJSON(ctx, t, engine, tableName, "metadata", "chunk-1", map[string]interface{}{"_group_id": "group-1", "custom": "json-value"})
	assertStoredJSON(ctx, t, engine, tableName, "extra", "chunk-1", map[string]interface{}{"custom_field": "kept-in-extra"})
	assertStoredJSON(ctx, t, engine, tableName, "q_2_vec", "chunk-1", []interface{}{0.25, 0.5})

	memoryTable := "memory_go_compat_" + suffix
	memoryA := "memory-a-" + suffix
	memoryB := "memory-b-" + suffix
	defer cleanupChunkStore(t, engine, memoryTable, "")
	if err := engine.CreateChunkStore(ctx, memoryTable, memoryA, 2, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{"id": memoryA + "_1", "message_id": "1", "memory_id": memoryA, "content": "first", "content_embed": []float64{0.1, 0.2}},
		{"id": memoryB + "_1", "message_id": "1", "memory_id": memoryB, "content": "second", "content_embed": []float64{0.3, 0.4}},
	}, memoryTable, memoryA); err != nil {
		t.Fatal(err)
	}
	if err := engine.DropChunkStore(ctx, memoryTable, memoryA); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.GetChunk(ctx, memoryTable, memoryA+"_1", []string{memoryA}); err == nil {
		t.Fatal("deleted memory rows are still readable")
	}
	if _, err := engine.GetChunk(ctx, memoryTable, memoryB+"_1", []string{memoryB}); err != nil {
		t.Fatalf("another memory's rows were removed: %v", err)
	}

	tenantID := "go_compat_" + suffix
	metadataTable := metadataTableName(tenantID)
	defer cleanupMetadataStore(t, engine, tenantID)
	if err := engine.CreateMetadataStore(ctx, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.db.ExecContext(ctx,
		"REPLACE INTO "+quoteIdentifier(metadataTable)+" (id, kb_id, meta_fields) VALUES (?, ?, ?)",
		"python-meta-1", datasetID, `{"tags":["a","b"],"source":"python"}`,
	); err != nil {
		t.Fatal(err)
	}
	metadataResult, err := engine.SearchMetadata(ctx, &types.SearchMetadataRequest{
		TenantID: tenantID, Limit: 10, SelectFields: []string{"id", "kb_id", "meta_fields"},
		Filter: map[string]interface{}{"id": "python-meta-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(metadataResult.MetadataRecords) != 1 {
		t.Fatalf("Python metadata row returned %#v", metadataResult.MetadataRecords)
	}
	metaFields, ok := metadataResult.MetadataRecords[0]["meta_fields"].(map[string]interface{})
	if !ok || metaFields["source"] != "python" {
		t.Fatalf("Python metadata JSON decoding failed: %#v", metadataResult.MetadataRecords[0])
	}
	if err := engine.UpdateMetadata(ctx, "go-meta-1", datasetID, map[string]interface{}{"source": "go", "tags": []string{"c"}}, tenantID); err != nil {
		t.Fatal(err)
	}
	assertStoredJSON(ctx, t, engine, metadataTable, "meta_fields", "go-meta-1", map[string]interface{}{"source": "go", "tags": []interface{}{"c"}})
}

func assertStoredJSON(ctx context.Context, t *testing.T, engine *Engine, tableName, columnName, rowID string, want interface{}) {
	t.Helper()
	var raw interface{}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE id = ?", quoteIdentifier(columnName), quoteIdentifier(tableName))
	if err := engine.db.QueryRowContext(ctx, query, rowID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var text string
	switch value := raw.(type) {
	case []byte:
		text = string(value)
	case string:
		text = value
	default:
		t.Fatalf("stored %s value has type %T", columnName, raw)
	}
	var got interface{}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("stored %s value %q is not JSON compatible: %v", columnName, text, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored %s value = %#v, want %#v", columnName, got, want)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func cleanupChunkStore(t *testing.T, engine *Engine, tableName, datasetID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := engine.DropChunkStore(ctx, tableName, datasetID); err != nil {
		t.Errorf("clean up chunk store %s: %v", tableName, err)
	}
}

func cleanupMetadataStore(t *testing.T, engine *Engine, tenantID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := engine.DropMetadataStore(ctx, tenantID); err != nil {
		t.Errorf("clean up metadata store for tenant %s: %v", tenantID, err)
	}
}
