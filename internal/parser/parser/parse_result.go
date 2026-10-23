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
// library. Parsers surface a Python-compatible stage-boundary payload:
//
//	file (enriched metadata)
//	JSON (structured backend output, typed as DocItems)
//	err
//	warnings
//
// The owning Parser component hardcodes the wire `output_format` to "json"
// for every family; that value is a property of the component boundary, not
// of the parser result, so it is no longer carried on ParseResult. Backend
// markdown responses are normalized to JSON inside the parser package before
// they reach the component, so no raw markdown travels on this struct either.
//
// Parser callers consume the structured ParseResult payload. Use Produced to
// check whether a result carries a usable payload: Err is nil and JSON or File
// is non-nil.

package parser

import (
	"context"

	"ragflow/internal/entity"
)

// ParseResult is the structured return value of a parse operation. JSON holds
// structured backend output when available; File carries parser-enriched
// metadata. On failure, Err is non-nil and the payload fields are zero values.
type ParseResult struct {
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

	// JSON is the structured payload when available, as typed DocItems.
	// Each item projects onto the historical wire form via DocItem.ToMap,
	// so the external key shape (text + doc_type_kwd, optional image /
	// layout / positions, table) is unchanged. Shape depends on the parser
	// family: PDF emits `text` + `doc_type_kwd` items (and optional
	// `image` / `layout` / `positions` fields); Markdown / HTML / text emit
	// normalized `{text, doc_type_kwd}` items; image emits an image payload
	// and, after ingestion enhancement, its VLM description.
	JSON []DocItem

	// Err is the failure reason. On non-nil Err, all payload
	// fields are zero values.
	Err error

	// Warnings contains non-fatal issues encountered while parsing.
	Warnings []string
}

// Produced reports whether the parse succeeded and yielded a payload the
// Parser component can forward: structured JSON items and/or enriched File
// metadata. It replaces the historical `OutputFormat != ""` success check. An
// error result is never produced, and a result with neither JSON nor File is
// treated as empty (the component falls back to raw-text indexing).
func (r ParseResult) Produced() bool {
	return r.Err == nil && (r.JSON != nil || r.File != nil)
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

// NewTextJSONItem constructs a canonical text DocItem for parser output.
func NewTextJSONItem(text string) DocItem {
	return TextItem{Text: text}
}

// NewTableDataJSONItem constructs a canonical structured table DocItem for
// parser output. It carries the parsed TableData contract (caption, header
// rows, cell text already normalized the same way the legacy chunker
// normalized HTML <td>/<th> text) instead of the legacy HTML markup string,
// so the chunker reads rows directly without re-parsing any markup.
func NewTableDataJSONItem(td *entity.TableData, sheet string, positions Positions) DocItem {
	return TableItem{
		Table:     td,
		Sheet:     sheet,
		Positions: positions,
	}
}
