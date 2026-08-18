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

import "ragflow/internal/common"

// TagLabel is a single labeled record from the tag definition file:
// a piece of content and the tags associated with it.
type TagLabel struct {
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// TaggedChunk is the result of tagging a chunk: the chunk content, the
// matched tags, and their computed relevance weights.
type TaggedChunk struct {
	Content    string         `json:"content"`
	Tags       []string       `json:"tags"`
	TagWeights map[string]int `json:"tag_weights,omitempty"`
}

// ExtractorFromUpstream is the upstream payload consumed by the
// Extractor component.
//
// The Python Extractor (rag/flow/extractor/extractor.py) does NOT
// validate a Pydantic *FromUpstream schema; instead it pulls inputs
// from the canvas's input-elements map:
//
//	inputs = self.get_input_elements()
//	for k, v in inputs.items():
//	    args[k] = v["value"]
//	    if isinstance(args[k], list):
//	        chunks = deepcopy(args[k])
//	        chunks_key = k
//
// To keep the Go port faithful, the Go FromUpstream mirrors that
// shape: a free-form map of named inputs plus an optional explicit
// chunks list (the typical case in pipeline wiring).
type ExtractorFromUpstream struct {
	// CreatedTime / ElapsedTime follow the package-wide convention
	// from upstream components.
	CreatedTime *float64 `json:"_created_time,omitempty"`
	ElapsedTime *float64 `json:"_elapsed_time,omitempty"`

	// Inputs mirrors `get_input_elements()` output. Each entry holds a
	// free-form value (string for the LLM template, list of chunks
	// for the chunk-list binding). Keys are the input names; the
	// component selects the first list-typed value as the chunk
	// stream and passes the rest as scalar args.
	Inputs map[string]any `json:"inputs,omitempty"`

	// Chunks is the explicit chunk list when wired in a linear
	// pipeline. Optional — when Inputs contains a list-typed entry,
	// the component uses that instead.
	Chunks []map[string]any `json:"chunks,omitempty"`
}

// Validate enforces no required fields today; the Python component
// happily runs on an empty input set (it produces a single output
// chunk from the LLM call).
func (ExtractorFromUpstream) Validate() error { return nil }

// KeywordExtractConfig configures automatic keyword extraction.
type KeywordExtractConfig struct {
	TopN         int    `json:"top_n,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// QuestionExtractConfig configures automatic question generation.
type QuestionExtractConfig struct {
	TopN         int    `json:"top_n,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// TagExtractConfig configures automatic tag extraction.
type TagExtractConfig struct {
	TopN      int    `json:"top_n,omitempty"`
	TagFileID string `json:"tag_file_id,omitempty"`
}

// SummaryExtractConfig configures summary / enhanced context extraction.
type SummaryExtractConfig struct {
	Enabled      bool   `json:"enabled,omitempty"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// MetadataExtractConfig configures structured metadata extraction.
type MetadataExtractConfig struct {
	Enabled         bool                      `json:"enabled,omitempty"`
	Metadata        []common.MetadataFieldDef `json:"metadata,omitempty"`
	BuiltInMetadata []common.MetadataFieldDef `json:"built_in_metadata,omitempty"`
}

// ExtractorParam is the static configuration for the Extractor
// component. Supports modular sub-configs (Keywords, Questions, Tags,
// Summary, Metadata) as well as legacy flat fields for backward compatibility.
type ExtractorParam struct {
	// Modular sub-configs
	Keywords       KeywordExtractConfig  `json:"keywords,omitempty"`
	Questions      QuestionExtractConfig `json:"questions,omitempty"`
	Tags           TagExtractConfig      `json:"tags,omitempty"`
	Summary        SummaryExtractConfig  `json:"summary,omitempty"`
	MetadataConfig MetadataExtractConfig `json:"metadata_config,omitempty"`

	// FieldName is the chunk key the LLM extraction result is written
	// to (Python: `self._param.field_name`). Optional — when empty,
	// auto_keywords or auto_questions may still be used.
	FieldName string `json:"field_name,omitempty"`

	// LLMID identifies the LLM model used for extraction. This is the
	// agent-side LLMParam.llm_id; on the ingestion side it is
	// resolved against the tenant's LLM provider registry.
	LLMID string `json:"llm_id,omitempty"`

	// SystemPrompt is the optional system prompt override.
	SystemPrompt string `json:"system_prompt,omitempty"`

	// Prompt is the user-side template passed to the LLM.
	Prompt string `json:"prompt,omitempty"`

	// AutoKeywords enables automatic keyword extraction with a fixed
	// prompt. The value determines the top-N count. Legacy flat field.
	AutoKeywords int `json:"auto_keywords,omitempty"`

	// AutoQuestions enables automatic question generation with a fixed
	// prompt. The value determines the top-N count. Legacy flat field.
	AutoQuestions int `json:"auto_questions,omitempty"`

	// AutoTags enables tag assignment on chunks. Legacy flat field.
	AutoTags int `json:"auto_tags,omitempty"`

	// TagFileID references a tag-definition file stored in object
	// storage. Legacy flat field.
	TagFileID string `json:"tag_file_id,omitempty"`

	// EnableSummary enables summary extraction. Legacy flat field.
	EnableSummary int `json:"enable_summary,omitempty"`

	// EnableMetadata enables automatic structured metadata extraction. Legacy flat field.
	EnableMetadata int `json:"enable_metadata,omitempty"`

	// Metadata lists the target field definitions for
	// EnableMetadata (mirrors parser_config.metadata / built_in_metadata:
	// {key, type, description, enum}). When empty, EnableMetadata is a
	// no-op (nothing to extract).
	Metadata []common.MetadataFieldDef `json:"metadata,omitempty"`
}

// Defaults returns the default ExtractorParam.
func (ExtractorParam) Defaults() ExtractorParam {
	return ExtractorParam{
		FieldName:      "",
		LLMID:          "",
		SystemPrompt:   "",
		Prompt:         "",
		AutoKeywords:   0,
		AutoQuestions:  0,
		AutoTags:       0,
		TagFileID:      "",
		EnableSummary:  0,
		EnableMetadata: 0,
		Metadata:       nil,
	}
}

// Validate always returns nil.
func (p *ExtractorParam) Validate() error {
	return nil
}

// ExtractorOutputs is the result of invoking the Extractor component.
// Mirrors what the Python component sets via `self.set_output(...)` at
// rag/flow/extractor/extractor.py:_invoke:
//
//	self.set_output("output_format", "chunks")
//	self.set_output("chunks", chunks)
type ExtractorOutputs struct {
	// OutputFormat is always "chunks".
	OutputFormat string `json:"output_format,omitempty"`

	// Chunks is the enriched chunk list. When the Extractor ran over
	// a non-empty input list, each chunk gains a new key named after
	// FieldName (e.g., field_name="summary" -> chunk["summary"]). When
	// the Extractor ran over an empty input, Chunks contains a single
	// entry with one key (FieldName) holding the LLM result.
	Chunks []map[string]any `json:"chunks,omitempty"`

	// Error is set when the component short-circuits with an error
	// message (Python: set_output("_ERROR", ...)).
	Error string `json:"_ERROR,omitempty"`
}
