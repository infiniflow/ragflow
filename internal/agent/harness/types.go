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

// Package harness ports Python's rag/advanced_rag/harness (agentic-RAG):
// route → planner → orchestrator → formalize_answer. This is a phased port; see
// tasks/agentic_search_port_plan.md §P4.
package harness

// RouteDecision mirrors Python RouteDecision.
type RouteDecision struct {
	Question              string
	ThinkingMode          string
	QuestionType          string // factual | comparative | analytical | procedural | exploratory | verification | summarization
	RequiresDecomposition bool
	ExecutionStrategy     string // direct_search | decompose_and_search | agentic_research | deep_research
	// SuggestsCompilation is the compiled-artifact type the route suggests using
	// for retrieval: "" (none) | "toc" | "graph" | "wiki". Mirrors Python's
	// route `suggests_compilation`. It is preserved so the production runner can
	// prefer a compiled wiki/graph tool when the bound KBs actually carry the
	// artifact, and fall back to general hybrid search otherwise.
	SuggestsCompilation string
	Reasoning           string
}

// ClaimTarget mirrors Python ClaimTarget.
type ClaimTarget struct {
	ClaimID        string
	Description    string
	Priority       int
	SuggestedTools []string
	IsVerified     bool
	Confidence     float64
	AgentResult    *AgentResult
}

// WorkflowPlan mirrors Python WorkflowPlan.
type WorkflowPlan struct {
	PlanType      string // direct | fact_decomposition | ...
	Claims        []ClaimTarget
	MaxIterations int
}

// ExecutionStrategy mirrors Python ExecutionStrategy and is instantiated by the
// four thinking modes.
type ExecutionStrategy struct {
	Label                 string
	Strategy              string
	RequiresDecomposition bool
	MaxOrchestratorCycles int
	MaxAgentCycles        int
	MaxParallelAgents     int
	AvailableTools        []string
	SufficiencyThreshold  float64
	PartialThreshold      float64
	FallbackToDirectLLM   bool
	RequiresSelectiveGen  bool
	AllowsReplan          bool
}

// AgentResult mirrors Python AgentResult.
type AgentResult struct {
	ClaimID     string
	Report      string
	IsVerified  bool
	Confidence  float64
	EvidenceIDs []int
}

// ClaimCrossCheckResult mirrors Python ClaimCrossCheckResult.
type ClaimCrossCheckResult struct {
	ClaimID          string
	CrossCheckPassed bool
	CrossCheckScore  float64
	EvidenceMatches  []string
	Mismatches       []string
	// HasEvidence reports whether any evidence chunk was actually examined. When
	// false the claim had no resolvable evidence, so it cannot be considered
	// verified regardless of the other fields.
	HasEvidence bool
}

// SufficiencyVerdict mirrors Python SufficiencyVerdict.
type SufficiencyVerdict struct {
	Status           string // SUFFICIENT | USEFUL_BUT_INCOMPLETE | INSUFFICIENT | CONFLICTING | UNANSWERABLE
	Score            float64
	AgentScore       float64
	CrossScore       float64
	ClaimAssessments []map[string]interface{}
	HasConflicts     bool
	MissingClaims    []string
	Feedback         string
	OverallReason    string
}

// OrchestratorContext mirrors Python OrchestratorContext.
type OrchestratorContext struct {
	Question  string
	Claims    []*ClaimTarget
	Mode      string
	Iteration int
}

// THINKING_MODES mirrors Python config.THINKING_MODES.
var THINKING_MODES = map[string]ExecutionStrategy{
	"low": {
		Label: "low", Strategy: "direct_search", RequiresDecomposition: false,
		MaxOrchestratorCycles: 1, MaxAgentCycles: 0, MaxParallelAgents: 1,
		AvailableTools: []string{"hybrid_search"}, SufficiencyThreshold: 0.85, PartialThreshold: 0.50,
	},
	"medium": {
		Label: "medium", Strategy: "decompose_and_search", RequiresDecomposition: true,
		MaxOrchestratorCycles: 3, MaxAgentCycles: 0, MaxParallelAgents: 1,
		AvailableTools: []string{"hybrid_search"}, SufficiencyThreshold: 0.75, PartialThreshold: 0.40,
		RequiresSelectiveGen: true,
	},
	"high": {
		Label: "high", Strategy: "agentic_research", RequiresDecomposition: true,
		MaxOrchestratorCycles: 3, MaxAgentCycles: 2, MaxParallelAgents: 2,
		AvailableTools: []string{
			"hybrid_search", "web_search", "ontology_navigate", "dataset_navigation_by_tree",
			"graph_explore", "inspector_open_context", "inspector_compare",
		},
		SufficiencyThreshold: 0.65, PartialThreshold: 0.30,
		RequiresSelectiveGen: true, AllowsReplan: true,
	},
	"ultra": {
		Label: "ultra", Strategy: "deep_research", RequiresDecomposition: true,
		MaxOrchestratorCycles: 4, MaxAgentCycles: 2, MaxParallelAgents: 3,
		AvailableTools: []string{
			"hybrid_search", "bm25_search", "web_search", "structured_query",
			"ontology_navigate", "dataset_navigation_by_tree", "mindmap_navigate",
			"graph_explore", "wiki_query", "inspector_open_context", "inspector_compare",
			"inspector_grep_within", "inspector_request_adjacent",
		},
		SufficiencyThreshold: 0.55, PartialThreshold: 0.20, FallbackToDirectLLM: true,
		RequiresSelectiveGen: true, AllowsReplan: true,
	},
}

// GetMode mirrors Python get_mode(label).
func GetMode(label string) (ExecutionStrategy, bool) {
	m, ok := THINKING_MODES[label]
	return m, ok
}
