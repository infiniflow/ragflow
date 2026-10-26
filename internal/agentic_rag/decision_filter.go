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
	"strconv"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/agentic_rag/decisionhead"
	"ragflow/internal/common"
)

// The filter's dependencies, held as variables so the default test tier can
// exercise the drop path — and the [ID:n] ordering it protects — without a
// 1.7 GB checkpoint on disk.
//
// Each one first consults the decision-model backend and only then falls back
// to the decisionhead package. The order is deliberate: selecting the backend must not
// require unsetting anything, and a deployment that does not select it must be
// byte-for-byte the run it was before this dispatch existed. The backend is
// resolved lazily (and memoised) rather than at init because the env is loaded
// after package init.
// The four dispatch points below choose the judge for each call.
//
// They resolve the backend from the run's context (chat -> tenant -> env) rather
// than from a process-wide singleton, so two tenants can use two different
// decision models — or none — in the same process. A caller that publishes no
// scope gets the env backend, which is what a run without a tenant got before
// this existed, so the fallback is the old behaviour rather than a new one.
var (
	decisionAvailable = func(ctx context.Context) bool {
		if b := resolveDecisionBackend(ctx); b.available() {
			return true
		}
		return decisionhead.Available()
	}
	decisionSplit = func(ctx context.Context, text string) ([]string, string) {
		if b := resolveDecisionBackend(ctx); b.available() {
			// No overflow on this path: the backend judges the passage as one
			// segment, so there is no tail it declined to look at.
			return b.split(text), ""
		}
		return decisionhead.Split(text)
	}
	decisionSplitWhole = func(ctx context.Context, text string, maxTokens int) ([]string, string) {
		if b := resolveDecisionBackend(ctx); b.available() {
			return b.splitWhole(text, maxTokens)
		}
		return decisionhead.SplitWhole(text, maxTokens)
	}
	decisionJudge = func(ctx context.Context, question string, segments []string) []decisionhead.Verdict {
		if b := resolveDecisionBackend(ctx); b.available() {
			return b.judge(ctx, question, segments)
		}
		return decisionhead.Judge(ctx, question, segments)
	}
)

// wholeChunkTokens is the default judging window when the whole-passage policy
// is on: a passage taller than this is judged on its opening.
const wholeChunkTokens = 5000

func wholeChunkEnabled() bool {
	return envBoolDefault(common.EnvDecisionWholeChunk, false)
}

func wholeChunkBudget() int {
	return envIntDefault(common.EnvDecisionWholeChunkTokens, wholeChunkTokens)
}

// keepTopN is how many leading passages of each result set bypass the filter:
// the retrieval's own ranking already selected them, so re-judging them only
// risks discarding the head of a ranking that was already the best signal the
// run had. 0 disables the exemption (the pre-existing all-or-nothing gate).
// defaultKeepTopN is the exemption size a deployment gets when it does not say.
//
// Three, not zero: zero is the all-or-nothing gate, which measured worse than
// everything else tried — it pruned the most text, cost the most tokens (+48.7%
// against the filter being off: the dropped passages come back as re-reads) and
// still lost most of the gold-document evidence. Exempting the leading passages
// bounds that damage, because an exempted passage is never judged: a wrong
// verdict can shorten a result set but can no longer empty it.
const defaultKeepTopN = 3

// keepTopN reads the exemption size. Zero is a MEANINGFUL value here — it
// restores the plain gate — so unlike the other numeric knobs an explicit 0 is
// honoured instead of being read as "unset".
func keepTopN() int {
	raw := strings.TrimSpace(common.GetEnv(common.EnvDecisionKeepTopN))
	if raw == "" {
		return defaultKeepTopN
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return defaultKeepTopN
	}
	return n
}

// envIntDefault reads an integer env var, returning def when unset or
// unparseable, so a typo degrades to the measured default instead of a zero
// budget that would judge every passage on nothing.
func envIntDefault(name string, def int) int {
	v := strings.TrimSpace(common.GetEnv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// The two model-visible exits the sufficiency filter can run on. Deep read is
// the original target; the snippet exit is opt-in because a dropped locate hit
// costs the model an anchor it may have needed before it could deep-read.
const (
	filterSideDeepRead = "deep_read"
	filterSideSnippet  = "snippet"
)

// passage identifies one member of one result set across the filter's layers.
//
// pos is its index in the set the retrieval returned. It is what makes the
// top-N exemption auditable after the fact: with pos recorded, the analysis can
// recompute from a single run which passages any other N would have kept, so the
// curve costs one run instead of one run per N.
type passage struct {
	side       string
	tool       string
	chunkID    string
	docID      string
	chunkIndex int
	pos        int
}

// envBoolDefault reads a boolean env var, returning def when unset or
// unrecognised. The default matters: RAGFLOW_DECISION_FILTER_DEEP_READ defaults on
// so a deployment that only sets RAGFLOW_DECISION_DIR keeps its existing behaviour,
// while RAGFLOW_DECISION_FILTER_SNIPPET defaults off so the locate path is never
// silently weakened by an upgrade.
func envBoolDefault(name string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(common.GetEnv(name))) {
	case "":
		return def
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func deepReadFilterEnabled() bool {
	return envBoolDefault(common.EnvDecisionFilterDeepRead, true)
}

func snippetFilterEnabled() bool {
	// On by default: the locate tools are where most of the discarded evidence
	// comes from, and the deep-read exit alone leaves those hits uncut. An
	// operator who wants the old behaviour sets RAGFLOW_DECISION_FILTER_SNIPPET=0.
	return envBoolDefault(common.EnvDecisionFilterSnippet, true)
}

// snippetFilterTools is the default scope of the snippet exit: the locate tools
// that return CORPUS PASSAGES, which is what the sufficiency head was trained to
// read.
//
// Two exclusions are deliberate and neither is a tuning choice:
//
//   - search_metadata is excluded because it is not a passage exit at all. It
//     renders a document-id selector, so it never reaches this code; listing it
//     here would only pretend to be a guard.
//   - web_search is excluded because its hits carry web text with no corpus
//     document behind them. The head's calibration is corpus-only, and a dropped
//     web hit costs an anchor the corpus cannot replace.
//
// RAGFLOW_DECISION_FILTER_SNIPPET_TOOLS replaces the set when set, so a run can
// widen or narrow the scope without a rebuild.
var snippetFilterTools = map[string]bool{
	searchBm25ChunksToolName:     true,
	searchSemanticChunksToolName: true,
	searchChunksToolName:         true,
	grepChunksToolName:           true,
}

// snippetFilterAppliesTo reports whether the snippet filter may prune results
// from this tool. An unrecognised tool is never pruned: the scope is an
// allowlist, so a tool added later is filtered only when someone says so.
func snippetFilterAppliesTo(tool string) bool {
	if raw := strings.TrimSpace(common.GetEnv(common.EnvDecisionSnippetTools)); raw != "" {
		for _, name := range strings.Split(raw, ",") {
			if strings.EqualFold(strings.TrimSpace(name), tool) {
				return true
			}
		}
		return false
	}
	return snippetFilterTools[tool]
}

// queryContextKey carries the turn's user question through the request context.
//
// The sufficiency decision is asked per (question, passage), but no retrieval
// tool is given the question — it reaches the renderer through the same per-run
// channel the evidence registry uses, so concurrent turns never share one.
type queryContextKey struct{}

// WithQuery returns a copy of ctx carrying the turn's question. An empty
// question is a no-op rather than an empty value: a blank question routes to the
// fallback language and would judge every passage against nothing.
func WithQuery(ctx context.Context, question string) context.Context {
	if strings.TrimSpace(question) == "" {
		return ctx
	}
	return context.WithValue(ctx, queryContextKey{}, question)
}

// queryFrom returns the question carried by ctx, or "" when the caller
// published none.
func queryFrom(ctx context.Context) string {
	q, _ := ctx.Value(queryContextKey{}).(string)
	return q
}

// intentContextKey carries what THIS tool call is trying to find, as the agent
// worded it. It is per-CALL, not per-run, which is why it travels on the call's
// own context rather than alongside the question: a run may issue several
// searches in one turn, each with its own information need.
//
// It exists because relevance to the final question is the wrong test for a
// multi-hop step. A passage can be a necessary link — relevant to "who was
// Sarah Polk's mother", useless for "what is my future wife's name" — and a
// judge asked only the final question drops it. That is the measured failure:
// the gate lost 30% of the labelled evidence (§13) while the accuracy did not
// move, i.e. it was discarding links it could not recognise.
type intentContextKey struct{}

// withIntent returns a copy of ctx carrying this call's stated intent. An empty
// intent is a no-op: a blank intent must not shadow the question.
func withIntent(ctx context.Context, intent string) context.Context {
	if strings.TrimSpace(intent) == "" {
		return ctx
	}
	return context.WithValue(ctx, intentContextKey{}, strings.TrimSpace(intent))
}

// intentFrom returns this call's intent, or "" when the tool published none.
func intentFrom(ctx context.Context) string {
	q, _ := ctx.Value(intentContextKey{}).(string)
	return q
}

// filterChunks drops the deep-read passages the decision model does not
// find sufficient to answer the turn's question.
//
// Granularity is the SUB-CHUNK. A passage longer than the segment budget is cut
// into segments, each is judged on its own, and the useful ones are kept — so a
// half-relevant chunk reaches the model as its relevant half rather than as all
// of it (too much context) or none of it (lost evidence). A passage with no
// useful segment is dropped entirely.
//
// Dropping a passage is what keeps it from consuming an [ID:n] handle: the
// caller runs this BEFORE stamping, so the model only ever cites text it was
// shown.
//
// Every other outcome is passthrough — no question in the context, no judge
// deployed, the exit disabled, a segment the model never ruled on (budget spent,
// cancelled run, failed forward pass). A filter that cannot run must not cost
// the caller its evidence, so a segment that was not ruled on is always kept.
func filterChunks(ctx context.Context, chunks []runtime.RetrievalChunk) []runtime.RetrievalChunk {
	if len(chunks) == 0 || !deepReadFilterEnabled() {
		return chunks
	}
	question := queryFrom(ctx)
	if question == "" || !decisionAvailable(ctx) {
		return chunks
	}
	head := keepTopN()
	var ps []passage
	var texts []string
	for i := head; i < len(chunks); i++ {
		chunk := chunks[i]
		ps = append(ps, passage{
			side:       filterSideDeepRead,
			tool:       listChunksToolName,
			chunkID:    chunk.ID,
			docID:      chunk.DocumentID,
			chunkIndex: chunk.ChunkIndex,
			pos:        i,
		})
		texts = append(texts, chunk.Content)
	}
	verdicts := judgeTexts(ctx, ps, question, texts)

	kept := make([]runtime.RetrievalChunk, 0, len(chunks))
	for i, chunk := range chunks {
		p := passage{
			side:       filterSideDeepRead,
			tool:       listChunksToolName,
			chunkID:    chunk.ID,
			docID:      chunk.DocumentID,
			chunkIndex: chunk.ChunkIndex,
			pos:        i,
		}
		if i < head {
			// Tail pruning, not a gate: the leading passages of the result set
			// are kept whatever the model thinks of them, so the filter can only
			// ever shorten the tail. This is what bounds the damage a wrong
			// verdict can do — it can no longer empty a result set.
			logExempt(ctx, p, chunk.Content)
			kept = append(kept, chunk)
			continue
		}
		v := verdicts[i-head]
		if !v.keep {
			common.InfoCtx(ctx, "agentic_rag: decision dropped chunk",
				zap.String("side", filterSideDeepRead),
				zap.String("tool", listChunksToolName),
				zap.String("chunk_id", chunk.ID),
				zap.String("doc_id", chunk.DocumentID),
				zap.Int("chunk_index", chunk.ChunkIndex))
			continue
		}
		if v.text != "" && v.text != chunk.Content {
			chunk.Content = v.text
		}
		kept = append(kept, chunk)
	}
	return kept
}

// filterSnippetHits drops the locate hits whose model-visible text the judge
// sufficiency model finds insufficient, and trims the survivors to their useful
// segments.
//
// The text judged is exactly what the model would read — the snippet when the
// renderer has one, the chunk body otherwise — because a locate hit is triage
// output, not the document. Keeping a hit's chunk element while emptying its
// snippet would preserve the anchor, but it is not what this filter does: the
// rule is the same as findChunks', so a dropped hit does not consume an [ID:n]
// handle either. This is deliberately opt-in (RAGFLOW_DECISION_FILTER_SNIPPET)
// because a locate hit is also the only way the model learns a document exists.
// snippetJudgeText is the text this filter asks the model about for one locate
// hit: the chunk BODY by default.
//
// The snippet is a window, not the passage — the keyword's own paragraph when
// truncated, the chunk's opening for a BM25 preview hit. Judging it asks the
// model about an excerpt that may not contain what the evidence is, and it is
// also the wrong unit for the threshold: the 0.15 operating point was fitted on
// whole passages, and the deep-read exit already judges whole passages, so the
// body is the one unit both exits can share without re-calibrating per exit.
//
// The cost is that the decision then rests on text the model may not be served.
// That is accepted because the drop is whole-hit: a kept hit is served exactly
// as it was built, and the renderer already tells the model when a snippet is
// truncated and where to read the rest.
//
// RAGFLOW_DECISION_SNIPPET_TEXT=snippet restores the old window-based read,
// which is what the previous arm measured against.
func snippetJudgeText(h snippetHit) string {
	if !strings.EqualFold(strings.TrimSpace(common.GetEnv(common.EnvDecisionSnippetText)), "snippet") {
		if body := strings.TrimSpace(h.chunk.Content); body != "" {
			return h.chunk.Content
		}
	}
	if strings.TrimSpace(h.snippet) != "" {
		return h.snippet
	}
	return h.chunk.Content
}

// decisionConcurrency is how many judgements of one result set may be in flight
// at once. Each one asks about a different passage, so they are independent.

// judgeInParallel runs fn for every k in [0,n) with at most decisionConcurrency
// of them in flight, and returns once all have finished.
//
// The judge is one HTTP round trip per passage, so a result set of sixteen hits
// used to cost the SUM of sixteen latencies — measured at 205ms median, i.e.
// several seconds of a turn spent waiting with nothing to order. The verdicts
// are independent, and the caller assembles them in index order afterwards, so
// concurrency changes no decision and no ordering; it only removes the wait.
// judgedText is one passage's outcome: whether it may reach the model, and the
// text to serve (unchanged unless the split path trimmed it).
type judgedText struct {
	text string
	keep bool
}

// judgeTexts judges a whole result set in ONE dispatch.
//
// A passage-per-call loop made one round trip per hit, and the judge is a hosted
// model that accepts many questions in a single request. Four passages fit in
// the same request as one, at the same latency, so the batching happens where
// the endpoint offers it — not as client-side concurrency, and not by
// concatenating passages into one question (which would make the verdict about
// the pile instead of about each passage).
func judgeTexts(ctx context.Context, ps []passage, question string, texts []string) []judgedText {
	out := make([]judgedText, len(texts))
	for i, t := range texts {
		out[i] = judgedText{text: t, keep: true}
	}
	if len(texts) == 0 {
		return out
	}
	if !wholeChunkEnabled() {
		// The split path judges several segments per passage, so its unit is not
		// the passage and it cannot share a batch keyed by passage index without
		// changing what the threshold was fitted on.
		for i := range texts {
			trimmed, ok := judgeText(ctx, ps[i], question, texts[i])
			out[i] = judgedText{text: trimmed, keep: ok}
		}
		return out
	}
	windows := make([]string, len(texts))
	truncated := make([]bool, len(texts))
	for i, t := range texts {
		w, _ := decisionSplitWhole(ctx, t, wholeChunkBudget())
		if len(w) == 0 {
			continue
		}
		windows[i] = w[0]
		truncated[i] = len(w[0]) < len(t)
	}
	verdicts := decisionJudge(ctx, question, windows)
	for i := range out {
		if windows[i] == "" {
			// Nothing to judge: the passage is short enough to serve whole.
			continue
		}
		if truncated[i] {
			common.InfoCtx(ctx, "agentic_rag: decision window truncated",
				zap.String("side", ps[i].side),
				zap.String("chunk_id", ps[i].chunkID),
				zap.Int("chunk_runes", utf8.RuneCountInString(texts[i])),
				zap.Int("window_runes", utf8.RuneCountInString(windows[i])))
		}
		if i >= len(verdicts) {
			continue
		}
		logDecision(ctx, ps[i], 0, verdicts[i])
		if !verdicts[i].Useful {
			out[i].text = ""
			out[i].keep = false
		}
	}
	return out
}

func filterSnippetHits(ctx context.Context, tool string, hits []snippetHit) []snippetHit {
	if len(hits) == 0 || !snippetFilterEnabled() || !snippetFilterAppliesTo(tool) {
		return hits
	}
	question := queryFrom(ctx)
	if question == "" || !decisionAvailable(ctx) {
		return hits
	}
	head := keepTopN()
	// One dispatch for the whole tail. The exempt head keeps its invariants: it
	// is never sent, and the assembly below is still the only place that decides
	// order, handles and truncation.
	var ps []passage
	var texts []string
	for i := head; i < len(hits); i++ {
		h := hits[i]
		ps = append(ps, passage{
			side:       filterSideSnippet,
			tool:       tool,
			chunkID:    h.chunk.ID,
			docID:      h.chunk.DocumentID,
			chunkIndex: h.chunk.ChunkIndex,
			pos:        i,
		})
		texts = append(texts, snippetJudgeText(h))
	}
	verdicts := judgeTexts(ctx, ps, question, texts)

	kept := make([]snippetHit, 0, len(hits))
	for i, h := range hits {
		p := passage{
			side:       filterSideSnippet,
			tool:       tool,
			chunkID:    h.chunk.ID,
			docID:      h.chunk.DocumentID,
			chunkIndex: h.chunk.ChunkIndex,
			pos:        i,
		}
		if i < head {
			// Same rule as the deep-read exit: the ranked head of a locate result
			// is never second-guessed, only its tail is pruned.
			logExempt(ctx, p, snippetJudgeText(h))
			kept = append(kept, h)
			continue
		}
		v := verdicts[i-head]
		if !v.keep {
			common.InfoCtx(ctx, "agentic_rag: decision dropped snippet",
				zap.String("side", filterSideSnippet),
				zap.String("tool", tool),
				zap.String("chunk_id", h.chunk.ID),
				zap.String("doc_id", h.chunk.DocumentID),
				zap.Int("chunk_index", h.chunk.ChunkIndex))
			continue
		}
		// The judged text may be the chunk BODY, in which case the verdict text
		// is not a snippet and must never be written back: doing so would replace
		// a one-paragraph window with the whole chunk, i.e. widen the context
		// while claiming to shrink it. Only a snippet-derived judgement is a
		// legal replacement, and only when it differs.
		if text := snippetJudgeText(h); text == h.snippet && v.text != h.snippet {
			h.snippet = v.text
		}
		kept = append(kept, h)
	}
	return kept
}

// filterChunk applies the decision to one passage. ok is false when the passage
// carries no text the model found sufficient and must not reach the caller.
//
// Segments the model did not rule on are appended as they are: text past the
// segment bound is evidence nobody paid to score, which is not the same as
// evidence the model rejected.
func filterChunk(ctx context.Context, question string, chunk runtime.RetrievalChunk, p passage) (runtime.RetrievalChunk, bool) {
	if strings.TrimSpace(chunk.Content) == "" {
		return chunk, true
	}
	content, ok := judgeText(ctx, p, question, chunk.Content)
	if !ok {
		return runtime.RetrievalChunk{}, false
	}
	// Nothing was cut: hand the passage back untouched. This is what keeps a run
	// whose filter keeps everything indistinguishable from one that never
	// filtered, and it is the common case for a short, relevant chunk.
	if content != chunk.Content {
		chunk.Content = content
	}
	return chunk, true
}

// judgeText splits text into sub-chunks, asks the model about each, and returns
// the useful ones concatenated. ok is false only when the text carried at least
// one segment and every one of them was ruled unusable; text the model never
// ruled on is always kept, so a filter that could not run never costs evidence.
func judgeText(ctx context.Context, p passage, question, text string) (string, bool) {
	if wholeChunkEnabled() {
		return judgeWholeChunk(ctx, p, question, text)
	}
	segments, overflow := decisionSplit(ctx, text)
	if len(segments) == 0 {
		return text, true
	}
	verdicts := decisionJudge(ctx, question, segments)
	var b strings.Builder
	for i, verdict := range verdicts {
		logDecision(ctx, p, i, verdict)
		if verdict.Useful {
			b.WriteString(verdict.Segment)
		}
	}
	b.WriteString(overflow)
	content := b.String()
	if content == "" {
		return "", false
	}
	return content, true
}

// judgeWholeChunk asks the model about the passage ONCE, on a window truncated
// to the whole-chunk budget, and keeps or drops it as a unit.
//
// The window bounds only what the MODEL was asked about: a passage it accepts is
// returned UNCHANGED, so the text the caller serves is exactly the text the
// caller passed in. That decoupling is the point of the mode — it changes the
// judging window, not what the model is allowed to read.
//
// A window the model never ruled on keeps the passage, like every other
// degraded path.
func judgeWholeChunk(ctx context.Context, p passage, question, text string) (string, bool) {
	window, _ := decisionSplitWhole(ctx, text, wholeChunkBudget())
	if len(window) == 0 {
		return text, true
	}
	truncated := len(window[0]) < len(text)
	verdicts := decisionJudge(ctx, question, window)
	useful := false
	for i, verdict := range verdicts {
		logDecision(ctx, p, i, verdict)
		if verdict.Useful {
			useful = true
		}
	}
	if truncated {
		// Record when the window actually bit: without it, "the model judged the
		// whole passage" cannot be told apart from "it judged the opening".
		common.InfoCtx(ctx, "agentic_rag: decision window truncated",
			zap.String("side", p.side),
			zap.String("chunk_id", p.chunkID),
			zap.Int("chunk_runes", utf8.RuneCountInString(text)),
			zap.Int("window_runes", utf8.RuneCountInString(window[0])))
	}
	if !useful {
		return "", false
	}
	return text, true
}

// logDecision emits the per-segment line the experiment harness consumes: one
// line per judged segment, carrying what the miss rate has to be recomputed
// from — which exit, which chunk, which segment, its size, the forward-pass
// latency, the calibrated probability, and whether it survived the filter.
//
// It is logged at info level on purpose: it only exists when RAGFLOW_DECISION_DIR is
// set, so a deployment that never enabled the filter emits none of it.
func logDecision(ctx context.Context, p passage, index int, verdict decisionhead.Verdict) {
	common.InfoCtx(ctx, "agentic_rag: decision verdict", decisionFields(ctx, p, index, verdict)...)
}

// decisionFields is the record of one judged passage. The identity fields
// (side/chunk_id/pos) are not decoration: the miss rate is recomputed from this
// line alone, and a field the analysis needs but the log does not carry cannot
// be recovered afterwards.
func decisionFields(ctx context.Context, p passage, index int, verdict decisionhead.Verdict) []zap.Field {
	return []zap.Field{
		zap.String("side", p.side),
		zap.String("tool", p.tool),
		zap.String("chunk_id", p.chunkID),
		zap.String("doc_id", p.docID),
		zap.Int("chunk_index", p.chunkIndex),
		zap.Int("pos", p.pos),
		zap.Int("seg_idx", index),
		zap.Int("seg_tokens", verdict.Tokens),
		zap.Duration("latency", verdict.Latency),
		zap.Float64("p_true", verdict.Probability),
		zap.Bool("judged", verdict.Judged),
		zap.Bool("kept", verdict.Useful),
		zap.Bool("exempt", false),
		// The call's stated intent, so the run can be audited afterwards: an
		// intent the agent wrote vaguely ("find relevant info") is a fact about
		// the run that no post-hoc reconstruction could recover.
		zap.String("intent", truncateRunes(intentFrom(ctx), 160)),
	}
}

// logExempt records a passage the top-N rule kept WITHOUT asking the model.
//
// It emits the same event as a real decision, with exempt=true and judged=false,
// so the miss rate stays computable: from the analysis side an exempted passage
// is simply one that survived, and leaving it out of the log would make the
// filter look like it had kept text it never actually passed on. seg_tokens is
// measured on the text that would have been judged, because no verdict carries
// it here.
func logExempt(ctx context.Context, p passage, text string) {
	common.InfoCtx(ctx, "agentic_rag: decision verdict", exemptFields(p, text)...)
}

// exemptFields is the same record with the one difference that matters: the
// model was never asked, so judged is false and p_true says nothing. It carries
// no probability and keeps kept=true, which is what lets a single pass over the
// log answer "what would N have kept".
func exemptFields(p passage, text string) []zap.Field {
	return []zap.Field{
		zap.String("side", p.side),
		zap.String("tool", p.tool),
		zap.String("chunk_id", p.chunkID),
		zap.String("doc_id", p.docID),
		zap.Int("chunk_index", p.chunkIndex),
		zap.Int("pos", p.pos),
		zap.Int("seg_idx", 0),
		zap.Int("seg_tokens", decisionhead.CountTokens(text)),
		zap.Duration("latency", 0),
		zap.Float64("p_true", 0),
		zap.Bool("judged", false),
		zap.Bool("kept", true),
		zap.Bool("exempt", true),
	}
}
