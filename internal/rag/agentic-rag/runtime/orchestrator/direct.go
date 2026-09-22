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

// Package orchestrator holds the two shapes the graph still shares with what used to be the
// orchestrator stage: JSONModel, the seam the graph's own model satisfies, and MissingPiece, the
// gap a round's record still yields.
//
// The stage itself is gone — the SCA (Sufficient Context Agent) review, the sufficiency round and
// the gap→query rewriter were removed with it (see the note in query_rewriter.go), leaving the
// vocabulary those call sites still need.
//
// The low-mode direct search deliberately has no counterpart in this package: its Go
// implementation is runDirect (agentic_rag.go, called from the low graph's direct_search node),
// because that step needs the full RAGTools config rather than a runtime-level dependency bundle.
// One implementation only; its contract is pinned by TestBuildLowGraphRunsFormalizeThenDirectSearch.
package orchestrator

import "context"

// JSONModel generates one JSON object from a rendered prompt.
//
// The prompt is sent as the system turn, "Output:\n" as the user turn, and the first JSON
// value is parsed out of the reply.
type JSONModel interface {
	GenJSON(ctx context.Context, prompt string) (any, error)
}
