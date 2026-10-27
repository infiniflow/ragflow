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

package agentic_rag

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

// compiledKnowledgeToolNames are the tools that only make sense on a corpus that
// has produced knowledge-compilation products (a navigation tree, a compiled
// structure, a knowledge graph, a per-document panorama). They are INJECTED,
// never declared in a template's tool list: Run adds them only when the bound
// datasets actually hold compiled products, so an uncompiled corpus never
// offers - or pays for - them.
//
// The injected set is the whole compiled-knowledge surface on purpose: a tool
// that is registered but neither declared by a template nor injected here is
// unreachable at runtime, which is indistinguishable from "the model chose not
// to use it" unless someone reads the tool list.
var compiledKnowledgeToolNames = []string{
	navigateTreeToolName,
	navigateStructureToolName,
	graphExploreToolName,
	graphDocOutlineToolName,
}

// compiledKnowledgeTools builds the injected compiled-knowledge tool set scoped
// to the given tenant/datasets.
func compiledKnowledgeTools(tenantID string, datasetIDs []string) []tool.BaseTool {
	reg := toolRegistry()
	out := make([]tool.BaseTool, 0, len(compiledKnowledgeToolNames))
	for _, name := range compiledKnowledgeToolNames {
		if f, ok := reg[name]; ok {
			out = append(out, f(tenantID, datasetIDs))
		}
	}
	return out
}

// datasetsHaveCompiledKnowledge reports whether any bound dataset carries a
// knowledge-compilation product. Compiled products are exactly the rows that
// carry compile_kwd; ordinary chunks do not (that is what the default retriever
// excludes with must_not/exists compile_kwd), so a single existence read answers
// it. A missing engine or an empty scope is false, leaving the tools undeclared.
func datasetsHaveCompiledKnowledge(ctx context.Context, de engine.DocEngine, tenantID string, datasetIDs []string) bool {
	if len(datasetIDs) == 0 {
		return false
	}
	if de == nil {
		return false
	}
	res, err := de.Search(ctx, &enginetypes.SearchRequest{
		IndexNames:         []string{indexNameFor(tenantID, "")},
		KbIDs:              datasetIDs,
		SelectFields:       []string{"id"},
		Filter:             map[string]interface{}{"exists": "compile_kwd"},
		Limit:              1,
		IncludeUnavailable: true,
	})
	if err != nil || res == nil {
		// A probe that fails for an infrastructure reason (engine down, index
		// missing) must not read as "this corpus was never compiled": say so,
		// and let the caller's own INFO line carry the decision.
		common.DebugCtx(ctx, "agentic_rag: compiled-knowledge probe failed",
			zap.String("tenant_id", tenantID),
			zap.Strings("datasets", datasetIDs),
			zap.Error(err))
		return false
	}
	return len(res.Chunks) > 0
}

func toolsForRun(ctx context.Context, tmpl Template, in Input, de engine.DocEngine) []tool.BaseTool {
	tools := toolsFor(ctx, tmpl, in.TenantID, in.DatasetIDs, in.Tools)
	if len(in.Tools) != 0 || tmpl.strictTools {
		common.InfoCtx(ctx, "agentic_rag: compiled-knowledge tools skipped",
			zap.Strings("datasets", in.DatasetIDs),
			zap.String("reason", "explicit tool selection"))
	} else if datasetsHaveCompiledKnowledge(ctx, de, in.TenantID, in.DatasetIDs) {
		compiled := compiledKnowledgeTools(in.TenantID, in.DatasetIDs)
		tools = append(tools, compiled...)
		common.InfoCtx(ctx, "agentic_rag: compiled-knowledge tools injected",
			zap.Strings("datasets", in.DatasetIDs),
			zap.String("reason", "compile_kwd rows present in the bound datasets"),
			zap.Int("tools", len(compiled)))
	} else {
		common.InfoCtx(ctx, "agentic_rag: compiled-knowledge tools skipped",
			zap.Strings("datasets", in.DatasetIDs),
			zap.String("reason", "no compile_kwd row in the bound datasets"))
	}
	return tools
}

// datasetsHaveClaims reports whether any bound dataset carries knowledge-compiled
// CLAIM rows (entity_type_kwd="claim").
//
// It is the gate for the claim prefetch and is deliberately STRICTER than
// datasetsHaveCompiledKnowledge: a dataset compiled into a navigation tree or a
// timeline has compile_kwd rows but no evidence rows, so the looser probe would
// let the pipeline spend a routing call plus two retrieval legs per document to
// learn that there is nothing to prefetch. claimRowsPresent wraps it so tests can
// drive the gate without an engine.
func datasetsHaveClaims(ctx context.Context, tenantID string, datasetIDs []string) bool {
	if len(datasetIDs) == 0 {
		return false
	}
	de := engine.Get()
	if de == nil {
		return false
	}
	res, err := de.Search(ctx, &enginetypes.SearchRequest{
		IndexNames:         []string{indexNameFor(tenantID, "")},
		KbIDs:              datasetIDs,
		SelectFields:       []string{"id"},
		Filter:             map[string]interface{}{"entity_type_kwd": []string{navClaimRowType}},
		Limit:              1,
		IncludeUnavailable: true,
	})
	if err != nil || res == nil {
		common.DebugCtx(ctx, "agentic_rag: claim probe failed",
			zap.String("tenant_id", tenantID),
			zap.Strings("datasets", datasetIDs),
			zap.Error(err))
		return false
	}
	return len(res.Chunks) > 0
}

// claimRowsPresent is the claim-prefetch gate; tests replace it.
var claimRowsPresent = datasetsHaveClaims
