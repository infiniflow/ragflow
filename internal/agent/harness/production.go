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
	"fmt"
	"log"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"

	"gorm.io/gorm"
	"ragflow/internal/agent/tool"
	"ragflow/internal/service/nav"
)

// ProductionRunner wires the real agentic-search tools (hybrid_search,
// dataset_navigation_by_tree) into the RunAgenticRAG flow, so the tools are
// actually invoked rather than merely registered. This is the production
// counterpart to the unit-testable SearchFn seam.
type ProductionRunner struct {
	db         *gorm.DB
	tenantID   string
	datasetIDs []string
	searchTool einotool.InvokableTool
	navSvc     nav.NavService // defaults to nav.GetNavService() when nil
}

// NewProductionRunner builds a ProductionRunner backed by the real tools. The
// dataset-nav router (harness.NavigateDatasetByTree) resolves its NavService
// lazily via nav.GetNavService().
func NewProductionRunner(db *gorm.DB, tenantID string, datasetIDs []string) (*ProductionRunner, error) {
	searchBase, err := tool.BuildByName("hybrid_search", nil)
	if err != nil {
		return nil, err
	}
	search, ok := searchBase.(einotool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("hybrid_search is not invokable")
	}
	return &ProductionRunner{db: db, tenantID: tenantID, datasetIDs: datasetIDs, searchTool: search}, nil
}

// newProductionRunnerWithTools builds a ProductionRunner with an injected
// search tool and nav service, for unit/E2E tests that want to fake the
// invocation surface without real services.
func newProductionRunnerWithTools(db *gorm.DB, tenantID string, datasetIDs []string, searchTool einotool.InvokableTool, navSvc nav.NavService) *ProductionRunner {
	return &ProductionRunner{db: db, tenantID: tenantID, datasetIDs: datasetIDs, searchTool: searchTool, navSvc: navSvc}
}

// Run executes the agentic-search graph with the real tools. It returns the
// final answer.
func (r *ProductionRunner) Run(ctx context.Context, question, keywords, modeLabel string) AnswerResult {
	if r.searchTool == nil {
		log.Printf("agentic_rag: production runner not fully wired (search tool missing)")
		return AnswerResult{FinalAnswer: emptyResultMessage, Empty: true}
	}
	// The router tool returns a doc list; feed it as the search DocScope.
	searchFn := func(ctx context.Context, query, kws string) ([]map[string]interface{}, []map[string]interface{}) {
		return r.search(ctx, query, kws, nil)
	}

	// For decomposition modes, route the doc scope first via the nav tool.
	if mode, _ := GetMode(modeLabel); mode.RequiresDecomposition {
		docs := r.routeDocs(ctx, question, keywords)
		if len(docs) > 0 {
			searchFn = func(ctx context.Context, query, kws string) ([]map[string]interface{}, []map[string]interface{}) {
				return r.search(ctx, query, kws, docs)
			}
		}
	}
	return RunAgenticRAG(ctx, r.db, question, keywords, modeLabel, searchFn)
}

// search invokes the hybrid_search tool and normalizes its chunk output.
func (r *ProductionRunner) search(ctx context.Context, query, keywords string, docScope []string) ([]map[string]interface{}, []map[string]interface{}) {
	args := map[string]interface{}{"query": query, "keywords": keywords, "kb_ids": r.datasetIDs, "top_n": 12}
	if len(docScope) > 0 {
		args["doc_scope"] = docScope
	}
	raw, err := r.searchTool.InvokableRun(ctx, mustJSON(args))
	if err != nil {
		log.Printf("agentic_rag: hybrid_search failed: %v", err)
		return nil, nil
	}
	var res struct {
		Chunks []map[string]interface{} `json:"chunks"`
	}
	if err := json.Unmarshal([]byte(raw), &res); err != nil {
		return nil, nil
	}
	return res.Chunks, nil
}

// routeDocs derives the doc scope via the canonical dataset-nav router
// (harness.NavigateDatasetByTree — the full LLM two-round selection). It routes
// across ALL bound datasets and merges the doc ids, so every KB contributes its
// own relevant docs to the shared scope (a multi-KB session must not collapse to
// the first KB only).
func (r *ProductionRunner) routeDocs(ctx context.Context, topic, keywords string) []string {
	ns := r.navSvc
	if ns == nil {
		ns = nav.GetNavService()
	}
	if ns == nil {
		log.Printf("agentic_rag: dataset nav service not initialized; skipping doc routing")
		return nil
	}
	// Combine topic + keywords into the routing query so the nav router actually
	// uses the full user signal (keywords must not be dropped).
	query := strings.TrimSpace(topic + " " + keywords)
	seen := map[string]bool{}
	var docs []string
	for _, kbID := range r.datasetIDs {
		for _, id := range NavigateDatasetByTree(ctx, r.db, ns, r.tenantID, kbID, query) {
			if id != "" && !seen[id] {
				seen[id] = true
				docs = append(docs, id)
			}
		}
	}
	return docs
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
