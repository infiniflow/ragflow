package parser

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestHTMLParser_TableProducesStructuredItems asserts the HTML table
// alignment fix (approach a, mirroring markdown_parser.go:305-328 and
// :366-367): a <table> must NOT be flattened into a single text blob. The
// parser must emit two items:
//
//  1. an inlined copy in the text flow (doc_type_kwd:"text") whose text keeps
//     the <table> markup, so embedding/retrieval/LLM rendering preserve the
//     row/column structure (no "collapse");
//  2. a structured table item (doc_type_kwd:"table", ck_type:"table") appended
//     after the walk, consumed by the downstream chunker (attachMediaContext).
//
// Before the fix, walkHTMLBlocks ran htmlLeafText over the <table>, producing
// ONE flat item (e.g. "Name Age Alice 30") tagged ck_type:"table" but with no
// markup — this test fails on that behavior.
func TestHTMLParser_TableProducesStructuredItems(t *testing.T) {
	const html = `<html><body>
<h1>Employee Table</h1>
<table>
<tr><th>Name</th><th>Age</th></tr>
<tr><td>Alice</td><td>30</td></tr>
</table>
<p>Trailing paragraph</p>
</body></html>`

	p := NewHTMLParser()
	res := p.ParseWithResult(context.Background(), "doc.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	var inlinedText, structuredText string
	inlinedIdx, structuredIdx, trailingIdx := -1, -1, -1
	for i, it := range res.JSON {
		text, _ := it["text"].(string)
		switch it["doc_type_kwd"] {
		case "text":
			if strings.Contains(text, "<table") {
				inlinedText = text
				inlinedIdx = i
			}
			if text == "Trailing paragraph" {
				trailingIdx = i
			}
		case "table":
			structuredText = text
			structuredIdx = i
		}
	}

	if inlinedIdx < 0 {
		t.Fatalf("no inlined doc_type_kwd:\"text\" item contains <table> markup; got items: %#v", res.JSON)
	}
	if !strings.Contains(inlinedText, "<table") ||
		!strings.Contains(inlinedText, "Name") ||
		!strings.Contains(inlinedText, "Alice") {
		t.Errorf("inlined table copy missing markup/cells: %q", inlinedText)
	}

	if structuredIdx < 0 {
		t.Fatalf("no structured doc_type_kwd:\"table\" item emitted; got items: %#v", res.JSON)
	}
	if got, want := res.JSON[structuredIdx]["ck_type"], "table"; got != want {
		t.Errorf("structured table item ck_type = %v, want %v", got, want)
	}
	if structuredText != inlinedText {
		t.Errorf("structured table text differs from inlined copy:\n inlined=%q\n struct =%q", inlinedText, structuredText)
	}

	// The structured table item must be appended after the walk, i.e. after
	// the in-walk text items (mirrors markdown_parser.go:366-367). The trailing
	// <p> is an in-walk text item, so the structured item must come after it.
	if trailingIdx < 0 {
		t.Fatalf("trailing paragraph text item missing; got items: %#v", res.JSON)
	}
	if structuredIdx <= trailingIdx {
		t.Errorf("structured table item at index %d must come after the trailing paragraph at index %d", structuredIdx, trailingIdx)
	}
}

// TestHTMLParser_NestedTableProducesStructuredItems asserts the table fix also
// covers tables that are NOT direct children of <body> — i.e. a <table> wrapped
// in a layout container such as <div>/<section>/<article> (the common real-world
// case). walkHTMLBlocks only special-cases a <table> that is a direct child of
// the walk root; a nested <table> is reached via the default-branch leaf-text
// extractor (htmlLeafText → walkHTMLLeaf). Before the fix, that path ran
// htmlLeafText over the wrapper, flattening the table into a single text blob
// ("Name Age Alice 30") with no markup — the same collapse as the top-level case,
// just hidden one level deeper. After the fix, walkHTMLLeaf must render the
// <table> markup inline (so it survives inside the wrapper's single text item)
// AND register a structured doc_type_kwd:"table"/ck_type:"table" item (appended
// after the walk like the top-level path).
func TestHTMLParser_NestedTableProducesStructuredItems(t *testing.T) {
	const html = `<html><body>
<h1>Heading</h1>
<div class="content">
<table>
<tr><th>Name</th><th>Age</th></tr>
<tr><td>Alice</td><td>30</td></tr>
</table>
</div>
<p>Trailing paragraph</p>
</body></html>`

	p := NewHTMLParser()
	res := p.ParseWithResult(context.Background(), "nested.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	var inlinedText, structuredText string
	inlinedIdx, structuredIdx, trailingIdx := -1, -1, -1
	for i, it := range res.JSON {
		text, _ := it["text"].(string)
		switch it["doc_type_kwd"] {
		case "text":
			if strings.Contains(text, "<table") {
				inlinedText = text
				inlinedIdx = i
			}
			if text == "Trailing paragraph" {
				trailingIdx = i
			}
		case "table":
			structuredText = text
			structuredIdx = i
		}
	}

	if inlinedIdx < 0 {
		t.Fatalf("no inlined doc_type_kwd:\"text\" item contains <table> markup (nested table flattened?); got items: %#v", res.JSON)
	}
	if !strings.Contains(inlinedText, "<table") ||
		!strings.Contains(inlinedText, "Name") ||
		!strings.Contains(inlinedText, "Alice") {
		t.Errorf("inlined nested table copy missing markup/cells: %q", inlinedText)
	}

	if structuredIdx < 0 {
		t.Fatalf("no structured doc_type_kwd:\"table\" item emitted for nested table; got items: %#v", res.JSON)
	}
	if got, want := res.JSON[structuredIdx]["ck_type"], "table"; got != want {
		t.Errorf("structured nested table item ck_type = %v, want %v", got, want)
	}
	if !strings.Contains(structuredText, "<table") ||
		!strings.Contains(structuredText, "Name") ||
		!strings.Contains(structuredText, "Alice") {
		t.Errorf("structured nested table item missing markup/cells: %q", structuredText)
	}

	// The structured table item must be appended after the walk, i.e. after
	// every in-walk text item (mirrors the top-level path and markdown).
	if trailingIdx < 0 {
		t.Fatalf("trailing paragraph text item missing; got items: %#v", res.JSON)
	}
	if structuredIdx <= inlinedIdx || structuredIdx <= trailingIdx {
		t.Errorf("structured nested table item at index %d must come after the inlined text item at %d and trailing paragraph at %d", structuredIdx, inlinedIdx, trailingIdx)
	}
}

// TestHTMLParser_NestedListItemsPreserved verifies a nested <ul> (a list
// containing a sublist) does NOT lose any item text — i.e. it is NOT subject to
// the same kind of collapse the table path once had. Unlike tables (whose markup is
// dropped and cells can fuse), list items are separated by hard breaks inside
// walkHTMLLeaf, so every <li> text survives in document order. This mirrors
// Python's RAGFlowHtmlParser: read_text_recursively (html_parser.py:108-160)
// assigns each block its own block_id and emits flat, depth-less records for
// nested <li>s — so BOTH sides preserve item content and NEITHER represents
// nesting depth. The accepted divergence is granularity: Python emits one
// record per <li> while Go emits the whole <ul> as a single text item (its
// "one item per block-level element" model); the primary alignment gate is
// content equivalence (PARSER_ALIGNMENT_HANDOFF.md §2.3), which this test
// guards. A regression here would be Go dropping or fusing list items.
func TestHTMLParser_NestedListItemsPreserved(t *testing.T) {
	const html = `<html><body>
<ul>
  <li>Item 1</li>
  <li>Item 2
    <ul>
      <li>Sub A</li>
      <li>Sub B</li>
    </ul>
  </li>
</ul>
</body></html>`

	p := NewHTMLParser()
	res := p.ParseWithResult(context.Background(), "nested-list.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	// Join every item's text in order to check content + order + fusion.
	var all string
	for _, it := range res.JSON {
		if txt, _ := it["text"].(string); txt != "" {
			all += txt + "\n"
		}
	}

	for _, want := range []string{"Item 1", "Item 2", "Sub A", "Sub B"} {
		if !strings.Contains(all, want) {
			t.Errorf("nested list lost item %q; joined output: %q", want, all)
		}
	}

	// Document order must survive: the sublist items must follow their parent.
	i2 := strings.Index(all, "Item 2")
	ia := strings.Index(all, "Sub A")
	ib := strings.Index(all, "Sub B")
	if i2 < 0 || ia < 0 || ib < 0 || !(ia > i2 && ib > i2) {
		t.Errorf("nested list order not preserved (parent must precede sub-items): %q", all)
	}

	// No collapse: items must not be fused without a separator
	// (e.g. "Item 2" directly adjacent to "Sub A" with no break).
	if strings.Contains(all, "Item 2Sub") || strings.Contains(all, "Sub ASub") {
		t.Errorf("nested list items fused together (collapse): %q", all)
	}

	// The whole top-level <ul> is emitted as ONE text item tagged
	// ck_type:"list" (Go's "one item per block-level element" model). Guard
	// against a regression that would drop the list tag or split it
	// unexpectedly. Note: a list nested inside a non-list container
	// (div/section/…) is folded into that container's single text item and
	// therefore loses ck_type:"list" (becomes "text") — a fidelity nuance,
	// not a collapse, since downstream chunking reads doc_type_kwd not ck_type.
	var listText string
	for _, it := range res.JSON {
		if it["ck_type"] == "list" {
			listText, _ = it["text"].(string)
			break
		}
	}
	if listText == "" {
		t.Fatalf("top-level <ul> not tagged ck_type:\"list\"; got items: %#v", res.JSON)
	}
	for _, want := range []string{"Item 1", "Item 2", "Sub A", "Sub B"} {
		if !strings.Contains(listText, want) {
			t.Errorf("ck_type:\"list\" item missing %q: %q", want, listText)
		}
	}
}

// TestHTMLParser_PrePreservesVerbatim guards that <pre> (and <textarea>) blocks
// are emitted verbatim — indentation and internal newlines must survive, NOT be
// CSS-collapsed into single spaces. Code is high-value content: if the verbatim
// path regressed to the folded leaf-text path, indented code would lose its
// structure and the LLM / user would see broken code. This mirrors Python's
// RAGFlowHtmlParser, which keeps pre/textarea verbatim (_PRE_TAGS in
// deepdoc/parser/html_parser.py). Both blocks keep doc_type_kwd:"text"; <pre>
// is tagged ck_type:"code" while <textarea> falls back to ck_type:"text"
// (htmlTagToCkType has no textarea case) — a labeling nuance, not a content
// divergence. Note: the single leading newline immediately after <pre>/<textarea>
// is dropped by the HTML parser per spec (pre/textarea eat one leading newline);
// that is correct browser behavior, not folding. What must survive is the
// indentation and the internal newlines.
func TestHTMLParser_PrePreservesVerbatim(t *testing.T) {
	const html = `<html><body>
<pre>
def foo():
    if x:
        return 1
    return 0
</pre>
<textarea>
SELECT * FROM t
  WHERE a = 1
</textarea>
</body></html>`

	p := NewHTMLParser()
	res := p.ParseWithResult(context.Background(), "pre.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	var preItem, textareaItem map[string]any
	for _, it := range res.JSON {
		txt, _ := it["text"].(string)
		switch {
		case strings.Contains(txt, "def foo()"):
			preItem = it
		case strings.Contains(txt, "SELECT * FROM t"):
			textareaItem = it
		}
	}

	if preItem == nil {
		t.Fatalf("no <pre> item found; got items: %#v", res.JSON)
	}
	if preItem["ck_type"] != "code" {
		t.Errorf("<pre> item ck_type = %v, want code", preItem["ck_type"])
	}
	preText, _ := preItem["text"].(string)
	// 4-space and 8-space indents must survive verbatim (a folded path would
	// collapse them to a single space).
	if !strings.Contains(preText, "    if x:") {
		t.Errorf("<pre> lost 4-space indent (folded?): %q", preText)
	}
	if !strings.Contains(preText, "        return 1") {
		t.Errorf("<pre> lost 8-space indent (folded?): %q", preText)
	}
	// Internal newlines must survive (line structure preserved).
	if !strings.Contains(preText, "def foo():\n") {
		t.Errorf("<pre> lost internal newline (folded?): %q", preText)
	}

	if textareaItem == nil {
		t.Fatalf("no <textarea> item found; got items: %#v", res.JSON)
	}
	taText, _ := textareaItem["text"].(string)
	// 2-space indent must survive verbatim.
	if !strings.Contains(taText, "  WHERE a = 1") {
		t.Errorf("<textarea> lost 2-space indent (folded?): %q", taText)
	}
}

// TestHTMLParser_AlignmentGolden verifies Go's ParseWithResult output is
// content-equivalent to Python's RAGFlowHtmlParser on the shared sample, using
// the shared concatenation-normalization alignment tool (align_test.go).
// Python keeps raw HTML and splits on block elements (deepdoc/parser/html_parser.py:
// read_text_recursively + merge_block_text); Go emits clean per-block text.
// The comparison normalizes both (HTML heading markers "#{1,6}", HTML tags,
// whitespace collapsed) and ignores "table" items, which are accepted
// representation differences (the <table> markup stays inline as a "text"
// item in both, see TestHTMLParser_TableProducesStructuredItems).
//
// Regenerate the baseline with:
//
//	.venv/bin/python internal/parser/parser/testdata/gen_html_golden.py
func TestHTMLParser_AlignmentGolden(t *testing.T) {
	ctx := t.Context()
	p := NewHTMLParser()

	sample, err := os.ReadFile("testdata/html.sample.html")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	res := p.ParseWithResult(ctx, "html.sample.html", sample)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	golden := LoadGolden(t, "testdata/html.python.golden.json")

	// Ignore "table" items on both sides (accepted divergence: Go appends a
	// structured table item; Python folds the inline <table> into the text
	// flow). The inline <table> markup itself remains a "text" item in both
	// runs so it is still compared — that is exactly what guards against collapse.
	goText := FilterByDocType(res.JSON, "text")
	pyText := FilterByDocType(golden, "text")

	if ok, diff := CompareAlignment(goText, pyText, HTMLAlignOptions()); !ok {
		t.Fatalf("html parser not aligned with Python golden:%s", diff)
	}
}
