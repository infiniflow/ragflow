//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// flattenPresentationMediaToText normalizes media items (tables and images) to
// plain text items when flatten_media_to_text is enabled. Structured tables are
// rendered into HTML strings (preserving cells and captions), and image and
// table items have their doc_type_kwd rewritten to "text".
func flattenPresentationMediaToText(items []map[string]any) []map[string]any {
	for _, item := range items {
		if table, ok := item["table"].(*entity.TableData); ok && table != nil {
			text, _ := item["text"].(string)
			rendered := tableutil.RenderTableHTML(table)
			if text != "" {
				item["text"] = strings.TrimSpace(rendered + "\n" + text)
			} else {
				item["text"] = rendered
			}
			delete(item, "table")
			item["doc_type_kwd"] = "text"
		} else if item["doc_type_kwd"] == "image" || item["doc_type_kwd"] == "table" {
			item["doc_type_kwd"] = "text"
		}
	}
	return items
}
