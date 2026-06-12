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

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ErrGraphRAGNotSupported is returned by the Retrieval tool when callers
// pass use_kg=true. GraphRAG is explicitly out of scope for the Go Canvas
// (plan §5 Phase 3 + §9 Q3); users must either disable use_kg or fall back
// to the Python Canvas.
var ErrGraphRAGNotSupported = errors.New("GraphRAG 检索暂不支持，请使用 Python Canvas 或关闭 use_kg")

// ErrRetrievalServiceMissing is returned by the stub when the
// internal/service/nlp RetrievalService is not wired. Plan §5 Phase 3
// batch 1 ships the tool shell; service wiring lands in Phase 5.
var ErrRetrievalServiceMissing = errors.New(
	"Retrieval service not yet implemented (Phase 5 wiring) — " +
		"use Python Canvas or implement internal/service/nlp/retrieval.go",
)

// retrievalToolName preserves the Python typo ("dateset") for backward
// compatibility with existing Canvas DSLs that reference the tool by name.
const retrievalToolName = "search_my_dateset"

const retrievalToolDescription = "This tool can be utilized for relevant content searching in the datasets."

// retrievalArgs is the JSON schema the model sends into InvokableRun. We
// accept both `query` (canonical) and `dataset_ids` / `use_kg` etc. to
// match the Python ToolMeta field set.
type retrievalArgs struct {
	Query      string   `json:"query"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
	TopN       int      `json:"top_n,omitempty"`
	UseKG      bool     `json:"use_kg,omitempty"`
}

// retrievalResult is the JSON shape returned to the model. The `_ERROR`
// field matches the Python tool's output convention; downstream components
// can pattern-match on it.
type retrievalResult struct {
	FormalizedContent string         `json:"formalized_content,omitempty"`
	Chunks            []chunkPayload `json:"chunks,omitempty"`
	Stub              bool           `json:"stub,omitempty"`
	Error             string         `json:"_ERROR,omitempty"`
}

// chunkPayload is the minimal chunk shape we surface. We don't try to
// match every Python field — the stub returns empty data; the wired
// implementation will populate the real shape.
type chunkPayload struct {
	ID         string  `json:"id,omitempty"`
	Content    string  `json:"content,omitempty"`
	DocumentID string  `json:"document_id,omitempty"`
	Score      float64 `json:"score,omitempty"`
}

// RetrievalTool is the Phase 3 batch 1 shell for the Retrieval tool
// (plan §2.11.4 row 15, §5 Phase 3 第 1 批).
//
// In Phase 3 batch 1 the tool is a STUB: it validates the input (rejecting
// use_kg=true with ErrGraphRAGNotSupported) and returns a structured
// "not-yet-wired" error so callers can detect the gap. Phase 5 wires the
// in-process call to internal/service/nlp.RetrievalService.
type RetrievalTool struct{}

// NewRetrievalTool returns a RetrievalTool implementing eino's
// tool.InvokableTool interface.
func NewRetrievalTool() *RetrievalTool {
	return &RetrievalTool{}
}

// Info returns the tool's metadata for the chat model. The schema mirrors
// the Python RetrievalParam ToolMeta (plan §5 Phase 3, 字段对齐).
func (r *RetrievalTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: retrievalToolName,
		Desc: retrievalToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The keywords to search the dataset. The keywords should be the most important words/terms (including synonyms) from the original request.",
				Required: true,
			},
			"dataset_ids": {
				Type:     schema.Array,
				Desc:     "Optional list of dataset IDs to restrict the search to.",
				Required: false,
			},
			"top_n": {
				Type:     schema.Integer,
				Desc:     "Number of top chunks to return. Defaults to 8 if omitted.",
				Required: false,
			},
			"use_kg": {
				Type:     schema.Boolean,
				Desc:     "GraphRAG toggle. Not supported in Go Canvas (plan §5 Phase 3); must be false.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun executes the tool. In Phase 3 batch 1 this validates the
// input and returns a structured error (or ErrGraphRAGNotSupported) so
// callers can detect the gap. Phase 5 will replace the stub body with
// the in-process call to internal/service/nlp.RetrievalService.
func (r *RetrievalTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args retrievalArgs
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("retrieval: parse arguments: %w", err)
		}
	}

	if args.UseKG {
		// Plan §5 Phase 3 + §9 Q3: GraphRAG is out of scope for the Go
		// Canvas. Return the structured error so the model can react.
		return stubJSON(retrievalResult{
			Stub:  true,
			Error: ErrGraphRAGNotSupported.Error(),
		}), ErrGraphRAGNotSupported
	}

	// Phase 3 batch 1: in-process wiring lands in Phase 5. The stub
	// surfaces a clear, machine-detectable error.
	return stubJSON(retrievalResult{
		Stub:  true,
		Error: ErrRetrievalServiceMissing.Error(),
	}), ErrRetrievalServiceMissing
}

// stubJSON marshals the result and returns it as a string. Marshaling
// failures are converted to a plain string error so the model can still
// surface something to the user.
func stubJSON(r retrievalResult) string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"retrieval: marshal stub result: %s","stub":true}`, err)
	}
	return string(b)
}
