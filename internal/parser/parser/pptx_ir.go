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
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/entity"
)

// buildPPTXJSONSections emits text runs, tables and images in slide order.
// Every item retains its slide number; a blank slide emits an empty text item.
// Container elements are traversed recursively so grouped tables remain structured.
func buildPPTXJSONSections(irJSON string, budget *embeddedMediaBudget) ([]map[string]any, error) {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return nil, fmt.Errorf("presentation ir-json decode: %w", err)
	}
	if budget == nil {
		budget = newEmbeddedMediaBudget()
	}
	items := make([]map[string]any, 0, len(ir.Sections))
	for i, sec := range ir.Sections {
		start := len(items)
		var lines []string
		flushText := func() {
			if len(lines) > 0 {
				items = append(items, map[string]any{"text": strings.Join(lines, "\n"), "doc_type_kwd": "text"})
				lines = nil
			}
		}
		tableSequence := 0
		var emitElements func([]docxIRElement)
		emitElements = func(elements []docxIRElement) {
			for _, el := range elements {
				var units []map[string]any
				switch el.Type {
				case "table":
					if len(el.Rows) == 0 {
						continue
					}
					flushText()
					tableSequence++
					items = appendOfficeTableSections(items, el, "", fmt.Sprintf("pptx-slide-%d-table-%d", i+1, tableSequence), budget)
					continue
				case "text_box":
					emitElements(el.contentBlocks())
					continue
				case "list":
					for _, item := range el.Items {
						emitElements(item.Content)
						if item.Nested != nil {
							emitElements([]docxIRElement{{Type: "list", Items: item.Nested.Items}})
						}
					}
					continue
				case "paragraph", "heading":
					units = appendDOCXParagraphSections(nil, el, false, budget)
				case "image":
					units, _ = appendDOCXImageSection(nil, el.Data, budget, nil)
				default:
					if text := strings.TrimSpace(docxElementText(el, "\n")); text != "" {
						lines = append(lines, text)
					}
				}
				for _, unit := range units {
					if unit["doc_type_kwd"] == "text" {
						lines = append(lines, unit["text"].(string))
						continue
					}
					flushText()
					items = append(items, unit)
				}
			}
		}
		emitElements(sec.Elements)
		flushText()
		if len(items) == start {
			items = append(items, map[string]any{"text": "", "doc_type_kwd": "text"})
		}
		mediaOrder := 0
		for _, item := range items[start:] {
			item["slide_number"] = i + 1
			if item["doc_type_kwd"] == "image" {
				mediaOrder++
				item["media_order"] = mediaOrder
			}
		}
	}
	return items, nil
}

// itemsAllEmpty reports whether a presentation has no text, tables or images.
func itemsAllEmpty(items []map[string]any) bool {
	for _, it := range items {
		if table, ok := it["table"].(*entity.TableData); ok && table != nil && len(table.Grid) > 0 {
			return false
		}
		if text, _ := it["text"].(string); strings.TrimSpace(text) != "" {
			return false
		}
		if image, _ := it["image"].(string); strings.TrimSpace(image) != "" {
			return false
		}
		if omitted, _ := it["media_omitted"].(bool); omitted {
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
