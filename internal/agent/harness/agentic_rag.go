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
	"log"
	"strings"

	"gorm.io/gorm"
)

// AgenticState carries the shared state across the agentic-RAG graph nodes.
type AgenticState struct {
	Question       string
	Keywords       string
	Route          RouteDecision
	SeedChunks     []string
	Plan           WorkflowPlan
	Kbinfos        *Kbinfos
	PartialAnswer  bool
	Abstain        bool
	EmptyResult    bool
	FinalAnswer    string
	FormalizeError string
}

// RunAgenticRAG drives the agentic-search graph: route → pre_search → planner →
// orchestrator → formalize_answer. Mirrors build_agentic_graph's linear flow.
//
//   - low (direct_search): one hybrid search → answer.
//   - medium+ (decompose_and_search / agentic_research / deep_research):
//     pre_search grounds the planner, then decompose-and-search runs until a
//     sufficiency verdict stops it.
//
// RunAgenticRAG drives the agentic-search graph. It computes the route itself
// and delegates to RunAgenticRAGWithRoute so production runners that need a
// route-aware search strategy (e.g. prefer wiki_query on a wiki suggestion) can
// reuse the same flow with a pre-computed route.
func RunAgenticRAG(ctx context.Context, db *gorm.DB, question, keywords, modeLabel string, search SearchFn) AnswerResult {
	return RunAgenticRAGWithRoute(ctx, db, question, keywords, modeLabel, RouteNode(ctx, db, question, modeLabel), search)
}

// RunAgenticRAGWithRoute is the route-aware core of RunAgenticRAG. It performs
// pre_search → planner → orchestrator → formalize_answer with the given route.
func RunAgenticRAGWithRoute(ctx context.Context, db *gorm.DB, question, keywords, modeLabel string, route RouteDecision, search SearchFn) AnswerResult {
	state := &AgenticState{
		Question: strings.TrimSpace(question),
		Keywords: keywords,
		Kbinfos:  &Kbinfos{},
	}
	if state.Question == "" {
		return AnswerResult{FinalAnswer: emptyResultMessage, Empty: true}
	}

	// ── route (pre-computed by the caller) ──
	state.Route = route

	// ── pre_search (decomposition modes only) ──
	if state.Route.RequiresDecomposition {
		chunks, aggs := search(ctx, state.Question, state.Keywords)
		state.SeedChunks = extractChunkTexts(chunks)
		state.Kbinfos.Merge(chunks, aggs)
	}

	// ── planner ──
	state.Plan = PlannerNode(ctx, db, state.Route, state.SeedChunks)

	// ── orchestrator ──
	var orch OrchestratorResult
	if state.Route.RequiresDecomposition {
		claims := make([]*ClaimTarget, len(state.Plan.Claims))
		for i := range state.Plan.Claims {
			claims[i] = &state.Plan.Claims[i]
		}
		orch = DecomposeAndSearch(ctx, search, state.Question, state.Keywords, claims, modeLabel, state.Kbinfos)
	} else {
		orch = DirectSearch(ctx, search, state.Question, state.Keywords, state.Kbinfos)
	}
	state.PartialAnswer = orch.PartialAnswer
	state.Abstain = orch.Abstain
	state.EmptyResult = orch.EmptyResult
	if orch.Kbinfos != nil {
		state.Kbinfos = orch.Kbinfos
	}

	// ── formalize_answer ──
	res := FormalizeAnswer(ctx, db, state.Question, state.Kbinfos, state.PartialAnswer, state.Abstain, state.EmptyResult)
	// Log only the question length, never its content, to avoid persisting user
	// input in logs.
	log.Printf("agentic_rag: finished (qlen=%d, strategy=%s, chunks=%d, partial=%v, abstain=%v)",
		len(state.Question), state.Route.ExecutionStrategy, len(state.Kbinfos.Chunks), state.PartialAnswer, state.Abstain)
	return res
}

func extractChunkTexts(chunks []map[string]interface{}) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if t := chunkText(c); t != "" {
			out = append(out, t)
		}
	}
	return out
}
