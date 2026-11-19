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

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

// DatasetVersionChange is one entry of a version's change set: a document or a
// chunk that the change touched, and what happened to it.
//
// The version row alone says "the content changed"; the change set is the
// readable half the versions page expands. It travels inside the version row
// (dataset_version.changes) because it is written, deleted and read with the
// version, and nothing queries one entry across versions.
//
// The set is bounded: a parse touches thousands of chunks, so the writer caps
// what it records and the counters still tell the whole story.
//
// The type is not behind the `ee` build tag because the shared content paths
// build a change set to hand to the edition-specific recorder.
type DatasetVersionChange struct {
	// DocID is the document the entry belongs to, and for a document entry also
	// the entry's own identity.
	DocID string `json:"doc_id,omitempty"`
	// Entity is document or chunk: what kind of thing the entry names.
	Entity string `json:"entity"`
	// Action is what happened to it: add, update, delete, parse or restore.
	Action string `json:"action"`
	// ChunkID is the chunk a chunk entry names; a document entry leaves it
	// empty because its DocID already names the document.
	ChunkID string `json:"chunk_id,omitempty"`
	// CommitID is the page commit an artifact entry points at: the diff lives in
	// that page's commit chain (file_commit_item.diff), not here. Empty on older
	// rows and when page history is not configured.
	CommitID string `json:"commit_id,omitempty"`
	// Name is what the UI shows: the document's name for a document entry,
	// because a removed document has no row left to read one from. A chunk entry
	// leaves it empty - the text is in the chunk history.
	Name string `json:"name,omitempty"`
	// PreviousName is what the document was called before a rename entry moved
	// it. It exists for the version that reads the timeline backwards: a version
	// older than the rename has no rename of its own to answer with, and the
	// document row only carries the name it has now, so the entry that renamed it
	// later is the only place the earlier name survives. Empty on entries that are
	// not renames and on rows written before the field existed.
	PreviousName string `json:"previous_name,omitempty"`
	// MetaChanges is what an edit of the document's metadata values moved, key by
	// key. Only a metadata version carries it, since the metadata store has no
	// diff of its own. It is bounded twice (see normalizeChanges).
	MetaChanges []MetadataFieldChange `json:"meta_changes,omitempty"`
}

// MetadataFieldChange is one metadata key a version moved: the value before the
// edit and the value after it. An absent side tells the two ends of an edit
// apart: introduced keys have no Before, removed ones no After.
type MetadataFieldChange struct {
	Key string `json:"key"`
	// Before is the value the key held, raw so a list stays a list.
	Before json.RawMessage `json:"before,omitempty"`
	// After is the value the key holds now, in the same shape.
	After json.RawMessage `json:"after,omitempty"`
}

// Identity is the entry's own id: the chunk a chunk entry names, the document a
// document entry names. It is what the set dedupes on and what the page keys on.
func (c DatasetVersionChange) Identity() string {
	if c.Entity == DatasetVersionChangeEntityChunk {
		return c.ChunkID
	}
	if c.DocID != "" {
		return c.DocID
	}
	return c.ChunkID
}

// DatasetVersionChanges is a version's change set as one JSON document, in the
// order the writer stored it.
type DatasetVersionChanges []DatasetVersionChange

// Value implements driver.Valuer: the slice travels as one JSON array, and an
// empty set stays NULL so a version written without changes costs no bytes.
func (c DatasetVersionChanges) Value() (driver.Value, error) {
	if len(c) == 0 {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements sql.Scanner.
func (c *DatasetVersionChanges) Scan(value interface{}) error {
	if value == nil {
		*c = nil
		return nil
	}
	var raw string
	switch v := value.(type) {
	case []byte:
		raw = string(v)
	case string:
		raw = v
	default:
		raw = fmt.Sprint(value)
	}
	if strings.TrimSpace(raw) == "" {
		*c = nil
		return nil
	}
	// Decode into fresh slices: unmarshalling into a reused non-nil slice keeps
	// stale entries from a previous Scan.
	var fresh DatasetVersionChanges
	if err := json.Unmarshal([]byte(raw), &fresh); err != nil {
		return fmt.Errorf("decode a dataset version change set: %w", err)
	}
	*c = fresh
	return nil
}

// Dataset version change entities.
const (
	DatasetVersionChangeEntityDocument = "document"
	DatasetVersionChangeEntityChunk    = "chunk"
	// DatasetVersionChangeEntityArtifact is a compiled wiki page: a wiki_page
	// entry in the document engine with no document row behind it, named by its
	// page file id (<dataset>/<page_type>/<slug>).
	DatasetVersionChangeEntityArtifact = "artifact"
)

// Dataset version change actions. A parse is listed apart from an update because
// it rewrites a whole document's chunk set rather than one chunk.
const (
	DatasetVersionChangeActionAdd    = "add"
	DatasetVersionChangeActionUpdate = "update"
	// DatasetVersionChangeActionRename is a document rename: the entry's Name is
	// the new name.
	DatasetVersionChangeActionRename  = "rename"
	DatasetVersionChangeActionDelete  = "delete"
	DatasetVersionChangeActionParse   = "parse"
	DatasetVersionChangeActionRestore = "restore"
	// The two directions of a visibility switch: a document or a chunk turned on
	// or off. They are actions of their own rather than an update, because the
	// line the timeline prints names what happened - "Enabled document a.pdf" -
	// and the kind alone is not what that line reads.
	DatasetVersionChangeActionEnable  = "enable"
	DatasetVersionChangeActionDisable = "disable"
)
