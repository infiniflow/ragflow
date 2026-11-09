// Copyright 2025 Infiniflow, Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package schema

import (
	"encoding/json"
	"testing"
)

// R1: a typed field (id) must not be clobbered by a colliding Extra key.
// Regression for yuzhichang's P1 review on PR #21273: wireMapErr previously
// overwrote typed fields unconditionally, so a ChunkDoc whose Extra carried a
// colliding "id" (which ChunkDocFromMap/UnmarshalJSON populate from legacy
// []map[string]any input) emitted the wrong id into indexing.
func TestWireMap_PreservesTypedIDOverExtra(t *testing.T) {
	d := ChunkDoc{
		ID:    "real-id",
		Text:  "body",
		Extra: map[string]json.RawMessage{"id": json.RawMessage(`"stale-id"`)},
	}
	m := d.ToMap()
	if got := m["id"]; got != "real-id" {
		t.Fatalf("ToMap id = %v, want %q (typed field must win over Extra)", got, "real-id")
	}
}

// R3 (MarshalJSON path): MarshalJSON delegates to wireMapErr, so the same
// precedence must hold on the JSON layer.
func TestMarshalJSON_PreservesTypedIDOverExtra(t *testing.T) {
	d := ChunkDoc{
		ID:    "real-id",
		Text:  "body",
		Extra: map[string]json.RawMessage{"id": json.RawMessage(`"stale-id"`)},
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got := out["id"]; got != "real-id" {
		t.Fatalf("marshaled id = %v, want %q", got, "real-id")
	}
}

// R2: fill-missing-only must still ADD Extra keys that are not typed fields.
func TestWireMap_AddsNonTypedExtraKeys(t *testing.T) {
	d := ChunkDoc{
		ID:    "real-id",
		Text:  "body",
		Extra: map[string]json.RawMessage{"custom_key": json.RawMessage(`"v"`)},
	}
	m := d.ToMap()
	if got := m["custom_key"]; got != "v" {
		t.Fatalf("custom Extra key = %v, want %q (non-typed Extra keys must survive)", got, "v")
	}
	if got := m["id"]; got != "real-id" {
		t.Fatalf("id = %v, want %q", got, "real-id")
	}
}

// R4/R5 corner: empty and nil Extra must not panic and must still emit typed
// fields.
func TestWireMap_EmptyAndNilExtra(t *testing.T) {
	d1 := ChunkDoc{ID: "a", Text: "t", Extra: map[string]json.RawMessage{}}
	if got := d1.ToMap()["id"]; got != "a" {
		t.Fatalf("empty Extra id = %v, want a", got)
	}
	d2 := ChunkDoc{ID: "b", Text: "t"}
	if got := d2.ToMap()["id"]; got != "b" {
		t.Fatalf("nil Extra id = %v, want b", got)
	}
}

// R4: ck_type in Extra must still be skipped (unrelated to the precedence fix,
// locked so the change does not resurrect the retired key).
func TestWireMap_SkipsRetiredCKType(t *testing.T) {
	d := ChunkDoc{
		ID:    "a",
		Text:  "t",
		Extra: map[string]json.RawMessage{"ck_type": json.RawMessage(`"text"`)},
	}
	m := d.ToMap()
	if _, ok := m["ck_type"]; ok {
		t.Fatalf("ck_type leaked: %+v", m)
	}
}

// R-bridge (reviewer's exact scenario): a legacy []map[string]any chunk whose
// id is later mutated must keep the mutated id through the GetChunks -> ToMap
// bridge, not the stale Extra copy that UnmarshalJSON populated. UnmarshalJSON's
// deletion whitelist omits "id", so decoding a legacy map copies id into both
// ChunkDoc.ID and ChunkDoc.Extra["id"]; a subsequent re-id step that updates
// only the typed field must not be shadowed by the stale Extra copy.
func TestGetChunksBridge_PreservesMutatedID(t *testing.T) {
	legacy := []map[string]any{{"id": "original-id", "text": "body"}}
	out := map[string]any{KeyChunks: legacy}

	typed, ok := GetChunks(out)
	if !ok || len(typed) != 1 {
		t.Fatalf("GetChunks ok=%v len=%d, want ok=true len=1", ok, len(typed))
	}
	// Simulate a re-id step that updates the typed field but leaves Extra stale.
	typed[0].ID = "reassigned-id"

	maps := ChunkDocsToMaps(typed)
	if got := maps[0]["id"]; got != "reassigned-id" {
		t.Fatalf("bridged id = %v, want %q (mutated typed id must survive the Extra copy)", got, "reassigned-id")
	}
}
