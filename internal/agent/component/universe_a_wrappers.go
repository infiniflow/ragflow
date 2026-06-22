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
	"fmt"
	"strconv"
	"strings"

	agenttool "ragflow/internal/agent/tool"
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
	argsJSON, _ := json.Marshal(merged)
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	if err != nil {
		return nil, fmt.Errorf("canvas: Retrieval: %w", err)
	}
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
		if k == "arguments" {
			continue // replaced with resolved args
		}
		merged[k] = v
	}
	merged["arguments"] = args
	// Override with runtime inputs (except state).
	for k, v := range inputs {
		if k == "state" || k == "arguments" {
			continue
		}
		merged[k] = v
	}

	// Marshal merged params and delegate to the tool's InvokableRun.
	tool := agenttool.NewCodeExecTool()
	argsJSON, mErr := json.Marshal(merged)
	if mErr != nil {
		return nil, fmt.Errorf("CodeExec: marshal: %w", mErr)
	}
	raw, err := tool.InvokableRun(ctx, string(argsJSON))
	parsed := parseToolEnvelope(raw)
	// The tool outputs raw_result but the legacy component surface
	// expects result.  Add an alias for backward compatibility.
	if rawResult, ok := parsed["raw_result"]; ok {
		parsed["result"] = rawResult
	}
	// Ensure _ERROR is always present (empty string = no error).
	if _, has := parsed["_ERROR"]; !has {
		parsed["_ERROR"] = ""
	}
	// Contract validation: check declared output types against the
	// actual result.  The test declares "result": {"type": "Number"}
	// and expects a mismatch when the sandbox returns a string.
	if outputs, _ := c.params["outputs"].(map[string]any); outputs != nil {
		if rschema, ok := outputs["result"].(map[string]any); ok {
			if wantType, _ := rschema["type"].(string); wantType != "" {
				gotType, _ := parsed["actual_type"].(string)
				if gotType != "" && !strings.EqualFold(gotType, wantType) {
					parsed["_ERROR"] = fmt.Sprintf("contract mismatch: expected type %s got %s", wantType, gotType)
					delete(parsed, "result")
					delete(parsed, "raw_result")
				}
			}
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
	_ Component = (*tavilySearchComponent)(nil)
	_ Component = (*exesqlComponent)(nil)
)

// Compile-time check that the InvokableTool methods we call
// are reachable (catches a future refactor that renames them).
