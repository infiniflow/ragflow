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
//
// Dedup folds the completed batch's products among themselves (in-memory, before
// any engine round-trip). Decide judges whether an incoming product duplicates
// an existing merged row found by KNN and, when so, returns the merged row
// (mirrors Python _struct_doc_storage_dedup_batch: KNN top1 + LLM merge).
type Deduper interface {
	Dedup(ctx context.Context, rows []kccommon.Product) ([]kccommon.Product, error)
	Decide(ctx context.Context, existing, incoming kccommon.Product, bestScore float64) (kccommon.Product, bool, error)
	// DecideBatch judges every (existing, candidates) group in a single LLM
	// round-trip, folding each group's candidates into its existing row and
	// reporting the merged row plus any candidates judged distinct (new rows).
	// It replaces the per-pair Decide loop at the batch hot path.
	DecideBatch(ctx context.Context, groups []MergeGroup) ([]MergeGroup, error)
}

// MergeGroup is one KNN-found existing merged row plus the batch of incoming
// products that all KNN-hit it. DecideBatch folds the candidates into Existing
// and fills Merged (the updated row), Duplicate (whether anything was merged),
// and Distinct (candidates judged not-duplicates, i.e. new merged rows).
type MergeGroup struct {
	Existing   kccommon.Product
	Candidates []kccommon.Product
	Score      float64

	Merged    kccommon.Product
	Duplicate bool
	Distinct  []kccommon.Product
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
// modelContentLength is the chat model's context window (content_length) and
// modelMaxOutput its generation cap (max_output); both bound how many pairs a
// single DecideBatch sub-call may judge (a value <= 0 disables per-batch token
// splitting). Together they keep every sub-call inside the input window and the
// output cap, so a large candidate set never overflows either.
func NewLLMDeduper(chat kccommon.ChatInvoker, embed kccommon.Embedder, llmID string, threshold float64, modelContentLength, modelMaxOutput int) Deduper {
	decider := structure.NewLLMMergeDecider(chat, llmID, embed, threshold)
	decider.SetMaxBatchTokens(modelContentLength, modelMaxOutput)
	// Share the process-wide, vCPU-sized compiler pool so DecideBatch's
	// token-bounded sub-batches run concurrently with the rest of the pipeline
	// (LLM-bounded), all under one concurrency limit.
	decider.SetSubmitter(func(ctx context.Context, jobs []func() error) error {
		return SubmitCompilerJobs(ctx, jobs)
	})
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

// Decide delegates the per-pair duplicate judgment to the LLM merge decider,
// which re-embeds the merged payload and unions provenance on a duplicate verdict.
func (x *llmDeduper) Decide(ctx context.Context, existing, incoming kccommon.Product, bestScore float64) (kccommon.Product, bool, error) {
	decision, merged, err := x.decider.Decide(ctx, existing, incoming, bestScore)
	if err != nil {
		return kccommon.Product{}, false, err
	}
	if decision == structure.DecisionMerge {
		return merged, true, nil
	}
	return kccommon.Product{}, false, nil
}

// DecideBatch folds every group into its dataset-level merged row. Wiki groups
// use the page-specific Markdown evidence merge, while structure groups are
// judged by the generic LLM merge decider.
// splitWikiGroups partitions the batch into wiki groups (page merge)
// and structure groups (LLM-merged). structIdx[i] is the ORIGINAL position in
// `groups` of structGroups[i]; it is what the fold uses to write results back to
// the right slice element. Recording the position inside structGroups (i.e.
// len(structGroups)) instead would always equal i and misroute structure results
// to the wrong groups whenever a wiki group appears earlier in the batch.
func splitWikiGroups(groups []MergeGroup) (wikiIdx, structIdx []int, structGroups []MergeGroup) {
	for gi := range groups {
		if isWikiGroup(groups[gi]) {
			wikiIdx = append(wikiIdx, gi)
		} else {
			structIdx = append(structIdx, gi)
			structGroups = append(structGroups, groups[gi])
		}
	}
	return wikiIdx, structIdx, structGroups
}

func (x *llmDeduper) DecideBatch(ctx context.Context, groups []MergeGroup) ([]MergeGroup, error) {
	// Split wiki and structure groups because Markdown needs a page-specific
	// merge and must not be parsed as generic JSON.
	wikiIdx, structIdx, structGroups := splitWikiGroups(groups)
	// Wiki groups: evidence-preserving page merge, in place.
	wikiGroups := make([]MergeGroup, len(wikiIdx))
	for i, gi := range wikiIdx {
		wikiGroups[i] = groups[gi]
	}
	for i, group := range wikiMergeBatch(ctx, wikiGroups) {
		groups[wikiIdx[i]] = group
	}
	if len(structGroups) == 0 {
		return groups, nil
	}

	// Assign a flat pair index to every (group, candidate) of the structure groups.
	var inputs []structure.MergePairInput
	pairIndexOf := make([][]int, len(structGroups))
	for gi := range structGroups {
		pairIndexOf[gi] = make([]int, len(structGroups[gi].Candidates))
		for ci := range structGroups[gi].Candidates {
			idx := len(inputs)
			pairIndexOf[gi][ci] = idx
			inputs = append(inputs, structure.MergePairInput{
				Index:    idx,
				Existing: structGroups[gi].Existing.Content,
				Incoming: structGroups[gi].Candidates[ci].Content,
			})
		}
	}
	if len(inputs) == 0 {
		// Only wiki groups had candidates; copy them back (already folded above).
		return groups, nil
	}
	results, err := x.decider.DecideBatch(ctx, inputs)
	if err != nil {
		return nil, err
	}
	byIndex := make(map[int]structure.BatchMergeResult, len(results))
	for _, r := range results {
		byIndex[r.Index] = r
	}

	for si, gi := range structIdx {
		existing := structGroups[si].Existing
		var distinct []kccommon.Product
		duplicated := false
		for ci, cand := range structGroups[si].Candidates {
			r := byIndex[pairIndexOf[si][ci]]
			if !r.Duplicated || r.Merged == nil {
				// Judged distinct: keep it as its own new merged row.
				c := cand
				c.Merged = true
				c.DocID = existing.DocID
				distinct = append(distinct, c)
				continue
			}
			replacement, err := x.decider.BuildReplacement(ctx, existing, cand, r.Merged)
			if err != nil {
				return nil, err
			}
			existing = replacement
			duplicated = true
		}
		groups[gi].Merged = existing
		groups[gi].Duplicate = duplicated
		groups[gi].Distinct = distinct
	}
	return groups, nil
}

// noopDeduper performs no LLM merge; it returns the input rows unchanged so
// the writer still emits dataset-level products (without cross-document merging).
// Used as a safe fallback when LLM deps are unavailable.
type noopDeduper struct{}

func (noopDeduper) Dedup(_ context.Context, rows []kccommon.Product) ([]kccommon.Product, error) {
	return rows, nil
}

// Decide never merges: without an LLM judge every incoming product is kept as a
// distinct merged row (matches the noop fallback's "no cross-document merge").
func (noopDeduper) Decide(_ context.Context, _, incoming kccommon.Product, _ float64) (kccommon.Product, bool, error) {
	return kccommon.Product{}, false, nil
}

// DecideBatch never merges: every candidate becomes its own new merged row.
func (noopDeduper) DecideBatch(_ context.Context, groups []MergeGroup) ([]MergeGroup, error) {
	for gi := range groups {
		existing := groups[gi].Existing
		var distinct []kccommon.Product
		for _, cand := range groups[gi].Candidates {
			c := cand
			c.Merged = true
			c.DocID = existing.DocID
			distinct = append(distinct, c)
		}
		groups[gi].Merged = existing
		groups[gi].Duplicate = false
		groups[gi].Distinct = distinct
	}
	return groups, nil
}

// NewNoopDeduper builds the fallback deduper.
func NewNoopDeduper() Deduper { return noopDeduper{} }
