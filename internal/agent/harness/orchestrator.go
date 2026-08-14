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
)

// Orchestration input/output mirrors Python orchestrator (direct.py / decompose.py).
// SearchFn performs one hybrid search for a query and returns chunks + doc aggs,
// so the orchestrator is decoupled from the concrete retrieval backend.
type SearchFn func(ctx context.Context, query, keywords string) ([]map[string]interface{}, []map[string]interface{})

// Kbinfos is the shared accumulation store (Python tools.kbinfos).
type Kbinfos struct {
	Chunks  []map[string]interface{}
	DocAggs []map[string]interface{}
	// PreSummary is the merged claim-report summary produced by AgenticResearch
	// (Python kbinfos["pre_summary"]). The final-answer call reads it when set.
	PreSummary string
}

func (k *Kbinfos) HasChunks() bool { return len(k.Chunks) > 0 }

// Merge appends the given chunks/aggs, deduplicating by chunkKey. It returns the
// GLOBAL indices (positions in k.Chunks after the merge) of the chunks this call
// contributed, so callers can store stable evidence references across multiple
// Merge calls (per-search indices would diverge from the accumulated list).
func (k *Kbinfos) Merge(chunks, aggs []map[string]interface{}) []int {
	seen := map[string]bool{}
	for _, c := range k.Chunks {
		seen[chunkKey(c)] = true
	}
	var added []int
	for _, c := range chunks {
		kk := chunkKey(c)
		if !seen[kk] {
			seen[kk] = true
			k.Chunks = append(k.Chunks, c)
		}
		// Record the global index of every contributed chunk (dedup or new),
		// so EvidenceIDs always reference the accumulated kbinfos positions.
		added = append(added, indexOfChunk(k.Chunks, kk))
	}
	dseen := map[string]bool{}
	for _, d := range k.DocAggs {
		if id, _ := d["doc_id"].(string); id != "" {
			dseen[id] = true
		}
	}
	for _, d := range aggs {
		if id, _ := d["doc_id"].(string); id != "" && !dseen[id] {
			dseen[id] = true
			k.DocAggs = append(k.DocAggs, d)
		}
	}
	return added
}

// indexOfChunk returns the global index of the chunk whose key matches kk.
func indexOfChunk(chunks []map[string]interface{}, kk string) int {
	for i, c := range chunks {
		if chunkKey(c) == kk {
			return i
		}
	}
	return -1
}

// chunkKey returns a stable dedup key for a chunk. Prefers chunk_id/id when
// present; otherwise falls back to a content-derived hash so chunks without an
// id still dedup correctly (and never collide on a shared empty/"" key).
func chunkKey(c map[string]interface{}) string {
	if id, ok := c["chunk_id"].(string); ok && id != "" {
		return "cid:" + id
	}
	if id, ok := c["id"].(string); ok && id != "" {
		return "id:" + id
	}
	content := ""
	if t, ok := c["content_with_weight"].(string); ok {
		content = t
	} else if t, ok := c["content"].(string); ok {
		content = t
	}
	if content == "" {
		// No stable identity: fall back to the doc reference so at least
		// per-document grouping is preserved (rarely reached).
		content = fmt.Sprintf("%s|%s", anyString(c["doc_id"]), anyString(c["docnm_kwd"]))
	}
	return "h:" + fnv64(content)
}

func anyString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// fnv64 is a deterministic non-crypto hash for dedup keys.
func fnv64(s string) string {
	h := uint64(14695981039346656037)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("%016x", h)
}

// OrchestratorResult mirrors the state updates returned by the Python
// orchestrator nodes.
type OrchestratorResult struct {
	Verdict       *SufficiencyVerdict
	PartialAnswer bool
	Abstain       bool
	EmptyResult   bool
	Kbinfos       *Kbinfos
	// Caveat is the decision-ladder explanation for a partial answer (e.g.
	// "evidence partially supports the answer"), surfaced in the final answer.
	Caveat string
	// ForceLLM is set by the FALLBACK_LLM verdict: even with no gathered
	// evidence, finalization must still call the direct LLM (rather than return
	// the canned "no evidence" string).
	ForceLLM bool
}

// DirectSearch is the low-mode orchestrator: one hybrid search → merge.
func DirectSearch(ctx context.Context, search SearchFn, question, keywords string, kbinfos *Kbinfos) OrchestratorResult {
	if kbinfos == nil {
		kbinfos = &Kbinfos{}
	}
	chunks, aggs := search(ctx, question, keywords)
	kbinfos.Merge(chunks, aggs)
	if !kbinfos.HasChunks() {
		return OrchestratorResult{EmptyResult: true, Kbinfos: kbinfos}
	}
	return OrchestratorResult{Kbinfos: kbinfos}
}

// DecomposeAndSearch is the medium-mode orchestrator: decompose → parallel
// search → cross-check → fusion → iterate until a verdict stops it.
func DecomposeAndSearch(ctx context.Context, search SearchFn, question, keywords string, claims []*ClaimTarget, modeLabel string, kbinfos *Kbinfos) OrchestratorResult {
	if kbinfos == nil {
		kbinfos = &Kbinfos{}
	}
	mode, _ := GetMode(modeLabel)
	if mode.Label == "" {
		mode = THINKING_MODES["medium"]
	}
	unverified := func() []*ClaimTarget {
		var out []*ClaimTarget
		for _, c := range claims {
			if !c.IsVerified {
				out = append(out, c)
			}
		}
		return out
	}

	for cycle := 0; cycle < mode.MaxOrchestratorCycles; cycle++ {
		uv := unverified()
		if len(uv) == 0 {
			break
		}
		for _, c := range uv {
			chunks, aggs := search(ctx, c.Description, keywords)
			if len(chunks) > 0 {
				c.IsVerified = true
				c.Confidence = 0.8
				// Merge returns the GLOBAL indices of this claim's chunks in the
				// accumulated kbinfos, which is what CrossCheckClaim resolves
				// against (allChunks is keyed by that global position).
				global := kbinfos.Merge(chunks, aggs)
				c.AgentResult = &AgentResult{
					ClaimID: c.ClaimID, Report: summarize(chunks), IsVerified: true, Confidence: 0.8,
					EvidenceIDs: global,
				}
			} else {
				c.AgentResult = &AgentResult{ClaimID: c.ClaimID, IsVerified: false, Confidence: 0.0}
			}
		}

		allChunks := map[int]map[string]interface{}{}
		for i, c := range kbinfos.Chunks {
			allChunks[i] = c
		}
		var agentResults []AgentResult
		var crossResults []ClaimCrossCheckResult
		for _, c := range claims {
			if c.AgentResult != nil {
				agentResults = append(agentResults, *c.AgentResult)
				crossResults = append(crossResults, CrossCheckClaim(c.AgentResult, allChunks))
			}
		}
		claimTargets := make([]ClaimTarget, 0, len(claims))
		for _, c := range claims {
			if c != nil {
				claimTargets = append(claimTargets, *c)
			}
		}
		verdict := ComputeFusionScore(agentResults, crossResults, mode, question, claimTargets, allChunks)
		action, _, caveat := RouteSufficiencyVerdict(verdict, modeLabel, cycle, mode.MaxOrchestratorCycles, nil)

		switch action {
		case "ANSWER":
			return OrchestratorResult{Verdict: &verdict, Kbinfos: kbinfos}
		case "ANSWER_PARTIAL":
			return OrchestratorResult{Verdict: &verdict, PartialAnswer: true, Caveat: caveat, Kbinfos: kbinfos}
		case "FALLBACK_LLM":
			// ultra route: no grounded answer, hand off to the direct LLM. Force
			// finalization to call the model even with empty evidence.
			return OrchestratorResult{Verdict: &verdict, PartialAnswer: true, Caveat: caveat, ForceLLM: true, Kbinfos: kbinfos}
		case "ABSTAIN":
			kbinfos.Chunks = nil
			return OrchestratorResult{Verdict: &verdict, Abstain: true, Kbinfos: kbinfos}
		case "CONTINUE":
			// fallthrough to next cycle
		}
	}
	return OrchestratorResult{Kbinfos: kbinfos}
}

func summarize(chunks []map[string]interface{}) string {
	var parts []string
	n := 3
	if len(chunks) < n {
		n = len(chunks)
	}
	for _, c := range chunks[:n] {
		text := ""
		if t, ok := c["content_with_weight"].(string); ok {
			text = t
		} else if t, ok := c["content"].(string); ok {
			text = t
		}
		if len(text) > 200 {
			text = text[:200]
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " | ")
}

func intRange(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
