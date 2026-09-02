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

// ParseWithResult extracts text for the DOC family. The Go side uses
// office_oxide, which supports legacy .doc. We prefer the structured IR
// (flattened to plain text) and fall back to ToMarkdown, then to PlainText.
//
// Note: office_oxide's .doc reader only surfaces paragraph and (heuristically
// detected) heading lines — it does NOT extract table or list structure from
// legacy .doc (the table stream's PAP/TAP/LSTF are ignored; see
// office_oxide issue #115). So this path normalizes paragraph/heading line
// breaks but does not recover tables/lists. OutputFormat stays "text" to keep
// the downstream contract unchanged.
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
// document. office_oxide exposes three text views:
//   - ToIRJSON: a structured DocumentIR, flattened to plain text via
//     flattenDocIR. For .doc this only carries paragraph and (heuristic)
//     heading lines — no table/list structure (office_oxide issue #115).
//   - ToMarkdown: paragraphs separated by blank lines.
//   - PlainText: the raw concatenated text.
//
// We return the longest non-empty view so a sparser view can never shadow a
// more complete one (e.g. the IR flatten can be shorter than PlainText for
// some documents). A failure at every stage degrades to the PlainText error,
// preserving the original "no text at all" failure semantics.
func extractDocText(doc *officeOxide.Document) (string, error) {
	var best string
	if irJSON, err := doc.ToIRJSON(); err == nil {
		if t := flattenDocIR(irJSON); len(strings.TrimSpace(t)) > len(strings.TrimSpace(best)) {
			best = t
		}
	}
	if md, err := doc.ToMarkdown(); err == nil {
		if len(strings.TrimSpace(md)) > len(strings.TrimSpace(best)) {
			best = md
		}
	}
	if plain, err := doc.PlainText(); err == nil {
		if len(strings.TrimSpace(plain)) > len(strings.TrimSpace(best)) {
			best = plain
		}
	} else if best == "" {
		return "", err
	}
	return best, nil
}
