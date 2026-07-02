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
// The legacy `FileParser.Parse(filename, []byte) error` interface
// is retained for the parser-stub callers that have not yet been
// ported. Parsers that have a real implementation satisfy the
// optional ParseResultProducer interface and provide
// ParseWithResult, which is the path forward.
//
// The migration pattern is additive and was chosen because:
//
//   - The 12-format parser matrix (PDF, Markdown, HTML, DOC/DOCX,
//     XLS/XLSX, PPT/PPTX, text&code, image, …) is not all ported
//     today; replacing Parse would break the unported stubs.
//
//   - Type-asserting to ParseResultProducer gives callers a typed
//     "yes this parser is structured-output-capable, route through
//     ParseWithResult" signal without paying for reflection in the
//     hot path until the parser is ported.
//
//   - When a parser IS ported, `components.parseStage` (component /
//     parser.go) switches to the structured path with no interface
//     churn elsewhere.

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

// ParseResultProducer is the optional interface a parser
// implements to surface a structured ParseResult. Once a parser
// family is ported, callers type-assert to ParseResultProducer and
// drive it through ParseWithResult — the path forward per plan
// §6.5.
//
// Parsers that have not yet been ported do not satisfy this
// interface; callers fall back to the raw-text fallback strategy
// (see component/parser.go) until the per-format implementation
// lands. No shim layer is required at the parser-package boundary —
// the optional-interface check is the migration seam.
type ParseResultProducer interface {
	ParseWithResult(filename string, data []byte) ParseResult
}
