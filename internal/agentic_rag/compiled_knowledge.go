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

	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

// compiledKnowledgeToolNames are the tools that only make sense on a corpus that
// has produced knowledge-compilation products (a navigation tree, a compiled
// structure, a knowledge graph). They are INJECTED, never declared in a
// template's tool list: Run adds them only when the bound datasets actually hold
// compiled products, so an uncompiled corpus never offers - or pays for - them.
var compiledKnowledgeToolNames = []string{
	navigateTreeToolName,
	navigateStructureToolName,
	graphExploreToolName,
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
func datasetsHaveCompiledKnowledge(ctx context.Context, tenantID string, datasetIDs []string) bool {
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
		Filter:             map[string]interface{}{"exists": "compile_kwd"},
		Limit:              1,
		IncludeUnavailable: true,
	})
	if err != nil || res == nil {
		return false
	}
	return len(res.Chunks) > 0
}
