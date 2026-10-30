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

//go:build integration

package opensearch

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/server/config"
)

func getOSTestConfig() config.OpenSearchConfig {
	hosts := common.GetEnv("OS_HOSTS")
	if hosts == "" {
		hosts = common.GetEnv("OS_HOST")
	}
	if hosts == "" {
		hosts = "http://localhost:1201"
	}
	user := common.GetEnv("OS_USERNAME")
	pass := common.GetEnv("OS_PASSWORD")
	if pass == "" && user != "" {
		pass = "infini_rag_flow_OS_01"
	}
	return config.OpenSearchConfig{
		Hosts:              hosts,
		Username:           user,
		Password:           pass,
		InsecureSkipVerify: true,
	}
}

func newIntegrationEngine(t *testing.T) (context.Context, *Engine) {
	t.Helper()
	if common.GetEnv("OS_HOSTS") == "" && common.GetEnv("OS_HOST") == "" {
		t.Skip("skipping OpenSearch integration test: OS_HOSTS or OS_HOST not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	cfg := getOSTestConfig()
	engine, err := NewEngine(ctx, cfg)
	if err != nil {
		t.Fatalf("NewEngine failed against OpenSearch at %s: %v", cfg.Hosts, err)
	}
	t.Cleanup(func() {
		_ = engine.Close()
	})
	return ctx, engine
}

func cleanupIndices(engine *Engine, indices ...string) {
	cleanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, idx := range indices {
		req, _ := http.NewRequestWithContext(cleanCtx, http.MethodDelete, fmt.Sprintf("/%s", idx), nil)
		if resp, err := engine.transportClient.Stream(req); err == nil && resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
}

func refreshIndex(ctx context.Context, engine *Engine, indexName string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("/%s/_refresh", indexName), nil)
	if err == nil {
		if resp, err := engine.transportClient.Stream(req); err == nil && resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
}

// TestIntegration_Lifecycle verifies node connectivity,
// index template setup, and cluster stats query against a real OpenSearch 2.19 instance.
func TestIntegration_Lifecycle(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)

	// Verify that NewEngine created the ragflow_os_mapping composable template
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/_index_template/ragflow_os_mapping", nil)
	if err != nil {
		t.Fatalf("failed to create request for template: %v", err)
	}
	resp, err := engine.transportClient.Stream(req)
	if err != nil {
		t.Fatalf("failed to get index template: %v", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected template ragflow_os_mapping status 200, got %d", resp.StatusCode)
	}

	if err := engine.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestIntegration_ChunkCRUD_AndReadback verifies writing chunks with suffixed fields
// and vector embeddings to a real OpenSearch cluster and reading them back.
func TestIntegration_ChunkCRUD_AndReadback(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "ragflow_itest_crud"
	datasetID := "kb_crud_1"
	chunkID := "chunk_crud_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 8, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	testVec := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8}
	chunk := map[string]interface{}{
		"id":                   chunkID,
		"doc_id":               "doc_crud_1",
		"kb_id":                datasetID,
		"docnm_kwd":            "sample.pdf",
		"content_with_weight":  "OpenSearch chunk CRUD integration test content",
		"create_timestamp_flt": 1700000000.0,
		"page_num_int":         int(1),
		"position_int":         int(42),
		"question_kwd":         []string{"q1", "q2"},
		"q_8_vec":              testVec,
	}

	// 1. Insert chunk
	_, err := engine.InsertChunks(ctx, []map[string]interface{}{chunk}, indexName, datasetID, "")
	if err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}

	refreshIndex(ctx, engine, indexName)

	// 2. Read back chunk and assert all typed fields and vectors
	readBackRaw, err := engine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
	if err != nil {
		t.Fatalf("GetChunk failed: %v", err)
	}
	stored, ok := readBackRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("GetChunk returned %T, want map[string]interface{}", readBackRaw)
	}

	if stored["docnm_kwd"] != "sample.pdf" {
		t.Errorf("docnm_kwd mismatch: got %v", stored["docnm_kwd"])
	}
	if stored["content_with_weight"] != "OpenSearch chunk CRUD integration test content" {
		t.Errorf("content_with_weight mismatch: got %v", stored["content_with_weight"])
	}
	if stored["kb_id"] != datasetID {
		t.Errorf("kb_id mismatch: got %v, want %s", stored["kb_id"], datasetID)
	}
	if stored["create_timestamp_flt"] != 1700000000.0 {
		t.Errorf("create_timestamp_flt mismatch: got %v", stored["create_timestamp_flt"])
	}
	if pageNum, ok := stored["page_num_int"].(float64); !ok || int(pageNum) != 1 {
		t.Errorf("page_num_int mismatch: got %v", stored["page_num_int"])
	}
	if pos, ok := stored["position_int"].(float64); !ok || int(pos) != 42 {
		t.Errorf("position_int mismatch: got %v", stored["position_int"])
	}

	// Assert 8-dimensional vector roundtrip
	vecRaw, ok := stored["q_8_vec"].([]interface{})
	if !ok || len(vecRaw) != 8 {
		t.Fatalf("q_8_vec vector mismatch: got %v", stored["q_8_vec"])
	}
	for i, expected := range testVec {
		if val, ok := vecRaw[i].(float64); !ok || math.Abs(val-expected) > 1e-5 {
			t.Errorf("q_8_vec[%d] mismatch: got %v, want %f", i, vecRaw[i], expected)
		}
	}

	// 3. Update chunk
	updatedChunk := map[string]interface{}{
		"content_with_weight": "Updated content in OpenSearch",
	}
	if err := engine.UpdateChunks(ctx, map[string]interface{}{"id": chunkID}, updatedChunk, indexName, datasetID); err != nil {
		t.Fatalf("UpdateChunks failed: %v", err)
	}

	refreshIndex(ctx, engine, indexName)

	readBackUpdatedRaw, err := engine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
	if err != nil {
		t.Fatalf("GetChunk after update failed: %v", err)
	}
	storedUpdated := readBackUpdatedRaw.(map[string]interface{})
	if storedUpdated["content_with_weight"] != "Updated content in OpenSearch" {
		t.Errorf("updated content mismatch: got %v", storedUpdated["content_with_weight"])
	}

	// 4. Delete chunk
	if _, err := engine.DeleteChunks(ctx, map[string]interface{}{"id": chunkID}, indexName, datasetID); err != nil {
		t.Fatalf("DeleteChunks failed: %v", err)
	}

	refreshIndex(ctx, engine, indexName)

	deletedChunk, err := engine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error after delete: %v", err)
	}
	if deletedChunk != nil {
		t.Errorf("expected chunk to be deleted, got %v", deletedChunk)
	}
}

// TestIntegration_HybridSearch tests executing hybrid search (BM25 + KNN vector query)
// with dynamic OpenSearch normalization pipeline.
func TestIntegration_HybridSearch(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "ragflow_itest_hybrid"
	datasetID := "kb_hybrid_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 8, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	chunks := []map[string]interface{}{
		{
			"id":                  "c1",
			"doc_id":              "d1",
			"kb_id":               datasetID,
			"docnm_kwd":           "doc1.txt",
			"content_with_weight": "Artificial intelligence and machine learning algorithms",
			"content_ltks":        "Artificial intelligence and machine learning algorithms",
			"q_8_vec":             []float64{0.9, 0.1, 0.1, 0.0, 0.0, 0.0, 0.0, 0.1},
		},
		{
			"id":                  "c2",
			"doc_id":              "d2",
			"kb_id":               datasetID,
			"docnm_kwd":           "doc2.txt",
			"content_with_weight": "Deep neural networks and computer vision models",
			"content_ltks":        "Deep neural networks and computer vision models",
			"q_8_vec":             []float64{0.1, 0.9, 0.1, 0.0, 0.0, 0.0, 0.0, 0.1},
		},
		{
			"id":                  "c3",
			"doc_id":              "d3",
			"kb_id":               datasetID,
			"docnm_kwd":           "doc3.txt",
			"content_with_weight": "Distributed systems and cloud computing infrastructure",
			"content_ltks":        "Distributed systems and cloud computing infrastructure",
			"q_8_vec":             []float64{0.0, 0.0, 0.0, 0.0, 0.9, 0.1, 0.1, 0.0},
		},
	}

	if _, err := engine.InsertChunks(ctx, chunks, indexName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}

	refreshIndex(ctx, engine, indexName)

	// Hybrid search: Text match for "artificial intelligence" + Vector match near c1
	queryVec := []float64{0.85, 0.15, 0.1, 0.0, 0.0, 0.0, 0.0, 0.1}
	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_ltks"},
				MatchingText: "artificial intelligence",
				TopN:         10,
			},
			&types.MatchDenseExpr{
				VectorColumnName: "q_8_vec",
				EmbeddingData:    queryVec,
				TopN:             10,
			},
			&types.FusionExpr{
				Method: "weighted_sum",
				TopN:   10,
				FusionParams: map[string]interface{}{
					"weights": "0.5,0.5",
				},
			},
		},
		Limit: 10,
	}

	res, err := engine.Search(ctx, searchReq)
	if err != nil {
		t.Fatalf("Hybrid Search failed: %v", err)
	}

	if len(res.Chunks) == 0 {
		t.Fatalf("expected hybrid search to return chunks, got 0")
	}

	// Verify top hit is c1
	topChunk := res.Chunks[0]
	if topChunk["_id"] != "c1" {
		t.Errorf("expected top hit to be c1, got %v", topChunk["_id"])
	}
	if topChunk["_index"] != indexName {
		t.Errorf("expected _index backfilled as %s, got %v", indexName, topChunk["_index"])
	}
	score, ok := topChunk["_score"].(float64)
	if !ok || score <= 0 {
		t.Errorf("expected positive float64 _score, got %v", topChunk["_score"])
	}
}

// TestIntegration_SQL tests the SQL plugin integration, named MATCH queries, and null handling.
func TestIntegration_SQL(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "ragflow_itest_sql"
	datasetID := "kb_sql_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 0, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	chunks := []map[string]interface{}{
		{
			"id":                  "sql_c1",
			"doc_id":              "doc_sql_1",
			"kb_id":               datasetID,
			"docnm_kwd":           "report_alpha.pdf",
			"title_kwd":           "", // explicit empty string
			"content_with_weight": "Financial report alpha quarterly statistics",
			"content_ltks":        "financial report alpha quarterly statistics",
			"page_num_int":        1,
		},
		{
			"id":        "sql_c2",
			"doc_id":    "doc_sql_2",
			"kb_id":     datasetID,
			"docnm_kwd": "report_beta.pdf",
			// title_kwd omitted to test SQL NULL
			"content_with_weight": "Financial report beta annual forecast",
			"content_ltks":        "financial report beta annual forecast",
			"page_num_int":        2,
		},
	}

	if _, err := engine.InsertChunks(ctx, chunks, indexName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}

	refreshIndex(ctx, engine, indexName)

	// 1. Run valid standard SQL query with NULL and empty string verification
	sqlQuery := fmt.Sprintf("SELECT doc_id, docnm_kwd, title_kwd, page_num_int FROM `%s` WHERE page_num_int > 0 ORDER BY doc_id ASC", indexName)
	rows, err := engine.RunSQL(ctx, sqlQuery, "json")
	if err != nil {
		t.Fatalf("RunSQL failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Verify NULL vs empty string:
	// Row 0 has title_kwd as ""; must not be nil.
	val0, ok0 := rows[0]["title_kwd"]
	if !ok0 {
		t.Errorf("expected title_kwd key to exist in row 0")
	}
	if val0 != "" {
		t.Errorf("expected empty string for title_kwd in row 0, got %v (%T)", val0, val0)
	}

	// Row 1 has title_kwd omitted/NULL; must be nil, not empty string.
	val1, ok1 := rows[1]["title_kwd"]
	if !ok1 {
		t.Errorf("expected title_kwd key to exist in row 1")
	}
	if val1 != nil {
		t.Errorf("expected nil for null title_kwd in row 1, got %v (%T)", val1, val1)
	}

	// 2. Format validation: default empty format succeeds; unsupported format errors out
	if defaultRows, err := engine.RunSQL(ctx, sqlQuery, ""); err != nil || len(defaultRows) != 2 {
		t.Errorf("expected default empty format to succeed, got rows=%v, err=%v", defaultRows, err)
	}
	if _, err := engine.RunSQL(ctx, sqlQuery, "csv"); err == nil {
		t.Errorf("expected error for unsupported format 'csv', got nil")
	}

	// 3. Run named MATCH rewrite query via content_ltks LIKE 'financial'
	matchSQL := fmt.Sprintf("SELECT doc_id, docnm_kwd FROM `%s` WHERE content_ltks LIKE 'financial'", indexName)
	matchRows, err := engine.RunSQL(ctx, matchSQL, "json")
	if err != nil {
		t.Fatalf("RunSQL with MATCH rewrite failed: %v", err)
	}
	if len(matchRows) != 2 {
		t.Errorf("expected 2 rows from MATCH query, got %d", len(matchRows))
	}

	// 4. Empty rows sentinel test
	emptySQL := fmt.Sprintf("SELECT doc_id FROM `%s` WHERE page_num_int > 999", indexName)
	emptyRows, err := engine.RunSQL(ctx, emptySQL, "json")
	if err != nil {
		t.Fatalf("RunSQL for empty result failed: %v", err)
	}
	if emptyRows != nil {
		t.Errorf("expected (nil, nil) sentinel on empty result, got %v", emptyRows)
	}
}

// TestIntegration_DeepPagination_SearchAfter verifies cursor progression across multiple pages
// and verifies deep pagination engaging search_after beyond MAX_RESULT_WINDOW.
func TestIntegration_DeepPagination_SearchAfter(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "ragflow_itest_deep_page"
	datasetID := "kb_page_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 0, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	var chunks []map[string]interface{}
	for i := 1; i <= 15; i++ {
		chunks = append(chunks, map[string]interface{}{
			"id":                   fmt.Sprintf("page_c%02d", i),
			"doc_id":               "doc_p",
			"kb_id":                datasetID,
			"docnm_kwd":            "pagination_test.pdf",
			"content_with_weight":  fmt.Sprintf("Chunk page item %d", i),
			"content_ltks":         fmt.Sprintf("Chunk page item %d", i),
			"create_timestamp_flt": float64(1700000000 + i),
			"available_int":        1,
		})
	}

	if _, err := engine.InsertChunks(ctx, chunks, indexName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}

	refreshIndex(ctx, engine, indexName)

	// 1. Standard pagination across pages using Search with OrderBy and Offset
	pageSize := 5
	var collectedIDs []string

	for page := 0; page < 3; page++ {
		req := &types.SearchRequest{
			IndexNames: []string{indexName},
			KbIDs:      []string{datasetID},
			MatchExprs: []interface{}{
				&types.MatchTextExpr{
					Fields:       []string{"content_ltks"},
					MatchingText: "Chunk page item",
				},
			},
			OrderBy: &types.OrderByExpr{
				Fields: []types.OrderByField{
					{Field: "create_timestamp_flt", Type: types.SortAsc},
				},
			},
			Offset: page * pageSize,
			Limit:  pageSize,
		}

		res, err := engine.Search(ctx, req)
		if err != nil {
			t.Fatalf("Search page %d failed: %v", page, err)
		}
		if len(res.Chunks) == 0 {
			break
		}

		for _, ch := range res.Chunks {
			id, _ := ch["_id"].(string)
			collectedIDs = append(collectedIDs, id)
		}
	}

	if len(collectedIDs) != 15 {
		t.Fatalf("expected 15 paginated chunks, got %d: %v", len(collectedIDs), collectedIDs)
	}

	// Verify no duplicates
	seen := make(map[string]bool)
	for _, id := range collectedIDs {
		if seen[id] {
			t.Errorf("duplicate chunk ID during pagination: %s", id)
		}
		seen[id] = true
	}

	// 2. Genuinely trigger search_after by setting offset > MAX_RESULT_WINDOW (10000)
	deepReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_ltks"},
				MatchingText: "Chunk page item",
			},
		},
		OrderBy: &types.OrderByExpr{
			Fields: []types.OrderByField{
				{Field: "create_timestamp_flt", Type: types.SortAsc},
			},
		},
		Offset: common.MAX_RESULT_WINDOW + 5,
		Limit:  5,
	}

	deepRes, err := engine.Search(ctx, deepReq)
	if err != nil {
		t.Fatalf("Search beyond MAX_RESULT_WINDOW using search_after failed: %v", err)
	}
	if deepRes.Total != 15 {
		t.Errorf("expected total hits 15 from deep pagination, got %d", deepRes.Total)
	}
}

// TestIntegration_MetadataOperations tests metadata document lifecycle and queries.
func TestIntegration_MetadataOperations(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	tenantID := "itest_meta_tenant"
	docID := "doc_meta_01"
	kbID := "kb_meta_01"

	metaIndex := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
	cleanupIndices(engine, metaIndex)
	t.Cleanup(func() { cleanupIndices(engine, metaIndex) })

	if err := engine.CreateMetadataStore(ctx, tenantID); err != nil {
		t.Fatalf("CreateMetadataStore failed: %v", err)
	}

	meta := map[string]interface{}{
		"id":    docID,
		"kb_id": kbID,
		"title": "Metadata Title Test",
		"meta_fields": map[string]interface{}{
			"category":    "finance",
			"status_code": 200,
		},
	}

	// 1. Insert metadata
	_, err := engine.InsertMetadata(ctx, []map[string]interface{}{meta}, tenantID)
	if err != nil {
		t.Fatalf("InsertMetadata failed: %v", err)
	}

	refreshIndex(ctx, engine, metaIndex)

	// 2. Get document
	gotDocRaw, err := engine.GetDocument(ctx, metaIndex, docID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	gotMeta, ok := gotDocRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("GetDocument returned %T, want map[string]interface{}", gotDocRaw)
	}
	if gotMeta["title"] != "Metadata Title Test" {
		t.Errorf("GetDocument title mismatch: %v", gotMeta["title"])
	}

	// 3. Update metadata
	metaUpdates := map[string]interface{}{
		"category": "legal",
		"verified": true,
	}
	if err := engine.UpdateMetadata(ctx, docID, kbID, metaUpdates, tenantID); err != nil {
		t.Fatalf("UpdateMetadata failed: %v", err)
	}

	refreshIndex(ctx, engine, metaIndex)

	gotDocUpdatedRaw, err := engine.GetDocument(ctx, metaIndex, docID)
	if err != nil {
		t.Fatalf("GetDocument after update failed: %v", err)
	}
	gotMetaUpdated := gotDocUpdatedRaw.(map[string]interface{})
	mf, ok := gotMetaUpdated["meta_fields"].(map[string]interface{})
	if !ok || mf["category"] != "legal" {
		t.Errorf("expected meta_fields.category 'legal', got %v", gotMetaUpdated["meta_fields"])
	}

	// 4. SearchMetadata
	searchRes, err := engine.SearchMetadata(ctx, &types.SearchMetadataRequest{
		TenantID: tenantID,
		Filter: map[string]interface{}{
			"kb_id": kbID,
		},
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchMetadata failed: %v", err)
	}
	if searchRes.Total != 1 || len(searchRes.MetadataRecords) != 1 {
		t.Fatalf("expected 1 doc from SearchMetadata, got total=%d docs=%d", searchRes.Total, len(searchRes.MetadataRecords))
	}
	if searchRes.MetadataRecords[0]["id"] != docID {
		t.Errorf("expected doc %s, got %v", docID, searchRes.MetadataRecords[0]["id"])
	}

	// 5. Delete metadata
	deletedCount, err := engine.DeleteMetadata(ctx, map[string]interface{}{"id": docID}, tenantID)
	if err != nil {
		t.Fatalf("DeleteMetadata failed: %v", err)
	}
	if deletedCount != 1 {
		t.Errorf("expected 1 deleted metadata, got %d", deletedCount)
	}
}

// TestIntegration_RegexpSearch tests native Lucene regexp execution on content_with_weight.
func TestIntegration_RegexpSearch(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	tenantID := "itest_regexp_tenant"
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	datasetID := "kb_regexp_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 0, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	chunks := []map[string]interface{}{
		{
			"id":                  "reg_c1",
			"doc_id":              "reg_doc_1",
			"kb_id":               datasetID,
			"content_with_weight": "Specialized alpha-9942 test record pattern",
		},
		{
			"id":                  "reg_c2",
			"doc_id":              "reg_doc_2",
			"kb_id":               datasetID,
			"content_with_weight": "Standard non-matching chunk description",
		},
	}

	if _, err := engine.InsertChunks(ctx, chunks, indexName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}

	refreshIndex(ctx, engine, indexName)

	regexpReq := &types.RegexpSearchRequest{
		TenantID:     tenantID,
		Pattern:      "alpha-[0-9]{4}",
		KbIDs:        []string{datasetID},
		SelectFields: []string{"id", "content_with_weight"},
	}

	res, err := engine.SearchByRegexp(ctx, regexpReq)
	if err != nil {
		t.Fatalf("SearchByRegexp failed: %v", err)
	}

	if res.Total != 1 || len(res.Chunks) != 1 {
		t.Fatalf("expected 1 regexp hit, got total=%d chunks=%d", res.Total, len(res.Chunks))
	}
	if res.Chunks[0]["_id"] != "reg_c1" {
		t.Errorf("expected chunk reg_c1, got %v", res.Chunks[0]["_id"])
	}
}

func TestIntegration_HybridWeightsAndPagination(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "ragflow_itest_hybrid_page"
	datasetID := "kb_hybrid_page_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 8, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	chunks := []map[string]interface{}{
		{
			"id":                  "hp1",
			"doc_id":              "d1",
			"kb_id":               datasetID,
			"content_with_weight": "quantum quantum computing quantum processor",
			"content_ltks":        "quantum quantum computing quantum processor",
			"q_8_vec":             []float64{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.9},
		},
		{
			"id":                  "hp2",
			"doc_id":              "d2",
			"kb_id":               datasetID,
			"content_with_weight": "quantum entanglement physics",
			"content_ltks":        "quantum entanglement physics",
			"q_8_vec":             []float64{0.8, 0.1, 0.1, 0.0, 0.0, 0.0, 0.0, 0.1},
		},
		{
			"id":                  "hp3",
			"doc_id":              "d3",
			"kb_id":               datasetID,
			"content_with_weight": "general physics mechanics",
			"content_ltks":        "general physics mechanics",
			"q_8_vec":             []float64{0.75, 0.15, 0.1, 0.0, 0.0, 0.0, 0.0, 0.1},
		},
		{
			"id":                  "hp4",
			"doc_id":              "d4",
			"kb_id":               datasetID,
			"content_with_weight": "quantum chemistry simulation",
			"content_ltks":        "quantum chemistry simulation",
			"q_8_vec":             []float64{0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2, 0.2},
		},
		{
			"id":                  "hp5",
			"doc_id":              "d5",
			"kb_id":               datasetID,
			"content_with_weight": "superconducting circuits quantum",
			"content_ltks":        "superconducting circuits quantum",
			"q_8_vec":             []float64{0.3, 0.3, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1},
		},
		{
			"id":                  "hp6",
			"doc_id":              "d6",
			"kb_id":               datasetID,
			"content_with_weight": "classical algorithm complexity",
			"content_ltks":        "classical algorithm complexity",
			"q_8_vec":             []float64{0.0, 0.0, 0.0, 0.0, 0.5, 0.5, 0.5, 0.5},
		},
	}

	if _, err := engine.InsertChunks(ctx, chunks, indexName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}
	refreshIndex(ctx, engine, indexName)

	queryVec := []float64{0.85, 0.15, 0.1, 0.0, 0.0, 0.0, 0.0, 0.1}

	// 1. Text-heavy weights "0.95,0.05"
	textHeavyReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_ltks"},
				MatchingText: "quantum",
				TopN:         10,
			},
			&types.MatchDenseExpr{
				VectorColumnName: "q_8_vec",
				EmbeddingData:    queryVec,
				TopN:             10,
			},
			&types.FusionExpr{
				Method: "weighted_sum",
				TopN:   10,
				FusionParams: map[string]interface{}{
					"weights": "0.95,0.05",
				},
			},
		},
		Limit: 10,
	}
	resTextHeavy, err := engine.Search(ctx, textHeavyReq)
	if err != nil {
		t.Fatalf("Search textHeavy failed: %v", err)
	}
	if len(resTextHeavy.Chunks) == 0 {
		t.Fatalf("expected chunks in textHeavy search")
	}

	// 2. Vector-heavy weights "0.05,0.95"
	vectorHeavyReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_ltks"},
				MatchingText: "quantum",
				TopN:         10,
			},
			&types.MatchDenseExpr{
				VectorColumnName: "q_8_vec",
				EmbeddingData:    queryVec,
				TopN:             10,
			},
			&types.FusionExpr{
				Method: "weighted_sum",
				TopN:   10,
				FusionParams: map[string]interface{}{
					"weights": "0.05,0.95",
				},
			},
		},
		Limit: 10,
	}
	resVectorHeavy, err := engine.Search(ctx, vectorHeavyReq)
	if err != nil {
		t.Fatalf("Search vectorHeavy failed: %v", err)
	}
	if len(resVectorHeavy.Chunks) == 0 {
		t.Fatalf("expected chunks in vectorHeavy search")
	}

	// 3. Multi-page pagination: page 1 (offset=0, limit=2), page 2 (offset=2, limit=2)
	page1Req := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: vectorHeavyReq.MatchExprs,
		Offset:     0,
		Limit:      2,
	}
	resPage1, err := engine.Search(ctx, page1Req)
	if err != nil {
		t.Fatalf("Search page 1 failed: %v", err)
	}
	if len(resPage1.Chunks) != 2 {
		t.Fatalf("expected 2 chunks on page 1, got %d", len(resPage1.Chunks))
	}

	page2Req := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: vectorHeavyReq.MatchExprs,
		Offset:     2,
		Limit:      2,
	}
	resPage2, err := engine.Search(ctx, page2Req)
	if err != nil {
		t.Fatalf("Search page 2 failed: %v", err)
	}
	if len(resPage2.Chunks) != 2 {
		t.Fatalf("expected 2 chunks on page 2, got %d", len(resPage2.Chunks))
	}

	// Ensure no duplicate IDs between page 1 and page 2
	p1IDs := map[string]bool{
		resPage1.Chunks[0]["_id"].(string): true,
		resPage1.Chunks[1]["_id"].(string): true,
	}
	if p1IDs[resPage2.Chunks[0]["_id"].(string)] || p1IDs[resPage2.Chunks[1]["_id"].(string)] {
		t.Errorf("overlap detected between page 1 and page 2: page1=%v, page2=%v",
			resPage1.Chunks, resPage2.Chunks)
	}
}

func TestIntegration_FilteredVectorSearch(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "ragflow_itest_filtered_vec"
	datasetID := "kb_filtered_vec_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 8, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	chunks := []map[string]interface{}{
		{
			"id":                  "fv1",
			"doc_id":              "doc_alpha",
			"kb_id":               datasetID,
			"content_with_weight": "Alpha chunk content",
			"q_8_vec":             []float64{0.9, 0.1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		},
		{
			"id":                  "fv2",
			"doc_id":              "doc_beta",
			"kb_id":               datasetID,
			"content_with_weight": "Beta chunk content",
			"q_8_vec":             []float64{0.95, 0.05, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		},
	}

	if _, err := engine.InsertChunks(ctx, chunks, indexName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}
	refreshIndex(ctx, engine, indexName)

	queryVec := []float64{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		Filter: map[string]interface{}{
			"doc_id": "doc_alpha",
		},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "q_8_vec",
				EmbeddingData:    queryVec,
				TopN:             10,
			},
		},
		Limit: 10,
	}

	res, err := engine.Search(ctx, searchReq)
	if err != nil {
		t.Fatalf("Filtered vector search failed: %v", err)
	}
	if len(res.Chunks) != 1 {
		t.Fatalf("expected exactly 1 chunk, got %d", len(res.Chunks))
	}
	if res.Chunks[0]["_id"] != "fv1" {
		t.Errorf("expected hit fv1, got %v", res.Chunks[0]["_id"])
	}
}

func TestIntegration_SkillIndex(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "skill_itest_tenant"
	datasetID := "skill"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 8, "", ""); err != nil {
		t.Fatalf("CreateChunkStore for skill failed: %v", err)
	}

	skillDoc := map[string]interface{}{
		"id":       "skill_doc_1",
		"name":     "weather forecast",
		"name_tks": "weather forecast",
		"status":   "1",
		"q_8_vec":  []float64{0.1, 0.8, 0.1, 0.0, 0.0, 0.0, 0.0, 0.0},
	}

	if err := engine.IndexDocument(ctx, indexName, "skill_doc_1", skillDoc); err != nil {
		t.Fatalf("IndexDocument failed: %v", err)
	}
	refreshIndex(ctx, engine, indexName)

	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				MatchingText: "weather",
			},
		},
		Limit: 5,
	}

	res, err := engine.Search(ctx, searchReq)
	if err != nil {
		t.Fatalf("Search skill index failed: %v", err)
	}
	if len(res.Chunks) != 1 {
		t.Fatalf("expected 1 skill chunk, got %d", len(res.Chunks))
	}
	if res.Chunks[0]["_id"] != "skill_doc_1" {
		t.Errorf("expected skill_doc_1, got %v", res.Chunks[0]["_id"])
	}
}

func TestIntegration_MultiVectorDimensions(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	indexName := "ragflow_itest_multivec"
	datasetID := "kb_multivec_1"

	cleanupIndices(engine, indexName)
	t.Cleanup(func() { cleanupIndices(engine, indexName) })

	// Initial store created with 8 dimensions
	if err := engine.CreateChunkStore(ctx, indexName, datasetID, 8, "", ""); err != nil {
		t.Fatalf("CreateChunkStore failed: %v", err)
	}

	// Insert chunks with both 8-dim and 16-dim vectors
	chunks := []map[string]interface{}{
		{
			"id":                  "mv1",
			"doc_id":              "d_mv",
			"kb_id":               datasetID,
			"content_with_weight": "Multi vector chunk one",
			"q_8_vec":             []float64{0.9, 0.1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
			"q_16_vec":            []float64{0.9, 0.1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.1, 0.1, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		},
	}

	if _, err := engine.InsertChunks(ctx, chunks, indexName, datasetID, ""); err != nil {
		t.Fatalf("InsertChunks failed: %v", err)
	}
	refreshIndex(ctx, engine, indexName)

	// Search via q_8_vec
	res8, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "q_8_vec",
				EmbeddingData:    []float64{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
				TopN:             5,
			},
		},
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search q_8_vec failed: %v", err)
	}
	if len(res8.Chunks) != 1 {
		t.Fatalf("expected 1 chunk on q_8_vec, got %d", len(res8.Chunks))
	}

	// Search via q_16_vec
	res16, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{indexName},
		KbIDs:      []string{datasetID},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "q_16_vec",
				EmbeddingData:    []float64{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
				TopN:             5,
			},
		},
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search q_16_vec failed: %v", err)
	}
	if len(res16.Chunks) != 1 {
		t.Fatalf("expected 1 chunk on q_16_vec, got %d", len(res16.Chunks))
	}
}

func TestIntegration_HighDimensionalVectors_1536_and_4096(t *testing.T) {
	ctx, engine := newIntegrationEngine(t)
	tenantID := "test_tenant"

	// 1. Test 1536 dimensions (standard OpenAI text-embedding-3-small/ada-002)
	idx1536 := fmt.Sprintf("ragflow_%s_kb_dim1536", tenantID)
	cleanupIndices(engine, idx1536)
	t.Cleanup(func() { cleanupIndices(engine, idx1536) })

	if err := engine.CreateChunkStore(ctx, idx1536, "kb_dim1536", 1536, "", ""); err != nil {
		t.Fatalf("CreateChunkStore for 1536 dims failed: %v", err)
	}

	vec1536 := make([]float64, 1536)
	vec1536[0] = 0.95
	vec1536[1535] = 0.05
	chunk1536 := []map[string]interface{}{
		{
			"id":                  "c1536",
			"doc_id":              "doc_1536",
			"kb_id":               "kb_dim1536",
			"content_with_weight": "1536 dimensional embedding test document",
			"q_1536_vec":          vec1536,
		},
	}
	if _, err := engine.InsertChunks(ctx, chunk1536, idx1536, "kb_dim1536", ""); err != nil {
		t.Fatalf("InsertChunks for 1536 dims failed: %v", err)
	}
	refreshIndex(ctx, engine, idx1536)

	query1536 := make([]float64, 1536)
	query1536[0] = 1.0
	res1536, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{idx1536},
		KbIDs:      []string{"kb_dim1536"},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "q_1536_vec",
				EmbeddingData:    query1536,
				TopN:             5,
			},
		},
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search on 1536 dims failed: %v", err)
	}
	if len(res1536.Chunks) != 1 || res1536.Chunks[0]["_id"] != "c1536" {
		t.Fatalf("expected chunk c1536, got: %v", res1536.Chunks)
	}

	// 2. Test 4096 dimensions (standard large embedding / LLM hidden state)
	idx4096 := fmt.Sprintf("ragflow_%s_kb_dim4096", tenantID)
	cleanupIndices(engine, idx4096)
	t.Cleanup(func() { cleanupIndices(engine, idx4096) })

	if err := engine.CreateChunkStore(ctx, idx4096, "kb_dim4096", 4096, "", ""); err != nil {
		t.Fatalf("CreateChunkStore for 4096 dims failed: %v", err)
	}

	vec4096 := make([]float64, 4096)
	vec4096[0] = 0.99
	vec4096[4095] = 0.01
	chunk4096 := []map[string]interface{}{
		{
			"id":                  "c4096",
			"doc_id":              "doc_4096",
			"kb_id":               "kb_dim4096",
			"content_with_weight": "4096 dimensional embedding test document",
			"q_4096_vec":          vec4096,
		},
	}
	if _, err := engine.InsertChunks(ctx, chunk4096, idx4096, "kb_dim4096", ""); err != nil {
		t.Fatalf("InsertChunks for 4096 dims failed: %v", err)
	}
	refreshIndex(ctx, engine, idx4096)

	query4096 := make([]float64, 4096)
	query4096[0] = 1.0
	res4096, err := engine.Search(ctx, &types.SearchRequest{
		IndexNames: []string{idx4096},
		KbIDs:      []string{"kb_dim4096"},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "q_4096_vec",
				EmbeddingData:    query4096,
				TopN:             5,
			},
		},
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("Search on 4096 dims failed: %v", err)
	}
	if len(res4096.Chunks) != 1 || res4096.Chunks[0]["_id"] != "c4096" {
		t.Fatalf("expected chunk c4096, got: %v", res4096.Chunks)
	}
}
