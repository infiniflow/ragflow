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
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/rag/agentic-rag/runtime"
)

// The opening DECOMPOSITION is gone from this file. It used to be a stage of its own: one chat
// call asking for 2-5 "fan-out" sub-questions, a shape guard that rejected lines looking like
// answers rather than queries (fanoutLooksLikeQuery), and a parser that split a prose reply into
// lines. Its output then became the slot table's `fanout_hint` — while the slot-table call asked
// the same model, on the same question, for the very queries that stage had just produced.
//
// Two consequences, both measured on 2026-09-20: the planner phase cost TWO model calls per
// question (the second-biggest output-token consumer in the run), and the shape guard is the same
// class of rule the rest of this design removes — a line is not a query because of how it looks.
//
// The paper's opening move is ONE call (`first_move`: decompose → the queries to run), and the
// slot table's own call already produces those queries (`first_queries`). See plannerNode.

// Metadata pre-filter channel: the metadata conditions a sub-question's wording supports.
//
// Channel C exists because a sub-question that NAMES a document is better served by
// pre-filtering the document set by metadata and searching inside it than by another
// whole-corpus round: the whole-corpus round has to rank the named document against every
// other one, and on a large corpus it loses. The conditions come from ONE chat call for the
// whole fan-out batch, and every value must look like a SHORT NAME — a copied sub-question or
// an answer would make the condition match nothing.

// fanoutFilterPrompt is the ONE chat call the pre-search metadata channel makes per batch: it
// turns each named thing a sub-question carries into a metadata condition over the dataset's
// OWN fields. The vocabulary is injected under AVAILABLE METADATA (see MetadataCatalog.Render),
// so the channel is not pinned to any single field: a dataset whose content fields are called
// `title` / `file_name` / `topic` is filtered on whichever of them the sub-question names.
const fanoutFilterPrompt = `For each numbered search sub-question below, decide whether it NAMES something a ` +
	`document's metadata would carry — a document title or file name, an author, an organisation, a place, ` +
	`an event — and turn each named thing into ONE metadata condition {key, op, value} over the fields ` +
	`listed under AVAILABLE METADATA. ` +
	`Use the field whose meaning fits the named thing; prefer "contains" unless the whole value is named. ` +
	`The value MUST be copied EXACTLY as it appears — keep underscores, punctuation and file extensions ` +
	`as written, never re-normalise them (a stored file name keeps its underscores; a stored title keeps ` +
	`its spaces) — and when the sub-question's wording matches one of the listed values, use that listed ` +
	`value verbatim. Keep each value under 10 words: never copy the whole sub-question, never answer it. ` +
	`Emit ONLY conditions you are confident about: a sub-question that names no such thing gets an EMPTY ` +
	`list — do not invent a condition just to fill one. ` +
	`Respond with JSON only: {"filters": [[{"key":"...","op":"...","value":"..."}], ...]} — one ` +
	`condition-array per sub-question, in the SAME ORDER as the sub-questions, JSON only, no prose.`

const (
	// metadataValueMaxWords / metadataValueMaxChars are the guard on a filter value taken from a
	// sub-question: a long string is a copied sub-question or an answer.
	metadataValueMaxWords = 10
	metadataValueMaxChars = 80
	// metadataMaxConditions caps the conditions taken from one sub-question.
	metadataMaxConditions = 3
	// fanoutMetadataQuota is the metadata channel's per-query quota. Modest like the
	// semantic quota: the channel's hits are only as good as the extraction.
	fanoutMetadataQuota = 4
)

// fanoutMetadataOps are the operators the pre-search channel may use: the POSITIVE
// value-matching subset of the tool's operators. The channel's contract is "the sub-question
// NAMED something, so look inside the documents whose metadata matches it"; `not contains` /
// `empty` / `not empty` are deliberately absent because they WIDEN a document set — the
// opposite of what a pre-filter is for, and a model reaching for one would scope the channel's
// search to everything it was meant to narrow.
var fanoutMetadataOps = map[string]bool{
	"=":          true,
	"contains":   true,
	"start with": true,
	"end with":   true,
	"in":         true,
}

// fanoutMetadataVocabulary returns the field vocabulary the pre-search channel may use, and
// whether the channel can run at all.
//
// The vocabulary IS the session's metadata catalog — the same "declared ∪ observed − blacklist"
// set the metadata_search tool advertises. There is no field name baked in anywhere: a caller
// that wired no toolset, or a session whose catalog is empty because the dataset carries no
// metadata, has no vocabulary to offer and the channel is skipped. (It would otherwise pay the
// extraction call and then run one guaranteed-empty scoped retrieval per sub-question.)
//
// The vocabulary is the catalog's FULL rendering, value samples included. That is deliberate:
// this channel has no feedback loop — a condition that matches nothing is a silent miss, not a
// retry — so the model must be able to pick the exact form the index stores. A guessed value
// that differs from the stored one by case, punctuation or an underscore normalisation is the
// observed failure (measured 2026-09-20: `file_name contains "02 Endpoint Isolation ..."` found
// nothing while the stored value kept its underscores).
func fanoutMetadataVocabulary(deps RAGTools) (string, map[string]bool, bool) {
	if deps.Tools == nil {
		return "", nil, false
	}
	cat := deps.Tools.MetadataFields
	if cat == nil || cat.Empty() {
		return "", nil, false
	}
	allowed := make(map[string]bool, len(cat.Keys))
	for _, k := range cat.Keys {
		allowed[k] = true
	}
	return cat.Render(), allowed, true
}

// fanoutValueLooksUsable reports whether a filter value is a short name rather than a copied
// sub-question or an answer.
func fanoutValueLooksUsable(value string) bool {
	s := strings.TrimSpace(value)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	for _, mark := range fanoutAnswerMarks {
		if strings.Contains(low, mark) {
			return false
		}
	}
	// The character cap counts code points: a byte-based cap would reject a legitimate CJK
	// value well below the 80-character limit.
	return utf8.RuneCountInString(s) <= metadataValueMaxChars && len(strings.Fields(s)) <= metadataValueMaxWords
}

// fanoutCondition validates one model-written condition against the dataset's offered fields
// and the pre-search operators, returning nil when it must be dropped.
//
// The value is coerced by runtime.NormalizeMetadataValue — the same function the
// metadata_search tool uses — so a condition means the same thing on either path.
func fanoutCondition(raw any, allowed map[string]bool) map[string]any {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	key := strings.TrimSpace(fmt.Sprint(m["key"]))
	op := strings.TrimSpace(fmt.Sprint(m["op"]))
	if !allowed[key] || !fanoutMetadataOps[op] {
		return nil
	}
	value := runtime.NormalizeMetadataValue(m["value"], op)
	if op == "in" {
		list, ok := value.([]any)
		if !ok || len(list) == 0 {
			return nil
		}
		for _, item := range list {
			if !fanoutValueLooksUsable(fmt.Sprint(item)) {
				return nil
			}
		}
		return map[string]any{"key": key, "op": op, "value": value}
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if value == nil || text == "" || text == "<nil>" || !fanoutValueLooksUsable(text) {
		return nil
	}
	return map[string]any{"key": key, "op": op, "value": text}
}

// parseFanoutFilters extracts up to metadataMaxConditions conditions per sub-question from a
// model reply, order-aligned with fanouts.
//
// Accepts {"filters": [[{...}, ...], ...]} (preferred), a flat {"filters": [{...}, ...]} (read
// positionally: one condition per sub-question), or
// {"filters": [{"sub_question": ..., "filters": [...]}]} matched by text. Conditions are
// validated by fanoutCondition: an unknown field, a non-pre-search operator, an over-long value
// or a duplicate is dropped. Sub-questions left with no condition simply skip the channel.
func parseFanoutFilters(text string, fanouts []string, allowed map[string]bool) [][]map[string]any {
	out := make([][]map[string]any, len(fanouts))
	data, ok := extractJSONObject(text).(map[string]any)
	if !ok {
		return out
	}
	items := asSliceOfAny(data["filters"])
	if len(items) == 0 {
		return out
	}
	byText := map[string][]map[string]any{}
	var positional [][]map[string]any
	for _, it := range items {
		switch v := it.(type) {
		case map[string]any:
			// A sub-question key means the object groups its own conditions; otherwise the
			// object IS one condition.
			if _, grouped := v["sub_question"]; grouped || v["fanout"] != nil || v["filters"] != nil {
				conds := fanoutConditions(asSliceOfAny(v["filters"]), allowed)
				sq := ""
				if sv := v["sub_question"]; sv != nil {
					sq = strings.TrimSpace(fmt.Sprint(sv))
				}
				if sq == "" && v["fanout"] != nil {
					sq = strings.TrimSpace(fmt.Sprint(v["fanout"]))
				}
				if sq != "" {
					byText[sq] = conds
				}
				positional = append(positional, conds)
				continue
			}
			positional = append(positional, fanoutConditions([]any{v}, allowed))
		case []any:
			positional = append(positional, fanoutConditions(v, allowed))
		}
	}
	for i, fq := range fanouts {
		var raw []map[string]any
		if len(byText) > 0 {
			raw = byText[fq]
		} else if i < len(positional) {
			raw = positional[i]
		}
		seen := make(map[string]bool, len(raw))
		cleaned := make([]map[string]any, 0, metadataMaxConditions)
		for _, c := range raw {
			fingerprint := fmt.Sprint(c["key"], c["op"], c["value"])
			if seen[fingerprint] {
				continue
			}
			seen[fingerprint] = true
			cleaned = append(cleaned, c)
			if len(cleaned) >= metadataMaxConditions {
				break
			}
		}
		out[i] = cleaned
	}
	return out
}

// fanoutConditions validates a raw condition list, dropping everything fanoutCondition rejects.
func fanoutConditions(raw []any, allowed map[string]bool) []map[string]any {
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if c := fanoutCondition(item, allowed); c != nil {
			out = append(out, c)
		}
	}
	return out
}

// ExtractFanoutFilters maps each sub-question to the metadata conditions its wording supports:
// ONE chat call for the whole batch.
//
// The conditions name fields from the session's catalog, so the channel filters on the dataset's
// real fields instead of a hard-coded one. Only sub-questions with at least one valid condition
// are returned, and any failure — no model, no filterable field, a model error, an unparseable
// reply — yields nil so the metadata channel is simply skipped for this round; pre-search is
// never blocked by it.
func ExtractFanoutFilters(ctx context.Context, deps RAGTools, fanouts []string) map[string][]map[string]any {
	queries := make([]string, 0, len(fanouts))
	for _, f := range fanouts {
		if q := strings.TrimSpace(f); q != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 || deps.Model == nil {
		return nil
	}
	fields, allowed, ok := fanoutMetadataVocabulary(deps)
	if !ok {
		return nil
	}
	listed := make([]string, 0, len(queries))
	for i, q := range queries {
		listed = append(listed, fmt.Sprintf("%d. %s", i+1, q))
	}
	reply, err := deps.Model.Complete(ctx, []schema.Message{
		*schema.SystemMessage(fanoutFilterPrompt + "\n\nAVAILABLE FIELDS:\n" + fields),
		*schema.UserMessage("Sub-questions:\n" + strings.Join(listed, "\n")),
	}, nil)
	if err != nil {
		common.Warn("prefetch: metadata filter extraction failed, skipping the metadata channel", zap.Error(err))
		return nil
	}
	filterSets := parseFanoutFilters(reply.Content, queries, allowed)
	out := make(map[string][]map[string]any, len(queries))
	for i, q := range queries {
		if len(filterSets[i]) > 0 {
			out[q] = filterSets[i]
		}
	}
	if len(out) == 0 {
		// Nothing survived the guards: report "no conditions" as nil, the same shape every
		// other failure of this best-effort call returns, so the caller needs no special case.
		//
		// The reply is logged here because this is the channel's SILENT outcome — a condition
		// rejected by the guards contributes nothing and produces no other trace, so without the
		// raw reply a miss is indistinguishable from "the model named nothing". (Empty
		// condition-lists are the COMMON, correct answer to a question that names no metadata
		// value; a non-empty reply here means the guards rejected something.) Newlines are
		// flattened so the reply stays one log line.
		flat := strings.ReplaceAll(runtime.TruncateRunes(reply.Content, 300), "\n", " ")
		common.Warn("prefetch: metadata channel produced no usable condition", zap.String("reply", flat))
		return nil
	}
	common.Info("prefetch: metadata filters", zap.Any("filters", out))
	return out
}

// Phase 2: programmatic fan-out search.

// FanoutSearch: the
// programmatic multi-query prefetch that seeds the snippet pool.
//
// It is a three-channel search, and the channels are deliberately different:
//
//   - channel A (exact): a keyword BM25 round over a wide pool (top_n=60) whose
//     hits are then narrowed by the query's own terms — the precision path. A
//     chunk only survives if it literally contains a query term.
//   - channel B (semantic bypass): a dense hybrid round (top_n=30) that
//     deliberately SKIPS narrowing, so a paraphrase-only hit can still enter the
//     pool. These are quarantined to a small per-fan-out quota
//     (fanoutSemanticQuota) because bypassing narrowing is what makes them
//     low-precision.
//   - channel C (metadata pre-filter): the sub-question's wording is turned into
//     metadata conditions over the SESSION's catalog fields, those documents are
//     pre-selected, and the retrieval runs INSIDE that subset (see
//     ExtractFanoutFilters). Best effort: no usable condition — or no metadata
//     resolver — simply skips the channel. Only runs when the caller asks for it
//     (useMetadata): the FIRST prefetch round, where the sub-questions are the
//     planner's raw wording; a rewrite round's queries are already targeted at a gap.
//
// Collapsing them into one call loses the split, and with it the ability to
// admit a paraphrase-only match without letting it displace an exact one.
//
// Admission order is the point: every fan-out's channel A is admitted before any
// fan-out's channel B, and C takes whatever room is left — its hits are the ones
// A/B did not reach at all.
//
// Returns the number of NEW snippets admitted to the pool.
// FanoutSearch runs the opening's per-clue retrieval and admits what it finds.
//
// It returns how many passages were NEW to the pool, and the RANKED UNION of the legs' results as
// chunk ids (see rankOpening): the second value is the opening's product — the ordered preview list
// the session is handed (o_1) — and it is why the legs' depth is not wasted. Returning only the
// count left the rank order to be re-derived, a second time, by whoever looked at the pool.
//
// useMetadata turns on channel C (the field-driven metadata channel): ONE extraction call for the whole
// batch, then one scoped retrieval per sub-question, admitted last because its hits are the ones A/B did
// not reach at all. It is the caller's switch, not a capability probe — the first prefetch round passes
// true, every later round false.
//
// There is no `capacity` parameter here even though the upstream signature has one: the ceiling it
// computed (maxTotal - len(seen), early-returning as soon as the pool looked full) is exactly what this
// branch removed. The pool has NO ceiling (see runtime/kbinfos.go) — a call is bounded by its own
// quotas (rawSnippetQuota / evidencePoolQuota) and by topN, never by how much the pool already holds —
// and re-adding that ceiling would refuse the very passages an enumeration's late members live in.
func FanoutSearch(ctx context.Context, deps RAGTools, st *AgenticState, queries []string, topN int, useMetadata bool) (int, []string) {
	if len(queries) == 0 || st.KB == nil {
		return 0, nil
	}
	sd := deps.Search
	if sd.Backend == nil {
		return 0, nil
	}

	// Dedup against what kbinfos ALREADY holds. There is no room to compute and no early
	// return when the pool looks "full": the pool has NO ceiling (see the note in
	// runtime/kbinfos.go), so a call is bounded only by its own per-call quotas
	// (rawSnippetQuota / evidencePoolQuota) and by topN — never by how much the pool already
	// holds. The ceiling computed here is what once refused the passage that would have
	// reached an enumeration's last members, and what made a rich round's own rewrite land
	// nowhere (the caller passed the snippet-pool number while the pool held up to twice it).
	seen := make(map[string]bool, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		seen[runtime.ChunkIDOf(c)] = true
	}

	// The planner's output is already []string, but blank entries still have to be dropped.
	qs := make([]string, 0, len(queries))
	for _, q := range queries {
		if q = strings.TrimSpace(q); q != "" {
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		return 0, nil
	}
	// topN bounds what the CLAIM channel recalls per query (below) and nothing else: the raw
	// channels' width is their own (fanoutBM25TopN / fanoutHybridTopN).
	capPerQuery := topN
	if capPerQuery < 1 {
		capPerQuery = 1
	}

	// The LEGS run in parallel: each one only RETRIEVES — kbinfos is mutated once, below, in a
	// single admit stretch — and everything the retrievals do share is guarded (the
	// per-request search cache holds its own mutex). A clue list is what the planner exists to
	// produce, and searching nine clues one after another is nine round trips; the paper
	// parallelises its per-clue retrieval for the same reason.
	//
	// What this makes explicit: the RETRIEVER contract is concurrent. Production retrievers are
	// HTTP/DB clients and already are (the multi-session rounds called them concurrently), so
	// the fixtures that stand in for them must be too — a stub that appends to a field needs a
	// lock, and the fan-out fixtures carry one.
	//
	// Results are written BY INDEX, so the admission order below is the query order whatever the
	// schedule does — that order is a contract: every fan-out's exact channel is admitted
	// before any fan-out's semantic channel.
	//
	// Channel C's conditions: ONE chat call for the whole batch, and only when the metadata channel was
	// asked for AND the dataset offers at least one filterable field. Without that second gate a
	// metadata-free dataset pays the extraction call and then runs one guaranteed-empty scoped retrieval
	// per sub-question, every round. A failure yields nil and the channel is skipped. The extraction is
	// SEQUENTIAL on purpose — it is one call for the batch and it must finish before the legs start.
	var filtersByQuery map[string][]map[string]any
	if useMetadata {
		if _, _, ok := fanoutMetadataVocabulary(deps); ok {
			filtersByQuery = ExtractFanoutFilters(ctx, deps, qs)
		} else {
			common.Info("prefetch: metadata channel skipped, the dataset offers no filterable metadata field")
		}
	}
	pairs := make([]fanoutPair, len(qs))
	var wg sync.WaitGroup
	for i, q := range qs {
		wg.Add(1)
		go func(i int, q string) {
			defer wg.Done()
			exact, semantic, metadata := fanoutSearchQuery(ctx, sd, q, capPerQuery, filtersByQuery[q])
			pairs[i] = fanoutPair{exact: exact, semantic: semantic, metadata: metadata}
		}(i, q)
	}
	wg.Wait()

	added := 0
	rawAdded := 0
	admit := func(batch []map[string]any) {
		for _, c := range batch {
			id := runtime.ChunkIDOf(c)
			isEvidence := strings.HasPrefix(id, "claim_")
			if id != "" {
				if seen[id] {
					continue
				}
				seen[id] = true
			}
			// Raw passages get their own budget;
			// anything left above it stays free for evidence rows, which are
			// far denser answer material. Evidence rows (claim_ prefix) bypass
			// the quota and never count against it.
			if !isEvidence && rawAdded >= rawSnippetQuota {
				continue
			}
			st.KB.Chunks = append(st.KB.Chunks, c)
			added++
			if !isEvidence {
				rawAdded++
			}
		}
	}
	// Channel 0: claim/evidence rows lead the pool —
	// they are the compact, verbatim-bearing proxy for the chunks they source.
	// Gated on the dataset having compiled rows at all; capped at
	// evidencePoolQuota rows across the whole prefetch; best effort.
	channel0 := [][]map[string]any{}
	channel0Rows := 0
	for _, q := range qs {
		select {
		case <-ctx.Done():
			return 0, nil
		default:
		}
		if !runtime.DatasetHasCompilation(ctx, sd) {
			break
		}
		// top_n=max(2, top_n) per query, so a single-fanout
		// question still recalls at least two claim rows.
		recallN := capPerQuery
		if recallN < 2 {
			recallN = 2
		}
		hits := runtime.RecallDatasetClaims(ctx, sd, q, recallN)
		if len(hits) == 0 {
			continue
		}
		pseudo := runtime.ClaimPseudoChunks(hits)
		kept := make([]map[string]any, 0, len(pseudo))
		for _, pc := range pseudo {
			if channel0Rows >= evidencePoolQuota {
				break
			}
			kept = append(kept, pc)
			channel0Rows++
		}
		if len(kept) == 0 {
			break
		}
		channel0 = append(channel0, kept)
		admit(kept)
	}
	// Evidence top-up: an evidence row carries a
	// verbatim quote but not its surrounding passage, so pull exactly the chunks
	// it cites instead of running another global recall. Deduped against the
	// pool (a chunk the claim already quotes verbatim adds nothing new) and
	// capped at evidenceTopUp ids. Best effort.
	if len(channel0) > 0 {
		var wanted []string
		inWanted := map[string]bool{}
		for _, pseudo := range channel0 {
			for _, c := range pseudo {
				ids, _ := c["source_chunk_ids"].([]string)
				for _, cid := range ids {
					cid = strings.TrimSpace(cid)
					if cid == "" || seen[cid] || inWanted[cid] {
						continue
					}
					inWanted[cid] = true
					wanted = append(wanted, cid)
				}
			}
		}
		if len(wanted) > evidenceTopUp {
			wanted = wanted[:evidenceTopUp]
		}
		if len(wanted) > 0 {
			fetched := runtime.LoadChunksForIDs(ctx, sd, wanted)
			if len(fetched) > 0 {
				admit(fetched)
			}
		}
	}
	// The RANKED UNION, computed BEFORE admission order matters (the pool keeps insertion order,
	// which is a contract for the citation list; the ranking is the opening's product and is
	// independent of it). Claims lead because they are verbatim-bearing and compact — the same
	// reason channel 0 admits them first.
	ranking := rankOpening(qs, pairs, channel0)

	// Channel A across every fan-out first, then channel B, then channel C — its hits are the ones
	// A/B did not reach at all.
	for _, p := range pairs {
		admit(p.exact)
	}
	for _, p := range pairs {
		admit(p.semantic)
	}
	for _, p := range pairs {
		admit(p.metadata)
	}
	return added, ranking
}

// fanoutPair is one clue's two channels, written BY INDEX by the parallel legs (see FanoutSearch).
type fanoutPair struct {
	exact    []map[string]any
	semantic []map[string]any
	// metadata is channel C's result: the scoped retrieval a sub-question's metadata filters select
	// (see fanoutSearchQuery and the admission order in FanoutSearch).
	metadata []map[string]any
}

// rankOpening fuses the opening's channels into ONE ranked list of chunk ids: reciprocal rank
// fusion over (clue × channel), claims first.
//
// RRF, not a score comparison, because the legs' scores are not comparable — a BM25 score, a vector
// similarity and a claim's fused rank live on different scales, and the codebase already fuses by
// reciprocal rank elsewhere (see the claim rows' q_<dim>_vec). The exact (keyword) channel is
// weighted above the semantic one: it is the deliberate surface probe, and its hits carry the
// corpus's own wording.
//
// This is the opening's o_1: the order the session is handed, and the order that decides which
// passages it reads first when its clock only affords a couple of calls.
func rankOpening(queries []string, pairs []fanoutPair, claims [][]map[string]any) []string {
	scores := make(map[string]float64)
	order := make([]string, 0, 64)
	add := func(list []map[string]any, weight float64) {
		for rank, c := range list {
			id := runtime.ChunkIDOf(c)
			if id == "" {
				continue
			}
			if _, seen := scores[id]; !seen {
				order = append(order, id)
			}
			scores[id] += weight / (openingRrfK + float64(rank+1))
		}
	}
	for _, batch := range claims {
		add(batch, claimChannelWeight)
	}
	for _, p := range pairs {
		add(p.exact, exactChannelWeight)
	}
	for _, p := range pairs {
		add(p.semantic, semanticChannelWeight)
	}
	// Channel C rides the same fusion: it is a scoped retrieval (metadata filters), so its head is
	// precision rather than recall and it must not outrank the deliberate exact probe.
	for _, p := range pairs {
		add(p.metadata, metadataChannelWeight)
	}
	// Stable sort by fused score, ties keeping first-seen order (clue order, then channel, then the
	// leg's own ranking) so the delivery is reproducible.
	sort.SliceStable(order, func(i, j int) bool { return scores[order[i]] > scores[order[j]] })
	return order
}

// The fusion weights: the keyword channel is the deliberate probe and outranks the semantic
// complement; a claim row is a verbatim, compact statement of the same text and leads both.
const (
	claimChannelWeight    = 3.0
	exactChannelWeight    = 2.0
	semanticChannelWeight = 1.0
	metadataChannelWeight = 1.0
)

// fanoutSearchQuery runs one fan-out's two channels. It only retrieves; the
// caller admits the results.
func fanoutSearchQuery(ctx context.Context, sd runtime.SearchDeps, fq string, capPerQuery int, filters []map[string]any) (exact, semantic, metadata []map[string]any) {
	terms := runtime.QueryToTerms(fq)
	keyed := runtime.FanoutKeyedTerms(terms)
	termList := keyed
	if len(termList) == 0 {
		termList = terms
	}

	// Channel A — exact: BM25Search is the dedicated keyword-only entry point (vector weight
	// 0).
	candidates, _ := runtime.BM25Search(ctx, sd, runtime.SearchParams{
		Question: fq,
		Keywords: strings.Join(termList, " "),
		TopN:     fanoutBM25TopN,
	})
	if len(candidates) > 0 {
		res := runtime.NarrowByTerms(candidates, termList, nil, fq,
			runtime.NarrowContext{Before: 0, After: 1},
			fanoutNarrowMaxOutPerChunk, fanoutNarrowMaxOutTotal)
		exact = res.Kept
		// NO per-query cut here any more. `exact[:capPerQuery]` was the opening's own ceiling: the
		// legs RECALLED wide and the caller then kept the first eight passages of each clue, so the
		// union that got ranked was three clues wide and eight deep — measured 2026-09-20 (三国),
		// the candidates the session could ever reach, before it searched for anything.
	}

	// Channel B — semantic bypass: HybridSearch gives the vector leg weight 0.3 whenever an
	// embedder is configured. NO narrowing (a paraphrase-only hit has no query term to match, so
	// narrowing would erase it).
	hits, _ := runtime.HybridSearch(ctx, sd, runtime.SearchParams{
		Question: fq,
		TopN:     fanoutHybridTopN,
	})
	exactIDs := make(map[string]bool, len(exact))
	for _, c := range exact {
		exactIDs[runtime.ChunkIDOf(c)] = true
	}
	for _, c := range hits {
		if exactIDs[runtime.ChunkIDOf(c)] {
			continue
		}
		semantic = append(semantic, c)
		if len(semantic) >= fanoutSemanticQuota {
			break
		}
	}

	// Channel C — metadata pre-filter: documents are pre-selected by the metadata conditions the
	// sub-question's wording supports (OR across them), and the retrieval runs INSIDE that
	// subset. The conditions name the session catalog's fields, and they are the SAME shape the
	// metadata_search tool takes, so a condition means the same thing on either path.
	// runtime.MetadataSearch is inert without a resolver, and the caller only spends the
	// extraction call when the dataset offers a filterable field (see fanoutMetadataVocabulary).
	if len(filters) > 0 {
		seenAB := make(map[string]bool, len(exact)+len(semantic))
		for _, c := range exact {
			seenAB[runtime.ChunkIDOf(c)] = true
		}
		for _, c := range semantic {
			seenAB[runtime.ChunkIDOf(c)] = true
		}
		hits, _ := runtime.MetadataSearch(ctx, sd, runtime.SearchParams{
			Question: fq,
			TopN:     capPerQuery,
		}, filters, "or")
		for _, c := range hits {
			if seenAB[runtime.ChunkIDOf(c)] {
				continue
			}
			metadata = append(metadata, c)
			if len(metadata) >= fanoutMetadataQuota {
				break
			}
		}
	}
	return exact, semantic, metadata
}
