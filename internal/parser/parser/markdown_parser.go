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
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	markdownlib "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	mdparser "github.com/gomarkdown/markdown/parser"
)

// dataURIPrefix is the MIME prefix for data URI images.
const dataURIPrefix = "data:image/"

// GoMarkdown is the lib_type identifier for the pure-Go Markdown backend.
const GoMarkdown = "go_markdown"

// ssrfAllowLoopback lets tests exercise the HTTP image fetch path against a
// loopback httptest server. It stays false in production so loopback
// addresses are rejected (SSRF protection).
var ssrfAllowLoopback bool

type MarkdownParser struct {
	libType            string
	ParseMethod        string
	OutputFormat       string
	VLM                map[string]any
	FlattenMediaToText bool
	// FetchRemoteImages controls whether HTTP(S) Markdown images are
	// downloaded. NewMarkdownParser enables it for direct Markdown parsing;
	// callers normalizing untrusted backend responses may disable it.
	FetchRemoteImages bool
}

func NewMarkdownParser(libType string) (*MarkdownParser, error) {
	switch libType {
	case GoMarkdown:
		return &MarkdownParser{
			libType:           GoMarkdown,
			FetchRemoteImages: true,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported Markdown library type: %s", libType)
	}
}

func (p *MarkdownParser) ConfigureFromSetup(setup map[string]any) {
	if p == nil || setup == nil {
		return
	}
	if v, ok := setup["parse_method"].(string); ok && v != "" {
		p.ParseMethod = v
	}
	if v, ok := setup["output_format"].(string); ok && v != "" {
		p.OutputFormat = v
	}
	if v, ok := setup["vlm"].(map[string]any); ok {
		p.VLM = v
	}
	if v, ok := setup["flatten_media_to_text"].(bool); ok {
		p.FlattenMediaToText = v
	}
}

// ParseWithResult implements ParseResultProducer (plan §6.5) and
// returns a structured Markdown payload that mirrors the Python
// parser's `output_format == "json"` shape. Each top-level block
// emits one item with `text` + `doc_type_kwd: "text"`. When the
// block contains a Markdown image reference (![alt](src)), the image
// data is resolved and the item carries `doc_type_kwd: "image"` with
// the base64-encoded image payload. The legacy debug-print path has
// been removed; callers consume ParseResult directly.
func (p *MarkdownParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	decoded, _ := DecodeToUTF8(data, "text/markdown")
	rawText := string(decoded)
	// Render any GFM/HTML table inline as an HTML block before parsing. This
	// keeps the document as one item per top-level block (the table becomes a
	// normal text item) instead of collapsing the whole document into a single
	// item. The result mirrors Python's `_markdown` (separate_tables=False),
	// which also inlines tables into the surrounding text. When no table is
	// present renderMarkdownTablesInlineText returns the input unchanged.
	rendered := renderMarkdownTablesInlineText(rawText)

	doc := markdownNew().Parse([]byte(rendered))

	var items []map[string]any
	walkMarkdownBlocksWithImages(doc, &items, p.FlattenMediaToText, p.FetchRemoteImages)
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name": filename,
		},
		JSON: items,
	}
}

func (p *MarkdownParser) String() string {
	return "MarkdownParser"
}

// markdownNew is a thin constructor so the extension set is owned
// in one place (both Parse and ParseWithResult consume it).
func markdownNew() *mdparser.Parser {
	// MathJax is masked out on purpose. CommonExtensions enables `$...$`
	// inline math, whose payload lives on ast.Math rather than on a Text
	// child; a real-world document that uses `$` as a currency symbol (a
	// billing report, a price list) therefore had every span between two
	// `$` silently deleted from the parsed text - with two far-apart `$`
	// the whole middle of the document disappears. Python's markdown
	// library has no math extension, so masking it also keeps the Go and
	// Python parsers in parity. walkLeaf still handles ast.Math /
	// ast.MathBlock defensively so that turning the extension back on
	// cannot lose text again.
	// OrderedListStart is not part of CommonExtensions, and without it the parser leaves
	// ast.List.Start at zero for every ordered list: listEntries then falls back to 1 and
	// renumbers the entries 1..N, so a list that the source spells "996. graphql: ..." is
	// indexed as "1. graphql: ...". The numbers are then gone from the index (a query for
	// "996" cannot match anything, and the gap counts against coverage), while Python's
	// _extract_list_block keeps the raw lines - markers included - verbatim. Enabling it
	// makes Start carry the number the source wrote (gomarkdown normalises a literal 1 to 0,
	// which the fallback in listEntries turns back into 1).
	extensions := mdparser.CommonExtensions&^mdparser.MathJax | mdparser.AutoHeadingIDs | mdparser.NoEmptyLineBeforeBlock | mdparser.OrderedListStart
	return mdparser.NewWithExtensions(extensions)
}

func renderMarkdownTablesInline(text string) (string, bool) {
	lines := strings.SplitAfter(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	var buf strings.Builder
	changed := false
	inFence := false
	var fenceChar byte
	var fenceLen int

	for i := 0; i < len(lines); {
		line := strings.TrimRight(lines[i], "\n")
		if ch, n, ok := markdownFenceMarker(line); ok {
			if inFence && ch == fenceChar && n >= fenceLen {
				inFence = false
			} else if !inFence {
				inFence = true
				fenceChar = ch
				fenceLen = n
			}
			buf.WriteString(lines[i])
			i++
			continue
		}
		if !inFence && i+1 < len(lines) && isMarkdownTableRow(line) && isMarkdownTableSeparator(strings.TrimRight(lines[i+1], "\n")) {
			start := i
			i += 2
			for i < len(lines) && isMarkdownTableRow(strings.TrimRight(lines[i], "\n")) {
				i++
			}
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
			tableHTML := markdownlib.ToHTML([]byte(strings.Join(balanceTableColumns(lines[start:i]), "")), markdownNew(), nil)
			// Wrap the inlined <table> HTML in blank lines so gomarkdown
			// keeps it as a single HTML block (one item) instead of
			// re-parsing it into scattered cell text. See
			// PARSER_ALIGNMENT_HANDOFF.md §3.1 (Markdown session A, 方案 Y).
			ensureTrailingBlankLine(&buf)
			buf.WriteString(strings.TrimRight(string(tableHTML), "\r\n"))
			buf.WriteString("\n\n")
			changed = true
			continue
		}
		buf.WriteString(lines[i])
		i++
	}
	return buf.String(), changed
}

// balanceTableColumns pads the header row and its separator up to the widest row's cell
// count, and returns the lines unchanged when no row is wider.
//
// This is a deliberate deviation from GFM, which ignores every cell beyond the header's
// column count. The corpus is Wikipedia-derived and its rows carry unescaped pipes inside a
// cell - an image spec such as "150x150px|alt=..." - so a row legitimately has more cells
// than its header, and GFM therefore drops the tail of the row without a word. On a list of
// World Heritage Sites that tail is the Year and Description columns: the words are in the
// file and nowhere in the index. Measured on the production index, tables of this shape cost
// 514 documents and 18,185 words that no query can reach.
//
// Padding the header keeps every cell, at the price of a few empty <th> elements. The
// alternative - matching GFM exactly - is to keep losing the text, and the indexed text is
// what this parser exists to preserve.
func balanceTableColumns(tableLines []string) []string {
	if len(tableLines) < 2 {
		return tableLines
	}
	cells := func(line string) int {
		s := strings.TrimRight(line, "\n")
		pipes, escaped := 0, false
		for i := 0; i < len(s); i++ {
			switch {
			case escaped:
				escaped = false
			case s[i] == '\\':
				escaped = true
			case s[i] == '|':
				pipes++
			}
		}
		trimmed := strings.TrimSpace(s)
		if strings.HasPrefix(trimmed, "|") {
			pipes--
		}
		if strings.HasSuffix(trimmed, "|") {
			pipes--
		}
		if pipes < 0 {
			return 0
		}
		return pipes + 1
	}

	head := cells(tableLines[0])
	widest := head
	for _, line := range tableLines[1:] {
		if n := cells(line); n > widest {
			widest = n
		}
	}
	if widest <= head {
		return tableLines
	}
	add := widest - head

	out := make([]string, len(tableLines))
	copy(out, tableLines)
	pad := func(line string, separator bool) string {
		nl := ""
		if strings.HasSuffix(line, "\n") {
			nl, line = "\n", strings.TrimRight(line, "\n")
		}
		cell := " |"
		if separator {
			cell = "---|"
		}
		line = strings.TrimRight(line, " \t")
		if !strings.HasSuffix(line, "|") {
			line += "|"
		}
		return line + strings.Repeat(cell, add) + nl
	}
	out[0] = pad(out[0], false)
	out[1] = pad(out[1], true)
	return out
}

// renderMarkdownTablesInlineText renders every GFM/HTML table inline as an
// HTML block and returns the rewritten text. When no table is present the
// input is returned unchanged. Unlike renderMarkdownTablesInline it always
// returns the full text (ignoring the changed flag) so callers can parse the
// result uniformly and emit one item per top-level block.
func renderMarkdownTablesInlineText(text string) string {
	out, _ := renderMarkdownTablesInline(text)
	return out
}

// ensureTrailingBlankLine makes sure b ends with a blank line (two
// newlines) so the next block is separated from what precedes it. gomarkdown
// only treats a <table> as a standalone HTML block (rather than re-parsing it
// into scattered cell nodes) when it is surrounded by blank lines.
func ensureTrailingBlankLine(b *strings.Builder) {
	s := b.String()
	switch {
	case strings.HasSuffix(s, "\n\n"):
		// already separated.
	case strings.HasSuffix(s, "\n"):
		b.WriteByte('\n')
	default:
		b.WriteString("\n\n")
	}
}

func markdownFenceMarker(line string) (byte, int, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return 0, 0, false
	}
	ch := trimmed[0]
	if ch != '`' && ch != '~' {
		return 0, 0, false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	return ch, n, n >= 3
}

func isMarkdownTableRow(line string) bool {
	cells := markdownTableCells(line)
	if len(cells) < 2 {
		return false
	}
	for _, cell := range cells {
		if strings.TrimSpace(cell) != "" {
			return true
		}
	}
	return false
}

func isMarkdownTableSeparator(line string) bool {
	cells := markdownTableCells(line)
	if len(cells) < 2 {
		return false
	}
	for _, cell := range cells {
		cell = strings.ReplaceAll(strings.TrimSpace(cell), " ", "")
		if cell == "" {
			return false
		}
		cell = strings.TrimPrefix(strings.TrimSuffix(cell, ":"), ":")
		if strings.Trim(cell, "-") != "" || !strings.Contains(cell, "-") {
			return false
		}
	}
	return true
}

func markdownTableCells(line string) []string {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, "|") {
		return nil
	}
	line = strings.TrimPrefix(strings.TrimSuffix(line, "|"), "|")
	return strings.Split(line, "|")
}

// walkMarkdownBlocksWithImages emits one normalized item per
// top-level block. Headings, paragraphs, lists, and code blocks are
// emitted with their text. When a block contains a Markdown image
// reference (![alt](src)), the image data is resolved via
// findBlockImage (per-block AST walk) and the item carries
// `doc_type_kwd: "image"` together with the base64-encoded image
// payload. When flatten is true, all items are forced to
// doc_type_kwd="text" (mirrors Python parser.py:1034
// flatten_media_to_text).
//
// Tables: a GFM/HTML table is rendered inline as a single <table> HTML
// block by renderMarkdownTablesInlineText and kept as one HTML block.
// It is emitted as ONE structured item (doc_type_kwd:"table",
// ck_type:"table") in its original document position — there is no
// duplicate doc_type_kwd:"text" copy. The downstream chunker consumes
// doc_type_kwd:"table" to keep the table whole and attach table context
// to neighbouring chunks (chunker/token.go). Non-table HTML blocks
// (<div>, <style>, …) are emitted as ordinary text with no ck_type.
func walkMarkdownBlocksWithImages(doc ast.Node, out *[]map[string]any, flatten, fetchRemoteImages bool) {
	for _, child := range doc.GetChildren() {
		var ckType string
		var docTypeKwd string
		var txt string

		switch n := child.(type) {
		case *ast.Heading:
			txt = headingText(n)
			ckType = "heading"
			docTypeKwd = "text"
		case *ast.Paragraph:
			txt = leafText(n)
			ckType = "text"
			docTypeKwd = "text"
		case *ast.List:
			// Python's _markdown keeps one item per list entry, marker included
			// ("- Eat a light diet before the exam?"), so a list never collapses into one blob.
			// Emit the same shape: the reconstructed marker plus the entry's own text, one item
			// each. The boundaries matter beyond alignment - the marker lives in the list node
			// rather than in any leaf literal, and concatenating entries glued their words
			// ("ChinaChina"), which destroys the word boundaries the tokenizer and the retrieval
			// index depend on.
			for _, entry := range listEntries(n) {
				*out = append(*out, map[string]any{
					"text":         entry,
					"doc_type_kwd": "text",
					"ck_type":      "list",
				})
			}
			continue
		case *ast.CodeBlock:
			txt = leafText(n)
			ckType = "code"
			docTypeKwd = "text"
		case *ast.HTMLBlock:
			// An HTML block is either an inlined table (kept as a single
			// HTML block thanks to the blank lines renderMarkdownTablesInline
			// wraps around it) or a plain HTML block such as <div>/<style>.
			// Only a table is emitted as a structured table item; everything
			// else is treated as ordinary text (no ck_type). We emit exactly
			// ONE item (doc_type_kwd:"table"/ck_type:"table") in document
			// order — no duplicate doc_type_kwd:"text" copy — so the table is
			// embedded once and its markup does not pollute prose chunks.
			txt = leafText(n)
			if isTableHTML(txt) {
				*out = append(*out, map[string]any{
					"text":         txt,
					"doc_type_kwd": "table",
					"ck_type":      "table",
				})
				continue
			}
			// Non-table HTML block: ordinary text, no ck_type.
			docTypeKwd = "text"
		default:
			txt = leafText(n)
			if strings.TrimSpace(txt) == "" {
				continue
			}
			docTypeKwd = "text"
		}

		item := map[string]any{
			"text":         txt,
			"doc_type_kwd": docTypeKwd,
		}
		if ckType != "" {
			item["ck_type"] = ckType
		}

		// Resolve Markdown images from the AST node of THIS block only, so
		// the image payload (and doc_type_kwd:"image") is attached to the
		// single block that actually contains the ![alt](src) reference.
		// Scanning the whole document (the old approach) wrongly tagged
		// every block as an image whenever any image was present. When
		// flatten is true, keep doc_type_kwd="text" (Python
		// parser.py:1034: flatten_media_to_text overrides image).
		if imgURL, ok := findBlockImage(child); ok {
			isRemote := strings.HasPrefix(imgURL, "http://") || strings.HasPrefix(imgURL, "https://")
			if fetchRemoteImages || !isRemote {
				if imgData, resolved := resolveImageURL(imgURL); resolved && imgData != "" {
					item["image"] = imgData
					if !flatten {
						item["doc_type_kwd"] = "image"
					}
				}
			}
		}

		*out = append(*out, item)
	}
}

// isTableHTML reports whether block text is an outer <table> element (the
// inlined GFM/HTML table). Only such blocks are emitted as structured table
// items; other raw HTML (e.g. <div>, <style>) is plain text.
func isTableHTML(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(s)), "<table")
}

// findBlockImage returns the destination URL of the first image node found
// anywhere under n. This associates an image with the specific block that
// contains it, instead of scanning the whole document (which would wrongly
// tag every block as an image when any image is present).
func findBlockImage(n ast.Node) (string, bool) {
	found := false
	var url string
	var walk func(c ast.Node)
	walk = func(c ast.Node) {
		if found || c == nil {
			return
		}
		if img, ok := c.(*ast.Image); ok {
			url = string(img.Destination)
			found = true
			return
		}
		for _, ch := range c.GetChildren() {
			walk(ch)
		}
	}
	walk(n)
	return url, found
}

// resolveImageURL resolves a Markdown image URL to its base64-encoded data.
// Supports:
//   - data:image/... URIs → decoded directly
//   - http:// / https:// URLs → fetched (with basic SSRF filtering)
//
// Local / relative paths are not fetched (security). Returns
// (base64String, true) on success, ("", false) when resolution fails.
func resolveImageURL(imageURL string) (string, bool) {
	if strings.HasPrefix(imageURL, dataURIPrefix) {
		// data:image/png;base64,xxxx
		idx := strings.Index(imageURL, "base64,")
		if idx < 0 {
			return "", false
		}
		return imageURL[idx+len("base64,"):], true
	}
	if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		b64, err := fetchImageAsBase64(imageURL)
		if err != nil {
			return "", false
		}
		return b64, true
	}
	// Local / relative paths — not fetched for security.
	return "", false
}

// fetchImageAsBase64 fetches an HTTP(S) image URL and returns its
// content as a base64-encoded string. Local/private addresses and
// redirects to them are rejected (SSRF guard). Hostnames are resolved
// once and the validated IP is pinned in a custom DialContext to
// prevent DNS-rebinding TOCTOU attacks.
func fetchImageAsBase64(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("markdown: invalid image URL: %w", err)
	}

	// pinned maps hostname (without port) → validated IP. The hostname
	// is resolved once per host, and the transport dials the pinned IP
	// directly instead of re-resolving DNS.
	var pinnedMu sync.Mutex
	pinned := make(map[string]net.IP)

	pinHost := func(host string) error {
		ip, err := resolveAndValidateHost(host)
		if err != nil {
			return err
		}
		h, _, _ := net.SplitHostPort(host)
		if h == "" {
			h = host
		}
		pinnedMu.Lock()
		pinned[h] = ip
		pinnedMu.Unlock()
		return nil
	}

	if err := pinHost(parsed.Host); err != nil {
		return "", err
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			pinnedMu.Lock()
			ip, ok := pinned[host]
			pinnedMu.Unlock()
			if ok {
				return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			}
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("markdown: too many redirects")
			}
			return pinHost(req.URL.Host)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("markdown: create image request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("markdown: fetch image %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("markdown: fetch image %s: HTTP %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024)) // 32 MiB cap
	if err != nil {
		return "", fmt.Errorf("markdown: read image %s: %w", rawURL, err)
	}
	return base64.StdEncoding.EncodeToString(body), nil
}

// resolveAndValidateHost resolves a host (which may include a port),
// validates none of its IPs are internal/private, and returns the
// first public IP for connection pinning.
func resolveAndValidateHost(host string) (net.IP, error) {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	ip := net.ParseIP(hostname)
	if ip != nil {
		if (ip.IsLoopback() && !ssrfAllowLoopback) || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsPrivate() || ip.IsUnspecified() {
			return nil, fmt.Errorf("markdown: rejected image URL to internal address: %s", host)
		}
		return ip, nil
	}

	addrs, err := net.DefaultResolver.LookupIPAddr(context.Background(), hostname)
	if err != nil {
		return nil, fmt.Errorf("markdown: cannot resolve image host: %s", hostname)
	}
	for _, addr := range addrs {
		ip = addr.IP
		if (ip.IsLoopback() && !ssrfAllowLoopback) || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
			ip.IsPrivate() || ip.IsUnspecified() {
			return nil, fmt.Errorf("markdown: rejected image URL resolving to internal address: %s (%s)", host, ip)
		}
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("markdown: no addresses resolved for host: %s", hostname)
	}
	return addrs[0].IP, nil
}

// headingText returns a canonical ATX heading (marker plus inline text).
// Keeping the marker in the parser payload preserves the Markdown structure
// for GeneralChunker and downstream LLM prompts; ck_type alone does not carry
// the heading level.
func headingText(h *ast.Heading) string {
	var buf bytes.Buffer
	for _, c := range h.GetChildren() {
		buf.WriteString(leafText(c))
	}
	text := strings.TrimSpace(buf.String())
	if h.Level <= 0 {
		return text
	}
	if text == "" {
		return strings.Repeat("#", h.Level)
	}
	return strings.Repeat("#", h.Level) + " " + text
}

// leafText mirrors gomarkdown's leaf walker: walks every descendant
// leaf (Text or Inline content) and returns the concatenated UTF-8.
// Non-text containers that have no leaf descendants return "".
func leafText(n ast.Node) string {
	var buf bytes.Buffer
	walkLeaf(n, &buf)
	return strings.TrimSpace(buf.String())
}

// listEntries renders a list's entries the way the source spells them: the marker first, then the
// entry text, one string per entry (Python's _markdown shape, see the en golden). The marker has to
// be reconstructed because the AST keeps it on the list node - BulletChar for bullet lists,
// Start/Delimiter for ordered ones - rather than in any leaf literal.
func listEntries(list *ast.List) []string {
	entries := make([]string, 0, len(list.GetChildren()))
	ordered := list.ListFlags&ast.ListTypeOrdered != 0
	for i, item := range list.GetChildren() {
		text := strings.TrimSpace(leafText(item))
		if text == "" {
			continue
		}
		var marker string
		switch {
		case ordered:
			start := list.Start
			if start == 0 {
				start = 1
			}
			delim := list.Delimiter
			if delim == 0 {
				delim = '.'
			}
			marker = fmt.Sprintf("%d%c ", start+i, delim)
		default:
			// The markdown parser records the bullet on the ITEM (parser/block.go sets
			// ListItem.BulletChar), not on the list, so read it there first.
			bullet := list.BulletChar
			if li, ok := item.(*ast.ListItem); ok && li.BulletChar != 0 {
				bullet = li.BulletChar
			}
			if bullet != 0 {
				marker = string(bullet) + " "
			}
		}
		entries = append(entries, marker+text)
	}
	return entries
}

// writeListItems emits a list's items separated by a newline. Without the separator the item texts
// run together ("bullet onebullet two"), because the marker lives in the list structure rather than
// in the item's leaf text - the same defect that merged table rows showed as "ChinaChina" in the
// indexed chunks.
func writeListItems(list *ast.List, buf *bytes.Buffer) {
	for i, item := range list.GetChildren() {
		if i > 0 {
			buf.WriteByte('\n')
		}
		walkLeaf(item, buf)
	}
}

func walkLeaf(n ast.Node, buf *bytes.Buffer) {
	switch t := n.(type) {
	case *ast.Text:
		buf.Write(t.Literal)
	case *ast.Code:
		buf.Write(t.Literal)
	case *ast.CodeBlock:
		// finalizeCodeBlock moves the fenced body into Literal and nils
		// Content; the indented form keeps it in Content. Emit both so the
		// code text is never dropped.
		buf.Write(t.Literal)
		buf.Write(t.Content)
	case *ast.HTMLBlock:
		// Inlined tables are HTML blocks. The generic parser path stores the
		// markup in Content, the markdown-block path in Literal. Emit both.
		buf.Write(t.Literal)
		buf.Write(t.Content)
	case *ast.Math:
		// Inline math (`$...$`) keeps its payload on the node literal, not in a
		// child Text node, so an unhandled ast.Math silently drops the whole
		// span. Emit the literal so the text can never be lost, even if the
		// MathJax extension is re-enabled (markdownNew masks it off because
		// `$` is currency in real documents far more often than it is math).
		buf.Write(t.Literal)
	case *ast.MathBlock:
		// Display math (`$$...$$`) stores its payload the same way.
		buf.Write(t.Literal)
	case *ast.HTMLSpan:
		// Inline raw HTML (a <table> that is glued to surrounding prose with no
		// blank line around it, <sub>/<sup>, <div>, …) is an HTMLSpan: the
		// markup AND the text inside it live on the node literal, with no child
		// Text nodes. Dropping it deleted whole regions of a document - a 664k
		// character CDC report lost 397k characters across 14 such spans. Emit
		// the literal (and Content for the paths that use it) so the text
		// survives verbatim, exactly as the source file spells it.
		buf.Write(t.Literal)
		buf.Write(t.Content)
	case *ast.List:
		// A list marker is structure, not a leaf: the item's own Text literal
		// carries neither the bullet nor a trailing newline, so concatenating the
		// items glued the last word of one item to the first word of the next
		// ("ChinaChina", "HathiTrustThe"). That is not a cosmetic difference: it
		// destroys the word boundaries the tokenizer and the retrieval index rely
		// on, so a query for "China" can no longer match the merged token. Python's
		// _markdown keeps the source lines, so a newline between items restores
		// parity and keeps the words separate.
		writeListItems(t, buf)
	case *ast.ListItem:
		for i, c := range t.GetChildren() {
			if i > 0 {
				buf.WriteByte('\n')
			}
			walkLeaf(c, buf)
		}
	default:
		for _, c := range n.GetChildren() {
			walkLeaf(c, buf)
		}
	}
}
