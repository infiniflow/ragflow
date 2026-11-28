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

package service

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/engine/opengauss"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/server/config"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openGaussTestEngine(t *testing.T) *opengauss.Engine {
	t.Helper()
	host := os.Getenv("RAGFLOW_TEST_OPENGAUSS_HOST")
	if host == "" {
		t.Skip("RAGFLOW_TEST_OPENGAUSS_HOST is not set")
	}
	port, err := strconv.Atoi(cmp.Or(os.Getenv("RAGFLOW_TEST_OPENGAUSS_PORT"), "5432"))
	if err != nil {
		t.Fatal(err)
	}
	engine, err := opengauss.NewEngine(config.OpenGaussConfig{Host: host, Port: port,
		User: os.Getenv("RAGFLOW_TEST_OPENGAUSS_USER"), Password: os.Getenv("RAGFLOW_TEST_OPENGAUSS_PASSWORD"),
		Database: cmp.Or(os.Getenv("RAGFLOW_TEST_OPENGAUSS_DATABASE"), "postgres"), Schema: cmp.Or(os.Getenv("RAGFLOW_TEST_OPENGAUSS_SCHEMA"), "public"),
		SSLMode: cmp.Or(os.Getenv("RAGFLOW_TEST_OPENGAUSS_SSL_MODE"), "disable"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine
}

func TestOpenGaussAggregateReferences(t *testing.T) {
	engine := openGaussTestEngine(t)
	ctx := t.Context()
	tenant := fmt.Sprintf("%032x", time.Now().UnixNano())
	table := "ragflow_" + tenant
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := engine.DropChunkStore(cleanup, table, ""); err != nil {
			t.Error(err)
		}
	})
	for _, kb := range []string{scopeAllowedKB, scopeVictimKB} {
		if _, err := engine.InsertChunks(ctx, []map[string]any{{"id": kb, "doc_id": kb, "docnm_kwd": "Notes"}}, table, kb, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := engine.InsertChunks(ctx, []map[string]any{{"id": "other", "doc_id": "other", "docnm_kwd": "Other"}}, table, scopeAllowedKB, ""); err != nil {
		t.Fatal(err)
	}
	kbs := []*entity.Knowledgebase{{ID: scopeAllowedKB, TenantID: tenant}}
	report, ok := addKBFilter("SELECT docnm_kwd, count(*) AS n FROM "+table+" WHERE available_int=1 AND lower(docnm_kwd)='notes' GROUP BY docnm_kwd", engine.GetType(), kbs)
	if !ok {
		t.Fatal("authorized report was rejected")
	}
	rows, err := engine.RunSQL(ctx, report, "json")
	if err != nil || len(rows) != 1 || rows[0]["n"] != int64(1) {
		t.Fatalf("aggregate report: %v, %v", rows, err)
	}
	s := new(ChatPipelineService)
	_, ref := s.buildSQLReference(ctx, engine, table, report, rows, "", engine.GetType(), kbs, nil, true)
	chunks, _ := ref["chunks"].([]map[string]interface{})
	if len(chunks) != 1 || chunks[0]["document_id"] != scopeAllowedKB || chunks[0]["dataset_id"] != scopeAllowedKB {
		t.Fatalf("aggregate citations: %v", chunks)
	}
	kbs[0].TenantID = ""
	chunks, _ = s.fetchAggregateChunks(ctx, engine, table, report, "docnm_kwd", kbs)
	if len(chunks) != 0 {
		t.Fatalf("citations without a tenant: %v", chunks)
	}
}

func TestOpenGaussSkillSearch(t *testing.T) {
	engine := openGaussTestEngine(t)
	ctx := t.Context()
	tenant := strings.Repeat("a", 32)
	space := fmt.Sprintf("%032x", time.Now().UnixNano())
	table := SkillIndexName(tenant, space)
	defer engine.DropChunkStore(ctx, table, "")
	doc := map[string]interface{}{"skill_id": "s1", "name": "quartz", "tags": "mineral", "description": "sample", "content": "unrelated", "version": "1.2", "q_2_vec": []float64{1, 0}}
	if err := engine.IndexDocument(ctx, table, "s1", doc); err != nil {
		t.Fatal(err)
	}
	// Both logical names exceed the database identifier limit and share its first 63 bytes.
	sibling := SkillIndexName(tenant, space[:len(space)-1]+"z")
	if exists, err := engine.ChunkStoreExists(ctx, sibling, "skill"); err != nil || exists {
		t.Fatalf("sibling skill store exists = %t, err = %v", exists, err)
	}
	svc := &SkillSearchService{}
	hits, err := svc.keywordSearch(ctx, engine, table, "quartz", nil, 0, "", "")
	if err != nil || len(hits) != 1 {
		t.Fatalf("name-only match: %v, %v", hits, err)
	}
	if hits[0].Name != "quartz" || hits[0].Description != "sample" || hits[0].Version != "1.2" || len(hits[0].Tags) != 1 || hits[0].Tags[0] != "mineral" {
		t.Fatalf("display fields: %+v", hits[0])
	}
	dense := &types.MatchDenseExpr{VectorColumnName: "q_2_vec", EmbeddingData: []float64{1, 0}, TopN: 1}
	r, err := engine.Search(ctx, &types.SearchRequest{IndexNames: []string{table}, MatchExprs: []interface{}{dense}})
	if err != nil || len(r.Chunks) != 1 || r.Chunks[0]["name"] != "quartz" {
		t.Fatalf("vector search: %v, %v", r, err)
	}
	r, err = engine.Search(ctx, &types.SearchRequest{IndexNames: []string{table}, MatchExprs: []interface{}{&types.MatchTextExpr{MatchingText: "quartz", Fields: []string{"name"}}, dense}})
	if err != nil || len(r.Chunks) != 1 {
		t.Fatalf("text and vector without explicit fusion: %v, %v", r, err)
	}
}

func TestOpenGaussSkillNameMatchingAnotherID(t *testing.T) {
	store := openGaussTestEngine(t)
	ctx := t.Context()
	tenant, space := "skills", strconv.FormatInt(time.Now().UnixNano(), 10)
	table := SkillIndexName(tenant, space)
	defer store.DropChunkStore(ctx, table, "")
	indexer := new(SkillIndexerService)
	for _, skill := range []SkillInfo{
		{ID: "guide", Name: "Original", Version: "1"},
		{ID: "second", Name: "guide", Version: "1"},
		{ID: "second", Name: "guide", Version: "2"},
	} {
		if err := indexer.deleteSkillByName(ctx, tenant, space, skill.Name, store); err != nil {
			t.Fatal(err)
		}
		row := map[string]any{"skill_id": skill.ID, "name": skill.Name, "version": skill.Version}
		if err := store.IndexDocument(ctx, table, skill.ID, row); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.Search(ctx, &types.SearchRequest{IndexNames: []string{table}, SelectFields: []string{"skill_id", "version"}})
	if err != nil {
		t.Fatal(err)
	}
	versions := map[string]any{}
	for _, row := range result.Chunks {
		versions[row["skill_id"].(string)] = row["version"]
	}
	if result.Total != 2 || versions["guide"] != "1" || versions["second"] != "2" {
		t.Fatalf("skill versions after replacement: %v", versions)
	}
}

func TestOpenGaussHydrateChunkVectors(t *testing.T) {
	store := openGaussTestEngine(t)
	ctx := t.Context()
	tenant := strconv.FormatInt(time.Now().UnixNano(), 10)
	table := "ragflow_" + tenant
	defer store.DropChunkStore(ctx, table, "")
	east, west := []float64{1, 0}, []float64{0, 1}
	for kb, vec := range map[string][]float64{"east": east, "west": west, "other": west} {
		if _, err := store.InsertChunks(ctx, []map[string]any{{"id": "same", "q_2_vec": vec}}, table, kb, ""); err != nil {
			t.Fatal(err)
		}
	}
	chunks := []map[string]any{
		{"chunk_id": "same", "kb_id": "east"}, {"chunk_id": "same", "dataset_id": "west"},
		{"chunk_id": "same", "kb_id": "east", "vector": []float64{0, 0}},
		{"chunk_id": "same"}, {"chunk_id": "same", "kb_id": "other"},
	}
	hits, err := HydrateChunkVectors(ctx, map[string]any{"chunks": chunks}, []string{tenant}, []string{"east", "west"}, 2, store)
	if err != nil {
		t.Fatal(err)
	}
	if hits != 3 || !slices.Equal(chunkVector(chunks[0]), east) || !slices.Equal(chunkVector(chunks[1]), west) ||
		!slices.Equal(chunkVector(chunks[2]), east) || chunks[3]["vector"] != nil || chunks[4]["vector"] != nil {
		t.Fatalf("hydrated %d hits: %v", hits, chunks)
	}
	// Chat supplies the tenant IDs and obtains the dataset from each citation.
	chat := []map[string]any{{"chunk_id": "same", "kb_id": []string{"east"}}, {"chunk_id": "same", "kb_id": "west"}}
	hits, err = HydrateChunkVectors(ctx, map[string]any{"chunks": chat}, []string{tenant}, nil, 2, store)
	if err != nil || hits != 2 || !slices.Equal(chunkVector(chat[0]), east) || !slices.Equal(chunkVector(chat[1]), west) {
		t.Fatalf("chat hydrated %d hits: %v, %v", hits, chat, err)
	}
}

func TestOpenGaussRetrievalAcrossDatasets(t *testing.T) {
	engine := openGaussTestEngine(t)
	ctx := t.Context()
	if !tokenizer.IsInitialized() {
		if err := tokenizer.Init(&tokenizer.PoolConfig{MinSize: 1, MaxSize: 2}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tokenizer.Close)
	}
	if err := nlp.InitQueryBuilder(""); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	if err := db.AutoMigrate(&entity.Document{}); err != nil {
		t.Fatal(err)
	}
	original := dao.DB
	dao.DB = db
	defer func() { dao.DB = original }()
	tenant := strconv.FormatInt(time.Now().UnixNano(), 10)
	table := "ragflow_" + tenant
	defer engine.DropChunkStore(ctx, table, "")
	for _, kb := range []string{"east", "west", "deleted"} {
		doc := "doc-" + kb
		if kb != "deleted" {
			if err := db.Create(&entity.Document{ID: doc, KbID: kb, ParserConfig: entity.JSONMap{}}).Error; err != nil {
				t.Fatal(err)
			}
		}
		content := "quartz " + kb
		_, err := engine.InsertChunks(ctx, []map[string]interface{}{{"id": "same", "doc_id": doc, "content_ltks": content, "content_with_weight": content}}, table, kb, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	svc := nlp.NewRetrievalService(engine, &dao.DocumentDAO{})
	threshold, highlight := 0.0, true
	res, err := svc.Retrieval(ctx, &nlp.RetrievalRequest{TenantIDs: []string{tenant}, KbIDs: []string{"east", "west", "deleted"}, Question: "quartz", Language: "en", PageSize: 10, SimilarityThreshold: &threshold, Highlight: &highlight})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Chunks) != 2 {
		t.Fatalf("hits: %+v", res)
	}
	seen := map[string]bool{}
	for _, row := range res.Chunks {
		kb, _ := row["kb_id"].(string)
		snippet, _ := row["highlight"].(string)
		if seen[kb] || (kb != "east" && kb != "west") || row["chunk_id"] != "same" || row["doc_id"] != "doc-"+kb || row["content_with_weight"] != "quartz "+kb || !strings.Contains(snippet, kb) {
			t.Fatalf("incorrect citation: %+v", row)
		}
		seen[kb] = true
	}
}

// Delete an already-read row as soon as the first database read returns,
// reproducing a concurrent writer without relying on goroutine scheduling.
type deletingMetadataReader struct {
	*opengauss.Engine
	deleted bool
}

func (r *deletingMetadataReader) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	result, err := r.Engine.SearchMetadata(ctx, req)
	if err == nil && !r.deleted {
		count, deleteErr := r.DeleteMetadata(ctx, map[string]any{"id": "00000"}, req.TenantID)
		if deleteErr != nil {
			return nil, deleteErr
		}
		if count != 1 {
			return nil, fmt.Errorf("deleted %d metadata rows, want 1", count)
		}
		r.deleted = true
	}
	return result, err
}

func TestOpenGaussMetadataBeyondFirstPage(t *testing.T) {
	store := openGaussTestEngine(t)
	ctx := t.Context()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	defer conn.Close()
	if err := db.AutoMigrate(&entity.Knowledgebase{}); err != nil {
		t.Fatal(err)
	}
	tenant := strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := db.Create(&entity.Knowledgebase{ID: "kb", TenantID: tenant, Name: "reports"}).Error; err != nil {
		t.Fatal(err)
	}
	previous := dao.DB
	dao.DB = db
	defer func() { dao.DB = previous }()
	defer store.DropMetadataStore(ctx, tenant)
	for start := 0; start <= 10000; start += 500 {
		var batch []map[string]any
		for i := start; i < min(start+500, 10001); i++ {
			id := fmt.Sprintf("%05d", i)
			batch = append(batch, map[string]any{"kb_id": "kb", "id": id, "doc_id": id, "meta_fields": map[string]any{"key": id}})
		}
		if _, err := store.InsertMetadata(ctx, batch, tenant); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewMetadataServiceForTest(dao.NewKnowledgebaseDAO(), &deletingMetadataReader{Engine: store})
	result, err := svc.GetFlattedMetaByKBs(ctx, []string{"kb"})
	if err != nil || len(result["key"]) != 10001 || len(result["key"]["10000"]) != 1 {
		t.Fatalf("flattened %d rows, err %v", len(result["key"]), err)
	}
}
