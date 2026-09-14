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
	entity := map[string]any{
		"aliases":          aliases,
		"mention_count":    1,
		"name":             name,
		"source_chunk_ids": unionOrdered(sourceChunkIDs),
		"type":             typ,
		"description":      description,
	}
	// Gate-verified verbatim evidence rides along for the artifacts detail
	// panel (mirrors _struct_graph_entity's evidence projection). Rows compiled
	// before the field existed simply omit it.
	if evidence, ok := payload["evidence"].([]any); ok {
		var verified []any
		for _, e := range evidence {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if q := strings.TrimSpace(stringOf(m["quote"])); q != "" {
				verified = append(verified, m)
			}
		}
		if len(verified) > 0 {
			entity["evidence"] = verified
		}
	}
	return entity
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
//
// compileType selects the kind-scoped post-processing: page_index templates
// declare that every claim hangs off exactly one container, so orphans the
// per-batch relation pass could not attach are re-parented (mirrors
// _struct_attach_orphan_entities).
func RebuildStructureGraph(products []common.Product, compileType string) map[string]any {
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
	if normalizeKind(compileType) == "page_index" || normalizeKind(compileType) == "pageindex" {
		relations = attachOrphanEntities(entities, relations)
	}
	return map[string]any{
		"entities":  mergeGraphEntities(entities),
		"relations": relations,
	}
}

// attachOrphanEntities mirrors _struct_attach_orphan_entities: re-parent leaf
// entities the per-batch relation pass could not attach.
//
// Relations are extracted inside one batch, so a container living in another
// batch is invisible to the edge prompt. Only types that NEVER act as a
// container are repaired — an entity of such a type with no incoming edge is
// missing its parent by definition, whereas a structural type (a heading) may
// legitimately be a root. Containers are classified by TYPE, not instance: a
// heading whose own children all fell in other batches never appears on a
// "from" side, yet it is still the right parent. No type names are baked in —
// containers are whatever appears on the "from" side.
//
// The parent is the container sharing the most source chunks with the orphan,
// preferring the most specific one (fewest chunks of its own), because a
// heading's source_chunk_ids are meant to cover its whole section.
func attachOrphanEntities(entities, relations []map[string]any) []map[string]any {
	if len(entities) == 0 || len(relations) == 0 {
		return relations
	}

	childNames := map[string]bool{}
	containerNames := map[string]bool{}
	for _, r := range relations {
		if name := strings.TrimSpace(stringOf(r["to"])); name != "" {
			childNames[name] = true
		}
		if name := strings.TrimSpace(stringOf(r["from"])); name != "" {
			containerNames[name] = true
		}
	}
	if len(containerNames) == 0 {
		return relations
	}

	byType := map[string][]map[string]any{}
	for _, e := range entities {
		typ := strings.TrimSpace(stringOf(e["type"]))
		if typ == "" {
			typ = "other"
		}
		byType[typ] = append(byType[typ], e)
	}

	// Classify by TYPE, not by instance (see doc comment above).
	containerTypes := map[string]bool{}
	for typ, ents := range byType {
		for _, e := range ents {
			if name := strings.TrimSpace(stringOf(e["name"])); containerNames[name] {
				containerTypes[typ] = true
				break
			}
		}
	}
	var containers []map[string]any
	for _, e := range entities {
		if containerTypes[strings.TrimSpace(stringOf(e["type"]))] {
			containers = append(containers, e)
		}
	}
	if len(containers) == 0 {
		return relations
	}

	// Synthetic relations reuse the dominant existing relation type.
	dominantType, best := "include", 0
	counts := map[string]int{}
	for _, r := range relations {
		if t := strings.TrimSpace(stringOf(r["type"])); t != "" {
			counts[t]++
			if counts[t] > best {
				best, dominantType = counts[t], t
			}
		}
	}

	existing := map[[2]string]bool{}
	for _, r := range relations {
		existing[[2]string{strings.TrimSpace(stringOf(r["from"])), strings.TrimSpace(stringOf(r["to"]))}] = true
	}

	var repaired []map[string]any
	for _, ent := range entities {
		name := strings.TrimSpace(stringOf(ent["name"]))
		typ := strings.TrimSpace(stringOf(ent["type"]))
		if typ == "" {
			typ = "other"
		}
		if name == "" || containerTypes[typ] || childNames[name] {
			continue
		}
		own := chunkSet(ent)
		if len(own) == 0 {
			continue
		}
		bestName := ""
		bestOverlap, bestSpecificity := 0, 0
		for _, cand := range containers {
			candName := strings.TrimSpace(stringOf(cand["name"]))
			if candName == "" || candName == name {
				continue
			}
			candChunks := chunkSet(cand)
			overlap := 0
			for c := range own {
				if candChunks[c] {
					overlap++
				}
			}
			if overlap <= 0 {
				continue
			}
			// Most overlapping wins; ties go to the narrower container, which
			// is the deeper (more specific) section.
			if bestName == "" || overlap > bestOverlap || (overlap == bestOverlap && len(candChunks) < bestSpecificity) {
				bestName, bestOverlap, bestSpecificity = candName, overlap, len(candChunks)
			}
		}
		if bestName == "" {
			continue
		}
		if existing[[2]string{bestName, name}] {
			continue
		}
		existing[[2]string{bestName, name}] = true
		repaired = append(repaired, map[string]any{"from": bestName, "to": name, "type": dominantType})
	}

	if len(repaired) > 0 {
		// Mirrors Python's log line; the message lands in the compile log.
		relations = append(relations, repaired...)
	}
	return relations
}

func chunkSet(entity map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, c := range toStrings(entity["source_chunk_ids"]) {
		if c = strings.TrimSpace(c); c != "" {
			out[c] = true
		}
	}
	return out
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
