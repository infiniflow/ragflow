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

// EPUBParser extracts text content from EPUB files. EPUB is a ZIP
// container with XHTML spine items. The parser:
//
//  1. Opens the EPUB as a ZIP archive.
//  2. Reads META-INF/container.xml to locate the OPF manifest.
//  3. Walks the OPF spine to collect content documents in order.
//  4. Emits text, tables and inline images from each chapter in document order.
//
// The output format is "json", matching the epub default in
// ParserParam.Defaults().

package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"ragflow/internal/parser/charset"
)

// EPUBParser extracts text from EPUB archives.
type EPUBParser struct{}

func NewEPUBParser() *EPUBParser {
	return &EPUBParser{}
}

func (p *EPUBParser) String() string {
	return "EPUBParser"
}

// ParseWithResult implements ParseResultProducer. It extracts XHTML
// content from the EPUB spine using the shared HTML structure walker.
func (p *EPUBParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	if len(data) == 0 {
		return ParseResult{
			File: map[string]any{
				"name":     filename,
				"size":     0,
				"encoding": "utf-8",
			},
			JSON: []DocItem{NewTextJSONItem("")},
		}
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ParseResult{Err: fmt.Errorf("epub: open zip: %w", err)}
	}

	opfPath, err := readContainerXML(reader)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("epub: container.xml: %w", err)}
	}

	spineItems, err := readOPFSpine(reader, opfPath)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("epub: opf: %w", err)}
	}

	items, detectedEnc, warnings, err := extractEPUBTextItems(reader, opfPath, spineItems)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("epub: %w", err)}
	}
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	return ParseResult{
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": detectedEnc,
		},
		JSON:     MapsToItems(items),
		Warnings: warnings,
	}
}

// --- container.xml parsing ---

type containerXML struct {
	RootFiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

func readContainerXML(r *zip.Reader) (string, error) {
	f, err := r.Open("META-INF/container.xml")
	if err != nil {
		return "", fmt.Errorf("open container.xml: %w", err)
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read container.xml: %w", err)
	}
	var c containerXML
	if err := xml.Unmarshal(raw, &c); err != nil {
		return "", fmt.Errorf("parse container.xml: %w", err)
	}
	if len(c.RootFiles) == 0 {
		return "", fmt.Errorf("no rootfile in container.xml")
	}
	return c.RootFiles[0].FullPath, nil
}

// --- OPF parsing ---

type opfPackage struct {
	Manifest struct {
		Items []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
			Type string `xml:"media-type,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		ItemRefs []struct {
			IDRef string `xml:"idref,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func readOPFSpine(r *zip.Reader, opfPath string) ([]string, error) {
	f, err := r.Open(opfPath)
	if err != nil {
		// Some EPUBs use a path with a leading slash stripped.
		clean := strings.TrimPrefix(opfPath, "/")
		if clean != opfPath {
			f, err = r.Open(clean)
			if err != nil {
				return nil, fmt.Errorf("open opf %q: %w", opfPath, err)
			}
		} else {
			return nil, fmt.Errorf("open opf %q: %w", opfPath, err)
		}
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read opf: %w", err)
	}
	var pkg opfPackage
	if err := xml.Unmarshal(raw, &pkg); err != nil {
		return nil, fmt.Errorf("parse opf: %w", err)
	}

	// Build id → href lookup.
	byID := make(map[string]string, len(pkg.Manifest.Items))
	for _, it := range pkg.Manifest.Items {
		byID[it.ID] = it.Href
	}

	// Build spine item list (ordered hrefs).
	var hrefs []string
	for _, ref := range pkg.Spine.ItemRefs {
		if href, ok := byID[ref.IDRef]; ok {
			hrefs = append(hrefs, href)
		}
	}
	return hrefs, nil
}

// --- text extraction ---

var epubSpaceEntity = regexp.MustCompile(`&(?:nbsp|#0*160|#[xX]0*[aA]0);`)

// epubSelfClosingRawText matches a self-closing non-void XHTML element whose
// content HTML parsers treat as raw text. HTML5 parsing turns <script src="x"/>
// into an unclosed start tag, so the rest of the chapter becomes script text
// and the walker drops it. Restoring the end tag keeps the chapter body.
var epubSelfClosingRawText = regexp.MustCompile(`(?is)<(script|style|title|textarea)(\s[^<>]*?)?\s*/>`)

func normalizeEPUBSelfClosing(source string) string {
	return epubSelfClosingRawText.ReplaceAllString(source, "<$1$2></$1>")
}

// extractEPUBTextItems reads chapters in spine order and preserves their blocks.
// An item that cannot be read is skipped; when items were
// skipped and no other item could be read, it returns an error instead of an
// empty result. An item that holds no text (an image-only chapter) still counts
// as read, and an empty item counts as neither.
func extractEPUBTextItems(r *zip.Reader, opfDir string, spineHrefs []string) ([]map[string]any, string, []string, error) {
	var items []map[string]any
	var failures []string
	read := 0
	state := &htmlWalkState{}
	detectedEncoding := "utf-8"
	for _, href := range spineHrefs {
		raw, err := readEPUBContentFile(r, opfDir, href)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", href, err))
			continue
		}
		if len(raw) == 0 {
			continue
		}
		read++
		decoded, enc := charset.DecodeToUTF8(raw, "application/xhtml+xml")
		if detectedEncoding == "utf-8" && enc != "" && enc != "utf-8" {
			detectedEncoding = enc
		}
		// Retain chapter text normalization for named quotes and spaces.
		source := epubSpaceEntity.ReplaceAllString(string(decoded), " ")
		source = strings.NewReplacer("&lsquo;", "'", "&rsquo;", "'", "&ldquo;", "\"", "&rdquo;", "\"").Replace(source)
		source = normalizeEPUBSelfClosing(source)
		doc, err := html.Parse(strings.NewReader(source))
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", href, err))
			continue
		}
		items = appendEPUBChapter(items, doc, state)
	}
	if len(failures) > 0 && read == 0 {
		return nil, detectedEncoding, state.warnings(), fmt.Errorf("no readable content: %d of %d content items could not be read (%s)", len(failures), len(spineHrefs), failures[0])
	}
	return items, detectedEncoding, state.warnings(), nil
}

// appendEPUBChapter keeps continuous chapter prose together, splitting only
// at tables and images. The shared leaf walker preserves their source order.
func appendEPUBChapter(items []map[string]any, doc *html.Node, state *htmlWalkState) []map[string]any {
	var chapter []map[string]any
	for root := doc.FirstChild; root != nil; root = root.NextSibling {
		if root.Type != html.ElementNode {
			continue
		}
		for part := root.FirstChild; part != nil; part = part.NextSibling {
			htmlLeafTextWithState(part, &chapter, "text", true, state)
		}
	}
	start := len(items)
	for _, item := range chapter {
		if item["doc_type_kwd"] != "text" {
			items = append(items, item)
			continue
		}
		text := strings.Join(strings.FieldsFunc(stringValue(item["text"]), isCollapsibleWS), " ")
		if text == "" {
			continue
		}
		if len(items) > start && items[len(items)-1]["doc_type_kwd"] == "text" {
			items[len(items)-1]["text"] = stringValue(items[len(items)-1]["text"]) + " " + text
		} else {
			items = append(items, map[string]any{"text": text, "doc_type_kwd": "text"})
		}
	}
	return items
}

// readEPUBContentFile resolves a spine href (relative to the OPF
// directory) inside the ZIP and returns the item's raw bytes.
func readEPUBContentFile(r *zip.Reader, opfDir, href string) ([]byte, error) {
	// Manifest hrefs are URLs: drop the fragment and decode %-escapes to get
	// the ZIP entry name.
	if i := strings.IndexByte(href, '#'); i >= 0 {
		href = href[:i]
	}
	if unescaped, err := url.PathUnescape(href); err == nil {
		href = unescaped
	}
	// Resolve href relative to the directory containing the OPF.
	// Use path.Join/Dir (slash separator) because ZIP entry paths are always POSIX.
	resolved := path.Join(path.Dir(opfDir), href)

	var f io.ReadCloser
	var err error
	// Try the resolved path first, then raw href.
	for _, name := range []string{resolved, href} {
		if strings.TrimSpace(name) == "" {
			continue
		}
		f, err = r.Open(name)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
