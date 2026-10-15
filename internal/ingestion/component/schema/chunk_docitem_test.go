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
// map path (ChunkDocsFromAny over []DocItem, which round-trips via ToMap).
// This pins the typed conversion to the exact wire shape the chunker relied
// on, so consuming DocItem directly cannot change chunking behavior.
func TestChunkDocsFromDocItem_ParallelsMapPath(t *testing.T) {
	cases := []struct {
		name  string
		items []parser.DocItem
	}{
		{
			name: "text with extra passthrough",
			items: []parser.DocItem{
				parser.TextItem{Text: "hello", CKType: "heading", Extra: map[string]any{"layout": "1", "page_number": 3}},
			},
		},
		{
			name: "image with extra passthrough",
			items: []parser.DocItem{
				parser.ImageItem{Text: "alt", Image: "data:image/png;base64,xxx", CKType: "image", Extra: map[string]any{"page_number": 2}},
			},
		},
		{
			name: "table with sheet and positions",
			items: []parser.DocItem{
				parser.TableItem{Table: &entity.TableData{}, Sheet: "s", Positions: [][]float64{{1, 2}}, Extra: map[string]any{"sheet_index": 0}},
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
				parser.ImageItem{Image: "i", CKType: "image"},
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
			want, wfound, werr := ChunkDocsFromAny(tc.items)
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
		parser.ImageItem{Image: "i", CKType: "image"},
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
