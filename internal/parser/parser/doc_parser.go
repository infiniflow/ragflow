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
	"context"
	"fmt"
	"strings"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

type DOCParser struct{}

func NewDOCParser() *DOCParser {
	return &DOCParser{}
}

func (p *DOCParser) String() string {
	return "DOCParser"
}

// ParseWithResult extracts text for the DOC family. Python parser.py routes
// .doc through tika; the Go side uses office_oxide which supports DOC. To
// match Tika's higher text fidelity (paragraphs, headings, lists, tables), we
// prefer the structured IR (flattened to plain text) and fall back to
// ToMarkdown, then to PlainText. OutputFormat="text" — the python side also
// falls back to text for legacy DOC files since structured extraction is
// unreliable, and keeping the downstream text contract unchanged avoids new
// consumer branches.
func (p *DOCParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	doc, err := officeOxide.OpenFromBytes(data, "doc")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("doc open: %w", err)}
	}
	defer doc.Close()

	text, err := extractDocText(doc)
	if err != nil {
		return ParseResult{Err: fmt.Errorf("doc extract: %w", err)}
	}

	return ParseResult{
		OutputFormat: "text",
		File:         map[string]any{"name": filename, "format": "doc"},
		Text:         text,
	}
}

// extractDocText returns the best-effort plain text for a legacy .doc
// document. office_oxide's ToIRJSON exposes paragraph/heading/list/table
// structure; flattenDocIR turns that into plain text so table and paragraph
// content is preserved (the Tika-parity goal). It falls back to ToMarkdown,
// then to PlainText, so a failure at any stage degrades gracefully instead of
// yielding empty text. The final PlainText call's error is propagated,
// preserving the original "no text at all" failure semantics.
func extractDocText(doc *officeOxide.Document) (string, error) {
	if irJSON, err := doc.ToIRJSON(); err == nil {
		if t := flattenDocIR(irJSON); strings.TrimSpace(t) != "" {
			return t, nil
		}
	}
	if md, err := doc.ToMarkdown(); err == nil {
		if strings.TrimSpace(md) != "" {
			return md, nil
		}
	}
	return doc.PlainText()
}
