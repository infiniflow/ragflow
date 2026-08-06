//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

type HTMLParser struct {
	RemoveHeaderFooter bool
	RemoveTOC          bool
}

func NewHTMLParser() *HTMLParser {
	return &HTMLParser{}
}

func (p *HTMLParser) String() string {
	return "HTMLParser"
}

// ConfigureFromSetup reads the HTML family setup map. Mirrors the
// Python parser.py HTML setup keys: remove_header_footer (pre-parse
// tag strip) and remove_toc (post-parse text heuristic).
func (p *HTMLParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["remove_header_footer"].(bool); ok {
		p.RemoveHeaderFooter = v
	}
	if v, ok := setup["remove_toc"].(bool); ok {
		p.RemoveTOC = v
	}
}

// ParseWithResult emits one item per block-level HTML element
// (headings, paragraphs, lists, pre blocks). The walker is a
// pure-Go replacement for the previous `fmt.Printf` debug output:
// it descends the html.Parse tree, joins the leaf text of each
// block-level element, and emits the python-compatible
// `{text, doc_type_kwd:"text"}` shape.
//
// Phase 2.5 (Slice 1) of port-rag-flow-pipeline-to-go.md makes
// HTMLParser a ParseResultProducer so the dispatch seam routes
// the html family through the structured path. Inline formatting
// (bold / links / images) is intentionally NOT surfaced as a
// separate ck_type — the python HtmlParser collapses inline
// formatting into the parent block's text.
func (p *HTMLParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	// remove_header_footer: pre-parse strip of <header>/<footer> tags
	// and ARIA role=banner/contentinfo elements (mirrors Python
	// parser.py:1083-1084 remove_header_footer_html_blob).
	if p.RemoveHeaderFooter {
		cleaned, err := stripHTMLHeaderFooter(data)
		if err != nil {
			return ParseResult{Err: fmt.Errorf("html remove_header_footer: %w", err)}
		}
		data = cleaned
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return ParseResult{Err: fmt.Errorf("html parse: %w", err)}
	}
	var items []map[string]any
	walkHTMLBlocks(doc, &items)
	// remove_toc: post-parse text heuristic (mirrors Python
	// parser.py:1087-1088 remove_toc → remove_contents_table).
	if p.RemoveTOC {
		items = removeContentsTable(items, isEnglishItems(items))
	}
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":     filename,
			"encoding": "utf-8",
		},
		JSON: items,
	}
}

// walkHTMLBlocks emits one normalized item per block-level
// descendant of root. Inline elements (b, i, a, span, …) are
// collapsed into the parent's text via leafText. <script>,
// <style>, and <noscript> blocks are skipped entirely so they
// don't pollute the downstream chunker input.
func walkHTMLBlocks(root *html.Node, out *[]map[string]any) {
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			if emitsLooseHTMLText(root) {
				appendHTMLTextItem(out, child.Data, "text", true)
			}
			continue
		}
		if child.Type != html.ElementNode {
			continue
		}
		tag := child.Data
		switch tag {
		case "script", "style", "noscript":
			// Skip executable / stylistic blocks entirely.
			continue
		case "head":
			// Skip document metadata so it does not pollute body text.
			continue
		case "html", "body":
			// Wrapper elements: descend into their children.
			walkHTMLBlocks(child, out)
			continue
		}
		text := htmlLeafText(child)
		appendHTMLTextItem(out, text, htmlTagToCkType(tag), tag != "pre" && tag != "textarea")
	}
}

func emitsLooseHTMLText(root *html.Node) bool {
	return root.Type == html.ElementNode && root.Data == "body"
}

func appendHTMLTextItem(out *[]map[string]any, text, ckType string, trim bool) {
	if trim {
		text = strings.TrimSpace(text)
	}
	if text == "" {
		return
	}
	*out = append(*out, map[string]any{
		"text":         text,
		"doc_type_kwd": "text",
		"ck_type":      ckType,
	})
}

// htmlTagToCkType maps HTML block tags to the python `ck_type`
// vocabulary used downstream by TitleChunker and similar
// components. Tags not in the map fall back to "text".
func htmlTagToCkType(tag string) string {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "p":
		return "paragraph"
	case "ul", "ol", "li":
		return "list"
	case "pre", "code":
		return "code"
	case "table", "tr", "td", "th":
		return "table"
	case "blockquote":
		return "quote"
	case "img":
		return "image"
	}
	return "text"
}

// leafWriter accumulates the visible text of an HTML subtree while applying
// CSS whitespace folding (the default white-space: normal rules):
//   - collapsible whitespace runs collapse to a single space;
//   - leading/trailing whitespace of a line is dropped;
//   - a <br> forces a hard line break (and resets the leading-whitespace state);
//   - <pre>/<textarea> are emitted verbatim (no folding, no injected breaks).
type leafWriter struct {
	b         *bytes.Buffer
	lastSpace bool // last written rune was a collapsed single space
	lineStart bool // at the start of a line, so leading whitespace is dropped
	endsNL    bool // builder currently ends with a hard line break
	pre       bool // inside <pre>/<textarea>: emit verbatim
}

func isCollapsibleWS(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\f'
}

// writeText appends s, folding collapsible whitespace unless in pre mode.
func (w *leafWriter) writeText(s string) {
	if w.pre {
		for _, r := range s {
			w.b.WriteRune(r)
			w.endsNL = r == '\n'
		}
		w.lastSpace = false
		w.lineStart = false
		return
	}
	for _, r := range s {
		if isCollapsibleWS(r) {
			if w.lineStart || w.lastSpace {
				continue
			}
			w.b.WriteRune(' ')
			w.lastSpace = true
			w.lineStart = false
			w.endsNL = false
			continue
		}
		w.b.WriteRune(r)
		w.lastSpace = false
		w.lineStart = false
		w.endsNL = false
	}
}

// hardBreak inserts a forced line break (a <br> or block boundary). Per CSS,
// whitespace immediately before a break is dropped (so "Hello <br>" yields
// "Hello\n", not "Hello \n"). Inside <pre>/<textarea> whitespace is preserved,
// so the preceding space is kept.
func (w *leafWriter) hardBreak() {
	if !w.pre && w.lastSpace && w.b.Len() > 0 {
		w.b.Truncate(w.b.Len() - 1)
	}
	w.b.WriteByte('\n')
	w.lastSpace = false
	w.lineStart = true
	w.endsNL = true
}

// htmlLeafText joins the visible text of an HTML node and its
// descendants. <script>/<style>/<noscript> subtrees are skipped. Whitespace
// is folded per CSS rules (so "<h1>Hello   world</h1>" becomes "Hello world"
// and "<br>" survives as a real line break), while <pre>/<textarea> keep
// their source formatting verbatim.
func htmlLeafText(n *html.Node) string {
	var b bytes.Buffer
	w := &leafWriter{b: &b}
	walkHTMLLeaf(n, w)
	return b.String()
}

func walkHTMLLeaf(n *html.Node, w *leafWriter) {
	switch n.Type {
	case html.TextNode:
		w.writeText(n.Data)
	case html.ElementNode:
		if n.Data == "script" || n.Data == "style" || n.Data == "noscript" {
			return
		}
		if n.Data == "br" {
			w.hardBreak()
			return
		}
		if n.Data == "pre" || n.Data == "textarea" {
			// Verbatim: no folding, no injected block breaks.
			w.pre = true
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walkHTMLLeaf(child, w)
			}
			w.pre = false
			return
		}
		// Add a line break between block children so headings, paragraphs,
		// and list items don't run together.
		if !w.pre {
			switch n.Data {
			case "h1", "h2", "h3", "h4", "h5", "h6", "p", "li", "pre",
				"tr", "blockquote":
				if w.b.Len() > 0 && !w.endsNL {
					w.hardBreak()
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walkHTMLLeaf(child, w)
		}
		if !w.pre && isBlockTag(n.Data) && w.b.Len() > 0 && !w.endsNL {
			w.hardBreak()
		}
	}
}

func isBlockTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "li", "pre",
		"tr", "blockquote", "div", "section", "article", "header", "footer":
		return true
	}
	return false
}
