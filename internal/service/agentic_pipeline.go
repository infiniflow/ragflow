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

// defaultAgenticTemplateID is the agent a reasoning-level-selected turn runs.
// It must name a template in conf/agentic_rag.yaml; resolveTemplateFor fails
// the turn loudly if it does not, which is the intended behaviour for an
// operator who removed or renamed the template.
const defaultAgenticTemplateID = "smart-reasoning"

// agenticRag drives ONE conversation turn through the smart-reasoning agent
// (internal/agentic_rag): an eino-ADK ReAct explorer whose toolset is the
// corpus itself (grep / lexical / semantic locate, list_chunks deep read).
// Dispatched from AsyncChat before the retrieval phases when the request names
// kwargs["agent_mode"] or asks for reasoning level reasoningLevelAgentic - the
// agent runs its own retrieval, so the pipeline's search and generation phases
// do not apply.
//
// The streamed shape matches the naive pipeline's: thinking deltas framed by
// StartToThink/EndToThink, and exactly one Final result carrying the answer
// with its reference payload built from the answer's own chunk_id citations
// (see buildAgenticReference).
func (s *ChatPipelineService) agenticRag(
	ctx context.Context,
	userID string,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
	kwargs map[string]interface{},
	useWebSearch bool,
	quote bool,
) (<-chan AsyncChatResult, error) {
	out := make(chan AsyncChatResult, 16)
	// agent_mode selects the template id for this run (validated non-empty by
	// AsyncChat before dispatch). Resolved per-run so conf/agentic_rag.yaml
	// edits take effect without restart.
	//
	// A turn selected by reasoning level reasoningLevelAgentic names no
	// template — the level is a retrieval strategy, not a config id — so it
	// lands on the one agent this build ships. An explicit agent_mode still
	// wins and is still validated strictly, so a bad id from a direct API
	// caller fails loudly rather than silently running a different agent.
	//
	// AsyncChat only dispatches here with a non-empty KB scope, so the
	// citation lookup below is always kb_id-bounded.
	mode, _ := kwargs["agent_mode"].(string)
	if mode == "" {
		mode = defaultAgenticTemplateID
	}

	go func() {
		defer close(out)

		runCtx, cancel := context.WithTimeout(ctx, agenticRunTimeout)
		defer cancel()
		emitResult := func(result AsyncChatResult) bool {
			select {
			case out <- result:
				return true
			case <-runCtx.Done():
				return false
			}
		}

		chain, chainErr := s.agenticModelChain(runCtx, userID, chat)
		if chainErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: resolve chat model", chainErr)
			emitResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", chainErr.Error()), Final: true})
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
		// Keep agent and synthesis on the ONE resolved model set, so a failover
		// that moved the chain's cursor mid-turn cannot leave the researcher
		// and the summariser on different providers.
		//
		// A single-member chain is wrapped by the same failover constructor: it
		// is the identity case (one entry, nothing to fail over to) and keeps
		// one construction path instead of branching on len(chain).
		model, chainErr := modelModule.NewFailoverEinoChatModel(chain, chatCfg)
		if chainErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: build chat model", chainErr)
			emitResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", chainErr.Error()), Final: true})
			return
		}
		synth, synthErr := modelModule.NewFailoverEinoChatModel(chain, chatCfg)
		if synthErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: build synthesis model", synthErr)
			emitResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", synthErr.Error()), Final: true})
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
			DatasetIDs:     chatDatasetIDs(chat),
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
					emitResult(AsyncChatResult{Final: false, StartToThink: true})
				}
				if contentDelta != "" || thinkingDelta != "" {
					emitResult(AsyncChatResult{
						Answer:    contentDelta,
						Reasoning: thinkingDelta,
						Final:     false,
					})
				}
				if endToThink {
					emitResult(AsyncChatResult{Final: false, EndToThink: true})
				}
			},
		})
		if thinking {
			emitResult(AsyncChatResult{Final: false, EndToThink: true})
		}
		if runErr != nil {
			common.ErrorCtx(runCtx, "agentic_rag: run", runErr)
			emitResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", runErr.Error()), Final: true})
			return
		}
		common.InfoCtx(ctx, "agentic turn done",
			zap.String("mode", mode),
			zap.Any("tool_calls", toolCounts),
			zap.Any("tool_errors", toolErrors),
			zap.Int("final_bytes", len(final)))

		// Citations off means neither the [ID:N] markers nor the reference
		// payload, matching what the regular pipeline ships when quote is
		// false. The answer text is still whatever the agent wrote.
		answer := final
		reference := map[string]interface{}{}
		if quote {
			reference, answer = s.buildAgenticReference(ctx, chat.TenantID, chatDatasetIDs(chat), final)
		} else if len(agentic_rag.ExtractCitedChunkIDs(final)) > 0 {
			common.InfoCtx(ctx, "agentic citations suppressed by quote=false")
		}
		emitResult(AsyncChatResult{Answer: answer, Reference: reference, Final: true})
	}()

	return out, nil
}

// agenticModelChain resolves only the model explicitly selected by the dialog
// (or the tenant default when no model is selected), plus any failover members
// the dialog configures. Agentic RAG must not silently broaden the dialog's
// model choice to every chat model owned by the tenant: a failover chain is
// exactly the list the dialog's author chose, and nothing else.
func (s *ChatPipelineService) agenticModelChain(ctx context.Context, userID string, chat *entity.Chat) ([]*modelModule.ChatModel, error) {
	access := ModelAccess{UserID: userID, TenantID: chat.TenantID}
	primary, err := s.agenticPrimaryModel(ctx, access, chat)
	if err != nil {
		return nil, err
	}

	chain := []*modelModule.ChatModel{primary}
	// Failover members are best-effort per entry: a member that no longer
	// resolves (deleted, deactivated, or re-typed) must not take the whole turn
	// down when a working primary exists. Skipping it keeps the surviving
	// members usable instead of collapsing to a hard error.
	for _, llmID := range agenticFailoverModelIDs(chat) {
		if llmID == "" || llmID == chat.LLMID {
			continue
		}
		model, resolveErr := s.ModelFactory.NewChatModel(ctx, access, llmID)
		if resolveErr != nil || model == nil {
			common.WarnCtx(ctx, "agentic_rag: skipping unresolvable failover model",
				zap.String("llm_id", llmID), zap.Error(resolveErr))
			continue
		}
		chain = append(chain, model)
	}
	common.InfoCtx(ctx, "agentic model chain resolved",
		zap.Int("chain", len(chain)), zap.String("primary", chat.LLMID))
	return chain, nil
}

// agenticPrimaryModel resolves the dialog's own model, or the tenant default
// when it selected none.
func (s *ChatPipelineService) agenticPrimaryModel(ctx context.Context, access ModelAccess, chat *entity.Chat) (*modelModule.ChatModel, error) {
	var (
		model *modelModule.ChatModel
		err   error
	)
	if chat.LLMID == "" {
		model, err = s.ModelFactory.NewDefaultChatModel(ctx, access)
	} else {
		model, err = s.ModelFactory.NewChatModel(ctx, access, chat.LLMID)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve chat model: %w", err)
	}
	if model == nil {
		return nil, fmt.Errorf("resolve chat model: no chat model resolved for tenant %s", chat.TenantID)
	}
	return model, nil
}

// agenticFailoverModelIDs reads the dialog's ordered failover list from
// llm_setting. Order is the author's priority: EinoChatModel walks the chain in
// order and the sticky cursor keeps a healthy member in front, so a member that
// failed once is not re-probed on every call of a long turn.
//
// The list is per conversation and lives in llm_setting rather than a tenant
// group table, so it needs no cross-entity join and cannot outlive its dialog.
func agenticFailoverModelIDs(chat *entity.Chat) []string {
	if chat == nil || chat.LLMSetting == nil {
		return nil
	}
	raw, ok := chat.LLMSetting["failover_llm_ids"]
	if !ok {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if id, ok := item.(string); ok && id != "" {
			out = append(out, id)
		}
	}
	return out
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
func chatDatasetIDs(chat *entity.Chat) []string {
	if chat == nil {
		return nil
	}
	ids := make([]string, 0, len(chat.KBIDs))
	for _, raw := range chat.KBIDs {
		if id, ok := raw.(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (s *ChatPipelineService) buildAgenticReference(ctx context.Context, tenantID string, datasetIDs []string, final string) (map[string]interface{}, string) {
	cited := agentic_rag.ExtractCitedChunkIDs(final)
	if len(cited) == 0 {
		return map[string]interface{}{}, final
	}
	rows := fetchChunksByIDs(ctx, tenantID, datasetIDs, cited)
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
func fetchChunksByIDs(ctx context.Context, tenantID string, datasetIDs, ids []string) []map[string]interface{} {
	de := engine.Get()
	if de == nil || len(ids) == 0 {
		return nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:   []string{fmt.Sprintf("ragflow_%s", tenantID)},
		KbIDs:        datasetIDs,
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
