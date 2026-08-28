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
	"context"
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// AgenticResearch is the high/ultra two-level loop (mirrors Python
// orchestrator/agentic.py agentic_research): the orchestrator assigns claims,
// the research agent researches each in parallel batches, and the decision
// ladder decides sufficiency each round.
//
// It drives the SAME pipeline (shared Kbinfos, compilation gating, doc routing)
// as the linear Run flow, so high/ultra is a strict superset of medium, not a
// parallel implementation.
func AgenticResearch(ctx context.Context, db *gorm.DB, pipeline *Pipeline, question string, claims []*ClaimTarget, mode ExecutionStrategy) OrchestratorResult {
	kbinfos := pipeline.kbinfos

	// Stagnation guard: stop early when the score stops improving.
	const stagnationCycles = 2
	const stagnationGain = 0.05
	prevScore := -1.0

	var pendingFollowups []string

	for cycle := 0; cycle < mode.MaxOrchestratorCycles; cycle++ {
		unverified := unverifiedClaims(claims)

		if len(unverified) > 0 {
			// Consume follow-ups ONCE per round, shared by every claim in the batch.
			followups := pendingFollowups
			pendingFollowups = nil

			// Research in batches of MaxParallelAgents.
			batchSize := mode.MaxParallelAgents
			if batchSize < 1 {
				batchSize = 1
			}
			for i := 0; i < len(unverified); i += batchSize {
				end := i + batchSize
				if end > len(unverified) {
					end = len(unverified)
				}
				batch := unverified[i:end]
				results := researchBatch(ctx, db, pipeline, batch, mode, followups)
				for j, c := range batch {
					r := results[j]
					c.IsVerified = r.IsVerified
					c.Confidence = r.Confidence
					c.AgentResult = &r

					// Ultra: dynamic claim expansion from discovered_claims.
					if mode.AllowsDynamicClaims {
						for _, dc := range r.DiscoveredClaims {
							if dc == "" || claimDescExists(claims, dc) {
								continue
							}
							claims = append(claims, &ClaimTarget{
								ClaimID:     fmt.Sprintf("c_dyn_%d", len(claims)),
								Description: dc,
							})
						}
					}
				}
			}
		}

		// ── Step A.5: note the discovered entity so graph_explore is eligible ──
		// (mirrors agentic.py ctx.note_entity(_discovered_entity(tools))). Gates
		// graph_explore via HasDiscoveredEntity on the pipeline.
		if ent := discoveredEntity(kbinfos.Chunks); ent != "" {
			pipeline.noteEntity(ent)
		}

		// ── Step B: sufficiency check ──
		allChunks := map[int]map[string]interface{}{}
		for i, c := range kbinfos.Chunks {
			allChunks[i] = c
		}
		var agentResults []AgentResult
		for _, c := range claims {
			if c.AgentResult != nil {
				agentResults = append(agentResults, *c.AgentResult)
			}
		}
		var crossResults []ClaimCrossCheckResult
		for _, r := range agentResults {
			crossResults = append(crossResults, CrossCheckClaim(&r, allChunks))
		}

		claimValues := make([]ClaimTarget, 0, len(claims))
		for _, c := range claims {
			if c != nil {
				claimValues = append(claimValues, *c)
			}
		}
		verdict := ComputeFusionScore(agentResults, crossResults, mode, question, claimValues, allChunks)

		// LLM Sufficient Context AutoRater (primary judge), invoked every round.
		var citedIDs []int
		for _, r := range agentResults {
			citedIDs = append(citedIDs, r.EvidenceIDs...)
		}
		boost := LLMSufficiencyBoost(ctx, db, question, &verdict, kbinfos, citedIDs)
		if boost != nil && len(boost.Followups) > 0 {
			pendingFollowups = boost.Followups
		}

		// LLM groundedness review (draft review): ungrounded claims merge into
		// hard_violations.
		var reports []ClaimReport
		for _, r := range agentResults {
			if r.Report != "" {
				reports = append(reports, ClaimReport{ClaimID: r.ClaimID, Report: r.Report})
			}
		}
		grounded := LLMGroundedVerify(ctx, db, question, reports, kbinfos, citedIDs)
		validIDs := map[string]bool{}
		for _, r := range agentResults {
			validIDs[r.ClaimID] = true
		}
		hvSet := map[string]bool{}
		for _, id := range verdict.HardViolations {
			hvSet[id] = true
		}
		for cid, g := range grounded {
			if validIDs[cid] && (!g.Grounded || len(g.Ungrounded) > 0) {
				hvSet[cid] = true
			}
		}
		verdict.HardViolations = sortedKeys(hvSet)

		action, shouldContinue, _ := RouteSufficiencyVerdict(verdict, mode.Label, cycle, mode.MaxOrchestratorCycles, boost)

		// Stagnation guard: override CONTINUE with a partial answer when the
		// score has not meaningfully improved.
		if shouldContinue && (verdict.Status == "INSUFFICIENT" || verdict.Status == "USEFUL_BUT_INCOMPLETE") {
			if prevScore >= 0 && cycle >= stagnationCycles && verdict.Score-prevScore < stagnationGain {
				action = "ANSWER_PARTIAL"
				shouldContinue = false
			} else {
				prevScore = verdict.Score
			}
		}

		switch action {
		case "ANSWER":
			finalizeAgentResults(kbinfos, claims)
			return OrchestratorResult{Verdict: &verdict, Kbinfos: kbinfos}
		case "ANSWER_PARTIAL":
			finalizeAgentResults(kbinfos, claims)
			return OrchestratorResult{Verdict: &verdict, PartialAnswer: true, Kbinfos: kbinfos}
		case "ABSTAIN":
			kbinfos.Chunks = nil
			return OrchestratorResult{Verdict: &verdict, Abstain: true, Kbinfos: kbinfos}
		case "FALLBACK_LLM":
			finalizeAgentResults(kbinfos, claims)
			return OrchestratorResult{Verdict: &verdict, PartialAnswer: true, ForceLLM: true, Kbinfos: kbinfos}
		}
	}

	// Max cycles reached.
	finalizeAgentResults(kbinfos, claims)
	return OrchestratorResult{Verdict: nil, PartialAnswer: true, Kbinfos: kbinfos}
}

// discoveredEntity picks a salient discovered name from the gathered evidence
// (mirrors Python _discovered_entity): prefer an explicit entity/keyword tag on a
// chunk, fall back to a source document name. Used only to gate graph_explore.
func discoveredEntity(chunks []map[string]interface{}) string {
	for _, c := range chunks {
		for _, key := range []string{"entities_kwd", "important_kwd"} {
			switch val := c[key].(type) {
			case []string:
				if len(val) > 0 {
					if first := strings.TrimSpace(val[0]); first != "" {
						return first
					}
				}
			case []interface{}:
				if len(val) > 0 {
					if s, ok := val[0].(string); ok {
						if first := strings.TrimSpace(s); first != "" {
							return first
						}
					}
				}
			case string:
				if s := strings.TrimSpace(val); s != "" {
					if fields := strings.Fields(s); len(fields) > 0 {
						return fields[0]
					}
				}
			}
		}
	}
	for _, c := range chunks {
		if name := strings.TrimSpace(chunkDoc(c)); name != "" {
			return name
		}
	}
	return ""
}

// researchBatch runs ResearchAgentLoop for each claim in parallel.
func researchBatch(ctx context.Context, db *gorm.DB, pipeline *Pipeline, batch []*ClaimTarget, mode ExecutionStrategy, followups []string) []AgentResult {
	results := make([]AgentResult, len(batch))
	var wg sync.WaitGroup
	for i, c := range batch {
		wg.Add(1)
		go func(idx int, claim *ClaimTarget) {
			defer wg.Done()
			results[idx] = ResearchAgentLoop(ctx, db, pipeline, *claim, mode, followups)
		}(i, c)
	}
	wg.Wait()
	return results
}

func unverifiedClaims(claims []*ClaimTarget) []*ClaimTarget {
	var out []*ClaimTarget
	for _, c := range claims {
		if c != nil && !c.IsVerified {
			out = append(out, c)
		}
	}
	return out
}

func claimDescExists(claims []*ClaimTarget, desc string) bool {
	for _, c := range claims {
		if c != nil && c.Description == desc {
			return true
		}
	}
	return false
}

// finalizeAgentResults mirrors Python _finalize + _merge_agent_results: build the
// pre_summary and trim evidence to only cited chunks.
func finalizeAgentResults(kbinfos *Kbinfos, claims []*ClaimTarget) {
	var combined []string
	seenEvidence := map[int]bool{}
	for _, c := range claims {
		if c == nil || c.AgentResult == nil {
			continue
		}
		if c.AgentResult.Report != "" {
			status := "❌"
			if c.IsVerified {
				status = "✅"
			}
			report := c.AgentResult.Report
			if len(report) > 500 {
				report = report[:500]
			}
			combined = append(combined, fmt.Sprintf("【%s】%s %s", c.ClaimID, status, report))
		}
		for _, eid := range c.AgentResult.EvidenceIDs {
			seenEvidence[eid] = true
		}
	}
	if len(combined) > 0 {
		kbinfos.PreSummary = strings.Join(combined, "\n\n")
	}

	// Trim evidence to cited chunks only (preserve order, never empty).
	if len(seenEvidence) > 0 && len(seenEvidence) < len(kbinfos.Chunks) {
		keep := make([]int, 0, len(seenEvidence))
		for i := range kbinfos.Chunks {
			if seenEvidence[i] {
				keep = append(keep, i)
			}
		}
		newChunks := make([]map[string]interface{}, 0, len(keep))
		for _, i := range keep {
			newChunks = append(newChunks, kbinfos.Chunks[i])
		}
		kbinfos.Chunks = newChunks
	}
}
