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

// Package component contains the remaining specialized Canvas adapters for
// Retrieval and CodeExec. Tools with a standard Canvas surface are registered
// through ToolBackedComponent instead.
package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func stringParam(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func anySlice(v any) []any {
	switch x := v.(type) {
	case []any:
		return x
	case []map[string]any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, item)
		}
		return out
	default:
		return []any{}
	}
}

// retrievalParams mirrors the Python RetrievalParam shape: the
// values the canvas node declares at build time, applied as
// defaults to the per-invocation RetrievalRequest. The fields are
// the same the Python agent/component/retrieval.py exposes.
type retrievalParams struct {
	KbIDs                    []string
	TopN                     int
	TopK                     int
	SimilarityThreshold      float64
	KeywordsSimilarityWeight float64
	RerankID                 string
	EmptyResponse            string
}

// parseRetrievalParams reads the v1 DSL node params for Retrieval.
// Unknown keys are ignored; nil/empty/missing-key inputs yield a
// zero-value retrievalParams which the tool layer treats as
// "default everything". This matches Python's
// component.retrieval.RetrievalParam.__init__ tolerance.
func parseRetrievalParams(params map[string]any) retrievalParams {
	out := retrievalParams{
		EmptyResponse: "Sorry, no relevant content was found in the knowledge base.",
	}
	if params == nil {
		return out
	}
	if v, ok := params["kb_ids"].([]any); ok {
		for _, x := range v {
			if s, ok := x.(string); ok {
				out.KbIDs = append(out.KbIDs, s)
			}
		}
	}
	if v, ok := params["kb_ids"].([]string); ok {
		out.KbIDs = append(out.KbIDs, v...)
	}
	if v, ok := params["top_n"]; ok {
		out.TopN = toIntParam(v)
	}
	if v, ok := params["top_k"]; ok {
		out.TopK = toIntParam(v)
	}
	if v, ok := params["similarity_threshold"]; ok {
		out.SimilarityThreshold = toFloatParam(v)
	}
	if v, ok := params["keywords_similarity_weight"]; ok {
		out.KeywordsSimilarityWeight = toFloatParam(v)
	}
	if v, ok := params["rerank_id"].(string); ok {
		out.RerankID = v
	}
	if v, ok := params["empty_response"].(string); ok {
		out.EmptyResponse = v
	}
	return out
}

// retrievalComponent delegates to internal/agent/tool/RetrievalTool.
// The wrapper captures the v1 DSL node params (kb_ids, top_n,
// top_k, similarity_threshold, keywords_similarity_weight,
// rerank_id, empty_response) at build time and applies them as
// defaults to each invocation. Per-call inputs override the
// defaults.
type retrievalComponent struct {
	inner  *agenttool.RetrievalTool
	params retrievalParams
}

var legacyRetrievalQueryPattern = regexp.MustCompile(`(?s)^\s*UserFillUp:\s*(.*?)\s+Input\s+(.*?)\s*$`)

func newRetrievalComponent(params map[string]any) (Component, error) {
	return &retrievalComponent{
		inner:  agenttool.NewRetrievalTool(),
		params: parseRetrievalParams(params),
	}, nil
}

func (c *retrievalComponent) Name() string { return "Retrieval" }

func (c *retrievalComponent) Inputs() map[string]string {
	return map[string]string{
		"query":       "Natural-language search query.",
		"dataset_ids": "Optional list of dataset IDs to restrict the search to (overrides node-level kb_ids).",
		"top_n":       "Maximum chunks to return (default 8, overrides node-level top_n).",
		"use_kg":      "GraphRAG toggle (returns ErrKGRetrievalServiceMissing until a kg adapter is registered).",
	}
}

func (c *retrievalComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered chunks for downstream LLM prompts.",
		"chunks":             "Raw chunk payloads (id, document_id, content, score).",
	}
}

func (c *retrievalComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := c.applyDefaults(inputs)
	normalizeLegacyRetrievalInputs(ctx, merged)
	common.Debug("agent retrieval component: invoke",
		zap.Any("inputs", inputs),
		zap.Any("merged", merged),
	)
	argsJSON, _ := json.Marshal(merged)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("canvas: Retrieval: %w", err)
	}
	common.Debug("agent retrieval component: output",
		zap.String("tool_output", out),
	)
	return parseToolEnvelope(out), nil
}

func (c *retrievalComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	// V1: retrieval is a non-streaming node (the Python
	// Retrieval component also blocks on Dealer.search). A
	// streaming retrieval lands with the streaming-dealer
	// follow-up phase. The single-chunk fallback convention
	// (one {"_raw": "..."} frame) is intentionally NOT
	// emitted here because the canvas scheduler treats
	// nil-stream as "non-streaming node, read Invoke() output"
	// — a fallback frame would confuse downstream cpn wiring.
	return nil, nil
}

// applyDefaults folds the node-level params into the per-call
// input map. Per-call values always win; node-level values fill
// the gaps. This mirrors Python's RetrievalParam semantics where
// the canvas DSL declares the defaults and the runtime input
// overrides them.
//
// v1 → tool field name: the wrapper writes node-level kb_ids
// (the v1 DSL / Python surface) but the tool's retrievalArgs
// JSON struct only declares `dataset_ids`. Passing kb_ids
// through unchanged would silently drop the filter (the tool's
// json.Unmarshal would never see the key). To avoid that
// regression, we normalise the merged map here: any `kb_ids`
// present (whether from defaults or inputs) is copied into
// `dataset_ids` (unless dataset_ids is already set), and the
// stale `kb_ids` key is removed so the marshalled payload to
// the tool carries exactly one canonical name.
func (c *retrievalComponent) applyDefaults(inputs map[string]any) map[string]any {
	out := make(map[string]any, len(inputs)+8)
	for k, v := range inputs {
		out[k] = v
	}
	if _, ok := out["kb_ids"]; !ok && len(c.params.KbIDs) > 0 {
		ids := make([]any, len(c.params.KbIDs))
		for i, s := range c.params.KbIDs {
			ids[i] = s
		}
		out["kb_ids"] = ids
	}
	if _, ok := out["top_n"]; !ok && c.params.TopN > 0 {
		out["top_n"] = c.params.TopN
	}
	if _, ok := out["top_k"]; !ok && c.params.TopK > 0 {
		out["top_k"] = c.params.TopK
	}
	if _, ok := out["similarity_threshold"]; !ok && c.params.SimilarityThreshold > 0 {
		out["similarity_threshold"] = c.params.SimilarityThreshold
	}
	if _, ok := out["keywords_similarity_weight"]; !ok && c.params.KeywordsSimilarityWeight > 0 {
		out["keywords_similarity_weight"] = c.params.KeywordsSimilarityWeight
	}
	if _, ok := out["rerank_id"]; !ok && c.params.RerankID != "" {
		out["rerank_id"] = c.params.RerankID
	}
	if _, ok := out["empty_response"]; !ok && c.params.EmptyResponse != "" {
		out["empty_response"] = c.params.EmptyResponse
	}
	// Translate v1 DSL name `kb_ids` to the tool's expected
	// name `dataset_ids`. dataset_ids already-set wins; kb_ids
	// is consumed and removed so the marshalled JSON carries a
	// single canonical key. Without this step, the tool's
	// retrievalArgs.DatasetIDs would be empty even when the
	// caller supplied kb_ids at build or call time.
	if kbIDs, ok := out["kb_ids"]; ok {
		if _, hasDatasetIDs := out["dataset_ids"]; !hasDatasetIDs {
			out["dataset_ids"] = kbIDs
		}
		delete(out, "kb_ids")
	}
	return out
}

func normalizeLegacyRetrievalInputs(ctx context.Context, out map[string]any) {
	if normalizeStructuredRetrievalInputs(ctx, out) {
		return
	}
	rawQuery, _ := out["query"].(string)
	rawQuery = strings.TrimSpace(rawQuery)
	if rawQuery == "" {
		return
	}
	matches := legacyRetrievalQueryPattern.FindStringSubmatch(rawQuery)
	if len(matches) != 3 {
		return
	}
	kbName := strings.TrimSpace(matches[1])
	queryText := strings.TrimSpace(matches[2])
	if queryText != "" {
		out["query"] = queryText
	}
	if _, hasDatasetIDs := out["dataset_ids"]; hasDatasetIDs {
		return
	}
	if kbName == "" {
		return
	}
	if datasetID := resolveRetrievalDatasetID(ctx, kbName); datasetID != "" {
		out["dataset_ids"] = []string{datasetID}
	}
}

func normalizeStructuredRetrievalInputs(ctx context.Context, out map[string]any) bool {
	_, hasDatasetIDs := out["dataset_ids"]
	candidateMaps := []map[string]any{}
	if stateMap, ok := out["state"].(map[string]any); ok {
		if raw, ok := stateMap["UserFillUp:KBInput"].(map[string]any); ok {
			candidateMaps = append(candidateMaps, raw)
		}
	}
	candidateMaps = append(candidateMaps, out)

	consumed := false
	for _, candidate := range candidateMaps {
		kbName, _ := candidate["kb"].(string)
		queryText, _ := candidate["query"].(string)
		if kbName == "" && legacyRetrievalQueryPattern.MatchString(strings.TrimSpace(queryText)) {
			continue
		}
		if kbName == "" && queryText == "" {
			continue
		}
		consumed = true
		if queryText != "" {
			out["query"] = queryText
		}
		if kbName != "" && !hasDatasetIDs {
			if datasetID := resolveRetrievalDatasetID(ctx, strings.TrimSpace(kbName)); datasetID != "" {
				out["dataset_ids"] = []string{datasetID}
				common.Debug("agent retrieval component: resolved dataset id")
			}
		}
		if queryText != "" {
			return true
		}
		if kbName != "" && out["dataset_ids"] != nil {
			return true
		}
	}
	return consumed
}

func resolveRetrievalDatasetID(ctx context.Context, kbName string) string {
	if kbName == "" {
		return ""
	}
	if kb, err := dao.NewKnowledgebaseDAO().GetByID(kbName); err == nil && kb != nil {
		common.Debug("agent retrieval component: resolved dataset id by direct id")
		return kb.ID
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.Warn("agent retrieval component: resolve dataset id by id failed",
			zap.Error(err))
	}
	if state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx); err == nil && state != nil {
		common.Debug("agent retrieval component: resolve dataset id context")
		if tenantID, _ := state.Sys["tenant_id"].(string); tenantID != "" {
			if kb, lookupErr := dao.NewKnowledgebaseDAO().GetByName(kbName, tenantID); lookupErr == nil && kb != nil {
				common.Debug("agent retrieval component: resolved dataset id by tenant")
				return kb.ID
			} else if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				common.Warn("agent retrieval component: resolve dataset id by tenant failed",
					zap.Error(lookupErr))
			} else {
				common.Debug("agent retrieval component: tenant lookup missed")
			}
		}
		if userID, _ := state.Sys["user_id"].(string); userID != "" {
			if kbs, lookupErr := dao.NewKnowledgebaseDAO().GetKBByNameAndUserID(kbName, userID); lookupErr == nil && len(kbs) > 0 {
				for _, kb := range kbs {
					if kb == nil || kb.Status == nil || *kb.Status != string(entity.StatusValid) {
						continue
					}
					common.Debug("agent retrieval component: resolved dataset id by user visibility")
					return kb.ID
				}
			} else if lookupErr != nil {
				common.Warn("agent retrieval component: resolve dataset id by name failed",
					zap.Error(lookupErr))
			} else {
				common.Debug("agent retrieval component: user visibility lookup missed")
			}
		}
	} else {
		common.Debug("agent retrieval component: resolve dataset id missing canvas state",
			zap.Error(err))
	}
	common.Debug("agent retrieval component: dataset id unresolved")
	return ""
}

// codeExecComponent delegates to internal/agent/tool/CodeExecTool.
// The node-level params map carries the legacy v1 DSL surface
// (`lang`, `script`, `arguments`, optional `timeout`). Per-call inputs
// override those defaults so resolved canvas refs win at invocation
// time, while static DSL-provided literals still flow through.
type codeExecComponent struct {
	inner   *agenttool.CodeExecTool
	params  map[string]any
	outputs map[string]any
}

func newCodeExecComponent(params map[string]any) (Component, error) {
	cloned := make(map[string]any, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return &codeExecComponent{
		inner:   agenttool.NewCodeExecTool(),
		params:  cloned,
		outputs: cloneAnyMap(asAnyMap(params["outputs"])),
	}, nil
}

func (c *codeExecComponent) Name() string { return "CodeExec" }

func (c *codeExecComponent) Inputs() map[string]string {
	return map[string]string{
		"lang":      "Programming language: python/python3/javascript/nodejs.",
		"script":    "Code to execute. Should define main(...).",
		"arguments": "Arguments passed to main(...) as keyword args / object fields.",
		"timeout":   "Optional per-execution timeout in seconds.",
	}
}

func (c *codeExecComponent) GetInputForm() map[string]any {
	res := make(map[string]any, len(c.params))
	for k, _ := range c.params {
		res[k] = map[string]any{
			"type": "line",
			"name": k,
		}
	}
	return res
}

func (c *codeExecComponent) Outputs() map[string]string {
	return map[string]string{
		"result":      "The main(...) return value rendered as the legacy CodeExec result field.",
		"content":     "Raw CodeExec tool content field.",
		"_ERROR":      "Execution or sandbox error message.",
		"actual_type": "Runtime type inferred by the sandbox bridge.",
		"stdout":      "Captured stdout.",
		"stderr":      "Captured stderr.",
		"exit_code":   "Process exit code.",
	}
}

func (c *codeExecComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := make(map[string]any, len(c.params)+len(inputs))
	for k, v := range c.params {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}
	if rawArgs, ok := merged["arguments"].(map[string]any); ok {
		state, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
		merged["arguments"] = resolveCodeExecArguments(rawArgs, merged, state)
	}
	common.Debug("CodeExec wrapper invoke",
		zap.Int("params_keys", len(c.params)),
		zap.Int("inputs_keys", len(inputs)),
		zap.Int("merged_keys", len(merged)),
		zap.Bool("has_arguments", merged["arguments"] != nil))
	argsJSON, _ := json.Marshal(merged)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if c.outputs != nil {
		applyCodeExecBusinessOutputs(decoded, c.outputs)
	} else if rawResult, ok := decoded["raw_result"]; ok {
		decoded["result"] = rawResult
		if _, ok := decoded["_ERROR"]; !ok {
			decoded["_ERROR"] = ""
		}
	} else if content, ok := decoded["content"]; ok {
		decoded["result"] = content
		if _, ok := decoded["_ERROR"]; !ok {
			decoded["_ERROR"] = ""
		}
	}
	if err != nil {
		return decoded, fmt.Errorf("canvas: CodeExec: %w", err)
	}
	return decoded, nil
}

func (c *codeExecComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// parseToolEnvelope decodes the JSON envelope returned by eino tool
// InvokableRun into a map[string]any. The result has whatever keys
// the tool's result type carries (rows/columns/chunks/etc.).
func parseToolEnvelope(jsonStr string) map[string]any {
	var out map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		// Tool returned non-JSON; surface the raw string under a
		// known key so the caller can still see something.
		return map[string]any{"_raw": jsonStr}
	}
	return out
}

func applyCodeExecBusinessOutputs(decoded map[string]any, outputs map[string]any) {
	if decoded == nil {
		return
	}
	rawResult := resolveCodeExecBusinessResult(decoded)
	common.Debug("CodeExec wrapper",
		zap.Int("decoded_keys", len(decoded)),
		zap.Bool("has_raw_result", rawResult != nil),
		zap.Bool("has_content", decoded["content"] != nil),
		zap.Int("outputs_keys", len(outputs)))
	if existingErr, _ := decoded["_ERROR"].(string); strings.TrimSpace(existingErr) != "" {
		for name := range outputs {
			if isCodeExecSystemOutput(name) {
				continue
			}
			decoded[name] = nil
		}
		if _, ok := decoded["actual_type"]; !ok {
			decoded["actual_type"] = agenttool.InferCodeExecActualType(rawResult)
		}
		if _, ok := decoded["content"]; !ok {
			decoded["content"] = agenttool.RenderCodeExecCanonicalContent(rawResult)
		}
		return
	}
	contract, err := agenttool.BuildCodeExecContract(outputs, rawResult)
	if err != nil {
		for name := range outputs {
			if isCodeExecSystemOutput(name) {
				continue
			}
			decoded[name] = nil
		}
		decoded["actual_type"] = agenttool.InferCodeExecActualType(rawResult)
		decoded["_ERROR"] = err.Error()
		if _, ok := decoded["content"]; !ok {
			decoded["content"] = agenttool.RenderCodeExecCanonicalContent(rawResult)
		}
		return
	}

	decoded["_ERROR"] = ""
	decoded["actual_type"] = contract.ActualType
	decoded["content"] = contract.Content
	decoded[contract.BusinessOutput] = contract.Value
}

func resolveCodeExecBusinessResult(decoded map[string]any) any {
	if decoded == nil {
		return nil
	}
	if rawResult, ok := decoded["raw_result"]; ok {
		return rawResult
	}
	content, _ := decoded["content"].(string)
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		return parsed
	}
	return content
}

func isCodeExecSystemOutput(name string) bool {
	switch name {
	case "content", "actual_type", "attachments", "_ERROR", "_ARTIFACTS", "_ATTACHMENT_CONTENT", "raw_result", "_created_time", "_elapsed_time":
		return true
	default:
		return false
	}
}

func asAnyMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func resolveCodeExecArguments(args map[string]any, merged map[string]any, state *runtime.CanvasState) map[string]any {
	if args == nil {
		return nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = resolveCodeExecArgumentValue(v, merged, state)
	}
	return out
}

func resolveCodeExecArgumentValue(v any, merged map[string]any, state *runtime.CanvasState) any {
	switch x := v.(type) {
	case map[string]any:
		return resolveCodeExecArguments(x, merged, state)
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, resolveCodeExecArgumentValue(item, merged, state))
		}
		return out
	case string:
		if resolved, ok := lookupCodeExecArgumentRef(x, merged, state); ok {
			return resolved
		}
		return x
	default:
		return v
	}
}

func lookupCodeExecArgumentRef(ref string, merged map[string]any, state *runtime.CanvasState) (any, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, false
	}
	if state != nil {
		if v, err := state.GetVar(ref); err == nil && v != nil {
			return v, true
		}
	}
	at := strings.Index(ref, "@")
	if at <= 0 || at >= len(ref)-1 {
		return nil, false
	}
	cpnID := ref[:at]
	param := ref[at+1:]

	stateByNode, _ := merged["state"].(map[string]map[string]any)
	if bucket, ok := stateByNode[cpnID]; ok {
		if v, ok := bucket[param]; ok {
			return v, true
		}
	}
	return nil, false
}

// toIntParam coerces a node-param int value to int. JSON-decoded
// values come in as float64 when numeric, so we tolerate that
// case explicitly. Strings that parse as int also work.
// (Renamed from toInt to avoid colliding with
// list_operations.go's same-name helper.)
func toIntParam(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	}
	return 0
}

// toFloatParam coerces a node-param float value to float64. Same
// JSON-float64 / numeric-string tolerance as toIntParam.
func toFloatParam(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}

// Compile-time interface checks.
var (
	_ Component = (*retrievalComponent)(nil)
	_ Component = (*codeExecComponent)(nil)
)

// Compile-time check that the eino InvokableTool methods we call
// are reachable (catches a future refactor that renames them).
