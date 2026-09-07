package parser

import "testing"

// TestBuildPPTXJSONSections feeds office_oxide presentation IR JSON
// directly into buildPPTXJSONSections and asserts one JSON item per
// slide (section), including empty slides. Pure Go, runs under !cgo.
func TestBuildPPTXJSONSections(t *testing.T) {
	tests := []struct {
		name      string
		irJSON    string
		wantTexts []string // one entry per expected slide item
		wantErr   bool
	}{
		{
			name: "multi slide paragraphs",
			irJSON: `{"sections":[
				{"elements":[{"type":"paragraph","content":[{"type":"text","text":"Slide One"}]}]},
				{"elements":[{"type":"paragraph","content":[{"type":"text","text":"Slide Two"}]}]}
			]}`,
			wantTexts: []string{"Slide One", "Slide Two"},
		},
		{
			name: "empty slide retained",
			irJSON: `{"sections":[
				{"elements":[{"type":"paragraph","content":[{"type":"text","text":"First"}]}]},
				{"elements":[]},
				{"elements":[{"type":"paragraph","content":[{"type":"text","text":"Third"}]}]}
			]}`,
			wantTexts: []string{"First", "", "Third"},
		},
		{
			name: "bullet list with nested sub-list",
			irJSON: `{"sections":[{"elements":[
				{"type":"paragraph","content":[{"type":"text","text":"Agenda"}]},
				{"type":"list","items":[
					{"content":[{"type":"paragraph","content":[{"type":"text","text":"First point"}]}]},
					{"content":[{"type":"paragraph","content":[{"type":"text","text":"Second point"}]}],
					 "nested":{"items":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"Sub point"}]}]}]}}
				]}
			]}]}`,
			wantTexts: []string{"Agenda\nFirst point\nSecond point\nSub point"},
		},
		{
			name: "table cells",
			irJSON: `{"sections":[{"elements":[
				{"type":"table","rows":[
					{"cells":[
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"H1"}]}]},
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"H2"}]}]}
					]},
					{"cells":[
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]},
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}
					]}
				]}
			]}]}`,
			wantTexts: []string{"H1\nH2\na\nb"},
		},
		{
			name: "hard line break keeps newline",
			irJSON: `{"sections":[{"elements":[
				{"type":"paragraph","content":[
					{"type":"text","text":"before"},
					{"type":"line_break"},
					{"type":"text","text":"after"}
				]}
			]}]}`,
			wantTexts: []string{"before\nafter"},
		},
		{
			// Internal newlines emitted by the shared IR walker (e.g. between
			// table rows or list items) are preserved as-is. The element-level
			// split (one element -> one value or none if empty) is the only
			// collapse applied at this layer; per-line collapse across
			// elements happens only at element boundaries, where blank
			// elements are dropped.
			name: "consecutive line breaks inside one element stay as newlines",
			irJSON: `{"sections":[{"elements":[
				{"type":"paragraph","content":[
					{"type":"text","text":"before"},
					{"type":"line_break"},
					{"type":"line_break"},
					{"type":"text","text":"after"}
				]}
			]}]}`,
			wantTexts: []string{"before\n\nafter"},
		},
		{
			// Element-level collapse: a blank element between two text
			// elements is dropped, leaving a single newline between the
			// non-empty element values.
			name: "blank element between two text elements is dropped",
			irJSON: `{"sections":[{"elements":[
				{"type":"paragraph","content":[{"type":"text","text":"before"}]},
				{"type":"paragraph","content":[]},
				{"type":"paragraph","content":[{"type":"text","text":"after"}]}
			]}]}`,
			wantTexts: []string{"before\nafter"},
		},
		{
			name:      "bare text block passes through",
			irJSON:    `{"sections":[{"elements":[{"type":"text","text":"loose text"}]}]}`,
			wantTexts: []string{"loose text"},
		},
		{
			name:      "image-only slide yields empty text",
			irJSON:    `{"sections":[{"elements":[{"type":"image","data":"aGVsbG8="}]}]}`,
			wantTexts: []string{""},
		},
		{
			// bwbd.pptx regression: office_oxide wraps grouped slide shapes
			// (graphicFrame tables) in a text_box, so the slide's text_box
			// content is a table, not paragraphs. Previously flattened to "".
			name: "text_box wrapping table keeps text",
			irJSON: `{"sections":[{"elements":[{"type":"text_box","content":[
				{"type":"table","rows":[
					{"cells":[
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"Q1"}]}]},
						{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]}
					]}
				]}
			]}]}]}`,
			wantTexts: []string{"Q1\nA1"},
		},
		{
			// Same flattening path with a list inside the text_box: the
			// old paragraph-only walk dropped the whole block.
			name: "text_box wrapping list keeps text",
			irJSON: `{"sections":[{"elements":[{"type":"text_box","content":[
				{"type":"list","items":[
					{"content":[{"type":"paragraph","content":[{"type":"text","text":"li1"}]}]},
					{"content":[{"type":"paragraph","content":[{"type":"text","text":"li2"}]}]}
				]}
			]}]}]}`,
			wantTexts: []string{"li1\nli2"},
		},
		{
			// Nested text boxes recurse through the same walker.
			name: "nested text_box keeps text",
			irJSON: `{"sections":[{"elements":[{"type":"text_box","content":[
				{"type":"paragraph","content":[{"type":"text","text":"outer"}]},
				{"type":"text_box","content":[
					{"type":"paragraph","content":[{"type":"text","text":"inner"}]}
				]}
			]}]}]}`,
			wantTexts: []string{"outer\ninner"},
		},
		{
			// Table cell holding a list: joinCellText must recurse instead
			// of reading paragraph runs only.
			name: "table cell wrapping list keeps text",
			irJSON: `{"sections":[{"elements":[{"type":"table","rows":[
				{"cells":[
					{"content":[
						{"type":"paragraph","content":[{"type":"text","text":"head"}]},
						{"type":"list","items":[
							{"content":[{"type":"paragraph","content":[{"type":"text","text":"li1"}]}]},
							{"content":[{"type":"paragraph","content":[{"type":"text","text":"li2"}]}]}
						]}
					]}
				]}
			]}]}]}`,
			wantTexts: []string{"head\nli1\nli2"},
		},
		{
			name:    "invalid JSON",
			irJSON:  "{not json",
			wantErr: true,
		},
		{
			name:      "no sections yields no items",
			irJSON:    `{"sections":[]}`,
			wantTexts: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := buildPPTXJSONSections(tt.irJSON)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildPPTXJSONSections: want error, got items %+v", items)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildPPTXJSONSections: %v", err)
			}
			if len(items) != len(tt.wantTexts) {
				t.Fatalf("items = %d, want %d: %+v", len(items), len(tt.wantTexts), items)
			}
			for i, want := range tt.wantTexts {
				if got := items[i]["text"]; got != want {
					t.Errorf("item %d text = %q, want %q", i, got, want)
				}
				if got := items[i]["slide_number"]; got != i+1 {
					t.Errorf("item %d slide_number = %v, want %d", i, got, i+1)
				}
				if got := items[i]["doc_type_kwd"]; got != "text" {
					t.Errorf("item %d doc_type_kwd = %v, want text", i, got)
				}
			}
		})
	}
}

// TestItemsAllEmpty pins the all-empty fallback gate: empty and whitespace
// items trigger the PlainText salvage, any non-empty item does not.
func TestItemsAllEmpty(t *testing.T) {
	tests := []struct {
		name  string
		items []map[string]any
		want  bool
	}{
		{name: "nil", items: nil, want: true},
		{name: "empty", items: []map[string]any{}, want: true},
		{
			name:  "all empty",
			items: []map[string]any{{"text": ""}, {"text": "  \n "}},
			want:  true,
		},
		{
			name:  "one non-empty",
			items: []map[string]any{{"text": ""}, {"text": "hello"}},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := itemsAllEmpty(tt.items); got != tt.want {
				t.Errorf("itemsAllEmpty = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestItemsFromPlainText pins the whole-document plain-text salvage used
// when the structured IR is unusable: non-empty text becomes a single
// trimmed item; blank text yields an empty, non-nil slice.
func TestItemsFromPlainText(t *testing.T) {
	items := itemsFromPlainText("  whole deck text  ")
	if len(items) != 1 || items[0]["text"] != "whole deck text" || items[0]["doc_type_kwd"] != "text" {
		t.Fatalf("items = %+v, want a single trimmed text item", items)
	}
	items = itemsFromPlainText("   ")
	if items == nil || len(items) != 0 {
		t.Fatalf("items = %+v, want an empty non-nil slice", items)
	}
}
