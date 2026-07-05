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

// ParserSetup is the per-filetype configuration block stored on
// ParserParam.setups[fileType]. The keys are heterogeneous (e.g.,
// `parse_method`, `lang`, `output_format`, `suffix`, `fields`, `vlm`),
// so a free-form map best mirrors the Python dict literal.
type ParserSetup map[string]any

// ParserParam is the static configuration for the Parser component.
// Mirrors rag/flow/parser/parser.py:ParserParam.
//
// Two top-level fields are configured in the Python class:
//
//	setups: dict[str, dict]  (one entry per file type: pdf, docx, ...)
//	allowed_output_format: dict[str, list[str]]  (per-file-type formats)
//
// `check()` runs further validation that is intentionally NOT
// replicated here — Validate() enforces wire-shape only; business-rule
// validation lives in the component implementation (Phase 2.2).
type ParserParam struct {
	// Setups holds the per-file-type parser config. Keys are file-type
	// identifiers ("pdf", "docx", "markdown", "spreadsheet", "image",
	// "audio", "video", "email", "epub", "doc", "text&code", "html",
	// "slides"); values are free-form config blobs.
	Setups map[string]ParserSetup `json:"setups"`

	// AllowedOutputFormat mirrors `allowed_output_format` from the
	// Python class. Used for client-side input-form validation.
	AllowedOutputFormat map[string][]string `json:"allowed_output_format"`
}

// Defaults returns a ParserParam populated with the Python defaults —
// the full setups table copied verbatim from
// rag/flow/parser/parser.py:ParserParam.__init__ and the corresponding
// allowed_output_format map.
func (ParserParam) Defaults() ParserParam {
	return ParserParam{
		AllowedOutputFormat: map[string][]string{
			"pdf":         {"json", "markdown"},
			"spreadsheet": {"json", "markdown", "html"},
			"doc":         {"json", "markdown"},
			"docx":        {"json", "markdown"},
			"slides":      {"json"},
			"image":       {"json"},
			"email":       {"text", "json"},
			"markdown":    {"text", "json"},
			"text&code":   {"text", "json"},
			"html":        {"text", "json"},
			"audio":       {"json"},
			"video":       {},
			"epub":        {"text", "json"},
		},
		Setups: map[string]ParserSetup{
			"pdf": {
				"parse_method":          "deepdoc",
				"lang":                  "Chinese",
				"flatten_media_to_text": false,
				"remove_toc":            false,
				"remove_header_footer":  false,
				"suffix":                []string{"pdf"},
				"output_format":         "json",
			},
			"spreadsheet": {
				"parse_method":          "deepdoc",
				"flatten_media_to_text": false,
				"output_format":         "html",
				"suffix":                []string{"xls", "xlsx", "csv"},
			},
			"doc": {
				"remove_toc":           false,
				"remove_header_footer": false,
				"suffix":               []string{"doc"},
				"output_format":        "json",
			},
			"docx": {
				"flatten_media_to_text": false,
				"remove_toc":            false,
				"remove_header_footer":  false,
				"suffix":                []string{"docx"},
				"output_format":         "json",
			},
			"markdown": {
				"flatten_media_to_text": false,
				"suffix":                []string{"md", "markdown", "mdx"},
				"remove_toc":            false,
				"output_format":         "json",
			},
			"text&code": {
				"suffix": []string{
					"txt", "py", "js", "java", "c", "cpp", "h", "php",
					"go", "ts", "sh", "cs", "kt", "sql",
				},
				"output_format": "json",
			},
			"html": {
				"suffix":               []string{"htm", "html"},
				"remove_toc":           false,
				"remove_header_footer": false,
				"output_format":        "json",
			},
			"slides": {
				"parse_method":  "deepdoc",
				"suffix":        []string{"pptx", "ppt"},
				"output_format": "json",
			},
			"image": {
				"parse_method":  "ocr",
				"llm_id":        "",
				"lang":          "Chinese",
				"system_prompt": "",
				"suffix":        []string{"jpg", "jpeg", "png", "gif"},
				"output_format": "json",
			},
			"email": {
				"suffix": []string{"eml", "msg"},
				"fields": []string{
					"from", "to", "cc", "bcc", "date", "subject",
					"body", "attachments", "metadata",
				},
				"output_format": "text",
			},
			"audio": {
				"suffix": []string{
					"da", "wave", "wav", "mp3", "aac", "flac", "ogg",
					"aiff", "au", "midi", "wma", "realaudio", "vqf",
					"oggvorbis", "ape",
				},
				"output_format": "text",
			},
			"video": {
				"suffix":        []string{"mp4", "avi", "mkv"},
				"output_format": "text",
				"prompt":        "",
			},
			"epub": {
				"suffix":        []string{"epub"},
				"output_format": "json",
			},
		},
	}
}

// Validate returns nil. ParserParam's field set is fully defaulted by
// Defaults(); the component's own `check()` method performs business
// validation (e.g., "parse_method" must be one of the allowed set), and
// that runs in the component implementation.
func (ParserParam) Validate() error { return nil }

// ParserOutputs is the result of invoking the Parser component. The
// Python component calls `self.set_output(...)` with a mix of
// string-typed, list-typed, and dict-typed values. The wire schema
// below is the typed surface consumed by downstream components.
//
// Mirrors what Parser sets at rag/flow/parser/parser.py:_invoke. The
// parser writes to EITHER ("json" | "markdown" | "text" | "html") and
// always sets "output_format" + "file" + "_ERROR".
type ParserOutputs struct {
	// OutputFormat is the active output format for this run
	// (one of "json", "markdown", "text", "html"). The downstream
	// Tokenizer branches on this field.
	OutputFormat string `json:"output_format,omitempty"`

	// JSON holds the list of structured sections when output_format == "json".
	JSON []map[string]any `json:"json,omitempty"`

	// Markdown holds the rendered markdown when output_format == "markdown".
	Markdown string `json:"markdown,omitempty"`

	// Text holds the rendered plain text when output_format == "text".
	Text string `json:"text,omitempty"`

	// HTML holds the rendered HTML when output_format == "html".
	HTML string `json:"html,omitempty"`

	// File is the upstream file descriptor with parser-derived metadata
	// (e.g., outlines) merged in. Mirrors the Python `set_output("file", ...)`
	// at parser.py:609, 791, 828.
	File map[string]any `json:"file,omitempty"`

	// Error is set when the component short-circuits with an error
	// message (Python: set_output("_ERROR", ...)).
	Error string `json:"_ERROR,omitempty"`
}
