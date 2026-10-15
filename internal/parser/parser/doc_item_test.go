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

package parser

import (
	"reflect"
	"testing"

	"ragflow/internal/entity"
)

// mustMap fails the test if the two maps differ.
func mustMap(t *testing.T, want, got map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("map mismatch\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestTextItem_ToMap(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		mustMap(t, map[string]any{"text": "", DocTypeKey: DocTypeText},
			TextItem{}.ToMap())
	})
	t.Run("with text and no ck_type", func(t *testing.T) {
		mustMap(t, map[string]any{"text": "body", DocTypeKey: DocTypeText},
			TextItem{Text: "body"}.ToMap())
	})
	t.Run("ck_type emitted only when non-empty", func(t *testing.T) {
		mustMap(t, map[string]any{"text": "h", DocTypeKey: DocTypeText, "ck_type": "heading"},
			TextItem{Text: "h", CKType: "heading"}.ToMap())
		// empty CKType must NOT appear as a key.
		if _, ok := (TextItem{Text: "h"}).ToMap()["ck_type"]; ok {
			t.Error("ck_type present despite empty CKType")
		}
	})
	t.Run("extra passthrough", func(t *testing.T) {
		mustMap(t, map[string]any{
			"text":        "x",
			DocTypeKey:    DocTypeText,
			"layout":      "text",
			"page_number": float64(3),
			"level":       float64(2),
		}, TextItem{Text: "x", Extra: map[string]any{
			"layout": "text", "page_number": float64(3), "level": float64(2),
		}}.ToMap())
	})
}

func TestTextItem_ExtraDoesNotClobberCanonical(t *testing.T) {
	// An attacker/stray Extra key must never shadow the typed fields.
	got := TextItem{
		Text:   "real",
		CKType: "heading",
		Extra: map[string]any{
			"text":     "evil",
			DocTypeKey: "evil",
			"ck_type":  "evil",
			"layout":   "kept", // not a canonical key -> passes through
		},
	}.ToMap()
	mustMap(t, map[string]any{
		"text":     "real",
		DocTypeKey: DocTypeText,
		"ck_type":  "heading",
		"layout":   "kept",
	}, got)
}

func TestTableItem_ToMap(t *testing.T) {
	t.Run("nil table still emits shape", func(t *testing.T) {
		mustMap(t, map[string]any{
			"table":    (*entity.TableData)(nil),
			DocTypeKey: DocTypeTable,
			"ck_type":  DocTypeTable,
		}, TableItem{}.ToMap())
	})
	t.Run("ck_type falls back to table when empty", func(t *testing.T) {
		mustMap(t, map[string]any{
			"table":    (*entity.TableData)(nil),
			DocTypeKey: DocTypeTable,
			"ck_type":  DocTypeTable,
		}, TableItem{CKType: ""}.ToMap())
	})
	t.Run("ck_type explicit wins", func(t *testing.T) {
		mustMap(t, map[string]any{
			"table":     (*entity.TableData)(nil),
			DocTypeKey:  DocTypeTable,
			"ck_type":   "table",
			"sheet":     "Sheet1",
			"positions": [][]float64{{0, 1, 1, 2, 2}},
		}, TableItem{
			CKType:    "table",
			Sheet:     "Sheet1",
			Positions: [][]float64{{0, 1, 1, 2, 2}},
		}.ToMap())
	})
	t.Run("empty sheet/positions omitted", func(t *testing.T) {
		got := TableItem{Table: &entity.TableData{}, Sheet: "", Positions: nil}.ToMap()
		if _, ok := got["sheet"]; ok {
			t.Error("sheet present despite empty Sheet")
		}
		if _, ok := got["positions"]; ok {
			t.Error("positions present despite nil Positions")
		}
	})
}

func TestImageItem_ToMap(t *testing.T) {
	t.Run("docx-like: no ck_type, empty image", func(t *testing.T) {
		// docx image items historically set doc_type_kwd:"image" with no
		// ck_type and an empty image when over budget.
		mustMap(t, map[string]any{
			"text":     "",
			"image":    "",
			DocTypeKey: DocTypeImage,
		}, ImageItem{}.ToMap())
		if _, ok := (ImageItem{}).ToMap()["ck_type"]; ok {
			t.Error("ck_type present despite empty CKType")
		}
	})
	t.Run("xlsx-like: ck_type image + sheet + positions", func(t *testing.T) {
		mustMap(t, map[string]any{
			"text":      "alt",
			"image":     "data:image/png;base64,xxx",
			DocTypeKey:  DocTypeImage,
			"ck_type":   "image",
			"sheet":     "Sheet2",
			"positions": [][]float64{{1, 0, 0, 3, 4}},
		}, ImageItem{
			Text:      "alt",
			Image:     "data:image/png;base64,xxx",
			CKType:    "image",
			Sheet:     "Sheet2",
			Positions: [][]float64{{1, 0, 0, 3, 4}},
		}.ToMap())
	})
	t.Run("extra passthrough (media_order/parent_table_id)", func(t *testing.T) {
		mustMap(t, map[string]any{
			"text":            "",
			"image":           "",
			DocTypeKey:        DocTypeImage,
			"media_order":     float64(1),
			"parent_table_id": "t0",
			"row_index":       float64(0),
		}, ImageItem{Extra: map[string]any{
			"media_order": float64(1), "parent_table_id": "t0", "row_index": float64(0),
		}}.ToMap())
	})
}

func TestRawItem_Passthrough(t *testing.T) {
	t.Run("exact shape preserved", func(t *testing.T) {
		in := map[string]any{
			"text":      "hello",
			DocTypeKey:  DocTypeText,
			"ck_type":   "heading",
			"layout":    "title",
			"arbitrary": []int{1, 2, 3},
			"nested":    map[string]any{"k": "v"},
		}
		got := RawItem{M: in}.ToMap()
		mustMap(t, in, got)
	})
	t.Run("DocType is empty (lives in map)", func(t *testing.T) {
		if (RawItem{M: map[string]any{DocTypeKey: DocTypeText}}).DocType() != "" {
			t.Error("RawItem.DocType should be empty; discriminator is in the map")
		}
	})
	t.Run("nil map does not panic", func(t *testing.T) {
		var ri RawItem
		if ri.ToMap() != nil {
			t.Error("ToMap of nil RawItem.M should be nil")
		}
	})
}

func TestItemsToMaps(t *testing.T) {
	t.Run("order preserved, nil skipped", func(t *testing.T) {
		items := []DocItem{
			TextItem{Text: "a"},
			nil,
			RawItem{M: map[string]any{DocTypeKey: DocTypeText, "text": "b"}},
			TableItem{Table: &entity.TableData{}},
		}
		got := ItemsToMaps(items)
		if len(got) != 3 {
			t.Fatalf("expected 3 items (nil skipped), got %d: %#v", len(got), got)
		}
		if got[0]["text"] != "a" || got[1]["text"] != "b" {
			t.Errorf("order not preserved: %#v", got)
		}
		if got[2][DocTypeKey] != DocTypeTable {
			t.Errorf("third item should be table: %#v", got[2])
		}
	})
	t.Run("nil slice", func(t *testing.T) {
		if ItemsToMaps(nil) != nil {
			t.Error("ItemsToMaps(nil) should be nil")
		}
	})
}

func TestConstructorWireShapeRegression(t *testing.T) {
	// Guards that the two canonical constructors still emit the exact wire form
	// parsers (and the chunker) depend on.
	t.Run("NewTextJSONItem", func(t *testing.T) {
		mustMap(t, map[string]any{"text": "hi", DocTypeKey: DocTypeText},
			NewTextJSONItem("hi").ToMap())
	})
	t.Run("NewTableDataJSONItem", func(t *testing.T) {
		td := &entity.TableData{}
		mustMap(t, map[string]any{
			"table":     td,
			DocTypeKey:  DocTypeTable,
			"ck_type":   DocTypeTable,
			"sheet":     "S",
			"positions": [][]float64{{0, 0, 0, 1, 1}},
		}, NewTableDataJSONItem(td, "S", [][]float64{{0, 0, 0, 1, 1}}).ToMap())
	})
}

func TestToMapIndependentPerCall(t *testing.T) {
	// Mutating the returned map must not affect a later call or shared Extra.
	it := TextItem{Text: "x", Extra: map[string]any{"k": "v"}}
	first := it.ToMap()
	first["text"] = "mutated"
	second := it.ToMap()
	if second["text"] != "x" {
		t.Errorf("ToMap result was not independent: %#v", second)
	}
	if first["k"] != "v" || second["k"] != "v" {
		t.Errorf("Extra passthrough broke: %#v %#v", first, second)
	}
}
