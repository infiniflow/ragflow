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

package agentic_rag

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/agent/chat"
	"ragflow/internal/rag/agentic-rag/runtime"
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
		fmt.Fprintf(&ev, "[%d] %s", i+1, runtime.TruncateRunes(runtime.ChunkTextOf(chunks[i]), draftChunkChars))
	}
	evidence := ev.String()
	if evidence == "" {
		return ""
	}

	// no model -> the raw evidence, capped at 4000.
	mdl := deps.Model // the innermost chat model.
	if mdl == nil {
		return runtime.TruncateRunes(evidence, draftFallbackChars)
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
		return runtime.TruncateRunes(evidence, draftFallbackChars)
	}
	answer := ""
	if reply != nil {
		answer = strings.TrimSpace(reply.Content)
	}
	if answer == "" {
		answer = evidence
	}
	// (ans or evidence)[:6000] — the draft's block bound comes from the delivery table (see
	// runtime.StageDraft), so it is the same number the rest of the run reads.
	return runtime.TruncateRunes(answer, runtime.StageChars(runtime.StageDraft))
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
func NaiveRAG(ctx context.Context, deps RAGTools, req runtime.RunRequest, kb *runtime.Kbinfos, resp *RunResponse, logger *log.Logger) {
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
		chunks, aggs = runtime.RetrieveSearch(ctx,
			searchDepsFor(ctx, deps, req, req.DatasetIDs, req.TenantID, kb, logger),
			runtime.SearchParams{
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
func runDirectFallback(ctx context.Context, deps RAGTools, req runtime.RunRequest, kb *runtime.Kbinfos, logger *log.Logger) {
	chunks, aggs := runtime.HybridSearch(ctx,
		searchDepsFor(ctx, deps, req, req.DatasetIDs, req.TenantID, kb, logger),
		runtime.SearchParams{
			Question:    req.Question,
			Keywords:    req.Keywords,
			UseCompiled: req.UseCompiled,
			TopN:        req.TopN,
			// The retrieve channel.
			Channel: runtime.ChannelRetrieve,
		})
	kb.Merge(chunks, aggs)
}

// Final answer composition (the graph's last node).
//
// Every thinking mode ends here: it turns the gathered evidence into a grounded, cited
// answer in the user's language. The evidence-block and citation-rule prompts it renders
// (kb_prompt / citation_prompt) live in internal/rag/prompts; the system-prompt texts live
// in the runtime package (report_prompt.go).

const (
	// evidenceBudgetTokens is the token ceiling of the evidence block.
	// The compose path still fits the whole prompt to the model window
	// (AnswerDeps.MaxLength / chat.FitMessages), so this is an upper bound.
	evidenceBudgetTokens = 80000
	// evidencePoolQuota: claim pseudo-chunk
	// cap across the whole first prefetch.
	evidencePoolQuota = 24
	// rawSnippetQuota: the raw-chunk admission budget inside the fan-out, so chunk channels
	// cannot crowd out the denser evidence rows.
	//
	// The pool has NO ceiling (see runtime/kbinfos.go) and nothing renders it whole (the session's seed
	// and the closing composition are both capped), so this number never bought prompt budget — it only
	// decided how much of the opening's recall reached the pool, and the pool is what the session can
	// still reach once its own calls are spent.
	//
	// 400, and it is coupled to the query count: the planner is asked for 8-12 first_queries (see
	// action_initialize_state.md), so a 30-passage budget is ~3 passages per query and a question whose
	// evidence is one table loses it. Measured 2026-09-21 on FRAMES: 8-12 queries with a 30-passage
	// budget scored 0.700, the same queries with 400 scored 0.850 (and 1-4 queries with 30 — the
	// a2110c7af spec — 0.900). The budget is the ceiling on how DEEP one query's recall can reach;
	// changing the query count without it starves every query in the plan.
	rawSnippetQuota = 400
	// citeChunkCap caps chunks rendered as citation reference.
	citeChunkCap = 6
	// answerTimeoutS bounds the answer-composition call.
	answerTimeoutS = 150.0
	// naiveEvidenceChunkCap caps evidence chunks in the naive path.
	naiveEvidenceChunkCap = 8
	// naiveEvidenceCharCap caps each naive evidence chunk.
	naiveEvidenceCharCap = 1500
	// composeStage tags the terminal composition in the log and in the think
	// block.
	composeStage = "Composing the answer"
	// providerErrorSummaryMax bounds the provider error a user reads inside the
	// think block: the message can be a whole JSON body or a wall of retry
	// advice, and the block is a narrative, not a log dump.
	providerErrorSummaryMax = 300
	// graphFailureFallback: last-resort message
	// (run_agentic_rag), used only when the graph failed AND produced nothing.
	graphFailureFallback = "I couldn't complete the search due to an internal error."
	// AgenticRecursionLimit: for the agentic
	// graph: the maximum number of node visits before the graph aborts. Go has
	// no graph runtime, so the loop counts its own node visits against it.
	AgenticRecursionLimit = 60
	// agenticRoundVisits is the number of node visits one research round costs: ONE. A round
	// used to cost three visits (rag_agent → draft → sca), so counting one would have let the
	// run do roughly 3x the work before the guard tripped — and the guard exists to bound WORK,
	// which is now a single node.
	agenticRoundVisits = 1
	// lowRecursionLimitBase is the floor of `max(25, max_loops * 8)` for the non-agentic
	// graph.
	lowRecursionLimitBase = 25
)

// FinalAnswerSystem and PartialAnswerPreamble are defined in the runtime package
// (runtime/report_prompt.go) and re-used here via the runtime import rather than
// duplicated.

// AnswerDeps are the dependencies of ComposeAnswerWith.
type AnswerDeps struct {
	// Model drives the composition call.
	Model runtime.SessionModel
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

type finalizeAnnouncedKey struct{}

// markFinalizeAnnounced records that [Finalize] has been reported for this run's
// composition.
func markFinalizeAnnounced(ctx context.Context) context.Context {
	return context.WithValue(ctx, finalizeAnnouncedKey{}, true)
}

// finalizeAnnounced reports whether [Finalize] was reported for this composition.
func finalizeAnnounced(ctx context.Context) bool {
	v, _ := ctx.Value(finalizeAnnouncedKey{}).(bool)
	return v
}

// sessionAnswer returns the answer the RESEARCH SESSION wrote, when it is usable: the session
// that read the passages is the answerer (see the design's R2), so its text IS the answer.
//
// It exists as its own function because there are TWO composition entry points — the streaming
// one (ComposeAnswerStream) and the one-shot one (ComposeAnswerWith) — and the check used to
// live in only one of them. That is how 15 of 18 answered requests had their session answer
// thrown away: production takes the streaming path (deps.AnswerSink != nil), which had no such
// branch, so the run's real answer — written against passages the model had read — was replaced
// by a composition call that had read none of them, and every citation was then decided by the
// handful of blocks that call happens to render (measured 2026-09-20: rendered_blocks=6 while
// the pool held up to 213 passages). This helper is consulted by the CALLER, before it picks a
// path, so both paths get it.
//
// The gate is the POOL, not the caller's empty_result flag: that flag is the compose prompt's
// no-evidence HEDGE, and the graph sets it TRUE by construction on every round (see the note at
// the formalize_answer node), so gating on it suppressed this answer everywhere — the fix above
// still logged nothing while 20 of 20 rounds wrote an answer (measured 2026-09-20, 19:07 run:
// "Using the answer the research session wrote" 0, compose ran 22 times). A session answer
// cannot exist without evidence anyway: the registry it cites is built from the passages it was
// shown, so `len(kb.Chunks) == 0` is exactly the case where it must not stand.
//
// Returning the text is not enough: the caller must also publish the session's own registry as
// the citation list (see useSessionAnswer), because the [ID:n] markers the session wrote index
// into the numbers it was shown.
func sessionAnswer(kb *runtime.Kbinfos, abstain bool) (string, bool) {
	if kb == nil || abstain || len(kb.Chunks) == 0 {
		return "", false
	}
	ans := strings.TrimSpace(kb.SessionAnswer)
	if ans == "" {
		return "", false
	}
	return ans, true
}

// sessionAnswerBlocked says WHY a written session answer is not being used, for the log. Empty
// means "there was no session answer to use".
func sessionAnswerBlocked(kb *runtime.Kbinfos, abstain bool) string {
	if kb == nil || strings.TrimSpace(kb.SessionAnswer) == "" {
		return ""
	}
	switch {
	case abstain:
		return "the run abstained"
	case len(kb.Chunks) == 0:
		return "the evidence pool is empty"
	}
	return ""
}

// citationIndexRE matches a citation marker the model wrote, capturing whatever it put inside.
var citationIndexRE = regexp.MustCompile(`(\s*)\[ID[:：]\s*([^\]]*?)\s*\]`)

// looseHandleRE matches a source reference the model may write where a [ID:n] handle belongs: a
// chunk id (16 hex) or a document id (32 hex) — bare, in backticks, or introduced by "doc" /
// "document" — which is how a tool result and metadata_search print them (measured 2026-09-22:
// “ `6e9e890eb6d944fda75d71ed5e6f8802` “ and `doc 0ee41271ba9b42e9b73618054fc351c7`).
//
// The optional prefix and the backticks are INSIDE the match on purpose: the replacement has to be a
// bare [ID:n]. Matching only the id left the model's own wording in place ("（doc [ID:0]）"), and the
// delivery contract has no such thing as a "doc" citation — only [ID:i] (see citation_prompt.md).
//
// The 32-hex alternative comes first so a document id is not read as its own first half, and the
// boundaries keep a longer hex run from being cut into pieces.
var looseHandleRE = regexp.MustCompile("(?i)`?\\b(?:documents?|docs?)?\\s*[:：]?\\s*([0-9a-f]{32}|[0-9a-f]{16})\\b`?")

// resolveLooseHandles is the fallback for an answer that cites NOTHING the registry can resolve.
//
// Why it exists (measured 2026-09-22, two runs in a row on the same assistant): the contract asks the
// model to put "the handle of the passage behind a fact" — the [ID:n] a seed block prints, or the ref
// number a tool result prints — and the model wrote something else instead. The first run answered a
// comparison question and wrote the raw chunk id out of a tool result (`6e9e890eb6d944fda75d71ed5e6f8802`);
// the second answered a DOCUMENT-LEVEL question ("which documents were indexed on 2026-09-20") and wrote
// `doc 0ee41271ba9b42e9b73618054fc351c7` beside a `[来源：metadata_search 结果]` tag. Both are honest
// source references and both resolved to nothing: the seed's handles cover PASSAGES, while metadata_search
// returns doc_ids and no passages at all — so a document-level answer has no handle to write, and the user
// was shown an answer with zero citations (citation markers: {"raw_markers": [], "resolved_blocks": 0}).
//
// The ids are therefore matched back against the POOL: a chunk id resolves to itself, a doc id to the
// first chunk of that document (the unit the client can open). ONLY ids the pool actually holds are
// accepted — the same rule the [ID:n] path applies, where a marker naming no published passage is
// dropped — and the id in the text is rewritten to the compact number the client indexes, so the
// reader gets a citation it can click rather than a uuid it cannot.
//
// It runs only when nothing resolved the proper way (see useSessionAnswer): an answer that cites by
// handle is never touched by it.
func resolveLooseHandles(kb *runtime.Kbinfos, ans string) (string, []string) {
	if kb == nil || ans == "" {
		return ans, nil
	}
	chunks := kb.Chunks
	if len(chunks) == 0 {
		return ans, nil
	}
	byChunkID := make(map[string]map[string]any, len(chunks))
	byDocID := make(map[string]map[string]any, len(chunks))
	for _, c := range chunks {
		if id := runtime.ChunkIDOf(c); id != "" {
			if _, seen := byChunkID[id]; !seen {
				byChunkID[id] = c
			}
		}
		if d := runtime.DocIDOf(c); d != "" {
			if _, seen := byDocID[d]; !seen {
				byDocID[d] = c
			}
		}
	}
	cited := make([]string, 0, 4)
	order := make(map[string]int, 4)
	out := looseHandleRE.ReplaceAllStringFunc(ans, func(m string) string {
		sub := looseHandleRE.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		id := strings.ToLower(strings.TrimSpace(sub[1]))
		c, ok := byChunkID[id]
		if !ok {
			c, ok = byDocID[id]
		}
		if !ok {
			// An id this run never published: leave it as the model wrote it. Nothing is dropped
			// here — this fallback adds citations, it does not remove text.
			return m
		}
		cid := runtime.ChunkIDOf(c)
		if cid == "" {
			return m
		}
		k, seen := order[cid]
		if !seen {
			k = len(cited)
			order[cid] = k
			cited = append(cited, cid)
		}
		return fmt.Sprintf("[ID:%d]", k)
	})
	return out, cited
}

// useSessionAnswer installs the session's answer with the handles the model cited, RENUMBERED COMPACTLY.
//
// The model cites with the handles the run printed beside its evidence ([ID:k] — the seed's numbering,
// which runs over everything the scan retrieved, 265 of them in one 三国/关羽 run). Those numbers belong
// to the RETRIEVAL, not to the answer: an answer that rests on sixteen passages should not carry
// [ID:265] in its prose. So the code does one mechanical thing — the first passage the ANSWER cites
// becomes [ID:0], the next passage it has not cited yet [ID:1], and so on — and kb.CiteChunkIDs is that
// list, in that order, which is the list the client resolves the markers against.
//
// The handle is not inferred from the prose: the model says which passage each claim rests on (that is
// what the handle it wrote IS), and a marker naming no passage this run published is dropped. ONE
// fallback runs before an answer is written off — resolveLooseHandles, for the case where the only
// source references the model could write are chunk ids or doc ids (a document-level question has no
// passage handle to write). An answer whose references resolve neither way still gets the registry.
func useSessionAnswer(kb *runtime.Kbinfos, resp *RunResponse, ans string) {
	if kb != nil {
		refs := kb.SessionEvidenceRefs
		var cited []string
		compact := map[string]int{}
		ans = citationIndexRE.ReplaceAllStringFunc(ans, func(m string) string {
			sub := citationIndexRE.FindStringSubmatch(m)
			if len(sub) < 3 {
				return ""
			}
			space := sub[1]
			n, err := strconv.Atoi(strings.TrimSpace(sub[2]))
			if err != nil || n < 0 || n >= len(refs) {
				// Not a handle this run published: a chunk id, a figure, a number that names nothing.
				return ""
			}
			id := refs[n]
			k, seen := compact[id]
			if !seen {
				k = len(cited)
				compact[id] = k
				cited = append(cited, id)
			}
			return space + fmt.Sprintf("[ID:%d]", k)
		})
		if len(cited) > 0 {
			kb.CiteChunkIDs = cited
		} else if looseAns, loose := resolveLooseHandles(kb, ans); len(loose) > 0 {
			// Nothing resolved as a handle, but the answer names passages or documents by their own
			// ids — the shape a document-level question produces (see resolveLooseHandles). Take
			// them, rather than publish a citation list that no marker in the text points at.
			//
			// Said out loud, because this path is otherwise INVISIBLE: the chat pipeline's citation
			// line reports the markers the model wrote, and on this path it wrote none — so without
			// this line the run looks like (and used to be) an answer with zero citations.
			_LOG.Printf("[Citations] no [ID:n] handle resolved; %d id(s) the answer named were "+
				"resolved against the pool instead (the answer had no handle to write).", len(loose))
			ans = looseAns
			kb.CiteChunkIDs = loose
		} else {
			kb.CiteChunkIDs = append([]string(nil), refs...)
		}
	}
	if resp != nil {
		resp.Answer = ans
	}
}

// ComposeAnswerWith turns the gathered evidence into a grounded, cited answer.
//
// Behaviour, in order:
//  1. no evidence + configured empty_response → return it WITHOUT calling the LLM;
//  2. rank chunks by similarity, keep the top citeChunkCap as citation reference;
//  3. render the evidence block under the token budget (kb_prompt);
//  4. prepend the fact-preserving pre_summary (the SCA-reviewed draft) when set;
//  5. call the model with FINAL_ANSWER_SYSTEM + the composed user content.
//
// "No evidence" is three distinct terms, not one: `abstain` is the run's own decision,
// `emptyResult` is the agentic loop's "nothing was found" signal, and an empty pool is the
// dataset happening to return nothing. All three steer the prompt's degradation
// instructions; only the first two are the run's own verdicts.
func ComposeAnswerWith(ctx context.Context, deps AnswerDeps, kb *runtime.Kbinfos, question string, partial, abstain, emptyResult bool) AnswerResult {
	logger := deps.Logger
	if logger == nil {
		logger = _LOG
	}
	chunks := []map[string]any{}
	if kb != nil {
		chunks = kb.Chunks
	}

	// The session that read the evidence writes the answer (see the design's R2). When it did,
	// that text IS the answer: composing again would hand the question to a model that never saw
	// the passages, and it would have to re-derive — by similarity, after the fact — the [ID:n]
	// markers the session wrote against the ref numbers it was actually shown. The registry the
	// session built is handed over instead, so its own citations resolve verbatim.
	if ans, ok := sessionAnswer(kb, abstain); ok {
		useSessionAnswer(kb, nil, ans)
		step(ctx, logger, "Composing the answer",
			"Using the answer the research session wrote (%d character(s)); %s in the citation registry.",
			utf8.RuneCountInString(ans), runtime.CountOf(len(kb.CiteChunkIDs), "passage"))
		return AnswerResult{Answer: ans}
	}
	if why := sessionAnswerBlocked(kb, abstain); why != "" {
		// Silence here is what hid the two bugs above: a written answer that is not used must say
		// so, and say why, on the one line a reader has.
		logger.Printf("[Composing the answer] the session's answer will not stand (%s); composing instead.", why)
	}

	started := time.Now()
	// The kickoff step is suppressed when the graph already reported [Finalize]
	// (finalizeAnnounced): the verdict and the evidence have been said, and a line
	// repeating their numbers only made the block longer. The paths that have no
	// [Finalize] — the low graph's last node, every fallback compose — keep it.
	if !finalizeAnnounced(ctx) {
		step(ctx, logger, composeStage, "Composing the answer from %s.",
			runtime.CountOf(len(chunks), "gathered passage"))
	}

	// The step kept below either way is the no-evidence short circuit: "no model was
	// called" is a fact no other line carries.

	// 1. No-evidence short circuit.
	// _compose_answer_from_evidence: no_evidence = abstain or empty_result or not chunks.
	noEvidence := abstain || emptyResult || len(chunks) == 0
	if noEvidence && deps.EmptyResponse != "" {
		step(ctx, logger, composeStage, "No supporting evidence was found, so the configured empty response is returned without calling the model.")
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
	callCtx, cancel := context.WithTimeout(ctx, runtime.DeadlineToDuration(answerTimeoutS))
	defer cancel()

	logger.Printf("[Formalize][record] question=%q record_len=%d draft_summary_len=%d evidence_len=%d using=%s\nrecord=%q",
		trunc(question, 160), len(record), len(preSummary), len(prompt.user), recordSource(record),
		runtime.TruncateRunes(record, 3000))

	logger.Printf("[Formalize][pre_summary] question=%q pre_summary_len=%d evidence_len=%d\npre_summary=%q",
		trunc(question, 160), len(preSummary), len(prompt.user), runtime.TruncateRunes(preSummary, 3000))

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
		// Deliberately NOT Python's bare "I'm sorry…" sentence. The ANSWER
		// carries the provider's own message in the classic `**ERROR**: …` shape
		// the rest of the chat pipeline uses for its failures, so an agentic
		// failure reads exactly like a naive-mode one instead of hiding the
		// cause — an exhausted quota or a provider 5xx used to look like a RAG
		// bug. The think block gets the same text as a reasoning step, and the
		// developer log line stays byte-for-byte what it was (the detail half of
		// StageLineDetail).
		summary := providerErrorSummary(err)
		runtime.StepsFrom(ctx).StageLineDetail(logger, composeStage,
			"Composing the answer failed: "+summary,
			fmt.Sprintf("composition failed: %v", err))
		return AnswerResult{Answer: errorAnswerText(err), Failed: true}
	}
	answer := cleanAnswer(reply.Content)
	logComposeDone(logger, started, answer, len(chunks))
	return AnswerResult{Answer: answer, Partial: partial}
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

// ComposeAnswerStream is ComposeAnswerWith for models that can emit incrementally:
// it renders the same prompt, forwards each piece as it arrives, and returns the
// assembled answer. A streaming failure is returned so the caller can fall back
// to the one-shot call.
//
// emptyResult is the state's empty_result term of
// `no_evidence = abstain or empty_result or not chunks` — the graph compose
// path forwards the state's value (always True there; see formalizeAnswerNode).
func ComposeAnswerStream(ctx context.Context, deps AnswerDeps, model runtime.StreamingSessionModel, kb *runtime.Kbinfos, question string, partial, emptyResult bool, onDelta func(delta string, isThink bool) error) (AnswerResult, error) {
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
	started := time.Now()
	// Same gated kickoff step as the one-shot path above (finalizeAnnounced). The
	// streaming path has no separate `abstain` signal (Go threads only
	// partial/emptyResult), so the abstain note term cannot fire here.
	if !finalizeAnnounced(ctx) {
		step(ctx, logger, composeStage, "Composing the answer from %s.",
			runtime.CountOf(len(chunks), "gathered passage"))
	}
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
		runtime.TruncateRunes(record, 3000))

	callCtx, cancel := context.WithTimeout(ctx, runtime.DeadlineToDuration(answerTimeoutS))
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
	// (runtime.InvokerSessionModel.StreamComplete) samples at 0.3 internally —
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
	answer := cleanAnswer(reply.Content)
	logComposeDone(logger, started, answer, len(chunks))
	return AnswerResult{Answer: answer, Partial: partial}, nil
}

// composeSystem builds the system prompt, applying the precedence rules.
//
// LANGUAGE IS DELIBERATELY LEFT OVERRIDABLE: "answer in the same language as the
// question" is exactly the rule a user setting "answer in English" means to
// replace, so it must not be listed as protected. What stays protected is the
// evidence contract: citing sources, answering the exact attribute asked for,
// and never substituting prior knowledge for missing evidence.
// citeChunkIDsAt returns the citation id of every chunk that rendered a block, in
// render order, from the render's source indices.
//
// It takes the SOURCE INDICES rather than the chunk list because the renderer emits
// no block for a chunk without content: one id per RENDERED block, or the published
// list runs ahead of the numbering at every skip and each marker past it lands on
// the wrong passage. A chunk without an id keeps its slot as an empty string —
// dropped, it would shift the later blocks the same way (citePoolIdx resolves "" to
// -1, so that block's own citation is dropped, but the blocks after it stay put).
func citeChunkIDsAt(chunks []map[string]any, sources []int) []string {
	out := make([]string, len(sources))
	for i, src := range sources {
		if src >= 0 && src < len(chunks) {
			out[i] = runtime.ChunkIDOf(chunks[src])
		}
	}
	return out
}

// logComposeDone reports the compose's own cost and size.
//
// It is a LOG line, not a step: the compose finishes after the answer has already
// started streaming, and the chat pipeline drops think lines emitted past that point
// (so a thought block never reopens after the answer) — a "composed …" step visible
// in the non-streaming path but not the streaming one would be worse than none.
//
// The count is in RUNES, not bytes: a Chinese answer is two to three times its rune
// count in bytes, and this number is read as "how much text came out".
func logComposeDone(logger *log.Logger, started time.Time, answer string, chunks int) {
	logger.Printf("[Composing the answer] composed %s from %s in %.1fs",
		runtime.CountOf(utf8.RuneCountInString(answer), "char"),
		runtime.CountOf(chunks, "gathered passage"),
		time.Since(started).Seconds())
}

// zeroBasedEvidenceRule states the evidence numbering for the compose model.
//
// The blocks this call renders are numbered from 0 because the client resolves a
// marker by indexing reference.chunks with its number, and the answer reaches that
// client as the model streams it. Saying so guards against the "the first source is
// 1" habit: a model that falls back on it cites the second passage and leaves the
// first one unreachable (its markers are still openable, they just point one block
// off).
//
// It belongs at the call site, not in CitationPrompt: that text is shared with the
// agent canvas, which numbers its blocks by hash id (component/prompts/citation.go).
const zeroBasedEvidenceRule = "\n\n# Evidence ids\n" +
	"The evidence blocks below are numbered from 0: the FIRST block is [ID:0]. " +
	"Cite the id printed at the start of the block you used, exactly as printed — " +
	"do not renumber the blocks yourself."

func (d AnswerDeps) composeSystem() string {
	// The citation rules are the citation_prompt template with an optional user-defined
	// override (RAGConfig.CiteRules), plus the 0-based evidence-numbering rule the
	// compose path renders with (see zeroBasedEvidenceRule).
	rules := prompts.CitationPrompt(d.CiteRules) + zeroBasedEvidenceRule
	system := strings.ReplaceAll(runtime.FinalAnswerSystem, "{cite_rules}", rules)
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
// Unlike ComposeAnswerWith this does NOT use kb_prompt and does NOT use
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
		content := runtime.ChunkTextOf(c)
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

	callCtx, cancel := context.WithTimeout(ctx, runtime.DeadlineToDuration(answerTimeoutS))
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
