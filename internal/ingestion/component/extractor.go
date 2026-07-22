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

// Package component — Extractor component (Phase 2.5 of
// port-rag-flow-pipeline-to-go.md §4 row 2.5).
//
// SCOPE (honest):
//
//   - PROVIDER-AGNOSTIC (§8 Q1): the Extractor does NOT depend on any
//     specific LLM provider. It dispatches every chat call through
//     internal/entity/models — the same factory routes 48 of the 56
//     Python ChatModel providers registered there (factory.go switch,
//     lines 36-156). The 8 providers NOT yet in the Go switch (LeptonAI,
//     Gemini LiteLLM path, PerfXCloud, 01.AI / Lingyi, DeerAPI,
//     Astraflow-CN, RAGcon, New API) ARE unreachable from this
//     component — an llm_id resolving to one of those falls through to
//     NewDummyModel and the chat call returns a deterministic "dummy"
//     response. We DO NOT panic: errors are surfaced as a clean
//     "no driver for %q" wrap that callers can log and route.
//
//   - LLM CALL SHAPE: one chat call per chunk (no batching). LLM
//     calls are inherently serial; sequential per-chunk processing
//     keeps test ordering deterministic under -race.
//
//   - TIMEOUT / ELAPSED: the call is wrapped in
//     runtime.WithTimeout(60s) and runtime.TrackElapsed so the
//     upstream pipeline gets _created_time / _elapsed_time stamps
//     matching the python ProcessBase contract (base.py:42, 58).
//
//   - JSON PARSING: the prompt asks the LLM to return a JSON object;
//     we best-effort parse the response into map[string]any. A
//     non-JSON response is NOT a hard error — it's surfaced as the
//     raw string under the same field name so downstream callers
//     can decide what to do.
//
//   - WHAT IS NOT YET PORTED: the python _build_TOC branch
//     (rag/flow/extractor/extractor.py:40-72) requires the TOC
//     generator (rag.prompts.generator.run_toc_from_text). That
//     service has no Go counterpart yet; the current Extractor
//     short-circuits with a clear error when field_name == "toc"
//     so a future Phase 2.5+ task can fill the gap without a
//     silent regression.
//
//   - SINGLE-CHUNK FAST PATH: when no chunk list is wired in,
//     the LLM is called once with the resolved args directly (no
//     chunk substitution). Matches python _invoke path
//     (line 108: msg, sys_prompt = self._sys_prompt_and_msg([], args)).
package component

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	eschema "github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

const componentNameExtractor = "Extractor"

// extractorTimeout bounds one LLM chat call. Matches the python
// `@timeout(60)` default at rag/flow/base.py:60. The pipeline
// orchestrator (Phase 3) overrides this if a stage-level ceiling
// is configured.
const extractorTimeout = 600 * time.Second

const (
	autoKeywordPrompt = `## Role
You are a text analyzer.

## Task
Extract the most important keywords/phrases of a given piece of text content.

## Requirements
- Summarize the text content, and give the top %d important keywords/phrases.
- The keywords MUST be in the same language as the given piece of text content.
- The keywords are delimited by ENGLISH COMMA.
- Output keywords ONLY.

---

## Text Content
%s`

	autoQuestionPrompt = `## Role
You are a text analyzer.

## Task
Propose questions about a given piece of text content.

## Requirements
- Understand and summarize the text content, and propose the top %d important questions.
- The questions SHOULD NOT have overlapping meanings.
- The questions SHOULD cover the main content of the text as much as possible.
- The questions MUST be in the same language as the given piece of text content.
- One question per line.
- Output questions ONLY.

---

## Text Content
%s`
)

// ExtractorComponent performs LLM-based extraction over a chunk
// list (or a single empty call when no chunks are wired in).
//
// The instance is safe for concurrent invocation: each Invoke
// reads Param read-only (Param is set at construction; per-call
// overrides flow through the inputs map). The single mutable
// package-level seam (extractorChatInvoker) is guarded by a
// RWMutex; tests swap it via SetExtractorChatInvoker.
type ExtractorComponent struct {
	Param schema.ExtractorParam
}

// NewExtractorComponent constructs an Extractor from a DSL param
// map. Missing keys fall back to schema.ExtractorParam.Defaults().
//
// Param map shape (all keys optional; missing → Defaults()):
//
//	{
//	  "field_name":     string,           — optional; key the extraction lands under
//	  "llm_id":         string,           — optional; resolves via models.NewModelFactory
//	  "system_prompt":  string,           — optional override
//	  "prompt":         string,           — optional user prompt
//	}
//
// errors here surface as canvas compile failures so a malformed
// param is caught at build time rather than mid-run.
func NewExtractorComponent(params map[string]any) (runtime.Component, error) {
	p := schema.ExtractorParam{}.Defaults()
	if params != nil {
		if v, ok := params["field_name"].(string); ok {
			p.FieldName = v
		}
		if v, ok := params["llm_id"].(string); ok {
			p.LLMID = v
		}
		if v, ok := params["system_prompt"].(string); ok {
			p.SystemPrompt = v
		}
		if v, ok := params["prompt"].(string); ok {
			p.Prompt = v
		}
		if v, ok := params["auto_keywords"]; ok {
			p.AutoKeywords = mapInt(v)
		}
		if v, ok := params["auto_questions"]; ok {
			p.AutoQuestions = mapInt(v)
		}
		if v, ok := params["auto_tags"]; ok {
			p.AutoTags = mapInt(v)
		}
		if v, ok := params["tag_file_id"].(string); ok {
			p.TagFileID = v
		}
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("extractor: param check: %w", err)
	}
	return &ExtractorComponent{Param: p}, nil
}

// Inputs returns the parameter metadata. Matches the python
// Extractor._invoke kwargs plus the optional per-call llm_id
// override (python: args["llm_id"] path is implicit via
// self.chat_mdl; the Go port exposes it explicitly).
func (c *ExtractorComponent) Inputs() map[string]string {
	return map[string]string{
		"chunks":        "List of map[string]any from upstream Tokenizer. Each entry must carry a string 'text' (or 'content_with_weight') field. Optional — when absent the LLM is called once with the resolved args.",
		"prompt":        "Optional user prompt template. Falls back to Param.Prompt when absent.",
		"llm_id":        "Optional per-call LLM id override. Falls back to Param.LLMID when absent.",
		"system_prompt": "Optional per-call system prompt override. Falls back to Param.SystemPrompt.",
	}
}

// Outputs returns the public surface downstream ingestion
// consumers can wire into. Mirrors schema.ExtractorOutputs.
//
//	chunks         []map[string]any — input chunks, each augmented
//	                                 with field_name=<LLM result>.
//	                                 When the input chunks list is
//	                                 absent, the slice contains a
//	                                 single map with the same shape.
//	output_format  string          — always "chunks". Parity with
//	                                 python set_output contract.
//	_ERROR         string          — populated on a short-circuit
//	                                 error (matches python
//	                                 set_output("_ERROR", ...)).
func (c *ExtractorComponent) Outputs() map[string]string {
	return map[string]string{
		"chunks":        "Extraction results — input chunks (or a single-element slice when no chunks were supplied), each enriched with field_name=<LLM response>.",
		"output_format": "Always \"chunks\". Parity marker for downstream consumers.",
		"_ERROR":        "Optional short-circuit error message (reserved for the future TOC branch and other error paths).",
	}
}

// extractorChatInvoker is the seam the Extractor uses to dispatch
// its chat call. The production implementation
// (einoExtractorChatInvoker below) mirrors
// internal/agent/component/llm.go:einoChatInvoker — same factory,
// same driver resolution, but kept self-contained so the
// ingestion package does NOT pull in agent/component for a
// one-method interface.
//
// Tests swap the package-level defaultExtractorChatInvoker to inject a
// canned-response stub (see SetExtractorChatInvoker and the test
// helpers in extractor_test.go). This is the testability seam the
// Phase 2.5 spec calls out as a hard rule.
type extractorChatInvoker interface {
	Chat(ctx context.Context, req extractorChatRequest) (*extractorChatResponse, error)
}

// extractorChatRequest is the minimal surface the Extractor needs
// to dispatch a chat call. Driver is the provider key
// (e.g. "openai"); ModelName is the model id alone or composite
// "model@provider". APIKey / BaseURL are passed through so the
// driver can authenticate without re-reading the tenant config.
type extractorChatRequest struct {
	Driver      string
	ModelName   string
	APIKey      string
	BaseURL     string
	Messages    []eschema.Message
	Temperature *float64
}

// extractorChatResponse holds the LLM's text answer. Token /
// stopped flags are not consumed by the Extractor yet, so they
// remain optional / 0-valued.
type extractorChatResponse struct {
	Content string
}

// extractorChatInvokerMu guards defaultExtractorChatInvoker swaps.
var extractorChatInvokerMu sync.RWMutex

// defaultExtractorChatInvoker is the package-level seam. Production
// uses einoExtractorChatInvoker; tests inject a stub.
var defaultExtractorChatInvoker extractorChatInvoker = &einoExtractorChatInvoker{}

var extractorChatTargetResolverMu sync.RWMutex

// extractorChatTargetResolverOverride is a narrow test seam for
// integration tests that need to supply real credentials without
// teaching the production Extractor a tenant-credential lookup path.
// When set, resolveExtractorChatTarget consults it first.
var extractorChatTargetResolverOverride func(llmID string) (driver, modelName, apiKey, baseURL string, ok bool)

// SetExtractorChatInvoker swaps the package-level chat invoker
// for tests. Pass nil to restore the default. Concurrent-safe.
func SetExtractorChatInvoker(inv extractorChatInvoker) {
	extractorChatInvokerMu.Lock()
	defer extractorChatInvokerMu.Unlock()
	defaultExtractorChatInvoker = inv
}

// SetExtractorChatTargetResolverOverride swaps the package-level
// llm_id target resolver override for tests. Pass nil to restore
// the default split-only resolver. Concurrent-safe.
func SetExtractorChatTargetResolverOverride(fn func(llmID string) (driver, modelName, apiKey, baseURL string, ok bool)) {
	extractorChatTargetResolverMu.Lock()
	defer extractorChatTargetResolverMu.Unlock()
	extractorChatTargetResolverOverride = fn
}

func getExtractorChatTargetResolverOverride() func(llmID string) (driver, modelName, apiKey, baseURL string, ok bool) {
	extractorChatTargetResolverMu.RLock()
	defer extractorChatTargetResolverMu.RUnlock()
	return extractorChatTargetResolverOverride
}

// getExtractorChatInvoker returns the current default invoker.
func getExtractorChatInvoker() extractorChatInvoker {
	extractorChatInvokerMu.RLock()
	defer extractorChatInvokerMu.RUnlock()
	if defaultExtractorChatInvoker == nil {
		return &einoExtractorChatInvoker{}
	}
	return defaultExtractorChatInvoker
}

// einoExtractorChatInvoker is the production seam. It dispatches
// through the entity/models factory (which knows 48 of 56
// providers) and returns the assistant text via
// models.EinoChatModel.Generate. An unknown provider falls
// through to NewDummyModel in the factory's default branch — we
// surface that as a typed "no driver for %q" wrap so callers can
// decide whether to retry, route around, or log.
type einoExtractorChatInvoker struct{}

// Chat implements extractorChatInvoker for the production path.
func (e *einoExtractorChatInvoker) Chat(ctx context.Context, req extractorChatRequest) (*extractorChatResponse, error) {
	if req.ModelName == "" {
		return nil, fmt.Errorf("extractor: chat: model_name is required")
	}
	driver := strings.ToLower(strings.TrimSpace(req.Driver))
	modelName := req.ModelName
	if driver == "" && modelName != "" {
		if bare, provider, ok := splitExtractorLLIDPair(modelName); ok {
			driver = provider
			modelName = bare
		}
	}
	if driver == "" {
		return nil, fmt.Errorf("extractor: chat: no driver resolved for model %q", modelName)
	}
	common.Info(fmt.Sprintf("extractor: chat: driver=%s modelName=%s baseUrl=%s", driver, modelName, req.BaseURL))
	d, err := models.GetPreconfiguredDriver(driver, req.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("extractor: resolve driver %q: %w", driver, err)
	}
	apiKey := req.APIKey
	cfg := &models.APIConfig{ApiKey: &apiKey}
	cm := models.NewChatModel(d, &modelName, cfg)
	var chatCfg *models.ChatConfig
	if req.Temperature != nil {
		temp := *req.Temperature
		chatCfg = &models.ChatConfig{Temperature: &temp}
	}
	wrapper := models.NewEinoChatModel(cm, chatCfg)
	// Honour ctx cancel up front so the caller's WithTimeout(...)
	// is observed even when the driver layer doesn't take a ctx.
	common.Info(fmt.Sprintf("try to chat with message: %v", req.Messages))
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, err := wrapper.Generate(ctx, toExtractorEinoMessages(req.Messages))
	if err != nil {
		common.Error(fmt.Sprintf("error when chat with message: %v", req.Messages), err)
		return nil, err
	}
	return &extractorChatResponse{Content: out.Content}, nil
}

// splitExtractorLLIDPair parses a composite llm_id "model@provider"
// mirroring agent/component/llm_credentials.go:parseLLMIDParts
// (the canonical composite form throughout the codebase). Returns
// ok=false when no "@" is present or the id is malformed.
//
//	"gpt-4o-mini@openai"           -> ("gpt-4o-mini", "openai", true)
//	"gpt-4o-mini"                  -> ("gpt-4o-mini", "", false)
//
// Kept local so the ingestion package doesn't import
// agent/component.
func splitExtractorLLIDPair(s string) (modelName, provider string, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), "@")
	switch len(parts) {
	case 2:
		return parts[0], parts[1], true
	default:
		return s, "", false
	}
}

// toExtractorEinoMessages converts eschema.Message → *eschema.Message
// for the eino bridge. The user / system / assistant roles pass
// through; multi-modal content is intentionally not propagated —
// extraction prompts are text-only today.
func toExtractorEinoMessages(msgs []eschema.Message) []*eschema.Message {
	out := make([]*eschema.Message, 0, len(msgs))
	for i := range msgs {
		m := msgs[i]
		role := m.Role
		if role == "" {
			role = eschema.User
		}
		out = append(out, &eschema.Message{
			Role:    role,
			Content: m.Content,
		})
	}
	return out
}

// extractorInputs is the post-Validation view of the upstream
// input map. Computed once at the top of Invoke so the rest of
// the function reads as straight-line code.
type extractorInputs struct {
	fieldName    string
	llmID        string
	systemPrompt string
	prompt       string
	lang         string
	chunks       []map[string]any
}

// resolveInputs overlays per-call inputs on top of the
// component's static Param. Missing keys fall back to the
// Param-level values; per-call values win on conflict (so a
// canvas can override LLM_ID at runtime). The python
// Extractor reads inputs directly from get_input_elements(); the
// Go port normalizes to extractorInputs once at the top so the
// rest of Invoke reads straight-line.
func (c *ExtractorComponent) resolveInputs(inputs map[string]any) extractorInputs {
	out := extractorInputs{
		fieldName:    c.Param.FieldName,
		llmID:        c.Param.LLMID,
		systemPrompt: c.Param.SystemPrompt,
		prompt:       c.Param.Prompt,
	}
	if inputs == nil {
		return out
	}
	if v, ok := inputs["llm_id"].(string); ok && v != "" {
		out.llmID = v
	}
	if v, ok := inputs["prompt"].(string); ok && v != "" {
		out.prompt = v
	}
	if v, ok := inputs["system_prompt"].(string); ok && v != "" {
		out.systemPrompt = v
	}
	if v, ok := inputs["lang"].(string); ok && v != "" {
		out.lang = v
	}
	for _, key := range extractorChunkInputOrder(inputs) {
		if chunks, ok := extractorChunkList(inputs[key]); ok {
			out.chunks = chunks
			break
		}
	}
	return out
}

func extractorChunkInputOrder(inputs map[string]any) []string {
	order := make([]string, 0, len(inputs))
	for _, preferred := range []string{"chunks", "json"} {
		if _, ok := inputs[preferred]; ok {
			order = append(order, preferred)
		}
	}
	var extra []string
	for key := range inputs {
		if key == "chunks" || key == "json" {
			continue
		}
		extra = append(extra, key)
	}
	sort.Strings(extra)
	order = append(order, extra...)
	return order
}

func extractorChunkList(v any) ([]map[string]any, bool) {
	switch list := v.(type) {
	case []map[string]any:
		return list, true
	case []any:
		out := make([]map[string]any, 0, len(list))
		for _, item := range list {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, m)
		}
		return out, true
	default:
		return nil, false
	}
}

// Invoke performs LLM-based extraction. Inputs:
//
//	chunks         (optional, []map[string]any) — upstream chunks; each must
//	                                            carry a string "text".
//	prompt         (optional, string)            — overrides Param.Prompt.
//	system_prompt  (optional, string)            — overrides Param.SystemPrompt.
//	llm_id         (optional, string)            — overrides Param.LLMID.
//
// Outputs:
//
//	chunks        ([]map[string]any) — input chunks augmented with
//	                                  field_name=<LLM result>. When
//	                                  the input list is empty, the
//	                                  slice contains a single map.
//	output_format (string)          — always "chunks".
//	_ERROR        (string, reserved) — populated when the component
//	                                  short-circuits with an error.
//	_created_time, _elapsed_time    — stamped by the canvas framework
//	                                 (realComponentBody), not here.
func (c *ExtractorComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	if err := c.Param.Validate(); err != nil {
		return nil, fmt.Errorf("extractor: %w", err)
	}
	in := c.resolveInputs(inputs)
	common.Debug("extractor stage",
		zap.String("component", "Extractor"),
		zap.Int("input_chunks", len(in.chunks)),
	)
	if in.fieldName == "toc" {
		return nil, fmt.Errorf("extractor: field_name %q requires the TOC prompt generator which is not yet ported to Go", "toc")
	}

	if err := runtime.WithTimeout(ctx, extractorTimeout, func(timeoutCtx context.Context) error {
		// Tag phase: run when auto_tags > 0 and we have chunks.
		if c.Param.AutoTags > 0 && len(in.chunks) > 0 {
			tagged, tagErr := c.runAutoTags(timeoutCtx, in)
			if tagErr != nil {
				return tagErr
			}
			in.chunks = tagged
		}

		if len(in.chunks) == 0 {
			ans, callErr := c.call(timeoutCtx, in, "")
			if callErr != nil {
				return callErr
			}
			in.chunks = []map[string]any{{in.fieldName: ans}}
			return nil
		}
		for i, ck := range in.chunks {
			text, _ := ck["content_with_weight"].(string)
			if strings.TrimSpace(text) == "" {
				text, _ = ck["text"].(string)
			}

			if c.Param.AutoKeywords > 0 {
				if err := c.runAutoKeywords(timeoutCtx, in, ck, text); err != nil {
					return fmt.Errorf("chunk %d keywords: %w", i, err)
				}
			}
			if c.Param.AutoQuestions > 0 {
				if err := c.runAutoQuestions(timeoutCtx, in, ck, text); err != nil {
					return fmt.Errorf("chunk %d questions: %w", i, err)
				}
			}

			if in.fieldName != "" {
				ans, callErr := c.call(timeoutCtx, in, text)
				if callErr != nil {
					return fmt.Errorf("chunk %d: %w", i, callErr)
				}
				ck[in.fieldName] = ans
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("extractor: %w", err)
	}
	common.Debug("extractor stage",
		zap.String("component", "Extractor"),
		zap.Int("output_chunks", len(in.chunks)),
	)
	return map[string]any{
		"chunks":        in.chunks,
		"output_format": "chunks",
	}, nil
}

func (c *ExtractorComponent) runAutoKeywords(ctx context.Context, in extractorInputs, ck map[string]any, chunkText string) error {
	if _, exists := ck["important_kwd"]; exists {
		return nil
	}
	kwIn := in
	kwIn.prompt = "Output: "
	kwIn.systemPrompt = fmt.Sprintf(autoKeywordPrompt, c.Param.AutoKeywords, chunkText)
	kwIn.fieldName = ""
	result, err := c.call(ctx, kwIn, "")
	if err != nil {
		return err
	}
	resultStr, _ := result.(string)
	resultStr = cleanExtractionResult(resultStr)
	if resultStr == "" {
		return nil
	}
	kwds := splitKeywords(resultStr)
	if len(kwds) == 0 {
		return nil
	}
	ck["important_kwd"] = kwds
	tok := tokenizer.New(in.lang)
	tks, tkErr := tok.Tokenize(strings.Join(kwds, " "))
	if tkErr == nil {
		ck["important_tks"] = tks
	}
	return nil
}

func (c *ExtractorComponent) runAutoQuestions(ctx context.Context, in extractorInputs, ck map[string]any, chunkText string) error {
	if _, exists := ck["question_kwd"]; exists {
		return nil
	}
	qIn := in
	qIn.prompt = "Output: "
	qIn.systemPrompt = fmt.Sprintf(autoQuestionPrompt, c.Param.AutoQuestions, chunkText)
	qIn.fieldName = ""
	result, err := c.call(ctx, qIn, "")
	if err != nil {
		return err
	}
	resultStr, _ := result.(string)
	resultStr = cleanExtractionResult(resultStr)
	if resultStr == "" {
		return nil
	}
	qs := strings.Split(resultStr, "\n")
	// Filter empty lines
	var filtered []string
	for _, q := range qs {
		q = strings.TrimSpace(q)
		if q != "" {
			filtered = append(filtered, q)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	ck["question_kwd"] = filtered
	tok := tokenizer.New(in.lang)
	tks, tkErr := tok.Tokenize(strings.Join(filtered, "\n"))
	if tkErr == nil {
		ck["question_tks"] = tks
	}
	return nil
}

// cleanExtractionResult strips `</think>` tags and rejects `**ERROR**` responses,
// matching Python's keyword_extraction and question_proposal post-processing.
func cleanExtractionResult(s string) string {
	if i := strings.Index(s, "</think>"); i >= 0 {
		s = s[i+len("</think>"):]
	}
	s = strings.TrimSpace(s)
	if strings.Contains(s, "**ERROR**") {
		return ""
	}
	return s
}

// splitKeywords splits a comma-delimited keyword string.
func splitKeywords(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '、' || r == '\r' || r == '\n'
	})
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// call dispatches one LLM chat call for the supplied chunk text
// (empty string in the no-chunk fast path). The result is the
// raw string from the model — JSON parsing happens here so
// callers can rely on a structured value downstream.
func (c *ExtractorComponent) call(ctx context.Context, in extractorInputs, chunkText string) (any, error) {
	driver, modelName, apiKey, baseURL, err := resolveExtractorChatTarget(ctx, in.llmID)
	if err != nil {
		return nil, err
	}
	msgs := buildExtractorMessages(in.systemPrompt, in.prompt, chunkText, in.chunks)
	inv := getExtractorChatInvoker()
	resp, err := inv.Chat(ctx, extractorChatRequest{
		Driver:    driver,
		ModelName: modelName,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		Messages:  msgs,
	})
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(resp.Content)
	if raw == "" {
		// No response — emit empty string so downstream code
		// can distinguish from "LLM errored" via the error
		// path above.
		return "", nil
	}
	// Best-effort JSON parse: a JSON object response is the
	// canonical structured-extraction shape. Other shapes are
	// returned verbatim so the caller can decide.
	if parsed, ok := tryParseJSONObject(raw); ok {
		return parsed, nil
	}
	return raw, nil
}

// resolveExtractorChatTarget resolves the llm_id into driver / model /
// api_key / base_url. The llm_id may be a bare tenant_model UUID or
// a composite "model@provider" string. Errors from DAO resolution are
// propagated so the caller sees the real failure reason.
func resolveExtractorChatTarget(ctx context.Context, llmID string) (driver, modelName, apiKey, baseURL string, err error) {
	if override := getExtractorChatTargetResolverOverride(); override != nil {
		if driver, modelName, apiKey, baseURL, ok := override(llmID); ok {
			return driver, modelName, apiKey, baseURL, nil
		}
	}

	cfg, cfgErr := resolveExtractorChatConfig(ctx, llmID)
	if cfgErr != nil {
		return "", "", "", "", cfgErr
	}
	if cfg.driver != "" {
		return cfg.driver, cfg.modelName, cfg.apiKey, cfg.baseURL, nil
	}

	// Fallback: when tenant credentials are not available
	// (no canvas state / no DB), fall back to the basic @ split
	// so callers can still use model@provider format in tests.
	if bare, provider, ok := splitExtractorLLIDPair(llmID); ok {
		return strings.ToLower(provider), bare, "", "", nil
	}
	// Nothing left to try — let Chat() surface a clear error when
	// the driver ends up empty.
	return "", llmID, "", "", nil
}

// extractorChatConfig holds the resolved chat model configuration.
type extractorChatConfig struct {
	driver    string // llm_factory
	modelName string // llm_name
	apiKey    string
	baseURL   string // api_base
}

// resolveExtractorChatConfig resolves tenant-scoped credentials for
// the given llm_id via the shared resolveModelConfig helper.
//
//   - Bare UUID → DAO lookup via resolveModelConfigByID.
//   - "model@provider" → parsed via resolveModelConfigFromProviderInstance.
//
// Returns nil error when there is no canvas state (unit tests) —
// the caller's @ split fallback handles that case.
func resolveExtractorChatConfig(ctx context.Context, compositeLLMID string) (extractorChatConfig, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return extractorChatConfig{}, nil
	}
	tidVal, _ := state.GetGlobal("tenant_id")
	tid, _ := tidVal.(string)
	if tid == "" {
		return extractorChatConfig{}, nil
	}

	// Pre-check: if this looks like a bare UUID, confirm the record
	// exists before delegating.  resolveModelConfig falls through to
	// parseCompositeModelName when GetByID returns ErrRecordNotFound,
	// which produces a confusing "provider name missing" error.
	if isBareTenantModelID(compositeLLMID) {
		if _, err := dao.NewTenantModelDAO().GetByID(compositeLLMID); err != nil {
			return extractorChatConfig{}, fmt.Errorf("extractor: tenant model %q not found or not usable: %w", compositeLLMID, err)
		}
	}

	driver, modelName, apiConfig, _, err := resolveModelConfig(tid, entity.ModelTypeChat, compositeLLMID)
	if err != nil {
		return extractorChatConfig{}, fmt.Errorf("extractor: resolve model %q: %w", compositeLLMID, err)
	}

	apiKey := ""
	baseURL := ""
	if apiConfig != nil {
		if apiConfig.ApiKey != nil {
			apiKey = *apiConfig.ApiKey
		}
		if apiConfig.BaseURL != nil {
			baseURL = *apiConfig.BaseURL
		}
	}
	return extractorChatConfig{
		driver:    strings.ToLower(driver.Name()),
		modelName: modelName,
		apiKey:    apiKey,
		baseURL:   baseURL,
	}, nil
}

// isBareTenantModelID reports whether s is a 32-character hex string
// (a tenant_model primary key), as opposed to a composite "model@provider".
func isBareTenantModelID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// buildExtractorMessages assembles system + user messages for
// one extraction call. The user prompt is rendered as
// "<prompt>\n\n<chunkText>" so the python behavior of
// substituting the chunk text into the args dict is preserved
// without invoking a template engine.
//
// Prompt placeholders of the form `{ComponentName:ParamName@chunks}`
// are substituted with the joined text of all upstream chunks
// when chunks is non-empty. The python rag/flow/extractor/extractor.py
// build_existing_prompt path performs the same substitution at
// runtime; the Go port surfaces it as a regex on the prompt
// template so the resume template's `{TitleChunker:FlatMiceFix@chunks}`
// reference resolves without invoking a template engine.
//
// Substitution is opt-in: when chunks is nil/empty the placeholder
// is left intact so a misconfigured template surfaces as a
// clear pattern rather than silently disappearing.
func buildExtractorMessages(system, prompt, chunkText string, chunks []map[string]any) []eschema.Message {
	out := make([]eschema.Message, 0, 2)
	if system != "" {
		out = append(out, eschema.Message{Role: eschema.System, Content: system})
	}
	user := prompt
	if chunkText != "" {
		if user != "" {
			user += "\n\n"
		}
		user += chunkText
	}
	if user == "" {
		// An empty prompt + empty chunk is a degenerate call.
		// The LLM driver returns an error; we surface that
		// unchanged.
		user = " "
	}
	user = substitutePromptPlaceholders(user, chunks)
	out = append(out, eschema.Message{Role: eschema.User, Content: user})
	return out
}

// substitutePromptPlaceholders replaces `{ComponentName:ParamName@chunks}`
// patterns in the user prompt with the joined text of all upstream
// chunks. The python rag/flow/extractor/extractor.py:build_existing_prompt
// path performs the same substitution at runtime using a Jinja
// template; the Go port keeps the regex form because the LLM
// driver does not require Jinja and the surface is small enough to
// avoid pulling in a template engine.
//
// Pattern grammar:
//
//	{CmpName:ParamName@chunks}
//
// The CmpName and ParamName are both matched but ignored — the
// substitute is always "the joined chunk text" today, because the
// only @chunks reference in production templates is the resume
// template's `{TitleChunker:FlatMiceFix@chunks}` pattern. The
// CmpName/ParamName parsing exists so a future per-component
// substitution can extend the function without breaking the
// existing call sites.
func substitutePromptPlaceholders(prompt string, chunks []map[string]any) string {
	if prompt == "" || len(chunks) == 0 {
		return prompt
	}
	// Build the substitution payload once. Each chunk's text is
	// joined with a blank line so a downstream LLM sees clear
	// chunk boundaries.
	var b strings.Builder
	for i, ck := range chunks {
		t, _ := ck["text"].(string)
		if t == "" {
			t, _ = ck["content_with_weight"].(string)
		}
		if t == "" {
			continue
		}
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(t)
	}
	repl := b.String()
	if repl == "" {
		return prompt
	}
	return placeholderRE.ReplaceAllString(prompt, repl)
}

// placeholderRE matches `{CmpName:ParamName@chunks}` patterns in
// Extractor user prompts. The CMP / Param groups are ignored for
// the @chunks variant but kept so the regex rejects arbitrary
// placeholders (a future per-component substitution extends here).
var placeholderRE = regexp.MustCompile(`\{[A-Za-z0-9_]+:[A-Za-z0-9_]+@chunks\}`)

// tryParseJSONObject tries to parse s as a JSON object. Returns
// (parsed, true) on success; (nil, false) on parse error or when
// s is not a JSON object. Trims common markdown code fences
// (```json ... ```) before parsing.
func tryParseJSONObject(s string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(s)
	// Strip a single ``` fence pair if present.
	if strings.HasPrefix(trimmed, "```") {
		if idx := strings.Index(trimmed, "\n"); idx >= 0 {
			trimmed = trimmed[idx+1:]
		}
		if strings.HasSuffix(trimmed, "```") {
			trimmed = trimmed[:len(trimmed)-3]
		}
		trimmed = strings.TrimSpace(trimmed)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, false
	}
	if out == nil {
		return nil, false
	}
	// An empty object carries no information the caller can act on;
	// surface as "could not extract" so downstream code can route
	// it to the same fallback it would use for malformed text.
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// init registers Extractor under CategoryIngestion (per plan §4
// Phase 2.5). Metadata is derived from the Inputs()/Outputs()
// methods on ExtractorComponent so the API layer (Phase 4) can
// enumerate the catalog without instantiating the component.
// mapInt converts a JSON-compatible value to int.
func mapInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// init registers Extractor under CategoryIngestion (per plan §4
// Phase 2.5). Metadata is derived from the Inputs()/Outputs()
// methods on ExtractorComponent so the API layer (Phase 4) can
// enumerate the catalog without instantiating the component.
func init() {
	c := &ExtractorComponent{}
	runtime.MustRegister(componentNameExtractor, runtime.CategoryIngestion,
		func(_ string, params map[string]any) (runtime.Component, error) {
			return NewExtractorComponent(params)
		},
		runtime.Metadata{
			Version: "1.0.0",
			Inputs:  c.Inputs(),
			Outputs: c.Outputs(),
		})
}
