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
//  4. Strips HTML tags from each content document to produce plain
//     text items, emitting one JSON item per spine entry.
//
// The output format is "json", matching the epub default in
// ParserParam.Defaults().

package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
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
// content from the EPUB spine and emits one JSON item per spine entry
// with {text, doc_type_kwd:"text"}.
func (p *EPUBParser) ParseWithResult(filename string, data []byte) ParseResult {
	if len(data) == 0 {
		return ParseResult{
			OutputFormat: "json",
			File: map[string]any{
				"name":     filename,
				"size":     0,
				"encoding": "utf-8",
			},
			JSON: []map[string]any{{"text": "", "doc_type_kwd": "text"}},
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

	items := extractEPUBTextItems(reader, opfPath, spineItems)
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": "utf-8",
		},
		JSON: items,
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

var (
	epubScriptRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	epubStyleRe  = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	epubTagRe    = regexp.MustCompile(`<[^>]+>`)
	epubEntityRe = regexp.MustCompile(`&[a-zA-Z]+;`)
	epubWSRe     = regexp.MustCompile(`\s+`)
)

func extractEPUBTextItems(r *zip.Reader, opfDir string, spineHrefs []string) []map[string]any {
	var items []map[string]any
	for _, href := range spineHrefs {
		text := readEPUBContentFile(r, opfDir, href)
		if strings.TrimSpace(text) == "" {
			continue
		}
		items = append(items, map[string]any{
			"text":         text,
			"doc_type_kwd": "text",
		})
	}
	return items
}

// readEPUBContentFile resolves a spine href (relative to the OPF
// directory) inside the ZIP, reads the raw bytes, and strips HTML to
// return clean text.
func readEPUBContentFile(r *zip.Reader, opfDir, href string) string {
	// Resolve href relative to the directory containing the OPF.
	resolved := filepath.Join(filepath.Dir(opfDir), href)

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
		return ""
	}
	defer f.Close()

	raw, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	return stripHTMLTags(string(raw))
}

// stripHTMLTags removes HTML markup and returns normalized plain text.
func stripHTMLTags(s string) string {
	// Remove script/style blocks first (including their content).
	s = epubScriptRe.ReplaceAllString(s, "")
	s = epubStyleRe.ReplaceAllString(s, "")
	// Replace <br>, <p>, </p>, </div>, </tr>, </li> etc. with newlines.
	s = epubTagRe.ReplaceAllStringFunc(s, func(tag string) string {
		lower := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(lower, "<br"),
			strings.HasPrefix(lower, "</p"),
			strings.HasPrefix(lower, "</div"),
			strings.HasPrefix(lower, "</h1"),
			strings.HasPrefix(lower, "</h2"),
			strings.HasPrefix(lower, "</h3"),
			strings.HasPrefix(lower, "</h4"),
			strings.HasPrefix(lower, "</h5"),
			strings.HasPrefix(lower, "</h6"),
			strings.HasPrefix(lower, "</tr"),
			strings.HasPrefix(lower, "</li"),
			strings.HasPrefix(lower, "</blockquote"):
			return "\n"
		case strings.HasPrefix(lower, "</td"),
			strings.HasPrefix(lower, "</th"):
			return " "
		}
		return ""
	})
	// Decode common XML entities.
	s = epubEntityRe.ReplaceAllStringFunc(s, xmlEntityValue)
	// Normalize whitespace.
	s = epubWSRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// xmlEntityValue decodes a few common XML named entities. A full
// entity table isn't needed for the typical EPUB vocabulary.
func xmlEntityValue(entity string) string {
	switch entity {
	case "&amp;":
		return "&"
	case "&lt;":
		return "<"
	case "&gt;":
		return ">"
	case "&quot;":
		return "\""
	case "&apos;":
		return "'"
	case "&nbsp;":
		return " "
	case "&ndash;":
		return "–"
	case "&mdash;":
		return "—"
	case "&lsquo;":
		return "'"
	case "&rsquo;":
		return "'"
	case "&ldquo;":
		return "\""
	case "&rdquo;":
		return "\""
	case "&hellip;":
		return "…"
	default:
		return entity
	}
}
