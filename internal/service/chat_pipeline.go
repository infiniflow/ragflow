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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/graph"
	"ragflow/internal/service/nlp"
	"regexp"
	"sort"
	"strings"
	"time"

	"ragflow/internal/dao"

	"go.uber.org/zap"
)

// ChatPipelineService is the shared RAG chat pipeline engine used by both
// the OpenAI-compatible endpoint (/api/v1/openai/<chat_id>/chat/completions)
// and the regular chat completion endpoint (/api/v1/chat/completions).
//
// It owns the core retrieval → generation pipeline (AsyncChat, AsyncChatSolo)
// and all their supporting helpers. Callers (OpenAIChatService, ChatSessionService)
// compose it to avoid code duplication.
type ChatPipelineService struct {
	ModelProviderSvc *ModelProviderService
	MetadataSvc      *MetadataService
	datasetService   *DatasetService
}

// NewChatPipelineService creates a new ChatPipelineService with all required dependencies.
func NewChatPipelineService() *ChatPipelineService {
	return &ChatPipelineService{
		ModelProviderSvc: NewModelProviderService(),
		MetadataSvc:      NewMetadataService(),
		datasetService:   NewDatasetService(),
	}
}

// ---------------------------------------------------------------------------
// AsyncChatResult mirrors the dicts yielded by Python's async_chat /
// async_chat_solo. The handler translates these into OpenAIStreamEvent or
// builds a non-streaming OpenAICompletionResponse.
// ---------------------------------------------------------------------------

// AsyncChatResult is a single yield from the chat pipeline.
//
// Reasoning carries chain-of-thought text routed by the driver to a
// separate `reason` channel (e.g. OpenAI's `delta.reasoning_content`
// from Qwen / SiliconFlow). It is kept distinct from Answer so the
// SSE handler can map it to `delta.reasoning_content` rather than
// `delta.content`. Mirrors Python's _async_chat_streamly, which wraps
// reasoning_content in <think>…</think> markers (rag/llm/chat_model.py:226-232)
// so _stream_with_think_delta can route it via in_think state. In Go
// the driver already separates the two streams, so we surface them as
// separate fields directly instead of merging-then-re-splitting.
type AsyncChatResult struct {
	Answer       string                 `json:"answer"`
	Reasoning    string                 `json:"reasoning,omitempty"`
	Reference    map[string]interface{} `json:"reference"`
	AudioBinary  interface{}            `json:"audio_binary"`
	Prompt       string                 `json:"prompt"`
	CreatedAt    float64                `json:"created_at"`
	Final        bool                   `json:"final"`
	StartToThink bool                   `json:"start_to_think,omitempty"`
	EndToThink   bool                   `json:"end_to_think,omitempty"`
	// Internal-only: accumulated answer for building the decorated final result.
	accumulatedAnswer string
}

// AsyncChat is the Go equivalent of Python's async_chat() in
// api/db/services/dialog_service.py:541.
//
// Full pipeline:
//
//	┌───────────────────────────────────────────────────────┐
//	│ 1. Entry validation                                   │
//	│    messages non-empty, last role = "user"             │
//	├───────────────────────────────────────────────────────┤
//	│    No KBs & no web search → AsyncChatSolo (LLM-only)  │
//	├───────────────────────────────────────────────────────┤
//	│ 2. Resolve LLM model config + max_tokens              │
//	│ 3. Langfuse trace setup                               │
//	├───────────────────────────────────────────────────────┤
//	│ 4. Bind Models: getModels() → embd, rerank, chat, tts │
//	│    + BindTools (toolcall session)                     │
//	├───────────────────────────────────────────────────────┤
//	│ 5. Extract questions, attachments, image_files        │
//	├───────────────────────────────────────────────────────┤
//	│ 6. SQL Retrieval (field_map + chat_model)             │
//	│    HIT  → return structured SQL result directly       │
//	│    MISS → fall through to vector retrieval            │
//	├───────────────────────────────────────────────────────┤
//	│ 7. Prompt parameters: resolve param_keys, auto-fix    │
//	│    {knowledge} placeholder, validate kwargs           │
//	│ 8. Query refinement(LLM):                             │
//	│    refine_multiturn → cross_languages →               │
//	│    meta_data_filter → keyword extraction              │
//	├───────────────────────────────────────────────────────┤
//	│ 9. Retrieval (if hasKnowledgeParam):                  │
//	│                                                       │
//	│  reasoning=true?                                      │
//	│   YES → DeepResearcher (recursive, maxDepth=3)        │
//	│         each layer: KB → Web(Tavily) → KG(use_kg)     │
//	│         → sufficiencyCheck → multiQueriesGen → recurse│
//	│   NO  → Standard vector retrieval                     │
//	│         vector/hybrid search → rerank →               │
//	│         TOC enhance → child chunk retrieval →         │
//	│         Tavily web search → KG retrieval (prepend)    │
//	│                                                       │
//	│    enrichChunksWithMetadata (doc metadata)            │
//	│    kbPrompt (build knowledge blocks)                  │
//	├───────────────────────────────────────────────────────┤
//	│ 10. Build LLM request:                                │
//	│     empty_response check → formatPrompt →             │
//	│     citationPrompt(quote) → messageFitIn(95% budget)  │
//	│     → multimodal conversion → adjust max_tokens       │
//	├───────────────────────────────────────────────────────┤
//	│ 11. Drive LLM (stream / non-stream)                   │
//	│     + answer decoration (citations, references,       │
//	│       timing stats, Langfuse trace, TTS synthesis)    │
//	└───────────────────────────────────────────────────────┘
//
// Parameters:
//   - chat: the chat/chat entity with KBs, prompt_config, etc.
//   - messages: pre-filtered user/assistant messages (system already stripped).
//   - stream: if true, yields content deltas as they arrive.
//   - kwargs: extra parameters (doc_ids, knowledge, quote, etc.).
func (s *ChatPipelineService) AsyncChat(
	ctx context.Context,
	userID string,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
	kwargs map[string]interface{},
) (<-chan AsyncChatResult, error) {

	common.Info("AsyncChat started", zap.String("chat_id", chat.ID))

	// === Phase 1: Entry Validation ===
	// Guard: messages must be non-empty and the last role must be "user".
	common.Info("phase 1: Entry Validation")
	if len(messages) == 0 {
		return nil, fmt.Errorf("AsyncChat: messages is empty")
	}
	lastMsg := messages[len(messages)-1]
	if role, _ := lastMsg["role"].(string); role != "user" {
		return nil, fmt.Errorf("The last content of this conversation is not from user.")
	}

	// No KBs & no web search → fast-path to LLM-only chat.
	hasKBs := false
	for _, raw := range chat.KBIDs {
		if id, ok := raw.(string); ok && id != "" {
			hasKBs = true
			break
		}
	}
	useWebSearch := s.shouldUseWebSearch(chat, kwargs["internet"])
	if useWebSearch {
		common.Debug("web_search",
			zap.Bool("kb", hasKBs),
			zap.Bool("tavily", chat.PromptConfig != nil && chat.PromptConfig["tavily_api_key"] != "" && chat.PromptConfig["tavily_api_key"] != nil),
			zap.Any("internet", kwargs["internet"]),
			zap.Bool("enabled", useWebSearch))
	}

	if !hasKBs && !useWebSearch {
		return s.AsyncChatSolo(ctx, userID, chat, messages, stream)
	}

	// Spawn goroutine for the async pipeline. All remaining phases run inside.
	out := make(chan AsyncChatResult, 16)

	go func() {
		defer close(out)

		timer := common.NewTimer()
		timer.Start()

		// === Phase 2: Resolve LLM Model Config + max_tokens ===
		common.Info("Phase 2: Resolve LLM Model Config + max_tokens")
		timer.Enter(common.PhaseCheckLLM)
		llmModelConfig, _, _, _, err := s.getLLMModelConfig(chat)
		if err != nil {
			out <- AsyncChatResult{
				Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
				Final:  true,
			}
			return
		}
		modelMaxTokens := 8192
		if llmModelConfig != nil {
			// Treat max_tokens=0 as unset (default 8192) — mirrors
			// PR #16413 Python fix: model_extra.get("max_tokens") or 8192
			if mt, ok := llmModelConfig["max_tokens"].(int); ok && mt > 0 {
				modelMaxTokens = mt
			}
		}
		timer.Exit(common.PhaseCheckLLM)

		// === Phase 3: Langfuse Trace Setup ===
		common.Info("Phase 3: Setup Langfuse Trace")
		timer.Enter(common.PhaseCheckLangfuse)
		var langfuseTraceID string
		if lfClient, ok := ctx.Value(langfuseCtxKey).(*LangfuseClient); ok && lfClient != nil {
			langfuseTraceID = fmt.Sprintf("trace-%d", time.Now().UnixNano())
			_ = lfClient.PostTrace(ctx, LangfuseTrace{
				ID:        langfuseTraceID,
				Name:      "openai_chat",
				UserID:    chat.TenantID,
				SessionID: chat.ID,
				Metadata: map[string]interface{}{
					"stream":   stream,
					"kb_count": len(chat.KBIDs),
				},
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			})
		}
		timer.Exit(common.PhaseCheckLangfuse)

		// === Phase 4: Bind Models (embedding, rerank, chat, TTS) + ToolCall ===
		common.Info("Phase 4: Bind Models (embedding, rerank, chat, TTS)")
		timer.Enter(common.PhaseBindModels)
		kbs, embModel, rerankModel, chatModel, ttsModel := s.getModels(ctx, chat)

		// Toolcall binding
		if toolcallSession, hasSession := kwargs["toolcall_session"]; hasSession && toolcallSession != nil {
			if tools, hasTools := kwargs["tools"]; hasTools && tools != nil {
				if chatModel != nil {
					if ts, ok := toolcallSession.(modelModule.ToolCallSession); ok {
						common.Info("Bind ToolCall")
						chatModel.BindTools(ts, tools)
					}
				}
			}
		}
		timer.Exit(common.PhaseBindModels)

		// === Phase 5: Extract Questions, doc_ids, Attachments ===
		common.Info("Phase 5: Extract questions, doc_ids, attachments")
		// Retrieve the last 3 user questions.
		var questions []string
		for _, m := range messages {
			if role, _ := m["role"].(string); role == "user" {
				if content, ok := m["content"].(string); ok {
					questions = append(questions, content)
				}
			}
		}
		if len(questions) > 3 {
			questions = questions[len(questions)-3:]
		}

		common.Debug("Extracted questions", zap.Strings("questions", questions))

		// Resolve doc_ids from kwargs or the last message.
		// Kwargs["doc_ids"] is a comma-separated string.
		// messages[-1]["doc_ids"] ALWAYS overrides the kwargs value.
		var docIDs []string
		if docIDsStr, ok := kwargs["doc_ids"].(string); ok {
			for _, p := range strings.Split(docIDsStr, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					docIDs = append(docIDs, p)
				}
			}
		}
		if docIDsRaw, ok := lastMsg["doc_ids"]; ok {
			docIDs = nil
			if v, ok := docIDsRaw.([]string); ok {
				for _, id := range v {
					if id != "" {
						docIDs = append(docIDs, id)
					}
				}
			} else {
				common.Warn("doc_ids in message is not []string, ignoring",
					zap.Any("type", fmt.Sprintf("%T", docIDsRaw)))
			}
		}
		if docIDs != nil {
			common.Debug("Resolved doc_ids", zap.Strings("doc_ids", docIDs))
		}

		// Parse file attachments from the last message.
		// Split text-file URLs (joined with "\n\n") and image URLs.
		// Chat model: images → imageAttachments (multimodal conversion).
		// Image2text model: images → imageFiles (raw URLs).
		var textAttachmentsList []string
		var imageAttachments []string
		var imageFiles []string
		// Joined text attachments (appended to system prompt).
		var attachments string
		// When files are file dicts, splitFileAttachments fetches blobs
		// from storage. When plain strings, falls back to string splitting.
		if files, hasFiles := lastMsg["files"]; hasFiles {
			modelType := "chat"
			if llmModelConfig != nil {
				if mt, ok := llmModelConfig["model_type"].(string); ok {
					modelType = mt
				}
			}
			if modelType == "chat" {
				textAttachmentsList, imageAttachments = splitFileAttachments(userID, files, false)
			} else {
				textAttachmentsList, imageFiles = splitFileAttachments(userID, files, true)
			}
			attachments = strings.Join(textAttachmentsList, "\n\n")
			common.Debug("Resolved attachments",
				zap.Strings("text_attachments_list", textAttachmentsList),
				zap.Strings("image_attachments", imageAttachments),
				zap.Strings("image_files", imageFiles),
				zap.String("attachments", attachments))
		}

		// === Phase 6: SQL Retrieval ===
		// Retrieve field_map for SQL retrieval (preferred over vector search)
		promptConfig := chat.PromptConfig
		fieldMap, fmErr := s.datasetService.GetFieldMap(kbIDStrings(kbs))
		if fmErr != nil {
			common.Warn("get_field_map failed; proceeding without field_map", zap.Error(fmErr))
			fieldMap = nil
		}
		// Try structured SQL retrieval before vector search.
		// Only runs on the last question
		// HIT → return structured result directly.
		// MISS → fall through to vector search.
		if len(fieldMap) > 0 && chatModel != nil && len(kbs) > 0 {
			common.Info("Phase 6: Use SQL to retrieval")
			common.Debug("field_map retrieved", zap.Any("field_map", fieldMap))
			quote := true
			if v, ok := promptConfig["quote"].(bool); ok {
				quote = v
			}

			ans, sqlErr := s.useSQL(
				ctx, chat, kbs, questions[len(questions)-1], chatModel, fieldMap, quote,
			)
			if sqlErr != nil {
				common.Warn("SQL retrieval error; falling through", zap.Error(sqlErr))
			}

			// For aggregate queries (COUNT, SUM, etc.), chunks may be empty
			// but answer is still valid.
			chunks := []map[string]interface{}{}
			ansStr := ""
			if ans != nil {
				if refs, ok := ans["reference"].(map[string]interface{}); ok {
					if c, ok := refs["chunks"].([]map[string]interface{}); ok {
						chunks = c
					}
				}
				ansStr, _ = ans["answer"].(string)
			}
			if ans != nil && (ansStr != "" || len(chunks) > 0) {
				common.Info("SQL retrieval succeeded, skipping vector retrieval")

				// Enrich chunks with document metadata
				if includeRefMeta, metadataFields := s.resolveReferenceMetadata(promptConfig, kwargs); includeRefMeta && len(chunks) > 0 {
					if len(kbs) != 1 {
						hasMissingKBID := false
						for _, cm := range chunks {
							if _, hasKBID := cm["kb_id"]; !hasKBID {
								hasMissingKBID = true
								break
							}
						}
						if hasMissingKBID {
							common.Warn("Skipping some _enrich_chunks_with_document_metadata results because chat.kb_ids has multiple entries and use_sql returned chunks without kb_id",
								zap.Int("kb_count", len(kbs)))
						}
					}
					kbinfos := map[string]interface{}{"chunks": chunks}
					s.enrichChunksWithMetadata(kbinfos, chat.TenantID, metadataFields)
				}

				out <- AsyncChatResult{
					Answer:    ansStr,
					Reference: ans["reference"].(map[string]interface{}),
					Final:     true,
				}
				return
			}
			common.Info("SQL retrieval: no valid result, falling back to vector search")
		}

		// === Phase 7: Prompt Parameters ===
		common.Info("Phase 7: Building Prompt Parameters")
		// Build param_keys from prompt_config["parameters"].
		// prompt_config["parameters"] is a JSON array of
		//   {key: string, optional: bool}
		// objects declaring which placeholder variables the system prompt
		// template expects to be substituted.
		//
		// hasKnowledgeParam gates the entire RAG retrieval phase below.
		// When true: vector / DeepResearcher / TOC / Tavily / KG retrieval
		// populates kbinfos (from which knowledges is derived afterward).
		// When false: skip retrieval and rely on caller-supplied
		// kwargs["knowledge"] or LLM-only.
		var parameters []interface{}
		if paramsRaw, ok := promptConfig["parameters"]; ok {
			if p, ok := paramsRaw.([]interface{}); ok {
				parameters = p
			}
		}
		var paramKeys []string
		hasKnowledgeParam := false
		for _, p := range parameters {
			if pMap, ok := p.(map[string]interface{}); ok {
				if key, _ := pMap["key"].(string); key != "" {
					paramKeys = append(paramKeys, key)
					if key == "knowledge" {
						hasKnowledgeParam = true
					}
				}
			}
		}

		// Auto-fix: ensure "knowledge" is in param_keys when the chat has
		// KBs and the system prompt references {knowledge}.
		if len(kbs) > 0 && !hasKnowledgeParam {
			systemPrompt, _ := promptConfig["system"].(string)
			if strings.Contains(systemPrompt, "{knowledge}") {
				common.Warn("prompt_config['parameters'] is missing 'knowledge' entry despite kb_ids being set; auto-fixing.")
				parameters = append(parameters, map[string]interface{}{
					"key":      "knowledge",
					"optional": false,
				})
				promptConfig["parameters"] = parameters
				paramKeys = append(paramKeys, "knowledge")
				hasKnowledgeParam = true
			}
		}

		// Validate prompt template parameters against caller-supplied kwargs.
		// - "knowledge" is always skipped (system-injected, not caller-supplied).
		// - Missing non-optional param => return error immediately.
		// - Missing optional param => replace "{key}" placeholder with space.
		systemPrompt, _ := promptConfig["system"].(string)
		for _, p := range parameters {
			pMap, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			key, _ := pMap["key"].(string)
			if key == "knowledge" {
				continue // system-injected, skip caller validation
			}
			if _, inKwargs := kwargs[key]; !inKwargs {
				optional, _ := pMap["optional"].(bool)
				if !optional {
					// Required parameter missing => fail fast
					out <- AsyncChatResult{
						Answer: fmt.Sprintf("**ERROR**: Miss parameter: %s", key),
						Final:  true,
					}
					return
				}
				// Optional parameter missing => erase placeholder from system prompt
				systemPrompt = strings.ReplaceAll(systemPrompt, "{"+key+"}", " ")
			}
		}
		promptConfig["system"] = systemPrompt

		common.Debug("Prompt parameters",
			zap.Strings("doc_ids", docIDs),
			zap.Strings("param_keys", paramKeys),
			zap.Bool("has_embd_mdl", embModel != nil),
			zap.Any("prompt_config.parameters", promptConfig["parameters"]),
			zap.String("prompt_config.system", systemPrompt))

		// === Phase 8: Query refinement(LLM) ===
		// Sub-steps: refine_multiturn → cross_languages → meta_data_filter → keyword.
		common.Info("Phase 8: Query refinement(LLM)")
		timer.Enter(common.PhaseQueryRefinement)

		// refine_multiturn — condense multi-turn conversation into a single
		// refined question via LLM. When disabled, simply keep the last question.
		if refine, _ := chat.PromptConfig["refine_multiturn"].(bool); refine && len(questions) > 1 && chatModel != nil {
			if refined, err := FullQuestion(ctx, chatModel, messages, ""); err == nil && refined != "" {
				questions = []string{refined} // replace with refined question
				common.Debug("refine_multiturn applied",
					zap.String("refined", truncateForLog(refined, 60)))
			} else if err != nil {
				common.Warn("refine_multiturn failed; using original question", zap.Error(err))
			}
		} else {
			// Keep only the last question.
			questions = questions[len(questions)-1:]
		}

		// cross_languages — translate the question into configured target
		// languages via LLM, replacing the original. Useful for cross-lingual retrieval.
		if crossLangs, ok := chat.PromptConfig["cross_languages"].([]interface{}); ok && len(crossLangs) > 0 && chatModel != nil && len(questions) > 0 {
			langs := make([]string, 0, len(crossLangs))
			for _, x := range crossLangs {
				if s, ok := x.(string); ok && s != "" {
					langs = append(langs, s)
				}
			}
			if len(langs) > 0 {
				if translated, err := CrossLanguages(ctx, chat.TenantID, chat.LLMID, questions[0], langs); err == nil && translated != "" {
					original := questions[0]
					questions = []string{translated} // replace with translated question
					common.Debug("cross_languages applied",
						zap.String("original_question", original),
						zap.String("translated_question", translated))
				} else if err != nil {
					common.Warn("cross_languages failed", zap.Error(err))
				}
			}
		}

		// meta_data_filter — use LLM to map the question to metadata
		// criteria, then filter docIDs to matching
		// documents only.
		if chat.MetaDataFilter != nil && len(*chat.MetaDataFilter) > 0 && len(kbs) > 0 {
			kbIDs := kbIDStrings(kbs)
			if metaQ := questions[len(questions)-1]; metaQ != "" {
				var flattedMeta common.MetaData
				var mErr error
				if s.MetadataSvc != nil {
					flattedMeta, mErr = s.MetadataSvc.GetFlattedMetaByKBs(kbIDs)
				}
				if mErr == nil {
					if filtered, ok := ApplyMetaDataFilter(
						ctx,
						*chat.MetaDataFilter,
						flattedMeta,
						metaQ,
						chatModel,
						docIDs,
						kbIDs,
					); ok {
						common.Debug("meta_data_filter applied",
							zap.Int("filtered_count", len(filtered)),
							zap.Int("pre_filter_count", len(docIDs)))
						docIDs = filtered
					}
				} else {
					common.Warn("loadMetaData failed; skipping meta_data_filter", zap.Error(mErr))
				}
			}
		}

		// keyword — extract top-N keywords from the question via LLM and
		// append them to the question text to boost lexical retrieval recall.
		if useKW, _ := chat.PromptConfig["keyword"].(bool); useKW && chatModel != nil && len(questions) > 0 {
			if kw, err := KeywordExtraction(ctx, chatModel, questions[len(questions)-1], 3); err == nil && kw != "" {
				original := questions[len(questions)-1]
				questions[len(questions)-1] = questions[len(questions)-1] + "," + kw
				common.Debug("keyword extraction applied",
					zap.String("original_question", original),
					zap.String("augmented_question", questions[len(questions)-1]))
			} else if err != nil {
				common.Warn("keyword extraction failed", zap.Error(err))
			}
		}
		timer.Exit(common.PhaseQueryRefinement)

		// === Phase 9: Retrieval ===
		promptReasoning, _ := chat.PromptConfig["reasoning"].(bool)
		kwargReasoning, _ := kwargs["reasoning"].(bool)
		useReasoning := promptReasoning || kwargReasoning
		common.Info("Phase 9: Retrieval",
			zap.Bool("has_knowledge_param", hasKnowledgeParam),
			zap.Bool("reasoning", useReasoning))

		timer.Enter(common.PhaseRetrieval)
		var kbinfos map[string]interface{}
		kbinfos = map[string]interface{}{
			"total":    0,
			"chunks":   []map[string]interface{}{},
			"doc_aggs": []interface{}{},
		}
		var knowledges []string

		// When hasKnowledgeParam is true, runs (mutually exclusive):
		//   a) If reasoning is enabled: DeepResearcher replaces vector retrieval.
		//   b) Otherwise: standard retrieval, then:
		//      - TOC enhancement (if toc_enhance is enabled).
		//      - Child chunk retrieval.
		//      - Tavily web search (if internet is enabled).
		//      - Knowledge graph retrieval (if use_kg is enabled).
		// Populates kbinfos (chunks + doc_aggs) and knowledges.
		// When false, the entire block is skipped.
		if hasKnowledgeParam {
			if useReasoning && chatModel != nil && len(kbs) > 0 {
				// DeepResearcher — replaces vector retrieval.
				// Yields <retrieving> / </retrieving> markers + intermediate messages.
				docEngine := engine.Get()
				if docEngine != nil {
					retSvc := nlp.NewRetrievalService(docEngine, dao.NewDocumentDAO())
					tenantIDs := kbTenantIDStrings(kbs)
					kbIDs := kbIDStrings(kbs)

					// KB retrieval callback for the deep researcher
					kbRetrieve := func(ctx context.Context, q string) (*nlp.RetrievalResult, error) {
						return retSvc.Retrieval(ctx, &nlp.RetrievalRequest{
							Question:       q,
							TenantIDs:      tenantIDs,
							KbIDs:          kbIDs,
							DocIDs:         docIDs,
							Page:           1,
							PageSize:       int(chat.TopN),
							EmbeddingModel: embModel,
						})
					}

					dr := NewDeepResearcher(
						chatModel,
						map[string]interface{}(chat.PromptConfig),
						kbRetrieve,
						useWebSearch,
						docEngine,
						kbIDs,
						tenantIDs,
						embModel,
					)
					question := strings.Join(questions, " ")

					drErr := dr.Research(ctx, kbinfos, question, question, func(msg string) {
						switch {
						case strings.HasPrefix(msg, "<START_DEEP_RESEARCH>"):
							out <- AsyncChatResult{
								Answer:      "<retrieving>",
								Reference:   map[string]interface{}{},
								AudioBinary: nil,
								Final:       false,
							}
						case strings.HasPrefix(msg, "<END_DEEP_RESEARCH>"):
							out <- AsyncChatResult{
								Answer:      "</retrieving>",
								Reference:   map[string]interface{}{},
								AudioBinary: nil,
								Final:       false,
							}
						default:
							out <- AsyncChatResult{
								Answer:      msg,
								Reference:   map[string]interface{}{},
								AudioBinary: nil,
								Final:       false,
							}
						}
					})
					if drErr != nil {
						common.Warn("DeepResearcher failed", zap.Error(drErr))
					} else {
						// kbinfos now contains real chunks with proper
						// chunk_ids from the recursive tree search.
						common.Debug("DeepResearcher completed",
							zap.Int("chunks", len(kbinfos["chunks"].([]map[string]interface{}))))
					}
				}
			} else {
				searchQuestion := strings.Join(questions, " ")
				if embModel != nil {
					// Retrieval
					rankFeature := s.MetadataSvc.LabelQuestion(searchQuestion, kbs)
					{
						tenantIDs := make([]string, 0)
						kbIDs := make([]string, 0)
						for _, kb := range kbs {
							tenantIDs = append(tenantIDs, kb.TenantID)
							kbIDs = append(kbIDs, kb.ID)
						}

						docEngine := engine.Get()
						documentDAO := dao.NewDocumentDAO()
						retrievalSvc := nlp.NewRetrievalService(docEngine, documentDAO)

						top := int(chat.TopK)
						threshold := chat.SimilarityThreshold
						vsw := chat.VectorSimilarityWeight
						topN := int(chat.TopN)

						req := &nlp.RetrievalRequest{
							Question:               searchQuestion,
							TenantIDs:              tenantIDs,
							KbIDs:                  kbIDs,
							DocIDs:                 docIDs,
							Page:                   1,
							PageSize:               topN,
							Top:                    &top,
							SimilarityThreshold:    &threshold,
							VectorSimilarityWeight: &vsw,
							RankFeature:            &rankFeature,
							RerankModel:            rerankModel,
							EmbeddingModel:         embModel,
							Aggs:                   func() *bool { v := true; return &v }(),
						}

						result, retErr := retrievalSvc.Retrieval(ctx, req)
						if retErr != nil {
							kbinfos = map[string]interface{}{
								"total":    0,
								"chunks":   []map[string]interface{}{},
								"doc_aggs": []interface{}{},
							}
							err = retErr
						} else {
							docAggs := make([]interface{}, len(result.DocAggs))
							for i, da := range result.DocAggs {
								docAggs[i] = da
							}

							kbinfos = map[string]interface{}{
								"total":    len(result.Chunks),
								"chunks":   result.Chunks,
								"doc_aggs": docAggs,
							}
						}
					}
					if err != nil {
						common.Warn("Retrieval failed", zap.Error(err))
						// Continue with empty kbinfos.
					}

					// TOC enhancement
					if useTOC, _ := chat.PromptConfig["toc_enhance"].(bool); useTOC && chatModel != nil && len(kbs) > 0 {
						enhancer := NewTOCEnhancer(
							engine.Get(),
							chatModel,
							kbTenantIDStrings(kbs),
							kbIDStrings(kbs),
							searchQuestion,
							int(chat.TopN),
						)
						if added, err := enhancer.Enhance(ctx, kbinfos); err != nil {
							common.Warn("TOC enhance failed", zap.Error(err))
						} else if added > 0 {
							common.Debug("TOC enhance added chunks", zap.Int("added", added))
						}
					}
				}

				// Child chunk retrieval
				if existingChunks, ok := kbinfos["chunks"].([]map[string]interface{}); ok && len(existingChunks) > 0 {
					kbinfos["chunks"] = nlp.RetrievalByChildren(existingChunks, kbTenantIDStrings(kbs), engine.Get(), ctx)
				}

				// Web search via Tavily
				if s.shouldUseWebSearch(chat, kwargs["internet"]) {
					tavilyKey, _ := chat.PromptConfig["tavily_api_key"].(string)
					tavResult, tavErr := s.tavilyRetrieve(ctx, tavilyKey, searchQuestion)
					if tavErr != nil {
						common.Warn("Tavily web search failed", zap.Error(tavErr))
					} else {
						// Extend chunks and doc_aggs with web search results.
						if existingChunks, ok := kbinfos["chunks"].([]map[string]interface{}); ok {
							if newChunks, ok := tavResult["chunks"].([]map[string]interface{}); ok {
								kbinfos["chunks"] = append(existingChunks, newChunks...)
							}
						}
						if existingAggs, ok := kbinfos["doc_aggs"].([]interface{}); ok {
							if newAggs, ok := tavResult["doc_aggs"].([]interface{}); ok {
								kbinfos["doc_aggs"] = append(existingAggs, newAggs...)
							}
						}
					}
				}

				// Knowledge Graph retrieval
				if useKG, _ := chat.PromptConfig["use_kg"].(bool); useKG && chatModel != nil && len(kbs) > 0 {
					kgIDs := kbIDStrings(kbs)
					if len(kgIDs) > 0 {
						kgPipeline := graph.NewPipeline(engine.Get(), kgIDs, kbTenantIDStrings(kbs), searchQuestion)
						kgPipeline.SetChatModel(chatModel)
						if embModel != nil {
							kgPipeline.SetEmbModel(embModel)
						}
						kgChunk, kgErr := kgPipeline.Retrieval(ctx)
						if kgErr != nil {
							common.Warn("KG retrieval failed; falling through to vector-only",
								zap.Error(kgErr))
						} else if kgChunk != nil {
							if _, hasContent := kgChunk["content_with_weight"]; hasContent {
								if existingChunks, ok := kbinfos["chunks"].([]map[string]interface{}); ok {
									newChunks := make([]map[string]interface{}, 0, len(existingChunks)+1)
									newChunks = append(newChunks, kgChunk)
									newChunks = append(newChunks, existingChunks...)
									kbinfos["chunks"] = newChunks
									common.Debug("KG chunk prepended",
										zap.Int("total_chunks", len(newChunks)))
								}
							}
						}
					}
				}
			}
		}

		// Enrich chunks with document metadata AFTER all retrieval adds.
		// Request values (kwargs) take precedence over config values.
		if includeRefMeta, metadataFields := s.resolveReferenceMetadata(promptConfig, kwargs); includeRefMeta {
			s.enrichChunksWithMetadata(kbinfos, chat.TenantID, metadataFields)
		}
		timer.Exit(common.PhaseRetrieval)

		// === Phase 10: Build LLM Request ===
		// Sub-steps: empty_response check → formatPrompt → citationPrompt →
		// messageFitIn (95% token budget) → multimodal conversion → adjust max_tokens.
		// If no knowledges and empty_response is configured, yield it and return.
		knowledges = s.kbPrompt(kbinfos, modelMaxTokens)
		common.Info("Phase 10: Build LLM Request")
		common.Debug("Knowledge prompt",
			zap.String("question", strings.Join(questions, " ")),
			zap.Strings("knowledges", knowledges))

		// empty_response check
		// When no knowledge chunks were retrieved, skip the LLM entirely and
		// return the user-configured fallback message (if set).
		// If empty_response is not configured, fall through to the LLM call
		// with an empty knowledge context.
		if len(knowledges) == 0 {
			if emptyResp, ok := promptConfig["empty_response"].(string); ok && emptyResp != "" {
				out <- AsyncChatResult{
					Answer:      emptyResp,
					Reference:   kbinfos,
					AudioBinary: s.synthesizeTTS(ttsModel, emptyResp),
					Prompt:      fmt.Sprintf("\n\n### Query:\n%s", strings.Join(questions, " ")),
					Final:       true,
				}
				return
			}
		}

		// Format the system prompt with knowledge.
		// Only overwrite kwargs["knowledge"] when retrieval produced something;
		// otherwise preserve any caller-supplied value.
		knowledge := strings.Join(knowledges, "\n\n------\n\n")
		if knowledge != "" {
			kwargs["knowledge"] = "\n------\n" + knowledge
		}
		systemPrompt = ""
		if sp, ok := promptConfig["system"].(string); ok {
			systemPrompt = s.formatPrompt(sp, kwargs) + attachments
			// If knowledge was retrieved but the template has no {knowledge}
			// placeholder, auto-append it so the LLM still sees the context.
			if len(knowledges) > 0 && !strings.Contains(sp, "{knowledge}") {
				if kw, ok := kwargs["knowledge"].(string); ok {
					systemPrompt += kw
				}
			}
		}
		if systemPrompt != "" {
			common.Info("System prompt built",
				zap.Int("length", len(systemPrompt)))
		}

		// Build citation prompt if quoting is enabled.
		prompt4citation := ""
		quote := true
		if v, ok := kwargs["quote"].(bool); ok {
			quote = v
		}
		if promptConfigQuote, ok := promptConfig["quote"].(bool); ok {
			quote = quote && promptConfigQuote
		}
		if len(knowledges) > 0 && quote {
			prompt4citation = citationPrompt()
		}

		if prompt4citation != "" {
			common.Info("Citation prompt built",
				zap.Bool("quote", quote),
				zap.Int("length", len(prompt4citation)))
		}

		// Build the message list: system + cleaned user/assistant messages.
		var llmMessages []map[string]interface{}
		llmMessages = append(llmMessages, map[string]interface{}{
			"role":    "system",
			"content": systemPrompt,
		})
		factoryName := ""
		if llmModelConfig != nil {
			if f, ok := llmModelConfig["llm_factory"].(string); ok && f != "" {
				factoryName = strings.ToLower(f)
			}
		}
		if factoryName == "" {
			factoryName = factoryFromLLMID(chat.LLMID)
		}
		for _, m := range messages {
			role, _ := m["role"].(string)
			if role == "system" {
				continue
			}
			content := m["content"]
			if contentStr, ok := content.(string); ok {
				content = cleanCitationMarkers(contentStr)
			}
			llmMessages = append(llmMessages, map[string]interface{}{
				"role":    role,
				"content": content,
			})
		}

		// Fit messages within token budget.
		usedTokenCount, llmMessages := s.messageFitIn(llmMessages, int(float64(modelMaxTokens)*0.95))
		common.Debug("Messages fitted in token budget",
			zap.Int("model max_tokens", modelMaxTokens),
			zap.Int("used_token_count", usedTokenCount),
			zap.Int("msg_count", len(llmMessages)))

		// Multimodal conversion
		allImages := make([]string, 0, len(imageAttachments)+len(imageFiles))
		allImages = append(allImages, imageAttachments...)
		allImages = append(allImages, imageFiles...)
		if len(llmMessages) >= 2 && len(allImages) > 0 {
			lastIdx := len(llmMessages) - 1
			if role, _ := llmMessages[lastIdx]["role"].(string); role == "user" {
				if converted, err := common.ConvertLastUserMsgToMultimodal(
					llmMessages[lastIdx],
					allImages,
					factoryName,
				); err == nil {
					llmMessages[lastIdx] = converted
				}
			}
		}

		prompt := systemPrompt
		if len(llmMessages) > 0 {
			if c, ok := llmMessages[0]["content"].(string); ok {
				prompt = c
			}
		}

		if len(llmMessages) < 2 {
			out <- AsyncChatResult{
				Answer: "**ERROR**: message_fit_in has bug",
				Final:  true,
			}
			return
		}

		// Adjust max_tokens so the LLM has room within the total budget.
		if chat.LLMSetting != nil {
			if mt, ok := chat.LLMSetting["max_tokens"].(float64); ok {
				original := int(mt)
				adjusted := original
				if adjusted > modelMaxTokens-usedTokenCount {
					adjusted = modelMaxTokens - usedTokenCount
				}
				chat.LLMSetting["max_tokens"] = float64(adjusted)
				common.Debug("Adjusted max_tokens", zap.Int("max_tokens in chat", adjusted))
			}
		}

		// === Phase 11: Drive LLM + Decorate Answer ===
		// Stream path: accumulate deltas → per-delta TTS → decorate final.
		// Non-stream path: one-shot chat → decorate (includes TTS).
		// Answer decoration: citation markers, references, timing stats, Langfuse.
		common.Info("Phase 11: Drive LLM + Decorate Answer",
			zap.Bool("stream", stream),
			zap.Int("llm_messages_count", len(llmMessages)))
		timer.Enter(common.PhaseGenerateAnswer)
		chatDriver := s.buildChatDriver(chat, chatModel)
		if chatDriver == nil {
			out <- AsyncChatResult{
				Answer: "**ERROR**: No chat model available for this chat.",
				Final:  true,
			}
			return
		}
		chatMessages := s.buildChatMessages(prompt+prompt4citation, llmMessages[1:])

		// Langfuse generation start observation.
		var langfuseGenerationID string
		if langfuseTraceID != "" {
			if lfClient, ok := ctx.Value(langfuseCtxKey).(*LangfuseClient); ok && lfClient != nil {
				langfuseGenerationID = fmt.Sprintf("gen-%s", langfuseTraceID)
				modelName := ""
				if llmModelConfig != nil {
					if mn, ok := llmModelConfig["llm_name"].(string); ok {
						modelName = mn
					}
				}
				// PostGeneration creates a start-observation span.
				// Error is non-fatal; end-observation fires regardless.
				genInput := map[string]interface{}{
					"prompt":          prompt,
					"prompt4citation": prompt4citation,
					"messages":        chatMessages,
				}
				if err := lfClient.PostGeneration(ctx, LangfuseGeneration{
					ID:        langfuseGenerationID,
					TraceID:   langfuseTraceID,
					Name:      "chat",
					Model:     modelName,
					StartTime: time.Now().UTC().Format(time.RFC3339Nano),
					Input:     genInput,
				}); err != nil {
					common.Warn("Langfuse start observation (PostGeneration) failed; continuing without start-side tracing",
						zap.String("langfuse_trace_id", langfuseTraceID),
						zap.Error(err))
					// Keep langfuseGenerationID set so the end
					// Keep langfuseGenerationID set so end-observation fires.
				}
			}
		}

		// Stream path: per-delta callbacks, accumulate answer.
		// Non-stream path: one-shot synchronous answer.
		if stream {
			// Streaming path: accumulate answer, emit deltas.
			var fullAnswer string
			thinkState := &ThinkStreamState{}

			chatCfg := BuildChatConfig(chat, nil)

			// Tool routing: use tool-loop method when tools are bound.
			var driverErr error
			if chatDriver.ToolConfig != nil {
				// Tool streaming path:
				// Wraps reasoning in <think></think> markers.
				// inThink tracks local state to route reasoning vs answer.
				var inThink bool
				_, driverErr = chatDriver.ChatStreamlyWithTools(ctx, prompt+prompt4citation, chatMessages, chatCfg,
					func(answerDelta *string, reason *string) error {
						if answerDelta == nil || *answerDelta == "" {
							return nil
						}
						text := *answerDelta
						fullAnswer += text

						if text == "<think>" {
							inThink = true
							out <- AsyncChatResult{
								Answer:       "",
								Reference:    map[string]interface{}{},
								AudioBinary:  nil,
								CreatedAt:    float64(time.Now().Unix()),
								Final:        false,
								StartToThink: true,
							}
							return nil
						}
						if text == "</think>" {
							inThink = false
							out <- AsyncChatResult{
								Answer:      "",
								Reference:   map[string]interface{}{},
								AudioBinary: nil,
								CreatedAt:   float64(time.Now().Unix()),
								Final:       false,
								EndToThink:  true,
							}
							return nil
						}
						if inThink {
							// Reasoning text — route to Reasoning field so
							// the SSE handler maps it to
							// `delta.reasoning_content`.
							out <- AsyncChatResult{
								Reasoning:   text,
								Reference:   map[string]interface{}{},
								AudioBinary: nil,
								CreatedAt:   float64(time.Now().Unix()),
								Final:       false,
							}
						} else {
							// Regular answer content
							out <- AsyncChatResult{
								Answer:      text,
								Reference:   map[string]interface{}{},
								AudioBinary: s.synthesizeTTS(ttsModel, text),
								CreatedAt:   float64(time.Now().Unix()),
								Final:       false,
							}
						}
						return nil
					})
			} else {
				driverErr = chatDriver.ModelDriver.ChatStreamlyWithSender(
					*chatDriver.ModelName, chatMessages, chatDriver.APIConfig, chatCfg,
					func(answer *string, reason *string) error {
						if reason != nil && *reason != "" {
							if thinkState.EnterReasoning() {
								out <- AsyncChatResult{
									Answer:       "",
									Reference:    map[string]interface{}{},
									AudioBinary:  nil,
									CreatedAt:    float64(time.Now().Unix()),
									Final:        false,
									StartToThink: true,
								}
							}
							deltas := NextThinkDelta(thinkState, *reason, 16)
							for _, d := range deltas {
								if d.Kind == ThinkDeltaText && d.Value != "" {
									fullAnswer += d.Value
									out <- AsyncChatResult{
										Answer:      d.Value,
										Reference:   map[string]interface{}{},
										AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
										CreatedAt:   float64(time.Now().Unix()),
										Final:       false,
									}
								}
							}
						}
						if isContentDelta(answer) {
							if thinkState.ExitReasoning() {
								for _, d := range FlushRemaining(thinkState) {
									if d.Kind == ThinkDeltaText && d.Value != "" {
										fullAnswer += d.Value
										out <- AsyncChatResult{
											Answer:      d.Value,
											Reference:   map[string]interface{}{},
											AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
											CreatedAt:   float64(time.Now().Unix()),
											Final:       false,
										}
									}
								}
								out <- AsyncChatResult{
									Answer:      "",
									Reference:   map[string]interface{}{},
									AudioBinary: nil,
									CreatedAt:   float64(time.Now().Unix()),
									Final:       false,
									EndToThink:  true,
								}
							}
							fullAnswer += *answer
							deltas := BufferAnswerDelta(thinkState, *answer, 16)
							for _, d := range deltas {
								if d.Kind == ThinkDeltaText && d.Value != "" {
									out <- AsyncChatResult{
										Answer:      d.Value,
										Reference:   map[string]interface{}{},
										AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
										CreatedAt:   float64(time.Now().Unix()),
										Final:       false,
									}
								}
							}
						}
						return nil
					},
				)
			}
			if driverErr != nil {
				out <- AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", driverErr.Error()),
					Final:  true,
				}
				return
			}

			// Flush remaining state matching Python's final flush order
			// (dialog_service.py:1601-1612): think_buffer → marker → answer_buffer → pending_after_close
			// Python has no Reasoning field — all text is Answer.
			hadThinkClose := false
			for _, d := range FlushRemaining(thinkState) {
				if d.Kind == ThinkDeltaMarker && d.Value == "</think>" {
					hadThinkClose = true
					out <- AsyncChatResult{
						Answer:      "",
						Reference:   map[string]interface{}{},
						AudioBinary: nil,
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
						EndToThink:  true,
					}
				} else if d.Kind == ThinkDeltaText && d.Value != "" {
					out <- AsyncChatResult{
						Answer:      d.Value,
						Reference:   map[string]interface{}{},
						AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
					}
				}
			}
			// Close reasoning if the stream ended while still in reasoning mode
			// (e.g. model returned only reasoning chunks with no content delta).
			// Skip when FlushRemaining already emitted a </think> marker.
			if !hadThinkClose && thinkState.ExitReasoning() {
				out <- AsyncChatResult{
					Answer:      "",
					Reference:   map[string]interface{}{},
					AudioBinary: nil,
					CreatedAt:   float64(time.Now().Unix()),
					Final:       false,
					EndToThink:  true,
				}
			}

			// Decorate and yield the final answer.
			// Python uses state.full_text (raw text with <think> tags) as input
			// to _extract_visible_answer → decorate_answer (dialog_service.py:914-920).
			visibleAnswer := s.extractVisibleAnswer(thinkState.fullText)

			// Pass nil for ttsModel — audio was already produced per-delta.
			final := s.decorateAnswer(ctx, visibleAnswer, kbinfos, prompt, questions, usedTokenCount, timer, embModel, chat.VectorSimilarityWeight, quote, nil, langfuseTraceID, llmModelConfig, chat.TenantID, kbTenantIDStrings(kbs), len(knowledges) > 0)
			final.Final = true
			final.AudioBinary = nil
			timer.Exit(common.PhaseGenerateAnswer)
			out <- final
		} else {
			// Non-streaming: get the answer synchronously.
			var answer string
			var err error
			chatCfg := BuildChatConfig(chat, nil)

			// Tool routing: use tool-loop when tools are bound.
			if chatDriver.ToolConfig != nil {
				answer, _, err = chatDriver.ChatWithTools(ctx, prompt+prompt4citation, chatMessages, chatCfg)
			} else {
				resp, respErr := chatDriver.ModelDriver.ChatWithMessages(
					*chatDriver.ModelName, chatMessages, chatDriver.APIConfig, chatCfg,
				)
				if respErr != nil {
					err = respErr
				} else if resp != nil && resp.Answer != nil {
					answer = *resp.Answer
				}
			}

			if err != nil {
				out <- AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
					Final:  true,
				}
				return
			}

			// Last user message's content for the debug log.
			userContent := "[content not available]"
			if len(llmMessages) > 1 {
				if c, ok := llmMessages[len(llmMessages)-1]["content"].(string); ok {
					userContent = c
				}
			}
			common.Debug("User: " + userContent + "|Assistant: " + answer)

			// Synthesize TTS for the full answer (non-stream, one-shot).
			final := s.decorateAnswer(ctx, answer, kbinfos, prompt, questions, usedTokenCount, timer, embModel, chat.VectorSimilarityWeight, quote, ttsModel, langfuseTraceID, llmModelConfig, chat.TenantID, kbTenantIDStrings(kbs), len(knowledges) > 0)
			final.Final = true
			timer.Exit(common.PhaseGenerateAnswer)
			out <- final
		}
		common.Info("AsyncChat completed", zap.String("chat_id", chat.ID))
	}()

	return out, nil
}

// AsyncChatSolo is the LLM-only chat path (no KBs, no web search).
// Equivalent to Python's async_chat_solo() in dialog_service.py:289-337.
func (s *ChatPipelineService) AsyncChatSolo(
	ctx context.Context,
	userID string,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
) (<-chan AsyncChatResult, error) {

	out := make(chan AsyncChatResult, 16)

	go func() {
		defer close(out)

		// Timer brackets the LLM call; other phases are N/A in solo mode.
		timer := common.NewTimer()
		timer.Start()

		// 1. Resolve system prompt.
		promptConfig := chat.PromptConfig
		systemPrompt := ""
		if sp, ok := promptConfig["system"].(string); ok {
			systemPrompt = sp
		}

		// 1b. Resolve LLM model config (needed early for model_type dispatch).
		llmModelConfig, _, _, _, err := s.getLLMModelConfig(chat)
		factoryName := ""
		if err == nil && llmModelConfig != nil {
			factoryName, _ = llmModelConfig["llm_factory"].(string)
		}
		if factoryName == "" {
			factoryName = factoryFromLLMID(chat.LLMID)
		}

		// 2. Process file attachments (chat → data URIs, image2text → raw URLs).
		attachmentsStr := ""
		var imageFiles []string
		modelType := "chat"
		if llmModelConfig != nil {
			if mt, ok := llmModelConfig["model_type"].(string); ok && mt != "" {
				modelType = mt
			}
		}
		isImage2Text := modelType == "image2text"
		if len(messages) > 0 {
			if files, hasFiles := messages[len(messages)-1]["files"]; hasFiles {
				attachmentsStr = s.processFileAttachments(userID, files)
				if isImage2Text {
					imageFiles = s.extractRawImageURLs(files)
				} else {
					imageFiles = s.extractImageFiles(userID, files)
				}
			}
		}

		// 3. Strip citation markers and drop system messages from history.
		var msg []map[string]interface{}
		for _, m := range messages {
			role, _ := m["role"].(string)
			if role == "system" {
				continue
			}
			content := m["content"]
			if contentStr, ok := content.(string); ok {
				content = cleanCitationMarkers(contentStr)
			}
			msg = append(msg, map[string]interface{}{
				"role":    role,
				"content": content,
			})
		}
		// Append text attachments to the last user message (no separator).
		if attachmentsStr != "" && len(msg) > 0 {
			if lastContent, ok := msg[len(msg)-1]["content"].(string); ok {
				msg[len(msg)-1]["content"] = lastContent + attachmentsStr
			}
		}

		// 4. Build the chat model wrapper.
		driver, modelName, apiConfig, _, err := s.ModelProviderSvc.GetChatModelConfig(chat.TenantID, chat.LLMID)
		if err != nil {
			out <- AsyncChatResult{
				Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
				Final:  true,
			}
			return
		}
		chatModel := modelModule.NewChatModel(driver, &modelName, apiConfig)

		// 5. Resolve TTS model. Best-effort: warn and proceed without TTS on lookup failure.
		var ttsModel *modelModule.ChatModel
		if promptConfig != nil {
			if useTTS, _ := promptConfig["tts"].(bool); useTTS {
				ttsDriver, ttsName, ttsConfig, _, ttsErr := s.ModelProviderSvc.GetTenantDefaultModelByType(
					chat.TenantID, entity.ModelTypeTTS,
				)
				if ttsErr != nil {
					common.Warn("AsyncChatSolo: TTS lookup failed; proceeding without TTS",
						zap.String("tenant_id", chat.TenantID),
						zap.Error(ttsErr))
				} else {
					ttsModel = modelModule.NewChatModel(ttsDriver, &ttsName, ttsConfig)
				}
			}
		}

		// 6. Build messages for driver. Convert last user msg to multimodal if images present.
		var chatMessages []modelModule.Message
		if systemPrompt != "" {
			chatMessages = append(chatMessages, modelModule.Message{
				Role:    "system",
				Content: systemPrompt,
			})
		}
		for i, m := range msg {
			role, _ := m["role"].(string)
			content := m["content"]
			// Multimodal conversion for the last user message.
			if i == len(msg)-1 && role == "user" && len(imageFiles) > 0 {
				if converted, err := common.ConvertLastUserMsgToMultimodal(
					map[string]interface{}{"role": role, "content": content},
					imageFiles,
					strings.ToLower(factoryName),
				); err == nil {
					content = converted["content"]
				}
			}
			chatMessages = append(chatMessages, modelModule.Message{
				Role:    role,
				Content: content,
			})
		}

		// 7. Drive the LLM: stream (per-delta with think markers) or non-stream (one-shot).
		if stream {
			var fullAnswer string
			thinkState := &ThinkStreamState{}
			chatCfg := BuildChatConfig(chat, nil)
			timer.Enter(common.PhaseGenerateAnswer)

			driverErr := chatModel.ModelDriver.ChatStreamlyWithSender(
				*chatModel.ModelName, chatMessages, chatModel.APIConfig, chatCfg,
				func(answer *string, reason *string) error {
					if reason != nil && *reason != "" {
						if thinkState.EnterReasoning() {
							out <- AsyncChatResult{
								Answer:       "",
								Reference:    map[string]interface{}{},
								AudioBinary:  nil,
								CreatedAt:    float64(time.Now().Unix()),
								Final:        false,
								StartToThink: true,
							}
						}
						deltas := NextThinkDelta(thinkState, *reason, 16)
						for _, d := range deltas {
							if d.Kind == ThinkDeltaText && d.Value != "" {
								fullAnswer += d.Value
								out <- AsyncChatResult{
									Answer:      d.Value,
									Reference:   map[string]interface{}{},
									AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
									CreatedAt:   float64(time.Now().Unix()),
									Final:       false,
								}
							}
						}
					}
					if isContentDelta(answer) {
						if thinkState.ExitReasoning() {
							for _, d := range FlushRemaining(thinkState) {
								if d.Kind == ThinkDeltaText && d.Value != "" {
									fullAnswer += d.Value
									out <- AsyncChatResult{
										Answer:      d.Value,
										Reference:   map[string]interface{}{},
										AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
										CreatedAt:   float64(time.Now().Unix()),
										Final:       false,
									}
								}
							}
							out <- AsyncChatResult{
								Answer:      "",
								Reference:   map[string]interface{}{},
								AudioBinary: nil,
								CreatedAt:   float64(time.Now().Unix()),
								Final:       false,
								EndToThink:  true,
							}
						}
						fullAnswer += *answer
						deltas := BufferAnswerDelta(thinkState, *answer, 16)
						for _, d := range deltas {
							if d.Kind == ThinkDeltaText && d.Value != "" {
								out <- AsyncChatResult{
									Answer:      d.Value,
									Reference:   map[string]interface{}{},
									AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
									CreatedAt:   float64(time.Now().Unix()),
									Final:       false,
								}
							}
						}
					}
					return nil
				},
			)
			if driverErr != nil {
				out <- AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", driverErr.Error()),
					Final:  true,
				}
				return
			}
			timer.Exit(common.PhaseGenerateAnswer)
			hadThinkClose := false
			for _, d := range FlushRemaining(thinkState) {
				if d.Kind == ThinkDeltaMarker && d.Value == "</think>" {
					hadThinkClose = true
					out <- AsyncChatResult{
						Answer:      "",
						Reference:   map[string]interface{}{},
						AudioBinary: nil,
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
						EndToThink:  true,
					}
				} else if d.Kind == ThinkDeltaText && d.Value != "" {
					out <- AsyncChatResult{
						Answer:      d.Value,
						Reference:   map[string]interface{}{},
						AudioBinary: s.synthesizeTTS(ttsModel, d.Value),
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
					}
				}
			}
			// Close reasoning if the stream ended while still in reasoning mode
			// (e.g. model returned only reasoning chunks with no content delta).
			// Skip when FlushRemaining already emitted a </think> marker.
			if !hadThinkClose && thinkState.ExitReasoning() {
				out <- AsyncChatResult{
					Answer:      "",
					Reference:   map[string]interface{}{},
					AudioBinary: nil,
					CreatedAt:   float64(time.Now().Unix()),
					Final:       false,
					EndToThink:  true,
				}
			}
			finalAnswer := ExtractVisibleAnswer(thinkState.fullText)
			if finalAnswer == "" {
				finalAnswer = fullAnswer
			}
			out <- AsyncChatResult{
				Answer:      finalAnswer,
				Reference:   map[string]interface{}{},
				AudioBinary: nil,
				CreatedAt:   float64(time.Now().Unix()),
				Final:       true,
			}
		} else {
			// Non-streaming: one-shot call.
			chatCfg := BuildChatConfig(chat, nil)
			timer.Enter(common.PhaseGenerateAnswer)
			resp, err := chatModel.ModelDriver.ChatWithMessages(
				*chatModel.ModelName, chatMessages, chatModel.APIConfig, chatCfg,
			)
			timer.Exit(common.PhaseGenerateAnswer)
			if err != nil {
				out <- AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
					Final:  true,
				}
				return
			}
			answer := ""
			if resp.Answer != nil {
				answer = *resp.Answer
			}
			// Debug log matching Python's dialog_service.py:335-336.
			userContent := "[content not available]"
			if len(msg) > 0 {
				if c, ok := msg[len(msg)-1]["content"].(string); ok {
					userContent = c
				}
			}
			common.Debug("User: " + userContent + "|Assistant: " + answer)

			// Raw answer with full TTS, no decorate_answer. Caller handles decoration.
			out <- AsyncChatResult{
				Answer:      answer,
				Reference:   map[string]interface{}{},
				AudioBinary: s.synthesizeTTS(ttsModel, answer),
				CreatedAt:   float64(time.Now().Unix()),
				Final:       true,
			}
		}
	}()

	return out, nil
}

// extractImageFiles extracts data-URI image attachments from the files list.
// Mirrors Python split_file_attachments raw mode.
func (s *ChatPipelineService) extractImageFiles(userID string, files interface{}) []string {
	// ── File-dict mode ──
	if fileDicts, ok := parseFileDicts(files); ok {
		fileSvc := NewFileService()
		// Use raw=false to get base64 data URIs for images.
		_, images, err := fileSvc.GetFileContents(userID, fileDicts, false)
		if err != nil {
			common.Warn("GetFileContents failed in extractImageFiles",
				zap.Error(err))
			return nil
		}
		return images
	}

	// ── String fallback ──
	var images []string
	switch v := files.(type) {
	case []string:
		for _, f := range v {
			if strings.HasPrefix(f, "data:") {
				images = append(images, f)
			}
		}
	case []interface{}:
		for _, f := range v {
			if s, ok := f.(string); ok && strings.HasPrefix(s, "data:") {
				images = append(images, s)
			}
		}
	}
	return images
}

// extractRawImageURLs extracts image references as raw URLs/data-URIs from
// the string-mode files list, WITHOUT fetching blobs and WITHOUT filtering
// to data: prefixes. Used for image2text models that expect URLs in the
// multimodal content (matches Python's `image_files` from
// `split_file_attachments(files, raw=True)` at
// dialog_service.py:371-392).
//
// The downstream ConvertLastUserMsgToMultimodal calls parseDataURIOrB64
// (multimodal.go:63-92) which correctly handles all three forms:
//   - data: URI → base64 source
//   - http:// or https:// URL → URL source
//   - raw base64 → base64 source (default media type)
//
// File-dict mode is a known limitation: returns empty for now. A future
// FileService.GetFileURLsForChat (mirror of GetFileContents with
// raw=true) would be needed to fully cover the file-dict + image2text
// combination. The Python equivalent has the same limitation
// (split_file_attachments calls FileService.get_files which doesn't
// fetch blobs in raw mode).
func (s *ChatPipelineService) extractRawImageURLs(files interface{}) []string {
	if fileDicts, ok := parseFileDicts(files); ok {
		_ = fileDicts // see file-dict limitation comment above
		common.Debug("AsyncChatSolo: file-dict + image2text not yet supported; image refs dropped",
			zap.Int("file_dict_count", len(fileDicts)))
		return nil
	}

	// String-mode: return all entries as-is. The downstream
	// ConvertLastUserMsgToMultimodal + parseDataURIOrB64 will
	// dispatch on prefix (data: → base64, http(s): → url, else →
	// raw base64).
	var urls []string
	switch v := files.(type) {
	case []string:
		for _, f := range v {
			if f != "" {
				urls = append(urls, f)
			}
		}
	case []interface{}:
		for _, f := range v {
			if s, ok := f.(string); ok && s != "" {
				urls = append(urls, s)
			}
		}
	}
	return urls
}

// ---------------------------------------------------------------------------
// Helper methods
// ---------------------------------------------------------------------------

// internetTruthyStrings / internetFalsyStrings mirror the case-insensitive,
// whitespace-trimmed alias sets at dialog_service.py:115-117 of
// _normalize_internet_flag. Kept in one place so a future addition (e.g.
// Python accepting "y"/"n") is a one-line change here.
var internetTruthyStrings = map[string]bool{"true": true, "1": true, "yes": true, "on": true}
var internetFalsyStrings = map[string]bool{"false": true, "0": true, "no": true, "off": true, "": true}

// normalizeInternetFlag is the Go port of Python's
// _normalize_internet_flag (dialog_service.py:108-119). Three-state
// return matches Python: *true → explicit truthy, *false → explicit
// falsy, nil → couldn't interpret (Python's `return None`). The caller
// decides what to do with nil — _should_use_web_search treats it as
// "not enabled," so shouldUseWebSearch below only returns true when
// the normalized result is explicitly true.
//
// Accepted inputs (mirroring Python):
//   - bool: returned as-is
//   - int / int64 / float64 with value 0 or 1: coerced to bool
//   - string (case-insensitive, trimmed): "true"/"1"/"yes"/"on" → true;
//     "false"/"0"/"no"/"off"/"" → false
//   - everything else (nil, slices, maps, other numeric values,
//     unrecognized strings, complex, etc.) → nil
func normalizeInternetFlag(v interface{}) *bool {
	switch x := v.(type) {
	case bool:
		return &x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		if internetTruthyStrings[s] {
			t := true
			return &t
		}
		if internetFalsyStrings[s] {
			f := false
			return &f
		}
	case int:
		if x == 0 || x == 1 {
			b := x == 1
			return &b
		}
	case int64:
		if x == 0 || x == 1 {
			b := x == 1
			return &b
		}
	case float64:
		if x == 0 || x == 1 {
			b := x == 1
			return &b
		}
	}
	return nil
}

// shouldUseWebSearch returns true if web search should be enabled.
// Mirrors Python's _should_use_web_search (dialog_service.py:122-126):
// Tavily key must be present on chat.PromptConfig AND the internet
// flag must normalize to explicit true.
//
// The second parameter takes the raw internet value (typically
// kwargs["internet"] at the call site) — same shape as Python's
// `_should_use_web_search(chat.prompt_config, kwargs.get("internet"))`.
func (s *ChatPipelineService) shouldUseWebSearch(chat *entity.Chat, internet interface{}) bool {
	if chat.PromptConfig == nil {
		return false
	}
	tavilyKey, _ := chat.PromptConfig["tavily_api_key"].(string)
	if tavilyKey == "" {
		return false
	}
	normalized := normalizeInternetFlag(internet)
	return normalized != nil && *normalized
}

// tavilyRetrieve calls the Tavily API and returns results in the same chunk
// format used by performRetrieval. Mirrors Python's Tavily.retrieve_chunks()
// in rag/utils/tavily_conn.py.
func (s *ChatPipelineService) tavilyRetrieve(ctx context.Context, apiKey, question string) (map[string]interface{}, error) {
	const tavilyURL = "https://api.tavily.com/search"

	body := map[string]interface{}{
		"query":        question,
		"search_depth": "advanced",
		"max_results":  6,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("tavily: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tavilyURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("tavily: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily: status %d", resp.StatusCode)
	}

	var tavilyResp struct {
		Results []struct {
			URL     string  `json:"url"`
			Title   string  `json:"title"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return nil, fmt.Errorf("tavily: decode response: %w", err)
	}

	chunks := make([]map[string]interface{}, 0, len(tavilyResp.Results))
	docAggs := make([]interface{}, 0, len(tavilyResp.Results))
	for _, r := range tavilyResp.Results {
		id := fmt.Sprintf("tavily-%s", r.URL)
		chunk := map[string]interface{}{
			"chunk_id":            id,
			"content_ltks":        tokenizeText(r.Content), // tokenized content
			"content_with_weight": r.Content,
			"doc_id":              id,
			"docnm_kwd":           r.Title,
			"kb_id":               []interface{}{},
			"important_kwd":       []interface{}{},
			"image_id":            "",
			"similarity":          r.Score,
			"vector_similarity":   1.0,
			"term_similarity":     0.0,
			"vector":              []float64{}, // empty; no embedding for web results
			"positions":           []interface{}{},
			"url":                 r.URL,
		}
		chunks = append(chunks, chunk)
		docAggs = append(docAggs, map[string]interface{}{
			"doc_name": r.Title,
			"doc_id":   id,
			"count":    1,
			"url":      r.URL,
		})
	}

	common.Info("[Tavily] question: "+question, zap.Int("results", len(chunks)))
	return map[string]interface{}{
		"chunks":   chunks,
		"doc_aggs": docAggs,
	}, nil
}

// tokenizeText is a lightweight tokenizer for Tavily content.
// It lowercases and splits on whitespace, similar to rag_tokenizer.tokenize.
func tokenizeText(text string) string {
	// Collapse multiple whitespaces and split.
	ws := regexp.MustCompile(`\s+`)
	text = ws.ReplaceAllString(text, " ")
	// Convert to lowercase for tokenization.
	return strings.ToLower(text)
}

// getLLMModelConfig resolves the LLM model configuration for the chat.
// Mirrors Python's three-branch resolver at dialog_service.py:552-561:
//
//	if chat.llm_id:
//	    if "image2text" in get_model_type_by_name(...): → IMAGE2TEXT
//	    else:                                            → CHAT
//	else:                                                → tenant default CHAT
//
// The returned `cfg` map's "model_type" field carries the chosen type
// so downstream code (e.g. the multimodal-conversion guard in AsyncChat
// at async_chat.go:632) can skip chat-only logic for image2text dialogs.
func (s *ChatPipelineService) getLLMModelConfig(chat *entity.Chat) (map[string]interface{}, string, string, string, error) {
	if chat.LLMID == "" {
		// Branch 3: no explicit LLM → tenant default chat model.
		return s.buildLLMModelConfig(
			s.ModelProviderSvc.GetTenantDefaultModelByType(chat.TenantID, entity.ModelTypeChat),
		)
	}

	// Branches 1/2: explicit LLM. Probe model types and pick IMAGE2TEXT
	// when the LLM is registered as such, otherwise CHAT.
	modelType := entity.ModelTypeChat
	modelTypeStr := "chat"
	if modelTypes, mtErr := s.ModelProviderSvc.GetModelTypeByName(chat.TenantID, chat.LLMID); mtErr == nil {
		for _, mt := range modelTypes {
			if mt == entity.ModelTypeImage2Text {
				modelType = entity.ModelTypeImage2Text
				modelTypeStr = "image2text"
				break
			}
		}
	}
	cfg, modelName, factoryName, baseURL, err := s.buildLLMModelConfig(
		s.ModelProviderSvc.GetModelConfigFromProviderInstance(chat.TenantID, modelType, chat.LLMID),
	)
	if err != nil {
		return nil, "", "", "", err
	}
	cfg["model_type"] = modelTypeStr
	return cfg, modelName, factoryName, baseURL, nil
}

// buildLLMModelConfig collapses the (driver, modelName, apiConfig,
// _, err) tuple from a model-provider lookup into the dict-shaped
// config the rest of async_chat.go consumes. Default "model_type" is
// "chat"; callers that resolved a different type overwrite the key
// before returning.
func (s *ChatPipelineService) buildLLMModelConfig(
	driver modelModule.ModelDriver,
	modelName string,
	apiConfig *modelModule.APIConfig,
	maxTokens int,
	err error,
) (map[string]interface{}, string, string, string, error) {
	if err != nil {
		return nil, "", "", "", err
	}
	// Match Python: llm.max_tokens if llm.max_tokens else 8192.
	if maxTokens == 0 {
		maxTokens = 8192
	}
	cfg := map[string]interface{}{
		"model_type":  "chat",
		"llm_name":    modelName,
		"max_tokens":  maxTokens,
		"llm_factory": driver.Name(),
	}
	baseURL := ""
	if apiConfig != nil && apiConfig.BaseURL != nil {
		baseURL = *apiConfig.BaseURL
	}
	return cfg, modelName, driver.Name(), baseURL, nil
}

// getModels resolves all models needed for the RAG pipeline.
// Mirrors Python's get_models() in dialog_service.py:340.
func (s *ChatPipelineService) getModels(ctx context.Context, chat *entity.Chat) (
	[]*entity.Knowledgebase,
	*modelModule.EmbeddingModel,
	*modelModule.RerankModel,
	*modelModule.ChatModel,
	*modelModule.ChatModel, // TTS model
) {
	kbDAO := dao.NewKnowledgebaseDAO()

	// Extract KB ID strings.
	kbIDs := make([]string, 0, len(chat.KBIDs))
	for _, raw := range chat.KBIDs {
		if id, ok := raw.(string); ok && id != "" {
			kbIDs = append(kbIDs, id)
		}
	}

	var kbs []*entity.Knowledgebase
	if len(kbIDs) > 0 {
		var err error
		kbs, err = kbDAO.GetByIDs(kbIDs)
		if err != nil {
			common.Warn("Failed to get KBs by IDs; retrieval may be incomplete",
				zap.Strings("kbIDs", kbIDs), zap.Error(err))
		}
	}

	// Embedding model.
	var embModel *modelModule.EmbeddingModel
	if len(kbs) > 0 {
		// All KBs must share the same embedding model.
		embdIDs := make(map[string]bool)
		for _, kb := range kbs {
			if kb.EmbdID != "" {
				embdIDs[kb.EmbdID] = true
			}
		}
		if len(embdIDs) > 1 {
			// Multiple embedding models across KBs — error.
			common.Warn("Knowledge bases use different embedding models")
		}
		if len(embdIDs) == 1 {
			for embdID := range embdIDs {
				embdTenantID := kbs[0].TenantID
				driver, modelName, apiConfig, maxTokens, err := s.ModelProviderSvc.GetModelConfigFromProviderInstance(
					embdTenantID, entity.ModelTypeEmbedding, embdID,
				)
				if err == nil {
					embModel = modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
				}
			}
		}
	}

	// Chat model.
	driver, modelName, apiConfig, _, err := s.ModelProviderSvc.GetChatModelConfig(chat.TenantID, chat.LLMID)
	var chatModel *modelModule.ChatModel
	if err == nil {
		chatModel = modelModule.NewChatModel(driver, &modelName, apiConfig)
	}

	// Rerank model.
	var rerankModel *modelModule.RerankModel
	if chat.RerankID != "" {
		rerankDriver, rerankName, rerankConfig, _, err := s.ModelProviderSvc.GetModelConfigFromProviderInstance(
			chat.TenantID, entity.ModelTypeRerank, chat.RerankID,
		)
		if err == nil {
			rerankModel = modelModule.NewRerankModel(rerankDriver, &rerankName, rerankConfig)
		}
	}

	// TTS model.
	var ttsModel *modelModule.ChatModel
	if chat.PromptConfig != nil {
		if useTTS, _ := chat.PromptConfig["tts"].(bool); useTTS {
			ttsDriver, ttsName, ttsConfig, _, err := s.ModelProviderSvc.GetTenantDefaultModelByType(
				chat.TenantID, entity.ModelTypeTTS,
			)
			if err == nil {
				ttsModel = modelModule.NewChatModel(ttsDriver, &ttsName, ttsConfig)
			}
		}
	}

	return kbs, embModel, rerankModel, chatModel, ttsModel
}

// lastUserQuestion returns the content of the most recent user message in
// `messages`, or "" if there is no user message. Used by the P2
// meta_data_filter wiring (Python's `questions[-1]` in
// dialog_service.py:655).

// factoryFromLLMID extracts the provider name from a composite LLM ID
// like "Qwen3-8B@ling@SILICONFLOW" → "SILICONFLOW". When the LLM ID has
// no "@provider" segment, returns "openai" as a default. The lowercase
// return value is what ConvertLastUserMsgToMultimodal /
// RenderContentPartsForFactory dispatch on.
func factoryFromLLMID(llmID string) string {
	if llmID == "" {
		return "openai"
	}
	parts := strings.Split(llmID, "@")
	if len(parts) < 3 {
		return "openai"
	}
	provider := strings.ToLower(parts[len(parts)-1])
	if provider == "" {
		return "openai"
	}
	return provider
}

// The handler in openai_chat.go has already rejected requests
// whose last message is not from the user, so this should always succeed.
func lastUserQuestion(messages []map[string]interface{}) string {
	for i := len(messages) - 1; i >= 0; i-- {
		role, _ := messages[i]["role"].(string)
		if role == "user" {
			if c, ok := messages[i]["content"].(string); ok {
				return c
			}
			return ""
		}
	}
	return ""
}

// processFileAttachments extracts text content from file attachments.
// Mirrors Python's split_file_attachments (dialog_service.py:371-392)
// in raw=false mode: returns text attachments joined by "\n\n",
// filtering out data-URI image attachments.
//
// When files are file dicts (Python-compatible format), calls
// FileService.GetFileContents to fetch actual blobs from storage.
func (s *ChatPipelineService) processFileAttachments(userID string, files interface{}) string {
	// ── File-dict mode ──
	if fileDicts, ok := parseFileDicts(files); ok {
		fileSvc := NewFileService()
		texts, _, err := fileSvc.GetFileContents(userID, fileDicts, false)
		if err != nil {
			common.Warn("GetFileContents failed in processFileAttachments",
				zap.Error(err))
			return ""
		}
		if len(texts) == 0 {
			return ""
		}
		return strings.Join(texts, "\n\n")
	}

	// ── String fallback ──
	var texts []string
	switch v := files.(type) {
	case []string:
		for _, f := range v {
			if s := strings.TrimSpace(f); s != "" && !strings.HasPrefix(s, "data:") {
				texts = append(texts, s)
			}
		}
	case []interface{}:
		for _, f := range v {
			if s, ok := f.(string); ok && strings.TrimSpace(s) != "" && !strings.HasPrefix(s, "data:") {
				texts = append(texts, s)
			}
		}
	}
	if len(texts) == 0 {
		return ""
	}
	return strings.Join(texts, "\n\n")
}

// splitFileAttachments mirrors Python's `split_file_attachments` at
// dialog_service.py:371-392. It separates `messages[-1]["files"]`
// into text-file content and image attachments.
//
// Two modes of operation:
//
//  1. File-dict mode: When `files` is `[]map[string]interface{}` (each dict
//     with keys "id", "created_by", "mime_type", "name"), the method calls
//     FileService.GetFileContents to fetch actual file blobs from
//     storage, mirroring Python's FileService.get_files().
//
//  2. String-fallback mode: When `files` is `[]string` or `[]interface{}` of
//     strings (pre-resolved content), the method does simple string splitting:
//     - raw=false: split by "data:" prefix. Text → textAttachments; data:
//     URIs → image files.
//     - raw=true: all items go to textAttachments (Python's FileService.get_files
//     with raw=True pre-separates images, so non-image content arrives here).
func splitFileAttachments(userID string, files interface{}, raw bool) (textAttachments []string, imageAttachments []string) {
	// ── Mode 1: file dicts (Python-compatible) ──
	if fileDicts, ok := parseFileDicts(files); ok {
		fileSvc := NewFileService()
		texts, images, err := fileSvc.GetFileContents(userID, fileDicts, raw)
		if err != nil {
			common.Warn("GetFileContents failed, falling back to string splitting",
				zap.Error(err))
		} else {
			return texts, images
		}
	}

	// ── Mode 2: string content fallback (backward compat) ──
	var texts []string
	var images []string

	if raw {
		// Mirrors Python raw=True: FileService.get_files already
		// separated images; only non-image content arrives here.
		switch v := files.(type) {
		case []string:
			for _, f := range v {
				f = strings.TrimSpace(f)
				if f != "" {
					texts = append(texts, f)
				}
			}
		case []interface{}:
			for _, f := range v {
				if s, ok := f.(string); ok {
					s = strings.TrimSpace(s)
					if s != "" {
						texts = append(texts, s)
					}
				}
			}
		}
		return texts, images
	}

	// raw=false: split by "data:" prefix.
	process := func(f string) {
		f = strings.TrimSpace(f)
		if f == "" {
			return
		}
		if strings.HasPrefix(f, "data:") {
			images = append(images, f)
		} else {
			texts = append(texts, f)
		}
	}
	switch v := files.(type) {
	case []string:
		for _, f := range v {
			process(f)
		}
	case []interface{}:
		for _, f := range v {
			if s, ok := f.(string); ok {
				process(s)
			}
		}
	}
	return texts, images
}

// parseFileDicts attempts to parse files as a list of file-dict maps
// (the Python-compatible format from messages[-1]["files"]).
// Returns the parsed slice and true on success.
func parseFileDicts(files interface{}) ([]map[string]interface{}, bool) {
	switch v := files.(type) {
	case []map[string]interface{}:
		if len(v) == 0 {
			return nil, false
		}
		// Verify the first element has a recognizable file-dict key.
		if _, ok := v[0]["id"]; ok {
			return v, true
		}
		return nil, false
	case []interface{}:
		if len(v) == 0 {
			return nil, false
		}
		// Check if the first element is a map with file-dict keys.
		if m, ok := v[0].(map[string]interface{}); ok {
			if _, hasID := m["id"]; hasID {
				result := make([]map[string]interface{}, len(v))
				for i, item := range v {
					if m2, mok := item.(map[string]interface{}); mok {
						result[i] = m2
					} else {
						return nil, false
					}
				}
				return result, true
			}
		}
	}
	return nil, false
}

// cleanTTSText sanitizes text for TTS synthesis.
// Mirrors dialog_service.py:1404-1423.
func cleanTTSText(text string) string {
	if text == "" {
		return ""
	}
	// Strip control chars.
	controlRe := regexp.MustCompile(`[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]`)
	text = controlRe.ReplaceAllString(text, "")
	// Strip emojis.
	emojiRe := regexp.MustCompile("[\U0001f600-\U0001f64f\U0001f300-\U0001f5ff\U0001f680-\U0001f6ff\U0001f1e0-\U0001f1ff\U00002700-\U000027bf\U0001f900-\U0001f9ff\U0001fa70-\U0001faff\U0001fad0-\U0001faff]+")
	text = emojiRe.ReplaceAllString(text, "")
	// Collapse whitespace.
	wsRe := regexp.MustCompile(`\s+`)
	text = wsRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

// synthesizeTTS calls the TTS model to convert text to audio.
// Mirrors dialog_service.py:1426-1432.
func (s *ChatPipelineService) synthesizeTTS(ttsModel *modelModule.ChatModel, text string) interface{} {
	if ttsModel == nil || text == "" {
		return nil
	}
	text = cleanTTSText(text)
	if text == "" {
		return nil
	}
	ttsResp, err := ttsModel.ModelDriver.AudioSpeech(
		ttsModel.ModelName, &text, ttsModel.APIConfig, &modelModule.TTSConfig{Format: "mp3"},
	)
	if err != nil {
		common.Warn("TTS synthesis failed", zap.Error(err))
		return nil
	}
	if ttsResp == nil || len(ttsResp.Audio) == 0 {
		return nil
	}
	return ttsResp.Audio
}

// truncateForLog returns at most n characters of s, appending an
// ellipsis when truncated. Used to keep zap log lines bounded.
func truncateForLog(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// resolveReferenceMetadata mirrors Python's
// `resolve_reference_metadata_preferences` in
// api/utils/reference_metadata_utils.py:22-62. Returns (include,
// fields). The Python algorithm:
//
//	resolved = {**config["reference_metadata"], **request["reference_metadata"]}
//	if "include_metadata" in request: resolved["include"] = ...
//	if "metadata_fields" in request: resolved["fields"] = ...
//	include = bool(resolved.get("include", False))
//	fields = resolved.get("fields") → list of strings
//
// Config is `promptConfig["reference_metadata"]`, request is `kwargs`.
// `kwargs` takes precedence for both `include_metadata` (legacy) and
// `metadata_fields` (legacy), and for the entire `reference_metadata`
// sub-dict (preferred).
func (s *ChatPipelineService) resolveReferenceMetadata(promptConfig map[string]interface{}, kwargs map[string]interface{}) (bool, []string) {
	resolved := map[string]interface{}{}

	// Layer 1: prompt_config["reference_metadata"] (config).
	if promptConfig != nil {
		if cfgRef, ok := promptConfig["reference_metadata"].(map[string]interface{}); ok {
			for k, v := range cfgRef {
				resolved[k] = v
			}
		}
	}
	// Layer 2: kwargs["reference_metadata"] (request, takes precedence).
	if kwargs != nil {
		if reqRef, ok := kwargs["reference_metadata"].(map[string]interface{}); ok {
			for k, v := range reqRef {
				resolved[k] = v
			}
		}
		// Layer 3: legacy request keys (kwargs).
		if v, ok := kwargs["include_metadata"]; ok {
			if b, ok := v.(bool); ok {
				resolved["include"] = b
			}
		}
		if v, ok := kwargs["metadata_fields"]; ok {
			resolved["fields"] = v
		}
	}

	include, _ := resolved["include"].(bool)
	if !include {
		return false, nil
	}
	rawFields, ok := resolved["fields"]
	if !ok || rawFields == nil {
		return true, nil
	}
	switch v := rawFields.(type) {
	case []string:
		return true, v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return true, out
	}
	return true, nil
}

// enrichChunksWithMetadata enriches chunk records in kbinfos with document-level
// metadata. Mirrors Python's enrich_chunks_with_document_metadata() in
// api/utils/reference_metadata_utils.py.
func (s *ChatPipelineService) enrichChunksWithMetadata(kbinfos map[string]interface{}, tenantID string, fields []string) {
	chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{})
	if !ok || len(chunksRaw) == 0 {
		return
	}

	chunks := make([]map[string]interface{}, 0, len(chunksRaw))
	chunks = append(chunks, chunksRaw...)
	if len(chunks) == 0 {
		return
	}

	s.MetadataSvc.EnrichChunksWithDocMetadata(chunks, tenantID, fields)
}

// kbPrompt builds knowledge prompt blocks from retrieved chunks.
// Mirrors Python's kb_prompt() in rag/prompts/generator.py.
func (s *ChatPipelineService) kbPrompt(kbinfos map[string]interface{}, maxTokens int) []string {
	chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{})
	if !ok || len(chunksRaw) == 0 {
		return nil
	}

	// Pass 1: count content tokens to determine how many chunks fit.
	type chunkContent struct {
		content string
	}
	contents := make([]chunkContent, 0, len(chunksRaw))
	for _, ck := range chunksRaw {
		c := getMapString(ck, "content_with_weight", "content")
		if c == "" {
			continue
		}
		contents = append(contents, chunkContent{content: c})
	}

	usedTokenCount := 0
	chunksNum := 0
	for _, cc := range contents {
		usedTokenCount += graph.NumTokensFromString(cc.content)
		chunksNum++
		if float64(maxTokens)*0.97 < float64(usedTokenCount) {
			common.Warn("Not all the retrieval into prompt",
				zap.Int("kept", chunksNum),
				zap.Int("total", len(contents)))
			break
		}
	}

	// Pass 2: format chunks with tree structure, capped at chunksNum.
	if chunksNum > len(chunksRaw) {
		chunksNum = len(chunksRaw)
	}
	var result []string
	for i := 0; i < chunksNum; i++ {
		ck := chunksRaw[i]
		c := getMapString(ck, "content_with_weight", "content")
		if c == "" {
			continue
		}

		cnt := fmt.Sprintf("\nID: %d", i)
		cnt += drawNode("Title", getMapString(ck, "docnm_kwd", "document_name"))
		cnt += drawNode("URL", getMapString(ck, "url"))
		if meta, ok := ck["document_metadata"].(map[string]interface{}); ok {
			for k, v := range meta {
				cnt += drawNode(k, v)
			}
		}
		cnt += "\n└── Content:\n"
		cnt += c
		result = append(result, cnt)
	}

	return result
}

// formatPrompt substitutes {key} placeholders in a prompt string.
func (s *ChatPipelineService) formatPrompt(template string, kwargs map[string]interface{}) string {
	result := template
	for key, value := range kwargs {
		placeholder := "{" + key + "}"
		if strings.Contains(result, placeholder) {
			strVal := fmt.Sprintf("%v", value)
			result = strings.ReplaceAll(result, placeholder, strVal)
		}
	}
	// Replace any remaining {unknown} placeholders with empty string.
	for _, key := range []string{"knowledge", "quote"} {
		placeholder := "{" + key + "}"
		if strings.Contains(result, placeholder) {
			result = strings.ReplaceAll(result, placeholder, " ")
		}
	}
	return result
}

// messageFitIn trims messages to fit within a token budget.
// Mirrors Python's message_fit_in() in rag/prompts/generator.py.
//
// Strategy:
//  1. If everything fits → return as-is.
//  2. Keep all system messages + the last user/assistant message.
//  3. If still too large, trim content proportionally:
//     - System dominates (>80%) → preserve last message first.
//     - Otherwise → preserve system first.
func (s *ChatPipelineService) messageFitIn(messages []map[string]interface{}, maxTokens int) (int, []map[string]interface{}) {
	if maxTokens <= 0 {
		maxTokens = 8192
	}

	// Step 1: everything fits.
	totalTokens := s.countAllTokens(messages)
	if totalTokens < maxTokens {
		return totalTokens, messages
	}

	// Step 2: keep all system messages + the last message.
	result := make([]map[string]interface{}, 0)
	for _, m := range messages {
		if role, _ := m["role"].(string); role == "system" {
			result = append(result, m)
		}
	}
	if len(messages) > 1 {
		result = append(result, messages[len(messages)-1])
	}

	totalTokens = s.countAllTokens(result)
	if totalTokens < maxTokens {
		return totalTokens, result
	}

	// Step 3: trim content to fit.
	ll := graph.NumTokensFromString(s.stringContent(result[0]))
	ll2 := graph.NumTokensFromString(s.stringContent(result[len(result)-1]))
	total := ll + ll2
	if total <= 0 {
		return 0, result
	}

	if len(result) == 1 {
		result[0]["content"] = graph.TrimContentToTokenLimit(s.stringContent(result[0]), maxTokens)
		return s.countAllTokens(result), result
	}

	if float64(ll)/float64(total) > 0.8 {
		preservedLast := min(ll2, maxTokens)
		result[len(result)-1]["content"] = graph.TrimContentToTokenLimit(s.stringContent(result[len(result)-1]), preservedLast)
		remaining := max(0, maxTokens-preservedLast)
		result[0]["content"] = graph.TrimContentToTokenLimit(s.stringContent(result[0]), remaining)
	} else {
		preservedSystem := min(ll, maxTokens)
		result[0]["content"] = graph.TrimContentToTokenLimit(s.stringContent(result[0]), preservedSystem)
		remaining := max(0, maxTokens-preservedSystem)
		result[len(result)-1]["content"] = graph.TrimContentToTokenLimit(s.stringContent(result[len(result)-1]), remaining)
	}

	return s.countAllTokens(result), result
}

// countAllTokens returns the total token count across all messages.
func (s *ChatPipelineService) countAllTokens(messages []map[string]interface{}) int {
	total := 0
	for _, m := range messages {
		total += graph.NumTokensFromString(s.stringContent(m))
	}
	return total
}

// stringContent extracts the "content" string from a message map, or "".
func (s *ChatPipelineService) stringContent(m map[string]interface{}) string {
	c, _ := m["content"].(string)
	return c
}

// buildChatMessages converts the internal message representation to
// modelModule.Message for the driver.
func (s *ChatPipelineService) buildChatMessages(systemContent string, messages []map[string]interface{}) []modelModule.Message {
	var result []modelModule.Message
	if systemContent != "" {
		result = append(result, modelModule.Message{Role: "system", Content: systemContent})
	}
	for _, m := range messages {
		role, _ := m["role"].(string)
		content := m["content"]
		if role == "" || content == nil {
			continue
		}
		result = append(result, modelModule.Message{Role: role, Content: content})
	}
	return result
}

// buildChatDriver creates a ChatModel wrapper from the chat.
func (s *ChatPipelineService) buildChatDriver(chat *entity.Chat, chatModel *modelModule.ChatModel) *modelModule.ChatModel {
	if chatModel != nil {
		return chatModel
	}
	driver, modelName, apiConfig, _, err := s.ModelProviderSvc.GetChatModelConfig(chat.TenantID, chat.LLMID)
	if err != nil {
		return nil
	}
	return modelModule.NewChatModel(driver, &modelName, apiConfig)
}

// HydrateChunkVectors fills the `vector` field on each chunk in `kbinfos`
// that lacks one, by issuing a single batched fetch via
// RetrievalService.FetchChunkVectors. Mirrors Python's
// `async_chat._hydrate_chunk_vectors` at
// api/db/services/dialog_service.py:62-106.
//
// The vector dimension is auto-detected from chunks that already carry a
// vector. If no chunk has a vector yet, no fetch is attempted (returns 0).
//
// Returns the number of chunks that gained a vector.
//
// Skips:
//   - chunks that already have a non-empty `vector`
//   - chunks without a `chunk_id`
//
// Errors are non-fatal: caller logs and proceeds with whatever vectors
// are available. InsertCitations tolerates missing vectors by falling
// back to token-only similarity (when the chat's
// vector_similarity_weight allows).
//
// Parameters:
//   - tenantIDs: tenant ID(s) to derive index/table names (ragflow_<tid>).
//     If empty, no fetch is attempted.
func HydrateChunkVectors(ctx context.Context, kbinfos map[string]interface{}, tenantIDs []string, kbIDs []string, docEngine engine.DocEngine) (int, error) {
	if kbinfos == nil {
		return 0, nil
	}
	chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{})
	if !ok || len(chunksRaw) == 0 {
		return 0, nil
	}
	if docEngine == nil {
		docEngine = engine.Get()
	}
	if docEngine == nil {
		return 0, nil
	}

	// Auto-detect vector dimension from chunks that already carry a
	// vector. If none do, there is nothing to hydrate against.
	var dim int
	var missing []string
	for _, cm := range chunksRaw {
		if cv, ok := cm["vector"].([]float64); ok && len(cv) > 0 {
			if dim == 0 {
				dim = len(cv)
			}
			continue
		}
		if cid, ok := cm["chunk_id"].(string); ok && cid != "" {
			missing = append(missing, cid)
		}
	}
	if len(missing) == 0 || dim == 0 || len(tenantIDs) == 0 {
		return 0, nil
	}

	// Use RetrievalService which mirrors Python's Dealer.fetch_chunk_vectors.
	retrievalSvc := nlp.NewRetrievalService(docEngine, dao.NewDocumentDAO())
	vectors, err := retrievalSvc.FetchChunkVectors(ctx, missing, tenantIDs, kbIDs, dim)
	if err != nil {
		common.Warn("HydrateChunkVectors: FetchChunkVectors failed", zap.Error(err))
		return 0, err
	}

	// Stitch the vectors back onto the chunks.
	hits := 0
	for _, cm := range chunksRaw {
		if cv, ok := cm["vector"].([]float64); ok && len(cv) > 0 {
			continue
		}
		cid, _ := cm["chunk_id"].(string)
		if cid == "" {
			continue
		}
		vec, ok := vectors[cid]
		if !ok || len(vec) == 0 {
			continue
		}
		cm["vector"] = vec
		hits++
	}
	common.Debug("HydrateChunkVectors complete",
		zap.Int("hits", hits), zap.Int("requested", len(missing)))
	return hits, nil
}

// embeddingModelEmbedder adapts an EmbeddingModel to the Embedder interface.
type embeddingModelEmbedder struct {
	embModel *modelModule.EmbeddingModel
}

func (e *embeddingModelEmbedder) Encode(texts []string) ([][]float64, error) {
	config := &modelModule.EmbeddingConfig{Dimension: 0}
	embeds, err := e.embModel.ModelDriver.Embed(e.embModel.ModelName, texts, e.embModel.APIConfig, config)
	if err != nil {
		return nil, err
	}
	vecs := make([][]float64, len(embeds))
	for i, v := range embeds {
		vecs[i] = v.Embedding
	}
	return vecs, nil
}

// decorateAnswer applies citation insertion, reference construction,
// timing stats, token accounting, TTS, and Langfuse generation end to
// the final answer.
//
// P1: the `timer` parameter carries the per-phase durations emitted in the
// `## Time elapsed:` block of the prompt. Caller must have called
// timer.Exit() for PhaseGenerateAnswer before invoking this function.
func (s *ChatPipelineService) decorateAnswer(
	ctx context.Context,
	answer string,
	kbinfos map[string]interface{},
	prompt string,
	questions []string,
	usedTokenCount int,
	timer *common.Timer,
	embModel *modelModule.EmbeddingModel,
	vectorSimilarityWeight float64,
	quote bool,
	ttsModel *modelModule.ChatModel,
	langfuseTraceID string,
	llmModelConfig map[string]interface{},
	tenantID string,
	tenantIDs []string,
	hasKnowledges bool,
) AsyncChatResult {

	// Handle think markers: split on </think>.
	think := ""
	ans := answer
	if strings.Contains(answer, "</think>") {
		parts := strings.Split(answer, "</think>")
		if len(parts) == 2 {
			think = parts[0] + "</think>"
			ans = strings.TrimSpace(parts[1])
		}
	}

	var citationIdx map[int]struct{}
	var refs map[string]interface{}
	// Citation insertion: encode answer sentences, score against chunks,
	// and insert [ID:N] markers. Mirrors Python's insert_citations().
	//
	// P0.11 (CITATION_MARKER_PATTERN pre-check): if the LLM already emitted
	// citation markers in canonical or near-canonical form, skip
	// insertCitations to avoid double-tagging. Mirrors
	// dialog_service.py:790-802.
	if hasKnowledges && quote {
		chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{})
		if ok && len(chunksRaw) > 0 {
			// P7 — _hydrate_chunk_vectors. Mirrors
			// dialog_service.py:794. If any chunk lacks a `vector`
			// field (true for the ES path; Infinity ships vectors
			// inline), fetch them in one batched engine call. We only
			// need this when we'll actually call insertCitations
			// (i.e., the LLM didn't already emit markers).
			if embModel != nil && !HasCitationMarkers(ans) {
				if _, err := HydrateChunkVectors(ctx, kbinfos, tenantIDs, nil, engine.Get()); err != nil {
					common.Warn("hydrate chunk vectors failed", zap.Error(err))
				}
			}
			if embModel != nil && !HasCitationMarkers(ans) {
				// Build chunkVectors aligned with chunksRaw.
				chunkVectors := make([][]float64, len(chunksRaw))
				allVec := len(chunksRaw) > 0
				for i, cm := range chunksRaw {
					cv, _ := cm["vector"].([]float64)
					chunkVectors[i] = cv
					if len(cv) == 0 {
						allVec = false
					}
				}
				if allVec {
					embedder := &embeddingModelEmbedder{embModel: embModel}
					if decorated, cited := InsertCitations(ans, NewSourcedChunks(chunksRaw), embedder, chunkVectors); len(cited) > 0 {
						ans = decorated
						citationIdx = make(map[int]struct{})
						for _, ci := range cited {
							citationIdx[ci] = struct{}{}
						}
					}
				}
			} else {
				// P0.11 pre-check matched: collect indices from existing
				// markers instead of calling insertCitations.
				for _, ci := range ExtractCitationMarkers(ans, len(chunksRaw)) {
					if citationIdx == nil {
						citationIdx = make(map[int]struct{})
					}
					citationIdx[ci] = struct{}{}
				}
			}
		}

		// repair_bad_citation_formats — runs even when chunks are empty.
		// Mirrors dialog_service.py:818.
		if ok {
			ans = RepairBadCitationFormats(ans)
			for _, ci := range ExtractCitationMarkers(ans, len(chunksRaw)) {
				if citationIdx == nil {
					citationIdx = make(map[int]struct{})
				}
				citationIdx[ci] = struct{}{}
			}
		}

		// Map cited chunk indices to doc_ids and filter doc_aggs.
		// Mirrors dialog_service.py:820-824.
		if len(citationIdx) > 0 {
			citedDocIDs := make(map[string]struct{})
			if chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{}); ok {
				for ci := range citationIdx {
					if ci >= 0 && ci < len(chunksRaw) {
						cm := chunksRaw[ci]
						if docID, ok := cm["doc_id"].(string); ok && docID != "" {
							citedDocIDs[docID] = struct{}{}
						}
					}
				}
			}
			if len(citedDocIDs) > 0 {
				if docAggsRaw, ok := kbinfos["doc_aggs"].([]interface{}); ok && len(docAggsRaw) > 0 {
					var filtered []interface{}
					for _, da := range docAggsRaw {
						if dam, ok := da.(map[string]interface{}); ok {
							if docID, ok := dam["doc_id"].(string); ok {
								if _, cited := citedDocIDs[docID]; cited {
									filtered = append(filtered, da)
								}
							}
						}
					}
					if len(filtered) > 0 {
						kbinfos["doc_aggs"] = filtered
					}
				}
			}
		}
	}

	// Build refs: deepcopy kbinfos and strip vectors — done whenever
	// hasKnowledges is true, regardless of quote flag.
	// Mirrors dialog_service.py:826-829.
	if hasKnowledges {
		refs = make(map[string]interface{})
		for k, v := range kbinfos {
			refs[k] = v
		}
		if chunksRaw, ok := refs["chunks"].([]map[string]interface{}); ok {
			newChunks := make([]map[string]interface{}, 0, len(chunksRaw))
			for _, cm := range chunksRaw {
				newChunk := make(map[string]interface{})
				for ck, cv := range cm {
					if ck == "vector" {
						continue
					}
					newChunk[ck] = cv
				}
				newChunks = append(newChunks, newChunk)
			}
			refs["chunks"] = chunksFormat(newChunks)
		}
	}

	// Check for invalid API key errors (outside knowledges guard).
	// Mirrors dialog_service.py:831-832.
	if strings.Contains(strings.ToLower(ans), "invalid key") ||
		strings.Contains(strings.ToLower(ans), "invalid api") {
		ans += " Please set LLM API-Key in 'User Setting -> Model providers -> API-Key'"
	}

	finishChatTs := time.Now()

	// Build timing stats.
	// P1: emit Timer.Markdown() (6 phase lines + Total) and then the
	// token-count / token-speed lines that the existing OpenAI endpoint
	// already exposes. Total wall-clock is rounded to ms.
	totalMs := timer.Total().Seconds() * 1000
	tkNum := graph.NumTokensFromString(think + ans)

	prompt += fmt.Sprintf("\n\n### Query:\n%s", strings.Join(questions, " "))

	timeStats := prompt + timer.Markdown() + "\n"
	timeStats += fmt.Sprintf("  - Generated tokens(approximately): %d\n", tkNum)
	if totalMs > 0 {
		timeStats += fmt.Sprintf("  - Token speed: %d/s", int(float64(tkNum)/(totalMs/1000.0)))
	}

	// TTS synthesis for the final answer.
	audioBinary := s.synthesizeTTS(ttsModel, think+ans)

	// Langfuse generation end observation.
	if langfuseTraceID != "" {
		if lfClient, ok := ctx.Value(langfuseCtxKey).(*LangfuseClient); ok && lfClient != nil {
			// Mirrors dialog_service.py:853-854. Python extracts
			// everything from `### Query:` onwards (the time-elapsed
			// + token-usage block) and replaces \n with "  \n" for
			// markdown line breaks.
			langfuseOutput := langfuseExtractTimeElapsed(timeStats)
			usage := &LangfuseUsage{
				PromptTokens:     usedTokenCount,
				CompletionTokens: tkNum,
				TotalTokens:      usedTokenCount + tkNum,
			}
			modelName := ""
			if llmModelConfig != nil {
				if mn, ok := llmModelConfig["llm_name"].(string); ok {
					modelName = mn
				}
			}
			_ = lfClient.PostGeneration(ctx, LangfuseGeneration{
				ID:        fmt.Sprintf("gen-%s", langfuseTraceID),
				TraceID:   langfuseTraceID,
				Name:      "chat",
				Model:     modelName,
				StartTime: time.Now().UTC().Format(time.RFC3339Nano),
				EndTime:   time.Now().UTC().Format(time.RFC3339Nano),
				Output:    langfuseOutput,
				Usage:     usage,
			})
		}
	}

	return AsyncChatResult{
		Answer:      think + ans,
		Reference:   refs,
		AudioBinary: audioBinary,
		// Fix 7: Apply the markdown line-break substitution
		// re.sub(r"\n", "  \n", prompt) at the very end, matching
		// dialog_service.py:865. This converts single \n to "  \n"
		// so multi-line prompt text renders as a single markdown
		// paragraph instead of being broken into separate lines.
		Prompt:    strings.ReplaceAll(timeStats, "\n", "  \n"),
		CreatedAt: float64(finishChatTs.Unix()),
		Final:     false, // caller sets Final = true
	}
}

// langfuseExtractTimeElapsed extracts the time-elapsed + token-usage
// block from the prompt and applies the \n → "  \n" substitution.
// Mirrors dialog_service.py:853-854:
//
//	langfuse_output = "\n" + re.sub(r"^.*?(### Query:.*)", r"\1", prompt, flags=re.DOTALL)
//	langfuse_output = {"time_elapsed:": re.sub(r"\n", "  \n", langfuse_output), ...}
func langfuseExtractTimeElapsed(prompt string) string {
	const marker = "### Query:"
	idx := strings.Index(prompt, marker)
	if idx < 0 {
		// Fallback: return the whole prompt with \n substitution.
		return strings.ReplaceAll(prompt, "\n", "  \n")
	}
	return strings.ReplaceAll(prompt[idx:], "\n", "  \n")
}

// extractVisibleAnswer mirrors Python's _extract_visible_answer.
func (s *ChatPipelineService) extractVisibleAnswer(text string) string {
	return ExtractVisibleAnswer(text)
}

// citationPrompt returns the citation instruction prompt.
// Mirrors Python's citation_prompt() in rag/prompts/generator.py.
func citationPrompt() string {
	return "\n\n### Citation\nWhen answering, please cite sources using the format [ID:N] " +
		"(where N is the chunk number) after each sentence where the information from that chunk is used."
}

// -----------------------------------------------------------------------
// Moved from sql_fallback.go (2026-06-12). SQL retrieval system, repair
// helpers, and Python parity helpers. Kept in async_chat.go because the
// orchestrator entry point is s.useSQL at async_chat.go:319.
// -----------------------------------------------------------------------

// SQL retrieval system + user prompts, dispatched by engine type.
// Mirrors dialog_service.py:1031-1105. The Go port previously used a
// single engine-agnostic prompt, which made Infinity/OceanBase queries
// fail because the LLM didn't know to use json_extract_string. These
// constants restore parity with Python's three-way engine dispatch.

// infinitySQLSysPrompt is for Infinity's JSON 'chunk_data' column.
// References docnm (no _kwd suffix) per the Python prompt at
// dialog_service.py:1035-1052.
const infinitySQLSysPrompt = `You are a Database Administrator. Write SQL for a table with JSON 'chunk_data' column.

JSON Extraction: json_extract_string(chunk_data, '$.FieldName')
Numeric Cast: CAST(json_extract_string(chunk_data, '$.FieldName') AS INTEGER/FLOAT)
NULL Check: json_extract_isnull(chunk_data, '$.FieldName') == false

RULES:
1. Use EXACT field names (case-sensitive) from the list below
2. For SELECT: include doc_id, docnm, and json_extract_string() for requested fields
3. For COUNT: use COUNT(*) or COUNT(DISTINCT json_extract_string(...))
4. Add AS alias for extracted field names
5. DO NOT select 'content' field
6. Only add NULL check (json_extract_isnull() == false) in WHERE clause when:
   - Question asks to "show me" or "display" specific columns
   - Question mentions "not null" or "excluding null"
   - Add NULL check for count specific column
   - DO NOT add NULL check for COUNT(*) queries (COUNT(*) counts all rows including nulls)
7. json_extract_string() returns JSON-quoted strings ("value"), so WHERE comparisons MUST wrap values in double-quotes inside single-quotes (no spaces between quotes): '"value"' (e.g. WHERE json_extract_string(chunk_data, '$.name') = '"Alice"')
8. For partial text search, use LIKE with wildcards: '"%value%"' (e.g. WHERE json_extract_string(chunk_data, '$.name') LIKE '"%Alice%"')
9. Output ONLY the SQL, no explanations`

// infinitySQLUserPromptTemplate has 4 %s placeholders:
// table_name, comma-joined field names, bullet list of field names,
// question. Mirrors dialog_service.py:1053-1059.
const infinitySQLUserPromptTemplate = `Table: %s
Fields (EXACT case): %s
%s
Question: %s
Write SQL using json_extract_string() with exact field names. Include doc_id, docnm for data queries. Only SQL.`

// oceanbaseSQLSysPrompt is identical to Infinity's but uses docnm_kwd
// (the _kwd suffix is the OceanBase convention). Mirrors
// dialog_service.py:1064-1081.
const oceanbaseSQLSysPrompt = `You are a Database Administrator. Write SQL for a table with JSON 'chunk_data' column.

JSON Extraction: json_extract_string(chunk_data, '$.FieldName')
Numeric Cast: CAST(json_extract_string(chunk_data, '$.FieldName') AS INTEGER/FLOAT)
NULL Check: json_extract_isnull(chunk_data, '$.FieldName') == false

RULES:
1. Use EXACT field names (case-sensitive) from the list below
2. For SELECT: include doc_id, docnm_kwd, and json_extract_string() for requested fields
3. For COUNT: use COUNT(*) or COUNT(DISTINCT json_extract_string(...))
4. Add AS alias for extracted field names
5. DO NOT select 'content' field
6. Only add NULL check (json_extract_isnull() == false) in WHERE clause when:
   - Question asks to "show me" or "display" specific columns
   - Question mentions "not null" or "excluding null"
   - Add NULL check for count specific column
   - DO NOT add NULL check for COUNT(*) queries (COUNT(*) counts all rows including nulls)
7. Output ONLY the SQL, no explanations`

// oceanbaseSQLUserPromptTemplate — same shape as Infinity, docnm_kwd in
// the trailing sentence. Mirrors dialog_service.py:1082-1088.
const oceanbaseSQLUserPromptTemplate = `Table: %s
Fields (EXACT case): %s
%s
Question: %s
Write SQL using json_extract_string() with exact field names. Include doc_id, docnm_kwd for data queries. Only SQL.`

// esSQLSysPrompt is for Elasticsearch / OpenSearch / default engines
// where fields are direct columns (no JSON extraction). Mirrors
// dialog_service.py:1092-1100.
const esSQLSysPrompt = `You are a Database Administrator. Write SQL queries.

RULES:
1. Use EXACT field names from the schema below (e.g., product_tks, not product)
2. Quote field names starting with digit: "123_field"
3. Add IS NOT NULL in WHERE clause when:
   - Question asks to "show me" or "display" specific columns
4. Include doc_id/docnm in non-aggregate statement
5. Output ONLY the SQL, no explanations`

// esSQLUserPromptTemplate — 3 %s placeholders: table_name, bullet
// list with types, question. Mirrors dialog_service.py:1101-1105.
const esSQLUserPromptTemplate = `Table: %s
Available fields:
%s
Question: %s
Write SQL using exact field names above. Include doc_id, docnm_kwd for data queries. Only SQL.`

// SQL retrieval repair prompts, split into TWO flows × TWO engine
// families, mirroring dialog_service.py:repair_table_for_missing_source_columns
// (lines 1129-1156) and the execution-error retry at lines 1164-1205.
//
// The previous single-template repair was generic and could not tell
// the LLM to keep using json_extract_string on Infinity, which led
// to fragile repairs. Per-engine prompts make the syntax intent
// explicit on the repair path too.
//
// Flow A (missing-source-columns): the SQL executed successfully but
// the result set is missing doc_id / docnm* columns. We call the LLM
// to rewrite the SQL with those columns added.
// Flow B (execution-error): the SQL failed to execute at all
// (syntax error, unknown column, etc.). We call the LLM with the
// error message and ask for a corrected SQL.
//
// Engine family A (Infinity / OceanBase): data lives in a JSON
// 'chunk_data' column, so JSON-extraction syntax must be preserved.
// Engine family B (Elasticsearch / OpenSearch / default): fields
// are direct columns.

// infinityMissingColumnsRepairPromptTemplate — 5 %s args:
// table_name, JSON field bullets, question, previous_sql,
// expected_doc_name_column. Mirrors dialog_service.py:1132-1143.
// OceanBase shares this template (line 1130 dispatch) with
// expected_doc_name_column="docnm_kwd" instead of "docnm".
const infinityMissingColumnsRepairPromptTemplate = `Table name: %s;
JSON fields available in 'chunk_data' column (use exact names):
%s

Question: %s
Previous SQL:
%s

The previous SQL result is missing required source columns for citations.
Rewrite SQL to keep the same query intent and include doc_id and %s in the SELECT list.
For extracted JSON fields, use json_extract_string(chunk_data, '$.field_name').
Return ONLY SQL.`

// esMissingColumnsRepairPromptTemplate — 4 %s args: table_name,
// ES field bullets (with types), question, previous_sql. Mirrors
// dialog_service.py:1145-1155.
const esMissingColumnsRepairPromptTemplate = `Table name: %s
Available fields:
%s

Question: %s
Previous SQL:
%s

The previous SQL result is missing required source columns for citations.
Rewrite SQL to keep the same query intent and include doc_id and docnm_kwd in the SELECT list.
Return ONLY SQL.`

// infinityExecutionErrorRepairPromptTemplate — 4 %s args:
// table_name, JSON field bullets, question, error. Mirrors
// dialog_service.py:1168-1181. Used for both Infinity and OceanBase
// (line 1165 dispatch).
const infinityExecutionErrorRepairPromptTemplate = `
Table name: %s;
JSON fields available in 'chunk_data' column (use these exact names in json_extract_string):
%s

Question: %s
Please write the SQL using json_extract_string(chunk_data, '$.field_name') with the field names from the list above. Only SQL, no explanations.


The SQL error you provided last time is as follows:
%s

Please correct the error and write SQL again using json_extract_string(chunk_data, '$.field_name') syntax with the correct field names. Only SQL, no explanations.`

// esExecutionErrorRepairPromptTemplate — 4 %s args: table_name,
// ES field bullets (with types), question, error. Mirrors
// dialog_service.py:1184-1198.
const esExecutionErrorRepairPromptTemplate = `
Table name: %s;
Table of database fields are as follows (use the field names directly in SQL):
%s

Question are as follows:
%s
Please write the SQL using the exact field names above, only SQL, without any other explanations or text.


The SQL error you provided last time is as follows:
%s

Please correct the error and write SQL again using the exact field names above, only SQL, without any other explanations or text.`

// useSQL is the Go port of dialog_service.use_sql
// (api/db/services/dialog_service.py:914-1226). It branches on the
// active document engine, asks the chat model to produce SQL,
// optionally repairs it once, and executes the query.
//
// The caller is responsible for resolving fieldMap (typically via
// s.KbService.GetFieldMap in AsyncChat before invoking this) so the
// structured-schema lookup happens once per request and is observable
// in logs at the AsyncChat call site. Pass nil/empty to short-circuit.
//
// Returns:
//
//   - ans: a map mirroring the Python use_sql return shape:
//
//     {"answer": <string>, "reference": {"chunks": [], "doc_aggs": [], "total": <int>}}
//
//     or nil when SQL retrieval doesn't apply / produced no usable
//     result. The caller checks `ans != nil && (ans["answer"] != ""
//     or non-empty chunks)` to decide whether to short-circuit.
//
//   - err: non-nil when something went wrong; caller should log and fall
//     through.
func (s *ChatPipelineService) useSQL(
	ctx context.Context,
	chat *entity.Chat,
	kbs []*entity.Knowledgebase,
	question string,
	chatModel *modelModule.ChatModel,
	fieldMap map[string]interface{},
	quote bool,
) (ans map[string]interface{}, err error) {
	if chat == nil || chatModel == nil || len(kbs) == 0 {
		return nil, nil
	}

	if fieldMap == nil || len(fieldMap) == 0 {
		// No structured schema → SQL retrieval doesn't apply.
		return nil, nil
	}

	docEngine := engine.Get()
	if docEngine == nil {
		return nil, nil
	}

	// Entry log. Mirrors `logging.debug(f"use_sql: Question: {question}")`
	// at dialog_service.py:934.
	common.Debug("SQL retrieval: question", zap.String("question", question))

	// Build the table name. Infinity: ragflow_{tenant}_{kb_id} (one per
	// KB). ES: ragflow_{tenant} (kb_id in WHERE).
	tableName := ragflowTableName(chat.TenantID, kbs, docEngine)

	// Build engine-specific prompts. Mirrors the three-way dispatch
	// at dialog_service.py:1031-1105.
	engineName := docEngine.GetType()
	sysPrompt, userPrompt, overrideSQL := buildSQLPrompts(engineName, tableName, question, fieldMap)

	// Step 1: generate SQL. If the question is a "how many rows in the
	// dataset" row-count question, buildSQLPrompts returns a hard-coded
	// override and we skip the LLM call entirely (matches Python
	// row_count_override at dialog_service.py:1034/1063).
	var sqlText string
	if overrideSQL != "" {
		sqlText = normalizeSQL(overrideSQL)
		common.Debug("SQL retrieval: using row-count override",
			zap.String("sql", sqlText))
	} else {
		var sqlErr error
		sqlText, sqlErr = generateSQL(ctx, chatModel, sysPrompt, userPrompt)
		if sqlErr != nil {
			common.Warn("SQL retrieval: LLM generation failed", zap.Error(sqlErr))
			return nil, nil
		}
	}

	// Step 1.5: inject the kb_id WHERE filter for ES / OS / OceanBase.
	// No-op for Infinity (the table name already encodes the KB scope).
	// Mirrors add_kb_filter at dialog_service.py:992-1021, called from
	// get_table right after normalize_sql.
	if filtered, ok := addKBFilter(sqlText, engineName, kbs); ok {
		sqlText = filtered
	} else {
		common.Warn("SQL retrieval: invalid kb_id UUID; SQL will run unfiltered")
	}

	// Step 2: try to execute. On failure, repair once with the
	// engine-specific execution-error prompt so the LLM regenerates
	// correctly (Flow B at dialog_service.py:1164-1205).
	rows, execErr := docEngine.RunSQL(ctx, tableName, sqlText, kbIDStrings(kbs), "json")
	if execErr != nil {
		common.Debug("SQL retrieval: initial execution failed, attempting repair",
			zap.String("sql", sqlText), zap.Error(execErr))
		repaired, repairErr := repairSQLForExecutionError(
			ctx, chatModel, sysPrompt, tableName, question, execErr.Error(), engineName, fieldMap,
		)
		if repairErr != nil {
			common.Warn("SQL retrieval: repair failed", zap.Error(repairErr))
			return nil, nil
		}
		// Re-apply the kb filter after the LLM-driven repair.
		if filtered, ok := addKBFilter(repaired, engineName, kbs); ok {
			repaired = filtered
		}
		rows, execErr = docEngine.RunSQL(ctx, tableName, repaired, kbIDStrings(kbs), "json")
		if execErr != nil {
			common.Warn("SQL retrieval: repaired SQL also failed", zap.Error(execErr))
			return nil, nil
		}
	}
	if len(rows) == 0 {
		common.Debug("SQL retrieval: execution succeeded but returned 0 rows")
		// Empty result set; let vector retrieval try.
		return nil, nil
	}

	// Step 3 (Python parity): for non-aggregate SQL, check that the
	// result has source-citation columns (Flow A at
	// dialog_service.py:1211-1221). If missing, call the LLM to
	// rewrite the SQL with the right columns and retry once. If the
	// repair doesn't yield source columns, fall through to the
	// best-effort answer (matches Python's `returning best-effort
	// answer` log at line 1221).
	if !isAggregateSQL(sqlText) && !hasSourceColumns(rows) {
		common.Debug("SQL retrieval: result missing source columns; attempting repair",
			zap.String("sql", sqlText))
		expectedCol := expectedDocNameColumn(engineName)
		repaired, repairErr := repairSQLForMissingColumns(
			ctx, chatModel, sysPrompt, tableName, question, sqlText, expectedCol, engineName, fieldMap,
		)
		if repairErr == nil && repaired != "" {
			// Re-apply the kb filter after the LLM-driven repair.
			if filtered, ok := addKBFilter(repaired, engineName, kbs); ok {
				repaired = filtered
			}
			repairedRows, repairedErr := docEngine.RunSQL(ctx, tableName, repaired, kbIDStrings(kbs), "json")
			if repairedErr == nil && len(repairedRows) > 0 && hasSourceColumns(repairedRows) {
				common.Debug("SQL retrieval: missing-columns repair succeeded",
					zap.String("sql", repaired))
				rows = repairedRows
				sqlText = repaired
			} else {
				common.Warn("SQL retrieval: missing-columns repair did not yield source columns; using best-effort answer",
					zap.String("sql", repaired))
			}
		} else if repairErr != nil {
			common.Warn("SQL retrieval: missing-columns repair failed; using best-effort answer",
				zap.Error(repairErr))
		}
	}

	// 4. Build the answer and reference from the rows. Mirrors Python's
	// `return {"answer": ..., "reference": {"chunks": ...,
	// "doc_aggs": ...}, "prompt": sys_prompt}` at dialog_service.py:1361
	// and 1377-1401. buildSQLReference handles all three branches:
	// primary (rows have source columns), aggregate secondary fetch,
	// and best-effort empty refs.
	answerStr, ref := s.buildSQLReference(
		ctx, docEngine, tableName, sqlText, rows,
		sysPrompt, engineName, kbs, fieldMap,
	)
	return map[string]interface{}{
		"answer":    answerStr,
		"reference": ref,
		"prompt":    sysPrompt,
	}, nil
}

// ragflowTableName returns the engine-specific SQL target name.
// Mirrors dialog_service.py:954-963. For Infinity with a single KB,
// validates the kb_id is a canonical UUID before interpolating
// (SQL injection guard matching _assert_valid_uuid at
// dialog_service.py:944-949).
func ragflowTableName(tenantID string, kbs []*entity.Knowledgebase, docEngine engine.DocEngine) string {
	if docEngine == nil {
		return "ragflow_" + tenantID
	}
	engineName := docEngine.GetType()
	if engineName == "infinity" && len(kbs) == 1 {
		if !isValidUUID(kbs[0].ID) {
			common.Warn("ragflowTableName: invalid kb_id; falling back to base index",
				zap.String("kb_id", kbs[0].ID))
			return "ragflow_" + tenantID
		}
		return fmt.Sprintf("ragflow_%s_%s", tenantID, kbs[0].ID)
	}
	// Elasticsearch / OpenSearch / default: single index, kb_id in WHERE.
	return "ragflow_" + tenantID
}

// isValidUUID returns true if s is a canonical UUID string (8-4-4-4-12
// hex format). Used to validate kb_id before SQL interpolation, matching
// Python's _assert_valid_uuid (dialog_service.py:944-949).
var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}(-?[0-9a-fA-F]{4}){3}-?[0-9a-fA-F]{12}$`)

func isValidUUID(s string) bool {
	if s == "" {
		return false
	}
	return uuidRe.MatchString(s)
}

// addKBFilter injects a validated kb_id WHERE filter into sqlText for
// ES / OS / OceanBase engines. Infinity is a no-op because the table
// name already encodes the KB scope. Mirrors dialog_service.py:992-1021.
//
// Returns the (possibly modified) SQL and a boolean indicating whether
// all kb_ids passed UUID validation. When validation fails, the SQL is
// returned unchanged — the engine will likely reject the un-filtered
// query, triggering the repair path (Python's `_assert_valid_uuid` raises
// ValueError, which `get_table`'s try/except catches and routes to the
// repair flow).
//
// If the SQL already has a WHERE clause with `kb_id =`, the filter is
// not duplicated. Otherwise a fresh WHERE is appended, or `kb_id = '...'
// AND` is prepended to an existing WHERE.
func addKBFilter(sqlText, engineName string, kbs []*entity.Knowledgebase) (string, bool) {
	if engineName == "infinity" || len(kbs) == 0 {
		return sqlText, true
	}

	// Validate all kb_ids as UUIDs.
	for _, kb := range kbs {
		if kb == nil || !isValidUUID(kb.ID) {
			return sqlText, false
		}
	}

	kbIDs := kbIDStrings(kbs)
	var kbFilter string
	if len(kbIDs) == 1 {
		kbFilter = fmt.Sprintf("kb_id = '%s'", kbIDs[0])
	} else {
		parts := make([]string, len(kbIDs))
		for i, kid := range kbIDs {
			parts[i] = fmt.Sprintf("kb_id = '%s'", kid)
		}
		kbFilter = "(" + strings.Join(parts, " OR ") + ")"
	}

	lower := strings.ToLower(sqlText)
	if !strings.Contains(lower, "where ") {
		// No WHERE clause: append one. Honor ORDER BY if present.
		if oIdx := strings.Index(lower, "order by"); oIdx >= 0 {
			sqlText = sqlText[:oIdx] + " WHERE " + kbFilter + "  order by " + sqlText[oIdx+len("order by"):]
		} else {
			sqlText += " WHERE " + kbFilter
		}
	} else if !strings.Contains(lower, "kb_id =") && !strings.Contains(lower, "kb_id=") {
		// Has WHERE but no kb_id: insert "kb_id = '...' AND" after WHERE.
		whereRe := regexp.MustCompile(`(?i)\bwhere\b `)
		sqlText = whereRe.ReplaceAllString(sqlText, "where "+kbFilter+" and ")
	}
	return sqlText, true
}

// generateSQL calls the chat model to produce a SQL SELECT.
// sysPrompt and userPrompt are pre-built by buildSQLPrompts and already
// carry engine-specific instructions (json_extract_string for Infinity/
// OceanBase, direct column access for ES/OS). Thin wrapper over
// chatForSQL.
func generateSQL(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	sysPrompt, userPrompt string,
) (string, error) {
	return chatForSQL(ctx, chatModel, sysPrompt, userPrompt, "sql generation")
}

// buildSQLPrompts returns the (system, user) prompt pair for the
// active document engine, plus an optional row-count override SQL.
// The override is non-empty only for "how many rows in the dataset/
// table/spreadsheet/excel" questions, matching Python's
// row_count_override at dialog_service.py:1034 and 1063.
//
// engineName comes from docEngine.GetType() and is one of:
// "infinity", "oceanbase", "elasticsearch", "opensearch", or any
// other value (treated as the ES/OS default).
//
// Field names are sorted alphabetically for stable test output and
// to match the order-independent iteration of Python's dict.
func buildSQLPrompts(engineName, tableName, question string, fieldMap map[string]interface{}) (sysPrompt, userPrompt, overrideSQL string) {
	names := make([]string, 0, len(fieldMap))
	for k := range fieldMap {
		names = append(names, k)
	}
	sort.Strings(names)

	switch engineName {
	case "infinity":
		sysPrompt = infinitySQLSysPrompt
		bullets := strings.Builder{}
		for _, n := range names {
			bullets.WriteString("  - " + n + "\n")
		}
		userPrompt = fmt.Sprintf(
			infinitySQLUserPromptTemplate,
			tableName,
			strings.Join(names, ", "),
			strings.TrimRight(bullets.String(), "\n"),
			question,
		)
		if isRowCountQuestion(question) {
			overrideSQL = fmt.Sprintf("SELECT COUNT(*) AS rows FROM %s", tableName)
		}
	case "oceanbase":
		sysPrompt = oceanbaseSQLSysPrompt
		bullets := strings.Builder{}
		for _, n := range names {
			bullets.WriteString("  - " + n + "\n")
		}
		userPrompt = fmt.Sprintf(
			oceanbaseSQLUserPromptTemplate,
			tableName,
			strings.Join(names, ", "),
			strings.TrimRight(bullets.String(), "\n"),
			question,
		)
		if isRowCountQuestion(question) {
			overrideSQL = fmt.Sprintf("SELECT COUNT(*) AS rows FROM %s", tableName)
		}
	default:
		// Elasticsearch / OpenSearch / unknown — direct column access.
		sysPrompt = esSQLSysPrompt
		bullets := strings.Builder{}
		for _, n := range names {
			bullets.WriteString(fmt.Sprintf("  - %s (%v)\n", n, fieldMap[n]))
		}
		userPrompt = fmt.Sprintf(
			esSQLUserPromptTemplate,
			tableName,
			strings.TrimRight(bullets.String(), "\n"),
			question,
		)
	}
	return
}

// isRowCountQuestion returns true when the question is asking for a
// total row count of a dataset/table. Mirrors Python's
// is_row_count_question at dialog_service.py:1023-1028. Uses word-
// boundary regex (not Contains) to match the Python implementation.
var rowCountPhraseRe = regexp.MustCompile(`(?i)\b(how many rows|number of rows|row count)\b`)
var rowCountSubjectRe = regexp.MustCompile(`(?i)\b(dataset|table|spreadsheet|excel)\b`)

func isRowCountQuestion(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	return rowCountPhraseRe.MatchString(q) && rowCountSubjectRe.MatchString(q)
}

// -----------------------------------------------------------------------
// Repair helpers (Python parity: dialog_service.py:1129-1205)
// -----------------------------------------------------------------------

// expectedDocNameColumn returns the column name the engine uses for
// the document name in source-citation joins. "docnm" for Infinity
// (no _kwd suffix), "docnm_kwd" for OceanBase / ES / OS / default.
// Mirrors dialog_service.py:965.
func expectedDocNameColumn(engineName string) string {
	if engineName == "infinity" {
		return "docnm"
	}
	return "docnm_kwd"
}

// hasSourceColumns reports whether the SQL result has the columns
// needed to build source citations: doc_id and (docnm OR docnm_kwd).
// Mirrors dialog_service.py:967-970. Returns false for empty rows
// (no schema to inspect).
func hasSourceColumns(rows []map[string]interface{}) bool {
	if len(rows) == 0 {
		return false
	}
	names := map[string]bool{}
	for k := range rows[0] {
		names[strings.ToLower(k)] = true
	}
	if !names["doc_id"] {
		return false
	}
	return names["docnm_kwd"] || names["docnm"]
}

// isAggregateSQL reports whether the SQL contains an aggregate
// function call (count, sum, avg, max, min, distinct). Mirrors
// dialog_service.py:972-974.
var aggregateFnRe = regexp.MustCompile(`(?i)\b(count|sum|avg|max|min|distinct)\s*\(`)

func isAggregateSQL(sqlText string) bool {
	return aggregateFnRe.MatchString(sqlText)
}

// sortedFieldNames returns the field_map keys in alphabetical order.
// Used to format prompt bullets deterministically (matches Python's
// dict-iteration order on small maps, and gives stable test output).
func sortedFieldNames(fieldMap map[string]interface{}) []string {
	names := make([]string, 0, len(fieldMap))
	for k := range fieldMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// buildMissingColumnsRepairPrompt returns the engine-specific user
// prompt for the missing-source-columns repair flow. The Infinity
// and OceanBase branches share the JSON-column template; ES and
// OpenSearch share the direct-column template. expectedCol is
// "docnm" for Infinity or "docnm_kwd" for everything else.
func buildMissingColumnsRepairPrompt(engineName, tableName, question, prevSQL, expectedCol string, fieldMap map[string]interface{}) string {
	isJSONEngine := engineName == "infinity" || engineName == "oceanbase"
	names := sortedFieldNames(fieldMap)
	bullets := strings.Builder{}
	if isJSONEngine {
		for _, n := range names {
			bullets.WriteString("  - " + n + "\n")
		}
		return fmt.Sprintf(
			infinityMissingColumnsRepairPromptTemplate,
			tableName,
			strings.TrimRight(bullets.String(), "\n"),
			question, prevSQL, expectedCol,
		)
	}
	// ES / OS: include types in bullets
	for _, n := range names {
		bullets.WriteString(fmt.Sprintf("  - %s (%v)\n", n, fieldMap[n]))
	}
	return fmt.Sprintf(
		esMissingColumnsRepairPromptTemplate,
		tableName,
		strings.TrimRight(bullets.String(), "\n"),
		question, prevSQL,
	)
}

// buildExecutionErrorRepairPrompt returns the engine-specific user
// prompt for the execution-error repair flow.
func buildExecutionErrorRepairPrompt(engineName, tableName, question, errMsg string, fieldMap map[string]interface{}) string {
	isJSONEngine := engineName == "infinity" || engineName == "oceanbase"
	names := sortedFieldNames(fieldMap)
	bullets := strings.Builder{}
	if isJSONEngine {
		for _, n := range names {
			bullets.WriteString("  - " + n + "\n")
		}
		return fmt.Sprintf(
			infinityExecutionErrorRepairPromptTemplate,
			tableName,
			strings.TrimRight(bullets.String(), "\n"),
			question, errMsg,
		)
	}
	for _, n := range names {
		bullets.WriteString(fmt.Sprintf("  - %s (%v)\n", n, fieldMap[n]))
	}
	return fmt.Sprintf(
		esExecutionErrorRepairPromptTemplate,
		tableName,
		strings.TrimRight(bullets.String(), "\n"),
		question, errMsg,
	)
}

// chatForSQL is the shared chat-model invocation for SQL generation
// and both repair flows. Returns the cleaned (normalized) SQL or an
// error. errPrefix is included in error messages to disambiguate
// which flow failed ("sql generation", "sql repair", etc.).
func chatForSQL(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	sysPrompt, userPrompt, errPrefix string,
) (string, error) {
	if chatModel == nil || chatModel.ModelDriver == nil {
		return "", fmt.Errorf("nil chat model")
	}
	// Python uses 0.06 (dialog_service.py:1115) for all SQL LLM calls.
	// Match it for parity — 0.0 made the LLM deterministic but produced
	// SQL that diverged from the Python reference on some prompts.
	tempLow := 0.06
	cfg := &modelModule.ChatConfig{
		Temperature: &tempLow,
	}
	modelName := ""
	if chatModel.ModelName != nil {
		modelName = *chatModel.ModelName
	}
	msgs := []modelModule.Message{
		modelModule.Message{Role: "system", Content: sysPrompt},
		modelModule.Message{Role: "user", Content: userPrompt},
	}
	resp, err := chatModel.ModelDriver.ChatWithMessages(
		modelName, msgs, chatModel.APIConfig, cfg,
	)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Answer == nil {
		return "", fmt.Errorf("%s: empty response", errPrefix)
	}
	cleaned := normalizeSQL(*resp.Answer)
	if cleaned == "" {
		return "", fmt.Errorf("%s: empty after normalize", errPrefix)
	}
	return cleaned, nil
}

// repairSQLForExecutionError calls the LLM to fix SQL that the engine
// refused to execute (syntax error, unknown column, etc.). Engine-
// specific user prompt keeps the right syntax (json_extract_string
// on Infinity, direct field access on ES).
func repairSQLForExecutionError(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	sysPrompt, tableName, question, errMsg, engineName string,
	fieldMap map[string]interface{},
) (string, error) {
	userPrompt := buildExecutionErrorRepairPrompt(engineName, tableName, question, errMsg, fieldMap)
	return chatForSQL(ctx, chatModel, sysPrompt, userPrompt, "sql repair")
}

// repairSQLForMissingColumns calls the LLM to fix SQL whose result
// set is missing the source-citation columns (doc_id, expectedCol).
// expectedCol is "docnm" for Infinity or "docnm_kwd" for everything
// else — see expectedDocNameColumn.
func repairSQLForMissingColumns(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	sysPrompt, tableName, question, prevSQL, expectedCol, engineName string,
	fieldMap map[string]interface{},
) (string, error) {
	userPrompt := buildMissingColumnsRepairPrompt(engineName, tableName, question, prevSQL, expectedCol, fieldMap)
	return chatForSQL(ctx, chatModel, sysPrompt, userPrompt, "sql missing-columns repair")
}

// normalizeSQL strips LLM artifacts from a SQL response. Mirrors the
// helper at dialog_service.py:976-990.
func normalizeSQL(s string) string {
	if s == "" {
		return ""
	}
	// Remove <think>...</think> blocks.
	thinkRe := regexp.MustCompile(`(?s)<think>.*?</think>`)
	s = thinkRe.ReplaceAllString(s, "")
	// Also strip Chinese reasoning markers (思考...) — some models
	// (notably Qwen) emit these instead of <think>. Mirrors
	// dialog_service.py:985: `re.sub(r"思考\n.*?\n", "", ...)`.
	chineseThinkRe := regexp.MustCompile(`(?s)思考\n.*?\n`)
	s = chineseThinkRe.ReplaceAllString(s, "")
	// Strip Markdown code fences.
	fenceRe := regexp.MustCompile("(?i)```(?:sql)?\\s*")
	s = fenceRe.ReplaceAllString(s, "")
	fenceEnd := regexp.MustCompile("```\\s*$")
	s = fenceEnd.ReplaceAllString(s, "")
	// Trim trailing semicolons (ES SQL parser doesn't like them) and
	// outer whitespace.
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ";")
	return strings.TrimSpace(s)
}

// -----------------------------------------------------------------------
// Python parity helpers (dialog_service.py:56-59, 1238-1309, 1321-1365)
// -----------------------------------------------------------------------

// Redundant-space cleanup regexes. Mirrors
// common.string_utils.remove_redundant_spaces (string_utils.py:20-46).
// Pass 1: drop spaces after a "left boundary" character (parens, <, >).
// Pass 2: drop spaces before a "right boundary" character (parens, !).
var (
	redundantSpacePass1Re = regexp.MustCompile(`([^a-z0-9.,)>\x{ff08}]) +([^ ])`) // left boundary + space + non-space
	redundantSpacePass2Re = regexp.MustCompile(`([^ ]) +([^a-z0-9.,(<])`)         // non-space + space + right boundary
)

// removeRedundantSpaces ports common.string_utils.remove_redundant_spaces.
// Two-pass regex cleanup; both passes use case-insensitive matching.
func removeRedundantSpaces(s string) string {
	s = redundantSpacePass1Re.ReplaceAllString(s, "$1$2")
	s = redundantSpacePass2Re.ReplaceAllString(s, "$1$2")
	return s
}

// ISO timestamp stripping regex. Mirrors the cleanup at
// dialog_service.py:1309. Matches `T13:24:55|` or `T13:24:55.123Z|`.
var isoTimestampCellRe = regexp.MustCompile(`T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+Z)?\|`)

// stripISOTimestamps removes ISO-8601 timestamps that end a markdown
// table cell. Operates on the full joined rows string (not per-cell).
func stripISOTimestamps(rows string) string {
	return isoTimestampCellRe.ReplaceAllString(rows, "|")
}

// asAliasRe extracts the `AS alias` portion of a SQL column expression.
var asAliasRe = regexp.MustCompile(`(?i)\s+AS\s+([^\s,)]+)`)

// parenSuffixRe strips `/...` and Chinese-parenthesized suffixes from
// display names (matches the regex in dialog_service.py:1251, 1255, 1263,
// 1269, 1279). The CJK variant `（...）` is intentionally non-greedy
// and stops at the first nested `（` or `）`.
var parenSuffixRe = regexp.MustCompile(`(/.*|（[^（）]+）)`)

// cleanDisplay applies the Python suffix-cleanup regex to a display name.
func cleanDisplay(s string) string {
	return parenSuffixRe.ReplaceAllString(s, "")
}

// mapColumnName translates a raw SQL column name to a human-readable
// display name using the field_map. Mirrors
// dialog_service.py:1238-1280 exactly. Algorithm:
//  1. Special case: literal "count(star)" → "COUNT(*)".
//  2. Try to extract `AS alias`; if alias is in fieldMap, return its
//     cleaned display value (exact, then case-insensitive, then alias
//     unchanged).
//  3. No AS: try fieldMap[colName] exact, then case-insensitive.
//  4. Still no match: bulk-replace each fieldMap key with its display
//     value in the raw column name (handles bare json_extract_string
//     expressions without AS).
func mapColumnName(colName string, fieldMap map[string]interface{}) string {
	if strings.EqualFold(colName, "count(star)") {
		return "COUNT(*)"
	}
	if m := asAliasRe.FindStringSubmatch(colName); len(m) >= 2 {
		alias := strings.Trim(m[1], `"'`)
		if disp, ok := fieldMap[alias]; ok {
			return cleanDisplay(fmt.Sprintf("%v", disp))
		}
		for k, v := range fieldMap {
			if strings.EqualFold(k, alias) {
				return cleanDisplay(fmt.Sprintf("%v", v))
			}
		}
		return alias
	}
	if disp, ok := fieldMap[colName]; ok {
		return cleanDisplay(fmt.Sprintf("%v", disp))
	}
	colLower := strings.ToLower(colName)
	for k, v := range fieldMap {
		if strings.ToLower(k) == colLower {
			return cleanDisplay(fmt.Sprintf("%v", v))
		}
	}
	result := colName
	for k, v := range fieldMap {
		result = strings.ReplaceAll(result, k, fmt.Sprintf("%v", v))
	}
	return cleanDisplay(result)
}

// chunkKBIDForDoc resolves the kb_id for a citation chunk. Mirrors
// dialog_service.py:56-59. Single-kb queries use the chat's known
// kb_id; multi-kb queries read it from the row.
func chunkKBIDForDoc(rowDict map[string]interface{}, kbIDs []string, docID interface{}) string {
	if len(kbIDs) == 1 {
		return kbIDs[0]
	}
	if v, ok := rowDict["kb_id"]; ok && v != nil && v != "" {
		return fmt.Sprintf("%v", v)
	}
	if v, ok := rowDict["kb_id_kwd"]; ok && v != nil && v != "" {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// cleanCellValue renders one cell value: replaces "None" with a space
// then runs the redundant-space cleanup. Mirrors dialog_service.py:1298.
func cleanCellValue(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	s = strings.ReplaceAll(s, "None", " ")
	return removeRedundantSpaces(s)
}

// extractSourceColumnIndexes returns, for a set of SQL result rows,
// parallel slices of column indices that match `doc_id`,
// `docnm_kwd`/`docnm`, and `kb_id`/`kb_id_kwd` (case-insensitive). Also
// returns the full sorted column-name list. The Go RunSQL result is
// already keyed by column name; this helper derives a positional view
// (sorted alphabetically for stable iteration) that mirrors Python's
// `enumerate(tbl["columns"])`.
func extractSourceColumnIndexes(rows []map[string]interface{}) (docIDIdx, docNameIdx, kbIDIdx []int, columns []string) {
	if len(rows) == 0 {
		return
	}
	for k := range rows[0] {
		columns = append(columns, k)
	}
	sort.Strings(columns)
	for i, c := range columns {
		switch strings.ToLower(c) {
		case "doc_id":
			docIDIdx = append(docIDIdx, i)
		case "docnm_kwd", "docnm":
			docNameIdx = append(docNameIdx, i)
		case "kb_id", "kb_id_kwd":
			kbIDIdx = append(kbIDIdx, i)
		}
	}
	return
}

// WHERE-clause extraction for the aggregate secondary fetch.
// Mirrors dialog_service.py:1321.
var whereClauseRe = regexp.MustCompile(`(?i)\bwhere\b(.+?)(?:\bgroup by\b|\border by\b|\blimit\b|$)`)

// limitClauseRe detects whether a SQL already has a LIMIT clause.
var limitClauseRe = regexp.MustCompile(`(?i)\blimit\b`)

// buildChunkFetchSQL extracts the WHERE clause from the original SQL
// and constructs a secondary SQL to fetch source chunks. Mirrors
// dialog_service.py:1327-1331. Returns ("", false) when no WHERE is
// present. The `multiKB` flag controls whether `kb_id` is included
// in the SELECT list (single-kb queries don't need it because the
// caller already knows the kb_id).
func buildChunkFetchSQL(originalSQL, tableName, expectedCol string, multiKB bool) (string, bool) {
	m := whereClauseRe.FindStringSubmatch(originalSQL)
	if len(m) < 2 {
		return "", false
	}
	where := strings.TrimSpace(m[1])
	kbCol := ""
	if multiKB {
		kbCol = ", kb_id"
	}
	sql := fmt.Sprintf("select doc_id, %s%s from %s where %s",
		expectedCol, kbCol, tableName, where)
	if !limitClauseRe.MatchString(sql) {
		sql += " limit 20"
	}
	return sql, true
}

// toIfaceSlice converts a []map[string]interface{} to []interface{} for
// the call-site contract at async_chat.go:334, which type-asserts
// `reference["chunks"].([]map[string]interface{})`.
func toIfaceSlice(maps []map[string]interface{}) []interface{} {
	out := make([]interface{}, len(maps))
	for i, m := range maps {
		out[i] = m
	}
	return out
}

// -----------------------------------------------------------------------
// Aggregate secondary fetch (dialog_service.py:1311-1367)
// -----------------------------------------------------------------------

// fetchAggregateChunks runs the secondary "select doc_id, docnm[, kb_id]
// from <table> where <extracted_where> [limit 20]" query and uses the
// result to build chunks and doc_aggs. Mirrors the aggregate path in
// dialog_service.py:1311-1365.
//
// Returns (nil, nil) when the secondary fetch should be skipped or
// fails. Skips on Infinity multi-KB (RunSQL rejects), on missing WHERE
// clause, and on engine errors — all matching Python's try/except
// semantics at dialog_service.py:1333-1364.
func (s *ChatPipelineService) fetchAggregateChunks(
	ctx context.Context,
	docEngine engine.DocEngine,
	tableName, originalSQL, expectedCol string,
	kbIDs []string,
) (chunks []map[string]interface{}, docAggs []map[string]interface{}) {
	multiKB := len(kbIDs) > 1

	// Infinity's RunSQL rejects multi-KB (see infinity/sql.go:63-65).
	// Python's add_kb_filter is a no-op for Infinity, so this branch is
	// never exercised in Python either. Skip explicitly to avoid a
	// hard error.
	if multiKB && docEngine != nil && docEngine.GetType() == "infinity" {
		common.Debug("SQL retrieval: skipping aggregate secondary fetch on Infinity multi-KB",
			zap.Strings("kb_ids", kbIDs))
		return nil, nil
	}

	chunksSQL, ok := buildChunkFetchSQL(originalSQL, tableName, expectedCol, multiKB)
	if !ok {
		common.Debug("SQL retrieval: aggregate secondary fetch skipped (no WHERE clause)",
			zap.String("sql", originalSQL))
		return nil, nil
	}

	rows, err := docEngine.RunSQL(ctx, tableName, chunksSQL, kbIDs, "json")
	if err != nil {
		common.Warn("SQL retrieval: aggregate secondary fetch failed",
			zap.String("sql", chunksSQL), zap.Error(err))
		return nil, nil
	}
	if len(rows) == 0 {
		return nil, nil
	}

	docIDIdx, docNameIdx, kbIDIdx, columns := extractSourceColumnIndexes(rows)
	if len(docIDIdx) == 0 || len(docNameIdx) == 0 {
		common.Warn("SQL retrieval: aggregate secondary fetch missing source columns",
			zap.Any("columns", columns))
		return nil, nil
	}

	chunks = make([]map[string]interface{}, 0, len(rows))
	docAggMap := map[string]map[string]interface{}{}
	for _, r := range rows {
		docID := r[columns[docIDIdx[0]]]
		docName := r[columns[docNameIdx[0]]]
		chunk := map[string]interface{}{"doc_id": docID, "docnm_kwd": docName}
		kid := chunkKBIDForDoc(r, kbIDs, docID)
		if kid == "" && len(kbIDIdx) > 0 {
			if v := r[columns[kbIDIdx[0]]]; v != nil && v != "" {
				kid = fmt.Sprintf("%v", v)
			}
		}
		if kid != "" {
			chunk["kb_id"] = kid
		}
		chunks = append(chunks, chunk)

		// doc_aggs aggregation: group by doc_id, count occurrences,
		// first-seen doc_name wins.
		if entry, ok := docAggMap[fmt.Sprintf("%v", docID)]; ok {
			entry["count"] = entry["count"].(int) + 1
		} else {
			docAggMap[fmt.Sprintf("%v", docID)] = map[string]interface{}{
				"doc_name": docName,
				"count":    1,
			}
		}
	}

	docAggs = make([]map[string]interface{}, 0, len(docAggMap))
	for did, d := range docAggMap {
		docAggs = append(docAggs, map[string]interface{}{
			"doc_id":   did,
			"doc_name": d["doc_name"],
			"count":    d["count"],
		})
	}
	common.Debug("SQL retrieval: aggregate secondary fetch produced chunks",
		zap.Int("chunks", len(chunks)),
		zap.Int("doc_aggs", len(docAggs)))
	return chunks, docAggs
}

// -----------------------------------------------------------------------
// Answer + reference assembly (replaces renderSQLAnswer)
// -----------------------------------------------------------------------

// buildSQLReference renders the Markdown table answer and assembles
// the reference (chunks + doc_aggs) for a SQL retrieval result. Mirrors
// dialog_service.py:1282-1401.
//
// Three branches match Python:
//  1. hasSrc: rows themselves carry doc_id + docnm*. Build chunks/doc_aggs
//     from the rows directly. (Python L1369-1401.)
//  2. isAggregateSQL: source columns missing. Run a secondary fetch to
//     build chunks/doc_aggs; preserve the rendered table as the answer.
//     (Python L1311-1367.)
//  3. Non-aggregate missing source: best-effort answer with empty refs.
//     (Python L1367.)
//
// Scalar shortcut: when the result is a single-cell (1 row, 1 column),
// return the value directly without a table — matches the previous
// renderSQLAnswer behavior and the Python non-aggregate path's
// one-cell edge case.
func (s *ChatPipelineService) buildSQLReference(
	ctx context.Context,
	docEngine engine.DocEngine,
	tableName, originalSQL string,
	rows []map[string]interface{},
	sysPrompt, engineName string,
	kbs []*entity.Knowledgebase,
	fieldMap map[string]interface{},
) (string, map[string]interface{}) {
	if len(rows) == 0 {
		return "No results.", map[string]interface{}{
			"chunks":   []map[string]interface{}{},
			"doc_aggs": []interface{}{},
			"total":    0,
		}
	}

	// Scalar shortcut — matches the previous renderSQLAnswer behavior.
	if len(rows) == 1 && len(rows[0]) == 1 {
		for _, v := range rows[0] {
			return cleanCellValue(v), map[string]interface{}{
				"chunks":   []map[string]interface{}{},
				"doc_aggs": []interface{}{},
				"total":    1,
			}
		}
	}

	kbIDs := kbIDStrings(kbs)
	docIDIdx, docNameIdx, kbIDIdx, columns := extractSourceColumnIndexes(rows)
	expectedCol := expectedDocNameColumn(engineName)
	hasSrc := len(docIDIdx) > 0 && len(docNameIdx) > 0

	// Build the set of "display column" indices (everything except
	// doc_id, docnm*, kb_id*). Python uses set subtraction at
	// dialog_service.py:1232.
	exclude := map[int]bool{}
	for _, i := range docIDIdx {
		exclude[i] = true
	}
	for _, i := range docNameIdx {
		exclude[i] = true
	}
	for _, i := range kbIDIdx {
		exclude[i] = true
	}
	displayCols := make([]int, 0, len(columns))
	for i := range columns {
		if !exclude[i] {
			displayCols = append(displayCols, i)
		}
	}

	// --- Header ---
	var header strings.Builder
	header.WriteString("|")
	for _, i := range displayCols {
		header.WriteString(mapColumnName(columns[i], fieldMap))
		header.WriteString("|")
	}
	if hasSrc {
		header.WriteString("Source|")
	}

	// --- Separator (Python L1285) ---
	sep := strings.Repeat("|------", len(displayCols)) + "|"
	if hasSrc {
		sep += "------|"
	}

	// --- Body rows + ##N$$ citation markers ---
	bodyRows := make([]string, 0, len(rows))
	for rowIdx, r := range rows {
		var cells strings.Builder
		cells.WriteString("|")
		for _, i := range displayCols {
			cells.WriteString(cleanCellValue(r[columns[i]]))
			cells.WriteString("|")
		}
		if hasSrc {
			cells.WriteString(fmt.Sprintf(" ##%d$$|", rowIdx))
		}
		// Skip rows that are entirely empty/whitespace (Python's
		// `if re.sub(r"[ |]+", "", row_str)` filter at L1303).
		rowStr := cells.String()
		if strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rowStr, "|", ""), " ", "")) != "" {
			bodyRows = append(bodyRows, rowStr)
		}
	}
	rowsJoined := stripISOTimestamps(strings.Join(bodyRows, "\n"))

	answer := strings.Join([]string{header.String(), sep, rowsJoined}, "\n")

	// --- Reference: chunks + doc_aggs ---
	ref := map[string]interface{}{
		"chunks":   []map[string]interface{}{},
		"doc_aggs": []interface{}{},
		"total":    len(rows),
	}

	if hasSrc {
		// Primary path — build chunks and doc_aggs from rows.
		chunks := make([]map[string]interface{}, 0, len(rows))
		docAggMap := map[string]map[string]interface{}{}
		for _, r := range rows {
			did := r[columns[docIDIdx[0]]]
			dn := r[columns[docNameIdx[0]]]
			entry := map[string]interface{}{"doc_id": did, "docnm_kwd": dn}
			if kid := chunkKBIDForDoc(r, kbIDs, did); kid != "" {
				entry["kb_id"] = kid
			} else if len(kbIDIdx) > 0 {
				if v := r[columns[kbIDIdx[0]]]; v != nil && v != "" {
					entry["kb_id"] = fmt.Sprintf("%v", v)
				}
			}
			chunks = append(chunks, entry)

			docIDKey := fmt.Sprintf("%v", did)
			if e, ok := docAggMap[docIDKey]; ok {
				e["count"] = e["count"].(int) + 1
			} else {
				docAggMap[docIDKey] = map[string]interface{}{
					"doc_name": dn,
					"count":    1,
				}
			}
		}
		docAggs := make([]map[string]interface{}, 0, len(docAggMap))
		for did, d := range docAggMap {
			docAggs = append(docAggs, map[string]interface{}{
				"doc_id":   did,
				"doc_name": d["doc_name"],
				"count":    d["count"],
			})
		}
		ref["chunks"] = chunksFormat(chunks)
		ref["doc_aggs"] = docAggs
		return answer, ref
	}

	// Source columns missing — try the aggregate secondary fetch.
	if isAggregateSQL(originalSQL) {
		chunks, docAggs := s.fetchAggregateChunks(ctx, docEngine, tableName, originalSQL, expectedCol, kbIDs)
		if len(chunks) > 0 {
			ref["chunks"] = chunksFormat(chunks)
			ref["doc_aggs"] = docAggs
		}
		return answer, ref
	}

	// Non-aggregate, no source columns: best-effort empty refs.
	common.Debug("SQL retrieval: non-aggregate SQL missing source columns; returning best-effort answer",
		zap.String("sql", originalSQL))
	return answer, ref
}

// jsonMarshal is a small wrapper around encoding/json to keep this
// file's imports tidy.
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// kbIDStrings extracts the string KB IDs from a slice of Knowledgebase.
// Returns nil if no KB has a non-empty ID. Mirrors Python's `kb_ids`
// iteration in dialog_service.py:651-660.
func kbTenantIDStrings(kbs []*entity.Knowledgebase) []string {
	if len(kbs) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		if kb.TenantID != "" {
			if _, ok := seen[kb.TenantID]; !ok {
				seen[kb.TenantID] = struct{}{}
				out = append(out, kb.TenantID)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// BuildChatConfig converts the dialog's LLM setting (with optional
// per-request overrides) into a typed ChatConfig for the LLM driver.
// Dialog values are read first; request config values win when present.
func BuildChatConfig(dialog *entity.Chat, config map[string]interface{}) *modelModule.ChatConfig {
	cfg := &modelModule.ChatConfig{}

	if dialog.LLMSetting != nil {
		if v, ok := dialog.LLMSetting["stream"].(bool); ok {
			cfg.Stream = &v
		}
		if v, ok := dialog.LLMSetting["thinking"].(bool); ok {
			cfg.Thinking = &v
		}
		if v, ok := dialog.LLMSetting["max_tokens"].(float64); ok {
			i := int(v)
			cfg.MaxTokens = &i
		}
		if v, ok := dialog.LLMSetting["temperature"].(float64); ok {
			cfg.Temperature = &v
		}
		if v, ok := dialog.LLMSetting["top_p"].(float64); ok {
			cfg.TopP = &v
		}
		if v, ok := dialog.LLMSetting["do_sample"].(bool); ok {
			cfg.DoSample = &v
		}
		if v, ok := dialog.LLMSetting["stop"].([]interface{}); ok {
			stops := make([]string, 0, len(v))
			for _, s := range v {
				if str, ok := s.(string); ok {
					stops = append(stops, str)
				}
			}
			cfg.Stop = &stops
		}
		if v, ok := dialog.LLMSetting["model_class"].(string); ok {
			cfg.ModelClass = &v
		}
		if v, ok := dialog.LLMSetting["effort"].(string); ok {
			cfg.Effort = &v
		}
		if v, ok := dialog.LLMSetting["verbosity"].(string); ok {
			cfg.Verbosity = &v
		}
	}

	if config != nil {
		if v, ok := config["stream"].(bool); ok {
			cfg.Stream = &v
		}
		if v, ok := config["thinking"].(bool); ok {
			cfg.Thinking = &v
		}
		if v, ok := config["max_tokens"].(float64); ok {
			i := int(v)
			cfg.MaxTokens = &i
		}
		if v, ok := config["temperature"].(float64); ok {
			cfg.Temperature = &v
		}
		if v, ok := config["top_p"].(float64); ok {
			cfg.TopP = &v
		}
		if v, ok := config["do_sample"].(bool); ok {
			cfg.DoSample = &v
		}
		if v, ok := config["stop"].([]interface{}); ok {
			stops := make([]string, 0, len(v))
			for _, s := range v {
				if str, ok := s.(string); ok {
					stops = append(stops, str)
				}
			}
			cfg.Stop = &stops
		}
		if v, ok := config["model_class"].(string); ok {
			cfg.ModelClass = &v
		}
		if v, ok := config["effort"].(string); ok {
			cfg.Effort = &v
		}
		if v, ok := config["verbosity"].(string); ok {
			cfg.Verbosity = &v
		}
	}

	return cfg
}

func kbIDStrings(kbs []*entity.Knowledgebase) []string {
	if len(kbs) == 0 {
		return nil
	}
	out := make([]string, 0, len(kbs))
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		if kb.ID != "" {
			out = append(out, kb.ID)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// chunksFormat normalizes raw chunk maps to the frontend-expected field names.
// Mirrors Python's chunks_format in rag/prompts/generator.py:41-65.
func chunksFormat(chunksRaw []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(chunksRaw))
	for _, chunk := range chunksRaw {
		formatted := map[string]interface{}{
			"id":                getChunkValue(chunk, "chunk_id", "id"),
			"content":           getChunkValue(chunk, "content", "content_with_weight"),
			"document_id":       getChunkValue(chunk, "doc_id", "document_id"),
			"document_name":     getChunkValue(chunk, "docnm_kwd", "document_name"),
			"dataset_id":        getChunkValue(chunk, "kb_id", "dataset_id"),
			"image_id":          getChunkValue(chunk, "image_id", "img_id"),
			"positions":         getChunkValue(chunk, "positions", "position_int"),
			"url":               chunk["url"],
			"similarity":        chunk["similarity"],
			"vector_similarity": chunk["vector_similarity"],
			"term_similarity":   chunk["term_similarity"],
			"row_id":            chunk["row_id"],
			"doc_type":          getChunkValue(chunk, "doc_type_kwd", "doc_type"),
			"document_metadata": chunk["document_metadata"],
		}
		result = append(result, formatted)
	}
	return result
}

// getChunkValue returns the first non-nil value from a chunk map, trying k1 first then k2.
// Mirrors Python's get_value helper in rag/prompts/generator.py:37-38.
func getChunkValue(chunk map[string]interface{}, k1, k2 string) interface{} {
	if v, ok := chunk[k1]; ok && v != nil {
		return v
	}
	return chunk[k2]
}
