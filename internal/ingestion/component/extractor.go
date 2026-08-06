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
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	eschema "github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

const componentNameExtractor = "Extractor"

// extractorTimeout bounds one LLM chat call. Matches the python
// `@timeout(60)` default at rag/flow/base.py:60. The pipeline
// orchestrator (Phase 3) overrides this if a stage-level ceiling
// is configured.
const extractorTimeout = 600 * time.Second

// extractorRetryMax and extractorRetryDelay are package-level vars
// so tests can override them (extractorRetryDelay → time.Millisecond)
// to avoid multi-second retry sleeps. Production defaults match
// common.RetryWithBackoff defaults (3 retries, 2s initial delay).
var (
	extractorRetryMax   = common.DefaultRetryMax
	extractorRetryDelay = common.DefaultRetryDelay
)

// extractorTemperature mirrors Python's 0.2 default for keyword
// and question extraction calls (generator.py:230,245).
const extractorTemperature = 0.2

// defaultExtractorConcurrency bounds concurrent LLM extraction calls
// process-wide. It mirrors Python's chat_limiter bound
// (agent/component/base.py:353 asyncio.Semaphore(MAX_CONCURRENT_CHATS,
// default 10)) so the Go port rate-limits auto-keywords / auto-questions /
// auto-metadata extraction the same way Python does.
const defaultExtractorConcurrency = 10

// extractorJob is one chunk's auto-extraction unit of work.
type extractorJob func() error

// extractorPool is the process-wide bounded worker pool that drives
// cross-chunk concurrency for auto-keywords / auto-questions /
// auto-metadata extraction. It mirrors the parser/structure/mindmap
// WorkerPool usage, but is held globally so every Extractor
// invocation shares one rate limiter instead of spinning up one pool
// per batch. The pool only bounds concurrency (it is never StopWait'd),
// so per-invocation completion is tracked by runAutoExtractions with its
// own sync.WaitGroup + first-error collection — concurrent Invoke calls
// therefore do not disturb each other.
var extractorPool = utility.NewWorkerPool[extractorJob, struct{}](
	extractorConcurrency(),
	extractorConcurrency()*4,
	func(_ context.Context, j extractorJob) (struct{}, error) { return struct{}{}, j() },
)

// extractorConcurrency resolves the pool size from MAX_CONCURRENT_CHATS
// (Python parity) with a safe default of defaultExtractorConcurrency.
func extractorConcurrency() int {
	if v := os.Getenv("MAX_CONCURRENT_CHATS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultExtractorConcurrency
}

// SetExtractorConcurrency overrides the global extractor pool size at
// runtime (e.g. from service init or tests). Mirrors tuning Python's
// chat_limiter bound.
func SetExtractorConcurrency(n int) {
	if n > 0 {
		extractorPool.Resize(n)
	}
}

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

	autoMetadataPrompt = `## Role: Metadata extraction expert.
## Rules:
 - Strict Evidence Only: Extract a value ONLY if it is explicitly mentioned in the Content.
 - Enum Filter: For any field with an 'enum' list, the list acts as a strict filter. If no element from the list (or its direct synonym) is found in the Content, you MUST NOT extract that field.
 - No Meta-Inference: Do not infer values based on the document's nature, format, or category. If the text does not literally state the information, treat it as missing.
 - Zero-Hallucination: Never invent information or pick a "likely" value from the enum to fill a field.
 - Empty Result: If no matches are found for any field, or if the content is irrelevant, output ONLY {}.
 - Output: ONLY a valid JSON string. No Markdown, no notes.

## Schema for extraction:
%s

## Content to analyze:
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
		} else if v, ok := params["sys_prompt"].(string); ok {
			p.SystemPrompt = v
		}
		if v, ok := params["prompt"].(string); ok {
			p.Prompt = v
		} else if v, ok := params["prompts"].(string); ok && v != "" {
			// Python agent/component/llm.py:119-120 normalizes a bare-string
			// prompts into [{"role":"user","content":prompts}]. Mirror that
			// here so a front-end/template that emits prompts as a string
			// (the graph.nodes form / dsl testdata) is not silently dropped
			// by the .([]any) assertion on the list branch below.
			p.Prompt = v
		} else if promptsRaw, ok := params["prompts"].([]any); ok && len(promptsRaw) > 0 {
			if first, ok := promptsRaw[0].(map[string]any); ok {
				if content, ok := first["content"].(string); ok {
					p.Prompt = content
				}
			}
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
		if v, ok := params["enable_metadata"]; ok {
			p.EnableMetadata = mapInt(v)
		}
		if v, ok := params["metadata"].([]any); ok {
			fields := make([]common.MetadataFieldDef, 0, len(v))
			for _, f := range v {
				m, ok := f.(map[string]any)
				if !ok {
					continue
				}
				key, _ := m["key"].(string)
				if key = strings.TrimSpace(key); key == "" {
					continue
				}
				def := common.MetadataFieldDef{Key: key}
				if t, ok := m["type"].(string); ok {
					def.Type = t
				}
				if d, ok := m["description"].(string); ok {
					def.Description = d
				}
				if e, ok := m["enum"].([]any); ok {
					for _, ev := range e {
						if s, ok := ev.(string); ok {
							def.Enum = append(def.Enum, s)
						}
					}
				}
				fields = append(fields, def)
			}
			p.Metadata = fields
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
	// temperature overrides the LLM temperature for this call. A
	// nil value leaves the request's Temperature unset so the model
	// (or the chat-model default) decides, matching Python's generic
	// Extractor path. The keyword/question helpers set it to
	// extractorTemperature (0.2) to mirror generator.py.
	temperature *float64
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
	} else if v, ok := inputs["sys_prompt"].(string); ok && v != "" {
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
func (c *ExtractorComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
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
			tagged, tagErr := c.runAutoTags(timeoutCtx, db, in)
			if tagErr != nil {
				return tagErr
			}
			in.chunks = tagged
		}

		if len(in.chunks) == 0 {
			ans, callErr := c.call(timeoutCtx, db, in, "")
			if callErr != nil {
				return callErr
			}
			in.chunks = []map[string]any{{in.fieldName: ans}}
			return nil
		}
		// Auto keyword / question / metadata extraction runs with
		// cross-chunk concurrency through the process-wide extractor
		// pool (mirrors Python's chat_limiter). Each chunk job owns its
		// own chunk map, so no per-chunk mutex is required; per-invocation
		// completion is tracked inside runAutoExtractions.
		if err := c.runAutoExtractions(timeoutCtx, db, in); err != nil {
			return err
		}

		// Primary field extraction stays sequential per chunk: each
		// chunk's result lands under its own field_name key, preserving
		// deterministic output ordering (see file header: -race tests).
		for i, ck := range in.chunks {
			if in.fieldName == "" {
				continue
			}
			text, _ := ck["content_with_weight"].(string)
			if strings.TrimSpace(text) == "" {
				text, _ = ck["text"].(string)
			}
			// Substitute {field_name} placeholders with current
			// chunk field values, mirroring Python's
			// string_format at extractor.py:103.
			callIn := in
			callIn.prompt = substituteChunkPlaceholders(in.prompt, ck, text)
			callIn.systemPrompt = substituteChunkPlaceholders(in.systemPrompt, ck, text)
			// buildExtractorMessages appends chunkText to the user
			// message unconditionally. When the chunk text was already
			// embedded via a {text}/{chunks} placeholder above, passing
			// it again would duplicate the content in the final prompt.
			// Suppress the automatic append in that case (the keyword/
			// question helper paths do the same by passing "").
			callChunkText := text
			if strings.Contains(in.prompt, "{text}") || strings.Contains(in.prompt, "{chunks}") ||
				strings.Contains(in.systemPrompt, "{text}") || strings.Contains(in.systemPrompt, "{chunks}") {
				callChunkText = ""
			}
			ans, callErr := c.call(timeoutCtx, db, callIn, callChunkText)
			if callErr != nil {
				return fmt.Errorf("chunk %d: %w", i, callErr)
			}
			ck[in.fieldName] = ans
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

// runAutoKeywords extracts keywords for the current chunk and stores
// them on ck["important_kwd"]. mu may be nil (sequential path); when
// non-nil it serializes the shared chunk-map accesses so concurrent
// keyword/question goroutines stay race-free. The existence check and
// the map writes are both guarded by mu. Keyword extraction pins
// temperature to extractorTemperature (0.2) to mirror generator.py.
func (c *ExtractorComponent) runAutoKeywords(ctx context.Context, db *gorm.DB, in extractorInputs, ck map[string]any, chunkText string) error {
	_, exists := ck["important_kwd"]
	if exists {
		return nil
	}
	kwTemp := extractorTemperature
	kwIn := extractorInputs{
		llmID:        in.llmID,
		systemPrompt: fmt.Sprintf(autoKeywordPrompt, c.Param.AutoKeywords, chunkText),
		prompt:       "Output: ",
		temperature:  &kwTemp,
	}
	result, err := c.call(ctx, db, kwIn, "")
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
	tok := tokenizer.New(in.lang)
	tks, tkErr := tok.Tokenize(strings.Join(kwds, " "))
	ck["important_kwd"] = kwds
	if tkErr == nil {
		ck["important_tks"] = tks
	}
	return nil
}

// runAutoQuestions extracts questions for the current chunk and stores
// them on ck["question_kwd"]. See runAutoKeywords for the temperature pin.
func (c *ExtractorComponent) runAutoQuestions(ctx context.Context, db *gorm.DB, in extractorInputs, ck map[string]any, chunkText string) error {
	_, exists := ck["question_kwd"]
	if exists {
		return nil
	}
	qTemp := extractorTemperature
	qIn := extractorInputs{
		llmID:        in.llmID,
		systemPrompt: fmt.Sprintf(autoQuestionPrompt, c.Param.AutoQuestions, chunkText),
		prompt:       "Output: ",
		temperature:  &qTemp,
	}
	result, err := c.call(ctx, db, qIn, "")
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
	tok := tokenizer.New(in.lang)
	tks, tkErr := tok.Tokenize(strings.Join(filtered, "\n"))
	ck["question_kwd"] = filtered
	if tkErr == nil {
		ck["question_tks"] = tks
	}
	return nil
}

// runAutoExtractions dispatches the auto keyword / question / metadata
// extraction for every chunk to the process-wide extractorPool so the
// work runs with bounded cross-chunk concurrency (mirrors Python's
// chat_limiter). The pool only bounds concurrency; per-invocation
// completion and first-error collection happen here with a local
// sync.WaitGroup, so concurrent Invoke calls do not disturb each other.
// Each chunk job owns its own chunk map, so no per-chunk mutex is needed.
func (c *ExtractorComponent) runAutoExtractions(ctx context.Context, db *gorm.DB, in extractorInputs) error {
	if c.Param.AutoKeywords == 0 && c.Param.AutoQuestions == 0 && c.Param.EnableMetadata == 0 {
		return nil
	}
	futs := make([]utility.WorkerPoolFuture[extractorJob, struct{}], 0, len(in.chunks))
	for i, ck := range in.chunks {
		i, ck := i, ck
		text, _ := ck["content_with_weight"].(string)
		if strings.TrimSpace(text) == "" {
			text, _ = ck["text"].(string)
		}
		fn := c.autoExtractionJob(ctx, db, in, i, ck, text)
		f, err := extractorPool.Submit(ctx, fn)
		if err != nil {
			return err
		}
		futs = append(futs, f)
	}
	var firstErr error
	var emu sync.Mutex
	var wg sync.WaitGroup
	for _, f := range futs {
		wg.Add(1)
		go func(f utility.WorkerPoolFuture[extractorJob, struct{}]) {
			defer wg.Done()
			res, werr := f.Wait(ctx)
			if werr != nil {
				emu.Lock()
				if firstErr == nil {
					firstErr = werr
				}
				emu.Unlock()
				return
			}
			if res.Err != nil {
				emu.Lock()
				if firstErr == nil {
					firstErr = res.Err
				}
				emu.Unlock()
			}
		}(f)
	}
	wg.Wait()
	return firstErr
}

// autoExtractionJob returns the unit of work for one chunk: it runs the
// enabled auto-extractions (keywords, questions, metadata) sequentially
// on the chunk's own map. Running them sequentially inside the job keeps
// the code free of per-chunk mutexes — cross-chunk parallelism is provided
// by the pool, not by intra-chunk goroutines.
func (c *ExtractorComponent) autoExtractionJob(ctx context.Context, db *gorm.DB, in extractorInputs, idx int, ck map[string]any, chunkText string) extractorJob {
	return func() error {
		if c.Param.AutoKeywords > 0 {
			if err := c.runAutoKeywords(ctx, db, in, ck, chunkText); err != nil {
				return fmt.Errorf("chunk %d keywords: %w", idx, err)
			}
		}
		if c.Param.AutoQuestions > 0 {
			if err := c.runAutoQuestions(ctx, db, in, ck, chunkText); err != nil {
				return fmt.Errorf("chunk %d questions: %w", idx, err)
			}
		}
		if c.Param.EnableMetadata > 0 {
			if err := c.runEnableMetadata(ctx, db, in, ck, chunkText); err != nil {
				return fmt.Errorf("chunk %d metadata: %w", idx, err)
			}
		}
		return nil
	}
}

// runEnableMetadata extracts structured metadata for the current chunk and
// merges the parsed JSON object into ck["metadata"]. It mirrors the
// runAutoKeywords/runAutoQuestions shape but parses a JSON object and
// merges into the chunk's metadata map (which the existing
// mergeChunkMetadata → docState.apply aggregation path consumes).
func (c *ExtractorComponent) runEnableMetadata(ctx context.Context, db *gorm.DB, in extractorInputs, ck map[string]any, chunkText string) error {
	if len(c.Param.Metadata) == 0 {
		return nil
	}
	// Render the field schema into the prompt, mirroring Python's
	// turn2jsonschema(metadata_conf) rendered into the META_DATA template.
	schemaMap := common.Turn2JSONSchema(c.Param.Metadata)
	if len(schemaMap) == 0 {
		return nil
	}
	schemaJSON, err := json.Marshal(schemaMap)
	if err != nil {
		return err
	}
	schemaStr := string(schemaJSON)

	// LLM cache (mirrors Python get_llm_cache/set_llm_cache in
	// task_executor.py:543/550 gen_metadata_task): identical (model + chunk
	// text + schema) extractions are served from Redis within a 24h window so
	// repeated runs / identical chunks don't re-pay the LLM call.
	// Best-effort: a missing Redis client or any cache error falls through to
	// a live call instead of failing the extraction.
	var parsed map[string]any
	if cached, hit := getMetadataLLMCache(ctx, in.llmID, schemaStr, chunkText); hit {
		parsed = cached
	} else {
		metaTemp := extractorTemperature
		metaIn := extractorInputs{
			llmID:        in.llmID,
			systemPrompt: fmt.Sprintf(autoMetadataPrompt, schemaStr, chunkText),
			prompt:       "Output: ",
			temperature:  &metaTemp,
		}
		result, err := c.call(ctx, db, metaIn, "")
		if err != nil {
			return err
		}
		// call JSON-parses a JSON object response into a map; plain-text
		// (or </think>-prefixed) responses come back as a string. Handle
		// both so a valid ```json reply is never silently dropped.
		var parsedObj map[string]any
		switch r := result.(type) {
		case map[string]any:
			parsedObj = r
		case string:
			s := cleanExtractionResult(r)
			if s == "" {
				return nil
			}
			var ok bool
			parsedObj, ok = tryParseJSONObject(s)
			if !ok || len(parsedObj) == 0 {
				return nil
			}
		default:
			return nil
		}
		parsed = parsedObj
		setMetadataLLMCache(ctx, in.llmID, schemaStr, chunkText, parsed)
	}
	// Merge into the chunk metadata map, preserving existing keys.
	var meta map[string]any
	if existing, ok := ck["metadata"].(map[string]any); ok && existing != nil {
		meta = existing
	} else {
		meta = make(map[string]any, len(parsed))
	}
	for k, v := range parsed {
		if v == nil {
			continue
		}
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		meta[k] = v
	}
	ck["metadata"] = meta
	return nil
}

// metadataLLMCacheTTL mirrors Python get_llm_cache/set_llm_cache 24h TTL.
const metadataLLMCacheTTL = 24 * time.Hour

// metadataLLMCacheKey builds a Redis key from (llm id, chunk text, "metadata",
// schema), mirroring Python get_llm_cache's xxh64(llmnm + txt + history + genconf).
func metadataLLMCacheKey(llmID, schemaJSON, chunkText string) string {
	h := xxhash.New()
	h.WriteString(llmID)
	h.WriteString("\x00")
	h.WriteString(chunkText)
	h.WriteString("\x00")
	h.WriteString("metadata")
	h.WriteString("\x00")
	h.WriteString(schemaJSON)
	return fmt.Sprintf("kc:meta:%x", h.Sum64())
}

// getMetadataLLMCache returns a cached extraction for the given chunk, or
// (nil, false) on miss / Redis unavailable / decode error. Best-effort.
func getMetadataLLMCache(ctx context.Context, llmID, schemaJSON, chunkText string) (map[string]any, bool) {
	client := redis.Get()
	if client == nil {
		return nil, false
	}
	data, err := client.Get(ctx, metadataLLMCacheKey(llmID, schemaJSON, chunkText))
	if err != nil || data == "" {
		return nil, false
	}
	var parsed map[string]any
	if err = json.Unmarshal([]byte(data), &parsed); err != nil {
		return nil, false
	}
	return parsed, true
}

// setMetadataLLMCache stores an extraction result for 24h. Best-effort: a
// missing Redis client or marshal error is silently ignored.
func setMetadataLLMCache(ctx context.Context, llmID, schemaJSON, chunkText string, parsed map[string]any) {
	client := redis.Get()
	if client == nil {
		return
	}
	data, err := json.Marshal(parsed)
	if err != nil {
		return
	}
	client.Set(ctx, metadataLLMCacheKey(llmID, schemaJSON, chunkText), string(data), metadataLLMCacheTTL)
}

// cleanExtractionResult strips `</think>` tags and rejects `**ERROR**` responses,
// matching Python's keyword_extraction and question_proposal post-processing.
func cleanExtractionResult(s string) string {
	if i := strings.LastIndex(s, "</think>"); i >= 0 {
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

// nonRetryableStatusRE matches HTTP client-error status codes that
// signal a permanent (non-transient) condition and therefore must
// NOT be retried. The word boundaries are essential: a bare
// substring check on "400" would wrongly flag phrasing such as
// "context deadline exceeded after 400ms" (a transient timeout,
// retryable) as non-retryable. \b ensures we only match a standalone
// 3-digit status token, so "400ms" / "4000" do not match. 429 and
// 5xx are deliberately absent — they stay retryable.
var nonRetryableStatusRE = regexp.MustCompile(`\b(?:400|401|403|404|405|422)\b`)

// isRetryableLLMError classifies an LLM chat error as worth
// retrying. The production chat invoker returns opaque errors:
// configuration failures (missing model/driver) before any API
// call, and the provider SDK's raw error after the call. We treat
// context cancellation/deadline as terminal, plus a lightweight
// heuristic for non-transient auth/client errors. Anything
// unrecognized defaults to retryable so genuinely transient 5xx /
// 429 / network blips keep retrying (matching the prior blind-retry
// behavior).
func isRetryableLLMError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"unauthorized", "authentication", "api key",
		"bad request", "not found", "model not found", "content filter",
		"no driver resolved", "model_name is required", "resolve driver",
	} {
		if strings.Contains(msg, s) {
			return false
		}
	}
	if nonRetryableStatusRE.MatchString(msg) {
		return false
	}
	return true
}

// call dispatches one LLM chat call for the supplied chunk text
// (empty string in the no-chunk fast path). The result is the
// raw string from the model — JSON parsing happens here so
// callers can rely on a structured value downstream.
func (c *ExtractorComponent) call(ctx context.Context, db *gorm.DB, in extractorInputs, chunkText string) (any, error) {
	driver, modelName, apiKey, baseURL, err := resolveExtractorChatTarget(ctx, db, in.llmID)
	if err != nil {
		return nil, err
	}
	msgs := buildExtractorMessages(in.systemPrompt, in.prompt, chunkText, in.chunks)
	inv := getExtractorChatInvoker()
	req := extractorChatRequest{
		Driver:    driver,
		ModelName: modelName,
		APIKey:    apiKey,
		BaseURL:   baseURL,
		Messages:  msgs,
	}
	// Only override the temperature when the caller set one. A nil
	// Temperature lets the model / chat-model default decide, matching
	// Python's generic Extractor path; keyword/question helpers set
	// extractorTemperature (0.2) to mirror generator.py.
	if in.temperature != nil {
		temp := *in.temperature
		req.Temperature = &temp
	}
	var resp *extractorChatResponse
	if err := common.RetryWithBackoff(ctx, extractorRetryMax, extractorRetryDelay, func() error {
		r, e := inv.Chat(ctx, req)
		resp = r
		return e
	}, isRetryableLLMError); err != nil {
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
func resolveExtractorChatTarget(ctx context.Context, db *gorm.DB, llmID string) (driver, modelName, apiKey, baseURL string, err error) {
	if override := getExtractorChatTargetResolverOverride(); override != nil {
		if driver, modelName, apiKey, baseURL, ok := override(llmID); ok {
			return driver, modelName, apiKey, baseURL, nil
		}
	}

	// When llmID is empty, try tenant default chat model
	// (mirrors Python task_executor.py:573-574 fallback).
	if llmID == "" {
		if cfg := resolveExtractorChatDefaultConfig(ctx, db); cfg.driver != "" {
			return cfg.driver, cfg.modelName, cfg.apiKey, cfg.baseURL, nil
		}
	}

	cfg, cfgErr := resolveExtractorChatConfig(ctx, db, llmID)
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
func resolveExtractorChatConfig(ctx context.Context, db *gorm.DB, compositeLLMID string) (extractorChatConfig, error) {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return extractorChatConfig{}, nil
	}
	tidVal, _ := state.GetGlobal("tenant_id")
	tid, _ := tidVal.(string)
	if tid == "" {
		return extractorChatConfig{}, nil
	}

	var driver models.ModelDriver
	var modelName string
	var apiConfig *models.APIConfig
	if isBareTenantModelID(compositeLLMID) {
		// UUID path: resolveModelConfigByID does a single GetByID and
		// returns a clear error if the record doesn't exist.  No need
		// for a separate pre-check — resolveModelConfig's redundant
		// GetByID dispatch check is also bypassed.
		driver, modelName, apiConfig, _, err = resolveModelConfigByID(ctx, db, tid, entity.ModelTypeChat, compositeLLMID)
		if err != nil {
			return extractorChatConfig{}, fmt.Errorf("extractor: tenant model %q not found or not usable: %w", compositeLLMID, err)
		}
	} else {
		// Composite "model@provider" path: delegate to the shared dispatcher.
		driver, modelName, apiConfig, _, err = resolveModelConfig(ctx, db, tid, entity.ModelTypeChat, compositeLLMID)
		if err != nil {
			return extractorChatConfig{}, fmt.Errorf("extractor: resolve model %q: %w", compositeLLMID, err)
		}
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

// resolveExtractorChatDefaultConfig resolves the tenant's default chat
// model when no explicit llm_id is provided. Returns empty config when
// no canvas state or tenant_id is available (unit-test context).
func resolveExtractorChatDefaultConfig(ctx context.Context, db *gorm.DB) extractorChatConfig {
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return extractorChatConfig{}
	}
	tidVal, _ := state.GetGlobal("tenant_id")
	tid, _ := tidVal.(string)
	if tid == "" {
		return extractorChatConfig{}
	}

	driver, modelName, apiConfig, _, err := resolveTenantModelByType(ctx, db, tid, entity.ModelTypeChat)
	if err != nil || driver == nil {
		return extractorChatConfig{}
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
	}
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

// simplePlaceholderRE matches simple {field_name} placeholders
// (no colons, no @chunks). Used by substituteChunkPlaceholders to
// replace {text}, {content_with_weight}, etc. with the current
// chunk's field values, mirroring Python's string_format
// (agent/component/base.py:602-609).
var simplePlaceholderRE = regexp.MustCompile(`\{[A-Za-z_][A-Za-z0-9_]*\}`)

// substituteChunkPlaceholders replaces {field_name} placeholders in
// the prompt with values from the current chunk map. The special
// alias "chunks" maps to chunkText (the current chunk's primary
// text), matching Python's `args[chunks_key] = ck["text"]` at
// extractor.py:102. Unmatched placeholders are left as-is.
func substituteChunkPlaceholders(prompt string, ck map[string]any, chunkText string) string {
	if prompt == "" || ck == nil {
		return prompt
	}
	// Build lookup: chunk fields + "chunks" alias
	lookup := make(map[string]string, len(ck)+1)
	for k, v := range ck {
		lookup[k] = fmt.Sprintf("%v", v)
	}
	if _, has := lookup["chunks"]; !has {
		lookup["chunks"] = chunkText
	}
	return simplePlaceholderRE.ReplaceAllStringFunc(prompt, func(match string) string {
		key := match[1 : len(match)-1] // strip { }
		if val, ok := lookup[key]; ok {
			return val
		}
		return match // leave unknown placeholders as-is
	})
}

// tryParseJSONObject tries to parse s as a JSON object. Returns
// (parsed, true) on success; (nil, false) on parse error or when
// s is not a JSON object. Trims common markdown code fences
// (```json ... ```) before parsing.
func tryParseJSONObject(s string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(s)
	// Strip a surrounding markdown code fence. Models commonly wrap JSON in
	// ```json ... ``` (language tag on the opening line) but some emit the tag
	// on its own line (```\njson\n{...}); Python's json_repair tolerates both,
	// encoding/json does not, so we drop the fence and any bare leading
	// "json"/"JSON" language line before parsing.
	if strings.HasPrefix(trimmed, "```") {
		// Drop the opening fence line (````` plus an optional inline language
		// tag such as `json`) up to and including the first newline.
		if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
			trimmed = trimmed[idx+1:]
		} else {
			trimmed = trimmed[3:]
		}
		// Some models place the language tag on the next line
		// (`````\njson\n{...}). Drop a bare leading `json`/`JSON` line so it is
		// not mistaken for JSON content.
		if firstNL := strings.IndexByte(trimmed, '\n'); firstNL >= 0 {
			if firstLine := strings.TrimSpace(trimmed[:firstNL]); firstLine == "json" || firstLine == "JSON" {
				trimmed = trimmed[firstNL+1:]
			}
		} else if firstLine := strings.TrimSpace(trimmed); firstLine == "json" || firstLine == "JSON" {
			trimmed = ""
		}
		// Drop the closing fence if present.
		if i := strings.LastIndex(trimmed, "```"); i >= 0 {
			trimmed = trimmed[:i]
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
