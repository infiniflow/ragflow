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
	"strings"
	"time"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/advanced_rag/slots"
)

// The enumeration's LAST node: the runtime asks the model about every window the
// enumeration found, and writes the members its verdicts name.
//
// Why this node exists at all: the evidence is not the limit — with the same corpus in
// hand, runs of one build answer different member counts. What varies is what a session
// WROTE: each session patches a handful of names and the round's union is whichever list
// happens to be longest. So the write-back stops being a session's memory and becomes the
// runtime's own step — every window gets a verdict, and a window nobody answered is
// reported as UNKNOWN rather than read as "the corpus does not say it".
//
// The judgement itself stays the model's (see harness.CoverageResolvePrompt): nothing here
// decides what a kill is, or which words mean one.
func RunCoverageResolve(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) harness.ResolveStats {
	if logger == nil {
		logger = _LOG
	}
	stats := harness.ResolveStats{}
	if st == nil || deps.Model == nil || st.KB == nil || len(st.SlotTable.State) == 0 {
		return stats
	}
	// The node belongs to the ENUMERATION strategy: a question whose answer is one value
	// even one that contains a count — pays nothing for it (see harness.Coverage.Ok).
	cov := harness.CoverageOf(st.SlotTable)
	if !cov.Ok() {
		return stats
	}
	// And the members go into a slot that ALREADY holds members, so a table whose slots
	// hold values (one name, one date, one number, one phrase) is not enumerated at all
	// and this step spends nothing — not even a call.
	slotID := coverageTargetSlot(&st.SlotTable)
	if slotID < 0 {
		return stats
	}
	set, reached, claimed := coverageResolveCandidates(st.KB, &st.SlotTable, cov)
	if set.Empty() {
		// Nothing to judge: no enumeration ran (no clock left, or the executor cannot search)
		// and no probe reached a name the slots do not already hold.
		return stats
	}
	if reached+claimed > 0 {
		// The split is worth a line: it is the difference between a node that judges only what
		// its own enumeration recalled and one that also judges what the run's search touched
		// and what the sessions ASSERTED but never showed a passage for.
		logger.Printf("[Coverage] resolve candidates: %d window(s) + %d reached name(s) + %d unjudged claim(s) = %d line(s)",
			len(set.Windows)-reached-claimed, reached, claimed, len(set.Windows))
	}

	// Bounded like every other single-purpose call in this graph: the answer composition
	// still has to fit inside the request's clock, so this step may not spend it.
	t := nodeClock(CoverageResolveTimeoutS, 10.0, st.RemainingS()-10.0)
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
	defer cancel()

	members, stats := harness.ResolveCoverage(callCtx, deps.Model, st.Question, cov, set)
	if stats.Failed > 0 {
		// Said out loud because a failed call is members nobody checked — the failure this
		// node exists to make visible, not to hide. It counts CALLS, so a window that had to
		// be re-asked shows up here twice; what the caller must read is the UNKNOWN count,
		// which is windows.
		logger.Printf("[Coverage] %d of %d resolve call(s) failed; %d window(s) left UNKNOWN", stats.Failed, stats.Batches, stats.Unknown)
	}
	if stats.Unknown > 0 {
		logger.Printf("[Coverage] %d of %d window(s) got no verdict — UNKNOWN, not absent", stats.Unknown, stats.Asked)
	}
	if len(members) == 0 {
		logger.Printf("[Coverage] %d window(s) asked, %d answered, 0 member(s) named", stats.Asked, stats.Answered)
		return stats
	}

	items := make([]slots.Item, 0, len(members))
	for _, m := range members {
		items = append(items, slots.Item{Value: m.Name, ChunkID: m.ChunkID, Quote: m.Quote})
	}
	value := slots.Items(items...)
	rendered := slots.Render(value)
	// 1.0, and the number is a statement about the EVIDENCE rather than a claim: every
	// member here was just matched to the line that names it, so the list is accountable
	// in a way a session's prose list is not (see slots.Union).
	strength := 1.0
	branch := harness.NewState([]harness.Variable{{
		ID:                slotID,
		Value:             &value,
		Candidate:         &rendered,
		CandidateStrength: &strength,
	}}, st.SlotTable.Depth+1, nil)
	if merged := MergeSlotPatch(st.SlotTable, branch); merged != nil {
		st.SlotTable = *merged
	}
	if raised := syncCountSlots(&st.SlotTable); len(raised) > 0 {
		logger.Printf("[Coverage] count slot(s) %v set to the enumerated members' size", raised)
	}
	// The answer reads the record, and the record was rendered BEFORE this write-back:
	// refresh it, or the members just proved would be invisible to the answer that must
	// list them.
	st.KB.Record = RenderSlotRecord(st.SlotTable, st.CollectedAnswer)
	if stats.Unknown > 0 {
		// An enumeration that did not finish must not read as one that did. The windows with
		// no verdict are members nobody judged, so the list above is a LOWER BOUND, and the
		// record says so: a count the corpus cannot support is worse than a count that names
		// its own gap, and only the record reaches the answer that states the number.
		st.KB.Record += fmt.Sprintf("\n- NOTE: %d of the %d enumerated passage(s) got no verdict, so the members above are a LOWER BOUND — state the count as what it is and do not present it as exact.\n",
			stats.Unknown, stats.Asked)
	}
	logger.Printf("[Coverage] resolve done: asked=%d answered=%d unknown=%d member(s)=%d written into slot %d",
		stats.Asked, stats.Answered, stats.Unknown, len(items), slotID)
	return stats
}

// coverageTargetSlot is the slot the members go into: the member slot that already holds
// the most members — the enumeration's own list slot, the one the record and the answer
// read. -1 when the table holds no members at all.
func coverageTargetSlot(table *harness.State) int {
	if table == nil {
		return -1
	}
	best, bestN := -1, -1
	for _, v := range table.State {
		if v.Typed().Kind != slots.KindItems {
			continue
		}
		if n := len(v.Typed().ItemValues()); n > bestN {
			best, bestN = v.ID, n
		}
	}
	return best
}

// coverageResolveCandidates assembles the lines the point-of-naming node judges: the
// enumeration's windows, then the names a probe already REACHED and no slot holds yet.
//
// The second source is not bookkeeping — it is the difference between "the corpus was asked
// about this direction" and "the run's own search touched this name". A reached name has the
// pool passage that carries it, so it is a member with its evidence whatever the session did
// with it afterwards, and a node that judges only its own enumeration drops exactly those
// names: the run holds the passage, names it nowhere, and the count comes out short of what
// the corpus states.
//
// Both sources are deduped before they are asked about: a name that is already SETTLED, and a
// name a window of the SAME passage already quotes, are not put in front of the model twice.
// Settled means anchored (see harness.AnchoredItems): a name a session wrote without a passage is
// a claim nobody has ruled on, so it stays a candidate — which is what gives the run a way to
// refute a claim it holds the refuting passage for.
//
// Returns the union, how many lines the reached terms contributed, and how many unanchored claims
// were handed over.
func coverageResolveCandidates(kb *harness.Kbinfos, table *harness.State, cov harness.Coverage) (harness.CoverageSet, int, int) {
	var set harness.CoverageSet
	if kb == nil {
		return set, 0, 0
	}
	set, _ = kb.CoverageSet()
	// Only anchored items are settled: a name written without a passage has not been judged, so it
	// stays a candidate here.
	known := map[string]bool{}
	for _, name := range harness.AnchoredItems(table) {
		known[strings.ToLower(strings.TrimSpace(name))] = true
	}
	added := 0
	for _, rt := range kb.ReachedTerms() {
		term := strings.TrimSpace(rt.Term)
		key := strings.ToLower(term)
		if term == "" || rt.ChunkID == "" || known[key] {
			continue
		}
		if windowQuotesTerm(set.Windows, rt.ChunkID, term) {
			continue
		}
		// The words are what makes the name answerable: a member no line can point at is a
		// member nobody can cite, so a reached term whose passage is no longer in the pool is
		// left to the record's own ledger instead of being handed over as a bare name.
		quote := ledgerQuote(kb, rt.ChunkID, term)
		if quote == "" {
			continue
		}
		set.Windows = append(set.Windows, harness.CoverageWindow{ChunkID: rt.ChunkID, Quote: quote})
		known[key] = true
		added++
	}
	// Third source: the names a session asserted with no passage. They are handed over as candidates
	// with whatever the pool can show about them, so a claim the run can refute is refuted rather
	// than counted or silently dropped.
	claimed := 0
	for _, name := range harness.UnanchoredItems(table) {
		term := strings.TrimSpace(name)
		key := strings.ToLower(term)
		if term == "" || known[key] {
			continue
		}
		windows := claimWindows(kb, term, cov, coverageClaimWindowsPerName)
		if len(windows) == 0 {
			continue
		}
		for _, w := range windows {
			if windowQuotesTerm(set.Windows, w.ChunkID, term) {
				continue
			}
			set.Windows = append(set.Windows, w)
		}
		known[key] = true
		claimed++
	}
	return set, added, claimed
}

// coverageClaimWindowsPerName bounds the passages ONE claimed name is shown with: enough to
// contain the one that states the deed (a name is usually mentioned in several places, and only
// one of them is the killing), and no more.
const coverageClaimWindowsPerName = 2

// claimWindows finds the pool passages that mention a claimed name, the ones carrying the deed's
// words or the actor first. A claim no passage mentions is returned nothing: it stays unjudged.
func claimWindows(kb *harness.Kbinfos, name string, cov harness.Coverage, max int) []harness.CoverageWindow {
	type scored struct {
		window harness.CoverageWindow
		score  int
	}
	var found []scored
	for _, c := range kb.ChunksFrom(0, kb.PoolSize()) {
		id := harness.ChunkIDOf(c)
		if id == "" {
			continue
		}
		text := harness.ChunkTextOf(c)
		if !strings.Contains(text, name) {
			continue
		}
		score := 0
		act := ""
		for _, a := range cov.Acts {
			if a != "" && strings.Contains(text, a) {
				act = a
				score++
				break
			}
		}
		for _, form := range cov.Actors() {
			if form != "" && strings.Contains(text, form) {
				score++
				break
			}
		}
		quote := ledgerQuote(kb, id, name)
		if quote == "" {
			continue
		}
		// The act word the passage carries rides the window, so the ranking that offered it is
		// visible in the log (see CoverageWindow.Act).
		found = append(found, scored{harness.CoverageWindow{ChunkID: id, Quote: quote, Act: act}, score})
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].score > found[j].score })
	out := make([]harness.CoverageWindow, 0, max)
	for _, f := range found {
		if len(out) >= max {
			break
		}
		out = append(out, f.window)
	}
	return out
}

// windowQuotesTerm reports whether a window of this passage already carries the term: that line
// names the member once, and a second line quoting the same words would ask about it twice.
func windowQuotesTerm(windows []harness.CoverageWindow, chunkID, term string) bool {
	for _, w := range windows {
		if w.ChunkID == chunkID && strings.Contains(w.Quote, term) {
			return true
		}
	}
	return false
}
