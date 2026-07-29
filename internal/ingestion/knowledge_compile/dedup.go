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

package knowledge_compile

import (
	"context"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
)

// Deduper folds a set of per-document compiled Products into the dataset-level
// merged set. The LLM-backed implementation reuses the component's
// GroupedDeduper + LLMMergeDecider (§11.6).
type Deduper interface {
	Dedup(ctx context.Context, rows []kccommon.Product) ([]kccommon.Product, error)
}

// DeduperFactory builds a per-tenant Deduper. It is invoked once per batch so
// the LLM deps can be resolved for the owning tenant.
type DeduperFactory func(tenant string) (Deduper, error)

// llmDeduper wraps the component's GroupedDeduper (which internally uses
// LLMMergeDecider for duplicate-judging), scoped to the whole KB batch.
type llmDeduper struct {
	group   *structure.GroupedDeduper
	decider *structure.LLMMergeDecider
	embed   kccommon.Embedder
}

// NewLLMDeduper builds a KB-scoped deduper from the runtime chat/embed deps.
func NewLLMDeduper(chat kccommon.ChatInvoker, embed kccommon.Embedder, llmID string, threshold float64) Deduper {
	decider := structure.NewLLMMergeDecider(chat, llmID, embed, threshold)
	return &llmDeduper{group: structure.NewGroupedDeduper(decider), decider: decider, embed: embed}
}

func (x *llmDeduper) Dedup(ctx context.Context, rows []kccommon.Product) ([]kccommon.Product, error) {
	for _, r := range rows {
		if err := x.group.Add(ctx, r); err != nil {
			return nil, err
		}
	}
	// Apply the aliases recorded by the LLM merge decider to relation endpoints
	// so merged entities collapse consistently with the per-document dedup path.
	if err := x.group.RewriteRelations(ctx, x.decider.Aliases(), x.embed); err != nil {
		return nil, err
	}
	return x.group.Rows(), nil
}

// noopDeduper performs no LLM merge; it returns the input rows unchanged so
// the writer still emits dataset-level products (without cross-document merging).
// Used as a safe fallback when LLM deps are unavailable.
type noopDeduper struct{}

func (noopDeduper) Dedup(_ context.Context, rows []kccommon.Product) ([]kccommon.Product, error) {
	return rows, nil
}

// NewNoopDeduper builds the fallback deduper.
func NewNoopDeduper() Deduper { return noopDeduper{} }
