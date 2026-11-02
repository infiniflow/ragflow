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
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

// TestChunksSerializationParity is the R1 regression gate: the []ChunkDoc wire
// form and the []map[string]any wire form (ChunkDocsToMaps) must serialize
// byte-semantically identically. Any
// code that json.Marshal's the envelope (e.g. the agent canvas marshalling
// node outputs) must produce JSON byte-semantically identical to the historical
// []map[string]any wire form produced by ChunkDocsToMaps. If this drifts,
// external consumers of the serialized envelope observe a shape change.
func TestChunksSerializationParity(t *testing.T) {
	cases := map[string]ChunkDoc{
		"simple text": {
			Text:    "hello world",
			DocType: "text",
			ID:      "id-1",
		},
		"empty text media-only": {
			Text:    "",
			Image:   "img/1.png",
			ImgID:   "imgid-1",
			DocType: "image",
		},
		"with extra": {
			Text: "body",
			Extra: map[string]json.RawMessage{
				"custom_key":  json.RawMessage(`"v"`),
				"page_number": json.RawMessage(`3`),
			},
		},
		"with positions": {
			Text:         "positioned",
			PDFPositions: json.RawMessage(`[[1,2,3,4]]`),
			Positions:    json.RawMessage(`[[10,20,30,40]]`),
		},
		"with table data": {
			Text:      "table chunk",
			TableData: &entity.TableData{},
		},
		"token nums pointer": {
			Text:   "counted",
			TKNums: intptr(42),
		},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			typedJSON, err := json.Marshal([]ChunkDoc{d})
			if err != nil {
				t.Fatalf("marshal []ChunkDoc: %v", err)
			}
			mapJSON, err := json.Marshal(ChunkDocsToMaps([]ChunkDoc{d}))
			if err != nil {
				t.Fatalf("marshal ChunkDocsToMaps: %v", err)
			}
			assertSemanticJSONEqual(t, typedJSON, mapJSON)
		})
	}
}

// assertSemanticJSONEqual compares two JSON documents for structural equality,
// ignoring object key order (which is not semantically meaningful to consumers).
func assertSemanticJSONEqual(t *testing.T, a, b []byte) {
	t.Helper()
	var av, bv any
	if err := json.Unmarshal(a, &av); err != nil {
		t.Fatalf("unmarshal A (%s): %v", a, err)
	}
	if err := json.Unmarshal(b, &bv); err != nil {
		t.Fatalf("unmarshal B (%s): %v", b, err)
	}
	if !reflect.DeepEqual(av, bv) {
		t.Fatalf("serialization parity mismatch:\n typed []ChunkDoc = %s\n map  wire form   = %s", a, b)
	}
}

// TestChunkDocMarshalJSON_PropagatesError locks that a marshal failure (e.g.
// invalid JSON bytes in the RawMessage position fields) is surfaced as an
// error, not silently swallowed into an empty object. The pre-refactor
// MarshalJSON propagated the error; the wireMap refactor must preserve that,
// otherwise a corrupt chunk serializes as {} with no signal.
func TestChunkDocMarshalJSON_PropagatesError(t *testing.T) {
	cases := map[string]ChunkDoc{
		"invalid PDFPositions": {Text: "x", PDFPositions: json.RawMessage("{not json")},
		"invalid Positions":    {Text: "x", Positions: json.RawMessage("[[1,2")},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := json.Marshal(d); err == nil {
				t.Fatalf("expected marshal error for %s, got nil (error swallowed)", name)
			}
		})
	}
}

// TestChunkDoc_RetiredCKTypeNotLeaked locks #21259's ck_type retirement
// contract through the refactored wireMap path: a ck_type that ends up in
// ChunkDoc.Extra (e.g. set programmatically) must not be re-emitted by ToMap /
// ChunkDocsToMaps, while legitimate Extra keys survive.
func TestChunkDoc_RetiredCKTypeNotLeaked(t *testing.T) {
	d := ChunkDoc{
		Text: "body",
		Extra: map[string]json.RawMessage{
			"ck_type": json.RawMessage(`"text"`), // retired parser key
			"custom":  json.RawMessage(`"keep"`),
		},
	}
	m := d.ToMap()
	if _, leaked := m["ck_type"]; leaked {
		t.Fatalf("retired ck_type leaked from Extra through ToMap: %+v", m)
	}
	if m["custom"] != "keep" {
		t.Fatalf("legitimate Extra key dropped: %+v", m)
	}
	// Same guarantee through the map wire form used by the envelope bridge.
	maps := ChunkDocsToMaps([]ChunkDoc{d})
	if _, leaked := maps[0]["ck_type"]; leaked {
		t.Fatalf("retired ck_type leaked through ChunkDocsToMaps: %+v", maps[0])
	}
}

func intptr(n int) *int { return &n }
