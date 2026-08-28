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
	"encoding/json"
	"log"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
	"ragflow/internal/common"

	"gorm.io/gorm"
)

// routePrompt mirrors Python harness/prompts/route_prompt.py.
const routePrompt = `Analyze the following question and output a structured query analysis.

Question: %s

Analyze it across these dimensions:
1. Question type: factual / comparative / analytical / procedural / exploratory / verification / summarization.
2. Whether it needs decomposition into atomic facts, meaning whether multiple independent pieces of information must be retrieved separately before answering: true/false.
3. Suggested knowledge compilation tool: null (none) / toc (document table of contents) / graph (knowledge graph) / wiki (compiled domain knowledge).

Output format (JSON):
{
    "question_type": "comparative",
    "requires_decomposition": true,
    "suggests_compilation": null,
    "reasoning": "This is a comparative question, so it needs to be decomposed into two independent facts and one comparison relation."
}
`

type routeResult struct {
	QuestionType        string `json:"question_type"`
	RequiresDecomp      *bool  `json:"requires_decomposition"`
	SuggestsCompilation string `json:"suggests_compilation"`
	Reasoning           string `json:"reasoning"`
}

// RouteNode mirrors Python route_node. It classifies the question into a
// RouteDecision using the default chat invoker. Pure classification, no KB
// dependency. Never fails — falls back to a factual/direct decision.
func RouteNode(ctx context.Context, db *gorm.DB, question, modeLabel string) RouteDecision {
	if strings.TrimSpace(question) == "" {
		return fallbackRoute(question, modeLabel, "fallback: empty question")
	}
	inv := chat.GetDefaultInvoker()
	if inv == nil {
		return fallbackRoute(question, modeLabel, "fallback: chat invoker not configured")
	}
	resp, err := inv.Invoke(ctx, db, chat.Request{
		Messages: []schema.Message{
			{Role: schema.System, Content: strings.ReplaceAll(routePrompt, "%s", question)},
			{Role: schema.User, Content: question},
		},
	})
	if err != nil {
		log.Printf("agentic_rag: route_node failed (fallback): %v", err)
		return fallbackRoute(question, modeLabel, "fallback: LLM error")
	}
	var res routeResult
	if err := unmarshalModelJSON(resp.Content, &res); err != nil {
		log.Printf("agentic_rag: route_node parse failed (fallback): %v", err)
		return fallbackRoute(question, modeLabel, "fallback: parse error")
	}
	return decide(question, modeLabel, res)
}

// decide applies the mode's execution strategy to the LLM route output.
func decide(question, modeLabel string, res routeResult) RouteDecision {
	mode, ok := GetMode(modeLabel)
	if !ok {
		mode = THINKING_MODES["medium"]
	}
	qType := res.QuestionType
	if qType == "" {
		qType = "factual"
	}
	needDecomp := true
	if res.RequiresDecomp != nil {
		needDecomp = *res.RequiresDecomp
	}
	return RouteDecision{
		Question:              question,
		ThinkingMode:          modeLabel,
		QuestionType:          qType,
		RequiresDecomposition: mode.RequiresDecomposition && needDecomp,
		ExecutionStrategy:     mode.Strategy,
		SuggestsCompilation:   normalizeCompilationSuggestion(res.SuggestsCompilation),
		Reasoning:             res.Reasoning,
	}
}

// normalizeCompilationSuggestion maps the LLM's free-text suggestion to a
// canonical compiled-artifact key: "", "toc", "graph", or "wiki". Anything else
// (including "null") collapses to "" so the production runner never routes on an
// unknown artifact name.
func normalizeCompilationSuggestion(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "toc":
		return "toc"
	case "graph", "knowledge_graph", "kg":
		return "graph"
	case "wiki", "compiled":
		return "wiki"
	default:
		return ""
	}
}

func fallbackRoute(question, modeLabel, reason string) RouteDecision {
	return RouteDecision{
		Question: question, ThinkingMode: modeLabel, QuestionType: "factual",
		RequiresDecomposition: false, ExecutionStrategy: "direct_search", Reasoning: reason,
	}
}

var (
	reFence = regexp.MustCompile("```(?:json)?\\s*|\\s*```")
)

// unmarshalModelJSON mirrors Python's _extract_json: strip thinking preamble and
// Markdown fences, then parse JSON.
func unmarshalModelJSON(text string, out interface{}) error {
	text = common.StripThinkTrailing(text)
	text = reFence.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	if text == "" {
		return json.Unmarshal([]byte("{}"), out)
	}
	return json.Unmarshal([]byte(text), out)
}
