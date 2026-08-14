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
	"sort"
	"strconv"
	"strings"
)

// Sufficiency scoring (code-only, mirrors Python sufficiency.py): cross-check an
// agent result against the evidence chunks, fuse agent confidence + cross-check
// pass rate, then route to a 5-way verdict.
//
// Cross-check alignment (Sufficient Context redesign, mirrors sufficiency.py):
//   - number extraction + noise filtering (drop 0<n<1, dedup)
//   - union evidence matching (a fact verified by ONE chunk is verified), not
//     the old per-chunk loop
//   - bounded word/phrase matching (Ann must not match Annual)
//   - numeric multi-source conflict detection (close-but-different figures)
//   - cross score = matches/total, pass at >= 0.5
//
// NER is intentionally NOT implemented in Go (no maintained native spaCy-level
// NER library; see plan §cross-check). Entity extraction is delegated to the
// LLM grounded review (grounded_llm.go). TODO(candle): adopt
// github.com/huggingface/candle via Rust↔Go binding + model conversion to
// restore spaCy-parity multilingual NER (en/zh/de/fr/es/pt/ja).

var reNumber = regexp.MustCompile(`\d+\.?\d*`)

// reCJKRun matches a run of CJK characters (RE2 requires \x{...} escapes; the
// Python-style \u escapes would panic at compile time).
var reCJKRun = regexp.MustCompile(`[\x{4e00}-\x{9fff}]+`)

// extractNumbers returns numeric values found in text (mirrors Python
// extract_numbers).
func extractNumbers(text string) []float64 {
	var out []float64
	for _, m := range reNumber.FindAllString(text, -1) {
		if f, err := strconv.ParseFloat(m, 64); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// filterRelevantNumbers drops numbers with no factual-claim signal (mirrors
// Python _filter_relevant_numbers): values in (0,1) are ratios/confidences, and
// duplicates are checked once.
func filterRelevantNumbers(numbers []float64) []float64 {
	var kept []float64
	for _, n := range numbers {
		if n > 0 && n < 1 {
			continue
		}
		dup := false
		for _, k := range kept {
			if k == n {
				dup = true
				break
			}
		}
		if !dup {
			kept = append(kept, n)
		}
	}
	return kept
}

// isCJK reports whether text contains Chinese/Japanese characters (mirrors
// Python _is_cjk).
func isCJK(text string) bool {
	for _, r := range text {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

// extractNamedEntities is a degraded stub: CJK substrings are still extracted
// (they need no word boundaries), but Latin-script NER is delegated to the LLM
// grounded review. See the TODO(candle) note in the package comment.
func extractNamedEntities(text string) []string {
	if text == "" {
		return nil
	}
	// CJK entities: every Han run is a candidate; the LLM review disambiguates.
	// This keeps number-adjacent Chinese evidence verifiable without spaCy.
	if isCJK(text) {
		return extractCJKSubstrings(text)
	}
	return nil
}

// extractCJKSubstrings returns runs of CJK characters as entity candidates.
// This is intentionally coarse (no NER): it only preserves the substring-match
// path for CJK evidence that the union matcher below can still use.
func extractCJKSubstrings(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range reCJKRun.FindAllString(text, -1) {
		// Skip single-char connective tissue noise; keep meaningful runs.
		if len([]rune(m)) < 2 || seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}

// boundedPhraseMatch mirrors Python's bounded word/phrase match
// `(?<![\w])needle(?![\w])`: the needle must not be adjacent to a word char on
// either side, so "Ann" does not match "Annual".
func boundedPhraseMatch(text, needle string) bool {
	text = strings.ToLower(text)
	needle = strings.ToLower(needle)
	start := 0
	for {
		idx := strings.Index(text[start:], needle)
		if idx < 0 {
			return false
		}
		idx += start
		before := idx > 0 && isWordChar(rune(text[idx-1]))
		after := idx+len(needle) < len(text) && isWordChar(rune(text[idx+len(needle)]))
		if !before && !after {
			return true
		}
		start = idx + 1
	}
}

// isWordChar mirrors Python \w (word chars are also bounded by the match).
func isWordChar(r rune) bool {
	if r >= '0' && r <= '9' {
		return true
	}
	if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
		return true
	}
	return r == '_'
}

// detectNumericConflict mirrors Python _detect_numeric_conflict: flag pairs of
// disclosed figures that are close-but-not-equal (ratio in (1, 1.3]) — the
// signature of a multi-source口径 conflict.
func detectNumericConflict(disclosed []string) []string {
	type fig struct {
		val  float64
		text string
	}
	reLead := regexp.MustCompile(`([\d][\d,]*(?:\.\d+)?)`)
	var figures []fig
	for _, entry := range disclosed {
		m := reLead.FindStringSubmatch(entry)
		if m == nil {
			continue
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		if err != nil {
			continue
		}
		text := entry
		if rs := []rune(text); len(rs) > 80 {
			text = string(rs[:80]) // Python entry[:80] is a character truncation
		}
		figures = append(figures, fig{val: v, text: text})
	}
	var conflicts []string
	for i := 0; i < len(figures); i++ {
		for j := i + 1; j < len(figures); j++ {
			a, b := figures[i].val, figures[j].val
			if a <= 0 || b <= 0 {
				continue
			}
			hi, lo := a, b
			if lo > hi {
				hi, lo = b, a
			}
			ratio := hi / lo
			if ratio > 1 && ratio <= 1.3 {
				conflicts = append(conflicts, fmt.Sprintf("%s vs %s", figures[i].text, figures[j].text))
			}
		}
	}
	return conflicts
}

// CrossCheckClaim performs a code-level cross-check of an agent result against
// the accumulated evidence chunks (number matching + CJK entity presence).
func CrossCheckClaim(agent *AgentResult, allChunks map[int]map[string]interface{}) ClaimCrossCheckResult {
	if agent == nil {
		return ClaimCrossCheckResult{ClaimID: "", CrossCheckPassed: false, Mismatches: []string{"nil agent result"}}
	}
	if !agent.IsVerified {
		return ClaimCrossCheckResult{ClaimID: agent.ClaimID, CrossCheckPassed: false, Mismatches: []string{"agent self-reported as unverified"}}
	}
	rawNumbers := extractNumbers(agent.Report)
	numbers := filterRelevantNumbers(rawNumbers)
	entities := extractNamedEntities(agent.Report)

	// Gather evidence chunk texts (union) — a fact supported by ONE chunk is
	// verified; the old per-chunk loop demanded every chunk confirm every fact.
	var chunkTexts []string
	for _, eid := range agent.EvidenceIDs {
		chunk, ok := allChunks[eid]
		if !ok {
			continue
		}
		text := ""
		if c, ok := chunk["content_with_weight"].(string); ok {
			text = strings.ToLower(c)
		} else if c, ok := chunk["content"].(string); ok {
			text = strings.ToLower(c)
		}
		chunkTexts = append(chunkTexts, text)
	}

	// Numeric multi-source conflict detection: several close-but-different
	// disclosed figures cap the claim below the pass floor.
	if disclosed := agent.Numbers; len(disclosed) > 0 {
		if conflict := detectNumericConflict(disclosed); len(conflict) > 0 {
			var m []string
			for _, c := range conflict {
				m = append(m, "numeric source conflict: "+c)
			}
			return ClaimCrossCheckResult{ClaimID: agent.ClaimID, CrossCheckPassed: false, CrossCheckScore: 0.0, Mismatches: m, HasEvidence: len(chunkTexts) > 0}
		}
	}

	anywhere := func(needle string) bool {
		for _, t := range chunkTexts {
			if strings.Contains(t, needle) {
				return true
			}
		}
		return false
	}

	var matches, mismatches []string
	for _, num := range numbers {
		// Numbers extracted as floats ("1976" → 1976.0) while chunk text spells
		// "1976" — match both raw and integral forms, bounded.
		var forms []string
		if num == float64(int64(num)) {
			forms = []string{strconv.FormatFloat(num, 'f', -1, 64), strconv.FormatInt(int64(num), 10)}
		} else {
			forms = []string{strconv.FormatFloat(num, 'f', -1, 64)}
		}
		found := false
		for _, f := range forms {
			if anyBounded(chunkTexts, f) {
				found = true
				break
			}
		}
		if found {
			matches = append(matches, fmt.Sprintf("number %s found in evidence", strconv.FormatFloat(num, 'f', -1, 64)))
		} else {
			mismatches = append(mismatches, fmt.Sprintf("number %s not found in any evidence chunk", strconv.FormatFloat(num, 'f', -1, 64)))
		}
	}

	for _, ent := range entities {
		found := false
		if isCJK(ent) {
			found = anywhere(strings.ToLower(ent))
		} else {
			found = anyBounded(chunkTexts, strings.ToLower(ent))
		}
		if found {
			matches = append(matches, fmt.Sprintf("entity '%s' found in evidence", ent))
		} else {
			mismatches = append(mismatches, fmt.Sprintf("entity '%s' not found in any evidence chunk", ent))
		}
	}

	total := len(matches) + len(mismatches)
	hasEvidence := len(chunkTexts) > 0

	if total == 0 {
		// No evidence examined → fail; ids but nothing extractable → neutral 0.5.
		if len(agent.EvidenceIDs) == 0 {
			return ClaimCrossCheckResult{ClaimID: agent.ClaimID, CrossCheckPassed: false, CrossCheckScore: 0.0, Mismatches: []string{"no evidence"}, HasEvidence: false}
		}
		return ClaimCrossCheckResult{ClaimID: agent.ClaimID, CrossCheckPassed: false, CrossCheckScore: 0.5, Mismatches: []string{"nothing extractable to cross-check"}, HasEvidence: hasEvidence}
	}

	crossScore := float64(len(matches)) / float64(total)
	crossPassed := crossScore >= 0.5
	return ClaimCrossCheckResult{
		ClaimID: agent.ClaimID, CrossCheckPassed: crossPassed, CrossCheckScore: crossScore,
		EvidenceMatches: matches, Mismatches: mismatches, HasEvidence: hasEvidence,
	}
}

// anyBounded reports whether needle appears in any chunk text with word
// boundaries (mirrors Python `(?<![\w])needle(?![\w])`).
func anyBounded(chunkTexts []string, needle string) bool {
	for _, t := range chunkTexts {
		if boundedPhraseMatch(t, needle) {
			return true
		}
	}
	return false
}

// sortedKeys returns sorted map keys (stable ordering for hard_violations).
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ComputeFusionScore extracts sufficiency *signals* (no longer a weighted
// fusion), mirroring Python compute_fusion_score. The LLM AutoRater is the
// primary judge; this function only produces the code-level inputs the decision
// ladder consumes:
//   - hard_violations: claims with a proven evidence gap (weak cross-check OR a
//     missing required entity) that veto "good enough";
//   - agent_confidence: mean self-confidence over the trusted subset;
//   - has_conflicts / missing_claims: surfaced for the ladder / caveat.
//
// question / claims / allChunks drive the required-entity AND-semantics veto
// (NER-degraded: see extractNamedEntities TODO). When omitted, only the
// report-based cross-check signals are produced.
func ComputeFusionScore(agentResults []AgentResult, crossResults []ClaimCrossCheckResult, mode ExecutionStrategy, question string, claims []ClaimTarget, allChunks map[int]map[string]interface{}) SufficiencyVerdict {
	// ── Required-entity gaps (Sufficient Context paper, AND semantics) ──
	// NER-degraded: with spaCy NER absent in Go, entity extraction yields only
	// CJK substrings, so this veto is partial for Latin-script questions. The
	// LLM grounded review (grounded_llm.go) covers the rest. TODO(candle).
	requiredGaps := map[string][]string{}
	if question != "" || len(claims) > 0 {
		requiredGaps = requiredEntityGaps(question, claims, allChunks)
	}
	gappedIDs := map[string]bool{}
	for cid := range requiredGaps {
		gappedIDs[cid] = true
	}

	// Signal A: agent self-assessed confidence (continuous). Only self-verified,
	// non-gapped claims count; a gapped claim's confidence is zeroed so it cannot
	// mask an evidence gap.
	var verified []AgentResult
	for _, r := range agentResults {
		if r.IsVerified && !gappedIDs[r.ClaimID] {
			verified = append(verified, r)
		}
	}
	agentScore := 0.0
	if len(verified) > 0 {
		total := 0.0
		for _, r := range verified {
			total += r.Confidence
		}
		agentScore = total / float64(len(verified))
	}

	// Signal B: cross-check score, excluding unrelated/ungrounded noise claims
	// (cross<0.2 AND agent self-unverified) so an invented claim doesn't punish
	// an otherwise sufficient verdict.
	noiseThreshold := 0.2
	agentVerified := map[string]bool{}
	for _, r := range agentResults {
		agentVerified[r.ClaimID] = r.IsVerified
	}
	var noiseIDs []string
	var kept []ClaimCrossCheckResult
	for _, r := range crossResults {
		if r.CrossCheckScore < noiseThreshold && !r.CrossCheckPassed && !agentVerified[r.ClaimID] {
			noiseIDs = append(noiseIDs, r.ClaimID)
			continue
		}
		kept = append(kept, r)
	}
	if len(noiseIDs) > 0 && len(kept) > 0 {
		crossResults = kept
	}

	// ── Hard-veto floor ──
	minCrossFloor := 0.5
	selfVerifiedIDs := map[string]bool{}
	for _, r := range agentResults {
		if r.IsVerified {
			selfVerifiedIDs[r.ClaimID] = true
		}
	}
	weakSet := map[string]bool{}
	for _, r := range crossResults {
		if selfVerifiedIDs[r.ClaimID] && r.CrossCheckScore < minCrossFloor {
			weakSet[r.ClaimID] = true
		}
	}
	for cid := range gappedIDs {
		weakSet[cid] = true
	}
	weak := sortedKeys(weakSet)

	// Conflict detection based on the kept (non-noisy) claims.
	hasConflicts := false
	for _, r := range crossResults {
		if len(r.Mismatches) > 0 {
			hasConflicts = true
			break
		}
	}

	// Cross-check status (code-only preliminary view, no AutoRater).
	anyPassed := false
	for _, r := range crossResults {
		if r.CrossCheckPassed {
			anyPassed = true
			break
		}
	}
	status := "INSUFFICIENT"
	switch {
	case !anyPassed:
		status = "UNANSWERABLE"
	case hasConflicts:
		status = "CONFLICTING"
	case len(weak) > 0:
		status = "INSUFFICIENT"
	case agentScore >= mode.SufficiencyThreshold:
		status = "SUFFICIENT"
	default:
		status = "USEFUL_BUT_INCOMPLETE"
	}

	var missing []string
	for _, r := range crossResults {
		if !r.CrossCheckPassed {
			missing = append(missing, r.ClaimID)
		}
	}
	missing = append(missing, noiseIDs...)
	for _, c := range weak {
		if !containsStr(missing, c) {
			missing = append(missing, c)
		}
	}

	assessments := make([]map[string]interface{}, 0, len(crossResults))
	for _, r := range crossResults {
		assessments = append(assessments, map[string]interface{}{
			"claim_id": r.ClaimID, "is_verified": r.CrossCheckPassed, "score": r.CrossCheckScore,
			"mismatches": r.Mismatches, "has_evidence": r.HasEvidence,
		})
	}

	return SufficiencyVerdict{
		Status: status, Score: agentScore, AgentScore: agentScore, CrossScore: 0,
		ClaimAssessments: assessments, HasConflicts: hasConflicts, MissingClaims: missing,
		Feedback: buildFeedback(missing, crossResults), OverallReason: fmt.Sprintf("%s agent_conf=%.2f hard_veto=%d", status, agentScore, len(weak)),
		HardViolations: weak, AgentConfidence: agentScore,
	}
}

// requiredEntityGaps mirrors Python required_entity_gaps. NER-degraded: entity
// extraction yields only CJK substrings (see extractNamedEntities TODO), so the
// AND-semantics veto is partial for Latin-script questions.
func requiredEntityGaps(question string, claims []ClaimTarget, allChunks map[int]map[string]interface{}) map[string][]string {
	var chunkTexts []string
	for _, chunk := range allChunks {
		text := ""
		if c, ok := chunk["content_with_weight"].(string); ok {
			text = c
		} else if c, ok := chunk["content"].(string); ok {
			text = c
		}
		if text != "" {
			chunkTexts = append(chunkTexts, strings.ToLower(text))
		}
	}
	qEntities := extractNamedEntities(question)
	gaps := map[string][]string{}
	for _, claim := range claims {
		if claim.ClaimID == "" || claim.Description == "" {
			continue
		}
		descEntities := extractNamedEntities(claim.Description)
		descLower := map[string]bool{}
		for _, e := range descEntities {
			descLower[strings.ToLower(e)] = true
		}
		var required []string
		required = append(required, descEntities...)
		for _, e := range qEntities {
			if descLower[strings.ToLower(e)] {
				required = append(required, e)
			}
		}
		var missing []string
		for _, e := range required {
			if !entityPresent(e, chunkTexts) {
				missing = append(missing, e)
			}
		}
		if len(missing) > 0 {
			gaps[claim.ClaimID] = missing
		}
	}
	return gaps
}

// entityPresent mirrors Python _entity_present: CJK → substring, else bounded.
func entityPresent(ent string, chunkTexts []string) bool {
	if isCJK(ent) {
		lower := strings.ToLower(ent)
		for _, t := range chunkTexts {
			if strings.Contains(t, lower) {
				return true
			}
		}
		return false
	}
	return anyBounded(chunkTexts, strings.ToLower(ent))
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
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

// AutoRating is the LLM Sufficient Context AutoRater output (mirrors the dict
// returned by Python llm_sufficiency_boost).
type AutoRating struct {
	IsSufficient   bool
	Confidence     float64
	Missing        []string
	Contradictions []string
	Followups      []string
}

// RouteSufficiencyVerdict returns (action, shouldContinue, caveat) from the
// verdict via the decision ladder + orchestrator action mapping. It mirrors
// Python route_sufficiency_verdict (sufficiency.py:791), which is the
// orchestrator-facing wrapper that calls sufficiency_ladder then maps the
// ladder action onto orchestrator actions.
//
// The AutoRater (auto) is the primary judge; when nil, the verdict's code-level
// status is used as a fallback so the loop still terminates (medium mode, or a
// missing LLM judge).
func RouteSufficiencyVerdict(v SufficiencyVerdict, modeLabel string, cycle, maxCycles int, auto *AutoRating) (string, bool, string) {
	mode, _ := GetMode(modeLabel)
	if mode.Label == "" {
		mode = THINKING_MODES["medium"]
	}

	// AutoRater signals, with sane defaults when it was not invoked.
	autoSufficient := v.Status == "SUFFICIENT"
	autoConfidence := 1.0
	missing := v.MissingClaims
	contradictions := []string{}
	if auto != nil {
		autoSufficient = auto.IsSufficient
		autoConfidence = auto.Confidence
		missing = auto.Missing
		contradictions = auto.Contradictions
	} else if v.HasConflicts {
		contradictions = []string{v.Feedback}
	}

	hardViolations := map[string][]string{}
	for _, id := range v.HardViolations {
		hardViolations[id] = []string{}
	}

	out := SufficiencyLadder(LadderInput{
		AutoSufficient:  autoSufficient,
		AutoConfidence:  autoConfidence,
		Missing:         missing,
		Contradictions:  contradictions,
		AgentConfidence: v.AgentConfidence,
		CHigh:           mode.CHigh,
		CLow:            mode.CLow,
		LLMFloor:        mode.LLMFloor,
		AllowsReconcile: mode.AllowsReconcile,
		Cycle:           cycle,
		MaxCycles:       maxCycles,
		HardViolations:  hardViolations,
	})

	// Map ladder action onto orchestrator actions (mirrors sufficiency.py:842-858).
	switch out.Action {
	case ActionAnswerWithCaveat:
		return "ANSWER_PARTIAL", false, out.Caveat
	case ActionReconcile:
		// medium has no reconcile loop → degrade to CONTINUE (keep searching).
		return "CONTINUE", true, out.Caveat
	case ActionUnanswerable:
		if mode.FallbackToDirectLLM {
			return "FALLBACK_LLM", false, out.Caveat
		}
		return "ABSTAIN", false, out.Caveat
	case ActionGap:
		return "CONTINUE", true, out.Caveat
	default: // ActionAnswer
		return "ANSWER", out.ShouldContinue, out.Caveat
	}
}
