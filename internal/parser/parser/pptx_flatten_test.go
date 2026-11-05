//
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
//

package parser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestPPTXParser_FlattenMediaToText(t *testing.T) {
	ctx := context.Background()
	orig := pptxExtract
	defer func() { pptxExtract = orig }()

	slideWithTableIR := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"Slide Header"}]},
		{"type":"table","rows":[
			{"cells":[
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"Col1"}]}]},
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"Col2"}]}]}
			]},
			{"cells":[
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"ValA"}]}]},
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"ValB"}]}]}
			]}
		]}
	]}]}`

	pptxExtract = func(data []byte, format string) (string, string, string, error, error) {
		return slideWithTableIR, "", "", nil, nil
	}

	// 1. By default FlattenMediaToText is false: table preserves doc_type_kwd = "table"
	pDefault := NewPPTXParser()
	resDefault := pDefault.ParseWithResult(ctx, "deck.pptx", []byte("PK\x03\x04dummy"))
	if resDefault.Err != nil {
		t.Fatalf("ParseWithResult default error: %v", resDefault.Err)
	}
	itemsDefault := ItemsToMaps(resDefault.JSON)
	if len(itemsDefault) != 2 {
		t.Fatalf("got %d items, want 2", len(itemsDefault))
	}
	if got := itemsDefault[0]["doc_type_kwd"]; got != "text" {
		t.Errorf("item[0] doc_type_kwd = %v, want text", got)
	}
	if got := itemsDefault[1]["doc_type_kwd"]; got != "table" {
		t.Errorf("default item[1] doc_type_kwd = %v, want 'table'", got)
	}
	if _, ok := itemsDefault[1]["table"].(*entity.TableData); !ok {
		t.Errorf("default item[1] missing *entity.TableData")
	}

	// 2. With FlattenMediaToText = true: table is flattened to doc_type_kwd = "text"
	pFlatten := NewPPTXParser()
	pFlatten.ConfigureFromSetup(map[string]any{"flatten_media_to_text": true})
	if !pFlatten.FlattenMediaToText {
		t.Fatal("FlattenMediaToText was not set to true")
	}
	resFlatten := pFlatten.ParseWithResult(ctx, "deck.pptx", []byte("PK\x03\x04dummy"))
	if resFlatten.Err != nil {
		t.Fatalf("ParseWithResult flatten error: %v", resFlatten.Err)
	}
	itemsFlatten := ItemsToMaps(resFlatten.JSON)
	if len(itemsFlatten) != 2 {
		t.Fatalf("got %d items, want 2", len(itemsFlatten))
	}
	if got := itemsFlatten[1]["doc_type_kwd"]; got != "text" {
		t.Errorf("flattened item[1] doc_type_kwd = %v, want 'text'", got)
	}
	if _, ok := itemsFlatten[1]["table"]; ok {
		t.Errorf("flattened item[1] should not have 'table' field")
	}
	tableHTML, _ := itemsFlatten[1]["text"].(string)
	if !strings.Contains(tableHTML, "<table>") || !strings.Contains(tableHTML, "Col1") || !strings.Contains(tableHTML, "ValA") {
		t.Errorf("flattened table HTML = %q, want rendered HTML containing Col1 and ValA", tableHTML)
	}
}

func TestPPTParser_FlattenMediaToText(t *testing.T) {
	ctx := context.Background()
	orig := pptxExtract
	defer func() { pptxExtract = orig }()

	slideWithTableIR := `{"sections":[{"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"Legacy Slide"}]},
		{"type":"table","rows":[
			{"cells":[
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"H1"}]}]},
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"H2"}]}]}
			]},
			{"cells":[
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"D1"}]}]},
				{"content":[{"type":"paragraph","content":[{"type":"text","text":"D2"}]}]}
			]}
		]}
	]}]}`

	pptxExtract = func(data []byte, format string) (string, string, string, error, error) {
		return slideWithTableIR, "", "", nil, nil
	}

	p := NewPPTParser()
	p.ConfigureFromSetup(map[string]any{"flatten_media_to_text": true})
	if !p.FlattenMediaToText {
		t.Fatal("PPTParser.FlattenMediaToText was not set to true")
	}

	res := p.ParseWithResult(ctx, "legacy.ppt", []byte("dummy binary ppt"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	items := ItemsToMaps(res.JSON)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if got := items[1]["doc_type_kwd"]; got != "text" {
		t.Errorf("flattened doc_type_kwd = %v, want 'text'", got)
	}
	if _, ok := items[1]["table"]; ok {
		t.Errorf("flattened item should not have 'table' field")
	}
	html, _ := items[1]["text"].(string)
	if !strings.Contains(html, "<table>") || !strings.Contains(html, "H1") {
		t.Errorf("rendered table html = %q, want HTML table", html)
	}
}

func TestFlattenPresentationMediaToText(t *testing.T) {
	td := &entity.TableData{
		Grid: [][]entity.TableCell{
			{{Text: "HeaderA"}, {Text: "HeaderB"}},
			{{Text: "Row1A"}, {Text: "Row1B"}},
		},
		HeaderRows: 1,
	}
	items := []map[string]any{
		{
			"text":         "Slide text paragraph",
			"doc_type_kwd": "text",
		},
		{
			"table":        td,
			"doc_type_kwd": "table",
		},
		{
			"text":         "",
			"image":        "media_ref_123",
			"doc_type_kwd": "image",
		},
	}

	flattened := flattenPresentationMediaToText(items)
	if len(flattened) != 3 {
		t.Fatalf("len(flattened) = %d, want 3", len(flattened))
	}

	// 1. Text item remains text
	if got := flattened[0]["doc_type_kwd"]; got != "text" {
		t.Errorf("item[0] doc_type_kwd = %v, want text", got)
	}
	if got := flattened[0]["text"]; got != "Slide text paragraph" {
		t.Errorf("item[0] text = %v, want 'Slide text paragraph'", got)
	}

	// 2. Table item rendered to HTML text and doc_type_kwd set to text
	if got := flattened[1]["doc_type_kwd"]; got != "text" {
		t.Errorf("item[1] doc_type_kwd = %v, want text", got)
	}
	if _, ok := flattened[1]["table"]; ok {
		t.Errorf("item[1] still has table field after flatten")
	}
	tableText, _ := flattened[1]["text"].(string)
	if !strings.Contains(tableText, "<table>") || !strings.Contains(tableText, "HeaderA") {
		t.Errorf("item[1] text = %q, want rendered HTML table", tableText)
	}

	// 3. Image item doc_type_kwd set to text
	if got := flattened[2]["doc_type_kwd"]; got != "text" {
		t.Errorf("item[2] doc_type_kwd = %v, want text", got)
	}
}

func TestPPTXParser_TCADP_FlattenMediaToText(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	zipPayload := tcadpZipFixture(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reconstruct_document":
			_, _ = w.Write([]byte(`{"DocumentRecognizeResultUrl":"` + server.URL + `/download.zip"}`))
		case "/download.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := NewPPTXParser()
	p.ConfigureFromSetup(map[string]any{
		"parse_method":          "tcadp",
		"tcadp_apiserver":       server.URL,
		"flatten_media_to_text": true,
	})
	if !p.FlattenMediaToText {
		t.Fatal("FlattenMediaToText was not set to true")
	}

	res := p.ParseWithResult(ctx, "sample.pptx", []byte("mock pptx content"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult TCADP error: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("expected items from TCADP response")
	}
	for i, item := range res.JSON {
		m := item.ToMap()
		if m["doc_type_kwd"] != "text" {
			t.Errorf("item[%d] doc_type_kwd = %v, want 'text'", i, m["doc_type_kwd"])
		}
		if _, ok := m["table"]; ok {
			t.Errorf("item[%d] still has table field after flatten", i)
		}
	}
}

