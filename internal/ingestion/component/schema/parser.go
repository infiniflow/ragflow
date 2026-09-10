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

package schema

// ParserFromUpstream is the upstream payload consumed by the Parser
// component. It mirrors rag/flow/parser/schema.py:ParserFromUpstream
// (Pydantic BaseModel with populate_by_name, extra="forbid").
//
//	created_time: float | None  (alias _created_time)
//	elapsed_time: float | None  (alias _elapsed_time)
//	name: str                   (required)
//	file: dict | None
//	abstract: bool = False
//	author:  bool = False
type ParserFromUpstream struct {
	CreatedTime *float64 `json:"_created_time,omitempty"`
	ElapsedTime *float64 `json:"_elapsed_time,omitempty"`

	Name string `json:"name"`

	// File is the optional upstream file descriptor. Python allows None
	// when the parser is invoked via a canvas-bound doc_id path.
	File map[string]any `json:"file,omitempty"`

	Abstract bool `json:"abstract,omitempty"`
	Author   bool `json:"author,omitempty"`
}

// Validate enforces the only required field in ParserFromUpstream: Name.
// Returns nil when Name is non-empty.
func (p *ParserFromUpstream) Validate() error {
	if p.Name == "" {
		return errRequiredField{Field: "name"}
	}
	return nil
}

// Page is one parsed document section. The Parser component does not emit
// a typed page model — Python code passes around `dict` literals with
// shape `{text, layout_type, doc_type_kwd, positions?, image?, ...}`. To
// keep the wire schema typed without overcommitting to a parser-specific
// shape, Page is left as a generic map and provided for forward
// documentation; downstream chunker code operates on the same dict shape.
type Page map[string]any

// ParserSetup is a per-filetype parser configuration block. The keys are heterogeneous (e.g.,
// `parse_method`, `lang`, `output_format`, `suffix`, `fields`, `vlm`),
// so a free-form map best mirrors the Python dict literal.
type ParserSetup map[string]any

// ParserOutputs is the result of invoking the Parser component. The
// wire format is "json", with structured JSON items as the primary payload.
// Companion payloads (markdown, text, html) are preserved when available.
type ParserOutputs struct {
	// OutputFormat is always "json". Downstream components consume structured JSON items.
	OutputFormat string `json:"output_format,omitempty"`

	// JSON holds the list of structured sections (primary payload).
	JSON []map[string]any `json:"json,omitempty"`

	// Markdown holds rendered Markdown when available as a companion payload.
	Markdown string `json:"markdown,omitempty"`

	// Text holds rendered plain text when available as a companion payload.
	Text string `json:"text,omitempty"`

	// HTML holds rendered HTML when available as a companion payload.
	HTML string `json:"html,omitempty"`

	// File is the upstream file descriptor with parser-derived metadata
	// (e.g., outlines) merged in. Mirrors the Python `set_output("file", ...)`
	// at parser.py:609, 791, 828.
	File map[string]any `json:"file,omitempty"`

	// Error is set when the component short-circuits with an error
	// message (Python: set_output("_ERROR", ...)).
	Error string `json:"_ERROR,omitempty"`
}
