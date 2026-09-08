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

// Package component — Extractor component.
//
// SCOPE:
//
//   - PROVIDER-AGNOSTIC: the Extractor does NOT depend on any
//     specific LLM provider. It dispatches every chat call through
//     internal/entity/models.
//
//   - CONCURRENCY: auto extraction tasks (keywords, questions, summary, metadata)
//     run concurrently across chunks via a bounded worker pool (extractorPool).
//
//   - TIMEOUT / ELAPSED: the call is wrapped in
//     runtime.WithTimeout(600s) and runtime.TrackElapsed so the
//     upstream pipeline gets _created_time / _elapsed_time stamps.
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
	"ragflow/internal/dao"
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

// extractorTopNPattern matches the {{ topn }} placeholder accepted in
// keyword/question system prompts (same convention as rag/prompts/*.md).
// It is replaced with the configured top_n so the count slider stays
// authoritative even when a prompt was pre-filled by the frontend.
var extractorTopNPattern = regexp.MustCompile(`\{\{\s*topn\s*\}\}`)

// renderExtractorPrompt substitutes every {{ topn }} placeholder in the
// system prompt with the configured extraction count.
func renderExtractorPrompt(prompt string, topN int) string {
	return extractorTopNPattern.ReplaceAllString(prompt, strconv.Itoa(topN))
}

const (
	autoKeywordPrompt = `## Role
You are a text analyzer.

## Task
Extract the most important keywords/phrases of a given piece of text content.

## Requirements
- Summarize the text content, and give the top {{ topn }} important keywords/phrases.
- The keywords MUST be in the same language as the given piece of text content.
- The keywords are delimited by ENGLISH COMMA.
- Output keywords ONLY.`

	autoQuestionPrompt = `## Role
You are a text analyzer.

## Task
Propose questions about a given piece of text content.

## Requirements
- Understand and summarize the text content, and propose the top {{ topn }} important questions.
- The questions SHOULD NOT have overlapping meanings.
- The questions SHOULD cover the main content of the text as much as possible.
- The questions MUST be in the same language as the given piece of text content.
- One question per line.
- Output questions ONLY.`

	autoMetadataPrompt = `## Role: Metadata extraction expert.
## Rules:
 - Strict Evidence Only: Extract a value ONLY if it is explicitly mentioned in the Content.
 - Enum Filter: For any field with an 'enum' list, the list acts as a strict filter. If no element from the list (or its direct synonym) is found in the Content, you MUST NOT extract that field.
 - No Meta-Inference: Do not infer values based on the document's nature, format, or category. If the text does not literally state the information, treat it as missing.
 - Zero-Hallucination: Never invent information or pick a "likely" value from the enum to fill a field.
 - Empty Result: If no matches are found for any field, or if the content is irrelevant, output ONLY {}.
 - Output: ONLY a valid JSON string. No Markdown, no notes.

## Schema for extraction:
%s`

	autoSummaryPrompt = `## Role
You are a precise and faithful text summarizer.

## Task
Create a concise and faithful summary of the provided text content.

## Requirements
- The summary MUST strictly rely on the provided text without hallucinating.
- The summary MUST be in the same language as the original text content.
- Be concise and focus on the main ideas, omitting redundant details.
- Output summary ONLY.`
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
//	  "llm_id":         string,           — optional; resolves via models.NewModelFactory
//	  "keywords":       map[string]any,   — optional auto keywords extraction config
//	  "questions":      map[string]any,   — optional auto questions extraction config
//	  "tags":           map[string]any,   — optional auto tags extraction config
//	  "summary":        map[string]any,   — optional auto summary extraction config
//	  "metadata":       map[string]any,   — optional auto metadata extraction config
//	}
//
// errors here surface as canvas compile failures so a malformed
// param is caught at build time rather than mid-run.
func NewExtractorComponent(params map[string]any) (runtime.Component, error) {
	p := schema.ExtractorParam{}.Defaults()
	if params != nil {
		if v, ok := params["llm_id"].(string); ok {
			p.LLMID = v
		}

		// 1. Keywords
		if kwRaw, ok := params["keywords"].(map[string]any); ok {
			if v, ok := kwRaw["top_n"]; ok {
				p.Keywords.TopN = mapInt(v)
			}
			if v, ok := kwRaw["system_prompt"].(string); ok {
				p.Keywords.SystemPrompt = v
			}
		}

		// 2. Questions
		if qRaw, ok := params["questions"].(map[string]any); ok {
			if v, ok := qRaw["top_n"]; ok {
				p.Questions.TopN = mapInt(v)
			}
			if v, ok := qRaw["system_prompt"].(string); ok {
				p.Questions.SystemPrompt = v
			}
		}

		// 3. Tags
		if tagRaw, ok := params["tags"].(map[string]any); ok {
			if v, ok := tagRaw["top_n"]; ok {
				p.Tags.TopN = mapInt(v)
			}
			if v, ok := tagRaw["tag_file_id"].(string); ok {
				p.Tags.TagFileID = v
			}
		}

		// 4. Summary
		if sumRaw, ok := params["summary"].(map[string]any); ok {
			if v, ok := sumRaw["enabled"].(bool); ok {
				p.Summary.Enabled = v
			} else if v, ok := sumRaw["enabled"]; ok {
				p.Summary.Enabled = mapInt(v) == 1
			}
			if v, ok := sumRaw["system_prompt"].(string); ok {
				p.Summary.SystemPrompt = v
			}
		}

		// 5. Metadata
		if metaRaw, ok := params["metadata"].(map[string]any); ok {
			if v, ok := metaRaw["enabled"].(bool); ok {
				p.Metadata.Enabled = v
			} else if v, ok := metaRaw["enabled"]; ok {
				p.Metadata.Enabled = mapInt(v) == 1
			}
			if v, ok := metaRaw["metadata"]; ok {
				p.Metadata.Metadata = parseMetadataFieldDefs(v)
			}
			// BuiltInMetadata is carried for persistence/replay; LLM extraction uses Metadata only.
			if v, ok := metaRaw["built_in_metadata"]; ok {
				p.Metadata.BuiltInMetadata = parseMetadataFieldDefs(v)
			}
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
		"chunks": "List of map[string]any from upstream Tokenizer. Each entry must carry a string 'text' (or 'content_with_weight') field. Optional — when absent the LLM is called once with the resolved args.",
		"llm_id": "Optional per-call LLM id override. Falls back to Param.LLMID when absent.",
	}
}

// Outputs returns the public surface downstream ingestion
// consumers can wire into. Mirrors schema.ExtractorOutputs.
//
//	chunks         []map[string]any — input chunks, each augmented with
//	                                 extracted modular fields (important_kwd,
//	                                 question_kwd, tag_kwd, summary, metadata).
//	output_format  string          — always "chunks". Parity with
//	                                 python set_output contract.
//	_ERROR         string          — populated on a short-circuit
//	                                 error (matches python
//	                                 set_output("_ERROR", ...)).
func (c *ExtractorComponent) Outputs() map[string]string {
	return map[string]string{
		"chunks":        "Extraction results — input chunks, each enriched with modular extraction fields (important_kwd, question_kwd, tag_kwd, summary, metadata).",
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
	chatCfg := &models.ChatConfig{}
	if req.Temperature != nil {
		temp := *req.Temperature
		chatCfg.Temperature = &temp
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
	common.Debug(fmt.Sprintf("extractor: chat completed for model %s, response_length=%d", modelName, len(out.Content)))
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
	llmID  string
	lang   string
	chunks []map[string]any
	// temperature overrides the LLM temperature for this call. A
	// nil value leaves the request's Temperature unset so the model
	// (or the chat-model default) decides. The keyword/question helpers set it to
	// extractorTemperature (0.2) to mirror generator.py.
	temperature *float64
}

// resolveInputs overlays per-call inputs on top of the
// component's static Param. Missing keys fall back to the
// Param-level values; per-call values win on conflict (so a
// canvas can override LLM_ID at runtime).
func (c *ExtractorComponent) resolveInputs(inputs map[string]any) extractorInputs {
	out := extractorInputs{
		llmID: c.Param.LLMID,
	}
	if inputs == nil {
		return out
	}
	if v, ok := inputs["llm_id"].(string); ok && v != "" {
		out.llmID = v
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
//	llm_id         (optional, string)            — overrides Param.LLMID.
//
// Outputs:
//
//	chunks        ([]map[string]any) — input chunks augmented with extraction results.
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
	if len(in.chunks) == 0 {
		return map[string]any{
			"chunks":        []map[string]any{},
			"output_format": "chunks",
		}, nil
	}

	if err := runtime.WithTimeout(ctx, extractorTimeout, func(timeoutCtx context.Context) error {
		// Phase 1: Keywords extraction (if enabled), running across chunks via extractorPool.
		if c.Param.Keywords.TopN > 0 {
			if err := c.runAutoKeywordsPool(timeoutCtx, db, in); err != nil {
				return err
			}
		}

		// Phase 2: Tag phase (if enabled), benefiting from title and freshly extracted keywords.
		if c.Param.Tags.TopN > 0 {
			tagged, tagErr := c.runAutoTags(timeoutCtx, db, in)
			if tagErr != nil {
				return tagErr
			}
			in.chunks = tagged
		}

		// Phase 3: Remaining extractions (Questions, Summary, Metadata) via extractorPool.
		return c.runRemainingExtractions(timeoutCtx, db, in)
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

// extractorLLMCacheKey builds a Redis key for textual extractions.
func extractorLLMCacheKey(taskType, modelID, systemPrompt, text string) string {
	h := xxhash.New()
	h.WriteString(taskType)
	h.WriteString("\x00")
	h.WriteString(modelID)
	h.WriteString("\x00")
	h.WriteString(systemPrompt)
	h.WriteString("\x00")
	h.WriteString(text)
	return fmt.Sprintf("kc:extractor:%s:%x", taskType, h.Sum64())
}

// callTextCached wraps callText with a 24-hour Redis cache.
func (c *ExtractorComponent) callTextCached(ctx context.Context, db *gorm.DB, in extractorInputs, taskType, systemPrompt, chunkText string) (string, error) {
	key := extractorLLMCacheKey(taskType, in.llmID, systemPrompt, chunkText)
	if client := redis.Get(); client != nil {
		if data, err := client.Get(ctx, key); err == nil && data != "" {
			return data, nil
		}
	}
	res, err := c.callText(ctx, db, in, systemPrompt, chunkText)
	if err != nil {
		return "", err
	}
	res = cleanExtractionResult(res)
	if res != "" && !strings.Contains(res, "**ERROR**") {
		if client := redis.Get(); client != nil {
			client.Set(ctx, key, res, 24*time.Hour)
		}
	}
	return res, nil
}

// runAutoKeywords extracts keywords for the current chunk and stores
// them on ck["important_kwd"]. Keyword extraction pins
// temperature to extractorTemperature (0.2) to mirror generator.py.
func (c *ExtractorComponent) runAutoKeywords(ctx context.Context, db *gorm.DB, in extractorInputs, ck map[string]any, chunkText string) error {
	if _, exists := ck["important_kwd"]; exists {
		return nil
	}
	topN := c.Param.Keywords.TopN
	if topN <= 0 {
		return nil
	}
	systemPrompt := strings.TrimSpace(c.Param.Keywords.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = autoKeywordPrompt
	}
	systemPrompt = renderExtractorPrompt(systemPrompt, topN)
	kwTemp := extractorTemperature
	kwIn := extractorInputs{
		llmID:       in.llmID,
		temperature: &kwTemp,
	}
	resultStr, err := c.callTextCached(ctx, db, kwIn, "keywords", systemPrompt, chunkText)
	if err != nil {
		return err
	}
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
	if _, exists := ck["question_kwd"]; exists {
		return nil
	}
	topN := c.Param.Questions.TopN
	if topN <= 0 {
		return nil
	}
	systemPrompt := strings.TrimSpace(c.Param.Questions.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = autoQuestionPrompt
	}
	systemPrompt = renderExtractorPrompt(systemPrompt, topN)
	qTemp := extractorTemperature
	qIn := extractorInputs{
		llmID:       in.llmID,
		temperature: &qTemp,
	}
	resultStr, err := c.callTextCached(ctx, db, qIn, "questions", systemPrompt, chunkText)
	if err != nil {
		return err
	}
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

// runAutoSummary extracts a concise summary for the current chunk using autoSummaryPrompt
// and stores it on ck["summary"].
func (c *ExtractorComponent) runAutoSummary(ctx context.Context, db *gorm.DB, in extractorInputs, ck map[string]any, chunkText string) error {
	if _, exists := ck["summary"]; exists {
		return nil
	}
	if !c.Param.Summary.Enabled {
		return nil
	}
	systemPrompt := c.Param.Summary.SystemPrompt
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = autoSummaryPrompt
	}
	sumTemp := extractorTemperature
	sumIn := extractorInputs{
		llmID:       in.llmID,
		temperature: &sumTemp,
	}
	resultStr, err := c.callTextCached(ctx, db, sumIn, "summary", systemPrompt, chunkText)
	if err != nil {
		return err
	}
	if resultStr == "" {
		return nil
	}
	ck["summary"] = resultStr
	return nil
}

// runAutoKeywordsPool dispatches keyword extraction across all chunks concurrently
// using extractorPool before the tagging stage.
func (c *ExtractorComponent) runAutoKeywordsPool(ctx context.Context, db *gorm.DB, in extractorInputs) error {
	if c.Param.Keywords.TopN <= 0 || len(in.chunks) == 0 {
		return nil
	}
	futs := make([]utility.WorkerPoolFuture[extractorJob, struct{}], 0, len(in.chunks))
	for i, ck := range in.chunks {
		i, ck := i, ck
		text, _ := ck["content_with_weight"].(string)
		if strings.TrimSpace(text) == "" {
			text, _ = ck["text"].(string)
		}
		fn := func() error {
			if err := c.runAutoKeywords(ctx, db, in, ck, text); err != nil {
				return fmt.Errorf("chunk %d keywords: %w", i, err)
			}
			return nil
		}
		f, err := extractorPool.Submit(ctx, fn)
		if err != nil {
			return err
		}
		futs = append(futs, f)
	}
	return awaitFutures(ctx, futs)
}

// runRemainingExtractions dispatches auto questions / summary / metadata
// extractions across all chunks concurrently using extractorPool.
func (c *ExtractorComponent) runRemainingExtractions(ctx context.Context, db *gorm.DB, in extractorInputs) error {
	if c.Param.Questions.TopN <= 0 && !c.Param.Summary.Enabled && !c.Param.Metadata.Enabled {
		return nil
	}
	futs := make([]utility.WorkerPoolFuture[extractorJob, struct{}], 0, len(in.chunks))
	for i, ck := range in.chunks {
		i, ck := i, ck
		text, _ := ck["content_with_weight"].(string)
		if strings.TrimSpace(text) == "" {
			text, _ = ck["text"].(string)
		}
		fn := c.remainingExtractionJob(ctx, db, in, i, ck, text)
		f, err := extractorPool.Submit(ctx, fn)
		if err != nil {
			return err
		}
		futs = append(futs, f)
	}
	return awaitFutures(ctx, futs)
}

func (c *ExtractorComponent) remainingExtractionJob(ctx context.Context, db *gorm.DB, in extractorInputs, idx int, ck map[string]any, chunkText string) extractorJob {
	return func() error {
		if c.Param.Questions.TopN > 0 {
			if err := c.runAutoQuestions(ctx, db, in, ck, chunkText); err != nil {
				return fmt.Errorf("chunk %d questions: %w", idx, err)
			}
		}
		if c.Param.Summary.Enabled {
			if err := c.runAutoSummary(ctx, db, in, ck, chunkText); err != nil {
				return fmt.Errorf("chunk %d summary: %w", idx, err)
			}
		}
		if c.Param.Metadata.Enabled {
			if err := c.runEnableMetadata(ctx, db, in, ck, chunkText); err != nil {
				return fmt.Errorf("chunk %d metadata: %w", idx, err)
			}
		}
		return nil
	}
}

func awaitFutures(ctx context.Context, futs []utility.WorkerPoolFuture[extractorJob, struct{}]) error {
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

// runEnableMetadata extracts structured metadata for the current chunk and
// merges the parsed JSON object into ck["metadata"]. It mirrors the
// runAutoKeywords/runAutoQuestions shape but parses a JSON object and
// merges into the chunk's metadata map.
func (c *ExtractorComponent) runEnableMetadata(ctx context.Context, db *gorm.DB, in extractorInputs, ck map[string]any, chunkText string) error {
	if !c.Param.Metadata.Enabled || len(c.Param.Metadata.Metadata) == 0 {
		return nil
	}
	// Render the field schema into the prompt, mirroring Python's
	// turn2jsonschema(metadata_conf) rendered into the META_DATA template.
	schemaMap := common.Turn2JSONSchema(c.Param.Metadata.Metadata)
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
			llmID:       in.llmID,
			temperature: &metaTemp,
		}
		systemPrompt := fmt.Sprintf(autoMetadataPrompt, schemaStr)
		parsed, err = c.callStructured(ctx, db, metaIn, systemPrompt, chunkText)
		if err != nil {
			return err
		}
		if parsed == nil {
			// Non-JSON or empty response — nothing to extract, not an error.
			return nil
		}
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

// callRaw runs one chat call against the LLM (per chunk in the normal path)
// and returns the raw response. It holds the shared plumbing — driver/target
// resolution, message assembly, temperature override, retry — but deliberately
// does NOT clean or parse the response. Callers pick the view they need:
//
//   - callText wraps callRaw with the LLM-layer two-step cleanup (think +
//     tool_call) and returns a plain string.
//   - callStructured wraps callRaw with cleanup + explicit JSON parsing and
//     returns a map — the metadata path, matching Python gen_metadata.
func (c *ExtractorComponent) callRaw(ctx context.Context, db *gorm.DB, in extractorInputs, systemPrompt, chunkText string) (*extractorChatResponse, error) {
	driver, modelName, apiKey, baseURL, err := resolveExtractorChatTarget(ctx, db, in.llmID)
	if err != nil {
		return nil, err
	}
	msgs := buildExtractorMessages(systemPrompt, chunkText)
	fitted, fitErr := fitExtractorMessages(ctx, db, in.llmID, msgs)
	if fitErr != nil {
		return nil, fitErr
	}
	msgs = fitted
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
	if resp == nil {
		// Defensive: a provider adapter returning (nil, nil) would otherwise
		// panic on resp.Content below. Surface a diagnosable error instead.
		return nil, fmt.Errorf("extractor: chat: nil response from invoker")
	}
	return resp, nil
}

// toolCallRE matches a <tool_call>...</tool_call> block, mirroring Python's
// `re.sub(r"<tool_call>.*?</tool_call>", "", txt, flags=re.DOTALL)` at
// llm_service.py:461. Non-greedy so consecutive tool_call blocks are each
// stripped.
var toolCallRE = regexp.MustCompile(`(?s)<tool_call>.*?</tool_call>`)

// cleanLLMText applies the two-step LLM-layer cleanup that Python performs
// in LLMBundle.async_chat (llm_service.py:459-461) for every response,
// before any parsing:
//
//  1. Reasoning content: when the response BEGINS with `<think>`, take
//     everything after the LAST `</think>`. A `<think>` that appears after
//     a non-empty prefix is treated as ordinary content and preserved, so a
//     legitimate prefix is never stripped. Python's _remove_reasoning_content
//     (llm_service.py:332-346) finds the first `<think>` anywhere; restricting
//     to a leading `<think>` avoids deleting real content that merely contains
//     a think-looking tag. A leading `<think>` with no closing `</think>` is
//     kept unchanged.
//  2. Tool-call blocks: remove `<tool_call>...</tool_call>` spans.
//
// The result is a plain string, ready for the caller to consume as text or
// to parse explicitly.
func cleanLLMText(s string) string {
	if strings.HasPrefix(s, "<think>") {
		s = common.StripThinkTrailing(s)
	}
	s = toolCallRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// cleanExtractionResult strips `</think>` tags and rejects `**ERROR**` responses,
// matching Python's keyword_extraction and question_proposal post-processing.
func cleanExtractionResult(s string) string {
	s = common.StripThinkTrailing(s)
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

// callText runs one chat call and returns the cleaned text response.
func (c *ExtractorComponent) callText(ctx context.Context, db *gorm.DB, in extractorInputs, systemPrompt, chunkText string) (string, error) {
	resp, err := c.callRaw(ctx, db, in, systemPrompt, chunkText)
	if err != nil {
		return "", err
	}
	return cleanLLMText(resp.Content), nil
}

// callStructured runs one chat call, cleans the response, and explicitly
// parses it as a JSON object. Used by the metadata path, which needs a
// structured value to merge into chunk metadata — matching Python
// gen_metadata's json_repair.loads. A non-JSON or empty response returns
// (nil, nil) so the caller treats it as "nothing extracted", not an error.
func (c *ExtractorComponent) callStructured(ctx context.Context, db *gorm.DB, in extractorInputs, systemPrompt, chunkText string) (map[string]any, error) {
	resp, err := c.callRaw(ctx, db, in, systemPrompt, chunkText)
	if err != nil {
		return nil, err
	}
	s := cleanLLMText(resp.Content)
	if s == "" {
		return nil, nil
	}
	// Second cleanup layer, mirroring Python gen_metadata's
	// re.sub(r"^.*</think>", "", ans, re.DOTALL): cleanLLMText only strips a
	// leading <think>, so a mid-text reasoning block preceded by a preamble
	// would otherwise survive into JSON parsing and silently drop the whole
	// metadata extraction that Python would have kept. No **ERROR** check here
	// — Python's gen_metadata has none either.
	s = common.StripThinkTrailing(s)
	parsed, ok := tryParseJSONObject(s)
	if !ok {
		return nil, nil
	}
	return parsed, nil
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

// extractorContextLengthOverride is a narrow test seam mirroring
// extractorChatTargetResolverOverride: it lets unit tests supply a context
// length without a real tenant model row, so the message-fitting wiring in
// callRaw/llmTagChunk can be exercised without a DB. When set,
// extractorContextLength consults it first.
var (
	extractorContextLengthOverrideMu sync.RWMutex
	extractorContextLengthOverride   func(ctx context.Context, llmID string) int
)

// SetExtractorContextLengthOverride swaps the package-level context-length
// resolver for tests. Pass nil to restore the default. Concurrent-safe.
func SetExtractorContextLengthOverride(fn func(ctx context.Context, llmID string) int) {
	extractorContextLengthOverrideMu.Lock()
	defer extractorContextLengthOverrideMu.Unlock()
	extractorContextLengthOverride = fn
}

func getExtractorContextLengthOverride() func(ctx context.Context, llmID string) int {
	extractorContextLengthOverrideMu.RLock()
	defer extractorContextLengthOverrideMu.RUnlock()
	return extractorContextLengthOverride
}

// extractorContextLength returns the chat model's context window
// (content_length) for the effective chat model used by a call, or 0 when
// unavailable. Mirrors Python's chat_mdl.max_length. Used as the token
// budget for message fitting so oversized prompts are trimmed instead of
// rejected by the provider. When llm_id is empty the call falls back to the
// tenant default chat model (see resolveExtractorChatTarget), so the same
// model is resolved here; otherwise the default-model path would never get
// message fitting. Returns 0 (skip fitting) when the model is unknown (e.g.
// unit tests with synthetic llm_id or no canvas state).
func extractorContextLength(ctx context.Context, db *gorm.DB, llmID string) int {
	if fn := getExtractorContextLengthOverride(); fn != nil {
		return fn(ctx, llmID)
	}
	if db == nil {
		db = dao.DB
	}
	state, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
	if err != nil || state == nil {
		return 0
	}
	tidVal, _ := state.GetGlobal("tenant_id")
	tid, _ := tidVal.(string)
	if tid == "" {
		return 0
	}
	if llmID == "" {
		llmID = defaultChatModelRef(ctx, db, tid)
	}
	if llmID == "" {
		return 0
	}
	return dao.ResolveModelContentLength(ctx, db, tid, llmID, "", "")
}

// defaultChatModelRef returns the tenant's default chat model reference —
// the tenant_model UUID when one is pinned, otherwise the composite
// "model@provider" id — or "" when the tenant has no default chat model.
func defaultChatModelRef(ctx context.Context, db *gorm.DB, tenantID string) string {
	if db == nil {
		// No database to read the tenant's default chat model from.
		return ""
	}
	tenant, err := dao.NewTenantDAO().GetByID(ctx, db, tenantID)
	if err != nil || tenant == nil {
		return ""
	}
	if tenant.TenantLLMID != nil && *tenant.TenantLLMID != "" {
		return *tenant.TenantLLMID
	}
	return tenant.LLMID
}

// extractorContextFitBudget returns 97% of the model's context window as the
// fitting budget, mirroring the agent component's contextFitBudget. The
// margin leaves headroom for the difference between the cl100k tokenizer used
// for counting and the model's own tokenizer, plus per-message formatting
// overhead, so a fitted prompt stays inside the provider's real context limit
// instead of landing exactly on it.
func extractorContextFitBudget(ctxLen int) int {
	budget := int(float64(ctxLen) * 0.97)
	if budget < 1 {
		// Never hand Fit a <=0 budget: Fit treats <=0 as the 8192
		// default, which would stop trimming entirely for a tiny context.
		return 1
	}
	return budget
}

// fitExtractorMessages trims msgs to the chat model's context window using
// the shared tokenizer fitter (mirrors Python's message_fit_in), dropping
// entries the fitter removed. It returns a clear error instead of letting a
// conversation whose final user turn was trimmed to empty reach the provider:
// the proportional trim can do that when the system prompt alone exceeds the
// context budget, and providers reject empty user turns with an obscure error
// after retries.
func fitExtractorMessages(ctx context.Context, db *gorm.DB, llmID string, msgs []eschema.Message) ([]eschema.Message, error) {
	ctxLen := extractorContextLength(ctx, db, llmID)
	if ctxLen <= 0 {
		return msgs, nil
	}
	fitMsgs := make([]tokenizer.Message, len(msgs))
	for i := range msgs {
		fitMsgs[i] = tokenizer.Message{Role: string(msgs[i].Role), Content: msgs[i].Content}
	}
	kept, keptIdx, _ := tokenizer.Fit(fitMsgs, extractorContextFitBudget(ctxLen))

	fitted := make([]eschema.Message, 0, len(kept))
	for j, i := range keptIdx {
		msgs[i].Content = kept[j].Content
		fitted = append(fitted, msgs[i])
	}
	if len(fitted) == 0 {
		return nil, errors.New("extractor: message fitting dropped every message; check the chat model context length setting")
	}
	// The system prompt carries the extraction contract (output format,
	// field definitions); sending without it would silently produce
	// garbage. The proportional trim can empty every system message when
	// the final user turn alone fills the budget, so require at least one
	// retained system message with non-empty content — a system message kept
	// but trimmed to empty is just as useless as a dropped one. The guard only
	// applies when the input actually had a system message: systemPrompt is
	// optional and a user-only prompt is a valid request.
	hadSystem := false
	for _, m := range msgs {
		if m.Role == eschema.System {
			hadSystem = true
			break
		}
	}
	if hadSystem {
		keptSystem := false
		for _, m := range fitted {
			if m.Role == eschema.System && strings.TrimSpace(m.Content) != "" {
				keptSystem = true
				break
			}
		}
		if !keptSystem {
			return nil, errors.New("extractor: message fitting emptied the system prompt; check the chat model context length setting or reduce the prompt size")
		}
	}
	last := fitted[len(fitted)-1]
	if last.Role != eschema.User || strings.TrimSpace(last.Content) == "" {
		return nil, errors.New("extractor: message fitting emptied the final user turn; check the chat model context length setting or reduce the prompt size")
	}
	return fitted, nil
}

// buildExtractorMessages assembles system + user messages for one extraction
// call. The user message strictly carries chunkText (or a fallback single space
// if empty), ensuring a clean and consistent message contract.
// System prompt is omitted if empty or whitespace-only to avoid sending empty
// system turns to LLM providers.
func buildExtractorMessages(systemPrompt, chunkText string) []eschema.Message {
	out := make([]eschema.Message, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		out = append(out, eschema.Message{Role: eschema.System, Content: systemPrompt})
	}
	userContent := chunkText
	if strings.TrimSpace(userContent) == "" {
		userContent = " "
	}
	out = append(out, eschema.Message{Role: eschema.User, Content: userContent})
	return out
}

// tryParseJSONObject tries to parse s as a JSON object. Returns
// (parsed, true) on success; (nil, false) on parse error or when
// s is not a JSON object. Trims common Markdown code fences
// (```json ... ```) before parsing.
func tryParseJSONObject(s string) (map[string]any, bool) {
	trimmed := strings.TrimSpace(s)
	// Strip a surrounding Markdown code fence. Models commonly wrap JSON in
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

// parseMetadataFieldDefs converts an any value (typically []any of maps)
// to a typed []common.MetadataFieldDef slice.
func parseMetadataFieldDefs(v any) []common.MetadataFieldDef {
	if v == nil {
		return nil
	}
	if defs, ok := v.([]common.MetadataFieldDef); ok {
		return defs
	}
	var arr []any
	switch typed := v.(type) {
	case []any:
		arr = typed
	case []map[string]any:
		arr = make([]any, 0, len(typed))
		for _, item := range typed {
			arr = append(arr, item)
		}
	default:
		return nil
	}
	fields := make([]common.MetadataFieldDef, 0, len(arr))
	for _, f := range arr {
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
	return fields
}

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
