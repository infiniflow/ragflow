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
	"regexp"
	"strings"

	"github.com/saintfish/chardet"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
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

// htmlCharsetFallbackLabels is the ordered brute-force decode loop for HTML
// bytes that are neither valid UTF-8 nor carrying a BOM / <meta charset>
// declaration, mirroring the pragmatic core of Python's find_codec
// (rag/nlp/__init__.py): legacy pages that omit a charset declaration are
// overwhelmingly GBK-family (saved-by-browser Chinese finance/gov pages), so
// GB18030 (the GBK superset) leads. Big5 / Shift-JIS / EUC-KR pages
// virtually always declare their charset and are caught by the meta prescan;
// when the declaration is missing, the statistical detector
// (htmlDetectedCharset) reorders the attempt so they are not silently
// absorbed by GB18030. ISO-8859-1 is intentionally terminal, the way latin1
// terminates Python's codec loop: it decodes ANY byte sequence without
// replacement runes, so once the chain reaches it, it always succeeds —
// Western pages without a declaration still yield readable text instead of
// U+FFFD soup. Labels resolve through the shared charsetEncoding in
// charset.go; only the HTML-specific ordering lives here.
var htmlCharsetFallbackLabels = []string{"gb18030", "big5", "shift_jis", "euc-kr", "iso-8859-1"}

// htmlCharsetDetectConfidence is the minimum statistical-detection
// confidence (on chardet's 0-100 scale) allowed to override the fallback
// chain order. Calibrated empirically: genuine CJK text scores 100, while
// the Go chardet port produces plausible-but-wrong guesses at low confidence
// (a sparse GBK balance-sheet page scores Big5 at 40; a short Big5 page
// scores ISO-8859-1 at 17). Only a confident pick may jump the queue;
// anything weaker keeps the GB18030-first domain heuristic.
const htmlCharsetDetectConfidence = 90

// htmlDetectedCharset mirrors Python find_codec's chardet.detect step for
// HTML bytes that are neither valid UTF-8 nor declared: a statistical
// detector picks which fallback-chain label to try FIRST, returned as one
// of htmlCharsetFallbackLabels. This matters because GB18030 maps nearly
// every byte sequence without replacement runes, so the ordered chain alone
// cannot recognize an undeclared Big5 / Shift-JIS / EUC-KR page — it would
// "decode" it as wrong Chinese text. Detection runs over the first 1024
// bytes, exactly like Python, but unlike Python it is gated on
// htmlCharsetDetectConfidence: the Go port's low-confidence guesses are
// demonstrably wrong often enough that they must not override the chain.
// A detection that fails, scores below the gate, or names an encoding the
// chain does not ship (e.g. windows-1252) returns "" and the chain order
// stands.
func htmlDetectedCharset(data []byte) string {
	sample := data
	if len(sample) > 1024 {
		sample = sample[:1024]
	}
	res, err := chardet.NewTextDetector().DetectBest(sample)
	if err != nil || res == nil || res.Charset == "" || res.Confidence < htmlCharsetDetectConfidence {
		return ""
	}
	detected := canonicalCharsetLabel(res.Charset)
	// The detector reports the GBK family as GB2312 / GBK; GB18030 is the
	// superset we ship, so both map onto the gb18030 candidate.
	if detected == "gb2312" || detected == "gbk" {
		detected = "gb18030"
	}
	for _, label := range htmlCharsetFallbackLabels {
		if canonicalCharsetLabel(label) == detected {
			return label
		}
	}
	return ""
}

// htmlMetaCharsetRe captures the charset label of a <meta> tag, matching both
// the <meta charset="..."> form and the http-equiv content-type form
// (<meta ... content="text/html; charset=...">), over the same first-1024-byte
// window x/net's prescan consumes.
var htmlMetaCharsetRe = regexp.MustCompile(`(?i)<meta[^>]*charset\s*=\s*["']?\s*([a-z0-9._+\-]+)`)

// htmlDeclaresWindows1252 reports whether data's <meta> tags declare
// windows-1252, either directly or via a WHATWG alias such as iso-8859-1 /
// us-ascii that charset.Lookup canonicalizes to windows-1252.
// charset.DetermineEncoding cannot tell a declared windows-1252 from its own
// "nothing declared" default: a meta prescan hit never sets certain (only
// BOMs do), so both come back as (windows-1252, certain=false). The
// declaration must therefore be confirmed explicitly, or a declared
// windows-1252 page is mis-decoded by the fallback chain: windows-1252's
// 0x80-0x9F range (smart quotes, €, ™) is C1 control characters in the
// chain's ISO-8859-1 terminal.
func htmlDeclaresWindows1252(data []byte) bool {
	if len(data) > 1024 {
		data = data[:1024]
	}
	for _, m := range htmlMetaCharsetRe.FindAllSubmatch(data, -1) {
		if _, name := charset.Lookup(string(m[1])); name == "windows-1252" {
			return true
		}
	}
	return false
}

// decodeHTMLToUTF8 converts non-UTF-8 HTML bytes to UTF-8 and reports the
// encoding label used. Valid UTF-8 passes through untouched. Otherwise the
// document's own BOM or <meta charset> declaration wins — including a
// declared windows-1252: DetermineEncoding returns that name both for a real
// declaration and as its "nothing declared" default, with certain=false
// either way, so htmlDeclaresWindows1252 disambiguates the two. Undeclared
// documents first honor a high-confidence statistical pick
// (htmlDetectedCharset) and then walk the fallback chain
// (htmlCharsetFallbackLabels via the shared decodeFirstCharsetMatch), where
// each candidate must decode the whole blob without producing replacement
// runes. If everything fails the original bytes are returned so behavior
// never regresses below the previous UTF-8-only parse.
func decodeHTMLToUTF8(data []byte) ([]byte, string) {
	if len(data) == 0 || utf8Valid(data) {
		return data, "utf-8"
	}
	// The prescan declaration wins for every name except the undeclared
	// windows-1252 sentinel: skipping the sentinel keeps undeclared pages on
	// the detect-then-chain path, while a declared windows-1252 (or its
	// iso-8859-1 / us-ascii aliases) is honored.
	if enc, name, _ := charset.DetermineEncoding(data, ""); enc != nil &&
		(name != "windows-1252" || htmlDeclaresWindows1252(data)) {
		if decoded, err := decodeTransform(data, enc.NewDecoder()); err == nil {
			return []byte(decoded), name
		}
	}
	if label := htmlDetectedCharset(data); label != "" {
		if decoded, err := decodeWithCharset(data, label); err == nil {
			return []byte(decoded), label
		}
	}
	if decoded, label, ok := decodeFirstCharsetMatch(data, htmlCharsetFallbackLabels); ok {
		return []byte(decoded), label
	}
	// Unreachable for non-empty input: the terminal ISO-8859-1 candidate
	// decodes every byte sequence without replacement runes, so the loop
	// above always returns first (the label is therefore neutral rather
	// than a bogus "utf-8" — the bytes are non-UTF-8 by construction).
	// Kept as a never-worse-than-before safety net.
	return data, ""
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
		case "table":
			// Emit the <table> as a SINGLE structured doc_type_kwd:"table"
			// item, in document order. Keeping the full <table>…</table>
			// markup (not flattened) preserves row/column structure for
			// embedding, retrieval, and LLM rendering, and doc_type_kwd/
			// ck_type drives downstream table handling (discrete chunk +
			// table context). We emit ONLY this item — no duplicate
			// doc_type_kwd:"text" copy — so the table is embedded once and
			// its markup does not pollute neighbouring prose chunks.
			markup := renderTableHTML(child)
			if strings.TrimSpace(markup) != "" {
				*out = append(*out, map[string]any{
					"text":         markup,
					"doc_type_kwd": "table",
					"ck_type":      "table",
				})
			}
			continue
		}
		ckType := htmlTagToCkType(tag)
		trim := tag != "pre" && tag != "textarea"
		htmlLeafText(child, out, ckType, trim)
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

// htmlLeafText joins the visible text of an HTML node and its
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
func htmlLeafText(n *html.Node, out *[]map[string]any, ckType string, trim bool) {
	var b bytes.Buffer
	w := &leafWriter{b: &b}
	walkHTMLLeaf(n, w, out)
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

func walkHTMLLeaf(n *html.Node, w *leafWriter, out *[]map[string]any) {
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
				walkHTMLLeaf(child, w, out)
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
			markup := renderTableHTML(n)
			if strings.TrimSpace(markup) != "" {
				flushLeafText(w, out, "text", true)
				*out = append(*out, map[string]any{
					"text":         markup,
					"doc_type_kwd": "table",
					"ck_type":      "table",
				})
			}
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
			walkHTMLLeaf(child, w, out)
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
