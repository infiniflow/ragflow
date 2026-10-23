package structure

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// ---- The hypernode / hyperedge layer ----
//
// OG-RAG's retrieval (Algorithm 1) consumes exactly two things: each hypernode's
// key embedding and value embedding (two independent top-k legs), and each
// hyperedge's membership (the input to greedy coverage). This file materializes
// both from the entity/relation rows the extractor already produced — see
// ontology.md §4.1, §4.2 and §5 step ④⑤.
//
// Vocabulary, fixed:
//
//	hypernode = one (path, value) pair, i.e. one assertion about one attribute
//	            path of one term. It is document-scoped: the same pair produced
//	            by two roots inside one document is the same hypernode (which is
//	            why a hypernode row carries no root identity — no name_kwd /
//	            entity_type_kwd — and roots reach it through hyperedge_ids),
//	            while two documents each keep their own row, because each can
//	            only compute its own membership.
//	hyperedge = one flattened factual block: a set of hypernodes sharing a root
//	            context. Its row payload IS the `{"key": "value", …}` dictionary
//	            OG-RAG's RAG_QUERY_PROMPT asks for.
//
// A hypernode becomes TWO rows, because the engine has exactly one vector
// column per row (`q_<dim>_vec`) while the algorithm needs the key and the value
// scored separately. Each row's payload holds only its own half, which is also
// what stops common.PayloadDescription from concatenating both halves into one
// mixed vector. The two rows share hypernode_hash and differ by the ":k"/":v"
// id suffix and node_role_kwd.

const (
	// hypernodeMaxDepth bounds the descent. The ontology's own chain length is
	// the real limit; this only stops a cyclic relation graph (A part_of B,
	// B part_of A) from expanding forever.
	hypernodeMaxDepth = 8

	// hyperedgeMaxDrafts bounds how many flattened blocks one document may
	// contribute. The depth bound alone does not bound the work: flatten emits one
	// draft per visited entity, and a wide graph multiplies breadth by depth, so a
	// document with a few hundred connected entities can produce tens of thousands
	// of drafts — each becoming a row, two hypernode rows and an embedding input.
	// The budget is global to the document, so it also bounds the sum over roots.
	hyperedgeMaxDrafts = 2000

	hypernodeKeyRole   = "key"
	hypernodeValueRole = "value"
)

// hypergraphTraversal accumulates one document's drafts under a shared budget.
// The budget lives here rather than in flatten's arguments because it has to hold
// across roots: a per-root bound would multiply with the root count.
type hypergraphTraversal struct {
	limit     int
	drafts    []hyperedgeDraft
	truncated bool
}

// add records one draft, reporting false once the budget is spent. The caller
// stops without descending further, so a truncated traversal costs no more than
// the drafts it kept.
func (t *hypergraphTraversal) add(draft hyperedgeDraft) bool {
	if len(t.drafts) >= t.limit {
		t.truncated = true
		return false
	}
	t.drafts = append(t.drafts, draft)
	return true
}

// full reports whether the budget is spent. It marks the traversal truncated,
// because a spent budget means something was left unvisited.
func (t *hypergraphTraversal) full() bool {
	if len(t.drafts) < t.limit {
		return false
	}
	t.truncated = true
	return true
}

// hypernodeFact is one (path, value) pair of a flattened factual block.
type hypernodeFact struct {
	// path is the mechanical attribute path, e.g.
	// crop:has_growing_zones.crop_growing_zone:name — no spaces, so it is a
	// single token in the whitespace-# analyzed path_kwd column.
	path string
	// key is the natural-language form of path, e.g.
	// "crop has growing zones crop growing zone name". It is what the key-side
	// vector embeds, so it has to read like the phrasing a question would use.
	key string
	// prop is the leaf attribute name (the last segment of path).
	prop string
	// val is the value exactly as the source states it.
	val string
}

// hyperedgeDraft is one flattened factual block, before ids are assigned.
type hyperedgeDraft struct {
	rootName  string
	rootClass string
	depth     int
	facts     []hypernodeFact
}

// hypergraphEdge is one outgoing object-property assertion of an entity.
type hypergraphEdge struct {
	prop   string
	target string
}

// hypergraphIndex is the lookup structure the descent walks. Every name key is
// lowercased, matching the name_kwd column and the graph's entity-name join key.
type hypergraphIndex struct {
	props    map[string]ontologyProperty        // declared property name -> spec
	attrs    map[string][]ontologyAttributeSpec // class -> datatype attributes (own + inherited)
	classOf  map[string]string                  // entity name -> class
	nameOf   map[string]string                  // entity name -> display name
	values   map[string]map[string]any          // entity name -> declared attribute values
	outEdges map[string][]hypergraphEdge        // entity name -> outgoing object assertions
	entityOf map[string]bool                    // entity name -> is an entity row
	targeted map[string]bool                    // entity name -> is some edge's target
}

// isObjectProperty reports whether a property joins two individuals, inferring
// the kind the same way the projection and the template validator do when the
// template omits it.
func isObjectProperty(p ontologyProperty, classes map[string]bool) bool {
	if p.kind != "" {
		return p.kind == "object"
	}
	for _, r := range p.ranges {
		if classes[r] {
			return true
		}
	}
	return false
}

// buildHypergraphIndex reads the compiled rows into the lookup the descent
// needs. It returns nil when the template declares no object properties, which
// is every non-ontology template — the caller then writes no extra rows.
func buildHypergraphIndex(prods []common.Product, parserConfig map[string]any) *hypergraphIndex {
	// Gate on the ontology template marker first: it is the switch documented in
	// ontology.md §10.7 ("only when the template declares config.ontology"), and
	// it keeps a stray domain/range declaration in some other variant from
	// quietly doubling that variant's row count.
	if !common.IsOntologyKind(stringOf(parserConfig["kind"])) {
		return nil
	}
	classes, props := declaredOntology(parserConfig)
	objectProps := map[string]ontologyProperty{}
	for name, p := range props {
		if isObjectProperty(p, classes) {
			objectProps[name] = p
		}
	}
	if len(objectProps) == 0 {
		return nil
	}

	idx := &hypergraphIndex{
		props:    objectProps,
		attrs:    ontologyAttributeSpecs(parserConfig),
		classOf:  map[string]string{},
		nameOf:   map[string]string{},
		values:   map[string]map[string]any{},
		outEdges: map[string][]hypergraphEdge{},
		entityOf: map[string]bool{},
		targeted: map[string]bool{},
	}
	for _, p := range prods {
		kind, _ := p.Meta["kind"].(string)
		switch kind {
		case "entity":
			name := strings.TrimSpace(stringOf(p.Meta["name"]))
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			idx.entityOf[key] = true
			idx.nameOf[key] = name
			idx.classOf[key] = strings.TrimSpace(stringOf(p.Meta["entity_type"]))
			if attrs, ok := p.Meta["attr"].(map[string]any); ok {
				idx.values[key] = attrs
			}
		case "relation":
			from := strings.TrimSpace(stringOf(p.Meta["from"]))
			to := strings.TrimSpace(stringOf(p.Meta["to"]))
			prop := strings.TrimSpace(stringOf(p.Meta["relation_type"]))
			if from == "" || to == "" || prop == "" {
				continue
			}
			fromKey, toKey := strings.ToLower(from), strings.ToLower(to)
			idx.targeted[toKey] = true
			// Only a declared object property contributes to the descent: a
			// template may also have left relation rows carrying vocabulary the
			// ontology does not declare, and those have no declared path to
			// descend along.
			if _, ok := idx.props[prop]; !ok {
				continue
			}
			idx.outEdges[fromKey] = append(idx.outEdges[fromKey], hypergraphEdge{
				prop:   prop,
				target: toKey,
			})
		}
	}
	if len(idx.entityOf) == 0 {
		return nil
	}
	for key := range idx.outEdges {
		sort.SliceStable(idx.outEdges[key], func(i, j int) bool {
			return idx.outEdges[key][i].prop < idx.outEdges[key][j].prop
		})
	}
	return idx
}

// roots returns the entities to flatten from: the ones no other entity points
// at through an object property. They are the flat-row equivalent of OG-RAG's
// top-level JSON-LD objects, so their sub-trees are not flattened a second time
// under an intermediate node. A cyclic relation graph leaves no such entity —
// then every entity is flattened instead, so the layer is never silently empty.
func (idx *hypergraphIndex) roots() []string {
	var roots, all []string
	for key := range idx.entityOf {
		all = append(all, key)
		if !idx.targeted[key] {
			roots = append(roots, key)
		}
	}
	sort.Strings(all)
	sort.Strings(roots)
	if len(roots) == 0 {
		return all
	}
	return roots
}

// nlKey turns a mechanical path into its natural-language phrasing by dropping
// the separators: crop:has_growing_zones.crop_growing_zone:name becomes
// "crop has growing zones crop growing zone name". Deliberately mechanical —
// the phrasing is the ontology's own terms, never a rewrite, so the key vector
// stays reproducible (ontology.md §4.3).
func nlKey(path string) string {
	return strings.Join(strings.Fields(strings.NewReplacer(":", " ", ".", " ", "_", " ").Replace(path)), " ")
}

// flattenValues expands one attribute value into the individual values the
// source states: a list contributes one entry per item, a scalar one entry.
// OG-RAG does the same when it flattens a 'List' namespace, and merging them
// would silently drop items.
func flattenValues(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s := strings.TrimSpace(stringOf(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s := strings.TrimSpace(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		if s := strings.TrimSpace(stringOf(v)); s != "" {
			return []string{s}
		}
		return nil
	}
}

// ownFacts returns the facts an entity contributes under the given path prefix:
// its own name, plus every declared datatype attribute it carries a value for.
// The prefix is empty for a root and, for a nested entity, the accumulated
// traversal segments ending in "." (e.g. "crop:has_growing_zones.").
func (idx *hypergraphIndex) ownFacts(key, prefix string) []hypernodeFact {
	cls := idx.classOf[key]
	if cls == "" {
		return nil
	}
	facts := make([]hypernodeFact, 0, 1+len(idx.attrs[cls]))
	namePath := prefix + cls + ":name"
	facts = append(facts, hypernodeFact{
		path: namePath,
		key:  nlKey(namePath),
		prop: "name",
		val:  idx.nameOf[key],
	})
	// idx.attrs is in template declaration order, so the facts of a block are
	// reproducible run to run.
	for _, a := range idx.attrs[cls] {
		for _, item := range flattenValues(idx.values[key][a.name]) {
			p := prefix + cls + ":" + a.name
			facts = append(facts, hypernodeFact{
				path: p,
				key:  nlKey(p),
				prop: a.name,
				val:  item,
			})
		}
	}
	return facts
}

// flatten walks the object-property graph from one root and emits one draft per
// visited entity: the root's own context first, then each descendant's facts
// appended to the ancestor context. That is OG-RAG's flatten_tree shape, where
// every node contributes the unit that terminates at it.
//
// seen is per-path, so a diamond (two properties reaching the same entity) still
// yields both paths while a cycle stops at the repeat instead of recursing.
//
// The traversal stops as soon as the shared budget is spent (see
// hypergraphTraversal), which bounds the whole document rather than one branch.
func (idx *hypergraphIndex) flatten(root string, facts []hypernodeFact, prefix string, depth int, seen map[string]bool, out *hypergraphTraversal) {
	if out.full() {
		return
	}
	cur := append(append([]hypernodeFact{}, facts...), idx.ownFacts(root, prefix)...)
	if !out.add(hyperedgeDraft{
		rootName:  idx.nameOf[root],
		rootClass: idx.classOf[root],
		depth:     depth,
		facts:     cur,
	}) {
		return
	}
	if depth >= hypernodeMaxDepth {
		return
	}
	for _, e := range idx.outEdges[root] {
		if seen[e.target] {
			continue
		}
		if out.full() {
			return
		}
		// The traversal segment becomes a path prefix for everything the target
		// contributes: <class>:<property>. — the term's class and the property
		// name, never an instance name, which is what makes the same hypernode
		// reachable from different roots (ontology.md §4.2).
		childPrefix := prefix + idx.classOf[root] + ":" + e.prop + "."
		next := make(map[string]bool, len(seen)+1)
		for k := range seen {
			next[k] = true
		}
		next[e.target] = true
		idx.flatten(e.target, cur, childPrefix, depth+1, next, out)
	}
}

// flattenDrafts walks every root under one budget shared by the whole document,
// so a wide first root cannot leave the later roots with a fresh allowance.
func (idx *hypergraphIndex) flattenDrafts(limit int) *hypergraphTraversal {
	t := &hypergraphTraversal{limit: limit}
	for _, root := range idx.roots() {
		// The budget is shared, so a root is skipped once an earlier one spent it.
		if t.full() {
			break
		}
		idx.flatten(root, nil, "", 0, map[string]bool{root: true}, t)
	}
	return t
}

// hyperedgePayload renders one draft as OG-RAG's dictionary form. Keys are the
// natural-language paths; a repeated key (the same attribute asserted twice with
// different values within one path) keeps the first and appends nothing, because
// the dictionary is the retrieval unit's identity.
func hyperedgePayload(d hyperedgeDraft) map[string]any {
	payload := make(map[string]any, len(d.facts))
	for _, f := range d.facts {
		if _, taken := payload[f.key]; taken {
			continue
		}
		payload[f.key] = f.val
	}
	return payload
}

// buildHypergraph returns the hyperedge and hypernode rows for prods. A nil
// index (no declared object properties) returns no rows at all, so every
// non-ontology template is untouched.
//
// Row ids: both families are document-scoped. A hyperedge's payload is one
// document's block; a hypernode is keyed on (path, value) *within* a document,
// so the same pair from two roots collapses into one row while the same pair
// from two documents does not — membership is computed per document, and the
// writer replaces whole rows.
func buildHypergraph(ctx context.Context, deps common.Deps, cfg CompileConfig, prods []common.Product) ([]common.Product, error) {
	idx := buildHypergraphIndex(prods, cfg.ParserConfig)
	if idx == nil {
		return nil, nil
	}

	traversal := idx.flattenDrafts(hyperedgeMaxDrafts)
	drafts := traversal.drafts
	if traversal.truncated {
		// Silent truncation reads as a complete graph, so the cut is reported where
		// an operator already watches progress: the document's output is partial,
		// and the fix is a narrower ontology rather than a longer ingest.
		runtime.ReportProgressMessage(ctx, "Compiler", fmt.Sprintf(
			"hypergraph staged %d blocks for doc %s and stopped at the %d-block budget; the remaining roots were not walked",
			len(drafts), cfg.DocID, hyperedgeMaxDrafts))
	}
	if len(drafts) == 0 {
		return nil, nil
	}

	// ---- hyperedges ----
	type edgeRow struct {
		id        string
		payload   map[string]any
		draft     hyperedgeDraft
		chunkIDs  []string
		rootLower string
	}
	edges := make([]edgeRow, 0, len(drafts))
	// hypernodeKey identifies a hypernode across every hyperedge.
	type hypernodeAcc struct {
		fact    hypernodeFact
		edgeIDs []string
		chunks  []string
		depth   int
	}
	nodes := map[string]*hypernodeAcc{}
	var nodeOrder []string

	byName := map[string][]string{} // entity name -> source chunk ids
	for _, p := range prods {
		if kind, _ := p.Meta["kind"].(string); kind != "entity" {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(stringOf(p.Meta["name"])))
		if name == "" {
			continue
		}
		byName[name] = append(byName[name], metaStrings(p.Meta, "source_chunk_ids")...)
	}

	for _, d := range drafts {
		payload := hyperedgePayload(d)
		content := payloadJSON(payload)
		idParts := []string{content, cfg.DocID}
		if cfg.TemplateID != "" {
			idParts = append(idParts, cfg.TemplateID)
		}
		rootLower := strings.ToLower(d.rootName)
		edges = append(edges, edgeRow{
			id:        common.StableRowID(idParts...),
			payload:   payload,
			draft:     d,
			chunkIDs:  byName[rootLower],
			rootLower: rootLower,
		})
	}

	// ---- hypernodes, with their hyperedge membership ----
	for _, e := range edges {
		for _, f := range e.draft.facts {
			key := f.path + "\x00" + f.val
			acc, ok := nodes[key]
			if !ok {
				acc = &hypernodeAcc{fact: f, depth: e.draft.depth}
				nodes[key] = acc
				nodeOrder = append(nodeOrder, key)
			}
			acc.edgeIDs = append(acc.edgeIDs, e.id)
			acc.chunks = append(acc.chunks, e.chunkIDs...)
		}
	}
	// Stable order so the node rows are reproducible, and sorted membership so a
	// hypernode's id list does not depend on the order the roots were walked.
	sort.Strings(nodeOrder)
	for _, key := range nodeOrder {
		acc := nodes[key]
		sort.Strings(acc.edgeIDs)
		acc.edgeIDs = dedupeStrings(acc.edgeIDs)
		acc.chunks = dedupeStrings(acc.chunks)
	}

	// ---- payloads, then one embedding call for all of them ----
	specs := make([]map[string]any, 0, len(edges)+2*len(nodeOrder))
	metas := make([]map[string]any, 0, len(edges)+2*len(nodeOrder))

	for _, e := range edges {
		specs = append(specs, e.payload)
		metas = append(metas, map[string]any{
			"kind":             "hyperedge",
			"name":             e.rootLower,
			"entity_type":      e.draft.rootClass,
			"depth":            e.draft.depth,
			"source_chunk_ids": e.chunkIDs,
			"mention_count":    len(e.draft.facts),
		})
	}
	for _, key := range nodeOrder {
		acc := nodes[key]
		base := map[string]any{
			"kind":          "hypernode",
			"path":          acc.fact.path,
			"prop":          acc.fact.prop,
			"hyperedge_ids": acc.edgeIDs,
			"depth":         acc.depth,
			// node_value is the slot's value in exact spelling, written on both
			// halves. It is what joins a key row to its value row (a key row's
			// payload holds the path, not the value) and what lets a retrieval
			// match an entity's name node by equality instead of by full text.
			"node_value":       acc.fact.val,
			"source_chunk_ids": acc.chunks,
			"mention_count":    len(acc.edgeIDs),
		}
		keyMeta := map[string]any{}
		for k, v := range base {
			keyMeta[k] = v
		}
		keyMeta["node_role"] = hypernodeKeyRole
		specs = append(specs, map[string]any{"key": acc.fact.key})
		metas = append(metas, keyMeta)

		valueMeta := map[string]any{}
		for k, v := range base {
			valueMeta[k] = v
		}
		valueMeta["node_role"] = hypernodeValueRole
		specs = append(specs, map[string]any{"value": acc.fact.val})
		metas = append(metas, valueMeta)
	}

	texts := make([]string, len(specs))
	for i, s := range specs {
		texts[i] = payloadDescription(s)
	}
	vectors, err := deps.Embed.Encode(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(specs) {
		return nil, fmt.Errorf("knowledge_compiler: hypergraph embedding count mismatch (%d vs %d)", len(vectors), len(specs))
	}

	rows := make([]common.Product, 0, len(specs))
	i := 0
	for _, e := range edges {
		rows = append(rows, common.Product{
			ID:       e.id,
			DocID:    cfg.DocID,
			TenantID: cfg.TenantID,
			Variant:  cfg.Variant,
			Content:  payloadJSON(e.payload),
			Vector:   vectors[i],
			Meta:     metas[i],
		})
		i++
	}
	for _, key := range nodeOrder {
		acc := nodes[key]
		// The document id is part of a hypernode's id. Without it the same
		// (path, value) produced by two documents is one row id, and since the
		// writer replaces whole rows the later document overwrites the earlier
		// one's membership — its hyperedge_ids, source_chunk_ids and counts —
		// and the earlier document's slots become unreachable from the layer.
		// Scoping the id to the document keeps the membership a document can
		// actually compute; a KB-wide node needs membership rows of its own, not
		// a longer array (see hypergraph_retrieval.md §3 P0-1).
		hash := common.StableRowID(acc.fact.path+"\x00"+acc.fact.val, cfg.DocID, cfg.TemplateID)
		for _, suffix := range []string{":k", ":v"} {
			rows = append(rows, common.Product{
				ID:       hash + suffix,
				DocID:    cfg.DocID,
				TenantID: cfg.TenantID,
				Variant:  cfg.Variant,
				Content:  payloadJSON(specs[i]),
				Vector:   vectors[i],
				Meta:     metas[i],
			})
			i++
		}
	}
	return rows, nil
}

// dedupeStrings returns the sorted-unique form of an already-sorted slice.
func dedupeStrings(in []string) []string {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}
