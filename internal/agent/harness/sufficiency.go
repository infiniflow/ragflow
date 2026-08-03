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
	"fmt"
	"regexp"
	"strings"
)

// Sufficiency scoring (code-only, mirrors Python sufficiency.py): cross-check an
// agent result against the evidence chunks, fuse agent confidence + cross-check
// pass rate, then route to a 5-way verdict.

var reNumber = regexp.MustCompile(`\d+\.?\d*`)
var reEntities = regexp.MustCompile(`\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*\b`)

// extractNumbers returns numeric values found in text.
func extractNumbers(text string) []string {
	return reNumber.FindAllString(text, -1)
}

// extractNamedEntities returns capitalized multi-word sequences.
func extractNamedEntities(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range reEntities.FindAllString(text, -1) {
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// CrossCheckClaim performs a code-level cross-check of an agent result against
// the accumulated evidence chunks (number matching + entity presence).
func CrossCheckClaim(agent *AgentResult, allChunks map[int]map[string]interface{}) ClaimCrossCheckResult {
	if agent == nil {
		return ClaimCrossCheckResult{ClaimID: "", CrossCheckPassed: false, Mismatches: []string{"nil agent result"}}
	}
	if !agent.IsVerified {
		return ClaimCrossCheckResult{ClaimID: agent.ClaimID, CrossCheckPassed: false, Mismatches: []string{"agent self-reported as unverified"}}
	}
	numbers := extractNumbers(agent.Report)
	entities := extractNamedEntities(agent.Report)

	var matches, mismatches []string
	for _, eid := range agent.EvidenceIDs {
		chunk, ok := allChunks[eid]
		if !ok {
			mismatches = append(mismatches, fmt.Sprintf("evidence_id=%d: chunk not found", eid))
			continue
		}
		text := ""
		if c, ok := chunk["content_with_weight"].(string); ok {
			text = strings.ToLower(c)
		} else if c, ok := chunk["content"].(string); ok {
			text = strings.ToLower(c)
		}
		for _, num := range numbers {
			if strings.Contains(text, num) {
				matches = append(matches, fmt.Sprintf("number %s found in chunk %d", num, eid))
			} else {
				mismatches = append(mismatches, fmt.Sprintf("number %s not found in chunk %d", num, eid))
			}
		}
		for _, ent := range entities {
			if strings.Contains(text, strings.ToLower(ent)) {
				matches = append(matches, fmt.Sprintf("entity '%s' found in chunk %d", ent, eid))
			} else {
				mismatches = append(mismatches, fmt.Sprintf("entity '%s' not found in chunk %d", ent, eid))
			}
		}
	}
	// HasEvidence is true when at least one evidence id resolved to a chunk with
	// content. A claim with no resolvable evidence is never considered verified.
	hasEvidence := len(matches)+len(mismatches) > 0
	total := len(matches) + len(mismatches)
	crossScore := 0.0
	if total > 0 {
		crossScore = float64(len(matches)) / float64(total)
	}
	// Entity presence now contributes to matches too, so it can raise the score;
	// use a float comparison to avoid integer-division truncation bias.
	crossPassed := hasEvidence && float64(len(mismatches)) < float64(len(matches))/2.0
	return ClaimCrossCheckResult{
		ClaimID: agent.ClaimID, CrossCheckPassed: crossPassed, CrossCheckScore: crossScore,
		EvidenceMatches: matches, Mismatches: mismatches, HasEvidence: hasEvidence,
	}
}

// ComputeFusionScore fuses agent confidence + cross-check pass rate into a
// SufficiencyVerdict for the given mode.
func ComputeFusionScore(agentResults []AgentResult, crossResults []ClaimCrossCheckResult, mode ExecutionStrategy) SufficiencyVerdict {
	verified := 0
	for _, r := range agentResults {
		if r.IsVerified {
			verified++
		}
	}
	agentScore := 0.0
	if len(agentResults) > 0 {
		agentScore = float64(verified) / float64(len(agentResults))
	}
	passed := 0
	for _, r := range crossResults {
		if r.CrossCheckPassed {
			passed++
		}
	}
	crossScore := 0.0
	if len(crossResults) > 0 {
		crossScore = float64(passed) / float64(len(crossResults))
	}

	fusionScore := agentScore
	if crossScore > fusionScore {
		fusionScore = crossScore // low/medium default: max
	}
	switch mode.Label {
	case "ultra":
		fusionScore = min(agentScore, crossScore)
	case "high":
		fusionScore = (agentScore + crossScore) / 2
	}

	hasConflicts := false
	for _, r := range crossResults {
		if len(r.Mismatches) > 0 {
			hasConflicts = true
			break
		}
	}

	// Empty-evidence guard: if no claim examined any evidence chunk, the answer
	// cannot be grounded at all — this is UNANSWERABLE, not merely incomplete.
	anyEvidence := false
	for _, r := range crossResults {
		if r.HasEvidence {
			anyEvidence = true
			break
		}
	}

	status := "INSUFFICIENT"
	switch {
	case !anyEvidence:
		status = "UNANSWERABLE"
	case hasConflicts && fusionScore < mode.PartialThreshold:
		status = "CONFLICTING"
	case fusionScore >= mode.SufficiencyThreshold:
		status = "SUFFICIENT"
	case fusionScore >= mode.PartialThreshold:
		status = "USEFUL_BUT_INCOMPLETE"
	case func() bool {
		for _, r := range crossResults {
			if r.CrossCheckPassed {
				return true
			}
		}
		return false
	}():
		status = "INSUFFICIENT"
	default:
		status = "UNANSWERABLE"
	}

	var missing []string
	for _, r := range crossResults {
		if !r.CrossCheckPassed || !r.HasEvidence {
			missing = append(missing, r.ClaimID)
		}
	}

	assessments := make([]map[string]interface{}, 0, len(crossResults))
	for _, r := range crossResults {
		assessments = append(assessments, map[string]interface{}{
			"claim_id": r.ClaimID, "is_verified": r.CrossCheckPassed && r.HasEvidence, "score": r.CrossCheckScore,
			"mismatches": r.Mismatches, "has_evidence": r.HasEvidence,
		})
	}

	return SufficiencyVerdict{
		Status: status, Score: fusionScore, AgentScore: agentScore, CrossScore: crossScore,
		ClaimAssessments: assessments, HasConflicts: hasConflicts, MissingClaims: missing,
		Feedback: buildFeedback(missing, crossResults), OverallReason: fmt.Sprintf("%s score=%.2f missing=%v", status, fusionScore, missing),
	}
}

func buildFeedback(missing []string, results []ClaimCrossCheckResult) string {
	if len(missing) == 0 {
		return "all claims verified"
	}
	var hints []string
	for _, r := range results {
		if !r.CrossCheckPassed {
			hints = append(hints, fmt.Sprintf("claim %s: %d mismatch(es)", r.ClaimID, len(r.Mismatches)))
		}
	}
	return "missing: " + strings.Join(hints, "; ")
}

// RouteSufficiencyVerdict returns (action, shouldContinue) from the verdict.
func RouteSufficiencyVerdict(v SufficiencyVerdict, modeLabel string, cycle, maxCycles int) (string, bool) {
	mode, _ := GetMode(modeLabel)
	if mode.Label == "" {
		mode = THINKING_MODES["medium"]
	}
	switch v.Status {
	case "SUFFICIENT":
		return "ANSWER", false
	case "USEFUL_BUT_INCOMPLETE":
		if mode.RequiresSelectiveGen {
			return "ANSWER_PARTIAL", false
		}
		return "CONTINUE", false
	case "INSUFFICIENT":
		if cycle >= int(float64(maxCycles)*0.8) {
			return "ANSWER_PARTIAL", false
		}
		return "CONTINUE", true
	case "CONFLICTING":
		if mode.AllowsReplan && cycle < int(float64(maxCycles)*0.5) {
			return "REPLAN", true
		}
		return "ANSWER_PARTIAL", false
	case "UNANSWERABLE":
		if mode.FallbackToDirectLLM {
			return "FALLBACK_LLM", false
		}
		return "ABSTAIN", false
	default:
		return "CONTINUE", true
	}
}
