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
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/advanced_rag/harness/orchestrator"
)

// The SCA stage: the review view, its scores, the gaps it leaves for the rewrite, and
// the feedback phrase the next round is handed.
// SelectSCAView: rank the stored pool
// down to the SCA review view (storage ≠ review).
//
// Score = retrieval relevance + surface-term coverage + freshness bonus for
// chunks admitted in later rounds. Returns (view, identity) where identity is a
// stable hash of the selected chunk ids — the caller uses it to detect an
// unproductive round (same view twice despite new storage ⇒ nothing new).
func SelectSCAView(chunks []map[string]any, focusTerms []string) ([]map[string]any, string) {
	terms := dedupe(focusTerms)
	lowered := make([]string, 0, len(terms))
	for _, t := range terms {
		if len(t) >= 3 {
			lowered = append(lowered, strings.ToLower(t))
		}
	}
	type scored struct {
		idx   int
		chunk map[string]any
		score float64
	}
	ranked := make([]scored, 0, len(chunks))
	for i, c := range chunks {
		text := strings.ToLower(strings.Join([]string{
			anyString(c["content"]), anyString(c["content_with_weight"]),
			anyString(c["title"]), anyString(c["question_toks"]),
		}, " "))
		cov := 0
		for _, t := range lowered {
			if strings.Contains(text, t) {
				cov++
			}
		}
		covRatio := 0.5
		if len(lowered) > 0 {
			covRatio = float64(cov) / float64(len(lowered))
		}
		// The pool's relevance reading, from the one place that defines it
		// (see similarityOrScore).
		rel := similarityOrScore(c)
		fresh := min(float64(i)/20.0, 0.2) // late arrivals (gap-pursuit evidence) get seen
		ranked = append(ranked, scored{i, c, rel*0.45 + min(covRatio, 1.0)*0.45 + fresh})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	// Evidence rows first: an atomic proposition carrying a
	// verbatim quote is what the reviewer should read before wading through raw
	// passages. Both groups keep their relevance order; only the grouping is
	// lifted before the view cap is applied, so claim evidence can no longer be
	// pushed out of the view by later, loosely-related raw chunks.
	var evidence, rest []map[string]any
	for _, r := range ranked {
		if strings.HasPrefix(harness.ChunkIDOf(r.chunk), evidenceChunkPrefix) {
			evidence = append(evidence, r.chunk)
		} else {
			rest = append(rest, r.chunk)
		}
	}
	limit := min(len(ranked), SCAViewCap)
	view := make([]map[string]any, 0, limit)
	view = append(view, evidence...)
	view = append(view, rest...)
	view = view[:min(len(view), limit)]
	ids := make([]string, 0, limit)
	for _, c := range view {
		ids = append(ids, harness.ChunkIDOf(c))
	}
	// identity is a hash of the SORTED id set, so this reordering cannot break
	// the unproductive-round detector.
	sort.Strings(ids)
	return view, joinHash(ids)
}

// joinHash builds a stable identity from the selected chunk ids: a deterministic digest
// rather than a randomised-per-process hash.
func joinHash(ids []string) string {
	return fmt.Sprintf("%x", strings.Join(ids, "|"))
}

// SCAGapsToRewrite
//
// Preference order:
//  1. Unsatisfied sub_queries (the precise "what is missing / where to search
//     next" signal from Q-CARE) — (missing_fact, search_hint).
//  2. Per-claim missing_information items — (what, search_hint).
//
// Returns [(what, search_hint), ...] (deduped, non-empty).
func SCAGapsToRewrite(sca map[string]any) []orchestrator.MissingPiece {
	var gaps []orchestrator.MissingPiece
	seen := map[string]bool{}
	add := func(what, hint string) {
		what = strings.TrimSpace(what)
		hint = strings.TrimSpace(hint)
		if what == "" && hint == "" {
			return
		}
		key := what + "|" + hint
		if seen[key] {
			return
		}
		seen[key] = true
		if what == "" {
			what = hint
		}
		if hint == "" {
			hint = what
		}
		gaps = append(gaps, orchestrator.MissingPiece{What: what, SearchHint: hint})
	}
	if raw, ok := sca["sub_queries"].([]any); ok {
		for _, item := range raw {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if b, ok := m["satisfied"].(bool); ok && b {
				continue
			}
			what := anyString(m["missing_fact"])
			if what == "" {
				what = anyString(m["sub_query"])
			}
			add(what, anyString(m["search_hint"]))
		}
	}
	if len(gaps) == 0 {
		// Fall back to per-claim missing_information.
		switch claims := sca["claims"].(type) {
		case map[string]orchestrator.ClaimVerdict:
			keys := make([]string, 0, len(claims))
			for k := range claims {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				for _, mi := range claims[k].MissingInformation {
					add(mi.What, mi.SearchHint)
				}
			}
		case map[string]any:
			for _, g := range claims {
				if gm, ok := g.(map[string]any); ok {
					for _, mi := range asSliceOfAny(gm["missing_information"]) {
						if mm, ok := mi.(map[string]any); ok {
							add(anyString(mm["what"]), anyString(mm["search_hint"]))
						} else {
							add(anyString(mi), "")
						}
					}
				}
			}
		}
	}
	if len(gaps) > MaxSCAGaps {
		gaps = gaps[:MaxSCAGaps]
	}
	return gaps
}

// Phase 1: fan-out expansion.

// scaNode mirrors the `sca` node: the Phase-3 quality-control review.
func scaNode(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) {
	ctx, done := harness.Phase(ctx, "sca")
	defer done()

	chunks := st.KB.Chunks
	view, viewID := SelectSCAView(chunks, ViewTerms(st))
	if st.SCAViewID != "" && viewID == st.SCAViewID {
		// Same evidence review twice despite new storage — further rounds cannot
		// change the verdict; stop instead of looping.
		logger.Printf("[SCA] review view UNCHANGED since last round (%d stored / %d viewed); closing out.", len(chunks), len(view))
		st.Verdict = VerdictInsufficient
		st.NoProgress = true
		st.SCA = map[string]any{}
		return
	}

	draftText := strings.TrimSpace(st.Draft)
	claims, grownView := buildSCAClaims(draftText, view, chunks, st.SlotEvidence)
	// Review against the GROWN view (sca appends to the same list).
	view = grownView
	// sca — the only case that skips the review outright is
	// "nothing retrieved AND nothing drafted". A non-empty view always reaches
	// the SCA (its default claim indexes the whole view, see buildSCAClaims).
	if draftText == "" && len(view) == 0 {
		logger.Printf("[SCA] nothing retrieved nor drafted; marking INSUFFICIENT to trigger a targeted re-search.")
		st.Verdict = VerdictInsufficient
		st.SCA = map[string]any{}
		st.SCAViewID = viewID
		return
	}

	// The SCA renders claim evidence from kbinfos BY INDEX, so review against a
	// view-only copy — the indexed ids then point at exactly the selected
	// chunks. The full pool is restored afterwards.
	orig := st.KB.Chunks
	st.KB.Chunks = view
	defer func() { st.KB.Chunks = orig }()

	review := func() orchestrator.SCAResult {
		t := nodeClock(SCATimeoutS, 15.0, st.RemainingS()-10.0)
		callCtx, cancel := context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
		defer cancel()
		return orchestrator.SufficientContextAgent(callCtx, orchestrator.SCADeps{
			KB:      st.KB,
			Model:   &jsonModelAdapter{inner: deps.Model, maxLength: deps.MaxLength},
			Prompts: deps.SCAPrompts,
		}, st.Question, claims)
	}
	res := review()
	if len(scaResultToMap(res)) == 0 && st.RemainingS() >= SCARetryHeadroomS {
		// One retry: a failed review is a fact about the moment, not about the question, and on a
		// count question this is the only stage that asks whether the list is complete. Spent only
		// while there is clock left for the answer beside it.
		logger.Printf("[SCA] review unavailable on the first attempt (%.0fs left); retrying once.", st.RemainingS())
		res = review()
	}

	st.SCAViewID = viewID
	st.SCA = scaResultToMap(res)
	if len(st.SCA) == 0 {
		// sca — an unavailable SCA (timeout, unparsable reply, no model) is NOT a
		// verdict: it is the absence of one. Marking it INSUFFICIENT used to be
		// the fallback, and it read as a judgement everywhere downstream — the
		// answer was labelled PARTIAL on no evidence (formalizeAnswerNode) and
		// the loop was told to research again by a review that never ran
		// (measured 2026-09-15: review timed out at 35s and the run closed out
		// with zero seconds left, so the "insufficiency" was never actionable
		// either). VerdictUnknown keeps the two apart; the loop's own record
		// (growth + gaps) decides whether another round is worth its budget, and
		// the deliverable is not dressed up as partly-unverified.
		logger.Printf("[SCA] unavailable (no review could be completed); recording UNKNOWN — this is not a sufficiency judgement, so the round's own record drives the loop.")
		st.SCA = map[string]any{}
		st.Verdict = VerdictUnknown
		// The fact goes where the answer can read it (see composedRecord). The partial label keeps
		// its evidence-based rule above: they are different claims.
		st.KB.NoteSufficiencyUnchecked()
		return
	}
	if res.IsSufficient {
		st.Verdict = VerdictSufficient
	} else {
		st.Verdict = VerdictInsufficient
	}
	logger.Printf("[SCA] verdict=%s (confidence=%.2f; view=%d/%d)", st.Verdict, res.Confidence, len(view), len(chunks))
}

// unresolvedClueGaps is the gap fallback: the first two question_clues of every unresolved
// slot become (what, hint) gaps, so a rewrite can still be issued when the SCA produced no
// structured gaps.
func unresolvedClueGaps(st *AgenticState) []orchestrator.MissingPiece {
	var gaps []orchestrator.MissingPiece
	for _, us := range st.UnresolvedSlots {
		clues, ok := us["question_clues"].([]string)
		if !ok {
			continue
		}
		for i, qc := range clues {
			if i >= 2 {
				break
			}
			qc = strings.TrimSpace(qc)
			if qc == "" {
				continue
			}
			gaps = append(gaps, orchestrator.MissingPiece{What: qc, SearchHint: qc})
		}
	}
	return gaps
}

// buildSCAClaims mirrors the sca node's claim construction (lines 971-997): the
// draft as claim "c0", plus one claim per slot carrying its evidence positions.
//
// It returns the (possibly grown) view alongside the claims: the slot-evidence chunks that
// the view missed are appended to the SAME list before review, so the caller must review
// against the returned slice, not the one it passed in.
func buildSCAClaims(draftText string, view, chunks []map[string]any, slotEvidence map[string]SlotEvidence) ([]orchestrator.ClaimDraft, []map[string]any) {
	var claims []orchestrator.ClaimDraft
	if draftText != "" {
		claims = append(claims, orchestrator.ClaimDraft{ID: "c0", Draft: draftText})
	}
	viewIndexByID := map[string]int{}
	for i, c := range view {
		if id := harness.ChunkIDOf(c); id != "" {
			viewIndexByID[id] = i
		}
	}
	sids := make([]string, 0, len(slotEvidence))
	for sid := range slotEvidence {
		sids = append(sids, sid)
	}
	sort.Strings(sids)
	for _, sid := range sids {
		meta := slotEvidence[sid]
		if len(meta.EvidenceIDs) == 0 {
			continue
		}
		// Resolve every evidence id to a POSITION in the view, appending chunks
		// that are missing so the SCA sees the passages that actually produced
		// the candidate (sca).
		positions := make([]string, 0, len(meta.EvidenceIDs))
		for _, eid := range meta.EvidenceIDs {
			pos := -1
			if p, ok := viewIndexByID[eid]; ok {
				pos = p
			} else if c := resolveEvidenceChunk(eid, chunks); c != nil {
				if p, ok := viewIndexByID[harness.ChunkIDOf(c)]; ok {
					pos = p
				} else {
					view = append(view, c)
					pos = len(view) - 1
					if id := harness.ChunkIDOf(c); id != "" {
						viewIndexByID[id] = pos
					}
				}
			}
			if pos >= 0 {
				positions = append(positions, strconv.Itoa(pos))
			}
		}
		draft := truncateRunes(meta.Candidate, draftCandidateChars)
		if draft == "" {
			draft = fmt.Sprintf("(slot %s evidence)", sid)
		}
		claims = append(claims, orchestrator.ClaimDraft{ID: sid, Draft: draft, EvidenceIDs: positions})
	}
	if len(claims) == 0 {
		// sca — an empty draft still gets ONE claim, carrying every
		// position in the view, so the SCA judges the evidence itself instead of
		// the node short-circuiting on "no draft".
		draft := draftText
		if strings.TrimSpace(draft) == "" {
			draft = "(no draft)"
		}
		claims = []orchestrator.ClaimDraft{{ID: "c0", Draft: draft, EvidenceIDs: allViewPositions(view)}}
	}
	return claims, view
}

// resolveEvidenceChunk maps one slot-evidence id back to a chunk. The ids are
// heterogeneous by construction:
//
//   - the retrieval tools emit the chunk id;
//   - the navigation tools emit document ids (tool_executor.go:455);
//   - the SCA view itself is keyed by POSITION (renderClaimContext /
//     sufficient_context.go:305-309), which the numeric branch below covers.
//
// slot_evidence holds chunk ids throughout, so both spellings are resolved here before
// giving up.
func resolveEvidenceChunk(eid string, chunks []map[string]any) map[string]any {
	eid = strings.TrimSpace(eid)
	if eid == "" {
		return nil
	}
	// Identity FIRST, position second: a chunk id that happens to be all digits (or a
	// doc id like "3") would otherwise be read as an index and resolve to a DIFFERENT
	// chunk — the ambiguity only disappears if exact ids win.
	for _, c := range chunks {
		if harness.ChunkIDOf(c) == eid {
			return c
		}
		if id := harness.DocIDOf(c); id != "" && id == eid {
			return c
		}
	}
	if n, err := strconv.Atoi(eid); err == nil && n >= 0 && n < len(chunks) {
		return chunks[n]
	}
	return nil
}

// allViewPositions is "every position in the view" (renderClaimContext keys evidence by
// index).
func allViewPositions(view []map[string]any) []string {
	positions := make([]string, 0, len(view))
	for i := range view {
		positions = append(positions, strconv.Itoa(i))
	}
	return positions
}

// scaResultToMap flattens an SCAResult back into the map shape the loop keeps in
// AgenticState.SCA (SCAGapsToRewrite reads both shapes).
func scaResultToMap(res orchestrator.SCAResult) map[string]any {
	if len(res.Claims) == 0 && res.Reasoning == "" && !res.IsSufficient {
		return nil
	}
	out := map[string]any{
		"is_sufficient":  res.IsSufficient,
		"confidence":     res.Confidence,
		"contradictions": toAnySlice(res.Contradictions),
		"reasoning":      res.Reasoning,
		"claims":         res.Claims,
	}
	if len(res.SubQueries) > 0 {
		sqs := make([]any, 0, len(res.SubQueries))
		for _, sq := range res.SubQueries {
			sqs = append(sqs, map[string]any{
				"sub_query":    sq.SubQuery,
				"satisfied":    sq.Satisfied,
				"missing_fact": sq.MissingFact,
				"search_hint":  sq.SearchHint,
			})
		}
		out["sub_queries"] = sqs
	}
	return out
}

// verdictStatusHint is the human phrase folded into rag()'s "[Research status]" note for each
// sufficiency status.
//
// Only INSUFFICIENT is reachable: the verdict dict carries a single "status" key whose value
// is either "SUFFICIENT" or "INSUFFICIENT", and SUFFICIENT is skipped outright.
// The upstream dict's USEFUL_BUT_INCOMPLETE and CONFLICTING entries are
// therefore unreachable, so Go does not carry cases for them.
func verdictStatusHint(verdict string) string {
	switch verdict {
	case VerdictInsufficient:
		return "evidence is not yet sufficient"
	case VerdictUnknown:
		// Not a judgement: the review did not complete, so nothing here may claim
		// the evidence was found wanting (nor that it was sufficient).
		return "the evidence review did not complete, so sufficiency is unverified"
	default:
		return "sufficiency status: " + verdict
	}
}

// scaFeedback is the body rag folds into the answer as the "[Research status]" note whenever
// research stays unsatisfying.
//
// It composes as `status_hint + missing_txt + hard_txt + conf_txt + fb_txt`, but every term
// after the first is empty in practice: `verdict` is `{"status": ...}` and carries no
// missing_claims / hard_violations / agent_confidence / feedback keys anywhere, so the note is
// the status hint alone.
//
// The SCA's rich payload is not lost — it stays in AgenticState.SCA and still drives
// SCAGapsToRewrite (which reads the separate `sca` state key). Non-empty for
// ANY non-SUFFICIENT verdict (not only after two consecutive misses), so the caller can
// append the appropriate trailing sentence ("STOP" vs "call rag again").
func scaFeedback(_ map[string]any, verdict string) string {
	if verdict == VerdictSufficient {
		return ""
	}
	return verdictStatusHint(verdict)
}

// renderResearchContext mirrors the query_rewrite node's context build
// (lines 1047-1066): the attempted-query ledger with outcomes, plus the first
// lines of the evidence pool.
func renderResearchContext(st *AgenticState) string {
	var historyLines []string
	for _, e := range st.Attempted {
		if e == nil {
			continue
		}
		q := truncateRunes(anyString(e["q"]), 120)
		r := anyString(e["r"])
		if r == "" {
			r = "?"
		}
		outcome := "no new passages"
		switch {
		case e["bound"] != nil:
			// A research round's row carries what ITS session bound. There is no
			// per-session "new to the pool" number to report: a round's sessions run
			// concurrently over ONE pool (see RunSlotResearchPass).
			if n, ok := toIntStrict(e["bound"]); ok {
				outcome = fmt.Sprintf("%d passage(s) bound", n)
			}
		default:
			// A prefetch/rewrite row: those DO know how many passages the fetch added.
			if n, ok := toIntStrict(e["new"]); ok && n != 0 {
				outcome = fmt.Sprintf("%d new passage(s)", n)
			}
		}
		historyLines = append(historyLines, fmt.Sprintf("- %s (round %s: %s)", q, r, outcome))
	}
	// The passages behind the members already confirmed come FIRST, because they
	// are the only place the rewriter can see (a) the wording this text uses for
	// the relation and (b) the names that are still missing. Those passages are
	// exactly the windows the seats were narrowed to (see
	// Kbinfos.RecordReachedTerm), so a name inside one of them is readable.
	//
	// The pool-head lines below are the fallback for a round that has confirmed
	// nothing yet. They are FIRST LINES only, so a name in the middle of a chunk
	// is invisible through them — which is why they are the fallback and not the
	// main channel.
	var memberLines []string
	for i, m := range memberLinesOf(st) {
		if i >= MemberWindowMax {
			break
		}
		memberLines = append(memberLines, "- "+m)
	}
	var poolLines []string
	if len(memberLines) == 0 && st.KB != nil {
		for i, c := range st.KB.Chunks {
			if i >= PoolHeadLines {
				break
			}
			// Read `content` FIRST here — unlike the content_with_weight-first order used
			// elsewhere. The pool-head lines are rewriter prompt content, so this order is
			// deliberate.
			first := anyString(c["content"])
			if first == "" {
				first = anyString(c["content_with_weight"])
			}
			if idx := strings.IndexByte(first, '\n'); idx >= 0 {
				first = first[:idx]
			}
			if first = strings.TrimSpace(first); first != "" {
				poolLines = append(poolLines, "- "+truncateRunes(first, 140))
			}
		}
	}
	var parts []string
	if len(historyLines) > 0 {
		parts = append(parts, "Previously searched queries and their outcomes:\n"+strings.Join(historyLines, "\n"))
	}
	if len(memberLines) > 0 {
		parts = append(parts, "Passages that carry names the searches ALREADY confirmed (read them for the wording this text uses for the relation, and for other names they mention — any of those can be asked about directly):\n"+strings.Join(memberLines, "\n"))
	}
	if len(poolLines) > 0 {
		parts = append(parts, "Evidence currently at hand (first lines of top stored snippets):\n"+strings.Join(poolLines, "\n"))
	}
	// The round's own record of what its probes ASKED and never reached. The
	// rewriter reads it for one reason: re-asking a name the corpus already came
	// back empty on is the loop's most common waste, and the productive move from
	// a dead name is a different ANGLE (the act, the relationship, the place),
	// which the rewriter cannot choose unless it knows the name is dead.
	if absent := st.KB.ProbedAbsentTerms(); len(absent) > 0 {
		parts = append(parts, "Terms already asked for and NOT reached by any passage (do NOT re-ask these on their own; ask for the act / relationship / place instead):\n"+strings.Join(absent, "、"))
	}
	return strings.Join(parts, "\n\n")
}

// memberLinesOf renders one line per confirmed member: the name, and the window
// that carries it, looked up in the pool by the id the seat recorded.
//
// It reads the round's own record rather than re-searching anything: a seat IS a
// probe of one individual that came back with a passage, so the pair (name,
// passage) is already established by the time the rewrite runs.
func memberLinesOf(st *AgenticState) []string {
	if st == nil || st.KB == nil {
		return nil
	}
	byID := make(map[string]map[string]any, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		if id := harness.ChunkIDOf(c); id != "" {
			byID[id] = c
		}
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range st.KB.ReachedTerms() {
		if seen[m.ChunkID] {
			continue
		}
		c, ok := byID[m.ChunkID]
		if !ok {
			continue
		}
		text := strings.TrimSpace(harness.ChunkTextOf(c))
		if text == "" {
			continue
		}
		seen[m.ChunkID] = true
		out = append(out, fmt.Sprintf("%s: %s", m.Term, truncateRunes(text, MemberWindowChars)))
	}
	return out
}
