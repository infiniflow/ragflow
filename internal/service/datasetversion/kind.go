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

import "ragflow/internal/entity"

// contentKinds are the version kinds a dataset's content produces: documents
// and chunks. They record that the content changed, not the configuration, so a
// restore cannot replay them — the configuration snapshot they carry is the one
// the change was made under, not something to write back.
var contentKinds = map[string]struct{}{
	entity.DatasetVersionKindUpload:           {},
	entity.DatasetVersionKindDelete:           {},
	entity.DatasetVersionKindParse:            {},
	entity.DatasetVersionKindChunk:            {},
	entity.DatasetVersionKindChunkAdd:         {},
	entity.DatasetVersionKindChunkUpdate:      {},
	entity.DatasetVersionKindChunkDelete:      {},
	entity.DatasetVersionKindChunkRestore:     {},
	entity.DatasetVersionKindChunkEnable:      {},
	entity.DatasetVersionKindChunkDisable:     {},
	entity.DatasetVersionKindDocumentRename:   {},
	entity.DatasetVersionKindDocumentMetadata: {},
	entity.DatasetVersionKindDocumentStatus:   {},
	entity.DatasetVersionKindDocumentEnable:   {},
	entity.DatasetVersionKindDocumentDisable:  {},
	entity.DatasetVersionKindArtifactEdit:     {},
}

// IsContentKind reports whether kind describes a content change - documents or
// chunks. It classifies a version for its label and its change set; whether a
// restore moves content is a different question, answered by RestoresContent.
func IsContentKind(kind string) bool {
	_, ok := contentKinds[kind]
	return ok
}

// recordedVersionKinds is every kind a version row can carry. The roster is what
// lets an unknown kind be refused instead of being treated as a restorable state:
// a row whose kind is not one this build writes describes a state it cannot read.
var recordedVersionKinds = map[string]struct{}{
	entity.DatasetVersionKindBaseline:         {},
	entity.DatasetVersionKindUpload:           {},
	entity.DatasetVersionKindDelete:           {},
	entity.DatasetVersionKindParse:            {},
	entity.DatasetVersionKindChunk:            {},
	entity.DatasetVersionKindChunkAdd:         {},
	entity.DatasetVersionKindChunkUpdate:      {},
	entity.DatasetVersionKindChunkDelete:      {},
	entity.DatasetVersionKindChunkRestore:     {},
	entity.DatasetVersionKindChunkEnable:      {},
	entity.DatasetVersionKindChunkDisable:     {},
	entity.DatasetVersionKindDocumentRename:   {},
	entity.DatasetVersionKindDocumentMetadata: {},
	entity.DatasetVersionKindDocumentStatus:   {},
	entity.DatasetVersionKindDocumentEnable:   {},
	entity.DatasetVersionKindDocumentDisable:  {},
	entity.DatasetVersionKindArtifactEdit:     {},
	entity.DatasetVersionKindRestore:          {},
	entity.DatasetVersionKindDatasetConfig:    {},
	entity.DatasetVersionKindDocumentConfig:   {},
}

// RestoresContent reports whether restoring a version of this kind moves the
// dataset's content as well as its configuration.
//
// Every version records a state of the dataset, not only the half its writer
// moved: the configuration through ConfigHash and the document set through
// DocumentsHash. A reader who asks to return to version N means the dataset as it
// stood at N, so a restore puts both halves back for every kind. A configuration
// version inherits the document set instead of storing it - a storage detail of
// the write, not a statement that the set is not part of the state - and the set
// it inherits is the one that held when it was written.
//
// The consequence is deliberate: restoring a configuration version removes the
// documents uploaded after it, exactly as restoring a content version does. A
// dry run reports the documents it would restore and remove, so the caller
// confirms with the full picture instead of discovering it afterwards.
func RestoresContent(kind string) bool {
	_, ok := recordedVersionKinds[kind]
	return ok
}
