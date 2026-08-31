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
	TopN         int    `json:"top_n"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// QuestionExtractConfig configures automatic question generation.
type QuestionExtractConfig struct {
	TopN         int    `json:"top_n"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// TagExtractConfig configures automatic tag extraction.
type TagExtractConfig struct {
	TopN      int    `json:"top_n"`
	TagFileID string `json:"tag_file_id,omitempty"`
}

// SummaryExtractConfig configures summary / enhanced context extraction.
type SummaryExtractConfig struct {
	Enabled      bool   `json:"enabled"`
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// MetadataExtractConfig configures structured metadata extraction.
// BuiltInMetadata is carried for persistence/replay; it is NOT LLM-extracted.
// Deterministic file_name/update_time is applied via PipelineResult -> doc_state.applyBuiltInMetadata.
type MetadataExtractConfig struct {
	Enabled         bool                      `json:"enabled"`
	Metadata        []common.MetadataFieldDef `json:"metadata,omitempty"`
	BuiltInMetadata []common.MetadataFieldDef `json:"built_in_metadata,omitempty"`
}

// ExtractorParam is the static configuration for the Extractor component.
// Fully modularized into base settings and 5 sub-extraction tasks.
type ExtractorParam struct {
	// Base settings
	LLMID string `json:"llm_id,omitempty"`

	// Modular sub-configs
	Keywords  KeywordExtractConfig  `json:"keywords,omitempty"`
	Questions QuestionExtractConfig `json:"questions,omitempty"`
	Tags      TagExtractConfig      `json:"tags,omitempty"`
	Summary   SummaryExtractConfig  `json:"summary,omitempty"`
	Metadata  MetadataExtractConfig `json:"metadata,omitempty"`
}

// Defaults returns the default ExtractorParam.
func (ExtractorParam) Defaults() ExtractorParam {
	return ExtractorParam{
		LLMID: "",
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

	// Chunks is the enriched chunk list. Each chunk is enriched with
	// modular extraction fields (important_kwd, question_kwd, tag_kwd,
	// summary, metadata).
	Chunks []map[string]any `json:"chunks,omitempty"`

	// Error is set when the component short-circuits with an error
	// message (Python: set_output("_ERROR", ...)).
	Error string `json:"_ERROR,omitempty"`
}
