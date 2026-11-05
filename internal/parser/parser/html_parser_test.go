package parser

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/entity"
)

func TestHTMLParser_ConfigureFromSetup(t *testing.T) {
	p := NewHTMLParser()
	p.ConfigureFromSetup(map[string]any{
		"flatten_media_to_text": true,
		"remove_header_footer":  true,
		"remove_toc":            true,
	})
	if !p.FlattenMediaToText {
		t.Error("FlattenMediaToText = false, want true")
	}
	if !p.RemoveHeaderFooter {
		t.Error("RemoveHeaderFooter = false, want true")
	}
	if !p.RemoveTOC {
		t.Error("RemoveTOC = false, want true")
	}
}

func TestHTMLParser_ConfigureFromSetup_NilSafe(t *testing.T) {
	p := NewHTMLParser()
	p.ConfigureFromSetup(nil) // should not panic
	if p.FlattenMediaToText || p.RemoveHeaderFooter || p.RemoveTOC {
		t.Error("flags should stay false after nil setup")
	}
}

// TestHTMLParser_BlockElementsStartANewLine checks that a block element's
// text starts and ends a line of its own. Only p, li, headings, tr and
// blockquote broke the line in front of them, and dl, figure, hr, nav and the
// other block elements outside isBlockTag broke it on neither side, so the
// text around them ran into theirs: "Name:Alice", "ATermDefinitionB".
func TestHTMLParser_BlockElementsStartANewLine(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"text before a nested div", `<div>Name:<div>Alice</div></div>`, "Name:\nAlice"},
		{"inline before a nested div", `<div><span>Total</span><div>42</div></div>`, "Total\n42"},
		{"definition list", `<div>A<dl><dt>Term</dt><dd>Definition</dd></dl>B</div>`, "A\nTerm\nDefinition\nB"},
		{"figure caption", `<div>See<figure><figcaption>Fig 1</figcaption></figure>below</div>`, "See\nFig 1\nbelow"},
		{"horizontal rule", `<div>above<hr>below</div>`, "above\nbelow"},
		{"nav inside an article", `<article>left<nav>Menu</nav>right</article>`, "left\nMenu\nright"},
		{"inline elements stay on the line", `<div>Hello <b>bold</b> and <a href="x">link</a> text</div>`, "Hello bold and link text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := NewHTMLParser().ParseWithResult(context.Background(), "page.html", []byte("<html><body>"+tt.body+"</body></html>"))
			if res.Err != nil {
				t.Fatalf("ParseWithResult: %v", res.Err)
			}
			items := ItemsToMaps(res.JSON)
			if len(items) != 1 {
				t.Fatalf("items = %v, want one text item", items)
			}
			if got := items[0]["text"]; got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestHTMLParser_RemoveHeaderFooter verifies that <header>/<footer>
// tags and role="banner"/role="contentinfo" elements are stripped
// before parsing (mirrors Python parser.py:1083-1084 pre-parse).
func TestHTMLParser_RemoveHeaderFooter(t *testing.T) {

	p := NewHTMLParser()
	p.ConfigureFromSetup(map[string]any{"remove_header_footer": true})

	html := `<html><body>
<header>Site Navigation</header>
<main>
<p>Main content here</p>
<div role="banner">Ad banner</div>
<footer>Copyright 2024</footer>
<div role="contentinfo">Legal info</div>
</main>
</body></html>`

	res := p.ParseWithResult(context.Background(), "test.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	for _, item := range ItemsToMaps(res.JSON) {
		text, _ := item["text"].(string)
		for _, banned := range []string{"Site Navigation", "Copyright 2024", "Ad banner", "Legal info"} {
			if strings.Contains(text, banned) {
				t.Errorf("header/footer text %q leaked into output item: %q", banned, text)
			}
		}
	}
	// Main content must survive.
	found := false
	for _, item := range ItemsToMaps(res.JSON) {
		if text, _ := item["text"].(string); strings.Contains(text, "Main content here") {
			found = true
		}
	}
	if !found {
		t.Error("main content was stripped along with header/footer")
	}
}

// TestHTMLParser_RemoveHeaderFooter_DisabledByDefault verifies that
// without the flag, <header>/<footer> content is preserved.
func TestHTMLParser_RemoveHeaderFooter_DisabledByDefault(t *testing.T) {

	p := NewHTMLParser()

	html := `<html><body><header>Nav</header><p>Body</p></body></html>`
	res := p.ParseWithResult(context.Background(), "test.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	found := false
	for _, item := range ItemsToMaps(res.JSON) {
		if text, _ := item["text"].(string); strings.Contains(text, "Nav") {
			found = true
		}
	}
	if !found {
		t.Error("header text should be preserved when flag is off")
	}
}

// TestHTMLParser_RemoveTOC verifies that TOC sections are filtered
// after parsing (mirrors Python parser.py:1087-1088 post-parse).
func TestHTMLParser_RemoveTOC(t *testing.T) {

	p := NewHTMLParser()
	p.ConfigureFromSetup(map[string]any{"remove_toc": true})

	html := `<html><body>
<h1>前言</h1>
<h1>目录</h1>
<p>第一章 概述</p>
<p>第二章 方法</p>
<p>正文开始</p>
</body></html>`

	res := p.ParseWithResult(context.Background(), "test.html", []byte(html))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	// "目录" heading and "第一章 概述" (first entry after heading)
	// should be removed by removeContentsTable.
	for _, item := range ItemsToMaps(res.JSON) {
		text, _ := item["text"].(string)
		if text == "目录" {
			t.Errorf("TOC heading %q was not removed", text)
		}
		if strings.HasPrefix(text, "第一章") {
			t.Errorf("first TOC entry %q was not removed", text)
		}
	}
	// "前言" and "正文开始" should survive.
	wantTexts := map[string]bool{"前言": false, "正文开始": false}
	for _, item := range ItemsToMaps(res.JSON) {
		text, _ := item["text"].(string)
		if _, ok := wantTexts[text]; ok {
			wantTexts[text] = true
		}
	}
	for text, found := range wantTexts {
		if !found {
			t.Errorf("expected %q to survive, but it was removed", text)
		}
	}
}

// TestHTMLParser_SkipsTemplateContent checks that markup inside a <template>
// is not indexed. It is a pattern a script may clone (often with placeholders
// such as {{name}}), never something the page shows, as the table parser's
// inert-element list already treats it.
func TestHTMLParser_SkipsTemplateContent(t *testing.T) {
	const doc = `<html><body><p>Shown</p>Run <template>inline {{x}}</template>on` +
		`<template><p>Hello {{name}}</p></template><p>Also shown</p></body></html>`
	res := NewHTMLParser().ParseWithResult(context.Background(), "page.html", []byte(doc))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if got := itemsText(res); strings.Contains(got, "{{") {
		t.Errorf("template content was indexed: %q", got)
	}
}

// TestHTMLParser_BodyTextAndInlineElementsFormOneItem checks that a run of
// text and inline elements directly under <body> is read as one item, the way
// a browser lays it out. Each text node and each inline element was its own
// item, so "Hello <b>world</b>, how are <a>you</a>?" came out as the five
// items "Hello", "world", ", how are", "you" and "?". A comment or a script
// inside the run does not end it; a block element does.
func TestHTMLParser_BodyTextAndInlineElementsFormOneItem(t *testing.T) {
	const doc = `<html><body>Hello <b>world</b>, how are <a href="x">you</a>?<br>` +
		`Run <code>make</code><!-- note --><script>track()</script> now` +
		`<p>A paragraph</p>tail <i>end</i></body></html>`
	res := NewHTMLParser().ParseWithResult(context.Background(), "page.html", []byte(doc))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var got []string
	for _, item := range ItemsToMaps(res.JSON) {
		text, _ := item["text"].(string)
		got = append(got, text)
	}
	want := []string{"Hello world, how are you?\nRun make now", "A paragraph", "tail end"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("items = %q, want %q", got, want)
	}
}

func TestHTMLParser_FlattenMediaToText(t *testing.T) {
	ctx, _ := recordingMediaContext()
	const doc = `<html><body>` +
		`<p>Intro</p>` +
		`<table><tr><th>Col 1</th><th>Col 2</th></tr><tr><td>Val A</td><td>Val B</td></tr></table>` +
		`<p><img alt="Sample Image" src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="></p>` +
		`<p>Outro</p>` +
		`</body></html>`

	// 1. Default: FlattenMediaToText = false preserves table and image doc_type_kwd
	pDefault := NewHTMLParser()
	resDefault := pDefault.ParseWithResult(ctx, "sample.html", []byte(doc))
	if resDefault.Err != nil {
		t.Fatalf("ParseWithResult default: %v", resDefault.Err)
	}
	itemsDefault := ItemsToMaps(resDefault.JSON)
	var defaultTypes []string
	for _, it := range itemsDefault {
		defaultTypes = append(defaultTypes, it["doc_type_kwd"].(string))
	}
	// Expected: text, table, image, text
	wantDefaultTypes := []string{"text", "table", "image", "text"}
	if strings.Join(defaultTypes, ",") != strings.Join(wantDefaultTypes, ",") {
		t.Fatalf("default doc_type_kwds = %v, want %v", defaultTypes, wantDefaultTypes)
	}
	if _, ok := itemsDefault[1]["table"]; !ok {
		t.Error("default table item should carry structured table field")
	}

	// 2. FlattenMediaToText = true: table and image normalized to text
	pFlatten := NewHTMLParser()
	pFlatten.ConfigureFromSetup(map[string]any{"flatten_media_to_text": true})
	resFlatten := pFlatten.ParseWithResult(ctx, "sample.html", []byte(doc))
	if resFlatten.Err != nil {
		t.Fatalf("ParseWithResult flatten: %v", resFlatten.Err)
	}
	itemsFlatten := ItemsToMaps(resFlatten.JSON)
	var flattenTypes []string
	for _, it := range itemsFlatten {
		flattenTypes = append(flattenTypes, it["doc_type_kwd"].(string))
	}
	// Expected: all text
	wantFlattenTypes := []string{"text", "text", "text", "text"}
	if strings.Join(flattenTypes, ",") != strings.Join(wantFlattenTypes, ",") {
		t.Fatalf("flattened doc_type_kwds = %v, want %v", flattenTypes, wantFlattenTypes)
	}
	if _, ok := itemsFlatten[1]["table"]; ok {
		t.Error("flattened table item should not carry table field")
	}
	tableText, _ := itemsFlatten[1]["text"].(string)
	if !strings.Contains(tableText, "<table>") || !strings.Contains(tableText, "Val A") {
		t.Errorf("flattened table text = %q, want rendered HTML table", tableText)
	}
}

func TestFlattenHTMLMediaToText(t *testing.T) {
	td := &entity.TableData{
		Grid: [][]entity.TableCell{
			{{Text: "Header 1"}, {Text: "Header 2"}},
			{{Text: "Row 1 Col 1"}, {Text: "Row 1 Col 2"}},
		},
		HeaderRows: 1,
	}
	items := []map[string]any{
		{
			"text":         "Intro paragraph",
			"doc_type_kwd": "text",
		},
		{
			"table":        td,
			"doc_type_kwd": "table",
		},
		{
			"text":         "alt description",
			"image":        "image-data",
			"doc_type_kwd": "image",
		},
	}

	flattened := flattenHTMLMediaToText(items)
	if len(flattened) != 3 {
		t.Fatalf("len(flattened) = %d, want 3", len(flattened))
	}

	// Text item
	if flattened[0]["doc_type_kwd"] != "text" || flattened[0]["text"] != "Intro paragraph" {
		t.Errorf("item 0 = %+v", flattened[0])
	}

	// Table item
	if flattened[1]["doc_type_kwd"] != "text" {
		t.Errorf("item 1 doc_type_kwd = %v, want text", flattened[1]["doc_type_kwd"])
	}
	if _, ok := flattened[1]["table"]; ok {
		t.Error("item 1 still carries table field")
	}
	rendered, _ := flattened[1]["text"].(string)
	if !strings.Contains(rendered, "<table>") || !strings.Contains(rendered, "Header 1") {
		t.Errorf("item 1 rendered text = %q", rendered)
	}

	// Image item
	if flattened[2]["doc_type_kwd"] != "text" || flattened[2]["text"] != "alt description" {
		t.Errorf("item 2 = %+v", flattened[2])
	}
}

