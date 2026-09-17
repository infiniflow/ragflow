package parser

import (
	"context"
	"os"
	"strings"
	"testing"
)

// TestHTMLParser_TableProducesStructuredItems asserts the HTML table contract:
// a <table> is emitted as exactly ONE structured doc_type_kwd:"table"/
// ck_type:"table" item, in its original document position (not relocated to
// the end), with NO duplicate doc_type_kwd:"text" copy carrying the <table>
// markup. Keeping the full <table>…</table> markup (not flattened) preserves
// row/column structure for embedding/retrieval/LLM rendering, and the single
// structured item is what the downstream chunker keeps as a discrete,
// independently retrievable chunk.
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

	var tableText string
	tableIdx, headingIdx, trailingIdx, inlineTableCount, tableCount := -1, -1, -1, 0, 0
	for i, it := range res.JSON {
		text, _ := it["text"].(string)
		switch it["doc_type_kwd"] {
		case "text":
			if strings.Contains(text, "<table") {
				inlineTableCount++
			}
			if text == "Employee Table" {
				headingIdx = i
			}
			if text == "Trailing paragraph" {
				trailingIdx = i
			}
		case "table":
			tableText = text
			tableIdx = i
			tableCount++
		default:
			t.Fatalf("unexpected doc_type_kwd %q", it["doc_type_kwd"])
		}
	}

	if tableIdx < 0 {
		t.Fatalf("no structured doc_type_kwd:\"table\" item emitted; got items: %#v", res.JSON)
	}
	if tableCount != 1 {
		t.Fatalf("structured doc_type_kwd:\"table\" item count = %d, want exactly 1", tableCount)
	}
	if got, want := res.JSON[tableIdx]["ck_type"], "table"; got != want {
		t.Errorf("structured table item ck_type = %v, want %v", got, want)
	}
	if !strings.Contains(tableText, "<table") ||
		!strings.Contains(tableText, "Name") ||
		!strings.Contains(tableText, "Alice") {
		t.Errorf("structured table text missing markup/cells: %q", tableText)
	}
	// No duplicate inline text copy of the table markup.
	if inlineTableCount != 0 {
		t.Errorf("found %d doc_type_kwd:\"text\" item(s) containing <table> markup; the table must not be duplicated as text", inlineTableCount)
	}
	// In document order: between the heading and the trailing paragraph,
	// NOT relocated after the trailing paragraph.
	if headingIdx < 0 {
		t.Fatalf("heading item missing")
	}
	if trailingIdx < 0 {
		t.Fatalf("trailing paragraph item missing")
	}
	if tableIdx <= headingIdx {
		t.Errorf("structured table item at %d must come after heading at %d", tableIdx, headingIdx)
	}
	if tableIdx >= trailingIdx {
		t.Errorf("structured table item at %d must come before trailing paragraph at %d (not relocated to end)", tableIdx, trailingIdx)
	}
}

// TestHTMLParser_NestedTableProducesStructuredItems asserts the contract also
// covers tables wrapped in a layout container such as <div>/<section>/<article>
// (the common real-world case). walkHTMLBlocks only special-cases a <table> that
// is a direct child of <body>; a nested <table> is reached via htmlLeafText →
// walkHTMLLeaf. The nested <table> must be emitted as exactly ONE structured
// doc_type_kwd:"table" item in document order, with NO duplicate inline text
// copy — the parent container's prose stays clean of raw <table> tags.
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

	var tableText string
	tableIdx, headingIdx, trailingIdx, inlineTableCount, tableCount := -1, -1, -1, 0, 0
	for i, it := range res.JSON {
		text, _ := it["text"].(string)
		switch it["doc_type_kwd"] {
		case "text":
			if strings.Contains(text, "<table") {
				inlineTableCount++
			}
			if text == "Heading" {
				headingIdx = i
			}
			if text == "Trailing paragraph" {
				trailingIdx = i
			}
		case "table":
			tableText = text
			tableIdx = i
			tableCount++
		default:
			t.Fatalf("unexpected doc_type_kwd %q", it["doc_type_kwd"])
		}
	}

	if tableIdx < 0 {
		t.Fatalf("no structured doc_type_kwd:\"table\" item emitted for nested table; got items: %#v", res.JSON)
	}
	if tableCount != 1 {
		t.Fatalf("structured doc_type_kwd:\"table\" item count = %d, want exactly 1", tableCount)
	}
	if got, want := res.JSON[tableIdx]["ck_type"], "table"; got != want {
		t.Errorf("structured nested table item ck_type = %v, want %v", got, want)
	}
	if !strings.Contains(tableText, "<table") ||
		!strings.Contains(tableText, "Name") ||
		!strings.Contains(tableText, "Alice") {
		t.Errorf("structured nested table item missing markup/cells: %q", tableText)
	}
	// No duplicate inline text copy of the table markup.
	if inlineTableCount != 0 {
		t.Errorf("found %d doc_type_kwd:\"text\" item(s) containing <table> markup; nested table must not be duplicated as inline text", inlineTableCount)
	}
	// In document order: between the heading and the trailing paragraph,
	// NOT relocated after the trailing paragraph.
	if headingIdx < 0 {
		t.Fatalf("heading item missing")
	}
	if trailingIdx < 0 {
		t.Fatalf("trailing paragraph item missing")
	}
	if tableIdx <= headingIdx {
		t.Errorf("structured nested table item at %d must come after heading at %d", tableIdx, headingIdx)
	}
	if tableIdx >= trailingIdx {
		t.Errorf("structured nested table item at %d must come before trailing paragraph at %d (not relocated to end)", tableIdx, trailingIdx)
	}
}

// TestHTMLParser_NestedTableWithSurroundingText guards the structural contract
// for a <table> nested inside a container that also carries prose before AND
// after the table (the common real-world shape, e.g.
// "<div><p>Before.</p><table>…</table><p>After.</p></div>"). The table must be
// emitted as ONE structured doc_type_kwd:"table" item, and the surrounding prose
// must be split into SEPARATE clean text items bracketing the table in document
// order — NOT merged into a single blob that embeds the raw <table> markup.
// This locks in the fix that removed the inline writeText(markup) of the table
// into the parent's text flow (the old code would produce one text item
// "Before <table>…</table> After").
func TestHTMLParser_NestedTableWithSurroundingText(t *testing.T) {
	const html = `<html><body>
<div class="box">
<p>Before the table.</p>
<table>
<tr><th>Name</th><th>Age</th></tr>
<tr><td>Alice</td><td>30</td></tr>
</table>
<p>After the table.</p>
</div>
</body></html>`

	p := NewHTMLParser()
	res := p.ParseWithResult(context.Background(), "nested.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	var tableText string
	tableIdx, beforeIdx, afterIdx, inlineTableCount, tableCount := -1, -1, -1, 0, 0
	for i, it := range res.JSON {
		text, _ := it["text"].(string)
		switch it["doc_type_kwd"] {
		case "text":
			if strings.Contains(text, "<table") {
				inlineTableCount++
			}
			if text == "Before the table." {
				beforeIdx = i
			}
			if text == "After the table." {
				afterIdx = i
			}
		case "table":
			tableText = text
			tableIdx = i
			tableCount++
		default:
			t.Fatalf("unexpected doc_type_kwd %q", it["doc_type_kwd"])
		}
	}

	if tableIdx < 0 {
		t.Fatalf("no structured doc_type_kwd:\"table\" item emitted; got items: %#v", res.JSON)
	}
	if tableCount != 1 {
		t.Fatalf("structured doc_type_kwd:\"table\" item count = %d, want exactly 1", tableCount)
	}
	if !strings.Contains(tableText, "<table") ||
		!strings.Contains(tableText, "Name") ||
		!strings.Contains(tableText, "Alice") {
		t.Errorf("structured table item missing markup/cells: %q", tableText)
	}
	// No duplicate inline text copy of the table markup.
	if inlineTableCount != 0 {
		t.Errorf("found %d doc_type_kwd:\"text\" item(s) containing <table> markup; nested table must not be duplicated as inline text", inlineTableCount)
	}
	// The surrounding prose is split into clean text items bracketing the
	// table in document order — NOT collapsed into one blob embedding the tags.
	if beforeIdx < 0 {
		t.Fatalf("'Before the table.' text item missing")
	}
	if afterIdx < 0 {
		t.Fatalf("'After the table.' text item missing")
	}
	if !(beforeIdx < tableIdx && tableIdx < afterIdx) {
		t.Errorf("document order wrong: before=%d table=%d after=%d (prose must bracket table, not merge into one blob)", beforeIdx, tableIdx, afterIdx)
	}
}

// TestHTMLParser_MultipleTablesOrdering is the HTML counterpart of
// TestMarkdownParser_MultipleTablesOrdering: two top-level tables must each be
// emitted as a SINGLE structured doc_type_kwd:"table" item, in document order,
// bracketing the "Middle." paragraph (not relocated to the end of the stream —
// the old deferred-append behaviour this PR removes). Cell text of both tables
// must be present and in source order.
func TestHTMLParser_MultipleTablesOrdering(t *testing.T) {
	ctx := t.Context()
	p := NewHTMLParser()
	const html = `<html><body>
<h1>Title</h1>
<table>
<tr><th>A</th><th>B</th></tr>
<tr><td>x</td><td>y</td></tr>
</table>
<p>Middle.</p>
<table>
<tr><th>C</th><th>D</th></tr>
<tr><td>p</td><td>q</td></tr>
</table>
<p>End.</p>
</body></html>`

	res := p.ParseWithResult(ctx, "doc.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	var tableItemIdx []int
	titleIdx, middleIdx, endIdx := -1, -1, -1
	for i, it := range res.JSON {
		text, _ := it["text"].(string)
		switch it["doc_type_kwd"] {
		case "text":
			switch text {
			case "Title":
				titleIdx = i
			case "Middle.":
				middleIdx = i
			case "End.":
				endIdx = i
			}
		case "table":
			tableItemIdx = append(tableItemIdx, i)
		default:
			t.Fatalf("unexpected doc_type_kwd %q", it["doc_type_kwd"])
		}
	}

	if titleIdx < 0 || middleIdx < 0 || endIdx < 0 {
		t.Fatalf("missing anchor text item (title=%d middle=%d end=%d)", titleIdx, middleIdx, endIdx)
	}
	if len(tableItemIdx) != 2 {
		t.Fatalf("table items = %d, want 2", len(tableItemIdx))
	}
	// Both tables appear in document order, bracketing "Middle.":
	// table1 before Middle, table2 between Middle and End.
	if !(titleIdx < tableItemIdx[0] && tableItemIdx[0] < middleIdx && middleIdx < tableItemIdx[1] && tableItemIdx[1] < endIdx) {
		t.Fatalf("table order wrong: tables=%v title=%d middle=%d end=%d", tableItemIdx, titleIdx, middleIdx, endIdx)
	}
	t1, _ := res.JSON[tableItemIdx[0]]["text"].(string)
	t2, _ := res.JSON[tableItemIdx[1]]["text"].(string)
	if !strings.Contains(t1, "x") || !strings.Contains(t1, "y") {
		t.Errorf("first table item missing x/y cells: %q", t1)
	}
	if !strings.Contains(t2, "p") || !strings.Contains(t2, "q") {
		t.Errorf("second table item missing p/q cells: %q", t2)
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
// item in both, see TestHTMLParser_TableProducesStructuredItems). The
// accepted divergences are declared in the golden's meta block, not hardcoded
// here.
//
// The baseline lives in testdata/html.python.en.golden.json / testdata/html.python.zh.golden.json as {meta, items}
// document (see its "meta" block for how to regenerate it from the Python
// engine — no committed generator script).
func TestHTMLParser_AlignmentGolden(t *testing.T) {
	ctx := t.Context()
	p := NewHTMLParser()

	cases := []struct {
		name   string
		sample string
		golden string
	}{
		{"en", "testdata/html.sample.en.html", "testdata/html.python.en.golden.json"},
		{"zh", "testdata/html.sample.zh.html", "testdata/html.python.zh.golden.json"},
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

			doc := LoadGoldenDoc(t, tc.golden)

			// Drop the meta-declared accepted divergences on both sides (here "table"):
			// Go emits a single structured doc_type_kwd:"table" item, while Python keeps
			// the <table> markup inlined as a "text" item. filterTableDivergence drops
			// the table content from both sides so the comparison stays on non-table prose.
			drop := AcceptedDivergences(doc.Meta)
			goText := filterTableDivergence(res.JSON, drop)
			pyText := filterTableDivergence(doc.Items, drop)

			// filterTableDivergence drops the table from the prose comparison;
			// the table-equivalence guard below (assertTablesEquivalent) checks
			// the table cell content still matches Python, so a collapse or
			// dropped column on either side is caught independently.
			assertTablesEquivalent(t, res.JSON, doc.Items)

			if ok, diff := CompareAlignment(goText, pyText, HTMLAlignOptions()); !ok {
				t.Fatalf("html parser not aligned with Python golden:%s", diff)
			}
		})
	}
}
