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
	"reflect"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

// ChunkDocsFromDocItem must produce the same []ChunkDoc as the historical
// map path. The historical contract is ChunkDocsFromAny over the []map form
// the parser produced before emitting DocItems directly: ItemsToMaps
// projects each DocItem via its canonical ToMap, then ChunkDocFromMap decodes
// it. ChunkDocsFromDocItem must converge to that exact result, otherwise
// consuming DocItem directly would silently change chunking behavior.
//
// The baseline is deliberately the map path (NOT ChunkDocsFromAny(items),
// which dispatches []DocItem straight back to ChunkDocsFromDocItem and would
// make the comparison tautological — the bug this test guards against).
func TestChunkDocsFromDocItem_ParallelsMapPath(t *testing.T) {
	cases := []struct {
		name  string
		items []parser.DocItem
	}{
		{
			name: "text with extra passthrough",
			items: []parser.DocItem{
				parser.TextItem{Text: "hello", Extra: map[string]any{"layout": "1", "page_number": 3}},
			},
		},
		{
			name: "image with extra passthrough",
			items: []parser.DocItem{
				parser.ImageItem{Text: "alt", Image: "data:image/png;base64,xxx", Extra: map[string]any{"page_number": 2}},
			},
		},
		{
			name: "table with sheet and positions",
			items: []parser.DocItem{
				parser.TableItem{Table: &entity.TableData{}, Sheet: "s", Positions: parser.SpreadsheetPositions([][]float64{{1, 2, 2, 1, 3}}), Extra: map[string]any{"sheet_index": 0}},
			},
		},
		{
			name: "raw item passthrough",
			items: []parser.DocItem{
				parser.RawItem{M: map[string]any{"text": "x", "doc_type_kwd": "text", "page_number": 5}},
			},
		},
		{
			name: "mixed typed and raw",
			items: []parser.DocItem{
				parser.TextItem{Text: "a"},
				parser.RawItem{M: map[string]any{"text": "b", "doc_type_kwd": "text"}},
				parser.ImageItem{Image: "i"},
			},
		},
		// A canonical key present in Extra must NOT shadow the typed field when
		// the typed field is empty. ToMap keeps the empty canonical field and
		// blocks Extra via mergeExtra; the typed conversion must match exactly.
		// Regression guard for the divergent docItemWireMapFromFields builder,
		// which omitted empty canonical keys and let Extra override them.
		{
			name: "empty text must not be shadowed by extra text",
			items: []parser.DocItem{
				parser.TextItem{Text: "", Extra: map[string]any{"text": "stale"}},
			},
		},
		{
			name: "empty image text must not be shadowed by extra text",
			items: []parser.DocItem{
				parser.ImageItem{Text: "", Image: "i", Extra: map[string]any{"text": "stale"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found, err := ChunkDocsFromDocItem(tc.items)
			if err != nil {
				t.Fatalf("ChunkDocsFromDocItem err: %v", err)
			}
			if !found {
				t.Fatalf("expected found=true")
			}
			// Historical map-path baseline: ItemsToMaps -> ChunkDocsFromAny.
			maps := parser.ItemsToMaps(tc.items)
			want, wfound, werr := ChunkDocsFromAny(maps)
			if werr != nil || !wfound {
				t.Fatalf("map-path baseline unexpected: err=%v found=%v", werr, wfound)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("ChunkDocsFromDocItem diverges from map path\n got=%#v\nwant=%#v", got, want)
			}
		})
	}
}

// The dispatch boundary in ChunkDocsFromAny must route []parser.DocItem to
// ChunkDocsFromDocItem and report found=true, while any non-slice/unknown
// shape falls through to the default case and reports found=false. This is
// the only place a "non-DocItem" input can be rejected: ChunkDocsFromDocItem
// itself is statically typed to []parser.DocItem, so the guard lives here.
func TestChunkDocsFromAny_DispatchRejectsNonDocItem(t *testing.T) {
	if _, found, _ := ChunkDocsFromAny([]parser.DocItem{parser.TextItem{Text: "a"}}); !found {
		t.Fatalf("expected found=true for []parser.DocItem (routed to ChunkDocsFromDocItem)")
	}
	if _, found, _ := ChunkDocsFromAny("not a slice"); found {
		t.Fatalf("expected found=false for non-slice input")
	}
	// []map[string]any keeps its own map path (found=true) — it is NOT a
	// DocItem and must not be silently routed to ChunkDocsFromDocItem.
	if _, found, _ := ChunkDocsFromAny([]map[string]any{{"text": "x"}}); !found {
		t.Fatalf("expected found=true for []map[string]any via map path")
	}
}

// Empty slice is a valid DocItem input: found=true, no docs.
func TestChunkDocsFromDocItem_EmptySlice(t *testing.T) {
	got, found, err := ChunkDocsFromDocItem([]parser.DocItem{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true for empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d", len(got))
	}
}

// A nil element inside the slice must be skipped (never dereferenced),
// matching ItemsToMaps behavior. Regression guard against a nil-interface
// method call panic in the DocItem conversion path.
func TestChunkDocsFromDocItem_SkipsNilElement(t *testing.T) {
	items := []parser.DocItem{
		parser.TextItem{Text: "a"},
		nil,
		parser.ImageItem{Image: "i"},
	}
	got, found, err := ChunkDocsFromDocItem(items)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true")
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 docs after skipping nil, got %d", len(got))
	}
	if got[0].Text != "a" || got[1].Image != "i" {
		t.Fatalf("unexpected docs after nil skip: %#v", got)
	}
}

// Producers emit []map[string]any and wrap with parser.MapsToItems, which now
// detects doc_type_kwd and builds typed DocItems. The resulting []ChunkDoc the
// chunker sees must be identical to the historical path that decoded the raw
// []map directly. This pins the producer-side typed emission to the exact
// wire contract, so the migration cannot change chunking behavior.
func TestDocItemsFromMaps_ParallelsMapPath(t *testing.T) {
	cases := []struct {
		name string
		maps []map[string]any
	}{
		{
			name: "text with extra passthrough",
			maps: []map[string]any{
				{"text": "hello", parser.DocTypeKey: parser.DocTypeText, "ck_type": "heading", "layout": "1", "page_number": 3},
			},
		},
		{
			name: "image with extra passthrough",
			maps: []map[string]any{
				{"text": "alt", "image": "data:image/png;base64,xxx", parser.DocTypeKey: parser.DocTypeImage, "ck_type": "image", "page_number": 2},
			},
		},
		{
			name: "table with sheet and positions",
			maps: []map[string]any{
				{"table": &entity.TableData{}, parser.DocTypeKey: parser.DocTypeTable, "ck_type": parser.DocTypeTable, "sheet": "s", "positions": [][]float64{{1, 2}}, "sheet_index": 0},
			},
		},
		{
			name: "opaque map without doc_type_kwd stays raw",
			maps: []map[string]any{
				{"foo": "bar", "nested": map[string]any{"a": 1}},
			},
		},
		{
			name: "mixed typed and opaque",
			maps: []map[string]any{
				{"text": "a", parser.DocTypeKey: parser.DocTypeText, "ck_type": "text"},
				{"foo": "bar"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found, err := ChunkDocsFromDocItem(parser.MapsToItems(tc.maps))
			if err != nil {
				t.Fatalf("ChunkDocsFromDocItem err: %v", err)
			}
			if !found {
				t.Fatalf("expected found=true")
			}
			want, wfound, werr := ChunkDocsFromAny(tc.maps)
			if werr != nil || !wfound {
				t.Fatalf("map-path baseline unexpected: err=%v found=%v", werr, wfound)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("DocItemsFromMaps path diverges from map path\n got=%#v\nwant=%#v", got, want)
			}
		})
	}
}
