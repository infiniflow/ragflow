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
	t.Run("with text", func(t *testing.T) {
		mustMap(t, map[string]any{"text": "body", DocTypeKey: DocTypeText},
			TextItem{Text: "body"}.ToMap())
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
	// An attacker/stray Extra key must never shadow the typed fields, and a
	// retired ck_type must not be resurrected through Extra.
	got := TextItem{
		Text: "real",
		Extra: map[string]any{
			"text":     "evil",
			DocTypeKey: "evil",
			"layout":   "kept", // not a canonical key -> passes through
		},
	}.ToMap()
	mustMap(t, map[string]any{
		"text":     "real",
		DocTypeKey: DocTypeText,
		"layout":   "kept",
	}, got)
}

func TestTableItem_ToMap(t *testing.T) {
	t.Run("nil table still emits shape", func(t *testing.T) {
		mustMap(t, map[string]any{
			"table":    (*entity.TableData)(nil),
			DocTypeKey: DocTypeTable,
		}, TableItem{}.ToMap())
	})
	t.Run("sheet and positions", func(t *testing.T) {
		mustMap(t, map[string]any{
			"table":     (*entity.TableData)(nil),
			DocTypeKey:  DocTypeTable,
			"sheet":     "Sheet1",
			"positions": [][]float64{{0, 1, 1, 2, 2}},
		}, TableItem{
			Sheet:     "Sheet1",
			Positions: SpreadsheetPositions([][]float64{{0, 1, 1, 2, 2}}),
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
	t.Run("docx-like: empty image", func(t *testing.T) {
		// docx image items historically set doc_type_kwd:"image" with an empty
		// image when over budget.
		mustMap(t, map[string]any{
			"text":     "",
			"image":    "",
			DocTypeKey: DocTypeImage,
		}, ImageItem{}.ToMap())
	})
	t.Run("xlsx-like: sheet + positions", func(t *testing.T) {
		mustMap(t, map[string]any{
			"text":      "alt",
			"image":     "data:image/png;base64,xxx",
			DocTypeKey:  DocTypeImage,
			"sheet":     "Sheet2",
			"positions": [][]float64{{1, 0, 0, 3, 4}},
		}, ImageItem{
			Text:      "alt",
			Image:     "data:image/png;base64,xxx",
			Sheet:     "Sheet2",
			Positions: SpreadsheetPositions([][]float64{{1, 0, 0, 3, 4}}),
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
			"layout":    "title",
			"arbitrary": []int{1, 2, 3},
			"nested":    map[string]any{"k": "v"},
		}
		got := RawItem{M: in}.ToMap()
		mustMap(t, in, got)
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
		// Spreadsheet positions render as [][]float64 on the wire (the historical
		// shape the ingestion indexer consumes); only PDF positions stay [][]any.
		mustMap(t, map[string]any{
			"table":     td,
			DocTypeKey:  DocTypeTable,
			"sheet":     "S",
			"positions": [][]float64{{0, 0, 0, 1, 1}},
		}, NewTableDataJSONItem(td, "S", SpreadsheetPositions([][]float64{{0, 0, 0, 1, 1}})).ToMap())
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

// DocItemFromMap detects doc_type_kwd and builds the matching typed item,
// extracting canonical fields into the struct and leaving the rest in Extra.
// For an opaque map without a known discriminator it falls back to RawItem,
// preserving the map exactly. The produced item's ToMap must equal the input
// map (the wire shape is unchanged), so producers can emit typed items
// without altering downstream chunking.
func TestDocItemFromMap_WireShapePreserved(t *testing.T) {
	textMap := map[string]any{
		"text":        "hello",
		DocTypeKey:    DocTypeText,
		"layout":      "1",
		"page_number": 3,
	}
	imageMap := map[string]any{
		"text":        "alt",
		"image":       "data:image/png;base64,xxx",
		DocTypeKey:    DocTypeImage,
		"page_number": 2,
	}
	tableMap := map[string]any{
		"table":       &entity.TableData{},
		DocTypeKey:    DocTypeTable,
		"sheet":       "s",
		"positions":   [][]float64{{1, 2}},
		"sheet_index": 0,
	}
	opaqueMap := map[string]any{"foo": "bar", "nested": map[string]any{"a": 1}}

	// PDF crop positions arrive as [][]any (normalizePDFPositions), not
	// [][]float64; the typed conversion must preserve them verbatim. Regression
	// guard for the positions-dropping bug caught during review.
	imageMapAny := map[string]any{
		"text":      "alt",
		"image":     "data:image/png;base64,xxx",
		DocTypeKey:  DocTypeImage,
		"positions": [][]any{{1, 2, 3, 4}},
	}
	tableMapAny := map[string]any{
		"table":     &entity.TableData{},
		DocTypeKey:  DocTypeTable,
		"positions": [][]any{{1, 2, 3, 4}},
	}
	// A table whose "table" value is a JSON-decoded map[string]any (not a typed
	// *entity.TableData — the shape a component-boundary JSON payload produces)
	// must NOT be silently coerced into a TableItem with a nil table, which would
	// emit "table": null and drop the data. The failed type assertion must fall
	// back to RawItem, preserving the map verbatim — the same round-trip guard
	// the text/image branches uphold. Regression guard for the silent
	// table-dropping path.
	tableMapJSONDecoded := map[string]any{
		"table":    map[string]any{"grid": []any{}, "header_rows": float64(1)},
		DocTypeKey: DocTypeTable,
	}

	for _, m := range []map[string]any{textMap, imageMap, tableMap, opaqueMap, imageMapAny, tableMapAny, tableMapJSONDecoded} {
		got := DocItemFromMap(m).ToMap()
		if !reflect.DeepEqual(got, m) {
			t.Fatalf("DocItemFromMap round-trip mismatch for %q\n got=%#v\nwant=%#v", m[DocTypeKey], got, m)
		}
	}

	// The typed detection must actually yield the concrete type, not RawItem.
	if _, ok := DocItemFromMap(textMap).(TextItem); !ok {
		t.Error("text map should produce a TextItem")
	}
	if _, ok := DocItemFromMap(imageMap).(ImageItem); !ok {
		t.Error("image map should produce an ImageItem")
	}
	if _, ok := DocItemFromMap(tableMap).(TableItem); !ok {
		t.Error("table map should produce a TableItem")
	}
	if _, ok := DocItemFromMap(opaqueMap).(RawItem); !ok {
		t.Error("opaque map should fall back to RawItem")
	}
}

// A legacy payload may still carry the retired ck_type key. Typed conversion
// must drop it rather than route it through Extra and re-emit it.
func TestDocItemFromMap_DropsRetiredCKType(t *testing.T) {
	legacy := map[string]any{
		"text":     "h",
		DocTypeKey: DocTypeText,
		"ck_type":  "heading",
		"layout":   "heading",
	}
	got := DocItemFromMap(legacy).ToMap()
	if _, ok := got["ck_type"]; ok {
		t.Fatalf("retired ck_type leaked through ToMap: %#v", got)
	}
	if got["layout"] != "heading" {
		t.Fatalf("layout lost during conversion: %#v", got)
	}
}

// DocItemsFromMaps maps a []map[string]any to typed DocItems the same way
// DocItemFromMap does, preserving order and skipping nothing.
func TestMapsToItems_OrderAndType(t *testing.T) {
	maps := []map[string]any{
		{"text": "a", DocTypeKey: DocTypeText},
		{"foo": "bar"},
	}
	items := MapsToItems(maps)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if _, ok := items[0].(TextItem); !ok {
		t.Errorf("first should be TextItem: %#v", items[0])
	}
	if _, ok := items[1].(RawItem); !ok {
		t.Errorf("second (opaque) should be RawItem: %#v", items[1])
	}
	if items[0].ToMap()["text"] != "a" {
		t.Errorf("first item text lost: %#v", items[0].ToMap())
	}
}
