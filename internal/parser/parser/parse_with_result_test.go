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
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/simplifiedchinese"

	"ragflow/internal/utility"
)

// TestTextParser_ParseWithResult_ParaSplit pins the paragraph-split
// rule. A blank-line-separated input yields one item per
// paragraph; the python TxtParser does the same.
func TestTextParser_ParseWithResult_ParaSplit(t *testing.T) {
	p := NewTextParser()
	src := []byte("First paragraph.\n\nSecond paragraph.\n\nThird.")
	ctx := t.Context()
	res := p.ParseWithResult(ctx, "doc.txt", src)
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
	ctx := t.Context()
	p := NewTextParser()
	res := p.ParseWithResult(ctx, "empty.txt", []byte{})
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Errorf("JSON len = %d, want 1 (empty-input fallback)", len(res.JSON))
	}
}

// TestTextParser_ParseWithResult_NoSizeCap pins that the parser performs no
// per-item byte slicing: a single continuous run longer than any prior cap
// (here 9000 'a's with no delimiter) stays as one item whose full content is
// preserved. Sizing is delegated to the chunker / embedding truncation, matching
// python's parser_txt (which also does no size slicing).
func TestTextParser_ParseWithResult_NoSizeCap(t *testing.T) {
	ctx := t.Context()
	p := NewTextParser()
	long := strings.Repeat("a", 9000)
	res := p.ParseWithResult(ctx, "long.txt", []byte(long))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("JSON len = %d, want 1 (no per-item size cap)", len(res.JSON))
	}
	if txt, _ := res.JSON[0]["text"].(string); txt != long {
		t.Errorf("text len = %d, want %d (full content preserved, not sliced)", len(txt), len(long))
	}
}

// TestTextParser_ParseWithResult_GBK pins that non-UTF-8 (e.g. GBK) text input
// is decoded to valid UTF-8 rather than returning an error.
func TestTextParser_ParseWithResult_GBK(t *testing.T) {
	ctx := t.Context()
	p := NewTextParser()
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("测试中文文本内容"))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}
	res := p.ParseWithResult(ctx, "chinese_gbk.txt", gbkBytes)
	if res.Err != nil {
		t.Fatalf("ParseWithResult(GBK): unexpected error %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("want non-empty items")
	}
	got, _ := res.JSON[0]["text"].(string)
	if got != "测试中文文本内容" {
		t.Errorf("got %q, want %q", got, "测试中文文本内容")
	}
	if enc, _ := res.File["encoding"].(string); enc != "gb18030" && enc != "gbk" {
		t.Errorf("File.encoding = %q, want gb18030 or gbk", enc)
	}
}

// TestHTMLParser_ParseWithResult_BlockSplit pins the HTML walker.
// Three block elements (heading, paragraph, list) yield three
// items with the python-compatible ck_type vocabulary.
func TestHTMLParser_ParseWithResult_BlockSplit(t *testing.T) {
	ctx := t.Context()
	p := NewHTMLParser()
	src := []byte(`<!DOCTYPE html><html><body>
<h1>Title</h1>
<p>First paragraph.</p>
<ul><li>Item one</li></ul>
</body></html>`)
	res := p.ParseWithResult(ctx, "doc.html", src)
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

func TestHTMLParser_ParseWithResult_PreservesLooseText(t *testing.T) {
	ctx := t.Context()
	p := NewHTMLParser()
	src := []byte(`<!DOCTYPE html><html><head>
<title>Head metadata</title>
</head><body>
Intro text
<h1>Title</h1>
Between blocks
<p>Body <span>inline</span>.<noscript>Inline fallback</noscript></p>
<script>alert("x")</script>
<style>body { color: red; }</style>
<noscript>Fallback text</noscript>
Tail text
</body></html>`)
	res := p.ParseWithResult(ctx, "doc.html", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	want := []struct {
		text   string
		ckType string
	}{
		{"Intro text", "text"},
		{"Title", "heading"},
		{"Between blocks", "text"},
		{"Body inline.", "paragraph"},
		{"Tail text", "text"},
	}
	if len(res.JSON) != len(want) {
		t.Fatalf("JSON len = %d, want %d: %#v", len(res.JSON), len(want), res.JSON)
	}
	for i, w := range want {
		if got := res.JSON[i]["text"]; got != w.text {
			t.Errorf("JSON[%d].text = %v, want %v", i, got, w.text)
		}
		if got := res.JSON[i]["doc_type_kwd"]; got != "text" {
			t.Errorf("JSON[%d].doc_type_kwd = %v, want text", i, got)
		}
		if got := res.JSON[i]["ck_type"]; got != w.ckType {
			t.Errorf("JSON[%d].ck_type = %v, want %v", i, got, w.ckType)
		}
	}
}

func TestHTMLParser_ParseWithResult_PreservesLooseTextWithoutExplicitBody(t *testing.T) {
	ctx := t.Context()
	p := NewHTMLParser()
	src := []byte(`<!DOCTYPE html>
Intro text
<h1>Title</h1>
Tail text`)
	res := p.ParseWithResult(ctx, "doc.html", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	want := []string{"Intro text", "Title", "Tail text"}
	if len(res.JSON) != len(want) {
		t.Fatalf("JSON len = %d, want %d: %#v", len(res.JSON), len(want), res.JSON)
	}
	for i, text := range want {
		if got := res.JSON[i]["text"]; got != text {
			t.Errorf("JSON[%d].text = %v, want %v", i, got, text)
		}
	}
}

func TestWalkHTMLBlocks_SkipsHeadLooseText(t *testing.T) {
	head := &html.Node{Type: html.ElementNode, Data: "head"}
	head.AppendChild(&html.Node{Type: html.TextNode, Data: "Head metadata"})

	var items []map[string]any
	walkHTMLBlocks(head, &items)
	if len(items) != 0 {
		t.Fatalf("JSON len = %d, want 0: %#v", len(items), items)
	}
}

// TestHTMLParser_ParseWithResult_SkipsScriptAndStyle pins the
// rule that <script> / <style> / <noscript> subtrees are skipped
// entirely so they don't pollute the downstream chunker input.
func TestHTMLParser_ParseWithResult_SkipsScriptAndStyle(t *testing.T) {
	ctx := t.Context()
	p := NewHTMLParser()
	src := []byte(`<html><body>
<p>Visible.</p>
<script>alert("x")</script>
<style>body { color: red; }</style>
<p>Also <script>inline alert</script><style>.inline { color: blue; }</style><noscript>fallback</noscript> visible.</p>
<p>Also visible.</p>
</body></html>`)
	res := p.ParseWithResult(ctx, "doc.html", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 3 {
		t.Errorf("JSON len = %d, want 3 (script+style+noscript skipped)", len(res.JSON))
	}
	for _, it := range res.JSON {
		if txt, _ := it["text"].(string); strings.Contains(txt, "alert") ||
			strings.Contains(txt, "color") ||
			strings.Contains(txt, "inline alert") ||
			strings.Contains(txt, "blue") ||
			strings.Contains(txt, "fallback") {
			t.Errorf("item text leaks script/style content: %q", txt)
		}
	}
}

// TestGetParser_RoutesTextAndCode pins the parser-type switch
// routing for the text&code family. After the Slice 1 additions
// `utility.FileTypeTXT` resolves to a TextParser that satisfies
// ParseResultProducer.
func TestGetParser_RoutesTextAndCode(t *testing.T) {
	p, err := GetParser(utility.FileTypeTXT)
	if err != nil {
		t.Fatalf("GetParser(FileTypeTXT): %v", err)
	}
	if _, ok := p.(ParseResultProducer); !ok {
		t.Fatal("TextParser does not implement ParseResultProducer")
	}
}

// TestTextParser_ParseWithResult_DefaultDelimiter pins the alignment fix:
// TextParser now splits on the flow parser's default delimiter set
// ("\n!?;。；！？"), mirroring deepdoc TxtParser.parser_txt, instead of only on
// blank lines. keep_delimiters=True (the flow _code path) keeps each trailing
// delimiter attached, so sentence-ending punctuation survives the split.
func TestTextParser_ParseWithResult_DefaultDelimiter(t *testing.T) {
	ctx := t.Context()
	p := NewTextParser()

	// Single newlines now split too (previously only "\n\n" did).
	src := []byte("First line.\nSecond line.\nThird line.")
	res := p.ParseWithResult(ctx, "doc.txt", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 3 {
		t.Fatalf("JSON len = %d, want 3 (single-newline split)", len(res.JSON))
	}

	// Sentence delimiters split and keep the delimiter attached. The period
	// "." is NOT in the default set, so "Foo. Bar" stays joined until the ";".
	// TrimSpace drops the incidental leading space before each delimiter (the
	// package's established convention, also used by markdown leafText).
	src = []byte("Hello! World? Foo. Bar; Baz。 Qux！")
	res = p.ParseWithResult(ctx, "doc.txt", src)
	want := []string{"Hello!", "World?", "Foo. Bar;", "Baz。", "Qux！"}
	if len(res.JSON) != len(want) {
		t.Fatalf("JSON len = %d, want %d: %#v", len(res.JSON), len(want), res.JSON)
	}
	for i, w := range want {
		if got := res.JSON[i]["text"]; got != w {
			t.Errorf("JSON[%d].text = %v, want %v", i, got, w)
		}
	}

	// Chinese sentence delimiters split the same way.
	src = []byte("这是第一句。这是第二句！第三句？结尾。")
	res = p.ParseWithResult(ctx, "doc.txt", src)
	if len(res.JSON) != 4 {
		t.Fatalf("JSON len = %d, want 4 (CJK delimiter split)", len(res.JSON))
	}
}

// TestTextParser_ParseWithResult_NewlineNormalization pins the
// normalizeTextNewlines contract: CRLF ("\r\n") and lone-CR ("\r") line
// endings fold to LF before splitting, so every variant of the same logical
// content yields identical items. This mirrors rag/nlp/delim.
// normalize_text_newlines, which is what Python splits on, so Windows-line
// documents parse identically to Unix ones.
func TestTextParser_ParseWithResult_NewlineNormalization(t *testing.T) {
	ctx := t.Context()
	p := NewTextParser()

	// Same logical content expressed with LF, CRLF, and lone-CR line endings.
	lf := "First line.\nSecond line! Third? Fourth."
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")
	cr := strings.ReplaceAll(lf, "\n", "\r")

	extract := func(src string) []string {
		res := p.ParseWithResult(ctx, "doc.txt", []byte(src))
		if res.Err != nil {
			t.Fatalf("ParseWithResult: %v", res.Err)
		}
		out := make([]string, 0, len(res.JSON))
		for _, it := range res.JSON {
			if txt, _ := it["text"].(string); txt != "" {
				out = append(out, txt)
			}
		}
		return out
	}

	want := extract(lf)
	if len(want) == 0 {
		t.Fatal("LF baseline produced no items")
	}
	for _, variant := range []struct {
		name string
		src  string
	}{
		{"crlf", crlf},
		{"cr", cr},
	} {
		got := extract(variant.src)
		if len(got) != len(want) {
			t.Fatalf("%s: JSON len = %d, want %d: %#v", variant.name, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s: JSON[%d].text = %q, want %q", variant.name, i, got[i], want[i])
			}
		}
	}
}

// TestTextParser_AlignmentGolden verifies Go's ParseWithResult output is
// content-equivalent to Python's _code on the shared sample, using the shared
// concatenation-normalization alignment tool (align_test.go). Python applies
// the OVER_CAP token merge (chunking ownership retained by the Go Chunker per
// contract #17799), so item counts differ; the
// comparison normalizes both (delimiters stripped, whitespace collapsed) and
// joins on whitespace, so only CONTENT equivalence — not byte-exact layout — is
// checked. The golden files are a NORMALIZED content baseline, not a verbatim
// Python transcript: a fresh _code run at chunk_token_num=128 may merge into a
// different item count/structure (e.g. the en sample collapses to one chunk while
// the golden keeps prose and code as two items) and may collapse inter-sentence
// newlines, so do not treat them as byte-exact.
//
// No generator script is committed. The baseline meta records generator, sample,
// delimiter, keep_delimiters and chunk_token_num (see textcode.python.en/zh.golden.json);
// sample, delimiter, keep_delimiters, chunk_token_num): call the python flow
// _code on the sample with keep_delimiters=True and the default delimiter set,
// then project each merged section to {"text": section[0], "doc_type_kwd": "text"}.
func TestTextParser_AlignmentGolden(t *testing.T) {
	ctx := t.Context()
	p := NewTextParser()

	cases := []struct {
		name   string
		sample string
		golden string
	}{
		{"en", "testdata/textcode.sample.en.txt", "testdata/textcode.python.en.golden.json"},
		{"zh", "testdata/textcode.sample.zh.txt", "testdata/textcode.python.zh.golden.json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sample, err := os.ReadFile(tc.sample)
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}
			res := p.ParseWithResult(ctx, tc.sample, sample)
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}

			gd := LoadGoldenDoc(t, tc.golden)
			ignore := AcceptedDivergences(gd.Meta)

			goText := FilterOutDocTypes(FilterByDocType(res.JSON, "text"), ignore)
			pyText := FilterOutDocTypes(FilterByDocType(gd.Items, "text"), ignore)

			if ok, diff := CompareAlignment(goText, pyText, TextCodeAlignOptions(DefaultTextCodeDelimiter)); !ok {
				t.Fatalf("text&code parser not aligned with Python golden:%s", diff)
			}
		})
	}
}

// TestTextParser_AdjacentDelimiters pins Go's behavior on adjacent
// delimiters, confirming it matches Python's deepdoc TxtParser.parser_txt
// delimiter-loop exactly (not a divergence). Both ports run the same
// re.split(r"(%s)" % dels, txt) loop with keep_delimiters=True and merge a
// run of adjacent delimiters into the preceding segment, so the standalone
// second delimiter is dropped on both sides: "a!!b" → ["a!", "b"] (verified
// against deepdoc/parser/txt_parser.py). The alignment test's delimiter-strip
// normalization also reconciles this, but this test guards splitCapturingDelims
// directly so a future silent change there is caught independently.
func TestTextParser_AdjacentDelimiters(t *testing.T) {
	ctx := t.Context()
	p := NewTextParser()

	// Two adjacent sentence delimiters: Go (and Python's parser_txt) merge
	// them into the preceding segment and drop the standalone second delimiter.
	src := []byte("a!!b")
	res := p.ParseWithResult(ctx, "doc.txt", src)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	want := []string{"a!", "b"}
	if len(res.JSON) != len(want) {
		t.Fatalf("adjacent delimiters: JSON len = %d, want %d: %#v", len(res.JSON), len(want), res.JSON)
	}
	for i, w := range want {
		if got := res.JSON[i]["text"]; got != w {
			t.Errorf("adjacent delimiters: JSON[%d].text = %v, want %v", i, got, w)
		}
	}

	// Delimiters separated by text each attach to their own segment (no merge
	// across the gap).
	src = []byte("x?y!z")
	res = p.ParseWithResult(ctx, "doc.txt", src)
	want = []string{"x?", "y!", "z"}
	if len(res.JSON) != len(want) {
		t.Fatalf("mixed delimiters: JSON len = %d, want %d: %#v", len(res.JSON), len(want), res.JSON)
	}
	for i, w := range want {
		if got := res.JSON[i]["text"]; got != w {
			t.Errorf("mixed delimiters: JSON[%d].text = %v, want %v", i, got, w)
		}
	}
}
