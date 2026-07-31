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

// --- extractDocID ---

func TestExtractDocID_FromID(t *testing.T) {
	chunk := map[string]interface{}{"id": "doc1", "doc_id": "doc2"}
	if got := extractDocID(chunk); got != "doc1" {
		t.Errorf("expected doc1, got %q", got)
	}
}

func TestExtractDocID_FromDocID(t *testing.T) {
	chunk := map[string]interface{}{"doc_id": "doc2"}
	if got := extractDocID(chunk); got != "doc2" {
		t.Errorf("expected doc2, got %q", got)
	}
}

func TestExtractDocID_Empty(t *testing.T) {
	chunk := map[string]interface{}{"title": "no id"}
	if got := extractDocID(chunk); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- ConvertSearchResultToDocMeta ---

func TestConvertSearchResultToDocMeta_Empty(t *testing.T) {
	result := ConvertSearchResultToDocMeta(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d", len(result))
	}
}

func TestConvertSearchResultToDocMeta_Single(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "doc1", "meta_fields": map[string]interface{}{"author": "Zhang San"}},
	}
	result := ConvertSearchResultToDocMeta(chunks)
	if len(result) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(result))
	}
	if result["doc1"]["author"] != "Zhang San" {
		t.Errorf("expected 'Zhang San', got %v", result["doc1"]["author"])
	}
}

func TestConvertSearchResultToDocMeta_Multiple(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "doc1", "meta_fields": map[string]interface{}{"author": "Zhang San"}},
		{"id": "doc2", "meta_fields": map[string]interface{}{"author": "Li Si"}},
	}
	result := ConvertSearchResultToDocMeta(chunks)
	if len(result) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(result))
	}
}

func TestConvertSearchResultToDocMeta_SkipEmptyDocID(t *testing.T) {
	chunks := []map[string]interface{}{
		{"meta_fields": map[string]interface{}{"author": "Zhang San"}},
	}
	result := ConvertSearchResultToDocMeta(chunks)
	if len(result) != 0 {
		t.Errorf("expected empty for missing doc_id, got %d", len(result))
	}
}

func TestConvertSearchResultToDocMeta_SkipEmptyMeta(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "doc1"},
	}
	result := ConvertSearchResultToDocMeta(chunks)
	if len(result) != 0 {
		t.Errorf("expected empty for missing meta_fields, got %d", len(result))
	}
}

func TestConvertSearchResultToDocMeta_LastWins(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "doc1", "meta_fields": map[string]interface{}{"author": "Zhang San"}},
		{"id": "doc1", "meta_fields": map[string]interface{}{"author": "Li Si"}},
	}
	result := ConvertSearchResultToDocMeta(chunks)
	if result["doc1"]["author"] != "Li Si" {
		t.Errorf("expected last value 'Li Si', got %v", result["doc1"]["author"])
	}
}

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

func TestCollectDocIDsByKB_UsesIDField(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "doc1", "kb_id": "kb1"},
	}
	result := CollectDocIDsByKB(chunks)
	if len(result["kb1"]) != 1 || result["kb1"][0] != "doc1" {
		t.Errorf("expected doc1 from id field, got %v", result["kb1"])
	}
}

func TestCollectDocIDsByKB_PrefersIDOverDocID(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "doc-from-id", "doc_id": "doc-from-doc-id", "kb_id": "kb1"},
	}
	result := CollectDocIDsByKB(chunks)
	if result["kb1"][0] != "doc-from-id" {
		t.Errorf("expected doc-from-id (id takes precedence), got %v", result["kb1"])
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

func TestAttachDocMetaToChunks_UsesIDField(t *testing.T) {
	chunks := []map[string]interface{}{{"id": "doc1"}}
	metaByDoc := DocMetaMap{"doc1": {"author": "Zhang San"}}
	AttachDocMetaToChunks(chunks, metaByDoc, nil)
	meta, ok := chunks[0]["document_metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("expected document_metadata when chunk uses id field")
	}
	if meta["author"] != "Zhang San" {
		t.Errorf("expected 'Zhang San', got %v", meta["author"])
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
