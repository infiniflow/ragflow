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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
)

// mdImagePattern matches markdown inline image syntax: ![alt](url).
var mdImagePattern = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)\)`)

// dataURIPrefix is the MIME prefix for data URI images.
const dataURIPrefix = "data:image/"

// GoMarkdown is the lib_type identifier for the pure-Go markdown backend.
const GoMarkdown = "go_markdown"

// ssrfAllowLoopback lets tests exercise the HTTP image fetch path against a
// loopback httptest server. It stays false in production so loopback
// addresses are rejected (SSRF protection).
var ssrfAllowLoopback bool

type MarkdownParser struct {
	libType      string
	ParseMethod  string
	OutputFormat string
	VLM          map[string]any
}

func NewMarkdownParser(libType string) (*MarkdownParser, error) {
	switch libType {
	case GoMarkdown:
		return &MarkdownParser{
			libType: GoMarkdown,
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
}

// ParseWithResult implements ParseResultProducer (plan §6.5) and
// returns a structured markdown payload that mirrors the Python
// parser's `output_format == "json"` shape. Each top-level block
// emits one item with `text` + `doc_type_kwd: "text"`. When the
// block contains a markdown image reference (![alt](src)), the image
// data is resolved and the item carries `doc_type_kwd: "image"` with
// the base64-encoded image payload. The legacy debug-print path has
// been removed; callers consume ParseResult directly.
func (p *MarkdownParser) ParseWithResult(filename string, data []byte) ParseResult {
	doc := markdownNew().Parse(data)
	rawText := string(data)

	var items []map[string]any
	walkMarkdownBlocksWithImages(doc, rawText, &items)
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
func markdownNew() *parser.Parser {
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	return parser.NewWithExtensions(extensions)
}

// walkMarkdownBlocksWithImages emits one normalized item per
// top-level block. Headings, paragraphs, lists, and code blocks are
// emitted with their text. When a block contains a markdown image
// reference (![alt](src)), the image data is resolved via
// resolveMarkdownImage and the item carries `doc_type_kwd: "image"`
// together with the base64-encoded image payload.
func walkMarkdownBlocksWithImages(doc ast.Node, rawText string, out *[]map[string]any) {
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
			txt = leafText(n)
			ckType = "list"
			docTypeKwd = "text"
		case *ast.CodeBlock:
			txt = leafText(n)
			ckType = "code"
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

		// Detect markdown image references in the raw source text
		// that corresponds to this block. When found, resolve the
		// image data so downstream vision enhancement can describe it.
		if imgData, imgFound := resolveMarkdownImage(txt, rawText); imgFound && imgData != "" {
			item["doc_type_kwd"] = "image"
			item["image"] = imgData
		}

		*out = append(*out, item)
	}
}

// resolveMarkdownImage extracts the first markdown image reference
// from the given text and returns its base64-encoded data. Supports:
//   - data:image/... URIs → decoded directly
//   - http:// / https:// URLs → fetched (with basic SSRF filtering)
//
// Returns (base64String, true) on success, ("", false) when no image
// is found or resolution fails.
func resolveMarkdownImage(leafText, rawFullText string) (string, bool) {
	// Prefer matching against the raw full text to catch images
	// whose alt-text was split across markdown rendering.
	searchIn := rawFullText
	if strings.TrimSpace(searchIn) == "" {
		searchIn = leafText
	}
	matches := mdImagePattern.FindStringSubmatch(searchIn)
	if len(matches) < 2 {
		return "", false
	}
	url := matches[1]

	if strings.HasPrefix(url, dataURIPrefix) {
		// data:image/png;base64,xxxx
		idx := strings.Index(url, "base64,")
		if idx < 0 {
			return "", false
		}
		return url[idx+len("base64,"):], true
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		b64, err := fetchImageAsBase64(url)
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
		ip := addr.IP
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

// headingText returns the inline-text of a heading node by
// concatenating every Leaf / Text child. Empty headings emit "".
func headingText(h *ast.Heading) string {
	var buf bytes.Buffer
	for _, c := range h.GetChildren() {
		buf.WriteString(leafText(c))
	}
	return strings.TrimSpace(buf.String())
}

// leafText mirrors gomarkdown's leaf walker: walks every descendant
// leaf (Text or Inline content) and returns the concatenated UTF-8.
// Non-text containers that have no leaf descendants return "".
func leafText(n ast.Node) string {
	var buf bytes.Buffer
	walkLeaf(n, &buf)
	return strings.TrimSpace(buf.String())
}

func walkLeaf(n ast.Node, buf *bytes.Buffer) {
	switch t := n.(type) {
	case *ast.Text:
		buf.Write(t.Literal)
	case *ast.Code:
		buf.Write(t.Literal)
	default:
		for _, c := range n.GetChildren() {
			walkLeaf(c, buf)
		}
	}
}
