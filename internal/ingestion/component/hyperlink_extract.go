//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Linked-content extraction reads the outbound links a source document declares:
// DOCX relationship targets, PDF /URI strings, and the link destinations a
// CommonMark parser resolves from Markdown. The fetch post-step
// (hyperlink_fetch.go) turns what it returns into indexed pages.

package component

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Hyperlink source tags carried in chunk Extra by the fetch post-step
// (hyperlink_fetch.go). Chunkers carry the whole `extra` slot generically; they
// never name these keys.
const (
	hyperlinkSourceURLKey  = "source_url"
	hyperlinkParentDocKey  = "parent_doc_id"
	hyperlinkRelTypeSuffix = "/hyperlink"

	// hyperlinkMaxRelsBytes caps the decompressed .rels part: a crafted docx
	// can compress a gigabyte into a tiny zip entry.
	hyperlinkMaxRelsBytes = 1 << 20
	// hyperlinkMaxURIBytes caps a single extracted URI; PDF string readers
	// otherwise allocate proportional to the remaining file size.
	hyperlinkMaxURIBytes = 8 << 10
	// hyperlinkMaxPDFScanBytes caps the raw bytes examined for one PDF URI
	// string, including its opening and closing delimiters. Without it, a
	// crafted PDF full of unterminated /URI markers makes every marker rescan
	// the rest of the file (quadratic CPU).
	hyperlinkMaxPDFScanBytes = hyperlinkMaxURIBytes*2 + 2
	// hyperlinkMaxURLCandidates caps how many raw link targets extraction
	// collects and how many PDF /URI markers it processes. Only MaxURLs are
	// ever fetched, so scanning a crafted document further only burns CPU and
	// memory.
	hyperlinkMaxURLCandidates = 4096
)

// htmlAnchorHrefRe reads hrefs out of raw HTML that a Markdown document
// carries verbatim; the AST keeps those as raw-HTML nodes rather than links.
var htmlAnchorHrefRe = regexp.MustCompile(`(?i)<a\s[^>]*?href\s*=\s*["']([^"']+)["']`)

// extractLinksFromDocx returns the raw hyperlink targets declared in the
// .docx relationships part. It mirrors Python's extract_links_from_docx
// (document.part.rels filtered by the hyperlink reltype).
func extractLinksFromDocx(binary []byte) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(binary), int64(len(binary)))
	if err != nil {
		return nil, fmt.Errorf("hyperlink docx: open zip: %w", err)
	}
	var relsXML []byte
	for _, f := range zr.File {
		if f.Name != "word/_rels/document.xml.rels" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("hyperlink docx: open rels: %w", err)
		}
		relsXML, err = io.ReadAll(io.LimitReader(rc, hyperlinkMaxRelsBytes+1))
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("hyperlink docx: read rels: %w", err)
		}
		if len(relsXML) > hyperlinkMaxRelsBytes {
			return nil, fmt.Errorf("hyperlink docx: rels part exceeds %d bytes", hyperlinkMaxRelsBytes)
		}
		break
	}
	if relsXML == nil {
		return nil, nil
	}
	var rels struct {
		Relationships []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal(relsXML, &rels); err != nil {
		return nil, fmt.Errorf("hyperlink docx: parse rels: %w", err)
	}
	var out []string
	for _, r := range rels.Relationships {
		if strings.HasSuffix(r.Type, hyperlinkRelTypeSuffix) && r.Target != "" {
			out = append(out, r.Target)
			if len(out) >= hyperlinkMaxURLCandidates {
				break
			}
		}
	}
	return out, nil
}

// extractLinksFromPDF returns the raw link URIs embedded in a PDF.
//
// It is a byte-level scan: every "/URI" followed by a literal (paren) or hex
// (<hex>) string is collected. That is where a link annotation writes its
// destination, but the scan does not verify the occurrence belongs to an
// annotation's /A action — reading the real /Annots -> /A -> /URI graph needs a
// PDF object parser, and the Go pdfium bindings expose no annotation API.
//
// Every gap degrades to "fewer links", never to an error: URIs inside
// compressed object streams or encrypted PDFs are missed, a URI split across
// objects is missed, and an uncompressed content stream that happens to contain
// "/URI(" or "/URI<" is collected as though it were an annotation.
func extractLinksFromPDF(binary []byte) []string {
	var out []string
	rest := binary
	scanned := 0
	for {
		i := bytes.Index(rest, []byte("/URI"))
		if i < 0 {
			break
		}
		if scanned >= hyperlinkMaxURLCandidates {
			break
		}
		scanned++
		rest = rest[i+len("/URI"):]
		rest = skipPDFWhitespace(rest)
		if len(rest) == 0 {
			break
		}
		var uri string
		var ok bool
		switch rest[0] {
		case '(':
			uri, rest, ok = readPDFLiteralString(rest)
		case '<':
			if len(rest) > 1 && rest[1] == '<' {
				continue
			}
			uri, rest, ok = readPDFHexString(rest)
		default:
			continue
		}
		if ok && uri != "" {
			out = append(out, uri)
		}
	}
	return out
}

func skipPDFWhitespace(b []byte) []byte {
	for len(b) > 0 && (b[0] == 0 || b[0] == 9 || b[0] == 10 || b[0] == 12 || b[0] == 13 || b[0] == 32) {
		b = b[1:]
	}
	return b
}

// readPDFLiteralString parses a (...) PDF string starting at b[0].
// It resolves the escape sequences relevant to URIs (\\, \), \(, \r, \n).
// Strings longer than hyperlinkMaxURIBytes are rejected without allocating
// past the cap. The raw scan is bounded by hyperlinkMaxPDFScanBytes so a
// malformed file cannot force repeated scans to EOF.
func readPDFLiteralString(b []byte) (string, []byte, bool) {
	var sb strings.Builder
	depth := 0
	truncated := false
	limit := len(b)
	if limit > hyperlinkMaxPDFScanBytes {
		limit = hyperlinkMaxPDFScanBytes
	}
	for i := 1; i < limit; i++ {
		c := b[i]
		switch c {
		case '\\':
			if i+1 >= limit {
				return "", b, false
			}
			i++
			e := b[i]
			if sb.Len() < hyperlinkMaxURIBytes {
				switch e {
				case 'n':
					sb.WriteByte('\n')
				case 'r':
					sb.WriteByte('\r')
				case 't':
					sb.WriteByte('\t')
				case '\n':
				case '\r':
					if i+1 < limit && b[i+1] == '\n' {
						i++
					}
				default:
					sb.WriteByte(e)
				}
			} else {
				truncated = true
			}
		case '(':
			depth++
			if sb.Len() < hyperlinkMaxURIBytes {
				sb.WriteByte(c)
			} else {
				truncated = true
			}
		case ')':
			if depth == 0 {
				if truncated {
					return "", b[i+1:], false
				}
				return sb.String(), b[i+1:], true
			}
			depth--
			if sb.Len() < hyperlinkMaxURIBytes {
				sb.WriteByte(c)
			} else {
				truncated = true
			}
		default:
			if sb.Len() < hyperlinkMaxURIBytes {
				sb.WriteByte(c)
			} else {
				truncated = true
			}
		}
	}
	return "", b, false
}

// readPDFHexString parses a <hex> PDF string starting at b[0]. The search
// window is bounded by hyperlinkMaxPDFScanBytes so an unterminated marker
// cannot rescan the whole file.
func readPDFHexString(b []byte) (string, []byte, bool) {
	window := b
	if len(window) > hyperlinkMaxPDFScanBytes {
		window = window[:hyperlinkMaxPDFScanBytes]
	}
	end := bytes.IndexByte(window, '>')
	if end < 0 {
		return "", b, false
	}
	clean := make([]byte, 0, end)
	for _, c := range b[1:end] {
		if c == 0 || c == 9 || c == 10 || c == 12 || c == 13 || c == 32 {
			continue
		}
		clean = append(clean, c)
	}
	if len(clean)%2 == 1 {
		clean = append(clean, '0')
	}
	raw, err := hex.DecodeString(string(clean))
	if err != nil {
		return "", b[end+1:], false
	}
	return pdfBytesToString(raw), b[end+1:], true
}

// pdfBytesToString decodes a PDF text string: UTF-16BE with BOM, else raw
// bytes (PDFDocEncoding overlaps ASCII, which is all URIs need).
func pdfBytesToString(raw []byte) string {
	if len(raw) >= 2 && raw[0] == 0xFE && raw[1] == 0xFF {
		u16 := make([]uint16, 0, len(raw)/2)
		for i := 2; i+1 < len(raw); i += 2 {
			u16 = append(u16, uint16(raw[i])<<8|uint16(raw[i+1]))
		}
		return string(utf16.Decode(u16))
	}
	return string(raw)
}

// extractLinksFromMarkdownText returns the link destinations the CommonMark
// parser resolves from the Markdown source: inline links, badge links wrapping
// an image, reference-style links through their definitions, angle autolinks,
// and inline or block-level <a href>. Parsing the AST instead of scanning the
// text is what keeps the answer honest about what the document links to: code
// spans and fenced or indented code blocks hold no link nodes, [a](<b c>)
// destinations survive, and a reference definition nobody uses stays prose.
// Image embeds are not links, so a standalone ![alt](i.png) contributes
// nothing.
func extractLinksFromMarkdownText(source string) []string {
	src := []byte(source)
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))
	var out []string
	seen := map[string]bool{}
	add := func(dest string) bool {
		dest = strings.TrimSpace(dest)
		if dest != "" && !seen[dest] {
			seen[dest] = true
			out = append(out, dest)
		}
		return len(out) < hyperlinkMaxURLCandidates
	}
	addAnchors := func(chunk string) bool {
		for _, m := range htmlAnchorHrefRe.FindAllStringSubmatch(chunk, hyperlinkMaxURLCandidates) {
			if !add(m[1]) {
				return false
			}
		}
		return true
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Link:
			if !add(string(node.Destination)) {
				return ast.WalkStop, nil
			}
		case *ast.AutoLink:
			if !add(string(node.URL(src))) {
				return ast.WalkStop, nil
			}
		case *ast.RawHTML:
			for i := 0; i < node.Segments.Len(); i++ {
				segment := node.Segments.At(i)
				if !addAnchors(string(segment.Value(src))) {
					return ast.WalkStop, nil
				}
			}
		case *ast.HTMLBlock:
			lines := node.Lines()
			for i := 0; i < lines.Len(); i++ {
				segment := lines.At(i)
				if !addAnchors(string(segment.Value(src))) {
					return ast.WalkStop, nil
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

// canonicalLink trims one raw link target into a fetchable URL plus a dedupe
// key. The fragment is dropped (the same page behind different anchors must not
// be fetched and indexed twice) and the scheme/host are case-folded, because
// they are case-insensitive while the path and query are not. ok is false for
// anything that is not an absolute http(s) URL: the SSRF guard re-checks the
// destination at request time.
func canonicalLink(raw string) (value, key string, ok bool) {
	value = strings.TrimSpace(raw)
	if i := strings.IndexByte(value, '#'); i >= 0 {
		value = value[:i]
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if (scheme != "http" && scheme != "https") || parsed.Host == "" {
		return "", "", false
	}
	return value, scheme + "://" + strings.ToLower(parsed.Host) + parsed.RequestURI(), true
}

// normalizeAndDedupeLinks canonicalizes, drops non-fetchable targets, removes
// duplicates (case-insensitive host/scheme, fragment-insensitive) and sorts for
// a deterministic fetch order. maxURLs caps the result; distinct reports how
// many unique fetchable targets the input held and truncated says whether the
// cap cut any.
func normalizeAndDedupeLinks(urls []string, maxURLs int) (kept []string, distinct int, truncated bool) {
	seen := map[string]bool{}
	for _, raw := range urls {
		value, key, ok := canonicalLink(raw)
		if !ok || seen[key] {
			continue
		}
		seen[key] = true
		kept = append(kept, value)
	}
	sort.Strings(kept)
	if maxURLs > 0 && len(kept) > maxURLs {
		kept = kept[:maxURLs]
		truncated = true
	}
	return kept, len(seen), truncated
}

// hyperlinkTargetCollectors is the single list of families that support
// linked-content ingestion and the extractor for each. Parser Check and the
// fetch post-step both read it, so a family cannot be accepted without an
// extractor.
var hyperlinkTargetCollectors = map[string]func([]byte) ([]string, []string){
	"pdf": func(binary []byte) ([]string, []string) {
		return extractLinksFromPDF(binary), nil
	},
	"docx": func(binary []byte) ([]string, []string) {
		links, err := extractLinksFromDocx(binary)
		if err != nil {
			return nil, []string{fmt.Sprintf("hyperlink: docx link extraction failed: %v", err)}
		}
		return links, nil
	},
	"markdown": func(binary []byte) ([]string, []string) {
		return extractLinksFromMarkdownText(string(binary)), nil
	},
}

// collectHyperlinkTargets extracts raw link targets for the family from the
// source bytes. Markdown uses the raw document rather than the parsed items:
// the markdown parser resolves links to their label, so the URL is already
// gone by the time items exist.
func collectHyperlinkTargets(family string, binary []byte) ([]string, []string) {
	collector, ok := hyperlinkTargetCollectors[family]
	if !ok {
		return nil, nil
	}
	return collector(binary)
}
