package wiki

import "sort"

// This file implements the PLAN-stage page-budget controls that align the Go
// wiki variant with Python's wiki.py:
//
//   - a global target_page_count derived from item count (clamp(8, total//3, 60));
//   - a dynamic max_page_count derived from the model's context window that acts
//     as an unbreakable hard cap (output-token capacity vs page-token estimate);
//   - per-batch page quotas distributed by largest-remainder so the quota sum
//     equals the global target and no batch silently multiplies the page count;
//   - a deterministic, mention-grounded truncation that selects the top pages
//     under the global cap and reports how many were excluded.
//
// The provider never receives an explicit output max_tokens on the wiki path
// (see P0 in tasks/2026-08-04-wiki-go-python-gap-alignment-plan.md): output
// scale is controlled purely through max_pages in the prompt + the in-code
// truncation below.

// Python alignment constants (wiki.py:1670-1674, 1783-1787).
const (
	wikiPlanMaxOutputTokens    = 4096
	wikiPlanOutputSafetyTokens = 256
	wikiPlanPageTokenEstimate  = 48
	wikiPlanTargetPageCountMin = 8
	wikiPlanTargetPageCountMax = 60
)

// wikiTargetPageCount mirrors Python _wiki_target_page_count:
// clamp(8, total//3, 60).
func wikiTargetPageCount(totalItems int) int {
	if totalItems <= 0 {
		return wikiPlanTargetPageCountMin
	}
	if n := totalItems / 3; n < wikiPlanTargetPageCountMin {
		return wikiPlanTargetPageCountMin
	} else if n > wikiPlanTargetPageCountMax {
		return wikiPlanTargetPageCountMax
	} else {
		return n
	}
}

// wikiPlanBudget is the resolved page budget for one planning run.
type wikiPlanBudget struct {
	// Target is the approximate page count the planner should aim for. It is
	// only approximate: batches are allocated quotas that sum to it, but the
	// merged result may differ slightly. Target is NOT a capacity guarantee.
	Target int
	// Max is the unbreakable global hard cap derived from output-token
	// capacity. The merged, slug-deduped page list is truncated to at most Max
	// pages regardless of what the batches produced. Max may be below Target
	// when the model's output capacity is smaller than the item-count-derived
	// target; that is deliberate (never ask a small-window model for more pages
	// than its output can hold).
	Max int
}

// Cap is the page budget the planner is actually allowed to emit. It is the
// achievable bound min(Target, Max): a capacity-limited model must never be
// asked for more pages than its output can hold, so the cap (not the
// item-count-derived Target) drives per-batch quota allocation.
func (b wikiPlanBudget) Cap() int {
	if b.Max < b.Target {
		return b.Max
	}
	return b.Target
}

// deriveWikiPlanBudget computes the global page budget from the model's context
// window and the reduced item count, mirroring Python's
// output_tokens / output_page_capacity / max_page_count derivation
// (wiki.py:2066-2078). modelContextLen is the chat model's context window in
// tokens (0 means unknown).
func deriveWikiPlanBudget(modelContextLen, totalItems int) wikiPlanBudget {
	target := wikiTargetPageCount(totalItems)

	if modelContextLen <= 0 {
		modelContextLen = 8192
	}
	// output_tokens = min(4096, max(1024, int(model_context * 0.4))).
	outputTokens := modelContextLen * 2 / 5 // 0.4
	if outputTokens < 1024 {
		outputTokens = 1024
	}
	if outputTokens > wikiPlanMaxOutputTokens {
		outputTokens = wikiPlanMaxOutputTokens
	}

	capacity := (outputTokens - wikiPlanOutputSafetyTokens) / wikiPlanPageTokenEstimate
	if capacity < 1 {
		capacity = 1
	}
	maxCount := capacity
	if n := target + 8; n < maxCount {
		maxCount = n
	}
	if n := target * 2; n < maxCount {
		maxCount = n
	}
	// Max is the unbreakable global hard cap. It is NOT raised back up to
	// Target when output-token capacity is small: a small-window model must
	// never be asked to emit more pages than its output capacity permits, or we
	// reintroduce truncated-JSON risk. When capacity < Target, Max simply lands
	// below Target and the achievable page count is capacity-bound.
	return wikiPlanBudget{Target: target, Max: maxCount}
}

// wikiExtractItemCount counts the planning items in one reduced extract. It is
// the unit used for proportional quota allocation.
func wikiExtractItemCount(e wikiExtract) int {
	return len(e.Entities) + len(e.Concepts) + len(e.Claims) + len(e.Relations) + len(e.Topics)
}

// allocatePlanQuotas distributes totalTarget pages across batches proportionally
// to each batch's item count using the largest-remainder method, padding by
// remainder in original batch order. When the number of batches exceeds the
// target, small batches naturally receive a zero quota (their floor rounds to
// zero and no remainder remains for them).
//
// Invariant: when len(batches) <= totalTarget, the returned quotas sum exactly
// to totalTarget; when len(batches) > totalTarget they sum to totalTarget but
// some entries are zero. In all cases no quota exceeds totalTarget, so the
// global target is never duplicated per batch.
func allocatePlanQuotas(batches []wikiExtract, totalTarget int) []int {
	if len(batches) == 0 {
		return nil
	}
	if len(batches) == 1 {
		return []int{totalTarget}
	}
	items := make([]int, len(batches))
	total := 0
	for i, b := range batches {
		items[i] = wikiExtractItemCount(b)
		total += items[i]
	}
	if total <= 0 {
		total = len(batches)
	}
	quotas := make([]int, len(batches))
	remaining := totalTarget
	for i := range batches {
		q := items[i] * totalTarget / total
		quotas[i] = q
		remaining -= q
	}
	// Largest-remainder: hand out the leftover pages to batches with the
	// largest fractional remainder, breaking ties by original index (stable
	// sort preserves first-seen order).
	type remItem struct {
		remainder int
		idx       int
	}
	rems := make([]remItem, len(batches))
	for i := range batches {
		rems[i] = remItem{remainder: items[i] * totalTarget % total, idx: i}
	}
	sort.SliceStable(rems, func(a, b int) bool {
		if rems[a].remainder == rems[b].remainder {
			return rems[a].idx < rems[b].idx
		}
		return rems[a].remainder > rems[b].remainder
	})
	for i := 0; i < len(rems) && remaining > 0; i++ {
		quotas[rems[i].idx]++
		remaining--
	}
	return quotas
}

// pageMentionCount estimates how strongly a planned page is grounded in the
// reduced extract by counting the distinct source chunks that mention its
// entities, concepts, or subject claims. It is used as the deterministic
// priority when pages must be dropped to fit the global hard cap.
func pageMentionCount(page wikiPlanPage, reduced wikiExtract) int {
	names := map[string]bool{}
	for _, n := range page.EntityNames {
		if k := normKey(n); k != "" {
			names[k] = true
		}
	}
	if len(names) == 0 {
		if t := normKey(page.Title); t != "" {
			names[t] = true
		}
		if topic := normKey(page.Topic); topic != "" {
			names[topic] = true
		}
	}
	chunks := map[string]bool{}
	for _, e := range reduced.Entities {
		if !names[normKey(e.Name)] {
			continue
		}
		for _, c := range e.SourceChunkIDs {
			chunks[c] = true
		}
	}
	for _, c := range reduced.Concepts {
		if !names[normKey(c.Term)] {
			continue
		}
		for _, cid := range c.SourceChunkIDs {
			chunks[cid] = true
		}
	}
	for _, c := range reduced.Claims {
		if !names[normKey(c.Subject)] {
			continue
		}
		for _, cid := range c.SourceChunkIDs {
			chunks[cid] = true
		}
	}
	return len(chunks)
}

// truncatePlanPagesByCap keeps at most maxPageCount planned pages, selecting by
// deterministic priority (mention count descending, then priority ascending,
// then slug), and returns the number of pages excluded by the cap. The output
// preserves the input order (original priority/slug order after normalize) so
// downstream slug-dedup and link normalization stay stable. It never fabricates
// a fallback page to fill the budget.
func truncatePlanPagesByCap(pages []wikiPlanPage, maxPageCount int, reduced wikiExtract) ([]wikiPlanPage, int) {
	if maxPageCount < 0 {
		maxPageCount = 0
	}
	if len(pages) <= maxPageCount {
		return pages, 0
	}
	type scored struct {
		idx int
		pg  wikiPlanPage
		mc  int
	}
	scoredPages := make([]scored, len(pages))
	for i, pg := range pages {
		scoredPages[i] = scored{idx: i, pg: pg, mc: pageMentionCount(pg, reduced)}
	}
	sort.SliceStable(scoredPages, func(a, b int) bool {
		if scoredPages[a].mc != scoredPages[b].mc {
			return scoredPages[a].mc > scoredPages[b].mc
		}
		if scoredPages[a].pg.Priority != scoredPages[b].pg.Priority {
			return scoredPages[a].pg.Priority < scoredPages[b].pg.Priority
		}
		return scoredPages[a].pg.Slug < scoredPages[b].pg.Slug
	})
	selected := scoredPages[:maxPageCount]
	// Restore original order by index so output order is deterministic.
	sort.SliceStable(selected, func(a, b int) bool { return selected[a].idx < selected[b].idx })
	out := make([]wikiPlanPage, 0, maxPageCount)
	for _, s := range selected {
		out = append(out, s.pg)
	}
	return out, len(pages) - maxPageCount
}
