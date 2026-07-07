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

// Slice 1 tests for port-rag-flow-pipeline-to-go.md Phase 2.5.
// These pin the new ParseWithResult contracts for the parsers
// that did not previously satisfy ParseResultProducer:
//
//   - HTMLParser — block-level walker that emits the python-compatible
//     {text, doc_type_kwd, ck_type} shape.
//   - TextParser — paragraph-splitting for the text&code family
//     (.txt / .py / .js / .java / .c / .cpp / .h / .php / .go / .ts
//     / .sh / .cs / .kt / .sql).
//
// MarkdownParser's ParseWithResult is already pinned by
// parse_result_test.go (prior slice). PDFParser and the office
// variants remain deferred to a follow-up slice that wires
// them to the existing internal/deepdoc/parser/pdf pipeline and
// office_oxide libraries.

package parser

import (
	"strings"
	"testing"

	"ragflow/internal/utility"
)

// TestTextParser_ParseWithResult_ParaSplit pins the paragraph-split
// rule. A blank-line-separated input yields one item per
// paragraph; the python TxtParser does the same.
func TestTextParser_ParseWithResult_ParaSplit(t *testing.T) {
	p, err := NewTextParser("")
	if err != nil {
		t.Fatalf("NewTextParser: %v", err)
	}
	src := []byte("First paragraph.\n\nSecond paragraph.\n\nThird.")
	res := p.ParseWithResult("doc.txt", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if got, want := res.File["name"], "doc.txt"; got != want {
		t.Errorf("File.name = %v, want %v", got, want)
	}
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3 (one per paragraph)", len(res.JSON))
	}
	if got, want := res.JSON[0]["text"], "First paragraph."; got != want {
		t.Errorf("JSON[0].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[2]["text"], "Third."; got != want {
		t.Errorf("JSON[2].text = %v, want %v", got, want)
	}
}

// TestTextParser_ParseWithResult_Empty pins the empty-input
// fallback (one empty item, not nil) so the downstream chunker
// sees a non-nil JSON slice. Mirrors the MarkdownParser convention
// at markdown_parser.go:71-76.
func TestTextParser_ParseWithResult_Empty(t *testing.T) {
	p, _ := NewTextParser("")
	res := p.ParseWithResult("empty.txt", []byte{})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Errorf("JSON len = %d, want 1 (empty-input fallback)", len(res.JSON))
	}
}

// TestTextParser_ParseWithResult_LongParagraphSlicing pins the
// maxItemBytes boundary behaviour. A single paragraph longer
// than 8192 bytes is sliced at the nearest line boundary.
func TestTextParser_ParseWithResult_LongParagraphSlicing(t *testing.T) {
	p, _ := NewTextParser("")
	long := strings.Repeat("a", 9000)
	res := p.ParseWithResult("long.txt", []byte(long))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) < 2 {
		t.Errorf("JSON len = %d, want >=2 (sliced at maxItemBytes)", len(res.JSON))
	}
	for i, it := range res.JSON {
		if txt, _ := it["text"].(string); len(txt) > 8192 {
			t.Errorf("JSON[%d].text len = %d, exceeds maxItemBytes=8192", i, len(txt))
		}
	}
}

// TestTextParser_ParseWithResult_InvalidUTF8 pins the UTF-8
// validation rule. Invalid bytes produce an error in the result
// (matching the python TxtParser's behaviour).
func TestTextParser_ParseWithResult_InvalidUTF8(t *testing.T) {
	p, _ := NewTextParser("")
	bad := []byte{0xff, 0xfe, 0xfd}
	res := p.ParseWithResult("bad.txt", bad)
	if res.Err == nil {
		t.Fatal("want error for invalid UTF-8, got nil")
	}
}

// TestHTMLParser_ParseWithResult_BlockSplit pins the HTML walker.
// Three block elements (heading, paragraph, list) yield three
// items with the python-compatible ck_type vocabulary.
func TestHTMLParser_ParseWithResult_BlockSplit(t *testing.T) {
	p, err := NewHTMLParser(Official)
	if err != nil {
		t.Fatalf("NewHTMLParser: %v", err)
	}
	src := []byte(`<!DOCTYPE html><html><body>
<h1>Title</h1>
<p>First paragraph.</p>
<ul><li>Item one</li></ul>
</body></html>`)
	res := p.ParseWithResult("doc.html", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", res.OutputFormat)
	}
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3 (h1, p, ul)", len(res.JSON))
	}
	if got, want := res.JSON[0]["ck_type"], "heading"; got != want {
		t.Errorf("JSON[0].ck_type = %v, want %v", got, want)
	}
	if got, want := res.JSON[0]["text"], "Title"; got != want {
		t.Errorf("JSON[0].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[1]["ck_type"], "paragraph"; got != want {
		t.Errorf("JSON[1].ck_type = %v, want %v", got, want)
	}
	if got, want := res.JSON[1]["text"], "First paragraph."; got != want {
		t.Errorf("JSON[1].text = %v, want %v", got, want)
	}
	if got, want := res.JSON[2]["ck_type"], "list"; got != want {
		t.Errorf("JSON[2].ck_type = %v, want %v", got, want)
	}
	if got, want := res.JSON[2]["text"], "Item one"; got != want {
		t.Errorf("JSON[2].text = %v, want %v", got, want)
	}
}

// TestHTMLParser_ParseWithResult_SkipsScriptAndStyle pins the
// rule that <script> / <style> subtrees are skipped entirely so
// they don't pollute the downstream chunker input.
func TestHTMLParser_ParseWithResult_SkipsScriptAndStyle(t *testing.T) {
	p, _ := NewHTMLParser(Official)
	src := []byte(`<html><body>
<p>Visible.</p>
<script>alert("x")</script>
<style>body { color: red; }</style>
<p>Also visible.</p>
</body></html>`)
	res := p.ParseWithResult("doc.html", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 2 {
		t.Errorf("JSON len = %d, want 2 (script+style skipped)", len(res.JSON))
	}
	for _, it := range res.JSON {
		if txt, _ := it["text"].(string); strings.Contains(txt, "alert") || strings.Contains(txt, "color") {
			t.Errorf("item text leaks script/style content: %q", txt)
		}
	}
}

// TestGetParser_RoutesTextAndCode pins the parser-type switch
// routing for the text&code family. After the Slice 1 additions
// `utility.FileTypeTXT` resolves to a TextParser that satisfies
// ParseResultProducer.
func TestGetParser_RoutesTextAndCode(t *testing.T) {
	p, err := GetParser(utility.FileTypeTXT, map[string]string{"lib_type": ""})
	if err != nil {
		t.Fatalf("GetParser(FileTypeTXT): %v", err)
	}
	if _, ok := p.(ParseResultProducer); !ok {
		t.Fatal("TextParser does not implement ParseResultProducer")
	}
}
