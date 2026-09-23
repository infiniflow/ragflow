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
)

// buildPPTXJSONSections converts office_oxide presentation IR JSON into one
// text item per slide followed by that slide's image items. The IR carries
// exactly one section per slide, so section index + 1 is the slide number.
// Per-slide text is flattened with the shared IR walker (docxElementText),
// which covers paragraphs, headings, text boxes, tables, and (nested) lists.
//
// A slide without extractable text (blank layout, image-only) still
// yields an item with an empty text field, so slide_number stays
// aligned with the source deck.
//
// Pure Go (no cgo) so the IR → items transform is unit-testable without
// the office_oxide native library.
func buildPPTXJSONSections(irJSON string) ([]map[string]any, error) {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil, fmt.Errorf("presentation ir-json decode: %w", err)
	}
	items := make([]map[string]any, 0, len(ir.Sections))
	for i, sec := range ir.Sections {
		var lines []string
		for _, el := range sec.Elements {
			// Each element's full text (paragraphs, table cells, list
			// items, etc.) becomes a single value; the shared IR walker
			// already inserts newlines between rows/items (cellSep "\n"),
			// so internal newlines are preserved as-is. Only the
			// element-level split is collapsed (one element → one value or
			// none if empty).
			if text := strings.TrimSpace(docxElementText(el, "\n")); text != "" {
				lines = append(lines, text)
			}
		}
		items = append(items, map[string]any{
			"text":         strings.Join(lines, "\n"),
			"doc_type_kwd": "text",
			"slide_number": i + 1,
		})
		var images [][]byte
		for _, el := range sec.Elements {
			images = append(images, docxIRImagesInElements([]docxIRElement{el})...)
		}
		for mediaOrder, data := range images {
			items = append(items, map[string]any{
				"text":         "",
				"image":        base64.StdEncoding.EncodeToString(data),
				"doc_type_kwd": "image",
				"ck_type":      "image",
				"slide_number": i + 1,
				"media_order":  mediaOrder + 1,
			})
		}
	}
	return items, nil
}

// itemsAllEmpty reports whether every item lacks both text and image payloads.
// A deck whose IR sections all flatten to "" and contain no images must fall
// back to PlainText instead of emitting empty chunks that the Tokenizer later
// filters, silently yielding 0 chunks.
func itemsAllEmpty(items []map[string]any) bool {
	for _, it := range items {
		if text, _ := it["text"].(string); strings.TrimSpace(text) != "" {
			return false
		}
		if image, _ := it["image"].(string); strings.TrimSpace(image) != "" {
			return false
		}
	}
	return true
}

// itemsFromPlainText wraps whole-document plain text as a single JSON
// item. It salvages a still-readable deck when the structured IR cannot
// be used (IR serialization failure or a sectionless IR); a document
// without extractable text yields an empty, non-nil item slice.
func itemsFromPlainText(text string) []map[string]any {
	items := []map[string]any{}
	if trimmed := strings.TrimSpace(text); trimmed != "" {
		items = append(items, map[string]any{"text": trimmed, "doc_type_kwd": "text"})
	}
	return items
}
