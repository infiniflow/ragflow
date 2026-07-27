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
		items = removeContentsTable(items, false)
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
				appendHTMLTextItem(out, child.Data, "text")
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
		appendHTMLTextItem(out, text, htmlTagToCkType(tag))
	}
}

func emitsLooseHTMLText(root *html.Node) bool {
	return root.Type == html.ElementNode && root.Data == "body"
}

func appendHTMLTextItem(out *[]map[string]any, text, ckType string) {
	text = strings.TrimSpace(text)
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

// htmlLeafText joins the visible text of an HTML node and its
// descendants. <script>/<style>/<noscript> subtrees are skipped.
// The output preserves whitespace runs so headings like
// "<h1>Hello   world</h1>" round-trip with their spacing intact.
func htmlLeafText(n *html.Node) string {
	var b strings.Builder
	walkHTMLLeaf(n, &b)
	return b.String()
}

func walkHTMLLeaf(n *html.Node, b *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(n.Data)
	case html.ElementNode:
		if n.Data == "script" || n.Data == "style" || n.Data == "noscript" {
			return
		}
		// Add a line break between block children so headings,
		// paragraphs, and list items don't run together.
		switch n.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6", "p", "li", "pre",
			"tr", "blockquote":
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
				b.WriteString("\n")
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walkHTMLLeaf(child, b)
		}
		if isBlockTag(n.Data) && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
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
