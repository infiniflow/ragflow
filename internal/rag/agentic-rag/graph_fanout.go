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

// Metadata (title) pre-filter channel: the entities a sub-question names.
//
// Channel C exists because a sub-question that NAMES a document is better served by
// pre-filtering the document set by title and searching inside it than by another
// whole-corpus round: the whole-corpus round has to rank the named document against every
// other one, and on a large corpus it loses. The entities come from ONE chat call for the
// whole fan-out batch, and every entity must look like a SHORT NAME — a copied
// sub-question or an answer would make the title filter match nothing.

const metadataEntityPrompt = `For each numbered search sub-question below, extract up to 3 short named entities ` +
	`it targets — the kind of name a document index stores as a title (an article/report ` +
	`name, an organisation, a place, an event, a person). Use the exact surface form with ` +
	`SPACES, never underscores. If a sub-question has no clear title-like entity, use an ` +
	`empty string for it. Keep each entity under 10 words; never copy the whole ` +
	`sub-question, never answer it. ` +
	`Respond with JSON only: {"entities": [["<e1>", "<e2>", ...], ...]} — a list of ` +
	`entity-arrays in the SAME ORDER as the sub-questions, JSON only, no prose.`

const (
	// metadataEntityMaxWords / metadataEntityMaxChars are the guard on an entity fed to
	// the title filter: a long string is a copied sub-question or an answer.
	metadataEntityMaxWords = 10
	metadataEntityMaxChars = 80
	// metadataMaxEntities caps the entities taken from one sub-question.
	metadataMaxEntities = 3
	// fanoutMetadataQuota is the metadata channel's per-query quota. Modest like the
	// semantic quota: the channel's hits are only as good as the entity extraction.
	fanoutMetadataQuota = 4
)

// metadataEntityStrings coerces a JSON entity list to trimmed strings.
func metadataEntityStrings(v any) []string {
	items := asSliceOfAny(v)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, strings.TrimSpace(fmt.Sprint(it)))
	}
	return out
}

// metadataEntityLooksUsable reports whether an extracted entity is a short name rather
// than a copied sub-question or an answer.
func metadataEntityLooksUsable(entity string) bool {
	s := strings.TrimSpace(entity)
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
	// entity well below the 80-character limit.
	return utf8.RuneCountInString(s) <= metadataEntityMaxChars && len(strings.Fields(s)) <= metadataEntityMaxWords
}

// parseFanoutEntities extracts up to metadataMaxEntities short entities per sub-question
// from a model reply, order-aligned with fanouts.
//
// Accepts {"entities": [["e1","e2"], ...]} (preferred), a flat {"entities": ["e1", ...]}
// (read positionally: one entity per sub-question), or
// {"entities": [{"sub_question": ..., "entities": [...]}]} matched by text. Entities that
// fail metadataEntityLooksUsable are dropped, as are duplicates.
func parseFanoutEntities(text string, fanouts []string) [][]string {
	out := make([][]string, len(fanouts))
	data, ok := extractJSONObject(text).(map[string]any)
	if !ok {
		return out
	}
	items := asSliceOfAny(data["entities"])
	if len(items) == 0 {
		return out
	}
	byText := map[string][]string{}
	var positional [][]string
	for _, it := range items {
		switch v := it.(type) {
		case map[string]any:
			ents := metadataEntityStrings(v["entities"])
			if len(ents) == 0 {
				ents = metadataEntityStrings(v["titles"])
			}
			sq := ""
			if sv := v["sub_question"]; sv != nil {
				sq = strings.TrimSpace(fmt.Sprint(sv))
			}
			if sq == "" && v["fanout"] != nil {
				sq = strings.TrimSpace(fmt.Sprint(v["fanout"]))
			}
			if sq != "" {
				byText[sq] = ents
			}
			positional = append(positional, ents)
		case string:
			positional = append(positional, []string{strings.TrimSpace(v)})
		case []any:
			positional = append(positional, metadataEntityStrings(v))
		}
	}
	for i, fq := range fanouts {
		var raw []string
		if len(byText) > 0 {
			raw = byText[fq]
		} else if i < len(positional) {
			raw = positional[i]
		}
		seen := make(map[string]bool, len(raw))
		cleaned := make([]string, 0, metadataMaxEntities)
		for _, e := range raw {
			e = strings.TrimSpace(e)
			if e != "" && !seen[strings.ToLower(e)] && metadataEntityLooksUsable(e) {
				seen[strings.ToLower(e)] = true
				cleaned = append(cleaned, e)
			}
			if len(cleaned) >= metadataMaxEntities {
				break
			}
		}
		out[i] = cleaned
	}
	return out
}

// ExtractFanoutEntities maps each sub-question to 1-3 short entities for metadata (title)
// filtering: ONE chat call for the whole batch.
//
// Only sub-questions with at least one usable entity are returned. Any failure — no model,
// a model error, an unparseable reply — yields nil so the metadata channel is simply
// skipped for this round; pre-search is never blocked by it.
func ExtractFanoutEntities(ctx context.Context, deps RAGTools, fanouts []string) map[string][]string {
	queries := make([]string, 0, len(fanouts))
	for _, f := range fanouts {
		if q := strings.TrimSpace(f); q != "" {
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 || deps.Model == nil {
		return nil
	}
	listed := make([]string, 0, len(queries))
	for i, q := range queries {
		listed = append(listed, fmt.Sprintf("%d. %s", i+1, q))
	}
	reply, err := deps.Model.Complete(ctx, []schema.Message{
		*schema.SystemMessage(metadataEntityPrompt),
		*schema.UserMessage("Sub-questions:\n" + strings.Join(listed, "\n")),
	}, nil)
	if err != nil {
		_LOG.Printf("[Prefetch] entity extraction failed; skipping the metadata channel: %v", err)
		return nil
	}
	entities := parseFanoutEntities(reply.Content, queries)
	out := make(map[string][]string, len(queries))
	for i, q := range queries {
		if len(entities[i]) > 0 {
			out[q] = entities[i]
		}
	}
	if len(out) > 0 {
		_LOG.Printf("[Prefetch] metadata entities: %v", out)
	}
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
//   - channel C (metadata title pre-filter): the sub-question's named entities
//     pre-select documents by title and the retrieval runs INSIDE that subset
//     (see ExtractFanoutEntities). Best effort: no usable entities — or no
//     metadata resolver — simply skips the channel. Only runs when the caller
//     asks for it (useMetadata): the FIRST prefetch round, where the sub-questions
//     are the planner's raw wording; a rewrite round's queries are already
//     targeted at a gap.
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
	// Channel C's entities: ONE chat call for the whole batch, and only when the metadata
	// channel was asked for. A failure yields nil and the channel is skipped.
	var entitiesByQuery map[string][]string
	if useMetadata {
		entitiesByQuery = ExtractFanoutEntities(ctx, deps, qs)
	}
	pairs := make([]fanoutPair, 0, len(qs))
	for _, q := range qs {
		select {
		case <-ctx.Done():
			return 0
		default:
		}
		exact, semantic, metadata := fanoutSearchQuery(ctx, sd, q, capPerQuery, entitiesByQuery[q])
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

// fanoutSearchQuery runs one fan-out's three channels. It only retrieves; the
// caller admits the results. entities are the sub-question's title entities; an empty
// list skips channel C.
func fanoutSearchQuery(ctx context.Context, sd runtime.SearchDeps, fq string, capPerQuery int, entities []string) (exact, semantic, metadata []map[string]any) {
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

	// Channel C — metadata title pre-filter: documents are pre-selected by their title
	// metadata matching ANY of the sub-question's entities (OR), and the retrieval runs
	// INSIDE that subset. Only hits the other two channels did not already surface are
	// kept; the merge dedups again. runtime.MetadataSearch is inert without a resolver.
	if len(entities) > 0 {
		filters := make([]map[string]any, 0, len(entities))
		for _, e := range entities {
			filters = append(filters, map[string]any{"key": "title", "op": "contains", "value": e})
		}
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
