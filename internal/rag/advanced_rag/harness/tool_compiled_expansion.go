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

package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"

	"ragflow/internal/entity"
)

// Dense-match parameters for compiled-row search, mirroring Python
// compiled_expansion.py _VECTOR_NUM_CANDIDATES / _VECTOR_SIMILARITY. When a
// SearchCompiled implementation answers matchText with a dense (ANN) leg it MUST
// pass num_candidates = max(topN, vectorNumCandidates) and similarity =
// vectorSimilarity as named options — Python's PR #19172 fixed a bug where 0.1
// was passed positionally as num_candidates (ef_search), collapsing the HNSW
// candidate list and silently destroying recall. Go expresses dense options as a
// named map (engine MatchDenseExpr.ExtraOptions), so the pitfall is avoided as
// long as the implementation uses these constants.
const (
	vectorNumCandidates = 256
	vectorSimilarity    = 0.1
)

// CompiledStore is the seam over the doc-store compiled-structure rows that the
// harness must not own directly (it would pull in the retrieval backend and
// break unit-tier `go test ./...`). It mirrors the three primitives
// compiled_expansion.py reaches for:
//
//   - settings.docStoreConn.search   -> SearchCompiled
//   - source_chunk_ids loading       -> LoadChunks
//   - settings.retriever.get_vector  -> Vectorize (kept for callers that need it)
//
// The production implementation lives in the integration layer and is injected
// through SearchDeps.Expand (see NewCompiledExpander).
type CompiledStore interface {
	// SearchCompiled returns compiled rows (entity / relation / synthesis page)
	// matching the given filters and free-text match, scoped to (kbID, tenantID,
	// docIDs). docIDs is applied to the doc store as the scope (Python passes it
	// as a "doc_id" filter for entity/relation rows and "source_doc_ids" for
	// synthesis pages; the exact key is the implementation's concern). A row
	// carries at least content_with_weight / source_chunk_ids / doc_id / docnm_kwd
	// / from_entity_kwd / to_entity_kwd / name_kwd.
	//
	// filters keys mirror Python's condition dict: "knowledge_graph_kwd"
	// (kind: entity/relation), "compile_kwd" (tree/wiki_page/artifact_page/essence),
	// "compilation_template_kind_kwd", plus exact-list lookups ("name_kwd",
	// "from_entity_kwd", "to_entity_kwd") and "available_int". Each value is an
	// OR-list; filters are AND-ed.
	//
	// When matchText is answered with a dense (ANN) leg the implementation must
	// set its num_candidates = max(topN, vectorNumCandidates) and its similarity
	// threshold to vectorSimilarity (see the consts above).
	SearchCompiled(ctx context.Context, kbID, tenantID string, docIDs []string, filters map[string][]string, matchText string, topN int) ([]map[string]any, error)
	// LoadChunks returns the raw chunks for the given source_chunk_ids.
	LoadChunks(ctx context.Context, kbID, tenantID string, chunkIDs []string) ([]map[string]any, error)
	// Vectorize returns the embedding of text. Kept for the interface contract;
	// a faithful port assigns chunks a similarity and blends them by that field,
	// so expansion itself does not call Vectorize.
	Vectorize(ctx context.Context, text string) ([]float64, error)
}

// compiledScope is one (kb_id, tenant_id, doc_ids) triple. Mirrors Python
// tools._kg_scopes, which returns exactly that list for a (possibly empty)
// doc_scope.
type compiledScope struct {
	kbID     string
	tenantID string
	docIDs   []string
}

// Template-kind labels for the entity 1-hop strategies, in the exact order
// Python iterates them in _expand_with_compiled.
var compiledEntityKinds = []struct {
	label string
	kind  string
}{
	{"knowledge_graph", "knowledge_graph"},
	{"mind_map", "mind_map"},
	{"timeline", "timeline"},
	{"page_index", "page_index"},
}

// Synthesis compile keywords, in the exact order Python iterates them.
var compiledSynthesisKinds = []struct {
	label string
	kind  string
}{
	{"wiki_page", "wiki_page"},
	{"artifact_page", "artifact_page"},
	{"essence", "essence"},
}

// CompiledScopeConfig carries the scope-resolution inputs of one search
// session, mirroring the slice of Python's RAGTools that _kg_scopes
// (exploration.py:_kg_scopes) reads: the bound datasets (each with its OWN owner
// tenant), the request tenant used as a fallback, and the optional
// scoped_doc_ids / doc→(kb,tenant) hooks.
type CompiledScopeConfig struct {
	// DatasetIDs are the bound dataset ids (Python RAGTools.kb_ids).
	DatasetIDs []string
	// TenantID is the request tenant, used only when a bound dataset carries no
	// tenant id of its own.
	TenantID string
	// KBs are the resolved dataset objects; their (ID, TenantID) pairs give each
	// bound dataset its own compiled-row index scope (Python kb.tenant_id).
	KBs []*entity.Knowledgebase
	// DocTenantResolver resolves a document id to its owning (kb, tenant) so a
	// doc scope is grouped by real owner (Python tools._resolve_doc_tenant). Nil
	// keeps the fallback: each bound dataset searched with the whole doc scope.
	DocTenantResolver DocTenantResolver
}

// compiledExpander is the real implementation of CompiledExpander. It ports
// tools/compiled_expansion.py::_expand_with_compiled and its strategy helpers,
// with the doc-store side delegated to CompiledStore.
type compiledExpander struct {
	store CompiledStore
	cfg   CompiledScopeConfig
}

// NewCompiledExpander builds a CompiledExpander for one search session. A nil
// store, no bound dataset, or no fallback tenant yields a nil expander
// (expansion disabled), matching SearchDeps.Expand == nil.
func NewCompiledExpander(store CompiledStore, cfg CompiledScopeConfig) CompiledExpander {
	if store == nil || len(cfg.DatasetIDs) == 0 || strings.TrimSpace(cfg.TenantID) == "" {
		return nil
	}
	return &compiledExpander{store: store, cfg: cfg}
}

// Expand mirrors Python _expand_with_compiled: a zero-LLM enrichment that layers
// the dataset's compiled products on top of the retrieved chunks. For each bound
// KB it runs the per-template 1-hop entity expansion (knowledge_graph / mind_map
// / timeline / page_index), the tree structure graph (compile_kwd="tree"), and
// the three synthesis-page expansions (wiki_page / artifact_page / essence) —
// each capped at max_chunks=5 — then re-sorts the whole kbinfos["chunks"] by
// similarity so compiled chunks blend by relevance with the regular ones.
//
// A doc-tenant resolution failure aborts the expansion and is returned rather
// than swallowed: Python's per-doc lookup raises out of _expand_with_compiled
// (compiled_expansion.py:224) and the caller reports it (tool_search.go,
// "compiled expansion failed") while the already-retrieved chunks stay in place.
func (e *compiledExpander) Expand(ctx context.Context, kb *Kbinfos, query, keywords string, docScope []string) error {
	if e == nil || e.store == nil || kb == nil {
		return nil
	}
	before := len(kb.Chunks)
	seen := make(map[string]bool, len(kb.Chunks))
	for _, c := range kb.Chunks {
		if id := ChunkIDOf(c); id != "" {
			seen[id] = true
		}
	}
	match := strings.TrimSpace(query)
	if match == "" {
		match = strings.TrimSpace(keywords)
	}

	scopes := e.kgScopes(docScope)
	for _, sc := range scopes {
		// 1-hop entity-graph expansion, per template kind (Python L71-89):
		// compile_kwd empty, template_kind selects the template.
		for _, tk := range compiledEntityKinds {
			chunks, err := e.expandCompiledStrategy(ctx, sc, match, seen, "", tk.kind, 5)
			if err != nil {
				return err
			}
			if len(chunks) > 0 {
				kb.Chunks = append(kb.Chunks, chunks...)
				_LOG.Printf("[Compiled expand] %s: +%d chunks", tk.label, len(chunks))
			}
		}
		// Tree structure graph, selected by compile_kwd (Python L91-104):
		// template_kind empty, compile_kwd="tree".
		chunks, err := e.expandCompiledStrategy(ctx, sc, match, seen, "tree", "", 5)
		if err != nil {
			return err
		}
		if len(chunks) > 0 {
			kb.Chunks = append(kb.Chunks, chunks...)
			_LOG.Printf("[Compiled expand] tree: +%d chunks", len(chunks))
		}
		// Synthesis pages — standalone rendered articles, searched directly
		// (Python L106-125).
		for _, ck := range compiledSynthesisKinds {
			chunks, err := e.expandWikiPageStrategy(ctx, sc, match, seen, ck.kind, 5)
			if err != nil {
				return err
			}
			if len(chunks) > 0 {
				kb.Chunks = append(kb.Chunks, chunks...)
				_LOG.Printf("[Compiled expand] %s: +%d chunks", ck.label, len(chunks))
			}
		}
	}

	// Re-sort so compiled-expansion chunks blend by similarity with regular ones
	// (Python L127-130).
	if len(kb.Chunks) > 0 {
		sort.SliceStable(kb.Chunks, func(i, j int) bool {
			return similarityOf(kb.Chunks[i]) > similarityOf(kb.Chunks[j])
		})
	}

	_LOG.Printf("[Hybrid search] Compiled expansion added %d chunks.", len(kb.Chunks)-before)
	return nil
}

// kgScopes mirrors Python _kg_scopes by delegating to the
// same resolver graph_explore uses: with a doc scope the documents are grouped
// by their real owning (kb, tenant) — which may lie OUTSIDE the bound datasets —
// and without one each bound dataset is scanned under its OWN tenant. The doc
// scope arrives already ceilinged by resolveDocScope (Python scoped_doc_ids).
func (e *compiledExpander) kgScopes(docScope []string) []compiledScope {
	scopes := resolveKGScope(SearchDeps{
		KbIDs:             e.cfg.DatasetIDs,
		TenantID:          e.cfg.TenantID,
		KBs:               e.cfg.KBs,
		DocTenantResolver: e.cfg.DocTenantResolver,
	}, docScope, e.cfg.DatasetIDs)
	out := make([]compiledScope, 0, len(scopes))
	for _, sc := range scopes {
		out = append(out, compiledScope{kbID: sc.KBID, tenantID: sc.TenantID, docIDs: sc.Docs})
	}
	return out
}

// searchCompiledRows mirrors Python _search_compiled_rows for a single scope:
// a compiled-row search scoped to one (kb_id, tenant_id, doc_ids) with the given
// OR-list filters (including exact name_kwd / from_entity_kwd / to_entity_kwd
// lookups) and an optional free-text match.
func (e *compiledExpander) searchCompiledRows(ctx context.Context, sc compiledScope, matchText string, topN int, filters map[string][]string) []map[string]any {
	rows, err := e.store.SearchCompiled(ctx, sc.kbID, sc.tenantID, sc.docIDs, filters, matchText, topN)
	if err != nil {
		// Python logs the failed search and carries on with no rows
		// (compiled_expansion.py:211): the expansion is an enrichment, so a
		// missing compiled leg must not fail the regular retrieval.
		_LOG.Printf("[Compiled expand] compiled-row search failed (kb=%s tenant=%s); treating as no rows: %v", sc.kbID, sc.tenantID, err)
		return nil
	}
	return rows
}

// expandCompiledStrategy mirrors Python _expand_compiled_strategy: within one
// scope, embedding-match seed entities of a given template_kind/compile_kwd,
// hop 1-hop through adjacent relations (forward + backward), collect neighbour
// entity names, look up their source_chunk_ids by exact name_kwd, then load the
// referenced source chunks, deduped and capped at maxChunks.
func (e *compiledExpander) expandCompiledStrategy(ctx context.Context, sc compiledScope, match string, seen map[string]bool, compileKwd, templateKwd string, maxChunks int) ([]map[string]any, error) {
	// -- 1. Seed entities (embedding match, top 5) --
	seedRows := e.searchCompiledRows(ctx, sc, match, 5, seedFilters("entity", compileKwd, templateKwd))
	if len(seedRows) == 0 {
		return nil, nil
	}
	seedNames := make(map[string]bool)
	for _, r := range seedRows {
		if name := seedNameFromRow(r); name != "" {
			seedNames[name] = true
		}
	}
	if len(seedNames) == 0 {
		return nil, nil
	}

	// -- 2. Adjacent relations (outgoing + incoming), top 50 each --
	// Provide both original and lowercased names — dataset_structure_merger
	// lowercases merged-row endpoints while per-doc rows keep original case.
	seedList := lowerUnionSorted(seedNames)
	var allRels []map[string]any
	for _, relFilter := range []map[string][]string{
		{"from_entity_kwd": seedList},
		{"to_entity_kwd": seedList},
	} {
		f := seedFilters("relation", compileKwd, templateKwd)
		for k, v := range relFilter {
			f[k] = v
		}
		allRels = append(allRels, e.searchCompiledRows(ctx, sc, "", 50, f)...)
	}

	// -- 3. Neighbour names (1-hop, exclude seeds) --
	seedLower := make(map[string]bool, len(seedNames))
	for n := range seedNames {
		seedLower[strings.ToLower(n)] = true
	}
	neighbourNames := make(map[string]bool)
	for _, rel := range allRels {
		frm := strings.TrimSpace(compiledRelationEndpoint(rel, "from_entity_kwd"))
		to := strings.TrimSpace(compiledRelationEndpoint(rel, "to_entity_kwd"))
		if frm != "" && seedLower[strings.ToLower(frm)] && to != "" && !seedLower[strings.ToLower(to)] {
			neighbourNames[to] = true
		}
		if to != "" && seedLower[strings.ToLower(to)] && frm != "" && !seedLower[strings.ToLower(frm)] {
			neighbourNames[frm] = true
		}
	}
	if len(neighbourNames) == 0 {
		return nil, nil
	}

	// -- 4. Neighbour entity source_chunk_ids (exact name_kwd lookup) --
	neighList := lowerUnionSorted(neighbourNames)
	if len(neighList) > 100 {
		neighList = neighList[:100] // reasonable cap for name_kwd search
	}
	neighFilter := seedFilters("entity", compileKwd, templateKwd)
	neighFilter["name_kwd"] = neighList // exact name_kwd lookup (Python L361)
	neighRows := e.searchCompiledRows(ctx, sc, "", len(neighList), neighFilter)

	// -- 5. Group chunk ids by doc, load, dedupe, cap --
	byDoc := map[string][]string{}
	order := []string{}
	for _, r := range neighRows {
		docID := compiledDocID(r)
		for _, cid := range compiledSourceChunkIDs(r) {
			if cid == "" || seen[cid] {
				continue
			}
			if _, ok := byDoc[docID]; !ok {
				order = append(order, docID)
			}
			byDoc[docID] = append(byDoc[docID], cid)
		}
	}
	return e.loadByDoc(ctx, sc, order, byDoc, seen, maxChunks)
}

// expandWikiPageStrategy mirrors Python _expand_wiki_page_strategy: search the
// synthesis-compiled pages of one compile_kwd directly, collect their
// source_chunk_ids, then load the referenced source chunks. Pages rank high, so
// each loaded chunk is assigned similarity=0.9 unless it already carries one.
func (e *compiledExpander) expandWikiPageStrategy(ctx context.Context, sc compiledScope, match string, seen map[string]bool, compileKwd string, maxChunks int) ([]map[string]any, error) {
	// -- 1. Search synthesis pages (no knowledge_graph_kwd filter) --
	rows := e.searchSynthesisPages(ctx, sc, match, compileKwd, 5)
	if len(rows) == 0 {
		return nil, nil
	}

	// -- 2. Collect source_chunk_ids from matching pages --
	byDoc := map[string][]string{}
	order := []string{}
	for _, r := range rows {
		docID := compiledDocID(r)
		for _, cid := range compiledSourceChunkIDs(r) {
			if cid == "" || seen[cid] {
				continue
			}
			if _, ok := byDoc[docID]; !ok {
				order = append(order, docID)
			}
			byDoc[docID] = append(byDoc[docID], cid)
		}
	}
	if len(order) == 0 {
		return nil, nil
	}

	// -- 3. Load chunks, assign high similarity for priority ranking --
	loaded, err := e.loadByDoc(ctx, sc, order, byDoc, seen, maxChunks)
	if err != nil {
		return nil, err
	}
	for _, c := range loaded {
		if _, has := c["similarity"]; !has {
			c["similarity"] = 0.9 // wiki pages rank high
		}
	}
	return loaded, nil
}

// searchSynthesisPages mirrors Python _search_synthesis_pages: find synthesis
// page rows whose title/topic/content matches the query, filtered by compile_kwd
// and available_int=1. Synthesis pages are standalone articles that do NOT carry
// the knowledge_graph_kwd field, so no entity/relation filter is applied.
func (e *compiledExpander) searchSynthesisPages(ctx context.Context, sc compiledScope, matchText, compileKwd string, topN int) []map[string]any {
	return e.searchCompiledRows(ctx, sc, matchText, topN, map[string][]string{
		"compile_kwd":   {compileKwd},
		"available_int": {"1"},
	})
}

// loadByDoc loads source chunks grouped per doc within one scope, honoring a
// global maxChunks cap and the seen-id dedup set. Returns newly added chunks in
// doc order. Mirrors Python's per-doc _load_chunks_for_doc loop and its
// max_chunks break/limit accounting.
//
// A doc whose owner cannot be resolved is dropped (the documented per-row rule),
// but a doc-tenant resolution FAILURE is returned: it is a transport/database
// error, not "no row resolved", and Python surfaces it as an exception.
func (e *compiledExpander) loadByDoc(ctx context.Context, sc compiledScope, order []string, byDoc map[string][]string, seen map[string]bool, maxChunks int) ([]map[string]any, error) {
	// Python _load_chunks_for_doc resolves EACH row's doc_id to its owning
	// (kb, tenant) — not the searched scope — and loads nothing when that
	// resolution fails (merged dataset rows carry a pseudo/empty doc_id). Loading
	// from the scope instead would surface chunks Python deliberately drops, and
	// would query the wrong dataset for a row whose doc belongs elsewhere.
	owners := map[string]DocTenant{}
	if e.cfg.DocTenantResolver != nil {
		ids := make([]string, 0, len(order))
		for _, docID := range order {
			if docID != "" {
				ids = append(ids, docID)
			}
		}
		resolved, err := e.cfg.DocTenantResolver.ResolveDocTenants(ctx, ids)
		if err != nil {
			// Python resolves one doc at a time and lets a lookup failure raise out
			// of _load_chunks_for_doc (compiled_expansion.py:224): nothing is
			// loaded, and the failure is loud. Return it so the caller reports it
			// instead of letting a transport error masquerade as the documented
			// per-row drop below.
			return nil, fmt.Errorf("doc-tenant resolution failed for %d doc(s): %w", len(ids), err)
		}
		owners = resolved
	}
	var out []map[string]any
	for _, docID := range order {
		if len(out) >= maxChunks {
			break
		}
		kbID, tenantID := sc.kbID, sc.tenantID
		if e.cfg.DocTenantResolver != nil {
			owner, ok := owners[docID]
			if docID == "" || !ok || owner.KBID == "" {
				continue
			}
			kbID, tenantID = owner.KBID, owner.TenantID
		}
		cids := byDoc[docID]
		remaining := maxChunks - len(out)
		if len(cids) > remaining {
			cids = cids[:remaining]
		}
		rows, err := e.store.LoadChunks(ctx, kbID, tenantID, cids)
		if err != nil {
			// Python logs the failed load per doc (compiled_expansion.py:248) and
			// drops only that doc; the remaining docs still load.
			_LOG.Printf("[Compiled expand] failed to load chunks for doc_id=%s: %v", docID, err)
			continue
		}
		for _, c := range rows {
			id := ChunkIDOf(c)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, c)
		}
	}
	return out, nil
}

// seedNameFromRow mirrors Python L293-300: parse content_with_weight as JSON and
// read payload.name or payload.title (stripped).
func seedNameFromRow(row map[string]any) string {
	cww, _ := row["content_with_weight"].(string)
	if cww == "" {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(cww), &payload); err != nil {
		return ""
	}
	name := strings.TrimSpace(asString(payload["name"]))
	if name == "" {
		name = strings.TrimSpace(asString(payload["title"]))
	}
	return name
}

// lowerUnionSorted returns the sorted union of the names and their lowercased
// forms, mirroring Python `sorted({n.lower() for n in X} | X)`.
func lowerUnionSorted(names map[string]bool) []string {
	seen := make(map[string]bool, len(names)*2)
	for n := range names {
		if n == "" {
			continue
		}
		seen[n] = true
		seen[strings.ToLower(n)] = true
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func compiledDocID(row map[string]any) string {
	return asString(row["doc_id"])
}

// compiledRelationEndpoint reads a relation's from_entity_kwd/to_entity_kwd
// (which may be a bare string or a single-element list).
func compiledRelationEndpoint(row map[string]any, key string) string {
	if v, ok := row[key].(string); ok {
		return v
	}
	return asString(row[key])
}

func compiledSourceChunkIDs(row map[string]any) []string {
	switch v := row["source_chunk_ids"].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		return []string{v}
	}
	return nil
}

// similarityOf returns a chunk's similarity score as a float64 (Python
// c.get("similarity", 0.0)); nil/absent counts as 0.
func similarityOf(c map[string]any) float64 {
	switch v := c["similarity"].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0.0
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// seedFilters builds the entity/relation condition, mirroring Python
// _search_compiled_rows: {"knowledge_graph_kwd": [kind]} plus an optional
// compile_kwd ("tree") or compilation_template_kind_kwd selector.
func seedFilters(kind, compileKwd, templateKwd string) map[string][]string {
	f := map[string][]string{"knowledge_graph_kwd": {kind}}
	if compileKwd != "" {
		f["compile_kwd"] = []string{compileKwd}
	}
	if templateKwd != "" {
		f["compilation_template_kind_kwd"] = []string{templateKwd}
	}
	return f
}

// cosine returns the cosine similarity of two equal-length vectors. Shared with
// tool_navigation.go (navigation.py _node_score). Returns 0 on empty/mismatched
// input or when either vector is zero.
func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
