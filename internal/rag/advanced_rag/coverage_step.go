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
	"log"
	"time"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/advanced_rag/slots"
)

// The enumeration's LAST node: the runtime asks the model about every window the
// enumeration found, and writes the members its verdicts name.
//
// Why this node exists at all is measured (2026-09-16, 三国/关羽): the evidence was never
// the limit — the corpus holds nineteen named kills, the pool held 244 passages, and runs
// of ONE build answered 18 / 16 / 15 / 14 / 12 members with the same corpus in hand. What
// varied was what a session WROTE: each session patched 12-14 names and the round's union
// was whichever list happened to be longest. So the write-back stops being a session's
// memory and becomes the runtime's own step — every window gets a verdict, and a window
// nobody answered is reported as UNKNOWN rather than read as "the corpus does not say it".
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
	set, ok := st.KB.CoverageSet()
	if !ok || set.Empty() {
		// No enumeration ran (no clock left, or the executor cannot search): the sessions'
		// own list is all there is, and there is nothing here to check it against.
		return stats
	}

	// Bounded like every other single-purpose call in this graph: the answer composition
	// still has to fit inside the request's clock, so this step may not spend it.
	t := nodeClock(CoverageResolveTimeoutS, 10.0, st.RemainingS()-10.0)
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
	defer cancel()

	members, stats := harness.ResolveCoverage(callCtx, deps.Model, st.Question, cov, set)
	if stats.Failed > 0 {
		// Said out loud because a failed batch is members nobody checked — the failure this
		// node exists to make visible, not to hide.
		logger.Printf("[Coverage] %d of %d resolve batch(es) failed; %d window(s) left UNKNOWN", stats.Failed, stats.Batches, stats.Unknown)
	}
	if stats.Unknown > 0 {
		logger.Printf("[Coverage] %d of %d window(s) got no verdict — UNKNOWN, not absent", stats.Unknown, stats.Asked)
	}
	if len(members) == 0 {
		logger.Printf("[Coverage] %d window(s) asked, %d answered, 0 member(s) named", stats.Asked, stats.Answered)
		return stats
	}

	items := make([]slots.Member, 0, len(members))
	for _, m := range members {
		items = append(items, slots.Member{Name: m.Name, ChunkID: m.ChunkID, Quote: m.Quote})
	}
	value := slots.Members(items...)
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
		if v.Typed().Kind != slots.KindMembers {
			continue
		}
		if n := len(v.Typed().Names()); n > bestN {
			best, bestN = v.ID, n
		}
	}
	return best
}
