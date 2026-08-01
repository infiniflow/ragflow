package structure

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/tokenizer"
)

// maxBatchTokenReserve is the share of the model's token budget kept in reserve
// for the system prompt, the batch instruction, and the model's JSON reply, so
// a single DecideBatch sub-call never overflows max_token.
const maxBatchTokenReserve = 0.15

// Merge prompts, verbatim from Python structure.py. The merge is driven by
// the user-supplied prompts; the decision instruction lets us branch on the
// LLM's verdict via GenJSON.
const mergeSystemPrompt = `You are an intelligent data merging assistant.
You will merge two JSON objects representing the same entity: Item A (existing) and Item B (incoming).

Merge strategy:
1. Combine information from both items.
2. If fields conflict, use your best judgment to pick the more detailed or recent-looking value.
3. If one item has a null/missing value and the other has data, keep the data.
4. For list fields, combine unique elements from both.
5. Do not invent new information not present in the inputs.
6. Return the result in the exact JSON format of the input items.`

const mergeUserPrompt = "Item A (existing):\n%s\n\nItem B (incoming):\n%s"

const mergeDecisionInstruction = `First decide whether Item A and Item B refer to the same logical entity (for entities) or the same logical relation (for relations). Use the merge strategy above only if they are the same.

Return ONLY a JSON object with this exact structure (no markdown fences, no commentary):
{
  "duplicated": <true | false>,
  "merged": <merged JSON object using the same keys as the inputs when duplicated=true; otherwise null>
}`

// batchMergeDecisionInstruction mirrors mergeDecisionInstruction but for many
// pairs at once: the model judges every pair in a single round-trip and returns
// an array (wrapped under "pairs") so the consumer can fold an entire batch of
// (existing, incoming) groups without one LLM call per pair.
const batchMergeDecisionInstruction = `You are judging several (Item A, Item B) pairs at once. For each pair decide whether they refer to the same logical entity (for entities) or the same logical relation (for relations), and merge them only if they are the same.

Return ONLY a JSON object with exactly one key "pairs", whose value is a JSON array with one element per pair, in the same order as listed. Each element MUST have this exact structure (no markdown fences, no commentary):
{
  "index": <the integer pair number from the input>,
  "duplicated": <true | false>,
  "merged": <merged JSON object using the same keys as the inputs when duplicated=true; otherwise null>
}`

// mergeJudgeTemperature mirrors Python's gen_conf for merge judging
// (_struct_merge_pair uses temperature 0.0).
var mergeJudgeTemperature = 0.0

// MergeDecision is the variant-level decision a MergeDecider returns for one
// incoming product against its best in-run match.
type MergeDecision int

const (
	// DecisionKeepBoth keeps the incoming product as a new entry.
	DecisionKeepBoth MergeDecision = iota
	// DecisionDropIncoming discards the incoming product (duplicate).
	DecisionDropIncoming
	// DecisionMerge replaces the existing entry with the returned product
	// (identity preserved via the existing id).
	DecisionMerge
)

// MergeDecider decides how to treat an incoming product given its best
// in-run cosine match. When it returns DecisionMerge it also returns the
// merged replacement product; for other decisions the product is ignored.
type MergeDecider interface {
	Decide(ctx context.Context, existing, incoming common.Product, bestScore float64) (MergeDecision, common.Product, error)
}

// CosineDecider drops the incoming product when its best cosine match meets
// (>=) Threshold. It performs no LLM call, so it is safe under the store lock.
type CosineDecider struct{ Threshold float64 }

func (d CosineDecider) Decide(_ context.Context, _, _ common.Product, bestScore float64) (MergeDecision, common.Product, error) {
	if bestScore >= d.Threshold {
		return DecisionDropIncoming, common.Product{}, nil
	}
	return DecisionKeepBoth, common.Product{}, nil
}

// LLMMergeDecider mirrors Python's local-dedup judge (_struct_merge_pair):
// pairs above Threshold are adjudicated by the LLM; on a duplicated verdict
// the merged payload replaces the existing row (re-embedded, provenance
// unioned, id preserved). Entity merges record aliases so relation endpoints
// can be rewritten afterwards (mirrors _struct_local_dedup's entity_aliases).
type LLMMergeDecider struct {
	Chat      common.ChatInvoker
	LLMID     string
	Embed     common.Embedder
	Threshold float64

	// maxBatchTokens caps the estimated prompt size of a single DecideBatch
	// sub-call. When 0 (the default) the whole batch is sent in one call,
	// preserving the historic behavior. When positive the batch is split into
	// token-bounded sub-calls so a large candidate set can never overflow the
	// model's max_token window.
	maxBatchTokens int

	// submit runs one LLM sub-batch off the shared knowledge-compilation pool.
	// When nil each sub-batch runs inline (sequentially). It is injected by the
	// knowledge_compile package so every stage shares one process-wide,
	// vCPU-sized concurrency bound.
	submit func(ctx context.Context, fn func() error) error

	mu      sync.Mutex
	aliases map[string]string
}

// NewLLMMergeDecider constructs the decider used by the structure variant.
func NewLLMMergeDecider(chat common.ChatInvoker, llmID string, embed common.Embedder, threshold float64) *LLMMergeDecider {
	return &LLMMergeDecider{Chat: chat, LLMID: llmID, Embed: embed, Threshold: threshold, aliases: map[string]string{}}
}

// SetMaxBatchTokens caps the estimated prompt size of a single DecideBatch
// sub-call. Non-positive values disable per-call batching. The effective budget
// is modelMaxTokens*(1-maxBatchTokenReserve), keeping headroom for the system
// prompt and the JSON reply.
func (d *LLMMergeDecider) SetMaxBatchTokens(modelMaxTokens int) {
	if modelMaxTokens <= 0 {
		d.maxBatchTokens = 0
		return
	}
	budget := int(float64(modelMaxTokens) * (1 - maxBatchTokenReserve))
	if budget <= 0 {
		d.maxBatchTokens = 0
		return
	}
	d.maxBatchTokens = budget
}

// SetSubmitter injects the shared knowledge-compilation pool so DecideBatch can
// run its token-bounded sub-batches concurrently. A nil submitter (the default)
// falls back to running sub-batches sequentially. The submitter must run fn on
// a bounded pool and return its error.
func (d *LLMMergeDecider) SetSubmitter(submit func(ctx context.Context, fn func() error) error) {
	d.submit = submit
}

// Decide implements MergeDecider.
func (d *LLMMergeDecider) Decide(ctx context.Context, existing, incoming common.Product, bestScore float64) (MergeDecision, common.Product, error) {
	if bestScore < d.Threshold {
		return DecisionKeepBoth, common.Product{}, nil
	}
	merged, err := mergePair(ctx, d.Chat, d.LLMID, existing.Content, incoming.Content)
	if err != nil {
		return DecisionKeepBoth, common.Product{}, err
	}
	if merged == nil {
		return DecisionKeepBoth, common.Product{}, nil
	}
	replacement, err := d.BuildReplacement(ctx, existing, incoming, merged)
	if err != nil {
		return DecisionKeepBoth, common.Product{}, err
	}
	return DecisionMerge, replacement, nil
}

// buildReplacement folds an LLM-merged payload back into a replacement Product
// for the existing row: aliases are recorded (entities), merge invariants are
// applied (relations), provenance is unioned, and the payload is re-embedded.
// Shared by Decide (single pair) and the batched judge so both paths produce
// identical merged rows.
func (d *LLMMergeDecider) BuildReplacement(ctx context.Context, existing, incoming common.Product, merged map[string]any) (common.Product, error) {
	kind, _ := existing.Meta["kind"].(string)
	if kind == "entity" {
		oldName := entityNameValue(existing)
		incomingName := entityNameValue(incoming)
		canonical := entityName(merged)
		if canonical == "" {
			canonical = oldName
		}
		for _, alias := range []string{oldName, incomingName} {
			if alias != "" && alias != canonical {
				d.recordAlias(alias, canonical)
			}
		}
	}
	merged = applyMergeInvariants(existing, merged)

	chunkIDs := unionOrdered(metaStrings(existing.Meta, "source_chunk_ids"), metaStrings(incoming.Meta, "source_chunk_ids"))
	texts := []string{payloadDescription(merged)}
	vecs, err := d.Embed.Encode(ctx, texts)
	if err != nil {
		return common.Product{}, err
	}
	if len(vecs) == 0 {
		return common.Product{}, fmt.Errorf("knowledge_compiler: re-embed of merged payload returned no vector")
	}

	meta := map[string]any{}
	for k, v := range existing.Meta {
		meta[k] = v
	}
	meta["source_chunk_ids"] = chunkIDs
	refreshMetaFromPayload(meta, kind, merged)
	return common.Product{
		ID:       existing.ID,
		DocID:    existing.DocID,
		TenantID: existing.TenantID,
		Variant:  existing.Variant,
		Content:  payloadJSON(merged),
		Vector:   vecs[0],
		Meta:     meta,
	}, nil
}

// MergePairInput is one (existing, incoming) pair fed to the batched judge.
type MergePairInput struct {
	Index    int
	Existing string
	Incoming string
}

// BatchMergeResult is the judge's verdict for one pair.
type BatchMergeResult struct {
	Index      int
	Duplicated bool
	Merged     map[string]any
}

// DecideBatch judges every pair and returns the verdicts in input order. It is
// the batched counterpart of Decide's single-pair judge. When a token budget is
// configured it splits the pairs into token-bounded sub-batches (each sub-batch
// still judged in a single LLM call, preserving pair order) so a large candidate
// set can never overflow the model's max_token window.
func (d *LLMMergeDecider) DecideBatch(ctx context.Context, pairs []MergePairInput) ([]BatchMergeResult, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	if d.maxBatchTokens <= 0 {
		return mergePairsBatch(ctx, d.Chat, d.LLMID, pairs)
	}
	chunks := splitByTokens(pairs, d.maxBatchTokens)
	// The merge decisions are LLM-bounded, not CPU-bounded: run the token-bounded
	// sub-batches concurrently on the injected shared pool (vCPU-sized) when more
	// than one chunk exists. Order is preserved by writing each chunk's verdicts
	// into its own slot (the model echoes the global pair index, so callers still
	// key by input index regardless of execution order).
	if len(chunks) == 1 || d.submit == nil {
		out := make([]BatchMergeResult, 0, len(pairs))
		for _, chunk := range chunks {
			sub, err := mergePairsBatch(ctx, d.Chat, d.LLMID, chunk)
			if err != nil {
				return nil, err
			}
			out = append(out, sub...)
		}
		return out, nil
	}
	results := make([][]BatchMergeResult, len(chunks))
	var (
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)
	for i, chunk := range chunks {
		i, chunk := i, chunk
		wg.Add(1)
		err := d.submit(ctx, func() error {
			defer wg.Done()
			sub, err := mergePairsBatch(ctx, d.Chat, d.LLMID, chunk)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return err
			}
			results[i] = sub
			return nil
		})
		if err != nil {
			// The job was never enqueued, so the closure's wg.Done() will never
			// run — decrement here and record the failure so we don't deadlock
			// on wg.Wait() and don't silently drop the submit error.
			wg.Done()
			errOnce.Do(func() { firstErr = err })
		}
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	out := make([]BatchMergeResult, 0, len(pairs))
	for _, sub := range results {
		out = append(out, sub...)
	}
	return out, nil
}

// splitByTokens groups pairs into contiguous chunks whose estimated prompt
// size stays within budget. The original (global) Index of every pair is
// preserved — the model echoes it back — so callers can still key verdicts by
// the input index. A single pair that alone exceeds budget is sent on its own
// (the model may still truncate, but we never silently drop it).
func splitByTokens(pairs []MergePairInput, budget int) [][]MergePairInput {
	var chunks [][]MergePairInput
	cur := make([]MergePairInput, 0, len(pairs))
	used := 0
	for _, p := range pairs {
		// The pair contents dominate the prompt size; the "Pair N:" wrappers
		// are a negligible constant overhead per pair, ignored here.
		est := tokenizer.NumTokensFromString(p.Existing) + tokenizer.NumTokensFromString(p.Incoming)
		if len(cur) > 0 && used+est > budget {
			chunks = append(chunks, cur)
			// Fresh backing array: cur[:0] shares the buffer with the chunk we
			// just appended, so the next iteration would overwrite it.
			cur = make([]MergePairInput, 0, len(pairs))
			used = 0
		}
		cur = append(cur, p)
		used += est
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// mergePairsBatch mirrors mergePair but judges every pair in one LLM call and
// returns the verdicts in input order. Pairs whose inputs are unparseable or
// whose verdict is missing are reported as not-duplicated (mirrors the
// per-pair skip behavior: Python logs and keeps both).
func mergePairsBatch(ctx context.Context, chat common.ChatInvoker, llmID string, pairs []MergePairInput) ([]BatchMergeResult, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	var b strings.Builder
	for _, p := range pairs {
		b.WriteString(fmt.Sprintf("Pair %d:\nItem A (existing):\n%s\n\nItem B (incoming):\n%s\n\n", p.Index, p.Existing, p.Incoming))
	}
	res, err := common.GenJSON(ctx, chat, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: mergeSystemPrompt + "\n\n" + batchMergeDecisionInstruction,
		UserPrompt:   b.String(),
		Temperature:  &mergeJudgeTemperature,
	})
	if err != nil {
		return nil, err
	}
	arr, ok := res["pairs"].([]any)
	if !ok {
		// Model returned no array: treat every pair as not duplicated rather
		// than failing the whole batch.
		out := make([]BatchMergeResult, len(pairs))
		for i, p := range pairs {
			out[i] = BatchMergeResult{Index: p.Index}
		}
		return out, nil
	}
	byIndex := make(map[int]BatchMergeResult, len(arr))
	for _, el := range arr {
		obj, ok := el.(map[string]any)
		if !ok {
			continue
		}
		idx, _ := obj["index"].(float64)
		dup, _ := obj["duplicated"].(bool)
		var merged map[string]any
		if m, ok := obj["merged"].(map[string]any); ok {
			merged = m
		}
		byIndex[int(idx)] = BatchMergeResult{Index: int(idx), Duplicated: dup, Merged: merged}
	}
	out := make([]BatchMergeResult, len(pairs))
	for i, p := range pairs {
		if r, ok := byIndex[p.Index]; ok {
			out[i] = r
		} else {
			out[i] = BatchMergeResult{Index: p.Index}
		}
	}
	return out, nil
}

// recordAlias adds one alias→canonical mapping (thread-safe).
func (d *LLMMergeDecider) recordAlias(alias, canonical string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.aliases[alias] = canonical
}

// Aliases returns the accumulated alias→canonical map (a copy).
func (d *LLMMergeDecider) Aliases() map[string]string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]string, len(d.aliases))
	for k, v := range d.aliases {
		out[k] = v
	}
	return out
}

// mergePair mirrors _struct_merge_pair: LLM-judged merge of two payload JSON
// documents. It returns the merged payload when the pair is a duplicate, nil
// when not (including unparseable inputs, which Python logs and skips).
func mergePair(ctx context.Context, chat common.ChatInvoker, llmID, existingContent, incomingContent string) (map[string]any, error) {
	existingPayload := parsePayload(existingContent)
	incomingPayload := parsePayload(incomingContent)
	if existingPayload == nil || incomingPayload == nil {
		return nil, nil
	}
	res, err := common.GenJSON(ctx, chat, common.ChatRequest{
		LLMID:        llmID,
		SystemPrompt: mergeSystemPrompt + "\n\n" + mergeDecisionInstruction,
		UserPrompt:   fmt.Sprintf(mergeUserPrompt, payloadJSON(existingPayload), payloadJSON(incomingPayload)),
		Temperature:  &mergeJudgeTemperature,
	})
	if err != nil {
		return nil, err
	}
	if dup, _ := res["duplicated"].(bool); !dup {
		return nil, nil
	}
	merged, ok := res["merged"].(map[string]any)
	if !ok {
		return nil, nil
	}
	return merged, nil
}

// applyMergeInvariants mirrors _struct_apply_merge_invariants: for relations,
// force the source/target fields back to the existing payload's values —
// from_entity_kwd / to_entity_kwd must not change across a merge.
func applyMergeInvariants(existing common.Product, mergedPayload map[string]any) map[string]any {
	kind, _ := existing.Meta["kind"].(string)
	if kind != "relation" {
		return mergedPayload
	}
	existingPayload := parsePayload(existing.Content)
	if existingPayload == nil {
		return mergedPayload
	}
	for _, field := range []string{"source", "src", "from"} {
		if v, ok := existingPayload[field]; ok {
			mergedPayload[field] = v
		}
	}
	for _, field := range []string{"target", "tgt", "to"} {
		if v, ok := existingPayload[field]; ok {
			mergedPayload[field] = v
		}
	}
	return mergedPayload
}

// resolveAlias mirrors _struct_resolve_entity_alias: follow the alias chain
// until a non-aliased name, tolerating cycles (stop at the first repeat).
func resolveAlias(name string, aliases map[string]string) string {
	current := strings.TrimSpace(name)
	seen := map[string]bool{}
	for {
		next, ok := aliases[current]
		if !ok || seen[current] {
			return current
		}
		seen[current] = true
		current = next
	}
}

// rewriteRelationPayload mirrors _struct_rewrite_relation_payload: resolve
// the relation's endpoint fields through the alias map in place. Returns
// whether anything changed.
func rewriteRelationPayload(payload map[string]any, aliases map[string]string) bool {
	changed := false
	for _, fields := range [][]string{{"source", "src", "from"}, {"target", "tgt", "to"}} {
		for _, field := range fields {
			v, ok := payload[field]
			if !ok || v == nil {
				continue
			}
			old := strings.TrimSpace(stringOf(v))
			if old == "" {
				continue
			}
			if newName := resolveAlias(old, aliases); newName != old {
				payload[field] = newName
				changed = true
			}
		}
	}
	return changed
}

// refreshMetaFromPayload re-derives the name/type/endpoint meta keys of a
// merged row from its new payload (mirrors the column re-derivation in
// _struct_rebuild_doc_storage_doc).
func refreshMetaFromPayload(meta map[string]any, kind string, payload map[string]any) {
	if kind == "entity" {
		if name := entityName(payload); name != "" {
			meta["name"] = name
		} else {
			delete(meta, "name")
		}
		if typ := strings.TrimSpace(stringOf(payload["type"])); typ != "" {
			meta["entity_type"] = typ
		}
		if desc := strings.TrimSpace(stringOf(payload["description"])); desc != "" {
			meta["description"] = desc
		}
		return
	}
	if from := relationEndpoint(payload, "", "source", "src", "from"); from != "" {
		meta["from"] = from
	}
	if to := relationEndpoint(payload, "", "target", "tgt", "to"); to != "" {
		meta["to"] = to
	}
	if typ := strings.TrimSpace(stringOf(payload["type"])); typ != "" {
		meta["relation_type"] = typ
	}
}

// entityNameValue reads an entity row's canonical name (payload first, then
// the meta stamp), mirroring _struct_entity_name.
func entityNameValue(p common.Product) string {
	if payload := parsePayload(p.Content); payload != nil {
		if name := entityName(payload); name != "" {
			return name
		}
	}
	name, _ := p.Meta["name"].(string)
	return strings.TrimSpace(name)
}

// metaStrings reads a []string meta value (tolerating []any).
func metaStrings(meta map[string]any, key string) []string {
	switch v := meta[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// unionOrdered mirrors _common.union_ordered: concatenated, deduped,
// first-seen order preserved.
func unionOrdered(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, lst := range lists {
		for _, v := range lst {
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// MergeStats accumulates dedup outcomes across a run.
type MergeStats struct {
	Inserted          int
	Updated           int
	DuplicatesDropped int
}

// MergeIntoStore folds the incoming rows into store, using DedupeAdd so the
// check-then-act is atomic and the decider (which may call the LLM) runs
// outside the store lock. It returns counts of inserted/merged/dropped rows.
//
// When a row is dropped as a duplicate, its source_chunk_ids are still merged
// into the surviving entry's source_chunk_ids (order-preserving union), so an
// entity that appears across multiple chunks accumulates provenance from all
// of them — mirroring Python's _struct_merge_graph_entities.
func MergeIntoStore(ctx context.Context, store *common.MemStore, decider MergeDecider, rows []common.Product) (MergeStats, error) {
	var stats MergeStats
	for _, row := range rows {
		var droppedExisting common.Product
		var dropped bool
		action, err := store.DedupeAdd(row, 0, func(existing common.Product, score float64) (common.KeepAction, common.Product, error) {
			d, replacement, e := decider.Decide(ctx, existing, row, score)
			if e != nil {
				return common.KeepAdd, common.Product{}, e
			}
			switch d {
			case DecisionDropIncoming:
				// Capture the existing entry so we can fold the incoming
				// source_chunk_ids into it after the drop (provenance union,
				// mirroring Python's _struct_merge_graph_entities).
				droppedExisting = existing
				dropped = true
				return common.KeepDrop, common.Product{}, nil
			case DecisionMerge:
				return common.KeepMerge, replacement, nil
			default:
				return common.KeepAdd, common.Product{}, nil
			}
		})
		if err != nil {
			return stats, err
		}
		switch action {
		case common.KeepDrop:
			stats.DuplicatesDropped++
			// Fold the incoming source_chunk_ids into the surviving entry.
			if dropped && droppedExisting.ID != "" {
				if ids := metaStrings(row.Meta, "source_chunk_ids"); len(ids) > 0 {
					store.MergeSourceChunkIDs(droppedExisting.ID, ids)
				}
			}
		case common.KeepMerge:
			stats.Updated++
			stats.DuplicatesDropped++
		default:
			stats.Inserted++
		}
	}
	return stats, nil
}

// GroupedDeduper mirrors _struct_local_dedup's group-by-filter-key behaviour:
// rows are bucketed by their relation endpoints (entities share one bucket,
// mirroring _struct_filter_key where entity rows carry no from/to), so an
// entity never merges with a relation and relations only merge with
// same-endpoint relations. One MemStore per bucket keeps cosine candidates
// group-scoped. doc/compile/template are constant per run and therefore not
// part of the key (they are in Python's key for the cross-document ES case).
type GroupedDeduper struct {
	decider MergeDecider

	mu     sync.Mutex
	order  []string
	stores map[string]*common.MemStore
	stats  MergeStats
}

// NewGroupedDeduper constructs a deduper around the given decider.
func NewGroupedDeduper(decider MergeDecider) *GroupedDeduper {
	return &GroupedDeduper{decider: decider, stores: map[string]*common.MemStore{}}
}

// groupKey mirrors _struct_filter_key minus the per-run constants.
func groupKey(row common.Product) string {
	from, _ := row.Meta["from"].(string)
	to, _ := row.Meta["to"].(string)
	return from + "\x00" + to
}

// Add folds one row into its endpoint bucket. Safe for concurrent use, but
// callers that need deterministic merge outcomes (mirroring Python's
// sequential _struct_local_dedup) should call it sequentially in batch order.
func (g *GroupedDeduper) Add(ctx context.Context, row common.Product) error {
	key := groupKey(row)
	g.mu.Lock()
	store, ok := g.stores[key]
	if !ok {
		store = common.NewMemStore()
		g.stores[key] = store
		g.order = append(g.order, key)
	}
	g.mu.Unlock()

	s, err := MergeIntoStore(ctx, store, g.decider, []common.Product{row})
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.stats.Inserted += s.Inserted
	g.stats.Updated += s.Updated
	g.stats.DuplicatesDropped += s.DuplicatesDropped
	g.mu.Unlock()
	return nil
}

// Stats returns the accumulated dedup counters.
func (g *GroupedDeduper) Stats() MergeStats {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stats
}

// Rows concatenates every bucket's surviving rows in first-seen bucket order.
func (g *GroupedDeduper) Rows() []common.Product {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []common.Product
	for _, key := range g.order {
		out = append(out, g.stores[key].Snapshot()...)
	}
	return out
}

// RewriteRelations mirrors the alias-rewrite tail of _struct_local_dedup:
// when entity merges recorded aliases, every relation row's endpoints are
// resolved through them (re-embedding the changed payloads, preserving row
// ids), then ALL relation rows are re-folded through the deduper so relations
// that became identical collapse (Python's second pass with
// rewrite_relations=False). It is a no-op without aliases.
func (g *GroupedDeduper) RewriteRelations(ctx context.Context, aliases map[string]string, embed common.Embedder) error {
	if len(aliases) == 0 {
		return nil
	}

	// Detach every relation row from its bucket (entity rows stay put).
	g.mu.Lock()
	var relations []common.Product
	keptOrder := make([]string, 0, len(g.order))
	for _, key := range g.order {
		store := g.stores[key]
		var remaining []common.Product
		for _, row := range store.Snapshot() {
			if kind, _ := row.Meta["kind"].(string); kind == "relation" {
				relations = append(relations, row)
			} else {
				remaining = append(remaining, row)
			}
		}
		if len(remaining) == len(store.Snapshot()) {
			keptOrder = append(keptOrder, key)
			continue
		}
		delete(g.stores, key)
		if len(remaining) > 0 {
			fresh := common.NewMemStore()
			for _, row := range remaining {
				fresh.Add(row)
			}
			g.stores[key] = fresh
			keptOrder = append(keptOrder, key)
		}
	}
	g.order = keptOrder
	g.mu.Unlock()

	if len(relations) == 0 {
		return nil
	}

	// Rewrite endpoints; re-embed only the rows whose payload changed
	// (mirrors _struct_rewrite_relation_doc). Row ids are preserved.
	var changedIdx []int
	for i, row := range relations {
		payload := parsePayload(row.Content)
		if payload == nil {
			continue
		}
		if !rewriteRelationPayload(payload, aliases) {
			continue
		}
		row.Content = payloadJSON(payload)
		if row.Meta == nil {
			row.Meta = map[string]any{}
		}
		if from := relationEndpoint(payload, "", "source", "src", "from"); from != "" {
			row.Meta["from"] = from
		}
		if to := relationEndpoint(payload, "", "target", "tgt", "to"); to != "" {
			row.Meta["to"] = to
		}
		relations[i] = row
		changedIdx = append(changedIdx, i)
	}
	if len(changedIdx) > 0 {
		texts := make([]string, len(changedIdx))
		for i, idx := range changedIdx {
			texts[i] = payloadDescription(parsePayload(relations[idx].Content))
		}
		vecs, err := embed.Encode(ctx, texts)
		if err != nil {
			return err
		}
		if len(vecs) != len(changedIdx) {
			return fmt.Errorf("knowledge_compiler: re-embed after relation rewrite returned %d vectors for %d rows", len(vecs), len(changedIdx))
		}
		for i, idx := range changedIdx {
			relations[idx].Vector = vecs[i]
		}
	}

	// Second dedup pass over the rewritten relation set.
	for _, row := range relations {
		if err := g.Add(ctx, row); err != nil {
			return err
		}
	}
	return nil
}
