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

package chunkversion

import (
	"encoding/json"
	"regexp"
	"strings"

	"ragflow/internal/entity"
)

// SnapshotMetaKey is the key a full-row snapshot keeps its producer metadata
// under. It is not a chunk field, so every reader of a full snapshot has to
// strip it before writing the row back.
const SnapshotMetaKey = "_meta"

// FullSnapshotSchemaRevision marks a whole-row snapshot. A row without it is
// rebuilt rather than copied.
const FullSnapshotSchemaRevision = 2

// SnapshotMeta describes how the derived fields of a full snapshot were
// produced. A restore compares it against the live row before handing the stored
// vector and tokens back: overwriting a row with a vector from another embedding
// model leaves a chunk nothing can find, and nothing about the row says so.
type SnapshotMeta struct {
	SchemaRevision int `json:"schema_revision"`
	// EmbeddingID is the embedding model the vector was computed with. A reader
	// that wants to copy the row requires it to match the dataset's own model: an
	// empty id means nobody recorded the producer, and the derived fields are then
	// rebuilt rather than trusted.
	EmbeddingID string `json:"embd_id,omitempty"`
	// Language is the dataset language the tokens were produced with.
	Language string `json:"language,omitempty"`
	// VectorField is the column the vector lives in ("q_1024_vec"). Its name
	// carries the dimension, which is what makes a vector from another model
	// detectable without a model lookup.
	VectorField string `json:"vector_field,omitempty"`
}

// fullSnapshotExcluded are the fields a snapshot never carries: they describe
// which row this is and where it lives, not what it says. A restore must keep
// the live values, and a replay recomputes them from the document it rebuilds.
var fullSnapshotExcluded = map[string]struct{}{
	"id":          {},
	"doc_id":      {},
	"kb_id":       {},
	"docnm_kwd":   {},
	"create_time": {},
	"update_time": {},
	"_score":      {},
	"snapshot":    {},
	"version":     {},
	"dataset_id":  {},
	// The legacy alias of the body is never written: a restored row carrying both
	// would let the panel submit the stale copy back and silently undo the edit.
	// The body itself is kept under content_with_weight.
	"content": {},
}

// vectorFieldPattern matches the dense-vector columns the engine rows carry
// ("q_1024_vec"). The dimension is part of the name, so the pattern is also how
// a snapshot's vector is located without knowing the dataset's model.
var vectorFieldPattern = regexp.MustCompile(`^q_[0-9]+_vec$`)

// VectorFieldOf returns the dense-vector column of an engine row, or "" when the
// row carries none.
//
// A row can carry more than one after an embedding-model change - the engine
// keeps the old column - so the choice has to be deterministic: the widest name
// in lexical order is picked, which is stable across reads of the same row.
func VectorFieldOf(row map[string]interface{}) string {
	field := ""
	for key := range row {
		if !vectorFieldPattern.MatchString(key) {
			continue
		}
		if field == "" || key > field {
			field = key
		}
	}
	return field
}

// FullSnapshot copies the whole engine row, minus the identity fields, and
// records how its derived fields were produced.
//
// Every writer stores this shape - the parse baseline, an added chunk, an edit
// and a restore - so restoring any version is a copy: no tokenizer run, no
// embedding round-trip, and the chunk ranks exactly as the version did. The price
// is that a document's vectors are held twice, live and in its history, which is
// what the history is for.
//
// A row without the derived fields or metadata is refused by FullSnapshotRow.
func FullSnapshot(row map[string]interface{}, meta SnapshotMeta) entity.JSONMap {
	snapshot := entity.JSONMap{}
	for key, value := range row {
		if _, excluded := fullSnapshotExcluded[key]; excluded {
			continue
		}
		if value == nil {
			continue
		}
		snapshot[key] = value
	}
	meta.SchemaRevision = FullSnapshotSchemaRevision
	if meta.VectorField == "" {
		meta.VectorField = VectorFieldOf(row)
	}
	snapshot[SnapshotMetaKey] = map[string]interface{}{
		"schema_revision": meta.SchemaRevision,
		"embd_id":         meta.EmbeddingID,
		"language":        meta.Language,
		"vector_field":    meta.VectorField,
	}
	return snapshot
}

// SnapshotMetaOf returns zero metadata (revision 0) when none was recorded.
func SnapshotMetaOf(snapshot entity.JSONMap) SnapshotMeta {
	var meta SnapshotMeta
	raw, ok := snapshot[SnapshotMetaKey]
	if !ok {
		return meta
	}
	fields, ok := raw.(map[string]interface{})
	if !ok {
		// The snapshot survives a JSON round-trip through the history store, so
		// a map written in Go comes back as map[string]interface{}; a document
		// engine that hands back entity.JSONMap is handled here as well.
		if typed, ok := raw.(entity.JSONMap); ok {
			fields = typed
		} else {
			return meta
		}
	}
	if revision, ok := numberToInt(fields["schema_revision"]); ok {
		meta.SchemaRevision = revision
	}
	meta.EmbeddingID, _ = fields["embd_id"].(string)
	meta.Language, _ = fields["language"].(string)
	meta.VectorField, _ = fields["vector_field"].(string)
	return meta
}

// SnapshotRow returns the row fields without the producer metadata.
func SnapshotRow(snapshot entity.JSONMap) entity.JSONMap {
	row := make(entity.JSONMap, len(snapshot))
	for key, value := range snapshot {
		if key == SnapshotMetaKey {
			continue
		}
		row[key] = value
	}
	return row
}

// FullSnapshotRow returns the recorded row and its metadata, or false for an
// incomplete snapshot that must be rebuilt. It is the single admission test
// for every reader that wants to copy rather than rebuild.
func FullSnapshotRow(snapshot entity.JSONMap) (entity.JSONMap, SnapshotMeta, bool) {
	meta := SnapshotMetaOf(snapshot)
	if meta.SchemaRevision < FullSnapshotSchemaRevision || meta.VectorField == "" {
		return nil, meta, false
	}
	row := SnapshotRow(snapshot)
	if vector, ok := SnapshotVector(row[meta.VectorField]); !ok || len(vector) == 0 {
		return nil, meta, false
	}
	if content, _ := row["content_with_weight"].(string); strings.TrimSpace(content) != "" {
		if _, ok := row["content_ltks"]; !ok {
			return nil, meta, false
		}
	}
	return row, meta, true
}

// SnapshotVector normalizes a vector read back from a snapshot into the shape
// the engines write. A snapshot survives a JSON round trip through the history
// store, so its vector comes back as []interface{} of float64 rather than the
// []float32 a live engine row carries, and an engine handed the generic slice
// would refuse the write.
func SnapshotVector(value interface{}) ([]float64, bool) {
	switch typed := value.(type) {
	case []float64:
		return typed, true
	case []float32:
		out := make([]float64, len(typed))
		for i, item := range typed {
			out[i] = float64(item)
		}
		return out, true
	case []interface{}:
		out := make([]float64, 0, len(typed))
		for _, item := range typed {
			switch number := item.(type) {
			case float64:
				out = append(out, number)
			case float32:
				out = append(out, float64(number))
			case int:
				out = append(out, float64(number))
			case int64:
				out = append(out, float64(number))
			case json.Number:
				parsed, err := number.Float64()
				if err != nil {
					return nil, false
				}
				out = append(out, parsed)
			default:
				return nil, false
			}
		}
		return out, true
	}
	return nil, false
}

// snapshotDerivedFields are the fields a version panel never shows: the token
// columns and the score are how the row was indexed and ranked, not what it
// says, and they are exactly the payload a full snapshot duplicates.
var snapshotDerivedFields = map[string]struct{}{
	"content_ltks":    {},
	"content_sm_ltks": {},
	"important_tks":   {},
	"question_tks":    {},
	"title_tks":       {},
	"title_sm_tks":    {},
	"authors_tks":     {},
	"authors_sm_tks":  {},
	"_score":          {},
}

// ClientSnapshot returns the fields a version panel shows, without producer
// metadata or derived fields.
func ClientSnapshot(snapshot entity.JSONMap) entity.JSONMap {
	row := make(entity.JSONMap, len(snapshot))
	for key, value := range snapshot {
		if key == SnapshotMetaKey {
			continue
		}
		if _, derived := snapshotDerivedFields[key]; derived {
			continue
		}
		if vectorFieldPattern.MatchString(key) {
			continue
		}
		row[key] = value
	}
	return row
}

// numberToInt accepts the shapes a number takes on its way back from a snapshot:
// an int written in Go, a float64 after a JSON round trip, and a json.Number from
// a decoder configured to keep the text.
func numberToInt(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	}
	return 0, false
}
