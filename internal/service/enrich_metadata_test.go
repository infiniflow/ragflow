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
	"testing"
)

// --- CollectDocIDsByKB ---

func TestCollectDocIDsByKB_Empty(t *testing.T) {
	result := CollectDocIDsByKB(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestCollectDocIDsByKB_Single(t *testing.T) {
	chunks := []map[string]interface{}{
		{"doc_id": "doc1", "kb_id": "kb1"},
	}
	result := CollectDocIDsByKB(chunks)
	if len(result) != 1 || len(result["kb1"]) != 1 || result["kb1"][0] != "doc1" {
		t.Errorf("unexpected: %v", result)
	}
}

func TestCollectDocIDsByKB_Dedup(t *testing.T) {
	chunks := []map[string]interface{}{
		{"doc_id": "doc1", "kb_id": "kb1"},
		{"doc_id": "doc1", "kb_id": "kb1"},
		{"doc_id": "doc1", "kb_id": "kb1"},
	}
	result := CollectDocIDsByKB(chunks)
	if len(result["kb1"]) != 1 {
		t.Errorf("expected 1 doc after dedup, got %v", result["kb1"])
	}
}

func TestCollectDocIDsByKB_MultipleKBs(t *testing.T) {
	chunks := []map[string]interface{}{
		{"doc_id": "doc1", "kb_id": "kb1"},
		{"doc_id": "doc2", "kb_id": "kb2"},
	}
	result := CollectDocIDsByKB(chunks)
	if len(result) != 2 {
		t.Errorf("expected 2 KBs, got %d", len(result))
	}
	if len(result["kb1"]) != 1 || result["kb1"][0] != "doc1" {
		t.Errorf("unexpected kb1: %v", result["kb1"])
	}
	if len(result["kb2"]) != 1 || result["kb2"][0] != "doc2" {
		t.Errorf("unexpected kb2: %v", result["kb2"])
	}
}

func TestCollectDocIDsByKB_SkipEmpty(t *testing.T) {
	chunks := []map[string]interface{}{
		{"doc_id": "", "kb_id": "kb1"},
		{"doc_id": "doc1", "kb_id": ""},
		{},
	}
	result := CollectDocIDsByKB(chunks)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

// --- AttachDocMetaToChunks ---

func TestAttachDocMetaToChunks_NoMatch(t *testing.T) {
	chunks := []map[string]interface{}{{"doc_id": "doc1"}}
	metaByDoc := DocMetaMap{"doc2": {"author": "Zhang San"}}
	AttachDocMetaToChunks(chunks, metaByDoc, nil)
	if _, ok := chunks[0]["document_metadata"]; ok {
		t.Error("should not attach metadata for no match")
	}
}

func TestAttachDocMetaToChunks_Match(t *testing.T) {
	chunks := []map[string]interface{}{{"doc_id": "doc1"}}
	metaByDoc := DocMetaMap{"doc1": {"author": "Zhang San", "date": "2024-01-01"}}
	AttachDocMetaToChunks(chunks, metaByDoc, nil)
	meta, ok := chunks[0]["document_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected document_metadata")
	}
	if meta["author"] != "Zhang San" {
		t.Errorf("expected 'Zhang San', got %v", meta["author"])
	}
	if meta["date"] != "2024-01-01" {
		t.Errorf("expected '2024-01-01', got %v", meta["date"])
	}
}

func TestAttachDocMetaToChunks_FilterFields(t *testing.T) {
	chunks := []map[string]interface{}{{"doc_id": "doc1"}}
	metaByDoc := DocMetaMap{"doc1": {"author": "Zhang San", "date": "2024-01-01", "category": "A"}}
	AttachDocMetaToChunks(chunks, metaByDoc, []string{"author", "date"})
	meta, ok := chunks[0]["document_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected document_metadata")
	}
	if len(meta) != 2 {
		t.Errorf("expected 2 fields, got %d: %v", len(meta), meta)
	}
	if _, ok := meta["category"]; ok {
		t.Error("category should be filtered out")
	}
}

func TestAttachDocMetaToChunks_EmptyMeta(t *testing.T) {
	chunks := []map[string]interface{}{{"doc_id": "doc1"}}
	AttachDocMetaToChunks(chunks, nil, nil)
	if _, ok := chunks[0]["document_metadata"]; ok {
		t.Error("should not attach when metaByDoc is empty")
	}
}

// --- EnrichChunksWithDocMetadata (integration) ---

func TestEnrichChunksWithDocMetadata_NoChunks(t *testing.T) {
	svc := NewMetadataService()
	svc.EnrichChunksWithDocMetadata(nil, "tenant-1", nil)
	// Should not panic
}

func TestEnrichChunksWithDocMetadata_EmptyChunks(t *testing.T) {
	svc := NewMetadataService()
	svc.EnrichChunksWithDocMetadata([]map[string]interface{}{}, "tenant-1", nil)
	// Should not panic
}
