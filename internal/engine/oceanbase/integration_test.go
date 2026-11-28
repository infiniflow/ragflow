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

func newIntegrationEngine(t *testing.T) *Engine {
	t.Helper()
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
	t.Cleanup(func() { engine.Close() })
	return engine
}

func TestLegacyStorageRoundTrip(t *testing.T) {
	engine := newIntegrationEngine(t)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	tableName := "ragflow_go_compat_" + suffix
	datasetID := "kb-" + suffix
	defer cleanupChunkStore(t, engine, tableName, "")

	if err := engine.CreateChunkStore(ctx, tableName, datasetID, 2, "naive", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{{
		"id": "chunk-1", "kb_id": datasetID, "doc_id": "doc-1", "content_with_weight": "hello oceanbase",
		"content_ltks": "hello oceanbase", "important_kwd": []string{"hello"},
		"metadata":     map[string]interface{}{"_group_id": "group-1", "custom": "json-value"},
		"custom_field": "kept-in-extra", "q_2_vec": []float64{0.25, 0.5},
	}}, tableName, datasetID, ""); err != nil {
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
	if err := engine.CreateChunkStore(ctx, memoryTable, memoryA, 2, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{
		{"id": memoryA + "_1", "message_id": "1", "memory_id": memoryA, "content": "first", "content_embed": []float64{0.1, 0.2}},
		{"id": memoryB + "_1", "message_id": "1", "memory_id": memoryB, "content": "second", "content_embed": []float64{0.3, 0.4}},
	}, memoryTable, memoryA, ""); err != nil {
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

func TestChunkExtraFilterAndMutation(t *testing.T) {
	for _, schema := range []string{"legacy", "current"} {
		t.Run(schema, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
			defer cancel()
			engine := newIntegrationEngine(t)
			table := "ragflow_go_extra_" + strconv.FormatInt(time.Now().UnixNano(), 10)
			defer cleanupChunkStore(t, engine, table, "")
			if schema == "legacy" {
				if err := engine.ensureTable(ctx, table, baseChunkColumns, table); err != nil {
					t.Fatal(err)
				}
			}
			const dataset = "target-kb"
			if err := engine.CreateChunkStore(ctx, table, dataset, 2, "naive", ""); err != nil {
				t.Fatal(err)
			}
			const compileType = "tree's"
			if _, err := engine.InsertChunks(ctx, []map[string]interface{}{
				{
					"id": "target", "kb_id": dataset, "compile_kwd": compileType, "token_num": 12,
					"outlinks_int": 2, "title_kwd": "Zebra",
					"children_kwd": []string{"child%", "keep"},
					"claims":       map[string]interface{}{"old": true},
					"extra":        map[string]interface{}{"custom_field": "keep"},
				},
				{"id": "literal-null", "kb_id": dataset, "compile_kwd": "null", "outlinks_int": 10, "title_kwd": "Beta"},
				{"id": "absent", "kb_id": dataset, "outlinks_int": 10, "title_kwd": "alpha"},
				{"id": "json-null", "kb_id": dataset, "compile_kwd": nil},
				{"id": "no-extra", "kb_id": dataset},
				{"id": "numeric-zero", "kb_id": dataset, "token_num": 0},
				{"id": "other-kb", "kb_id": "other-kb", "compile_kwd": compileType, "token_num": 12, "children_kwd": []string{"child%"}},
			}, table, dataset, ""); err != nil {
				t.Fatal(err)
			}
			// Test an incorrectly typed legacy value and a stale extra key next
			// to a physical NULL in the current schema.
			extra := `{"token_num":"invalid","compile_kwd":null}`
			if schema == "current" {
				extra = `{"token_num":"invalid","compile_kwd":"tree's"}`
			}
			if _, err := engine.db.ExecContext(ctx, "UPDATE "+quoteIdentifier(table)+" SET extra = ? WHERE id = ?",
				extra, "json-null"); err != nil {
				t.Fatal(err)
			}

			checkSearch := func(filter map[string]interface{}, fields []string, wantIDs ...string) []map[string]interface{} {
				t.Helper()
				result, err := engine.Search(ctx, &types.SearchRequest{
					IndexNames: []string{table}, KbIDs: []string{dataset},
					SelectFields: fields, Filter: filter,
					OrderBy: (&types.OrderByExpr{}).Asc("id"),
				})
				if err != nil {
					t.Fatalf("search with %v: %v", filter, err)
				}
				ids := make([]string, 0, len(result.Chunks))
				for _, row := range result.Chunks {
					ids = append(ids, stringValue(row["id"]))
				}
				if result.Total != int64(len(wantIDs)) || !reflect.DeepEqual(ids, wantIDs) {
					t.Fatalf("search with %v returned %v (%d total), want %v", filter, ids, result.Total, wantIDs)
				}
				return result.Chunks
			}
			matches := map[string]interface{}{
				"compile_kwd": []string{"other", compileType}, "children_kwd": []string{"miss", "child%"}, "token_num": 12,
			}
			checkSearch(matches, []string{"compile_kwd", "children_kwd"}, "target")
			checkSearch(map[string]interface{}{"exists": "compile_kwd"}, nil, "literal-null", "target")
			checkSearch(map[string]interface{}{"must_not": map[string]interface{}{"exists": "compile_kwd"}}, nil, "absent", "json-null", "no-extra", "numeric-zero")
			checkSearch(map[string]interface{}{"compile_kwd": "null"}, nil, "literal-null")
			checkSearch(map[string]interface{}{"compile_kwd": "TREE'S"}, nil, "target")
			checkSearch(map[string]interface{}{"token_num": 0}, nil, "numeric-zero")

			page, err := engine.Search(ctx, &types.SearchRequest{
				IndexNames: []string{table}, KbIDs: []string{dataset},
				SelectFields: []string{"id"},
				Filter:       map[string]interface{}{"id": []string{"target", "literal-null", "absent"}},
				OrderBy:      (&types.OrderByExpr{}).Desc("outlinks_int").Asc("title_kwd"),
				Limit:        2,
			})
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != 3 || len(page.Chunks) != 2 || page.Chunks[0]["id"] != "absent" || page.Chunks[1]["id"] != "literal-null" {
				t.Fatalf("wiki order should rank 10 before 2, then alpha before Beta: %#v", page)
			}

			if err := engine.UpdateChunks(ctx, matches, map[string]interface{}{
				"compile_kwd": "wiki", "claims": map[string]interface{}{"new": true},
				"children_kwd": []string{"new-child"}, "token_num": 21, "content_with_weight": "updated",
			}, table, dataset); err != nil {
				t.Fatal(err)
			}
			row := checkSearch(map[string]interface{}{"compile_kwd": "wiki", "token_num": 21, "children_kwd": "new-child"},
				[]string{"*", "compile_kwd"}, "target")[0]
			if row["content_with_weight"] != "updated" || !reflect.DeepEqual(row["claims"], map[string]interface{}{"new": true}) ||
				!reflect.DeepEqual(row["children_kwd"], []interface{}{"new-child"}) ||
				asStringMap(row["extra"])["custom_field"] != "keep" {
				t.Fatalf("update lost values or JSON types: %#v", row)
			}
			byID := map[string]interface{}{"id": "target"}
			if err := engine.UpdateChunks(ctx, byID, map[string]interface{}{"remove": "claims"}, table, dataset); err != nil {
				t.Fatal(err)
			}
			row = checkSearch(byID, nil, "target")[0]
			if _, present := row["claims"]; present {
				t.Fatalf("claims survived remove: %#v", row)
			}
			// A row with SQL NULL extra can gain its first JSON key.
			if err := engine.UpdateChunks(ctx, map[string]interface{}{"id": "no-extra"},
				map[string]interface{}{"compile_kwd": compileType}, table, dataset); err != nil {
				t.Fatal(err)
			}
			deleted, err := engine.DeleteChunks(ctx, map[string]interface{}{"compile_kwd": compileType}, table, dataset)
			if err != nil || deleted != 1 {
				t.Fatalf("DeleteChunks() = %d, %v, want one deleted row", deleted, err)
			}
			other, err := engine.GetChunk(ctx, table, "other-kb", []string{"other-kb"})
			if err != nil || asStringMap(other)["compile_kwd"] != compileType {
				t.Fatalf("another dataset's chunk was changed: %#v, %v", other, err)
			}
		})
	}
}

func TestChunkExtraTextUsesTableCollation(t *testing.T) {
	engine := newIntegrationEngine(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	table := "ragflow_go_collation_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	defer cleanupChunkStore(t, engine, table, "")

	// The binary collation differs from the engine's DDL default. Use legacy
	// storage to verify that filters and sorting respect the live table.
	if _, err := engine.db.ExecContext(ctx, "CREATE TABLE "+quoteIdentifier(table)+
		" (id VARCHAR(64) PRIMARY KEY, kb_id VARCHAR(64) NOT NULL, extra JSON) DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_bin"); err != nil {
		t.Fatal(err)
	}
	const dataset = "collation-kb"
	if _, err := engine.db.ExecContext(ctx, "INSERT INTO "+quoteIdentifier(table)+" (id,kb_id,extra) VALUES (?,?,?),(?,?,?)",
		"upper", dataset, `{"compile_kwd":"Tree","title_kwd":"Zebra"}`,
		"lower", dataset, `{"compile_kwd":"tree","title_kwd":"alpha"}`); err != nil {
		t.Fatal(err)
	}

	req := &types.SearchRequest{
		IndexNames: []string{table}, KbIDs: []string{dataset}, SelectFields: []string{"id"},
		OrderBy: (&types.OrderByExpr{}).Asc("title_kwd"),
	}
	result, err := engine.Search(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Chunks) != 2 || result.Chunks[0]["id"] != "upper" || result.Chunks[1]["id"] != "lower" {
		t.Fatalf("expected binary sort order Zebra, alpha: %#v", result)
	}
	req.Filter = map[string]interface{}{"compile_kwd": "Tree"}
	result, err = engine.Search(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Chunks) != 1 || result.Chunks[0]["id"] != "upper" {
		t.Fatalf("case-sensitive filter matched unexpected rows: %#v", result)
	}
	if err := engine.UpdateChunks(ctx, req.Filter, map[string]interface{}{"title_kwd": "Updated"}, table, dataset); err != nil {
		t.Fatal(err)
	}
	assertStoredJSON(ctx, t, engine, table, "extra", "upper", map[string]interface{}{"compile_kwd": "Tree", "title_kwd": "Updated"})
	assertStoredJSON(ctx, t, engine, table, "extra", "lower", map[string]interface{}{"compile_kwd": "tree", "title_kwd": "alpha"})
	deleted, err := engine.DeleteChunks(ctx, req.Filter, table, dataset)
	if err != nil || deleted != 1 {
		t.Fatalf("case-sensitive delete affected %d rows, error %v", deleted, err)
	}
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
