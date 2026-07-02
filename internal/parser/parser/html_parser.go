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
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

const (
	Official string = "official"
)

type HTMLParser struct {
	libType string
}

func NewHTMLParser(libType string) (*HTMLParser, error) {
	switch libType {
	case Official:
		return &HTMLParser{
			libType: Official,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported HTML library type: %s", libType)
	}
}

func (p *HTMLParser) Parse(filename string, data []byte) error {
	switch p.libType {
	case Official:
		return p.OfficialHTMLParse(data)
	default:
		return fmt.Errorf("unsupported HTML library type: %s", p.libType)
	}
}

func (p *HTMLParser) OfficialHTMLParse(data []byte) error {
	doc, _ := html.Parse(strings.NewReader(string(data)))
	p.WalkIterative(doc)
	return nil
}

func (p *HTMLParser) WalkIterative(root *html.Node) {
	if root == nil {
		return
	}

	// Stack: stores node and its depth
	type item struct {
		node  *html.Node
		depth int
	}
	stack := []item{{root, 0}}

	for len(stack) > 0 {
		// Pop the top of the stack
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		indent := strings.Repeat("  ", current.depth)

		// Handle different node types
		switch current.node.Type {
		case html.ElementNode:
			// Print opening tag
			fmt.Printf("%s<%s", indent, current.node.Data)
			// Optionally print attributes
			for _, attr := range current.node.Attr {
				fmt.Printf(" %s=%q", attr.Key, attr.Val)
			}
			fmt.Println(">")

		case html.TextNode:
			// Print text content (trim extra whitespace)
			text := strings.TrimSpace(current.node.Data)
			if text != "" {
				fmt.Printf("%stext: %q\n", indent, text)
			}

		case html.CommentNode:
			fmt.Printf("%scomment: %s\n", indent, current.node.Data)

		case html.DoctypeNode:
			fmt.Printf("%sDOCTYPE: %s\n", indent, current.node.Data)
		}

		// Push children onto stack in reverse order to maintain original sequence
		var children []*html.Node
		for child := current.node.FirstChild; child != nil; child = child.NextSibling {
			children = append([]*html.Node{child}, children...) // Reverse order
		}
		for _, child := range children {
			stack = append(stack, item{child, current.depth + 1})
		}
	}
}

func (p *HTMLParser) String() string {
	return "HTMLParser"
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
func (p *HTMLParser) ParseWithResult(filename string, data []byte) ParseResult {
	if p.libType != Official {
		return ParseResult{Err: fmt.Errorf("unsupported HTML library type: %s", p.libType)}
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return ParseResult{Err: fmt.Errorf("html parse: %w", err)}
	}
	var items []map[string]any
	walkHTMLBlocks(doc, &items)
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
// collapsed into the parent's text via leafText. <script> and
// <style> blocks are skipped entirely so they don't pollute the
// downstream chunker input.
func walkHTMLBlocks(root *html.Node, out *[]map[string]any) {
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		tag := child.Data
		switch tag {
		case "script", "style", "noscript":
			// Skip executable / stylistic blocks entirely.
			continue
		case "html", "head", "body":
			// Wrapper elements: descend into their children.
			walkHTMLBlocks(child, out)
			continue
		}
		text := htmlLeafText(child)
		if strings.TrimSpace(text) == "" {
			continue
		}
		*out = append(*out, map[string]any{
			"text":         strings.TrimSpace(text),
			"doc_type_kwd": "text",
			"ck_type":      htmlTagToCkType(tag),
		})
	}
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
// descendants. <script>/<style> subtrees are skipped (mirrors
// the python html.parser behaviour). The output preserves
// whitespace runs so headings like "<h1>Hello   world</h1>"
// round-trip with their spacing intact.
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
		if n.Data == "script" || n.Data == "style" {
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
	for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
		walkHTMLLeaf(sib, b)
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
