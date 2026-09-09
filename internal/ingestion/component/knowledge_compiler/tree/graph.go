package tree

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// stringsContains reports whether s contains substring sub.
func stringsContains(s, sub string) bool {
	return strings.Contains(s, sub)
}

// strSliceContains reports whether s contains v.
func strSliceContains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

// stringsJoinNonEmpty joins the non-empty parts with sep.
func stringsJoinNonEmpty(parts []string, sep string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, sep)
}

// stringMetaSlice coerces a Product.Meta value ([]string or []any of strings)
// into a []string.
func stringMetaSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// payloadChunkIDs extracts the source_chunk_ids from a tree-graph payload.
func payloadChunkIDs(payload map[string]any) []string {
	switch v := payload["source_chunk_ids"].(type) {
	case []string:
		return v
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// payloadDescription is the embedding input for a tree-graph entity/relation:
// the concatenated string values of every field except description (lists
// flattened), matching Python _struct_payload_description.
func payloadDescription(payload map[string]any) string {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		if k != "description" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		switch v := payload[k].(type) {
		case string:
			if v != "" {
				parts = append(parts, v)
			}
		case []string:
			for _, e := range v {
				if e != "" {
					parts = append(parts, e)
				}
			}
		case []any:
			for _, e := range v {
				if s, ok := e.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
		}
	}
	return strings.Join(parts, " ")
}

// payloadJSON serialises a payload the way Python's json.dumps(ensure_ascii=
// False) does (no HTML escaping), with alphabetically sorted keys for a
// canonical, hash-stable form.
func payloadJSON(payload map[string]any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "{}"
	}
	return strings.TrimSpace(b.String())
}

// graphNode mirrors Python's RAPTOR tree node dict (title/description/children/
// source_chunk_ids) reconstructed from the flat products emitted by buildTree,
// so the tree can be projected to a {entities, relations} graph exactly like
// Python's raptor_tree_to_graph (chunk_post_processor.py:470).
type graphNode struct {
	title          string
	description    string
	sourceChunkIDs []string
	children       []*graphNode
}

// collapseUnary merges a node that wraps exactly one child into that child,
// mirroring Python raptor_tree_to_graph._collapse_unary: the parent's and the
// child's descriptions/source-chunk-ids are concatenated (dedup'd), then the
// collapsed node adopts the child's children.
func collapseUnary(node *graphNode) *graphNode {
	collapsed := &graphNode{
		title:          node.title,
		description:    node.description,
		sourceChunkIDs: node.sourceChunkIDs,
	}
	for _, c := range node.children {
		collapsed.children = append(collapsed.children, collapseUnary(c))
	}
	for len(collapsed.children) == 1 {
		child := collapsed.children[0]
		parentTitle := collapsed.title
		childTitle := child.title
		parentDesc := collapsed.description
		if parentDesc == "" {
			parentDesc = parentTitle
		}
		childDesc := child.description
		if childDesc == "" {
			childDesc = childTitle
		}

		var descriptions []string
		descriptions = append(descriptions, parentDesc)
		if childTitle != "" && childTitle != parentTitle &&
			!stringsContains(childDesc, childTitle) {
			descriptions = append(descriptions, childTitle)
		}
		if childDesc != "" && !strSliceContains(descriptions, childDesc) {
			descriptions = append(descriptions, childDesc)
		}

		sourceChunkIDs := append([]string{}, collapsed.sourceChunkIDs...)
		for _, id := range child.sourceChunkIDs {
			if id != "" && !strSliceContains(sourceChunkIDs, id) {
				sourceChunkIDs = append(sourceChunkIDs, id)
			}
		}

		collapsed.description = stringsJoinNonEmpty(descriptions, "\n\n")
		collapsed.sourceChunkIDs = sourceChunkIDs
		collapsed.children = child.children
	}
	return collapsed
}

// raptorTreeToGraph projects a RAPTOR tree onto {entities, relations}, matching
// Python raptor_tree_to_graph: every node becomes an entity of type "tree_node";
// every parent→child edge (that is not a self-loop) becomes a "child" relation.
func raptorTreeToGraph(root *graphNode) ([]map[string]any, []map[string]any) {
	var entities []map[string]any
	var relations []map[string]any
	var walk func(node *graphNode, parentTitle string)
	walk = func(node *graphNode, parentTitle string) {
		if node == nil {
			return
		}
		title := node.title
		ent := map[string]any{
			"name":          title,
			"type":          "tree_node",
			"description":   firstNonEmpty(node.description, title),
			"mention_count": 1,
		}
		if len(node.sourceChunkIDs) > 0 {
			ent["source_chunk_ids"] = node.sourceChunkIDs
		}
		entities = append(entities, ent)
		if parentTitle != "" && parentTitle != title {
			relations = append(relations, map[string]any{
				"from": parentTitle,
				"to":   title,
				"type": "child",
			})
		}
		for _, child := range node.children {
			walk(child, title)
		}
	}
	walk(root, "")
	return entities, relations
}

// buildTreeGraph reconstructs the tree from the flat summary products and
// produces the entity/relation/graph products Python writes for a tree variant
// (_struct_upsert_tree_graph_rows + _struct_upsert_graph_json):
//   - one entity product per tree node (kind "entity", knowledge_graph_kwd via
//     the writer), type "tree_node";
//   - one relation product per parent→child edge (kind "relation");
//   - one compact graph blob product (kind "graph") carrying the whole
//     {entities, relations} projection, which is the /structure/graph discovery
//     row (Python scans knowledge_graph_kwd="graph").
//
// templateID is stamped into each row so the document-structure endpoint can
// group by template id; compileKWD is "tree".
func buildTreeGraph(ctx context.Context, deps common.Deps, docID string, products []common.Product) ([]common.Product, error) {
	if deps.Embed == nil {
		return nil, fmt.Errorf("tree: embedder required to build the tree graph")
	}
	root := reconstructTree(products)
	if root == nil {
		// No root summary survived; there is no tree to project.
		return nil, nil
	}
	root = collapseUnary(root)
	entities, relations := raptorTreeToGraph(root)

	var out []common.Product
	var descs []string
	var payloads []map[string]any
	var kinds []string
	for _, ent := range entities {
		descs = append(descs, payloadDescription(ent))
		payloads = append(payloads, ent)
		kinds = append(kinds, "entity")
	}
	for _, rel := range relations {
		descs = append(descs, payloadDescription(rel))
		payloads = append(payloads, rel)
		kinds = append(kinds, "relation")
	}
	vecs, err := deps.Embed.Encode(ctx, descs)
	if err != nil {
		return nil, err
	}
	for i, payload := range payloads {
		kind := kinds[i]
		var vec []float32
		if i < len(vecs) {
			vec = vecs[i]
		}
		meta := map[string]any{
			"kind":             kind,
			"compile_kwd":      "tree",
			"source_chunk_ids": payloadChunkIDs(payload),
			"mention_count":    1,
		}
		if kind == "entity" {
			if name, ok := payload["name"].(string); ok && name != "" {
				meta["name"] = name
			}
			if typ, ok := payload["type"].(string); ok && typ != "" {
				meta["entity_type"] = typ
			} else {
				meta["entity_type"] = "other"
			}
		} else {
			if from, ok := payload["from"].(string); ok {
				meta["from"] = from
			}
			if to, ok := payload["to"].(string); ok {
				meta["to"] = to
			}
		}
		out = append(out, common.Product{
			ID:       common.StableRowID(payloadJSON(payload), docID),
			DocID:    docID,
			TenantID: deps.TenantID,
			Variant:  common.VariantTree,
			Content:  payloadJSON(payload),
			Vector:   vec,
			Meta:     meta,
		})
	}

	// Compact graph blob discovery row (knowledge_graph_kwd="graph").
	graph := map[string]any{"entities": entities, "relations": relations}
	graphContent := payloadJSON(graph)
	graphVecs, err := deps.Embed.Encode(ctx, []string{graphContent})
	if err != nil {
		return nil, err
	}
	var gv []float32
	if len(graphVecs) > 0 {
		gv = graphVecs[0]
	}
	out = append(out, common.Product{
		ID:       common.StableRowID(docID, "tree", "structure_graph"),
		DocID:    docID,
		TenantID: deps.TenantID,
		Variant:  common.VariantTree,
		Content:  graphContent,
		Vector:   gv,
		Meta: map[string]any{
			"kind":        "graph",
			"compile_kwd": "tree",
		},
	})
	return out, nil
}

// reconstructTree assembles a graphNode tree from the flat summary products:
// the root has Meta.kind=="root"; every other node's parent is the product with
// ID == node.ParentID. Node title comes from Meta.title, description from
// Content, source chunk ids from Meta.source_chunk_ids.
func reconstructTree(products []common.Product) *graphNode {
	byID := make(map[string]*graphNode, len(products))
	for _, p := range products {
		title, _ := p.Meta["title"].(string)
		byID[p.ID] = &graphNode{
			title:          title,
			description:    p.Content,
			sourceChunkIDs: stringMetaSlice(p.Meta["source_chunk_ids"]),
		}
	}
	var root *graphNode
	for _, p := range products {
		kind, _ := p.Meta["kind"].(string)
		node := byID[p.ID]
		if kind == "root" {
			root = node
			continue
		}
		if parent := byID[p.ParentID]; parent != nil {
			parent.children = append(parent.children, node)
		}
	}
	return root
}
