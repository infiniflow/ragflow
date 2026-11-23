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

package datasetversion

import (
	"testing"

	"ragflow/internal/entity"
)

func TestIsContentKind(t *testing.T) {
	content := []string{
		entity.DatasetVersionKindUpload,
		entity.DatasetVersionKindDelete,
		entity.DatasetVersionKindParse,
		entity.DatasetVersionKindChunk,
		entity.DatasetVersionKindChunkAdd,
		entity.DatasetVersionKindChunkUpdate,
		entity.DatasetVersionKindChunkDelete,
		entity.DatasetVersionKindChunkRestore,
		entity.DatasetVersionKindChunkEnable,
		entity.DatasetVersionKindChunkDisable,
		entity.DatasetVersionKindDocumentStatus,
		entity.DatasetVersionKindDocumentEnable,
		entity.DatasetVersionKindDocumentDisable,
	}
	for _, kind := range content {
		if !IsContentKind(kind) {
			t.Fatalf("%q should be a content kind", kind)
		}
	}

	// The configuration kinds are restorable: they carry a state a restore can
	// write back, which a content version does not.
	configuration := []string{
		entity.DatasetVersionKindBaseline,
		entity.DatasetVersionKindDatasetConfig,
		entity.DatasetVersionKindRestore,
	}
	for _, kind := range configuration {
		if IsContentKind(kind) {
			t.Fatalf("%q should be a configuration kind", kind)
		}
	}

	if IsContentKind("") || IsContentKind("unknown") {
		t.Fatalf("unknown kinds must not be treated as content")
	}
}

// TestRestoresContent pins that every recorded version restores the documents
// with the configuration. A version records a state of the dataset even when its
// writer only moved one half - the other is reached through ConfigHash /
// DocumentsHash - and "return to version N" means the dataset as it stood at N, so
// a configuration version puts the document set back too.
func TestRestoresContent(t *testing.T) {
	kinds := []string{
		entity.DatasetVersionKindBaseline,
		entity.DatasetVersionKindUpload,
		entity.DatasetVersionKindDelete,
		entity.DatasetVersionKindParse,
		entity.DatasetVersionKindChunk,
		entity.DatasetVersionKindChunkAdd,
		entity.DatasetVersionKindChunkUpdate,
		entity.DatasetVersionKindChunkDelete,
		entity.DatasetVersionKindChunkRestore,
		entity.DatasetVersionKindChunkEnable,
		entity.DatasetVersionKindChunkDisable,
		entity.DatasetVersionKindDocumentStatus,
		entity.DatasetVersionKindDocumentEnable,
		entity.DatasetVersionKindDocumentDisable,
		entity.DatasetVersionKindRestore,
		entity.DatasetVersionKindDatasetConfig,
		entity.DatasetVersionKindDocumentConfig,
	}
	for _, kind := range kinds {
		if !RestoresContent(kind) {
			t.Fatalf("%q restore should restore the content", kind)
		}
	}

	if RestoresContent("") || RestoresContent("unknown") {
		t.Fatalf("unknown kinds must not be treated as restores")
	}
}
