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
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"

	"gorm.io/gorm"
)

// decomposePrompt is a faithful port of Python's DECOMPOSE_FACTUAL prompt shape.
// Question-type-specific variants share the same structure but tune the
// instructions; the Go planner selects by question_type.
const decomposePrompt = `Break down the research question into %d atomic claims (facts or sub-questions).

Question: %s

Detail level: %s
Grounding context (preliminary retrieval, may be empty):
%s

Output format:
{
  "claims": [
    {"claim_id": "c0", "description": "...", "priority": 0, "suggested_tools": []}
  ]
}
`

type plannerClaim struct {
	ClaimID        string   `json:"claim_id"`
	Description    string   `json:"description"`
	Priority       int      `json:"priority"`
	SuggestedTools []string `json:"suggested_tools"`
}

type plannerResult struct {
	Claims []plannerClaim `json:"claims"`
}

// PlannerNode mirrors Python planner_node. It decomposes the routed question
// into ClaimTargets. Direct mode (no decomposition) returns a single coarse
// claim. On any failure it falls back to the direct plan.
func PlannerNode(ctx context.Context, db *gorm.DB, route RouteDecision, seedChunks []string) WorkflowPlan {
	if !route.RequiresDecomposition {
		return directPlan(route.Question)
	}
	mode, ok := GetMode(route.ThinkingMode)
	if !ok {
		// Unknown or empty mode label: fall back to medium so the planner is not
		// driven by a zero-valued mode (which would yield a degenerate plan).
		mode = THINKING_MODES["medium"]
	}
	maxClaims := maxClaimsFor(mode.Label)
	detail := detailLevelFor(mode.Label)

	prompt := fmt.Sprintf(decomposePrompt, maxClaims, route.Question, detail, formatSeedChunks(seedChunks))

	inv := chat.GetDefaultInvoker()
	if inv == nil {
		log.Printf("agentic_rag: planner_node skipped (chat invoker not configured); fallback direct")
		return directPlan(route.Question)
	}
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: prompt},
			{Role: schema.User, Content: route.Question},
		},
	})
	if err != nil {
		log.Printf("agentic_rag: planner_node failed (fallback direct): %v", err)
		return directPlan(route.Question)
	}
	var res plannerResult
	if err := unmarshalModelJSON(resp.Content, &res); err != nil {
		log.Printf("agentic_rag: planner_node parse failed (fallback direct): %v", err)
		return directPlan(route.Question)
	}
	claims := make([]ClaimTarget, 0, len(res.Claims))
	for i, c := range res.Claims {
		desc := strings.TrimSpace(c.Description)
		if desc == "" {
			continue
		}
		claims = append(claims, ClaimTarget{
			ClaimID:        orDefault(c.ClaimID, fmt.Sprintf("c%d", i)),
			Description:    desc,
			Priority:       c.Priority,
			SuggestedTools: c.SuggestedTools,
		})
	}
	if len(claims) == 0 {
		return directPlan(route.Question)
	}
	return WorkflowPlan{
		PlanType:      planTypeFor(route.QuestionType),
		Claims:        claims,
		MaxIterations: mode.MaxOrchestratorCycles,
	}
}

func directPlan(question string) WorkflowPlan {
	return WorkflowPlan{
		PlanType:      "direct",
		Claims:        []ClaimTarget{{ClaimID: "c0", Description: question, Priority: 0}},
		MaxIterations: 1,
	}
}

func planTypeFor(qt string) string {
	switch qt {
	case "factual":
		return "fact_decomposition"
	case "comparative":
		return "comparative_decomposition"
	case "procedural":
		return "procedural_decomposition"
	default:
		return "exploratory_decomposition"
	}
}

func maxClaimsFor(modeLabel string) int {
	return map[string]int{"low": 1, "medium": 3, "high": 5, "ultra": 8}[modeLabel]
}

func detailLevelFor(modeLabel string) string {
	return map[string]string{"low": "coarse", "medium": "normal", "high": "fine", "ultra": "extra_fine"}[modeLabel]
}

func formatSeedChunks(seedChunks []string) string {
	if len(seedChunks) == 0 {
		return "(no preliminary results)"
	}
	return strings.Join(seedChunks, "\n")
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
