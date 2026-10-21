package parser

import (
	"context"
	"strings"
	"testing"
)

func TestHTMLParser_ConfigureFromSetup(t *testing.T) {
	p := NewHTMLParser()
	p.ConfigureFromSetup(map[string]any{
		"remove_header_footer": true,
		"remove_toc":           true,
	})
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
	if p.RemoveHeaderFooter || p.RemoveTOC {
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
