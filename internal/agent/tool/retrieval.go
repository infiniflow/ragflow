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
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
)

// ErrGraphRAGNotSupported is returned by the Retrieval tool when
// callers pass use_kg=true. GraphRAG support is a future
// enhancement; users must either disable use_kg or fall back to
// the Python Canvas.
var ErrGraphRAGNotSupported = errors.New("GraphRAG 检索暂不支持，请使用 Python Canvas 或关闭 use_kg")

// ErrRetrievalServiceMissing is returned when the
// internal/service/nlp RetrievalService is not registered. Wire a
// real implementation via SetRetrievalService at boot to resolve.
var ErrRetrievalServiceMissing = errors.New(
	"Retrieval service not yet implemented (service not registered) — " +
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

// RetrievalTool is the Retrieval tool. It validates the input
// (rejecting use_kg=true with ErrGraphRAGNotSupported) and
// dispatches to the registered RetrievalService via
// SetRetrievalService. When no service is registered, the call
// surfaces ErrRetrievalServiceMissing.
type RetrievalTool struct{}

// NewRetrievalTool returns a RetrievalTool implementing eino's
// tool.InvokableTool interface.
func NewRetrievalTool() *RetrievalTool {
	return &RetrievalTool{}
}

// Info returns the tool's metadata for the chat model. The schema mirrors
// the Python RetrievalParam ToolMeta (plan , 字段对齐).
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
				Desc:     "GraphRAG toggle. Not supported in Go Canvas (plan ); must be false.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun executes the tool. It validates the input and
// dispatches to the registered RetrievalService. When no
// service is registered, the call surfaces
// ErrRetrievalServiceMissing.
func (r *RetrievalTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	var args retrievalArgs
	if argumentsInJSON != "" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("retrieval: parse arguments: %w", err)
		}
	}
	common.Debug("agent retrieval tool: parsed arguments",
		zap.String("query", args.Query),
		zap.Strings("dataset_ids", args.DatasetIDs),
		zap.Int("top_n", args.TopN),
		zap.Bool("use_kg", args.UseKG),
	)

	if args.UseKG {
		// Plan  + §9 Q3: GraphRAG is out of scope for the Go
		// Canvas. Return the structured error so the model can react.
		return stubJSON(retrievalResult{
			Stub:  true,
			Error: ErrGraphRAGNotSupported.Error(),
		}), ErrGraphRAGNotSupported
	}

	// Dispatch to the registered RetrievalService. When the
	// default stub is in place, the call surfaces
	// ErrRetrievalServiceMissing; once a real impl is installed
	// via SetRetrievalService (or SetSimpleRetrievalService for
	// dev), the chunks flow through normally.
	svc := GetRetrievalService()
	chunks, err := svc.Search(ctx, RetrievalRequest{
		Query:          args.Query,
		DatasetIDs:     args.DatasetIDs,
		TopN:           args.TopN,
		UseKG:          args.UseKG,
		UseRerank:      false, // future enhancement
		RerankID:       "",
		TOCEnhance:     false, // future enhancement
		MetadataFilter: nil,   // future enhancement
	})
	if err != nil {
		return stubJSON(retrievalResult{
			Stub:  true,
			Error: err.Error(),
		}), err
	}
	common.Debug("agent retrieval tool: search result",
		zap.Int("chunks_count", len(chunks)),
	)
	// Map the chunks into the result envelope. The retrievalResult
	// type carries the eino-tool envelope shape (chunkPayload, not
	// RetrievalChunk), so we translate.
	payload := make([]chunkPayload, 0, len(chunks))
	for _, c := range chunks {
		payload = append(payload, chunkPayload{
			ID:         c.ID,
			Content:    c.Content,
			DocumentID: c.DocumentID,
			Score:      c.Score,
		})
	}
	out := retrievalResult{
		FormalizedContent: renderChunks(chunks, args.Query),
		Chunks:            payload,
	}
	// Record chunks into canvas state so the Agent's post-stream
	// citation grounding call can read them. The recording is
	// best-effort — when the canvas state is not
	// attached (e.g. unit tests), we skip silently.
	if state, _, sErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); sErr == nil && state != nil && len(chunks) > 0 {
		asMap := make([]map[string]any, 0, len(chunks))
		for _, c := range chunks {
			asMap = append(asMap, map[string]any{
				"id":          c.ID,
				"content":     c.Content,
				"document_id": c.DocumentID,
				"score":       c.Score,
			})
		}
		state.SetRetrievalChunks(asMap)
	}
	result, err := stubJSONWithErr(out)
	if err != nil {
		return "", err
	}
	return result, nil
}

// renderChunks concatenates the retrieved chunks into a human-
// readable content string. Mirrors Python's
// `kb_prompt(kbinfos, ...)` format: each chunk gets a header
// line with its ID and document, then the content.
func renderChunks(chunks []RetrievalChunk, query string) string {
	var sb strings.Builder
	for _, c := range chunks {
		fmt.Fprintf(&sb, "[ID:%s] %s\n", c.ID, c.Content)
	}
	return sb.String()
}

// stubJSONWithErr is the (string, error) variant for call sites
// that need to propagate marshal failures.
func stubJSONWithErr(r retrievalResult) (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("retrieval: marshal result: %w", err)
	}
	return string(b), nil
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
