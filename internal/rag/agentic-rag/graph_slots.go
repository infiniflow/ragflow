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
	"fmt"
	"strings"
	"unicode/utf8"

	"ragflow/internal/rag/agentic-rag/runtime"
	"ragflow/internal/rag/agentic-rag/slots"
)

// The slot-research stage: the table a round is driven by, the members and counts it
// holds, the passes that fill it, and the write-back folded into it.
// The SCA-facing DRAFT used to be rendered here (RenderSlotDraft): the table plus the machine
// fields a reviewer needed to check a candidate against the passages that produced it — candidate
// strength, terminal type, evidence ids, clue tails.
//
// It is gone with the reviewer. The table still renders, once, as the RECORD (RenderSlotRecord),
// which is what the round reports and what a fallback composition reads; and the answer itself is
// written by the session that read the passages (see routeResearch and the answer contract in
// runtime/action_session.go), so there is no second rendering for it to agree or disagree with.

// RenderSlotRecord renders the run's own record for the ANSWER prompt: the values the model itself
// wrote into the table, verbatim, and the session's own draft answer labelled for what it is.
//
// It used to reconcile a LIST against a COUNT — "enumerated members across the slots above: N", a
// "claimed WITHOUT a passage" list, and the warnings that came with them. Every one of those lines was
// the runtime reading the table's values to decide what counted as a member: a SEMANTIC judgement,
// made from values whose type the plan chose and whose text is opaque (see the note on SessionRecord in
// runtime/session_state_line.go). Only the passages can decide between a list and a count, and the
// answer is the stage that reads them.
func RenderSlotRecord(slotTable runtime.State, collectedAnswer string) string {
	var lines []string
	for _, v := range slotTable.State {
		vtype := v.Type
		if vtype == "" {
			vtype = "entity"
		}
		if v.Candidate != nil && strings.TrimSpace(*v.Candidate) != "" {
			lines = append(lines, fmt.Sprintf("- slot %d [%s]: %s", v.ID, vtype, *v.Candidate))
		} else {
			lines = append(lines, fmt.Sprintf("- slot %d [%s]: NOT RESOLVED", v.ID, vtype))
		}
		// Claims that lost the slot comparison (see MergeSlotPatch). They are not the slot's value,
		// but they are not nothing either: a session produced them from the same corpus.
		for _, alt := range alternateCandidatesOf(v) {
			lines = append(lines, "    alternate (claimed by another session, not adopted): "+runtime.TruncateRunes(alt, 400))
		}
	}
	if strings.TrimSpace(collectedAnswer) != "" {
		lines = append(lines, "One session's own draft answer (UNVERIFIED — written before the other sessions were merged. Its PROSE is not the record: reconcile it against the slots above and the passages in evidence, and answer from the evidence): "+collectedAnswer)
	}
	return strings.Join(lines, "\n")
}

// The batched slot-fill pass used to live here: it clustered the slots that had retrieved
// overlapping evidence and answered them in ONE generation call.
//
// It went with the session fan-out. There is one session per round, its direction already covers
// every clue the plan produced, and the slots it patches are its own scratchpad — so there is no
// cluster of slots to batch and no second consumer of the evidence map. What the map is still for
// is the round's LEDGER (see the [SlotResearch] line in RunSlotResearchPass) and, later, the
// opening's per-clue ranking.

// PrefillSlotsFromEvidence
// answer slots that an already-pooled evidence row directly answers.
//
// An evidence row is an atomic proposition carrying a verbatim quote, so when
// it already covers a slot's question there is nothing for an action session
// to research — skipping it removes a WHOLE session (the dominant cost), not
// just tokens inside one. Saves calls, uses real evidence, and a wrong guess
// is still caught later by the SCA.
func PrefillSlotsFromEvidence(slotTable *runtime.State, kb *runtime.Kbinfos) int {
	// evidence rows are the claim pseudo-chunks.
	evRows := []map[string]any{}
	if kb != nil {
		for _, c := range kb.Chunks {
			if strings.HasPrefix(runtime.ChunkIDOf(c), evidenceChunkPrefix) {
				evRows = append(evRows, c)
			}
		}
	}
	if len(evRows) == 0 {
		return 0
	}

	filled := 0
	for i := range slotTable.State {
		v := &slotTable.State[i]
		if v.Filled() {
			continue
		}
		clues := []string{}
		for _, c := range v.QuestionClues {
			if strings.TrimSpace(c) != "" {
				clues = append(clues, c)
			}
		}
		if len(clues) == 0 {
			continue
		}
		// lowercase terms of ≥3 code points over all clues.
		terms := map[string]bool{}
		for _, c := range clues {
			for _, t := range runtime.QueryToTerms(c) {
				if utf8.RuneCountInString(t) >= 3 {
					terms[strings.ToLower(t)] = true
				}
			}
		}
		if len(terms) == 0 {
			continue
		}

		best, bestCov := -1, 0.0
		for j, e := range evRows {
			// content_with_weight ONLY (not content).
			text := strings.ToLower(anyString(e["content_with_weight"]))
			if text == "" {
				continue
			}
			cov := 0
			for t := range terms {
				if strings.Contains(text, t) {
					cov++
				}
			}
			covF := float64(cov) / float64(len(terms))
			// strictly greater, so ties keep the FIRST row.
			if covF > bestCov {
				best, bestCov = j, covF
			}
		}
		if best < 0 || bestCov < EvidencePrefillCoverage {
			continue
		}

		// The row renders as
		// "[evidence] <name> — <desc>\nEvidence (verbatim): ...".
		//
		// Format provenance (TWO deliberate claim formats): the fan-out channel-0 rows
		// this prefill consumes carry the "[evidence] " prefix (see
		// runtime.ClaimPseudoChunks). The OTHER producers — the action-session claim
		// prefetch and the navigate_structure publish — render "[claim #N] <name>", and
		// prefill never sees "[evidence]" on those either: the replace below simply
		// leaves their marker intact. Both producers are matched one-to-one.
		head := anyString(evRows[best]["content_with_weight"])
		if idx := strings.IndexByte(head, '\n'); idx >= 0 {
			head = head[:idx]
		}
		name := strings.TrimSpace(strings.Trim(strings.ReplaceAll(head, "[evidence]", ""), " —-"))
		if name == "" {
			continue
		}
		// candidate = name[:400], strength = coverage.
		cand := runtime.TruncateRunes(name, 400)
		v.Candidate = &cand
		strength := bestCov
		v.CandidateStrength = &strength
		filled++
	}
	return filled
}

// BuildSlotTable: decompose the question into
// a slot table, seeding it with the planner's fan-outs.
//
// Returns (root, firstQueries) — never an empty root: on failure it degrades to
// one "aspect" slot per fan-out (or a single "answer" slot for the raw
// question), so the research pass always has something to work on.
func BuildSlotTable(ctx context.Context, deps runtime.SessionDeps, question string, fanouts []string, deadlineLeft float64) (runtime.State, []string) {
	root, firstQueries, err := buildSlotTableFrom(ctx, deps, question, fanouts, deadlineLeft)
	if err != nil {
		_LOG.Printf("[SlotTable] initialize_state failed; building from fanouts: %v", err)
		// _build_slot_table — the exception path keeps the FULL fan-out list as
		// first_queries; the [:3] cap below belongs to the empty-root path only.
		if len(fanouts) > 0 {
			firstQueries = fanouts
		} else {
			firstQueries = []string{question}
		}
	}
	if len(root.State) == 0 {
		queries := fanouts
		if len(queries) == 0 {
			queries = []string{question}
		}
		vars := make([]runtime.Variable, 0, 4)
		for i, q := range queries {
			if i >= 4 {
				break
			}
			vars = append(vars, runtime.Variable{
				ID:            i,
				Type:          "aspect",
				QuestionClues: []string{runtime.TruncateRunes(q, slotFallbackClueChars)},
			})
		}
		root = runtime.NewState(vars, 0, nil)
		if len(firstQueries) == 0 {
			firstQueries = queries
			if len(firstQueries) > 3 {
				firstQueries = firstQueries[:3]
			}
		}
	}
	_LOG.Printf("[SlotTable] built %d slot(s): %s", len(root.State), root.Brief())
	if len(firstQueries) == 0 {
		firstQueries = []string{question}
	}
	return root, firstQueries
}

// RunSlotResearchPass: drive ONE research round.
//
// The round is ONE session — the researcher AND the answerer — seeded with the question, the
// plan's clues and whatever the review named (see the body). The slot table travels with it as
// its own scratchpad: nothing here reads it to decide anything. A nil result means the round
// produced nothing at all.
func RunSlotResearchPass(ctx context.Context, parent context.Context, deps runtime.SessionDeps, question string, st *AgenticState, deadlineLeft float64) *SlotResearchResult {
	slotTable := st.SlotTable
	if len(slotTable.State) == 0 {
		// No planner ran (medium single-pass, or the planner failed): build the
		// table from the raw question so the research still executes.
		root, _ := BuildSlotTable(ctx, deps, question, nil, max(15.0, deadlineLeft-10.0))
		slotTable = root
	}
	if question == "" {
		question = st.Question
	}
	unresolved := slotTable.Unresolved()

	// Evidence-row prefill（）: slots the pooled evidence
	// already answers cost no action session at all — this is where whole
	// calls get removed. It is pure (no model, no I/O) and cannot fail.
	kb := deps.KB
	if kb == nil {
		// `tools.kbinfos or state.kbinfos`.
		kb = st.KB
	}
	prefillN := PrefillSlotsFromEvidence(&slotTable, kb)
	if prefillN > 0 {
		_LOG.Printf("[SlotResearch] evidence prefill answered %d slot(s) with no session", prefillN)
		unresolved = slotTable.Unresolved()
	}

	// The directions this round works: one per unresolved slot, and — when the
	// table is already filled — the gaps the REVIEW named.
	//
	// A filled table is not a finished round for a question whose answer slot is
	// DERIVED: a count computed from the members it found is filled by
	// construction, however few members that is, so `unresolved == 0` silently
	// cancelled rounds the routing had started precisely because the review named gaps: a
	// rewrite round prefetches its evidence, logs "all slots filled; no session to run", and
	// spends its budget with a draft byte-identical to the previous round's — the passages it
	// had just admitted were never read into the record, which is the step a session exists
	// for.
	type direction struct {
		slotID int
		text   string
	}
	// ONE session, and it is the WHOLE research round.
	//
	// What this replaced: one session PER DIRECTION (an unresolved slot, or a gap the review
	// named) run concurrently, each seeing only what it had searched itself, with the answer
	// written afterwards by a separate compose call from the merged record. That shape bought
	// parallelism and paid for it twice — the writer of the answer had not read the evidence it
	// wrote from, and the machinery for choosing directions, merging patches and batching
	// generations existed only to serve it.
	//
	// The session is seeded with the question, the plan's clues and whatever the review named:
	// the clues say WHAT TO SEARCH (the planner's only job), the question is what the answer
	// must satisfy, and the review's gaps are this round's follow-up. The table still travels,
	// but as the session's own scratchpad — nothing here reads it to decide anything.
	gaps := unresolvedClueGaps(st)
	gapTexts := make([]string, 0, len(gaps))
	for _, g := range gaps {
		text := strings.TrimSpace(g.SearchHint)
		if text == "" {
			text = strings.TrimSpace(g.What)
		}
		if text != "" {
			gapTexts = append(gapTexts, text)
		}
	}
	dirText := strings.TrimSpace(question)
	if len(st.Plan) > 0 {
		// The block is what to COVER, addressed to the reader — and the heading is a PURE label on
		// its own line.
		//
		// It used to carry the instruction in the same line ("Clues to cover (turn each into your
		// OWN SHORT probe query — a few words; never pass this block…):"), which made the label
		// unmatchable as a heading and the INSTRUCTION the first content line: a session that
		// copied the block into `retrieve` handed retrieval the instruction, which then searched
		// its own words and recorded them as terms asked and absent (measured 2026-09-20 三国:
		// `[BM25 search] Searching by keyword for "probe"`, and asked-nothing-back=10
		// "Clues、to、cover、turn…"). The instruction lives in the session's playbook now
		// (action_run.md), where it costs no query.
		dirText += "\n\nClues to cover:"
		for _, c := range st.Plan {
			if c = strings.TrimSpace(c); c != "" {
				dirText += "\n- " + c
			}
		}
	}
	for _, g := range gapTexts {
		dirText += "\nA gap still to close (probe it the same way): " + g
	}
	// What the PREVIOUS round's session said it could not establish (see routeResearch). This is
	// the round that statement reopened, so it is the round's first target — otherwise the reopened
	// round re-walks the ground the last one covered and the open part is missed again.
	if open := strings.TrimSpace(st.SessionUnresolved); open != "" {
		dirText += "\n\nAn earlier round could not establish: " + open +
			"\nThat is what THIS round exists for: probe it directly."
	}
	dirs := []direction{{slotID: -1, text: dirText}}
	_LOG.Printf("[SlotResearch] one session this round (clue(s)=%d, review gap(s)=%d, open part=%t).",
		len(st.Plan), len(gapTexts), strings.TrimSpace(st.SessionUnresolved) != "")

	// The session's own tool cache and query list. They used to be SHARED, because a round ran
	// several sessions at once and a duplicate retrieval was worth serving once; with one
	// session they are simply its state.
	sharedToolCache := runtime.NewToolCache()
	var sharedSearchQueries []string

	type outcome struct {
		slotID int
		// direction travels with the result so the round's ledger can say what this
		// session was ASKED to research without re-deriving it from its messages (see
		// the ledger row below).
		direction string
		result    runtime.Result
	}
	results := make([]outcome, 0, len(dirs))

	// One session, on this round's own clock: whatever time the round has is the session's,
	// with a floor so a session is never started with nothing to spend.
	//
	// There is no per-shape extension any more (see SetBudgetExtensionS): a set question is not
	// a different kind of question here, and buying one shape extra time out of the same total
	// is exactly how another shape loses it.
	sessionCtx := ctx
	sessionBudget := max(20.0, deadlineLeft-10.0)

	for _, d := range dirs {
		// DisableTool mutates the shared Toolset, so the call stays in the caller's goroutine —
		// there is exactly one session now. The Kbinfos merge happens inside the executor.
		res := runtime.RunActionSession(sessionCtx, deps, d.text, slotTable, sessionBudget, "", sharedToolCache, sharedSearchQueries)
		results = append(results, outcome{slotID: d.slotID, direction: d.text, result: res})
	}

	collected := st.CollectedAnswer
	sessionEvidence := map[string]SlotEvidence{}
	ledger := append([]map[string]any(nil), st.Attempted...)
	// The round's evidence registry, in session order (see SessionState.EvidenceRefs): the
	// answer that comes out of this round cites these numbers.
	var evidenceRefs []string
	seenRefs := map[string]bool{}
	// What the session said it could not establish (see Result.Unresolved): a round whose answer
	// names an open part is not a finished round, and the routing reads this to decide whether the
	// question gets another one (see routeResearch).
	sessionUnresolved := st.SessionUnresolved

	for _, item := range results {
		r := item.result
		if r.FoundAnswer != nil && collected == "" {
			collected = *r.FoundAnswer
		}
		if u := strings.TrimSpace(r.Unresolved); u != "" {
			sessionUnresolved = u
		}
		for _, id := range r.EvidenceRefs {
			if id == "" || seenRefs[id] {
				continue
			}
			seenRefs[id] = true
			evidenceRefs = append(evidenceRefs, id)
		}
		if len(r.RetrievedEvidenceIDs) > 0 {
			terminalType := ""
			if r.TerminalType != nil {
				terminalType = *r.TerminalType
			}
			var candidate string
			if r.FoundAnswer != nil {
				candidate = *r.FoundAnswer
			}
			sessionEvidence[fmt.Sprint(item.slotID)] = SlotEvidence{
				EvidenceIDs:  runtime.Dedupe(r.RetrievedEvidenceIDs),
				TerminalType: terminalType,
				Candidate:    candidate,
			}
		}
		for _, ns := range r.NewStates {
			before := slotTable
			merged := MergeSlotPatch(slotTable, ns)
			if merged != nil {
				slotTable = *merged
			}
			// What this session CLAIMED, and whether the merge kept it.
			//
			// The merge is a tournament per slot: one candidate survives, chosen by
			// the strength the MODEL reported (MergeSlotPatch:3304), and nothing
			// logged the contestants — which is why three rounds of analysis here
			// could only GUESS whether some session's list had been dropped. The log
			// showed the merged table and never a single patch.
			logSessionPatch(item.slotID, before, slotTable, ns)
		}
		// _run_slot_research_pass — a session without a found answer still logs a hint, taken
		// from its message history (`(r.found_answer or str(r.messages))[:80]`).
		// Without the fallback the rewriter's research context shows a bare
		// "-  (round N: …)" row for those sessions.
		q := ""
		if r.FoundAnswer != nil {
			q = runtime.TruncateRunes(*r.FoundAnswer, 80)
		} else {
			// No answer: the row shows what this session was ASKED to research, which is
			// the query of the pair (query, outcome). It used to be a langchain-style repr
			// of the session's messages, and that repr is a CONSTANT in production: every
			// session's history opens with the same system prompt, so its first 80 runes —
			// a message header, not a query — went into every row of every question.
			q = runtime.TruncateRunes(item.direction, 80)
		}
		// "bound" is the evidence THIS session retrieved (the number the row can
		// stand behind); "new" would be a lie here: sessions run concurrently and
		// share one pool, so no per-session count of newly admitted passages exists.
		// The row used to say `"new": 1` for every session, which the rewriter read
		// as "1 new passage(s)" on every round it did not retrieve anything. `r` is
		// the round the row belongs to, the same key the prefetch/rewrite rows use.
		ledger = append(ledger, map[string]any{
			"q":     q,
			"r":     st.SearchRounds,
			"bound": len(runtime.Dedupe(r.RetrievedEvidenceIDs)),
		})
	}

	// _run_slot_research_pass — report how many passages each slot's session bound, so
	// a round that retrieved nothing for a slot is visible in the run log.
	if len(sessionEvidence) > 0 {
		bounds := make(map[string]int, len(sessionEvidence))
		for sid, ev := range sessionEvidence {
			bounds[sid] = len(ev.EvidenceIDs)
		}
		_LOG.Printf("[SlotResearch] slot evidence bound: %v", bounds)
	}

	unresolvedOut := make([]map[string]any, 0, len(unresolved))
	for _, v := range slotTable.Unresolved() {
		clues := v.DiscoveredClues
		if len(clues) > 4 {
			clues = clues[len(clues)-4:]
		}
		unresolvedOut = append(unresolvedOut, map[string]any{
			"id":               v.ID,
			"type":             v.Type,
			"question_clues":   append([]string(nil), v.QuestionClues...),
			"discovered_clues": append([]string(nil), clues...),
		})
	}

	record := RenderSlotRecord(slotTable, collected)
	_LOG.Printf("[SlotResearch] round done — %d slot(s) filled, unresolved=%d, collected_answer=%v",
		countFilled(slotTable), len(unresolvedOut), collected != "")
	_LOG.Printf("[SlotResearch] slot table after round:\n%s", record)

	return &SlotResearchResult{
		SlotTable:       slotTable,
		CollectedAnswer: collected,
		UnresolvedSlots: unresolvedOut,
		SlotEvidence:    sessionEvidence,
		// The record is the only rendering of the table that leaves this round. There used to be a
		// second one — a "draft" carrying the machine fields a reviewer needed (candidate strength,
		// evidence ids, clue tails) — and the two were rendered together "so they cannot drift". With
		// no reviewer there is nothing to drift FROM: the round's deliverable is what it found, and
		// the answer is written by the session that read it (see the answer contract in
		// runtime/action_session.go).
		SlotRecord: record,
		Attempted:  ledger,
		// What the session said it could not establish, if it said anything. The round has ONE
		// session, so the last statement wins: it is the one written against everything the session
		// had read by then.
		Unresolved:   sessionUnresolved,
		EvidenceRefs: evidenceRefs,
	}
}

// alternateCandidatesOf returns the candidates a slot lost to, in the order they were
// kept (see runtime.Variable.Alternates).
func alternateCandidatesOf(v runtime.Variable) []string {
	return append([]string(nil), v.Alternates...)
}

// logSessionPatch reports one session's claims and what the merge did with them.
//
// One line per patched slot, because the losing side of MergeSlotPatch leaves no
// trace anywhere else: a candidate that was dropped is not in the table, not in
// the draft, not in the record — it is simply gone, and "gone" is the failure this
// line makes visible.
func logSessionPatch(directionSlot int, before, after, patch runtime.State) {
	for _, pv := range patch.State {
		if pv.Candidate == nil || *pv.Candidate == "" {
			continue
		}
		bv := before.ByID(pv.ID)
		base := "(empty)"
		if bv != nil && bv.Candidate != nil && *bv.Candidate != "" {
			base = fmt.Sprintf("%q (%.2f)", runtime.TruncateRunes(*bv.Candidate, 80), strengthOf(*bv))
		}
		// THREE outcomes, because the fold has three: the patch's text IS the slot's
		// value; the slot CHANGED without keeping this text verbatim (a union — member
		// lists merge by name, so a patch can be inside the value without being its
		// rendering); or the slot did not move at all. The old two-way test called every
		// union "NOT ADOPTED (the base stands)", which hid every merge that mattered.
		result := "NOT ADOPTED (the base stands)"
		if av := after.ByID(pv.ID); av != nil {
			switch {
			case av.Candidate != nil && *av.Candidate == *pv.Candidate:
				result = "adopted"
			case av.Candidate != nil && bv != nil && bv.Candidate != nil && *av.Candidate != *bv.Candidate:
				result = "merged (the union kept both; the slot's value is not this candidate verbatim)"
			}
		}
		_LOG.Printf("[SlotResearch] patch (direction slot %d) → slot %d: %q (%.2f); base was %s; result: %s",
			directionSlot, pv.ID, runtime.TruncateRunes(*pv.Candidate, 120), strengthOf(pv), base, result)
	}
}

// MergeSlotPatch folds a session's new-state branch into the shared slot table: a slot
// holding a SET merges by union, anything else adopts the STRONGER candidate (see
// slots.Union for why).
//
// Returns nil when nothing changed, so
// callers can skip no-op merges — and so a round that changed nothing can be told apart
// from one that did.
func MergeSlotPatch(base, branch runtime.State) *runtime.State {
	if len(branch.State) == 0 {
		return nil
	}
	branchByID := map[int]runtime.Variable{}
	for _, v := range branch.State {
		branchByID[v.ID] = v
	}
	merged := make([]runtime.Variable, 0, len(base.State))
	changed := false
	for _, v := range base.State {
		bv, ok := branchByID[v.ID]
		if !ok {
			merged = append(merged, v)
			continue
		}
		// A slot holding a SET merges by UNION; a slot holding one VALUE is still settled by
		// strength (see slots.Union for why, and for the case where a count outvoted the
		// members it was counting).
		//
		// The union is over TYPED values: members union by name, a membership claim
		// beats a number, two numbers keep the larger. Text is not the union's
		// business (invariant I1), which is what keeps a value question merging by
		// strength exactly as it always did.
		var cand *string
		var value *slots.Value
		strength := v.CandidateStrength
		var loser string
		if union, dropped, ok := slots.Union(v.Typed(), bv.Typed()); ok {
			rendered := slots.Render(union)
			cand = &rendered
			value = &union
			if strengthOf(bv) > strengthOf(v) {
				strength = bv.CandidateStrength
			}
			if !dropped.IsZero() {
				loser = slots.Render(dropped)
			}
		} else {
			// Adopt the branch candidate only when STRONGER. Sessions run
			// concurrently and their branches fold in completion order, so an
			// unconditional "branch wins" made the result both order-dependent and
			// destructive: a weak session (0.3, tentative) could downgrade a slot
			// another session had already proven (0.95).
			adoptBranch := bv.Candidate != nil && *bv.Candidate != "" &&
				(v.Candidate == nil || *v.Candidate == "" || strengthOf(bv) > strengthOf(v))
			cand = v.Candidate
			value = v.Value
			if adoptBranch {
				cand, strength = bv.Candidate, bv.CandidateStrength
				value = bv.Value
			}
			// ONE slot holds ONE candidate, so the claim that lost the comparison used
			// to leave no trace at all: not in the table, not in the draft, not in the
			// record. Two sessions enumerating the same question from different angles
			// therefore produced whichever LIST the model happened to call stronger,
			// and the other list was silently gone: runs of the same question return different
			// member counts, with no slot holding the complete list.
			//
			// The losing claim is kept in the slot's Alternates field —
			// the one field MergeSlotPatch unions, never replaces. The framework does not decide
			// which list is true; it stops discarding the one that lost.
			switch {
			case adoptBranch && v.Candidate != nil && *v.Candidate != "" && *v.Candidate != *bv.Candidate:
				loser = *v.Candidate
			case !adoptBranch && bv.Candidate != nil && *bv.Candidate != "" && (v.Candidate == nil || *v.Candidate != *bv.Candidate):
				loser = *bv.Candidate
			}
		}
		clues := runtime.Dedupe(append(append([]string(nil), v.DiscoveredClues...), bv.DiscoveredClues...))
		alternates := runtime.Dedupe(append(append([]string(nil), v.Alternates...), bv.Alternates...))
		if loser != "" {
			alternates = runtime.Dedupe(append(alternates, loser))
		}
		if !equalStringPtr(cand, v.Candidate) || !equalStrings(clues, v.DiscoveredClues) || !equalStrings(alternates, v.Alternates) {
			changed = true
		}
		merged = append(merged, runtime.Variable{
			ID:                v.ID,
			Type:              v.Type,
			QuestionClues:     append([]string(nil), v.QuestionClues...),
			DiscoveredClues:   clues,
			Alternates:        alternates,
			Candidate:         cand,
			CandidateStrength: strength,
			Value:             value,
			// The DECLARATION travels with the slot. Terms/Subject are what the
			// enumeration is built from (runtime.CoverageOf), and rebuilding the slot
			// without them is why a later round can have no enumeration at all: the seed carries
			// the method AND the enumerated windows on the first pass and only the method
			// afterwards, so the recovery round — the one the routing opened because the record
			// was still short — runs with the enumeration switched off.
			Terms:    append([]string(nil), v.Terms...),
			Subjects: v.Subjects,
		})
	}
	if !changed {
		return nil
	}
	out := runtime.NewState(merged, base.Depth+1, append([]string(nil), base.RetrievedEvidenceIDs...))
	return &out
}

// memberLine renders a slot holding items as each item WITH the passage behind it, so the citation
// instruction below ("every member you list carries the words behind it") has something to point
// at. Without the ids it asked for citations nobody could make.
func memberLine(v runtime.Variable) string {
	val := v.Typed()
	if val.Kind != slots.KindItems {
		return ""
	}
	parts := make([]string, 0, len(val.Items))
	for _, it := range val.Items {
		name := strings.TrimSpace(it.Value)
		if name == "" {
			continue
		}
		if id := strings.TrimSpace(it.ChunkID); id != "" {
			// The words travel WITH the name. The evidence block the answer reads is budgeted, so a
			// member whose passage did not fit used to arrive as a bare name and had to be dropped
			// from the list: measured (2026-09-17, 三国/关羽) 18 members against the 13 passages the
			// budget carried — the answer named the other five as "listed in the record, no original
			// text provided". The quote here is the verdict's own window (bounded by
			// coverageWindowBefore/After): the words the membership rests on, and the record's own
			// contract accepts "quoted above" as the evidence for a member.
			parts = append(parts, name+" ←"+id+itemQuote(it.Quote))
		} else {
			parts = append(parts, name+" ←(no passage)")
		}
	}
	return strings.Join(parts, "、")
}

// itemQuoteMaxRunes bounds the words a member carries into the record. Short because the record is
// read by the answer model and travelled with the name only to keep it citable at all.
const itemQuoteMaxRunes = 120

// itemQuote renders the words an item rests on, or nothing when the item has none: a member with no
// quote is a claim, and the record already says so with "(no passage)".
func itemQuote(quote string) string {
	if quote = strings.TrimSpace(quote); quote == "" {
		return ""
	}
	return " “" + runtime.TruncateRunes(quote, itemQuoteMaxRunes) + "”"
}

// The count RECONCILIATION used to live here: `syncCountSlots` overwrote every number-claiming
// slot with the size of the enumerated set (keeping the old claim as an alternate), and
// `isCountSlot` read the planner's word for a slot to decide what to say about it. Both are gone.
//
// What the machine did was answer part of the question: the set's size, written into the slot the
// answer reads its number from. That is the one place where "code reads the answer's structure"
// became "code writes the answer", and it is the one that cannot coexist with a session that reads
// the passages and writes the answer itself (see routeResearch and the answer contract in
// runtime/action_session.go). The members, their words and their ids are all still in the table —
// what is gone is the second author.
//
// What replaces it is a STATEMENT the record makes about itself (see RenderSlotRecord): the set is
// enumerated, and if no slot records a number then the record says so instead of inventing one.

// recordsANumber reports whether any slot holds a DECLARED number (see slots.KindCount /
// slots.KindRange). It reads the value, never the planner's word for the slot.
func recordsANumber(table runtime.State) bool {
	for _, v := range table.State {
		if _, ok := v.Typed().Number(); ok {
			return true
		}
	}
	return false
}

// countFilled counts slots holding a candidate.
func countFilled(slotTable runtime.State) int {
	n := 0
	for _, v := range slotTable.State {
		if v.Filled() {
			n++
		}
	}
	return n
}

// Graph driver.
