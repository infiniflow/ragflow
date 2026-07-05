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

// TokenizerFromUpstream is the upstream payload consumed by the
// Tokenizer component. It mirrors
// rag/flow/tokenizer/schema.py:TokenizerFromUpstream, including the
// Pydantic `model_validator(mode="after")` invariant on
// `output_format <-> payload` consistency.
//
// Wire shape (Pydantic):
//
//	created_time:      float | None  (alias _created_time)
//	elapsed_time:      float | None  (alias _elapsed_time)
//	name:              str          (default "")
//	file:              dict | None
//	output_format:     Literal["json","markdown","text","html","chunks"] | None
//	chunks:            list[dict]   | None
//	json_result:       list[dict]   | None  (alias "json")
//	markdown_result:   str          | None  (alias "markdown")
//	text_result:       str          | None  (alias "text")
//	html_result:       str          | None  (alias "html")
type TokenizerFromUpstream struct {
	CreatedTime *float64 `json:"_created_time,omitempty"`
	ElapsedTime *float64 `json:"_elapsed_time,omitempty"`

	// Name is the source document name. Optional in this schema
	// (Python default = "").
	Name string `json:"name,omitempty"`

	// File is the optional upstream file descriptor.
	File *ChunkerFileMeta `json:"file,omitempty"`

	// OutputFormat controls which of the *Result fields below is the
	// active payload. Allowed values:
	//   "json"     -> JSONResult
	//   "markdown" -> MarkdownResult
	//   "text"     -> TextResult
	//   "html"     -> HTMLResult
	//   "chunks"   -> Chunks
	OutputFormat PayloadFormat `json:"output_format,omitempty"`

	// Chunks is the upstream chunk list. Set when OutputFormat == "chunks".
	Chunks []ChunkDoc `json:"chunks,omitempty"`

	// JSONResult is the upstream structured JSON list (alias "json").
	JSONResult []ChunkDoc `json:"json,omitempty"`

	// MarkdownResult is the upstream markdown payload (alias "markdown").
	MarkdownResult *string `json:"markdown,omitempty"`

	// TextResult is the upstream plain-text payload (alias "text").
	TextResult *string `json:"text,omitempty"`

	// HTMLResult is the upstream HTML payload (alias "html").
	HTMLResult *string `json:"html,omitempty"`
}

// Validate enforces the Python model_validator invariants: when
// OutputFormat is "markdown" / "text" / "html", the matching
// *Result field must be non-nil (the Go equivalent of a non-None
// string in Python); when OutputFormat is empty, nil, or any other
// value, JSONResult (or Chunks) must be supplied.
//
// The intent is to mirror the Python error messages verbatim where
// possible. The Tokenizer's runtime contract treats an empty chunk
// list as valid (the component short-circuits silently), so Chunks
// being nil is allowed even when OutputFormat == "chunks".
func (t *TokenizerFromUpstream) Validate() error {
	if !t.OutputFormat.isKnown() {
		return errInvalidValue{Field: "output_format", Value: string(t.OutputFormat)}
	}
	switch t.OutputFormat {
	case PayloadFormatChunks:
		// Chunks may be nil (zero-length is valid). No-op.
		return nil
	case PayloadFormatMarkdown:
		if t.MarkdownResult == nil {
			return errRequiredField{Field: "markdown"}
		}
	case PayloadFormatText:
		if t.TextResult == nil {
			return errRequiredField{Field: "text"}
		}
	case PayloadFormatHTML:
		if t.HTMLResult == nil {
			return errRequiredField{Field: "html"}
		}
	default:
		// Empty / "json" / any other value: require a JSON list payload
		// OR a Chunks list. Mirrors the Python check.
		if t.JSONResult == nil && t.Chunks == nil {
			return errRequiredField{Field: "json"}
		}
	}
	return nil
}

// TokenizerParam is the static configuration for the Tokenizer
// component. Mirrors rag/flow/tokenizer/tokenizer.py:TokenizerParam.
//
//	search_method:        list[str]   # ["full_text", "embedding"]
//	filename_embd_weight: float       # 0.1
//	fields:               list[str]   # ["text"]
type TokenizerParam struct {
	// SearchMethod controls which tokenization/embedding passes run.
	// Allowed values: "full_text", "embedding".
	SearchMethod []string `json:"search_method"`

	// FilenameEmbdWeight blends the document-name embedding into each
	// chunk embedding. Value in [0.0, 1.0].
	FilenameEmbdWeight float64 `json:"filename_embd_weight"`

	// Fields selects which fields of each chunk to embed. Python
	// supports either a single string (then auto-wrapped to a list at
	// runtime) or a list of strings. Go callers should pass a slice.
	Fields []string `json:"fields"`
}

// Defaults returns the Python default TokenizerParam.
func (TokenizerParam) Defaults() TokenizerParam {
	return TokenizerParam{
		SearchMethod:       []string{"full_text", "embedding"},
		FilenameEmbdWeight: 0.1,
		Fields:             []string{"text"},
	}
}

// Validate enforces the SearchMethod enum. Fields and
// FilenameEmbdWeight have no schema-level range checks in the Python
// `check()`.
func (p *TokenizerParam) Validate() error {
	if len(p.SearchMethod) == 0 {
		return errRequiredField{Field: "search_method"}
	}
	for _, m := range p.SearchMethod {
		switch m {
		case "full_text", "embedding":
		default:
			return errInvalidValue{Field: "search_method", Value: m}
		}
	}
	return nil
}

// TokenizerOutputs is the result of invoking the Tokenizer component.
// Mirrors what the Python component sets via `self.set_output(...)` at
// rag/flow/tokenizer/tokenizer.py:_invoke.
//
// Always sets:
//   - output_format = "chunks"
//   - chunks        (the tokenized chunk list)
//
// Optionally sets:
//   - embedding_token_consumption (when search_method includes "embedding")
//   - _ERROR
type TokenizerOutputs struct {
	// OutputFormat is always "chunks".
	OutputFormat PayloadFormat `json:"output_format,omitempty"`

	// Chunks is the tokenized chunk list. Each entry is a free-form map
	// containing tokenized fields (title_tks, content_ltks, ...),
	// chunk_order_int, and (when "embedding" is in search_method)
	// the q_<n>_vec vector field.
	Chunks []ChunkDoc `json:"chunks,omitempty"`

	// EmbeddingTokenConsumption records the token count consumed by
	// the embedding call. Set only when search_method includes
	// "embedding". *int to distinguish unset (zero value) from 0.
	EmbeddingTokenConsumption *int `json:"embedding_token_consumption,omitempty"`

	// Error is set when the component short-circuits with an error
	// message (Python: set_output("_ERROR", ...)).
	Error string `json:"_ERROR,omitempty"`
}
