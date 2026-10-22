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

var (
	// The label group allows one nested bracket pair so a badge link
	// `[![alt](img.png)](https://x)` still yields its destination.
	markdownLinkRe = regexp.MustCompile(`(!?)\[((?:[^\[\]]|\[[^\]]*\])*)\]\(\s*([^\s)]+)`)
	// `[label][ref]` / `[label][]` reference usages, and the `[ref]: <dest>`
	// definitions they resolve to. A definition nobody references is prose, not
	// a link in the document, so only referenced definitions are collected.
	markdownRefUseRe = regexp.MustCompile(`(!?)\[((?:[^\[\]]|\[[^\]]*\])*)\]\[([^\]]*)\]`)
	markdownRefDefRe = regexp.MustCompile(`(?m)^[ \t]*\[([^\]]+)\]:[ \t]*<?([^\s>]+)`)
	markdownAutolink = regexp.MustCompile(`(?i)<(https?://[^>\s]+)>`)
	htmlAnchorHrefRe = regexp.MustCompile(`(?i)<a\s[^>]*?href\s*=\s*["']([^"']+)["']`)
)

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
// It scans the same /A -> /URI annotation objects Python's
// extract_links_from_pdf (PyPDF2) reads, covering literal (paren) and hex
// (<hex>) string forms. It is stdlib-only on purpose: the Go pdfium
// bindings expose no annotation API, and binding one is out of scope.
//
// Known gaps: encrypted PDFs and URIs split across objects are missed;
// the caller treats that as "no links", never as an error.
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

// extractLinksFromMarkdownText returns link destinations from the Markdown
// source: inline links (including badge links wrapping an image), inline
// <a href>, CommonMark autolinks, and reference-style links resolved through
// their definitions. Image embeds (![...](...)) are not links and are skipped.
// Python collected these by rendering Markdown to HTML first; reading the
// source directly keeps the Parser free of a second renderer.
func extractLinksFromMarkdownText(text string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(url string) {
		url = strings.TrimSpace(url)
		if url == "" || seen[url] {
			return
		}
		seen[url] = true
		out = append(out, url)
	}
	capped := func() bool { return len(out) >= hyperlinkMaxURLCandidates }

	for _, m := range markdownLinkRe.FindAllStringSubmatch(text, hyperlinkMaxURLCandidates) {
		if m[1] == "!" {
			continue
		}
		dest := m[3]
		if i := strings.IndexAny(dest, " \t\n\"'"); i >= 0 {
			dest = dest[:i]
		}
		add(dest)
		if capped() {
			return out
		}
	}
	for _, m := range htmlAnchorHrefRe.FindAllStringSubmatch(text, hyperlinkMaxURLCandidates) {
		add(m[1])
		if capped() {
			return out
		}
	}
	for _, m := range markdownAutolink.FindAllStringSubmatch(text, hyperlinkMaxURLCandidates) {
		add(m[1])
		if capped() {
			return out
		}
	}

	// Reference-style links resolve through a definition list, matched
	// case-insensitively as CommonMark requires. A definition no label uses is
	// prose, not a link in the document, so it is not fetched.
	defs := map[string]string{}
	for _, m := range markdownRefDefRe.FindAllStringSubmatch(text, hyperlinkMaxURLCandidates) {
		defs[strings.ToLower(strings.TrimSpace(m[1]))] = strings.Trim(m[2], `"'`)
	}
	if len(defs) == 0 {
		return out
	}
	for _, m := range markdownRefUseRe.FindAllStringSubmatch(text, hyperlinkMaxURLCandidates) {
		if m[1] == "!" {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(m[3]))
		if id == "" {
			id = strings.ToLower(strings.TrimSpace(m[2]))
		}
		if dest, ok := defs[id]; ok {
			add(dest)
			if capped() {
				break
			}
		}
	}
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
			return nil, []string{fmt.Sprintf("hyperlink: docx link extraction failed (%v)", err)}
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
