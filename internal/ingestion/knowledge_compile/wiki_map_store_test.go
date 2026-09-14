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
	"testing"

	"ragflow/internal/engine/types"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

type wikiMapStoreEngine struct {
	fakeEngine
	rows          map[string]map[string]interface{}
	inserted      []map[string]interface{}
	insertBase    string
	insertDataset string
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
	if expected, ok := filter["available_int"].(int); ok {
		if value, ok := row["available_int"].(int); !ok || value != expected {
			return false
		}
	}
	if mustNot, ok := filter["must_not"].(map[string]interface{}); ok {
		if exists, ok := mustNot["exists"].(string); ok {
			if _, present := row[exists]; present {
				return false
			}
		}
	}
	for _, field := range []string{"compile_kwd", "type_kwd"} {
		if expected, ok := filter[field].(string); ok && mapStoreString(row[field]) != expected {
			return false
		}
	}
	return true
}

func (e *wikiMapStoreEngine) InsertChunks(_ context.Context, chunks []map[string]interface{}, baseName, datasetID string) ([]string, error) {
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
	store := NewWikiMapVersionStore(engine)
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
		"compile_kwd":    wikiMapExtractCompileKWD,
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
