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

package advanced_rag

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/agent/chat"
	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/prompts"
	"ragflow/internal/tokenizer"
)

// The answer stage: the terminal node's prompt, its budgeted evidence, and the
// composition paths (streaming, one-shot, naive, and the failure fallbacks).
// ComposeFallbackDraft: an
// intermediate draft synthesized from the snippet pool when a research pass
// produced no report (budget exhaustion).
//
// This is an LLM call, not a concatenation. The draft is what the
// sufficient-context agent reviews and what later lands in PreSummary as
// "Research findings (authoritative — use these facts verbatim)", so the model is asked
// for a specific shape:
//
//   - the FOUND facts, stated plainly;
//   - a final "MISSING:" line naming what is still unknown.
//
// The MISSING line is load-bearing: it is what tells the next round (and the
// final answer) which unknowns remain. A plain list of truncated snippets
// cannot express it.
func ComposeFallbackDraft(ctx context.Context, deps RAGTools, st *AgenticState) string {
	// Nil-safe: the draft node runs on every round, including a state whose KB was never
	// populated.
	if st == nil || st.KB == nil || len(st.KB.Chunks) == 0 {
		return ""
	}

	// strongest evidence first — the pool is in insertion order, so
	// sort by similarity/score before spending the 16-slot budget on it.
	chunks := rankByScore(st.KB.Chunks)

	// "[i] text" joined by "\n", 1-indexed, 16 chunks x 1200
	// chars. Empty chunk text still produces its "[i] " line — nothing is skipped or
	// trimmed.
	var ev strings.Builder
	n := min(len(chunks), draftChunkCap)
	for i := 0; i < n; i++ {
		if ctx.Err() != nil {
			break
		}
		if i > 0 {
			ev.WriteString("\n")
		}
		fmt.Fprintf(&ev, "[%d] %s", i+1, truncateRunes(harness.ChunkTextOf(chunks[i]), draftChunkChars))
	}
	evidence := ev.String()
	if evidence == "" {
		return ""
	}

	// no model -> the raw evidence, capped at 4000.
	mdl := deps.Model // the innermost chat model.
	if mdl == nil {
		return truncateRunes(evidence, draftFallbackChars)
	}

	// The design injects the latest research_feedback as a "focus", but that state field is
	// vestigial: it is declared and initialised to [] and never appended anywhere, so the
	// focus is always empty. There is therefore no feedback field here — populating one
	// would invent behaviour that does not exist.
	//
	// The prompt asks for the FOUND list plus a final
	// "MISSING:" line is what gives the SCA precise gaps.
	system := "You are a research assistant writing an INTERMEDIATE DRAFT toward answering the user's " +
		"question, using ONLY the retrieved evidence snippets below.\n" +
		"Requirements:\n" +
		"1. First list concrete FACTS FOUND in the snippets (exact numbers, dates, names preserved).\n" +
		"2. Then output a line starting with 'MISSING:' naming precisely which part(s) of the " +
		"question the snippets do NOT answer yet.\n" +
		"3. No conclusions beyond the evidence; no general knowledge.\n" +
		"Keep it under 250 words."
	if containsNonASCII(st.Question) {
		// _compose_fallback_draft — appended directly, with no separator.
		system += "Write your draft in the same language as the question."
	}

	user := "Question: " + st.Question + "\n\nRetrieved evidence:\n" + evidence

	reply, err := mdl.Complete(ctx, []schema.Message{
		*schema.SystemMessage(system),
		*schema.UserMessage(user),
	}, nil)
	if err != nil {
		// a failed composition degrades to the raw evidence
		// (capped at 4000, unlike the composed draft's 6000).
		_LOG.Printf("[Draft] fallback composition failed; using snippet text: %v", err)
		return truncateRunes(evidence, draftFallbackChars)
	}
	answer := ""
	if reply != nil {
		answer = strings.TrimSpace(reply.Content)
	}
	if answer == "" {
		answer = evidence
	}
	// (ans or evidence)[:6000].
	return truncateRunes(answer, draftMaxChars)
}

// NaiveRAG: answer with one retrieve pass
// and no agentic graph at all.
//
// Used when the thinking mode is unrecognised — instead of failing the request
// (the label comes from user input) this degrades to plain retrieval plus one
// composed answer.
//
// The composition itself lives in ComposeNaiveAnswer: a fixed short prompt, flat
// "[i] content" evidence over
// the first 8 chunks at 1500 chars, message_fit_in, temperature 0.3, and the
// raw evidence as the failure fallback).
func NaiveRAG(ctx context.Context, deps RAGTools, req harness.RunRequest, kb *harness.Kbinfos, resp *RunResponse, logger *log.Logger) {
	if logger == nil {
		logger = _LOG
	}
	question := strings.TrimSpace(req.Question)

	logger.Printf("[Naive RAG] single-pass retrieval for question_len=%d", len(question))

	// tools.retrieve(question) — one PLAIN pass. Unlike runDirect
	// (low mode) this extracts no weighted keywords and expands no compiled
	// structure: naive is deliberately plain retrieval, and a failure degrades
	// to "no evidence" rather than erroring.
	var chunks []map[string]any
	if question != "" {
		// The retrieve call reads the full run config — including the tag boost. Projecting
		// only a subset here would silently drop that tuning for the naive path.
		var aggs []map[string]any
		chunks, aggs = harness.RetrieveSearch(ctx,
			searchDepsFor(ctx, deps, req, req.DatasetIDs, req.TenantID, kb, logger),
			harness.SearchParams{
				Question: question,
				TopN:     req.TopN,
			})
		// accumulate onto the shared pool so the composed
		// answer can be cited and callers see the same shape as the agentic path.
		kb.Merge(chunks, aggs)
	}

	// no evidence -> yield the configured empty response.
	if len(chunks) == 0 {
		resp.Answer = deps.EmptyResponse
		return
	}

	// compose from the flat "[i] content" evidence under the
	// fixed short naiveAnswerSystem — NOT FinalAnswerSystem / kb_prompt. This is
	// why the naive path must not fall through to composeFinalAnswer.
	out := ComposeNaiveAnswer(ctx, AnswerDeps{
		Model:         deps.Model,
		EmptyResponse: deps.EmptyResponse,
		MaxLength:     deps.MaxLength,
		Logger:        logger,
	}, chunks, question)

	resp.Answer = out.Answer
}

// runDirectFallback retrieves once when no model is configured: the loop cannot
// plan, research, or review without one, so the caller still gets evidence
// rather than an error.
func runDirectFallback(ctx context.Context, deps RAGTools, req harness.RunRequest, kb *harness.Kbinfos, logger *log.Logger) {
	chunks, aggs := harness.HybridSearch(ctx,
		searchDepsFor(ctx, deps, req, req.DatasetIDs, req.TenantID, kb, logger),
		harness.SearchParams{
			Question:    req.Question,
			Keywords:    req.Keywords,
			UseCompiled: req.UseCompiled,
			TopN:        req.TopN,
			// The retrieve channel.
			Channel: harness.ChannelRetrieve,
		})
	kb.Merge(chunks, aggs)
}

// Final answer composition (the graph's last node).
//
// Every thinking mode ends here: it turns the gathered evidence into a grounded, cited
// answer in the user's language. The evidence-block and citation-rule prompts it renders
// (kb_prompt / citation_prompt) live in internal/rag/prompts; the system-prompt texts live
// in the harness package (report_prompt.go).

const (
	// evidenceBudgetTokens is the token ceiling of the evidence block.
	evidenceBudgetTokens = 8000
	// evidencePoolQuota: claim pseudo-chunk
	// cap across the whole first prefetch.
	evidencePoolQuota = 24
	// rawSnippetQuota: the raw-chunk
	// admission budget inside the fan-out, so chunk channels cannot crowd out
	// the denser evidence rows.
	rawSnippetQuota = 30
	// citeChunkCap caps chunks rendered as citation reference.
	citeChunkCap = 6
	// answerTimeoutS bounds the answer-composition call.
	answerTimeoutS = 150.0
	// naiveEvidenceChunkCap caps evidence chunks in the naive path.
	naiveEvidenceChunkCap = 8
	// naiveEvidenceCharCap caps each naive evidence chunk.
	naiveEvidenceCharCap = 1500
	// answerErrorFallback is the stream-failure message.
	answerErrorFallback = "I'm sorry, I encountered an error while composing the answer."
	// graphFailureFallback: last-resort message
	// (run_agentic_rag), used only when the graph failed AND produced nothing.
	graphFailureFallback = "I couldn't complete the search due to an internal error."
	// AgenticRecursionLimit: for the agentic
	// graph: the maximum number of node visits before the graph aborts. Go has
	// no graph runtime, so the loop counts its own node visits against it.
	AgenticRecursionLimit = 60
	// agenticRoundVisits is the number of node visits one research round costs:
	// rag_agent → draft → sca.
	agenticRoundVisits = 3
	// lowRecursionLimitBase is the floor of `max(25, max_loops * 8)` for the non-agentic
	// graph.
	lowRecursionLimitBase = 25
)

// FinalAnswerSystem and PartialAnswerPreamble are defined in the harness package
// (harness/report_prompt.go) and re-used here via the harness import rather than
// duplicated.

// AnswerDeps are the dependencies of ComposeAnswer.
type AnswerDeps struct {
	// Model drives the composition call.
	Model harness.SessionModel
	// CiteRules overrides DEFAULT_CITE_RULES. Empty uses the default.
	CiteRules string
	// SystemPrompt is the dialog-level UI configuration. Appended AFTER the agentic
	// contract — see the
	// precedence note in composeSystem.
	SystemPrompt string
	// EmptyResponse is returned verbatim when there is no evidence, skipping the LLM
	// entirely.
	EmptyResponse string
	// MaxTokens caps the evidence block. <=0 uses evidenceBudgetTokens.
	MaxTokens int
	// MaxLength is the chat model's context window. It bounds the prompt fit; <=0 falls back
	// to chat.EffectiveContextLength's 8192 default.
	MaxLength int
	// Logger is optional; nil uses the default logger.
	Logger *log.Logger
	// UserImages are the vision-gated base64 data URIs that survive
	// gateImageAttachments. The direct fallback is called with the original multimodal
	// messages, so they are attached to the final-answer user message and the compose model
	// sees the images even on the non-outer path. Empty for text-only models / no images.
	UserImages []string
}

// ComposeAnswerWith is ComposeAnswer with the third no-evidence term: `no_evidence =
// abstain or empty_result or not chunks`, used both for the empty_response short circuit
// and for the degradation instructions in the prompt. The extra `emptyResult` term is the
// agentic loop's own "nothing was found" signal, distinct from "we abstained" and from
// "the pool happens to be empty".
func ComposeAnswerWith(ctx context.Context, deps AnswerDeps, kb *harness.Kbinfos, question string, partial, abstain, emptyResult bool) AnswerResult {
	logger := deps.Logger
	if logger == nil {
		logger = _LOG
	}
	chunks := []map[string]any{}
	if kb != nil {
		chunks = kb.Chunks
	}
	note := ""
	if partial {
		note = " — partial answer, some gaps remain"
	} else if abstain {
		note = " — not enough evidence to answer"
	}
	logger.Printf("[Composing the answer] Writing the final answer to %q from %d gathered passage(s)%s.",
		trunc(question, 60), len(chunks), note)

	// 1. No-evidence short circuit.
	// _compose_answer_from_evidence: no_evidence = abstain or empty_result or not chunks.
	noEvidence := abstain || emptyResult || len(chunks) == 0
	if noEvidence && deps.EmptyResponse != "" {
		logger.Printf("[Composing the answer] No supporting evidence was found; returning the configured empty response without calling the answer model.")
		return AnswerResult{Answer: deps.EmptyResponse, NoEvidence: true}
	}
	if deps.Model == nil {
		return AnswerResult{Answer: "", Failed: true, NoEvidence: len(chunks) == 0}
	}

	// 2. Build the prompt: ranked evidence under its token budget, plus the
	// question and any research findings.
	preSummary := ""
	if kb != nil {
		preSummary = kb.PreSummary
	}
	// Both the prompt and this log go through composedRecord, so the line below
	// reports the record block the answer actually carried.
	record := composedRecord(kb)
	prompt := deps.answerPromptWithEvidence(kb, question, partial, noEvidence)

	// 3. Call the model.
	callCtx, cancel := context.WithTimeout(ctx, deadlineToDuration(answerTimeoutS))
	defer cancel()

	logger.Printf("[Formalize][record] question=%q record_len=%d draft_summary_len=%d evidence_len=%d using=%s\nrecord=%q",
		trunc(question, 160), len(record), len(preSummary), len(prompt.user), recordSource(record),
		truncateRunes(record, 3000))

	logger.Printf("[Formalize][pre_summary] question=%q pre_summary_len=%d evidence_len=%d\npre_summary=%q",
		trunc(question, 160), len(preSummary), len(prompt.user), truncateRunes(preSummary, 3000))

	// The composed prompt is fitted ONCE before the call, bounded by the smaller of the
	// model's window and the evidence budget: msg[0] is the system turn, msg[-1] the user
	// turn. The fit must not synthesize an extra system turn.
	systemTurn, userTurn := fitComposePrompt(prompt.system, prompt.user, deps.MaxLength)
	userMsg := schema.UserMessage(userTurn)
	if len(deps.UserImages) > 0 {
		// The direct fallback is called with the original multimodal messages: attach the
		// vision-gated images to the final-answer user message so the compose model can see
		// them.
		userMsg = multimodalUserMsg(prompt.user, deps.UserImages)
	}
	// The compose call samples at the configured temperature, defaulting to 0.3; the graph
	// runs with no per-run override, so 0.3 applies.
	reply, err := modelWithTemperature(deps.Model, answerTemperature).Complete(callCtx, []schema.Message{
		*schema.SystemMessage(systemTurn),
		*userMsg,
	}, nil)
	if err != nil {
		logger.Printf("[Composing the answer] composition failed: %v", err)
		return AnswerResult{Answer: answerErrorFallback, Failed: true}
	}
	return AnswerResult{Answer: cleanAnswer(reply.Content), Partial: partial}
}

// fitComposePrompt is the compose-time fit: min(model window, evidence budget). The fit
// normalizes a non-positive budget to 8192, returns the
// pair untouched when it already fits, then trims by TOKENS (trim_content):
// the system share branch (>0.8 of the total) preserves the user turn first,
// otherwise the system turn is preserved first and the user turn gets the
// remainder. form_message is exactly the [system, user]
// pair, so no extra system turn is synthesized here.
func fitComposePrompt(system, user string, maxLength int) (string, string) {
	budget := maxLength
	if budget > evidenceBudgetTokens {
		budget = evidenceBudgetTokens
	}
	if budget <= 0 {
		// message_fit_in normalizes a non-positive max_length to 8192
		// .
		budget = 8192
	}
	ll := tokenizer.NumTokensFromString(system)
	ll2 := tokenizer.NumTokensFromString(user)
	if ll+ll2 < budget {
		return system, user
	}
	if ll+ll2 <= 0 {
		// message_fit_in's degenerate branch: token counts are zero — keep the
		// content unchanged rather than trimming blindly.
		return system, user
	}
	if float64(ll)/float64(ll+ll2) > 0.8 {
		// System-dominated prompt: the USER turn is preserved first.
		preservedLast := min(ll2, budget)
		user = tokenizer.TrimContentToTokenLimit(user, preservedLast)
		remaining := max(0, budget-preservedLast)
		system = tokenizer.TrimContentToTokenLimit(system, remaining)
		return system, user
	}
	preservedSystem := min(ll, budget)
	system = tokenizer.TrimContentToTokenLimit(system, preservedSystem)
	remaining := max(0, budget-preservedSystem)
	user = tokenizer.TrimContentToTokenLimit(user, remaining)
	return system, user
}

// answerPrompt is the terminal node's input: the ranked evidence under its token
// budget plus the question and any research findings. Shared by the one-shot and
// the streaming compose so both render exactly the same prompt.
type answerPrompt struct {
	system  string
	user    string
	partial bool
}

// noEvidenceWithSummary / noEvidenceWithoutSummary are the no-evidence instructions.
// Unlike the empty_response short circuit they apply when composition still goes ahead (no
// empty_response configured), and they tell the model to degrade honestly rather than
// guess.
const (
	noEvidenceWithSummary = "The retrieved passages are limited. Answer as completely as possible " +
		"from the Research Summary below, using the known facts; where a " +
		"specific number/entity is missing, say what is known and avoid " +
		"flatly refusing to answer.\n"
	noEvidenceWithoutSummary = "No supporting evidence was retrieved. State clearly that the available " +
		"sources are insufficient, and do not answer from general knowledge.\n"
)

// ComposeAnswerStream is ComposeAnswer for models that can emit incrementally:
// it renders the same prompt, forwards each piece as it arrives, and returns the
// assembled answer. A streaming failure is returned so the caller can fall back
// to the one-shot call.
//
// emptyResult is the state's empty_result term of
// `no_evidence = abstain or empty_result or not chunks` — the graph compose
// path forwards the state's value (always True there; see formalizeAnswerNode).
func ComposeAnswerStream(ctx context.Context, deps AnswerDeps, model harness.StreamingSessionModel, kb *harness.Kbinfos, question string, partial, emptyResult bool, onDelta func(delta string, isThink bool) error) (AnswerResult, error) {
	logger := deps.Logger
	if logger == nil {
		logger = _LOG
	}
	if model == nil {
		return AnswerResult{Answer: "", Failed: true}, nil
	}
	chunks := []map[string]any{}
	if kb != nil {
		chunks = kb.Chunks
	}
	// Same no-evidence rule as the one-shot path: the prompt must carry the degradation
	// instruction even when no empty_response short-circuits.
	noEvidence := emptyResult || len(chunks) == 0
	// The compose kickoff line. The
	// streaming path has no separate `abstain` signal (Go threads only
	// partial/emptyResult), so the abstain note term cannot fire here.
	note := ""
	if partial {
		note = " — partial answer, some gaps remain"
	}
	logger.Printf("[Composing the answer] Writing the final answer to %q from %d gathered passage(s)%s.",
		trunc(question, 60), len(chunks), note)
	prompt := deps.answerPromptWithEvidence(kb, question, partial, noEvidence)
	if noEvidence && deps.EmptyResponse != "" {
		return AnswerResult{Answer: deps.EmptyResponse, NoEvidence: true}, nil
	}
	// The record the compose prompt actually carries, content included (first 3000 chars), so
	// a run that answered without the slot facts is diagnosable from the log alone.
	preSummary := ""
	if kb != nil {
		preSummary = kb.PreSummary
	}
	// Both the prompt and this log go through composedRecord, so the line below
	// reports the record block the answer actually carried.
	record := composedRecord(kb)
	logger.Printf("[Formalize][record] question=%q record_len=%d draft_summary_len=%d evidence_len=%d using=%s\nrecord=%q",
		trunc(question, 160), len(record), len(preSummary), len(prompt.user), recordSource(record),
		truncateRunes(record, 3000))

	callCtx, cancel := context.WithTimeout(ctx, deadlineToDuration(answerTimeoutS))
	defer cancel()

	// Same prompt fit as the one-shot path: composition runs once and streams from the
	// fitted messages.
	systemTurn, userTurn := fitComposePrompt(prompt.system, prompt.user, deps.MaxLength)
	userMsg := schema.UserMessage(userTurn)
	if len(deps.UserImages) > 0 {
		// Same as ComposeAnswerWith: attach the vision-gated images so the
		// compose model sees them on the non-outer path.
		userMsg = multimodalUserMsg(prompt.user, deps.UserImages)
	}
	// Temperature: the production streaming carrier
	// (harness.InvokerSessionModel.StreamComplete) samples at 0.3 internally —
	// the same value answer_conf carries here; SessionModel's streaming
	// surface has no per-call temperature, so other carriers run at their own default
	// (a known limitation, flagged in the port notes).
	reply, err := model.StreamComplete(callCtx, []schema.Message{
		*schema.SystemMessage(systemTurn),
		*userMsg,
	}, nil, func(delta string, isThink bool) error {
		if onDelta == nil || delta == "" {
			return nil
		}
		return onDelta(delta, isThink)
	})
	if err != nil {
		logger.Printf("[Composing the answer] streaming composition failed: %v", err)
		return AnswerResult{Answer: "", Failed: true}, err
	}
	if reply == nil {
		return AnswerResult{Answer: "", Failed: true}, errors.New("streaming composition returned no reply")
	}
	return AnswerResult{Answer: cleanAnswer(reply.Content), Partial: partial}, nil
}

// composeSystem builds the system prompt, applying the precedence rules.
//
// LANGUAGE IS DELIBERATELY LEFT OVERRIDABLE: "answer in the same language as the
// question" is exactly the rule a user setting "answer in English" means to
// replace, so it must not be listed as protected. What stays protected is the
// evidence contract: citing sources, answering the exact attribute asked for,
// and never substituting prior knowledge for missing evidence.
func (d AnswerDeps) composeSystem() string {
	// The citation rules are the citation_prompt template with an optional user-defined
	// override (RAGConfig.CiteRules).
	rules := prompts.CitationPrompt(d.CiteRules)
	system := strings.ReplaceAll(harness.FinalAnswerSystem, "{cite_rules}", rules)
	if sp := strings.TrimSpace(d.SystemPrompt); sp != "" {
		system = fmt.Sprintf("%s\n\n# Assistant configuration (set by the user)\n%s\n\nFollow the configuration above for language, tone, style, format and any other presentational instruction, including where it overrides the language rule above. Where it conflicts with the citation rules, attribute fidelity, or the requirement to answer only from the provided evidence, those three take precedence.",
			system, sp)
	}
	return system
}

// cleanAnswer strips a leading thinking preamble from the composed answer.
func cleanAnswer(s string) string {
	return strings.TrimSpace(reAnswerThink.ReplaceAllString(s, ""))
}

// ComposeNaiveAnswer: answer composition
// (lines 1555-1568): one retrieve pass, then a single composed answer over the
// top chunks.
//
// Unlike ComposeAnswer this does NOT use kb_prompt and does NOT use
// FinalAnswerSystem — the naive path renders a flat "[i] content" list truncated to the
// first 1500 chars of each chunk, capped at 8 chunks, and sends it under a short fixed
// system prompt.
//
// The messages are run through message_fit_in against
// deps.MaxLength before the call.
func ComposeNaiveAnswer(ctx context.Context, deps AnswerDeps, chunks []map[string]any, question string) AnswerResult {
	logger := deps.Logger
	if logger == nil {
		logger = _LOG
	}
	if len(chunks) == 0 {
		if deps.EmptyResponse != "" {
			return AnswerResult{Answer: deps.EmptyResponse, NoEvidence: true}
		}
		return AnswerResult{NoEvidence: true}
	}

	// a flat "[i] content" list over the first 8 chunks, each
	// truncated to 1500 characters.
	var parts []string
	for i, c := range chunks {
		if i >= naiveEvidenceChunkCap {
			break
		}
		content := harness.ChunkTextOf(c)
		if len(content) > naiveEvidenceCharCap {
			content = content[:naiveEvidenceCharCap]
		}
		parts = append(parts, fmt.Sprintf("[%d] %s", i+1, content))
	}
	evidence := strings.Join(parts, "\n\n")
	fallback := evidence
	if len(fallback) > naiveFallbackChars {
		fallback = fallback[:naiveFallbackChars]
	}

	if deps.Model == nil {
		// Calling a nil model would panic; return the evidence the same way a failed call
		// does.
		return AnswerResult{Answer: fallback, Failed: true}
	}

	callCtx, cancel := context.WithTimeout(ctx, deadlineToDuration(answerTimeoutS))
	defer cancel()

	// message_fit_in(form_message(system, user), max_length).
	fitted, fitErr := chat.FitMessages(naiveAnswerSystem, []schema.Message{
		*schema.UserMessage(fmt.Sprintf("Question: %s\n\nEvidence:\n%s", question, evidence)),
	}, deps.MaxLength)
	if fitErr != "" {
		logger.Printf("[Naive RAG] prompt fitting failed: %s", fitErr)
		return AnswerResult{Answer: fallback, Failed: true}
	}

	// async_chat(msg[0]["content"], msg[1:], answer_conf) — the
	// fitted system text is the first entry and the rest is the history.
	// Splitting explicitly keeps the call shape: Complete takes the two parts re-joined.
	system := naiveAnswerSystem
	history := fitted
	if len(fitted) > 0 && fitted[0].Role == schema.System {
		system = fitted[0].Content
		history = fitted[1:]
	}
	messages := make([]schema.Message, 0, 1+len(history))
	messages = append(messages, *schema.SystemMessage(system))
	messages = append(messages, history...)

	reply, err := modelWithTemperature(deps.Model, naiveAnswerTemperature).Complete(callCtx, messages, nil)
	if err != nil {
		// on failure yield the raw evidence, not an error.
		logger.Printf("[Naive RAG] composition failed: %v", err)
		return AnswerResult{Answer: fallback, Failed: true}
	}
	// str(ans or "").strip or empty_response — an empty answer
	// degrades to the configured empty response.
	answer := cleanAnswer(reply.Content)
	if answer == "" {
		answer = deps.EmptyResponse
	}
	return AnswerResult{Answer: answer}
}
