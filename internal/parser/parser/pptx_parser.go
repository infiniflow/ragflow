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
	"fmt"
	"strings"

	officeOxide "github.com/yfedoseev/office_oxide/go"
)

type PPTXParser struct {
	libType string
}

func NewPPTXParser(libType string) (*PPTXParser, error) {
	switch libType {
	case OfficeOxide:
		return &PPTXParser{
			libType: OfficeOxide,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported PPTX library type: %s", libType)
	}
}

func (p *PPTXParser) String() string {
	return "PPTXParser"
}

// ParseWithResult emits one JSON item per slide with the slide's
// plain text. Mirrors the python parser.py:slides branch which
// forces output_format="json" for the slide family.
func (p *PPTXParser) ParseWithResult(filename string, data []byte) ParseResult {
	doc, err := officeOxide.OpenFromBytes(data, "pptx")
	if err != nil {
		return ParseResult{Err: fmt.Errorf("pptx open: %w", err)}
	}
	defer doc.Close()

	text, err := doc.PlainText()
	if err != nil {
		return ParseResult{Err: fmt.Errorf("pptx plain-text: %w", err)}
	}

	// Split on form-feed (the python TxtParser convention used by
	// ragflow's slide parser) — each block becomes a JSON item.
	var items []map[string]any
	for i, raw := range strings.Split(text, "\f") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		items = append(items, map[string]any{
			"text":         trimmed,
			"doc_type_kwd": "text",
			"slide_number": i + 1,
		})
	}
	if items == nil {
		items = []map[string]any{{"text": strings.TrimSpace(text), "doc_type_kwd": "text"}}
	}

	return ParseResult{
		OutputFormat: "json",
		File:         map[string]any{"name": filename, "format": "pptx"},
		JSON:         items,
	}
}
