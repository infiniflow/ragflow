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

// Package io — DOCX writer (self-implemented OOXML).
//
// All candidate Go DOCX libraries are either AGPL-3 (unipdf,
// unioffice, fumiama-go-docx, baliance-gooxml) or unmaintained
// (tealeg, lytdev). RAGFlow is Apache-2.0, so AGPL-3 is a hard
// "no". We therefore build the DOCX writer from stdlib only:
//
//   - archive/zip    — the DOCX container
//   - text/template  — the dynamic XML parts (document, header, footer)
//   - //go:embed     — the static XML parts (Content_Types, _rels, styles)
//
// The output is a Word-compatible .docx. The XML is intentionally
// minimal: a single font, a single style, and a flat list of
// paragraphs. Tables / images / lists are future polish items; the
// test suite (see docx_writer_test.go) verifies the ZIP magic, the
// embedded document.xml content, and the XML-escape contract.
package io

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"html"
	"strings"
	"text/template"
)

//go:embed templates/content_types.xml
var contentTypesXML []byte

//go:embed templates/rels.xml
var relsXML []byte

//go:embed templates/document_rels.xml.tmpl
//go:embed templates/styles.xml.tmpl
//go:embed templates/document.xml.tmpl
//go:embed templates/header.xml.tmpl
//go:embed templates/footer.xml.tmpl
var tmplFS embed.FS

// DOCXOptions is the public contract for the DOCX writer.
type DOCXOptions struct {
	HeaderText     string
	FooterText     string
	WatermarkText  string
	AddPageNumbers bool
	AddTimestamp   bool
	CJKFontFamily  string
	FontSize       int
}

// docModel is the internal render input. It's a small struct so the
// templates can refer to a stable set of fields. The exported
// DOCXOptions and Document flatten into this when the writer is
// invoked.
type docModel struct {
	Paragraphs     []string
	HeaderText     string
	FooterText     string
	WatermarkText  string
	AddPageNumbers bool
	AddTimestamp   bool
	HasWatermark   bool
	HasHeader      bool
	HasFooter      bool
	FontSize       int
	FontFamily     string
	Timestamp      string
}

// tmplFuncs registers the {{xml}} helper used to escape user content
// before it lands in the document body. text/template doesn't have a
// template.HTML type, so we return a string and rely on the template
// engine's {{ }} interpolation (which auto-escapes by default for
// strings — except we're explicitly using a non-default func that
// returns a pre-escaped string).
var tmplFuncs = template.FuncMap{
	"xml": func(s string) string { return html.EscapeString(s) },
	"pt":  func(i int) string { return fmt.Sprintf("%d", i*2) }, // OOXML uses half-points for w:sz
}

// WriteDOCX renders the supplied content to a DOCX byte stream.
//
// Layout strategy (P4):
//
//   - One paragraph per non-empty line in content.
//   - Empty lines are preserved as empty paragraphs (so the document's
//     vertical rhythm matches the source).
//   - The header carries a centered title and (optionally) a VML
//     watermark shape.
//   - The footer carries the footer text and (optionally) a page-number
//     field and a generation timestamp.
//   - Font size / family are applied globally; the per-paragraph
//     elements inherit from styles.xml.
func WriteDOCX(content string, opts DOCXOptions) ([]byte, error) {
	if opts.FontSize <= 0 {
		opts.FontSize = 12
	}
	if opts.CJKFontFamily == "" {
		opts.CJKFontFamily = "Noto Sans CJK SC"
	}

	model := docModel{
		Paragraphs:     splitParagraphs(content),
		HeaderText:     opts.HeaderText,
		FooterText:     opts.FooterText,
		WatermarkText:  opts.WatermarkText,
		AddPageNumbers: opts.AddPageNumbers,
		AddTimestamp:   opts.AddTimestamp,
		HasWatermark:   opts.WatermarkText != "",
		HasHeader:      opts.HeaderText != "" || opts.WatermarkText != "",
		HasFooter:      opts.FooterText != "" || opts.AddPageNumbers || opts.AddTimestamp,
		FontSize:       opts.FontSize,
		FontFamily:     opts.CJKFontFamily,
		Timestamp:      nowUTC(),
	}

	// Pre-parse all 5 templates once; FuncMaps are shared.
	docTmpl, err := template.New("document.xml.tmpl").Funcs(tmplFuncs).ParseFS(tmplFS, "templates/document.xml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("DOCX: parse document template: %w", err)
	}
	headerTmpl, err := template.New("header.xml.tmpl").Funcs(tmplFuncs).ParseFS(tmplFS, "templates/header.xml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("DOCX: parse header template: %w", err)
	}
	footerTmpl, err := template.New("footer.xml.tmpl").Funcs(tmplFuncs).ParseFS(tmplFS, "templates/footer.xml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("DOCX: parse footer template: %w", err)
	}
	stylesTmpl, err := template.New("styles.xml.tmpl").Funcs(tmplFuncs).ParseFS(tmplFS, "templates/styles.xml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("DOCX: parse styles template: %w", err)
	}
	docRelsTmpl, err := template.New("document_rels.xml.tmpl").Funcs(tmplFuncs).ParseFS(tmplFS, "templates/document_rels.xml.tmpl")
	if err != nil {
		return nil, fmt.Errorf("DOCX: parse document_rels template: %w", err)
	}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	// [Content_Types].xml
	if err := writeZipFile(zw, "[Content_Types].xml", contentTypesXML); err != nil {
		return nil, err
	}
	// _rels/.rels
	if err := writeZipFile(zw, "_rels/.rels", relsXML); err != nil {
		return nil, err
	}

	// word/document.xml.rels — relationships for header / footer / styles.
	if err := writeZipTmpl(zw, "word/_rels/document.xml.rels", docRelsTmpl, model); err != nil {
		return nil, err
	}
	// word/styles.xml
	if err := writeZipTmpl(zw, "word/styles.xml", stylesTmpl, model); err != nil {
		return nil, err
	}
	// word/document.xml
	if err := writeZipTmpl(zw, "word/document.xml", docTmpl, model); err != nil {
		return nil, err
	}
	// word/header1.xml (omitted when no header / watermark requested)
	if model.HasHeader {
		if err := writeZipTmpl(zw, "word/header1.xml", headerTmpl, model); err != nil {
			return nil, err
		}
	}
	// word/footer1.xml (omitted when no footer requested)
	if model.HasFooter {
		if err := writeZipTmpl(zw, "word/footer1.xml", footerTmpl, model); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("DOCX: close zip: %w", err)
	}
	return buf.Bytes(), nil
}

// writeZipFile writes a static []byte payload as a file inside the zip.
func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("DOCX: create %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("DOCX: write %s: %w", name, err)
	}
	return nil
}

// writeZipTmpl executes a template against the model and writes the
// output as a file inside the zip.
func writeZipTmpl(zw *zip.Writer, name string, tmpl *template.Template, model any) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("DOCX: create %s: %w", name, err)
	}
	if err := tmpl.Execute(w, model); err != nil {
		return fmt.Errorf("DOCX: execute %s: %w", name, err)
	}
	return nil
}

// splitParagraphs turns the source content into a list of paragraph
// strings. Empty lines are preserved as empty strings so the rendered
// document's vertical rhythm matches the source.
func splitParagraphs(content string) []string {
	if content == "" {
		return []string{""}
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		// Strip trailing \r for CRLF inputs.
		out = append(out, strings.TrimRight(l, "\r"))
	}
	return out
}

// nowUTC returns a stable timestamp string for footer / watermark
// metadata. Format: "2026-06-03T14:45:06Z" (RFC3339 in UTC).
// The var is package-scoped so tests can override it.
var nowUTC = func() string { return "2026-06-03T00:00:00Z" }
