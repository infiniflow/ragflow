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

// The graph's CROSS-STAGE numbers live here: every budget and timeout that more than one
// stage reasons about — "how long may this step take" is answered in one place (nodeClock in
// every node). There is no per-shape clock and no budget extension any more: ONE clock per
// question, set by the caller, and nothing inside a run widens it. The pool has NO storage
// ceiling: see the note in runtime/kbinfos.go.
//
// What is deliberately NOT here: the numbers ONE stage spends — the fan-out shape guards
// beside fanoutLooksLikeQuery, the draft caps beside ComposeFallbackDraft, the sampling
// temperatures beside the calls they configure.
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

// openingShareS is how much of the question the OPENING may spend (planner + prefetch together).
func openingShareS(remaining float64) float64 {
	return min(OpeningMaxS, max(OpeningMinS, 0.25*remaining))
}

// finaleShareS is how much of the question the ANSWER turn keeps: the research stops early enough
// to hand it over, so a call that takes its time still returns an answer inside the question's
// clock.
func finaleShareS(remaining float64) float64 {
	return min(FinaleMaxS, max(FinaleMinS, 0.25*remaining))
}

// researchRoomS is what the research may spend right now: the clock minus the finale's share.
func researchRoomS(remaining float64) float64 {
	return remaining - finaleShareS(remaining)
}

// canOpenRound reports whether another round still fits: the round itself AND the finale it has to
// hand the question over to. This is the ONE gate the routing uses for "is there time for more
// research" — it replaced a bare MinRoundHeadroomS, which asked only whether a round could START
// and let it start without room for the answer (see routeResearch).
func canOpenRound(remaining float64) bool {
	return researchRoomS(remaining) >= MinRoundS
}

// The question's clock is DIVIDED here, once, and every phase asks for its share.
//
// The phases used to carry one cap each — planner 45, prefetch 90, round 120, finale 60 — whose
// SUM was 315s against a 180s question, and each node took `min(its cap, what is left)`. Nothing
// declared how much of the question the OPENING was allowed, so a slow prefetch could spend half
// of it with zero model calls and hand the research whatever happened to remain (measured
// 2026-09-20, FRAMES: 4 of 23 questions had an opening of 90/90/90/65s, and 5 sessions died at
// exactly 110s because the round's own ctx was the last thing in the question).
//
// So the numbers below are SHARES, not caps, and the nodes read them instead of doing their own
// arithmetic: opening (planner + prefetch together) / research (every round) / finale (the answer
// turn plus the composition that may follow it). They sum to less than the budget by construction.
const (
	TotalBudgetS = 180.0 // whole-graph wall-clock ceiling per question

	// OpeningMaxS bounds the OPENING AS A WHOLE — planner and prefetch share this deadline rather
	// than owning one budget each. Measured: the opening's median is 2-9s, and the deep-recall
	// opening (Stage 3) needs more room than that, so the ceiling is generous while the tail (90s)
	// that used to starve the research is gone.
	OpeningMaxS = 45.0
	OpeningMinS = 15.0

	// FinaleMinS is what the ANSWER turn must still have when the research stops.
	//
	// The finale's own instrumentation reads ~6s, but one model call on the provider we run has a
	// 40-60s tail, and the answer call is the one call that must never be raced by the wall (see
	// sessionClockGuardS in runtime/action_session.go): 40s is that call's floor, not its median.
	FinaleMinS = 40.0
	FinaleMaxS = 60.0

	// MinRoundS is the shortest research round worth opening. A round's job is to READ what the
	// last one pointed at and write the answer, so a slice that cannot afford that is not spent.
	MinRoundS = 40.0

	PassTimeoutS = 120.0 // slot research pass wall-clock
	// PrefetchTimeoutS is the fan-out's own ceiling. It is NOT the opening's budget any more: the
	// opening share above is, and prefetch gets what the planner left of it (see plannerNode).
	PrefetchTimeoutS = 90.0
	// FanoutTopN is the per-query result count for the programmatic fetch.
	FanoutTopN = 8
	// MaxFanouts caps planner fan-outs.
	MaxFanouts = 5
	// PoolHeadLines caps the evidence-pool summary shown to the rewriter.
	PoolHeadLines = 12

	// Research constants: how many plan clues one round may open, and the caps on a
	// fallback clue and a draft candidate.
	//
	// There is no session concurrency any more (rounds run ONE session, see
	// RunSlotResearchPass): the concurrency that remains is between the retrieval legs of a
	// single call, which is where it belongs.
	planMaxFanouts        = 3
	slotFallbackClueChars = 160
)
