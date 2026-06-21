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

// Package component — DocsGenerator (T5).
//
// DocsGenerator is a lambda that routes by output_format to one of
// the 5 in-package writers (PDF / DOCX / TXT / Markdown / HTML). The
// Python original (agent/component/docs_generator.py) used
// pypandoc + xelatex; the Go port uses pure-Go libraries
// (signintech/gopdf, xuri/excelize, yuin/goldmark) and a
// self-implemented OOXML writer for DOCX, avoiding the AGPL-3 /
// archive / oversized-image-stack concerns of the Python
// toolchain.
//
// The component is the canvas entry point. It does NOT call MinIO;
// the produced bytes (or for HTML/MD, the rendered text) are
// surfaced on the output map for downstream nodes to attach /
// serve.
package component

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	iow "ragflow/internal/agent/component/io"
)

const componentNameDocsGenerator = "DocsGenerator"

// Default font size for the rendered documents. Plan §2.11.3 row 21
// mandates a minimum of 12pt for accessibility; we default to 12.
const defaultDocsFontSize = 12

// Default font families. Operators can register a real TTF
// asset at boot via gopdf.SetFont.
const (
	defaultPDFFontFamily    = "Noto Sans CJK SC"
	defaultDOCXFontFamily   = "Noto Sans CJK SC"
	defaultHTMLFontFamily   = "Noto Sans CJK SC"
	defaultMarkdownRenderer = "goldmark"
)

// Allowed output formats. Keep this in sync with the param.Check
// validator.
var validOutputFormats = map[string]bool{
	"pdf":      true,
	"docx":     true,
	"txt":      true,
	"markdown": true,
	"html":     true,
	"md":       true, // alias for markdown
}

// docsGeneratorParam is the static DSL param surface.
type docsGeneratorParam struct {
	OutputFormat   string `json:"output_format"`
	Content        string `json:"content"`
	Filename       string `json:"filename"`
	HeaderText     string `json:"header_text"`
	FooterText     string `json:"footer_text"`
	WatermarkText  string `json:"watermark_text"`
	AddPageNumbers bool   `json:"add_page_numbers"`
	AddTimestamp   bool   `json:"add_timestamp"`
	FontSize       int    `json:"font_size"`
}

// Update copies a fresh params map into the receiver.
func (p *docsGeneratorParam) Update(conf map[string]any) error {
	if conf == nil {
		conf = map[string]any{}
	}
	if v, ok := stringFrom(conf, "output_format"); ok {
		p.OutputFormat = v
	}
	if v, ok := stringFrom(conf, "content"); ok {
		p.Content = v
	}
	if v, ok := stringFrom(conf, "filename"); ok {
		p.Filename = v
	}
	if v, ok := stringFrom(conf, "header_text"); ok {
		p.HeaderText = v
	}
	if v, ok := stringFrom(conf, "footer_text"); ok {
		p.FooterText = v
	}
	if v, ok := stringFrom(conf, "watermark_text"); ok {
		p.WatermarkText = v
	}
	if v, ok := boolFrom(conf, "add_page_numbers"); ok {
		p.AddPageNumbers = v
	} else {
		p.AddPageNumbers = true
	}
	if v, ok := boolFrom(conf, "add_timestamp"); ok {
		p.AddTimestamp = v
	} else {
		p.AddTimestamp = true
	}
	if v, ok := intFrom(conf, "font_size"); ok {
		p.FontSize = v
	} else {
		p.FontSize = defaultDocsFontSize
	}
	return nil
}

// Check validates the param. FontSize must be ≥ 12; output_format must
// be one of pdf / docx / txt / markdown / html.
func (p *docsGeneratorParam) Check() error {
	if !validOutputFormats[strings.ToLower(strings.TrimSpace(p.OutputFormat))] {
		return &ParamError{
			Field:  "output_format",
			Reason: "must be one of: pdf, docx, txt, markdown, html",
		}
	}
	if p.FontSize < 12 {
		return &ParamError{
			Field:  "font_size",
			Reason: "must be ≥ 12",
		}
	}
	return nil
}

// AsDict returns the param as a plain map.
func (p *docsGeneratorParam) AsDict() map[string]any {
	return map[string]any{
		"output_format":    p.OutputFormat,
		"content":          p.Content,
		"filename":         p.Filename,
		"header_text":      p.HeaderText,
		"footer_text":      p.FooterText,
		"watermark_text":   p.WatermarkText,
		"add_page_numbers": p.AddPageNumbers,
		"add_timestamp":    p.AddTimestamp,
		"font_size":        p.FontSize,
	}
}

// DocsGenerator is the T5 multi-format document writer.
type DocsGenerator struct {
	name  string
	param docsGeneratorParam
}

// NewDocsGenerator builds a DocsGenerator from a DSL params map.
func NewDocsGenerator(params map[string]any) (Component, error) {
	p := &docsGeneratorParam{}
	if err := p.Update(params); err != nil {
		return nil, fmt.Errorf("DocsGenerator: param update: %w", err)
	}
	if err := p.Check(); err != nil {
		return nil, fmt.Errorf("DocsGenerator: param check: %w", err)
	}
	return &DocsGenerator{name: componentNameDocsGenerator, param: *p}, nil
}

// Name returns the registered component name.
func (d *DocsGenerator) Name() string { return d.name }

// Invoke dispatches to the appropriate writer. Input overrides for
// content / filename are honored.
func (d *DocsGenerator) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	param := d.param
	if v, ok := stringFrom(inputs, "content"); ok && v != "" {
		param.Content = v
	}
	if v, ok := stringFrom(inputs, "filename"); ok && v != "" {
		param.Filename = v
	}
	if v, ok := stringFrom(inputs, "output_format"); ok && v != "" {
		param.OutputFormat = v
	}
	// Re-check after overrides.
	if err := (&docsGeneratorParam{
		OutputFormat: param.OutputFormat,
		FontSize:     param.FontSize,
	}).Check(); err != nil {
		return nil, fmt.Errorf("DocsGenerator: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("DocsGenerator: %w", err)
	}

	format := strings.ToLower(strings.TrimSpace(param.OutputFormat))
	ext := formatExtension(format)
	safeName := sanitizeFilename(param.Filename, ext)

	var (
		payload []byte
		mime    string
	)
	switch format {
	case "pdf":
		var err error
		payload, err = iow.WritePDF(param.Content, iow.PDFOptions{
			FontSize:       param.FontSize,
			HeaderText:     param.HeaderText,
			FooterText:     param.FooterText,
			WatermarkText:  param.WatermarkText,
			AddPageNumbers: param.AddPageNumbers,
			AddTimestamp:   param.AddTimestamp,
			FontFamily:     defaultPDFFontFamily,
		})
		if err != nil {
			return nil, fmt.Errorf("DocsGenerator: pdf: %w", err)
		}
		mime = "application/pdf"
	case "docx":
		var err error
		payload, err = iow.WriteDOCX(param.Content, iow.DOCXOptions{
			HeaderText:     param.HeaderText,
			FooterText:     param.FooterText,
			WatermarkText:  param.WatermarkText,
			AddPageNumbers: param.AddPageNumbers,
			AddTimestamp:   param.AddTimestamp,
			CJKFontFamily:  defaultDOCXFontFamily,
			FontSize:       param.FontSize,
		})
		if err != nil {
			return nil, fmt.Errorf("DocsGenerator: docx: %w", err)
		}
		mime = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "txt":
		payload = iow.WriteTXT(param.Content, iow.TXTOptions{
			HeaderText:   param.HeaderText,
			FooterText:   param.FooterText,
			AddTimestamp: param.AddTimestamp,
		})
		mime = "text/plain; charset=utf-8"
	case "markdown", "md":
		// Markdown "writer" returns the original content (with optional
		// front-matter). Round-tripping Markdown → Markdown is a no-op
		// apart from header/footer/watermark rendering as comments.
		payload = iow.WriteMarkdown(param.Content, iow.MarkdownOptions{
			HeaderText:   param.HeaderText,
			FooterText:   param.FooterText,
			AddTimestamp: param.AddTimestamp,
		})
		mime = "text/markdown; charset=utf-8"
	case "html":
		payload = iow.WriteHTML(param.Content, iow.HTMLOptions{
			HeaderText:    param.HeaderText,
			FooterText:    param.FooterText,
			WatermarkText: param.WatermarkText,
			AddTimestamp:  param.AddTimestamp,
			FontSize:      param.FontSize,
			FontFamily:    defaultHTMLFontFamily,
		})
		mime = "text/html; charset=utf-8"
	}

	docID := uuid.New().String()
	size := len(payload)
	downloadStub := fmt.Sprintf("inline://docs/%s/%s", docID, safeName)

	return map[string]any{
		"doc_id":    docID,
		"filename":  safeName,
		"mime_type": mime,
		"size":      size,
		"bytes":     payload,
		"download":  downloadStub,
		"created":   time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Stream mirrors Invoke; DocsGenerator is a single-shot generator.
func (d *DocsGenerator) Stream(ctx context.Context, inputs map[string]any) (<-chan map[string]any, error) {
	out, err := d.Invoke(ctx, inputs)
	if err != nil {
		return nil, err
	}
	ch := make(chan map[string]any, 1)
	ch <- out
	close(ch)
	return ch, nil
}

// Inputs returns parameter metadata.
func (d *DocsGenerator) Inputs() map[string]string {
	return map[string]string{
		"content":       "Override: source text/markdown body (otherwise uses the static param).",
		"filename":      "Override: output filename (sanitized; extension auto-appended if missing).",
		"output_format": "Override: pdf | docx | txt | markdown | html.",
	}
}

// Outputs returns the response surface.
func (d *DocsGenerator) Outputs() map[string]string {
	return map[string]string{
		"doc_id":    "Generated document id (UUID).",
		"filename":  "Sanitized filename (extension matches output_format).",
		"mime_type": "MIME type for the payload.",
		"size":      "Payload size in bytes.",
		"bytes":     "Raw document bytes (for storage upload).",
		"download":  "Stub URI the canvas engine can resolve to a signed URL.",
		"created":   "RFC3339 timestamp of the generation.",
	}
}

// formatExtension returns the conventional file extension for a format
// string. Accepts the canonical forms and the "md" alias.
func formatExtension(format string) string {
	switch format {
	case "pdf":
		return ".pdf"
	case "docx":
		return ".docx"
	case "txt":
		return ".txt"
	case "markdown", "md":
		return ".md"
	case "html":
		return ".html"
	}
	return ""
}

// sanitizeFilename applies the plan §2.11.5 helper: strip forbidden
// chars, collapse whitespace, cap the base at 180 chars, and append the
// conventional extension when missing. Returns "file.<ext>" when the
// resulting base is empty.
func sanitizeFilename(raw, ext string) string {
	const forbidden = `\/:*?"<>|`
	const maxBase = 180
	trimmed := strings.TrimSpace(raw)
	// Strip control characters first; they're never valid in filenames.
	var b strings.Builder
	for _, r := range trimmed {
		if r < 0x20 || r == 0x7f {
			continue
		}
		if strings.ContainsRune(forbidden, r) {
			r = '_'
		}
		b.WriteRune(r)
	}
	base := strings.Join(strings.Fields(b.String()), "_")
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	if base == "" {
		return "file" + ext
	}
	if ext != "" && !strings.HasSuffix(strings.ToLower(base), strings.ToLower(ext)) {
		return base + ext
	}
	return base
}

// renderTXT / renderMarkdown / renderHTML live in
// internal/agent/component/io/{txt,markdown,html}_writer.go so
// every per-format writer lives in its own file under io/,
// matching the pdf_writer.go / docx_writer.go packaging.

func init() {
	Register(componentNameDocsGenerator, NewDocsGenerator)
}
