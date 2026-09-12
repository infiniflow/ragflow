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

// Package advanced_rag is the outer agentic-search loop (medium / high / ultra).
//
// This file mirrors Python rag/advanced_rag/agentic_rag.py — the RAGTools
// methods: the run configuration (RAGTools), the rag entry point (Rag)
// and question formalization (Formalize). In Python, RAGTools is the shared
// capability carrier that the agentic graph (agentic_rag_graph.py) and the
// harness action session both read from and write to. In Go the equivalent
// carrier is harness.Toolset (see harness/action_session.go), while this file
// holds the RAGTools-side logic.
//
// The graph driver itself lives in agentic_rag_graph.go (mirroring
// agentic_rag_graph.py), so this package replicates the Python layout:
//   - agentic_rag.go      ↔ Python agentic_rag.py            (RAGTools methods)
//   - agentic_rag_graph.go ↔ Python agentic_rag_graph.py      (the pipeline)
//   - harness/            ↔ Python harness/                   (primitives)
package advanced_rag

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/prompts"
	"ragflow/internal/service/nlp"
)

// RAGTools method map — where each Python method lives in Go.
//
// Python's RAGTools is one object exposing every capability. Go keeps the same
// shape: the RAGTools methods live in this package (agentic_rag_graph.go and
// this file), while harness/ stays the
// leaf capability library they orchestrate (retrieval, sessions, tools). Nothing
// below is re-implemented as a forwarding shim — the Go column IS the
// implementation:
//
//	__init__                221 → RAGTools (below)
//	has_*                   340 → harness.Toolset            (action_session.go)
//	_fit_messages           360 → chat.FitMessages           (chat/message_fit.go)
//	get_citation_guidelines 367 → GetCitationGuidelines      (below)
//	sys_prompt              371 → SysPrompt                  (below)
//	formalize               412 → Formalize                  (below)
//	retrieve                593 → HybridSearch               (harness/tools/search.go)
//	_extract_keywords_weighted 571 → ExtractWeightedKeywords (harness/keywords.go)
//	extract_keywords        581 → ExtractWeightedKeywords (harness/keywords.go) + CompactKeywords (harness/tool_text_processing.go)
//	web_retrieve            676 → searchExecutor.webSearch   (harness/tool_executor.go)
//	_compose_answer_from_evidence 604 → ComposeAnswer        (agentic_rag_graph.go)
//	_naive_rag                   1517 → ComposeNaiveAnswer   (agentic_rag_graph.go)
//	_fit_evidence           716 → FitEvidence                (below)
//	judge_sufficiency       734 → SCA review                 (harness/orchestrator/sufficient_context.go)
//	gen_followups           750 → SCA gap rewrite            (harness/orchestrator/sufficient_context.go)
//	rag                     817 → Rag                       (below)
//
// Also mirrored, outside this file:
//
//	scoped_doc_ids           352 → toolDocScope          (harness/tool_executor.go)
//	_filter_known_doc_ids    981 → DocIDLookup           (below)
//	_resolve_doc_tenant      987 → docInDatasets         (harness/search.go)
//	fetch_full_document      761 → fetchFullDocument     (harness/doc_fetch.go)
//	summarize_document       934 → summarizeDocument     (harness/doc_fetch.go)
//
// All live Python methods now have a Go counterpart, including
// harness.LLMUsageStats (Python harness/stats.py::LLMUsageStats) — Python counts
// usage through a ContextVar and a wrapper chat model, so Go instead passes an
// explicit *harness.LLMUsageStats collector (RAGTools.Stats) to the calls it
// makes; the per-round metrics Python derives from RAGTools.rounds are not
// tracked.
//
// Deliberately not ported — dead code in Python: pick_documents returns None on
// its first statement (agentic_rag.py:500) and is never called, so
// _select_by_titles, _filter_by_metadata, _get_cached_metas and
// _collect_doc_titles are unreachable too; structured_retrieve has neither a
// @tool decorator nor a caller.
//
// RAGTools configures one agentic-search run. It is the single Go mirror of
// Python RAGTools.__init__: every capability the run needs (retriever, model,
// prompts, embedder, kb accumulation, retrieval tuning, answer composition) is
// carried on this one object — there is no second dependency struct. The
// harness layer keeps narrow per-call views (harness.SearchDeps /
// harness.SessionDeps) that are projected from a RAGTools at call time, mirroring
// how Python methods receive self plus extra per-call arguments. Construct it directly
// (RAGTools{...}); its zero value is a valid starting point for the narrower call sites
// that only set a subset of fields.
type RAGTools struct {
	// Tools is the shared capability carrier (Python RAGTools instance). The
	// graph and the action session both reach retrieval through it.
	Tools *harness.Toolset
	// Search is the fully-projected retrieval configuration for this run. The
	// graph's DUAL-CHANNEL fan-out needs it directly: unlike the action session
	// (which goes through Tools.Exec's single "search_chunks" tool), fan-out
	// issues a keyword BM25 leg and a dense semantic leg under different
	// settings, which the tool interface cannot express.
	Search harness.SearchDeps
	// Retriever is the retrieval backend. When nil, the runtime singleton
	// (runtime.GetRetrievalService) is used.
	Retriever harness.Retriever
	// Model drives the LLM turns. Required for agentic modes; low/naive never
	// call it.
	Model harness.SessionModel
	// ModelName is the RESOLVED chat model identity (Python chat_mdl.llm_name,
	// e.g. "gpt-4o"), one component of the gen_json reply-cache key. Empty
	// disables that cache: keying on an empty name would collapse every model
	// onto one bucket and serve a reply produced by another model.
	ModelName string
	// Embedder is the external embedding handle used by graph/structure seed
	// encoding (Python tools.embed_mdl). Nil falls back to this package's
	// internal tenant-default resolver, which additionally degrades to keyword
	// matching when the DB is uninitialised or encoding panics.
	Embedder nlp.NavEmbedder
	// Keywords extracts the entity-weighted retrieval query. Optional.
	Keywords KeywordExtractorFn
	// Prompts are the report/SCA/rewrite prompt templates (user_defined_prompts).
	Prompts harness.PromptLoader
	// Expand runs compiled-structure expansion. Optional.
	Expand harness.CompiledExpander
	// SCAPrompts overrides Prompts for the sufficient-context review.
	SCAPrompts harness.PromptLoader
	// RewritePrompts overrides Prompts for the gap→query rewrite call.
	RewritePrompts harness.PromptLoader
	// KB is the in-flight retrieval accumulation (Python RAGTools.kbinfos).
	KB *harness.Kbinfos
	// Logger is optional; nil uses the default logger.
	Logger *log.Logger
	// Answer-composition configuration (see AnswerDeps). All optional; the
	// defaults match Python's configured behaviour.
	// CiteRules overrides the default citation rules.
	CiteRules string
	// SystemPrompt is the dialog-level UI configuration appended after the
	// agentic contract (language/tone/style may override; evidence contract may not).
	SystemPrompt string
	// EmptyResponse is returned verbatim when no evidence was found, skipping
	// the composition call entirely (Python tools.empty_response).
	EmptyResponse string
	// EvidenceMaxTokens caps the evidence block. <=0 uses evidenceBudgetTokens.
	EvidenceMaxTokens int
	// MaxLength is the chat model's context window (Python
	// tools.chat_mdl.max_length). It bounds message_fit_in for prompt
	// assembly; <=0 falls back to chat.EffectiveContextLength's 8192 default.
	MaxLength int
	// ComposeAnswer enables the terminal composition node. Defaults to true
	// when a model is configured; set false to receive raw evidence only.
	ComposeAnswer *bool
	// Finalize runs the terminal composition (Python _compose_answer_from_evidence).
	// It is wired by Rag so the graph can invoke it FROM INSIDE its last node —
	// Python's agentic and low graphs both end in a formalize_answer node that
	// composes and streams the answer. Nil means "the caller
	// composes afterwards", which is only correct for the naive path.
	Finalize func(ctx context.Context)
	// Messages is the conversation history used by Formalize to resolve
	// pronouns/ellipses into a standalone question. The agentic and low graphs
	// formalize it as their first node (Python build_agentic_graph and
	// build_low_graph); naive retrieval does not formalize.
	Messages []schema.Message
	// WebSearch is the optional open-web provider backing the `web_search` tool
	// (Python RAGTools.web_search). Nil HIDES the tool from the mode's surface —
	// sessionDeps sets HasWebSearch = mode.HasTool("web_search") && WebSearch != nil
	// and ActiveToolSpecs drops the spec — so the model is never shown a dead tool.
	// Should a call reach the executor anyway (stale spec, direct invocation), it
	// answers StatusError/ReasonInfra with a do-not-retry note, NOT a query-level
	// MISS: WebSearchTool returns the same infra error Python's _exec_web_search
	// does for an absent provider (tool_search.go:826).
	WebSearch harness.WebSearcher
	// DocIDVerifier checks which requested document ids really belong to the
	// datasets being searched (Python RAGTools._filter_known_doc_ids). Nil
	// disables the check and leaves the requested scope untouched.
	DocIDVerifier harness.DocIDVerifier
	// DocChunks pages through a single document's chunks in reading order
	// (Python settings.retriever.chunk_list, sort_by_position=True). It backs
	// the whole-document-reading path (summarize_document / fetch_full_document)
	// mirroring Python's chunk-list reads. Nil disables whole-document reading:
	// the document-level tools report that it is unavailable.
	DocChunks harness.DocChunkLister
	// Outer is the outer-layer chat model that drives the Python-style
	// rag_agent react loop (Python RAGTools.chat_mdl, used by
	// dialog_service.rag_agent via chat_mdl.async_chat_with_tools with
	// tools=[rag, summarize_document] and terminal_tools={"rag"}). When non-nil,
	// Rag runs that outer loop: the model may call `rag` (terminal — runs the
	// inner agentic graph and returns the cited answer) or `summarize_document`
	// (non-terminal — reads a whole document into evidence and lets the model
	// answer from it on the next round); with no tool call it answers directly.
	// When nil, Rag falls back to the existing direct RunAgenticRAG path, so
	// callers that don't wire an outer model see unchanged behaviour.
	Outer *models.ChatModel
	// OuterSupportsTools mirrors Python dialog_service.rag_agent's
	// `if not getattr(chat_mdl, "is_tools", False)` gate: when the outer model
	// exists but cannot emit tool calls, the outer react loop must be skipped
	// (it would otherwise bind tools and, producing no tool_call, return a
	// retrieval-less direct answer). Callers set it from
	// ModelProviderService.ResolveModelToolSupport.
	OuterSupportsTools bool
	// Cache answers near-identical re-asks from an earlier answer (Python
	// RAGTools._rag_cache). Rag builds a fresh per-turn RAGCache when this is
	// nil, so caching is ON by default (matching Python's always-present
	// _rag_cache). To share one cache across multiple Rag() calls within a
	// single turn (Python's concurrent tool_call case), pass the same *RAGCache
	// here instead of relying on the auto-built one.
	Cache *RAGCache
	// OriginalQuestion is the user's own, unrewritten question (Python
	// tools.original_user_question). When the outer caller passes a compressed
	// rewrite as Question, the original is preferred if both clearly describe
	// the same turn.
	OriginalQuestion string
	// TextAttachments is appended to the question and bypasses the re-ask cache
	// (Python tools.text_attachments_content).
	TextAttachments string
	// AnswerSink receives the answer as it is produced (Python
	// tools.answer_sink). Nil disables streaming; the answer is still returned
	// in full. Requires a model implementing harness.StreamingSessionModel,
	// otherwise the run falls back to one-shot composition.
	AnswerSink *AnswerSink
	// ToolStarted is called once research begins, before any retrieval, so the
	// caller can show progress (Python tools.tool_started_sink).
	ToolStarted func()
	// Progress receives engine-stage progress lines tagged like Python's
	// think-log entries (e.g. "[Planner] Splitting the research question into
	// tasks", "[Hybrid search] Searching for answers in the knowledge base") as
	// the agentic loop runs. It is the Go counterpart of Python's
	// rag/advanced_rag/think_log.py — those tagged lines used to be fished out of
	// the log stream and forwarded into the <think> block of the chat UI. Nil
	// disables streaming progress; the pipeline still completes with the answer.
	// A caller that also sets AnswerSink typically routes each line as a
	// isThink=true delta so the reasoning block shows live research progress.
	Progress func(line string)
	// Retrieval tuning, mirroring Python RAGTools.retrieve: an explicit tool
	// argument still wins over these, and zero selects the harness defaults.
	TopN                int
	SimilarityThreshold float64
	// VectorSimilarityWeight is the vector leg's weight (Python
	// tools.vector_similarity_weight). A pointer so an explicit 0 — keyword-only,
	// which is what Python's agentic retrieve uses — is distinguishable from
	// "not configured". Nil selects DefaultAgenticVectorWeight (0).
	VectorSimilarityWeight *float64
	// UsingEmbedding is the Go spelling of Python RAGTools.retrieve's
	// `using_embedding: bool = False` (agentic_rag.py:599). When false (the
	// agentic default) retrieval is keyword-only (vector weight 0); when true
	// the embedder is engaged and VectorSimilarityWeight (default 0.7) applies.
	//
	// SCOPE: only the RAGTools.retrieve channel honours it — Python's
	// using_embedding lives on that function alone.
	UsingEmbedding bool
	// HasEmbedder is the Go spelling of Python `tools.embed_mdl` being set
	// (dialog_service.py:2075). It gates hybrid_search's vector leg
	// (search.py:143 `vector_weight = _setting(...) if embd_mdl else 0`), i.e.
	// the semantic recall of the search_chunks tool.
	HasEmbedder           bool
	RerankCandidatesCount int
	TopK                  int
	// DocScope is the session-wide document restriction (Python
	// tools.doc_scope). Nil means "search everything".
	DocScope []string
	// MetaDataFilter restricts retrieval by chunk metadata (Python
	// tools.meta_data_filter).
	MetaDataFilter map[string]any
	// KBs mirrors Python RAGTools' self.kbs (agentic_rag.py:283) — the full
	// Knowledgebase objects (carrying parser_config / tenant_id) the agentic
	// tools run over. Python loads these via
	// KnowledgebaseService.get_by_ids(kb_ids) in __init__; Go receives the
	// already-resolved objects from the caller (this package stays DB-free) and
	// feeds them to the Tagger, mirroring retrieve's
	// `rank_feature=label_question(question, self.kbs)`.
	KBs []*entity.Knowledgebase
	// Tagger mirrors Python RAGTools.retrieve's
	// `rank_feature=label_question(question, self.kbs)` (agentic_rag.py:668): it
	// classifies the query into question-type tags the retriever uses to boost
	// matching chunks. It is the Go equivalent of rag.app.tag.label_question,
	// implemented by internal/service.MetadataService.LabelQuestion. Nil means
	// no tag boost (Python's label_question returning None).
	Tagger harness.QuestionLabeler
	// Stats receives per-phase LLM usage for this run (Python
	// RAGTools.llm_stats, mirrored by harness.LLMUsageStats). Nil disables
	// collection.
	Stats *harness.LLMUsageStats
}

// sessionDeps projects RAGTools onto the harness.SessionDeps the action session
// and slot-table builders consume.
//
// Every field is forwarded unconditionally. In particular a nil Prompts is NOT
// a reason to drop Tools/KB: the harness resolves a nil loader to its embedded
// canonical templates (harness.resolveLoader / prompts.EmbeddedPromptLoader), so
// the action session still runs with its toolset and the shared evidence pool.
// Returning a Model-only projection here silently turned the slot research pass
// into a tool-less question-answering turn.
func (d RAGTools) sessionDeps() harness.SessionDeps {
	return harness.SessionDeps{
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
// Mirrors Python agentic_rag.RAGTools.formalize (line 412): rewrite the latest
// user message into a standalone question AND derive its search keywords, in one
// LLM call.
//
// Single-turn shortcut: when there is nothing to resolve, the question is kept
// VERBATIM (no rewrite) and only keywords are extracted. Rewriting a
// self-contained question risks silently changing its meaning — and the
// single-turn case is the overwhelming majority.
//
// The keyword/JSON extraction helpers (ExtractWeightedKeywords, ExtractJSON)
// stay in harness as the leaf capability library.

const (
	// formalizeTimeoutS bounds the formalize call.
	formalizeTimeoutS = 45.0
	// formalizeTemperature mirrors Python's chat conf at agentic_rag.py:471:
	// formalization is a mechanical rewrite, so it must be stable.
	formalizeTemperature = 0.1
)

var reFormalizeThink = regexp.MustCompile(`(?s)^.*</think>`)

var formalizePrompt = (`You are given a conversation. Do BOTH of the following and return JSON only:
1. Rewrite the LAST user message into a single, self-contained question that can be understood without the prior conversation — resolve pronouns, ellipses and follow-up shortcuts using the earlier turns. In most cases, it should be EXACTLY THE SAME as the last user query — only rewrite when there is something to resolve (a pronoun/ellipsis pointing back at an earlier turn). Preserve the original language.
2. Extract keywords for a keyword search: the salient content words and phrases that literally appear in the (standalone) question — key nouns, named entities, domain terms — PLUS 2-3 close synonyms/abbreviations/aliases/alternative spellings of each, in the SAME language as the question. Maximize recall. Do NOT include terms that would be part of the answer.
   Example — "In which year did Apple acquire Beats?" -> keywords = "Apple, Apple Inc., AAPL, acquire, acquisition, acquired, Beats, Beats Electronics".

Output ONLY JSON, no prose, no code fences: {"question": "<standalone question>", "keywords": "<term1, term2, synonym1, ...>"}`)

// Formalize mirrors Python RAGTools.formalize: return (question, keywords) for
// the given conversation.
//
// messages may be []schema.Message (preferred) or pre-formatted "Speaker: text"
// strings. On any failure it degrades to (last user message, "") — formalization
// is an optimization, never a precondition for answering.
//
// maxLength is the chat model's context window (Python tools.chat_mdl.max_length);
// it bounds the prompt fit. <=0 falls back to chat.EffectiveContextLength's 8192.
func Formalize(ctx context.Context, deps harness.SessionDeps, messages []schema.Message, maxLength int) (string, string) {
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
	// self-contained question risks silently changing its meaning), and extract
	// only the search keywords (Python: tools.extract_keywords).
	if !isMultiTurn(messages) {
		_LOG.Printf("[Formalize] Single-turn self-contained question — kept verbatim (no rewrite): %s", trunc(lastUser, 120))
		_, kw := harness.ExtractWeightedKeywords(ctx, deps.Model, lastUser)
		return lastUser, kw
	}

	ctx, done := harness.Phase(ctx, harness.PhaseFormalize)
	// Phase returns a child context carrying the phase label; the timeout is
	// derived from it so the call is both attributed and bounded.
	defer done()

	callCtx, cancel := context.WithTimeout(ctx, deadlineToDuration(formalizeTimeoutS))
	defer cancel()

	// Python 470: message_fit_in(form_message(system, user), chat_mdl.max_length).
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

	// Python 471: async_chat(system, history, {"temperature": 0.1}).
	reply, err := modelWithTemperature(deps.Model, formalizeTemperature).Complete(callCtx, msgs, nil)
	if err != nil {
		_LOG.Printf("[Formalize] failed; keeping the raw question: %v", err)
		return lastUser, ""
	}

	data, _ := harness.ExtractJSON(stripThinkAndFences(reply.Content)).(map[string]any)
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
	return question, harness.CompactKeywords(keywords)
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
// Mirrors Python's `multi_turn = len(user_msgs) > 1`.
func isMultiTurn(messages []schema.Message) bool {
	n := 0
	for _, m := range messages {
		if m.Role == schema.User {
			n++
		}
	}
	return n > 1
}

// stripThinkAndFences mirrors Python agentic_rag.py:474-475: drop a leading
// thinking preamble, then strip Markdown fences. Python's fence regex removes
// the delimiters wherever they appear, so a truncated or inline fence leaves no
// residue either.
func stripThinkAndFences(s string) string {
	s = reFormalizeThink.ReplaceAllString(s, "")
	s = reFenceDelimiters.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// reFenceDelimiters mirrors Python's `re.sub(r"```(?:json)?\s*|\s*```", "", s)`.
var reFenceDelimiters = regexp.MustCompile("```(?:json)?\\s*|\\s*```")

// End-to-end entry point: one call that runs the harness and publishes the
// evidence it collected.
//
// Rag mirrors Python RAGTools.rag (agentic_rag.py:817) — the RAGTools method
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
// the advanced_rag package's agentic planner (Rag's agentic-loop registration).
// So:
//
//   - low / naive: fully wired here (direct search through HybridSearch).
//   - medium/high/ultra: Rag delegates to the registered AgenticLoop, which the
//     advanced_rag package registers (see SetAgenticLoop). Without it, Rag falls
//     back to a single action session.
//
// The harness package keeps the low-level retrieval / session / tool machinery
// (RunActionSession, HybridSearch, Toolset, Kbinfos, searchExecutor, ...).

// RunRequest lives in the harness package (see harness/tool_executor.go) because the
// harness engine's searchExecutor needs it to carry the per-run query, and the
// advanced_rag package must not be imported from harness (import-cycle rule).
// RunResponse is the agent-side result type returned by RAGTools.rag.

// RunResponse is the outcome of one harness run.
type RunResponse struct {
	// Answer is the session's terminal answer, when it produced one.
	Answer string
	// Slots are the filled slot-table values discovered by the session.
	Slots []harness.Variable
	// Chunks is the evidence accumulated in Kbinfos (narrowed, as the model saw
	// it). Citations are built from this.
	Chunks []map[string]any
	// DocAggs is the per-document aggregation of Chunks.
	DocAggs []map[string]any
	// EmptyResult is true when nothing was retrieved, which the caller turns
	// into an "I don't have enough information" answer (Python direct_search's
	// empty_result).
	EmptyResult bool
	// Mode is the resolved spec, for logging.
	Mode harness.ModeSpec
	// Partial is true when research ended without a satisfying verdict, so the
	// caller surfaces the residual findings honestly instead of refusing.
	Partial bool
	// SearchRounds is the number of completed SCA→rewrite iterations (0 for the
	// non-agentic paths).
	SearchRounds int
	// GraphFailed is true when the research graph itself errored. Python pairs it
	// with "produced nothing" before falling back to an internal-error message
	// (run_agentic_rag); an empty result on its own is not a failure.
	GraphFailed bool
	// Verdict is the final sufficiency verdict ("SUFFICIENT"/"INSUFFICIENT").
	Verdict string
	// SCAFeedback is the body of the SCA feedback note — agentic_rag.py:902-929's
	// string, which is the sufficiency status hint alone (the verdict dict carries
	// only "status"; see scaFeedback). Rag() appends it as the "[Research status]"
	// note for every INSUFFICIENT verdict, adding the trailing "STOP" vs "call rag
	// again" sentence based on the consecutive-unanswerable count.
	SCAFeedback string
	// CollectedAnswer is the research draft (SCA-reviewed) produced by the
	// agentic loop. It feeds the final composition; prefer Answer for display.
	CollectedAnswer string
	// Kbinfos carries the full accumulated state (including the lossless
	// memory store) for callers that need more than the summary above.
	Kbinfos *harness.Kbinfos
}

// AnswerSink forwards a partially produced answer while the model is still
// writing it (Python tools.answer_sink). Nil disables streaming; the answer is
// still returned in full.
//
// Requires a model implementing harness.StreamingSessionModel; otherwise the run
// falls back to a single completion.
type AnswerSink struct {
	// OnDelta receives each successive piece of the answer. isThink marks pieces
	// of a hidden reasoning block, which must not be shown as part of the answer
	// (Python answer_sink(delta, kind == "think")).
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

// Run executes one harness request end to end and publishes the evidence into
// the canvas state (when the context carries one) so citation grounding can
// read it.
//
// It never fails for a recoverable reason: a missing component degrades to
// "no evidence" rather than erroring, matching Python's behaviour of logging
// and returning an empty kbinfos.
// ragCacheMinOverlap is the word-overlap ratio at which a new question counts
// as a re-ask of a cached one (Python _RAG_CACHE_MIN_OVERLAP).
const ragCacheMinOverlap = 0.6

// ragCacheMinShared is the minimum number of shared significant words before a
// cached answer may be reused (Python _RAG_CACHE_MIN_SHARED).
const ragCacheMinShared = 2

// effectiveQuestionMinShared is the overlap _resolve_effective_question needs
// before trusting the original question over the outer rewrite. Python uses the
// literal 2 here, independent of _RAG_CACHE_MIN_SHARED.
const effectiveQuestionMinShared = 2

// ragCacheStopwords is Python _RAG_CACHE_STOPWORDS: for cross-`rag`-call dedup
// only, never for retrieval or answer quality.
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

// reQuestionTokens mirrors Python's `re.findall(r"[a-zA-Z0-9一-鿿]+", ...)`.
var reQuestionTokens = regexp.MustCompile(`[a-zA-Z0-9\x{4e00}-\x{9fff}]+`)

// questionGram is Python _question_keywords' return value: the significant
// words plus the numeric tokens kept apart, so questions naming different
// numbers are never treated as the same question.
type questionGram struct {
	words   map[string]bool
	numbers map[string]bool
}

// questionKeywords mirrors Python _question_keywords. For English, plain
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
		// Python falls back to every non-numeric token, so an all-stopword
		// question still has something to compare.
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

// cacheSimilar mirrors Python _cache_similar: significant-word overlap
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

// FitEvidence mirrors Python RAGTools._fit_evidence (agentic_rag.py:716): trim
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

// routerPromptBody is Python's router_prompt (agentic_rag.py:383-403) with the
// summarize_document line left as %s: that line is only included when
// unstructured retrieval is available (Python has_unstructured).
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

// routerSummarizeLine is the conditional summarize_document instruction
// (Python agentic_rag.py:378-381).
const routerSummarizeLine = "- Call `summarize_document` ONLY when the user explicitly asks to summarise a specific document ('summarise the security audit', 'tldr the onboarding guide'). It needs a document ID.\n"

// SysPrompt mirrors Python RAGTools.sys_prompt (agentic_rag.py:371): the thin
// router prompt for callers that bind the tool set. The workflow itself lives in
// the rag graph; the outer model only chooses between retrieval and an explicit
// single-document summary.
//
// hasUnstructured mirrors Python has_unstructured(): when false the
// summarize_document instruction is omitted. systemPrompt is the dialog-level UI
// configuration, prepended when set (Python :404-405).
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

// GetCitationGuidelines mirrors Python RAGTools.get_citation_guidelines
// (agentic_rag.py:367): the citation rules the final answer must follow, with
// an optional user-defined override.
//
// Python renders `citation_prompt(self.user_defined_prompts)`; Go loads the same
// citation_prompt.md via prompts.CitationPrompt and takes a non-empty override
// verbatim — exactly as Python returns citation_prompt(self.user_defined_prompts).
func GetCitationGuidelines(userDefined string) string {
	return prompts.CitationPrompt(userDefined)
}

// resolveEffectiveQuestion mirrors Python _resolve_effective_question: prefer
// the user's ORIGINAL, complete question over the outer model's rewrite, but
// only when both clearly describe the same user turn. The outer rewrite often
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

// RAGCache mirrors Python RAGTools._rag_cache (agentic_rag.py:837-889): it
// answers a near-identical re-ask from a previous answer instead of re-running
// the whole graph.
//
// Lifetime: Python keeps _rag_cache on the RAGTools instance, which is rebuilt
// for every dialog turn, so caching is per-turn by construction and is ON by
// default. Go mirrors that default-on, per-turn behavior: Rag builds a fresh
// RAGCache when deps.Cache is nil, so caching is never off, and the auto-built
// cache lives only for that call (the per-turn degenerate case under the
// current single-Rag()-per-turn path), so it can never serve a stale cross-turn
// hit. A caller that wants to share one cache across multiple Rag() calls within
// a single turn (Python's concurrent tool_call case) injects the same *RAGCache
// via RAGTools.Cache instead of relying on the auto-built one. This matches
// Python, where _rag_cache lives on the per-turn RAGTools instance.
type RAGCache struct {
	mu          sync.Mutex
	entries     map[string]ragCacheEntry
	lastVerdict string
	// consecutiveUnanswerable mirrors Python RAGTools._consecutive_unanswerable
	// (agentic_rag.py:818): how many consecutive rag() calls ended without a
	// satisfying verdict. After two in a row, RAGTools.rag appends a
	// "[Research status] … STOP calling rag again" note to the answer so the
	// outer agent stops re-asking. Python resets it on every RAGTools instance
	// (rebuilt per turn), so it counts consecutive insufficient rounds within a
	// single request; it is not persisted across turns unless a caller injects a
	// long-lived *RAGCache via RAGTools.Cache.
	//
	// Private and guarded by mu: a turn's concurrent rag() calls share ONE
	// *RAGCache (Rag builds/stores it on deps.Cache before the outer react
	// loop), so an unsynchronized counter would race.
	consecutiveUnanswerable int
}

// NoteUnanswerable records one research round's verdict on the shared cache.
func (c *RAGCache) NoteUnanswerable(verdict string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if verdict == VerdictSufficient {
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

// noteVerdict records the verdict of the last research round.
func (c *RAGCache) noteVerdict(verdict string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastVerdict = verdict
}

// researchStatusTrailer mirrors Python rag (:902-929): for every non-SUFFICIENT
// verdict it returns the trailing sentence folded into the "[Research status]"
// note. After two consecutive unsatisfying rag() calls (ConsecutiveUnanswerable
// >= 2 on the shared *RAGCache) it tells the outer agent to STOP calling rag
// again; otherwise it invites a focused re-ask. It returns "" when there is
// nothing to annotate — a SUFFICIENT verdict, an empty answer, or no SCA
// feedback.
func researchStatusTrailer(cache *RAGCache, resp *RunResponse) string {
	if resp.Verdict != VerdictInsufficient || resp.SCAFeedback == "" || resp.Answer == "" {
		return ""
	}
	if cache != nil && cache.ConsecutiveUnanswerable() >= 2 {
		return " STOP calling rag again: the same gaps remain."
	}
	return " If these gaps are material, call rag again with a question focused on them."
}

// reuseAllowed mirrors Python's `_cache_ok = not last_status or last_status ==
// "SUFFICIENT"`.
func (c *RAGCache) reuseAllowed() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastVerdict == "" || c.lastVerdict == "SUFFICIENT"
}

// wrapModelForStats wraps the model's invoker so calls made through it are
// counted per phase. It reports false when the model carries no chat.Invoker,
// in which case the caller leaves it untouched.
//
// Python achieves the same by handing a CountingChatModel proxy to the tools
// (agentic_rag.py:267); Go's SessionModel does not expose its invoker in a
// uniform way, so only the known carrier is wrapped. Callers that go through
// other session models still get counted when they record via CurrentStats(ctx).
func wrapModelForStats(model harness.SessionModel, stats *harness.LLMUsageStats) (harness.SessionModel, bool) {
	src, ok := model.(*harness.InvokerSessionModel)
	if !ok || src == nil || src.Invoker == nil {
		return model, false
	}
	// Copy before mutating: the caller may share this model across requests, and
	// wrapping in place would attach another run's stats to it.
	wrapped := *src
	wrapped.Invoker = &harness.CountingInvoker{Inner: src.Invoker, Stats: stats}
	return &wrapped, true
}

// searchDepsFor projects a RAGTools' FULL retrieval configuration into the harness
// view for one request, including tenant/dataset resolution and the nil-backend
// fallback. Projecting here rather than at each call site keeps the retrieval paths
// (Rag's low/naive pass, the agentic fan-out and action session, the no-model
// fallback) from drifting — a partial copy silently drops tuning such as
// rank_feature (Python's `rank_feature=label_question(question, self.kbs)`, :668).
func searchDepsFor(ctx context.Context, deps RAGTools, req harness.RunRequest, datasetIDs []string, tenantID string, kb *harness.Kbinfos, logger *log.Logger) harness.SearchDeps {
	if tenantID == "" {
		tenantID = harness.TenantIDFromContext(ctx)
	}
	backend := deps.Retriever
	if backend == nil {
		backend = &harness.RuntimeRetriever{}
	}
	return harness.SearchDeps{
		Backend:           backend,
		KbIDs:             datasetIDs,
		TenantID:          tenantID,
		KB:                kb,
		Expand:            deps.Expand,
		Logger:            logger,
		DocIDVerifier:     deps.DocIDVerifier,
		DocChunks:         deps.DocChunks,
		DocTenantResolver: dbDocTenantResolver{},
		Model:             deps.Model, // the calculate tool writes its expression via the model
		DocScope:          deps.DocScope,
		// Python retrieve:614-646 — configuration is the middle precedence
		// level, between an explicit tool argument and the module defaults.
		TopN:                   deps.TopN,
		SimilarityThreshold:    deps.SimilarityThreshold,
		VectorSimilarityWeight: deps.VectorSimilarityWeight,
		UsingEmbedding:         deps.UsingEmbedding,
		HasEmbedder:            deps.HasEmbedder,
		RerankCandidatesCount:  deps.RerankCandidatesCount,
		TopK:                   deps.TopK,
		MetaDataFilter:         deps.MetaDataFilter,
		// rank_feature (Python retrieve:668): RAGTools carries the KB objects
		// and a tagger, mirroring rank_feature=label_question(question, self.kbs).
		KBs:    deps.KBs,
		Tagger: deps.Tagger,
		// External embedding handle (Python tools.embed_mdl). When the caller
		// supplied one on RAGTools it is used directly; nil keeps the package's
		// internal tenant-default resolver as a fallback (which itself degrades
		// to keyword matching when the DB is uninitialised or encoding panics).
		Embedder: deps.Embedder,
	}
}

// NewDocTenantResolver returns the DB-backed doc→(kb, tenant) resolver used to
// group a document scope by its real owner (Python tools._resolve_doc_tenant).
// It is the same resolver graph_explore uses; the compiled expander takes it too
// so a doc scope is scanned per owning dataset instead of being attached to
// every bound one.
func NewDocTenantResolver() harness.DocTenantResolver { return dbDocTenantResolver{} }

// dbDocTenantResolver implements harness.DocTenantResolver against the database: it
// maps each document id to its real owning (kb, tenant) so graph_explore can group
// documents by owner and search knowledge bases outside the caller's datasetIDs.
// Mirrors Python tools._resolve_doc_tenant (exploration.py:_kg_scopes); the lookup is
// tenant-unscoped on purpose — DocScope is already constrained to the user's authorized
// documents — so a document may legitimately resolve to a KB not in the search set.
type dbDocTenantResolver struct{}

func (dbDocTenantResolver) ResolveDocTenants(ctx context.Context, docIDs []string) (map[string]harness.DocTenant, error) {
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
	out := make(map[string]harness.DocTenant, len(docs))
	for _, d := range docs {
		out[d.ID] = harness.DocTenant{KBID: d.KbID, TenantID: tenantByKB[d.KbID]}
	}
	return out, nil
}

func Rag(ctx context.Context, deps RAGTools, req harness.RunRequest) *RunResponse {
	logger := deps.Logger
	if logger == nil {
		logger = _LOG
	}
	spec := harness.GetMode(req.ThinkingMode)
	if !IsKnownMode(req.ThinkingMode) {
		// An unrecognised label resolves to NAIVE; log it so a typo in the mode
		// is visible instead of silently downgrading the request.
		logger.Printf("[Agentic RAG] unrecognised thinking mode %q; falling back to naive (non-agentic) retrieval", req.ThinkingMode)
	}

	// Python :266-267 — build the per-call usage counters and bind them to the
	// context so every LLM call beneath is attributed to its phase. Without this
	// the counting machinery is inert: CurrentStats(ctx) would stay nil and the
	// phase markers already sprinkled through the graph would record nothing.
	stats := deps.Stats
	if stats == nil {
		stats = harness.NewLLMUsageStats()
		deps.Stats = stats
	}
	ctx = harness.WithStats(ctx, stats)
	// Bind the caller's per-request progress sink (Python think_log counterpart)
	// so engine stages and every search beneath can forward tagged lines to the
	// live reasoning block. It flows through the whole RunAgenticRAG ctx lineage,
	// which runSearch / CurrentProgress read.
	if deps.Progress != nil {
		ctx = harness.WithProgress(ctx, deps.Progress)
	}
	// Mirror Python think_log (rag/advanced_rag/think_log.py): rebuild the run's
	// logger on top of the progress sink so every bracket-tagged stage line
	// streams into the chat <think> block. Python did this with a root
	// logging.Handler; Go has no per-record hook on *log.Logger, so the same
	// filtering happens at the writer instead. This logger is threaded into
	// SearchDeps below and into every graph node by RunAgenticRAG, so one wrap
	// here covers the whole pipeline without touching individual call sites.
	if deps.Progress != nil {
		logger = thinkLogger(logger, deps.Progress)
	}
	// Python :267 wraps the chat model in CountingChatModel(chat_mdl.clone(),
	// self.llm_stats). Go wraps the session model's invoker the same way, so
	// calls made through deps.Model are counted per phase too.
	if wrapped, ok := wrapModelForStats(deps.Model, stats); ok {
		deps.Model = wrapped
	}
	defer stats.Log(logger)

	// Python :831 — tell the caller research is starting, before any retrieval
	// work, so it can show progress for the (potentially long) graph run.
	if deps.ToolStarted != nil {
		deps.ToolStarted()
	}

	// Python dialog_service.rag_agent: when an outer chat model is wired
	// (RAGTools.chat_mdl), the request is driven by the outer react loop that
	// binds [rag, summarize_document] and treats `rag` as a terminal tool —
	// mirroring chat_mdl.async_chat_with_tools(tools=rag_tools.tools,
	// terminal_tools={"rag"}). The inner graph runs only when the model decides
	// to call `rag`; otherwise the model answers directly (or calls
	// summarize_document to read a whole document first). When no outer model is
	// configured, or the outer model cannot emit tool calls (mirroring Python
	// dialog_service.rag_agent's `if not chat_mdl.is_tools` fallback to
	// async_chat), we fall through to the existing direct RunAgenticRAG path so
	// behaviour is unchanged for every current caller.
	// Mirror Python dialog_service.rag_agent: the last user message
	// carries the question, text attachments, and — for vision models — image
	// content blocks. In Go these reach the outer react loop's history
	// (prepareOuterReact) so a vision model actually sees the images instead of
	// silently dropping them. Assembled here (not in the caller) so the
	// multimodal construction stays in the advanced_rag package beside the
	// schema import.
	if len(deps.Messages) == 0 {
		deps.Messages = multimodalUserMessage(req.Question, req.TextAttachments, req.Images)
	}
	// Text attachments also feed the direct (non-outer) path, which appends them
	// to the question (Python text_attachments_content handling); attachments
	// bypass the near-duplicate cache (see cacheable above).
	if deps.TextAttachments == "" {
		deps.TextAttachments = req.TextAttachments
	}

	// Build the per-request RAGCache up-front so the outer react loop (below)
	// reuses the SAME cache across its multiple rag() calls within one turn.
	// Keeping deps.Cache non-nil here is what makes the _consecutive_unanswerable
	// guard (Python rag:921-924) live: ConsecutiveUnanswerable is incremented
	// inside RunAgenticRAG and gated on deps.Cache != nil, so without a shared
	// cache the outer loop would never reach the "STOP calling rag again" verdict.
	// Python keeps the counter on the persistent RAGTools instance, which the
	// outer loop naturally shares; Go mirrors that by setting deps.Cache before
	// any branch rather than only on the direct path.
	if deps.Cache == nil {
		deps.Cache = NewRAGCache()
	}

	if deps.Outer != nil && deps.OuterSupportsTools {
		if deps.AnswerSink != nil {
			return runOuterReactStream(ctx, deps, req, logger)
		}
		return runOuterReact(ctx, deps, req, logger)
	}

	// Re-ask guard: reuse a near-identical question's cached answer instead of
	// re-running the graph (Python rag:837-858). Attachments bypass it, since
	// their content is appended to the question below and is not part of the key.
	// Python keeps _rag_cache on the RAGTools instance, which is rebuilt for
	// every turn, so the cache is per-turn by construction. Go mirrors that
	// Go mirrors Python's default-on caching: deps.Cache is guaranteed non-nil
	// here (built above, before the outer-react branch, so it is shared across
	// the outer loop's multiple rag() calls). Python keeps _rag_cache on the
	// RAGTools instance, which is rebuilt for every turn, so the cache is
	// per-turn by construction. The auto-built cache lives only for this call
	// (the per-turn degenerate case, since the current reasoning path issues a
	// single Rag() per turn), so it can never serve a stale cross-turn hit. A
	// caller that wants to share one cache across multiple Rag() calls within a
	// single turn (Python's concurrent tool_call case) injects the same
	// *RAGCache via deps.Cache instead of relying on the auto-built one.
	cache := deps.Cache
	cacheable := deps.TextAttachments == ""
	if cacheable && cache != nil {
		if cached, hit := cache.Lookup(req.Question); hit {
			logger.Printf("[Agentic RAG] cache hit — reused prior answer for near-identical question %q; skipped research", trunc(req.Question, 80))
			return &RunResponse{Answer: cached, Mode: spec}
		}
	}

	// Prefer the user's ORIGINAL, complete question over the outer rewrite
	// (Python rag:865). The outer rewrite often drops the final target of a
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
		tenantID = harness.TenantIDFromContext(ctx)
	}
	datasetIDs := req.DatasetIDs

	kb := &harness.Kbinfos{}
	searchDeps := searchDepsFor(ctx, deps, req, datasetIDs, tenantID, kb, logger)

	resp := &RunResponse{Mode: spec, Kbinfos: kb}

	// Python :877 — hand over to the graph driver, which owns the mode dispatch
	// (agentic graph vs. direct/naive retrieval) and formalization.
	//
	// The terminal composition is handed to the graph instead of being run here:
	// Python composes inside the last node of both graphs (agentic
	// formalize_answer, low formalize_answer). The closure is
	// idempotent so the post-graph call below is a no-op once the graph has
	// composed — a graph that never reaches its last node (or the naive path,
	// which Python composes under naiveAnswerSystem instead) still gets an
	// answer here.
	composed := false
	compose := func(ctx context.Context) {
		if composed {
			return
		}
		composed = true
		composeFinalAnswer(ctx, deps, req, kb, resp, logger)
	}
	deps.Finalize = compose

	RunAgenticRAG(ctx, deps, req, searchDeps, kb, resp, logger, spec)

	resp.Chunks = kb.Chunks
	resp.DocAggs = kb.DocAggs
	resp.EmptyResult = len(kb.Chunks) == 0

	// Publish evidence for citation grounding. Best-effort: when no canvas
	// state is attached (e.g. unit tests), skip silently — mirroring
	// RetrievalTool's behaviour.
	if len(kb.Chunks) > 0 {
		harness.PublishReferences(ctx, kb)
	}

	// The graph's last node composed already (see compose above); this only
	// catches runs that never reached it. The naive path (_naive_rag)
	// already composed under naiveAnswerSystem from flat "[i] content"
	// evidence, so composing here would replace that with the
	// FinalAnswerSystem / kb_prompt answer.
	if spec.Label != "naive" {
		compose(ctx)
	}

	// Python rag (:902-929) appends a "[Research status]" note for EVERY
	// non-SUFFICIENT verdict. When research has stayed unsatisfying for two
	// consecutive turns it tells the outer agent to STOP calling rag again;
	// otherwise it invites a focused re-ask. The counter lives on the
	// conversation-scoped cache; it was incremented back in NewAgenticLoop once
	// the SCA verdict was known. Skip the naive/sufficient paths: an empty
	// answer or a SUFFICIENT verdict has nothing to annotate.
	if t := researchStatusTrailer(deps.Cache, resp); t != "" {
		resp.Answer += "\n\n[Research status] " + resp.SCAFeedback + t
	}

	// Cache the freshly produced answer for later near-identical questions, and
	// remember the verdict so a following re-ask is not answered from an
	// admittedly incomplete one (Python rag:888-889 and :848-852).
	if cacheable && cache != nil {
		cache.Store(req.Question, resp.Answer)
		cache.noteVerdict(resp.Verdict)
	}
	return resp
}

// composeFinalAnswer runs the graph's terminal node and writes the result onto
// the response.
//
// When no model is configured this is a no-op: the caller still receives the
// evidence and can answer with it, matching Python's behaviour of returning an
// empty kbinfos rather than failing.
func composeFinalAnswer(ctx context.Context, deps RAGTools, req harness.RunRequest, kb *harness.Kbinfos, resp *RunResponse, logger *log.Logger) {
	if deps.Model == nil {
		return
	}
	// Python tags the terminal node @in_phase("finalize")
	// (formalize_answer); without it every call made here is
	// attributed to "unknown" in the usage table.
	ctx, done := harness.Phase(ctx, harness.PhaseFinalize)
	defer done()
	if deps.ComposeAnswer != nil && !*deps.ComposeAnswer {
		return
	}
	adeps := AnswerDeps{
		Model:         deps.Model,
		CiteRules:     deps.CiteRules,
		SystemPrompt:  deps.SystemPrompt,
		EmptyResponse: deps.EmptyResponse,
		MaxTokens:     deps.EvidenceMaxTokens,
		MaxLength:     deps.MaxLength,
		Logger:        logger,
		// UserImages mirrors Python's direct async_chat fallback receiving the
		// original multimodal messages: the non-outer compose model sees the
		// vision-gated images too. The outer react path clears these in the rag
		// terminal tool (Python inner _compose_answer_from_evidence is text-only).
		UserImages: req.Images,
	}
	// Stream the answer when the model and the caller both support it, so the
	// user sees text while it is produced instead of only at the end.
	if deps.AnswerSink != nil {
		if streamer, ok := deps.Model.(harness.StreamingSessionModel); ok {
			deps.AnswerSink.reset()
			streamed, err := ComposeAnswerStream(ctx, adeps, streamer, kb, req.Question, resp.Partial, func(delta string, isThink bool) error {
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
			// to drop what it already forwarded and fall back to one shot.
			logger.Printf("[Agentic RAG] streaming compose failed (%v); falling back to a single call", err)
			deps.AnswerSink.reset()
		}
	}
	// No manual RecordCall here: the call is attributed to the "finalize" phase
	// by the wrapped invoker above (Python relies on @in_phase alone).
	// empty_result is Python's third no-evidence term (_compose_answer_from_evidence) — the loop's own
	// "nothing was found" signal, distinct from abstain and from an empty pool.
	res := ComposeAnswerWith(ctx, adeps, kb, req.Question, resp.Partial, false, resp.EmptyResult)
	if deps.Stats != nil && res.Failed {
		deps.Stats.RecordFailed(harness.PhaseFinalize)
	}
	resp.Answer = res.Answer
	if res.Partial {
		resp.Partial = true
	}
	if deps.AnswerSink != nil {
		deps.AnswerSink.deliver(res.Answer, false)
	}
}

// outerReactParts holds the pieces shared by the streaming and non-streaming
// outer react loops (Python rag_agent's agent_messages + rag_tools.tools).
type outerReactParts struct {
	kb      *harness.Kbinfos
	sd      harness.SearchDeps
	spec    harness.ModeSpec
	resp    *RunResponse
	tools   []map[string]any
	history []models.Message
	system  string
}

// multimodalUserMessage mirrors Python dialog_service.rag_agent's assembly of
// agent_messages[-1]: a user turn whose text is the question plus any text
// attachments, augmented with image content blocks for vision-capable models.
// imageFiles are vision-gated base64 data URIs (data:...); textAttachments is
// joined file content. When there is nothing to carry (no question, no
// attachments, no images) it returns an empty slice so callers can skip it.
//
// The multimodal part list is used ONLY when images are attached, mirroring
// Python dialog_service.rag_agent: the text (question +
// attachments) stays a plain string, and the message is converted to content
// blocks just for image attachments. Sending a content-block array for a
// text-only question is not harmless — text-only providers silently drop it
// (Zhipu GLM answers an empty user turn with "Hello! How can I assist you
// today?" instead of calling the `rag` tool), and Python explicitly avoids it
// for that reason.
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
func prepareOuterReact(ctx context.Context, deps RAGTools, req harness.RunRequest, logger *log.Logger) *outerReactParts {
	tenantID := req.TenantID
	if tenantID == "" {
		tenantID = harness.TenantIDFromContext(ctx)
	}
	kb := &harness.Kbinfos{}
	sd := searchDepsFor(ctx, deps, req, req.DatasetIDs, tenantID, kb, logger)
	spec := harness.GetMode(req.ThinkingMode)

	tools := []map[string]any{
		{
			"type": "function",
			"function": map[string]any{
				"name":        "rag",
				"description": "Run the full agentic research graph over the configured datasets and return a cited answer.",
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
				"name":        "summarize_document",
				"description": "Read an entire document by id into the evidence set and return a short summary the model can answer from.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"doc_id": map[string]any{
							"type":        "string",
							"description": "The document id to read.",
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

// outerStreamMux merges the outer model's stream with the inner rag stream into
// one sink, mirroring Python rag_agent's event-queue state machine
// (dialog_service.py:2212-2274): the outer model's reasoning, the inner
// research log and the inner answer all share a single think/answer block.
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
	// outerText accumulates the outer model's OWN non-think text. Python
	// rag_agent treats a tool-less reply as the answer, so the streaming loop
	// must be able to hand it back to the caller instead of dropping it (the
	// inner `rag` stream arrives through deliver, never through here).
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
		// Python drops non-think outer text once the terminal tool has fired:
		// what follows is the aggregate tool result, and the answer has already
		// been streamed from inside the tool.
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

// markTerminal records that the terminal `rag` tool ran, so later outer text is
// treated as the aggregate tool result and dropped (Python outer_tool_started).
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

// runOuterReact drives the Python-style rag_agent outer loop (Python
// dialog_service.rag_agent + RAGTools.tools=[rag, summarize_document] under
// chat_mdl.async_chat_with_tools with terminal_tools={"rag"}). The outer chat
// model decides whether to:
//   - call `rag` (terminal): run the full inner agentic graph and return the
//     cited answer, which the terminal short-circuit turns into the final answer;
//   - call `summarize_document` (non-terminal): read a whole document into the
//     evidence set and let the model answer from it on the next round;
//   - answer directly with no tool call.
//
// deps.Outer must be non-nil; Rag() only delegates here when it is set, so
// existing callers that don't wire an outer model keep using the direct
// RunAgenticRAG path unchanged.
func runOuterReact(ctx context.Context, deps RAGTools, req harness.RunRequest, logger *log.Logger) *RunResponse {
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

	answer, _, err := outer.ChatWithTools(ctx, p.system, p.history, &models.ChatConfig{})
	if err != nil {
		logger.Printf("[Agentic RAG] outer react failed: %v; falling back to direct graph", err)
		// Fall back to the inner graph directly so the user still gets an answer.
		RunAgenticRAG(ctx, deps, req, p.sd, p.kb, p.resp, logger, p.spec)
		composeFinalAnswer(ctx, deps, req, p.kb, p.resp, logger)
		return p.resp
	}
	// The fold returned the LOWEST-INDEX terminal rag call's answer: keep that
	// call's evidence so the answer's [ID:n] markers line up with the chunks.
	session.selectEvidence(answer)
	p.resp.Answer = answer
	p.resp.Chunks = p.kb.Chunks
	p.resp.DocAggs = p.kb.DocAggs
	p.resp.EmptyResult = len(p.kb.Chunks) == 0
	return p.resp
}

// runOuterReactStream is the streaming counterpart of runOuterReact: it drives
// the same [rag, summarize_document] react loop but merges two live streams
// into the single sink the caller supplied, mirroring Python rag_agent's
// event-queue state machine (dialog_service.py:2212-2274):
//   - the outer model's own reasoning/text, arriving through the models layer;
//   - the inner rag run's research log (think) and composed answer (answer),
//     produced while the terminal `rag` tool executes.
//
// `rag` streams its answer itself and returns "", so the terminal short-circuit
// stops the loop without re-streaming it (Python ignores the terminal result
// once the inner answer_sink has streamed).
func runOuterReactStream(ctx context.Context, deps RAGTools, req harness.RunRequest, logger *log.Logger) *RunResponse {
	p := prepareOuterReact(ctx, deps, req, logger)
	mux := &outerStreamMux{sink: deps.AnswerSink}

	// Route the inner run's progress and answer through the mux so they share
	// one think/answer block with the outer model's stream.
	inner := deps
	// Same separator as thinkWriter: the block is HTML, so "\n" would collapse.
	inner.Progress = func(line string) { mux.deliver(line+ThinkLineBreak, true) }
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

	stream := true
	_, err := outer.ChatStreamlyWithTools(ctx, p.system, p.history, &models.ChatConfig{Stream: &stream}, mux.sender)
	if err != nil {
		logger.Printf("[Agentic RAG] outer react stream failed: %v; falling back to direct graph", err)
		// Fall back with the caller's own sink so the answer still streams out.
		RunAgenticRAG(ctx, deps, req, p.sd, p.kb, p.resp, logger, p.spec)
		composeFinalAnswer(ctx, deps, req, p.kb, p.resp, logger)
		return p.resp
	}
	// A tool-less reply is the answer (Python rag_agent returns the model's
	// text verbatim when it decides not to call a tool). The stream already
	// delivered it to the caller's sink; recording it on the response is what
	// lets the chat pipeline short-circuit instead of composing a second,
	// contradicting answer from the (empty) evidence set. When the terminal
	// `rag` tool fired, composeFinalAnswer already owns resp.Answer.
	if p.resp.Answer == "" && !mux.terminalFired {
		p.resp.Answer = strings.TrimSpace(mux.outerText.String())
	}
	// Keep only the evidence of the call that owns the answer (selectEvidence
	// no-ops when the answer came from the outer model instead of a rag call).
	session.selectEvidence(p.resp.Answer)
	p.resp.Chunks = p.kb.Chunks
	p.resp.DocAggs = p.kb.DocAggs
	p.resp.EmptyResult = len(p.kb.Chunks) == 0
	return p.resp
}

// outerReactSession implements models.ToolCallSession for the outer react loop.
type outerReactSession struct {
	ctx    context.Context
	deps   RAGTools
	req    harness.RunRequest
	sd     harness.SearchDeps
	kb     *harness.Kbinfos
	resp   *RunResponse
	logger *log.Logger
	spec   harness.ModeSpec
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
}

// ToolCall routes the two outer tools to their Go implementations.
//
// A round's tool calls arrive CONCURRENTLY — models.appendToolResults runs one
// goroutine per call — exactly as Python gathers them (asyncio.gather over the
// round's tool_calls, chat_model.py:670/:2555). Each rag call therefore runs on
// its own request/evidence/response and publishes the outcome under mu; sharing
// the session's would let two calls mix questions, evidence and answers.
func (s *outerReactSession) ToolCall(name string, arguments map[string]interface{}) (string, error) {
	switch name {
	case "rag":
		// Work on a COPY of the request: this call must not rewrite the
		// session's question/images, which a concurrent call is reading.
		req := s.req
		if q, ok := arguments["question"].(string); ok && q != "" {
			req.Question = q
		}
		// Python's inner _compose_answer_from_evidence is text-only: the outer
		// model already saw the images via multimodal history, and the rephrased
		// question carries no images. Drop the original images so the inner
		// compose model (ComposeAnswerWith via composeFinalAnswer) does not
		// receive them — matching Python's inner compose.
		req.Images = nil

		// Per-call evidence, response and the projections that point at them:
		// Python gives every rag() invocation its own graph state and shares only
		// the tools object (agentic_rag.py:877 run_agentic_rag(self, messages)).
		kb := &harness.Kbinfos{}
		resp := &RunResponse{Mode: s.spec, Kbinfos: kb}
		sd := s.sd
		sd.KB = kb
		inner := s.deps
		inner.KB = kb

		if s.mux != nil {
			// Streaming: s.deps already routes the inner research log and the
			// composed answer into the caller's sink, so return "" — the
			// terminal short-circuit stops the loop and sendTerminal stays
			// silent, so the answer is not streamed twice.
			s.mux.markTerminal()
			RunAgenticRAG(s.ctx, inner, req, sd, kb, resp, s.logger, s.spec)
			composeFinalAnswer(s.ctx, inner, req, kb, resp, s.logger)
			s.publish(resp, kb)
			return "", nil
		}
		// Non-streaming: disable the sink so the answer is not emitted twice,
		// and hand this call's own answer back for the terminal short-circuit.
		inner.AnswerSink = nil
		RunAgenticRAG(s.ctx, inner, req, sd, kb, resp, s.logger, s.spec)
		composeFinalAnswer(s.ctx, inner, req, kb, resp, s.logger)
		s.publish(resp, kb)
		return resp.Answer, nil
	case "summarize_document":
		docID, _ := arguments["doc_id"].(string)
		if docID == "" {
			return "Error: missing doc_id argument.", nil
		}
		blocks := harness.SummarizeDocument(s.ctx, s.sd, docID, s.deps.EvidenceMaxTokens)
		if len(blocks) == 0 {
			return "The document is unavailable or has no readable chunks.", nil
		}
		return strings.Join(blocks, "\n\n"), nil
	}
	return fmt.Sprintf("unknown tool %q", name), nil
}

// publish merges one rag call's outcome into the session state. The inner run
// used to write that state directly, before each call got its own; the merge
// reproduces what a single call contributed, and for concurrent calls it keeps
// the FIRST non-empty answer/verdict (appendToolResults folds the first terminal
// hit in call order) while the evidence is unioned, so the caller still sees the
// chunks the answer was composed from.
func (s *outerReactSession) publish(call *RunResponse, kb *harness.Kbinfos) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Remember this call's own answer + evidence so selectEvidence can restore
	// the pool the winning answer was composed from.
	s.calls = append(s.calls, ragCallResult{answer: call.Answer, kb: kb})
	if s.resp.Answer == "" {
		s.resp.Answer = call.Answer
	}
	if s.resp.Verdict == "" {
		s.resp.Verdict = call.Verdict
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
	kb     *harness.Kbinfos
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
		return
	}
}

// runDirect is the low/naive path: one hybrid search, no tool loop.
//
// It is the ONE Go implementation of Python orchestrator/direct.py::direct_search
// (called from the low graph node) — there is deliberately no harness-level
// duplicate in harness/orchestrator. It lives here rather than there because the
// step reads the full RAGTools config; the graph node that calls it forces
// UseCompiled=true, which Python's direct_search hardcodes.
func runDirect(ctx context.Context, deps RAGTools, req harness.RunRequest, sd harness.SearchDeps, kb *harness.Kbinfos, resp *RunResponse, logger *log.Logger) {
	ctx, done := harness.Phase(ctx, harness.PhaseDirect)
	retrievalQuery := ""
	if deps.Keywords != nil {
		rq, err := deps.Keywords(ctx, req.Question)
		if err != nil {
			logger.Printf("[Agentic RAG] weighted keyword extraction failed: %v", err)
		} else {
			retrievalQuery = rq
		}
	} else if deps.Model != nil {
		// Default: the four-aspect weighted extraction (Python
		// tools.extract_keywords / harness/keywords.py). It gives BM25 both the
		// discriminating entity and the surface variants the corpus may use.
		rq, kw := harness.ExtractWeightedKeywords(ctx, deps.Model, req.Question)
		retrievalQuery = rq
		if req.Keywords == "" {
			req.Keywords = kw
		}
	}
	defer done()

	chunks, aggs := harness.HybridSearch(ctx, sd, harness.SearchParams{
		Question:       req.Question,
		Keywords:       req.Keywords,
		RetrievalQuery: retrievalQuery,
		UseCompiled:    req.UseCompiled,
		TopN:           req.TopN,
		// Mirrors Python RAGTools.retrieve — the only function honouring
		// using_embedding.
		Channel: harness.ChannelRetrieve,
	})
	kb.Merge(chunks, aggs)
	if !kb.HasChunks() {
		logger.Printf("[Agentic RAG] direct search found no matching passages")
	}
}

// AgenticLoop runs the full agentic search loop (planner / prefetch / SCA↔
// rewriter iteration) for an agentic mode.
//
// It is registered by the advanced_rag package's init rather than called directly: the
// advanced_rag package owns RAGTools, so the harness cannot import it back. The
// indirection keeps the dependency graph acyclic. Registering is optional — see
// Run for the fallback when nothing is registered.
type AgenticLoop func(ctx context.Context, deps RAGTools, req harness.RunRequest, kb *harness.Kbinfos, resp *RunResponse, logger *log.Logger)

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
func runAgentic(ctx context.Context, deps RAGTools, req harness.RunRequest, sd harness.SearchDeps, kb *harness.Kbinfos, resp *RunResponse, logger *log.Logger) {
	// Formalization happens inside the graph: Python wires it as the
	// build_agentic_graph entry node (add_edge(START, "formalize_question")).
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
func runSingleSession(ctx context.Context, deps RAGTools, req harness.RunRequest, sd harness.SearchDeps, kb *harness.Kbinfos, resp *RunResponse, logger *log.Logger) {
	// Seed the slot table and run one bounded session against it. A fresh tool
	// cache and query history per run: they exist to dedupe WITHIN a session,
	// not across requests.
	sd.WebSearch = deps.WebSearch
	sd.CiteRules = deps.CiteRules
	sessionDeps := harness.SessionDeps{
		Tools: &harness.Toolset{
			ThinkingMode: resp.Mode.Label,
			// Provider gate, mirroring Python action_session.py:463: without a
			// wired provider the tool is hidden rather than advertised dead.
			HasWebSearch:  resp.Mode.HasTool("web_search") && deps.WebSearch != nil,
			DisabledTools: map[string]bool{},
			Exec:          harness.NewSearchExecutor(sd, req),
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
	init := harness.InitializeState(ctx, sessionDeps, req.Question, nil, req.DeadlineLeft)
	root := init.Root

	result := harness.RunActionSession(ctx, sessionDeps, req.Question, root, req.DeadlineLeft, "", nil, nil)
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
	_, ok := harness.THINKING_MODES[strings.ToLower(strings.TrimSpace(label))]
	return ok
}

// DocIDLookup mirrors Python RAGTools._filter_known_doc_ids (agentic_rag.py:981):
// it resolves which of a candidate document id set exist within the given
// datasets.
//
// Like Python, it reads the document table directly; the harness package takes
// it through the harness.DocIDVerifier interface so the retrieval path stays
// free of a database dependency.
type DocIDLookup struct {
	docs *dao.DocumentDAO
}

// NewDocIDLookup returns the document-ownership check backed by the document
// table.
func NewDocIDLookup() *DocIDLookup {
	return &DocIDLookup{docs: &dao.DocumentDAO{}}
}

// KnownDocIDs implements harness.DocIDVerifier: it returns the subset of
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

// compile-time check: the lookup satisfies the harness seam.
var _ harness.DocIDVerifier = (*DocIDLookup)(nil)
