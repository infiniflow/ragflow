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

import "fmt"

// ChunkerFromUpstream is the shared upstream payload consumed by all four
// chunker variants (TokenChunker, TitleChunker, GroupTitleChunker,
// HierarchyTitleChunker).
//
// It mirrors the wire shape defined across two equivalent Pydantic
// schemas in the Python codebase:
//
//	rag/flow/chunker/schema.py:TokenChunkerFromUpstream
//	rag/flow/chunker/title_chunker/schema.py:TitleChunkerFromUpstream
//
// Both classes are field-identical (apart from the class name) — the
// variants share one upstream payload; their *Param structs differ.
//
// Wire shape (Pydantic):
//
//	created_time: float | None  (alias _created_time)
//	elapsed_time: float | None  (alias _elapsed_time)
//	name: str                   (required)
//	file: dict | None
//	chunks: list[dict] | None
//	output_format: Literal["json","markdown","text","html","chunks"] | None
//	json_result:      list[dict] | None  (alias "json")
//	markdown_result:  str        | None  (alias "markdown")
//	text_result:      str        | None  (alias "text")
//	html_result:      str        | None  (alias "html")
type ChunkerFromUpstream struct {
	CreatedTime *float64 `json:"_created_time,omitempty"`
	ElapsedTime *float64 `json:"_elapsed_time,omitempty"`

	// Name is the source document name. Required.
	Name string `json:"name"`

	// File is the optional upstream file descriptor.
	File *ChunkerFileMeta `json:"file,omitempty"`

	// Chunks is the upstream chunk list, set when output_format == "chunks".
	Chunks []ChunkDoc `json:"chunks,omitempty"`

	// OutputFormat controls which of the *Result fields below is the
	// active payload. Allowed values:
	//   "json"     -> JSONResult
	//   "markdown" -> MarkdownResult
	//   "text"     -> TextResult
	//   "html"     -> HTMLResult
	//   "chunks"   -> Chunks
	OutputFormat PayloadFormat `json:"output_format,omitempty"`

	// JSONResult is the upstream structured JSON list (alias "json" in
	// Python). Set when OutputFormat == "json".
	JSONResult []ChunkDoc `json:"json,omitempty"`

	// MarkdownResult is the upstream markdown payload (alias "markdown").
	// Set when OutputFormat == "markdown".
	MarkdownResult *string `json:"markdown,omitempty"`

	// TextResult is the upstream plain-text payload (alias "text").
	// Set when OutputFormat == "text".
	TextResult *string `json:"text,omitempty"`

	// HTMLResult is the upstream HTML payload (alias "html").
	// Set when OutputFormat == "html".
	HTMLResult *string `json:"html,omitempty"`
}

// Validate enforces the only required field in ChunkerFromUpstream: Name.
// OutputFormat is not strictly required (defaults to "" and the
// component decides what to do with an empty payload), but the
// combination of `OutputFormat == "chunks"` with a non-nil Chunks is the
// happy path.
func (c *ChunkerFromUpstream) Validate() error {
	if c.Name == "" {
		return errRequiredField{Field: "name"}
	}
	if !c.OutputFormat.isKnown() {
		return errInvalidValue{Field: "output_format", Value: string(c.OutputFormat)}
	}
	switch c.OutputFormat {
	case PayloadFormatChunks:
		return nil
	case PayloadFormatJSON:
		if c.JSONResult == nil {
			return errRequiredField{Field: "json"}
		}
	case PayloadFormatMarkdown:
		if c.MarkdownResult == nil {
			return errRequiredField{Field: "markdown"}
		}
	case PayloadFormatText:
		if c.TextResult == nil {
			return errRequiredField{Field: "text"}
		}
	case PayloadFormatHTML:
		if c.HTMLResult == nil {
			return errRequiredField{Field: "html"}
		}
	}
	return nil
}

// ChunkerOutputs is the result of invoking any chunker variant. All
// four chunker components emit the same shape: a list of chunk maps
// under the "chunks" key, plus a marker output_format = "chunks".
//
// Mirrors what each chunker sets at the end of _invoke:
//
//	self.set_output("output_format", "chunks")
//	self.set_output("chunks", chunks)
type ChunkerOutputs struct {
	// OutputFormat is always "chunks" on success.
	OutputFormat PayloadFormat `json:"output_format,omitempty"`

	// Chunks is the produced chunk list. Each entry is a free-form map
	// mirroring the dict shape the Python code builds (text, doc_type_kwd,
	// tk_nums, PDF_POSITIONS_KEY, mom, img_id, etc.).
	Chunks []ChunkDoc `json:"chunks,omitempty"`

	// Error is set when the component short-circuits with an error
	// message (Python: set_output("_ERROR", ...)).
	Error string `json:"_ERROR,omitempty"`
}

// ---------------------------------------------------------------------------
// TokenChunkerParam
// ---------------------------------------------------------------------------
//
// Mirrors rag/flow/chunker/token_chunker.py:TokenChunkerParam.__init__.
// All fields are user-tunable; defaults match the Python values.

type TokenChunkerParam struct {
	// DelimiterMode selects the chunking strategy.
	// Allowed values: "token_size", "delimiter", "one".
	DelimiterMode string `json:"delimiter_mode"`

	// ChunkTokenSize is the target chunk size in tokens.
	ChunkTokenSize int `json:"chunk_token_size"`

	// Delimiters is the list of split tokens. Strings wrapped in
	// backticks (e.g., "`\\n`") denote user-defined regex split points.
	Delimiters []string `json:"delimiters"`

	// OverlappedPercent is the chunk-overlap ratio in [0, 100).
	OverlappedPercent float64 `json:"overlapped_percent"`

	// ChildrenDelimiters is the secondary split applied to text chunks.
	ChildrenDelimiters []string `json:"children_delimiters"`

	// TableContextSize is the number of surrounding tokens to attach
	// to table chunks. 0 disables.
	TableContextSize int `json:"table_context_size"`

	// ImageContextSize is the number of surrounding tokens to attach
	// to image chunks. 0 disables.
	ImageContextSize int `json:"image_context_size"`
}

// Defaults returns the Python default TokenChunkerParam.
func (TokenChunkerParam) Defaults() TokenChunkerParam {
	return TokenChunkerParam{
		DelimiterMode:      "token_size",
		ChunkTokenSize:     512,
		Delimiters:         []string{"\n"},
		OverlappedPercent:  0,
		ChildrenDelimiters: []string{},
		TableContextSize:   0,
		ImageContextSize:   0,
	}
}

// Validate enforces the same enum/range checks the runtime component expects
// at construction time, keeping the schema and component decoder aligned.
func (p TokenChunkerParam) Validate() error {
	switch p.DelimiterMode {
	case "token_size", "delimiter", "one":
	default:
		return errInvalidValue{Field: "delimiter_mode", Value: p.DelimiterMode}
	}
	if p.ChunkTokenSize <= 0 {
		return errInvalidValue{Field: "chunk_token_size", Value: fmt.Sprintf("%d", p.ChunkTokenSize)}
	}
	if p.OverlappedPercent < 0 || p.OverlappedPercent >= 1 {
		return errInvalidValue{Field: "overlapped_percent", Value: fmt.Sprintf("%v", p.OverlappedPercent)}
	}
	if p.TableContextSize < 0 {
		return errInvalidValue{Field: "table_context_size", Value: fmt.Sprintf("%d", p.TableContextSize)}
	}
	if p.ImageContextSize < 0 {
		return errInvalidValue{Field: "image_context_size", Value: fmt.Sprintf("%d", p.ImageContextSize)}
	}
	return nil
}

// ---------------------------------------------------------------------------
// TitleChunkerParam
// ---------------------------------------------------------------------------
//
// Mirrors rag/flow/chunker/title_chunker/common.py:TitleChunkerParam.
// The class also reads `self.method` (set externally — see Python
// title_chunker.py:31-37 routing on `self._param.method`). The Go port
// captures it explicitly here. The component's `check()` enforces enum
// values.

type TitleChunkerParam struct {
	// Method routes to the right title-chunker strategy.
	// Allowed values: "hierarchy", "group".
	Method string `json:"method,omitempty"`

	// Levels is a list of regex-list groups, one per hierarchy level.
	// Each group is a list of regex strings; the component picks the
	// best-matching group at runtime.
	Levels [][]string `json:"levels"`

	// Hierarchy is the heading depth used by HierarchyTitleChunker.
	// Stored as a pointer to distinguish nil (unset) from 0.
	Hierarchy *int `json:"hierarchy,omitempty"`

	// IncludeHeadingContent, when true, makes the heading text part
	// of each emitted chunk.
	IncludeHeadingContent bool `json:"include_heading_content"`

	// RootChunkAsHeading, when true, prepends the root chunk's text
	// to every emitted chunk (and drops the root chunk itself).
	RootChunkAsHeading bool `json:"root_chunk_as_heading"`
}

// Defaults returns the Python default TitleChunkerParam. `Method` is
// not initialized in the Python `__init__` (it is set externally); the
// default is left as the empty string and the component must supply it.
func (TitleChunkerParam) Defaults() TitleChunkerParam {
	return TitleChunkerParam{
		Levels:                [][]string{},
		Hierarchy:             nil,
		IncludeHeadingContent: false,
		RootChunkAsHeading:    false,
	}
}

// Validate enforces the Python `check()` invariants that are also
// expressible in pure-data terms: when Method == "hierarchy" the
// hierarchy depth and level config must be present.
func (p *TitleChunkerParam) Validate() error {
	switch p.Method {
	case "hierarchy", "group":
	case "":
		return nil
	default:
		return errInvalidValue{Field: "method", Value: p.Method}
	}
	switch p.Method {
	case "hierarchy", "group":
		if len(p.Levels) == 0 {
			return errRequiredField{Field: "levels"}
		}
	}
	if p.Method == "hierarchy" && (p.Hierarchy == nil || *p.Hierarchy <= 0) {
		return errRequiredField{Field: "hierarchy"}
	}
	return nil
}

// ---------------------------------------------------------------------------
// GroupTitleChunkerParam / HierarchyTitleChunkerParam
// ---------------------------------------------------------------------------
//
// In the Python codebase, both variants share the SAME
// `TitleChunkerParam` class — there is no per-variant param. The
// dispatch happens in title_chunker.py:31-37 by reading
// `self._param.method`.
//
// In Go we model the shared class as `TitleChunkerParam` and expose
// type aliases so component files can name the param type they
// actually use. This keeps the wire schema faithful to Python while
// giving each variant a self-documenting entry point in the registry.

type GroupTitleChunkerParam = TitleChunkerParam
type HierarchyTitleChunkerParam = TitleChunkerParam
