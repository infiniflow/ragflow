//go:build cgo

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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

// DOCXFigure represents one embedded image plus its surrounding text
// context, mirroring the chunk-level shape that Python's
// naive_merge_docx produces for vision_figure_parser_docx_wrapper_naive.
type DOCXFigure struct {
	Image        string `json:"image"`         // base64-encoded image bytes
	ContextAbove string `json:"context_above"` // text before the image block
	ContextBelow string `json:"context_below"` // text after the image block
	Marker       string `json:"marker"`        // substring to locate image position in markdown
}

type DOCXParser struct {
	libType string
}

func NewDOCXParser() *DOCXParser {
	return &DOCXParser{}
}

// ParseWithResult captures the office_oxide ToMarkdown output
// and additionally extracts embedded images with their surrounding
// text context so the downstream vision-figure dispatch can enrich
// the markdown with LLM-generated image descriptions.
//
// Mirrors python naive.py: Docx() → naive_merge_docx() →
// vision_figure_parser_docx_wrapper_naive().
func (p *DOCXParser) ParseWithResult(filename string, data []byte) ParseResult {
	doc, err := officeOxide.OpenFromBytes(data, "docx")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("docx open: %w", err)}
	}
	defer doc.Close()

	md, err := doc.ToMarkdown()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("docx to-markdown: %w", err)}
	}

	fileMeta := map[string]any{
		"name":   filename,
		"format": "docx",
	}

	// Extract embedded images with their text context from the
	// office_oxide IR so the downstream vision dispatch can
	// enrich them. The already-opened doc handle is reused
	// (no second OpenFromBytes).
	irJSON, irErr := doc.ToIRJSON()
	var figures []DOCXFigure
	if irErr == nil {
		figures = extractDOCXFiguresFromIR(irJSON)
	}
	if len(figures) > 0 {
		figs := make([]map[string]any, 0, len(figures))
		for _, f := range figures {
			figs = append(figs, map[string]any{
				"image":         f.Image,
				"context_above": f.ContextAbove,
				"context_below": f.ContextBelow,
				"marker":        f.Marker,
			})
		}
		fileMeta["figures"] = figs
	}

	return ParseResult{
		OutputFormat: "markdown",
		File:         fileMeta,
		Markdown:     md,
	}
}

// extractDOCXFiguresFromIR parses the office_oxide IR JSON and
// returns every embedded image block together with the plain text
// immediately surrounding it. The context matches what Python's
// naive_merge_docx attaches as context_above / context_below on
// each chunk that carries an image.
//
// Reuses the IR already obtained from the doc handle in
// ParseWithResult so the binary is not opened twice.
func extractDOCXFiguresFromIR(irJSON string) []DOCXFigure {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil
	}

	var flat []flatBlock
	for _, sec := range ir.Sections {
		for _, el := range sec.Elements {
			if el.Type == "image" {
				b64 := base64.StdEncoding.EncodeToString(el.Data)
				flat = append(flat, flatBlock{image: b64})
				continue
			}
			text := joinDOCXIRRuns(el.Content)
			flat = append(flat, flatBlock{text: text})
		}
	}

	var figures []DOCXFigure
	for i, block := range flat {
		if block.image == "" {
			continue
		}
		fig := DOCXFigure{Image: block.image}

		// Collect text above (backward scan up to docxContextWindow
		// chars, or until another image is hit).
		above := collectDOCXPrevText(flat, i, 512)
		fig.ContextAbove = strings.TrimSpace(above)

		// Collect text below (forward scan up to docxContextWindow
		// chars, or until another image is hit).
		below := collectDOCXNextText(flat, i, 512)
		fig.ContextBelow = strings.TrimSpace(below)

		// Marker: text of the immediately preceding flat block,
		// used by the vision dispatcher to locate the image position
		// in the rendered markdown for inline insertion.
		for j := i - 1; j >= 0; j-- {
			if flat[j].text != "" {
				fig.Marker = flat[j].text
				break
			}
		}

		figures = append(figures, fig)
	}
	return figures
}

// --- internal types ---

// flatBlock is a flattened IR element used internally to collect
// text / image context around embedded figures.
type flatBlock struct {
	text  string
	image string // base64-encoded image data (empty for non-image)
}

const docxContextWindow = 512

func collectDOCXPrevText(flat []flatBlock, idx, maxLen int) string {
	var parts []string
	remaining := maxLen
	for i := idx - 1; i >= 0 && remaining > 0; i-- {
		if flat[i].image != "" {
			break // stop at previous image
		}
		if flat[i].text == "" {
			continue
		}
		r := []rune(flat[i].text)
		if len(r) > remaining {
			r = r[len(r)-remaining:]
		}
		parts = append([]string{string(r)}, parts...)
		remaining -= len(r)
	}
	return strings.Join(parts, "\n")
}

func collectDOCXNextText(flat []flatBlock, idx, maxLen int) string {
	var parts []string
	remaining := maxLen
	for i := idx + 1; i < len(flat) && remaining > 0; i++ {
		if flat[i].image != "" {
			break // stop at next image
		}
		if flat[i].text == "" {
			continue
		}
		r := []rune(flat[i].text)
		if len(r) > remaining {
			r = r[:remaining]
		}
		parts = append(parts, string(r))
		remaining -= len(r)
	}
	return strings.Join(parts, "\n")
}

func joinDOCXIRRuns(runs []docxIRRun) string {
	var b strings.Builder
	for _, r := range runs {
		if r.Type == "text" {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

// --- office_oxide IR types (local copy, independent of deepdoc) ---

type docxIRDocument struct {
	Sections []docxIRSection `json:"sections"`
}

type docxIRSection struct {
	Title    string          `json:"title"`
	Elements []docxIRElement `json:"elements"`
}

type docxIRElement struct {
	Type    string      `json:"type"`
	Content []docxIRRun `json:"content"`
	Data    []byte      `json:"data"`
}

type docxIRRun struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (p *DOCXParser) String() string {
	return "DOCXParser"
}
