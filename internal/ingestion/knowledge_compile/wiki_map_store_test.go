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

package knowledge_compile

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

type wikiMapStoreEngine struct {
	fakeEngine
	rows              map[string]map[string]interface{}
	inserted          []map[string]interface{}
	insertBase        string
	insertDataset     string
	engineType        string
	chunkStoreExists  bool
	chunkStoreChecks  int
	createdStores     int
	createdVectorSize int
	createdLanguage   string
}

func (e *wikiMapStoreEngine) GetType() string {
	if e.engineType != "" {
		return e.engineType
	}
	return e.fakeEngine.GetType()
}

func (e *wikiMapStoreEngine) ChunkStoreExists(context.Context, string, string) (bool, error) {
	e.chunkStoreChecks++
	return e.chunkStoreExists, nil
}

func (e *wikiMapStoreEngine) CreateChunkStore(_ context.Context, _, _ string, vectorSize int, _, language string) error {
	e.createdStores++
	e.createdVectorSize = vectorSize
	e.createdLanguage = language
	e.chunkStoreExists = true
	return nil
}

func (e *wikiMapStoreEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	e.lastSearchReq = req
	var ids []string
	switch value := req.Filter["id"].(type) {
	case []string:
		ids = value
	case string:
		ids = []string{value}
	}
	if len(ids) == 0 {
		for id := range e.rows {
			ids = append(ids, id)
		}
	}
	chunks := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		if row, ok := e.rows[id]; ok && rowMatchesFilter(row, req.Filter) {
			chunks = append(chunks, row)
		}
	}
	return &types.SearchResult{Chunks: chunks, Total: int64(len(chunks))}, nil
}

func rowMatchesFilter(row map[string]interface{}, filter map[string]interface{}) bool {
	return matchesStoredCompilationRow(row, types.CompilationFilter(filter))
}

func matchesStoredCompilationRow(row, filter map[string]interface{}) bool {
	for field, value := range filter {
		switch field {
		case "and":
			for _, child := range types.FilterClauses(value) {
				if !matchesStoredCompilationRow(row, child) {
					return false
				}
			}
		case "or":
			matched := false
			for _, child := range types.FilterClauses(value) {
				matched = matched || matchesStoredCompilationRow(row, child)
			}
			if !matched {
				return false
			}
		case "must_not":
			if matchesStoredCompilationRow(row, value.(map[string]interface{})) {
				return false
			}
		case "exists":
			if mapStoreString(row[value.(string)]) == "" {
				return false
			}
		default:
			if expected, ok := value.([]string); ok {
				matched := false
				for _, v := range expected {
					matched = matched || mapStoreString(row[field]) == v
				}
				if !matched {
					return false
				}
			} else if !reflect.DeepEqual(row[field], value) {
				return false
			}
		}
	}
	return true
}

func (e *wikiMapStoreEngine) InsertChunks(_ context.Context, chunks []map[string]interface{}, baseName, datasetID, _ string) ([]string, error) {
	e.insertBase = baseName
	e.insertDataset = datasetID
	for _, chunk := range chunks {
		copyOfChunk := make(map[string]interface{}, len(chunk))
		for key, value := range chunk {
			copyOfChunk[key] = value
		}
		e.inserted = append(e.inserted, copyOfChunk)
		if e.rows == nil {
			e.rows = map[string]map[string]interface{}{}
		}
		e.rows[mapStoreString(chunk["id"])] = copyOfChunk
	}
	return nil, nil
}

func TestWikiMapVersionStoreUsesNonSearchableDocStoreRows(t *testing.T) {
	engine := &wikiMapStoreEngine{rows: map[string]map[string]interface{}{}}
	store := NewWikiMapVersionStoreWithVectorSizeResolver(engine, nil)
	version := kccommon.WikiMapVersion{
		Key:                 "version-a",
		TenantID:            "tenant-1",
		DatasetID:           "kb-1",
		DocumentID:          "doc-1",
		ChunkID:             "chunk-1",
		ContentHash:         "hash-a",
		TemplateFingerprint: "template-a",
		LLMFingerprint:      "llm-a",
		Payload:             []byte(`{"topics":["original"]}`),
	}
	if err := store.PutWikiMapVersions(t.Context(), []kccommon.WikiMapVersion{version}); err != nil {
		t.Fatalf("PutWikiMapVersions() error = %v", err)
	}
	if len(engine.inserted) != 1 {
		t.Fatalf("inserted rows = %d, want 1", len(engine.inserted))
	}
	row := engine.inserted[0]
	for key, want := range map[string]interface{}{
		"id":             "version-a",
		"doc_id":         "wiki_map_cache:doc-1",
		"kb_id":          "kb-1",
		"compile_kwd":    "wiki",
		"type_kwd":       wikiMapExtractCompileKWD,
		"available_int":  0,
		"chunk_hash_kwd": "hash-a",
	} {
		if got := row[key]; !reflect.DeepEqual(got, want) {
			t.Errorf("row[%q] = %#v, want %#v", key, got, want)
		}
	}
	if got := row["source_chunk_ids"]; !reflect.DeepEqual(got, []string{"chunk-1"}) {
		t.Errorf("source_chunk_ids = %#v", got)
	}
	if got := row["source_doc_ids"]; !reflect.DeepEqual(got, []string{"doc-1"}) {
		t.Errorf("source_doc_ids = %#v", got)
	}
	if _, exists := row["q_1024_vec"]; exists {
		t.Fatal("MAP cache row must not carry an embedding")
	}
	if engine.insertBase != "ragflow_tenant-1" || engine.insertDataset != "kb-1" {
		t.Fatalf("insert scope = %q/%q", engine.insertBase, engine.insertDataset)
	}

	version.Payload = []byte(`{"topics":["replacement"]}`)
	if err := store.PutWikiMapVersions(t.Context(), []kccommon.WikiMapVersion{version}); err != nil {
		t.Fatalf("duplicate PutWikiMapVersions() error = %v", err)
	}
	if len(engine.inserted) != 1 {
		t.Fatalf("immutable duplicate inserted; rows = %d, want 1", len(engine.inserted))
	}

	got, err := store.GetWikiMapVersions(t.Context(), "tenant-1", "kb-1", []string{"version-a", "missing"})
	if err != nil {
		t.Fatalf("GetWikiMapVersions() error = %v", err)
	}
	if string(got["version-a"]) != `{"topics":["original"]}` {
		t.Fatalf("stored payload = %s, want immutable original", got["version-a"])
	}
	if engine.lastSearchReq == nil || !reflect.DeepEqual(engine.lastSearchReq.KbIDs, []string{"kb-1"}) {
		t.Fatalf("search KbIDs = %#v", engine.lastSearchReq)
	}
}

func TestWikiMapVersionStoreSkipsExistingInfinityStoreBootstrap(t *testing.T) {
	engine := &wikiMapStoreEngine{
		rows:             map[string]map[string]interface{}{},
		engineType:       "infinity",
		chunkStoreExists: true,
	}
	resolverCalls := 0
	store := NewWikiMapVersionStoreWithVectorSizeResolver(engine, func(context.Context) (int, error) {
		resolverCalls++
		return 0, nil
	})
	version := kccommon.WikiMapVersion{
		Key: "version-a", TenantID: "tenant-1", DatasetID: "kb-1",
		DocumentID: "doc-1", ChunkID: "chunk-1", Payload: []byte(`{}`),
	}

	if err := store.PutWikiMapVersions(t.Context(), []kccommon.WikiMapVersion{version}); err != nil {
		t.Fatalf("PutWikiMapVersions() error = %v", err)
	}
	if resolverCalls != 0 || engine.createdStores != 0 || engine.chunkStoreChecks != 1 {
		t.Fatalf("bootstrap calls: resolver=%d create=%d checks=%d", resolverCalls, engine.createdStores, engine.chunkStoreChecks)
	}
}

func TestWikiMapVersionStoreCreatesMissingInfinityStore(t *testing.T) {
	engine := &wikiMapStoreEngine{rows: map[string]map[string]interface{}{}, engineType: "infinity"}
	store := NewWikiMapVersionStoreWithVectorSizeResolver(engine, func(context.Context) (int, error) {
		return 768, nil
	})
	// The table is the dataset's chunk table, so it is created with the
	// dataset language, which fixes Infinity's fulltext analyzer.
	store.(*wikiMapVersionStore).resolveLanguage = func(_ context.Context, datasetID string) (string, error) {
		if datasetID != "kb-1" {
			t.Fatalf("language resolved for dataset %q", datasetID)
		}
		return "slovak", nil
	}
	version := kccommon.WikiMapVersion{
		Key: "version-a", TenantID: "tenant-1", DatasetID: "kb-1",
		DocumentID: "doc-1", ChunkID: "chunk-1", Payload: []byte(`{}`),
	}

	if err := store.PutWikiMapVersions(t.Context(), []kccommon.WikiMapVersion{version}); err != nil {
		t.Fatalf("PutWikiMapVersions() error = %v", err)
	}
	if engine.createdStores != 1 || engine.createdVectorSize != 768 || engine.createdLanguage != "slovak" {
		t.Fatalf("created store: count=%d vector_size=%d language=%q", engine.createdStores, engine.createdVectorSize, engine.createdLanguage)
	}
}

func TestWikiMapActiveStateRequiresResolverOnlyWhenInfinityStoreMissing(t *testing.T) {
	engine := &wikiMapStoreEngine{rows: map[string]map[string]interface{}{}, engineType: "infinity"}
	store := NewWikiMapVersionStoreWithVectorSizeResolver(engine, nil).(kccommon.WikiMapActiveStateStore)
	state := kccommon.WikiMapActiveState{
		Key: "active-a", TenantID: "tenant-1", DatasetID: "kb-1",
		DocumentID: "doc-1", Payload: []byte(`{}`),
	}

	err := store.PutWikiMapActiveState(t.Context(), state)
	if err == nil || !strings.Contains(err.Error(), "vector-size resolver is not configured") {
		t.Fatalf("PutWikiMapActiveState() error = %v", err)
	}
}
