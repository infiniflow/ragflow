// Package-level claim recall for the agentic harness: a KB-wide FLAT search
// over claim/evidence rows (entity_type_kwd="claim"), mirroring Python
// harness/tools/navigation.py::recall_dataset_claims and its prefetch wiring
// (action_session.py::_claim_prefetch + agentic_rag_graph.py channel 0).
//
// Two legs per search — BM25 over content_ltks/content_sm_ltks and KNN over
// the claim rows' q_<dim>_vec — fused by reciprocal rank. No similarity
// threshold anywhere: the store ranks, we never scan. The BM25 leg alone
// keeps recall alive without an embedder, so claim recall can only ever ADD
// to a search result.

package harness

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	nlp "ragflow/internal/service/nlp"
)

const (
	// ClaimPrefetchTopN mirrors Python _CLAIM_PREFETCH_TOP_N.
	ClaimPrefetchTopN = 6
	// ClaimEvidenceChars caps the rendered verbatim quote (Python
	// _STRUCT_CLAIM_EVIDENCE_CHARS). 1200 over 600: the quote must be
	// self-sufficient in ONE shot — a thin quote makes the SCA judge the
	// context insufficient, and one extra re-search turn costs a full
	// 10k+-token prompt.
	ClaimEvidenceChars = 1200
	// claimRecallLegFloor is the per-leg store limit floor (Python
	// `limit = max(top_n, 32)`).
	claimRecallLegFloor = 32
	// claimPrefetchCacheTTL / claimPrefetchCacheCap mirror Python
	// _CLAIM_PREFETCH_CACHE_TTL/_CAP: the agent re-issues near-identical
	// queries across turns and each miss costs one KNN round-trip per KB.
	claimPrefetchCacheTTL = 300.0
	claimPrefetchCacheCap = 64
	// EvidenceQuoteChars caps the verbatim quote in the GRAPH fan-out's
	// channel-0 pseudo chunks (Python agentic_rag_graph.py:723 `[:400]`). The
	// action-session prefetch uses the longer ClaimEvidenceChars (1200,
	// _STRUCT_CLAIM_EVIDENCE_CHARS) — the two conventions are deliberately
	// different on the Python side and must not be conflated.
	EvidenceQuoteChars = 400
)

// claimRowTypes lists the entity_type_kwd values the agentic claim search
// matches: Python _evidence_row_types resolves to ("claim",) for every
// compile kind (page_index's pre-rename fact/conclusion spellings are not
// searched — a recompile retypes them).
var _ = []string{"claim", "fact", "conclusion"} // legacy spellings, API-side only (Python CLAIM_ROW_TYPES)

// claimRowTypes lists the entity_type_kwd values that carry a claim (Python
// _EVIDENCE_ROW_TYPES_BY_COMPILE flattened: every compiler writes "claim"
// now; the legacy spellings keep pre-rename rows visible).
var claimRowTypes = []string{"claim"}

// compilationKwds mirrors Python _COMPILATION_KWDS — the compile_kwd values
// the knowledge-compilation paths write, used by the has-compilation probe.
var compilationKwds = []string{"tree", "page_index", "pageindex", "timeline", "dataset_nav"}

// ClaimHit is one recalled claim, in Python's recall_dataset_claims shape.
type ClaimHit struct {
	Name        string
	Description string
	Quote       string
	DocID       string
	ChunkIDs    []string
	Score       float64
	Rank        int
}

type claimCacheEntry struct {
	at   time.Time
	hits []*ClaimHit
}

var (
	claimRecallMu    sync.Mutex
	claimRecallCache = map[string]claimCacheEntry{}
	compilationMu    sync.Mutex
	compilationCache = map[string]compilationProbe{}
)

type compilationProbe struct {
	at  time.Time
	has bool
}

// claimEngine resolves the doc engine: the deps' handle first, the process
// singleton as fallback (mirrors runSearch's DocEngine resolution).
func claimEngine(deps SearchDeps) engine.DocEngine {
	if deps.DocEngine != nil {
		return deps.DocEngine
	}
	return engine.Get()
}

// claimIndexName resolves the tenant index (deps override, then the default).
func claimIndexName(deps SearchDeps) string {
	if v := strings.TrimSpace(deps.IndexName); v != "" {
		return v
	}
	return fmt.Sprintf("ragflow_%s", deps.TenantID)
}

// DatasetHasCompilation reports whether any in-scope KB carries compiled
// rows (Python dataset_has_compilation + dataset_compilation_kinds): one
// cheap probe per KB set, TTL-cached. Probe failures FAIL OPEN (true) so a
// glitch never silently disables claim recall.
func DatasetHasCompilation(ctx context.Context, deps SearchDeps) bool {
	de := claimEngine(deps)
	if de == nil || len(deps.KbIDs) == 0 {
		return false
	}
	key := deps.TenantID + "|" + strings.Join(deps.KbIDs, ",")
	now := time.Now()

	compilationMu.Lock()
	if p, ok := compilationCache[key]; ok && now.Sub(p.at).Seconds() < claimPrefetchCacheTTL {
		compilationMu.Unlock()
		return p.has
	}
	compilationMu.Unlock()

	has := false
	res, err := de.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{claimIndexName(deps)},
		KbIDs:        deps.KbIDs,
		Offset:       0,
		Limit:        1,
		SelectFields: []string{"id"},
		Filter:       map[string]interface{}{"compile_kwd": compilationKwds},
	})
	if err != nil {
		has = true // fail open, mirroring Python
	} else {
		has = res != nil && len(res.Chunks) > 0
	}

	compilationMu.Lock()
	compilationCache[key] = compilationProbe{at: now, has: has}
	compilationMu.Unlock()
	return has
}

// RecallDatasetClaims runs the two-leg hybrid claim recall and returns up to
// topN fused hits. Empty (never an error) on any failure — the prefetch can
// only ever add to a search result.
func RecallDatasetClaims(ctx context.Context, deps SearchDeps, query string, topN int) []*ClaimHit {
	return recallDatasetClaimsFiltered(ctx, deps, query, topN, nil, nil)
}

// recallDatasetClaimsFiltered is the recall core with optional compile_kwd /
// entity_type_kwd pinning (Python claim_agg queries ("tree","claim") and
// ("page_index","claim") as two separate passes; the session-level recall
// passes nil/nil and matches row_types=("claim",) unconditionally).
func recallDatasetClaimsFiltered(ctx context.Context, deps SearchDeps, query string, topN int, compileKwds, rowTypes []string) []*ClaimHit {
	de := claimEngine(deps)
	if de == nil || deps.TenantID == "" || len(deps.KbIDs) == 0 {
		return nil
	}
	query = strings.TrimSpace(query)
	if query == "" || topN <= 0 {
		return nil
	}
	key := deps.TenantID + "|" + strings.Join(deps.KbIDs, ",") + "|" + strings.Join(compileKwds, ",") + "|" + strings.ToLower(query)
	now := time.Now()

	claimRecallMu.Lock()
	if cached, ok := claimRecallCache[key]; ok && now.Sub(cached.at).Seconds() < claimPrefetchCacheTTL {
		claimRecallMu.Unlock()
		return cached.hits
	}
	if len(claimRecallCache) >= claimPrefetchCacheCap {
		claimRecallCache = map[string]claimCacheEntry{}
	}
	claimRecallMu.Unlock()

	limit := topN
	if limit < claimRecallLegFloor {
		limit = claimRecallLegFloor
	}
	rowTypesEffective := rowTypes
	if len(rowTypesEffective) == 0 {
		rowTypesEffective = claimRowTypes
	}
	filter := map[string]interface{}{
		"entity_type_kwd": rowTypesEffective,
		"scope_kwd":       []string{"doc"},
	}
	if len(compileKwds) > 0 {
		filter["compile_kwd"] = compileKwds
	}
	fields := []string{"content_with_weight", "source_chunk_ids", "doc_id"}

	// KNN leg (needs an embedder); BM25 leg always runs.
	var denseLeg, textLeg []*ClaimHit
	if deps.HasEmbedder && deps.Embedder != nil {
		if vecs := mustEncodeQueries(deps, ctx, query); len(vecs) > 0 && len(vecs[0]) > 0 {
			qvec := vecs[0]
			dense := make([]float64, len(qvec))
			for i, v := range qvec {
				dense[i] = float64(v)
			}
			exprs := []interface{}{&types.MatchDenseExpr{
				VectorColumnName:  fmt.Sprintf("q_%d_vec", len(qvec)),
				EmbeddingData:     dense,
				EmbeddingDataType: "float",
				DistanceType:      "cosine",
				TopN:              limit,
			}}
			denseLeg = claimSearchLeg(ctx, de, deps, filter, fields, exprs, limit)
		}
	}
	exprs := []interface{}{&types.MatchTextExpr{
		Fields:       []string{"content_ltks", "content_sm_ltks"},
		MatchingText: query,
		TopN:         limit,
	}}
	textLeg = claimSearchLeg(ctx, de, deps, filter, fields, exprs, limit)

	if len(denseLeg) == 0 && len(textLeg) == 0 {
		return nil
	}
	out := rrfFuseClaims(denseLeg, textLeg)
	if len(out) > topN {
		out = out[:topN]
	}

	claimRecallMu.Lock()
	claimRecallCache[key] = claimCacheEntry{at: now, hits: out}
	claimRecallMu.Unlock()
	return out
}

// claimSearchLeg runs one store search and parses the claim hits.
func claimSearchLeg(ctx context.Context, de engine.DocEngine, deps SearchDeps, filter map[string]interface{}, fields []string, exprs []interface{}, limit int) []*ClaimHit {
	res, err := de.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{claimIndexName(deps)},
		KbIDs:        deps.KbIDs,
		Offset:       0,
		Limit:        limit,
		SelectFields: fields,
		Filter:       filter,
		MatchExprs:   exprs,
	})
	if err != nil || res == nil {
		return nil
	}
	hits := make([]*ClaimHit, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		if hit := parseClaimHit(row); hit != nil {
			hits = append(hits, hit)
		}
	}
	return hits
}

// parseClaimHit extracts one claim hit from a store row. Nil when the row is
// not a usable claim (bad payload, unnamed, or no chunk pointer).
func parseClaimHit(row map[string]interface{}) *ClaimHit {
	raw, _ := row["content_with_weight"].(string)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil || payload == nil {
		return nil
	}
	name := strings.TrimSpace(claimStr(payload["name"]))
	if name == "" {
		return nil
	}
	chunkIDs := claimChunkIDs(row["source_chunk_ids"], payload["source_chunk_ids"])
	if len(chunkIDs) == 0 {
		return nil
	}
	quote := ""
	if ev, ok := payload["evidence"].([]interface{}); ok {
		for _, item := range ev {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if q := strings.TrimSpace(claimStr(m["quote"])); q != "" {
				quote = q
				break
			}
		}
	}
	hit := &ClaimHit{
		Name:        name,
		Description: strings.TrimSpace(claimStr(payload["description"])),
		Quote:       quote,
		DocID:       strings.TrimSpace(claimStr(row["doc_id"])),
		ChunkIDs:    chunkIDs,
	}
	if v, ok := row["similarity"].(float64); ok {
		hit.Score = v
	}
	return hit
}

// rrfFuseClaims fuses the retrieval legs by reciprocal rank (Python
// _rrf_fuse, k=60): each fused hit keeps its best leg score and gains a
// 1-based rank.
func rrfFuseClaims(legs ...[]*ClaimHit) []*ClaimHit {
	const k = 60
	type key struct {
		docID string
		name  string
	}
	best := map[key]*ClaimHit{}
	fused := map[key]float64{}
	for _, leg := range legs {
		for rank, hit := range leg {
			hk := key{docID: hit.DocID, name: hit.Name}
			fused[hk] += 1.0 / float64(k+rank+1)
			if cur := best[hk]; cur == nil || hit.Score > cur.Score {
				best[hk] = hit
			}
		}
	}
	out := make([]*ClaimHit, 0, len(best))
	for hk, hit := range best {
		hit.Rank = 0 // set below, after sorting
		_ = fused[hk]
		out = append(out, hit)
	}
	sort.Slice(out, func(i, j int) bool {
		ki := key{docID: out[i].DocID, name: out[i].Name}
		kj := key{docID: out[j].DocID, name: out[j].Name}
		return fused[ki] > fused[kj]
	})
	for i, hit := range out {
		hit.Rank = i + 1
	}
	return out
}

// ClaimPseudoChunks renders recalled claims as the pool-shaped pseudo chunks
// the GRAPH fan-out's channel 0 admits (Python agentic_rag_graph.py
// _collect_evidence :712-733): the row leads with the literal "[evidence]"
// prefix, the quote is a LITERAL quoted span capped at EvidenceQuoteChars
// (400), and the whole content is capped at 1200. source_chunk_ids ride along
// so the directional top-up and _prefill_slots_from_evidence can find the
// underlying chunks.
//
// This is NOT the action-session prefetch's format — Python's _claim_prefetch
// renders "[claim #rank]" with a 1200-char quote (action_session.py:741-747),
// which ClaimPrefetch builds inline. The two formats are Python-exact and
// deliberately distinct.
func ClaimPseudoChunks(claims []*ClaimHit) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(claims))
	for _, c := range claims {
		cid := claimHitID(c)
		content := "[evidence] " + c.Name
		if c.Description != "" && c.Description != c.Name {
			content += " — " + c.Description
		}
		if c.Quote != "" {
			// Python [:_STRUCT_CLAIM_EVIDENCE_CHARS] counts CODE POINTS.
			quote := truncateRunes(c.Quote, EvidenceQuoteChars)
			content += "\nEvidence (verbatim): \"" + quote + "\""
		}
		// Python content[:1200] (:728) counts CODE POINTS, not bytes.
		content = truncateRunes(content, 1200)
		out = append(out, map[string]interface{}{
			"chunk_id":            cid,
			"content_with_weight": content,
			"doc_id":              c.DocID,
			"source_chunk_ids":    c.ChunkIDs,
		})
	}
	return out
}

// claimHitID mirrors Python's "claim_" + md5(f"{doc_id}:{name}")[:12] pool id.
func claimHitID(c *ClaimHit) string {
	return "claim_" + fmt.Sprintf("%x", md5.Sum([]byte(c.DocID+":"+c.Name)))[:12]
}

// ClaimPrefetch runs the gated prefetch for one corpus search (Python
// action_session._claim_prefetch): the has-compilation gate, the recall, and
// the exclusive passage build. ok is false when the caller must fall through
// to the plain chunk search.
func ClaimPrefetch(ctx context.Context, deps SearchDeps, query string, seen map[string]bool) ([]map[string]any, []string, []map[string]interface{}, bool) {
	if strings.TrimSpace(query) == "" {
		return nil, nil, nil, false
	}
	if !DatasetHasCompilation(ctx, deps) {
		return nil, nil, nil, false
	}
	claims := RecallDatasetClaims(ctx, deps, query, ClaimPrefetchTopN)
	if len(claims) == 0 {
		return nil, nil, nil, false
	}
	var payload []map[string]any
	var ids []string
	var pseudo []map[string]interface{}
	for _, c := range claims {
		cid := claimHitID(c)
		if seen[cid] {
			continue
		}
		content := fmt.Sprintf("[claim #%d] %s", c.Rank, c.Name)
		if c.Description != "" && c.Description != c.Name {
			content += " — " + c.Description
		}
		if c.Quote != "" {
			// Python [:_STRUCT_CLAIM_EVIDENCE_CHARS] counts CODE POINTS.
			quote := truncateRunes(c.Quote, ClaimEvidenceChars)
			content += fmt.Sprintf("\nEvidence (verbatim): %q", quote)
		}
		// Python content[:1200] (:747) counts CODE POINTS, not bytes.
		content = truncateRunes(content, 1200)
		docID := c.DocID
		payload = append(payload, map[string]any{"id": cid, "content": content, "doc_id": docID})
		ids = append(ids, cid)
		pseudo = append(pseudo, map[string]interface{}{
			"chunk_id":            cid,
			"content_with_weight": content,
			"doc_id":              docID,
			"source_chunk_ids":    c.ChunkIDs,
		})
	}
	if len(payload) == 0 {
		return nil, nil, nil, false
	}
	return payload, ids, pseudo, true
}

// claimChunkIDs merges the row column and the payload's own ids.
func claimChunkIDs(values ...interface{}) []string {
	for _, v := range values {
		switch x := v.(type) {
		case []string:
			if out := claimTrimIDs(x); len(out) > 0 {
				return out
			}
		case []interface{}:
			out := make([]string, 0, len(x))
			for _, item := range x {
				if s := strings.TrimSpace(claimStr(item)); s != "" {
					out = append(out, s)
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if s := strings.TrimSpace(x); s != "" {
				return []string{s}
			}
		}
	}
	return nil
}

func claimTrimIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if s := strings.TrimSpace(id); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// claimStr coerces a JSON value to its trimmed string form.
func claimStr(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}
}

// ---------------------------------------------------------------------------
// Document-level claim recall (Python navigation.py::_recall_claim_hits)
// ---------------------------------------------------------------------------

// DocClaimHit is one claim recalled for a SINGLE document, in the shape
// Python _recall_claim_hits returns to _render_toc_drilldown: the source
// chunk pointer, the fused rank/score, the statement and its verbatim
// evidence ([{"quote": ...}] when the claim carries one, else empty).
type DocClaimHit struct {
	ChunkID     string
	Score       float64
	Rank        int
	Name        string
	Description string
	Evidence    []map[string]any
}

// evidenceRowTypes mirrors Python _evidence_row_types: the entity_type_kwd
// values carrying evidence for the given compile kinds. Every compiler writes
// its evidence rows as "claim", so any known kind yields ["claim"]; with no
// kinds the drill searches ["claim"] outright. Unknown kinds contribute
// nothing (Python: the same map lookup misses).
func evidenceRowTypes(kinds []string) []string {
	if len(kinds) == 0 {
		return []string{"claim"}
	}
	for _, k := range kinds {
		switch k {
		case "tree", "raptor", "page_index":
			return []string{"claim"}
		}
	}
	return nil
}

// docClaimCacheEntry memoizes one (doc_id, query) claim recall. The drill calls
// it once per routed doc and the model re-issues navigate_structure, so the
// same pair recurs within a session; each miss costs two store round-trips.
type docClaimCacheEntry struct {
	at   time.Time
	hits []DocClaimHit
}

var (
	docClaimMu    sync.Mutex
	docClaimCache = map[string]docClaimCacheEntry{}
)

// RecallDocClaimHits runs the two-leg hybrid claim recall for ONE document and
// returns up to topN fused hits (Python navigation.py:_recall_claim_hits):
// a KNN leg over the claim rows' q_<dim>_vec (when qvec is available) and a
// BM25 leg over content_ltks/content_sm_ltks, fused by reciprocal rank. No
// similarity threshold: the store ranks, the top-N ARE the hit set. Empty —
// never an error — when the document has no compiled claims or the store
// fails; a failed recall simply leaves the drill-down in charge.
func RecallDocClaimHits(ctx context.Context, deps SearchDeps, query, docID string, kinds []string, qvec []float64, topN int) []DocClaimHit {
	if docID == "" || topN <= 0 {
		return nil
	}
	de := claimEngine(deps)
	if de == nil || deps.TenantID == "" {
		return nil
	}
	query = strings.TrimSpace(query)
	key := docID + "|" + strings.ToLower(query)
	now := time.Now()

	docClaimMu.Lock()
	if cached, ok := docClaimCache[key]; ok && now.Sub(cached.at).Seconds() < claimPrefetchCacheTTL {
		docClaimMu.Unlock()
		return cached.hits
	}
	if len(docClaimCache) >= claimPrefetchCacheCap {
		docClaimCache = map[string]docClaimCacheEntry{}
	}
	docClaimMu.Unlock()

	limit := topN
	if limit < claimRecallLegFloor {
		limit = claimRecallLegFloor
	}
	rowTypes := evidenceRowTypes(kinds)
	filter := map[string]interface{}{
		"doc_id":          []string{docID},
		"entity_type_kwd": rowTypes,
		// Doc-scope rows only: the Build button's KB-wide merged rows carry a
		// doc_id too, and without this filter they would answer for a single
		// document. Mirrors the dataset-level claim leg.
		"scope_kwd": []string{"doc"},
	}
	if len(kinds) > 0 {
		sorted := make([]string, 0, len(kinds))
		for _, k := range kinds {
			if k = strings.TrimSpace(k); k != "" {
				sorted = append(sorted, k)
			}
		}
		sort.Strings(sorted)
		filter["compile_kwd"] = sorted
	}
	fields := []string{"content_with_weight", "source_chunk_ids", "entity_type_kwd", "compile_kwd"}

	// KNN leg (needs a query vector); BM25 leg always runs.
	var denseLeg, textLeg []*ClaimHit
	if len(qvec) > 0 {
		exprs := []interface{}{&types.MatchDenseExpr{
			VectorColumnName:  fmt.Sprintf("q_%d_vec", len(qvec)),
			EmbeddingData:     qvec,
			EmbeddingDataType: "float",
			DistanceType:      "cosine",
			TopN:              limit,
		}}
		denseLeg = claimSearchLeg(ctx, de, deps, filter, fields, exprs, limit)
	}
	exprs := []interface{}{&types.MatchTextExpr{
		Fields:       []string{"content_ltks", "content_sm_ltks"},
		MatchingText: query,
		TopN:         limit,
	}}
	textLeg = claimSearchLeg(ctx, de, deps, filter, fields, exprs, limit)

	if len(denseLeg) == 0 && len(textLeg) == 0 {
		return nil
	}
	fused := rrfFuseClaims(denseLeg, textLeg)
	if len(fused) > topN {
		fused = fused[:topN]
	}
	out := make([]DocClaimHit, 0, len(fused))
	for _, h := range fused {
		hit := DocClaimHit{
			ChunkID:     "",
			Score:       h.Score,
			Rank:        h.Rank,
			Name:        h.Name,
			Description: h.Description,
			Evidence:    []map[string]any{},
		}
		if len(h.ChunkIDs) > 0 {
			hit.ChunkID = h.ChunkIDs[0]
		}
		// Python: description = h["description"] or h["name"].
		if hit.Description == "" {
			hit.Description = h.Name
		}
		if h.Quote != "" {
			hit.Evidence = []map[string]any{{"quote": h.Quote}}
		}
		out = append(out, hit)
	}

	docClaimMu.Lock()
	docClaimCache[key] = docClaimCacheEntry{at: now, hits: out}
	docClaimMu.Unlock()
	return out
}

// docClaimQuote returns the first non-empty verbatim quote of a hit's evidence
// list (Python: the `for ev in hit["evidence"]` loop).
func docClaimQuote(h DocClaimHit) string {
	for _, ev := range h.Evidence {
		if q, ok := ev["quote"].(string); ok {
			if s := strings.TrimSpace(q); s != "" {
				return s
			}
		}
	}
	return ""
}

// publishClaimHits mirrors the rendered claims of ONE document into the shared
// evidence pool (Python navigation.py::_publish_claim_hits): the SCA, the slot
// prefill and the final compose all read ONLY the pool, so a claim that
// already states the fact must land there. Pure addition: no retrieval is
// suppressed, entries dedup by the SAME "claim_"+md5(doc_id:name) id the
// session prefetch writes, so a claim found by either path is one entry.
// Returns how many NEW entries the pool gained.
func publishClaimHits(deps SearchDeps, hits []DocClaimHit, docID string) int {
	if deps.KB == nil || len(hits) == 0 {
		return 0
	}
	added := 0
	deps.KB.Admit(func(p *PoolAdmitter) {
		for _, h := range hits {
			name := strings.TrimSpace(h.Name)
			if name == "" {
				continue
			}
			cid := claimHitID(&ClaimHit{DocID: docID, Name: name})
			rank := "?"
			if h.Rank > 0 {
				rank = fmt.Sprint(h.Rank)
			}
			content := fmt.Sprintf("[claim #%s] %s", rank, name)
			if desc := strings.TrimSpace(h.Description); desc != "" && desc != name {
				content += " — " + desc
			}
			if quote := docClaimQuote(h); quote != "" {
				// Python [:_STRUCT_CLAIM_EVIDENCE_CHARS] counts CODE POINTS.
				quote := truncateRunes(docClaimQuote(h), ClaimEvidenceChars)
				content += "\nEvidence (verbatim): \"" + quote + "\""
			}
			// Python content[:1200] (navigation.py:1585-1600) counts CODE POINTS.
			content = truncateRunes(content, 1200)
			src := []string{}
			if h.ChunkID != "" {
				src = []string{h.ChunkID}
			}
			if p.Add(map[string]interface{}{
				"chunk_id":            cid,
				"content_with_weight": content,
				"doc_id":              docID,
				"source_chunk_ids":    src,
			}) {
				added++
			}
		}
	})
	if added > 0 {
		_LOG.Printf("[navigate_structure] published %d claim(s) to the evidence pool (doc=%s)", added, docID)
	}
	return added
}

// LoadChunksForIDs fetches source chunks by id, any owning document (Python
// navigation.py::_load_chunks_for_ids). Zero LLM, zero recall — a directed
// fetch used by the fan-out evidence top-up to pull the passages an admitted
// claim row cites. Empty on any failure: best effort by contract.
func LoadChunksForIDs(ctx context.Context, deps SearchDeps, ids []string) []map[string]any {
	ids = claimTrimIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	de := claimEngine(deps)
	if de == nil {
		return nil
	}
	res, err := de.Search(ctx, &types.SearchRequest{
		IndexNames:   []string{claimIndexName(deps)},
		KbIDs:        deps.KbIDs,
		Offset:       0,
		Limit:        len(ids),
		SelectFields: []string{"content_with_weight", "docnm_kwd", "doc_id"},
		Filter:       map[string]interface{}{"id": ids},
	})
	if err != nil || res == nil {
		return nil
	}
	out := make([]map[string]any, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		out = append(out, map[string]any{
			"chunk_id":            row["id"],
			"content_with_weight": row["content_with_weight"],
			"docnm_kwd":           row["docnm_kwd"],
			"doc_id":              row["doc_id"],
		})
	}
	return out
}

// mustEncodeQueries embeds one search query: query-side encoding when the
// embedder is asymmetric (Python tools.embed_mdl.encode_queries; NavQueryEmbedder
// for Cohere/Voyage/Jina/NVIDIA), plain Encode otherwise. Returns nil on
// failure — the BM25 leg alone keeps claim recall alive.
func mustEncodeQueries(deps SearchDeps, ctx context.Context, query string) [][]float32 {
	if deps.Embedder == nil {
		return nil
	}
	if qe, ok := deps.Embedder.(nlp.NavQueryEmbedder); ok {
		vecs, err := qe.EncodeQueries(ctx, deps.TenantID, []string{query})
		if err != nil {
			return nil
		}
		return vecs
	}
	vecs, err := deps.Embedder.Encode(ctx, deps.TenantID, []string{query})
	if err != nil {
		return nil
	}
	return vecs
}
