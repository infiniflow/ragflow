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
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		for _, banned := range []string{"Site Navigation", "Copyright 2024", "Ad banner", "Legal info"} {
			if strings.Contains(text, banned) {
				t.Errorf("header/footer text %q leaked into output item: %q", banned, text)
			}
		}
	}
	// Main content must survive.
	found := false
	for _, item := range res.JSON {
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
	for _, item := range res.JSON {
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
	for _, item := range res.JSON {
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
	for _, item := range res.JSON {
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
