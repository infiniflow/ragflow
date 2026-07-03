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
//	output_format ∈ {"json","markdown","text","html"}
//	file         (enriched metadata)
//	exactly one payload family populated (matching output_format)
//	err
//
// Go parser callers now consume only the structured ParseResult
// contract. The legacy `Parse(filename, []byte) error` interface has
// been removed so parser dispatch, ingestion, and service paths all
// share the same typed payload contract.

package parser

// ParseResult is the structured return value of a successful parse.
// Exactly one of the payload fields (JSON / Markdown / Text / HTML)
// is populated on success, matching the Python contract — see
// port-rag-flow-pipeline-to-go.md §4.2:
//
//   - OutputFormat = "json"     → JSON populated
//   - OutputFormat = "markdown" → Markdown populated
//   - OutputFormat = "text"     → Text populated
//   - OutputFormat = "html"     → HTML populated
//
// On failure (Err != nil), all payload fields are zero values and
// OutputFormat is empty.
type ParseResult struct {
	// OutputFormat is the wire-compatible format the parser
	// chose. Empty when Err is non-nil.
	OutputFormat string

	// File is the enriched file metadata the parser emits. In
	// Python this is the dict form of the original `file`
	// descriptor, augmented with format-specific keys (e.g.
	// `outline` on the PDF path, `page_count` for paginated
	// formats). Nil when the parser did not enrich.
	File map[string]any

	// JSON is the structured payload when OutputFormat == "json".
	// Shape depends on the parser family: PDF emits
	// `[]map[string]any` with `text` + `doc_type_kwd` keys (and
	// optional `image` / `layout` / `positions` fields);
	// markdown / html / text emit normalized
	// `{text, doc_type_kwd}` items; image emits OCR/VLM result
	// items. Exactly one payload family is populated on success.
	JSON []map[string]any

	// Markdown is the string payload when OutputFormat ==
	// "markdown". Empty otherwise.
	Markdown string

	// Text is the string payload when OutputFormat == "text".
	// Empty otherwise.
	Text string

	// HTML is the string payload when OutputFormat == "html".
	// Empty otherwise.
	HTML string

	// Err is the failure reason. On non-nil Err, all payload
	// fields are zero values.
	Err error
}

// ParseResultProducer is the parser package's single structured-output
// contract. Every parser returned by GetParser must implement it.
type ParseResultProducer interface {
	ParseWithResult(filename string, data []byte) ParseResult
}
