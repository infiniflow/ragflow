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

// navigate_structure: drill into ONE document's compiled structure (the
// entity/relation outline built at compile time) and return a query-focused
// <structure_navigation> outline with chunk pointers the model can feed to
// list_chunks.
//
// This is the experiment-grade port of the harness navigate_structure
// (internal/rag/agentic-rag/runtime/tool_navigation.go). The harness version
// was never wired to a Go-side structure reader in production, so this tool
// reads the compiled-structure rows directly from the doc-store and reuses the
// harness's keyword-relevance beam drill (its documented no-vector fallback).
// The dense-vector seed and claim-recall legs are intentionally omitted here;
// they are additive and can be layered on without changing the tool's contract.
//
// Typical use: call navigate_tree first to route to a document, take its
// doc_id, then call navigate_structure with that doc_id to see where the answer
// sits inside the document.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

const navigateStructureToolName = "navigate_structure"

const navigateStructureToolDescription = `Drill into ONE document's COMPILED STRUCTURE (the entity/relation outline built at compile time) and return a query-focused outline of where the answer sits, with chunk pointers you can pass to list_chunks to deep-read.
CALL IT AFTER navigate_tree has routed you to a document: take a doc_id from the <tree_navigation> result and pass it as doc_id. It shows the document's internal structure (entities and how they relate) instead of scanning chunks blindly.
Returns an XML <structure_navigation> document: one <doc> element per readable structure, each with a <structure> outline of "name (type): description [chunks: c1,c2]" lines and optional <claims_sufficient/>. Feed the chunk pointers to list_chunks (anchor_chunk_ids).
An empty result (<structure_navigation count="0" error="...">) means this document has NO compiled structure of the requested kind - the error attribute says which: "no compiled navigation tree" (route first with navigate_tree), "no document located" (no doc_id), or "no structure" (structure absent for this kind - fall back to grep_chunks / search_*_chunks).`

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

// Empty-reason constants, mirroring the harness navigation contract.
const (
	navStructReasonBadArgs     = "bad_args"
	navStructReasonNoStructure = "no_structure"
	navStructReasonNoDoc       = "no_doc"
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
	vec            []float64
}

// chunkWithText pairs a chunk id with its text for structure snippet ranking.
type chunkWithText struct {
	id   string
	text string
}

// structureDrillout is the per-document result of the TOC drill-down.
type structureDrillout struct {
	outline    string
	nodes      int
	chunkPtrs  int
	topScore   float64
	chunkPaths map[string]string
	selector   string
}

// StructureRow is one compiled-structure row: the doc-store fields the structure
// reader reads.
type StructureRow struct {
	CompileKwd        string
	TemplateKind      string
	KnowledgeGraphKwd string
	Content           string
	Vec               []float64
}

// structureReader reads a document's compiled structure rows. The production
// implementation reads from the doc-store; tests install a fake. Until a reader
// is installed (or the engine is available) navigate_structure reports
// EMPTY/no_structure, falling through to the text tools.
type structureReader interface {
	ReadStructure(ctx context.Context, tenantID, kbID, docID string) ([]StructureRow, error)
}

var structReaderInst structureReader

// SetStructureReader installs the (production or test) structure reader.
func SetStructureReader(r structureReader) {
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

func (engineStructureReader) ReadStructure(ctx context.Context, tenantID, kbID, docID string) ([]StructureRow, error) {
	de := engine.Get()
	if de == nil {
		return nil, nil
	}
	req := &enginetypes.SearchRequest{
		IndexNames:         []string{indexNameFor(tenantID, "")},
		KbIDs:              []string{kbID},
		SelectFields:       []string{"content_with_weight", "compile_kwd", "compilation_template_kind_kwd", "type_kwd", "knowledge_graph_kwd"},
		Filter:             map[string]interface{}{"doc_id": []string{docID}, "knowledge_graph_kwd": shapeKwds},
		Limit:              3000,
		IncludeUnavailable: true,
	}
	res, err := de.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	rows := make([]StructureRow, 0, len(res.Chunks))
	for _, row := range res.Chunks {
		kg, _ := row["type_kwd"].(string)
		if kg == "" {
			kg, _ = row["knowledge_graph_kwd"].(string)
		}
		sr := StructureRow{
			CompileKwd:        fmt.Sprint(row["compile_kwd"]),
			TemplateKind:      fmt.Sprint(row["compilation_template_kind_kwd"]),
			KnowledgeGraphKwd: kg,
			Content:           fmt.Sprint(row["content_with_weight"]),
		}
		rows = append(rows, sr)
	}
	return rows, nil
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

func normalizeKind(row map[string]interface{}) string {
	return normalizeKindKey(compilationKind(row))
}

func compilationKind(row map[string]interface{}) string {
	if v, ok := row["compilation_template_kind_kwd"].(string); ok && v != "" {
		return v
	}
	return fmt.Sprint(row["knowledge_graph_kwd"])
}

// rowKind derives the normalized structure kind from a compiled-structure row,
// using its knowledge_graph_kwd (the row SHAPE: graph / entity / relation) or,
// failing that, its compilation template kind. This is what keeps a "graph"
// blob from being filtered out as the wrong kind.
func rowKind(row StructureRow) string {
	kind := row.KnowledgeGraphKwd
	if kind == "" {
		kind = row.TemplateKind
	}
	return normalizeKindKey(kind)
}

// ParseCompiledStructure merges the two row shapes (graph blob / per-entity /
// per-relation) into entities / relations, mirroring _query in navigation.py.
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

// structureGraphFromRaw maps what ParseCompiledStructure returns onto the typed
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
		if v, ok := e["_vec"].([]float64); ok {
			n.vec = v
		}
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

// nodeScore mirrors _node_score: cosine when a query vector and the node
// embedding both exist, else keyword relevance. This port leaves nodes
// vector-less, so it always uses keyword relevance.
func nodeScore(qvec []float64, terms []string, n structureNode) float64 {
	if len(qvec) > 0 && len(n.vec) > 0 {
		return cosine(qvec, n.vec)
	}
	return float64(nodeRelevance(terms, n.name, n.desc))
}

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
	return dot / (sqrt(na) * sqrt(nb))
}

func sqrt(x float64) float64 {
	if x < 0 {
		return 0
	}
	r := x
	for i := 0; i < 16; i++ {
		if r == 0 {
			break
		}
		r = 0.5 * (r + x/r)
	}
	return r
}

func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
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

// collectChunkIDs mirrors _collect_chunk_ids: a deduped, capped union of source
// chunk ids across a set of nodes.
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

// drillWithAncestors pulls the ancestors of names into the kept set so a path
// renders as root -> ... -> node.
func drillWithAncestors(names map[string]bool, parents map[string]string) map[string]bool {
	for name := range names {
		cur := parents[name]
		guard := 0
		for cur != "" && !names[cur] && guard < structMaxDepth {
			names[cur] = true
			cur = parents[cur]
			guard++
		}
	}
	return names
}

// drillKeptNodes mirrors _drill_kept_nodes: vector-beam BFS descent from the TOC
// roots, keeping top-K children per level by keyword relevance (this port has no
// node vectors, so it always uses the keyword path), returning the kept nodes,
// the single-parent map, the kept-name set and the overall best score.
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
			if e, ok := byName[n]; ok {
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
			if !keptNames[s.name] {
				keptNames[s.name] = true
			}
			next = append(next, children[s.name]...)
		}
		frontier = next
		depth++
	}
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

// outlineStats mirrors _outline_stats: the entity / chunk-pointer counts for the
// flat fallback outline (no drill happened => top_score 0).
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
		line += ": " + Snippet(desc, structDescSnippet)
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

// renderTocDrilldown mirrors _render_toc_drilldown's flat + beam paths: when no
// terms/vector it falls back to the flat outline; otherwise it vector-beam
// descends toward the query by keyword relevance. loader, when non-nil, fetches
// chunk texts so short snippets can be appended; a nil loader skips the snippet
// pass.
func renderTocDrilldown(query string, qvec []float64, nodes []structureNode, rels []structureRel, loader func(ids []string) []chunkWithText) structureDrillout {
	flat := func() structureDrillout {
		out := renderOutline(nodes, rels)
		n, p := outlineStats(nodes)
		return structureDrillout{outline: out, nodes: n, chunkPtrs: p, topScore: 0.0, chunkPaths: map[string]string{}, selector: "flat"}
	}
	terms := queryTerms(query)
	if len(terms) == 0 && len(qvec) == 0 {
		return flat()
	}
	kept, parents, keptNames, best := drillKeptNodes(qvec, terms, nodes, rels)
	if len(kept) == 0 {
		out := flat()
		out.selector = "beam"
		return out
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
	if len(kept) > 0 && loader != nil {
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
				lines = append(lines, "- [chunk "+c.id+"]: "+Snippet(text, 300))
			}
		}
	}
	return structureDrillout{
		outline:    strings.Join(lines, "\n"),
		nodes:      len(kept),
		chunkPtrs:  len(kept),
		topScore:   best,
		chunkPaths: chunkPaths,
		selector:   "beam",
	}
}

// structureDocSegment renders one <doc> element (doc_id / entities / relations
// plus the <structure> outline).
func structureDocSegment(docID, query, kind string, rank int, nodes []structureNode, rels []structureRel, drill structureDrillout) string {
	esc := func(s string) string { return xmlEscape(s) }
	var b strings.Builder
	fmt.Fprintf(&b, `  <doc rank="%d" doc_id="%s" entities="%d" relations="%d">`, rank, esc(docID), len(nodes), len(rels))
	if drill.outline != "" {
		b.WriteString("\n    <structure>" + esc(drill.outline) + "</structure>")
	}
	b.WriteString("\n  </doc>")
	return b.String()
}

// emptyReasonLabel maps an empty-reason constant to the XML error label.
func emptyReasonLabel(reason string) string {
	switch reason {
	case navStructReasonBadArgs:
		return "query is required"
	case navStructReasonNoDoc:
		return "no document located"
	default:
		return "no structure"
	}
}

// navigateStructures renders the compiled structures of the given documents into
// a single <structure_navigation> with one <doc> per readable structure. A
// document without a compiled structure of the requested kind is skipped; an
// empty result (no doc carried a structure) reports no_structure.
func navigateStructures(ctx context.Context, tenantID string, query string, docIDs, datasetIDs []string, kind string, reader structureReader) (string, string) {
	if len(docIDs) == 0 {
		return navStructEmpty(navStructReasonNoDoc, "route first with navigate_tree"), navStructReasonNoDoc
	}
	kinds := structureKindsFor(kind)
	var segs []string
	var docIDsSeen []string
	for _, did := range docIDs {
		drill, nodes, rels, empty := readStructureDocCore(ctx, tenantID, query, did, kind, kinds, datasetIDs, reader)
		if empty != "" {
			continue
		}
		segs = append(segs, structureDocSegment(did, query, kind, len(segs)+1, nodes, rels, drill))
		docIDsSeen = append(docIDsSeen, did)
	}
	if len(segs) == 0 {
		return navStructEmpty(navStructReasonNoStructure, "no compiled structure for this kind"), navStructReasonNoStructure
	}
	esc := func(s string) string { return xmlEscape(s) }
	parts := append([]string{
		fmt.Sprintf(`<structure_navigation count="%d" query="%s" kind="%s">`, len(segs), esc(query), esc(kind)),
	}, segs...)
	parts = append(parts, "</structure_navigation>")
	return strings.Join(parts, "\n"), ""
}

// readStructureDocCore reads ONE document's compiled structure of the requested
// kind and drills it toward the query. It returns the per-document drill data,
// the nodes/rels, and an empty-reason ("" means read succeeded).
func readStructureDocCore(ctx context.Context, tenantID, query, docID, kind string, kinds map[string]bool, datasetIDs []string, reader structureReader) (structureDrillout, []structureNode, []structureRel, string) {
	none := structureDrillout{chunkPaths: map[string]string{}}
	switch {
	case query == "":
		return none, nil, nil, navStructReasonBadArgs
	case docID == "":
		return none, nil, nil, navStructReasonNoDoc
	}
	var nodes []structureNode
	var rels []structureRel
	anyStruct := false
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
		anyStruct = true
		var kindList []string
		for k := range kinds {
			kindList = append(kindList, k)
		}
		sort.Strings(kindList)
		entities, relations := ParseCompiledStructure(rows, kindList)
		es, rs := structureGraphFromRaw(entities, relations)
		nodes = append(nodes, es...)
		rels = append(rels, rs...)
	}
	if !anyStruct || len(nodes) == 0 {
		return none, nil, nil, navStructReasonNoStructure
	}
	loader := func(ids []string) []chunkWithText { return structureNodeLoader(ctx, tenantID, ids) }
	drill := renderTocDrilldown(query, nil, nodes, rels, loader)
	return drill, nodes, rels, ""
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
	v := ExtractJSON(s)
	m, ok := v.(map[string]any)
	return m, ok
}

// ExtractJSON finds the first JSON object in s (tolerating surrounding prose or
// code fences) and returns it decoded.
func ExtractJSON(s string) any {
	start := strings.Index(s, "{")
	if start < 0 {
		return nil
	}
	depth := 0
	inStr := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inStr = !inStr
			continue
		}
		if inStr {
			continue
		}
		switch c {
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

// Snippet truncates s to at most n runes, appending an ellipsis when cut.
func Snippet(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// navStructEmpty returns the empty <structure_navigation count="0"> XML carrying
// an optional error="..." attribute, mirroring the harness navigation contract.
func navStructEmpty(reason, label string) string {
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
      "description": "The document to drill. Take it from a navigate_tree <doc doc_id> result. Required unless doc_ids is given."
    },
    "doc_ids": {
      "type": "array",
      "description": "Optional explicit document ids to drill (at most 10). When omitted, doc_id is used.",
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
	if navToolDisabled(ctx, navigateStructureToolName) {
		// Session-level disable: this conversation's bound datasets were proven
		// to have no compiled structure, so skip the backend and return the same
		// verdict the caller falls back from.
		common.DebugCtx(ctx, "navigate_structure: session-disabled; skipping backend (no structure)")
		return navStructEmpty(navStructReasonNoStructure, "no structure"), nil
	}
	datasetIDs, err := resolveDatasetScope(g.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("navigate_structure: %w", err)
	}
	if len(datasetIDs) == 0 {
		return navStructEmpty(navStructReasonNoStructure, "no bound datasets"), nil
	}

	docIDs := args.DocIDs
	if len(docIDs) == 0 && args.DocID != "" {
		docIDs = []string{args.DocID}
	}
	if len(docIDs) == 0 {
		return navStructEmpty(navStructReasonNoDoc, "route first with navigate_tree"), nil
	}
	if len(docIDs) > 10 {
		docIDs = docIDs[:10]
	}

	reader := getStructureReader()
	if reader == nil {
		return navStructEmpty(navStructReasonNoStructure, "no structure reader installed"), nil
	}

	text := strings.TrimSpace(query + " " + strings.TrimSpace(args.Keywords))
	out, reason := navigateStructures(ctx, g.tenantID, text, docIDs, datasetIDs, args.Kind, reader)
	_ = reason
	return out, nil
}
