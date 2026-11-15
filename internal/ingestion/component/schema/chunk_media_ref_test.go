// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestChunkDocMediaRefRoundTrip(t *testing.T) {
	ref := &entity.MediaRef{
		Format: "html",
		Kind:   entity.MediaKindKV,
		Scope:  "run-1",
		SHA256: strings.Repeat("a", 64),
		MIME:   "image/png",
		Size:   5,
	}
	doc, err := ChunkDocFromMap(map[string]any{
		"text":         "",
		"doc_type_kwd": "image",
		"media_ref":    ref,
	})
	if err != nil {
		t.Fatalf("ChunkDocFromMap: %v", err)
	}
	if doc.MediaRef == nil || doc.MediaRef.SHA256 != ref.SHA256 {
		t.Fatalf("media_ref lost in decode: %+v", doc.MediaRef)
	}
	if _, ok := doc.Extra["media_ref"]; ok {
		t.Fatal("media_ref leaked into Extra")
	}
	got := entity.MediaRefFromAny(doc.ToMap()["media_ref"])
	if got == nil || got.SHA256 != ref.SHA256 || got.Format != "html" || got.Kind != entity.MediaKindKV || got.Scope != "run-1" {
		t.Fatalf("media_ref lost in encode: %+v", doc.ToMap()["media_ref"])
	}
}

func TestChunkDocEmptyMediaRefNormalizesToNil(t *testing.T) {
	doc, err := ChunkDocFromMap(map[string]any{
		"text":      "x",
		"media_ref": map[string]any{},
	})
	if err != nil {
		t.Fatalf("ChunkDocFromMap: %v", err)
	}
	if doc.MediaRef != nil {
		t.Fatalf("empty media_ref survived as %+v", doc.MediaRef)
	}
	if _, ok := doc.ToMap()["media_ref"]; ok {
		t.Fatal("empty media_ref reappeared on the wire")
	}
}

func TestChunkDocUnknownMediaRefFieldsFail(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  map[string]any
	}{
		{"unknown only", map[string]any{"unknown": "value"}},
		{"empty known field", map[string]any{"format": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ChunkDocFromMap(map[string]any{
				"text":      "x",
				"media_ref": tc.raw,
			}); err == nil {
				t.Fatal("malformed media_ref was normalized away")
			}
		})
	}
}

func TestChunkDocNullMediaRefNormalizesToNil(t *testing.T) {
	doc, err := ChunkDocFromMap(map[string]any{
		"text":      "x",
		"media_ref": nil,
	})
	if err != nil {
		t.Fatalf("ChunkDocFromMap: %v", err)
	}
	if doc.MediaRef != nil {
		t.Fatalf("null media_ref survived as %+v", doc.MediaRef)
	}
}

func TestChunkDocPartialMediaRefSurvives(t *testing.T) {
	doc, err := ChunkDocFromMap(map[string]any{
		"text":      "x",
		"media_ref": map[string]any{"format": "html"},
	})
	if err != nil {
		t.Fatalf("ChunkDocFromMap: %v", err)
	}
	if doc.MediaRef == nil {
		t.Fatal("partially filled media_ref was normalized away")
	}
}
