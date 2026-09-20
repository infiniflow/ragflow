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
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"ragflow/internal/rag/agentic-rag/runtime"
)

// The fan-out stage: decompose the question into first-hop sub-questions, parse what
// the model wrote, and fetch them programmatically.
//
// Its parsing is shape-guarded rather than answer-keyed (see fanoutLooksLikeQuery):
// the model is asked for sub-questions and hands back whatever it produced, so what
// a line IS decides what the stage does with it.
// fanoutLooksLikeQuery: reject
// prose/answer lines before they can enter the retrieval + slot pipeline.
//
// Fan-outs are used verbatim as BM25/hybrid queries and as the slot table's
// fanout_hint, so an answered fact ("The woman was **X**") must never survive
// here: it both poisons retrieval and asserts a hallucinated entity as a known
// aspect.
func fanoutLooksLikeQuery(line string, loose bool) bool {
	s := strings.TrimSpace(line)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	for _, mark := range fanoutAnswerMarks {
		if strings.Contains(low, mark) {
			return false
		}
	}
	// The cap counts characters, not bytes; a byte-based cap would reject a legitimate CJK
	// fan-out well below the 160-character limit.
	if utf8.RuneCountInString(s) > fanoutMaxChars {
		return false
	}
	words := len(strings.Fields(s))
	if loose && !strings.HasSuffix(s, "?") && words > fanoutLooseMaxWords {
		return false
	}
	return words <= fanoutMaxWords
}

// fanoutLineBreak reports whether r is a line boundary.
// Splitting on "\n" alone misses a lone "\r" and the other Unicode line breaks a model
// could emit, so the loose path would fuse several fan-out lines into one.
func fanoutLineBreak(r rune) bool {
	switch r {
	case '\n', '\r', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// parseFanouts: extract fan-outs from a
// model reply, validating every entry's shape.
func parseFanouts(text string) []string {
	var raw []string
	loose := false
	if data, ok := extractJSONObject(text).(map[string]any); ok {
		for _, f := range asSliceOfAny(data["fanouts"]) {
			if s := strings.TrimSpace(fmt.Sprint(f)); s != "" {
				raw = append(raw, s)
			}
		}
	} else {
		// Loose fallback: the model answered in prose. Only lines that still
		// look like a search query are kept — answer sentences and source
		// lists are dropped.
		loose = true
		// Split on every line boundary, not just "\n".
		for _, ln := range strings.FieldsFunc(text, fanoutLineBreak) {
			if strings.TrimSpace(ln) == "" {
				continue
			}
			// The bullet / numbering cutset is stripped from BOTH ends, then whitespace is
			// trimmed, so a trailing period/digit never leaks into the retrieval query.
			raw = append(raw, strings.TrimSpace(strings.Trim(ln, "-•0123456789. ")))
		}
	}
	kept := make([]string, 0, len(raw))
	for _, q := range raw {
		if fanoutLooksLikeQuery(q, loose) {
			kept = append(kept, q)
		}
	}
	if len(raw) > 0 && len(kept) == 0 {
		_LOG.Printf("[Planner] discarding %d fan-out candidate(s): none look like search queries", len(raw))
	}
	kept = dedupe(kept)
	if len(kept) > MaxFanouts {
		kept = kept[:MaxFanouts]
	}
	return kept
}

// ExpandFanouts: ONE chat call (no
// tools) producing 2-5 first-hop fan-outs, plus one strict retry when the
// reply was not parseable JSON (_expand_fanouts).
//
// Falls back to the raw question alone on any failure — a fan-out failure never
// blocks the pipeline.
func ExpandFanouts(ctx context.Context, deps RAGTools, question string) []string {
	if question == "" {
		return nil
	}
	if deps.Model == nil {
		return []string{question}
	}
	reply, err := deps.Model.Complete(ctx, []schema.Message{
		*schema.SystemMessage(fanoutPrompt),
		*schema.UserMessage("Question: " + question),
	}, nil)
	if err != nil {
		_LOG.Printf("[Planner] fan-out expansion failed; falling back to raw question: %v", err)
		return []string{question}
	}
	fanouts := parseFanouts(reply.Content)
	if len(fanouts) == 0 {
		// The model answered the question instead of decomposing it (no JSON,
		// or JSON that failed the shape guard). One strict retry, then give up.
		_LOG.Printf("[Planner] fan-out expansion produced no usable sub-question; retrying with a strict JSON instruction")
		if retry, rerr := deps.Model.Complete(ctx, []schema.Message{
			*schema.SystemMessage(fanoutPrompt + fanoutStrictRetry),
			*schema.UserMessage("Question: " + question),
		}, nil); rerr != nil {
			_LOG.Printf("[Planner] strict fan-out retry failed: %v", rerr)
		} else {
			fanouts = parseFanouts(retry.Content)
		}
	}
	if len(fanouts) == 0 {
		fanouts = []string{question}
	}
	_LOG.Printf("[Planner] fan-out expansion: %d sub-question(s): %v", len(fanouts), fanouts)
	return fanouts
}

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
		_LOG.Printf("[Prefetch] metadata filter extraction failed; skipping the metadata channel: %v", err)
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
		flat := strings.ReplaceAll(truncateRunes(reply.Content, 300), "\n", " ")
		_LOG.Printf("[Prefetch] metadata channel produced no usable condition; reply was: %s", flat)
		return nil
	}
	_LOG.Printf("[Prefetch] metadata filters: %v", out)
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
func FanoutSearch(ctx context.Context, deps RAGTools, st *AgenticState, queries []string, topN, capacity int, useMetadata bool) int {
	if len(queries) == 0 || st.KB == nil {
		return 0
	}
	sd := deps.Search
	if sd.Backend == nil {
		return 0
	}

	// Dedup against what kbinfos ALREADY holds, then cap admissions at the remaining room —
	// the pool is a shared, cross-round ceiling.
	seen := make(map[string]bool, len(st.KB.Chunks))
	for _, c := range st.KB.Chunks {
		seen[runtime.ChunkIDOf(c)] = true
	}
	maxTotal := capacity
	if maxTotal <= 0 {
		maxTotal = MaxSnippetPool
	}
	room := maxTotal - len(seen)
	if room <= 0 {
		return 0
	}

	// The planner's output is already []string, but blank entries still have to be dropped.
	qs := make([]string, 0, len(queries))
	for _, q := range queries {
		if q = strings.TrimSpace(q); q != "" {
			qs = append(qs, q)
		}
	}
	if len(qs) == 0 {
		return 0
	}
	capPerQuery := topN
	if capPerQuery < 1 {
		capPerQuery = 1
	}

	// The fan-outs could run in parallel, but each one only RETRIEVES — kbinfos is mutated
	// once, back in the caller. This keeps that "retrieve, then mutate once" structure, hence
	// sequential: the per-request search cache and Kbinfos are shared mutable state that the
	// retrievals never touch concurrently.
	type fanoutPair struct {
		exact    []map[string]any
		semantic []map[string]any
		metadata []map[string]any
	}
	// Channel C's conditions: ONE chat call for the whole batch, and only when the metadata
	// channel was asked for AND the dataset offers at least one filterable field. Without that
	// second gate a metadata-free dataset pays the extraction call and then runs one
	// guaranteed-empty scoped retrieval per sub-question, every round. A failure yields nil and
	// the channel is skipped.
	var filtersByQuery map[string][]map[string]any
	if useMetadata {
		if _, _, ok := fanoutMetadataVocabulary(deps); ok {
			filtersByQuery = ExtractFanoutFilters(ctx, deps, qs)
		} else {
			_LOG.Printf("[Prefetch] metadata channel skipped — the dataset offers no filterable metadata field")
		}
	}
	pairs := make([]fanoutPair, 0, len(qs))
	for _, q := range qs {
		select {
		case <-ctx.Done():
			return 0
		default:
		}
		exact, semantic, metadata := fanoutSearchQuery(ctx, sd, q, capPerQuery, filtersByQuery[q])
		pairs = append(pairs, fanoutPair{exact: exact, semantic: semantic, metadata: metadata})
	}

	added := 0
	rawAdded := 0
	admit := func(batch []map[string]any) bool {
		for _, c := range batch {
			if added >= room {
				return true
			}
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
		return added >= room
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
			return 0
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
		if admit(kept) {
			return added
		}
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
			if len(fetched) > 0 && admit(fetched) {
				return added
			}
		}
	}
	// Channel A across every fan-out first, then channel B, then channel C.
	for _, p := range pairs {
		if admit(p.exact) {
			return added
		}
	}
	for _, p := range pairs {
		if admit(p.semantic) {
			return added
		}
	}
	for _, p := range pairs {
		if admit(p.metadata) {
			return added
		}
	}
	return added
}

// fanoutSearchQuery runs one fan-out's three channels. It only retrieves; the caller admits the
// results. filters are the sub-question's metadata conditions (key/op/value over the session's
// catalog); an empty list skips channel C.
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
		if len(exact) > capPerQuery {
			exact = exact[:capPerQuery]
		}
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
