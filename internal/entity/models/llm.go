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

// Package models — EinoChatModel thin wrapper (Phase 2 P0, plan §2.11.6 D1).
//
// Bridges the existing RAGFlow provider-specific *ChatModel (OpenAI, Anthropic,
// Gemini, …) to eino's model.BaseChatModel / model.ToolCallingChatModel
// interface so the ReAct agent (internal/agent/component/agent.go) can
// consume it directly. The wrapper does NOT reimplement provider logic — it
// translates eino's []schema.Message + model.Option into the existing
// ChatModel + APIConfig + ChatConfig call shape, and converts the
// *ChatResponse back into a *schema.Message.
//
// Why a separate file: the plan forbids editing existing files in this
// package (types.go, dummy.go, openai.go, …). Adding llm.go keeps the bridge
// self-contained and easy to remove if/when providers get first-class eino
// adapters.
package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// EinoChatModel adapts one or more RAGFlow *ChatModels to eino's chat model
// interfaces. With more than one model it forms a FAILOVER chain: Generate /
// Stream start at the STICKY CURSOR and, when a call ends in ANY terminal
// error (provider quota walls, rate limits, outages, auth failures), move to
// the next entry — only a full sweep of the chain reports failure. The cursor
// stays on the model that last served a call, so a dead primary is not re-hit
// on every single Generate of a long conversation.
//
// It is safe for concurrent use: the cursor is mutex-guarded, and the
// per-request tool state is only mutated through WithTools (which returns a
// new instance, never mutating in place — see eino's
// components/model/interface.go:84-99 for the rationale).
type EinoChatModel struct {
	chain  []*ChatModel
	labels []string // human-readable tag per chain entry (model @ instance), logs only
	// sweep caches the outcome of the last FULL-CHAIN failure. A provider
	// quota wall does not recover within one question, and an agentic turn
	// issues tens of Generate calls — without this, one dead roster costs
	// (calls × models) doomed round-trips per question. Inside the cooldown a
	// new Generate returns the cached error immediately instead of sweeping
	// the chain again.
	sweep struct {
		failedAt time.Time
		err      error
	}

	chatCfg    *ChatConfig
	tools      []*schema.ToolInfo
	toolChoice *string

	mu     sync.Mutex // guards cursor + sweep
	cursor int        // chain index to try first on the next call
}

// failoverCooldown is how long a full-chain failure short-circuits later
// Generate calls. Long enough to absorb an agentic turn (which would otherwise
// re-sweep the roster on every ReAct step), short enough that a replenished
// plan is picked up without a restart.
const failoverCooldown = 30 * time.Second

// cacheableSweepFailure reports whether a terminal chain failure may be cached
// to short-circuit later Generate/Stream calls for failoverCooldown.
//
// Only provider-WIDE failures qualify. A request-level failure (invalid
// messages, an unknown tool, a bad parameter) also fails on every chain entry,
// but it says nothing about the next request — caching it would answer every
// valid turn with that stale error for the whole cooldown.
func cacheableSweepFailure(err error) bool {
	if err == nil {
		return false
	}
	// A cancelled context is the caller's doing, not the provider's health.
	if errors.Is(err, context.Canceled) {
		return false
	}
	// Transport failures (timeout, DNS, refused connection) hit every request.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	// A provider status: rate limiting and 5xx mean the provider cannot serve
	// anything right now; any other 4xx is about THIS request.
	var statusErr *APIStatusError
	if errors.As(err, &statusErr) {
		return statusErr.Status == http.StatusTooManyRequests || statusErr.Status >= 500
	}
	return false
}

// NewEinoChatModel wraps an existing RAGFlow *ChatModel so it can be passed
// to eino constructs (ReAct agent, Workflow, etc.). The chatConfig argument
// carries temperature / max_tokens / etc. — pass nil for provider defaults.
//
// Driver is taken from cm.ModelDriver, model name from cm.ModelName, and
// API key / region from cm.APIConfig. These are fixed for the lifetime of
// the wrapper; per-request variations belong in WithTools / a new instance.
// For a model chain with automatic failover, see NewFailoverEinoChatModel.
func NewEinoChatModel(cm *ChatModel, chatConfig *ChatConfig) *EinoChatModel {
	return &EinoChatModel{
		chain:   []*ChatModel{cm},
		chatCfg: chatConfig,
	}
}

// NewFailoverEinoChatModel wraps a chain of RAGFlow *ChatModels. models[0] is
// the primary; on a terminal Generate/Stream error the next entry is tried,
// and the chain is swept at most once per call before the last error is
// returned. Use NewEinoChatModel for a single model.
func NewFailoverEinoChatModel(models []*ChatModel, chatConfig *ChatConfig) (*EinoChatModel, error) {
	return NewFailoverEinoChatModelWithLabels(models, nil, chatConfig)
}

// NewFailoverEinoChatModelWithLabels is NewFailoverEinoChatModel plus a
// human-readable label per entry (e.g. "MiniMax-M3 @ zyf"). Chain entries are
// frequently the same model name on different provider instances, so the
// label is what makes failover logs attributable. A missing label falls back
// to the model name.
func NewFailoverEinoChatModelWithLabels(models []*ChatModel, labels []string, chatConfig *ChatConfig) (*EinoChatModel, error) {
	chain := make([]*ChatModel, 0, len(models))
	tags := make([]string, 0, len(models))
	for i, cm := range models {
		if cm == nil || cm.ModelDriver == nil {
			continue
		}
		chain = append(chain, cm)
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		if label == "" {
			label = modelNameOf(cm)
		}
		tags = append(tags, label)
	}
	if len(chain) == 0 {
		return nil, fmt.Errorf("models: NewFailoverEinoChatModel: no usable chat model in chain")
	}
	return &EinoChatModel{
		chain:   chain,
		labels:  tags,
		chatCfg: chatConfig,
	}, nil
}

// name returns the primary model's name (best-effort; nil-safe).
func (m *EinoChatModel) name() string {
	// chain[0] itself can be nil: NewEinoChatModel takes the caller's *ChatModel
	// as-is (unlike the failover constructor, which filters nil entries), so a
	// nil model must not turn Name() into a nil dereference.
	if m == nil || len(m.chain) == 0 || m.chain[0] == nil || m.chain[0].ModelName == nil {
		return ""
	}
	return *m.chain[0].ModelName
}

// toInternalMessages converts eino's []schema.Message into the existing
// RAGFlow []Message type. System / user / assistant roles are preserved;
// tool-role messages are mapped to "tool" (the existing model layer already
// speaks that string — see types.go:9).
func toInternalMessages(msgs []*schema.Message) []Message {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]Message, 0, len(msgs))
	for _, mm := range msgs {
		if mm == nil {
			continue
		}
		role := string(mm.Role)
		if role == "" {
			role = "user"
		}
		msg := Message{Role: role, Content: mm.Content}
		if len(mm.UserInputMultiContent) > 0 {
			if blocks := openAIContentBlocksFromEino(mm.UserInputMultiContent); len(blocks) > 0 {
				msg.Content = blocks
			}
		}
		if len(mm.ToolCalls) > 0 {
			msg.ToolCalls = toolCallsToInternal(mm.ToolCalls)
		}
		if mm.ToolCallID != "" {
			msg.ToolCallID = mm.ToolCallID
		}
		out = append(out, msg)
	}
	return out
}

// openAIContentBlocksFromEino converts eino multi-modal input parts into
// OpenAI-style content blocks ("text" / "image_url"). Message.Content is
// interface{} and every driver already understands this block shape: the
// generic OpenAI-compatible request builder marshals it verbatim
// (buildChatMessages in base_model.go), while the native anthropic /
// google converters type-switch on []interface{} (anthropicContent /
// googleMessageParts). The slice MUST therefore be []interface{}, not
// []map[string]interface{}, or googleMessageParts misses it. Unsupported
// part types are skipped; a nil return tells the caller to fall back to
// the plain string Content.
func openAIContentBlocksFromEino(parts []schema.MessageInputPart) []interface{} {
	blocks := make([]interface{}, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			if part.Text == "" {
				continue
			}
			blocks = append(blocks, map[string]interface{}{"type": "text", "text": part.Text})
		case schema.ChatMessagePartTypeImageURL:
			url := einoImagePartURL(part.Image)
			if url == "" {
				continue
			}
			blocks = append(blocks, map[string]interface{}{
				"type":      "image_url",
				"image_url": map[string]interface{}{"url": url},
			})
		}
	}
	if len(blocks) == 0 {
		return nil
	}
	return blocks
}

// einoImagePartURL resolves an image part to a single URL string: either
// the direct URL (the agent component carries data URIs this way) or a
// reassembled data URI from Base64Data + MIMEType.
func einoImagePartURL(img *schema.MessageInputImage) string {
	if img == nil {
		return ""
	}
	if img.URL != nil && *img.URL != "" {
		return *img.URL
	}
	if img.Base64Data != nil && *img.Base64Data != "" {
		mime := img.MIMEType
		if mime == "" {
			mime = "image/png"
		}
		return "data:" + mime + ";base64," + *img.Base64Data
	}
	return ""
}

// fromInternalResponse converts a *ChatResponse to *schema.Message. The
// existing ChatResponse only carries answer text (+ optional reasoning), so
// the resulting Message has Role=Assistant and Content=answer.
func fromInternalResponse(resp *ChatResponse) *schema.Message {
	if resp == nil {
		return &schema.Message{Role: schema.Assistant, Content: ""}
	}
	content := ""
	if resp.Answer != nil {
		content = *resp.Answer
	}
	msg := &schema.Message{Role: schema.Assistant, Content: content}
	if resp.ReasonContent != nil {
		msg.ReasoningContent = *resp.ReasonContent
	}
	if len(resp.ToolCalls) > 0 {
		msg.ToolCalls = toolCallsFromInternal(resp.ToolCalls)
	}
	if resp.Usage != nil {
		// The call's token split travels ON THE MESSAGE (eino's own per-response
		// metadata), so a caller that reports a node's cost reads it from the value it
		// was handed instead of a field shared by every call on the ChatModel - where a
		// concurrent call could replace it between the write and the read.
		msg.ResponseMeta = &schema.ResponseMeta{
			Usage: &schema.TokenUsage{
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			},
		}
	}
	return msg
}

// Generate blocks until the model returns a complete response. Mirrors
// eino's model.BaseChatModel.Generate. With a failover chain, a terminal
// error moves the call to the next entry (wrapping around once), and the
// sticky cursor stays on the entry that served the call.
func (m *EinoChatModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if m == nil || len(m.chain) == 0 {
		return nil, fmt.Errorf("models: EinoChatModel: empty model chain")
	}
	// A full sweep that just failed stays failed for the cooldown: replaying
	// the whole roster on every ReAct step of one question is what turns a
	// dead plan into a wall of identical provider errors.
	m.mu.Lock()
	start := m.cursor
	if waited := time.Since(m.sweep.failedAt); m.sweep.err != nil && waited < failoverCooldown {
		err := m.sweep.err
		m.mu.Unlock()
		common.DebugCtx(ctx, "models: eino generate short-circuited by failover cooldown",
			zap.Duration("waited", waited), zap.Error(err))
		return nil, err
	}
	m.mu.Unlock()

	var lastErr error
	// cacheable stays true only while EVERY failed attempt was a provider-wide
	// failure: a sweep that mixes a request-specific 400 with a transient 503 must not
	// be remembered as "the whole roster is down", or the next request is rejected on
	// the cooldown without trying a provider that could have served it.
	cacheable := true
	for i := 0; i < len(m.chain); i++ {
		idx := (start + i) % len(m.chain)
		cm := m.chain[idx]
		if err := ctx.Err(); err != nil {
			// The shared budget is spent: every remaining model would fail
			// identically, so don't burn the chain on a dead context.
			return nil, err
		}
		resp, err := m.generateOnce(ctx, cm, msgs, opts...)
		if err == nil {
			m.mu.Lock()
			m.cursor = idx
			m.sweep.failedAt = time.Time{}
			m.sweep.err = nil
			m.mu.Unlock()
			return resp, nil
		}
		lastErr = err
		cacheable = cacheable && cacheableSweepFailure(err)
		next := (idx + 1) % len(m.chain)
		common.WarnCtx(ctx, "models: eino generate failed, failing over to next model",
			zap.String("failed_model", m.labelOf(idx)),
			zap.String("next_model", m.labelOf(next)),
			zap.Error(err))
	}

	// Terminal: every entry failed. Rotate the cursor so the next attempt starts
	// on a different entry instead of always spending the primary's error budget
	// first, and cache the outcome only when EVERY attempt was a provider-wide
	// failure (see cacheable above), not merely the last one.
	cached := lastErr != nil && cacheable
	m.mu.Lock()
	m.cursor = (start + 1) % len(m.chain)
	if cached {
		m.sweep.failedAt = time.Now()
		m.sweep.err = lastErr
	}
	m.mu.Unlock()
	common.WarnCtx(ctx, "models: eino generate failed on every model in the chain",
		zap.Int("models", len(m.chain)), zap.Duration("cooldown", failoverCooldown),
		zap.Bool("cached", cached), zap.Error(lastErr))
	return nil, lastErr
}

// labelOf returns the human-readable chain entry tag (model @ instance).
func (m *EinoChatModel) labelOf(idx int) string {
	if idx < 0 || idx >= len(m.chain) {
		return "(out of range)"
	}
	if idx < len(m.labels) && m.labels[idx] != "" {
		return m.labels[idx]
	}
	return modelNameOf(m.chain[idx])
}

// generateOnce runs one Generate attempt against a single chain entry.
func (m *EinoChatModel) generateOnce(ctx context.Context, cm *ChatModel, msgs []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if cm == nil || cm.ModelDriver == nil {
		return nil, fmt.Errorf("models: EinoChatModel: nil inner ModelDriver")
	}
	internal := toInternalMessages(msgs)
	if cm.ModelName == nil {
		return nil, fmt.Errorf("models: EinoChatModel: nil model name")
	}
	// ChatWithMessages does not take a context.Context today — Phase 0 kept
	// the signature stable. We log a guard so a future context-aware
	// signature can be slotted in without changing call sites.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	chatCfg, err := m.chatConfigForGenerate()
	if err != nil {
		return nil, err
	}
	// eino's ChatModelAgent binds tools via the per-call model.WithTools option
	// (not by calling WithTools). Merge those into the config so the model
	// actually emits tool_calls; otherwise the ReAct loop would have no tools.
	chatCfg, err = m.chatConfigWithOptsTools(chatCfg, opts)
	if err != nil {
		return nil, err
	}
	// Once a tool result is in context the model must be free to answer in
	// prose. Code-exec agents pin tool_choice to "required"/execute_code, and
	// keeping that pin on the follow-up turn would loop forever instead of
	// returning the answer.
	if chatCfg != nil && containsToolResult(internal) {
		choice := "auto"
		chatCfg.ToolChoice = &choice
		chatCfg.ToolChoiceValue = nil
	}
	common.Debug("models: eino generate request",
		zap.String("model", *cm.ModelName),
		zap.Int("messages", len(internal)),
		zap.Int("tools", toolCount(chatCfg)),
	)
	common.Debug("models: eino generate message skeleton",
		zap.String("skeleton", describeInternalMessages(internal)))
	resp, err := cm.ModelDriver.ChatWithMessages(ctx, *cm.ModelName, internal, cm.APIConfig, chatCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("models: EinoChatModel.Generate(%s): %w", *cm.ModelName, err)
	}
	// Record the per-call token usage so the canvas-level aggregator (and
	// Langfuse) can compute the run total. Mirrors Python's
	// LLMBundle._report_usage() / self.mdl.last_usage pattern.
	if resp != nil && resp.Usage != nil {
		// The run sink gets this call's own split. The canvas component gets the same
		// numbers from the message it is handed (ResponseMeta.Usage, set in
		// fromInternalResponse): nothing about one call's usage lives on the shared
		// ChatModel, where a concurrent call could replace it between write and read.
		recordUsage(ctx, *cm.ModelName, &TokenUsage{
			PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens, TotalTokens: resp.Usage.TotalTokens,
		})
	}
	// Guard the debug log against a nil resp: some drivers may return (nil, nil)
	// on an aborted/empty completion, and len(resp.ToolCalls) would panic.
	toolCalls := 0
	if resp != nil {
		toolCalls = len(resp.ToolCalls)
	}
	// METADATA only: the answer body is user-visible (and often retrieved private)
	// content, so a debug line must not copy it into the log file (CWE-532).
	// Length + shape still separate an empty completion from a truncated one.
	common.Debug("models: eino generate response",
		zap.String("model", *cm.ModelName),
		zap.Int("answer_bytes", len(answerHead(resp))),
		zap.Int("tool_calls", toolCalls),
		zap.Int("completion_tokens", usageCompletion(resp)),
	)
	return fromInternalResponse(resp), nil
}

// modelNameOf returns the model's display name (best-effort; nil-safe).
func modelNameOf(cm *ChatModel) string {
	if cm == nil || cm.ModelName == nil {
		return "(nil)"
	}
	return *cm.ModelName
}

// answerHead returns a short preview of the response answer for log lines.
func answerHead(resp *ChatResponse) string {
	if resp == nil || resp.Answer == nil {
		return ""
	}
	return *resp.Answer
}

func usageCompletion(resp *ChatResponse) int {
	if resp == nil || resp.Usage == nil {
		return 0
	}
	return resp.Usage.CompletionTokens
}

// toolCount returns the number of tools on a config, tolerating the
// interface{} storage type.
func toolCount(cfg *ChatConfig) int {
	if cfg == nil {
		return 0
	}
	switch t := cfg.Tools.(type) {
	case []map[string]any:
		return len(t)
	case nil:
		return 0
	default:
		return 0
	}
}

func (m *EinoChatModel) chatConfigForGenerate() (*ChatConfig, error) {
	// Always hand back a COPY. Both callers (generateOnce, Stream) release the
	// tool_choice on the config when the turn carries a tool result, and m.chatCfg
	// is the shared base every WithTools/WithToolChoice instance carries: writing
	// through it would persist one turn's "auto" into later turns (losing a
	// required choice) and race concurrent calls on the same fields.
	cfg := &ChatConfig{}
	if m.chatCfg != nil {
		cp := *m.chatCfg
		cfg = &cp
	}
	if len(m.tools) == 0 {
		return cfg, nil
	}
	tools, err := openAIToolsFromEino(m.tools)
	if err != nil {
		return nil, err
	}
	cfg.Tools = tools
	choice := "auto"
	for _, tool := range m.tools {
		if tool != nil && tool.Name == "execute_code" {
			// MiniMax may answer with prose instead of emitting the callable
			// CodeExec request. Require one tool dispatch for code-exec agents;
			// the subsequent ReAct turn remains free to produce the final text.
			choice = "required"
			break
		}
	}
	cfg.ToolChoice = &choice
	for _, tool := range m.tools {
		if tool != nil && tool.Name == "execute_code" {
			cfg.ToolChoiceValue = map[string]any{
				"type":     "function",
				"function": map[string]any{"name": "execute_code"},
			}
			break
		}
	}
	// An explicit WithToolChoice overrides both defaults above; the setter
	// documents that the choice reaches the driver through this config. A
	// keyword choice leaves ToolChoiceValue as an untyped nil: the driver only
	// falls back to the plain string when the field is nil (base_model.go:565),
	// and a typed-nil map would be sent as `"tool_choice": null`.
	if m.toolChoice != nil && *m.toolChoice != "" {
		cfg.ToolChoice = m.toolChoice
		cfg.ToolChoiceValue = nil
		if body := toolChoiceBody(*m.toolChoice); body != nil {
			cfg.ToolChoiceValue = body
		}
	}
	return cfg, nil
}

// toolChoiceBody returns the OpenAI tool_choice OBJECT form for a choice naming
// a specific tool, and nil for the keyword forms ("auto"/"none"/"required"),
// which travel as the plain ToolChoice string (base_model.go:561-568 prefers
// this value whenever it is set).
func toolChoiceBody(choice string) map[string]any {
	switch choice {
	case "auto", "none", "required":
		return nil
	}
	return map[string]any{
		"type":     "function",
		"function": map[string]any{"name": choice},
	}
}

// chatConfigWithOptsTools overlays tools supplied via the per-call
// model.WithTools option onto base. eino's ChatModelAgent binds tools this way,
// so Generate/Stream must honor opts or the model will never emit tool_calls.
// When opts carries no tools, base is returned unchanged.
func (m *EinoChatModel) chatConfigWithOptsTools(base *ChatConfig, opts []model.Option) (*ChatConfig, error) {
	co := model.GetCommonOptions(nil, opts...)
	if co == nil || len(co.Tools) == 0 {
		return base, nil
	}
	tools, err := openAIToolsFromEino(co.Tools)
	if err != nil {
		return nil, err
	}
	cfg := &ChatConfig{}
	if base != nil {
		cp := *base
		cfg = &cp
	}
	cfg.Tools = tools
	choice := "auto"
	cfg.ToolChoice = &choice
	return cfg, nil
}

func openAIToolsFromEino(infos []*schema.ToolInfo) ([]map[string]any, error) {
	tools := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		if info == nil {
			continue
		}
		fn := map[string]any{
			"name":        info.Name,
			"description": info.Desc,
		}
		if info.ParamsOneOf != nil {
			params, err := info.ParamsOneOf.ToJSONSchema()
			if err != nil {
				return nil, fmt.Errorf("models: convert tool %q schema: %w", info.Name, err)
			}
			fn["parameters"] = params
		} else {
			fn["parameters"] = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, map[string]any{
			"type":     "function",
			"function": fn,
		})
	}
	return tools, nil
}

// describeInternalMessages renders the message sequence's tool-call skeleton —
// "sys|user|asst[tc=ID1,ID2]|tool(ID1)|tool(ID2)|asst|user" — the exact shape a
// provider's tool-id validator sees, so a rejection like MiniMax's
// `tool result's tool id(X) not found` can be matched against the replayed
// sequence directly from the log instead of being reproduced blind.
func describeInternalMessages(msgs []Message) string {
	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte('|')
		}
		switch m.Role {
		case "assistant":
			b.WriteString("asst")
			if len(m.ToolCalls) > 0 {
				ids := make([]string, 0, len(m.ToolCalls))
				for _, tc := range m.ToolCalls {
					id, _ := tc["id"].(string)
					ids = append(ids, id)
				}
				b.WriteString(fmt.Sprintf("[tc=%s]", strings.Join(ids, ",")))
			}
		case "tool":
			b.WriteString("tool(" + m.ToolCallID + ")")
		default:
			b.WriteString(m.Role)
		}
	}
	return b.String()
}

func toolCallsToInternal(calls []schema.ToolCall) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(calls))
	for _, call := range calls {
		fn := map[string]interface{}{
			"name":      call.Function.Name,
			"arguments": call.Function.Arguments,
		}
		out = append(out, map[string]interface{}{
			"id":       call.ID,
			"type":     call.Type,
			"function": fn,
		})
	}
	return out
}

func toolCallsFromInternal(calls []map[string]interface{}) []schema.ToolCall {
	out := make([]schema.ToolCall, 0, len(calls))
	for i, call := range calls {
		id, _ := call["id"].(string)
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		callType, _ := call["type"].(string)
		if callType == "" {
			callType = "function"
		}
		var fnName, fnArgs string
		if fn, ok := call["function"].(map[string]interface{}); ok {
			fnName, _ = fn["name"].(string)
			switch args := fn["arguments"].(type) {
			case string:
				fnArgs = args
			case nil:
				fnArgs = "{}"
			default:
				b, err := json.Marshal(args)
				if err == nil {
					fnArgs = string(b)
				}
			}
		}
		// Index MUST be set: stream consumers merge tool-call chunks by Index
		// (nil reads as 0), and this message reaches them as one complete
		// chunk carrying EVERY parallel call. With nil indexes the whole batch
		// collapses into a single call — the last ID wins, the others' results
		// are orphaned, and a provider replay (gate repair turn) rejects the
		// sequence with `tool result's tool id(X) not found`.
		idx := i
		out = append(out, schema.ToolCall{
			ID:    id,
			Type:  callType,
			Index: &idx,
			Function: schema.FunctionCall{
				Name:      fnName,
				Arguments: fnArgs,
			},
		})
	}
	return out
}

// containsToolResult reports whether any message already carries a tool
// result. ReAct turns that contain results must keep tool_choice free: the
// model is expected to answer, not to fire another tool.
func containsToolResult(messages []Message) bool {
	for _, message := range messages {
		if message.Role == "tool" {
			return true
		}
	}
	return false
}

// Stream returns a schema.StreamReader that yields message chunks
// incrementally. Uses the existing ChatStreamlyWithSender pathway; the
// sender callback pushes the streamed delta into the StreamReader.
// With a failover chain, a stream that fails BEFORE any delta was emitted
// moves to the next model; once deltas have reached the client the call
// cannot be replayed, so the error is surfaced as-is.
func (m *EinoChatModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if m == nil || len(m.chain) == 0 {
		return nil, fmt.Errorf("models: EinoChatModel: empty model chain")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	internalMessage := toInternalMessages(msgs)
	// Some OpenAI-compatible providers (including the configured MiniMax
	// endpoint) stream tool intent as ordinary prose. Use the provider's
	// non-streaming parser for tool-bound turns so structured tool_calls are
	// preserved; ReAct still streams the final answer turn normally.
	if len(m.tools) > 0 && !containsToolResult(internalMessage) {
		msg, err := m.Generate(ctx, msgs, opts...)
		if err != nil {
			return nil, err
		}
		sr, sw := schema.Pipe[*schema.Message](1)
		if !sw.Send(msg, nil) {
			sw.Close()
			return sr, nil
		}
		sw.Close()
		return sr, nil
	}
	chatCfg, err := m.chatConfigForGenerate()
	if err != nil {
		return nil, err
	}
	// Same tool-binding fix as Generate: honor the per-call model.WithTools
	// option so the ReAct loop's model requests actually carry tool definitions.
	chatCfg, err = m.chatConfigWithOptsTools(chatCfg, opts)
	if err != nil {
		return nil, err
	}
	// Same tool_choice release as generateOnce: a turn carrying tool results
	// must be able to produce the final text instead of being forced to call
	// a tool again.
	if chatCfg != nil && containsToolResult(internalMessage) {
		choice := "auto"
		chatCfg.ToolChoice = &choice
		chatCfg.ToolChoiceValue = nil
	}
	common.Debug("models: eino stream request",
		zap.String("model", modelNameOf(m.chain[0])),
		zap.Int("messages", len(internalMessage)),
		zap.Int("tools", toolCount(chatCfg)),
		zap.Int("chain", len(m.chain)),
	)

	sr, sw := schema.Pipe[*schema.Message](1)
	var sendMu sync.Mutex
	var sentAny bool
	sender := func(content *string, reasoning *string) error {
		sendMu.Lock()
		defer sendMu.Unlock()
		if content == nil && reasoning == nil {
			return nil
		}
		// Provider drivers use the OpenAI-compatible [DONE] sentinel to
		// signal the end of their transport stream. It is not assistant
		// content and must not reach Eino's message stream or callback.
		if content != nil && *content == "[DONE]" {
			return nil
		}
		msg := &schema.Message{Role: schema.Assistant}
		if content != nil {
			msg.Content = *content
		}
		if reasoning != nil {
			msg.ReasoningContent = *reasoning
		}
		if closed := sw.Send(msg, nil); closed {
			return fmt.Errorf("models: stream closed before send completed")
		}
		sentAny = true
		return nil
	}
	go func() {
		defer sw.Close()
		m.mu.Lock()
		start := m.cursor
		if waited := time.Since(m.sweep.failedAt); m.sweep.err != nil && waited < failoverCooldown {
			err := m.sweep.err
			m.mu.Unlock()
			_ = sw.Send(nil, err)
			return
		}
		m.mu.Unlock()
		var lastErr error
		// Same rule as Generate: cache the sweep only when every failed attempt was
		// provider-wide, never on the strength of the last error alone.
		cacheable := true
		for i := 0; i < len(m.chain); i++ {
			idx := (start + i) % len(m.chain)
			cm := m.chain[idx]
			if cm == nil || cm.ModelDriver == nil {
				// generateOnce reports this entry as a failed attempt ("nil inner
				// ModelDriver"); mirror that here, and run it through the same
				// cacheable rule. Skipping it silently left lastErr nil, so a
				// chain of unusable entries reported success with no message.
				lastErr = fmt.Errorf("models: EinoChatModel: nil inner ModelDriver on chain entry %d", idx)
				cacheable = cacheable && cacheableSweepFailure(lastErr)
				continue
			}
			if cm.ModelName == nil {
				// generateOnce reports this entry as a failed attempt; the
				// streaming path used to dereference it, and a panic raised here
				// runs inside a goroutine, which kills the process. It is a chain
				// misconfiguration, not provider health, so it must not be cached
				// as a sweep-wide failure either.
				lastErr = fmt.Errorf("models: EinoChatModel: nil model name on chain entry %d", idx)
				cacheable = cacheable && cacheableSweepFailure(lastErr)
				continue
			}
			if err := ctx.Err(); err != nil {
				_ = sw.Send(nil, err)
				return
			}
			sendMu.Lock()
			sentBefore := sentAny
			sendMu.Unlock()
			// Attempt-local config: the driver writes ToolCallsResult/UsageResult
			// INTO the config it is handed, so reusing one instance would let a
			// failed attempt's leftovers be attributed to the model that actually
			// served the turn.
			attemptCfg := &ChatConfig{}
			if chatCfg != nil {
				cp := *chatCfg
				attemptCfg = &cp
			}
			attemptCfg.ToolCallsResult = nil
			attemptCfg.UsageResult = nil
			err := cm.ModelDriver.ChatStreamlyWithSender(ctx, *cm.ModelName, internalMessage, cm.APIConfig, attemptCfg, nil, sender)
			if err == nil {
				// Streamed turns report their token usage through the config
				// (stream_options.include_usage), not through the nil modelUsage
				// argument, so the run-level accumulator has to be fed from here —
				// otherwise every streamed turn's tokens are missing from the total.
				if attemptCfg.UsageResult != nil && attemptCfg.UsageResult.TotalTokens > 0 {
					recordUsage(ctx, *cm.ModelName, attemptCfg.UsageResult)
				}
				if attemptCfg.ToolCallsResult != nil && len(*attemptCfg.ToolCallsResult) > 0 {
					common.Debug("models: eino stream tool calls",
						zap.String("model", *cm.ModelName),
						zap.Int("tool_calls", len(*attemptCfg.ToolCallsResult)))
					msg := &schema.Message{
						Role:      schema.Assistant,
						ToolCalls: toolCallsFromInternal(*attemptCfg.ToolCallsResult),
					}
					_ = sw.Send(msg, nil)
				}
				m.mu.Lock()
				m.cursor = idx
				m.sweep.failedAt = time.Time{}
				m.sweep.err = nil
				m.mu.Unlock()
				return
			}
			lastErr = err
			cacheable = cacheable && cacheableSweepFailure(err)
			sendMu.Lock()
			sentAfter := sentAny
			sendMu.Unlock()
			if sentAfter != sentBefore || sentAfter {
				// Deltas already reached the client: the stream cannot be
				// replayed on another model, so fail as-is.
				_ = sw.Send(nil, err)
				return
			}
			next := (idx + 1) % len(m.chain)
			common.WarnCtx(ctx, "models: eino stream failed before first delta, failing over to next model",
				zap.String("failed_model", m.labelOf(idx)),
				zap.String("next_model", m.labelOf(next)),
				zap.Error(err))
		}
		// Terminal: sweep exhausted. Rotate, and cache only when EVERY attempt was a
		// provider-wide failure - mirroring Generate.
		cached := lastErr != nil && cacheable
		m.mu.Lock()
		m.cursor = (start + 1) % len(m.chain)
		if cached {
			m.sweep.failedAt = time.Now()
			m.sweep.err = lastErr
		}
		m.mu.Unlock()
		common.Debug("models: eino stream response error",
			zap.String("model", modelNameOf(m.chain[0])), zap.Bool("cached", cached), zap.Error(lastErr))
		_ = sw.Send(nil, lastErr)
	}()
	return sr, nil
}

// WithTools returns a NEW EinoChatModel instance with the given tools
// attached. The receiver is never mutated — this satisfies eino's
// ToolCallingChatModel contract and is safe under concurrent use.
//
// P0 caveat: the existing RAGFlow provider drivers do not natively consume
// eino's *schema.ToolInfo; the tools are stored on the wrapper for
// future use (Phase 2.5 will plumb them into the driver call). For now
// returning them in the streamed / generated content is a no-op on the
// wire — agents that depend on tool calling will surface this gap during
// Phase 3 ReAct integration.
func (m *EinoChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if m == nil {
		return nil, fmt.Errorf("models: EinoChatModel.WithTools: nil receiver")
	}
	m.mu.Lock()
	cursor := m.cursor
	m.mu.Unlock()
	// Field-by-field copy: EinoChatModel contains a sync.Mutex, which must
	// never be copied (go vet copylocks).
	cp := EinoChatModel{
		chain:      m.chain,
		labels:     m.labels,
		chatCfg:    m.chatCfg,
		toolChoice: m.toolChoice,
		cursor:     cursor,
	}
	cp.tools = append([]*schema.ToolInfo(nil), tools...)
	return &cp, nil
}

// WithToolChoice returns a NEW EinoChatModel instance constrained to the given
// tool_choice string ("auto", "none", "required", or a specific tool name).
// Empty string leaves the default ("auto"). Mirrors eino's WithToolChoice but
// operates on the RAGFlow wrapper so the choice reaches the driver's request
// body via chatConfigForGenerate.
func (m *EinoChatModel) WithToolChoice(choice string) *EinoChatModel {
	m.mu.Lock()
	cursor := m.cursor
	m.mu.Unlock()
	// Field-by-field copy for the same reason as WithTools: cp := *m would copy
	// the sync.Mutex (go vet copylocks) and could hand back a model whose mutex
	// is permanently locked, deadlocking its next Generate/Stream/WithTools.
	cp := EinoChatModel{
		chain:   m.chain,
		labels:  m.labels,
		chatCfg: m.chatCfg,
		tools:   append([]*schema.ToolInfo(nil), m.tools...),
		cursor:  cursor,
	}
	if choice == "" {
		cp.toolChoice = nil
		return &cp
	}
	cp.toolChoice = &choice
	return &cp
}

// Tools returns the tools currently bound to the wrapper (used by
// introspection; not part of any eino interface).
func (m *EinoChatModel) Tools() []*schema.ToolInfo {
	if m == nil {
		return nil
	}
	return append([]*schema.ToolInfo(nil), m.tools...)
}

// Inner exposes the primary wrapped *ChatModel for callers that need direct
// access (e.g. to read token usage from the response after a custom
// Generate call). Not part of any eino interface.
func (m *EinoChatModel) Inner() *ChatModel {
	if m == nil || len(m.chain) == 0 {
		return nil
	}
	return m.chain[0]
}

// Name returns the wrapped model name (used by tools / debugging).
func (m *EinoChatModel) Name() string {
	return m.name()
}
