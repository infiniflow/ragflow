//go:build cgo

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
	"encoding/base64"
	"strings"
	"testing"
)

func TestBuildDOCXJSONSections_Paragraphs(t *testing.T) {
	ir := `{"sections":[{"title":"","elements":[
		{"type":"paragraph","content":[{"type":"text","text":"Hello world"}],"style":"Normal"}
	]}]}`
	sections := buildDOCXJSONSections(ir)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	item := sections[0]
	if got, ok := item["text"].(string); !ok || got != "Hello world" {
		t.Errorf("text = %q, want %q", got, "Hello world")
	}
	if got, ok := item["doc_type_kwd"].(string); !ok || got != "text" {
		t.Errorf("doc_type_kwd = %q, want %q", got, "text")
	}
}

func TestBuildDOCXJSONSections_Headings(t *testing.T) {
	ir := `{"sections":[{"title":"","elements":[
		{"type":"heading","level":1,"content":[{"type":"text","text":"Chapter 1"}]}
	]}]}`
	sections := buildDOCXJSONSections(ir)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	item := sections[0]
	if got, ok := item["text"].(string); !ok || got != "Chapter 1" {
		t.Errorf("text = %q, want %q", got, "Chapter 1")
	}
	if got, ok := item["doc_type_kwd"].(string); !ok || got != "text" {
		t.Errorf("doc_type_kwd = %q, want %q", got, "text")
	}
	if got, ok := item["ck_type"].(string); !ok || got != "heading" {
		t.Errorf("ck_type = %q, want %q", got, "heading")
	}
}

func TestBuildDOCXJSONSections_Images(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("fake-image-data"))
	ir := `{"sections":[{"title":"","elements":[
		{"type":"image","data":"` + b64 + `"}
	]}]}`
	sections := buildDOCXJSONSections(ir)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	item := sections[0]
	if got, ok := item["text"].(string); !ok || got != "" {
		t.Errorf("text = %q, want empty", got)
	}
	if got, ok := item["image"].(string); !ok || got != b64 {
		t.Errorf("image = %q, want %q", got, b64)
	}
	if got, ok := item["doc_type_kwd"].(string); !ok || got != "image" {
		t.Errorf("doc_type_kwd = %q, want %q", got, "image")
	}
}

func TestBuildDOCXJSONSections_Tables(t *testing.T) {
	ir := `{"sections":[{"title":"","elements":[
		{"type":"table","rows":[
			{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A1"}]}]},{"content":[{"type":"paragraph","content":[{"type":"text","text":"B1"}]}]}]},
			{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"A2"}]}]},{"content":[{"type":"paragraph","content":[{"type":"text","text":"B2"}]}]}]}
		]}
	]}]}`
	sections := buildDOCXJSONSections(ir)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	item := sections[0]
	html, ok := item["text"].(string)
	if !ok {
		t.Fatal("text field missing or not string")
	}
	if !strings.Contains(html, "<table>") || !strings.Contains(html, "</table>") {
		t.Errorf("html = %q, missing <table> tags", html)
	}
	if !strings.Contains(html, "<tr>") || !strings.Contains(html, "</tr>") {
		t.Errorf("html = %q, missing <tr> tags", html)
	}
	if !strings.Contains(html, "<td>A1</td>") || !strings.Contains(html, "<td>B2</td>") {
		t.Errorf("html = %q, missing cell content", html)
	}
	if got, ok := item["doc_type_kwd"].(string); !ok || got != "table" {
		t.Errorf("doc_type_kwd = %q, want %q", got, "table")
	}
}

func TestBuildDOCXJSONSections_MixedContent(t *testing.T) {
	ir := `{"sections":[{"title":"","elements":[
		{"type":"paragraph","content":[{"type":"text","text":"First para"}]},
		{"type":"image","data":"aW1hZ2U="},
		{"type":"table","rows":[{"cells":[{"content":[{"type":"paragraph","content":[{"type":"text","text":"cell"}]}]}]}]},
		{"type":"heading","level":2,"content":[{"type":"text","text":"Sub title"}]}
	]}]}`
	sections := buildDOCXJSONSections(ir)
	if len(sections) != 4 {
		t.Fatalf("got %d sections, want 4", len(sections))
	}
	// paragraph first
	if got, _ := sections[0]["doc_type_kwd"].(string); got != "text" {
		t.Errorf("item[0].doc_type_kwd = %q, want %q", got, "text")
	}
	// image second
	if got, _ := sections[1]["doc_type_kwd"].(string); got != "image" {
		t.Errorf("item[1].doc_type_kwd = %q, want %q", got, "image")
	}
	// table third
	if got, _ := sections[2]["doc_type_kwd"].(string); got != "table" {
		t.Errorf("item[2].doc_type_kwd = %q, want %q", got, "table")
	}
	// heading fourth
	if got, _ := sections[3]["doc_type_kwd"].(string); got != "text" {
		t.Errorf("item[3].doc_type_kwd = %q, want %q", got, "text")
	}
	if got, _ := sections[3]["ck_type"].(string); got != "heading" {
		t.Errorf("item[3].ck_type = %q, want %q", got, "heading")
	}
}

func TestBuildDOCXJSONSections_EmptySkipped(t *testing.T) {
	ir := `{"sections":[{"title":"","elements":[
		{"type":"paragraph","content":[{"type":"text","text":""}]},
		{"type":"paragraph","content":[{"type":"text","text":"Only this matters"}]},
		{"type":"table","rows":[]}
	]}]}`
	sections := buildDOCXJSONSections(ir)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1 (only non-empty paragraph)", len(sections))
	}
	if got, _ := sections[0]["text"].(string); got != "Only this matters" {
		t.Errorf("text = %q, want %q", got, "Only this matters")
	}
}

func TestDocxIRTableToHTML_Empty(t *testing.T) {
	el := docxIRElement{Type: "table", Rows: []docxIRRow{}}
	html := docxIRTableToHTML(el)
	if html != "<table></table>" {
		t.Errorf("empty table html = %q, want %q", html, "<table></table>")
	}
}

func TestDocxIRTableToHTML_Single(t *testing.T) {
	el := docxIRElement{
		Type: "table",
		Rows: []docxIRRow{{
			Cells: []docxIRCell{{
				Content: []docxIRElement{{
					Type:    "paragraph",
					Content: []docxIRRun{{Type: "text", Text: "hello"}},
				}},
			}},
		}},
	}
	html := docxIRTableToHTML(el)
	want := "<table><tr><td>hello</td></tr></table>"
	if html != want {
		t.Errorf("single cell html = %q, want %q", html, want)
	}
}

func TestDocxIRTableToHTML_MultiRowCol(t *testing.T) {
	cell := func(text string) docxIRCell {
		return docxIRCell{
			Content: []docxIRElement{{
				Type:    "paragraph",
				Content: []docxIRRun{{Type: "text", Text: text}},
			}},
		}
	}
	el := docxIRElement{
		Type: "table",
		Rows: []docxIRRow{
			{Cells: []docxIRCell{cell("A1"), cell("B1")}},
			{Cells: []docxIRCell{cell("A2"), cell("B2")}},
		},
	}
	html := docxIRTableToHTML(el)
	want := "<table><tr><td>A1</td><td>B1</td></tr><tr><td>A2</td><td>B2</td></tr></table>"
	if html != want {
		t.Errorf("multi cell html = %q, want %q", html, want)
	}
}
