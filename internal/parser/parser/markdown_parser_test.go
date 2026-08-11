package parser

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMarkdownParser_ParseWithResult_Basic(t *testing.T) {
	ctx := t.Context()
	p, err := NewMarkdownParser(GoMarkdown)
	if err != nil {
		t.Fatalf("NewMarkdownParser: %v", err)
	}
	md := "# Hello\n\nThis is a paragraph.\n\n* List item 1\n* List item 2\n\n```go\nfunc main() {}\n```\n"
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got, want := res.OutputFormat, "json"; got != want {
		t.Fatalf("OutputFormat = %q, want %q", got, want)
	}
	if len(res.JSON) == 0 {
		t.Fatal("JSON is empty; want at least one item")
	}
	// Verify heading
	if got, _ := res.JSON[0]["text"].(string); got != "Hello" {
		t.Fatalf("first item text = %q, want %q", got, "Hello")
	}
	if got, _ := res.JSON[0]["ck_type"].(string); got != "heading" {
		t.Fatalf("first item ck_type = %q, want %q", got, "heading")
	}
}

func TestMarkdownParser_ParseWithResult_EmptyInput(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	res := p.ParseWithResult(ctx, "empty.md", []byte(""))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(JSON) = %d, want 1", len(res.JSON))
	}
}

func TestMarkdownParser_ParseWithResult_ImageDataURI(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	// 1×1 pixel transparent PNG encoded as data URI
	pixelPNG := make([]byte, 68) // minimal 1x1 PNG header
	pixelB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
	md := "Some text with an image\n![test](data:image/png;base64," + pixelB64 + ")\n"
	_ = pixelPNG
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	// Find the image item
	found := false
	for _, item := range res.JSON {
		if kd, _ := item["doc_type_kwd"].(string); kd == "image" {
			found = true
			if img, ok := item["image"].(string); !ok || img != pixelB64 {
				t.Fatalf("image data = %q, want %q", img, pixelB64)
			}
			break
		}
	}
	if !found {
		t.Fatal("no item with doc_type_kwd == 'image' found")
	}
}

func TestMarkdownParser_ParseWithResult_NoImage(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "# Title\n\nJust some text, no images here.\n\nMore text."
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	for _, item := range res.JSON {
		if kd, _ := item["doc_type_kwd"].(string); kd == "image" {
			t.Fatal("unexpected image item in text-only markdown")
		}
	}
}

func TestMarkdownParser_ParseWithResult_RendersTableInline(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "[M03] Health check package comparison:\n\n| Check item | Basic 699 CNY | Advanced 1299 CNY |\n| --- | --- | --- |\n| Blood routine / Urine routine | Yes | Yes |\n\nNote: All packages require fasting.\n"
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	// 方案 Y: a document containing a table must NOT collapse into a single
	// item. Each top-level block is its own item, and the table is emitted as
	// both an inlined text copy (doc_type_kwd:"text") and a separate
	// structured table item (doc_type_kwd:"table"), so the item count is > 1.
	if len(res.JSON) < 2 {
		t.Fatalf("len(JSON) = %d, want >= 2 (table must not collapse the doc)", len(res.JSON))
	}
	var all strings.Builder
	for _, item := range res.JSON {
		kd, _ := item["doc_type_kwd"].(string)
		if kd != "text" && kd != "table" {
			t.Fatalf("unexpected doc_type_kwd %q; want text or table", kd)
		}
		text, _ := item["text"].(string)
		all.WriteString(text)
		all.WriteString("\n")
		if strings.Contains(text, "| Check item |") {
			t.Fatalf("raw markdown table leaked into text: %q", text)
		}
	}
	// The table is kept as raw <table>…</table> HTML (one HTML block), so the
	// cell text survives as text inside the tags — possibly duplicated across
	// the inlined copy and the table item. Check the concatenated text.
	concat := all.String()
	if !strings.Contains(concat, "Check item") || !strings.Contains(concat, "Blood routine") {
		t.Fatalf("table cell content (Check item / Blood routine) not present: %q", concat)
	}
}

func TestMarkdownParser_ConfigureFromSetup(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	p.ConfigureFromSetup(map[string]any{
		"parse_method":          "deepdoc",
		"output_format":         "json",
		"vlm":                   map[string]any{"llm_id": "gpt-4-vision"},
		"flatten_media_to_text": false,
	})
	if p.ParseMethod != "deepdoc" {
		t.Fatalf("ParseMethod = %q, want %q", p.ParseMethod, "deepdoc")
	}
	if p.OutputFormat != "json" {
		t.Fatalf("OutputFormat = %q, want %q", p.OutputFormat, "json")
	}
	if p.VLM == nil {
		t.Fatal("VLM is nil; want map")
	}
	if id, _ := p.VLM["llm_id"].(string); id != "gpt-4-vision" {
		t.Fatalf("VLM[llm_id] = %q, want %q", id, "gpt-4-vision")
	}
}

func TestMarkdownParser_ConfigureFromSetup_NilSafe(t *testing.T) {
	p, _ := NewMarkdownParser(GoMarkdown)
	p.ConfigureFromSetup(nil) // should not panic
	if p.ParseMethod != "" {
		t.Fatalf("ParseMethod should be empty after nil setup, got %q", p.ParseMethod)
	}
}

// TestMarkdownParser_FlattenMediaToText verifies that when
// flatten_media_to_text is true, image items are emitted with
// doc_type_kwd="text" (mirroring Python parser.py:1034). When false,
// image items keep doc_type_kwd="image".
func TestMarkdownParser_FlattenMediaToText(t *testing.T) {
	ctx := t.Context()
	pixelB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
	md := "Some text with an image\n![test](data:image/png;base64," + pixelB64 + ")\n"

	cases := []struct {
		name        string
		flatten     bool
		wantDocType string
	}{
		{"flatten=false keeps image", false, "image"},
		{"flatten=true forces text", true, "text"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := NewMarkdownParser(GoMarkdown)
			p.ConfigureFromSetup(map[string]any{"flatten_media_to_text": tc.flatten})
			if p.FlattenMediaToText != tc.flatten {
				t.Fatalf("FlattenMediaToText field = %v, want %v", p.FlattenMediaToText, tc.flatten)
			}
			res := p.ParseWithResult(ctx, "test.md", []byte(md))
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			for _, item := range res.JSON {
				if kd, _ := item["doc_type_kwd"].(string); kd != tc.wantDocType {
					t.Errorf("doc_type_kwd = %q, want %q (item text=%q)",
						kd, tc.wantDocType, item["text"])
				}
			}
		})
	}
}

func TestResolveImageURL_DataURI(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("fakeimage"))
	result, found := resolveImageURL("data:image/png;base64," + b64)
	if !found {
		t.Fatal("expected image found for data URI")
	}
	if result != b64 {
		t.Fatalf("got %q, want %q", result, b64)
	}
}

func TestResolveImageURL_LocalPathNotFetched(t *testing.T) {
	// Local / relative paths are not fetched (security); resolution fails.
	if _, found := resolveImageURL("./local/image.png"); found {
		t.Fatal("expected no image resolved for a local path")
	}
}

func TestResolveImageURL_HTTPImage(t *testing.T) {
	// httptest servers bind loopback, which the SSRF guard rejects by
	// default. Allow loopback for this test so the HTTP fetch path is
	// exercised (production keeps ssrfAllowLoopback == false).
	prev := ssrfAllowLoopback
	ssrfAllowLoopback = true
	defer func() { ssrfAllowLoopback = prev }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("fake-png-bytes"))
	}))
	defer ts.Close()

	result, found := resolveImageURL(ts.URL + "/image.png")
	if !found {
		t.Fatal("expected image found for HTTP URL")
	}
	expectedB64 := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
	if result != expectedB64 {
		t.Fatalf("got %q, want %q", result, expectedB64)
	}
}

// TestFindBlockImage resolves the image per-block from the AST so only the
// owning block is tagged (the fix for the whole-document scan bug).
func TestFindBlockImage(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)

	withImg := "# T\n\nText without image.\n\n![alt](data:image/png;base64,AAA)\n"
	res := p.ParseWithResult(ctx, "a.md", []byte(withImg))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var imgItems, textItems int
	for _, item := range res.JSON {
		switch kd, _ := item["doc_type_kwd"].(string); kd {
		case "image":
			imgItems++
			if _, ok := item["image"].(string); !ok {
				t.Fatal("image item missing base64 payload")
			}
		case "text":
			textItems++
		}
	}
	if imgItems != 1 {
		t.Fatalf("imgItems = %d, want 1 (only the block owning the image)", imgItems)
	}
	if textItems < 2 {
		t.Fatalf("textItems = %d, want >= 2 (other blocks must stay text, not image)", textItems)
	}

	// A document with no image must not produce any image item.
	noImg := p.ParseWithResult(ctx, "b.md", []byte("# T\n\nNo images here.\n"))
	for _, item := range noImg.JSON {
		if kd, _ := item["doc_type_kwd"].(string); kd == "image" {
			t.Fatal("unexpected image item in text-only markdown")
		}
	}
}

func TestFetchImageAsBase64_RejectsCredentials(t *testing.T) {
	_, err := fetchImageAsBase64("https://user:pass@example.com/img.png")
	if err == nil {
		t.Fatal("expected error for URL with credentials")
	}
}

func TestFetchImageAsBase64_InvalidURL(t *testing.T) {
	withSSRFBypass(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	_, err := fetchImageAsBase64(ts.URL + "/nonexistent.png")
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

// TestMarkdownParser_TableNotCollapsed is the core regression guard for the
// Markdown table fix (方案 Y): a document containing a GFM table must keep one
// item per top-level block instead of collapsing into one giant item, AND the
// table must be emitted as a single raw <table>…</table> HTML block (not
// scattered cell text), appearing both as an inlined text copy and as a
// separate doc_type_kwd:"table" item. A heading, the table, and the trailing
// paragraph must each be present.
func TestMarkdownParser_TableNotCollapsed(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "# Title\n\nIntro paragraph.\n\n| A | B |\n| --- | --- |\n| x | y |\n\nTrailing note.\n"
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	// Title, Intro, inlined table copy, separate table item, Trailing = 5.
	if len(res.JSON) < 5 {
		t.Fatalf("len(JSON) = %d, want >= 5 (one per top-level block + table item)", len(res.JSON))
	}
	var all strings.Builder
	sawTitle, sawTrailing, sawTableItem, sawRawTableHTML := false, false, false, false
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		all.WriteString(text)
		all.WriteString("\n")
		if text == "Title" {
			sawTitle = true
		}
		switch kd, _ := item["doc_type_kwd"].(string); kd {
		case "text":
			// inlined copy carries the raw <table> HTML with cell text.
			if strings.Contains(text, "<table") && strings.Contains(text, "A") && strings.Contains(text, "B") {
				sawRawTableHTML = true
			}
		case "table":
			sawTableItem = true
			if !strings.Contains(text, "<table") {
				t.Fatalf("table item text is not raw <table> HTML: %q", text)
			}
			if ck, _ := item["ck_type"].(string); ck != "table" {
				t.Fatalf("table item ck_type = %q, want \"table\"", ck)
			}
		}
	}
	concat := all.String()
	if strings.Contains(concat, "Trailing note.") {
		sawTrailing = true
	}
	if !sawTitle {
		t.Fatal("heading item not emitted")
	}
	if !sawRawTableHTML {
		t.Fatal("inlined table not emitted as raw <table> HTML with cell text")
	}
	if !sawTableItem {
		t.Fatal("separate doc_type_kwd:\"table\" item not emitted")
	}
	if !sawTrailing {
		t.Fatal("trailing paragraph content not present in concatenated items")
	}
}

// TestMarkdownParser_NonTableHTMLBlockNotTable guards against the blanket
// ck_type:"table" bug: a raw HTML block that is NOT a <table> (e.g. <div>,
// <style>) must be emitted as ordinary text with no ck_type, so downstream
// consumers (chunker) do not mistake it for a table.
func TestMarkdownParser_NonTableHTMLBlockNotTable(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "Before.\n\n<div class=\"note\">just a div block</div>\n\n<style>.a{color:red}</style>\n\nAfter.\n"
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		if strings.Contains(text, "<div") || strings.Contains(text, "<style") {
			if ck, ok := item["ck_type"].(string); ok && ck == "table" {
				t.Fatalf("non-table HTML block wrongly tagged ck_type:\"table\": %q", text)
			}
			if kd, _ := item["doc_type_kwd"].(string); kd == "table" {
				t.Fatalf("non-table HTML block wrongly tagged doc_type_kwd:\"table\": %q", text)
			}
		}
	}
}

// TestMarkdownParser_TableInCodeFenceNotRendered guards against a regression
// where pipe rows INSIDE a fenced code block would be mis-identified as a GFM
// table and rewritten into <table> HTML, corrupting the code. renderMarkdownTablesInline
// tracks fence state (inFence) so the table detector must skip lines inside a fence.
func TestMarkdownParser_TableInCodeFenceNotRendered(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "# Title\n\n```\n| A | B |\n| --- | --- |\n| x | y |\n```\n\nAfter fence.\n"
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	// No table item may be emitted — the pipe rows live inside a code block.
	for _, item := range res.JSON {
		if kd, _ := item["doc_type_kwd"].(string); kd == "table" {
			t.Fatalf("code-fence pipe rows wrongly emitted as table item: %q", item["text"])
		}
	}
	// The code block item must retain the raw pipe text and must NOT contain
	// any <table> markup.
	sawCode := false
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		if strings.Contains(text, "| A | B |") {
			sawCode = true
			if strings.Contains(text, "<table") {
				t.Fatalf("code block text was rewritten into table HTML: %q", text)
			}
		}
	}
	if !sawCode {
		t.Fatal("code block with pipe rows not found in output")
	}
}

// TestMarkdownParser_RawHTMLTableHandled covers a user-written raw <table> HTML
// block (not a GFM pipe table). renderMarkdownTablesInline only rewrites GFM
// pipe tables, so the raw <table> passes through and is caught by isTableHTML
// as an HTMLBlock, producing the same inlined copy + separate doc_type_kwd:"table"
// item shape as a GFM table.
func TestMarkdownParser_RawHTMLTableHandled(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "Before.\n\n<table><tr><td>X</td><td>Y</td></tr></table>\n\nAfter.\n"
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	sawInlined, sawTableItem := false, false
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		switch kd, _ := item["doc_type_kwd"].(string); kd {
		case "text":
			if strings.Contains(text, "<table") && strings.Contains(text, "X") && strings.Contains(text, "Y") {
				sawInlined = true
			}
		case "table":
			sawTableItem = true
			if !strings.Contains(text, "<table") {
				t.Fatalf("raw table item text is not raw <table> HTML: %q", text)
			}
			if ck, _ := item["ck_type"].(string); ck != "table" {
				t.Fatalf("raw table item ck_type = %q, want \"table\"", ck)
			}
		}
	}
	if !sawInlined {
		t.Fatal("raw <table> HTML block not emitted as inlined copy in text flow")
	}
	if !sawTableItem {
		t.Fatal("raw <table> HTML block not emitted as separate doc_type_kwd:\"table\" item")
	}
}

// TestMarkdownParser_MultipleTablesOrdering guards the ordering contract: each
// GFM table emits an inlined copy in its original document position (among the
// surrounding text blocks) and a separate doc_type_kwd:"table" item appended at
// the end of the stream (mirroring Python's _markdown, which appends tables
// after all sections). Both tables' cell text must be present and in source order.
func TestMarkdownParser_MultipleTablesOrdering(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)
	md := "# Title\n\n| A | B |\n| --- | --- |\n| x | y |\n\nMiddle.\n\n| C | D |\n| --- | --- |\n| p | q |\n\nEnd.\n"
	res := p.ParseWithResult(ctx, "test.md", []byte(md))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	var inlinedTableIdx []int
	var tableItemIdx []int
	var titleIdx, middleIdx, endIdx = -1, -1, -1
	for i, item := range res.JSON {
		text, _ := item["text"].(string)
		switch kd, _ := item["doc_type_kwd"].(string); kd {
		case "text":
			switch text {
			case "Title":
				titleIdx = i
			case "Middle.":
				middleIdx = i
			case "End.":
				endIdx = i
			}
			if strings.Contains(text, "<table") {
				inlinedTableIdx = append(inlinedTableIdx, i)
			}
		case "table":
			tableItemIdx = append(tableItemIdx, i)
		}
	}

	if titleIdx < 0 || middleIdx < 0 || endIdx < 0 {
		t.Fatalf("missing anchor text item (title=%d middle=%d end=%d)", titleIdx, middleIdx, endIdx)
	}
	if len(inlinedTableIdx) != 2 {
		t.Fatalf("inlined table copies = %d, want 2", len(inlinedTableIdx))
	}
	if len(tableItemIdx) != 2 {
		t.Fatalf("separate table items = %d, want 2", len(tableItemIdx))
	}
	// Inlined copies appear in document order, bracketing "Middle.":
	// table1 before Middle, table2 between Middle and End.
	if !(inlinedTableIdx[0] < middleIdx && middleIdx < inlinedTableIdx[1] && inlinedTableIdx[1] < endIdx) {
		t.Fatalf("inlined table order wrong: tables=%v middle=%d end=%d", inlinedTableIdx, middleIdx, endIdx)
	}
	// Separate table items are appended after all text items, in source order:
	// table1 (x,y) then table2 (p,q), each strictly after the previous.
	if !(tableItemIdx[0] > endIdx && tableItemIdx[0] < tableItemIdx[1]) {
		t.Fatalf("table items not appended after text items in source order: %v end=%d", tableItemIdx, endIdx)
	}
	t1, _ := res.JSON[tableItemIdx[0]]["text"].(string)
	t2, _ := res.JSON[tableItemIdx[1]]["text"].(string)
	if !strings.Contains(t1, "x") || !strings.Contains(t1, "y") {
		t.Fatalf("first table item missing x/y cells: %q", t1)
	}
	if !strings.Contains(t2, "p") || !strings.Contains(t2, "q") {
		t.Fatalf("second table item missing p/q cells: %q", t2)
	}
}

// TestMarkdownParser_AlignmentGolden verifies Go's ParseWithResult output is
// content-equivalent to Python's _markdown on the shared sample, using the
// shared concatenation-normalization alignment tool (align_test.go). Python
// keeps raw Markdown and splits on the delimiter set; Go emits clean per-block
// text. The comparison normalizes both (Markdown syntax, html tags, delimiters
// stripped; whitespace collapsed) and ignores "table"/"image" items, which are
// accepted representation differences (PARSER_ALIGNMENT_HANDOFF.md §3.1).
//
// Regenerate the baseline with:
//
//	.venv/bin/python internal/parser/parser/testdata/gen_markdown_golden.py
func TestMarkdownParser_AlignmentGolden(t *testing.T) {
	ctx := t.Context()
	p, _ := NewMarkdownParser(GoMarkdown)

	sample, err := os.ReadFile("testdata/markdown.sample.md")
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	res := p.ParseWithResult(ctx, "markdown.sample.md", sample)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}

	golden := LoadGolden(t, "testdata/markdown.python.golden.json")

	// Ignore "table"/"image" items on both sides (accepted divergences).
	goText := FilterByDocType(res.JSON, "text")
	pyText := FilterByDocType(golden, "text")

	if ok, diff := CompareAlignment(goText, pyText, MarkdownAlignOptions(DefaultMarkdownDelimiter)); !ok {
		t.Fatalf("markdown parser not aligned with Python golden:%s", diff)
	}
}
