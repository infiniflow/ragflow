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

// Package parser: this file flattens office_oxide's format-independent IR
// JSON into plain text for legacy .doc files. It is intentionally cgo-free
// (like docx_ir.go) so it can be unit-tested without the native library; the
// cgo boundary lives entirely in doc_parser.go (officeOxide.OpenFromBytes).
//
// The IR schema produced by Document.ToIRJSON is shared across office formats
// by office_oxide, so the docx_ir.go model and helpers (joinDOCXIRRuns,
// joinCellText, extractTextFromListItem, extractTextFromBlockElements) apply
// here unchanged. Flattening — rather than returning ToMarkdown directly —
// keeps the .doc output in the same plain-text field the downstream already
// consumes, while still recovering paragraph/heading/list/table content that
// PlainText alone drops.

package parser

import (
	"encoding/json"
	"strings"
)

// flattenDocIR converts an office_oxide IR JSON string (from Document.ToIRJSON)
// into plain text that preserves paragraph, heading, list and table content.
// Tables are flattened with cells separated by " | " and rows by newlines, so
// their text is not silently dropped — this is the text-fidelity gain over the
// previous PlainText-only path and mirrors what Python's Tika text extraction
// recovers for legacy .doc files.
//
// Returns "" if irJSON cannot be parsed or yields no text; the caller then
// falls back to ToMarkdown / PlainText.
func flattenDocIR(irJSON string) string {
	var ir docxIRDocument
	if err := json.Unmarshal([]byte(irJSON), &ir); err != nil {
		return ""
	}
	var parts []string
	for _, sec := range ir.Sections {
		if t := strings.TrimSpace(sec.Title); t != "" {
			parts = append(parts, t)
		}
		for _, el := range sec.Elements {
			if t := docxElementText(el, " | "); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n")
}
