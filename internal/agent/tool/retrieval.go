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
	"ragflow/internal/dao"
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
	Query                    string         `json:"query"`
	DatasetIDs               []string       `json:"dataset_ids,omitempty"`
	KBIDs                    []string       `json:"kb_ids,omitempty"`
	MemoryIDs                []string       `json:"memory_ids,omitempty"`
	UserID                   string         `json:"user_id,omitempty"`
	TopN                     int            `json:"top_n,omitempty"`
	RerankCandidatesCount    int            `json:"rerank_candidates_count,omitempty"`
	TopK                     int            `json:"top_k,omitempty"`
	KeywordsSimilarityWeight *float64       `json:"keywords_similarity_weight,omitempty"`
	UseKG                    bool           `json:"use_kg,omitempty"`
	SimilarityThreshold      *float64       `json:"similarity_threshold,omitempty"`
	RerankID                 string         `json:"rerank_id,omitempty"`
	CrossLanguages           []string       `json:"cross_languages,omitempty"`
	TOCEnhance               bool           `json:"toc_enhance,omitempty"`
	MetaDataFilter           map[string]any `json:"meta_data_filter,omitempty"`
	RetrievalFrom            string         `json:"retrieval_from,omitempty"`
	EmptyResponse            string         `json:"empty_response,omitempty"`
}

// retrievalResult is the JSON shape returned to the model. The `_ERROR`
// field matches the Python tool's output convention; downstream components
// can pattern-match on it.
type retrievalResult struct {
	FormalizedContent string         `json:"formalized_content"`
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
	resolvedQuery, err := resolveRetrievalQuery(ctx, args.Query)
	if err != nil {
		return "", err
	}
	args.Query = resolvedQuery
	resolvedUserID, err := resolveRetrievalUserID(ctx, args.UserID)
	if err != nil {
		return "", err
	}
	args.UserID = resolvedUserID
	common.Debug("agent retrieval tool: parsed arguments",
		zap.String("query", args.Query),
		zap.Strings("dataset_ids", args.DatasetIDs),
		zap.Int("top_n", args.TopN),
		zap.Int("top_k", args.TopK),
		zap.Float64p("keywords_similarity_weight", args.KeywordsSimilarityWeight),
		zap.Bool("use_kg", args.UseKG),
	)
	if args.Query == "" {
		return stubJSONWithErr(retrievalResult{FormalizedContent: args.EmptyResponse})
	}
	if args.UseKG {
		return stubJSON(retrievalResult{
			Stub:  true,
			Error: ErrGraphRAGNotSupported.Error(),
		}), ErrGraphRAGNotSupported
	}
	if args.RetrievalFrom == "" {
		return stubJSONWithErr(retrievalResult{FormalizedContent: args.EmptyResponse})
	}
	if args.RetrievalFrom != "dataset" && args.RetrievalFrom != "memory" {
		return "", fmt.Errorf("retrieval: unsupported retrieval_from %q", args.RetrievalFrom)
	}
	if args.RetrievalFrom == "dataset" && len(args.DatasetIDs) == 0 {
		return "", fmt.Errorf("retrieval: dataset_ids is required")
	}
	if args.RetrievalFrom == "memory" && len(args.MemoryIDs) == 0 {
		return "", fmt.Errorf("retrieval: memory_ids is required")
	}
	resolvedDatasetIDs, err := resolveRetrievalDatasetIDs(ctx, args.DatasetIDs)
	if err != nil {
		return "", err
	}
	args.DatasetIDs = resolvedDatasetIDs
	resolvedFilter, err := resolveRetrievalFilter(ctx, args.MetaDataFilter)
	if err != nil {
		return "", err
	}
	args.MetaDataFilter = resolvedFilter

	// Dispatch to the registered RetrievalService. When the
	// default stub is in place, the call surfaces
	// ErrRetrievalServiceMissing; once a real impl is installed
	// via SetRetrievalService (or SetSimpleRetrievalService for
	// dev), the chunks flow through normally.
	searchReq := RetrievalRequest{
		Query:                    args.Query,
		DatasetIDs:               args.DatasetIDs,
		MemoryIDs:                args.MemoryIDs,
		TopN:                     args.TopN,
		RerankCandidatesCount:    args.RerankCandidatesCount,
		TopK:                     args.TopK,
		KeywordsSimilarityWeight: args.KeywordsSimilarityWeight,
		UseKG:                    args.UseKG,
		SimilarityThreshold:      args.SimilarityThreshold,
		RerankID:                 args.RerankID,
		CrossLanguages:           append([]string(nil), args.CrossLanguages...),
		TOCEnhance:               args.TOCEnhance,
		MetaDataFilter:           cloneStringAnyMap(args.MetaDataFilter),
		RetrievalFrom:            args.RetrievalFrom,
		UserID:                   args.UserID,
		TenantID:                 retrievalTenantID(ctx),
	}

	var chunks []RetrievalChunk
	if args.RetrievalFrom == "memory" {
		chunks, err = GetMemoryRetrievalService().Search(ctx, dao.DB, searchReq)
	} else {
		chunks, err = GetRetrievalService().Search(ctx, dao.DB, searchReq)
	}
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
	formalizedContent := renderChunks(chunks, args.Query)
	if args.RetrievalFrom == "memory" {
		formalizedContent = renderMemoryChunks(chunks)
	}
	if len(chunks) == 0 {
		formalizedContent = args.EmptyResponse
	}
	out := retrievalResult{FormalizedContent: formalizedContent, Chunks: payload}
	// Record chunks into canvas state so the Agent's post-stream
	// citation grounding call can read them. The recording is
	// best-effort — when the canvas state is not
	// attached (e.g. unit tests), we skip silently.
	if state, _, sErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); sErr == nil && state != nil && len(chunks) > 0 && args.RetrievalFrom == "dataset" {
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
	if len(args.MemoryIDs) == 0 && len(r.defaults.MemoryIDs) != 0 {
		args.MemoryIDs = append([]string(nil), r.defaults.MemoryIDs...)
	}
	if args.TopN <= 0 {
		args.TopN = r.defaults.TopN
	}
	if args.RerankCandidatesCount <= 0 {
		args.RerankCandidatesCount = r.defaults.RerankCandidatesCount
	}
	if args.TopK <= 0 {
		args.TopK = r.defaults.TopK
	}
	if args.KeywordsSimilarityWeight == nil {
		args.KeywordsSimilarityWeight = r.defaults.KeywordsSimilarityWeight
	}
	if args.SimilarityThreshold == nil {
		args.SimilarityThreshold = r.defaults.SimilarityThreshold
	}
	if args.EmptyResponse == "" {
		args.EmptyResponse = r.defaults.EmptyResponse
	}
	if args.UserID == "" {
		args.UserID = r.defaults.UserID
	}
	if args.RerankID == "" {
		args.RerankID = r.defaults.RerankID
	}
	if len(args.CrossLanguages) == 0 && len(r.defaults.CrossLanguages) != 0 {
		args.CrossLanguages = append([]string(nil), r.defaults.CrossLanguages...)
	}
	if args.MetaDataFilter == nil && r.defaults.MetaDataFilter != nil {
		args.MetaDataFilter = cloneStringAnyMap(r.defaults.MetaDataFilter)
	}
	if args.RetrievalFrom == "" {
		args.RetrievalFrom = r.defaults.RetrievalFrom
	}
	if args.RetrievalFrom == "" && len(args.DatasetIDs) > 0 {
		args.RetrievalFrom = "dataset"
	}
	if args.RetrievalFrom == "" && len(args.MemoryIDs) > 0 {
		args.RetrievalFrom = "memory"
	}
	args.TOCEnhance = args.TOCEnhance || r.defaults.TOCEnhance
	args.UseKG = args.UseKG || r.defaults.UseKG
	return args
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func resolveRetrievalQuery(ctx context.Context, query string) (string, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return query, nil
	}
	resolved, err := runtime.ResolveTemplateAuto(query, state)
	if err != nil {
		return "", fmt.Errorf("retrieval: resolve query variables: %w", err)
	}
	return resolved, nil
}

// resolveRetrievalUserID resolves the memory user_id filter. Mirrors Python's
// Retrieval._retrieve_memory: a variable reference — `{sys.user_id}` template
// or bare `sys.*` / `env.*` / `component@param` form — is looked up in the
// canvas state; anything else is a literal user id and passes through.
//
// Two deliberate divergences from Python, which resolves only the fully
// braced form and passes bare refs through literally (agent/tools/retrieval.py
// `_retrieve_memory`):
//
//   - bare ref forms are resolved too, so a canvas storing `sys.user_id`
//     unbraced still filters per user; as a corollary, a literal user id
//     that collides with a live state variable name is substituted (RAGFlow
//     user ids are UUID-like and never look like refs);
//   - an unset bare `sys.*` / `env.*` ref resolves to "" (no user filter)
//     instead of the literal ref string, which could never match a real user
//     id and would silently empty the result. Bare `component@param` refs
//     keep the literal fallback because an "@" in the value is ambiguous
//     with an email-style literal user id.
func resolveRetrievalUserID(ctx context.Context, userID string) (string, error) {
	trimmed := strings.TrimSpace(userID)
	if trimmed == "" {
		return "", nil
	}
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return trimmed, nil
	}
	if strings.ContainsAny(trimmed, "{}") {
		resolved, err := runtime.ResolveTemplateAuto(trimmed, state)
		if err != nil {
			return "", fmt.Errorf("retrieval: resolve user_id variable: %w", err)
		}
		return strings.TrimSpace(resolved), nil
	}
	if value, getErr := state.GetVar(trimmed); getErr == nil && value != nil {
		if text, ok := value.(string); ok {
			return text, nil
		}
		return fmt.Sprintf("%v", value), nil
	}
	if strings.HasPrefix(trimmed, "sys.") || strings.HasPrefix(trimmed, "env.") {
		// An unset sys./env. ref cannot be a literal user id; treat it as
		// "no user filter" per the RetrievalRequest.UserID contract.
		return "", nil
	}
	return trimmed, nil
}

func resolveRetrievalDatasetIDs(ctx context.Context, datasetIDs []string) ([]string, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return compactStrings(datasetIDs), nil
	}
	resolved := make([]string, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		if !strings.Contains(datasetID, "@") {
			resolved = append(resolved, datasetID)
			continue
		}
		value, getErr := state.GetVar(datasetID)
		if getErr != nil {
			return nil, fmt.Errorf("retrieval: resolve dataset variable %q: %w", datasetID, getErr)
		}
		if value == nil {
			return nil, fmt.Errorf("retrieval: dataset variable %q is empty", datasetID)
		}
		switch typed := value.(type) {
		case string:
			resolved = append(resolved, typed)
		case []string:
			resolved = append(resolved, typed...)
		case []any:
			for _, item := range typed {
				text, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("retrieval: dataset variable %q contains non-string value", datasetID)
				}
				resolved = append(resolved, text)
			}
		default:
			return nil, fmt.Errorf("retrieval: dataset variable %q must be a string or string list", datasetID)
		}
	}
	return compactStrings(resolved), nil
}

func resolveRetrievalFilter(ctx context.Context, filter map[string]any) (map[string]any, error) {
	if filter == nil {
		return nil, nil
	}
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return cloneStringAnyMap(filter), nil
	}
	resolved, err := resolveRetrievalValue(filter, state)
	if err != nil {
		return nil, err
	}
	result, ok := resolved.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("retrieval: metadata filter must be an object")
	}
	return result, nil
}

func resolveRetrievalValue(value any, state *runtime.CanvasState) (any, error) {
	switch typed := value.(type) {
	case string:
		resolved, err := runtime.ResolveTemplateAuto(typed, state)
		if err != nil {
			return nil, fmt.Errorf("retrieval: resolve metadata filter value: %w", err)
		}
		return resolved, nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			resolved, err := resolveRetrievalValue(item, state)
			if err != nil {
				return nil, err
			}
			result[key] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			resolved, err := resolveRetrievalValue(item, state)
			if err != nil {
				return nil, err
			}
			result[index] = resolved
		}
		return result, nil
	default:
		return value, nil
	}
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

func renderMemoryChunks(chunks []RetrievalChunk) string {
	var builder strings.Builder
	for index, chunk := range chunks {
		if index > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(chunk.Content)
	}
	return builder.String()
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
