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

// ParseResult is the structured output contract for the Go parser
// library. Port-rag-flow-pipeline-to-go.md §6.5 mandates that
// parsers surface enough data to reconstruct a Python-compatible
// stage-boundary payload:
//
//	output_format
//	file (enriched metadata)
//	backend payload to be normalized by the owning component
//	err
//
// Go parser callers now consume only the structured ParseResult
// contract. The legacy `Parse(filename, []byte) error` interface has
// been removed so parser dispatch, ingestion, and service paths all
// share the same typed payload contract.

package parser

import "context"

// ParseResult is the structured return value of a parse operation. JSON holds
// structured backend output when available; the rendered fields carry
// non-JSON backend responses to the Parser component's normalization boundary.
// On failure, Err is non-nil and OutputFormat is empty.
type ParseResult struct {
	// OutputFormat identifies the backend payload representation. The Parser
	// component normalizes successful results to JSON. Empty when Err is non-nil.
	OutputFormat string

	// File is metadata produced by the parser backend (for example
	// `outline` on the PDF path or `page_count` for paginated formats).
	// Nil when the backend does not produce metadata. It is not an augmented
	// copy of the upstream file descriptor.
	//
	// For the office family (docx/doc, pptx/ppt), File["format"]
	// reflects the real container format detected via magic-byte
	// sniffing (officeContainer), not just the file extension.
	// E.g. a legacy OLE .doc uploaded as .docx yields
	// File["format"] == "doc" so downstream consumers see the
	// effective format. This is an intentional contract change
	// from the pre-fallback behaviour where File["format"] always
	// echoed the requested extension.
	File map[string]any

	// JSON is the structured payload when available.
	// Shape depends on the parser family: PDF emits
	// `[]map[string]any` with `text` + `doc_type_kwd` keys (and
	// optional `image` / `layout` / `positions` fields);
	// Markdown / HTML / text emit normalized
	// `{text, doc_type_kwd}` items; image emits OCR/VLM result
	// items.
	JSON []map[string]any

	// Markdown is a backend response awaiting normalization.
	Markdown string

	// Text is a backend response awaiting normalization.
	Text string

	// HTML is a backend response awaiting normalization.
	HTML string

	// Err is the failure reason. On non-nil Err, all payload
	// fields are zero values.
	Err error

	// Warnings contains non-fatal issues encountered while parsing.
	Warnings []string
}

// ParseResultProducer is the parser package's single structured-output
// contract. Every parser returned by GetParser must implement it.
type ParseResultProducer interface {
	ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult
}

// Canonical document type identifiers used in structured JSON items.
// These align directly with Python's doc_type_kwd contract.
const (
	DocTypeKey   = "doc_type_kwd"
	DocTypeText  = "text"
	DocTypeTable = "table"
	DocTypeImage = "image"
)

// NewTextJSONItem constructs a canonical text JSON item for parser output.
func NewTextJSONItem(text string) map[string]any {
	return map[string]any{
		"text":     text,
		DocTypeKey: DocTypeText,
	}
}

// NewTableJSONItem constructs a canonical table JSON item for parser output.
// Chunker-owned ck_type is derived from doc_type_kwd at the Chunker boundary.
func NewTableJSONItem(html string, sheet string, positions [][]float64) map[string]any {
	item := map[string]any{
		"text":     html,
		DocTypeKey: DocTypeTable,
	}
	if sheet != "" {
		item["sheet"] = sheet
	}
	if len(positions) > 0 {
		item["positions"] = positions
	}
	return item
}

// NormalizeSpreadsheetOutputFormat returns the canonical output format for spreadsheet
// parsers (CSV, XLS, XLSX). Because spreadsheet parsers always populate structured
// table JSON items as their primary output (with companion HTML), the active format
// is unified to "json", avoiding mismatched formats (e.g. "markdown" or "text") where
// payloads would be empty.
func NormalizeSpreadsheetOutputFormat(_ string) string {
	return "json"
}
