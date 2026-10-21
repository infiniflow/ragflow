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

package chunker

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestMergePositionMatrixAcceptsPageLists pins the Title-family parity fix:
// positions produced by layout.SectionsToJSON keep the page as a list until
// normalization, and a source may be a single flat 5-tuple (TCADP). Both must
// survive the merge instead of being dropped, so the on-demand PDF crop can
// still run.
func TestMergePositionMatrixAcceptsPageLists(t *testing.T) {
	listPage := json.RawMessage(`[[[1],10,100,10,100]]`)
	flatRow := json.RawMessage(`[2,5,50,5,50]`)
	duplicate := json.RawMessage(`[[[1],10,100,10,100]]`)

	got := mergePositionMatrix(listPage, flatRow, duplicate)
	want := [][]any{
		{[]any{float64(1)}, float64(10), float64(100), float64(10), float64(100)},
		{float64(2), float64(5), float64(50), float64(5), float64(50)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergePositionMatrix = %#v, want %#v", got, want)
	}
}

// TestTitleFamilyKeepsListPagePositions is the end-to-end guard: an image
// record whose page cell is a list must keep its coordinates on the emitted
// chunk, matching General's pass-through behavior.
func TestTitleFamilyKeepsListPagePositions(t *testing.T) {
	pos := [][]any{{[]any{1}, 10, 100, 10, 100}}
	items := []map[string]any{
		{"text": "before", "doc_type_kwd": "text"},
		{"text": "Figure 1: caption", "doc_type_kwd": "image", "positions": pos, "_pdf_positions": pos},
	}
	levels := [][]string{{`^# `}}

	title, err := NewGroupTitleChunker(map[string]any{"levels": levels})
	if err != nil {
		t.Fatalf("NewGroupTitleChunker: %v", err)
	}
	out, err := title.Invoke(t.Context(), nil, map[string]any{
		"name": "doc.pdf", "output_format": "json", "json": items,
	})
	if err != nil {
		t.Fatalf("Title Invoke: %v", err)
	}
	titleChunks, _ := out["chunks"].([]map[string]any)
	var titleImage map[string]any
	for _, ck := range titleChunks {
		if ck["doc_type_kwd"] == "image" {
			titleImage = ck
		}
	}
	if titleImage == nil {
		t.Fatalf("Title emitted no image chunk: %#v", titleChunks)
	}
	assertListPagePositions(t, "title", titleImage)

	general, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	gout, err := general.Invoke(t.Context(), nil, map[string]any{
		"name": "doc.pdf", "file_type": "pdf", "output_format": "json", "json": items,
	})
	if err != nil {
		t.Fatalf("General Invoke: %v", err)
	}
	generalChunks, _ := gout["chunks"].([]map[string]any)
	var generalImage map[string]any
	for _, ck := range generalChunks {
		if ck["doc_type_kwd"] == "image" {
			generalImage = ck
		}
	}
	if generalImage == nil {
		t.Fatalf("General emitted no image chunk: %#v", generalChunks)
	}
	assertListPagePositions(t, "general", generalImage)
}

func assertListPagePositions(t *testing.T, name string, ck map[string]any) {
	t.Helper()
	want := []any{[]any{[]any{float64(1)}, float64(10), float64(100), float64(10), float64(100)}}
	for _, key := range []string{"positions", "_pdf_positions"} {
		raw, err := json.Marshal(ck[key])
		if err != nil {
			t.Fatalf("%s %s: marshal: %v", name, key, err)
		}
		var got any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("%s %s: unmarshal: %v", name, key, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s %s = %s, want [[[1],10,100,10,100]]", name, key, raw)
		}
	}
}
