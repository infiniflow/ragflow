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

package entity

// Dataset version kinds. The kind records what produced a version so the list
// can label it and a restore can explain what it undoes. Each kind names one
// milestone of the timeline and must not be reused.
//
// Every value has to fit the kind columns, which are varchar(16): the value is
// stored in dataset_version.kind and, for the document kinds, in
// document_version.kind. That is why the metadata kind is spelled
// "document_meta" rather than "document_metadata" (17 bytes), and why the
// longest value today, "document_disable", is 16 bytes and fills the column
// exactly.
//
// The constants live outside the `ee`-tagged file because shared code passes a
// kind to the edition-specific recorder.
const (
	// DatasetVersionKindBaseline is version 0: the state a dataset had when the
	// feature first observed it, seeded lazily. It is what a restore to "the
	// initial state" resolves to.
	DatasetVersionKindBaseline = "baseline"
	// DatasetVersionKindDatasetConfig is a change to the dataset's own
	// configuration. It is spelled dataset_config to read as the counterpart of
	// document_config. It names a kind, not the snapshots: those live in
	// dataset_config_snapshot, and a state-recording version points at one.
	DatasetVersionKindDatasetConfig = "dataset_config"
	// DatasetVersionKindRestore is the state a restore left behind; it records
	// the version it was restored from.
	DatasetVersionKindRestore = "restore"
	// DatasetVersionKindDocumentConfig is a change to the parse settings of
	// existing documents. It leaves the dataset configuration alone but moves
	// what the next parse of those files runs under, so it is a version of its
	// own.
	DatasetVersionKindDocumentConfig = "document_config"
	// DatasetVersionKindDocumentRename is a rename of a document. The name is
	// metadata, so the version records it in its change set and leaves the
	// document set alone.
	DatasetVersionKindDocumentRename = "document_rename"
	// DatasetVersionKindDocumentMetadata is an edit of a document's metadata
	// values, which live in the metadata store rather than on the document row.
	// The value is "document_meta" rather than "document_metadata" because it
	// has to fit the varchar(16) kind columns.
	DatasetVersionKindDocumentMetadata = "document_meta"
	// DatasetVersionKindDocumentStatus is a visibility switch whose direction
	// no kind of its own names: a row written before the two below existed, and
	// a switch to a status that is neither enabled nor disabled. It is also the
	// kind the document_version state rows carry, which is what a restore reads
	// them by - that value must not change with the direction the timeline
	// records.
	//
	// "document_status" is 15 bytes, which still fits the varchar(16) kind
	// columns.
	DatasetVersionKindDocumentStatus = "document_status"
	// The two directions a visibility switch can take. A batch names one status
	// for every document it moves (see BatchUpdateDocumentStatus), so a single
	// kind describes the whole version, and the timeline reads "Document
	// enabled" or "Document disabled" instead of one label for both.
	//
	// "document_disable" is 16 bytes: it is the longest kind value and fills the
	// varchar(16) column exactly. A longer spelling would need a migration.
	DatasetVersionKindDocumentEnable  = "document_enable"
	DatasetVersionKindDocumentDisable = "document_disable"
	// DatasetVersionKindArtifactEdit is an edit of a compiled wiki page: a
	// wiki_page entry in the document engine with no document row, keeping its
	// own history in the file commit chain. It needs a version of its own so a
	// hand edit can be restored like a document.
	DatasetVersionKindArtifactEdit = "artifact_edit"
	// The content milestones: an upload, a deletion, a parse and a chunk edit.
	DatasetVersionKindUpload = "upload"
	DatasetVersionKindDelete = "delete"
	DatasetVersionKindParse  = "parse"
	// DatasetVersionKindChunk is a chunk version whose action the switch below
	// does not name: a batch that mixed actions, or a row written before the chunk
	// kinds that follow existed. It is the label an old timeline keeps reading, so
	// the value cannot be reused for a narrower one.
	DatasetVersionKindChunk = "chunk"
	// The chunk milestones. A chunk version names one action - the writers batch
	// one action per version - so the kind says which of them it was, and the
	// timeline reads "Chunk edited" / "Chunk removed" / "Chunk added" / "Chunk
	// restored" instead of one label for all of them. A restore is its own kind
	// and not the document-level DatasetVersionKindRestore: it points back at one
	// chunk's history, which is what a replay of that version rebuilds.
	//
	// A batch that did mix actions stays DatasetVersionKindChunk: a kind cannot
	// name two things, and guessing one of them would mislabel the version.
	DatasetVersionKindChunkAdd     = "chunk_add"
	DatasetVersionKindChunkUpdate  = "chunk_update"
	DatasetVersionKindChunkDelete  = "chunk_delete"
	DatasetVersionKindChunkRestore = "chunk_restore"
	// A chunk's visibility switch, which the chunk history records as a version
	// of its own (see SwitchChunks). The change set keeps the edit action - the
	// switch rewrites the row like an edit does - so the direction lives in the
	// kind alone, and the timeline reads "Chunk enabled" or "Chunk disabled"
	// instead of "Chunk edited".
	//
	// A batch names one target state for every chunk it moves, so one kind
	// describes the whole version.
	DatasetVersionKindChunkEnable  = "chunk_enable"
	DatasetVersionKindChunkDisable = "chunk_disable"
)

// DatasetVersionKindRecordsState reports whether a version of this kind records
// a state of the dataset - its configuration - rather than naming a content
// change that ran under one. Content changes name the snapshot that holds the
// configuration instead of copying it.
func DatasetVersionKindRecordsState(kind string) bool {
	switch kind {
	case DatasetVersionKindBaseline, DatasetVersionKindDatasetConfig,
		DatasetVersionKindRestore:
		return true
	}
	return false
}

// DatasetVersionKindEstablishesDocuments reports whether a version of this kind
// has to record the document set itself rather than point at the version that
// holds it. Only a kind that can move the set has to; a parse, a chunk edit and
// a configuration change inherit it through DatasetVersion.DocumentsHash.
func DatasetVersionKindEstablishesDocuments(kind string) bool {
	switch kind {
	case DatasetVersionKindBaseline, DatasetVersionKindUpload,
		DatasetVersionKindDelete, DatasetVersionKindRestore:
		return true
	}
	return false
}
