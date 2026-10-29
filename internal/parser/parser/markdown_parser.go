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
	mdhtml "github.com/gomarkdown/markdown/html"
	mdparser "github.com/gomarkdown/markdown/parser"
	"golang.org/x/net/html"

	"ragflow/internal/parser/charset"
	"ragflow/internal/parser/media"
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
	if v, ok := setup["vlm"].(map[string]any); ok {
		p.VLM = v
	}
	if v, ok := setup["flatten_media_to_text"].(bool); ok {
		p.FlattenMediaToText = v
	}
}

// ParseWithResult implements ParseResultProducer (plan §6.5) and
// returns a structured JSON payload. Each top-level block
// emits one item with `text` + `doc_type_kwd: "text"`. When the
// block contains a Markdown image reference (![alt](src)), the image
// data is resolved and the item carries `doc_type_kwd: "image"` with
// the base64-encoded image payload. The legacy debug-print path has
// been removed; callers consume ParseResult directly.
func (p *MarkdownParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	state := &htmlWalkState{}
	result := p.parseWithState(ctx, filename, data, state)
	result.Warnings = append(state.warnings(), result.Warnings...)
	return result
}

// parseWithState keeps media identity and budget across fragments of one document.
// Its warnings describe this fragment; the caller adds state.warnings once.
func (p *MarkdownParser) parseWithState(ctx context.Context, filename string, data []byte, state *htmlWalkState) ParseResult {
	decoded, _ := charset.DecodeToUTF8(data, "text/markdown")
	// The final blank line also terminates a raw HTML block at EOF.
	rawText := string(decoded) + "\n\n"
	blockParser := markdownNew()
	blockParser.Block(mdparser.NormalizeNewlines([]byte(rawText)))
	// Expand tight-list tables before the inline pass. gomarkdown can also
	// end a list before its indented table, leaving uneven paragraph lines.
	ast.WalkFunc(blockParser.Doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering || node.AsContainer() == nil {
			return ast.GoToNext
		}
		var children []ast.Node
		changed := false
		for index, child := range node.GetChildren() {
			if paragraph, ok := child.(*ast.Paragraph); ok && bytes.Contains(paragraph.Content, []byte("|")) {
				_, inList := node.(*ast.ListItem)
				afterList := false
				if index > 0 {
					_, afterList = node.GetChildren()[index-1].(*ast.List)
				}
				content := paragraph.Content
				if afterList {
					content = dedentMarkdownTable(content)
				}
				if !inList && bytes.Equal(content, paragraph.Content) {
					children = append(children, child)
					continue
				}
				blocks := markdownNew()
				blocks.Block(content)
				if containsMarkdownTable(blocks.Doc) {
					children = append(children, blocks.Doc.GetChildren()...)
					changed = true
					continue
				}
			}
			children = append(children, child)
		}
		if !changed {
			return ast.GoToNext
		}
		node.SetChildren(nil)
		for _, child := range children {
			child.SetParent(nil)
			ast.AppendChild(node, child)
		}
		return ast.GoToNext
	})
	doc := blockParser.Parse(nil)

	var items []map[string]any
	if state.mediaBudget == nil {
		state.mediaBudget = newEmbeddedMediaBudget()
	}
	mediaBudget := state.mediaBudget
	imageCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	unresolvedImages := walkMarkdownBlocksWithImages(imageCtx, doc, &items, p.FlattenMediaToText, p.FetchRemoteImages, state)
	if err := ctx.Err(); err != nil {
		return ParseResult{Err: err}
	}
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	var warnings []string
	if mediaBudget.items > 0 && imageCtx.Err() == context.DeadlineExceeded && ctx.Err() == nil {
		warnings = append(warnings, "Markdown image processing reached the document time budget")
	}
	if unresolvedImages > 0 {
		warnings = append(warnings, fmt.Sprintf("Markdown parser could not resolve %d image source(s)", unresolvedImages))
	}
	return ParseResult{
		File: map[string]any{
			"name": filename,
		},
		JSON:     MapsToItems(items),
		Warnings: warnings,
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
	// ast.List.Start at zero for every ordered list: markdownListMarker then falls back to 1 and
	// renumbers the entries 1..N, so a list that the source spells "996. graphql: ..." is
	// indexed as "1. graphql: ...". The numbers are then gone from the index (a query for
	// "996" cannot match anything, and the gap counts against coverage), while Python's
	// _extract_list_block keeps the raw lines - markers included - verbatim. Enabling it
	// makes Start carry the number the source wrote (gomarkdown normalises a literal 1 to 0,
	// which markdownListMarker turns back into 1).
	extensions := mdparser.CommonExtensions&^mdparser.MathJax | mdparser.AutoHeadingIDs | mdparser.NoEmptyLineBeforeBlock | mdparser.OrderedListStart
	p := mdparser.NewWithExtensions(extensions)
	p.Opts.ParserHook = parseWideMarkdownTable
	return p
}

// Pad over-wide tables only when the Markdown parser reaches a real block.
// Fences, raw HTML, lists and indentation remain the parser's responsibility.
func parseWideMarkdownTable(data []byte) (ast.Node, []byte, int) {
	end := bytes.IndexByte(data, '\n')
	if end < 0 || bytes.HasPrefix(data, []byte("\t")) || bytes.HasPrefix(data, []byte("    ")) {
		return nil, nil, 0
	}
	header := string(data[:end+1])
	if !isMarkdownTableRow(header) {
		return nil, nil, 0
	}
	next := bytes.IndexByte(data[end+1:], '\n')
	if next < 0 {
		return nil, nil, 0
	}
	consumed := end + next + 2
	separator := string(data[end+1 : consumed])
	if !isMarkdownTableSeparator(separator) {
		return nil, nil, 0
	}
	lines := []string{header, separator}
	for consumed < len(data) {
		next = bytes.IndexByte(data[consumed:], '\n')
		if next < 0 {
			next = len(data) - consumed - 1
		}
		line := string(data[consumed : consumed+next+1])
		if !isMarkdownTableRow(line) {
			break
		}
		lines = append(lines, line)
		consumed += next + 1
	}
	balanced := strings.Join(balanceTableColumns(lines), "")
	if balanced == string(data[:consumed]) {
		return nil, nil, 0
	}
	if strings.HasPrefix(strings.TrimSpace(header), "<") {
		// The hook runs before HTML recognition. Let the library distinguish
		// raw HTML from a table whose first cell contains inline markup.
		probe := markdownNew()
		first := true
		probe.Opts.ParserHook = func(rest []byte) (ast.Node, []byte, int) {
			if first {
				first = false
				return nil, nil, 0
			}
			// The first block determines HTML priority; do not parse the suffix.
			return nil, nil, len(rest)
		}
		probe.Block(data)
		if children := probe.Doc.GetChildren(); len(children) > 0 {
			if _, table := children[0].(*ast.Table); !table {
				return nil, nil, 0
			}
		}
	}
	tableParser := markdownNew()
	tableParser.Opts.ParserHook = nil
	tableParser.Block([]byte(balanced))
	children := tableParser.Doc.GetChildren()
	if len(children) == 0 {
		return nil, nil, 0
	}
	table, ok := children[0].(*ast.Table)
	if !ok {
		return nil, nil, 0
	}
	// Keep cell Content for the document parser's inline pass and references.
	// Detach before AddBlock so ast.AppendChild does not clear the subtree.
	table.SetParent(nil)
	return table, nil, consumed
}

// List paragraphs can have a dedented first line but indented continuation
// lines. Normalize only a table paragraph with a shared continuation prefix.
func dedentMarkdownTable(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	if len(lines) < 2 || !isMarkdownTableRow(lines[0]) || !isMarkdownTableSeparator(lines[1]) {
		return data
	}
	prefix := lines[1][:len(lines[1])-len(strings.TrimLeft(lines[1], " \t"))]
	if prefix == "" {
		return data
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, prefix) {
			return data
		}
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.TrimPrefix(lines[i], prefix)
	}
	return mdparser.NormalizeNewlines([]byte(strings.Join(lines, "\n")))
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
	cells := func(line string) int { return len(markdownTableCells(line)) }

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

func isMarkdownTableRow(line string) bool {
	return len(markdownTableCells(line)) >= 2
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

// markdownTableCells follows the Markdown library's table-row rules: only
// a code span at the start of a cell can hide its internal pipes.
func markdownTableCells(line string) []string {
	line = strings.TrimSpace(line)
	var cells []string
	start := 0
	atCellStart := true
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			continue
		case '\\':
			i++
			atCellStart = false
		case '`':
			if !atCellStart {
				continue
			}
			atCellStart = false
			end := i + 1
			for end < len(line) && line[end] == '`' {
				end++
			}
			marker := line[i:end]
			if close := strings.Index(line[end:], marker); close >= 0 {
				i = end + close + len(marker) - 1
			} else {
				i = end - 1
			}
		case '|':
			if i > 0 || start > 0 {
				cells = append(cells, line[start:i])
			}
			start = i + 1
			atCellStart = true
		default:
			atCellStart = false
		}
	}
	if start == 0 {
		return nil
	}
	if start < len(line) {
		cells = append(cells, line[start:])
	}
	return cells
}

// walkMarkdownBlocksWithImages emits one normalized item per
// top-level block. Headings, paragraphs, lists, and code blocks are
// emitted with their text. When a block contains a Markdown image
// reference (![alt](src)), the image data is resolved via
// findBlockImage (per-block AST walk) and the item carries
// `doc_type_kwd: "image"` together with the base64-encoded image
// payload. When flatten is true, image items use doc_type_kwd="text".
//
// Tables are emitted through the HTML walker as structured items in source
// order, with embedded images linked to their parent table.
func walkMarkdownBlocksWithImages(ctx context.Context, doc ast.Node, out *[]map[string]any, flatten, fetchRemoteImages bool, htmlState *htmlWalkState) int {
	if htmlState.mediaBudget == nil {
		htmlState.mediaBudget = newEmbeddedMediaBudget()
	}
	budget := htmlState.mediaBudget
	start := len(*out)
	defer func() {
		if flatten {
			for _, item := range (*out)[start:] {
				if item["doc_type_kwd"] == "image" {
					item["doc_type_kwd"] = "text"
				}
			}
		}
	}()
	unresolvedImages := 0
	previousResolver := htmlState.resolveImage
	htmlState.resolveImage = func(src string) (string, bool) {
		remote := strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://")
		if remote && !fetchRemoteImages {
			return "", false
		}
		encoded, raw, omitted, resolved := resolveImageURL(ctx, src, budget)
		if !resolved {
			if !omitted {
				unresolvedImages++
			}
			return "", false
		}
		if raw != nil {
			mime := http.DetectContentType(raw)
			if !strings.HasPrefix(mime, "image/") {
				mime = "image/octet-stream"
			}
			return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw), true
		}
		return strings.ToLower(src[:strings.IndexByte(src, ',')+1]) + encoded, true
	}
	defer func() { htmlState.resolveImage = previousResolver }()
	for _, child := range doc.GetChildren() {
		var layout string
		var docTypeKwd string
		var txt string

		switch n := child.(type) {
		case *ast.Table:
			markup := markdownlib.Render(n, mdhtml.NewRenderer(mdhtml.RendererOptions{}))
			tableDoc, _ := html.Parse(bytes.NewReader(markup))
			walkHTMLBlocksWithState(tableDoc, out, htmlState)
			continue
		case *ast.Heading:
			txt = headingText(n)
			layout = "heading"
			docTypeKwd = "text"
		case *ast.Paragraph:
			if containsMarkdownTable(n) {
				markup := markdownlib.Render(n, mdhtml.NewRenderer(mdhtml.RendererOptions{}))
				doc, _ := html.Parse(bytes.NewReader(markup))
				walkHTMLBlocksWithState(doc, out, htmlState)
				continue
			}
			txt = leafText(n)
			docTypeKwd = "text"
		case *ast.List:
			// Keep ordinary entries whole, and visit entries containing tables
			// as blocks so their cells are not flattened into the list text.
			for i, entry := range n.GetChildren() {
				if containsMarkdownTable(entry) {
					start := len(*out)
					unresolvedImages += walkMarkdownBlocksWithImages(ctx, entry, out, flatten, fetchRemoteImages, htmlState)
					for _, item := range (*out)[start:] {
						if item["doc_type_kwd"] == "text" && item["layout"] == nil {
							item["layout"] = "list"
						}
					}
					if len(*out) > start && (*out)[start]["doc_type_kwd"] == "text" {
						(*out)[start]["text"] = markdownListMarker(n, i, entry) + stringValue((*out)[start]["text"])
					}
					continue
				}
				text := strings.TrimSpace(leafText(entry))
				if text != "" {
					*out = append(*out, map[string]any{"text": markdownListMarker(n, i, entry) + text, "doc_type_kwd": "text", "layout": "list"})
				}
			}

			continue
		case *ast.BlockQuote:
			if containsMarkdownTable(n) {
				unresolvedImages += walkMarkdownBlocksWithImages(ctx, n, out, flatten, fetchRemoteImages, htmlState)
				continue
			}
			txt = leafText(n)
			docTypeKwd = "text"
		case *ast.CodeBlock:
			txt = leafText(n)
			layout = "code"
			docTypeKwd = "text"
		case *ast.HTMLBlock:
			txt = leafText(n)
			if isTableHTML(txt) {
				tableDoc, _ := html.Parse(strings.NewReader(txt))
				walkHTMLBlocksWithState(tableDoc, out, htmlState)
				continue
			}
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
		if layout != "" {
			item["layout"] = layout
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
				encoded, raw, omitted, resolved := resolveImageURL(ctx, imgURL, budget)
				if omitted {
					item["media_omitted"] = true
				} else if !resolved {
					unresolvedImages++
				} else {
					if raw != nil {
						encoded = base64.StdEncoding.EncodeToString(raw)
					}
					item["image"] = encoded
					item["doc_type_kwd"] = "image"
				}
			}
		}

		*out = append(*out, item)
	}
	return unresolvedImages
}

// isTableHTML reports whether block text starts with a table element.
func isTableHTML(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if !strings.HasPrefix(s, "<table") || len(s) <= len("<table") {
		return false
	}
	return strings.ContainsRune("> \t\r\n", rune(s[len("<table")]))
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

// resolveImageURL resolves one bounded Markdown image attempt. It returns
// either normalized base64 or downloaded bytes, and distinguishes a budget
// or timeout omission from an unresolved source. Local paths are never fetched.
func resolveImageURL(ctx context.Context, imageURL string, budget *embeddedMediaBudget) (encoded string, raw []byte, omitted, resolved bool) {
	if ctx.Err() != nil || !budget.reserveItem() {
		return "", nil, true, false
	}
	var size int
	if strings.HasPrefix(strings.ToLower(imageURL), dataURIPrefix) {
		idx := strings.Index(strings.ToLower(imageURL), "base64,")
		if idx < 0 {
			return "", nil, false, false
		}
		var decodedSize int64
		encoded, decodedSize, _, resolved = media.NormalizeBase64ImagePayload(imageURL[idx+len("base64,"):], media.MaxImagePayloadBytes)
		if !resolved {
			return "", nil, false, false
		}
		size = int(decodedSize)
	} else if strings.HasPrefix(imageURL, "http://") || strings.HasPrefix(imageURL, "https://") {
		var err error
		raw, err = fetchImage(ctx, imageURL)
		if err != nil {
			return "", nil, false, false
		}
		size = len(raw)
	} else {
		return "", nil, false, false
	}
	if !budget.includeReservedSize(size) {
		return "", nil, true, false
	}
	return encoded, raw, false, true
}

// fetchImage fetches a bounded HTTP(S) image payload. Local/private addresses
// and redirects to them are rejected (SSRF guard). Hostnames are resolved
// once and the validated IP is pinned in a custom DialContext to prevent
// DNS-rebinding TOCTOU attacks.
func fetchImage(ctx context.Context, rawURL string) ([]byte, error) {
	rawURL = strings.TrimSpace(rawURL)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("markdown: invalid image URL: %w", err)
	}

	// pinned maps hostname (without port) → validated IP. The hostname
	// is resolved once per host, and the transport dials the pinned IP
	// directly instead of re-resolving DNS.
	var pinnedMu sync.Mutex
	pinned := make(map[string]net.IP)

	pinHost := func(host string) error {
		ip, err := resolveAndValidateHost(ctx, host)
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
		return nil, err
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("markdown: create image request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("markdown: fetch image %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("markdown: fetch image %s: HTTP %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, media.MaxImagePayloadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("markdown: read image %s: %w", rawURL, err)
	}
	if len(body) > media.MaxImagePayloadBytes {
		return nil, fmt.Errorf("markdown: image %s exceeds %d-byte limit", rawURL, media.MaxImagePayloadBytes)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("markdown: image %s is empty", rawURL)
	}
	return body, nil
}

// resolveAndValidateHost resolves a host (which may include a port),
// validates none of its IPs are internal/private, and returns the
// first public IP for connection pinning.
func resolveAndValidateHost(ctx context.Context, host string) (net.IP, error) {
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

	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, hostname)
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
// for GeneralChunker and downstream LLM prompts; layout alone does not carry
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

// markdownListMarker reconstructs the marker stored on the list and item nodes.
func markdownListMarker(list *ast.List, index int, item ast.Node) string {
	if list.ListFlags&ast.ListTypeOrdered != 0 {
		start := list.Start
		if start == 0 {
			start = 1
		}
		delimiter := list.Delimiter
		if delimiter == 0 {
			delimiter = '.'
		}
		return fmt.Sprintf("%d%c ", start+index, delimiter)
	}
	bullet := list.BulletChar
	if entry, ok := item.(*ast.ListItem); ok && entry.BulletChar != 0 {
		bullet = entry.BulletChar
	}
	if bullet != 0 {
		return string(bullet) + " "
	}
	return ""
}

func containsMarkdownTable(node ast.Node) bool {
	found := false
	ast.WalkFunc(node, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}
		switch n := node.(type) {
		case *ast.Table:
			found = true
		case *ast.HTMLSpan:
			found = isTableHTML(string(n.Literal))
		case *ast.HTMLBlock:
			found = isTableHTML(string(n.Literal)) || isTableHTML(string(n.Content))
		}
		if found {
			return ast.Terminate
		}
		return ast.GoToNext
	})
	return found
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

// shouldPreserveImageReference reports whether an image source should be kept
// as a Markdown reference in text. Resolved remote images already carry an
// image payload / img_id, and data URIs must stay out of text.
func shouldPreserveImageReference(s string) bool {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	return !strings.HasPrefix(lower, "data:") &&
		!strings.HasPrefix(lower, "http://") &&
		!strings.HasPrefix(lower, "https://")
}

func walkLeaf(n ast.Node, buf *bytes.Buffer) {
	switch t := n.(type) {
	case *ast.Text:
		buf.Write(t.Literal)
	case *ast.Image:
		// Preserve the source reference so an unresolved local path survives
		// text extraction. Data URIs stay out of text to avoid indexing base64.
		if !shouldPreserveImageReference(string(t.Destination)) {
			for _, c := range t.GetChildren() {
				walkLeaf(c, buf)
			}
			return
		}
		buf.WriteString("![")
		for _, c := range t.GetChildren() {
			walkLeaf(c, buf)
		}
		buf.WriteString("](")
		buf.Write(t.Destination)
		buf.WriteByte(')')
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
