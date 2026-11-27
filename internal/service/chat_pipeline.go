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
	"errors"
	"fmt"
	"maps"
	"math"
	"net/http"

	"ragflow/internal/agentic_rag"
	"ragflow/internal/common"
	"ragflow/internal/compliance"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/permission"
	ragprompts "ragflow/internal/rag/prompts"
	"ragflow/internal/service/file"
	"ragflow/internal/service/graph"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"ragflow/internal/dao"

	"go.uber.org/zap"
)

// rebindToolcallSession re-attaches the toolcall session from kwargs onto a
// freshly resolved chat model. Phase 4 binds tools only to the originally
// resolved instance, so without this a gp- group-failover retry would run a
// new model without ToolConfig and silently lose the tool-call capability.
func rebindToolcallSession(model *modelModule.ChatModel, kwargs map[string]interface{}) {
	if model == nil {
		return
	}
	session, hasSession := kwargs["toolcall_session"]
	tools, hasTools := kwargs["tools"]
	if !hasSession || session == nil || !hasTools || tools == nil {
		return
	}
	if ts, ok := session.(modelModule.ToolCallSession); ok {
		common.Info("Bind ToolCall after failover re-resolve")
		model.BindTools(ts, tools)
	}
}

// Fields defined by the chat-completions message schema. Stored messages also
// carry RAGFlow bookkeeping such as id, created_at, doc_ids, and conversationId,
// which strict providers reject.
var llmMessageFields = [...]string{"role", "content", "name", "tool_calls", "tool_call_id", "function_call", "refusal", "audio"}

// ChatPipelineService is the shared RAG chat pipeline engine used by both
// the OpenAI-compatible endpoint (/api/v1/openai/<chat_id>/chat/completions)
// and the regular chat completion endpoint (/api/v1/chat/completions).
//
// It owns the core retrieval → generation pipeline (AsyncChat, AsyncChatSolo)
// and all their supporting helpers. Callers (OpenAIChatService, ChatSessionService)
// compose it to avoid code duplication.
type ChatPipelineService struct {
	ModelFactory *ModelFactory
	MetadataSvc  *MetadataService
	kbDAO        *dao.KnowledgebaseDAO
}

// NewChatPipelineService creates a new ChatPipelineService with all required dependencies.
func NewChatPipelineService() *ChatPipelineService {
	return &ChatPipelineService{
		ModelFactory: NewModelFactory(),
		MetadataSvc:  NewMetadataService(),
		kbDAO:        dao.NewKnowledgebaseDAO(),
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
	// ThinkEvent is one STRUCTURED reasoning step (the machine-readable twin of
	// the think-block sentence), forwarded so a client can render steps —
	// tool, status, counts, sources, duration — instead of paragraphs.
	ThinkEvent *ThinkEvent `json:"think_event,omitempty"`
	// ElapsedMs is this turn's client-facing wall-clock cost, emitted on the
	// final chunk so a client can show what the answer cost. Usage (the token
	// accounting) is declared once further down, as *TurnUsage.
	ElapsedMs float64 `json:"elapsed_ms,omitempty"`
	// Internal-only: set when Answer is the run's authoritative full text —
	// <think> stretch included, citation handles already compacted against the
	// shipped reference — so the session writer must persist it instead of the
	// accumulated stream deltas, which still carry the pre-compaction handles.
	AnswerIsAuthoritative bool
	// ToolCallCounts carries how many times each tool ran during this turn,
	// keyed by tool name (e.g. {"search_chunks": 7, "list_chunks": 4}).
	// Only agentic runs populate it, and only on the final result — the
	// numbers are only complete once the ReAct loop (and its delivery gate)
	// is done. Benchmarks report it as average search calls per question.
	ToolCallCounts map[string]int `json:"tool_call_counts,omitempty"`
	// ToolCallErrors tallies how many of those calls returned a failure
	// notice (<tool_error> / severity="error") — a backend outage or
	// repeated invalid arguments shows up here instead of hiding inside the
	// call counts. Same population rule as ToolCallCounts.
	ToolCallErrors map[string]int `json:"tool_call_errors,omitempty"`
	// ToolErrorSamples carries one representative failure message per tool
	// (first occurrence, truncated) so a benchmark row is diagnosable
	// without opening the server logs. Same population rule.
	ToolErrorSamples map[string]string `json:"tool_error_samples,omitempty"`
	// RetrievedDocIDs is every document identifier (doc_id and doc_name) the
	// turn's retrieval tools RETRIEVED - opened with full chunk content
	// (list_chunks, search_chunks, search_semantic_chunks), deduplicated and
	// sorted. A document seen only through a <match_snippet> window
	// (search_bm25_chunks, grep_chunks) was LOCATED, not retrieved, and does
	// NOT appear here: the two sets answer different questions, and the
	// benchmark's Recall (%) scores against this one. Same population rule as
	// ToolCallCounts. Measured 2026-10-05: q1093 of a ten-question run
	// reported 6 ids here while its retrieval had served 270 documents -
	// reading this field as "what the agent saw" understates the context by an
	// order of magnitude. ServedDocIDs is the set for that question.
	RetrievedDocIDs []string `json:"retrieved_docids,omitempty"`
	// ServedDocIDs is every document identifier the turn's retrieval tools
	// SURFACED: the full-content tools above PLUS the snippet-only locators
	// (search_bm25_chunks, grep_chunks), ranked most-served first. It is the
	// set that answers "was the answer's own document ever in front of the
	// model", which RetrievedDocIDs cannot: a document can surface a dozen
	// times and never be opened, and that gap is exactly where a retained
	// candidate sits on a document nobody read.
	ServedDocIDs []string `json:"served_docids,omitempty"`
	// DeepReadChunks / ShallowReadChunks count the chunks the turn's
	// retrieval tools put in front of the model, split by read depth: deep =
	// full chunk content (list_chunks, search_chunks), shallow =
	// <match_snippet> windows (grep_chunks, search_bm25_chunks). They show
	// where a run's context weight came from — document reading vs triage
	// traffic — which raw tool call counts cannot distinguish. Same
	// population rule as ToolCallCounts.
	DeepReadChunks    int `json:"deep_read_chunks,omitempty"`
	ShallowReadChunks int `json:"shallow_read_chunks,omitempty"`
	// DeepReadChunkIDs / ShallowReadChunkIDs name the chunks behind those two
	// counts, sorted. The counts say how MUCH a run read; the ids say WHICH —
	// and the difference between a chunk that was read and never cited and one
	// that was never read at all is the one a total cannot make. Reported as
	// raw facts: whether an uncited read chunk matters is the auditor's
	// judgement, not the ledger's.
	DeepReadChunkIDs    []string `json:"deep_read_chunk_ids,omitempty"`
	ShallowReadChunkIDs []string `json:"shallow_read_chunk_ids,omitempty"`
	// Compactions counts the context compactions the engine performed this turn
	// (mode 8 only; 0 or 1). Codex only — an in-process engine sends its whole
	// history every turn and has no server-side context to compact. It is
	// observability: it says the model's own view of the conversation was
	// summarized mid-turn, not that any stored turn was dropped.
	Compactions int `json:"compactions,omitempty"`
	// Usage is the turn's token accounting. A benchmark reports cost per
	// question, and only the pipeline sees every LLM call the ReAct loop and
	// its delivery gate made — the response's own token counts cover the final
	// answer alone. The regular chat path fills the three plain token fields;
	// agentic runs add the per-call split (LLMCalls / LLMTurns).
	Usage *TurnUsage `json:"usage,omitempty"`
	// ElapsedSeconds is the server-side wall-clock duration of the whole turn,
	// including the delivery gate. Client-side timing additionally covers
	// transport and queueing, which a benchmark wants to see separately.
	ElapsedSeconds float64 `json:"elapsed_seconds,omitempty"`
	// Internal-only: accumulated answer for building the decorated final result.
	accumulatedAnswer string
}

// ThinkEvent is one structured agentic reasoning step as it goes over the
// wire (the SSE `think_event` field). This is the ONE definition, so a
// producer that needs structured steps aliases this type instead of carrying
// a second copy bridged field by field — that older shape compiled for as
// long as one side stayed a subset, and a field added on the producer side
// then reached the client as nothing at all.
//
// The JSON tags below ARE the client contract; TestThinkEventWireKeys pins
// them.
type ThinkEvent struct {
	Kind string `json:"kind"`
	// Stage is the bracketed tag the step belongs to ("Function tool",
	// "RAGAgent", ...). It is the grouping key.
	Stage string `json:"stage,omitempty"`
	// Tool is the tool name for tool_call / tool_result events.
	Tool string `json:"tool,omitempty"`
	// Args is the tool call's arguments, rendered for display.
	Args string `json:"args,omitempty"`
	// Status and Reason describe a tool step's outcome.
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Cause is the tool's own explanation of a failure: a bare reason such as
	// "infra" tells a client nothing about what actually broke.
	Cause string `json:"cause,omitempty"`
	// Results is how many results the step returned; Documents how many
	// distinct documents they came from.
	Results   int `json:"results,omitempty"`
	Documents int `json:"documents,omitempty"`
	// Sources are the evidence anchors the step produced (capped).
	Sources []string `json:"sources,omitempty"`
	// DurationMS is the wall time of a tool call; 0 for a stage step.
	DurationMS int64 `json:"duration_ms,omitempty"`
	// Summary is the human sentence for this step, exactly what the think
	// block shows.
	Summary string `json:"summary"`
}

// TurnUsage is the aggregated token cost of every LLM call one agentic turn
// made: the loop's research steps, the delivery gate's audits, and the
// synthesis fallback alike.
type TurnUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	LLMCalls         int `json:"llm_calls"`
	// LLMTurns is the per-call split the aggregate above sums up: one entry per
	// LLM call (ReAct iteration, repair turn, audit pass, synthesis), with its
	// input/output tokens, the model that served it and when it started. A
	// benchmark reads it to see WHERE a question's cost went, not just how much
	// it was. Omitted by backends that do not report it.
	LLMTurns []tokenizer.CallUsage `json:"llm_turns,omitempty"`
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
//	│         each layer: KB → Web search → KG(use_kg)      │
//	│         → sufficiencyCheck → multiQueriesGen → recurse│
//	│   NO  → Standard vector retrieval                     │
//	│         vector/hybrid search → rerank →               │
//	│         child chunk retrieval →                       │
//	│         Web search → KG retrieval (prepend)           │
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
//   - messages: saved user/assistant history, or the full payload for non-storing model tests.
//   - stream: if true, yields content deltas as they arrive.
//   - kwargs: extra parameters (doc_ids, knowledge, quote, etc.).
func (s *ChatPipelineService) AsyncChat(
	ctx context.Context,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
	kwargs map[string]interface{},
) (results <-chan AsyncChatResult, runErr error) {
	defer func() {
		if runErr == nil && results != nil {
			results = s.filterChatOutput(ctx, chat, stream, results)
			results = logChatCompletion(ctx, chat, messages, stream, results)
		}
	}()

	common.Info("AsyncChat started", zap.String("chat_id", chat.ID))

	// === Phase 1: Entry Validation ===
	// Guard: messages must be non-empty and the last role must be "user".
	common.Info("phase 1: Entry Validation")
	if len(messages) == 0 {
		return nil, fmt.Errorf("AsyncChat: messages is empty")
	}
	if err := validateUserMessageContent(messages); err != nil {
		return nil, common.NewCodedError(common.CodeArgumentError, err.Error())
	}
	lastMsg := messages[len(messages)-1]
	if role, _ := lastMsg["role"].(string); role != "user" {
		return nil, fmt.Errorf("the last content of this conversation is not from user")
	}
	if err := validateChatDatasetAccess(ctx, chat.KBIDs); err != nil {
		return nil, err
	}
	question, err := NormalizeOpenAIMessageContent(lastMsg["content"])
	if err != nil {
		return nil, err
	}
	ctx, err = compliance.WithOutputFilter(ctx)
	if err != nil {
		if ctx.Err() != nil {
			out := make(chan AsyncChatResult)
			close(out)
			return out, nil
		}
		return nil, err
	}
	if err := compliance.ValidateQuestion(ctx, question); err != nil {
		if ctx.Err() != nil {
			out := make(chan AsyncChatResult)
			close(out)
			return out, nil
		}
		out := make(chan AsyncChatResult, 2)
		if stream {
			out <- AsyncChatResult{Answer: err.Error()}
		}
		out <- AsyncChatResult{Answer: err.Error(), Final: true}
		close(out)
		return out, nil
	}

	// Resolve what this conversation can reach BEFORE dispatching: both the
	// classic pipeline and the agentic ReAct path are gated on them, and the
	// agentic path needs the resolution to know whether it may offer the
	// web_search tool.
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
			zap.Bool("configured", resolveWebSearchProvider(chat.PromptConfig) != nil),
			zap.Any("internet", kwargs["internet"]),
			zap.Bool("enabled", useWebSearch))
	}

	// The agentic engines (mode 5, mode 8) run their own retrieval — mode 5
	// through its in-process corpus tools, mode 8 through Codex calling this
	// process's MCP bridge — so the pipeline's search and generation phases do not
	// apply. selectAgenticEngine owns which one, if any, a turn runs on, from the
	// reasoning level and the dialog's KB scope.
	//
	// The level is resolved from the request first and the dialog's
	// prompt_config second, exactly as Phase 9 resolves it; a dialog pinned to
	// an agentic level therefore keeps that engine when the request omits the
	// level, and the dispatch stays ahead of the solo-chat short-circuit below so
	// a level-selected turn is never diverted to a plain LLM answer.
	level := resolveReasoningLevel(kwargs, map[string]interface{}(chat.PromptConfig))
	selectedEngine := selectAgenticEngine(level, hasKBs)
	// The engine dispatch hands the caller's identity to the engine runs below;
	// the regular pipeline phases read it from ctx themselves.
	userID := permission.UserIDFromContext(ctx)
	// mode 8 needs a configured shared Codex app-server AND a dialog model that
	// speaks the Responses wire API. Without either, fall back to the regular
	// pipeline (and never leak an unconfigured engine to a caller) — the honest
	// reading of "the engine it asked for is not available here", mirroring how a
	// level-5 turn with no KBs drops to the regular branch.
	if selectedEngine == engineCodex {
		if !codexConfigured() {
			common.WarnCtx(ctx, "codex (mode 8) requested but no codex.endpoint/mcp_public_base is configured; using the regular pipeline",
				zap.String("chat_id", chat.ID))
			selectedEngine = ""
		} else if !s.codexResponsesCapable(ctx, userID, chat) {
			common.WarnCtx(ctx, "codex (mode 8) requires a Responses-API dialog model; using the regular pipeline",
				zap.String("chat_id", chat.ID))
			selectedEngine = ""
		}
	}
	switch selectedEngine {
	case engineAgenticRAG:
		// agenticRag reads the web-search provider from the context — upstream
		// dropped its useWebSearch argument. The Codex engine still takes it as
		// a parameter and converts it internally.
		if useWebSearch {
			ctx = agentic_rag.WithWebSearch(ctx, s.webSearchFunc(chat.PromptConfig))
		}
		return s.agenticRag(ctx, chat, messages, stream, kwargs, quoteEnabled(kwargs, chat.PromptConfig))
	case engineCodex:
		return s.codexAgent(ctx, userID, chat, messages, stream, kwargs, useWebSearch, quoteEnabled(kwargs, chat.PromptConfig))
	}

	// No KBs & no web search → fast-path to LLM-only chat.
	if !hasKBs && !useWebSearch {
		return s.AsyncChatSolo(ctx, chat, messages, stream, kwargs)
	}
	if kwargs == nil {
		kwargs = make(map[string]interface{})
	}

	// Spawn goroutine for the async pipeline. All remaining phases run inside.
	out := make(chan AsyncChatResult, 16)

	go func() {
		defer close(out)

		timer := common.NewTimer()
		timer.Start()

		sendResult := func(res AsyncChatResult) bool {
			if ctx.Err() != nil {
				return false
			}
			select {
			case out <- res:
				return true
			case <-ctx.Done():
				return false
			}
		}

		// === Phase 2: Resolve LLM Model Config + max_tokens ===
		common.Info("Phase 2: Resolve LLM Model Config + max_tokens")
		timer.Enter(common.PhaseCheckLLM)
		llmModelConfig, _, _, _, err := s.getLLMModelConfig(ctx, chat)
		if err != nil {
			sendResult(AsyncChatResult{
				Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
				Final:  true,
			})
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
		kbs, embModel, rerankModel, chatModel, ttsModel, err := s.getModels(ctx, chat)
		if err != nil {
			sendResult(AsyncChatResult{
				Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
				Final:  true,
			})
			return
		}

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
		// Entry validation guarantees the final user content normalizes to a question.
		var questions []string
		for _, m := range messages {
			if role, _ := m["role"].(string); role == "user" {
				if content, err := NormalizeOpenAIMessageContent(m["content"]); err == nil {
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
		// Only vision-capable (image2text-typed) models receive image
		// content blocks; text-only chat providers reject them (e.g.
		// Zhipu GLM error 1210: messages.content.type only allows
		// 'text'), so their images are dropped here — mirroring
		// Python's dialog_model_vision_capable gate.
		var textAttachmentsList []string
		var imageFiles []string
		// Whether the message carried image attachments before the
		// vision gate (read by the empty-response fallback below).
		var hasImageAttachments bool
		// Joined text attachments (appended to the last user message).
		var attachments string
		// When files are file dicts, splitFileAttachments fetches blobs
		// from storage. When plain strings, falls back to string splitting;
		// raw only changes that split for pre-separated image payloads.
		if files, hasFiles := lastMsg["files"]; hasFiles {
			modelType := "chat"
			if llmModelConfig != nil {
				if mt, ok := llmModelConfig["model_type"].(string); ok {
					modelType = mt
				}
			}
			var images []string
			textAttachmentsList, images, err = splitFileAttachments(ctx, files, modelType == "image2text")
			if err != nil {
				sendResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", err), Final: true})
				return
			}
			imageFiles, hasImageAttachments = gateImageAttachments(chat.LLMID, modelType, images)
			attachments = strings.Join(textAttachmentsList, "\n\n")
			// Log counts only: attachment payloads are base64 images or
			// full document contents.
			common.Debug("Resolved attachments",
				zap.Int("text_attachments", len(textAttachmentsList)),
				zap.Int("image_files", len(imageFiles)),
				zap.Int("attachment_bytes", len(attachments)))
		}

		var historyImageFiles []string
		for _, previous := range messages[:len(messages)-1] {
			if previous["role"] == "user" && previous["files"] != nil {
				_, images, attachmentErr := s.splitChatAttachments(ctx, previous["files"])
				if attachmentErr != nil {
					sendResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", attachmentErr), Final: true})
					return
				}
				modelType, _ := llmModelConfig["model_type"].(string)
				images, _ = gateImageAttachments(chat.LLMID, modelType, images)
				historyImageFiles = append(historyImageFiles, images...)
			}
		}
		hasImageAttachments = hasImageAttachments || len(historyImageFiles) > 0

		// === Phase 6: SQL Retrieval ===
		// Retrieve field_map for SQL retrieval (preferred over vector search)
		promptConfig := chat.PromptConfig
		// Either the chat setting or the request can disable citations. Resolved
		// once before any retrieval path can return early. The agentic dispatch
		// above resolves the same value through quoteEnabled.
		quote := quoteEnabled(kwargs, promptConfig)
		fieldMap, fmErr := s.kbDAO.GetFieldMap(ctx, dao.DB, kbIDStrings(kbs))
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
					s.enrichChunksWithMetadata(ctx, kbinfos, chat.TenantID, metadataFields)
				}

				sendResult(AsyncChatResult{
					Answer:    ansStr,
					Reference: ans["reference"].(map[string]interface{}),
					Final:     true,
				})
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
		kwargs["date"] = time.Now().UTC().Format(time.RFC3339)
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
					sendResult(AsyncChatResult{
						Answer: fmt.Sprintf("**ERROR**: Miss parameter: %s", key),
						Final:  true,
					})
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
					zap.String("refined", refined))
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
		docIDs = s.chatMetadataDocIDs(ctx, chat, questions[len(questions)-1], chatModel, docIDs, kbIDStrings(kbs))

		// keyword — extract top-N keywords from the question via LLM and
		// append them to the question text to boost lexical retrieval recall.
		if useKW, _ := chat.PromptConfig["keyword"].(bool); useKW && chatModel != nil && len(questions) > 0 {
			if kw, err := KeywordExtraction(ctx, chatModel, questions[len(questions)-1], 3); err == nil && kw != "" {
				original := questions[len(questions)-1]
				questions[len(questions)-1] = AppendKeywords(original, kw)
				common.Debug("keyword extraction applied",
					zap.String("original_question", original),
					zap.String("augmented_question", questions[len(questions)-1]))
			} else if err != nil {
				common.Warn("keyword extraction failed", zap.Error(err))
			}
		}
		timer.Exit(common.PhaseQueryRefinement)

		// === Phase 9: Retrieval ===
		// The retrieval strategy was chosen before this goroutine started:
		// AsyncChat hands reasoning level reasoningLevelAgentic to the
		// smart-reasoning agent, which runs its own retrieval and never returns
		// here. Every other level — including a stale 1..4 sent by an older
		// client — retrieves through the standard path below.
		common.Info("Phase 9: Retrieval",
			zap.Bool("has_knowledge_param", hasKnowledgeParam))

		timer.Enter(common.PhaseRetrieval)
		var kbinfos map[string]interface{}
		kbinfos = map[string]interface{}{
			"total":    0,
			"chunks":   []map[string]interface{}{},
			"doc_aggs": []interface{}{},
		}
		var knowledges []string
		rerankCandidatesCount := int(chat.RerankCandidatesCount)
		if rerankCandidatesCount <= 0 {
			rerankCandidatesCount = 64
		}

		// When hasKnowledgeParam is true, runs standard retrieval, then:
		//   - Child chunk retrieval.
		//   - Web search provider (if internet is enabled).
		//   - Knowledge graph retrieval (if use_kg is enabled).
		// Populates kbinfos (chunks + doc_aggs) and knowledges.
		// When false, the entire block is skipped.
		if hasKnowledgeParam {
			// Regular RAG mirroring Python's async_chat — native docStore
			// retrieval, then the child chunk / web search / KG
			// enhancements below.
			searchQuestion := strings.Join(questions, " ")
			if embModel != nil {
				rankFeature := s.MetadataSvc.LabelQuestion(ctx, searchQuestion, kbs)
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
						RerankCandidatesCount:  &rerankCandidatesCount,
						KNNTopK:                &top,
						SimilarityThreshold:    &threshold,
						VectorSimilarityWeight: &vsw,
						RankFeature:            &rankFeature,
						RerankModel:            rerankModel,
						EmbeddingModel:         embModel,
						Aggs:                   func() *bool { v := true; return &v }(),
						Language:               entity.KnowledgebasesLanguage(kbs),
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
					sendResult(AsyncChatResult{
						Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
						Final:  true,
					})
					return
				}
			}

			// Child chunk retrieval
			if existingChunks, ok := kbinfos["chunks"].([]map[string]interface{}); ok && len(existingChunks) > 0 {
				kbinfos["chunks"] = nlp.RetrievalByChildren(existingChunks, kbTenantIDStrings(kbs), engine.Get(), ctx)
			}

			// Web search
			if s.shouldUseWebSearch(chat, kwargs["internet"]) {
				provider := resolveWebSearchProvider(chat.PromptConfig)
				webResult, webErr := s.retrieveWebSearch(ctx, provider, searchQuestion)
				if webErr != nil {
					common.Warn("Web search failed", zap.Error(webErr))
				} else {
					// Extend chunks and doc_aggs with web search results.
					if existingChunks, ok := kbinfos["chunks"].([]map[string]interface{}); ok {
						if newChunks, ok := webResult["chunks"].([]map[string]interface{}); ok {
							kbinfos["chunks"] = append(existingChunks, newChunks...)
						}
					}
					if existingAggs, ok := kbinfos["doc_aggs"].([]interface{}); ok {
						if newAggs, ok := webResult["doc_aggs"].([]interface{}); ok {
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

		// Enrich chunks with document metadata AFTER all retrieval adds.
		// Request values (kwargs) take precedence over config values.
		if includeRefMeta, metadataFields := s.resolveReferenceMetadata(promptConfig, kwargs); includeRefMeta {
			s.enrichChunksWithMetadata(ctx, kbinfos, chat.TenantID, metadataFields)
		}
		timer.Exit(common.PhaseRetrieval)

		// === Phase 10: Build LLM Request ===
		// Sub-steps: empty_response check → formatPrompt → citationPrompt →
		// messageFitIn (95% token budget) → multimodal conversion → adjust max_tokens.
		// If no knowledges and empty_response is configured, yield it and return.
		// Compiled-serve: swap/extend the retrieved chunk text with knowledge-compiled
		// products when enabled. Mirrors the agentic locate exit for the standard
		// (reasoning level 1-4) retrieval path. No-op when inactive or no products
		// were found for the recalled chunks.
		if rawChunks, ok := kbinfos["chunks"].([]map[string]interface{}); ok && len(rawChunks) > 0 {
			if served := agentic_rag.CompiledServeChunks(ctx, chat.TenantID, rawChunks, kbIDStrings(kbs)); served != nil {
				kbinfos["chunks"] = served
			}
		}
		// The passages get what the window leaves after the system prompt, the
		// citation rules appended to it and the completion reserve.
		chatCfg := BuildChatConfig(chat, kwargs)
		systemTemplate, _ := promptConfig["system"].(string)
		modelMaxOutput, _ := llmModelConfig["max_output"].(int)
		reserve := completionReserve(chatCfg, modelMaxOutput, modelMaxTokens)
		knowledges = s.kbPrompt(kbinfos, s.knowledgeTokenBudget(modelMaxTokens, systemTemplate, kwargs, quote, reserve))
		common.Info("Phase 10: Build LLM Request")
		common.Debug("Knowledge prompt",
			zap.String("question", strings.Join(questions, " ")),
			zap.Strings("knowledges", knowledges))

		// empty_response check
		// When no knowledge chunks were retrieved, skip the LLM entirely and
		// return the user-configured fallback message (if set).
		// If empty_response is not configured, fall through to the LLM call
		// with an empty knowledge context.
		//
		// EXCEPTION: when the user attached files to their message, the
		// attachment text provides context that should be sent to the LLM
		// even if KB retrieval returned nothing — and image attachments
		// must reach a vision model instead of being swallowed by the
		// canned response. In that case we skip the early return and fall
		// through to the normal LLM call where attachments are appended to
		// the last user message.
		//
		// Two results are yielded (mirroring Python dialog_service.py):
		//   1. Final=false — carries the answer text so streaming consumers
		//      actually display the fallback message.
		//   2. Final=true   — closes the stream with the same full answer plus
		//      the reference/prompt. Python yields the full answer again in the
		//      final event (dialog_service.py:807); consumers that only look at
		//      the final event (e.g. the OpenAI-compatible endpoint) would
		//      otherwise see an empty reply.
		if emptyResponseApplies(len(knowledges), attachments, hasImageAttachments) {
			if emptyResp, ok := promptConfig["empty_response"].(string); ok && emptyResp != "" {
				finalReference := emptyResponseReference(kbinfos, knowledges, quote)
				if !sendResult(AsyncChatResult{
					Answer:    emptyResp,
					Reference: map[string]interface{}{},
				}) {
					return
				}
				sendResult(AsyncChatResult{
					Answer:      emptyResp,
					Reference:   finalReference,
					AudioBinary: s.synthesizeTTS(ctx, ttsModel, emptyResp),
					Prompt:      fmt.Sprintf("\n\n### Query:\n%s", strings.Join(questions, " ")),
					Final:       true,
				})
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
			systemPrompt = s.formatPrompt(sp, kwargs)
			// If knowledge was retrieved but the template has no {knowledge}
			// placeholder, auto-append it so the LLM still sees the context.
			if len(knowledges) > 0 && !strings.Contains(sp, "{knowledge}") {
				if kw, ok := kwargs["knowledge"].(string); ok {
					systemPrompt += kw
				}
			}
		}
		if systemPrompt != "" {
			// Python logs characters; a Chinese prompt is ~3x longer in bytes, so
			// report both to keep the two comparable.
			common.Info("System prompt built",
				zap.Int("length", len(systemPrompt)),
				zap.Int("runes", utf8.RuneCountInString(systemPrompt)),
				zap.Int("knowledgeRunes", utf8.RuneCountInString(knowledge)))
		}

		// Build citation prompt if quoting is enabled.
		prompt4citation := ""
		if len(knowledges) > 0 && quote {
			// Python's citation_prompt() (generator.py:226) renders
			// citation_prompt.md — the full rules with examples, which the agent
			// path already reads from the same embedded copy. The standard path
			// used a ~380-char paraphrase of it.
			prompt4citation = ragprompts.CitationPrompt("")
		}

		if prompt4citation != "" {
			common.Info("Citation prompt built",
				zap.Bool("quote", quote),
				zap.Int("length", len(prompt4citation)),
				zap.Int("runes", utf8.RuneCountInString(prompt4citation)))
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
		modelType, _ := llmModelConfig["model_type"].(string)
		for i, m := range messages {
			role, _ := m["role"].(string)
			if role == "system" && kwargs["store_history_messages"] != false {
				continue
			}
			llmMessage := normalizeLLMMessage(m)
			if i < len(messages)-1 {
				llmMessage["files"] = m["files"]
			}
			content := llmMessage["content"]
			if contentStr, ok := content.(string); ok {
				content = cleanCitationMarkers(contentStr)
			}
			llmMessage["content"] = content
			llmMessages = append(llmMessages, llmMessage)
		}
		if attachments != "" && len(llmMessages) > 0 {
			if lastContent, ok := llmMessages[len(llmMessages)-1]["content"].(string); ok {
				llmMessages[len(llmMessages)-1]["content"] = lastContent + attachments
			}
		}

		// Fit messages within token budget.
		usedTokenCount, llmMessages := s.messageFitIn(llmMessages, int(float64(modelMaxTokens)*0.95))
		for i := range llmMessages[:len(llmMessages)-1] {
			llmMessages[i], err = s.chatHistoryMessage(ctx, llmMessages[i], modelType, factoryName)
			if err != nil {
				sendResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", err), Final: true})
				return
			}
		}
		common.Debug("Messages fitted in token budget",
			zap.Int("model max_tokens", modelMaxTokens),
			zap.Int("used_token_count", usedTokenCount),
			zap.Int("msg_count", len(llmMessages)))

		// Multimodal conversion. imageFiles only survives the vision gate
		// above, so only vision-capable (image2text) models reach this
		// with images.
		if len(llmMessages) >= 2 && len(imageFiles) > 0 {
			lastIdx := len(llmMessages) - 1
			if role, _ := llmMessages[lastIdx]["role"].(string); role == "user" {
				if converted, err := common.ConvertLastUserMsgToMultimodal(
					llmMessages[lastIdx],
					imageFiles,
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
			sendResult(AsyncChatResult{
				Answer: "**ERROR**: message_fit_in has bug",
				Final:  true,
			})
			return
		}

		// The citation template joins the prompt only when the request is issued
		// (prompt+prompt4citation, below), which is after messageFitIn sized the
		// window — so its tokens are added here by hand. They are deliberately NOT
		// folded into messageFitIn: the reference implementation trims on the same
		// budget (dialog_service async_chat: message_fit_in(msg, max_tokens*0.95),
		// then prompt + prompt4citation), and moving the trim boundary would change
		// which messages get cut. The completion clamp is where the omission bites:
		// it derives the completion budget from the room the prompt leaves, so
		// leaving ~2k tokens of citation instructions out of that sum can hand the
		// provider a prompt plus completion that overruns the context window.
		citationTokens := 0
		if prompt4citation != "" {
			citationTokens = graph.NumTokensFromString(prompt4citation)
		}
		if adjusted, ok, err := clampChatConfigMaxTokens(chatCfg, modelMaxTokens, usedTokenCount+citationTokens); err != nil {
			sendResult(AsyncChatResult{
				Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
				Final:  true,
			})
			return
		} else if ok {
			common.Debug("Adjusted max_tokens", zap.Int("max_tokens in chat", adjusted))
		}

		// === Phase 11: Drive LLM + Decorate Answer ===
		// Stream path: accumulate deltas → per-delta TTS → decorate final.
		// Non-stream path: one-shot chat → decorate (includes TTS).
		// Answer decoration: citation markers, references, timing stats, Langfuse.
		common.Info("Phase 11: Drive LLM + Decorate Answer",
			zap.Bool("stream", stream),
			zap.Int("llm_messages_count", len(llmMessages)))
		timer.Enter(common.PhaseGenerateAnswer)
		chatDriver := s.buildChatDriver(ctx, chat, chatModel)
		if chatDriver == nil {
			sendResult(AsyncChatResult{
				Answer: "**ERROR**: No chat model available for this chat.",
				Final:  true,
			})
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
			var answerFilter, reasoningFilter citationStreamFilter
			emit := func(result AsyncChatResult) bool {
				if !result.Final && result.Answer != "" {
					result.AudioBinary = s.synthesizeTTS(ctx, ttsModel, result.Answer)
				}
				return sendResult(result)
			}
			send := func(result AsyncChatResult) bool {
				if !quote {
					// Flush before a boundary so buffered text stays in its section.
					if result.StartToThink || result.EndToThink || result.Final {
						if text := reasoningFilter.flush(); text != "" {
							if !emit(AsyncChatResult{Reasoning: text, Reference: map[string]interface{}{}, CreatedAt: float64(time.Now().Unix())}) {
								return false
							}
						}
						if text := answerFilter.flush(); text != "" {
							if !emit(AsyncChatResult{Answer: text, Reference: map[string]interface{}{}, CreatedAt: float64(time.Now().Unix())}) {
								return false
							}
						}
					}
					if !result.Final && (result.Answer != "" || result.Reasoning != "") {
						result.Answer = answerFilter.write(result.Answer)
						result.Reasoning = reasoningFilter.write(result.Reasoning)
						if result.Answer == "" && result.Reasoning == "" {
							return true
						}
					}
				}
				return emit(result)
			}

			// Tool routing: use tool-loop method when tools are bound.
			var driverErr error
			onToolDelta := func(answerDelta *string, reason *string) error {
				// Tool streaming path:
				// Wraps reasoning in <think></think> markers.
				// inThink tracks local state to route reasoning vs answer.
				var inThink bool
				if answerDelta == nil || *answerDelta == "" {
					return nil
				}
				text := *answerDelta
				fullAnswer += text

				if text == "<think>" {
					inThink = true
					if !send(AsyncChatResult{
						Answer:       "",
						Reference:    map[string]interface{}{},
						AudioBinary:  nil,
						CreatedAt:    float64(time.Now().Unix()),
						Final:        false,
						StartToThink: true,
					}) {
						return ctx.Err()
					}
					return nil
				}
				if text == "</think>" {
					inThink = false
					if !send(AsyncChatResult{
						Answer:      "",
						Reference:   map[string]interface{}{},
						AudioBinary: nil,
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
						EndToThink:  true,
					}) {
						return ctx.Err()
					}
					return nil
				}
				if inThink {
					// Reasoning text — route to Reasoning field so
					// the SSE handler maps it to
					// `delta.reasoning_content`.
					if !send(AsyncChatResult{
						Reasoning:   text,
						Reference:   map[string]interface{}{},
						AudioBinary: nil,
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
					}) {
						return ctx.Err()
					}
				} else {
					// Regular answer content
					if !send(AsyncChatResult{
						Answer:    text,
						Reference: map[string]interface{}{},
						CreatedAt: float64(time.Now().Unix()),
						Final:     false,
					}) {
						return ctx.Err()
					}
				}
				return nil
			}
			onDelta := func(answer *string, reason *string) error {
				if reason != nil && *reason != "" {
					if thinkState.EnterReasoning() {
						if !send(AsyncChatResult{
							Answer:       "",
							Reference:    map[string]interface{}{},
							AudioBinary:  nil,
							CreatedAt:    float64(time.Now().Unix()),
							Final:        false,
							StartToThink: true,
						}) {
							return ctx.Err()
						}
					}
					deltas := NextThinkDelta(thinkState, *reason, 16)
					for _, d := range deltas {
						if d.Kind == ThinkDeltaText && d.Value != "" {
							fullAnswer += d.Value
							if !send(AsyncChatResult{
								Answer:    d.Value,
								Reference: map[string]interface{}{},
								CreatedAt: float64(time.Now().Unix()),
								Final:     false,
							}) {
								return ctx.Err()
							}
						}
					}
				}
				if isContentDelta(answer) {
					if thinkState.ExitReasoning() {
						for _, d := range FlushRemaining(thinkState) {
							if d.Kind == ThinkDeltaText && d.Value != "" {
								fullAnswer += d.Value
								if !send(AsyncChatResult{
									Answer:    d.Value,
									Reference: map[string]interface{}{},
									CreatedAt: float64(time.Now().Unix()),
									Final:     false,
								}) {
									return ctx.Err()
								}
							}
						}
						if !send(AsyncChatResult{
							Answer:      "",
							Reference:   map[string]interface{}{},
							AudioBinary: nil,
							CreatedAt:   float64(time.Now().Unix()),
							Final:       false,
							EndToThink:  true,
						}) {
							return ctx.Err()
						}
					}
					fullAnswer += *answer
					deltas := BufferAnswerDelta(thinkState, *answer, 16)
					for _, d := range deltas {
						if d.Kind == ThinkDeltaText && d.Value != "" {
							if !send(AsyncChatResult{
								Answer:    d.Value,
								Reference: map[string]interface{}{},
								CreatedAt: float64(time.Now().Unix()),
								Final:     false,
							}) {
								return ctx.Err()
							}
						}
					}
				}
				return nil
			}
			// Failover: a gp- group reference whose invocation fails before
			// any delta reached the client (canRetry sees fullAnswer) flips
			// its current member inactive and retries once on the next
			// member. Once deltas have streamed the answer cannot be replayed
			// on another model, so the error surfaces as-is.
			driverErr = dao.InvokeWithGroupFailover(ctx, dao.DB, chat.TenantID, entity.ModelTypeChat, chat.LLMID,
				func(context.Context) (*modelModule.ChatModel, error) { return chatDriver, nil },
				func(ctx context.Context) (*modelModule.ChatModel, error) {
					fresh, rerr := s.ModelFactory.NewChatModel(permission.WithTenantID(ctx, chat.TenantID), chat.LLMID)
					if rerr == nil {
						rebindToolcallSession(fresh, kwargs)
					}
					return fresh, rerr
				},
				func(ctx context.Context, model *modelModule.ChatModel) error {
					fullAnswer = ""
					thinkState = &ThinkStreamState{}
					if model.ToolConfig != nil {
						_, err := model.ChatStreamlyWithTools(ctx, prompt+prompt4citation, chatMessages, chatCfg, onToolDelta)
						return err
					}
					return model.ChatStreamlyWithSender(ctx, chatMessages, chatCfg, nil, onDelta)
				},
				func(error) bool { return fullAnswer == "" })
			if driverErr != nil {
				if ctx.Err() != nil {
					return
				}
				send(AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", driverErr.Error()),
					Final:  true,
				})
				return
			}
			timer.Exit(common.PhaseGenerateAnswer)
			if ctx.Err() != nil {
				return
			}

			// Flush remaining state matching Python's final flush order
			// (dialog_service.py:1601-1612): think_buffer → marker → answer_buffer → pending_after_close
			// Python has no Reasoning field — all text is Answer.
			hadThinkClose := false
			for _, d := range FlushRemaining(thinkState) {
				if d.Kind == ThinkDeltaMarker && d.Value == "</think>" {
					hadThinkClose = true
					if !send(AsyncChatResult{
						Answer:      "",
						Reference:   map[string]interface{}{},
						AudioBinary: nil,
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
						EndToThink:  true,
					}) {
						return
					}
				} else if d.Kind == ThinkDeltaText && d.Value != "" {
					if !send(AsyncChatResult{
						Answer:    d.Value,
						Reference: map[string]interface{}{},
						CreatedAt: float64(time.Now().Unix()),
						Final:     false,
					}) {
						return
					}
				}
			}
			// Close reasoning if the stream ended while still in reasoning mode
			// (e.g. model returned only reasoning chunks with no content delta).
			// Skip when FlushRemaining already emitted a </think> marker.
			if !hadThinkClose && thinkState.ExitReasoning() {
				if !send(AsyncChatResult{
					Answer:      "",
					Reference:   map[string]interface{}{},
					AudioBinary: nil,
					CreatedAt:   float64(time.Now().Unix()),
					Final:       false,
					EndToThink:  true,
				}) {
					return
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
			send(final)
		} else {
			// Non-streaming: get the answer synchronously, with gp- group
			// failover (flip + re-resolve + retry once on invocation error).
			var answer string
			err := dao.InvokeWithGroupFailover(ctx, dao.DB, chat.TenantID, entity.ModelTypeChat, chat.LLMID,
				func(context.Context) (*modelModule.ChatModel, error) { return chatDriver, nil },
				func(ctx context.Context) (*modelModule.ChatModel, error) {
					fresh, rerr := s.ModelFactory.NewChatModel(permission.WithTenantID(ctx, chat.TenantID), chat.LLMID)
					if rerr == nil {
						rebindToolcallSession(fresh, kwargs)
					}
					return fresh, rerr
				},
				func(ctx context.Context, model *modelModule.ChatModel) error {
					if model.ToolConfig != nil {
						a, _, callErr := model.ChatWithTools(ctx, prompt+prompt4citation, chatMessages, chatCfg)
						answer = a
						return callErr
					}
					resp, callErr := model.ChatWithMessages(ctx, chatMessages, chatCfg, nil)
					if callErr != nil {
						return callErr
					}
					if resp != nil && resp.Answer != nil {
						answer = *resp.Answer
					}
					return nil
				},
				nil)
			timer.Exit(common.PhaseGenerateAnswer)

			if err != nil {
				sendResult(AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
					Final:  true,
				})
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
			sendResult(final)
		}
		common.Info("AsyncChat completed", zap.String("chat_id", chat.ID))
	}()

	return out, nil
}

func validateUserMessageContent(messages []map[string]interface{}) error {
	for i, message := range messages {
		role, _ := message["role"].(string)
		if role != "user" {
			continue
		}
		content, ok := message["content"]
		if !ok || content == nil {
			return fmt.Errorf("messages[%d].content is required for user messages", i)
		}
		if _, err := NormalizeOpenAIMessageContent(content); err != nil {
			return fmt.Errorf("messages[%d].content is invalid: %w", i, err)
		}
	}
	return nil
}

// deepResearchProgressCallback builds the progress callback handed to
// DeepResearcher.Research. Progress deltas are non-essential: once the
// consumer is gone (ctx canceled) they are dropped instead of blocking, so
// parallel sub-research goroutines can drain before Research returns and
// the pipeline goroutine closes the channel.
func (s *ChatPipelineService) deepResearchProgressCallback(ctx context.Context, out chan<- AsyncChatResult) func(string) {
	return func(msg string) {
		answer := msg
		switch {
		case strings.HasPrefix(msg, "<START_DEEP_RESEARCH>"):
			answer = "<retrieving>"
		case strings.HasPrefix(msg, "<END_DEEP_RESEARCH>"):
			answer = "</retrieving>"
		}
		select {
		case out <- AsyncChatResult{
			Answer:      answer,
			Reference:   map[string]interface{}{},
			AudioBinary: nil,
			Final:       false,
		}:
		case <-ctx.Done():
		}
	}
}

// AsyncChatSolo is the LLM-only chat path (no KBs, no web search).
// Equivalent to Python's async_chat_solo() in dialog_service.py:289-337.
func (s *ChatPipelineService) AsyncChatSolo(
	ctx context.Context,
	chat *entity.Chat,
	messages []map[string]interface{},
	stream bool,
	config map[string]interface{},
) (<-chan AsyncChatResult, error) {

	out := make(chan AsyncChatResult, 16)

	go func() {
		defer close(out)

		// Timer brackets the LLM call; other phases are N/A in solo mode.
		timer := common.NewTimer()
		timer.Start()

		sendResult := func(res AsyncChatResult) bool {
			if ctx.Err() != nil {
				return false
			}
			select {
			case out <- res:
				return true
			case <-ctx.Done():
				return false
			}
		}

		// 1. Resolve system prompt.
		promptConfig := chat.PromptConfig
		systemPrompt := ""
		if sp, ok := promptConfig["system"].(string); ok {
			systemPrompt = strings.ReplaceAll(sp, "{date}", time.Now().UTC().Format(time.RFC3339))
		}

		// 1b. Resolve LLM model config (needed early for model_type dispatch).
		llmModelConfig, _, _, _, err := s.getLLMModelConfig(ctx, chat)
		factoryName := ""
		if err == nil && llmModelConfig != nil {
			factoryName, _ = llmModelConfig["llm_factory"].(string)
		}
		if factoryName == "" {
			factoryName = factoryFromLLMID(chat.LLMID)
		}

		// 2. Process file attachments. Only vision-capable (image2text)
		// models receive image content; text-only chat models reject
		// image blocks at the provider (e.g. Zhipu GLM error 1210:
		// messages.content.type only allows 'text'), so their image
		// attachments are dropped here.
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
				var images []string
				attachmentsStr, images, err = s.splitChatAttachments(ctx, files)
				if err != nil {
					sendResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", err), Final: true})
					return
				}
				imageFiles, _ = gateImageAttachments(chat.LLMID, modelType, images)
				common.Info("AsyncChatSolo: file attachments resolved",
					zap.Bool("vision_model", isImage2Text),
					zap.Int("image_files", len(imageFiles)),
					zap.Int("text_attachment_bytes", len(attachmentsStr)))
			}
		}

		// 3. Strip citation markers; non-storing model tests retain payload system messages.
		var msg []map[string]interface{}
		for i, m := range messages {
			role, _ := m["role"].(string)
			if role == "system" && config["store_history_messages"] != false {
				continue
			}
			if i < len(messages)-1 {
				m, err = s.chatHistoryMessage(ctx, m, modelType, factoryName)
				if err != nil {
					sendResult(AsyncChatResult{Answer: fmt.Sprintf("**ERROR**: %s", err), Final: true})
					return
				}
			}
			llmMessage := normalizeLLMMessage(m)
			content := llmMessage["content"]
			if contentStr, ok := content.(string); ok {
				content = cleanCitationMarkers(contentStr)
			}
			llmMessage["content"] = content
			msg = append(msg, llmMessage)
		}
		// Append text attachments to the last user message (no separator).
		if attachmentsStr != "" && len(msg) > 0 {
			if lastContent, ok := msg[len(msg)-1]["content"].(string); ok {
				msg[len(msg)-1]["content"] = lastContent + attachmentsStr
			}
		}

		// 4. Construct the selected chat model through the factory.
		modelCtx := permission.WithTenantID(ctx, chat.TenantID)
		var chatModel *modelModule.ChatModel
		if strings.TrimSpace(chat.LLMID) == "" {
			chatModel, err = s.ModelFactory.NewDefaultChatModel(modelCtx)
		} else {
			chatModel, err = s.ModelFactory.NewChatModel(modelCtx, chat.LLMID)
		}
		if err != nil {
			common.WarnCtx(ctx, "chat completions: chat model construction failed",
				zap.String("tenant_id", chat.TenantID),
				zap.String("llm_id", chat.LLMID),
				zap.Error(err))
			sendResult(AsyncChatResult{
				Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
				Final:  true,
			})
			return
		}

		// 5. Resolve TTS model. Best-effort: warn and proceed without TTS on lookup failure.
		var ttsModel *modelModule.TTSModel
		if promptConfig != nil {
			if useTTS, _ := promptConfig["tts"].(bool); useTTS {
				ttsModel, err = s.ModelFactory.NewDefaultTTSModel(modelCtx)
				if err != nil || ttsModel == nil {
					common.Warn("AsyncChatSolo: TTS lookup failed; proceeding without TTS",
						zap.String("tenant_id", chat.TenantID),
						zap.Error(err))
					ttsModel = nil
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
			m["content"] = content
			chatMessages = append(chatMessages, modelMessageFromMap(m))
		}

		// 7. Drive the LLM: stream (per-delta with think markers) or non-stream (one-shot).
		if stream {
			var fullAnswer string
			thinkState := &ThinkStreamState{}
			chatCfg := BuildChatConfig(chat, config)
			timer.Enter(common.PhaseGenerateAnswer)

			onDelta := func(answer *string, reason *string) error {
				if reason != nil && *reason != "" {
					if thinkState.EnterReasoning() {
						if !sendResult(AsyncChatResult{
							Answer:       "",
							Reference:    map[string]interface{}{},
							AudioBinary:  nil,
							CreatedAt:    float64(time.Now().Unix()),
							Final:        false,
							StartToThink: true,
						}) {
							return ctx.Err()
						}
					}
					deltas := NextThinkDelta(thinkState, *reason, 16)
					for _, d := range deltas {
						if d.Kind == ThinkDeltaText && d.Value != "" {
							fullAnswer += d.Value
							if !sendResult(AsyncChatResult{
								Answer:      d.Value,
								Reference:   map[string]interface{}{},
								AudioBinary: s.synthesizeTTS(ctx, ttsModel, d.Value),
								CreatedAt:   float64(time.Now().Unix()),
								Final:       false,
							}) {
								return ctx.Err()
							}
						}
					}
				}
				if isContentDelta(answer) {
					if thinkState.ExitReasoning() {
						for _, d := range FlushRemaining(thinkState) {
							if d.Kind == ThinkDeltaText && d.Value != "" {
								fullAnswer += d.Value
								if !sendResult(AsyncChatResult{
									Answer:      d.Value,
									Reference:   map[string]interface{}{},
									AudioBinary: s.synthesizeTTS(ctx, ttsModel, d.Value),
									CreatedAt:   float64(time.Now().Unix()),
									Final:       false,
								}) {
									return ctx.Err()
								}
							}
						}
						if !sendResult(AsyncChatResult{
							Answer:      "",
							Reference:   map[string]interface{}{},
							AudioBinary: nil,
							CreatedAt:   float64(time.Now().Unix()),
							Final:       false,
							EndToThink:  true,
						}) {
							return ctx.Err()
						}
					}
					fullAnswer += *answer
					deltas := BufferAnswerDelta(thinkState, *answer, 16)
					for _, d := range deltas {
						if d.Kind == ThinkDeltaText && d.Value != "" {
							if !sendResult(AsyncChatResult{
								Answer:      d.Value,
								Reference:   map[string]interface{}{},
								AudioBinary: s.synthesizeTTS(ctx, ttsModel, d.Value),
								CreatedAt:   float64(time.Now().Unix()),
								Final:       false,
							}) {
								return ctx.Err()
							}
						}
					}
				}
				return nil
			}
			// Failover: a gp- group reference whose invocation fails before
			// any delta reached the client flips its current member inactive
			// and retries once on the next member. Once deltas have streamed
			// the answer cannot be replayed on another model, so the error
			// surfaces as-is.
			driverErr := dao.InvokeWithGroupFailover(ctx, dao.DB, chat.TenantID, entity.ModelTypeChat, chat.LLMID,
				func(context.Context) (*modelModule.ChatModel, error) { return chatModel, nil },
				func(ctx context.Context) (*modelModule.ChatModel, error) {
					return s.ModelFactory.NewChatModel(modelCtx, chat.LLMID)
				},
				func(ctx context.Context, model *modelModule.ChatModel) error {
					thinkState = &ThinkStreamState{}
					fullAnswer = ""
					return model.ChatStreamlyWithSender(ctx, chatMessages, chatCfg, nil, onDelta)
				},
				func(error) bool { return fullAnswer == "" })
			if driverErr != nil {
				if ctx.Err() != nil {
					return
				}
				sendResult(AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", driverErr.Error()),
					Final:  true,
				})
				return
			}
			if ctx.Err() != nil {
				return
			}
			timer.Exit(common.PhaseGenerateAnswer)
			hadThinkClose := false
			for _, d := range FlushRemaining(thinkState) {
				if d.Kind == ThinkDeltaMarker && d.Value == "</think>" {
					hadThinkClose = true
					if !sendResult(AsyncChatResult{
						Answer:      "",
						Reference:   map[string]interface{}{},
						AudioBinary: nil,
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
						EndToThink:  true,
					}) {
						return
					}
				} else if d.Kind == ThinkDeltaText && d.Value != "" {
					if !sendResult(AsyncChatResult{
						Answer:      d.Value,
						Reference:   map[string]interface{}{},
						AudioBinary: s.synthesizeTTS(ctx, ttsModel, d.Value),
						CreatedAt:   float64(time.Now().Unix()),
						Final:       false,
					}) {
						return
					}
				}
			}
			// Close reasoning if the stream ended while still in reasoning mode
			// (e.g. model returned only reasoning chunks with no content delta).
			// Skip when FlushRemaining already emitted a </think> marker.
			if !hadThinkClose && thinkState.ExitReasoning() {
				if !sendResult(AsyncChatResult{
					Answer:      "",
					Reference:   map[string]interface{}{},
					AudioBinary: nil,
					CreatedAt:   float64(time.Now().Unix()),
					Final:       false,
					EndToThink:  true,
				}) {
					return
				}
			}
			finalAnswer := ExtractVisibleAnswer(thinkState.fullText)
			if finalAnswer == "" {
				finalAnswer = fullAnswer
			}
			sendResult(AsyncChatResult{
				Answer:      finalAnswer,
				Reference:   map[string]interface{}{},
				AudioBinary: nil,
				CreatedAt:   float64(time.Now().Unix()),
				Final:       true,
			})
		} else {
			// Non-streaming: one-shot call, with gp- group failover (flip +
			// re-resolve + retry once on invocation error).
			chatCfg := BuildChatConfig(chat, config)
			var resp *modelModule.ChatResponse
			err := dao.InvokeWithGroupFailover(ctx, dao.DB, chat.TenantID, entity.ModelTypeChat, chat.LLMID,
				func(context.Context) (*modelModule.ChatModel, error) { return chatModel, nil },
				func(ctx context.Context) (*modelModule.ChatModel, error) {
					return s.ModelFactory.NewChatModel(modelCtx, chat.LLMID)
				},
				func(ctx context.Context, model *modelModule.ChatModel) error {
					timer.Enter(common.PhaseGenerateAnswer)
					r, callErr := model.ChatWithMessages(ctx, chatMessages, chatCfg, nil)
					timer.Exit(common.PhaseGenerateAnswer)
					if callErr != nil {
						return callErr
					}
					resp = r
					return nil
				},
				nil)
			if err != nil {
				sendResult(AsyncChatResult{
					Answer: fmt.Sprintf("**ERROR**: %s", err.Error()),
					Final:  true,
				})
				return
			}
			answer := ""
			if resp != nil && resp.Answer != nil {
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
			sendResult(AsyncChatResult{
				Answer:      answer,
				Reference:   map[string]interface{}{},
				AudioBinary: s.synthesizeTTS(ctx, ttsModel, answer),
				CreatedAt:   float64(time.Now().Unix()),
				Final:       true,
			})
		}
	}()

	return out, nil
}

// flipChatGroupMember deactivates the currently picked member of a gp- chat
// model reference (best-effort) so the next NewChatModel on the same
// reference picks the next member, mirroring the ingestion dispatchers'
// failover. No-op for non-group references; failures are logged without
// masking the original model error.
func (s *ChatPipelineService) flipChatGroupMember(ctx context.Context, tenantID, llmID string) {
	groupID, ok := common.ParseGroupModelRef(llmID)
	if !ok {
		return
	}
	if _, err := dao.MarkGroupMemberInactive(ctx, dao.DB, strings.TrimSpace(tenantID), groupID, entity.ModelTypeChat); err != nil {
		common.WarnCtx(ctx, "chat completions: model group failover flip failed",
			zap.String("llm_id", llmID),
			zap.Error(err))
	}
}

// text content (entries joined by "\n\n") and image data URIs, reading
// each blob from storage at most once.
//
//   - File-dict mode (the chat UI's upload_info flow): FileService.
//     GetFileContents fetches the blobs; non-visual files are parsed to
//     text, visual files come back as base64 data URIs.
//   - String mode (pre-resolved content): data:-prefixed entries become
//     images; the remaining non-empty entries become text.
//
// Downstream ConvertLastUserMsgToMultimodal → parseDataURIOrB64 accepts the
// returned data URIs directly.
func (s *ChatPipelineService) splitChatAttachments(ctx context.Context, files interface{}) (string, []string, error) {
	// ── File-dict mode ──
	if fileDicts, ok := parseFileDicts(files); ok {
		// Only used for GetFileContents (read-only); nil DocRemover means
		// this FileService MUST NOT be used for DeleteFiles.
		fileSvc := file.NewFileService(nil)
		texts, images, err := fileSvc.GetFileContents(ctx, fileDicts)
		if err != nil {
			if errors.Is(err, utility.ErrFeatureDisabled) {
				return "", nil, err
			}
			common.Warn("GetFileContents failed in splitChatAttachments",
				zap.Error(err))
			return "", nil, nil
		}
		if len(texts) == 0 {
			return "", images, nil
		}
		return strings.Join(texts, "\n\n"), images, nil
	}

	// ── String mode ──
	var texts []string
	var images []string
	switch v := files.(type) {
	case []string:
		for _, f := range v {
			if f = strings.TrimSpace(f); f == "" {
				continue
			}
			if strings.HasPrefix(f, "data:") {
				images = append(images, f)
			} else {
				texts = append(texts, f)
			}
		}
	case []interface{}:
		for _, f := range v {
			str, ok := f.(string)
			if !ok {
				continue
			}
			if str = strings.TrimSpace(str); str == "" {
				continue
			}
			if strings.HasPrefix(str, "data:") {
				images = append(images, str)
			} else {
				texts = append(texts, str)
			}
		}
	}
	if len(texts) == 0 {
		return "", images, nil
	}
	return strings.Join(texts, "\n\n"), images, nil
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

func (s *ChatPipelineService) shouldUseWebSearch(chat *entity.Chat, internet interface{}) bool {
	if chat.PromptConfig == nil {
		return false
	}
	if resolveWebSearchProvider(chat.PromptConfig) == nil {
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

	client := tavilyWebSearchHTTPClient
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

var tokenizeWhitespaceRe = regexp.MustCompile(`\s+`)

// tokenizeText is a lightweight tokenizer for Tavily content.
// It lowercases and splits on whitespace, similar to rag_tokenizer.tokenize.
func tokenizeText(text string) string {
	// Collapse multiple whitespaces and split.
	text = tokenizeWhitespaceRe.ReplaceAllString(text, " ")
	// Convert to lowercase for tokenization.
	return strings.ToLower(text)
}

// getLLMModelConfig resolves the LLM model configuration for the chat.
// Mirrors Python's three-branch resolver at dialog_service.py:552-561. Chat
// model resolution always requires the enrolled Chat type; vision capability
// is determined separately for attachment dispatch:
//
//	if chat.llm_id:
//	    if "chat" and "image2text" in get_model_type_by_name(...): → IMAGE2TEXT
//	    else:                                                       → CHAT
//	else:                                                → tenant default
//	    (IMAGE2TEXT when the default model is vision-capable, else CHAT)
//
// The returned `cfg` map's "model_type" field carries the chosen type.
// Downstream code gates image attachments on it: only image2text
// (vision-capable) models receive image content blocks; text-only chat
// models reject them at the provider (e.g. Zhipu GLM error 1210:
// messages.content.type only allows 'text').
func (s *ChatPipelineService) getLLMModelConfig(ctx context.Context, chat *entity.Chat) (map[string]interface{}, string, string, string, error) {
	ctx = permission.WithTenantID(ctx, chat.TenantID)
	var info *modelModule.ModelInfo
	var err error
	if strings.TrimSpace(chat.LLMID) == "" {
		info, err = s.ModelFactory.ResolveDefaultInfo(ctx, entity.ModelTypeChat)
	} else {
		info, err = s.ModelFactory.ResolveInfo(ctx, entity.ModelTypeChat, chat.LLMID)
	}
	if err != nil {
		return nil, "", "", "", err
	}
	cfg, modelName, factoryName, baseURL, err := s.buildLLMModelConfig(info)
	if err != nil {
		return nil, "", "", "", err
	}
	cfg["model_type"] = "chat"
	if entity.ModelTypeFromStrings(info.ModelTypes).Has(entity.ModelTypeImage2Text) {
		cfg["model_type"] = "image2text"
	}
	cfg["is_tools"] = info.SupportsTools
	return cfg, modelName, factoryName, baseURL, nil
}

// buildLLMModelConfig returns only the public metadata needed by RAG's token
// fitting and attachment dispatch. The persisted legacy max_tokens value is
// the input/context budget; max_output is the catalog output limit, which
// sizes the completion reserve and which ChatModel enforces on generation.
func (s *ChatPipelineService) buildLLMModelConfig(info *modelModule.ModelInfo) (map[string]interface{}, string, string, string, error) {
	if info == nil {
		return nil, "", "", "", fmt.Errorf("chat model metadata is unavailable")
	}
	contextLength := info.ContextLength
	if contextLength <= 0 {
		contextLength = 8192
	}
	cfg := map[string]interface{}{
		"model_type":  "chat",
		"llm_name":    info.Name,
		"max_tokens":  contextLength,
		"max_output":  info.MaxOutput,
		"llm_factory": info.ProviderName,
	}
	return cfg, info.Name, info.ProviderName, "", nil
}

// getModels resolves all models needed for the RAG pipeline.
// Mirrors Python's get_models() in dialog_service.py:340.
func (s *ChatPipelineService) getModels(ctx context.Context, chat *entity.Chat) (
	[]*entity.Knowledgebase,
	*modelModule.EmbeddingModel,
	*modelModule.RerankModel,
	*modelModule.ChatModel,
	*modelModule.TTSModel, // TTS model
	error,
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
		kbs, err = kbDAO.GetByIDs(ctx, dao.DB, kbIDs)
		if err != nil {
			common.Warn("Failed to get KBs by IDs; retrieval may be incomplete",
				zap.Strings("kbIDs", kbIDs), zap.Error(err))
		}
	}

	// Embedding model.
	var embModel *modelModule.EmbeddingModel
	if len(kbs) > 0 {
		if err := ValidateDatasetEmbeddingModels(ctx, dao.DB, kbs); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		if kbs[0].EmbdID != "" {
			embdTenantID := kbs[0].TenantID
			model, err := s.ModelFactory.NewEmbeddingModel(permission.WithTenantID(ctx, embdTenantID), kbs[0].EmbdID)
			if err != nil {
				common.Warn("Failed to get embedding model for chat retrieval",
					zap.String("embdID", kbs[0].EmbdID),
					zap.String("tenantID", embdTenantID),
					zap.Error(err))
				return nil, nil, nil, nil, nil, fmt.Errorf("failed to get embedding model: %w", err)
			}
			embModel = model
		}
	}

	// Chat model.
	modelCtx := permission.WithTenantID(ctx, chat.TenantID)
	var chatModel *modelModule.ChatModel
	if strings.TrimSpace(chat.LLMID) == "" {
		chatModel, _ = s.ModelFactory.NewDefaultChatModel(modelCtx)
	} else {
		chatModel, _ = s.ModelFactory.NewChatModel(modelCtx, chat.LLMID)
	}

	// Rerank model.
	var rerankModel *modelModule.RerankModel
	if chat.RerankID != "" {
		rerankModel, _ = s.ModelFactory.NewRerankModel(modelCtx, chat.RerankID)
	}

	// TTS model.
	var ttsModel *modelModule.TTSModel
	if chat.PromptConfig != nil {
		if useTTS, _ := chat.PromptConfig["tts"].(bool); useTTS {
			ttsModel, _ = s.ModelFactory.NewDefaultTTSModel(modelCtx)
		}
	}

	return kbs, embModel, rerankModel, chatModel, ttsModel, nil
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

// agenticRag drives the agentic (ReAct) conversation mode via eino ADK's
// adk.ChatModelAgent. It mirrors AsyncChat's channel contract: yields
// AsyncChatResult deltas (answer / reasoning / final) over a buffered channel
// consumed by the same callers (ChatCompletions / OpenAIChatCompletions).

// smartReasoningTimeout is the total wall-clock budget for one agentic agent
// run, shared by the model and every tool — tools carry no per-call limits of
// their own. The two HTTP entrypoints pass Request.Context(), which carries no
// deadline (http.Server.WriteTimeout does not become a handler context
// deadline), so without an explicit budget here a client that keeps the
// connection open could let the agent burn CPU indefinitely.
//
// The budget must cover the main research loop AND the whole delivery gate:
// auditing a deliverable costs one auditor sub-agent run per pass (deep
// list_chunks reads plus the echoed verdict), and each repair turn repeats the
// retrieval cycle. Ten minutes sufficed while the loop ended early; once the
// templates mandate constraint-first retrieval and multi-pass auditing, the
// main loop alone can consume half of it and the gate then dies mid-flight —
// every in-flight tool call fails with "context deadline exceeded" at the
// deadline (observed on q268, where two Elasticsearch queries died at exactly
// T+10m while the cluster was green, and the gate shipped pure narration
// because there was no budget left to repair or even to synthesize).
// Raised to 30 minutes on 2026-09-17 for the slow-plan experiment: with 10-minute per-call
// budgets a single stalled call plus the audit passes can consume most of 20 minutes and
// leave nothing for the synthesis turn.
var smartReasoningTimeout = 30 * time.Minute

// buildCodexReference turns a mode 8 answer's citations into the reference payload and
// rewrites the answer's provenance to that payload's numbering. The model ends each sourced
// line with a provenance tuple — `(doc: …, doc_id: …, chunk_id: <id>, snippet: "…")` for a
// corpus passage, `(source: web, url: <url>, title: <title>, snippet: "…")` for a web one —
// and this resolves them, strips the tuples and appends the markers (see
// agentic_rag.RewriteProvenance), so `[ID:N]` indexes reference.chunks[N].
//
// Both sources encode into ONE 0-based sequence: the corpus chunks that resolved first (in
// citation order), then the web passages (in citation order). The payload array carries the
// same order, so a single numbering covers both and the client needs no source-specific
// rule.
//
// Verification differs by source, deliberately:
//   - a corpus chunk_id must exist AND belong to the conversation's datasets
//     (fetchChunksByIDs). That is the whole of the corpus check — there is no served-chunk
//     ledger (see D12), so a chunk the model names but never retrieved still resolves as
//     long as it is in scope;
//   - a web url is the MODEL's own claim. The turn records nothing about what a search
//     served, so url/title/snippet are taken from the answer as written.
//
// An unresolvable corpus id keeps its tuple in the answer: an unverifiable citation stays
// visible instead of silently vanishing or pointing at a wrong index.
func (s *ChatPipelineService) buildCodexReference(ctx context.Context, tenantID string, datasetIDs []string, final string) (map[string]interface{}, string) {
	corpus := agentic_rag.ExtractCitedChunkIDs(final)
	web := agentic_rag.ExtractCitedWebSources(final)
	if len(corpus) == 0 && len(web) == 0 {
		return map[string]interface{}{}, final
	}
	byID := make(map[string]map[string]interface{}, len(corpus)+len(web))
	for _, r := range fetchChunksByIDs(ctx, datasetIDs, corpus) {
		if id, ok := r["id"].(string); ok {
			byID[id] = r
		}
	}
	// One numbering, corpus first then web, each in citation order. Only what resolved
	// takes a number: the marker IS the payload position.
	order := make([]string, 0, len(corpus)+len(web))
	for _, id := range corpus {
		if _, ok := byID[id]; ok {
			order = append(order, id)
		}
	}
	for _, w := range web {
		if _, dup := byID[w.URL]; dup {
			continue
		}
		title := w.Title
		if title == "" {
			title = w.URL
		}
		// Same shape the in-process web path builds (see buildAgenticReference), so the
		// client renders a web citation exactly like a corpus one.
		byID[w.URL] = map[string]interface{}{
			"id": w.URL, "doc_id": w.URL, "docnm_kwd": title,
			"content_with_weight": w.Snippet, "url": w.URL,
			"kb_id": []interface{}{}, "img_id": "", "positions": []interface{}{},
		}
		order = append(order, w.URL)
	}
	if len(order) == 0 {
		return map[string]interface{}{}, final
	}
	ordered := make([]map[string]interface{}, 0, len(order))
	index := make(map[string]int, len(order))
	for i, key := range order {
		ordered = append(ordered, byID[key])
		index[key] = i
	}
	marked := agentic_rag.RewriteProvenance(final, index)
	common.InfoCtx(ctx, "codex citations built",
		zap.Int("corpus_cited", len(corpus)),
		zap.Int("web_cited", len(web)),
		zap.Int("resolved", len(order)),
		zap.Int("final_bytes", len(marked)))
	return map[string]interface{}{
		"chunks":   chunksFormat(ordered),
		"doc_aggs": agenticDocAggs(ordered),
	}, marked
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

// gateImageAttachments applies the vision-capability policy to split
// attachments: images survive only for vision-capable (image2text-typed)
// models — text-only chat providers reject image content blocks (e.g.
// Zhipu GLM error 1210: messages.content.type only allows 'text'), so
// their images are dropped with a warning instead of failing the whole
// request. It returns the surviving images and whether any image was
// attached before the drop, so callers can still route image-bearing
// questions past the empty-response fallback. Mirrors Python's
// dialog_model_vision_capable gate in dialog_service.py.
func gateImageAttachments(llmID, modelType string, images []string) (kept []string, attached bool) {
	attached = len(images) > 0
	if attached && modelType != "image2text" {
		common.Warn("dropping image attachments for text-only chat model",
			zap.String("llm_id", llmID),
			zap.Int("dropped_images", len(images)))
		return nil, attached
	}
	return images, attached
}

// emptyResponseApplies reports whether the configured empty-response
// fallback should short-circuit the LLM call: only when retrieval found
// nothing AND the message carries no attachment context (text or images)
// the model could still answer from.
func emptyResponseApplies(knowledgeCount int, attachments string, imageAttachments bool) bool {
	return knowledgeCount == 0 && attachments == "" && !imageAttachments
}

// emptyResponseReference is the reference the empty-response fallback closes
// the stream with. With citations off it is empty. With citations on it is the
// retrieval result, unless retrieval returned chunks but none of them entered
// the prompt (no room in the knowledge budget): then it is empty too, so the
// client never gets sources for passages the answer could not have used.
func emptyResponseReference(kbinfos map[string]interface{}, knowledges []string, quote bool) map[string]interface{} {
	if !quote || (len(knowledges) == 0 && len(chunksFromKBInfos(kbinfos)) > 0) {
		return map[string]interface{}{}
	}
	return kbinfos
}

// splitFileAttachments mirrors Python's `split_file_attachments` at
// dialog_service.py:371-392. It separates `messages[-1]["files"]`
// into text-file content and image attachments.
//
// Two modes of operation:
//
//  1. File-dict mode: When `files` is `[]map[string]interface{}` (each dict
//     with keys "id", "created_by", "mime_type", "name"), the method calls
//     FileService.GetFileContents to fetch actual file blobs from storage;
//     images come back as base64 data URIs.
//
//  2. String-fallback mode: When `files` is `[]string` or `[]interface{}` of
//     strings (pre-resolved content), the method does simple string splitting:
//     - raw=false: split by "data:" prefix. Text → textAttachments; data:
//     URIs → image files.
//     - raw=true: all items go to textAttachments (Python's FileService.get_files
//     with raw=True pre-separates images, so non-image content arrives here).
func splitFileAttachments(ctx context.Context, files interface{}, raw bool) (textAttachments []string, imageAttachments []string, err error) {
	// ── Mode 1: file dicts (Python-compatible) ──
	if fileDicts, ok := parseFileDicts(files); ok {
		// Only used for GetFileContents (read-only); nil DocRemover means
		// this FileService MUST NOT be used for DeleteFiles.
		fileSvc := file.NewFileService(nil)
		texts, images, err := fileSvc.GetFileContents(ctx, fileDicts)
		if err != nil {
			if errors.Is(err, utility.ErrFeatureDisabled) {
				return nil, nil, err
			}
			common.Warn("GetFileContents failed, falling back to string splitting", zap.Error(err))
		} else {
			return texts, images, nil
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
		return texts, images, nil
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
	return texts, images, nil
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

var (
	ttsControlRe    = regexp.MustCompile(`[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]`)
	ttsEmojiRe      = regexp.MustCompile("[\U0001f600-\U0001f64f\U0001f300-\U0001f5ff\U0001f680-\U0001f6ff\U0001f1e0-\U0001f1ff\U00002700-\U000027bf\U0001f900-\U0001f9ff\U0001fa70-\U0001faff\U0001fad0-\U0001faff]+")
	ttsWhitespaceRe = regexp.MustCompile(`\s+`)
	ttsTagRe        = regexp.MustCompile(`<(/?)([A-Za-z][A-Za-z0-9:-]*)(?:\s+(?:[^<>"']|"[^"]*"|'[^']*')*)?\s*/?>`)
)

// cleanTTSText sanitizes text for TTS synthesis.
func cleanTTSText(text string) string {
	if text == "" {
		return ""
	}
	// Drop invalid UTF-8 before applying the character limit.
	text = strings.ToValidUTF8(text, "")
	// Strip control chars.
	text = ttsControlRe.ReplaceAllString(text, "")
	// Strip emojis.
	text = ttsEmojiRe.ReplaceAllString(text, "")
	text = stripTTSTags(text)
	// Collapse whitespace.
	text = ttsWhitespaceRe.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > 500 {
		text = string(r[:500])
	}
	return text
}

// stripTTSTags removes paired markup and standalone reasoning/void tags.
// Unmatched tags stay literal so compact comparisons such as a<b>c survive.
func stripTTSTags(text string) string {
	// Quoted attributes may contain > without ending the tag.
	matches := ttsTagRe.FindAllStringSubmatchIndex(text, -1)
	strip := make([]bool, len(matches))
	open := make(map[string][]int)
	for i, match := range matches {
		name := strings.ToLower(text[match[4]:match[5]])
		switch name {
		case "think", "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
			strip[i] = true
		default:
			if strings.HasSuffix(text[match[0]:match[1]], "/>") {
				strip[i] = true
			} else if match[2] == match[3] {
				open[name] = append(open[name], i)
			} else if stack := open[name]; len(stack) > 0 {
				strip[stack[len(stack)-1]] = true
				strip[i] = true
				open[name] = stack[:len(stack)-1]
			}
		}
	}
	var result strings.Builder
	last := 0
	for i, match := range matches {
		if strip[i] {
			result.WriteString(text[last:match[0]])
			last = match[1]
		}
	}
	result.WriteString(text[last:])
	return result.String()
}

// synthesizeTTS calls the TTS model to convert text to audio.
func (s *ChatPipelineService) synthesizeTTS(ctx context.Context, ttsModel *modelModule.TTSModel, text string) interface{} {
	// Filtered turns synthesize the sanitized final answer in filterChatOutput;
	// per-delta synthesis could speak a word before its remaining chunks arrive.
	if compliance.OutputFilterEnabled(ctx) {
		return nil
	}
	if ttsModel == nil || text == "" {
		return nil
	}
	text = cleanTTSText(text)
	if text == "" {
		return nil
	}
	ttsResp, err := ttsModel.Speech(ctx, &text, &modelModule.TTSConfig{Format: "mp3"}, nil)
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
func (s *ChatPipelineService) enrichChunksWithMetadata(ctx context.Context, kbinfos map[string]interface{}, tenantID string, fields []string) {
	chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{})
	if !ok || len(chunksRaw) == 0 {
		return
	}

	chunks := make([]map[string]interface{}, 0, len(chunksRaw))
	chunks = append(chunks, chunksRaw...)
	if len(chunks) == 0 {
		return
	}

	s.MetadataSvc.EnrichChunksWithDocMetadata(ctx, chunks, tenantID, fields)
}

// kbPrompt builds knowledge prompt blocks from retrieved chunks, keeping
// blocks until their tokens, headers included, pass 97% of maxTokens. A
// non-positive maxTokens leaves no room for any block.
func (s *ChatPipelineService) kbPrompt(kbinfos map[string]interface{}, maxTokens int) []string {
	chunksRaw, ok := kbinfos["chunks"].([]map[string]interface{})
	if !ok || len(chunksRaw) == 0 || maxTokens <= 0 {
		return nil
	}

	var result []string
	usedTokenCount := 0
	for i, ck := range chunksRaw {
		c := getMapString(ck, "content_with_weight", "content")
		if c == "" {
			continue
		}

		// The id is the chunk's pool position, which is also its index in the
		// reference the client receives (chunksFormat(chunksRaw)), so a marker copied
		// from a block opens exactly that passage. 0-based: the first block is "ID: 0".
		// (The compose renders a ranked subset instead, hence its own CiteChunkIDs order.)
		cnt := fmt.Sprintf("\nID: %d", i)
		cnt += drawNode("Title", getMapString(ck, "docnm_kwd", "document_name"))
		cnt += drawNode("URL", getMapString(ck, "url"))
		if meta, ok := ck["document_metadata"].(map[string]interface{}); ok {
			keys := make([]string, 0, len(meta))
			for k := range meta {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				cnt += drawNode(k, meta[k])
			}
		}
		cnt += "\n└── Content:\n"
		cnt += c
		result = append(result, cnt)

		usedTokenCount += graph.NumTokensFromString(cnt)
		if float64(maxTokens)*0.97 < float64(usedTokenCount) {
			common.Warn("Not all the retrieval into prompt",
				zap.Int("kept", len(result)),
				zap.Int("total", len(chunksRaw)))
			break
		}
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
		if role == "" {
			continue
		}
		result = append(result, modelMessageFromMap(m))
	}
	return result
}

func (s *ChatPipelineService) chatHistoryMessage(ctx context.Context, message map[string]interface{}, modelType, factory string) (map[string]interface{}, error) {
	if message["role"] != "user" || message["files"] == nil {
		return message, nil
	}
	_, images, attachmentErr := s.splitChatAttachments(ctx, message["files"])
	if attachmentErr != nil {
		return nil, attachmentErr
	}
	images, _ = gateImageAttachments("", modelType, images)
	msg := normalizeLLMMessage(message)
	converted, err := common.ConvertLastUserMsgToMultimodal(msg, images, factory)
	if err != nil {
		common.Warn("chat history multimodal conversion failed", zap.Error(err))
		return msg, nil
	}
	return converted, nil
}

func normalizeLLMMessage(message map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(llmMessageFields))
	for _, field := range llmMessageFields {
		if value, ok := message[field]; ok {
			normalized[field] = value
		}
	}
	return normalized
}

func modelMessageFromMap(message map[string]interface{}) modelModule.Message {
	msg := modelModule.Message{Role: stringFromMap(message, "role"), Content: message["content"], Name: message["name"], FunctionCall: message["function_call"], Refusal: message["refusal"], Audio: message["audio"]}
	msg.ToolCallID, _ = message["tool_call_id"].(string)
	switch toolCalls := message["tool_calls"].(type) {
	case []map[string]interface{}:
		msg.ToolCalls = toolCalls
	case []interface{}:
		for _, toolCall := range toolCalls {
			if value, ok := toolCall.(map[string]interface{}); ok {
				msg.ToolCalls = append(msg.ToolCalls, value)
			}
		}
	}
	return msg
}

// buildChatDriver creates a ChatModel wrapper from the chat.
func (s *ChatPipelineService) buildChatDriver(ctx context.Context, chat *entity.Chat, chatModel *modelModule.ChatModel) *modelModule.ChatModel {
	if chatModel != nil {
		return chatModel
	}
	modelCtx := permission.WithTenantID(ctx, chat.TenantID)
	if strings.TrimSpace(chat.LLMID) == "" {
		model, err := s.ModelFactory.NewDefaultChatModel(modelCtx)
		if err != nil {
			return nil
		}
		return model
	}
	model, err := s.ModelFactory.NewChatModel(modelCtx, chat.LLMID)
	if err != nil {
		return nil
	}
	return model
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
//   - chunks whose `vector` already carries signal (non-zero components)
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
//   - dim: embedding dimension, used to name the q_{dim}_vec field. Zero means
//     "infer from the chunks"; the agentic evidence pool carries no chunk
//     vectors at all, so its caller passes the model dimension explicitly.
func HydrateChunkVectors(ctx context.Context, kbinfos map[string]interface{}, tenantIDs []string, kbIDs []string, dim int, docEngine engine.DocEngine) (int, error) {
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

	// A vector counts as present only when it carries signal. The agentic
	// evidence pool reaches this function with no `vector` key at all (the
	// runtime chunk shape has no vector field), and the naive path ships zero
	// placeholders for chunks whose embedding was not selected. Python refills
	// both, because _hydrate_chunk_vectors treats a chunk as already hydrated
	// only when `any(x for x in v)` (dialog_service.py:93-95). A length check
	// alone skips exactly the chunks hydration exists for, leaving them at zero
	// similarity where they can never be cited.
	if dim <= 0 {
		dim = firstChunkVectorDim(chunksRaw)
	}
	var missing []string
	for _, cm := range chunksRaw {
		if vectorHasSignal(chunkVector(cm)) {
			continue
		}
		if cid, ok := cm["chunk_id"].(string); ok && cid != "" {
			missing = append(missing, cid)
		}
	}
	if len(missing) == 0 || dim <= 0 || len(tenantIDs) == 0 {
		return 0, nil
	}

	// Use RetrievalService which mirrors Python's Dealer.fetch_chunk_vectors.
	retrievalSvc := nlp.NewRetrievalService(docEngine, dao.NewDocumentDAO())
	vectors, err := retrievalSvc.FetchChunkVectors(ctx, missing, tenantIDs, kbIDs, dim)
	if err != nil {
		common.Warn("HydrateChunkVectors: FetchChunkVectors failed", zap.Error(err))
		return 0, err
	}

	// Stitch the vectors back onto the chunks. FetchChunkVectors answers with
	// zeros for chunks it could not resolve, so only a vector with signal counts
	// as a hit.
	hits := 0
	for _, cm := range chunksRaw {
		if vectorHasSignal(chunkVector(cm)) {
			continue
		}
		cid, _ := cm["chunk_id"].(string)
		if cid == "" {
			continue
		}
		vec := vectors[cid]
		if !vectorHasSignal(vec) {
			continue
		}
		cm["vector"] = vec
		hits++
	}
	common.Debug("HydrateChunkVectors complete",
		zap.Int("hits", hits), zap.Int("requested", len(missing)))
	return hits, nil
}

// chunkVector reads a chunk's embedding vector in the shapes the in-process
// layers produce ([]float64; []interface{} after a JSON round-trip).
func chunkVector(cm map[string]interface{}) []float64 {
	switch v := cm["vector"].(type) {
	case []float64:
		return v
	case []interface{}:
		out := make([]float64, 0, len(v))
		for _, item := range v {
			f, ok := item.(float64)
			if !ok {
				return nil
			}
			out = append(out, f)
		}
		return out
	}
	return nil
}

// vectorHasSignal reports whether a vector has any non-zero component, mirroring
// Python's `any(x for x in v)` (dialog_service.py:94). A placeholder of zeros is
// the shape both retrieval paths leave behind for a chunk whose embedding was
// not selected, and it must be refilled rather than treated as hydrated.
func vectorHasSignal(v []float64) bool {
	for _, x := range v {
		if x != 0 {
			return true
		}
	}
	return false
}

// firstChunkVectorDim returns the dimension carried by the first chunk vector,
// which is the dimension naming the q_{dim}_vec field. Zero means no chunk
// carries a vector at all.
func firstChunkVectorDim(chunksRaw []map[string]interface{}) int {
	for _, cm := range chunksRaw {
		if v := chunkVector(cm); len(v) > 0 {
			return len(v)
		}
	}
	return 0
}

// embeddingModelEmbedder adapts an EmbeddingModel to the Embedder interface.
type embeddingModelEmbedder struct {
	embModel *modelModule.EmbeddingModel
}

func (e *embeddingModelEmbedder) Encode(ctx context.Context, texts []string) ([][]float64, error) {
	config := &modelModule.EmbeddingConfig{Dimension: 0}
	// Embed inside the model's window: the caller supplies arbitrary text and the
	// provider rejects an over-window input with 400/20015 instead of truncating it.
	embeds, err := e.embModel.Embed(ctx, modelModule.EmbedRequest{Texts: texts}, config, nil)
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
	ttsModel *modelModule.TTSModel,
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
	if !quote {
		think = stripCitations(think)
		ans = stripCitations(ans)
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
			think = RepairBadCitationFormats(think)
			ans = RepairBadCitationFormats(ans)
			normalizedOutput := normalizeArabicDigits(think + ans)
			// P7 — _hydrate_chunk_vectors. Mirrors
			// dialog_service.py:794. Fetch the chunk embeddings insertCitations
			// scores against, in one batched engine call. The agentic evidence
			// pool reaches here with no `vector` key at all and the naive path
			// ships zero placeholders, so the dimension comes from the embedding
			// model whenever no chunk carries one. Only needed when we'll
			// actually call insertCitations (the LLM didn't emit markers).
			if embModel != nil && !HasCitationMarkers(normalizedOutput) {
				dim := firstChunkVectorDim(chunksRaw)
				if dim <= 0 {
					// One short probe encode buys the q_{dim}_vec field name.
					// Every citation-scored answer already pays for a full
					// sentence encode, so this is not a new call class.
					probe, perr := (&embeddingModelEmbedder{embModel: embModel}).Encode(ctx, []string{"x"})
					if perr == nil && len(probe) > 0 {
						dim = len(probe[0])
					}
				}
				if _, err := HydrateChunkVectors(ctx, kbinfos, tenantIDs, nil, dim, engine.Get()); err != nil {
					common.Warn("hydrate chunk vectors failed", zap.Error(err))
				}
			}
			if embModel != nil && !HasCitationMarkers(ans) {
				// Build chunkVectors aligned with chunksRaw. A chunk that still
				// has no vector scores as zeros — exactly what Python does when
				// it substitutes [0.0]*dim on a dimension mismatch
				// (rag/nlp/search.py:463-468): that chunk simply never wins a
				// citation, instead of one vector-less chunk (an image chunk, a
				// web snippet) suppressing every citation in the answer.
				dim := firstChunkVectorDim(chunksRaw)
				chunkVectors := make([][]float64, len(chunksRaw))
				for i, cm := range chunksRaw {
					cv := chunkVector(cm)
					if len(cv) != dim {
						cv = make([]float64, dim)
					}
					chunkVectors[i] = cv
				}
				if dim > 0 {
					embedder := &embeddingModelEmbedder{embModel: embModel}
					if decorated, cited := InsertCitations(ctx, ans, NewSourcedChunks(chunksRaw), embedder, chunkVectors); len(cited) > 0 {
						ans = decorated
						citationIdx = make(map[int]struct{})
						for _, ci := range cited {
							citationIdx[ci] = struct{}{}
						}
					}
				}
			} else if HasCitationMarkers(normalizedOutput) {
				// P0.11 pre-check matched: collect indices from existing
				// markers in both the thinking block and final answer.
				citationIdx = make(map[int]struct{})
				for _, ci := range ExtractCitationMarkers(normalizedOutput, len(chunksRaw)) {
					citationIdx[ci] = struct{}{}
				}
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
				if docAggsRaw, ok := kbinfos["doc_aggs"].([]interface{}); ok {
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
					kbinfos["doc_aggs"] = filtered
				}
			}
		}
	}

	// Reference assembly. `chunks` is the evidence the answer was grounded on,
	// so it ships whenever knowledge was retrieved and independent of the cite
	// setting; `doc_aggs` is the list of source cards the UI opens from a
	// citation marker, so it ships only while citations are enabled — with no
	// marker to open them from, the cards would be dead weight. Chunk vectors
	// never leave this function.
	if hasKnowledges {
		refs = make(map[string]interface{}, len(kbinfos))
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
		if !quote {
			delete(refs, "doc_aggs")
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
	// P1: emit Timer.Markdown() (phase lines, Other, and Total) and then the
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
	audioBinary := s.synthesizeTTS(ctx, ttsModel, think+ans)

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
		// Fix 7: Apply the Markdown line-break substitution
		// re.sub(r"\n", "  \n", prompt) at the very end, matching
		// dialog_service.py:865. This converts single \n to "  \n"
		// so multi-line prompt text renders as a single Markdown
		// paragraph instead of being broken into separate lines.
		Prompt:    strings.ReplaceAll(timeStats, "\n", "  \n"),
		CreatedAt: float64(finishChatTs.Unix()),
		Final:     false, // caller sets Final = true
		// Same numbers the Langfuse generation above records, on the wire so a
		// client does not have to parse the human-readable prompt to learn them.
		Usage: &TurnUsage{
			PromptTokens:     usedTokenCount,
			CompletionTokens: tkNum,
			TotalTokens:      usedTokenCount + tkNum,
		},
		ElapsedMs: totalMs,
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
// chatConfigWithTemperature copies chatCfg and pins the given temperature;
// a nil temp leaves the temperature field unset (the model's own default
// applies) and strips whatever the producer carried.
func chatConfigWithTemperature(chatCfg *modelModule.ChatConfig, temp *float64) *modelModule.ChatConfig {
	if temp == nil {
		if chatCfg == nil {
			return &modelModule.ChatConfig{}
		}
		out := *chatCfg
		out.Temperature = nil
		return &out
	}
	if chatCfg == nil {
		return &modelModule.ChatConfig{Temperature: temp}
	}
	out := *chatCfg
	out.Temperature = temp
	return &out
}

func BuildChatConfig(dialog *entity.Chat, config map[string]interface{}) *modelModule.ChatConfig {
	cfg := &modelModule.ChatConfig{Thinking: new(false)}

	if dialog.LLMSetting != nil {
		if v, ok := dialog.LLMSetting["stream"].(bool); ok {
			cfg.Stream = &v
		}
		if v, ok := dialog.LLMSetting["thinking"].(bool); ok {
			cfg.Thinking = &v
		} else if v, ok := dialog.LLMSetting["thinking"].(string); ok {
			cfg.Thinking = new(v == "enabled")
		}
		if v, ok := chatConfigPositiveInt(dialog.LLMSetting["max_tokens"]); ok {
			i := v
			cfg.MaxTokens = &i
		}
		if v, ok := chatConfigFloat(dialog.LLMSetting["temperature"]); ok {
			cfg.Temperature = &v
		}
		if v, ok := chatConfigFloat(dialog.LLMSetting["top_p"]); ok {
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
		} else if v, ok := config["thinking"].(string); ok {
			cfg.Thinking = new(v == "enabled")
		}
		if v, ok := chatConfigPositiveInt(config["max_tokens"]); ok {
			i := v
			cfg.MaxTokens = &i
		}
		if v, ok := chatConfigFloat(config["temperature"]); ok {
			cfg.Temperature = &v
		}
		if v, ok := chatConfigFloat(config["top_p"]); ok {
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

// defaultCompletionReserve is the completion room kept when neither the chat
// nor the model catalog sets an output limit. It matches the web UI model presets.
const defaultCompletionReserve = 4096

// completionReserve is the room kept for the answer in a window of
// modelMaxTokens: the chat's max_tokens when set, honoured exactly; otherwise
// the model's catalog output limit, else defaultCompletionReserve, both capped
// at a quarter of the window so a large catalog limit cannot take it all.
func completionReserve(cfg *modelModule.ChatConfig, modelMaxOutput, modelMaxTokens int) int {
	if cfg != nil && cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
		return *cfg.MaxTokens
	}
	reserve := defaultCompletionReserve
	if modelMaxOutput > 0 {
		reserve = modelMaxOutput
	}
	return min(reserve, modelMaxTokens/4)
}

// knowledgeTokenBudget is the room the retrieved passages may take in a window
// of modelMaxTokens: what the system prompt, the citation rules sent after it
// (when quoting) and the completion reserve leave. The system prompt is measured
// as formatPrompt renders it from kwargs, with the knowledge left out, so a
// prompt parameter carrying a large value is counted. formatPrompt writes the
// passages at every {knowledge} of the template, and AsyncChat appends them once
// to a template without one, so the room is split across those insertions.
func (s *ChatPipelineService) knowledgeTokenBudget(modelMaxTokens int, systemTemplate string, kwargs map[string]interface{}, quote bool, reserve int) int {
	params := maps.Clone(kwargs)
	delete(params, "knowledge")
	used := graph.NumTokensFromString(s.formatPrompt(systemTemplate, params)) + reserve
	if quote {
		used += graph.NumTokensFromString(ragprompts.CitationPrompt(""))
	}
	insertions := max(strings.Count(systemTemplate, "{knowledge}"), 1)
	return max(modelMaxTokens-used, 0) / insertions
}

func clampChatConfigMaxTokens(cfg *modelModule.ChatConfig, modelMaxTokens, usedTokenCount int) (int, bool, error) {
	if usedTokenCount >= modelMaxTokens {
		return 0, false, fmt.Errorf("prompt uses %d tokens, leaving no completion capacity for model max context length %d", usedTokenCount, modelMaxTokens)
	}
	if cfg == nil || cfg.MaxTokens == nil {
		return 0, false, nil
	}
	adjusted := *cfg.MaxTokens
	if adjusted <= 0 {
		cfg.MaxTokens = nil
		return 0, false, nil
	}
	remainingTokens := modelMaxTokens - usedTokenCount
	if adjusted > remainingTokens {
		adjusted = remainingTokens
	}
	cfg.MaxTokens = &adjusted
	return adjusted, true, nil
}

func chatConfigPositiveInt(value interface{}) (int, bool) {
	v, ok := chatConfigInt(value)
	if !ok || v <= 0 {
		return 0, false
	}
	return v, true
}

func chatConfigInt(value interface{}) (int, bool) {
	return common.ToIntNum(value)
}

func chatConfigFloat(value interface{}) (float64, bool) {
	// Keep the pre-convergence contract: only native numeric kinds are valid
	// overrides. common.ToFloat also parses strings (including "NaN"/"Inf"),
	// which would put non-finite values into the LLM request body.
	switch value.(type) {
	case float64, float32, int, int64:
		f, ok := common.ToFloat(value)
		if ok && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f, true
		}
	}
	return 0, false
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

// getChunkValue returns chunk[k1] when the key is present, otherwise chunk[k2].
// Mirrors Python's get_value helper in rag/prompts/generator.py:37-38
// (`d.get(k1, d.get(k2))`), which falls back on KEY ABSENCE only: a key present
// with a nil/empty value is returned as-is rather than skipping to k2.
func getChunkValue(chunk map[string]interface{}, k1, k2 string) interface{} {
	if v, ok := chunk[k1]; ok {
		return v
	}
	return chunk[k2]
}

// toAnySlice widens a []map[string]any to []interface{} so the doc_aggs field
// keeps the element type the downstream phases expect from the native pipeline.
func toAnySlice(in []map[string]any) []interface{} {
	out := make([]interface{}, 0, len(in))
	for _, m := range in {
		out = append(out, m)
	}
	return out
}

// asInt64 coerces a JSON-decoded value (float64/json.Number/int/string/bool) to
// an int64, returning ok=false when the value is absent or not numeric.
func asInt64(v interface{}) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int:
		return int64(t), true
	case int64:
		return t, true
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n, true
		}
	case string:
		var n int64
		if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
			return n, true
		}
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// quoteEnabled resolves the effective citation-visibility setting: either the
// chat setting or the request can disable citations, and both must agree. An
// absent setting on either side leaves citations on.
//
// It is a function because two dispatch points need it — the agentic branch
// and Phase 6 — and the agentic branch returns before Phase 6 runs. Resolving
// it twice is how the agentic path ended up ignoring a request's quote:false.
func quoteEnabled(kwargs map[string]interface{}, promptConfig map[string]interface{}) bool {
	quote := true
	if v, ok := kwargs["quote"].(bool); ok {
		quote = v
	}
	if promptConfigQuote, ok := promptConfig["quote"].(bool); ok {
		quote = quote && promptConfigQuote
	}
	return quote
}

// reasoningLevelAgentic is the reasoning level that hands the turn to the
// smart-reasoning agent (internal/agentic_rag) instead of running the pipeline's
// own retrieval phases.
//
// It is the only level that means anything beyond "retrieve normally": every
// other value — including a stale 1..4 from a client still sending one of the
// removed thinking modes — runs the standard retrieval path below.
const reasoningLevelAgentic = 5

// reasoningLevelCodex is the reasoning level that hands the turn to the Codex
// engine: this process brokers a shared Codex app-server (dialed over WebSocket)
// and exposes the agent_rag retrieval tools to it through an MCP bridge.
//
// Sibling of the other engine levels for the same reason: it names an ENGINE, not a
// harness depth, so it must never reach harnessModeForLevel. Unlike level 5 it
// does NOT require knowledge bases — Codex is a general assistant and can still converse
// without one; the MCP retrieval tools return an error when the conversation binds
// no KB (see the mode 8 plan, D8).
const reasoningLevelCodex = 8

// agenticEngine names the engine a turn runs on. The zero value means the regular
// chat pipeline.
type agenticEngine string

const (
	// engineAgenticRAG is mode 5: the plain eino ReAct explorer (direct answer).
	engineAgenticRAG agenticEngine = "agentic"
	// engineCodex is mode 8: the Codex engine (shared app-server over WebSocket,
	// agent_rag tools via MCP).
	engineCodex agenticEngine = "codex"
)

// selectAgenticEngine decides which engine answers a turn, from the reasoning
// level and the dialog's knowledge-base scope.
//
// The LEVEL is the only selector. An `agent_mode` request parameter used to name
// a template; with it gone, the level names the engine and nothing else does.
// Which mode-5 template runs is not a request concern at all: it is
// defaultAgenticTemplateID, resolved inside agenticRag.
//
// It is a named function rather than an inline condition so the dispatch and its
// test cannot drift apart: the mapping below IS the contract, and a test that
// re-derived it would pass while the production branch said something else.
//
// No knowledge bases means NO mode-5 engine: it resolves citations against an
// explicit kb_id scope, and an empty scope drops that term from the ES query
// (buildBoolQueryFromCondition), letting a cited chunk resolve out of any KB in
// the tenant. Falling through to the regular pipeline keeps the empty-scope
// query unconstructible instead of trusting every citation path to defend
// itself.
//
// mode 8 is deliberately exempt from that gate: Codex is a general assistant
// that can converse without a KB, and its MCP retrieval tools return an error on
// a no-scope call (never an empty/default scope query). So level 8 is handled
// BEFORE the !hasKBs return.
func selectAgenticEngine(level int, hasKBs bool) agenticEngine {
	if level == reasoningLevelCodex {
		return engineCodex
	}
	if !hasKBs {
		return ""
	}
	if level == reasoningLevelAgentic {
		return engineAgenticRAG
	}
	// 0 (off) and the harness depths 1..4 stay on the pipeline.
	return ""
}

// resolveReasoningLevel mirrors Python's rag_agent: the requesting reasoning
// level is taken from the request kwargs first, falling back to the chat
// prompt_config, and is an integer 0..4 (0 = off, 1..4 = low/medium/high/
// ultra), reasoningLevelAgentic for the smart-reasoning agent, or
// reasoningLevelCodex for the Codex engine. Frontend
// sends Number(getThinkingLevel()), so the raw value is numeric, not a bool.
func resolveReasoningLevel(kwargs map[string]interface{}, promptConfig map[string]interface{}) int {
	if kwargs != nil {
		if v, ok := kwargs["reasoning"]; ok {
			if n, ok2 := asInt64(v); ok2 {
				return int(n)
			}
		}
	}
	if promptConfig != nil {
		if v, ok := promptConfig["reasoning"]; ok {
			if n, ok2 := asInt64(v); ok2 {
				return int(n)
			}
		}
	}
	return 0
}
