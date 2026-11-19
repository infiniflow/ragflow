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
	"sort"
	"testing"

	"ragflow/internal/entity"
)

// snapshotKeys returns the sorted keys of a snapshot so a test can compare the
// whole field set instead of probing for individual fields.
func snapshotKeys(snapshot entity.JSONMap) []string {
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func assertSnapshotKeys(t *testing.T, snapshot entity.JSONMap, want ...string) {
	t.Helper()
	sort.Strings(want)
	got := snapshotKeys(snapshot)
	if len(got) != len(want) {
		t.Fatalf("snapshot keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("snapshot keys = %v, want %v", got, want)
		}
	}
}

// fullSnapshotRow is the engine row an edit version is stored from: the whole
// row an indexed chunk carries, derived fields included.
func fullSnapshotRow() map[string]interface{} {
	return map[string]interface{}{
		"id":                  "ck-1",
		"doc_id":              "doc-1",
		"kb_id":               "kb-1",
		"docnm_kwd":           "manual.pdf",
		"content_with_weight": "the body",
		"content_ltks":        "the body",
		"content_sm_ltks":     "the body",
		"important_kwd":       []string{"kw"},
		"tag_kwd":             []string{"tag"},
		"tag_feas":            map[string]float64{"tag": 1},
		"available_int":       1,
		"chunk_order_int":     3,
		"img_id":              "",
		"q_1024_vec":          []float32{0.5, -0.25},
		"create_time":         "2026-01-01 00:00:00",
	}
}

// TestFullSnapshotKeepsTheWholeRow pins that a full snapshot is the engine row
// minus identity, and that it says which model produced its derived fields: that
// metadata is what lets a restore decide whether the stored vector is usable.
func TestFullSnapshotKeepsTheWholeRow(t *testing.T) {
	snapshot := FullSnapshot(fullSnapshotRow(), SnapshotMeta{EmbeddingID: "embd-1", Language: "en"})
	assertSnapshotKeys(t, snapshot,
		"available_int", "chunk_order_int", "content_ltks", "content_sm_ltks",
		"content_with_weight", "img_id", "important_kwd", "q_1024_vec",
		"tag_feas", "tag_kwd", SnapshotMetaKey)

	meta := SnapshotMetaOf(snapshot)
	if meta.SchemaRevision != FullSnapshotSchemaRevision {
		t.Fatalf("schema revision = %d, want %d", meta.SchemaRevision, FullSnapshotSchemaRevision)
	}
	if meta.EmbeddingID != "embd-1" || meta.Language != "en" || meta.VectorField != "q_1024_vec" {
		t.Fatalf("meta = %#v, want the producer and the vector column", meta)
	}
}

// TestFullSnapshotRowRejectsStateItCannotCopy: incomplete snapshots are
// rebuilt, not copied.
func TestFullSnapshotRowRejectsStateItCannotCopy(t *testing.T) {
	if _, _, ok := FullSnapshotRow(entity.JSONMap{"content_with_weight": "the body"}); ok {
		t.Fatalf("FullSnapshotRow(no metadata) = ok, want a rebuild")
	}

	noVector := FullSnapshot(fullSnapshotRow(), SnapshotMeta{EmbeddingID: "embd-1", Language: "en"})
	delete(noVector, "q_1024_vec")
	if _, _, ok := FullSnapshotRow(noVector); ok {
		t.Fatalf("FullSnapshotRow(without its vector) = ok, want a rebuild")
	}

	noTokens := FullSnapshot(fullSnapshotRow(), SnapshotMeta{EmbeddingID: "embd-1", Language: "en"})
	delete(noTokens, "content_ltks")
	if _, _, ok := FullSnapshotRow(noTokens); ok {
		t.Fatalf("FullSnapshotRow(without its tokens) = ok, want a rebuild")
	}

	row, meta, ok := FullSnapshotRow(FullSnapshot(fullSnapshotRow(), SnapshotMeta{EmbeddingID: "embd-1", Language: "en"}))
	if !ok {
		t.Fatalf("FullSnapshotRow(full) = not ok, want the stored row")
	}
	if meta.VectorField != "q_1024_vec" {
		t.Fatalf("meta.VectorField = %q, want the stored column", meta.VectorField)
	}
	if _, leaked := row[SnapshotMetaKey]; leaked {
		t.Fatalf("row = %#v, want the producer metadata stripped", row)
	}
}

// TestFullSnapshotRowAcceptsAStoredVectorAfterJSONRoundTrip pins that the
// admission test and the normalization agree: a snapshot read back from the
// history store carries []interface{} of float64, which is what the engines have
// to be handed as a typed slice.
func TestFullSnapshotRowAcceptsAStoredVectorAfterJSONRoundTrip(t *testing.T) {
	encoded, err := json.Marshal(FullSnapshot(fullSnapshotRow(), SnapshotMeta{EmbeddingID: "embd-1", Language: "en"}))
	if err != nil {
		t.Fatalf("marshal the snapshot: %v", err)
	}
	stored := entity.JSONMap{}
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatalf("unmarshal the snapshot: %v", err)
	}

	row, meta, ok := FullSnapshotRow(stored)
	if !ok {
		t.Fatalf("FullSnapshotRow(round trip) = not ok, want the stored row")
	}
	vector, ok := SnapshotVector(row[meta.VectorField])
	if !ok || len(vector) != 2 || vector[0] != 0.5 || vector[1] != -0.25 {
		t.Fatalf("vector = %v (ok %v), want the stored values as float64", vector, ok)
	}
}

// TestSnapshotVectorAcceptsOnlyNumbers pins that a malformed vector is refused
// rather than written back as garbage: a caller falls back to rebuilding then.
func TestSnapshotVectorAcceptsOnlyNumbers(t *testing.T) {
	if _, ok := SnapshotVector([]interface{}{"0.5"}); ok {
		t.Fatalf("SnapshotVector(%v) = ok, want a refusal", []interface{}{"0.5"})
	}
	if _, ok := SnapshotVector(nil); ok {
		t.Fatalf("SnapshotVector(nil) = ok, want a refusal")
	}
	if got, ok := SnapshotVector([]float32{0.5, -0.25}); !ok || len(got) != 2 || got[0] != 0.5 || got[1] != -0.25 {
		t.Fatalf("SnapshotVector([]float32) = %v (ok %v), want the values as float64", got, ok)
	}
}

// TestClientSnapshotDropsWhatTheRowWasIndexedBy pins the response shape: the
// version panel shows what the version said, so the token columns, the vector and
// the producer metadata never reach a client.
func TestClientSnapshotDropsWhatTheRowWasIndexedBy(t *testing.T) {
	snapshot := FullSnapshot(fullSnapshotRow(), SnapshotMeta{EmbeddingID: "embd-1", Language: "en"})
	client := ClientSnapshot(snapshot)
	assertSnapshotKeys(t, client,
		"available_int", "chunk_order_int", "content_with_weight", "img_id",
		"important_kwd", "tag_feas", "tag_kwd")
}
