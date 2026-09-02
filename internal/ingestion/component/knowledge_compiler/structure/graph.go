package structure

import (
	"strings"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// Graph projections and rebuild, mirroring Python structure.py's
// _struct_graph_entity / _struct_graph_relation / _struct_merge_graph_entities
// and _struct_rebuild_graph_json. The compact graph is derived from the
// surviving entity/relation payload rows (content_with_weight JSON).

// graphEntity mirrors _struct_graph_entity: project an entity payload to the
// graph-node shape (mention_count starts at 1; the row's filtered
// source_chunk_ids column is authoritative for provenance).
func graphEntity(payload map[string]any, sourceChunkIDs []string) map[string]any {
	name := entityName(payload)
	if name == "" {
		return nil
	}
	typ := strings.TrimSpace(stringOf(payload["type"]))
	if typ == "" {
		typ = "other"
	}
	var aliases []string
	switch v := payload["aliases"].(type) {
	case string:
		if s := strings.TrimSpace(v); s != "" {
			aliases = []string{s}
		}
	case []any:
		for _, e := range v {
			if s := strings.TrimSpace(stringOf(e)); s != "" {
				aliases = append(aliases, s)
			}
		}
	case []string:
		for _, e := range v {
			if s := strings.TrimSpace(e); s != "" {
				aliases = append(aliases, s)
			}
		}
	}
	description := strings.TrimSpace(stringOf(payload["description"]))
	if description == "" {
		description = strings.TrimSpace(stringOf(payload["definition_excerpt"]))
	}
	return map[string]any{
		"aliases":          aliases,
		"mention_count":    1,
		"name":             name,
		"source_chunk_ids": unionOrdered(sourceChunkIDs),
		"type":             typ,
		"description":      description,
	}
}

// graphRelation mirrors _struct_graph_relation: project a relation payload to
// the {from, to, type} edge shape.
func graphRelation(payload map[string]any) map[string]any {
	src := relationEndpoint(payload, "", "source", "src", "from")
	tgt := relationEndpoint(payload, "", "target", "tgt", "to")
	if src == "" || tgt == "" {
		return nil
	}
	typ := strings.TrimSpace(stringOf(payload["type"]))
	if typ == "" {
		typ = "related"
	}
	return map[string]any{"from": src, "to": tgt, "type": typ}
}

// mergeGraphEntities mirrors _struct_merge_graph_entities: collapse entities
// sharing (name, type) — mention_count sums, aliases/description/
// source_chunk_ids union, first-seen order preserved.
func mergeGraphEntities(entities []map[string]any) []map[string]any {
	type key struct{ name, typ string }
	merged := map[key]int{}
	var order []key
	var out []map[string]any
	for _, entity := range entities {
		k := key{name: entity["name"].(string), typ: entity["type"].(string)}
		idx, ok := merged[k]
		if !ok {
			merged[k] = len(out)
			order = append(order, k)
			out = append(out, entity)
			continue
		}
		target := out[idx]
		target["mention_count"] = mentionOf(target) + mentionOf(entity)
		target["aliases"] = unionOrdered(toStrings(target["aliases"]), toStrings(entity["aliases"]))
		if target["description"] == "" && entity["description"] != "" {
			target["description"] = entity["description"]
		}
		target["source_chunk_ids"] = unionOrdered(toStrings(target["source_chunk_ids"]), toStrings(entity["source_chunk_ids"]))
	}
	return out
}

// RebuildStructureGraph mirrors _struct_rebuild_graph_json: rebuild the
// compact {"entities": [...], "relations": [...]} graph from the surviving
// structure products. Entity mention counts sum across rows sharing
// (name, type); relation rows project verbatim (already deduped in-run).
func RebuildStructureGraph(products []common.Product) map[string]any {
	var entities []map[string]any
	var relations []map[string]any
	for _, p := range products {
		kind, _ := p.Meta["kind"].(string)
		payload := parsePayload(p.Content)
		if payload == nil {
			continue
		}
		switch kind {
		case "relation":
			if rel := graphRelation(payload); rel != nil {
				relations = append(relations, rel)
			}
		case "entity":
			if ent := graphEntity(payload, metaStrings(p.Meta, "source_chunk_ids")); ent != nil {
				entities = append(entities, ent)
			}
		}
	}
	return map[string]any{
		"entities":  mergeGraphEntities(entities),
		"relations": relations,
	}
}

func mentionOf(entity map[string]any) int {
	switch v := entity["mention_count"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 1
}

func toStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
