package wiki

import (
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// This file implements the REDUCE-stage canonical-entity enhancement that
// narrows the gap with Python's wiki.py canonicalization:
//
//   - entities with distinct names but high embedding similarity are treated as
//     ambiguous and sent to an LLM merge decision (collapsing near-duplicates);
//   - concepts keep exact-term dedup, matching Python's current semantic (any
//     embedding/LLM dedup for concepts must be a separate new capability with
//     its own quality bar, not an alignment claim).
//
// The exact-key merge in reduceExtracts stays the deterministic baseline; this
// step layers embedding + LLM disambiguation on top. When the embedder or chat
// seam is unavailable, entities pass through unchanged (degrade gracefully).

// wikiEntityMergeThreshold is the embedding-cosine similarity at or above which
// two distinct-name entities are considered ambiguous and sent to the LLM merge
// decision. It is deliberately high so only genuinely similar candidates reach
// the LLM.
const wikiEntityMergeThreshold = 0.85

// wikiEntityMergeMaxCalls caps how many LLM disambiguation calls a single
// REDUCE run may make, bounding the cost on entity-dense documents.
const wikiEntityMergeMaxCalls = 16

// wikiEntityMergeMaxCandidates caps how many candidate partners one entity is
// checked against to keep the pairwise scan bounded.
const wikiEntityMergeMaxCandidates = 8

// dedupeEntities returns a copy of in with ambiguous near-duplicate entities
// collapsed via LLM disambiguation. It is a no-op when fewer than two entities
// are present or when deps.Embed / deps.Chat are unavailable.
func (p *wikiPipeline) dedupeEntities(in []wikiEntity) []wikiEntity {
	if len(in) < 2 || p.deps.Embed == nil || p.deps.Chat == nil {
		return in
	}
	names := make([]string, len(in))
	for i, e := range in {
		names[i] = e.Name
	}
	vecs, err := p.deps.Embed.Encode(p.ctx, names)
	if err != nil || len(vecs) != len(in) {
		return in
	}

	// Canonical entity per input index: which index owns the final entity.
	canon := make([]int, len(in))
	for i := range canon {
		canon[i] = i
	}
	llmCalls := 0

	// Greedy best-partner scan in input order (already deterministic after
	// reduceExtracts sorts by name).
	for i := 0; i < len(in) && llmCalls < wikiEntityMergeMaxCalls; i++ {
		if canon[i] != i {
			// Already merged into another canonical entity.
			continue
		}
		bestIdx, bestSim := -1, -1.0
		checked := 0
		for j := 0; j < len(in) && checked < wikiEntityMergeMaxCandidates; j++ {
			if i == j {
				continue
			}
			if canon[j] != j {
				// Consumed by an earlier merge; never a standalone partner.
				continue
			}
			if normKey(in[i].Name) == normKey(in[j].Name) {
				// Exact-name duplicates are already merged by reduceExtracts;
				// never treat them as a pair here.
				continue
			}
			// Same-type-only candidate filtering (Python canonicalizes entities
			// within the same type). Two entities with provably different types
			// are never ambiguous regardless of embedding similarity. An empty
			// type is treated as compatible (cannot prove a difference).
			if in[i].Type != "" && in[j].Type != "" && !strings.EqualFold(in[i].Type, in[j].Type) {
				continue
			}
			checked++
			sim := cosine32(vecs[i], vecs[j])
			if sim >= wikiEntityMergeThreshold && sim > bestSim {
				bestSim = sim
				bestIdx = j
			}
		}
		if bestIdx < 0 {
			continue
		}
		// i and bestIdx are guaranteed standalone by the loop guards above.
		// Count the call BEFORE issuing it so a persistent failure cannot drive
		// unbounded external requests: the llmCalls budget is consumed even when
		// the request fails. The outer loop's `llmCalls < max` guard then stops
		// further iterations once the budget is exhausted.
		llmCalls++
		merge, err := p.llmMergeEntityDecision(in[i], in[bestIdx])
		if err != nil {
			// A failed disambiguation call should not abort the whole REDUCE;
			// keep the entities separate and move on.
			continue
		}
		if !merge {
			continue
		}
		// Merge j into i: i is canonical, j is consumed.
		canon[bestIdx] = i
		in[i].Aliases = mergeStrings(in[i].Aliases, in[bestIdx].Aliases)
		if in[i].Name != in[bestIdx].Name {
			in[i].Aliases = mergeStrings(in[i].Aliases, []string{in[bestIdx].Name})
		}
		in[i].SourceChunkIDs = mergeStrings(in[i].SourceChunkIDs, in[bestIdx].SourceChunkIDs)
		if in[i].Type == "" {
			in[i].Type = in[bestIdx].Type
		}
	}

	out := make([]wikiEntity, 0, len(in))
	for i := range in {
		if canon[i] != i {
			continue
		}
		out = append(out, in[i])
	}
	return out
}

// llmMergeEntityDecision asks the chat seam whether two distinct-name entities
// refer to the same real-world concept. Returns true to merge.
func (p *wikiPipeline) llmMergeEntityDecision(a, b wikiEntity) (bool, error) {
	raw, err := common.GenJSON(p.ctx, p.deps.Chat, common.ChatRequest{
		LLMID:        p.llmID,
		SystemPrompt: wikiReduceEntityDisambiguateSystem,
		UserPrompt: renderWikiTemplate(wikiReduceEntityDisambiguateUserTemplate, map[string]string{
			"entity_a": mustPrettyJSON(a),
			"entity_b": mustPrettyJSON(b),
		}),
	})
	if err != nil {
		return false, err
	}
	v, ok := raw["merge"]
	if !ok {
		return false, nil
	}
	return toBoolValue(v), nil
}

// toBoolValue interprets a loosely-typed boolean field returned by the LLM (the
// model may emit JSON true or a string like "true"/"yes").
func toBoolValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "yes", "1", "same", "merge":
			return true
		}
	case float64:
		return x != 0
	}
	return false
}

// cosine32 computes the cosine similarity between two float32 vectors.
func cosine32(a, b []float32) float64 {
	na := l2Norm32(a)
	nb := l2Norm32(b)
	if na == 0 || nb == 0 {
		return 0
	}
	var dot float64
	for i := 0; i < len(a) && i < len(b); i++ {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot / (na * nb)
}
