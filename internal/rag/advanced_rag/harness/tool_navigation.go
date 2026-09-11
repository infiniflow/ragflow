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
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

// Compiled-structure navigation: ontology_navigate / mindmap_navigate /
// navigate_tree / wiki_query.
//
// Mirrors Python harness/tools/navigation.py (the structure side of the module):
// structure navigation, the nav-tree router, the compiled-structure reader and
// dataset-tree LLM selection. The knowledge-graph walk (graph_explore) moved to
// exploration.go to match Python's exploration.py, which hosts both graph_explore
// and wiki_query; the shared seams (verdict struct, prompt, load helpers) stay
// here and are reached by those files through the harness package. In Go these
// were spread across several files (navigation / navtools / navservice /
// datasetnav / kg_explore); they are consolidated here.

// ---------------------------------------------------------------------------
// Structure navigation (ontology_navigate / mindmap_navigate)
// ---------------------------------------------------------------------------

var catalogKinds = map[string]bool{"tree": true, "timeline": true, "raptor": true, "page_index": true, "pageindex": true}
var mindmapKinds = map[string]bool{"mindmap": true, "mind_map": true}

const navSystemPrompt = `You are given the {noun} of one or more documents — an outline of entities and their relations — and a question.

Decide whether that outline alone already answers the question.

Rules:
1. Answer ONLY from the outline below. Do not invent facts.
2. Set "is_sufficient" to true only when the outline genuinely answers the question; otherwise false with an empty answer.
3. Always fill "relevant_entities" with the exact ` + "`name`" + ` values of the entities most related to the question (up to 10), even when the outline is not sufficient — they are used to pull the underlying source text.

Output ONLY JSON, no prose, no code fences:
{"is_sufficient": true/false, "answer": "<answer, or empty>", "relevant_entities": ["<entity name>", ...]}`

const (
	maxStructureEntities  = 300
	maxStructureRelations = 300
	maxEvidenceChunks     = 24
)

type structureEntity struct {
	Name           string    `json:"name"`
	Type           string    `json:"type"`
	Description    string    `json:"description"`
	SourceChunkIDs []string  `json:"source_chunk_ids"`
	DocID          string    `json:"-"`
	Vec            []float64 `json:"-"` // q_<dim>_vec row embedding, when the row carries one
}

type structureNavVerdict struct {
	IsSufficient     bool     `json:"is_sufficient"`
	Answer           string   `json:"answer"`
	RelevantEntities []string `json:"relevant_entities"`
}

// shapeKwds are the knowledge_graph_kwd ROW SHAPES a compiled structure can be
// written as. Reading BOTH the compact "graph" blob and the per-entity /
// per-relation rows is what mirrors Python _load_compiled_structure (which
// issues one query for the graph blob and a second for the per-entity rows and
// merges them). Otherwise navigation silently returns EMPTY for datasets that
// were compiled into per-entity/relation rows rather than a graph blob.
var shapeKwds = []string{"graph", "entity", "relation"}

// loadStructureGraph reads a document's compiled structure rows and splits them
// into both the entity list and the parent->child relations. Python merges the
// graph-blob and the per-entity/per-relation row shapes and feeds both entities
// and relations into _render_toc_drilldown / _build_toc_tree; mirroring that,
// navigation must carry relations through or the tree hierarchy (children,
// parents, roots, ancestors) collapses.
func loadStructureGraph(ctx context.Context, indexName, docID string, kinds map[string]bool, vecField string) ([]structureEntity, []structureRel) {
	de := engine.Get()
	if de == nil {
		return nil, nil
	}
	idx := indexName
	selectFields := []string{"content_with_weight", "compile_kwd", "compilation_template_kind_kwd", "knowledge_graph_kwd"}
	if vecField != "" {
		selectFields = append(selectFields, vecField)
	}
	req := &types.SearchRequest{
		IndexNames:   []string{idx},
		Filter:       map[string]interface{}{"doc_id": []string{docID}, "knowledge_graph_kwd": shapeKwds},
		SelectFields: selectFields,
		Limit:        3000,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil, nil
	}
	rows := make([]StructureRow, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		kg, _ := row["knowledge_graph_kwd"].(string)
		sr := StructureRow{
			CompileKwd:        fmt.Sprint(row["compile_kwd"]),
			TemplateKind:      fmt.Sprint(row["compilation_template_kind_kwd"]),
			KnowledgeGraphKwd: kg,
			Content:           fmt.Sprint(row["content_with_weight"]),
		}
		if vecField != "" {
			if vec, ok := parseFloats(row[vecField]); ok {
				sr.Vec = vec
			}
		}
		rows = append(rows, sr)
	}
	kindList := make([]string, 0, len(kinds))
	for k := range kinds {
		kindList = append(kindList, k)
	}
	rawEntities, rawRels := ParseCompiledStructure(rows, kindList)
	return structureGraphFromRaw(rawEntities, rawRels)
}

// structureGraphFromRaw maps what ParseCompiledStructure returns onto the typed
// entities/relations the drill-down consumes.
//
// A field the compiled payload omits must read as ABSENT, not as its Go
// rendering: fmt.Sprint(nil) is the non-empty string "<nil>", which survives
// navTypeOr's empty check and reaches the model as "- Name (<nil>): <nil>" with
// a phantom edge to "<nil>". Python renders "- Name (other)" and no description,
// because `(e.get("type") or "other")` / `(e.get("description") or "")` treat a
// missing key as falsy (navigation.py:1714-1715), and it DROPS a relation with a
// missing endpoint instead of keeping it (navigation.py:1783-1786). Leaving the
// field empty is also what lets the renderers apply Python's defaults —
// "other" for a type (navigation.py:1714), "related_to" for a relation type
// (navigation.py:1788).
func structureGraphFromRaw(rawEntities, rawRels []map[string]any) ([]structureEntity, []structureRel) {
	var out []structureEntity
	for _, e := range rawEntities {
		name, _ := e["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		typ, _ := e["type"].(string)
		desc, _ := e["description"].(string)
		se := structureEntity{
			Name:        name,
			Type:        typ,
			Description: desc,
		}
		if v, ok := e["_vec"].([]float64); ok {
			se.Vec = v
		}
		if ids, ok := e["source_chunk_ids"].([]interface{}); ok {
			for _, id := range ids {
				if s, ok := id.(string); ok && s != "" {
					se.SourceChunkIDs = append(se.SourceChunkIDs, s)
				}
			}
		}
		out = append(out, se)
	}
	var rels []structureRel
	for _, r := range rawRels {
		pRaw, _ := r["from"].(string)
		cRaw, _ := r["to"].(string)
		p := strings.TrimSpace(pRaw)
		c := strings.TrimSpace(cRaw)
		// Python _build_toc_tree skips empty or self-loop relations.
		if p == "" || c == "" || p == c {
			continue
		}
		relType, _ := r["type"].(string)
		rels = append(rels, structureRel{
			from:    p,
			to:      c,
			relType: relType,
		})
	}
	return out, rels
}

// parseFloats normalizes an engine vector cell — a []float64, or a []any of
// numbers (engine decoders may return either) — into []float64.
func parseFloats(v any) ([]float64, bool) {
	switch t := v.(type) {
	case []float64:
		return t, true
	case []any:
		out := make([]float64, 0, len(t))
		for _, item := range t {
			f, ok := item.(float64)
			if !ok {
				return nil, false
			}
			out = append(out, f)
		}
		return out, true
	}
	return nil, false
}

func normalizeKind(row map[string]interface{}) string {
	if ck, _ := row["compile_kwd"].(string); ck == "raptor_graph" {
		return "raptor"
	}
	kind, _ := row["compilation_template_kind_kwd"].(string)
	if kind == "" {
		kind, _ = row["compile_kwd"].(string)
	}
	kind = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(kind, "-", "_")))
	if kind == "pageindex" || kind == "page_index" || kind == "knowledge_graph" {
		return "timeline"
	}
	return kind
}

func loadChunksByIDs(ctx context.Context, indexName string, ids []string) []map[string]interface{} {
	if len(ids) == 0 {
		return nil
	}
	de := engine.Get()
	if de == nil {
		return nil
	}
	idx := indexName
	limit := maxEvidenceChunks
	if len(ids) < limit {
		limit = len(ids)
	}
	req := &types.SearchRequest{
		IndexNames:   []string{idx},
		Filter:       map[string]interface{}{"id": ids},
		SelectFields: []string{"content_with_weight", "docnm_kwd", "doc_id"},
		Limit:        limit,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	for _, row := range res.Chunks {
		out = append(out, map[string]interface{}{
			"chunk_id": row["id"], "content_with_weight": row["content_with_weight"],
			"docnm_kwd": row["docnm_kwd"], "doc_id": row["doc_id"],
		})
	}
	return out
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func orStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// indexNameFor resolves the document/structure search index. Python reads it
// from the dataset's search.index_name (navigation.py:_navigate_structure_impl); the RAGFlow default
// is "ragflow_<tenant_id>", so an empty configured name falls back to that —
// keeping Go behaviour identical unless a tenant overrides index_name.
func indexNameFor(tenantID, configured string) string {
	if configured != "" {
		return configured
	}
	return "ragflow_" + tenantID
}

// ---------------------------------------------------------------------------
// The routing seam + NavResult payload
// ---------------------------------------------------------------------------

// Compiled-navigation tools: locate (dataset tree routing) and drill
// (in-document structure pinpointing).
//
// Mirrors Python harness/tools/navigation.py:
//   - _navigate_tree_impl      (line 830)
//   - _nav_search_titled       (line 659)
//   - NavResult                (line 62)
//   - _load_compiled_structure (line 111)
//   - _normalize_kind          (line 101)
//
// These are the payload builders; the tool dispatch lives in tool_executor.go.

// Navigation constants (Python navigation.py:_NAV_SEARCH_MAX_DOCS, 656, 773).
const (
	// navSearchMaxDocs is how many documents the hybrid nav search routes to.
	navSearchMaxDocs = 12
	// navMinDocScore drops documents below this score.
	navMinDocScore = 0.2
	// navTreeMaxDocs caps the documents surfaced by navigate_tree.
	navTreeMaxDocs = 8
)

// NavResult mirrors Python NavResult: the structured outcome of ONE
// compiled-navigation call.
//
// Text is what the MODEL sees (XML, unchanged). The remaining fields are the
// signals the ORCHESTRATOR routes on: does this dataset have the structure at
// all, did THIS query reach anything, and was the result worth using? Without
// them the caller can only regex the XML — which is how a `count="1"
// entities="0"` empty shell used to read as a successful hit.
type NavResult struct {
	// Text is the XML the model consumes.
	Text string
	// DocIDs are the documents reached (the routing result).
	DocIDs []string
	// RoutedDocs are (doc_id, summary) pairs — the routed documents WITH their
	// overall summaries. The summary is the "hint" fed back into retrieval: nav
	// is a hint, not a constraint, so these become a soft boost/rerank signal
	// instead of a hard doc_scope filter.
	RoutedDocs [][2]string
	// Entities are compiled entities discovered across those documents.
	Entities []map[string]any
	// Relations are compiled relations discovered across those documents.
	Relations []map[string]any
	// ChunkPaths maps chunk_id -> root→chunk structure path.
	ChunkPaths map[string]string
	// EmptyReason is "" when the call succeeded, else the machine-readable cause:
	// "infra" (no backend), "bad_args" (missing query), "no_structure"
	// (dataset-level: no compiled structure of this kind here), "no_doc"
	// (query-level: structure exists but THIS query reached nothing).
	EmptyReason string
}

// HasStructure reports whether the dataset has the compiled structure at all.
// A query-level miss is not a structure absence.
func (n NavResult) HasStructure() bool { return n.EmptyReason != ReasonNoStructure }

// navEmpty returns an empty NavResult carrying Python's <tree_navigation> text:
// count="0" plus an error="<label>" attribute, omitted when label is empty —
// exactly the shapes navigation.py builds (:877 "no retriever", :880 "query is
// required", :899 no attribute at all for a query-level miss). That text is not
// what the model reads for an empty result — both sides substitute their own
// note for every empty_reason (action_session.py:869-876, tool_executor.go:402)
// — but the NavResult must still carry it. The status is derived from the reason
// by ReasonStatus.
func navEmpty(reason, label string) NavResult {
	attr := ""
	if label != "" {
		attr = ` error="` + XMLEscape(label) + `"`
	}
	return NavResult{
		EmptyReason: reason,
		Text:        "<tree_navigation count=\"0\"" + attr + ">\n</tree_navigation>",
	}
}

// ---------------------------------------------------------------------------
// The routing seam
// ---------------------------------------------------------------------------

// NavTreeRouter descends a dataset's compiled navigation tree and returns the
// routed documents.
//
// Mirrors Python's `search_dataset_layers(kb.id, tenant_id, query,
// "navigation_tree", top_k, doc_scope)` — a hybrid BFS beam descent (vector +
// BM25) from the root clusters down to the nav_doc leaves.
//
// The Go side exposes this through internal/service/nav's NavService.Search,
// which is the same KNN-over-nav-rows capability. It is an interface here (not a
// direct call) for two reasons: the harness must not import the service layer,
// and a dataset without a compiled tree must be distinguishable from a query
// that routed to nothing.
type NavTreeRouter interface {
	// Route descends the nav tree for one dataset and returns the routed
	// documents with their summaries, ordered by descending score.
	// Returns (nil, nil) when the dataset has no compiled tree at all.
	Route(ctx context.Context, tenantID, kbID, query string, docScope []string, topK int) ([][2]string, error)
}

// NavTreeInput is one navigate_tree call.
type NavTreeInput struct {
	Query    string
	Keywords string
	DocScope []string
	// TenantID and KbIDs bound the datasets to route over.
	TenantID string
	KbIDs    []string
}

// NavigateTree mirrors Python _navigate_tree_impl: locate the document(s) most
// likely to hold the answer by descending the compiled navigation tree.
//
// This tool ROUTES, it does not retrieve. It deliberately does NOT fetch
// document content: loading full documents here cost ~79 ES queries per document
// only to render a snippet that navigate_structure supersedes — under per-pass
// fan-out that alone was enough to knock ES over.
//
// Each nav row already carries the document's overall summary, so labelling the
// route costs ZERO extra queries.
//
// There is deliberately NO chunk-retrieval fallback: when routing misses, the
// caller falls back to retrieve/search_chunks — the same work, owned by the
// orchestrator instead of hidden inside a "route" call.
func NavigateTree(ctx context.Context, router NavTreeRouter, in NavTreeInput) NavResult {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return navEmpty(ReasonBadArgs, "query is required")
	}
	if router == nil {
		// Python's counterpart is a missing retriever, and the error label is its
		// string verbatim (navigation.py:877): the router IS the retrieval backend.
		return navEmpty(ReasonInfra, "no retriever")
	}
	// Descend on the topic + keywords: routing descends on similarity and does
	// not require keyword hits, so keywords only enrich the query.
	full := strings.TrimSpace(strings.TrimSpace(in.Query) + " " + strings.TrimSpace(in.Keywords))

	var (
		// ordered keeps the router's ranking: it returns documents in DESCENDING
		// score order, and that ordering IS the routing result. Re-sorting by
		// doc_id would discard the relevance the descent computed.
		ordered  [][2]string
		seen     = map[string]bool{}
		anyTree  bool
		anyError error
	)
	for _, kbID := range in.KbIDs {
		routed, err := router.Route(ctx, in.TenantID, kbID, full, in.DocScope, navSearchMaxDocs)
		if err != nil {
			anyError = err
			_LOG.Printf("[Dataset navigation search] nav-tree descent failed for kb=%s: %v", kbID, err)
			continue
		}
		if routed == nil {
			// No compiled tree for this dataset (distinguished from "routed to
			// nothing" by the router returning an empty non-nil slice).
			continue
		}
		anyTree = true
		for _, pair := range routed {
			did := strings.TrimSpace(pair[0])
			if did == "" || seen[did] {
				continue
			}
			seen[did] = true
			ordered = append(ordered, [2]string{did, strings.TrimSpace(pair[1])})
		}
	}
	if !anyTree {
		if anyError != nil {
			// A backend failure is an INFRASTRUCTURE error, not a dataset verdict:
			// no read succeeded, so "this dataset has no tree" would be a verdict
			// drawn from zero evidence — and Route() documents that contract
			// itself ("(nil, nil) when the dataset has no compiled tree at all",
			// so a non-nil error is a failure, not an absence). Python never
			// reaches no_structure from a failed descent either: its nav-tree
			// route yields only infra / bad_args / no_doc (navigation.py:877-899),
			// while no_structure belongs to the STRUCTURE path, where a successful
			// read found zero entities (:1046).
			return navEmpty(ReasonInfra, "nav tree descent failed")
		}
		// Every dataset lacks a compiled tree — a DATASET-level fact, so the
		// caller may disable the tool for the session.
		return navEmpty(ReasonNoStructure, "no compiled navigation tree")
	}
	if len(ordered) == 0 {
		// Structure exists but THIS query reached nothing: a query-level miss.
		// The dataset may still have a tree a better-formed query would hit.
		// No error attribute: Python's query-level miss carries none
		// (navigation.py:899), unlike infra/bad_args.
		return navEmpty(ReasonNoDoc, "")
	}

	// Cap after dedup, preserving the score order.
	if len(ordered) > navTreeMaxDocs {
		ordered = ordered[:navTreeMaxDocs]
	}
	docs := make([]string, 0, len(ordered))
	for _, pair := range ordered {
		docs = append(docs, pair[0])
	}

	var parts []string
	parts = append(parts, fmt.Sprintf(`<tree_navigation count="%d" query="%s">`, len(ordered), XMLEscape(query)))
	for i, pair := range ordered {
		if pair[1] != "" {
			parts = append(parts,
				fmt.Sprintf(`  <doc rank="%d" doc_id="%s">`, i+1, XMLEscape(pair[0])),
				fmt.Sprintf("    <summary>%s</summary>", XMLEscape(pair[1])),
				"  </doc>")
		} else {
			parts = append(parts, fmt.Sprintf(`  <doc rank="%d" doc_id="%s"/>`, i+1, XMLEscape(pair[0])))
		}
	}
	parts = append(parts, "</tree_navigation>")

	return NavResult{
		Text:       strings.Join(parts, "\n"),
		DocIDs:     docs,
		RoutedDocs: ordered,
	}
}

// ---------------------------------------------------------------------------
// Compiled-structure reading (in-document)
// ---------------------------------------------------------------------------

// StructureRow is one compiled-structure row, mirroring the doc-store fields
// Python reads (navigation.py:_load_compiled_structure).
type StructureRow struct {
	// CompileKwd distinguishes the COMPILE TYPE (tree / page_index / timeline /
	// raptor_graph / ...). NOT knowledge_graph_kwd.
	CompileKwd string
	// TemplateKind is the template-authored kind (compilation_template_kind_kwd).
	TemplateKind string
	// KnowledgeGraphKwd selects the ROW SHAPE:
	//   "graph"    → one compact blob whose content is {"entities":[], "relations":[]}
	//                (written by RAPTOR / tree compilation)
	//   "entity"   → one row per node, content is a single entity dict
	//   "relation" → one row per edge, content is a single relation dict
	//                (written by page_index and the pipeline Compiler tree)
	KnowledgeGraphKwd string
	// Content is the row's JSON payload.
	Content string
	// Vec is the row's q_<dim>_vec embedding, when the reader selected it. A
	// graph blob row carries the one vector its nested nodes share (RAPTOR); a
	// per-entity row carries that node's own vector (page_index).
	Vec []float64
}

// StructureReader reads a document's compiled structure rows.
//
// Mirrors Python _load_compiled_structure, which issues
// three doc-store queries (graph blob, per-entity/relation rows, raptor_graph)
// and merges the matching buckets.
//
// Go has no doc-store structured-query interface (see runtime.RetrievalService,
// which only exposes Search), so this is a seam: the harness ships the full
// parsing/merging logic below, and the reader is injected. Until a Go-side
// implementation is wired, navigate_structure reports EMPTY/no_structure and the
// navigation ladder falls through to `global` — the same behaviour as a dataset
// with no compiled structures.
type StructureReader interface {
	// ReadStructure returns the compiled rows for a document. Returns nil when
	// the backend has no compiled-structure support at all.
	ReadStructure(ctx context.Context, tenantID, kbID, docID string) ([]StructureRow, error)
}

// normalizeKind is defined above; rowKind projects a StructureRow into that
// shape. It mirrors Python _normalize_kind: the API's kind normalization
// (page_index / knowledge_graph → timeline), plus the raptor_graph override.
func rowKind(row StructureRow) string {
	return normalizeKind(map[string]any{
		"compile_kwd":                   row.CompileKwd,
		"compilation_template_kind_kwd": row.TemplateKind,
	})
}

// ParseCompiledStructure mirrors Python _load_compiled_structure's merge step
// (navigation.py:_query): keep rows whose compile TYPE is in kinds, then split
// by row shape into entities / relations.
//
// Two row shapes coexist for compiled structures, and a given compile type may
// produce either:
//
//	graph blob    (knowledge_graph_kwd="graph"):    nested entities/relations
//	per-entity    (knowledge_graph_kwd="entity"):   one node per row
//	per-relation  (knowledge_graph_kwd="relation"): one edge per row
//
// Reading BOTH and merging is what makes navigation work regardless of which
// shape a compile type produced.
func ParseCompiledStructure(rows []StructureRow, kinds []string) ([]map[string]any, []map[string]any) {
	want := map[string]bool{}
	for _, k := range kinds {
		if k != "" {
			want[k] = true
		}
	}
	var entities, relations []map[string]any
	for _, row := range rows {
		kind := rowKind(row)
		if len(want) > 0 && !want[kind] {
			continue
		}
		graph, ok := decodeJSONObject(row.Content)
		if !ok {
			continue
		}
		attachVec := func(list []map[string]any) {
			if len(row.Vec) == 0 {
				return
			}
			for _, m := range list {
				m["_vec"] = row.Vec
			}
		}
		switch row.KnowledgeGraphKwd {
		case "graph":
			es := objectList(graph["entities"])
			rs := objectList(graph["relations"])
			attachVec(es)
			attachVec(rs)
			entities = append(entities, es...)
			relations = append(relations, rs...)
		case "entity":
			attachVec([]map[string]any{graph})
			entities = append(entities, graph)
		case "relation":
			attachVec([]map[string]any{graph})
			relations = append(relations, graph)
		}
	}
	return entities, relations
}

func decodeJSONObject(s string) (map[string]any, bool) {
	v := ExtractJSON(s)
	m, ok := v.(map[string]any)
	return m, ok
}

func objectList(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// In-document structure drill-down (mirrors navigation.py, distributed helpers)
// ---------------------------------------------------------------------------
//
// These are the Go counterparts of the functions navigation.py DEFINES in its
// own body. Like Python, they reuse primitives imported from elsewhere: cosine /
// appendUnique (compiled_expansion.go, action_session.go), XMLEscape / Snippet /
// ChunkTextOf (chunk_utils.go). They stay here so navigation.go owns the
// navigate-tree / navigate-structure algorithm, mirroring how navigation.py owns
// its orchestration while importing its primitives.

// Structure drill-down bounds (mirror navigation.py's module-level limits).
const (
	structMaxDepth     = 12                             // _STRUCT_MAX_DEPTH: stop walking past this depth
	structMaxNodes     = 300                            // _STRUCT_MAX_NODES: cap on nodes kept per level
	structBranchK      = 12                             // _STRUCT_BRANCH_K: children kept per node
	structVecBeamRatio = 0.65                           // _STRUCT_VEC_BEAM_RATIO: score floor relative to best
	structRelevanceMin = 1                              // min keyword relevance to keep a node without a vector
	structDescSnippet  = 300                            // snippet length for a node's description in the outline
	structMaxChunks    = 6                              // _STRUCT_MAX_CHUNKS: top chunks exposed per drilled doc
	structCatalogKinds = "tree_node|page_index|section" // kinds eligible for catalog outline

	// structTocMaxDepth bounds the ancestor walk in renderTocDrilldown's
	// chunk-retrieval and llm_toc branches (Python _STRUCT_TOC_MAX_DEPTH). The
	// remaining _STRUCT_TOC_* limits gate the whole-TOC LLM pass, which Python
	// leaves off by default; they are not mirrored (they would be unreachable in
	// Go).
	structTocMaxDepth = 6

	// Chunk-index selection bounds for structures that cannot score themselves
	// (RAPTOR blob path). Mirror _STRUCT_RECALL_TOP_N and _STRUCT_MAX_CHUNK_HITS.
	structRecallTopN   = 24 // _STRUCT_RECALL_TOP_N: hybrid chunk hits recalled per document
	structMaxChunkHits = 8  // _STRUCT_MAX_CHUNK_HITS: retrieved chunks shown per drilled doc
)

// queryTerms mirrors navigation.py's regex tokenizer: lowercase coarse word
// tokens of length >= 2 (the BM25-ish keyword signal).
func queryTerms(q string) []string {
	if q == "" {
		return nil
	}
	lower := strings.ToLower(q)
	var out []string
	var cur []byte
	for i := 0; i < len(lower); i++ {
		c := lower[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			cur = append(cur, c)
			continue
		}
		if len(cur) >= 2 {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	if len(cur) >= 2 {
		out = append(out, string(cur))
	}
	return out
}

// nodeRelevance mirrors navigation.py _node_relevance: how many query terms occur
// in a TOC node's name+description.
func nodeRelevance(terms []string, name, desc string) int {
	if len(terms) == 0 {
		return 0
	}
	text := strings.ToLower(name + " " + desc)
	n := 0
	for _, t := range terms {
		if strings.Contains(text, t) {
			n++
		}
	}
	return n
}

// structureNode is a node of a compiled structure hierarchy during drill-down:
// the entity plus its optional embedding vector (for cosine scoring).
type structureNode struct {
	name           string
	nodeType       string
	desc           string
	sourceChunkIDs []string
	vec            []float64
}

// structureRel is a parent->child relation of a compiled structure hierarchy.
type structureRel struct {
	from    string
	to      string
	relType string
}

// buildTocTree mirrors navigation.py _build_toc_tree: a name->node map, the
// parent->children map, the child->parent map (single parent, last-wins) and the
// root names (tree nodes with no parent; isolated nodes are their own roots).
func buildTocTree(nodes []structureNode, rels []structureRel) (map[string]structureNode, map[string][]string, map[string]string, []string) {
	byName := map[string]structureNode{}
	for _, n := range nodes {
		if n.name != "" {
			byName[n.name] = n
		}
	}
	children := map[string][]string{}
	parents := map[string]string{}
	for _, r := range rels {
		p, c := r.from, r.to
		if p == "" || c == "" || p == c {
			continue
		}
		if _, ok := children[p]; !ok {
			children[p] = []string{}
		}
		dup := false
		for _, x := range children[p] {
			if x == c {
				dup = true
				break
			}
		}
		if !dup {
			children[p] = append(children[p], c)
		}
		parents[c] = p
	}
	var roots []string
	for n := range byName {
		if _, has := parents[n]; !has {
			roots = append(roots, n)
		}
	}
	if len(roots) == 0 {
		for n := range byName {
			if _, has := children[n]; !has {
				roots = append(roots, n)
			}
		}
	}
	if len(roots) == 0 {
		for n := range byName {
			roots = append(roots, n)
		}
	}
	return byName, children, parents, roots
}

// chunkPtrs mirrors navigation.py _chunk_ptrs: a bounded, deduped comma-joined
// slice of an item's source chunk ids (<=8, the anchors the model sees).
func chunkPtrs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	var seen []string
	for _, c := range ids {
		if c == "" {
			continue
		}
		if !sliceContains(seen, c) {
			seen = append(seen, c)
			if len(seen) >= 8 {
				break
			}
		}
	}
	return strings.Join(seen, ",")
}

func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// collectChunkIDs mirrors navigation.py _collect_chunk_ids: a deduped, capped
// union of source chunk ids across a set of nodes.
func collectChunkIDs(nodes []structureNode, capN int) []string {
	if capN <= 0 {
		capN = 32
	}
	var out []string
	for _, n := range nodes {
		for _, c := range n.sourceChunkIDs {
			if c == "" {
				continue
			}
			if sliceContains(out, c) {
				continue
			}
			out = append(out, c)
			if len(out) >= capN {
				return out
			}
		}
	}
	return out
}

// nodeScore mirrors navigation.py _node_score: cosine when a query vector and the
// node embedding both exist, else keyword relevance.
func nodeScore(qvec []float64, terms []string, n structureNode) float64 {
	if len(qvec) > 0 && len(n.vec) > 0 {
		return cosine(qvec, n.vec)
	}
	return float64(nodeRelevance(terms, n.name, n.desc))
}

// vecsEqual reports whether two node embeddings are element-wise equal, mirroring
// navigation.py _vecs_equal. Zero-length or differently-sized vectors are unequal.
func vecsEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasDistinctNodeVectors reports whether the nodes carry per-node embeddings
// rather than one shared vector. Mirrors navigation.py _has_distinct_node_vectors:
// a page_index / per-node tree has distinct node vectors, while a RAPTOR blob
// inherits the blob's one vector for every node. Returns false when fewer than
// two nodes carry a vector or all share the same one.
func hasDistinctNodeVectors(nodes []structureNode) bool {
	var first []float64
	haveFirst := false
	for _, n := range nodes {
		if len(n.vec) == 0 {
			continue
		}
		if !haveFirst {
			first = n.vec
			haveFirst = true
			continue
		}
		if !vecsEqual(first, n.vec) {
			return true
		}
	}
	return false
}

// nodesCoveringChunks returns the names of nodes whose source_chunk_ids cover any
// of chunkIDs, mirroring navigation.py _nodes_covering_chunks. Used only to give
// retrieved chunks a structural home; a chunk no node covers is still shown.
func nodesCoveringChunks(nodes []structureNode, chunkIDs []string) []string {
	if len(chunkIDs) == 0 {
		return nil
	}
	want := make(map[string]bool, len(chunkIDs))
	for _, c := range chunkIDs {
		if c != "" {
			want[c] = true
		}
	}
	var names []string
	for _, n := range nodes {
		name := strings.TrimSpace(n.name)
		if name == "" {
			continue
		}
		for _, c := range n.sourceChunkIDs {
			if c != "" && want[c] {
				names = append(names, name)
				break
			}
		}
	}
	return names
}

// recallChunkIDsInDoc mirrors navigation.py _recall_chunk_ids_in_doc: for a
// structure whose nodes share one blob vector (RAPTOR) it asks the retrieval
// backend for the document's most relevant ORIGINAL chunks (the backend already
// excludes compiled rows), and returns them as (id, score) hits. A nil
// retriever, empty query, or an error yields no hits.
func recallChunkIDsInDoc(ctx context.Context, retriever Retriever, query, docID, tenantID string, datasetIDs []string, topN int) []chunkHit {
	if retriever == nil || query == "" || docID == "" {
		return nil
	}
	if topN <= 0 {
		topN = structRecallTopN
	}
	chunks, err := retriever.Retrieve(ctx, RetrieveRequest{
		Query:      query,
		DatasetIDs: datasetIDs,
		DocScope:   []string{docID},
		TopN:       topN,
		TenantID:   tenantID,
	})
	if err != nil || len(chunks) == 0 {
		return nil
	}
	hits := make([]chunkHit, 0, len(chunks))
	for _, c := range chunks {
		id, _ := c["chunk_id"].(string)
		if id == "" {
			continue
		}
		score := 0.0
		if s, ok := c["similarity"].(float64); ok {
			score = s
		}
		hits = append(hits, chunkHit{id: id, score: score})
	}
	return hits
}

// drillKeptNodes mirrors navigation.py _drill_kept_nodes: vector-beam BFS descent
// from the TOC roots, keeping top-K children per level by cosine (relative beam
// threshold) or by keyword relevance (absolute minimum), returning the kept
// nodes, the single-parent map, the kept-name set and the overall best score.
func drillKeptNodes(qvec []float64, terms []string, nodes []structureNode, rels []structureRel) ([]structureNode, map[string]string, map[string]bool, float64) {
	byName, children, parents, roots := buildTocTree(nodes, rels)
	if len(roots) == 0 {
		return nil, nil, nil, 0.0
	}
	frontier := roots
	keptNames := map[string]bool{}
	depth := 0
	bestOverall := 0.0
	for len(frontier) > 0 && depth <= structMaxDepth {
		type scored struct {
			score float64
			name  string
		}
		var scoredList []scored
		for _, n := range frontier {
			e, ok := byName[n]
			if ok {
				scoredList = append(scoredList, scored{nodeScore(qvec, terms, e), n})
			}
		}
		if len(scoredList) == 0 {
			break
		}
		sort.SliceStable(scoredList, func(i, j int) bool { return scoredList[i].score > scoredList[j].score })
		best := scoredList[0].score
		if best > bestOverall {
			bestOverall = best
		}
		if len(qvec) == 0 && best < float64(structRelevanceMin) {
			break
		}
		var top []scored
		if len(qvec) > 0 {
			for _, s := range scoredList {
				if s.score >= best*structVecBeamRatio {
					top = append(top, s)
					if len(top) >= structBranchK {
						break
					}
				}
			}
		} else {
			for _, s := range scoredList {
				if s.score >= float64(structRelevanceMin) {
					top = append(top, s)
					if len(top) >= structBranchK {
						break
					}
				}
			}
		}
		if len(top) == 0 {
			break
		}
		var next []string
		for _, s := range top {
			if !keptNames[s.name] {
				keptNames[s.name] = true
			}
			next = append(next, children[s.name]...)
		}
		frontier = next
		depth++
	}
	// Order-independent (see drillWithAncestors): the ancestor closure is the same
	// SET whatever order the names are visited in.
	for name := range keptNames {
		cur := parents[name]
		guard := 0
		for cur != "" && !keptNames[cur] && guard < structMaxDepth {
			keptNames[cur] = true
			cur = parents[cur]
			guard++
		}
	}
	var kept []structureNode
	for _, n := range sortedKeptNames(keptNames) {
		if e, ok := byName[n]; ok {
			kept = append(kept, e)
		}
	}
	return kept, parents, keptNames, bestOverall
}

// outlineStats mirrors navigation.py _outline_stats: the entity / chunk-pointer
// counts for the flat fallback outline (no drill happened => top_score 0).
func outlineStats(nodes []structureNode) (nodeCount, ptrCount int) {
	count := len(nodes)
	if count > structMaxNodes {
		count = structMaxNodes
	}
	ptrs := 0
	for _, n := range nodes[:count] {
		ptrs += len(collectChunkIDs([]structureNode{n}, 32))
	}
	return count, ptrs
}

// navTypeOr returns a node type, defaulting to "other" as Python's
// “(e.get("type") or "other")“ does.
func navTypeOr(t string) string {
	if t == "" {
		return "other"
	}
	return t
}

// navOutlineLine renders one "- name (type): desc [chunks: c1,c2]" line.
func navOutlineLine(indent, name, nodeType, desc string, chunks []string) string {
	line := indent + "- " + name + " (" + navTypeOr(nodeType) + ")"
	if desc != "" {
		line += ": " + Snippet(desc, structDescSnippet)
	}
	if c := chunkPtrs(chunks); c != "" {
		line += " [chunks: " + c + "]"
	}
	return line
}

// renderOutline mirrors navigation.py _render_outline: a compact flat outline of
// entities then relations (fallback when no query terms are usable).
func renderOutline(nodes []structureNode, rels []structureRel) string {
	var lines []string
	capE, capR := 40, 40
	for i, n := range nodes {
		if i >= capE || n.name == "" {
			continue
		}
		lines = append(lines, navOutlineLine("", n.name, n.nodeType, n.desc, n.sourceChunkIDs))
	}
	for i, r := range rels {
		if i >= capR || r.from == "" || r.to == "" {
			continue
		}
		rt := r.relType
		if rt == "" {
			rt = "related_to"
		}
		lines = append(lines, "- "+r.from+" -["+rt+"]-> "+r.to)
	}
	return strings.Join(lines, "\n")
}

// rankChunksByTerms mirrors navigation.py _rank_chunks_by_terms: rank candidate
// chunks by how many query terms overlap with their text. Zero-LLM keyword
// relevance for the precise chunk_ids path. The accessor abstracts reading a
// chunk's text so the harness stays store-agnostic and testable.
func rankChunksByTerms(candidates []chunkWithText, queries []string) []chunkWithText {
	terms := make([]string, 0, 8)
	for _, q := range queries {
		for _, tok := range queryTerms(q) {
			if !sliceContains(terms, tok) {
				terms = append(terms, tok)
			}
		}
	}
	if len(terms) == 0 {
		return candidates
	}
	type scored struct {
		hits int
		c    chunkWithText
	}
	var scoredList []scored
	for _, c := range candidates {
		text := strings.ToLower(c.text)
		hits := 0
		for _, t := range terms {
			if strings.Contains(text, t) {
				hits++
			}
		}
		if hits > 0 {
			scoredList = append(scoredList, scored{hits, c})
		}
	}
	sort.SliceStable(scoredList, func(i, j int) bool { return scoredList[i].hits > scoredList[j].hits })
	out := make([]chunkWithText, 0, len(scoredList))
	for _, s := range scoredList {
		out = append(out, s.c)
	}
	return out
}

// chunkWithText pairs a chunk id with its text for structure snippet ranking.
type chunkWithText struct {
	id   string
	text string
}

// structureDrillout is the per-document result of the TOC drill-down, mirroring
// what Python's _render_toc_drilldown returns and the per-doc stats
// _navigate_structure_impl aggregates. selector records which strategy chose the
// nodes: llm_toc / chunk_retrieval / beam (stats["selector"] in Python).
type structureDrillout struct {
	outline    string
	nodes      int
	chunkPtrs  int
	topScore   float64
	chunkPaths map[string]string
	selector   string
}

// chunkHit is a retrieved chunk id with its retrieval score, the input shape of
// the chunk_retrieval strategy (Python chunk_hits: list[(chunk_id, score)]).
type chunkHit struct {
	id    string
	score float64
}

// sortedKeptNames returns a kept-name set as a sorted slice.
//
// The kept set is a map because it is used for membership tests, but three
// order-sensitive outputs are materialised from it: the rendered outline
// (kept[:structMaxNodes]), chunkPaths (first writer wins for a chunk covered by
// several nodes) and collectChunkIDs(kept, 32) — the chunk list whose snippets
// reach the model. Python iterates the kept_names SET (navigation.py:1660, 1673 and
// _drill_kept_nodes:1594), so its order is arbitrary and, because str hashing is
// randomised per process, not even reproducible across runs: there is no
// canonical order to mirror. Go randomises map iteration per range statement, so
// leaving it to the map would vary those three outputs even between two calls in
// one process; sorting gives a stable, machine-independent order instead.
func sortedKeptNames(names map[string]bool) []string {
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// drillWithAncestors pulls the ancestors of names into the kept set so a path
// renders as root -> ... -> node, mirroring _with_ancestors in _render_toc_drilldown.
func drillWithAncestors(names map[string]bool, parents map[string]string) map[string]bool {
	// Ranging a map while adding keys visits the new ones in an unspecified order,
	// but the result is order-independent: every start walks up to the root (or the
	// depth guard), so the SET is the same whatever order the names are visited in
	// — the same closure Python builds from its `list(names)` snapshot (:1648).
	for name := range names {
		cur := parents[name]
		guard := 0
		for cur != "" && !names[cur] && guard < structTocMaxDepth {
			names[cur] = true
			cur = parents[cur]
			guard++
		}
	}
	return names
}

// renderTocDrilldown mirrors navigation.py _render_toc_drilldown: it renders a
// query-focused outline of one document's compiled structure, choosing the nodes
// to show by one of three strategies in priority order:
//
//   - selected (non-empty): node names picked by the whole-TOC LLM pass, kept
//     whole with their ancestors — selector "llm_toc".
//   - chunkHits (non-empty): [(id, score)] from chunk retrieval (RAPTOR blob path);
//     the chunks are shown as retrieved and the tree only labels where they sit —
//     selector "chunk_retrieval".
//   - neither: VECTOR BEAM descent toward the query, falling back to keyword
//     overlap when no vectors are available — selector "beam".
//
// When no terms/vector and neither selection is provided it falls back to the flat
// outline. loader, when non-nil, fetches chunk texts so short snippets can be
// appended; a nil loader skips the snippet pass.
func renderTocDrilldown(query string, qvec []float64, nodes []structureNode, rels []structureRel, loader func(ids []string) []chunkWithText, chunkHits []chunkHit, selected []string) structureDrillout {
	flat := func() structureDrillout {
		out := renderOutline(nodes, rels)
		n, p := outlineStats(nodes)
		return structureDrillout{outline: out, nodes: n, chunkPtrs: p, topScore: 0.0, chunkPaths: map[string]string{}}
	}
	terms := queryTerms(query)
	// Pinned nodes (chunk retrieval / whole-TOC) stand even when the query yields
	// no usable terms or vector; bailing out here would discard a valid selection.
	if len(chunkHits) == 0 && len(selected) == 0 && len(terms) == 0 && len(qvec) == 0 {
		return flat()
	}

	byName, _, parents, _ := buildTocTree(nodes, rels)

	var kept []structureNode
	var keptNames map[string]bool
	var selectedIDs []string
	best := 0.0
	selector := "beam"

	switch {
	case len(selected) > 0:
		names := make(map[string]bool)
		for _, n := range selected {
			if _, ok := byName[n]; ok {
				names[n] = true
			}
		}
		keptNames = drillWithAncestors(names, parents)
		for _, n := range sortedKeptNames(keptNames) {
			if e, ok := byName[n]; ok {
				kept = append(kept, e)
			}
		}
		best = 0.0
		selector = "llm_toc"
	case len(chunkHits) > 0:
		seen := make(map[string]bool)
		for _, h := range chunkHits {
			if h.id != "" && !seen[h.id] {
				seen[h.id] = true
				selectedIDs = append(selectedIDs, h.id)
			}
		}
		covering := nodesCoveringChunks(nodes, selectedIDs)
		names := make(map[string]bool)
		for _, n := range covering {
			if _, ok := byName[n]; ok {
				names[n] = true
			}
		}
		keptNames = drillWithAncestors(names, parents)
		for _, n := range sortedKeptNames(keptNames) {
			if e, ok := byName[n]; ok {
				kept = append(kept, e)
			}
		}
		for _, h := range chunkHits {
			if h.score > best {
				best = h.score
			}
		}
		selector = "chunk_retrieval"
	default:
		kept, parents, keptNames, best = drillKeptNodes(qvec, terms, nodes, rels)
		selector = "beam"
	}
	if len(kept) == 0 && len(selectedIDs) == 0 {
		// A strategy was chosen but kept nothing; still report which one ran.
		out := flat()
		out.selector = selector
		return out
	}
	// depth_of: node -> number of kept ancestors (rendering indentation). The
	// iteration order does not matter here: each entry is a pure function of
	// (node, parents, keptNames), so the map comes out the same either way.
	depthOf := map[string]int{}
	for n := range keptNames {
		d := 0
		cur := parents[n]
		for cur != "" && keptNames[cur] {
			d++
			cur = parents[cur]
		}
		depthOf[n] = d
	}
	// root -> ... -> node path for each kept node, mapped onto its chunks.
	chunkPaths := map[string]string{}
	for _, e := range kept {
		name := strings.TrimSpace(e.name)
		if name == "" {
			continue
		}
		var segs []string
		segs = append(segs, name)
		cur := parents[name]
		guard := 0
		for cur != "" && guard < structMaxDepth {
			segs = append(segs, cur)
			cur = parents[cur]
			guard++
		}
		for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
			segs[i], segs[j] = segs[j], segs[i]
		}
		path := strings.Join(segs, " -> ")
		for _, cid := range e.sourceChunkIDs {
			if cid != "" {
				if _, seen := chunkPaths[cid]; !seen {
					chunkPaths[cid] = path
				}
			}
		}
	}
	var lines []string
	count := len(kept)
	if count > structMaxNodes {
		count = structMaxNodes
	}
	for _, e := range kept[:count] {
		name := strings.TrimSpace(e.name)
		if name == "" {
			continue
		}
		indent := strings.Repeat("  ", depthOf[name])
		lines = append(lines, navOutlineLine(indent, name, e.nodeType, e.desc, e.sourceChunkIDs))
	}
	// Chunk snippets. When chunk retrieval did the selecting, the hits are shown
	// as-is (filtering them back through the nodes would drop chunks no node
	// covers); otherwise take the chunks behind the drilled nodes.
	wanted := collectChunkIDs(kept, 32)
	if len(selectedIDs) > 0 {
		wanted = selectedIDs
	}
	if len(wanted) > 0 && loader != nil {
		chunks := loader(wanted)
		if len(chunks) > 0 {
			var ranked []chunkWithText
			limit := structMaxChunks
			if len(selectedIDs) > 0 {
				// Retrieval order is the relevance order; re-ranking by terms would
				// discard the score that selected them.
				pos := make(map[string]int, len(selectedIDs))
				for i, id := range selectedIDs {
					pos[id] = i
				}
				ranked = append(ranked, chunks...)
				sort.SliceStable(ranked, func(i, j int) bool { return pos[ranked[i].id] < pos[ranked[j].id] })
				limit = structMaxChunkHits
			} else {
				ranked = rankChunksByTerms(chunks, []string{query})
			}
			capN := len(ranked)
			if capN > limit {
				capN = limit
			}
			for _, c := range ranked[:capN] {
				text := strings.TrimSpace(c.text)
				if text == "" {
					continue
				}
				lines = append(lines, "- [chunk "+c.id+"]: "+Snippet(text, 300))
			}
		}
	}
	return structureDrillout{
		outline:    strings.Join(lines, "\n"),
		nodes:      len(kept),
		chunkPtrs:  len(wanted),
		topScore:   best,
		chunkPaths: chunkPaths,
		selector:   selector,
	}
}

// ---------------------------------------------------------------------------
// navigate_structure (zero-LLM vector-beam drill-down; mirrors _navigate_structure_impl)
// ---------------------------------------------------------------------------

// structureKindsFor maps a navigate_structure kind string to the compiled-kinds
// set to read, mirroring navigation.py _structure_kinds_for. Defaults to catalog.
func structureKindsFor(kind string) map[string]bool {
	k := strings.TrimSpace(strings.ToLower(kind))
	if k == "" {
		k = "catalog"
	}
	switch k {
	case "mindmap", "mind_map", "concept":
		out := make(map[string]bool, len(mindmapKinds))
		for kk := range mindmapKinds {
			out[kk] = true
		}
		return out
	case "graph", "kg", "entity", "ontology":
		return map[string]bool{"graph": true, "ontology": true, "entity": true, "raptor": true}
	default:
		out := make(map[string]bool, len(catalogKinds))
		for kk := range catalogKinds {
			out[kk] = true
		}
		return out
	}
}

// structureNodesFromEntities converts loaded structure entities into the
// drill-down node shape (no vectors unless filled in by a vector pass).
func structureNodesFromEntities(es []structureEntity) []structureNode {
	if len(es) == 0 {
		return nil
	}
	nodes := make([]structureNode, 0, len(es))
	for _, e := range es {
		if strings.TrimSpace(e.Name) == "" {
			continue
		}
		nodes = append(nodes, structureNode{
			name:           strings.TrimSpace(e.Name),
			nodeType:       e.Type,
			desc:           e.Description,
			sourceChunkIDs: append([]string(nil), e.SourceChunkIDs...),
			vec:            e.Vec,
		})
	}
	return nodes
}

// structureNodeLoader fetches chunk texts by id for the drill-down snippet pass,
// mirroring the chunk loader _render_toc_drilldown uses. Returns nil when the
// store/engine is not available (outline still renders, minus snippets).
func structureNodeLoader(ctx context.Context, indexName string, ids []string) []chunkWithText {
	raw := loadChunksByIDs(ctx, indexName, ids)
	if len(raw) == 0 {
		return nil
	}
	out := make([]chunkWithText, 0, len(raw))
	for _, c := range raw {
		id := ChunkIDOf(c)
		if id == "" {
			continue
		}
		out = append(out, chunkWithText{id: id, text: ChunkTextOf(c)})
	}
	return out
}

// readStructureDocCore reads ONE document's compiled structure of the requested
// kind and vector-beam drills it toward the query. It returns the per-document
// drill data (entities, relations, outline, chunk pointers) rather than the XML,
// so both the single-doc wrapper and the multi-doc router render the same doc as
// a <doc> segment. emptyReason == "" means the structure was read.
func readStructureDocCore(ctx context.Context, tenantID, query, docID, kind string, deps SearchDeps) (structureDrillout, []structureNode, []structureRel, string) {
	none := structureDrillout{chunkPaths: map[string]string{}}
	switch {
	case query == "":
		return none, nil, nil, ReasonBadArgs
	case docID == "":
		return none, nil, nil, ReasonNoDoc
	}
	// Python _load_compiled_structure → _resolve_doc_tenant: a document that is
	// not in the bound datasets yields an empty structure rather than an
	// unscoped read, so a stale doc_id cannot pull in another dataset's outline.
	if belongs, verified := docInDatasets(ctx, deps, docID); verified && !belongs {
		return none, nil, nil, ReasonNoStructure
	}
	kinds := structureKindsFor(kind)
	indexName := indexNameFor(tenantID, deps.IndexName)
	// Vector-aware load mirrors _read_structures: encode the query first so the
	// structure rows can be read with the q_<dim>_vec field, then judge whether
	// the nodes carry per-node embeddings.
	var qvec []float64
	var vecField string
	if vec := encodeSeedVector(ctx, deps, tenantID, query); vec != nil {
		qvec = vec
		vecField = "q_" + strconv.Itoa(len(vec)) + "_vec"
	}
	entities, rels := loadStructureGraph(ctx, indexName, docID, kinds, vecField)
	nodes := structureNodesFromEntities(entities)
	if len(nodes) == 0 {
		return none, nil, nil, ReasonNoStructure
	}
	loader := func(ids []string) []chunkWithText { return structureNodeLoader(ctx, indexName, ids) }

	// RAPTOR / shared-vector blobs cannot score their nodes against the query, so
	// they defer to chunk retrieval for the drill (mirrors Python routing them to
	// _recall_chunk_ids_in_doc). Per-node-vector structures keep the beam drill.
	var chunkHits []chunkHit
	if !hasDistinctNodeVectors(nodes) {
		chunkHits = recallChunkIDsInDoc(ctx, deps.Backend, query, docID, tenantID, deps.KbIDs, 0)
	}
	drill := renderTocDrilldown(query, qvec, nodes, rels, loader, chunkHits, nil)
	return drill, nodes, rels, ""
}

// structureDocSegment renders one <doc> element (doc_id / doc_title="" /
// entities / relations plus the <structure> outline), mirroring Python
// navigation._navigate_structure_impl where doc_title is always empty.
func structureDocSegment(docID, query, kind string, rank int, nodes []structureNode, rels []structureRel, drill structureDrillout) string {
	esc := func(s string) string { return XMLEscape(s) }
	var b strings.Builder
	fmt.Fprintf(&b, `  <doc rank="%d" doc_id="%s" doc_title="" entities="%d" relations="%d">`, rank, esc(docID), len(nodes), len(rels))
	if drill.outline != "" {
		b.WriteString("\n    <structure>" + esc(drill.outline) + "</structure>")
	}
	b.WriteString("\n  </doc>")
	return b.String()
}

// emptyReasonLabel maps an empty-reason constant to the XML error label Python
// writes into <structure_navigation error="...">.
func emptyReasonLabel(reason string) string {
	switch reason {
	case ReasonBadArgs:
		return "query is required"
	case ReasonNoDoc:
		return "no document located"
	default:
		return "no structure"
	}
}

// navigateStructures renders the compiled structures of MULTIPLE documents into a
// single <structure_navigation> with one <doc> per readable structure, mirroring
// Python _read_structures + _navigate_structure_impl: each document is read and
// drilled independently, documents without a compiled structure of the requested
// kind are skipped, and the resulting outline(s) are merged with per-doc ranks.
// The aggregate drillout carries summed node/pointer counts, the max top score,
// and the union of chunk paths across the documents.
// navigateStructures drills the requested compiled-structure kind across the
// given documents. When docIDs is empty it mirrors Python _navigate_structure_impl
// (:1012): with no doc_id it vector-routes the documents FIRST (search_dataset_layers /
// the nav tree) and then drills those — so a direct call need not rely on its
// caller pre-routing. The Go navigate_structure TOOL already routes before calling
// this, so in the tool path docIDs is non-empty and routing here is a no-op.
func navigateStructures(ctx context.Context, tenantID, query string, docIDs []string, kind string, router NavTreeRouter, deps SearchDeps) (NavResult, structureDrillout) {
	agg := structureDrillout{chunkPaths: map[string]string{}}
	var segs []string
	var docIDsSeen []string
	var entities []map[string]any
	if len(docIDs) == 0 {
		// Python _navigate_structure_impl with an empty doc scope vector-routes
		// the documents first; only if that also reaches nothing is it a MISS.
		if router != nil {
			// DocScope: the session ceiling, mirroring _nav_search_titled
			// (navigation.py:_navigate_structure_impl), which _navigate_structure_impl routes
			// through when doc_ids is empty (:1012).
			routed := NavigateTree(ctx, router, NavTreeInput{
				Query:    query,
				KbIDs:    deps.KbIDs,
				TenantID: tenantID,
				DocScope: deps.DocScope,
			})
			docIDs = dedupStrings(routed.DocIDs)
		}
		if len(docIDs) == 0 {
			return NavResult{
				Text:        `<structure_navigation count="0" error="no document located">` + "\n</structure_navigation>",
				EmptyReason: ReasonNoDoc,
			}, agg
		}
	}
	for _, did := range docIDs {
		drill, nodes, rels, empty := readStructureDocCore(ctx, tenantID, query, did, kind, deps)
		if empty != "" {
			// No compiled structure of this kind — Python's per-doc read skips it.
			continue
		}
		segs = append(segs, structureDocSegment(did, query, kind, len(segs)+1, nodes, rels, drill))
		docIDsSeen = append(docIDsSeen, did)
		entities = append(entities, entityMaps(nodes)...)
		agg.nodes += drill.nodes
		agg.chunkPtrs += drill.chunkPtrs
		if drill.topScore > agg.topScore {
			agg.topScore = drill.topScore
		}
		for cid, p := range drill.chunkPaths {
			if _, ok := agg.chunkPaths[cid]; !ok {
				agg.chunkPaths[cid] = p
			}
		}
	}
	if len(segs) == 0 {
		// doc_ids were given but none carried a compiled structure of this kind —
		// Python _navigate_structure_impl sets empty_reason="no_structure" when
		// total_entities == 0 (its count="0" <structure_navigation> carries no
		// <doc> elements).
		return NavResult{
			Text:        `<structure_navigation count="0" error="no structure">` + "\n</structure_navigation>",
			EmptyReason: ReasonNoStructure,
		}, agg
	}
	esc := func(s string) string { return XMLEscape(s) }
	parts := append([]string{
		fmt.Sprintf(`<structure_navigation count="%d" query="%s" kind="%s">`, len(segs), esc(query), esc(kind)),
	}, segs...)
	parts = append(parts, "</structure_navigation>")
	return NavResult{
		Text:        strings.Join(parts, "\n"),
		DocIDs:      docIDsSeen,
		Entities:    entities,
		ChunkPaths:  agg.chunkPaths,
		EmptyReason: "",
	}, agg
}

// entityMaps renders drill nodes back to the generic maps the orchestrator's
// NavResult.Entities slice carries.
func entityMaps(nodes []structureNode) []map[string]any {
	out := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, map[string]any{
			"name":             n.name,
			"type":             n.nodeType,
			"description":      n.desc,
			"source_chunk_ids": n.sourceChunkIDs,
		})
	}
	return out
}
