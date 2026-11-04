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

package parser

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestNewTableDataJSONItem_IncludesDocType(t *testing.T) {
	td := &entity.TableData{
		Grid:       [][]entity.TableCell{{{Text: "col"}}, {{Text: "value"}}},
		HeaderRows: 1,
		Caption:    "Sheet1",
	}
	item := NewTableDataJSONItem(td, "Sheet1", SpreadsheetPositions([][]float64{{1, 2, 2, 1, 1}}))
	m := item.ToMap()

	if got, want := m[DocTypeKey], DocTypeTable; got != want {
		t.Fatalf("item[%q] = %v, want %v", DocTypeKey, got, want)
	}
	got, ok := m["table"].(*entity.TableData)
	if !ok {
		t.Fatalf("item[%q] = %T, want *entity.TableData", "table", m["table"])
	}
	if got.Caption != "Sheet1" || len(got.Grid) != 2 {
		t.Fatalf("item table = %+v, want caption Sheet1 and 2 rows", got)
	}
	if _, ok := m["positions"].([][]float64); !ok {
		t.Fatalf("item[%q] missing positions matrix", "positions")
	}
}

// TestParseResult_Contract pins the public-key invariants the component-level
// caller (component/parser.go) relies on after the produced/Markdown fields
// were removed:
//
//   - a successful result carries a populated payload (JSON and/or File)
//   - a failed result has Err set and no payload
//   - Produced() is the single success/failure boundary
func TestParseResult_Contract(t *testing.T) {
	cases := []struct {
		name         string
		in           ParseResult
		wantErr      bool
		wantProduced bool
	}{
		{
			name:         "json payload",
			in:           ParseResult{JSON: MapsToItems([]map[string]any{{"text": "hello", "doc_type_kwd": "text"}})},
			wantProduced: true,
		},
		{
			name:         "file metadata only",
			in:           ParseResult{File: map[string]any{"name": "x"}},
			wantProduced: true,
		},
		{
			name:    "failure carries no payload",
			in:      ParseResult{Err: errSentinel},
			wantErr: true,
		},
		{
			name:         "empty success carries no payload",
			in:           ParseResult{},
			wantProduced: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.wantErr && tc.in.Err == nil {
				t.Errorf("Err: want non-nil, got nil")
			}
			if tc.in.Produced() != tc.wantProduced {
				t.Errorf("Produced() = %v, want %v (Err=%v)", tc.in.Produced(), tc.wantProduced, tc.in.Err)
			}
		})
	}
}

// errSentinel is a tiny stand-in so the test does not need to
// import errors just to declare one.
var errSentinel = sentinelErr("sentinel")

type sentinelErr string

func (s sentinelErr) Error() string { return string(s) }

// TestMarkdownParser_ParseWithResult pins the migration exemplar
// — port-rag-flow-pipeline-to-go.md §6.5 marks MarkdownParser as
// the first ported format whose ParseWithResult surface emulates
// the Python "json" produced. The fixture is intentionally
// tiny: 1 heading, 1 paragraph, 1 unordered list item, no nested
// formatting.
func TestMarkdownParser_ParseWithResult(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	src := []byte("# Title\n\nFirst paragraph.\n\n- Item one\n")
	res := p.ParseWithResult(ctx, "doc.md", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !res.Produced() {
		t.Errorf("Produced() = false, want true (markdown parser emits JSON)")
	}
	if res.File == nil || res.File["name"] != "doc.md" {
		t.Errorf("File.name = %v, want doc.md", res.File)
	}
	if res.JSON == nil {
		t.Fatal("JSON: want non-nil slice, got nil")
	}
	// 1 heading + 1 paragraph + 1 list = 3 items.
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3 (heading, paragraph, list)", len(res.JSON))
	}
	heading := res.JSON[0].ToMap()
	if txt, _ := heading["text"].(string); !strings.Contains(txt, "Title") {
		t.Errorf("JSON[0].text = %q, want contains 'Title'", txt)
	}
	if got, _ := heading["layout"].(string); got != "heading" {
		t.Errorf("JSON[0].layout = %q, want heading", got)
	}
	for i, it := range ItemsToMaps(res.JSON) {
		if _, ok := it["doc_type_kwd"]; !ok {
			t.Errorf("JSON[%d] missing doc_type_kwd: %+v", i, it)
		}
	}
}

// TestParseResultProducer_PDFIsProducer pins the explicit
// PDFParser ParseResultProducer wiring. The dispatch seam routes
// PDFs through ParseWithResult regardless of whether the current
// build has the cgo-backed DeepDOC engine enabled.
func TestParseResultProducer_PDFIsProducer(t *testing.T) {
	pdf := &PDFParser{}
	if _, ok := any(pdf).(ParseResultProducer); !ok {
		t.Error("PDFParser must implement ParseResultProducer so the " +
			"dispatch seam routes PDFs through ParseWithResult")
	}
}
