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
	"testing"
)

func TestPutGetChunks_RoundTrip(t *testing.T) {
	docs := []ChunkDoc{{Text: "a", ID: "1"}, {Text: "b", ID: "2"}}
	out := map[string]any{KeyChunks: docs}

	got, ok := GetChunks(out)
	if !ok {
		t.Fatal("GetChunks ok=false, want true")
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Text != "a" || got[1].Text != "b" {
		t.Fatalf("content mismatch: %+v", got)
	}
}

func TestGetChunks_LegacyMapBridge(t *testing.T) {
	legacy := []map[string]any{
		{"text": "a", "id": "1"},
		{"text": "b", "id": "2"},
	}
	out := map[string]any{KeyChunks: legacy}

	got, ok := GetChunks(out)
	if !ok {
		t.Fatal("GetChunks ok=false for legacy []map[string]any, want true")
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Text != "a" || got[1].Text != "b" {
		t.Fatalf("content mismatch: %+v", got)
	}
	if got[0].ID != "1" {
		t.Fatalf("id field lost through bridge: %+v", got[0])
	}
}

func TestGetChunks_EmptyAndNil(t *testing.T) {
	// nil input
	if _, ok := GetChunks(nil); ok {
		t.Fatal("nil input should return ok=false")
	}
	// absent key
	if _, ok := GetChunks(map[string]any{}); ok {
		t.Fatal("absent key should return ok=false")
	}
	// typed empty slice -> ok=true, len 0
	out := map[string]any{KeyChunks: []ChunkDoc{}}
	got, ok := GetChunks(out)
	if !ok {
		t.Fatal("empty typed []ChunkDoc should return ok=true")
	}
	if len(got) != 0 {
		t.Fatalf("empty typed len=%d, want 0", len(got))
	}
	// legacy empty slice -> ok=true, len 0
	out2 := map[string]any{KeyChunks: []map[string]any{}}
	got2, ok2 := GetChunks(out2)
	if !ok2 {
		t.Fatal("empty legacy []map[string]any should return ok=true")
	}
	if len(got2) != 0 {
		t.Fatalf("empty legacy len=%d, want 0", len(got2))
	}
}

func TestGetChunks_Garbage(t *testing.T) {
	// A slice of non-map values must not panic and must not yield partial
	// chunks. ChunkDocsFromAny skips non-map entries, so we accept either
	// (empty, true) or (nil, false) as safe.
	out := map[string]any{KeyChunks: []any{"not-a-map", 42}}
	got, ok := GetChunks(out)
	if ok && len(got) != 0 {
		t.Fatalf("garbage produced %d chunks, want 0", len(got))
	}
	// A scalar value for the key is not a chunk carrier.
	out2 := map[string]any{KeyChunks: 42}
	if _, ok2 := GetChunks(out2); ok2 {
		t.Fatal("scalar chunks value should return ok=false")
	}
}

func TestChunkCarrierToMaps(t *testing.T) {
	// []map[string]any is returned verbatim — no ChunkDoc round-trip, so
	// non-ChunkDoc fields (content_with_weight) survive.
	maps := []map[string]any{{"id": "c1", "content_with_weight": "X", "text": "t"}}
	got, ok := ChunkCarrierToMaps(maps)
	if !ok || len(got) != 1 {
		t.Fatalf("[]map carrier: ok=%v len=%d", ok, len(got))
	}
	if got[0]["content_with_weight"] != "X" {
		t.Fatalf("[]map carrier lost content_with_weight: %+v", got[0])
	}
	// []ChunkDoc is converted to the map wire form.
	got2, ok2 := ChunkCarrierToMaps([]ChunkDoc{{Text: "a", ID: "1"}})
	if !ok2 || len(got2) != 1 || got2[0]["text"] != "a" {
		t.Fatalf("[]ChunkDoc carrier: ok=%v got=%+v", ok2, got2)
	}
	// []any of maps is unwrapped.
	got3, ok3 := ChunkCarrierToMaps([]any{map[string]any{"id": "z"}})
	if !ok3 || len(got3) != 1 || got3[0]["id"] != "z" {
		t.Fatalf("[]any carrier: ok=%v got=%+v", ok3, got3)
	}
	// []any containing a non-map element is rejected wholesale (matches toChunkMaps).
	if _, ok4 := ChunkCarrierToMaps([]any{"not-a-map"}); ok4 {
		t.Fatal("[]any with non-map element should be ok=false")
	}
	// A scalar / unrelated value is not a chunk carrier.
	if _, ok5 := ChunkCarrierToMaps(42); ok5 {
		t.Fatal("scalar value should be ok=false")
	}
}
