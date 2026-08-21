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

package harness

import (
	"sort"
	"strings"
)

// Action constants (shared with orchestrators), mirroring Python
// sufficiency_ladder.py. The ladder decides *presentation*: full answer vs.
// caveated answer vs. re-investigate vs. unanswerable.
const (
	ActionAnswer           = "ANSWER"
	ActionAnswerWithCaveat = "ANSWER_WITH_CAVEAT"
	ActionGap              = "GAP"
	ActionReconcile        = "RECONCILE"
	ActionUnanswerable     = "UNANSWERABLE"
)

// LadderInput is the decision-ladder input set, mirroring the keyword arguments
// of Python sufficiency_ladder(). AutoSufficient/AutoConfidence are the LLM
// AutoRater's judgment; Missing/Contradictions are its concrete gaps; and
// AgentConfidence is the aggregated agent self-confidence (risk gate).
type LadderInput struct {
	AutoSufficient  bool
	AutoConfidence  float64
	Missing         []string
	Contradictions  []string
	AgentConfidence float64
	CHigh           float64
	CLow            float64
	LLMFloor        float64
	AllowsReconcile bool
	Cycle           int
	MaxCycles       int
	HardViolations  map[string][]string // claimID -> gaps (or empty slices as markers)
}

// LadderOutput is the decision-ladder result.
type LadderOutput struct {
	Action         string
	ShouldContinue bool
	Caveat         string
	Missing        []string
}

// AggregateAgentConfidence mirrors Python aggregate_agent_confidence: mean
// self-confidence over the trusted (self-verified, non-violating) claims.
// HardViolations keys are claim IDs that must be excluded regardless of their
// self-reported verification.
func AggregateAgentConfidence(results []AgentResult, hardViolations map[string][]string) float64 {
	violated := map[string]bool{}
	for id := range hardViolations {
		violated[id] = true
	}
	total := 0.0
	count := 0
	for _, r := range results {
		if !r.IsVerified || violated[r.ClaimID] {
			continue
		}
		total += r.Confidence
		count++
	}
	if count == 0 {
		return 0.0
	}
	return total / float64(count)
}

// SufficiencyLadder evaluates the monotonic decision ladder and returns the
// action. It is a pure function (no LLM dependency), mirroring Python
// sufficiency_ladder() decision-by-decision.
func SufficiencyLadder(in LadderInput) LadderOutput {
	violations := map[string][]string{}
	for id, gaps := range in.HardViolations {
		violations[id] = gaps
	}

	// 1. Hard veto floor: code-proven evidence gap beats the LLM's "good enough".
	if len(violations) > 0 {
		ids := make([]string, 0, len(violations))
		for id := range violations {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		first := ids
		if len(first) > 6 {
			first = first[:6]
		}
		return LadderOutput{
			Action:         ActionAnswerWithCaveat,
			ShouldContinue: false,
			Caveat:         "hard evidence gap in claim(s): " + strings.Join(first, ", "),
			Missing:        in.Missing,
		}
	}

	// 2. AutoRater says insufficient.
	if !in.AutoSufficient {
		if len(in.Missing) > 0 {
			return LadderOutput{Action: ActionGap, ShouldContinue: true, Missing: in.Missing}
		}
		return LadderOutput{Action: ActionUnanswerable, ShouldContinue: false, Missing: in.Missing}
	}

	// 3. AutoRater is not confident in its own sufficiency call.
	if in.AutoConfidence < in.LLMFloor {
		if in.AllowsReconcile && in.Cycle < in.MaxCycles-1 {
			return LadderOutput{
				Action:         ActionReconcile,
				ShouldContinue: true,
				Caveat:         "AutoRater itself is unsure; re-investigating",
			}
		}
		return LadderOutput{
			Action:         ActionAnswerWithCaveat,
			ShouldContinue: false,
			Caveat:         "AutoRater sufficiency judgment is low-confidence",
		}
	}

	// 4. Sufficient + confident: agent confidence sets the presentation.
	caveat := ""
	shouldContinue := false
	action := ActionAnswer
	switch {
	case in.AgentConfidence >= in.CHigh:
		action = ActionAnswer
	case in.AgentConfidence >= in.CLow:
		action = ActionAnswerWithCaveat
		caveat = "evidence partially supports the answer"
	case in.AllowsReconcile && in.Cycle < in.MaxCycles-1:
		action = ActionReconcile
		shouldContinue = true
	default:
		action = ActionAnswerWithCaveat
		caveat = "evidence partially supports the answer"
	}

	if len(in.Contradictions) > 0 {
		// Never silently pick one side of a contradiction; surface it.
		caveat = "evidence contains conflicting figures"
		action = ActionAnswerWithCaveat
		shouldContinue = false
	}

	return LadderOutput{Action: action, ShouldContinue: shouldContinue, Caveat: caveat, Missing: in.Missing}
}
