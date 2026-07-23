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
	Query                    string   `json:"query"`
	DatasetIDs               []string `json:"dataset_ids,omitempty"`
	KBIDs                    []string `json:"kb_ids,omitempty"`
	TopN                     int      `json:"top_n,omitempty"`
	TopK                     int      `json:"top_k,omitempty"`
	KeywordsSimilarityWeight *float64 `json:"keywords_similarity_weight,omitempty"`
	UseKG                    bool     `json:"use_kg,omitempty"`
	SimilarityThreshold      float64  `json:"similarity_threshold,omitempty"`
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
type RetrievalTool struct {
	defaults retrievalArgs
}

// NewRetrievalTool returns a RetrievalTool implementing eino's
// tool.InvokableTool interface.
func NewRetrievalTool() *RetrievalTool {
	return NewRetrievalToolWithDefaults(retrievalArgs{})
}

// NewRetrievalToolWithDefaults returns a RetrievalTool with node-level
// defaults from the Agent tool configuration.
func NewRetrievalToolWithDefaults(defaults retrievalArgs) *RetrievalTool {
	if len(defaults.DatasetIDs) == 0 && len(defaults.KBIDs) != 0 {
		defaults.DatasetIDs = append([]string(nil), defaults.KBIDs...)
	}
	return &RetrievalTool{defaults: defaults}
}

// Info returns the tool's metadata for the chat model. The schema mirrors
// the Python RetrievalParam ToolMeta (plan, field alignment).
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
	args = r.mergeDefaults(args)
	common.Debug("agent retrieval tool: parsed arguments",
		zap.String("query", args.Query),
		zap.Strings("dataset_ids", args.DatasetIDs),
		zap.Int("top_n", args.TopN),
		zap.Int("top_k", args.TopK),
		zap.Float64p("keywords_similarity_weight", args.KeywordsSimilarityWeight),
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
		Query:                    args.Query,
		DatasetIDs:               args.DatasetIDs,
		TopN:                     args.TopN,
		TopK:                     args.TopK,
		KeywordsSimilarityWeight: args.KeywordsSimilarityWeight,
		UseKG:                    args.UseKG,
		SimilarityThreshold:      args.SimilarityThreshold,
		TenantID:                 retrievalTenantID(ctx),
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
		state.SetRetrievalReferences(referenceChunksFromRetrieval(chunks), referenceDocAggsFromRetrieval(chunks))
	}
	result, err := stubJSONWithErr(out)
	if err != nil {
		return "", err
	}
	return result, nil
}

func (r *RetrievalTool) mergeDefaults(args retrievalArgs) retrievalArgs {
	if len(args.DatasetIDs) == 0 && len(args.KBIDs) != 0 {
		args.DatasetIDs = append([]string(nil), args.KBIDs...)
	}
	if len(args.DatasetIDs) == 0 && len(r.defaults.DatasetIDs) != 0 {
		args.DatasetIDs = append([]string(nil), r.defaults.DatasetIDs...)
	}
	if args.TopN <= 0 {
		args.TopN = r.defaults.TopN
	}
	if args.TopK <= 0 {
		args.TopK = r.defaults.TopK
	}
	if args.KeywordsSimilarityWeight == nil {
		args.KeywordsSimilarityWeight = r.defaults.KeywordsSimilarityWeight
	}
	if args.SimilarityThreshold <= 0 {
		args.SimilarityThreshold = r.defaults.SimilarityThreshold
	}
	args.UseKG = args.UseKG || r.defaults.UseKG
	return args
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

func retrievalTenantID(ctx context.Context) string {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return ""
	}
	if tenantID, _ := state.Sys["tenant_id"].(string); tenantID != "" {
		return tenantID
	}
	userID, _ := state.Sys["user_id"].(string)
	return userID
}

func referenceChunksFromRetrieval(chunks []RetrievalChunk) []map[string]any {
	out := make([]map[string]any, 0, len(chunks))
	for idx, c := range chunks {
		id := c.ID
		if id == "" {
			id = fmt.Sprint(idx)
		}
		chunk := map[string]any{
			"id":                  id,
			"chunk_id":            c.ID,
			"content":             c.Content,
			"content_with_weight": c.Content,
			"document_id":         c.DocumentID,
			"doc_id":              c.DocumentID,
			"document_name":       c.DocumentName,
			"docnm_kwd":           c.DocumentName,
			"dataset_id":          c.DatasetID,
			"kb_id":               c.DatasetID,
			"image_id":            c.ImageID,
			"img_id":              c.ImageID,
			"similarity":          c.Score,
			"term_similarity":     c.TermSimilarity,
			"vector_similarity":   c.VectorSimilarity,
		}
		if c.URL != "" {
			chunk["url"] = c.URL
			chunk["document_url"] = c.URL
		}
		if c.Positions != nil {
			chunk["positions"] = c.Positions
			chunk["position_int"] = c.Positions
		}
		out = append(out, chunk)
	}
	return out
}

func referenceDocAggsFromRetrieval(chunks []RetrievalChunk) []map[string]any {
	byDocID := make(map[string]map[string]any)
	order := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if c.DocumentID == "" && c.DocumentName == "" {
			continue
		}
		key := c.DocumentID
		if key == "" {
			key = c.DocumentName
		}
		agg, exists := byDocID[key]
		if !exists {
			agg = map[string]any{
				"count":    0,
				"doc_id":   c.DocumentID,
				"doc_name": c.DocumentName,
			}
			if c.URL != "" {
				agg["url"] = c.URL
			}
			byDocID[key] = agg
			order = append(order, key)
		}
		agg["count"] = agg["count"].(int) + 1
	}

	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		out = append(out, byDocID[key])
	}
	return out
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
