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
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/advanced_rag/slots"
)

// The slot-research stage: the table a round is driven by, the members and counts it
// holds, the passes that fill it, and the write-back folded into it.
// RenderSlotDraft
// render the slot table into a fact-preserving draft for the SCA.
//
// A resolved slot carries the model's own candidate strength, the evidence ids /
// terminal type of the passages that produced it (from slotEvidence), and the
// tail of its discovered clues, so the SCA can verify the candidate against the
// passages that actually produced it. Unresolved slots are listed explicitly
// with their question clues so the SCA can call them out as gaps. A collected
// answer leads the draft — it is the strongest candidate — with the same
// evidence metadata under the "_answer" key.
func RenderSlotDraft(slotTable harness.State, collectedAnswer string, slotEvidence map[string]SlotEvidence) string {
	var lines []string
	if collectedAnswer != "" {
		// The collected answer leads the draft — it is the strongest candidate — and
		// carries the same evidence metadata, read from the "_answer" key.
		lines = append(lines, "Candidate answer: "+collectedAnswer+draftEvidenceSuffix(slotEvidence["_answer"]))
		lines = append(lines, "")
	}
	if len(slotTable.State) == 0 {
		return strings.Join(lines, "\n")
	}
	for _, v := range slotTable.State {
		vtype := v.Type
		if vtype == "" {
			vtype = "entity"
		}
		cand := ""
		if v.Candidate != nil {
			cand = *v.Candidate
		}
		if cand == "" {
			clues := v.QuestionClues
			if len(clues) > 2 {
				clues = clues[:2]
			}
			lines = append(lines, fmt.Sprintf("- slot %d [%s]: NOT RESOLVED (%s)",
				v.ID, vtype, strings.Join(truncateEach(clues, draftUnresolvedClueChars), "; ")))
			continue
		}
		strength := "?"
		if v.CandidateStrength != nil {
			strength = fmt.Sprintf("%.2f", *v.CandidateStrength)
		}
		line := fmt.Sprintf("- slot %d [%s]: %s (strength=%s)%s",
			v.ID, vtype, cand, strength, draftEvidenceSuffix(slotEvidence[fmt.Sprint(v.ID)]))
		if tail := draftClueTail(v.DiscoveredClues); tail != "" {
			line += " — " + tail
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// RenderSlotRecord renders the slot table for the ANSWER prompt.
//
// It is deliberately NOT RenderSlotDraft. The draft exists for the SCA, which
// verifies a candidate against the passages that produced it, so it carries the
// machine fields that make that verification possible — the candidate strength,
// the terminal type, the evidence ids. Handing those to the answer model is a
// different act with a different failure mode: composing with the draft as
// "Research Summary (primary evidence)" produces an answer that quotes the
// bookkeeping verbatim, machine fields and all.
//
// So the answer sees the FACTS the research settled — which slot holds what —
// with no strength, no evidence ids, no clue tails: the evidence ids are already
// in the evidence block with their citation markers, and the strengths are the
// runtime's business. The prompt labels the block as the model's own record and
// tells it not to copy the lines (see answerPromptWithEvidence).
func RenderSlotRecord(slotTable harness.State, collectedAnswer string) string {
	var lines []string
	// Is this record about a SET at all? Everything this function does beyond the
	// slots themselves — the enumerated size, the count/members warning, the
	// demotion of the session's own prose — is about reconciling a list with a
	// count, and a single-value question has neither.
	//
	// Rendered ungated, the enumerated line and the prose demotion appear on questions
	// whose answer is one date or one number, where a "count" computed from a single
	// value's text is meaningless: one name cut at its separators reads as several
	// members, and the session's own draft answer is then demoted below a line that does
	// not apply to it.
	// The record's SET handling follows the PLANNER'S DECLARATION — count / set / list,
	// the shape half of harness.Coverage — exactly as it did before the enumeration was
	// refactored: a table the planner typed as a set states its size and demotes a
	// session's prose, while a table whose slots hold values (one name, one date, one
	// number, one phrase) does neither. The size line additionally needs at least one
	// member to be worth printing, and a member can only come from a slot that HOLDS
	// members (see harness.MemberNames): the waterfall whose name was cut at its
	// separators is a TEXT slot, contributes no members, and is never counted.
	enumerating := harness.CoverageOf(slotTable).Set
	if !enumerating && collectedAnswer != "" {
		// A value record leads with the session's own answer, exactly as it did
		// before any of this existed (see the note on the demotion below for why the
		// SET case is different).
		lines = append(lines, "Candidate answer: "+collectedAnswer)
		lines = append(lines, "")
	}
	for _, v := range slotTable.State {
		vtype := v.Type
		if vtype == "" {
			vtype = "entity"
		}
		switch {
		case memberLine(v) != "":
			lines = append(lines, fmt.Sprintf("- slot %d [%s]: %s", v.ID, vtype, memberLine(v)))
		case v.Candidate == nil || *v.Candidate == "":
			lines = append(lines, fmt.Sprintf("- slot %d [%s]: NOT RESOLVED", v.ID, vtype))
		default:
			lines = append(lines, fmt.Sprintf("- slot %d [%s]: %s", v.ID, vtype, *v.Candidate))
		}
		// Claims that lost the slot comparison (see MergeSlotPatch). They are not
		// the slot's value, but they are not nothing either: a session produced
		// them from the same corpus, and a name only in an alternate is a name the
		// answer has to account for.
		for _, alt := range alternateCandidatesOf(v) {
			lines = append(lines, "    alternate (claimed by another session, not adopted): "+truncateRunes(alt, 400))
		}
	}
	// The size of the set the slots above ENUMERATE — a fact the answer can take a
	// number from without trusting anybody's prose.
	//
	// A count slot is written by whichever session last touched it, and that
	// candidate can be a sentence ("约 14 人（华雄、程远志…）"), a stale number, or
	// one session's claim written before the other sessions' findings were merged.
	// The members are here in the table either way, so their union is the number
	// the record can stand behind.
	if n := enumeratedSize(slotTable); enumerating && n > 0 {
		lines = append(lines, fmt.Sprintf("- enumerated members across the slots above: %d", n))
		// A set answer is only as good as what it can point at, member by member. Handed a
		// list of names and no per-member evidence, an answer counts members it cannot cite
		// and cites one range for all of them. The citation contract already forbids ranges;
		// this states it where a set answer is assembled, together with the rule that keeps a
		// count honest — a member nobody can point at a passage is not counted.
		lines = append(lines, "State the count and the members TOGETHER: every member you list carries the words behind it (quoted above, or its own [ID:n]) — never one range for the list — and a member you cannot point at a passage for is left out of both the list and the count.")
		// The names the number above leaves out are listed, so the answer can tell a complete list
		// from one that is short by a claim nobody could quote.
		if claims := harness.UnanchoredItems(&slotTable); len(claims) > 0 {
			lines = append(lines, fmt.Sprintf(
				"- claimed WITHOUT a passage in hand (NOT in the %d above — the count may be short by up to %d): %s. Each one is a name somebody asserted and no passage in hand states: find the words that put it in the answer, or leave it out of both the list and the count.",
				n, len(claims), strings.Join(claims, "、")))
		}
		// A count larger than the members it counts is a claim about members that are NOT in
		// the record, and the answer has to be told that rather than left to reconcile it:
		// left alone it explains the gap as "more members whose details the material does not
		// list" — members that never existed.
		//
		// The note STATES the disagreement; it does not order which number to take. An order
		// was tried and reverted: told to take the number from the enumerated members, the
		// answer took it even when that number had been computed over the wrong slots. A count
		// can be an over-claim and a list can be incomplete; only the passages decide between
		// them, and the answer is the stage that reads them.
		for _, v := range slotTable.State {
			// A DECLARED number (slots.KindCount / KindRange) compared with the members
			// above. Text claims no number, so prose can no longer masquerade as a
			// count here (or as a member list on the other side of the comparison).
			if claimed, ok := v.Typed().Number(); ok {
				if claimed == n {
					continue
				}
				lines = append(lines, fmt.Sprintf(
					"- NOTE: slot %d [%s] says %d while the slots above enumerate %d — the count and the members listed disagree. Reconcile them against the evidence before answering: a count larger than the members that are listed is not evidence of members, and a list is only as complete as the passages behind it.",
					v.ID, v.Type, claimed, n))
				continue
			}
			// The other half of the same disagreement: a count slot holding WORDS. It claims no
			// number by contract (KindText is opaque and nothing derives from it), so it used to
			// pass in silence — the record showed its prose beside the enumerated size and let the
			// answer take either, which is how 14 became an answer to a table that enumerated 16.
			// The words are not parsed; the disagreement is stated.
			if isCountSlot(v) && v.Typed().Kind != slots.KindItems && strings.TrimSpace(*v.Candidate) != strconv.Itoa(n) {
				lines = append(lines, fmt.Sprintf(
					"- NOTE: slot %d [%s] holds %q as text, not a number, so nothing was derived from it while the slots above enumerate %d. State the count the evidence supports.",
					v.ID, v.Type, truncateRunes(strings.TrimSpace(*v.Candidate), 60), n))
			}
		}
	}
	// A session's own draft answer goes LAST and is labelled for what it is.
	//
	// It used to lead the record, and the answer copied it: whatever number a session wrote
	// in its own prose became the number the answer reported, even when the slots above
	// enumerated a different one. The prose is one session's recollection, written before the
	// other sessions' findings were merged into the table above; it is a claim to reconcile
	// with the members, not the record.
	//
	// On a VALUE record there are no members to reconcile against — the prose is the
	// only candidate answer the record has, so it leads the record instead (see the
	// top of this function), which is also what every run before the demotion scored
	// on.
	if collectedAnswer != "" && enumerating {
		lines = append(lines, "")
		// The draft is not a member LIST — copying its prose is how a fifteen-member answer
		// came out of a seventeen-member record — but the PASSAGES it quotes are evidence
		// like any other, and a name those passages attribute to the actor is a member even
		// even when no slot above lists it. A record can enumerate far fewer members than its
		// own draft quotes the text for, and an answer told that "the members stand" then drops
		// every one of them. The evidence was in hand; the rule threw it away. So the draft is
		// demoted as a SOURCE
		// of members and promoted as evidence: its quotations are the arbiter, and neither
		// the slots nor the draft decides on its own.
		lines = append(lines, "One session's own draft answer (UNVERIFIED — written before the other sessions were merged. Its PROSE is not a member list: do not copy its count or its wording. Its QUOTATIONS are evidence like any other: a name those passages attribute to the actor is a member even when no slot above lists it, and a name whose passage attributes the deed to someone else is not. Reconcile the draft with the slots — with the quotations as the arbiter, not either list — and include every member the evidence supports): "+collectedAnswer)
	}
	return strings.Join(lines, "\n")
}

// enumeratedSize is how many distinct items the table can point at (harness.AnchoredItems): one
// source spells one item several ways, and a set counts entities, not spellings.
func enumeratedSize(table harness.State) int { return len(harness.AnchoredItems(&table)) }

// BuildSlotTable: decompose the question into
// a slot table, seeding it with the planner's fan-outs.
//
// Returns (root, firstQueries) — never an empty root: on failure it degrades to
// one "aspect" slot per fan-out (or a single "answer" slot for the raw
// question), so the research pass always has something to work on.
func BuildSlotTable(ctx context.Context, deps harness.SessionDeps, question string, fanouts []string, deadlineLeft float64) (harness.State, []string) {
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
		vars := make([]harness.Variable, 0, 4)
		for i, q := range queries {
			if i >= 4 {
				break
			}
			vars = append(vars, harness.Variable{
				ID:            i,
				Type:          "aspect",
				QuestionClues: []string{truncateRunes(q, slotFallbackClueChars)},
			})
		}
		root = harness.NewState(vars, 0, nil)
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

// BatchFillSlots: answer several
// unresolved slots in ONE call when their evidence overlaps.
//
// Pure efficiency: neither the slot structure nor the evidence semantics
// change — only the number of generation calls made over the same passages.
//
// Best-effort inside: a missing model, a failed call, or an unparsable reply skips that
// cluster and the remaining clusters still run.
func BatchFillSlots(ctx context.Context, deps harness.SessionDeps, slotTable *harness.State, slotEvidence map[string]SlotEvidence) int {
	// Unresolved slots only, with slot_by_id over them.
	unresolvedN := 0
	slotByID := map[int]*harness.Variable{}
	evIDs := map[int][]string{} // ordered evidence ids, first-seen order
	evSet := map[int]map[string]bool{}
	for i := range slotTable.State {
		v := &slotTable.State[i]
		if v.Filled() {
			continue
		}
		unresolvedN++
		slotByID[v.ID] = v
		// evidence ids recorded for this slot by its session,
		// keyed by str(slot id), blanks dropped.
		ids := map[string]bool{}
		var ordered []string
		for _, id := range slotEvidence[fmt.Sprint(v.ID)].EvidenceIDs {
			if id == "" || ids[id] {
				continue
			}
			ids[id] = true
			ordered = append(ordered, id)
		}
		if len(ordered) > 0 {
			evIDs[v.ID] = ordered
			evSet[v.ID] = ids
		}
	}
	if len(evSet) < 2 {
		// Diagnostics: batching needs TWO slots that are
		// both unresolved AND carrying evidence. Log why it did not happen,
		// otherwise a never-firing path is indistinguishable from a working one.
		_LOG.Printf("[SlotResearch] batching skipped: unresolved=%d with_evidence=%d", unresolvedN, len(evSet))
		return 0
	}

	ids := make([]int, 0, len(evSet))
	for id := range evSet {
		ids = append(ids, id)
	}
	// The id set has no order of its own, so sort first: the clustering below is
	// deterministic across runs only because this order is fixed.
	sort.Ints(ids)

	// sim: Jaccard over evidence-id sets.
	sim := func(a, b int) float64 {
		inter := intersectionSize(evSet[a], evSet[b])
		union := len(evSet[a]) + len(evSet[b]) - inter
		if union == 0 {
			return 0.0
		}
		return float64(inter) / float64(union)
	}

	// Largest-incompatible-first (APT-RAG,): place the least
	// compatible slot first, so it is not left without a cluster at the end.
	// Counts are precomputed: the sort key is fixed before sorting.
	incompatible := make(map[int]int, len(ids))
	for _, i := range ids {
		n := 0
		for _, j := range ids {
			if j != i && sim(i, j) < EvidenceBatchMinSim {
				n++
			}
		}
		incompatible[i] = n
	}
	order := append([]int(nil), ids...)
	sort.SliceStable(order, func(x, y int) bool { return incompatible[order[x]] > incompatible[order[y]] })

	// Greedy clustering（）: a slot joins the first cluster
	// that is under the size cap AND shares ≥MIN_SHARED ids AND ≥MIN_SIM with
	// EVERY member; otherwise it starts its own cluster.
	clusters := [][]int{}
	for _, i := range order {
		placed := false
		for ci := range clusters {
			cl := clusters[ci]
			if len(cl) >= EvidenceBatchMaxSlots {
				continue
			}
			compatible := true
			for _, j := range cl {
				if intersectionSize(evSet[i], evSet[j]) < EvidenceBatchMinShared || sim(i, j) < EvidenceBatchMinSim {
					compatible = false
					break
				}
			}
			if compatible {
				clusters[ci] = append(cl, i)
				placed = true
				break
			}
		}
		if !placed {
			clusters = append(clusters, []int{i})
		}
	}

	// by_chunk_id（）over the whole pool.
	byChunkID := map[string]map[string]any{}
	if deps.KB != nil {
		for _, c := range deps.KB.Chunks {
			if cid := harness.ChunkIDOf(c); cid != "" {
				byChunkID[cid] = c
			}
		}
	}

	filled := 0
	for _, cl := range clusters {
		if len(cl) < 2 {
			continue
		}
		// -1645 builds union_ids as a set (arbitrary order); the
		// port keeps first-seen order across the cluster for determinism.
		unionIDs := []string{}
		seenID := map[string]bool{}
		for _, i := range cl {
			for _, id := range evIDs[i] {
				if !seenID[id] {
					seenID[id] = true
					unionIDs = append(unionIDs, id)
				}
			}
		}
		body, total := []string{}, 0
		for _, cid := range unionIDs {
			c := byChunkID[cid]
			if c == nil {
				continue
			}
			// content_with_weight first, content fallback.
			text := anyString(c["content_with_weight"])
			if text == "" {
				text = anyString(c["content"])
			}
			text = strings.TrimSpace(text)
			if text == "" {
				continue
			}
			// The cap counts RUNES, not bytes: a clue that is CJK would otherwise spend
			// three bytes of the budget per character.
			n := utf8.RuneCountInString(text)
			if total+n > EvidenceBatchMaxChars {
				break
			}
			body = append(body, text)
			total += n
		}
		if len(body) == 0 {
			continue
		}

		lines := []string{}
		for _, sid := range cl {
			v := slotByID[sid]
			if v == nil {
				continue
			}
			// join ALL clues, then cut the JOINED text
			// to 300 code points.
			clues := truncateRunes(strings.Join(v.QuestionClues, "; "), 300)
			lines = append(lines, fmt.Sprintf("- %d: %s", sid, clues))
		}
		if len(lines) < 2 {
			continue
		}

		// .
		user := "Sub-questions:\n" + strings.Join(lines, "\n") + "\n\nShared evidence:\n" + strings.Join(body, "\n---\n")
		// no model: stop batching entirely.
		if deps.Model == nil {
			return filled
		}
		// async_chat(system_prompt, [user], answer_conf);
		// the Go SessionModel takes the system message inline.
		reply, err := deps.Model.Complete(ctx, []schema.Message{
			*schema.SystemMessage(EvidenceBatchPrompt),
			*schema.UserMessage(user),
		}, nil)
		if err != nil {
			// one failed call must not cost the other
			// clusters.
			_LOG.Printf("[SlotResearch] batched answer call failed: %v", err)
			continue
		}

		// _extract_json_object; a non-object reply
		// skips the cluster.
		data, ok := extractJSONObject(reply.Content).(map[string]any)
		if !ok {
			continue
		}
		for _, sid := range cl {
			v := slotByID[sid]
			if v == nil {
				continue
			}
			// the reply maps str(slot id) to its answer;
			// never overwrite an already-filled slot, and a blank answer
			// counts as "evidence does not answer this".
			val, isStr := data[fmt.Sprint(sid)].(string)
			if !isStr {
				continue
			}
			val = strings.TrimSpace(val)
			if val == "" || v.Filled() {
				continue
			}
			cand := truncateRunes(val, 400)
			v.Candidate = &cand
			filled++
		}
	}
	return filled
}

// PrefillSlotsFromEvidence
// answer slots that an already-pooled evidence row directly answers.
//
// An evidence row is an atomic proposition carrying a verbatim quote, so when
// it already covers a slot's question there is nothing for an action session
// to research — skipping it removes a WHOLE session (the dominant cost), not
// just tokens inside one. Saves calls, uses real evidence, and a wrong guess
// is still caught later by the SCA.
func PrefillSlotsFromEvidence(slotTable *harness.State, kb *harness.Kbinfos) int {
	// evidence rows are the claim pseudo-chunks.
	evRows := []map[string]any{}
	if kb != nil {
		for _, c := range kb.Chunks {
			if strings.HasPrefix(harness.ChunkIDOf(c), evidenceChunkPrefix) {
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
			for _, t := range harness.QueryToTerms(c) {
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
		// harness.ClaimPseudoChunks). The OTHER producers — the action-session claim
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
		cand := truncateRunes(name, 400)
		v.Candidate = &cand
		strength := bestCov
		v.CandidateStrength = &strength
		filled++
	}
	return filled
}

// logCoverageWindows writes the enumeration's windows to the run log — the QUOTES, not just their
// count.
//
// Whether a member the answer missed was ever IN FRONT of a session is otherwise unknowable, and
// that difference decides which fault to fix: a name quoted inside a window the sessions read and
// did not write is a WRITE-BACK fault (the record, the last node), while a name the enumeration
// never showed is a COVERAGE fault (the operand recall, the window budget). Without the quotes a
// log cannot say whether a missing member was ever shown at all, and the same code answers very
// different member counts on one question, so every window change made from such a log is a
// guess, and two of them were wrong.
func logCoverageWindows(set harness.CoverageSet) {
	for _, w := range set.Windows {
		_LOG.Printf("[Coverage] window chunk_id=%s act=%s %q", w.ChunkID, w.Act, truncateRunes(w.Quote, 120))
	}
}

// RunSlotResearchPass: drive ONE
// research round with slot-aware action sessions.
//
// Unresolved slots are worked concurrently under a semaphore; each session's
// branches are folded back into the shared table. A nil result means "nothing to
// do" (all slots already filled).
func RunSlotResearchPass(ctx context.Context, parent context.Context, deps harness.SessionDeps, question string, st *AgenticState, deadlineLeft float64) *SlotResearchResult {
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
	var dirs []direction
	for _, v := range unresolved {
		text := question
		if len(v.QuestionClues) > 0 {
			text = v.QuestionClues[0]
		}
		dirs = append(dirs, direction{slotID: v.ID, text: text})
	}
	if len(dirs) == 0 {
		for _, g := range SCAGapsToRewrite(st.SCA) {
			text := strings.TrimSpace(g.SearchHint)
			if text == "" {
				text = strings.TrimSpace(g.What)
			}
			if text == "" {
				continue
			}
			// slotID -1: a gap is not a slot, and whatever the session patches is
			// applied by id in the fold, so nothing is attributed to a slot the
			// review never named.
			dirs = append(dirs, direction{slotID: -1, text: text})
		}
		if len(dirs) == 0 {
			// Nothing for a session to do. A PREFILL still counts as work — it
			// edited the table — so its result must travel back to the caller;
			// only a round that changed nothing at all is a nil pass.
			if prefillN == 0 {
				_LOG.Printf("[SlotResearch] all slots filled and the review named no gap; no session to run.")
				return nil
			}
		} else {
			_LOG.Printf("[SlotResearch] all slots filled, but the review named %d gap(s); running session(s) on them.", len(dirs))
		}
	}

	// Shared across sessions so duplicate retrievals are served from cache.
	sharedToolCache := harness.NewToolCache()
	var sharedSearchQueries []string

	sem := make(chan struct{}, slotSessionConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	type outcome struct {
		slotID int
		// direction travels with the result so the round's ledger can say what this
		// session was ASKED to research without re-deriving it from its messages (see
		// the ledger row below).
		direction string
		result    harness.Result
	}
	results := make([]outcome, 0, slotSessionsPerRound)
	limit := min(len(dirs), slotSessionsPerRound)

	// The direction's SHAPE is only known here, after the table was built — and the
	// round's own context was fixed before that, by a caller that could not know it.
	// A session therefore cannot ride this round's clock, or the enumeration it is
	// midway through is cut before it can patch: the pass timeout can cancel a session at the
	// deadline just before its patch, and the member it had already reached — its passages
	// admitted to the shared pool by the session's own batch — dies with it. So an
	// enumeration pass
	// hangs its sessions off the PARENT context with the shape's own clock
	// (harness.SessionWallS), and buys the question the one-shot budget extension
	// that lets the NEXT round start and pick up whatever this one could not record.
	sessionCtx := ctx
	sessionBudget := max(20.0, deadlineLeft-10.0)
	if harness.CoverageOf(slotTable).Ok() {
		sessionBudget = max(sessionBudget, harness.SessionWallS(slotTable))
		if parent != nil {
			var cancelSessions context.CancelFunc
			sessionCtx, cancelSessions = context.WithTimeout(parent, deadlineToDuration(sessionBudget+setSessionSlackS))
			defer cancelSessions()
		}
		// The extension must leave room for what FOLLOWS the research — the review,
		// the draft, the composed answer (downstreamReserveS) — inside the caller's own
		// deadline: buying time the answer then lacks is the one way this change could
		// make a question worse (a timed-out request instead of a missing member). So
		// the request's remaining room decides, not the extension's own size, and an
		// extension too small to let a round start is not worth buying at all.
		ext := SetBudgetExtensionS
		if room, bounded := ctxRoomS(parent); bounded {
			ext = min(ext, room-st.RemainingS()-downstreamReserveS)
		}
		if ext >= minBudgetExtensionS {
			if st.ExtendDeadline(ext) {
				_LOG.Printf("[SlotResearch] enumeration table: session clock %.0fs, research budget extended by %.0fs so a following round can pick up what this one could not record.",
					sessionBudget, ext)
			}
		} else {
			_LOG.Printf("[SlotResearch] enumeration table: session clock %.0fs; research budget NOT extended (only %.0fs of the request is left, and %.0fs is reserved for the review, the draft and the answer).",
				sessionBudget, ctxLeftS(parent), downstreamReserveS)
		}
	}

	// The ENUMERATION runs HERE — in code, before any session starts.
	//
	// The direction's own act words used to be rendered into the seed as a list of queries to
	// make, and that is not enough: a list of queries in a prompt is advice, and advice may
	// simply not be taken — the sessions improvise their own word lists instead and re-probe
	// the same names by hand in the next round. The completeness of an enumeration cannot
	// rest on advice. So the runtime asks the corpus ITSELF — one recall per operand, the
	// windows where the deed is stated (harness.EnumerateCoverage) — admits them to the pool,
	// and seeds the sessions with what came back: the session's job becomes reading evidence
	// rather than guessing names.
	//
	// Run once per QUESTION, not once per round: the windows stay in the pool under the same
	// ids and the set is kept (Kbinfos.CoverageSet), so the last node resolves the same
	// windows a later round would have re-found.
	cov := harness.CoverageOf(slotTable)
	switch {
	case !cov.Ok():
		// A table of counts and dates declares act words too (the planner is told to for "a
		// count of things someone DID"), and no name an enumeration could return changes a
		// count of events: running it there only carries a reading list into sessions that
		// cannot use it.
		if len(cov.Acts) > 0 {
			_LOG.Printf("[SlotResearch] %d act word(s) declared but this table is not an ENUMERATION (it declared no count/set/list slot, or no NAME-carrying slot) — not run: a value question pays nothing for a set's bookkeeping.", len(cov.Acts))
		}
	case kb != nil:
		if set, done := kb.CoverageSet(); done {
			deps.CoverageSeed = set.Render()
			_LOG.Printf("[SlotResearch] enumeration already ran for this question; reusing its %d window(s) over %d operand(s).", len(set.Windows), len(set.Operands))
		} else if deps.Tools != nil {
			if runner, ok := deps.Tools.Exec.(harness.CoverageRunner); ok {
				set := runner.EnumerateCoverage(ctx, cov, kb)
				if len(set.Operands) > 0 {
					_LOG.Printf("[SlotResearch] enumeration: %d operand(s) asked, %d passage(s) recalled, %d window(s) found; seed +%d char(s).",
						len(set.Operands), set.Recalled, len(set.Windows), len(set.Render()))
					logCoverageWindows(set)
					kb.MarkCoverage(cov)
					kb.StoreCoverageSet(set)
					deps.CoverageSeed = set.Render()
				}
			} else {
				// Said out loud, like the batching diagnostic above: a step that can
				// never fire has to be distinguishable from one that works, or the
				// sessions silently read a seed with no evidence in it.
				_LOG.Printf("[SlotResearch] the tool executor cannot enumerate (it does not implement harness.CoverageRunner); the direction's act words were NOT run over the corpus.")
			}
		}
	}

	for i := 0; i < limit; i++ {
		d := dirs[i]
		wg.Add(1)
		go func(d direction) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Sessions share ONE Toolset; DisableTool mutates it, so the call is
			// guarded here. The Kbinfos merge happens inside the executor.
			res := harness.RunActionSession(sessionCtx, deps, d.text, slotTable, sessionBudget, "", sharedToolCache, sharedSearchQueries)
			mu.Lock()
			results = append(results, outcome{slotID: d.slotID, direction: d.text, result: res})
			mu.Unlock()
		}(d)
	}
	wg.Wait()

	// Fold in slot-id order so the merge is deterministic regardless of which
	// session finished first.
	sort.Slice(results, func(i, j int) bool { return results[i].slotID < results[j].slotID })

	collected := st.CollectedAnswer
	sessionEvidence := map[string]SlotEvidence{}
	ledger := append([]map[string]any(nil), st.Attempted...)

	for _, item := range results {
		r := item.result
		if r.FoundAnswer != nil && collected == "" {
			collected = *r.FoundAnswer
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
				EvidenceIDs:  dedupe(r.RetrievedEvidenceIDs),
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
			q = truncateRunes(*r.FoundAnswer, 80)
		} else {
			// No answer: the row shows what this session was ASKED to research, which is
			// the query of the pair (query, outcome). It used to be a langchain-style repr
			// of the session's messages, and that repr is a CONSTANT in production: every
			// session's history opens with the same system prompt, so its first 80 runes —
			// a message header, not a query — went into every row of every question.
			q = truncateRunes(item.direction, 80)
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
			"bound": len(dedupe(r.RetrievedEvidenceIDs)),
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

	// Evidence-guided batching（）: slots that retrieved the
	// same passages get answered together instead of one generation call each.
	// BatchFillSlots is best-effort internally: per-cluster failures are logged and
	// skipped, so one bad cluster never costs the others.
	if batched := BatchFillSlots(ctx, deps, &slotTable, sessionEvidence); batched > 0 {
		_LOG.Printf("[SlotResearch] batched generation filled %d slot(s)", batched)
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

	// One set, one answer: a table can hold the members in one slot and the count in
	// another, written by different sessions, with nothing keeping them in step. Raised
	// here, before the draft the answer is written from.
	if raised := syncCountSlots(&slotTable); len(raised) > 0 {
		_LOG.Printf("[SlotResearch] count slot(s) %v set to the enumerated members' size", raised)
	}

	draft := RenderSlotDraft(slotTable, collected, sessionEvidence)
	_LOG.Printf("[SlotResearch] round done — %d slot(s) filled, unresolved=%d, collected_answer=%v",
		countFilled(slotTable), len(unresolvedOut), collected != "")
	_LOG.Printf("[SlotResearch] slot table after round:\n%s", draft)

	return &SlotResearchResult{
		SlotTable:       slotTable,
		CollectedAnswer: collected,
		UnresolvedSlots: unresolvedOut,
		SlotEvidence:    sessionEvidence,
		SlotDraft:       draft,
		// The answer-facing record (no machine fields) is rendered here, next to
		// the SCA-facing draft, so the two can never drift apart.
		SlotRecord: RenderSlotRecord(slotTable, collected),
		Attempted:  ledger,
	}
}

// alternateCandidatesOf returns the candidates a slot lost to, in the order they were
// kept (see harness.Variable.Alternates).
func alternateCandidatesOf(v harness.Variable) []string {
	return append([]string(nil), v.Alternates...)
}

// logSessionPatch reports one session's claims and what the merge did with them.
//
// One line per patched slot, because the losing side of MergeSlotPatch leaves no
// trace anywhere else: a candidate that was dropped is not in the table, not in
// the draft, not in the record — it is simply gone, and "gone" is the failure this
// line makes visible.
func logSessionPatch(directionSlot int, before, after, patch harness.State) {
	for _, pv := range patch.State {
		if pv.Candidate == nil || *pv.Candidate == "" {
			continue
		}
		bv := before.ByID(pv.ID)
		base := "(empty)"
		if bv != nil && bv.Candidate != nil && *bv.Candidate != "" {
			base = fmt.Sprintf("%q (%.2f)", truncateRunes(*bv.Candidate, 80), strengthOf(*bv))
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
			directionSlot, pv.ID, truncateRunes(*pv.Candidate, 120), strengthOf(pv), base, result)
	}
}

// MergeSlotPatch folds a session's new-state branch into the shared slot table: a slot
// holding a SET merges by union, anything else adopts the STRONGER candidate (see
// slots.Union for why).
//
// Returns nil when nothing changed, so
// callers can skip no-op merges — and so a round that changed nothing can be told apart
// from one that did.
func MergeSlotPatch(base, branch harness.State) *harness.State {
	if len(branch.State) == 0 {
		return nil
	}
	branchByID := map[int]harness.Variable{}
	for _, v := range branch.State {
		branchByID[v.ID] = v
	}
	merged := make([]harness.Variable, 0, len(base.State))
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
		clues := dedupe(append(append([]string(nil), v.DiscoveredClues...), bv.DiscoveredClues...))
		alternates := dedupe(append(append([]string(nil), v.Alternates...), bv.Alternates...))
		if loser != "" {
			alternates = dedupe(append(alternates, loser))
		}
		if !equalStringPtr(cand, v.Candidate) || !equalStrings(clues, v.DiscoveredClues) || !equalStrings(alternates, v.Alternates) {
			changed = true
		}
		merged = append(merged, harness.Variable{
			ID:                v.ID,
			Type:              v.Type,
			QuestionClues:     append([]string(nil), v.QuestionClues...),
			DiscoveredClues:   clues,
			Alternates:        alternates,
			Candidate:         cand,
			CandidateStrength: strength,
			Value:             value,
			// The DECLARATION travels with the slot. Terms/Subject are what the
			// enumeration is built from (harness.CoverageOf), and rebuilding the slot
			// without them is why a later round can have no enumeration at all: the seed carries
			// the method AND the enumerated windows on the first pass and only the method
			// afterwards, so the recovery round — the one the routing opened because the record
			// was still short — runs with the enumeration switched off.
			Terms:   append([]string(nil), v.Terms...),
			Subject: v.Subject,
		})
	}
	if !changed {
		return nil
	}
	out := harness.NewState(merged, base.Depth+1, append([]string(nil), base.RetrievedEvidenceIDs...))
	return &out
}

// memberLine renders a slot holding items as each item WITH the passage behind it, so the citation
// instruction below ("every member you list carries the words behind it") has something to point
// at. Without the ids it asked for citations nobody could make.
func memberLine(v harness.Variable) string {
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
	return " “" + truncateRunes(quote, itemQuoteMaxRunes) + "”"
}

// isCountSlot reports whether a slot declares itself a count — the planner's word for a number
// slot. It decides only whether a disagreement is worth STATING; nothing is derived from it
// (syncCountSlots reads the value's KIND for that, never the type word).
func isCountSlot(v harness.Variable) bool {
	return v.Candidate != nil && strings.EqualFold(strings.TrimSpace(v.Type), "count")
}

// countDerivedFloor is the smallest set a count may be derived from: one quotable item is not an
// enumeration, and a half-filled table must not collapse a claimed number to one.
const countDerivedFloor = 2

// syncCountSlots writes the size of the enumerable set into every slot that claims a number, and
// keeps the old claim as an alternate. Only a number-claiming slot is touched (slots.Value.Number),
// and the size counts the items the table can point at (harness.AnchoredItems): a count of claims
// is a count of nothing.
//
// The size is read across the whole table, so a question with two sets gets one number for both
// until the planner says which count counts which set. Below countDerivedFloor there is nothing to
// derive from and the claim stands.
//
// It returns the ids it changed, for the log.
func syncCountSlots(table *harness.State) []int {
	if table == nil || len(table.State) == 0 {
		return nil
	}
	union := harness.AnchoredItems(table)
	if len(union) < countDerivedFloor {
		return nil
	}
	var raised, unreadable []int
	for i := range table.State {
		v := &table.State[i]
		claimed, ok := v.Typed().Number()
		if !ok {
			// A slot holding ITEMS is read, just not as a number: the size is derived from the items
			// and the record states it, so it is not an unreadable count (a session may patch the
			// members into the slot the planner typed "count").
			if isCountSlot(*v) && v.Typed().Kind != slots.KindItems {
				unreadable = append(unreadable, v.ID)
			}
			continue
		}
		if claimed == len(union) {
			continue
		}
		// The claim loses either way — a count larger than the members listed is not evidence
		// of members, and a smaller one is a set that lost some. Left standing, the claim is the
		// number the answer reports, whatever the enumerated members say.
		//
		// Deduped because this runs after EVERY pass (and again after the last node), so
		// an undeduped append printed the same alternate line once per run.
		v.Alternates = dedupe(append(v.Alternates, slots.Render(v.Typed())))
		derived := slots.Number(len(union))
		v.Value = &derived
		rendered := slots.Render(derived)
		v.Candidate = &rendered
		raised = append(raised, v.ID)
	}
	if len(unreadable) > 0 {
		// A count slot holding words cannot be corrected — nothing derives from text — so it is
		// said out loud instead, and the record states the disagreement (see RenderSlotRecord).
		_LOG.Printf("[Coverage] count slot(s) %v hold text, not a number: nothing was derived from them", unreadable)
	}
	return raised
}

// countFilled counts slots holding a candidate.
func countFilled(slotTable harness.State) int {
	n := 0
	for _, v := range slotTable.State {
		if v.Filled() {
			n++
		}
	}
	return n
}

// Graph driver.
