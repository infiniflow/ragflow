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
			// Regression: two consecutive breaks must collapse to exactly
			// one newline - not zero, and not a preserved blank line. Blank
			// lines are dropped, keeping one newline between two non-empty
			// lines.
			name: "consecutive hard line breaks collapse to one newline",
			irJSON: `{"sections":[{"elements":[
				{"type":"paragraph","content":[
					{"type":"text","text":"before"},
					{"type":"line_break"},
					{"type":"line_break"},
					{"type":"text","text":"after"}
				]}
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
