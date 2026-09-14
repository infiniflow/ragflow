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

// Package orchestrator holds the top-level pipeline stages that sit above the
// action session: the Sufficient Context Agent (SCA) review and the gap→query
// rewriter.
//
// Mirrors Python rag/advanced_rag/harness/orchestrator/{sufficient_context,
// query_rewriter}.py.
//
// Python's orchestrator/direct.py — the low-mode direct search — deliberately
// has no counterpart in this package: its Go implementation is runDirect
// (agentic_rag.go, called from the low graph's direct_search node), the low graph
// node's body, because that step needs the full RAGTools config rather than a
// harness-level dependency bundle. One implementation only; its contract is
// pinned by TestBuildLowGraphRunsFormalizeThenDirectSearch.
package orchestrator

import (
	"context"
	"fmt"

	"ragflow/internal/common"
)

var _LOG = common.StdLogger()

// JSONModel generates one JSON object from a rendered prompt.
//
// Mirrors Python rag.prompts.generator.gen_json(prompt, "Output:\n", chat_mdl),
// which sends the prompt as the system turn, "Output:\n" as the user turn, and
// parses the first JSON value out of the reply.
type JSONModel interface {
	GenJSON(ctx context.Context, prompt string) (any, error)
}

func fmtClaimCount(n int) string { return fmt.Sprintf("%d claim draft(s)", n) }
