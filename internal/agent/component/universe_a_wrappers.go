//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Universe A delegation wrappers. Canvas-facing components that
// delegate to their corresponding Universe B tool
// implementations. The delegation pattern keeps the canvas
// scheduler's Component contract thin and the tool's
// InvokableRun interface as the actual implementation seam.
//
// Primary registration: TavilySearch, Retrieval (incl. the
// Python-typo SearchMyDataset alias), and ExeSQL all delegate to
// the real Universe B tools. fixture_stubs.go's init() wires the
// registry to these wrappers; the legacy stub-only path is
// preserved as NewRetrievalStub / NewExeSQLStub for unit tests
// that want to assert the "no service wired" state directly.
package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"ragflow/internal/agent/runtime"
)

// tavilySearchComponent delegates to internal/agent/tool/TavilyTool.
// The underlying tool makes a real HTTP call; the wrapper is the
// canvas-facing surface.
type tavilySearchComponent struct {
	inner *agenttool.TavilyTool
}

func newTavilySearchComponent(_ map[string]any) (Component, error) {
	return &tavilySearchComponent{inner: agenttool.NewTavilyTool()}, nil
}

func (c *tavilySearchComponent) Name() string { return "TavilySearch" }

func (c *tavilySearchComponent) Inputs() map[string]string {
	return map[string]string{
		"query":        "Search query.",
		"api_key":      "Tavily API key (overrides TAVILY_API_KEY env var).",
		"max_results":  "Maximum results to return (default 5).",
		"search_depth": "\"basic\" (default) or \"advanced\".",
	}
}

func (c *tavilySearchComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
		"results":            "Raw result list (url, title, content).",
	}
}

func (c *tavilySearchComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	argsJSON, _ := json.Marshal(inputs)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("canvas: TavilySearch: %w", err)
	}
	return parseToolEnvelope(out), nil
}

func (c *tavilySearchComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
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

func (c *retrievalComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
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

// exesqlComponent delegates to internal/agent/tool/ExeSQLTool. The
// connection params (db_type, host, port, database, username,
// password) are passed via the canvas node's params map at build
// time, matching Python's ExeSQLParam semantics.
//
// v1 → tool param translation: the legacy v1 ExeSQL canvas node
// surface used (database, username, host, port, password, top_n)
// and did NOT declare db_type. The tool, by contrast, REQUIRES
// db_type (and uses max_records for the row cap, not top_n). A
// naive passthrough would turn every v1 canvas into a build-time
// error (NewExeSQLConnParams returns "missing required connection
// params (db_type/host/database/username)"). The adapter below
// bridges the two surfaces so existing v1 DSLs keep compiling.
//
// Defaults applied: db_type defaults to "mysql" (matches the v1
// Python default); top_n is mapped to max_records; port is coerced
// from JSON-decoded float64 to int. See TestExeSQL_V1DSLParamsAccepted.
func newExeSQLComponent(params map[string]any) (Component, error) {
	toolParams := translateExeSQLParamsToToolShape(params)
	conn, err := agenttool.NewExeSQLConnParams(toolParams)
	if err != nil {
		return nil, fmt.Errorf("canvas: ExeSQL: %w", err)
	}
	return &exesqlComponent{inner: agenttool.NewExeSQLTool(conn)}, nil
}

// translateExeSQLParamsToToolShape adapts a v1 DSL ExeSQL params
// map into the tool's expected param surface. Idempotent: callers
// that already supply db_type / max_records / int-typed port pass
// through unchanged.
//
// Field map:
//
//	v1 surface     → tool surface
//	-------------------  --------------
//	db_type (optional) → db_type    (defaults to "mysql")
//	database      → database
//	username      → username
//	host        → host
//	port (float64)   → port      (coerced to int)
//	password      → password
//	top_n (numeric)   → max_records  (and dropped from out)
//
// Returns a fresh map; the input is not mutated.
func translateExeSQLParamsToToolShape(v1Params map[string]any) map[string]any {
	out := make(map[string]any, len(v1Params)+2)
	for k, v := range v1Params {
		out[k] = v
	}
	// db_type: required by the tool, absent in v1 DSL — default
	// to mysql to match the v1 Python default and most legacy
	// canvases. Operators wanting a different engine can set
	// db_type explicitly in the params map.
	if _, ok := out["db_type"]; !ok {
		out["db_type"] = "mysql"
	}
	// port: JSON-decoded numeric comes through as float64, but
	// NewExeSQLConnParams asserts on int via type-switch. Coerce.
	if v, ok := out["port"]; ok {
		switch x := v.(type) {
		case float64:
			out["port"] = int(x)
		case int64:
			out["port"] = int(x)
		}
	}
	// top_n: v1's row-limit param. Map to max_records (the tool's
	// equivalent). If both keys are present, max_records wins — the
	// tool's name is the canonical one.
	if v, ok := out["top_n"]; ok {
		if _, hasMaxRecords := out["max_records"]; !hasMaxRecords {
			switch x := v.(type) {
			case float64:
				out["max_records"] = int(x)
			case int:
				out["max_records"] = x
			case int64:
				out["max_records"] = int(x)
			}
		}
		delete(out, "top_n")
	}
	return out
}

type exesqlComponent struct {
	inner *agenttool.ExeSQLTool
}

func (c *exesqlComponent) Name() string { return "ExeSQL" }

func (c *exesqlComponent) Inputs() map[string]string {
	return map[string]string{
		"sql":      "SQL statement to execute (SELECT-only; DML/DDL rejected).",
		"database": "Optional target database/schema (overrides the tool's configured DB).",
	}
}

func (c *exesqlComponent) GetInputForm() map[string]any {
	return map[string]any{
		"sql": map[string]any{
			"name": "SQL",
			"type": "line",
		},
	}
}

func (c *exesqlComponent) Outputs() map[string]string {
	return map[string]string{
		"columns": "Result-set column names.",
		"rows":    "Result-set rows as column→value maps.",
		"sql":     "Resolved SQL string (after parameter substitution).",
	}
}

func (c *exesqlComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	argsJSON, _ := json.Marshal(inputs)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("canvas: ExeSQL: %w", err)
	}
	return parseToolEnvelope(out), nil
}

func (c *exesqlComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// codeExecComponentDelegate wraps the CodeExec tool as a Component.
// It delegates to agenttool.CodeExecTool.InvokableRun which handles
// sandbox dispatch, argument resolution (including {{ref}} templates),
// result normalization, and contract validation — matching the full
// Python tool surface.
type codeExecComponentDelegate struct {
	params map[string]any // default DSL params (lang, script, arguments, outputs)
}

func (c *codeExecComponentDelegate) Name() string { return componentNameCodeExec }

func (c *codeExecComponentDelegate) Inputs() map[string]string {
	return map[string]string{
		"lang":      "Programming language (python / nodejs).",
		"script":    "Code to execute. Must define a main function.",
		"arguments": "Arguments passed to the code (key-value map).",
		"outputs":   "Expected output schema (e.g. {\"result\": {\"type\": \"Number\"}}).",
	}
}

func (c *codeExecComponentDelegate) Outputs() map[string]string {
	return map[string]string{
		"result":      "Normalized return value of the executed code.",
		"content":     "String representation of the result.",
		"actual_type": "Detected type of the result (Number / String / List / …).",
		"_ERROR":      "Error message when execution fails.",
	}
}

func (c *codeExecComponentDelegate) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	// Resolve {{ref}} template patterns in argument values against
	// the state snapshot passed via inputs["state"].  The snapshot
	// maps cpn_id → param_name → value; a value like
	// "UserFillUp:CodeInput@x" is resolved against it.
	args := c.resolveArguments(inputs)

	// Merge resolved arguments into the merged input map.
	merged := make(map[string]any, len(c.params)+2)
	for k, v := range c.params {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}
	merged["arguments"] = args

	// Resolve code-exec-specific argument references against merged state.
	if rawArgs, ok := merged["arguments"].(map[string]any); ok {
		state, _, _ := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
		merged["arguments"] = resolveCodeExecArguments(rawArgs, merged, state)
	}

	common.Debug("CodeExec wrapper invoke",
		zap.Int("params_keys", len(c.params)),
		zap.Int("inputs_keys", len(inputs)),
		zap.Int("merged_keys", len(merged)),
		zap.Bool("has_arguments", merged["arguments"] != nil))

	tool := agenttool.NewCodeExecTool()
	argsJSON, mErr := json.Marshal(merged)
	if mErr != nil {
		return nil, fmt.Errorf("CodeExec: marshal: %w", mErr)
	}

	raw, err := tool.InvokableRun(ctx, string(argsJSON))
	parsed := parseToolEnvelope(raw)

	if outputs, _ := c.params["outputs"].(map[string]any); outputs != nil {
		applyCodeExecBusinessOutputs(parsed, outputs)
	} else {
		if rawResult, ok := parsed["raw_result"]; ok {
			parsed["result"] = rawResult
		}
		if _, has := parsed["_ERROR"]; !has {
			parsed["_ERROR"] = ""
		}
	}

	if err != nil {
		return parsed, fmt.Errorf("CodeExec: %w", err)
	}
	return parsed, nil
}

// resolveArguments resolves bare refs (e.g. "UserFillUp:CodeInput@x")
// in argument values against the state snapshot carried in
// inputs["state"].  The snapshot shape is map[cpnID]map[param]value —
// same as what statePre passes to component bodies.
func (c *codeExecComponentDelegate) resolveArguments(inputs map[string]any) map[string]any {
	defaults, _ := c.params["arguments"].(map[string]any)
	runtime, _ := inputs["arguments"].(map[string]any)

	// Build a flat lookup: cpn_id@param → value.
	// inputs["state"] can be map[string]any (pre-statePre snapshot) or
	// map[string]map[string]any (from tests that pass state directly).
	lookup := make(map[string]any)
	if stateRaw, ok := inputs["state"].(map[string]any); ok {
		for cpnID, paramsAny := range stateRaw {
			switch p := paramsAny.(type) {
			case map[string]any:
				for param, val := range p {
					lookup[cpnID+"@"+param] = val
				}
			}
		}
	} else if stateRaw, ok := inputs["state"].(map[string]map[string]any); ok {
		for cpnID, params := range stateRaw {
			for param, val := range params {
				lookup[cpnID+"@"+param] = val
			}
		}
	}

	out := make(map[string]any, len(defaults))
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range runtime {
		out[k] = v
	}
	// Resolve bare ref strings against the lookup.
	for k, v := range out {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if resolved, found := lookup[s]; found {
			out[k] = resolved
		}
	}
	return out
}

func (c *codeExecComponentDelegate) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// newCodeExecComponent is the factory registered in fixture_stubs.go.
func newCodeExecComponent(params map[string]any) (Component, error) {
	return &codeExecComponentDelegate{params: params}, nil
}

// parseToolEnvelope decodes the JSON envelope returned by tool
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

// yahooFinanceComponent delegates to internal/agent/tool/YahooFinanceTool.
type yahooFinanceComponent struct {
	inner *agenttool.YahooFinanceTool
}

func newYahooFinanceComponent(_ map[string]any) (Component, error) {
	return &yahooFinanceComponent{inner: agenttool.NewYahooFinanceTool()}, nil
}

func (c *yahooFinanceComponent) Name() string { return "YahooFinance" }

func (c *yahooFinanceComponent) Inputs() map[string]string {
	return map[string]string{
		"stock_code": "Stock symbol to look up (e.g. AAPL, MSFT, 0005.HK).",
	}
}

func (c *yahooFinanceComponent) Outputs() map[string]string {
	return map[string]string{
		"report": "Stock quote data (JSON).",
	}
}

func (c *yahooFinanceComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	stockCode, _ := inputs["stock_code"].(string)
	if strings.TrimSpace(stockCode) == "" {
		return map[string]any{"_ERROR": "stock_code is required"}, nil
	}
	toolInput := map[string]any{
		"symbols": []string{stockCode},
	}
	argsJSON, _ := json.Marshal(toolInput)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		if out != "" {
			return parseToolEnvelope(out), nil
		}
		return nil, fmt.Errorf("canvas: YahooFinance: %w", err)
	}
	result := parseToolEnvelope(out)
	return map[string]any{"report": result["results"]}, nil
}

func (c *yahooFinanceComponent) GetInputForm() map[string]any {
	return map[string]any{
		"stock_code": map[string]any{
			"type": "line",
			"name": "Stock code/Company name",
		},
	}
}

func (c *yahooFinanceComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

// arxivComponent delegates to internal/agent/tool/ArxivTool. Query is the
// runtime input; top_n and sort_by are validated node parameters.
type arxivComponent struct {
	inner *agenttool.ArxivTool
}

func newArxivComponent(params map[string]any) (Component, error) {
	topN := 12
	if v, ok := params["top_n"]; ok {
		topN = toIntParam(v)
	}
	if topN <= 0 {
		return nil, fmt.Errorf("canvas: ArXiv: top_n must be a positive integer")
	}
	sortBy := "submittedDate"
	if v, ok := params["sort_by"].(string); ok && strings.TrimSpace(v) != "" {
		sortBy = strings.TrimSpace(v)
	}
	if !agenttool.ArxivSortBySupported(sortBy) {
		return nil, fmt.Errorf("canvas: ArXiv: unsupported sort_by %q", sortBy)
	}
	return &arxivComponent{inner: agenttool.NewArxivToolWithParams(nil, topN, sortBy)}, nil
}

func (c *arxivComponent) Name() string { return "ArXiv" }

func (c *arxivComponent) Inputs() map[string]string {
	return map[string]string{
		"query": "Search query.",
	}
}

func (c *arxivComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered arXiv papers for downstream LLM prompts.",
		"json":               "Raw arXiv paper list.",
	}
}

func (c *arxivComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *arxivComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	argsJSON, _ := json.Marshal(map[string]any{"query": query})
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: ArXiv: %w", err)
	}
	results := anySlice(decoded["results"])
	return map[string]any{
		"formalized_content": renderArxivResults(results),
		"json":               results,
	}, nil
}

func (c *arxivComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func renderArxivResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		paper, ok := item.(map[string]any)
		if !ok {
			continue
		}
		summary := strings.TrimSpace(stringParam(paper["summary"]))
		if summary == "" {
			continue
		}
		title := strings.TrimSpace(stringParam(paper["title"]))
		url := strings.TrimSpace(stringParam(paper["pdf_url"]))
		lines := make([]string, 0, 3)
		if title != "" {
			lines = append(lines, fmt.Sprintf("Title: %s", title))
		}
		if url != "" {
			lines = append(lines, fmt.Sprintf("URL: %s", url))
		}
		lines = append(lines, summary)
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// googleScholarComponent delegates to internal/agent/tool/GoogleScholarTool.
type googleScholarComponent struct {
	inner  googleScholarInvoker
	params map[string]any
}

type googleScholarInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string) (string, error)
}

type pubmedInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string) (string, error)
}

func newGoogleScholarComponent(params map[string]any) (Component, error) {
	cloned := make(map[string]any, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return &googleScholarComponent{
		inner:  agenttool.NewGoogleScholarTool(),
		params: cloned,
	}, nil
}

func newGoogleScholarComponentWithInvoker(inner googleScholarInvoker, params map[string]any) Component {
	cloned := make(map[string]any, len(params))
	for k, v := range params {
		cloned[k] = v
	}
	return &googleScholarComponent{inner: inner, params: cloned}
}

func (c *googleScholarComponent) Name() string { return "GoogleScholar" }

func (c *googleScholarComponent) Inputs() map[string]string {
	return map[string]string{
		"query":     "Search query.",
		"top_n":     "Maximum number of results (default 12).",
		"sort_by":   "Sort order: relevance or date.",
		"year_low":  "Earliest publication year to include.",
		"year_high": "Latest publication year to include.",
		"patents":   "Whether to include patents, defaults to true.",
	}
}

func (c *googleScholarComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered search results for downstream LLM prompts.",
		"json":               "Raw result list.",
	}
}

func (c *googleScholarComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *googleScholarComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	merged := make(map[string]any, len(c.params)+len(inputs))
	for k, v := range c.params {
		merged[k] = v
	}
	for k, v := range inputs {
		merged[k] = v
	}

	query := strings.TrimSpace(stringParam(merged["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	args := map[string]any{
		"query": query,
	}
	if topN := toIntParam(merged["top_n"]); topN > 0 {
		args["top_n"] = topN
	}
	if sortBy := strings.TrimSpace(stringParam(merged["sort_by"])); sortBy != "" {
		args["sort_by"] = sortBy
	}
	if yearLow := toIntParam(merged["year_low"]); yearLow > 0 {
		args["year_low"] = yearLow
	}
	if yearHigh := toIntParam(merged["year_high"]); yearHigh > 0 {
		args["year_high"] = yearHigh
	}
	if patents, ok := merged["patents"].(bool); ok {
		args["patents"] = patents
	} else {
		args["patents"] = true
	}

	argsJSON, _ := json.Marshal(args)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               []any{},
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: GoogleScholar: %w", err)
	}

	results := anySlice(decoded["results"])
	return map[string]any{
		"formalized_content": renderGoogleScholarResults(results),
		"json":               results,
	}, nil
}

func (c *googleScholarComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func renderGoogleScholarResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, item := range results {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		field := func(key string) string {
			v, ok := m[key]
			if !ok || v == nil {
				return "-"
			}
			text := strings.TrimSpace(fmt.Sprintf("%v", v))
			if text == "" {
				return "-"
			}
			return text
		}
		blocks = append(blocks, strings.Join([]string{
			fmt.Sprintf("Title: %s", field("title")),
			fmt.Sprintf("URL: %s", field("link")),
			fmt.Sprintf("Authors: %s", field("authors")),
			fmt.Sprintf("Year: %s", field("year")),
			fmt.Sprintf("Snippet: %s", field("snippet")),
		}, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// pubMedComponent delegates to the PubMed tool. Its node parameters are
// consumed at construction time, leaving query as the sole runtime input.
type pubMedComponent struct {
	inner pubmedInvoker
}

func newPubMedComponent(params map[string]any) (Component, error) {
	toolParams := make(map[string]any, 2)
	for _, key := range []string{"top_n", "email"} {
		if value, ok := params[key]; ok {
			toolParams[key] = value
		}
	}
	inner, err := agenttool.BuildByName("pubmed", toolParams)
	if err != nil {
		return nil, err
	}
	invoker, ok := inner.(pubmedInvoker)
	if !ok {
		return nil, fmt.Errorf("PubMed: tool does not implement InvokableRun")
	}
	return newPubMedComponentWithInvoker(invoker), nil
}

func newPubMedComponentWithInvoker(inner pubmedInvoker) Component {
	return &pubMedComponent{inner: inner}
}

func (c *pubMedComponent) Name() string { return "PubMed" }

func (c *pubMedComponent) Inputs() map[string]string {
	return map[string]string{
		"query": "PubMed search query.",
	}
}

func (c *pubMedComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered PubMed references for downstream LLM prompts.",
		"json":               "Raw PubMed result list.",
	}
}

func (c *pubMedComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *pubMedComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := strings.TrimSpace(stringParam(inputs["query"]))
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	argsJSON, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return nil, fmt.Errorf("canvas: PubMed: encode query: %w", err)
	}
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	results := anySlice(decoded["results"])
	if existing, _ := decoded["_ERROR"].(string); strings.TrimSpace(existing) != "" {
		return map[string]any{
			"formalized_content": "",
			"json":               results,
			"_ERROR":             existing,
		}, nil
	}
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{
				"formalized_content": "",
				"json":               results,
				"_ERROR":             decoded["_ERROR"],
			}, nil
		}
		return nil, fmt.Errorf("canvas: PubMed: %w", err)
	}
	return map[string]any{
		"formalized_content": renderPubMedResults(results),
		"json":               results,
	}, nil
}

func (c *pubMedComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func renderPubMedResults(results []any) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for i, item := range results {
		result, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := strings.TrimSpace(stringParam(result["content"]))
		if content == "" {
			continue
		}
		lines := []string{fmt.Sprintf("ID: %d", i)}
		if title := strings.TrimSpace(stringParam(result["title"])); title != "" {
			lines = append(lines, "Title: "+title)
		}
		if link := strings.TrimSpace(stringParam(result["url"])); link != "" {
			lines = append(lines, "URL: "+link)
		}
		lines = append(lines, "Content:", content)
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

// Compile-time interface checks.
var (
	_ Component = (*retrievalComponent)(nil)
	_ Component = (*tavilySearchComponent)(nil)
	_ Component = (*exesqlComponent)(nil)
	_ Component = (*codeExecComponentDelegate)(nil)
	_ Component = (*arxivComponent)(nil)
	_ Component = (*googleScholarComponent)(nil)
	_ Component = (*pubMedComponent)(nil)
	_ Component = (*yahooFinanceComponent)(nil)
)
