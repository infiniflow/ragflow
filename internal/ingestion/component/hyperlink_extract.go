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
	"regexp"
	"sort"
	"strings"
	"unicode/utf16"
)

// Hyperlink source tags carried in chunk Extra. Shared by the fetch
// post-step (hyperlink_fetch.go) and the chunker passthrough.
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
)

var (
	markdownLinkRe   = regexp.MustCompile(`(!?)\[[^\]]*\]\(\s*([^\s)]+)`)
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
	for {
		i := bytes.Index(rest, []byte("/URI"))
		if i < 0 {
			break
		}
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
// Strings longer than hyperlinkMaxURIBytes are rejected without allocating;
// scanning still advances to the closing paren so the caller keeps making
// progress through the file.
func readPDFLiteralString(b []byte) (string, []byte, bool) {
	var sb strings.Builder
	depth := 0
	truncated := false
	for i := 1; i < len(b); i++ {
		c := b[i]
		switch c {
		case '\\':
			if i+1 >= len(b) {
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
					if i+1 < len(b) && b[i+1] == '\n' {
						i++
					}
				default:
					sb.WriteByte(e)
				}
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
				if truncated || sb.Len() > hyperlinkMaxURIBytes {
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

// readPDFHexString parses a <hex> PDF string starting at b[0]. A hex string
// longer than the URI cap is rejected without building the decoded buffer.
func readPDFHexString(b []byte) (string, []byte, bool) {
	end := bytes.IndexByte(b, '>')
	if end < 0 {
		return "", b, false
	}
	if end-1 > hyperlinkMaxURIBytes*2 {
		return "", b[end+1:], false
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

// extractLinksFromMarkdownText returns link destinations from Markdown
// link syntax and inline <a href>, mirroring Python's hyperlink_urls
// collection (Markdown.md_to_html + get_hyperlink_urls). Image embeds
// (![...](...)) are not links and are skipped.
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
	for _, m := range markdownLinkRe.FindAllStringSubmatch(text, -1) {
		if m[1] == "!" {
			continue
		}
		dest := m[2]
		if i := strings.IndexAny(dest, " \t\n\"'"); i >= 0 {
			dest = dest[:i]
		}
		add(dest)
	}
	for _, m := range htmlAnchorHrefRe.FindAllStringSubmatch(text, -1) {
		add(m[1])
	}
	return out
}

// normalizeAndDedupeLinks trims, drops fragments (same page behind different
// anchors must not be fetched and indexed twice), rejects non-fetchable
// targets (only http(s) is ever fetched — the SSRF guard re-checks at request
// time), dedupes and sorts for a deterministic fetch order. maxURLs caps the
// result; truncated reports whether input was cut.
func normalizeAndDedupeLinks(urls []string, maxURLs int) (kept []string, truncated bool) {
	seen := map[string]bool{}
	for _, raw := range urls {
		u := strings.TrimSpace(raw)
		if i := strings.IndexByte(u, '#'); i >= 0 {
			u = u[:i]
		}
		if u == "" || seen[u] {
			continue
		}
		lower := strings.ToLower(u)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			continue
		}
		seen[u] = true
		kept = append(kept, u)
	}
	sort.Strings(kept)
	if maxURLs > 0 && len(kept) > maxURLs {
		kept = kept[:maxURLs]
		truncated = true
	}
	return kept, truncated
}
