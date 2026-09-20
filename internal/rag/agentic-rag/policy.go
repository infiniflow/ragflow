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
	// The one-shot budget extension (SetBudgetExtensionS / setSessionSlackS /
	// downstreamReserveS / minBudgetExtensionS) used to live here: a question whose table
	// declared a set was handed ONE extra slice of the research clock because an enumeration
	// needs two passes and the members a cut first pass never patched had nowhere else to be
	// picked up. It is gone. The extension did not add time to the question, it MOVED time
	// into one shape's second pass — out of whatever the question actually was — and that is
	// exactly the trade this design removes: one clock, set by the caller.
	PrefetchTimeoutS = 90.0 // programmatic fan-out fetch
	DraftTimeoutS    = 60.0 // fallback draft synthesis
	SCATimeoutS      = 60.0 // sufficient-context review call
	// SCARetryHeadroomS is the clock that must be left before a FAILED review is retried once:
	// the second attempt plus the answer that follows it. Below it the retry is skipped and the
	// unavailable review is recorded as such (see graph_sca), because a retry that eats the
	// answer's own clock trades a missing verdict for a missing answer.
	SCARetryHeadroomS = 120.0
	RewriteTimeoutS   = 45.0 // gap → query rewrite call
	// SCAViewCap is the view the SCA is shown: large enough that the passage carrying the
	// answer is not the one that gets cut.
	SCAViewCap = 60
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

	// Research constants: how many plan clues one round may open, and the caps on a
	// fallback clue and a draft candidate.
	//
	// There is no session concurrency any more (rounds run ONE session, see
	// RunSlotResearchPass): the concurrency that remains is between the retrieval legs of a
	// single call, which is where it belongs.
	planMaxFanouts        = 3
	slotFallbackClueChars = 160
	// draftCandidateChars caps a claim's draft text handed to the SCA.
	draftCandidateChars = 400
	// draftClueTailChars / draftUnresolvedClueChars are the per-clue caps in the
	// rendered draft (240 for a resolved slot's discovered-clue tail, 80 for an
	// unresolved slot's question clues).
	draftClueTailChars       = 240
	draftUnresolvedClueChars = 80
)
