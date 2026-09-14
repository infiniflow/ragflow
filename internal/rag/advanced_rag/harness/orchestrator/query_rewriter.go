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

package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/prompts"
)

// Query rewriter (Phase 4 of the pipeline).
//
// Mirrors Python orchestrator/query_rewriter.py::rewrite_gap_to_query.
//

// RewriteDeps are the dependencies of RewriteGapToQuery.
type RewriteDeps struct {
	// Model generates the JSON object.
	Model JSONModel
	// Prompts supplies the "sca_query_rewrite" template.
	Prompts harness.PromptLoader
	// BridgeValues are already-resolved values across the claims (e.g. the
	// confirmed shows in a "which finale ran longest" question). The rewriter
	// anchors the new query to these, so a missing hop is re-searched with the
	// resolved upstream value instead of re-deriving it.
	BridgeValues []string
	// ResearchContext is the rendered block describing what previous rounds
	// already tried (queries + outcomes) and what the evidence pool contains.
	// It lets the rewriter aim at UNCOVERED angles instead of paraphrasing dead
	// ones.
	ResearchContext string
}

// RewriteGapToQuery mirrors Python rewrite_gap_to_query.
//
// gaps are (what, search_hint) tuples — the forward gaps the SCA identified.
//
// Returns [{"query": "..."}] — targeted, retrievable queries. Empty when the
// rewrite is unavailable or fails, in which case the caller falls back to the
// original gap text.
func RewriteGapToQuery(ctx context.Context, deps RewriteDeps, question string, gaps []MissingPiece) []map[string]string {
	if deps.Model == nil || len(gaps) == 0 {
		return nil
	}
	// Python wraps the call in @in_phase("rewrite").
	ctx, done := harness.Phase(ctx, harness.PhaseRewrite)
	defer done()

	var gapsLines []string
	for _, g := range gaps {
		gapsLines = append(gapsLines, fmt.Sprintf("- what: %s; hint: %s", g.What, g.SearchHint))
	}
	gapsText := strings.Join(gapsLines, "\n")

	var bridgeLines []string
	for _, b := range deps.BridgeValues {
		if strings.TrimSpace(b) != "" {
			bridgeLines = append(bridgeLines, "- "+b)
		}
	}
	bridgeText := strings.Join(bridgeLines, "\n")

	prompt := prompts.Render(deps.Prompts, "sca_query_rewrite", "", map[string]string{
		"question":         question,
		"gaps":             gapsText,
		"bridge_values":    bridgeText,
		"research_context": deps.ResearchContext,
	})

	result, err := deps.Model.GenJSON(ctx, prompt)
	if err != nil {
		_LOG.Printf("[QueryRewrite] failed: %v", err)
		return nil
	}
	data, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	rawQueries, _ := data["queries"].([]any)

	var out []map[string]string
	for _, q := range rawQueries {
		var query string
		if m, ok := q.(map[string]any); ok {
			// Accept `query` (canonical) or `question` (observed drift).
			if v := m["query"]; v != nil {
				query = strings.TrimSpace(fmt.Sprint(v))
			} else if v := m["question"]; v != nil {
				query = strings.TrimSpace(fmt.Sprint(v))
			}
		} else if q != nil {
			query = strings.TrimSpace(fmt.Sprint(q))
		}
		if query == "" || containsQuery(out, query) {
			continue
		}
		out = append(out, map[string]string{"query": query})
	}
	return out
}

func containsQuery(queries []map[string]string, q string) bool {
	for _, item := range queries {
		if item["query"] == q {
			return true
		}
	}
	return false
}
