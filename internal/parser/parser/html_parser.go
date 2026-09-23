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
	"encoding/base64"
	"fmt"
	"net/url"
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

// ParseWithResult emits normalized text items for block-level HTML elements
// and independent image items for supported <img> sources. Relative sources
// are carried as locators for the ingestion component to resolve from storage.
// The walker is a pure-Go replacement for the previous `fmt.Printf` debug output: it descends
// the html.Parse tree, joins visible text, and keeps image payloads available
// to the parser's media enhancement stage.
//
// Phase 2.5 (Slice 1) of port-rag-flow-pipeline-to-go.md makes
// HTMLParser a ParseResultProducer so the dispatch seam routes
// the html family through the structured path. Inline formatting such as
// bold and links stays in the parent text; supported images are emitted as
// separate items so OCR and vision enhancement can consume their payloads.
func (p *HTMLParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	// x/net/html assumes UTF-8 input, so a GBK/Big5/Shift-JIS page would
	// otherwise surface as U+FFFD mojibake. Decode first, mirroring the
	// Python dataflow path (RAGFlowHtmlParser decodes the blob via
	// rag.nlp.find_codec before parsing). See decodeHTMLToUTF8.
	data, encName := decodeHTMLToUTF8(data)
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
			"encoding": encName,
		},
		JSON: items,
	}
}

// decodeHTMLToUTF8 converts non-UTF-8 HTML bytes to UTF-8 and reports the
// encoding label used. It delegates to the shared DecodeToUTF8 helper.
func decodeHTMLToUTF8(data []byte) ([]byte, string) {
	return DecodeToUTF8(data, "text/html")
}

// walkHTMLBlocks emits one normalized item per block-level
// descendant of root. Inline elements (b, i, a, span, …) are
// collapsed into the parent's text via leafText. <script>,
// <style>, and <noscript> blocks are skipped entirely so they
// don't pollute the downstream chunker input.
func walkHTMLBlocks(root *html.Node, out *[]map[string]any) {
	walkHTMLBlocksWithState(root, out, &htmlWalkState{})
}

type htmlWalkState struct {
	tableSequence int
	mediaOrder    int
}

func walkHTMLBlocksWithState(root *html.Node, out *[]map[string]any, state *htmlWalkState) {
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
			walkHTMLBlocksWithState(child, out, state)
			continue
		case "table":
			// Emit the <table> as a SINGLE structured doc_type_kwd:"table"
			// item, in document order. Keeping the full <table>…</table>
			// markup (not flattened) preserves row/column structure for
			// embedding, retrieval, and LLM rendering, and doc_type_kwd/
			// ck_type drives downstream table handling (discrete chunk +
			// table context). We emit ONLY this item — no duplicate
			// doc_type_kwd:"text" copy — so the table is embedded once and
			// its markup does not pollute neighbouring prose chunks.
			emitHTMLTable(child, out, state)
			continue
		}
		ckType := htmlTagToCkType(tag)
		trim := tag != "pre" && tag != "textarea"
		htmlLeafTextWithState(child, out, ckType, trim, state)
	}
}

// renderTableHTML serializes a <table> node back to its outer HTML markup
// (tags preserved), mirroring Python's HtmlParser which keeps the full
// <table>…</table> string as the section text. This preserves row/column
// structure for embedding, retrieval, and LLM rendering, instead of
// flattening cells into a single text blob. It is used both for top-level
// tables (walkHTMLBlocks) and for tables reached via the leaf-text extractor
// (walkHTMLLeaf, i.e. a <table> nested in a div/section/…). On any rendering
// error it returns "" so callers skip the table rather than risk a render
// loop through the leaf extractor — html.Render only fails on unsupported
// node kinds, and a parsed <table> never triggers it.
func renderTableHTML(n *html.Node) string {
	var b bytes.Buffer
	if err := html.Render(&b, n); err != nil {
		return ""
	}
	return b.String()
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

// htmlLeafTextWithState joins the visible text of an HTML node and its
// descendants and emits items directly into out. <script>/<style>/<noscript>
// subtrees are skipped. Whitespace is folded per CSS rules (so
// "<h1>Hello   world</h1>" becomes "Hello world" and "<br>" survives as a
// real line break), while <pre>/<textarea> keep their source formatting
// verbatim. ckType is the block's ck_type (from htmlTagToCkType) applied to the
// accumulated prose item; trim controls whether trailing/leading whitespace is
// collapsed (false for <pre>/<textarea>, which must stay verbatim). Any <table>
// encountered in the subtree is emitted as a single structured
// doc_type_kwd:"table" item at its document position (see walkHTMLLeaf's
// "table" case) — the prose around it is flushed as ordinary text items, so
// the table is never relocated to the end and never duplicated.
func htmlLeafTextWithState(n *html.Node, out *[]map[string]any, ckType string, trim bool, state *htmlWalkState) {
	var b bytes.Buffer
	w := &leafWriter{b: &b}
	walkHTMLLeaf(n, w, out, ckType, trim, state)
	flushLeafText(w, out, ckType, trim)
}

// flushLeafText emits any text accumulated in w as a doc_type_kwd:"text" item
// (dropping empty output) and resets the writer. ckType/trim mirror
// appendHTMLTextItem. It is called at block boundaries and at <table> elements
// so tables are emitted in their original document position rather than being
// relocated. Prose with no specific block tag (e.g. text accumulated just
// before a nested table) is flushed as ck_type "text".
func flushLeafText(w *leafWriter, out *[]map[string]any, ckType string, trim bool) {
	text := w.b.String()
	if trim {
		text = strings.TrimSpace(text)
	}
	if text == "" {
		return
	}
	appendHTMLTextItem(out, text, ckType, false)
	w.b.Reset()
	w.lastSpace = false
	w.lineStart = true
	w.endsNL = false
}

func walkHTMLLeaf(n *html.Node, w *leafWriter, out *[]map[string]any, ckType string, trim bool, state *htmlWalkState) {
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
		if n.Data == "img" {
			src := htmlAttribute(n, "src")
			if usableHTMLImageSource(src) || relativeHTMLImageSource(src) {
				flushLeafText(w, out, ckType, trim)
				appendHTMLImageItem(out, state, src, htmlAttribute(n, "alt"), "", 0, 0)
			}
			return
		}
		if n.Data == "pre" || n.Data == "textarea" {
			// Verbatim: no folding, no injected block breaks.
			w.pre = true
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walkHTMLLeaf(child, w, out, ckType, trim, state)
			}
			w.pre = false
			return
		}
		if n.Data == "table" {
			// Emit the <table> as a SINGLE structured doc_type_kwd:"table"
			// item, in document order. flushLeafText first emits any prose
			// accumulated before the table so the table stays in its original
			// position rather than being relocated to the end. The full
			// <table>…</table> markup is preserved (not flattened) so
			// row/column structure survives; we do NOT also inline the markup
			// into the parent's text, which would duplicate the table and
			// pollute the prose chunk with raw tags.
			flushLeafText(w, out, "text", true)
			emitHTMLTable(n, out, state)
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
			walkHTMLLeaf(child, w, out, ckType, trim, state)
		}
		if !w.pre && isBlockTag(n.Data) && w.b.Len() > 0 && !w.endsNL {
			w.hardBreak()
		}
	}
}

type htmlTableImage struct {
	src    string
	alt    string
	row    int
	column int
}

func emitHTMLTable(n *html.Node, out *[]map[string]any, state *htmlWalkState) {
	markup := renderTableHTML(n)
	if strings.TrimSpace(markup) == "" {
		return
	}
	images := htmlTableImages(n)
	table := map[string]any{
		"text":         markup,
		"doc_type_kwd": "table",
		"ck_type":      "table",
	}
	tableID := ""
	if len(images) > 0 {
		state.tableSequence++
		tableID = fmt.Sprintf("html-table-%d", state.tableSequence)
		table["source_table_id"] = tableID
	}
	*out = append(*out, table)
	for _, media := range images {
		appendHTMLImageItem(out, state, media.src, media.alt, tableID, media.row, media.column)
	}
}

func htmlTableImages(table *html.Node) []htmlTableImage {
	var images []htmlTableImage
	rowCount := 0
	columnCounts := make(map[int]int)
	var walk func(node *html.Node, row, column int)
	walk = func(node *html.Node, row, column int) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "tr":
				rowCount++
				row = rowCount
				column = 0
			case "td", "th":
				columnCounts[row]++
				column = columnCounts[row]
			case "img":
				src := htmlAttribute(node, "src")
				if usableHTMLImageSource(src) || relativeHTMLImageSource(src) {
					images = append(images, htmlTableImage{
						src:    src,
						alt:    htmlAttribute(node, "alt"),
						row:    row,
						column: column,
					})
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, row, column)
		}
	}
	walk(table, 0, 0)
	return images
}

func appendHTMLImageItem(out *[]map[string]any, state *htmlWalkState, src, alt, parentTableID string, row, column int) {
	if comma := strings.IndexByte(src, ','); usableHTMLImageSource(src) && comma > 0 && len(src) >= len("data:image/") && strings.EqualFold(src[:len("data:image/")], "data:image/") {
		src = strings.ToLower(src[:comma]) + src[comma:]
	}
	state.mediaOrder++
	item := map[string]any{
		"text":         strings.TrimSpace(alt),
		"doc_type_kwd": "image",
		"ck_type":      "image",
		"media_order":  state.mediaOrder,
	}
	if usableHTMLImageSource(src) {
		item["image"] = src
	} else {
		item["image_src"] = strings.TrimSpace(src)
	}
	if parentTableID != "" {
		item["parent_table_id"] = parentTableID
		item["row_index"] = row
		item["column_index"] = column
	}
	*out = append(*out, item)
}

func usableHTMLImageSource(src string) bool {
	src = strings.TrimSpace(src)
	const dataPrefix = "data:image/"
	if len(src) >= len(dataPrefix) && strings.EqualFold(src[:len(dataPrefix)], dataPrefix) {
		separator := strings.IndexByte(src, ',')
		if separator <= len(dataPrefix) || separator > 128 || !strings.EqualFold(src[separator-len(";base64"):separator], ";base64") {
			return false
		}
		payload := src[separator+1:]
		if payload == "" {
			return false
		}
		if _, err := base64.StdEncoding.DecodeString(payload); err == nil {
			return true
		}
		_, err := base64.RawStdEncoding.DecodeString(payload)
		return err == nil
	}
	u, err := url.Parse(src)
	return err == nil && (strings.EqualFold(u.Scheme, "https") || strings.EqualFold(u.Scheme, "http")) && u.Host != ""
}

func relativeHTMLImageSource(src string) bool {
	src = strings.TrimSpace(src)
	if src == "" {
		return false
	}
	u, err := url.Parse(strings.ReplaceAll(src, " ", "%20"))
	return err == nil && !u.IsAbs() && u.Host == "" && u.Opaque == "" && u.Path != "" && !strings.HasPrefix(u.Path, "/")
}

func htmlAttribute(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func isBlockTag(tag string) bool {
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6", "p", "li", "pre",
		"tr", "blockquote", "div", "section", "article", "header", "footer":
		return true
	}
	return false
}
