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

// ExtractorParam is the static configuration for the Extractor
// component. Mirrors rag/flow/extractor/extractor.py:ExtractorParam,
// which extends both ProcessParamBase and LLMParam. The LLM fields
// (`llm_id`, `parameters`, `system_prompt`, `prompt`, `messages`,
// etc.) live on the agent-side `LLMParam`; the Go port captures only
// the Extractor-specific field plus a pointer to the LLM config so
// the wiring is explicit.
type ExtractorParam struct {
	// FieldName is the chunk key the LLM extraction result is written
	// to (Python: `self._param.field_name`). Required — `check()`
	// raises when empty. Mapped to "Result Destination" in the
	// frontend.
	FieldName string `json:"field_name"`

	// LLMID identifies the LLM model used for extraction. This is the
	// agent-side LLMParam.llm_id; on the ingestion side it is
	// resolved against the tenant's LLM provider registry.
	LLMID string `json:"llm_id,omitempty"`

	// SystemPrompt is the optional system prompt override.
	SystemPrompt string `json:"system_prompt,omitempty"`

	// Prompt is the user-side template passed to the LLM.
	Prompt string `json:"prompt,omitempty"`
}

// Defaults returns the Python default ExtractorParam: FieldName is
// the empty string and is meant to be supplied at runtime.
func (ExtractorParam) Defaults() ExtractorParam {
	return ExtractorParam{
		FieldName:    "",
		LLMID:        "",
		SystemPrompt: "",
		Prompt:       "",
	}
}

// Validate enforces the Python `check()` invariant: FieldName must
// be non-empty.
func (p *ExtractorParam) Validate() error {
	if p.FieldName == "" {
		return errRequiredField{Field: "field_name"}
	}
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
