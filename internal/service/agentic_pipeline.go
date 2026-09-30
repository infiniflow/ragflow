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

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

// agenticRunTimeout is the wall-clock budget of one agentic turn. The ReAct
// loop is bounded by MaxIterations on its own, but a provider that answers
// every call just slowly enough can otherwise hold a request open indefinitely.
const agenticRunTimeout = 15 * time.Minute

// agenticRag drives ONE conversation turn through the smart-reasoning agent
// (internal/agentic_rag): an eino-ADK ReAct explorer whose toolset is the
// corpus itself (grep / lexical / semantic locate, list_chunks deep read).
// Dispatched from AsyncChat on kwargs["agent_mode"] before the retrieval
// phases - the agent runs its own retrieval, so the pipeline's search and
// generation phases do not apply.
//
// The streamed shape matches the naive pipeline's: thinking deltas framed by
// StartToThink/EndToThink, and exactly one Final result carrying the answer
// with its reference payload built from the answer's own chunk_id citations
// (see buildAgenticReference).
func (s *ChatPipelineService) agenticRag(
	ctx context.Context,
	_ string,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
	kwargs map[string]interface{},
	useWebSearch bool,
) (<-chan AsyncChatResult, error) {
	out := make(chan AsyncChatResult, 16)
	// agent_mode selects the template id for this run (validated non-empty by
	// AsyncChat before dispatch). Resolved per-run so conf/agentic_rag.yaml
	// edits take effect without restart.
	mode, _ := kwargs["agent_mode"].(string)

	go func() {
		defer close(out)

		runCtx, cancel := context.WithTimeout(ctx, agenticRunTimeout)
		defer cancel()

		chain, labels, chainErr := s.agenticModelChain(runCtx, chat)
		if chainErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: resolve chat model", chainErr)
			out <- AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", chainErr.Error()), Final: true}
			return
		}

		// Apply the dialog's LLM settings with per-request overrides exactly
		// like the regular AsyncChat path does, then let the template pin its
		// sampling when it declares one (smart-reasoning pins 0.5).
		chatCfg := BuildChatConfig(chat, kwargs)
		if temp := agentic_rag.TemplateTemperature(mode); temp != nil {
			pinned := *chatCfg
			pinned.Temperature = temp
			chatCfg = &pinned
		}
		model, modelErr := modelModule.NewFailoverEinoChatModelWithLabels(chain, labels, chatCfg)
		if modelErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: build model", modelErr)
			out <- AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", modelErr.Error()), Final: true}
			return
		}
		// The last-resort synthesis runs on its OWN instance over the same
		// chain: a failover instance caches the error of its last full-chain
		// failure and short-circuits every later call with it for 30s, so
		// sharing the agent's instance hands the synthesis a stale error from
		// whatever malformed call tripped the cooldown.
		synth, synthErr := modelModule.NewFailoverEinoChatModelWithLabels(chain, labels, chatCfg)
		if synthErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: build synthesis model", synthErr)
			out <- AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", synthErr.Error()), Final: true}
			return
		}

		if useWebSearch {
			if web := s.agenticWebSearch(chat.PromptConfig); web != nil {
				runCtx = agentic_rag.WithWebSearch(runCtx, web)
			}
		}

		// Per-turn tool accounting, logged (not shipped on the result: the
		// channel contract here is the naive pipeline's answer/reference one).
		toolCounts := map[string]int{}
		toolErrors := map[string]int{}

		// thinking tracks whether we are inside the think block so the
		// StartToThink marker is emitted once and EndToThink fires when the
		// agent's final answer arrives.
		thinking := false
		final, runErr := agentic_rag.Run(runCtx, agentic_rag.Input{
			Model:          model,
			SynthModel:     synth,
			Messages:       convertMessagesToEino(messages),
			TemplateID:     mode,
			TenantID:       chat.TenantID,
			Stream:         stream,
			ToolCallCounts: toolCounts,
			ToolCallErrors: toolErrors,
			OnDelta: func(contentDelta, thinkingDelta string) {
				startToThink, endToThink := false, false
				if thinkingDelta != "" {
					if !thinking {
						startToThink = true
						thinking = true
					}
				} else if thinking {
					endToThink = true
					thinking = false
				}
				// Markers travel on their own chunks: the frontend appends
				// '<think>' / '</think>' AFTER the chunk's answer text, so a
				// marker riding on a content chunk would strand that text on
				// the wrong side of the think section.
				if startToThink {
					out <- AsyncChatResult{Final: false, StartToThink: true}
				}
				if contentDelta != "" || thinkingDelta != "" {
					out <- AsyncChatResult{
						Answer:    contentDelta,
						Reasoning: thinkingDelta,
						Final:     false,
					}
				}
				if endToThink {
					out <- AsyncChatResult{Final: false, EndToThink: true}
				}
			},
		})
		if thinking {
			out <- AsyncChatResult{Final: false, EndToThink: true}
		}
		if runErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: run", runErr)
			out <- AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", runErr.Error()), Final: true}
			return
		}
		common.InfoCtx(ctx, "agentic turn done",
			zap.String("mode", mode),
			zap.Any("tool_calls", toolCounts),
			zap.Any("tool_errors", toolErrors),
			zap.Int("final_bytes", len(final)))

		reference, marked := s.buildAgenticReference(ctx, chat.TenantID, final)
		out <- AsyncChatResult{Answer: marked, Reference: reference, Final: true}
	}()

	return out, nil
}

// agenticModelChain resolves the dialog's chat model as the primary and every
// OTHER chat-capable model the tenant owns as a fallback: when the primary
// dies (quota wall, outage, rate limit) the run fails over instead of burning.
func (s *ChatPipelineService) agenticModelChain(ctx context.Context, chat *entity.Chat) ([]*modelModule.ChatModel, []string, error) {
	var (
		target *ModelTarget
		err    error
	)
	if chat.LLMID == "" {
		target, err = s.ModelProviderSvc.modelSolver().ResolveDefaultModelConfig(ctx, chat.TenantID, entity.ModelTypeChat)
	} else {
		target, err = s.ModelProviderSvc.modelSolver().ResolveModelConfig(ctx, chat.TenantID, entity.ModelTypeChat, chat.LLMID)
	}
	if err != nil || target == nil {
		if err == nil {
			err = fmt.Errorf("no chat model resolved for tenant %s", chat.TenantID)
		}
		return nil, nil, fmt.Errorf("resolve chat model: %w", err)
	}

	chain := []*modelModule.ChatModel{modelModule.NewChatModel(target.Driver, &target.ModelName, target.APIConfig)}
	labels := []string{fmt.Sprintf("%s @ %s", target.ModelName, target.InstanceName)}
	refs, refErr := s.ModelProviderSvc.ListTenantChatModelRefs(ctx, chat.TenantID)
	if refErr != nil {
		// A resolvable primary with no roster is enough to run.
		return chain, labels, nil
	}
	for _, ref := range refs {
		if ref.Ref == chat.LLMID {
			labels[0] = fmt.Sprintf("%s @ %s [primary]", ref.ModelName, ref.InstanceName)
			continue
		}
		fallback, fbErr := s.ModelProviderSvc.modelSolver().ResolveModelConfig(ctx, chat.TenantID, entity.ModelTypeChat, ref.Ref)
		if fbErr != nil || fallback == nil {
			continue // a single broken fallback is non-fatal
		}
		chain = append(chain, modelModule.NewChatModel(fallback.Driver, &fallback.ModelName, fallback.APIConfig))
		labels = append(labels, fmt.Sprintf("%s @ %s", ref.ModelName, ref.InstanceName))
	}
	return chain, labels, nil
}

// agenticWebSearch adapts the pipeline's configured provider to the agent's
// per-query web_search tool. nil hides the tool entirely (the same rule the
// harness path applies).
func (s *ChatPipelineService) agenticWebSearch(promptConfig map[string]interface{}) agentic_rag.WebSearchFunc {
	provider := resolveWebSearchProvider(promptConfig)
	if provider == nil {
		return nil
	}
	return func(ctx context.Context, query string) ([]agentic_rag.WebResult, error) {
		res, err := s.retrieveWebSearch(ctx, provider, query)
		if err != nil {
			return nil, err
		}
		chunks, _ := res["chunks"].([]map[string]interface{})
		out := make([]agentic_rag.WebResult, 0, len(chunks))
		for _, c := range chunks {
			out = append(out, agentic_rag.WebResult{
				Title:   stringValue(c["docnm_kwd"]),
				URL:     stringValue(c["url"]),
				Content: stringValue(c["content_with_weight"]),
			})
		}
		return out, nil
	}
}

// convertMessagesToEino converts pre-filtered user/assistant messages into
// eino schema messages. Only string content is supported; multimodal parts are
// not carried into the ReAct loop.
func convertMessagesToEino(messages []map[string]interface{}) []*schema.Message {
	out := make([]*schema.Message, 0, len(messages))
	for _, m := range messages {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		switch role {
		case "user":
			out = append(out, schema.UserMessage(content))
		case "assistant":
			out = append(out, schema.AssistantMessage(content, nil))
		default:
			// system messages are stripped upstream; skip anything else.
			continue
		}
	}
	return out
}

// buildAgenticReference turns the answer's own chunk_id citations into the
// reference payload the naive pipeline ships, and rewrites the answer with
// [ID:N] markers. With no resolvable citations both come back unchanged: an
// empty reference keeps the SSE shape the UI expects, and unmarked chunk_id
// text is the honest state of an answer whose sources could not be loaded.
func (s *ChatPipelineService) buildAgenticReference(ctx context.Context, tenantID, final string) (map[string]interface{}, string) {
	cited := agentic_rag.ExtractCitedChunkIDs(final)
	if len(cited) == 0 {
		return map[string]interface{}{}, final
	}
	rows := fetchChunksByIDs(ctx, tenantID, cited)
	if len(rows) == 0 {
		return map[string]interface{}{}, final
	}
	// Keep only ids that actually resolved, preserving first-appearance
	// order: the marker number IS the chunk's position in the payload array.
	byID := make(map[string]map[string]interface{}, len(rows))
	for _, r := range rows {
		if id, ok := r["id"].(string); ok {
			byID[id] = r
		}
	}
	resolved := make([]string, 0, len(cited))
	for _, id := range cited {
		if _, ok := byID[id]; ok {
			resolved = append(resolved, id)
		}
	}
	if len(resolved) == 0 {
		return map[string]interface{}{}, final
	}
	ordered := make([]map[string]interface{}, 0, len(resolved))
	for _, id := range resolved {
		ordered = append(ordered, byID[id])
	}
	marked := agentic_rag.InsertCitationMarkers(final, resolved)
	common.InfoCtx(ctx, "agentic citations built",
		zap.Int("cited", len(cited)),
		zap.Int("resolved", len(resolved)),
		zap.Int("final_bytes", len(marked)))
	return map[string]interface{}{
		"chunks":   chunksFormat(ordered),
		"doc_aggs": agenticDocAggs(ordered),
	}, marked
}

// fetchChunksByIDs loads the cited chunks from the tenant's index, keeping the
// engine row shape (chunk_id / content_with_weight / docnm_kwd / doc_id /
// kb_id / img_id / positions) so chunksFormat can normalize it. The index is
// the chat's OWNING tenant's - the same scoping the agent's retrieval tools
// use.
func fetchChunksByIDs(ctx context.Context, tenantID string, ids []string) []map[string]interface{} {
	de := engine.Get()
	if de == nil || len(ids) == 0 {
		return nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		Filter:       map[string]interface{}{"id": ids},
		SelectFields: []string{"content_with_weight", "docnm_kwd", "doc_id", "kb_id", "img_id", "positions"},
		Limit:        len(ids),
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		common.WarnCtx(ctx, "agentic citation chunk fetch failed", zap.Error(err))
		return nil
	}
	return res.Chunks
}

// agenticDocAggs aggregates the fetched chunks per document - the doc_aggs
// shape the UI's citation popover resolves documents from (doc_id + doc_name,
// plus a chunk count for parity with the naive payload).
func agenticDocAggs(chunks []map[string]interface{}) []interface{} {
	order := make([]string, 0, len(chunks))
	agg := make(map[string]map[string]interface{}, len(chunks))
	for _, ck := range chunks {
		docID, _ := ck["doc_id"].(string)
		if docID == "" {
			continue
		}
		if _, ok := agg[docID]; !ok {
			order = append(order, docID)
			agg[docID] = map[string]interface{}{
				"doc_id":   docID,
				"doc_name": ck["docnm_kwd"],
				"count":    0,
			}
		}
		if n, ok := agg[docID]["count"].(int); ok {
			agg[docID]["count"] = n + 1
		}
	}
	out := make([]interface{}, 0, len(order))
	for _, docID := range order {
		out = append(out, agg[docID])
	}
	return out
}
