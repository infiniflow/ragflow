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

func TestEnrichChunksWithDocMetadata_EmptyDocID(t *testing.T) {
	svc := NewMetadataService()
	chunks := []map[string]interface{}{
		{"doc_id": "", "kb_id": "kb1"},
	}
	svc.EnrichChunksWithDocMetadata(chunks, "tenant-1", nil)
	// Should not panic, should skip empty doc_id
}

func TestEnrichChunksWithDocMetadata_DuplicateDocIDs(t *testing.T) {
	svc := NewMetadataService()
	chunks := []map[string]interface{}{
		{"doc_id": "doc1", "kb_id": "kb1"},
		{"doc_id": "doc1", "kb_id": "kb1"},
	}
	svc.EnrichChunksWithDocMetadata(chunks, "tenant-1", nil)
	// Should not panic, deduplication should work
}

func TestEnrichChunksWithDocMetadata_MultipleKBs(t *testing.T) {
	svc := NewMetadataService()
	chunks := []map[string]interface{}{
		{"doc_id": "doc1", "kb_id": "kb1"},
		{"doc_id": "doc2", "kb_id": "kb2"},
	}
	svc.EnrichChunksWithDocMetadata(chunks, "tenant-1", nil)
	// Should not panic, should handle multiple KBs
}

func TestEnrichChunksWithDocMetadata_WithMetadataFields(t *testing.T) {
	svc := NewMetadataService()
	chunks := []map[string]interface{}{
		{"doc_id": "doc1", "kb_id": "kb1"},
		{"doc_id": "doc2", "kb_id": "kb1"},
	}
	svc.EnrichChunksWithDocMetadata(chunks, "tenant-1", []string{"author", "date"})
	// Should not panic
}

func TestEnrichChunksWithDocMetadata_MixedFields(t *testing.T) {
	svc := NewMetadataService()
	chunks := []map[string]interface{}{
		{"doc_id": "doc1", "kb_id": "kb1"},
		{"doc_id": "", "kb_id": "kb1"},  // incomplete, should be skipped
		{"kb_id": "kb1"},                // no doc_id, should be skipped
	}
	svc.EnrichChunksWithDocMetadata(chunks, "tenant-1", nil)
	// Should not panic
}
