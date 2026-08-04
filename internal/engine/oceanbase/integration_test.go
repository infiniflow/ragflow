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
	"fmt"
	"os"
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
	engine, err := NewEngine("oceanbase", config.OceanBaseConnectionConfig{
		DBName: envOr("RAGFLOW_TEST_OCEANBASE_DBNAME", "test"),
		User:   envOr("RAGFLOW_TEST_OCEANBASE_USER", "root@test"), Password: os.Getenv("RAGFLOW_TEST_OCEANBASE_PASSWORD"),
		Host: host, Port: port, MaxConnections: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	tableName := "ragflow_go_compat_" + suffix
	datasetID := "kb-" + suffix
	defer engine.DropChunkStore(context.Background(), tableName, "")

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
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func TestIntegrationTableNamesAreIsolated(t *testing.T) {
	if got := fmt.Sprintf("ragflow_go_compat_%d", time.Now().UnixNano()); !identifierPattern.MatchString(got) {
		t.Fatalf("temporary integration table name is invalid: %s", got)
	}
}
