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

// Package agentic_rag is the outer agentic-search loop (medium / high / ultra).
//
// This file holds the run's configuration and entry-point logic: the run
// configuration (RAGTools), the rag entry point (Rag) and question formalization
// (Formalize). The shared capability carrier that the agentic graph and the runtime
// action session both read from and write to is runtime.Toolset (see
// runtime/action_session.go); this file holds the RAGTools-side logic.
//
// The package is laid out in three layers:
//   - agentic_rag.go       the run configuration and entry points
//   - agentic_rag_graph.go the pipeline (nodes, routing, assembly)
//   - runtime/             the leaf primitives
package agentic_rag

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/rag/agentic-rag/runtime"
	"ragflow/internal/rag/prompts"
	"ragflow/internal/service/nlp"
)

// Capability map — where each part of the run lives.
//
// The RAGTools methods live in this package (this file and
// agentic_rag_graph.go), while runtime/ stays the leaf capability library they
// orchestrate (retrieval, sessions, tools). Nothing below is a forwarding shim —
// the right-hand side IS the implementation:
//
//	run configuration   → RAGTools                 (below)
//	tool surface        → runtime.Toolset          (runtime/action_session.go)
//	message fitting     → chat.FitMessages         (agent/chat/message_fit.go)
//	citation guidelines → GetCitationGuidelines    (below)
//	system prompt       → SysPrompt                (below)
//	formalization       → Formalize                (below)
//	retrieval           → HybridSearch             (runtime/tool_search.go)
//	keyword extraction  → ExtractWeightedKeywords  (runtime/keywords.go)
//	keyword compaction  → CompactKeywords          (runtime/tool_text_processing.go)
//	web retrieval       → searchExecutor.webSearch (runtime/tool_executor.go)
//	answer composition  → ComposeAnswer            (agentic_rag_graph.go)
//	naive answer        → ComposeNaiveAnswer       (agentic_rag_graph.go)
//	evidence fitting    → FitEvidence              (below)
//	sufficiency review  → SCA review               (runtime/orchestrator/sufficient_context.go)
//	gap rewrite         → SCA gap rewrite          (runtime/orchestrator/sufficient_context.go)
//	entry point         → Rag                      (below)
//
//	document scope      → toolDocScope          (runtime/tool_executor.go)
//	doc-id verification → DocIDLookup           (below)
//	tenant resolution   → docInDatasets         (runtime/tool_search.go)
//	full document fetch → fetchFullDocument     (runtime/doc_fetch.go)
//	document summary    → summarizeDocument     (runtime/doc_fetch.go)
//
// Usage is counted through an explicit *runtime.LLMUsageStats collector (RAGTools.Stats)
// passed to the calls the run makes, rather than through a ContextVar and a wrapper chat
// model; per-round metrics derived from the round list are not tracked.
//
// Deliberately not implemented — dead code upstream: the document-picking helpers return
// nil on their first statement and are never called, so the title/metadata selectors are
// unreachable too, and structured_retrieve has neither a @tool decorator nor a caller.
//
// RAGTools configures one agentic-search run: every capability the run needs (retriever,
// model, prompts, embedder, kb accumulation, retrieval tuning, answer composition) is
// carried on this one object — there is no second dependency struct. The runtime layer
// keeps narrow per-call views (runtime.SearchDeps / runtime.SessionDeps) projected from a
// RAGTools at call time. Construct it directly
// (RAGTools{...}); its zero value is a valid starting point for the narrower call sites
// that only set a subset of fields.
type RAGTools struct {
	// Tools is the shared capability carrier. The graph and the action session both
	// reach retrieval through it.
	Tools *runtime.Toolset
	// Search is the fully-projected retrieval configuration for this run. The
	// graph's DUAL-CHANNEL fan-out needs it directly: unlike the action session
	// (which goes through Tools.Exec's single "search_chunks" tool), fan-out
	// issues a keyword BM25 leg and a dense semantic leg under different
	// settings, which the tool interface cannot express.
	Search runtime.SearchDeps
	// Retriever is the retrieval backend. When nil, the runtime singleton
	// (runtime.GetRetrievalService) is used.
	Retriever runtime.Retriever
	// Model drives the LLM turns. Required for agentic modes; low/naive never
	// call it.
	Model runtime.SessionModel
	// ModelName is the RESOLVED chat model identity (e.g. "gpt-4o"), one component of the
	// gen_json reply-cache key. Empty disables that cache: keying on an empty name would
	// collapse every model onto one bucket and serve a reply produced by another model.
	ModelName string
	// Embedder is the external embedding handle used by graph/structure seed encoding.
	// Nil falls back to this package's internal tenant-default resolver, which additionally
	// degrades to keyword matching when the DB is uninitialised or encoding panics.
	Embedder nlp.NavEmbedder
	// Keywords extracts the entity-weighted retrieval query. Optional.
	Keywords KeywordExtractorFn
	// Prompts are the report/SCA/rewrite prompt templates (user_defined_prompts).
	Prompts runtime.PromptLoader
	// Expand runs compiled-structure expansion. Optional.
	Expand runtime.CompiledExpander
	// KB is the in-flight retrieval accumulation.
	KB *runtime.Kbinfos
	// Logger is optional; nil uses the default logger.
	Logger *log.Logger
	// Answer-composition configuration (see AnswerDeps). All optional; the defaults are
	// the configured behaviour.
	// CiteRules overrides the default citation rules.
	CiteRules string
	// SystemPrompt is the dialog-level UI configuration appended after the
	// agentic contract (language/tone/style may override; evidence contract may not).
	SystemPrompt string
	// EmptyResponse is returned verbatim when no evidence was found, skipping the
	// composition call entirely.
	EmptyResponse string
	// EvidenceMaxTokens caps the evidence block. <=0 uses evidenceBudgetTokens.
	EvidenceMaxTokens int
	// MaxLength is the chat model's context window. It bounds message fitting for prompt
	// assembly; <=0 falls back to chat.EffectiveContextLength's 8192 default.
	MaxLength int
	// ComposeAnswer enables the terminal composition node. Defaults to true
	// when a model is configured; set false to receive raw evidence only.
	ComposeAnswer *bool
	// Finalize runs the terminal composition. It is wired by Rag so the graph can invoke
	// it FROM INSIDE its last node — the agentic and low graphs both end in a
	// formalize_answer node that composes and streams the answer. The graph forwards the
	// node's state values the compose prompt reads: partialAnswer and emptyResult
	// (orchestrator/direct.go) plus question —
	// the graph state's FORMALIZED question (
	// `question = state.get("question")`), which the formalize_question node
	// wrote. The compose prompt must be built from the formalized question,
	// not the outer tool argument: the graph researched the formalized
	// multi-hop question, so composing from the compressed outer argument
	// collapses the final answer to the first completed sub-answer. Empty
	// question means "the caller composes afterwards" (post-graph fallback) —
	// composeFinalAnswer then falls back to req.Question. Nil Finalize means
	// the caller composes afterwards, which is only correct for the naive path.
	Finalize func(ctx context.Context, partialAnswer, emptyResult bool, question string)
	// Messages is the conversation history used by Formalize to resolve
	// pronouns/ellipses into a standalone question. The agentic and low graphs
	// formalize it as their first node; naive retrieval does not formalize.
	Messages []schema.Message
	// WebSearch is the optional open-web provider backing the `web_search` tool.
	// Nil HIDES the tool from the mode's surface —
	// sessionDeps sets HasWebSearch = mode.HasTool("web_search") && WebSearch != nil
	// and ActiveToolSpecs drops the spec — so the model is never shown a dead tool.
	// Should a call reach the executor anyway (stale spec, direct invocation), it
	// answers StatusError/ReasonInfra with a do-not-retry note, NOT a query-level
	// MISS: WebSearchTool returns the same infra error for an absent provider.
	WebSearch runtime.WebSearcher
	// DocIDVerifier checks which requested document ids really belong to the datasets
	// being searched. Nil disables the check and leaves the requested scope untouched.
	DocIDVerifier runtime.DocIDVerifier
	// DocChunks pages through a single document's chunks in reading order
	// (sort_by_position). It backs the whole-document-reading path
	// (summarize_document / fetch_full_document)
	// Nil disables it: the document-level tools report that reading is unavailable.
	DocChunks runtime.DocChunkLister
	// MetadataResolver resolves document sets from document metadata. It backs the
	// metadata_search tool and the pre-search metadata channel
	// (implemented by internal/service.MetadataService). Nil leaves both unavailable.
	MetadataResolver runtime.MetadataResolver
	// DeclaredMetadata reads the metadata fields the datasets DECLARE in their
	// parser_config, which is what lets the metadata_search catalog describe a field
	// (its meaning and allowed values) instead of only naming it. Same implementation as
	// MetadataResolver; nil leaves the catalog with the metadata index alone.
	DeclaredMetadata runtime.DeclaredMetadataResolver
	// Outer is the outer-layer chat model that drives the rag_agent react loop
	// (dialog_service.rag_agent): it binds tools=[rag, summarize_document] with
	// terminal_tools={"rag"}. When non-nil, Rag runs that outer loop: the model may call
	// `rag` (terminal — runs the inner agentic graph and returns the cited answer) or
	// `summarize_document` (non-terminal — reads a whole document into evidence and lets
	// the model answer from it on the next round); with no tool call it answers directly.
	// When nil, Rag falls back to the direct RunAgenticRAG path, so callers that don't wire
	// an outer model see unchanged behaviour.
	Outer *models.ChatModel
	// Cache answers near-identical re-asks from an earlier answer. Rag builds a fresh
	// per-turn RAGCache when this is nil, so caching is ON by default. To share one cache
	// across multiple Rag() calls within a single turn (the concurrent tool_call case),
	// pass the same *RAGCache here instead of relying on the auto-built one.
	Cache *RAGCache
	// OriginalQuestion is the user's own, unrewritten question. When the outer caller
	// passes a compressed rewrite as Question, the original is preferred if both clearly
	// describe the same turn.
	OriginalQuestion string
	// TextAttachments is appended to the question and bypasses the re-ask cache.
	TextAttachments string
	// AnswerSink receives the answer as it is produced. Nil disables streaming; the answer
	// is still returned in full. Requires a model implementing
	// runtime.StreamingSessionModel, otherwise the run falls back to one-shot composition.
	AnswerSink *AnswerSink
	// ToolStarted is called once research begins, before any retrieval, so the caller can
	// show progress.
	ToolStarted func()
	// Steps reports the run's reasoning steps. Each stage declares its own step
	// where it happens and each tool reports both halves of its call; Text feeds
	// the think block and Events feeds structured clients, either may be nil.
	//
	// This replaces the per-request progress sink this branch used (a logger
	// wrapper that fished bracket-tagged lines out of the log stream and forwarded
	// them into the <think> block). Visibility is now the producer's decision: the
	// developer log keeps the diagnostics, and a step is a step because a stage
	// SAYS so — not because a tag matched a pattern — so rewording a log line
	// can never silently hide a step.
	Steps runtime.StepReporter
	// Retrieval tuning: an explicit tool argument still wins over these, and zero selects
	// the runtime defaults.
	TopN                int
	SimilarityThreshold float64
	// KeywordsSimilarityWeight is the keyword leg's weight. A pointer keeps an
	// explicit zero (pure vector search) distinct from an unset value.
	KeywordsSimilarityWeight *float64
	// UsingEmbedding is the Go spelling of Python RAGTools.retrieve's
	// `using_embedding: bool = False` (agentic_rag.py:599). When false (the
	// agentic default) retrieval is keyword-only; when true the embedder is
	// engaged and KeywordsSimilarityWeight (default 0.3) applies.
	//
	// SCOPE: only the retrieve channel honours it.
	UsingEmbedding bool
	// HasEmbedder: whether an embedder is configured. It gates the hybrid leg's vector
	// weight, i.e. the semantic recall of the search_chunks tool.
	HasEmbedder           bool
	RerankCandidatesCount int
	TopK                  int
	// DocScope is the session-wide document restriction. Nil means "search everything".
	DocScope []string
	// MetaDataFilter restricts retrieval by chunk metadata.
	MetaDataFilter map[string]any
	// KBs is the full set of Knowledgebase objects (carrying parser_config / tenant_id)
	// the agentic tools run over. They arrive already resolved from the caller (this
	// package stays DB-free) and are fed to the Tagger for the question-type tag boost.
	KBs []*entity.Knowledgebase
	// Tagger classifies the query into question-type tags the retriever uses to boost
	// matching chunks (implemented by internal/service.MetadataService.LabelQuestion).
	// Nil means no tag boost.
	Tagger runtime.QuestionLabeler
	// Stats receives per-phase LLM usage for this run (runtime.LLMUsageStats). Nil
	// disables collection.
	Stats *runtime.LLMUsageStats
}

// sessionDeps projects RAGTools onto the runtime.SessionDeps the action session
// and slot-table builders consume.
//
// Every field is forwarded unconditionally. In particular a nil Prompts is NOT
// a reason to drop Tools/KB: the runtime resolves a nil loader to its embedded
// canonical templates (retrieval.resolveLoader / prompts.EmbeddedPromptLoader), so
// the action session still runs with its toolset and the shared evidence pool.
// Returning a Model-only projection here silently turned the slot research pass
// into a tool-less question-answering turn.
func (d RAGTools) sessionDeps() runtime.SessionDeps {
	return runtime.SessionDeps{
		Tools:   d.Tools,
		Model:   d.Model,
		Prompts: d.Prompts,
		KB:      d.KB,
	}
}

func (d RAGTools) logger() *log.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return _LOG
}

// Question formalization (the graph's first node).
//
// Rewrite the latest user message into a standalone question AND derive its search
// keywords, in one LLM call.
//
// Single-turn shortcut: when there is nothing to resolve, the question is kept
// VERBATIM (no rewrite) and only keywords are extracted. Rewriting a
// self-contained question risks silently changing its meaning — and the
// single-turn case is the overwhelming majority.
//
// The keyword/JSON extraction helpers (ExtractWeightedKeywords, ExtractJSON)
// stay in runtime as the leaf capability library.

const (
	// formalizeTimeoutS bounds the formalize call.
	formalizeTimeoutS = 45.0
	// formalizeTemperature: formalization is a mechanical rewrite, so it must be stable.
	formalizeTemperature = 0.1
)

var reFormalizeThink = regexp.MustCompile(`(?s)^.*</think>`)

var formalizePrompt = (`You are given a conversation. Do BOTH of the following and return JSON only:
1. Rewrite the LAST user message into a single, self-contained question that can be understood without the prior conversation — resolve pronouns, ellipses and follow-up shortcuts using the earlier turns. In most cases, it should be EXACTLY THE SAME as the last user query — only rewrite when there is something to resolve (a pronoun/ellipsis pointing back at an earlier turn). Preserve the original language.
2. Extract keywords for a keyword search: the salient content words and phrases that literally appear in the (standalone) question — key nouns, named entities, domain terms — PLUS 2-3 close synonyms/abbreviations/aliases/alternative spellings of each, in the SAME language as the question. Maximize recall. Do NOT include terms that would be part of the answer.
   Example — "In which year did Apple acquire Beats?" -> keywords = "Apple, Apple Inc., AAPL, acquire, acquisition, acquired, Beats, Beats Electronics".

Output ONLY JSON, no prose, no code fences: {"question": "<standalone question>", "keywords": "<term1, term2, synonym1, ...>"}`)

// Formalize: return (question, keywords) for
// the given conversation.
//
// messages may be []schema.Message (preferred) or pre-formatted "Speaker: text"
// strings. On any failure it degrades to (last user message, "") — formalization
// is an optimization, never a precondition for answering.
//
// maxLength is the chat model's context window; it bounds the prompt fit. <=0 falls back
// to chat.EffectiveContextLength's 8192.
func Formalize(ctx context.Context, deps runtime.SessionDeps, messages []schema.Message, maxLength int) (string, string) {
	lastUser, transcript := transcriptOf(messages)
	if lastUser == "" {
		return "", ""
	}
	if deps.Model == nil {
		// No model: keep the raw question and skip keyword extraction rather
		// than failing the request.
		return lastUser, ""
	}

	// Single-turn: nothing to resolve — keep the question VERBATIM (rewriting a
	// self-contained question risks silently changing its meaning), and extract only the
	// search keywords.
	if !isMultiTurn(messages) {
		_LOG.Printf("[Formalize] Single-turn self-contained question — kept verbatim (no rewrite): %s", trunc(lastUser, 120))
		_, kw := runtime.ExtractWeightedKeywords(ctx, deps.Model, lastUser)
		return lastUser, kw
	}

	ctx, done := runtime.Phase(ctx, runtime.PhaseFormalize)
	// Phase returns a child context carrying the phase label; the timeout is
	// derived from it so the call is both attributed and bounded.
	defer done()

	callCtx, cancel := context.WithTimeout(ctx, deadlineToDuration(formalizeTimeoutS))
	defer cancel()

	// Fit the prompt to the model's context window.
	fitted, fitErr := chat.FitMessages(formalizePrompt, []schema.Message{
		*schema.UserMessage("Conversation:\n" + transcript + "\n\nOutput JSON:"),
	}, maxLength)
	if fitErr != "" {
		_LOG.Printf("[Formalize] prompt fitting failed: %s", fitErr)
		return lastUser, ""
	}
	system := formalizePrompt
	history := fitted
	if len(fitted) > 0 && fitted[0].Role == schema.System {
		system = fitted[0].Content
		history = fitted[1:]
	}
	msgs := make([]schema.Message, 0, 1+len(history))
	msgs = append(msgs, *schema.SystemMessage(system))
	msgs = append(msgs, history...)

	// async_chat(system, history, {"temperature": 0.1}).
	reply, err := modelWithTemperature(deps.Model, formalizeTemperature).Complete(callCtx, msgs, nil)
	if err != nil {
		_LOG.Printf("[Formalize] failed; keeping the raw question: %v", err)
		return lastUser, ""
	}

	data, _ := runtime.ExtractJSON(stripThinkAndFences(reply.Content)).(map[string]any)
	question := ""
	if data != nil {
		if raw, ok := data["question"]; ok && raw != nil {
			question = strings.Trim(strings.TrimSpace(fmt.Sprint(raw)), "\"'")
		}
	}
	if question == "" {
		// Fall back to the raw last user message rather than an empty question.
		question = lastUser
	}
	keywords := ""
	if data != nil {
		switch v := data["keywords"].(type) {
		case string:
			keywords = v
		case []any:
			parts := make([]string, 0, len(v))
			for _, k := range v {
				if s := strings.TrimSpace(fmt.Sprint(k)); s != "" {
					parts = append(parts, s)
				}
			}
			keywords = strings.Join(parts, ", ")
		}
	}
	return question, runtime.CompactKeywords(keywords)
}

// transcriptOf renders the conversation and extracts the last user message.
func transcriptOf(messages []schema.Message) (lastUser, transcript string) {
	var lines []string
	for _, m := range messages {
		content := m.Content
		switch m.Role {
		case schema.User:
			lastUser = strings.TrimSpace(content)
			lines = append(lines, "User: "+content)
		case schema.Assistant:
			lines = append(lines, "Assistant: "+content)
		case schema.System:
			lines = append(lines, "System: "+content)
		default:
			lines = append(lines, strings.TrimSpace(content))
		}
	}
	return strings.TrimSpace(lastUser), strings.Join(lines, "\n")
}

// isMultiTurn reports whether the conversation has more than one user turn.
func isMultiTurn(messages []schema.Message) bool {
	n := 0
	for _, m := range messages {
		if m.Role == schema.User {
			n++
		}
	}
	return n > 1
}

// stripThinkAndFences: drop a leading thinking preamble, then strip Markdown fences. The
// fence regex removes the delimiters wherever they appear, so a truncated or inline fence
// leaves no residue either.
func stripThinkAndFences(s string) string {
	s = reFormalizeThink.ReplaceAllString(s, "")
	s = reFenceDelimiters.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// reFenceDelimiters removes ```json / ``` delimiters wherever they appear.
var reFenceDelimiters = regexp.MustCompile("```(?:json)?\\s*|\\s*```")

// End-to-end entry point: one call that runs the runtime and publishes the
// evidence it collected.
//
// Rag: the RAGTools method
// that drives the research pipeline. It owns the conversation-level concerns
// (re-ask cache, the effective question, attachments, composing the final
// answer) and delegates the mode dispatch to RunAgenticRAG, which mirrors
// run_agentic_rag and carries its dispatch:
//
//	mode is NAIVE      → _naive_rag            (plain retrieval)
//	mode.Agentic       → build_agentic_graph   (medium / high / ultra)
//	else               → build_low_graph       (low: formalize → direct_search)
//
// SCOPE NOTE — read before relying on this for agentic modes: the outer
// orchestration that agentic_rag_graph.py owns — planner decomposition, prefetch
// fan-out, and the SCA↔rewriter iteration loop that drives medium/high/ultra — is
// the agentic_rag package's agentic planner (Rag's agentic-loop registration).
// So:
//
//   - low / naive: fully wired here (direct search through HybridSearch).
//   - medium/high/ultra: Rag delegates to the registered AgenticLoop, which the
//     agentic_rag package registers (see SetAgenticLoop). Without it, Rag falls
//     back to a single action session.
//
// The runtime package keeps the low-level retrieval / session / tool machinery
// (RunActionSession, HybridSearch, Toolset, Kbinfos, searchExecutor, ...).

// RunRequest lives in the runtime package (see runtime/tool_executor.go) because the
// runtime engine's searchExecutor needs it to carry the per-run query, and the
// agentic_rag package must not be imported from runtime (import-cycle rule).
// RunResponse is the agent-side result type returned by RAGTools.rag.

// RunResponse is the outcome of one runtime run.
type RunResponse struct {
	// Answer is the session's terminal answer, when it produced one.
	Answer string
	// Slots are the filled slot-table values discovered by the session.
	Slots []runtime.Variable
	// Chunks is the evidence accumulated in Kbinfos (narrowed, as the model saw
	// it). Citations are built from this.
	Chunks []map[string]any
	// DocAggs is the per-document aggregation of Chunks.
	DocAggs []map[string]any
	// EmptyResult is true when nothing was retrieved, which the caller turns into an
	// "I don't have enough information" answer.
	EmptyResult bool
	// Mode is the resolved spec, for logging.
	Mode runtime.ModeSpec
	// Partial is true when research ended without a satisfying verdict, so the
	// caller surfaces the residual findings honestly instead of refusing.
	Partial bool
	// SearchRounds is the number of completed SCA→rewrite iterations (0 for the
	// non-agentic paths).
	SearchRounds int
	// GraphFailed is true when the research graph itself errored. It is paired with
	// "produced nothing" before falling back to an internal-error message; an empty result
	// on its own is not a failure.
	GraphFailed bool
	// SCAFeedback is the body of the "[Research status]" note: the round's OWN record when it
	// could not answer (see researchStatusNote) — how many passages it read, and what the plan
	// still lists as unresolved. It used to be the reviewer's verdict text; with no reviewer it
	// is a report of what happened rather than a judgement. Rag() appends the trailing "STOP" vs
	// "call rag again" sentence based on the consecutive-unanswerable count.
	SCAFeedback string
	// ResearchStatus is that note, ready to print: SCAFeedback plus the trailing "STOP" vs "call
	// rag again" sentence. It is MODEL-FACING — it belongs in the `rag` tool's RESULT, so an outer
	// loop can decide whether to re-ask — and it is NOT part of the answer.
	//
	// It used to be concatenated onto Answer, which meant a run whose tool result IS the final
	// answer handed the note to the user verbatim, and the answer cache stored it too (measured
	// 2026-09-20: an answer ending with "[Research status] this round did not settle the question
	// (66 passages read). If these gaps are material, call rag again with a question focused on
	// them." — a self-contradicting answer, judged as such). Nothing the user sees reads this
	// field; only the tool-result builder does (see outerReactSession.ToolCall).
	ResearchStatus string
	// CollectedAnswer is the research draft (SCA-reviewed) produced by the
	// agentic loop. It feeds the final composition; prefer Answer for display.
	CollectedAnswer string
	// Kbinfos carries the full accumulated state (including the lossless
	// memory store) for callers that need more than the summary above.
	Kbinfos *runtime.Kbinfos
	// SlotCitations maps a slot-table id ("0", "1", ...) to the evidence
	// chunk ids that filled the slot. The chat pipeline's citation decoration
	// uses it to rewrite leaked "[ID:Slot N]" markers into citations of the
	// chunk the slot was filled from (those markers index the internal slot
	// table — nothing the user can open).
	SlotCitations map[string][]string
	// CiteChunkIDs is the ordered id list of the chunks the final-answer call
	// showed the model as numbered evidence (runtime.Kbinfos.CiteChunkIDs).
	// A "[ID:n]" marker the model wrote refers to position n in this list.
	CiteChunkIDs []string
}

// AnswerSink forwards a partially produced answer while the model is still writing it.
// Nil disables streaming; the answer is still returned in full.
//
// Requires a model implementing runtime.StreamingSessionModel; otherwise the run
// falls back to a single completion.
type AnswerSink struct {
	// OnDelta receives each successive piece of the answer. isThink marks pieces of a
	// hidden reasoning block, which must not be shown as part of the answer.
	OnDelta func(delta string, isThink bool)
	// OnReset drops what has been forwarded so far. It is called before a
	// fallback re-sends the answer from scratch, so a partially streamed answer
	// is never shown twice. Nil means the sink cannot rewind, and the caller
	// only sends the complete answer.
	OnReset func()
}

// deliver forwards one piece, if the sink has a handler.
func (s *AnswerSink) deliver(delta string, isThink bool) {
	if s == nil || s.OnDelta == nil || delta == "" {
		return
	}
	s.OnDelta(delta, isThink)
}

// reset discards previously forwarded pieces, if the sink supports it.
func (s *AnswerSink) reset() {
	if s == nil || s.OnReset == nil {
		return
	}
	s.OnReset()
}

// KeywordExtractorFn extracts the weighted retrieval query for a question.
type KeywordExtractorFn func(ctx context.Context, question string) (retrievalQuery string, err error)

// Run executes one runtime request end to end and publishes the evidence into
// the canvas state (when the context carries one) so citation grounding can
// read it.
//
// It never fails for a recoverable reason: a missing component degrades to
// "no evidence" rather than erroring: the failure is logged and an empty kbinfos is
// returned.
// ragCacheMinOverlap is the word-overlap ratio at which a new question counts as a
// re-ask of a cached one.
const ragCacheMinOverlap = 0.6

// ragCacheMinShared is the minimum number of shared significant words before a cached
// answer may be reused.
const ragCacheMinShared = 2

// effectiveQuestionMinShared is the overlap the effective-question resolution needs before
// trusting the original question over the outer rewrite. It is deliberately independent of
// ragCacheMinShared.
const effectiveQuestionMinShared = 2

// ragCacheStopwords: for cross-`rag`-call dedup only, never for retrieval or answer
// quality.
var ragCacheStopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "was": true, "were": true,
	"what": true, "which": true, "when": true, "where": true, "who": true,
	"how": true, "of": true, "in": true, "to": true, "for": true, "and": true,
	"or": true, "but": true, "on": true, "at": true, "by": true, "be": true,
	"as": true, "it": true, "that": true, "this": true, "about": true,
	"with": true, "their": true, "its": true, "have": true, "has": true,
	"had": true, "been": true, "being": true, "from": true, "over": true,
	"under": true, "do": true, "does": true, "did": true, "not": true,
	"no": true, "yes": true, "can": true, "could": true, "should": true,
	"would": true, "also": true, "only": true, "very": true, "much": true,
	"more": true, "most": true, "some": true, "any": true,
}

// reQuestionTokens splits a question into words and CJK runs.
var reQuestionTokens = regexp.MustCompile(`[a-zA-Z0-9\x{4e00}-\x{9fff}]+`)

// questionGram is the significant words plus the numeric tokens kept apart, so questions
// naming different numbers are never treated as the same question.
type questionGram struct {
	words   map[string]bool
	numbers map[string]bool
}

// questionKeywords: For English, plain
// tokenisation suffices; CJK tokens survive as whole significant units.
func questionKeywords(question string) questionGram {
	gram := questionGram{words: map[string]bool{}, numbers: map[string]bool{}}
	tokens := reQuestionTokens.FindAllString(strings.ToLower(question), -1)
	for _, t := range tokens {
		if isDigitToken(t) {
			gram.numbers[t] = true
		}
	}
	for _, t := range tokens {
		if len([]rune(t)) > 1 && !isDigitToken(t) && !ragCacheStopwords[t] {
			gram.words[t] = true
		}
	}
	if len(gram.words) == 0 {
		// Fall back to every non-numeric token, so an all-stopword question still has
		// something to compare.
		for _, t := range tokens {
			if len([]rune(t)) > 1 && !isDigitToken(t) {
				gram.words[t] = true
			}
		}
	}
	return gram
}

func isDigitToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// cacheSimilar: significant-word overlap
// (shared / min cardinality), and numbers must be both empty or identical.
func cacheSimilar(a, b questionGram) bool {
	if len(a.words) == 0 || len(b.words) == 0 {
		return false
	}
	if len(a.numbers) > 0 || len(b.numbers) > 0 {
		if !sameTokens(a.numbers, b.numbers) {
			return false
		}
	}
	shared := 0
	for w := range a.words {
		if b.words[w] {
			shared++
		}
	}
	if shared < ragCacheMinShared {
		return false
	}
	minLen := len(a.words)
	if len(b.words) < minLen {
		minLen = len(b.words)
	}
	return float64(shared)/float64(minLen) >= ragCacheMinOverlap
}

func sameTokens(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// FitEvidence: trim
// evidence so question + evidence + the template stay inside a FIXED budget.
//
// The budget is deliberately not the model's full context: a large retrieval
// pool must never fill the window with evidence, so each sufficiency/answer call
// stays cheap. message_fit_in keeps the small side (the question) whole and
// trims the large side (the evidence), which is what the two-message shape
// below asks for.
func FitEvidence(question, evidence string) string {
	if evidence == "" {
		return evidence
	}
	fitted, fitErr := chat.FitMessages("", []schema.Message{
		*schema.UserMessage(question),
		*schema.UserMessage(evidence),
	}, evidenceBudgetTokens)
	if fitErr != "" || len(fitted) == 0 {
		return evidence
	}
	return fitted[len(fitted)-1].Content
}

// routerPromptBody is the rag_agent router prompt, with the summarize_document line left
// as %s: that line is only included when unstructured retrieval is available.
const routerPromptBody = "You are a smart agent. For any question that needs " +
	"evidence from the knowledge bases or the web, call the `rag` tool " +
	"with a self-contained question — it runs the full search-and-answer " +
	"pipeline and returns a cited answer.\n" +
	"After the `rag` tool returns, do not call `rag` again for the same " +
	"user question. Use the returned cited answer as the final answer " +
	"unless the user explicitly asks a new question.\n" +
	"CRITICAL — preserve the full multi-hop structure when phrasing the " +
	"`rag` question. A question that compares two or more DISTINCT " +
	"targets or needs an arithmetic result across them (\"how much taller " +
	"is X than Y\", \"how many days after A's death did B die\", \"which of " +
	"these was discovered last\") MUST keep every target and relation in " +
	"the question you pass to `rag`. Never rewrite a comparison into a " +
	"single-entity question — dropping the second entity (e.g. the " +
	"purchaser in \"how many days after his death did the man who " +
	"purchased it in 1933 die\") makes the pipeline answer only the first " +
	"part. Pass the complete comparison.\n" +
	"%s" +
	"Do not invent facts and do not fabricate document IDs."

// routerSummarizeLine is the conditional summarize_document instruction.
const routerSummarizeLine = "- Call `summarize_document` ONLY when the user explicitly asks to summarise a specific document ('summarise the security audit', 'tldr the onboarding guide'). It needs a document ID.\n"

// SysPrompt: the thin router prompt for callers that bind the tool set. The workflow
// itself lives in the rag graph; the outer model only chooses between retrieval and an
// explicit single-document summary.
//
// hasUnstructured: when false the summarize_document instruction is omitted. systemPrompt
// is the dialog-level UI configuration, prepended when set.
func SysPrompt(systemPrompt string, hasUnstructured bool) string {
	summarizeLine := ""
	if hasUnstructured {
		summarizeLine = routerSummarizeLine
	}
	router := fmt.Sprintf(routerPromptBody, summarizeLine)
	if strings.TrimSpace(systemPrompt) != "" {
		return systemPrompt + "\n\n" + router
	}
	return router
}

// GetCitationGuidelines returns the citation rules the final answer must follow, with an
// optional user-defined override. The citation_prompt.md template is loaded via
// prompts.CitationPrompt, and a non-empty override is taken verbatim.
func GetCitationGuidelines(userDefined string) string {
	return prompts.CitationPrompt(userDefined)
}

// resolveEffectiveQuestion: prefer the user's ORIGINAL, complete question over the outer
// model's rewrite, but only when both clearly describe the same user turn. The outer
// rewrite often
// drops the final target of a multi-hop question, and no later stage can
// recover a deleted answer-attribute.
func resolveEffectiveQuestion(question, originalUserQuestion string) string {
	if question == "" || originalUserQuestion == "" {
		return question
	}
	original := strings.TrimSpace(originalUserQuestion)
	if original == "" {
		return question
	}
	qk := questionKeywords(question)
	ok := questionKeywords(original)
	if len(qk.words) == 0 || len(ok.words) == 0 {
		return question
	}
	shared := 0
	for w := range qk.words {
		if ok.words[w] {
			shared++
		}
	}
	if shared >= effectiveQuestionMinShared {
		return original
	}
	return question
}

// RAGCache: it
// answers a near-identical re-ask from a previous answer instead of re-running
// the whole graph.
//
// Lifetime: the cache is per-turn by construction and is ON by default. Rag builds a
// fresh RAGCache when deps.Cache is nil, so caching is never off, and the auto-built cache
// lives only for that call (the per-turn degenerate case under the current
// single-Rag()-per-turn path), so it can never serve a stale cross-turn hit. A caller that
// wants to share one cache across multiple Rag() calls within a single turn (the concurrent
// tool_call case) injects the same *RAGCache via RAGTools.Cache instead of relying on the
// auto-built one.
type RAGCache struct {
	mu      sync.Mutex
	entries map[string]ragCacheEntry
	// lastAnswered / lastRated: whether the LAST research round wrote an answer (see
	// noteAnswered). They replace the recorded sufficiency verdict; lastRated keeps "it did not
	// answer" distinguishable from "no round has been rated yet".
	lastAnswered bool
	lastRated    bool
	// consecutiveUnanswerable
	// how many consecutive rag calls ended without a
	// satisfying verdict. After two in a row, RAGTools.rag appends a
	// "[Research status] … STOP calling rag again" note to the answer so the
	// outer agent stops re-asking. The counter is reset per turn, so it counts consecutive
	// insufficient rounds within a single request; it is not persisted across turns unless a
	// caller injects a long-lived *RAGCache via RAGTools.Cache.
	//
	// Private and guarded by mu: a turn's concurrent rag() calls share ONE
	// *RAGCache (Rag builds/stores it on deps.Cache before the outer react
	// loop), so an unsynchronized counter would race.
	consecutiveUnanswerable int
}

// NoteUnanswerable records whether one research round ANSWERED, on the shared cache.
//
// The counter used to be driven by the reviewer's verdict (SUFFICIENT reset it, anything else
// bumped it). With no reviewer the fact it tracks is the one the loop actually has: the round
// either wrote an answer or it did not.
func (c *RAGCache) NoteUnanswerable(answered bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if answered {
		c.consecutiveUnanswerable = 0
	} else {
		c.consecutiveUnanswerable++
	}
}

// ConsecutiveUnanswerable reports how many consecutive rag() rounds ended
// without a satisfying verdict.
func (c *RAGCache) ConsecutiveUnanswerable() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.consecutiveUnanswerable
}

type ragCacheEntry struct {
	answer string
	gram   questionGram
}

// NewRAGCache returns an empty per-request cache.
func NewRAGCache() *RAGCache {
	return &RAGCache{entries: map[string]ragCacheEntry{}}
}

// Lookup returns the cached answer for a question judged near-identical to an
// earlier one. Attachments bypass the cache (their content is not part of the
// key), and a previous round that was not SUFFICIENT invalidates reuse: the
// caller asked again precisely because it needs more evidence.
func (c *RAGCache) Lookup(question string) (string, bool) {
	if c == nil || question == "" || !c.reuseAllowed() {
		return "", false
	}
	gram := questionKeywords(question)

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.entries {
		if cacheSimilar(gram, e.gram) {
			return e.answer, true
		}
	}
	return "", false
}

// Store records answer for question.
func (c *RAGCache) Store(question, answer string) {
	if c == nil || question == "" || answer == "" {
		return
	}
	gram := questionKeywords(question)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]ragCacheEntry{}
	}
	c.entries[question] = ragCacheEntry{answer: answer, gram: gram}
}

// noteAnswered records whether the last research round ANSWERED (see NoteUnanswerable).
//
// It replaces the recorded verdict. The cache must not serve a following re-ask out of an answer
// of its own that a previous round had already failed to ground, and with no reviewer the fact
// the round leaves behind is simply whether it wrote an answer at all.
func (c *RAGCache) noteAnswered(answered bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastAnswered, c.lastRated = answered, true
}

// researchStatusTrailer: for a round that did NOT answer it returns the trailing sentence folded
// into the "[Research status]" note. After two consecutive unanswered rag() calls
// (ConsecutiveUnanswerable >= 2 on the shared *RAGCache) it tells the outer
// agent to STOP calling rag again; otherwise it invites a focused re-ask. It
// returns "" when there is nothing to annotate: the round answered, there is no
// status note, or there is no answer to annotate.
func researchStatusTrailer(cache *RAGCache, resp *RunResponse) string {
	if resp.SCAFeedback == "" || resp.Answer == "" {
		return ""
	}
	if cache != nil && cache.ConsecutiveUnanswerable() >= 2 {
		return fmt.Sprintf(" STOP calling rag again: %d consecutive research rounds returned insufficient evidence. The sources likely lack the required data. Give your best answer from the evidence already gathered; do not re-run rag.", cache.ConsecutiveUnanswerable())
	}
	return " If these gaps are material, call rag again with a question focused on them."
}

// reuseAllowed reports whether a cached answer may be reused for the next question.
//
// lastRated keeps "the last round did not answer" distinguishable from "no round has been rated
// yet": the old check allowed reuse when no verdict had been recorded at all, and an unheard-of
// round is not a failure.
func (c *RAGCache) reuseAllowed() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.lastRated || c.lastAnswered
}

// wrapModelForStats wraps the model's invoker so calls made through it are
// counted per phase. It reports false when the model carries no chat.Invoker,
// in which case the caller leaves it untouched.
//
// A counter can only be installed on the known carrier: SessionModel does not expose its
// invoker in a uniform way, so only the carrier that does is wrapped. Callers that go
// through other session models still get counted when they record via CurrentStats(ctx).
func wrapModelForStats(model runtime.SessionModel, stats *runtime.LLMUsageStats) (runtime.SessionModel, bool) {
	src, ok := model.(*runtime.InvokerSessionModel)
	if !ok || src == nil || src.Invoker == nil {
		return model, false
	}
	// Copy before mutating: the caller may share this model across requests, and
	// wrapping in place would attach another run's stats to it.
	wrapped := *src
	wrapped.Invoker = &runtime.CountingInvoker{Inner: src.Invoker, Stats: stats}
	return &wrapped, true
}

// searchDepsFor projects a RAGTools' FULL retrieval configuration into the runtime
// view for one request, including tenant/dataset resolution and the nil-backend
// fallback. Projecting here rather than at each call site keeps the retrieval paths
// (Rag's low/naive pass, the agentic fan-out and action session, the no-model
// fallback) from drifting — a partial copy silently drops tuning such as the
// question-type tag boost.
func searchDepsFor(ctx context.Context, deps RAGTools, req runtime.RunRequest, datasetIDs []string, tenantID string, kb *runtime.Kbinfos, logger *log.Logger) runtime.SearchDeps {
	if tenantID == "" {
		tenantID = runtime.TenantIDFromContext(ctx)
	}
	backend := deps.Retriever
	if backend == nil {
		backend = &runtime.RuntimeRetriever{}
	}
	return runtime.SearchDeps{
		Backend:           backend,
		KbIDs:             datasetIDs,
		TenantID:          tenantID,
		KB:                kb,
		Expand:            deps.Expand,
		Logger:            logger,
		DocIDVerifier:     deps.DocIDVerifier,
		DocChunks:         deps.DocChunks,
		DocTenantResolver: dbDocTenantResolver{},
		MetadataResolver:  deps.MetadataResolver,
		DeclaredMetadata:  deps.DeclaredMetadata,
		Model:             deps.Model, // the calculate tool writes its expression via the model
		DocScope:          deps.DocScope,
		// Python retrieve:614-646 — configuration is the middle precedence
		// level, between an explicit tool argument and the module defaults.
		TopN:                     deps.TopN,
		SimilarityThreshold:      deps.SimilarityThreshold,
		KeywordsSimilarityWeight: deps.KeywordsSimilarityWeight,
		UsingEmbedding:           deps.UsingEmbedding,
		HasEmbedder:              deps.HasEmbedder,
		RerankCandidatesCount:    deps.RerankCandidatesCount,
		TopK:                     deps.TopK,
		MetaDataFilter:           deps.MetaDataFilter,
		// rank_feature (Python retrieve:668): RAGTools carries the KB objects
		// and a tagger, mirroring rank_feature=label_question(question, self.kbs).
		KBs:    deps.KBs,
		Tagger: deps.Tagger,
		// External embedding handle. When the caller supplied one on RAGTools it is used
		// directly; nil keeps the package's internal tenant-default resolver as a fallback
		// (which itself degrades to keyword matching when the DB is uninitialised or
		// encoding panics).
		Embedder: deps.Embedder,
	}
}

// NewDocTenantResolver returns the DB-backed doc→(kb, tenant) resolver used to group a
// document scope by its real owner. It is the same resolver graph_explore uses; the
// compiled expander takes it too
// so a doc scope is scanned per owning dataset instead of being attached to
// every bound one.
func NewDocTenantResolver() runtime.DocTenantResolver { return dbDocTenantResolver{} }

// dbDocTenantResolver implements runtime.DocTenantResolver against the database: it
// maps each document id to its real owning (kb, tenant) so graph_explore can group
// documents by owner and search knowledge bases outside the caller's datasetIDs.
// The lookup is tenant-unscoped on purpose — DocScope is already constrained to the
// user's authorized
// documents — so a document may legitimately resolve to a KB not in the search set.
type dbDocTenantResolver struct{}

func (dbDocTenantResolver) ResolveDocTenants(ctx context.Context, docIDs []string) (map[string]runtime.DocTenant, error) {
	if len(docIDs) == 0 || dao.DB == nil {
		return nil, nil
	}
	var docs []struct {
		ID   string `gorm:"column:id"`
		KbID string `gorm:"column:kb_id"`
	}
	if err := dao.DB.WithContext(ctx).Model(&entity.Document{}).
		Where("id IN ?", docIDs).Select("id", "kb_id").Scan(&docs).Error; err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, nil
	}
	kbIDs := make([]string, 0, len(docs))
	seen := make(map[string]bool, len(docs))
	for _, d := range docs {
		if !seen[d.KbID] {
			seen[d.KbID] = true
			kbIDs = append(kbIDs, d.KbID)
		}
	}
	var kbs []struct {
		ID       string `gorm:"column:id"`
		TenantID string `gorm:"column:tenant_id"`
	}
	if err := dao.DB.WithContext(ctx).Model(&entity.Knowledgebase{}).
		Where("id IN ?", kbIDs).Select("id", "tenant_id").Scan(&kbs).Error; err != nil {
		return nil, err
	}
	tenantByKB := make(map[string]string, len(kbs))
	for _, k := range kbs {
		tenantByKB[k.ID] = k.TenantID
	}
	out := make(map[string]runtime.DocTenant, len(docs))
	for _, d := range docs {
		out[d.ID] = runtime.DocTenant{KBID: d.KbID, TenantID: tenantByKB[d.KbID]}
	}
	return out, nil
}

// chatModelProbeTimeout bounds the pre-flight completion. An unreachable or
// out-of-credit provider answers in milliseconds; a slow one must not hold the
// request before the think block has even opened.
const chatModelProbeTimeout = 20 * time.Second

// probePrompt is the one-token question the pre-flight sends. It is a constant
// so the test models recognise the probe without a bare "ping" literal.
const probePrompt = "ping"

// errProbeTimeout marks a probe that exhausted ITS OWN budget while the request
// context was still alive — a slow model, not a dead provider.
var errProbeTimeout = errors.New("chat model pre-flight timed out")

// probeChatModel makes the smallest possible completion to learn whether the
// run's chat model is usable at all. It is a DETECTION call: the reply is
// discarded and only its error matters. A nil model reports no error — the
// existing "no model configured" fallbacks own that case.
//
// A probe that only timed out comes back as errProbeTimeout: a slow model is
// not a dead one, and the caller must not turn a long first token into an
// outage.
func probeChatModel(ctx context.Context, deps RAGTools) error {
	return probeChatModelWithin(ctx, deps, chatModelProbeTimeout)
}

// probeChatModelWithin is probeChatModel with an injectable budget, so a test
// can exercise the timeout path without waiting out the real 20s.
func probeChatModelWithin(ctx context.Context, deps RAGTools, budget time.Duration) error {
	if deps.Model == nil {
		return nil
	}
	probeCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	if _, err := deps.Model.Complete(probeCtx, []schema.Message{*schema.UserMessage(probePrompt)}, nil); err != nil {
		if ctx.Err() == nil && probeCtx.Err() != nil {
			return fmt.Errorf("%w after %s: %v", errProbeTimeout, budget, err)
		}
		return err
	}
	return nil
}

func Rag(ctx context.Context, deps RAGTools, req runtime.RunRequest) *RunResponse {
	logger := deps.Logger
	if logger == nil {
		logger = _LOG
	}
	spec := runtime.GetMode(req.ThinkingMode)
	if !IsKnownMode(req.ThinkingMode) {
		// An unrecognised label resolves to NAIVE; log it so a typo in the mode
		// is visible instead of silently downgrading the request.
		logger.Printf("[Agentic RAG] unrecognised thinking mode %q; falling back to naive (non-agentic) retrieval", req.ThinkingMode)
	}

	// build the per-call usage counters and bind them to the
	// context so every LLM call beneath is attributed to its phase. Without this
	// the counting machinery is inert: CurrentStats(ctx) would stay nil and the
	// phase markers already sprinkled through the graph would record nothing.
	stats := deps.Stats
	if stats == nil {
		stats = runtime.NewLLMUsageStats()
		deps.Stats = stats
	}
	ctx = runtime.WithStats(ctx, stats)
	// Bind the caller's step reporter for the whole run: every stage and every
	// tool beneath reads it (runtime.StepsFrom) and reports its own step, so the
	// per-request trace is assembled from the stages that actually run.
	ctx = runtime.WithSteps(ctx, deps.Steps)

	// LLM pre-flight, before anything is announced. The agentic modes cannot
	// answer without a working model, and a dead provider used to surface only at
	// COMPOSE time — after the think block had narrated a research run that could
	// never finish (measured: MiniMax "insufficient balance" burned seconds of
	// narration plus a series of degraded-failure lines before the fallback
	// answer). One minimal call answers "is the provider usable at all"; when it
	// fails the run reports the provider's own message in the classic
	// `**ERROR**: …` shape and returns BEFORE reporting a step or starting the
	// tool, so no think block opens and the user sees what a naive-mode failure
	// shows.
	//
	// It runs ONCE per turn, at the turn's entry: Rag is called once per user turn
	// (the outer react loop's later `rag` tool calls re-enter the research graph
	// directly), so there is no second round to re-probe and no per-round state to
	// keep. It is UNCONDITIONAL: nothing between the request and this call decides
	// whether the model is checked — a provider that cannot answer must be
	// reported, never tolerated.
	//
	// The probe deliberately runs BEFORE wrapModelForStats: it is a health check,
	// not a phase's work, so its call is kept out of llm_stats.
	if spec.Label != "naive" {
		err := probeChatModel(ctx, deps)
		if errors.Is(err, errProbeTimeout) {
			// The probe's own budget ran out, not the provider's answer: let
			// the run proceed and fail (if it must) on its real budget.
			logger.Printf("[Agentic RAG] %v; continuing without a pre-flight verdict", err)
		} else if err != nil {
			// DEVELOPER LOG ONLY — no step is reported: the whole point is that
			// the user gets the error instead of a think block narrating work
			// that cannot happen. No GraphFailed either: that flag means the
			// research graph itself errored (RunResponse.GraphFailed) and the
			// graph never started; the pipeline keys on the `**ERROR**:` prefix.
			logger.Printf("[Agentic RAG] chat model pre-flight failed: %v", err)
			return &RunResponse{Answer: errorAnswerText(err), Mode: spec}
		}
	}

	// The run's logger stays the DEVELOPER log — nothing intercepts it. The
	// user-visible trace comes from the steps stages report themselves
	// (deps.Steps), so a diagnostic line can never leak into the think block and
	// a reworded line can never silently drop a step.
	//
	// wraps the chat model in CountingChatModel(chat_mdl.clone,
	// self.llm_stats). Go wraps the session model's invoker the same way, so
	// calls made through deps.Model are counted per phase too. The pre-flight
	// above probes the UNWRAPPED model on purpose, so a health check is never
	// counted as a phase's work.
	if wrapped, ok := wrapModelForStats(deps.Model, stats); ok {
		deps.Model = wrapped
	}
	defer stats.Log(logger)

	// tell the caller research is starting, before any retrieval
	// work, so it can show progress for the (potentially long) graph run.
	if deps.ToolStarted != nil {
		deps.ToolStarted()
	}

	// When an outer chat model is wired, the request is driven by the outer react loop
	// that binds [rag, summarize_document] and treats `rag` as a terminal tool. The inner
	// graph runs only when the model decides to call `rag`; otherwise the model answers
	// directly (or calls summarize_document to read a whole document first). When no outer
	// model is configured, or it cannot emit tool calls, we fall through to the direct
	// RunAgenticRAG path so behaviour is unchanged for every current caller.
	// The last user message carries the question, text attachments, and — for vision
	// models — image content blocks. These reach the outer react loop's history
	// (prepareOuterReact) so a vision model actually sees the images instead of silently
	// dropping them. Assembled here (not in the caller) so the multimodal construction
	// stays in the agentic_rag package beside the schema import.
	if len(deps.Messages) == 0 {
		deps.Messages = multimodalUserMessage(req.Question, req.TextAttachments, req.Images)
	} else if req.TextAttachments != "" || len(req.Images) > 0 {
		last := len(deps.Messages) - 1
		if deps.Messages[last].Role == schema.User {
			deps.Messages = append([]schema.Message(nil), deps.Messages...)
			deps.Messages[last] = multimodalUserMessage(deps.Messages[last].Content, req.TextAttachments, req.Images)[0]
		}
	}
	// Text attachments also feed the direct (non-outer) path, which appends them to the
	// question; attachments bypass the near-duplicate cache (see cacheable above).
	if deps.TextAttachments == "" {
		deps.TextAttachments = req.TextAttachments
	}

	// Build the per-request RAGCache up-front so the outer react loop (below)
	// reuses the SAME cache across its multiple rag() calls within one turn.
	// Keeping deps.Cache non-nil here is what makes the _consecutive_unanswerable
	// guard live: ConsecutiveUnanswerable is incremented
	// inside RunAgenticRAG and gated on deps.Cache != nil, so without a shared
	// cache the outer loop would never reach the "STOP calling rag again" verdict.
	// The outer loop must share the counter, so deps.Cache is set before any branch rather
	// than only on the direct path.
	if deps.Cache == nil {
		deps.Cache = NewRAGCache()
	}

	if deps.Outer != nil {
		if deps.AnswerSink != nil {
			return runOuterReactStream(ctx, deps, req, logger)
		}
		return runOuterReact(ctx, deps, req, logger)
	}

	// Re-ask guard: reuse a near-identical question's cached answer instead of re-running
	// the graph. Attachments bypass it, since their content is appended to the question
	// below and is not part of the key. Caching is on by default: deps.Cache is guaranteed
	// non-nil here (built above, before the outer-react branch, so it is shared across the
	// outer loop's multiple rag() calls), and the auto-built cache lives only for this call
	// (the per-turn degenerate case, since the current reasoning path issues a single Rag()
	// per turn), so it can never serve a stale cross-turn hit. A caller that wants to share
	// one cache across multiple Rag() calls within a single turn (the concurrent tool_call
	// case) injects the same *RAGCache via deps.Cache instead of relying on the auto-built
	// one.
	cache := deps.Cache
	cacheable := deps.TextAttachments == ""
	if cacheable && cache != nil {
		if cached, hit := cache.Lookup(req.Question); hit {
			step(ctx, logger, "Agentic RAG", "Cache hit: reused the answer for the near-identical question %q and skipped research.", trunc(req.Question, 80))
			return &RunResponse{Answer: cached, Mode: spec}
		}
	}

	// Prefer the user's ORIGINAL, complete question over the outer rewrite
	// . The outer rewrite often drops the final target of a
	// multi-hop question, and no later stage can recover it.
	if deps.OriginalQuestion != "" {
		if effective := resolveEffectiveQuestion(req.Question, deps.OriginalQuestion); effective != req.Question {
			logger.Printf("[Agentic RAG] using original user question over outer rewrite (original=%q → rewrite=%q)",
				trunc(deps.OriginalQuestion, 80), trunc(req.Question, 80))
			req.Question = effective
		}
	}
	if deps.TextAttachments != "" {
		req.Question += deps.TextAttachments
	}

	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = runtime.TenantIDFromContext(ctx)
	}
	datasetIDs := req.DatasetIDs

	kb := &runtime.Kbinfos{}
	searchDeps := searchDepsFor(ctx, deps, req, datasetIDs, tenantID, kb, logger)

	resp := &RunResponse{Mode: spec, Kbinfos: kb}

	// hand over to the graph driver, which owns the mode dispatch
	// (agentic graph vs. direct/naive retrieval) and formalization.
	//
	// The terminal composition is handed to the graph instead of being run here:
	// The graph composes inside the last node of both graphs (agentic and low
	// formalize_answer). The closure is idempotent so the post-graph call below is a no-op
	// once the graph has composed — a graph that never reaches its last node (or the naive
	// path, which composes under naiveAnswerSystem instead) still gets an answer here.
	composed := false
	compose := func(ctx context.Context, partialAnswer, emptyResult bool, question string) {
		if composed {
			return
		}
		composed = true
		composeFinalAnswer(ctx, deps, req, kb, resp, logger, partialAnswer, emptyResult, question)
	}
	deps.Finalize = compose

	// Say why this run goes straight to the research graph — DEVELOPER LOG ONLY.
	// Only one reason remains: no outer model was wired (the outer branch above did
	// not fire, and no cache hit short-circuited). Whether the request's chat model
	// can call tools at all is decided before this call, by the caller that picks
	// between the agentic path and the regular chat.
	//
	// It stays out of the think block on purpose: it describes the runtime' wiring,
	// not the question or the research, and a reader gets one of these on every run
	// in a deployment without an outer loop. The reader-visible trace simply has no
	// "[Tool loop]" section in that configuration, which is the honest shape — the
	// section exists when a loop ran.
	logger.Printf("[Agentic RAG] No outer model is wired, so this run has no outer tool loop.")

	RunAgenticRAG(ctx, deps, req, searchDeps, kb, resp, logger, spec)

	resp.Chunks = kb.Chunks
	resp.DocAggs = kb.DocAggs
	resp.EmptyResult = len(kb.Chunks) == 0

	// Publish evidence for citation grounding. Best-effort: when no canvas
	// state is attached (e.g. unit tests), skip silently — mirroring
	// RetrievalTool's behaviour.
	if len(kb.Chunks) > 0 {
		runtime.PublishReferences(ctx, kb)
	}

	// The graph's last node composed already (see compose above); this only
	// catches runs that never reached it. The naive path (_naive_rag)
	// already composed under naiveAnswerSystem from flat "[i] content"
	// evidence, so composing here would replace that with the
	// FinalAnswerSystem / kb_prompt answer.
	if spec.Label != "naive" {
		// A run that never reached the graph's last node produced neither a
		// partial answer nor empty-result state flags (the fallback compose reads the same
		// defaults: no partial preamble, no-evidence hedge only when the pool is actually
		// empty). The question is empty —
		// the graph never formalized, so compose falls back to req.Question.
		compose(ctx, false, false, "")
	}

	// A "[Research status]" note is produced for every round that did NOT settle the question:
	// it tells the outer agent to STOP calling rag again after two consecutive unanswered
	// rounds, and otherwise invites a focused re-ask.
	//
	// It goes to resp.ResearchStatus, NOT to resp.Answer. Appending it to the answer put the
	// runtime's bookkeeping in the user's text (and in the answer cache this block writes
	// below, so the note then travelled into a later question's answer).
	if t := researchStatusTrailer(deps.Cache, resp); t != "" {
		// The period closes the hint clause before the trailer sentence.
		resp.ResearchStatus = "\n\n[Research status] " + resp.SCAFeedback + "." + t
	}

	// Cache the freshly produced answer for later near-identical questions, and
	// remember the verdict so a following re-ask is not answered from an
	// admittedly incomplete one.
	if cacheable && cache != nil {
		cache.Store(req.Question, resp.Answer)
		cache.noteAnswered(strings.TrimSpace(resp.CollectedAnswer) != "")
	}
	return resp
}

// composeFinalAnswer runs the graph's terminal node and writes the result onto
// the response.
//
// When no model is configured this is a no-op: the caller still receives the evidence and
// can answer with it, rather than failing.
//
// partialAnswer / emptyResult are the compose prompt inputs read off the graph state:
// `partial_answer` drives the partial-information preamble, `empty_result` is the
// always-true-in-graph term of `no_evidence = abstain or empty_result or not chunks`.
// question is the graph state's FORMALIZED question the last node forwarded; empty falls
// back to req.Question — only the post-graph fallback composes after a run that never
// formalized.
// The graph's Finalize forwards the formalize_answer node's own state values;
// the Go-only post-graph fallback passes the response flags instead.
func composeFinalAnswer(ctx context.Context, deps RAGTools, req runtime.RunRequest, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger, partialAnswer, emptyResult bool, question string) {
	if deps.Model == nil {
		return
	}
	// Compose from the graph state's formalized question: the graph researched the full
	// multi-hop question, so the compose prompt must carry it — not the outer tool argument,
	// which compresses multi-hop questions to their first hop and collapses the final answer
	// to the first completed sub-answer.
	composeQuestion := question
	if composeQuestion == "" {
		composeQuestion = req.Question
	}
	// The terminal node runs in the "finalize" phase; without it every call made here is
	// attributed to "unknown" in the usage table.
	ctx, done := runtime.Phase(ctx, runtime.PhaseFinalize)
	defer done()
	if deps.ComposeAnswer != nil && !*deps.ComposeAnswer {
		return
	}
	// The compose publishes the ordered evidence list it rendered (kb's
	// CiteChunkIDs) while it runs; hand it to the caller so the chat pipeline
	// can resolve the answer's [ID:n] markers against that same list. Covered by
	// a defer because both the streaming and the one-shot path return from
	// inside the branches below.
	defer func() {
		if kb != nil {
			resp.CiteChunkIDs = kb.CiteChunkIDs
		}
	}()
	// Stamp each pool chunk with its document's declared metadata, so the evidence
	// blocks carry the passage's document attributes (file name, update time, ...)
	// beside its text: a two-hop question — which documents, and what is in them —
	// is then answerable from the evidence alone, instead of from the slot record
	// the answer is told not to quote.
	attachDocMetadata(ctx, deps, kb, logger)

	adeps := AnswerDeps{
		Model:         deps.Model,
		CiteRules:     deps.CiteRules,
		SystemPrompt:  deps.SystemPrompt,
		EmptyResponse: deps.EmptyResponse,
		MaxTokens:     deps.EvidenceMaxTokens,
		MaxLength:     deps.MaxLength,
		Logger:        logger,
		// The direct fallback receives the original multimodal messages, so the non-outer
		// compose model sees the vision-gated images too. The outer react path clears these
		// in the rag terminal tool (the inner compose is text-only).
		UserImages: req.Images,
	}
	// The session that read the evidence writes the answer (see the design's R2), and its text is
	// the answer — so it is taken HERE, before the streaming/one-shot fork, because this funnel is
	// the only code both paths share.
	//
	// The check used to live inside ComposeAnswerWith alone, which production never reaches: with
	// an AnswerSink the run streams (ComposeAnswerStream) and returns. Measured 2026-09-20: 15 of
	// 18 answered requests had a session answer, and every one of them was replaced by a compose
	// call that had not read the passages — the answers then cited the six blocks that call
	// renders while the pool held up to 213 passages, and 6 questions came back "the evidence does
	// not contain it".
	if ans, ok := sessionAnswer(kb, false); ok {
		useSessionAnswer(kb, resp, ans)
		if partialAnswer {
			resp.Partial = true
		}
		// Deliver it through the sink so a streaming client still receives the answer the same
		// way it receives a composed one (in one piece: the session's text is already complete).
		//
		// What is delivered is resp.Answer — the text useSessionAnswer just built, prose plus the
		// numbered evidence the run rendered — not the raw session text. Delivering the raw text
		// showed the reader the session's own numbers, which the delivered answer had already
		// dropped (measured 2026-09-21, 三国/关羽: the streamed draft carried [ID:93] beside a
		// sentence that is not there, while the answer the client ended up with carried none of it).
		if deps.AnswerSink != nil {
			deps.AnswerSink.reset()
			deps.AnswerSink.deliver(resp.Answer, false)
		}
		log := logger
		if log == nil {
			log = _LOG
		}
		log.Printf("[Agentic RAG] Using the answer the research session wrote (%d character(s)); %d passage(s) in the citation registry.",
			utf8.RuneCountInString(resp.Answer), len(kb.CiteChunkIDs))
		return
	}
	if why := sessionAnswerBlocked(kb, false); why != "" {
		// A written answer that is not used must say so, and say why: this silence is what hid the
		// fact that 20 of 20 rounds had an answer while the compose still ran every time.
		log := logger
		if log == nil {
			log = _LOG
		}
		log.Printf("[Agentic RAG] the session's answer will not stand (%s); composing instead.", why)
	}

	// Stream the answer when the model and the caller both support it, so the
	// user sees text while it is produced instead of only at the end.
	if deps.AnswerSink != nil {
		if streamer, ok := deps.Model.(runtime.StreamingSessionModel); ok {
			deps.AnswerSink.reset()
			streamed, err := ComposeAnswerStream(ctx, adeps, streamer, kb, composeQuestion, partialAnswer, emptyResult, func(delta string, isThink bool) error {
				deps.AnswerSink.deliver(delta, isThink)
				return nil
			})
			if err == nil {
				resp.Answer = streamed.Answer
				if streamed.Partial {
					resp.Partial = true
				}
				return
			}
			// A partially streamed answer must not be sent twice: tell the sink
			// to drop what it already forwarded and fall back to one shot. The
			// think block reports the provider's own error (bounded): on a dead
			// provider this is the first failure a user would otherwise never
			// see, and the one-shot retry below fails the same way.
			runtime.StepsFrom(ctx).StageLineDetail(logger, "Agentic RAG",
				"Streaming generation failed; falling back to a single call: "+providerErrorSummary(err),
				fmt.Sprintf("streaming compose failed (%v); falling back to a single call", err))
			deps.AnswerSink.reset()
		}
	}
	// No manual RecordCall here: the call is attributed to the "finalize" phase
	// by the wrapped invoker above.
	// empty_result is the third no-evidence term — the loop's own
	// "nothing was found" signal, distinct from abstain and from an empty pool.
	res := ComposeAnswerWith(ctx, adeps, kb, composeQuestion, partialAnswer, false, emptyResult)
	if deps.Stats != nil && res.Failed {
		deps.Stats.RecordFailed(runtime.PhaseFinalize)
	}
	resp.Answer = res.Answer
	if res.Partial || partialAnswer {
		// Reflect the graph state's partial flag back on the response even when the
		// composed text itself predates the flag.
		resp.Partial = true
	}
	// A FAILED compose is a terminal message, not a stream: the pipeline carries
	// it in the final result, exactly how the classic path delivers its own
	// `**ERROR**: …` answer. Streaming it here as well made the client show the
	// text twice — once as an answer delta, once in the final.
	if deps.AnswerSink != nil && !res.Failed {
		deps.AnswerSink.deliver(res.Answer, false)
	}
}

// outerReactParts holds the pieces shared by the streaming and non-streaming outer react
// loops.
type outerReactParts struct {
	kb      *runtime.Kbinfos
	sd      runtime.SearchDeps
	spec    runtime.ModeSpec
	resp    *RunResponse
	tools   []map[string]any
	history []models.Message
	system  string
}

// multimodalUserMessage assembles the last user turn: its text is the question plus any
// text attachments, augmented with image content blocks for vision-capable models.
// imageFiles are vision-gated base64 data URIs (data:...); textAttachments is
// joined file content. When there is nothing to carry (no question, no
// attachments, no images) it returns an empty slice so callers can skip it.
//
// The multimodal part list is used ONLY when images are attached: the text (question +
// attachments) stays a plain string, and the message is converted to content blocks just
// for image attachments. Sending a content-block array for a text-only question is not
// harmless — text-only providers silently drop it (Zhipu GLM answers an empty user turn
// with "Hello! How can I assist you today?" instead of calling the `rag` tool), which is
// why it is avoided here.
func multimodalUserMessage(question, textAttachments string, imageFiles []string) []schema.Message {
	text := question
	if textAttachments != "" {
		if text != "" {
			text = text + "\n\n" + textAttachments
		} else {
			text = textAttachments
		}
	}
	if len(imageFiles) == 0 {
		if text == "" {
			return nil
		}
		return []schema.Message{{Role: schema.User, Content: text}}
	}
	parts := make([]schema.MessageInputPart, 0, 1+len(imageFiles))
	if text != "" {
		parts = append(parts, schema.MessageInputPart{Type: schema.ChatMessagePartTypeText, Text: text})
	}
	for i := range imageFiles {
		uri := imageFiles[i]
		parts = append(parts, schema.MessageInputPart{
			Type:  schema.ChatMessagePartTypeImageURL,
			Image: &schema.MessageInputImage{MessagePartCommon: schema.MessagePartCommon{URL: &uri}},
		})
	}
	if len(parts) == 0 {
		return nil
	}
	return []schema.Message{{Role: schema.User, UserInputMultiContent: parts}}
}

// multimodalContentBlocks converts schema.MessageInputPart multimodal content
// into the OpenAI-style []interface{} block slice entity/models.Message.Content
// expects (mirroring entity/models toInternalMessages). Plain text Content is
// kept as a leading text block so the model sees both the original text and the
// multimodal parts.
func multimodalContentBlocks(m schema.Message) interface{} {
	blocks := make([]interface{}, 0, 1+len(m.UserInputMultiContent))
	if m.Content != "" {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": m.Content})
	}
	for _, part := range m.UserInputMultiContent {
		switch part.Type {
		case schema.ChatMessagePartTypeText:
			if part.Text != "" {
				blocks = append(blocks, map[string]interface{}{"type": "text", "text": part.Text})
			}
		case schema.ChatMessagePartTypeImageURL:
			if part.Image != nil && part.Image.URL != nil {
				blocks = append(blocks, map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]interface{}{"url": *part.Image.URL},
				})
			}
		}
	}
	return blocks
}

// prepareOuterReact builds the state both outer react loops need: the evidence set,
// the search deps, the two tool definitions and the outer conversation history
// rendered as models.Message.
func prepareOuterReact(ctx context.Context, deps RAGTools, req runtime.RunRequest, logger *log.Logger) *outerReactParts {
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = runtime.TenantIDFromContext(ctx)
	}
	kb := &runtime.Kbinfos{}
	sd := searchDepsFor(ctx, deps, req, req.DatasetIDs, tenantID, kb, logger)
	spec := runtime.GetMode(req.ThinkingMode)

	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name": "rag",
				"description": "Run the full agentic research graph over the configured datasets and return a cited answer. " +
					"WHEN TO CALL: every question whose answer must come from the knowledge base — a fact, an enumeration, " +
					"a count, \"which documents ...\", a date or updated-time range, a comparison, a multi-hop relation, " +
					"or anything that needs citations. " +
					"DO NOT CALL: only for pure chit-chat or a rewriting task that cannot need the datasets. " +
					"You MUST call this before answering a knowledge-base question: the dataset names and your own prior " +
					"knowledge are not evidence, and answering without it is a wrong answer even when it reads well.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"question": map[string]any{
							"type":        "string",
							"description": "The research question to investigate.",
						},
					},
					"required": []string{"question"},
				},
			},
		},
		{
			"type": "function",
			"function": map[string]any{
				"name": "summarize_document",
				"description": "Read an entire document by id into the evidence set and return a short summary the model can answer from. " +
					"WHEN TO CALL: only when you ALREADY hold a doc_id — returned by an earlier `rag` call, or given by the user. " +
					"DO NOT CALL: when you have no doc_id, or the question asks WHICH documents exist, or it needs several " +
					"documents compared or aggregated — that is the `rag` tool's job. This tool cannot find a document: " +
					"called without a real doc_id it only returns an error and wastes a round.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"doc_id": map[string]any{
							"type":        "string",
							"description": "The document id to read — one you already hold from a prior `rag` result or from the user.",
						},
					},
					"required": []string{"doc_id"},
				},
			},
		},
	}

	history := make([]models.Message, 0, len(deps.Messages))
	for _, m := range deps.Messages {
		msg := models.Message{Role: string(m.Role), Content: m.Content}
		if len(m.UserInputMultiContent) > 0 {
			msg.Content = multimodalContentBlocks(m)
		}
		history = append(history, msg)
	}
	system := deps.SystemPrompt
	if strings.TrimSpace(system) == "" {
		system = "You are a research assistant. Use the tools provided to gather evidence and answer the user's question with citations where possible."
	}

	return &outerReactParts{
		kb:      kb,
		sd:      sd,
		spec:    spec,
		resp:    &RunResponse{Mode: spec, Kbinfos: kb},
		tools:   tools,
		history: history,
		system:  system,
	}
}

// outerStreamMux merges the outer model's stream with the inner rag stream into one sink,
// so the outer model's reasoning, the inner research log and the inner answer all share a
// single think/answer block.
//
// The models layer hands the outer stream down as (delta, reason) text that may
// carry literal <think>/</think> markers, while the sink takes an explicit
// isThink flag; the mux tracks the markers to keep the state and forwards plain
// text only.
type outerStreamMux struct {
	mu            sync.Mutex
	sink          *AnswerSink
	inThink       bool
	terminalFired bool
	// outerText accumulates the outer model's OWN non-think text. A tool-less reply is the
	// answer, so the streaming loop must be able to hand it back to the caller instead of
	// dropping it (the inner `rag` stream arrives through deliver, never through here).
	outerText strings.Builder
}

// sender is the (delta, reason) callback handed to ChatStreamlyWithTools.
func (m *outerStreamMux) sender(delta, reason *string) error {
	var text string
	if delta != nil {
		text = *delta
	}
	if reason != nil && *reason != "" {
		text += *reason
	}
	if text == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for text != "" {
		if strings.HasPrefix(text, "<think>") {
			m.inThink = true
			text = text[len("<think>"):]
			continue
		}
		if strings.HasPrefix(text, "</think>") {
			m.inThink = false
			text = text[len("</think>"):]
			continue
		}
		next := len(text)
		if i := strings.Index(text, "<think>"); i >= 0 && i < next {
			next = i
		}
		if i := strings.Index(text, "</think>"); i >= 0 && i < next {
			next = i
		}
		chunk := text[:next]
		text = text[next:]
		if chunk == "" {
			continue
		}
		// Non-think outer text is dropped once the terminal tool has fired: what follows is
		// the aggregate tool result, and the answer has already been streamed from inside
		// the tool.
		if m.terminalFired && !m.inThink {
			continue
		}
		if !m.inThink {
			m.outerText.WriteString(chunk)
		}
		m.emit(chunk, m.inThink)
	}
	return nil
}

// deliver is the inner-stream entry point: research progress (isThink) and the
// composed answer (not think).
func (m *outerStreamMux) deliver(delta string, isThink bool) {
	if delta == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inThink = isThink
	m.emit(delta, isThink)
}

// markTerminal records that the terminal `rag` tool ran, so later outer text is treated
// as the aggregate tool result and dropped.
func (m *outerStreamMux) markTerminal() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.terminalFired = true
}

// emit forwards one chunk. Caller must hold m.mu.
func (m *outerStreamMux) emit(chunk string, isThink bool) {
	if m.sink == nil || m.sink.OnDelta == nil {
		return
	}
	m.sink.OnDelta(chunk, isThink)
}

// runOuterReact drives the rag_agent outer loop (tools=[rag, summarize_document], with
// `rag` terminal). The outer chat model decides whether to:
//   - call `rag` (terminal): run the full inner agentic graph and return the
//     cited answer, which the terminal short-circuit turns into the final answer;
//   - call `summarize_document` (non-terminal): read a whole document into the
//     evidence set and let the model answer from it on the next round;
//   - answer directly with no tool call.
//
// deps.Outer must be non-nil; Rag() only delegates here when it is set, so
// existing callers that don't wire an outer model keep using the direct
// RunAgenticRAG path unchanged.
func runOuterReact(ctx context.Context, deps RAGTools, req runtime.RunRequest, logger *log.Logger) *RunResponse {
	p := prepareOuterReact(ctx, deps, req, logger)

	session := &outerReactSession{
		ctx:    ctx,
		deps:   deps,
		req:    req,
		sd:     p.sd,
		kb:     p.kb,
		resp:   p.resp,
		logger: logger,
		spec:   p.spec,
	}

	outer := deps.Outer
	outer.BindTools(session, p.tools)
	outer.SetTerminalTools("rag")

	// The loop's opening and closing steps are reported HERE — where the loop
	// actually is. The chat pipeline used to synthesize these lines before
	// calling the runtime, so they appeared even when no outer loop ran and the
	// research came from the direct graph: the trace described a loop that had
	// not happened.
	loop := runtime.StepsFrom(ctx)
	loop.Stage(logger, "Tool loop", "Deciding what to do next (step 1); available tools: %s", outerToolNames(p.tools))

	answer, _, err := outer.ChatWithTools(ctx, p.system, p.history, &models.ChatConfig{})
	if err != nil {
		loop.StageLine(logger, "Tool loop", "The outer model call failed; running the research graph directly.")
		logger.Printf("[Agentic RAG] outer react failed: %v; falling back to direct graph", err)
		// Fall back to the inner graph directly so the user still gets an
		// answer. The graph composes inside its last node with the FORMALIZED question
		// (read from the graph state); the guarded direct call below only fires when the
		// graph never composed.
		composed := false
		deps.Finalize = func(fctx context.Context, partial, empty bool, question string) {
			if composed {
				return
			}
			composed = true
			composeFinalAnswer(fctx, deps, req, p.kb, p.resp, logger, partial, empty, question)
		}
		RunAgenticRAG(ctx, deps, req, p.sd, p.kb, p.resp, logger, p.spec)
		if !composed {
			composeFinalAnswer(ctx, deps, req, p.kb, p.resp, logger, p.resp.Partial, true, "")
		}
		return p.resp
	}
	// The fold returned the LOWEST-INDEX terminal rag call's answer: keep that
	// call's evidence so the answer's [ID:n] markers line up with the chunks.
	session.selectEvidence(answer)
	p.resp.Answer = answer
	p.resp.Chunks = p.kb.Chunks
	p.resp.DocAggs = p.kb.DocAggs
	// The numbering the answer's [ID:n] markers were written with travels WITH those chunks:
	// the chat pipeline resolves a marker by position in this list, so a response carrying the
	// passages without their numbering leaves the list empty and every marker indexes the POOL
	// instead — the citation then opens whatever passage happens to sit at that position.
	p.resp.CiteChunkIDs = append([]string(nil), p.kb.CiteChunkIDs...)
	p.resp.EmptyResult = len(p.kb.Chunks) == 0
	loop.StageLine(logger, "Tool loop", outerLoopEndLine(session.ragCalls(), strings.TrimSpace(p.resp.Answer) != ""))
	return p.resp
}

// runOuterReactStream is the streaming counterpart of runOuterReact: it drives the same
// [rag, summarize_document] react loop but merges two live streams into the single sink
// the caller supplied:
//   - the outer model's own reasoning/text, arriving through the models layer;
//   - the inner rag run's research log (think) and composed answer (answer),
//     produced while the terminal `rag` tool executes.
//
// `rag` streams its answer itself and returns "", so the terminal short-circuit stops the
// loop without re-streaming it (the terminal result is ignored once the inner sink has
// streamed it).
func runOuterReactStream(ctx context.Context, deps RAGTools, req runtime.RunRequest, logger *log.Logger) *RunResponse {
	p := prepareOuterReact(ctx, deps, req, logger)
	mux := &outerStreamMux{sink: deps.AnswerSink}

	// Route the inner run's ANSWER through the mux so it shares one block with
	// the outer model's stream. The inner run's STEPS are not routed here: the
	// reporter was already bound on ctx by Rag, and steps read it from there
	// (overwriting a copy's Steps would have no effect).
	inner := deps
	inner.AnswerSink = &AnswerSink{OnDelta: func(delta string, isThink bool) { mux.deliver(delta, isThink) }}

	session := &outerReactSession{
		ctx:    ctx,
		deps:   inner,
		req:    req,
		sd:     p.sd,
		kb:     p.kb,
		resp:   p.resp,
		logger: logger,
		spec:   p.spec,
		mux:    mux,
	}

	outer := deps.Outer
	outer.BindTools(session, p.tools)
	outer.SetTerminalTools("rag")

	// Same opening step as runOuterReact, reported where the loop is.
	loop := runtime.StepsFrom(ctx)
	loop.Stage(logger, "Tool loop", "Deciding what to do next (step 1); available tools: %s", outerToolNames(p.tools))

	stream := true
	_, err := outer.ChatStreamlyWithTools(ctx, p.system, p.history, &models.ChatConfig{Stream: &stream}, mux.sender)
	if err != nil {
		loop.StageLine(logger, "Tool loop", "The outer model call failed; running the research graph directly.")
		logger.Printf("[Agentic RAG] outer react stream failed: %v; falling back to direct graph", err)
		// Fall back with the caller's own sink so the answer still streams out.
		// Same guarded compose as runOuterReact: the graph composes inside its
		// last node with the FORMALIZED question; the direct call only fires
		// when the graph never reached it.
		composed := false
		deps.Finalize = func(fctx context.Context, partial, empty bool, question string) {
			if composed {
				return
			}
			composed = true
			composeFinalAnswer(fctx, deps, req, p.kb, p.resp, logger, partial, empty, question)
		}
		RunAgenticRAG(ctx, deps, req, p.sd, p.kb, p.resp, logger, p.spec)
		if !composed {
			composeFinalAnswer(ctx, deps, req, p.kb, p.resp, logger, p.resp.Partial, true, "")
		}
		return p.resp
	}
	// A tool-less reply is the answer (the outer model's text is returned verbatim when it
	// decides not to call a tool). The stream already delivered it to the caller's sink;
	// recording it on the response is what lets the chat pipeline short-circuit instead of
	// composing a second,
	// contradicting answer from the (empty) evidence set. When the terminal
	// `rag` tool fired, composeFinalAnswer already owns resp.Answer.
	if p.resp.Answer == "" && !mux.terminalFired {
		p.resp.Answer = strings.TrimSpace(mux.outerText.String())
	}
	// Keep only the evidence of the call that owns the answer (selectEvidence
	// no-ops when the answer came from the outer model instead of a rag call).
	session.selectEvidence(p.resp.Answer)
	// Same as runOuterReact: the numbering the answer's [ID:n] markers were written with has to
	// travel with the passages it numbers. Without it the chat pipeline's published list is
	// empty, citePoolIdx falls back to pool order, and every marker opens whichever passage
	// happens to sit at that position (rendered_blocks == pool size is the tell).
	p.resp.CiteChunkIDs = append([]string(nil), p.kb.CiteChunkIDs...)
	p.resp.Chunks = p.kb.Chunks
	p.resp.DocAggs = p.kb.DocAggs
	p.resp.EmptyResult = len(p.kb.Chunks) == 0
	loop.StageLine(logger, "Tool loop", outerLoopEndLine(session.ragCalls(), strings.TrimSpace(p.resp.Answer) != ""))
	return p.resp
}

// outerToolNames renders the bound tool list for the loop's opening step: the
// OpenAI-style schemas the outer model was actually given.
func outerToolNames(tools []map[string]any) string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		fn, _ := t["function"].(map[string]any)
		name, _ := fn["name"].(string)
		if name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, ", ")
}

// outerLoopEndLine closes the loop's narration with what actually ended it. Its two
// inputs are independent — whether the `rag` tool was called at all, and whether the
// reply ended up carrying an answer — so there are four endings, not two: the
// terminal `rag` call produced the cited answer, or it ran and produced nothing,
// while the outer model may also have answered on its own, with no research behind
// it, which is why THAT reply can legitimately carry no citations.
func outerLoopEndLine(ragCalls int, answered bool) string {
	switch {
	case ragCalls == 0 && answered:
		return "The outer model produced the answer without running research."
	case ragCalls == 0:
		return "The outer model returned no answer and ran no research."
	case !answered:
		return "The rag tool ran but produced no answer."
	default:
		return "The rag tool produced the final answer, done."
	}
}

// outerReactSession implements models.ToolCallSession for the outer react loop.
type outerReactSession struct {
	ctx    context.Context
	deps   RAGTools
	req    runtime.RunRequest
	sd     runtime.SearchDeps
	kb     *runtime.Kbinfos
	resp   *RunResponse
	logger *log.Logger
	spec   runtime.ModeSpec
	// mux is set only on the streaming path; it merges the inner run's output
	// into the caller's sink.
	mux *outerStreamMux
	// mu guards the per-call result merge (publish) and the recorded per-call
	// outcomes (calls): appendToolResults invokes this session's tool calls
	// concurrently.
	mu sync.Mutex
	// calls records each rag call's answer + evidence pool in completion order so
	// selectEvidence can match the terminal fold's winning answer back to its own
	// pool. Guarded by mu.
	calls []ragCallResult

	// flightMu guards inflight, the single-flight registry for CONCURRENT
	// IDENTICAL rag calls.
	flightMu sync.Mutex
	// inflight maps the rag question argument to its in-progress flight.
	// Guarded by flightMu.
	inflight map[string]*ragFlight
}

// ragFlight is ONE in-progress rag execution shared by every concurrent identical tool
// call — Go-only robustness, since the outer model sometimes emits the SAME rag call 2-3
// times in one round (observed with MiniMax-M3 in frame_benchmark 20260912_091520 — two
// graph runs started at the same millisecond, three planner runs for one question). The
// tool layer (models.appendToolResults) executes them all concurrently while the terminal
// fold keeps only the FIRST result. The duplicates each burn a full graph run
// (double provider load → the SCA deadline overruns in the same log) and
// their — sometimes better — answers are discarded. The first caller executes
// and publishes; the rest block on done and return the same answer.
type ragFlight struct {
	done   chan struct{}
	answer string
}

// beginRagFlight registers the caller as the OWNER of question's execution, or returns
// the existing flight to wait on (owner=nil, wait!=nil). The key is the literal question
// argument; near-identical (but not identical) re-asks stay on the RAGCache similarity
// path (Rag's Lookup).
func (s *outerReactSession) beginRagFlight(question string) (owner, wait *ragFlight) {
	s.flightMu.Lock()
	defer s.flightMu.Unlock()
	if existing, ok := s.inflight[question]; ok {
		return nil, existing
	}
	flight := &ragFlight{done: make(chan struct{})}
	if s.inflight == nil {
		s.inflight = make(map[string]*ragFlight)
	}
	s.inflight[question] = flight
	return flight, nil
}

// endRagFlight removes the flight and releases its waiters. The answer MUST be
// stored on the flight before endRagFlight runs: closing done happens-before
// every waiter's receive, so their subsequent read of answer sees the write
// (Go memory model, channel close).
func (s *outerReactSession) endRagFlight(question string, flight *ragFlight) {
	s.flightMu.Lock()
	delete(s.inflight, question)
	s.flightMu.Unlock()
	close(flight.done)
}

// ToolCall routes the two outer tools to their Go implementations.
//
// A round's tool calls arrive CONCURRENTLY — models.appendToolResults runs one goroutine
// per call. Each rag call therefore runs on
// its own request/evidence/response and publishes the outcome under mu; sharing
// the session's would let two calls mix questions, evidence and answers.
func (s *outerReactSession) ToolCall(name string, arguments map[string]interface{}) (string, error) {
	// Mirror Python's FunctionToolSession (rag/llm/tool_decorator.py:311), which
	// logs "[Function tool] Running the {name} tool with: {args}" for the OUTER
	// tools too — the inner retrieval tools log the same line from
	// retrieval.searchExecutor.Execute. Without it the outer loop's only trace is
	// chat_pipeline's synthetic "[Tool loop] Step N: running rag…", which never
	// shows the arguments the outer model actually chose.
	renderedArgs := runtime.RenderToolArgs(arguments)
	// Same split as the inner executor: the log and the event's Args keep the
	// arguments verbatim, the step reports the query (runtime.ToolCallLine).
	if s.logger != nil && name != "" {
		s.logger.Printf("[Function tool] Running the %s tool with: %s", name, renderedArgs)
	}
	if name != "" {
		runtime.StepsFrom(s.ctx).Emit(runtime.ThinkEvent{
			Kind:    runtime.ThinkKindToolCall,
			Stage:   "Function tool",
			Tool:    name,
			Args:    renderedArgs,
			Summary: "[Function tool] " + runtime.ToolCallLine(name, arguments),
		})
	}
	started := time.Now()
	switch name {
	case "rag":
		// Work on a COPY of the request: this call must not rewrite the
		// session's question/images, which a concurrent call is reading.
		req := s.req
		if q, ok := arguments["question"].(string); ok && q != "" {
			req.Question = q
		}
		// Prefer the user's ORIGINAL, complete
		// question over the outer model's rewritten `question` argument when
		// both clearly describe the same turn. The outer rewrite often drops
		// the final target of a multi-hop question, and no later stage can
		// recover a deleted answer-attribute. The defense line sits inside the `rag` tool
		// itself, i.e. per outer tool call.
		if effective := resolveEffectiveQuestion(req.Question, s.deps.OriginalQuestion); effective != req.Question {
			s.logger.Printf("[Agentic RAG] using original user question over outer rewrite (original=%q → rewrite=%q)",
				trunc(s.deps.OriginalQuestion, 80), trunc(req.Question, 80))
			req.Question = effective
		}
		// The inner compose is text-only: the outer model already saw the images via
		// multimodal history, and the rephrased question carries no images. Drop the original
		// images so the inner compose model (ComposeAnswerWith via composeFinalAnswer) does
		// not receive them.
		req.Images = nil

		// Single-flight (see ragFlight): the FIRST caller owns the execution;
		// identical concurrent calls block until it finishes and return the
		// same answer instead of re-running the whole graph.
		flight, wait := s.beginRagFlight(req.Question)
		if wait != nil {
			<-wait.done
			// A waiting duplicate still closes its own call/result pair, so a
			// client rendering steps never sees a dangling tool_call.
			s.narrateToolStep(runtime.ThinkEvent{
				Tool:       name,
				Args:       renderedArgs,
				Status:     runtime.StatusOK,
				DurationMS: time.Since(started).Milliseconds(),
			}, fmt.Sprintf("The rag tool reused the answer of an identical concurrent call%s.", runtime.ArgsLabel(arguments)))
			return wait.answer, nil
		}
		defer s.endRagFlight(req.Question, flight)

		// Per-call evidence, response and the projections that point at them: every rag()
		// invocation gets its own graph state and shares only the tools object.
		kb := &runtime.Kbinfos{}
		resp := &RunResponse{Mode: s.spec, Kbinfos: kb}
		sd := s.sd
		sd.KB = kb
		inner := s.deps
		inner.KB = kb

		// Composition happens INSIDE the graph, from the formalize_answer node's state:
		// partial_answer from the node, empty_result always true there, and question =
		// state["question"], the FORMALIZED multi-hop question. Wire the per-call Finalize
		// so the graph's last node composes itself; the guarded direct call below only fires
		// when the graph never reached that node.
		composed := false
		inner.Finalize = func(fctx context.Context, partial, empty bool, question string) {
			if composed {
				return
			}
			composed = true
			composeFinalAnswer(fctx, inner, req, kb, resp, s.logger, partial, empty, question)
		}

		if s.mux != nil {
			// Streaming: s.deps already routes the inner research log and the
			// composed answer into the caller's sink, so return "" — the
			// terminal short-circuit stops the loop and sendTerminal stays
			// silent, so the answer is not streamed twice.
			s.mux.markTerminal()
		} else {
			// Non-streaming: disable the sink so the answer is not emitted
			// twice, and hand this call's own answer back for the terminal
			// short-circuit.
			inner.AnswerSink = nil
		}
		// Nested: this whole research graph runs INSIDE the `rag` tool call reported
		// above, so its steps (and everything beneath them) are indented under that
		// call line.
		RunAgenticRAG(runtime.Nested(s.ctx), inner, req, sd, kb, resp, s.logger, s.spec)
		if !composed {
			composeFinalAnswer(s.ctx, inner, req, kb, resp, s.logger, resp.Partial, true, "")
		}
		// The "[Research status]" note rides on the TOOL RESULT (below), so the outer model can
		// decide whether to re-run `rag` from the reported gaps — and after two consecutive
		// unsatisfying rounds it tells that model to STOP.
		//
		// It is deliberately NOT part of resp.Answer. This branch used to append it there, and
		// this branch is the harness path: when the `rag` call is what ends the loop, the tool
		// result IS the final answer, so the runtime's bookkeeping reached the user verbatim
		// (measured 2026-09-20: "[Research status] this round did not settle the question (66
		// passages read). If these gaps are material, call rag again with a question focused on
		// them." inside an answer the reviewer then called self-contradicting).
		if t := researchStatusTrailer(s.deps.Cache, resp); t != "" {
			// Same fold as Rag's direct path: the period closes the hint
			// clause before the trailer sentence.
			resp.ResearchStatus = "\n\n[Research status] " + resp.SCAFeedback + "." + t
		}
		s.publish(resp, kb)
		// Close the "[Function tool] Running the rag tool with: …" line. The
		// result IS the cited answer — far too long to repeat in a log line — so
		// narrate what it was grounded in instead.
		s.narrateToolStep(runtime.ThinkEvent{
			Tool:       name,
			Args:       renderedArgs,
			Status:     runtime.StatusOK,
			Results:    len(kb.Chunks),
			Documents:  len(runtime.ChunkDocIDs(kb.Chunks)),
			Sources:    runtime.ChunkEvidenceIDs(kb.Chunks, runtime.ThinkMaxSources),
			DurationMS: time.Since(started).Milliseconds(),
		}, ragToolResultLine(resp, kb, runtime.ArgsLabel(arguments)))
		if s.mux != nil {
			// The answer already streamed through the shared mux; waiters
			// return "" too — sendTerminal's empty guard keeps the fold from
			// re-streaming it (markTerminal already fired on this call).
			flight.answer = ""
			return "", nil
		}
		// Waiters replay this exact answer; publish already ran once above, so
		// the evidence pool is not unioned twice and selectEvidence keeps a
		// single unambiguous call record.
		//
		// The status note rides the TOOL RESULT and not the answer (see above): it is the outer
		// model that has to act on it, while `publish` already handed the answer on.
		toolResult := resp.Answer + resp.ResearchStatus
		flight.answer = toolResult
		return toolResult, nil
	case "summarize_document":
		docID, _ := arguments["doc_id"].(string)
		if docID == "" {
			s.narrateToolStep(runtime.ThinkEvent{
				Tool: name, Args: renderedArgs, Status: runtime.StatusError, Reason: runtime.ReasonBadArgs,
				DurationMS: time.Since(started).Milliseconds(),
			}, "The summarize_document tool could not run: it was called without a doc_id.")
			return "Error: missing doc_id argument.", nil
		}
		// The document read and its prompt are budgeted at the model's FULL context window,
		// not a fixed cap. No citation header: this dialog path runs with do_refer=false, so
		// summarize_document returns bare blocks there.
		blocks := runtime.SummarizeDocument(s.ctx, s.sd, docID, s.deps.MaxLength)
		if len(blocks) == 0 {
			s.narrateToolStepDetail(runtime.ThinkEvent{
				Tool: name, Args: renderedArgs, Status: runtime.StatusEmpty, Reason: runtime.ReasonNoDoc,
				Sources:    []string{docID},
				DurationMS: time.Since(started).Milliseconds(),
			}, "The summarize_document tool found nothing readable in the requested document.",
				fmt.Sprintf("The summarize_document tool found nothing readable in document %s.", docID))
			return "The document is unavailable or has no readable chunks.", nil
		}
		// Only the summary knows the id (the blocks are the document's own text),
		// so the think copy names what was read — "the requested document" — and
		// the log copy carries the id.
		s.narrateToolStepDetail(runtime.ThinkEvent{
			Tool: name, Args: renderedArgs, Status: runtime.StatusOK,
			// The blocks are the document's own text, so "results" is the block
			// count and there is exactly one source document.
			Results:    len(blocks),
			Documents:  1,
			Sources:    []string{docID},
			DurationMS: time.Since(started).Milliseconds(),
		}, fmt.Sprintf("The summarize_document tool read %s from the requested document.",
			runtime.CountOf(len(blocks), "evidence block")),
			fmt.Sprintf("The summarize_document tool read %s from document %s.",
				runtime.CountOf(len(blocks), "evidence block"), docID))
		return strings.Join(blocks, "\n\n"), nil
	}
	// An unknown name is a tool this deployment has no binding for: nothing ran,
	// which is MISS-level (ReasonUnwired), not an infra failure. Same wording as
	// the inner executor's unwired branch so both layers read alike.
	s.narrateToolStep(runtime.ThinkEvent{
		Tool: name, Args: renderedArgs, Status: runtime.StatusMiss, Reason: runtime.ReasonUnwired,
		DurationMS: time.Since(started).Milliseconds(),
	}, fmt.Sprintf("The %s tool is not wired in this deployment, so nothing ran.", name))
	return fmt.Sprintf("unknown tool %q", name), nil
}

// narrateToolStep closes one outer "[Function tool] Running the …" line: it logs
// what the call returned (the developer log) and reports the step (think block +
// structured twin), so the trace records a whole tool step instead of only its
// invocation.
// ragCalls reports how many rag executions this session published, i.e. whether
// the outer loop actually ran research instead of the model answering on its own.
func (s *outerReactSession) ragCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *outerReactSession) narrateToolStep(ev runtime.ThinkEvent, line string) {
	s.narrateToolStepDetail(ev, line, line)
}

// narrateToolStepDetail is narrateToolStep for the steps whose two audiences need
// different words: `human` is what the think block reports, `dev` is what the
// developer log records. It mirrors the inner executor's split
// (runtime.StageLineDetail and the two labels renderToolOutcome accepts) and
// exists for the same reason: a 32-hex document id is a developer's handle and a
// reader's noise, so summarize_document names the document in words in the think
// block and by id in the log.
func (s *outerReactSession) narrateToolStepDetail(ev runtime.ThinkEvent, human, dev string) {
	if s.logger != nil {
		s.logger.Printf("[Function tool] %s", dev)
	}
	ev.Kind = runtime.ThinkKindToolResult
	ev.Stage = "Function tool"
	ev.Summary = "[Function tool] " + human
	runtime.StepsFrom(s.ctx).Emit(ev)
}

// ragToolResultLine summarises one outer `rag` call for the think block. The
// result is the cited answer itself, so the line reports what it was grounded in
// and whether research came back empty instead. `label` names the sub-question
// this call researched (runtime.ArgsLabel): the outer loop can run several `rag`
// calls concurrently, and without it two parallel calls' result lines are
// indistinguishable.
func ragToolResultLine(resp *RunResponse, kb *runtime.Kbinfos, label string) string {
	chunks := 0
	if kb != nil {
		chunks = len(kb.Chunks)
	}
	if chunks == 0 {
		return fmt.Sprintf("The rag tool returned no answer%s: research gathered no evidence.", label)
	}
	if strings.TrimSpace(resp.Answer) == "" {
		return fmt.Sprintf("The rag tool gathered %s but composed no answer%s.", runtime.CountOf(chunks, "passage"), label)
	}
	return fmt.Sprintf("The rag tool returned a cited answer grounded in %s%s.", runtime.CountOf(chunks, "passage"), label)
}

// publish merges one rag call's outcome into the session state. The inner run
// used to write that state directly, before each call got its own; the merge
// reproduces what a single call contributed, and for concurrent calls it keeps
// the FIRST non-empty answer/verdict (appendToolResults folds the first terminal
// hit in call order) while the evidence is unioned, so the caller still sees the
// chunks the answer was composed from.
func (s *outerReactSession) publish(call *RunResponse, kb *runtime.Kbinfos) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Remember this call's own answer + evidence so selectEvidence can restore
	// the pool the winning answer was composed from.
	s.calls = append(s.calls, ragCallResult{answer: call.Answer, kb: kb})
	if s.resp.Answer == "" {
		s.resp.Answer = call.Answer
	}
	if s.resp.SCAFeedback == "" {
		s.resp.SCAFeedback = call.SCAFeedback
	}
	if s.resp.CollectedAnswer == "" {
		s.resp.CollectedAnswer = call.CollectedAnswer
	}
	if len(s.resp.Slots) == 0 {
		s.resp.Slots = call.Slots
	}
	s.resp.Partial = s.resp.Partial || call.Partial
	s.kb.Chunks = append(s.kb.Chunks, kb.Chunks...)
	s.kb.DocAggs = append(s.kb.DocAggs, kb.DocAggs...)
	s.kb.Memory = append(s.kb.Memory, kb.Memory...)
	if s.kb.PreSummary == "" {
		s.kb.PreSummary = kb.PreSummary
	}
}

// ragCallResult is one rag call's outcome: the answer it composed and the
// evidence pool that produced it.
type ragCallResult struct {
	answer string
	kb     *runtime.Kbinfos
}

// selectEvidence leaves only the evidence of the rag call whose answer the
// terminal fold returned. The fold picks the LOWEST-INDEX terminal call while
// publish sees completion order, so the union alone can leave that answer's
// [ID:n] markers pointing into another call's chunks — the answer and the
// reference list would disagree. No-op when no recorded call matches (the
// terminal tool was summarize_document, or the answer did not come from a rag
// call), in which case the union stands.
//
// Matching is by exact answer text, so two calls that produce byte-identical
// answers are indistinguishable and the FIRST recorded (first to finish) one
// wins. That collision is accepted: plumbing the tool-call index through
// ToolCallSession is not worth it for a case that cannot happen with distinct
// questions.
func (s *outerReactSession) selectEvidence(answer string) {
	if answer == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.calls {
		if r.answer != answer {
			continue
		}
		s.kb.Chunks = append([]map[string]any(nil), r.kb.Chunks...)
		s.kb.DocAggs = append([]map[string]any(nil), r.kb.DocAggs...)
		s.kb.Memory = append([]map[string]any(nil), r.kb.Memory...)
		s.kb.PreSummary = r.kb.PreSummary
		// The published evidence list belongs to THIS call too: the answer's [ID:n] markers
		// were written against it, and the chat pipeline resolves them against whatever is on
		// the response. Restoring the passages without their numbering leaves marker n pointing
		// at another call's n-th passage — the citation opens a passage the answer never cited.
		s.kb.CiteChunkIDs = append([]string(nil), r.kb.CiteChunkIDs...)
		return
	}
}

// runDirect is the low/naive path: one hybrid search, no tool loop.
//
// It is the ONE implementation of the direct search (called from the low graph node) —
// there is deliberately no runtime-level duplicate in runtime/orchestrator. It lives here
// rather than there because the step reads the full RAGTools config; the graph node that
// calls it forces UseCompiled=true.
func runDirect(ctx context.Context, deps RAGTools, req runtime.RunRequest, sd runtime.SearchDeps, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger) {
	ctx, done := runtime.Phase(ctx, runtime.PhaseDirect)
	retrievalQuery := ""
	if deps.Keywords != nil {
		rq, err := deps.Keywords(ctx, req.Question)
		if err != nil {
			logger.Printf("[Agentic RAG] weighted keyword extraction failed: %v", err)
		} else {
			retrievalQuery = rq
		}
	} else if deps.Model != nil {
		// Default: the four-aspect weighted extraction (runtime/keywords.go). It gives BM25
		// both the discriminating entity and the surface variants the corpus may use.
		rq, kw := runtime.ExtractWeightedKeywords(ctx, deps.Model, req.Question)
		retrievalQuery = rq
		if req.Keywords == "" {
			req.Keywords = kw
		}
	}
	defer done()

	chunks, aggs := runtime.HybridSearch(ctx, sd, runtime.SearchParams{
		Question:       req.Question,
		Keywords:       req.Keywords,
		RetrievalQuery: retrievalQuery,
		UseCompiled:    req.UseCompiled,
		TopN:           req.TopN,
		// The only channel honouring UsingEmbedding.
		Channel: runtime.ChannelRetrieve,
	})
	kb.Merge(chunks, aggs)
	if !kb.HasChunks() {
		logger.Printf("[Agentic RAG] direct search found no matching passages")
	}
}

// AgenticLoop runs the full agentic search loop (planner / prefetch / SCA↔
// rewriter iteration) for an agentic mode.
//
// It is registered by the agentic_rag package's init rather than called directly: the
// agentic_rag package owns RAGTools, so the runtime cannot import it back. The
// indirection keeps the dependency graph acyclic. Registering is optional — see
// Run for the fallback when nothing is registered.
type AgenticLoop func(ctx context.Context, deps RAGTools, req runtime.RunRequest, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger)

var agenticLoop AgenticLoop

// SetAgenticLoop registers the outer agentic loop. Called from the agent
// package's init(); importing that package (even blank) activates the full
// medium/high/ultra pipeline. Without it those modes degrade to a single action
// session.
func SetAgenticLoop(fn AgenticLoop) { agenticLoop = fn }

// runAgentic drives medium/high/ultra.
//
// When an agentic loop is registered (see SetAgenticLoop) it runs the full
// five-phase pipeline: planner fan-out → slot table → slot research rounds →
// SCA review → gap rewrite → repeat until sufficient or the budget runs out.
//
// Fallback (no loop registered): ONE action session via RunActionSession. This
// keeps `Run` usable without the planner, at the cost of the
// planner/fan-out/SCA iteration.
func runAgentic(ctx context.Context, deps RAGTools, req runtime.RunRequest, sd runtime.SearchDeps, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger) {
	// Formalization happens inside the graph: it is the graph's entry node
	// (START → formalize_question).
	if agenticLoop != nil {
		agenticLoop(ctx, deps, req, kb, resp, logger)
		return
	}
	if deps.Model == nil {
		logger.Printf("[Agentic RAG] no model configured for mode %q; degrading to a direct search", resp.Mode.Label)
		runDirect(ctx, deps, req, sd, kb, resp, logger)
		return
	}
	runSingleSession(ctx, deps, req, sd, kb, resp, logger)
}

// runSingleSession is the fallback agentic path: seed a slot table and run ONE
// bounded action session against it.
func runSingleSession(ctx context.Context, deps RAGTools, req runtime.RunRequest, sd runtime.SearchDeps, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger) {
	// Seed the slot table and run one bounded session against it. A fresh tool
	// cache and query history per run: they exist to dedupe WITHIN a session,
	// not across requests.
	sd.WebSearch = deps.WebSearch
	sd.CiteRules = deps.CiteRules
	sessionDeps := runtime.SessionDeps{
		Tools: &runtime.Toolset{
			ThinkingMode: resp.Mode.Label,
			// Provider gate: without a wired provider the tool is hidden rather than
			// advertised dead.
			HasWebSearch: resp.Mode.HasTool("web_search") && deps.WebSearch != nil,
			// The dataset's real metadata fields (see NewAgenticLoop).
			MetadataFields: runtime.MetadataCatalogPtr(ctx, sd),
			DisabledTools:  map[string]bool{},
			Exec:           runtime.NewSearchExecutor(sd, req),
		},
		Model:   deps.Model,
		Prompts: deps.Prompts,
		// Surface already-retrieved evidence into the action session seed so the
		// ReAct loop fills slots from what it has instead of re-searching.
		KB: kb,
	}

	// run_agentic_rag — there is no whole-graph wall clock: research stays
	// bounded by per-node timeouts, the routing guards and the recursion limit.
	// The only budget is the one AgenticState carries (formalize_question), and it is
	// set when the state is created, so formalization is not charged to it.
	init := runtime.InitializeState(ctx, sessionDeps, req.Question, nil, req.DeadlineLeft)
	root := init.Root

	result := runtime.RunActionSession(ctx, sessionDeps, req.Question, root, req.DeadlineLeft, "", nil, nil)
	// run_agentic_rag — failure to run research is recorded separately from
	// "research found nothing": the session yielded neither messages nor states,
	// so nothing was produced at all.
	if len(result.Messages) == 0 && len(result.NewStates) == 0 && result.FoundAnswer == nil {
		resp.GraphFailed = true
		logger.Printf("[Agentic RAG] graph execution produced nothing (mode=%q)", resp.Mode.Label)
	}

	if result.FoundAnswer != nil {
		resp.Answer = *result.FoundAnswer
	}
	// Report the deepest slot table the session produced: later states carry
	// strictly more filled slots than the root.
	if len(result.NewStates) > 0 {
		best := result.NewStates[len(result.NewStates)-1]
		resp.Slots = append(resp.Slots, best.State...)
	} else {
		resp.Slots = append(resp.Slots, root.State...)
	}
}

// IsKnownMode reports whether label names a configured thinking mode.
func IsKnownMode(label string) bool {
	_, ok := runtime.THINKING_MODES[strings.ToLower(strings.TrimSpace(label))]
	return ok
}

// DocIDLookup
// it resolves which of a candidate document id set exist within the given
// datasets.
//
// It reads the document table directly; the runtime package takes it through the
// runtime.DocIDVerifier interface so the retrieval path stays free of a database
// dependency.
type DocIDLookup struct {
	docs *dao.DocumentDAO
}

// NewDocIDLookup returns the document-ownership check backed by the document
// table.
func NewDocIDLookup() *DocIDLookup {
	return &DocIDLookup{docs: &dao.DocumentDAO{}}
}

// KnownDocIDs implements runtime.DocIDVerifier: it returns the subset of
// candidates that belong to datasetIDs.
func (l *DocIDLookup) KnownDocIDs(ctx context.Context, datasetIDs, candidates []string) (map[string]bool, error) {
	if l == nil || l.docs == nil || len(candidates) == 0 || len(datasetIDs) == 0 {
		return nil, nil
	}
	docs, err := l.docs.GetByIDs(ctx, dao.DB, candidates)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]bool, len(datasetIDs))
	for _, id := range datasetIDs {
		allowed[id] = true
	}
	known := make(map[string]bool, len(docs))
	for _, d := range docs {
		if d != nil && allowed[d.KbID] {
			known[d.ID] = true
		}
	}
	return known, nil
}

// compile-time check: the lookup satisfies the runtime seam.
var _ runtime.DocIDVerifier = (*DocIDLookup)(nil)
