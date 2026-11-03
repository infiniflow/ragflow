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
//  4. Emits text, tables and image references from each chapter in document order.
//
// Chapter images are read from their ZIP entries and written to the run media
// cache; the result carries only a MediaRef. Inline data URIs take the same
// ref-only path as the HTML parser.
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

	"ragflow/internal/entity"
	"ragflow/internal/parser/charset"
	"ragflow/internal/parser/media"
)

// EPUBParser extracts text from EPUB archives.
type EPUBParser struct {
	textOnly bool
}

func NewEPUBParser() *EPUBParser {
	return &EPUBParser{}
}

// NewEPUBTextParser extracts EPUB text without reading or caching images.
// Callers without a run media cache use it to keep the previous text-only
// behavior.
func NewEPUBTextParser() *EPUBParser {
	return &EPUBParser{textOnly: true}
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

	opf, err := readOPFManifest(reader, opfPath)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("epub: opf: %w", err)}
	}

	items, detectedEnc, warnings, err := extractEPUBTextItems(ctx, reader, opf, p.textOnly)
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

// opfManifest is the parsed OPF package document.
type opfManifest struct {
	// Dir is the ZIP directory that contains the package document. Spine and
	// manifest hrefs resolve against it.
	Dir string
	// SpineHrefs lists content-document hrefs in reading order.
	SpineHrefs []string
	// Images maps a resolved ZIP entry path to the media type the package
	// declares for it.
	Images map[string]string
}

func readOPFManifest(r *zip.Reader, opfPath string) (*opfManifest, error) {
	raw, dir, err := readOPFBytes(r, opfPath)
	if err != nil {
		return nil, err
	}
	var pkg opfPackage
	if err := xml.Unmarshal(raw, &pkg); err != nil {
		return nil, fmt.Errorf("parse opf: %w", err)
	}

	// Build id → href lookup.
	byID := make(map[string]string, len(pkg.Manifest.Items))
	images := make(map[string]string)
	for _, it := range pkg.Manifest.Items {
		byID[it.ID] = it.Href
		mime := strings.ToLower(strings.TrimSpace(it.Type))
		if !strings.HasPrefix(mime, "image/") {
			continue
		}
		if entry, ok := resolveEPUBEntry(dir, it.Href); ok {
			images[entry] = mime
		}
	}

	// Build spine item list (ordered hrefs).
	var hrefs []string
	for _, ref := range pkg.Spine.ItemRefs {
		if href, ok := byID[ref.IDRef]; ok {
			hrefs = append(hrefs, href)
		}
	}
	return &opfManifest{Dir: dir, SpineHrefs: hrefs, Images: images}, nil
}

// readOPFBytes reads the package document and returns its ZIP directory. Some
// EPUBs declare the OPF path with a leading slash, so the trimmed name is
// tried as well.
func readOPFBytes(r *zip.Reader, opfPath string) ([]byte, string, error) {
	names := []string{strings.TrimSpace(opfPath)}
	if trimmed := strings.TrimPrefix(names[0], "/"); trimmed != names[0] {
		names = append(names, trimmed)
	}
	var lastErr error
	for _, name := range names {
		f, err := r.Open(name)
		if err != nil {
			lastErr = err
			continue
		}
		raw, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, "", fmt.Errorf("read opf: %w", err)
		}
		return raw, path.Dir(name), nil
	}
	return nil, "", fmt.Errorf("open opf %q: %w", opfPath, lastErr)
}

// resolveEPUBEntry resolves an EPUB href or image source to a ZIP entry path.
// baseDir is the ZIP directory of the document that references it. Remote
// URLs, absolute paths, and references that escape the archive root are
// rejected.
func resolveEPUBEntry(baseDir, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	if i := strings.IndexByte(ref, '#'); i >= 0 {
		ref = ref[:i]
	}
	unescaped, err := url.PathUnescape(ref)
	if err != nil {
		return "", false
	}
	ref = unescaped
	if ref == "" || strings.HasPrefix(ref, "/") {
		return "", false
	}
	if u, err := url.Parse(ref); err == nil && u.Scheme != "" {
		return "", false
	}
	resolved := path.Join(baseDir, ref)
	if resolved == "" || resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", false
	}
	return resolved, true
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
func extractEPUBTextItems(ctx context.Context, r *zip.Reader, opf *opfManifest, textOnly bool) ([]map[string]any, string, []string, error) {
	var items []map[string]any
	var failures []string
	read := 0
	state := &htmlWalkState{
		refOnly:     true,
		ctx:         ctx,
		mediaCache:  media.CacheFromContext(ctx),
		budget:      media.NewBudget(),
		mediaFormat: "epub",
		skipImages:  textOnly,
	}
	if textOnly {
		// Preserve alt text without resolving or caching image bytes, matching
		// NewHTMLTextParser.
		state.resolveImage = func(string) (string, bool) { return "", false }
	}
	entries := make(map[string]*zip.File, len(r.File))
	for _, f := range r.File {
		// Match zip.Reader.Open, which resolves duplicate names to the first
		// entry.
		if _, ok := entries[f.Name]; !ok {
			entries[f.Name] = f
		}
	}
	detectedEncoding := "utf-8"
	for _, href := range opf.SpineHrefs {
		if state.err != nil {
			break
		}
		raw, chapterEntry, err := readEPUBContentFile(r, opf.Dir, href)
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
		// Image sources are relative to the chapter, not to the OPF.
		state.resolveMedia = epubMediaResolver(entries, state, chapterEntry, opf.Images)
		items = appendEPUBChapter(items, doc, state)
	}
	if state.err != nil {
		return nil, detectedEncoding, state.warnings(), state.err
	}
	if len(failures) > 0 && read == 0 {
		return nil, detectedEncoding, state.warnings(), fmt.Errorf("no readable content: %d of %d content items could not be read (%s)", len(failures), len(opf.SpineHrefs), failures[0])
	}
	return items, detectedEncoding, state.warnings(), nil
}

// epubMediaResolver returns the image resolver for one chapter. src is an image
// reference relative to the chapter's ZIP entry. The entry bytes are stored in
// the run media cache and the caller receives only the reference.
//
// A source that cannot be mapped to a readable manifest image is skipped and
// counted in the shared warning. Cache failures fail the task; a run media
// limit skips the image with a warning instead.
func epubMediaResolver(entries map[string]*zip.File, state *htmlWalkState, chapterEntry string, images map[string]string) func(string) (*entity.MediaRef, bool) {
	chapterDir := path.Dir(chapterEntry)
	return func(src string) (*entity.MediaRef, bool) {
		if state.err != nil {
			return nil, false
		}
		// Inline data URIs keep the shared data-URI path.
		if media.IsDataImageSource(src) {
			return nil, false
		}
		if state.mediaCache == nil {
			state.err = fmt.Errorf("%s media cache is not configured", state.cacheFormat())
			return nil, false
		}
		name, ok := resolveEPUBEntry(chapterDir, src)
		if !ok {
			state.unresolvedImages++
			return nil, false
		}
		// The memo is shared by every chapter: one ZIP entry is read, hashed,
		// and cached once per parse, even when different chapters reference it.
		// A nil value records a source that was already rejected, so repeated
		// references do not re-read it or inflate the warning counters.
		if ref, seen := state.mediaOutput().Lookup("entry:" + name); seen {
			if ref == nil {
				return nil, false
			}
			return ref, true
		}
		mime, ok := images[name]
		if !ok {
			state.unresolvedImages++
			state.mediaOutput().Skip("entry:" + name)
			return nil, false
		}
		entry, ok := entries[name]
		if !ok {
			state.unresolvedImages++
			state.mediaOutput().Skip("entry:" + name)
			return nil, false
		}
		return state.loadEPUBMedia(name, mime, entry)
	}
}

// loadEPUBMedia applies size admission, reads one ZIP entry, and writes it to
// the run media cache. It memoizes both success and failure so a source shared
// by multiple chapters performs the work and records warnings once.
func (s *htmlWalkState) loadEPUBMedia(name, mime string, entry *zip.File) (*entity.MediaRef, bool) {
	maxBytes := media.ImageMaxBytes()
	declared := media.Size(maxBytes + 1)
	if entry.UncompressedSize64 <= uint64(maxBytes) {
		declared = media.Size(entry.UncompressedSize64)
	}
	ref, err := s.mediaOutput().Resolve("entry:"+name, declared, func() (string, []byte, bool) {
		f, err := entry.Open()
		if err != nil {
			s.unresolvedImages++
			return "", nil, false
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
		if err != nil || len(data) == 0 {
			s.unresolvedImages++
			return "", nil, false
		}
		return mime, data, true
	})
	s.err = err
	return ref, ref != nil
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

// readEPUBContentFile resolves a spine href (relative to the OPF directory)
// inside the ZIP and returns the item's raw bytes together with the entry name
// it actually opened.
func readEPUBContentFile(r *zip.Reader, opfDir, href string) ([]byte, string, error) {
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
	resolved := path.Join(opfDir, href)

	var lastErr error
	// Try the resolved path first, then raw href.
	for _, name := range []string{resolved, href} {
		if strings.TrimSpace(name) == "" {
			continue
		}
		f, err := r.Open(name)
		if err != nil {
			lastErr = err
			continue
		}
		raw, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			return nil, "", err
		}
		return raw, name, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("empty href")
	}
	return nil, "", lastErr
}
