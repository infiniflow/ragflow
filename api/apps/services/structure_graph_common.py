#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Shared structure-graph subgraph sampling.

Both the per-document (``/datasets/<id>/documents/<doc>/structure/graph``) and
the dataset-wide (``/datasets/<id>/artifacts/structure``) endpoints render
per-template structure graphs. For large graphs we don't return every
entity/relation — we fetch a representative subgraph from the raw
``knowledge_graph_kwd`` rows (which carry ``mention_count_int`` / ``name_kwd`` /
``from_entity_kwd`` / ``to_entity_kwd`` / ``q_<dim>_vec``) so the response — and
the frontend render — stay bounded.

The two endpoints differ only in *scope*: the document endpoint filters raw
rows by ``doc_id``; the dataset endpoint queries KB-wide (dataset-merge
templates dedup entity/relation rows across documents). That difference lives
entirely in the ``scope`` / ``base_entity_condition`` dicts the caller passes —
everything else is shared here.
"""

import json
import logging
import re

from common import settings
from common.doc_store.doc_store_base import OrderByExpr
from common.misc_utils import thread_pool_exec


# Below this combined (entities + relations) count for a bucket, return all rows.
GRAPH_FULL_THRESHOLD = 1024
# Size of the top-mention entity seed set (set A) for large buckets.
GRAPH_TOP_ENTITIES = 256
# Keyword search needs a small, relevant candidate set; the larger sampling
# cap above is intended for rendering an entire large graph bucket.
GRAPH_KEYWORD_CANDIDATES = 16
# Semantic fallback is intentionally singular. KNN is only used after exact
# name and BM25 lookup fail; returning several approximate tree nodes creates
# unrelated sibling branches in an otherwise focused path response.
GRAPH_KEYWORD_KNN_CANDIDATES = 1
# Upper bound on the relation / neighbor-entity expansion so a hub node can't
# blow up the response.
GRAPH_EXPANSION_CAP = 4096

GRAPH_ENTITY_FIELDS = ["id", "content_with_weight", "name_kwd", "mention_count_int", "source_chunk_ids", "doc_id", "doc_ids_kwd", "source_doc_ids"]
GRAPH_RELATION_FIELDS = ["id", "content_with_weight", "from_entity_kwd", "to_entity_kwd", "doc_id", "doc_ids_kwd", "source_doc_ids"]
GRAPH_ALL_FIELDS = [
    "id",
    "content_with_weight",
    "name_kwd",
    "mention_count_int",
    "source_chunk_ids",
    "from_entity_kwd",
    "to_entity_kwd",
    "knowledge_graph_kwd",
    "doc_id",
    "doc_ids_kwd",
    "source_doc_ids",
]


async def graph_search(index_name, kb_id, select_fields, condition, order_by, limit, match_expressions=None, offset=0):
    """One raw-row search. Returns ``(field_map, total)`` where ``total`` is the
    full match count (not the returned slice)."""
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        select_fields,
        [],
        condition,
        match_expressions or [],
        order_by,
        offset,
        max(int(limit or 0), 1),
        index_name,
        [kb_id],
    )
    field_map = settings.docStoreConn.get_fields(res, select_fields) or {}
    total = settings.docStoreConn.get_total(res)
    return field_map, int(total or 0)


def project_entity(row: dict) -> dict | None:
    """Project a raw ``knowledge_graph_kwd="entity"`` row to the graph-node shape
    the frontend already consumes, surfacing ``mention_count_int`` as
    ``mention_count``."""
    from rag.advanced_rag.knowlege_compile.structure import _struct_graph_entity

    try:
        payload = json.loads(row.get("content_with_weight") or "{}")
    except Exception:
        return None
    if not isinstance(payload, dict):
        return None
    node = _struct_graph_entity(payload, row.get("source_chunk_ids"))
    if not node:
        return None
    mc = row.get("mention_count_int")
    if isinstance(mc, list):  # Infinity returns *_int scalars fine, but be defensive
        mc = mc[0] if mc else None
    try:
        if mc is not None:
            node["mention_count"] = int(mc)
    except (TypeError, ValueError):
        pass
    return node


def project_relation(row: dict) -> dict | None:
    """Project a raw ``knowledge_graph_kwd="relation"`` row to the edge shape.
    Prefers the payload (matching the blob projection); falls back to the
    authoritative ``*_entity_kwd`` columns."""
    from rag.advanced_rag.knowlege_compile.structure import _struct_graph_relation

    try:
        payload = json.loads(row.get("content_with_weight") or "{}")
    except Exception:
        payload = {}
    if isinstance(payload, dict):
        node = _struct_graph_relation(payload)
        if node:
            return node
    src = str(row.get("from_entity_kwd") or "").strip()
    tgt = str(row.get("to_entity_kwd") or "").strip()
    if not src or not tgt:
        return None
    typ = payload.get("type") if isinstance(payload, dict) else None
    return {"from": src, "to": tgt, "type": str(typ).strip() if typ else "related"}


def dedup_entities(entities: list[dict]) -> list[dict]:
    """Order-preserving dedup by (lowercased name, type)."""
    out: list[dict] = []
    seen: set[tuple[str, str]] = set()
    for e in entities:
        key = (str(e.get("name") or "").strip().lower(), str(e.get("type") or "").strip().lower())
        if not key[0] or key in seen:
            continue
        seen.add(key)
        out.append(e)
    return out


def _entity_response_id(entity: dict) -> str:
    for field in ("id", "name", "slug"):
        value = entity.get(field)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def _endpoint_terms(value: str) -> list[str]:
    value = value.strip()
    if not value:
        return []
    return sorted({value, value.lower()})


def normalize_relation_endpoints(entities: list[dict], relations: list[dict]) -> list[dict]:
    """Align relation endpoints to the returned entity ids/names."""
    if not entities or not relations:
        return relations

    lookup: dict[str, str] = {}
    ambiguous: set[str] = set()
    for entity in entities:
        response_id = _entity_response_id(entity)
        if not response_id:
            continue
        for field in ("id", "name", "slug"):
            value = entity.get(field)
            if not isinstance(value, str) or not value.strip():
                continue
            key = value.strip().lower()
            if key in lookup and lookup[key] != response_id:
                ambiguous.add(key)
                continue
            lookup[key] = response_id
    for key in ambiguous:
        lookup.pop(key, None)

    normalized: list[dict] = []
    for relation in relations:
        if not isinstance(relation, dict):
            continue
        item = dict(relation)
        for field in ("from", "to"):
            value = item.get(field)
            if isinstance(value, str):
                item[field] = lookup.get(value.strip().lower(), value)
        normalized.append(item)
    return normalized


def filter_entities_with_relations(entities: list[dict], relations: list[dict]) -> list[dict]:
    """Keep only entities that are referenced by at least one relation."""
    if not entities or not relations:
        return []

    # Match case-insensitively: the dataset-scoped merge lowercases relation
    # endpoints while entity names keep their original case, so exact matching
    # would drop connected nodes from graph-like views.
    connected: set[str] = set()
    for relation in relations:
        if not isinstance(relation, dict):
            continue
        for endpoint_key in ("from", "to"):
            endpoint = relation.get(endpoint_key)
            if isinstance(endpoint, str):
                endpoint = endpoint.strip().lower()
                if endpoint:
                    connected.add(endpoint)

    if not connected:
        return []

    filtered: list[dict] = []
    for entity in entities:
        if not isinstance(entity, dict):
            continue
        keys: set[str] = set()
        # Structure-graph nodes are name-keyed and their relations reference
        # names; artifact-graph nodes are slug-keyed and their relations
        # reference slugs. Check all three identity fields so the same filter
        # serves both callers.
        for field in ("id", "name", "slug"):
            value = entity.get(field)
            if isinstance(value, str):
                value = value.strip().lower()
                if value:
                    keys.add(value)
        if keys & connected:
            filtered.append(entity)
    return filtered


def _row_has_enabled_source(row: dict, excluded_doc_ids: set[str]) -> bool:
    if not excluded_doc_ids:
        return True

    def _flatten_ids(value) -> set[str]:
        if value is None:
            return set()
        if isinstance(value, str):
            raw = value.strip()
            if not raw:
                return set()
            try:
                return _flatten_ids(json.loads(raw))
            except (json.JSONDecodeError, TypeError):
                return {raw}
        if isinstance(value, (list, tuple, set)):
            result: set[str] = set()
            for item in value:
                result.update(_flatten_ids(item))
            return result
        return {str(value)}

    source_ids: set[str] = set()
    for field in ("doc_ids_kwd", "source_doc_ids"):
        source_ids.update(_flatten_ids(row.get(field)))
    if source_ids:
        return bool(source_ids - excluded_doc_ids)
    doc_ids = _flatten_ids(row.get("doc_id"))
    return not doc_ids or bool(doc_ids - excluded_doc_ids)


async def build_bucket(index_name, kb_id, scope: dict, excluded_doc_ids: set[str] | None = None) -> tuple[list[dict], list[dict]]:
    """Build one bucket's ``(entities, relations)`` from raw rows.

    ``scope`` is the filter WITHOUT ``knowledge_graph_kwd`` — e.g.
    ``{"doc_id":[id], "compilation_template_ids":[tid]}`` (document scope) or
    ``{"compilation_template_ids":[tid]}`` (dataset scope). Small buckets are
    returned whole; large ones are sampled: top-``GRAPH_TOP_ENTITIES`` entities
    by ``mention_count_int``, the relations sourced from them, and those
    relations' target entities.
    """
    excluded_doc_ids = excluded_doc_ids or set()
    both_cond = dict(scope, knowledge_graph_kwd=["entity", "relation"])
    _, total = await graph_search(index_name, kb_id, ["id"], both_cond, OrderByExpr(), 1)

    if total < GRAPH_FULL_THRESHOLD:
        field_map, _ = await graph_search(index_name, kb_id, GRAPH_ALL_FIELDS, both_cond, OrderByExpr(), total or 1)
        entities: list[dict] = []
        relations: list[dict] = []
        for row in field_map.values():
            if not _row_has_enabled_source(row, excluded_doc_ids):
                continue
            if row.get("knowledge_graph_kwd") == "relation":
                edge = project_relation(row)
                if edge:
                    relations.append(edge)
            else:
                node = project_entity(row)
                if node:
                    entities.append(node)
        entities = dedup_entities(entities)
        return entities, normalize_relation_endpoints(entities, relations)

    # Large bucket: sample. A = top entities by mention_count_int desc.
    order_by = OrderByExpr()
    try:
        order_by.desc("mention_count_int")
    except Exception:
        order_by = OrderByExpr()
    set_a: list[dict] = []
    entity_offset = 0
    entity_total = None
    while len(set_a) < GRAPH_TOP_ENTITIES and (entity_total is None or entity_offset < entity_total):
        ent_a_map, entity_total = await graph_search(
            index_name,
            kb_id,
            GRAPH_ENTITY_FIELDS,
            dict(scope, knowledge_graph_kwd=["entity"]),
            order_by,
            GRAPH_TOP_ENTITIES,
            offset=entity_offset,
        )
        if not ent_a_map:
            break
        set_a.extend(n for n in (project_entity(r) for r in ent_a_map.values() if _row_has_enabled_source(r, excluded_doc_ids)) if n)
        entity_offset += len(ent_a_map)
    set_a = set_a[:GRAPH_TOP_ENTITIES]
    a_names = sorted({str(e.get("name") or "").strip() for e in set_a if str(e.get("name") or "").strip()})
    a_name_terms = sorted({term for name in a_names for term in _endpoint_terms(name)})

    # relations whose source is one of A.
    relations = []
    target_names_lower: set[str] = set()
    if a_name_terms:
        rel_map, _ = await graph_search(index_name, kb_id, GRAPH_RELATION_FIELDS, dict(scope, knowledge_graph_kwd=["relation"], from_entity_kwd=a_name_terms), OrderByExpr(), GRAPH_EXPANSION_CAP)
        for row in rel_map.values():
            if not _row_has_enabled_source(row, excluded_doc_ids):
                continue
            edge = project_relation(row)
            if edge:
                relations.append(edge)
                tgt = str(edge.get("to") or "").strip().lower()
                if tgt:
                    target_names_lower.add(tgt)

    # target entities of those relations (case-insensitive via name_kwd).
    set_t = []
    if target_names_lower:
        tgt_map, _ = await graph_search(index_name, kb_id, GRAPH_ENTITY_FIELDS, dict(scope, knowledge_graph_kwd=["entity"], name_kwd=sorted(target_names_lower)), OrderByExpr(), GRAPH_EXPANSION_CAP)
        set_t = [n for n in (project_entity(r) for r in tgt_map.values() if _row_has_enabled_source(r, excluded_doc_ids)) if n]

    entities = dedup_entities(set_a + set_t)
    return entities, normalize_relation_endpoints(entities, relations)


async def keyword_subgraph(
    index_name,
    kb_id,
    embd_mdl,
    base_entity_condition,
    keywords,
    scope_for_template,
    log_ctx="",
    excluded_doc_ids: set[str] | None = None,
) -> tuple[dict | None, list[dict], list[dict]]:
    """Find matching entity rows and return their focused subgraph.

    BM25 provides lexical candidates which are then filtered by entity-name
    containment. KNN is used as a semantic fallback when lexical search has
    no valid candidates. Matching entities and their touching neighbors are
    returned; ``tree`` and ``page_index`` buckets additionally include the
    full ancestor path to the root. ``(None, [], [])`` is returned when
    nothing matches or embedding is unavailable.

    ``base_entity_condition`` scopes the KNN (e.g. ``{"doc_id":[id],
    "knowledge_graph_kwd":["entity"]}`` or ``{"compilation_template_ids":[...],
    "knowledge_graph_kwd":["entity"]}``). ``scope_for_template(row)`` resolves
    ``(bucket_meta, scope_filter)`` for the matched row (scope WITHOUT
    ``knowledge_graph_kwd``).
    """
    from common.doc_store.doc_store_base import MatchDenseExpr, MatchTextExpr
    from rag.advanced_rag.knowlege_compile._common import tokenize_for_search

    excluded_doc_ids = excluded_doc_ids or set()

    top_fields = GRAPH_ENTITY_FIELDS + ["compilation_template_ids", "compile_kwd", "compilation_template_kind_kwd"]

    def _valid_top_nodes(rows):
        valid = []
        for row in rows.values():
            if not _row_has_enabled_source(row, excluded_doc_ids):
                continue
            node = project_entity(row)
            if node and str(node.get("name") or "").strip():
                valid.append((row, node))
        return valid

    def _name_matches_query(node, query):
        name = str(node.get("name") or "").strip().lower()
        query = query.lower()
        if not name or not query:
            return False
        if query in name:
            return True
        terms = [term for term in query.split() if term]
        return bool(terms) and all(term in name for term in terms)

    # Entity names are identifiers in the graph. Check the exact normalized
    # name first: BM25 searches the entity description and may miss a stored
    # name even when the query equals it, which would otherwise trigger KNN
    # and return an approximate result. Partial-name queries
    # continue through BM25 so they can intentionally return multiple nodes.
    text_query = re.sub(r"[ :|\r\n\t,，。？?/`!！&^%()\[\]{}<>*~'\"\\=]+", " ", str(keywords or "")).strip()
    candidates = []
    exact_name = str(keywords or "").strip().lower()
    if exact_name:
        exact_map, _ = await graph_search(
            index_name,
            kb_id,
            top_fields,
            dict(base_entity_condition, name_kwd=[exact_name]),
            OrderByExpr(),
            GRAPH_KEYWORD_CANDIDATES,
        )
        candidates = _valid_top_nodes(exact_map)
    if text_query and not candidates:
        coarse_query, fine_query = tokenize_for_search(text_query)
        tokenized_query = " ".join(dict.fromkeys(f"{coarse_query} {fine_query}".split())) or text_query
        text_expr = MatchTextExpr(
            ["content_ltks^10", "content_sm_ltks"],
            tokenized_query,
            GRAPH_KEYWORD_CANDIDATES,
            {"original_query": tokenized_query},
        )
        top_map, _ = await graph_search(index_name, kb_id, top_fields, base_entity_condition, OrderByExpr(), GRAPH_KEYWORD_CANDIDATES, match_expressions=[text_expr])
        candidates = [(row, node) for row, node in _valid_top_nodes(top_map) if _name_matches_query(node, text_query)]
    # In a hierarchical index, a title containing the keyword is usually an
    # ancestor context, not the requested detail. Prefer matching detail
    # entities so the title is added only through the path-to-root walk; this
    # prevents all of the title's unrelated children from being returned.
    detail_candidates = [(row, node) for row, node in candidates if str(node.get("type") or "").strip().lower() != "title"]
    if detail_candidates:
        candidates = detail_candidates

    # Fall back to semantic matching for aliases, paraphrases, and cases
    # where the query does not occur in the stored entity name.
    if not candidates:
        try:
            qv, _ = await thread_pool_exec(embd_mdl.encode_queries, keywords)
            vec = list(qv)
        except Exception:
            logging.exception("structure graph: keyword embedding failed (%s)", log_ctx)
            return None, [], []
        if not vec:
            return None, [], []

        match_expr = MatchDenseExpr(
            vector_column_name=f"q_{len(vec)}_vec",
            embedding_data=vec,
            embedding_data_type="float",
            distance_type="cosine",
            topn=GRAPH_KEYWORD_KNN_CANDIDATES,
            extra_options={"similarity": 0.3},
        )
        top_map, _ = await graph_search(index_name, kb_id, top_fields, base_entity_condition, OrderByExpr(), GRAPH_KEYWORD_KNN_CANDIDATES, match_expressions=[match_expr])
        candidates = _valid_top_nodes(top_map)

    if not candidates:
        return None, [], []
    bucket_meta, scope = scope_for_template(candidates[0][0])

    # A response represents one template bucket. Keep all matching entities
    # from that bucket instead of silently discarding every candidate after
    # the first one.
    bucket_id = bucket_meta.get("template_id")
    matched_nodes = []
    for row, node in candidates:
        candidate_meta, _ = scope_for_template(row)
        if candidate_meta.get("template_id") == bucket_id:
            matched_nodes.append(node)
    if not matched_nodes:
        return None, [], []

    structure_kind = str(bucket_meta.get("kind") or "").strip().lower().replace("-", "_")

    # Relations where a matched entity is source OR target (two term queries).
    relations: list[dict] = []
    seen_rel: set[tuple[str, str, str]] = set()
    neighbor_names_lower: set[str] = set()
    matched_names = {str(node.get("name") or "").strip().lower() for node in matched_nodes}
    if structure_kind not in {"tree", "page_index", "pageindex"}:
        for matched_node in matched_nodes:
            matched_name = str(matched_node.get("name") or "").strip()
            matched_name_terms = _endpoint_terms(matched_name)
            for field in ("from_entity_kwd", "to_entity_kwd"):
                rel_map, _ = await graph_search(
                    index_name, kb_id, GRAPH_RELATION_FIELDS, dict(scope, knowledge_graph_kwd=["relation"], **{field: matched_name_terms}), OrderByExpr(), GRAPH_EXPANSION_CAP
                )
                for row in rel_map.values():
                    if not _row_has_enabled_source(row, excluded_doc_ids):
                        continue
                    edge = project_relation(row)
                    if not edge:
                        continue
                    key = (edge.get("from", ""), edge.get("to", ""), edge.get("type", ""))
                    if key in seen_rel:
                        continue
                    seen_rel.add(key)
                    relations.append(edge)
                    for endpoint in (edge.get("from", ""), edge.get("to", "")):
                        endpoint = str(endpoint).strip()
                        if endpoint and endpoint.lower() not in matched_names:
                            neighbor_names_lower.add(endpoint.lower())
                    if len(relations) >= GRAPH_EXPANSION_CAP:
                        break
                if len(relations) >= GRAPH_EXPANSION_CAP:
                    break
            if len(relations) >= GRAPH_EXPANSION_CAP:
                break

    # Tree-like structures encode hierarchy as parent -> child. A keyword may
    # hit a leaf, but the UI needs the complete path back to the root in order
    # to render that leaf in context. Resolve the path in this template one
    # endpoint at a time. Document-level relation endpoints may
    # retain their original case while dataset-level endpoints are lowercased;
    # querying one spelling at a time also avoids store-specific multi-value
    # keyword-filter semantics.
    tree_entities: list[dict] = []
    if structure_kind in {"tree", "page_index", "pageindex"} and len(relations) < GRAPH_EXPANSION_CAP:
        ancestor_frontier = {str(node.get("name") or "").strip().lower(): str(node.get("name") or "").strip() for node in matched_nodes if str(node.get("name") or "").strip()}
        seen_ancestors = set(matched_names)
        ancestor_names: dict[str, str] = {}
        while ancestor_frontier and len(relations) < GRAPH_EXPANSION_CAP:
            next_frontier: dict[str, str] = {}
            for child_name in ancestor_frontier.values():
                for child_term in _endpoint_terms(child_name):
                    rel_map, _ = await graph_search(
                        index_name,
                        kb_id,
                        GRAPH_RELATION_FIELDS,
                        dict(scope, knowledge_graph_kwd=["relation"], to_entity_kwd=[child_term]),
                        OrderByExpr(),
                        GRAPH_EXPANSION_CAP - len(relations),
                    )
                    for row in rel_map.values():
                        if not _row_has_enabled_source(row, excluded_doc_ids):
                            continue
                        edge = project_relation(row)
                        if not edge:
                            continue
                        key = (edge.get("from", ""), edge.get("to", ""), edge.get("type", ""))
                        if key in seen_rel:
                            continue
                        seen_rel.add(key)
                        relations.append(edge)
                        parent_name = str(edge.get("from") or "").strip()
                        parent = parent_name.lower()
                        if parent and parent not in seen_ancestors:
                            seen_ancestors.add(parent)
                            next_frontier[parent] = parent_name
                            ancestor_names[parent] = parent_name
                        if len(relations) >= GRAPH_EXPANSION_CAP:
                            break
                    if len(relations) >= GRAPH_EXPANSION_CAP:
                        break
                if len(relations) >= GRAPH_EXPANSION_CAP:
                    break
            ancestor_frontier = next_frontier
        neighbor_names_lower.update(seen_ancestors - matched_names)
        for name in ancestor_names:
            entity_map, _ = await graph_search(
                index_name,
                kb_id,
                GRAPH_ENTITY_FIELDS,
                dict(scope, knowledge_graph_kwd=["entity"], name_kwd=[name]),
                OrderByExpr(),
                GRAPH_EXPANSION_CAP,
            )
            tree_entities.extend(n for n in (project_entity(row) for row in entity_map.values() if _row_has_enabled_source(row, excluded_doc_ids)) if n)

    entities = list(matched_nodes) + tree_entities
    if neighbor_names_lower and structure_kind not in {"tree", "page_index", "pageindex"}:
        nb_map, _ = await graph_search(index_name, kb_id, GRAPH_ENTITY_FIELDS, dict(scope, knowledge_graph_kwd=["entity"], name_kwd=sorted(neighbor_names_lower)), OrderByExpr(), GRAPH_EXPANSION_CAP)
        entities.extend(n for n in (project_entity(r) for r in nb_map.values() if _row_has_enabled_source(r, excluded_doc_ids)) if n)

    entities = dedup_entities(entities)
    if structure_kind in {"tree", "page_index", "pageindex"}:
        entity_names = {str(entity.get("name") or "").strip().lower() for entity in entities}
        relations = [relation for relation in relations if str(relation.get("from") or "").strip().lower() in entity_names and str(relation.get("to") or "").strip().lower() in entity_names]
    return bucket_meta, entities, normalize_relation_endpoints(entities, relations)
