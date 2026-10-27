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

package agentic_rag

// navigate_structure: drill into documents' compiled structure (the
// entity/relation outline built at compile time) and return a query-focused
// <structure_navigation> outline with chunk pointers the model can feed to
// list_chunks.
//
// The structure is drilled with a keyword-relevance beam. Claim rows are NOT
// searched here: retrieval in this package is chunk retrieval, and a chunk's
// claims are fetched by the chunk, never by querying the claim rows.
//
// Typical use: call navigate_tree first to route to a document, take its
// doc_id, then call navigate_structure with that doc_id to see where the answer
// sits inside the document.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/service/nav"
)

const navigateStructureToolName = "navigate_structure"

const navigateStructureToolDescription = `Drill into documents' COMPILED STRUCTURE (the entity/relation outline built at compile time) and return a query-focused outline of where the answer sits, with chunk pointers you can pass to list_chunks to deep-read.
Call it with just a query: it routes the query over the datasets' compiled navigation trees itself, so no prior navigate_tree call is needed. To drill ONE document the model already chose, pass the doc_id from a <tree_navigation> result; that path returns the document's internal structure (entities and how they relate).
Returns an XML <structure_navigation> document: one <doc> element per readable structure, each with a <structure> outline of lines "- name (type): description [chunks: c1,c2]". Feed the chunk pointers to list_chunks (anchor_chunk_ids).
An empty result (<structure_navigation count="0" error="...">) means the routed documents have NO compiled structure of the requested kind - the error attribute says which: "route first with navigate_tree" (no doc_id), or "no compiled structure for this kind" (absent in THOSE documents - fall back to grep_chunks / search_*_chunks; the verdict is per document, so the tool stays available for other documents).`

// navigateStructureArgs is the JSON the model sends into InvokableRun.
type navigateStructureArgs struct {
	Query      string   `json:"query"`
	DocID      string   `json:"doc_id,omitempty"`
	DocIDs     []string `json:"doc_ids,omitempty"`
	Kind       string   `json:"kind,omitempty"`
	Keywords   string   `json:"keywords,omitempty"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
}

// Compiled-structure shape / kind constants, mirroring the harness navigation
// contract so the model sees the same signal shape.
var (
	// catalogKinds is the default structure-kind set read when kind is omitted.
	// It accepts the catalog/timeline shapes AND the raw graph/entity/relation
	// shapes so the default "show me this document's structure" reads every
	// compiled-shape row instead of silently dropping graph blobs.
	catalogKinds = map[string]bool{
		"tree": true, "timeline": true, "page_index": true, "pageindex": true,
		"graph": true, "entity": true, "relation": true, "knowledge_graph": true,
	}
	mindmapKinds = map[string]bool{"mindmap": true, "mind_map": true}
	// shapeKwds are the knowledge_graph_kwd ROW SHAPES a compiled structure can
	// be written as. Reading BOTH the compact "graph" blob and the per-entity /
	// per-relation rows is required: otherwise navigation silently returns EMPTY
	// for datasets compiled into per-entity/relation rows rather than a graph blob.
	shapeKwds = []string{"graph", "entity", "relation"}
)

// Drill-down bounds (mirroring the harness _STRUCT_* limits).
const (
	structMaxDepth     = 3   // max TOC levels drilled
	structMaxNodes     = 10  // cap on nodes rendered in the outline
	structBranchK      = 2   // keep top-K most relevant nodes per level
	structRelevanceMin = 1   // a node must match at least this many query terms to descend
	structDescSnippet  = 180 // cap on a node's description snippet
	structMaxChunks    = 4   // cap on chunk snippets returned in the outline
)

// navQueryRouteDocs bounds the documents a query-only call investigates. The
// two-step contract (navigate_tree -> navigate_structure) is a real adoption
// barrier: a query-only call removes it, at the cost of drilling a few routed
// documents instead of one chosen by the model.
const navQueryRouteDocs = 3

// navStructureRoute resolves candidate doc_ids for a query when the caller
// supplied no doc_id. It is the same navigation-tree router navigate_tree uses,
// so a query-only call and the explicit two-step path rank documents identically.
var navStructureRoute = func(ctx context.Context, tenantID, kbID, query string, limit int) ([][2]string, error) {
	return nav.NewNavServiceRouter().Route(ctx, tenantID, kbID, query, nil, limit)
}

// routeDocIDsForQuery routes the query across the bound datasets and returns the
// top doc_ids, deduped across datasets and capped at limit.
func routeDocIDsForQuery(ctx context.Context, tenantID, query string, datasetIDs []string, limit int) []string {
	if limit <= 0 {
		limit = navQueryRouteDocs
	}
	seen := map[string]bool{}
	var out []string
	for _, kbID := range datasetIDs {
		routed, err := navStructureRoute(ctx, tenantID, kbID, query, limit)
		if err != nil {
			common.DebugCtx(ctx, "navigate_structure: query route failed",
				zap.String("kb_id", kbID), zap.Error(err))
			continue
		}
		for _, pair := range routed {
			did := strings.TrimSpace(pair[0])
			if did == "" || seen[did] {
				continue
			}
			seen[did] = true
			out = append(out, did)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

// structureRel is a parent->child relation of a compiled structure hierarchy.
type structureRel struct {
	from    string
	to      string
	relType string
}

// structureNode is a node of a compiled structure hierarchy during drill-down.
type structureNode struct {
	name           string
	nodeType       string
	desc           string
	sourceChunkIDs []string
}

// chunkWithText pairs a chunk id with its text for structure snippet ranking.
type chunkWithText struct {
	id   string
	text string
}

// structureRow is one compiled-structure row: the doc-store fields the structure
// reader reads.
type structureRow struct {
	TemplateKind      string
	KnowledgeGraphKwd string
	Content           string
}

// structureReader reads a document's compiled structure rows. The production
// implementation reads from the doc-store; tests install a fake. Until a reader
// is installed (or the engine is available) navigate_structure reports
// EMPTY/no_structure, falling through to the text tools.
type structureReader interface {
	ReadStructure(ctx context.Context, tenantID, kbID, docID string) ([]structureRow, error)
}

var structReaderInst structureReader

// setStructureReader installs the (production or test) structure reader.
func setStructureReader(r structureReader) {
	structReaderInst = r
}

func getStructureReader() structureReader {
	if structReaderInst != nil {
		return structReaderInst
	}
	return &engineStructureReader{}
}

// engineStructureReader reads compiled-structure rows directly from the
// doc-store via engine.Get(). It mirrors the harness loadStructureGraph, but is
// scoped to ONE document (the tool already knows which document to drill).
type engineStructureReader struct{}

func (engineStructureReader) ReadStructure(ctx context.Context, tenantID, kbID, docID string) ([]structureRow, error) {
	de := engine.Get()
	if de == nil {
		return nil, nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:         []string{indexNameFor(tenantID, "")},
		KbIDs:              []string{kbID},
		SelectFields:       []string{"content_with_weight", "compilation_template_kind_kwd", "type_kwd", "knowledge_graph_kwd"},
		Filter:             map[string]interface{}{"doc_id": []string{docID}, "knowledge_graph_kwd": shapeKwds},
		Limit:              3000,
		IncludeUnavailable: true,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	rows := make([]structureRow, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		kg, _ := row["type_kwd"].(string)
		if kg == "" {
			kg, _ = row["knowledge_graph_kwd"].(string)
		}
		sr := structureRow{
			TemplateKind:      fmt.Sprint(row["compilation_template_kind_kwd"]),
			KnowledgeGraphKwd: kg,
			Content:           fmt.Sprint(row["content_with_weight"]),
		}
		rows = append(rows, sr)
	}
	return rows, nil
}

// navStringList coerces a doc-store column into a string slice: engines return
// []string, []interface{} or a bare string for the same field.
func navStringList(v any) []string {
	switch x := v.(type) {
	case []string:
		var out []string
		for _, s := range x {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		var out []string
		for _, item := range x {
			if s := strings.TrimSpace(stringFromAny(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if s := strings.TrimSpace(x); s != "" {
			return []string{s}
		}
	}
	return nil
}

// indexNameFor resolves the document/structure search index: the tenant's
// default "ragflow_<tenant_id>" unless a tenant overrides index_name.
func indexNameFor(tenantID, configured string) string {
	if configured != "" {
		return configured
	}
	return "ragflow_" + tenantID
}

// normalizeKindKey applies the API's kind normalization (page_index / graph ->
// timeline, mind_map -> mindmap).
func normalizeKindKey(kind string) string {
	switch strings.ToLower(kind) {
	case "page_index", "graph":
		return "timeline"
	case "mind_map":
		return "mindmap"
	}
	return strings.ToLower(kind)
}

// rowKind derives the normalized structure kind from a compiled-structure row,
// using its knowledge_graph_kwd (the row SHAPE: graph / entity / relation) or,
// failing that, its compilation template kind. This is what keeps a "graph"
// blob from being filtered out as the wrong kind.
func rowKind(row structureRow) string {
	kind := row.KnowledgeGraphKwd
	if kind == "" {
		kind = row.TemplateKind
	}
	return normalizeKindKey(kind)
}

// parseCompiledStructure merges the two row shapes (graph blob / per-entity /
// per-relation) into entities / relations, mirroring _query in navigation.py.
func parseCompiledStructure(rows []structureRow, kinds []string) ([]map[string]any, []map[string]any) {
	want := map[string]bool{}
	for _, k := range kinds {
		if k != "" {
			// Normalize the requested kind the same way rowKind normalizes the
			// row's own: without this "graph" (which rowKind maps to "timeline")
			// would be filtered out before its entities/relations are parsed.
			want[normalizeKindKey(k)] = true
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
		switch row.KnowledgeGraphKwd {
		case "graph":
			entities = append(entities, objectList(graph["entities"])...)
			relations = append(relations, objectList(graph["relations"])...)
		case "entity":
			entities = append(entities, graph)
		case "relation":
			relations = append(relations, graph)
		}
	}
	return entities, relations
}

// structureGraphFromRaw maps what parseCompiledStructure returns onto the typed
// nodes/relations the drill-down consumes (entity -> node conversion included so
// the drill-down only ever sees structureNode).
func structureGraphFromRaw(rawEntities, rawRels []map[string]any) ([]structureNode, []structureRel) {
	var out []structureNode
	for _, e := range rawEntities {
		name, _ := e["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		typ, _ := e["type"].(string)
		desc, _ := e["description"].(string)
		n := structureNode{name: name, nodeType: typ, desc: desc}
		if ids, ok := e["source_chunk_ids"].([]interface{}); ok {
			for _, id := range ids {
				if s, ok := id.(string); ok && s != "" {
					n.sourceChunkIDs = append(n.sourceChunkIDs, s)
				}
			}
		}
		out = append(out, n)
	}
	var rels []structureRel
	for _, r := range rawRels {
		pRaw, _ := r["from"].(string)
		cRaw, _ := r["to"].(string)
		p := strings.TrimSpace(pRaw)
		c := strings.TrimSpace(cRaw)
		if p == "" || c == "" || p == c {
			continue
		}
		relType, _ := r["type"].(string)
		rels = append(rels, structureRel{from: p, to: c, relType: relType})
	}
	return out, rels
}

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

// nodeRelevance mirrors _node_relevance: how many query terms occur in a node's
// name+description.
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

// buildTocTree mirrors _build_toc_tree: a name->node map, the parent->children
// map, the child->parent map (single parent, last-wins) and the root names.
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

// chunkPtrs mirrors _chunk_ptrs: a bounded, deduped comma-joined slice of an
// item's source chunk ids (<=8, the anchors the model sees).
func chunkPtrs(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	var seen []string
	for _, c := range ids {
		if c == "" || slices.Contains(seen, c) {
			continue
		}
		seen = append(seen, c)
		if len(seen) >= 8 {
			break
		}
	}
	return strings.Join(seen, ",")
}

// collectChunkIDs mirrors _collect_chunk_ids: a deduped, capped union of source
// chunk ids across a set of nodes.
func collectChunkIDs(nodes []structureNode, capN int) []string {
	if capN <= 0 {
		capN = 32
	}
	var out []string
	for _, n := range nodes {
		for _, c := range n.sourceChunkIDs {
			if c == "" || slices.Contains(out, c) {
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

// sortedKeptNames returns a kept-name set as a sorted slice (map iteration is
// randomised per process in Go, so sorting gives a stable, reproducible order).
func sortedKeptNames(names map[string]bool) []string {
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// drillKeptNodes descends from the TOC roots, keeping top-K children per level by
// keyword relevance, and returns the kept nodes, the single-parent map and the
// kept-name set.
func drillKeptNodes(terms []string, nodes []structureNode, rels []structureRel) ([]structureNode, map[string]string, map[string]bool) {
	byName, children, parents, roots := buildTocTree(nodes, rels)
	if len(roots) == 0 {
		return nil, nil, nil
	}
	frontier := roots
	keptNames := map[string]bool{}
	depth := 0
	for len(frontier) > 0 && depth <= structMaxDepth {
		type scored struct {
			score float64
			name  string
		}
		var scoredList []scored
		for _, n := range frontier {
			if e, ok := byName[n]; ok {
				scoredList = append(scoredList, scored{float64(nodeRelevance(terms, e.name, e.desc)), n})
			}
		}
		if len(scoredList) == 0 {
			break
		}
		sort.SliceStable(scoredList, func(i, j int) bool { return scoredList[i].score > scoredList[j].score })
		var top []scored
		for _, s := range scoredList {
			if s.score >= float64(structRelevanceMin) {
				top = append(top, s)
				if len(top) >= structBranchK {
					break
				}
			}
		}
		if len(top) == 0 {
			break
		}
		var next []string
		for _, s := range top {
			keptNames[s.name] = true
			next = append(next, children[s.name]...)
		}
		frontier = next
		depth++
	}
	// Ancestors of a kept node are pulled in so a path renders root -> node.
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
	return kept, parents, keptNames
}

// navTypeOr returns a node type, defaulting to "other" (a missing key reads as
// absent, not "<nil>").
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
		line += ": " + snippetRunes(desc, structDescSnippet)
	}
	if c := chunkPtrs(chunks); c != "" {
		line += " [chunks: " + c + "]"
	}
	return line
}

// renderOutline mirrors _render_outline: a compact flat outline of entities then
// relations (fallback when no query terms are usable).
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

// renderTocDrilldown drills a document's compiled structure toward the query: no
// usable query terms fall back to the flat outline, otherwise the tree is
// descended by keyword relevance. loader, when non-nil, fetches chunk texts so
// short snippets can be appended.
func renderTocDrilldown(query string, nodes []structureNode, rels []structureRel, loader func(ids []string) []chunkWithText) string {
	terms := queryTerms(query)
	if len(terms) == 0 {
		return renderOutline(nodes, rels)
	}
	kept, parents, keptNames := drillKeptNodes(terms, nodes, rels)
	if len(kept) == 0 {
		return renderOutline(nodes, rels)
	}
	// depth_of: node -> number of kept ancestors (rendering indentation).
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
	if loader != nil {
		wanted := collectChunkIDs(kept, 32)
		if len(wanted) > 0 {
			chunks := loader(wanted)
			capN := len(chunks)
			if capN > structMaxChunks {
				capN = structMaxChunks
			}
			for _, c := range chunks[:capN] {
				text := strings.TrimSpace(c.text)
				if text == "" {
					continue
				}
				lines = append(lines, "- [chunk "+c.id+"]: "+snippetRunes(text, 300))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// structureDocSegment renders one <doc> element (doc_id / entities / relations
// plus the <structure> outline).
func structureDocSegment(docID string, rank, entities, relations int, outline string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `  <doc rank="%d" doc_id="%s" entities="%d" relations="%d">`, rank, xmlEscape(docID), entities, relations)
	if outline != "" {
		b.WriteString("\n    <structure>" + xmlEscape(outline) + "</structure>")
	}
	b.WriteString("\n  </doc>")
	return b.String()
}

// navigateStructures renders the compiled structures of the given documents into
// a single <structure_navigation> with one <doc> per readable structure. A
// document without a compiled structure of the requested kind is skipped; an
// empty result (no doc carried a structure) reports no_structure.
func navigateStructures(ctx context.Context, tenantID string, query string, docIDs, datasetIDs []string, kind string, reader structureReader) string {
	if len(docIDs) == 0 {
		return navStructEmpty("route first with navigate_tree")
	}
	kinds := structureKindsFor(kind)
	var segs []string
	for _, did := range docIDs {
		outline, entities, relations, ok := readStructureDocCore(ctx, tenantID, query, did, kinds, datasetIDs, reader)
		if !ok {
			continue
		}
		segs = append(segs, structureDocSegment(did, len(segs)+1, entities, relations, outline))
	}
	if len(segs) == 0 {
		return navStructEmpty("no compiled structure for this kind")
	}
	parts := append([]string{
		fmt.Sprintf(`<structure_navigation count="%d" query="%s" kind="%s">`, len(segs), xmlEscape(query), xmlEscape(kind)),
	}, segs...)
	parts = append(parts, "</structure_navigation>")
	return strings.Join(parts, "\n")
}

// kindListOf flattens the requested structure kinds into the sorted list the
// structure reader parses. Rows are selected by compile_kwd, so every read needs
// it.
func kindListOf(kinds map[string]bool) []string {
	out := make([]string, 0, len(kinds))
	for k := range kinds {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// readStructureDocCore reads ONE document's compiled structure of the requested
// kind and drills it toward the query. It returns the rendered outline, the
// entity/relation counts, and whether the document carried a structure at all.
func readStructureDocCore(ctx context.Context, tenantID, query, docID string, kinds map[string]bool, datasetIDs []string, reader structureReader) (string, int, int, bool) {
	if query == "" || docID == "" {
		return "", 0, 0, false
	}
	kindList := kindListOf(kinds)

	var nodes []structureNode
	var rels []structureRel
	read := false
	for _, kbID := range datasetIDs {
		rows, err := reader.ReadStructure(ctx, tenantID, kbID, docID)
		if err != nil {
			common.DebugCtx(ctx, "navigate_structure: read failed",
				zap.String("kb_id", kbID), zap.String("doc_id", docID), zap.Error(err))
			continue
		}
		if len(rows) == 0 {
			continue
		}
		read = true
		entities, relations := parseCompiledStructure(rows, kindList)
		es, rs := structureGraphFromRaw(entities, relations)
		nodes = append(nodes, es...)
		rels = append(rels, rs...)
	}
	if !read || len(nodes) == 0 {
		return "", 0, 0, false
	}
	loader := func(ids []string) []chunkWithText { return structureNodeLoader(ctx, tenantID, ids) }
	return renderTocDrilldown(query, nodes, rels, loader), len(nodes), len(rels), true
}

// structureNodeLoader fetches chunk texts by id for the drill-down snippet pass.
func structureNodeLoader(ctx context.Context, tenantID string, ids []string) []chunkWithText {
	de := engine.Get()
	if de == nil || len(ids) == 0 {
		return nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:   []string{indexNameFor(tenantID, "")},
		SelectFields: []string{"content_with_weight", "docnm_kwd", "doc_id", "id"},
		Filter:       map[string]interface{}{"id": ids},
		Limit:        len(ids),
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil
	}
	out := make([]chunkWithText, 0, len(res.Chunks))
	for _, c := range res.Chunks {
		id := fmt.Sprint(c["id"])
		if id == "" {
			continue
		}
		out = append(out, chunkWithText{id: id, text: fmt.Sprint(c["content_with_weight"])})
	}
	return out
}

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

func decodeJSONObject(s string) (map[string]any, bool) {
	v := firstJSONValue(s)
	m, ok := v.(map[string]any)
	return m, ok
}

// firstJSONValue finds the first JSON object in s (tolerating surrounding prose or
// code fences) and returns it decoded.
func firstJSONValue(s string) any {
	start := strings.Index(s, "{")
	if start < 0 {
		return nil
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			// Inside a string literal a backslash escapes the next byte, so a
			// `\"` must not be read as the closing quote (which would flip the
			// in-string state and corrupt the brace depth - observed with entity
			// descriptions that carry escaped quotes).
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				raw := s[start : i+1]
				var v any
				if err := json.Unmarshal([]byte(raw), &v); err == nil {
					return v
				}
				return nil
			}
		}
	}
	return nil
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

// snippetRunes truncates s to at most n runes, appending an ellipsis when cut.
func snippetRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// navStructEmpty returns the empty <structure_navigation count="0"> XML carrying
// an optional error="..." attribute, mirroring the harness navigation contract.
func navStructEmpty(label string) string {
	attr := ""
	if label != "" {
		attr = ` error="` + xmlEscape(label) + `"`
	}
	return "<structure_navigation count=\"0\"" + attr + ">\n</structure_navigation>"
}

// NavigateStructureTool drills a document's compiled structure. It is stateless:
// the tenant and dataset scope are injected at construction from the session.
type NavigateStructureTool struct {
	tenantID   string
	datasetIDs []string
}

// NewNavigateStructureTool returns a NavigateStructureTool scoped to the given
// tenant and datasets, implementing eino's runtime.InvokableTool.
func NewNavigateStructureTool(tenantID string, datasetIDs []string) *NavigateStructureTool {
	return &NavigateStructureTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model.
func (g *NavigateStructureTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "REQUIRED: the question (or topic) to focus the structure drill on. Phrase it as the subject you are investigating."
    },
    "doc_id": {
      "type": "string",
      "description": "OPTIONAL: the document to drill, taken from a navigate_tree <doc doc_id> result. Omit it to route the query yourself and drill the best-matching documents' structure directly (no navigate_tree call needed)."
    },
    "doc_ids": {
      "type": "array",
      "description": "Optional explicit document ids to drill (at most 10). When omitted, doc_id is used; when both are omitted the query is routed.",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "kind": {
      "type": "string",
      "description": "Optional structure kind to read: \"catalog\" (default, tree/page_index/timeline), \"mindmap\", or \"graph\"/\"kg\"/\"entity\"/\"ontology\"."
    },
    "keywords": {
      "type": "string",
      "description": "Optional extra keywords that enrich the drill query without being required to match."
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict to (at most 10). When omitted, the current conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    }
  },
  "required": ["query"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("navigate_structure: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        navigateStructureToolName,
		Desc:        navigateStructureToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun drills the document structure and returns the XML
// <structure_navigation> document.
func (g *NavigateStructureTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, navigateStructureToolName, g.invokableRun, argumentsInJSON)
}

func (g *NavigateStructureTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args navigateStructureArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("navigate_structure: parse arguments: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("navigate_structure: query is required and must be a non-empty string")
	}
	// Entry trace: logged BEFORE the enablement check and the backend call, so
	// the tool-usage gate can tell "never called" from "called and bailed out".
	common.InfoCtx(ctx, "agentic_rag: navigate_structure called",
		zap.String("query", query), zap.String("kind", args.Kind),
		zap.Int("docs", len(args.DocIDs)))
	if !navToolEnabled(ctx, navigateStructureToolName) {
		// Marked unavailable for this conversation (bound datasets have no
		// compiled structure): skip the backend and return the same verdict the
		// caller falls back from.
		common.DebugCtx(ctx, "navigate_structure: marked unavailable; skipping backend (no structure)")
		return navStructEmpty("no structure"), nil
	}
	datasetIDs, err := resolveDatasetScope(g.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("navigate_structure: %w", err)
	}
	if len(datasetIDs) == 0 {
		return navStructEmpty("no bound datasets"), nil
	}

	reader := getStructureReader()

	text := strings.TrimSpace(query + " " + strings.TrimSpace(args.Keywords))

	docIDs := args.DocIDs
	if len(docIDs) == 0 && args.DocID != "" {
		docIDs = []string{args.DocID}
	}
	// A doc_id is optional: with none, route the query ourselves so the tool is
	// usable in one call.
	routedByQuery := false
	if len(docIDs) == 0 {
		// The router ranks the whole bound corpus, so a turn restricted to a set
		// of documents must intersect here.
		docIDs = intersectDocumentScope(ctx, routeDocIDsForQuery(ctx, g.tenantID, text, datasetIDs, navQueryRouteDocs))
		if len(docIDs) == 0 {
			return navStructEmpty("route first with navigate_tree"), nil
		}
		routedByQuery = true
	}
	if len(docIDs) > 10 {
		docIDs = docIDs[:10]
	}

	out := navigateStructures(ctx, g.tenantID, text, docIDs, datasetIDs, args.Kind, reader)
	// Per-call INFO: the tool-usage gate (kg_tool_calls) is read off the server
	// log, so a graph tool that rendered a structure without leaving a line here
	// cannot be told from one that was never called.
	common.InfoCtx(ctx, "agentic_rag: navigate_structure",
		zap.String("query", query),
		zap.Int("datasets", len(datasetIDs)),
		zap.Int("docs", len(docIDs)),
		zap.Bool("routed_by_query", routedByQuery),
		zap.Strings("names", docIDs),
		zap.Int("out_bytes", len(out)))
	return out, nil
}
