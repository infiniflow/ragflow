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

// The graph's CROSS-STAGE numbers live here: every budget, timeout and pool / session cap
// that more than one stage reasons about — "how long may this step take" and "how much may
// the pool hold" are answered in one place (see the call sites: nodeClock in every node,
// SessionWallS and SetBudgetExtensionS in the research pass, MaxSnippetPool in the
// prefetch).
//
// What is deliberately NOT here: the numbers ONE stage spends — the fan-out shape guards
// beside fanoutLooksLikeQuery, the batching thresholds beside BatchFillSlots, the draft caps
// beside ComposeFallbackDraft, the sampling temperatures beside the calls they configure.
// Moving those here would put a constant a long way from the only code that can explain it,
// and a stage's tuning is exactly the thing that changes with its own measurements. The rule
// is cohesion, not one file: a number shared across stages belongs here, a number one stage
// owns belongs with that stage.
//
// What is ALSO not here: the DECISIONS those numbers feed. A node asks for a clock with
// nodeClock and gets one; whether the round continues afterwards is a routing decision, and
// those live with the routes.

// nodeClock is THE shape every node's clock takes: the node's own budget, floored at
// floorS, and capped by the room the QUESTION still has (roomS is what is left after the
// reserve the callers below it need).
//
// The FLOOR WINS over a room that is merely small: with room 2s and a floor of 10s the
// call gets 10s, because a node that cannot finish inside its floor is not worth starting
// at all. A caller that needs the room protected therefore passes a smaller floor, or a
// roomS that already has the reserve subtracted.
//
// A room already GONE (roomS <= 0) yields zero, which every caller treats as "do not
// start" rather than "start and get cancelled".
func nodeClock(budgetS, floorS, roomS float64) float64 {
	if roomS <= 0 {
		return 0
	}
	return min(budgetS, max(floorS, roomS))
}

const (
	TotalBudgetS      = 180.0 // whole-graph wall-clock ceiling per question
	MinRoundHeadroomS = 50.0  // need at least this much left to start a new round
	PassTimeoutS      = 120.0 // slot research pass wall-clock
	// SetBudgetExtensionS is added ONCE to a question's research budget when its
	// table IS an enumeration — a count/set/list slot, a NAME-carrying slot and the
	// act words (see runtime.Coverage.Ok and RunSlotResearchPass).
	//
	// The question budget is sized for one pass (TotalBudgetS 180 ⊃ PassTimeoutS
	// 120) and an enumeration needs a second one: its first pass spends the wall
	// clock on batches of names, so a member that a cut session never patched has
	// nowhere to be picked up: a spent pass leaves too little room for another round
	// (MinRoundHeadroomS), and a member that was reached but never recorded is lost with
	// it. The extension is what lets the round AFTER that one start at all.
	SetBudgetExtensionS = 120.0
	// setSessionSlackS is added to the session clock an ENUMERATION pass hands its
	// sessions, so a session's own finalize/salvage step still fits inside the
	// context the pass derived it from.
	setSessionSlackS = 20.0
	// downstreamReserveS is what the steps AFTER research need: the SCA review
	// (SCATimeoutS), the draft and the composed answer. The budget extension is only
	// bought when the caller's own context still holds it — otherwise the second
	// research round would spend the time the answer needs, turning a missing member
	// into a timed-out question, which is strictly worse.
	downstreamReserveS = 90.0
	// minBudgetExtensionS is the smallest extension worth buying: less than this and
	// the following round could not start anyway (MinRoundHeadroomS), so the budget
	// would be widened without anything being able to use it.
	minBudgetExtensionS = 40.0
	PrefetchTimeoutS    = 90.0 // programmatic fan-out fetch
	DraftTimeoutS       = 60.0 // fallback draft synthesis
	SCATimeoutS         = 60.0 // sufficient-context review call
	// SCARetryHeadroomS is the clock that must be left before a FAILED review is retried once:
	// the second attempt plus the answer that follows it. Below it the retry is skipped and the
	// unavailable review is recorded as such (see graph_sca), because a retry that eats the
	// answer's own clock trades a missing verdict for a missing answer.
	SCARetryHeadroomS = 120.0
	RewriteTimeoutS   = 45.0 // gap → query rewrite call
	// CoverageResolveTimeoutS bounds the enumeration's last node (see RunCoverageResolve): it runs
	// before the answer is composed, so it may not spend the clock the answer needs. It bounds the
	// WHOLE resolve, which is batched and parallel, rather than one call carrying every
	// window: one call that carries them all hits this clock and answers nothing at all.
	CoverageResolveTimeoutS = 30.0
	// SCAViewCap is the view the SCA is shown: large enough that the passage carrying the
	// answer is not the one that gets cut.
	SCAViewCap = 60
	// CoverageEnrollHeadroomS is the clock the resolve node needs before it may run the
	// direction's enumeration itself (see enrollEnumeration): the enumeration plus the answer
	// that still has to be composed. Below it the node judges what it already has.
	CoverageEnrollHeadroomS = 45.0
	// MaxSnippetPool is the storage ceiling of the snippet pool across ALL
	// rounds. Storage and REVIEW are decoupled: the SCA only reads a ranked
	// view, so the pool may accumulate freely while prompts stay bounded.
	MaxSnippetPool = 60
	// DrillReserve: slots kept free after the FIRST prefetch so the research
	// executor can top up evidence.
	DrillReserve = 12
	// FanoutTopN is the per-query result count for the programmatic fetch.
	FanoutTopN = 8
	// FanoutTopNRewrite is the reduced count used after a rewrite round.
	FanoutTopNRewrite = 6
	// MaxFanouts caps planner fan-outs.
	MaxFanouts = 5
	// MaxSCAGaps caps gaps handed to the rewriter.
	MaxSCAGaps = 8
	// PoolHeadLines caps the evidence-pool summary shown to the rewriter.
	PoolHeadLines = 12

	// Slot-table research constants: how many sessions run at once, how many run per
	// round, and the caps on a fallback clue and a draft candidate.
	slotSessionConcurrency = 2
	slotSessionsPerRound   = 3
	slotFallbackClueChars  = 160
	// draftCandidateChars caps a claim's draft text handed to the SCA.
	draftCandidateChars = 400
	// draftClueTailChars / draftUnresolvedClueChars are the per-clue caps in the
	// rendered draft (240 for a resolved slot's discovered-clue tail, 80 for an
	// unresolved slot's question clues).
	draftClueTailChars       = 240
	draftUnresolvedClueChars = 80
)
