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
the dataset-wide (``/datasets/<id>/artifacts_structure``) endpoints render
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

from common import settings
from common.doc_store.doc_store_base import OrderByExpr
from common.misc_utils import thread_pool_exec


# Below this combined (entities + relations) count for a bucket, return all rows.
GRAPH_FULL_THRESHOLD = 1024
# Size of the top-mention entity seed set (set A) for large buckets.
GRAPH_TOP_ENTITIES = 256
# Upper bound on the relation / neighbor-entity expansion so a hub node can't
# blow up the response.
GRAPH_EXPANSION_CAP = 4096

GRAPH_ENTITY_FIELDS = ["id", "content_with_weight", "name_kwd", "mention_count_int", "source_chunk_ids"]
GRAPH_RELATION_FIELDS = ["id", "content_with_weight", "from_entity_kwd", "to_entity_kwd"]
GRAPH_ALL_FIELDS = ["id", "content_with_weight", "name_kwd", "mention_count_int", "source_chunk_ids", "from_entity_kwd", "to_entity_kwd", "knowledge_graph_kwd"]


async def graph_search(index_name, kb_id, select_fields, condition, order_by, limit, match_expressions=None):
    """One raw-row search. Returns ``(field_map, total)`` where ``total`` is the
    full match count (not the returned slice)."""
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        select_fields,
        [],
        condition,
        match_expressions or [],
        order_by,
        0,
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


def filter_entities_with_relations(entities: list[dict], relations: list[dict]) -> list[dict]:
    """Keep only entities that are referenced by at least one relation."""
    if not entities or not relations:
        return []

    connected: set[str] = set()
    for relation in relations:
        if not isinstance(relation, dict):
            continue
        for endpoint_key in ("from", "to"):
            endpoint = relation.get(endpoint_key)
            if isinstance(endpoint, str):
                endpoint = endpoint.strip()
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
                value = value.strip()
                if value:
                    keys.add(value)
        if keys & connected:
            filtered.append(entity)
    return filtered


async def build_bucket(index_name, kb_id, scope: dict) -> tuple[list[dict], list[dict]]:
    """Build one bucket's ``(entities, relations)`` from raw rows.

    ``scope`` is the filter WITHOUT ``knowledge_graph_kwd`` — e.g.
    ``{"doc_id":[id], "compilation_template_ids":[tid]}`` (document scope) or
    ``{"compilation_template_ids":[tid]}`` (dataset scope). Small buckets are
    returned whole; large ones are sampled: top-``GRAPH_TOP_ENTITIES`` entities
    by ``mention_count_int``, the relations sourced from them, and those
    relations' target entities.
    """
    both_cond = dict(scope, knowledge_graph_kwd=["entity", "relation"])
    _, total = await graph_search(index_name, kb_id, ["id"], both_cond, OrderByExpr(), 1)

    if total < GRAPH_FULL_THRESHOLD:
        field_map, _ = await graph_search(index_name, kb_id, GRAPH_ALL_FIELDS, both_cond, OrderByExpr(), total or 1)
        entities: list[dict] = []
        relations: list[dict] = []
        for row in field_map.values():
            if row.get("knowledge_graph_kwd") == "relation":
                edge = project_relation(row)
                if edge:
                    relations.append(edge)
            else:
                node = project_entity(row)
                if node:
                    entities.append(node)
        return dedup_entities(entities), relations

    # Large bucket: sample. A = top entities by mention_count_int desc.
    order_by = OrderByExpr()
    try:
        order_by.desc("mention_count_int")
    except Exception:
        order_by = OrderByExpr()
    ent_a_map, _ = await graph_search(index_name, kb_id, GRAPH_ENTITY_FIELDS, dict(scope, knowledge_graph_kwd=["entity"]), order_by, GRAPH_TOP_ENTITIES)
    set_a = [n for n in (project_entity(r) for r in ent_a_map.values()) if n]
    a_names = sorted({str(e.get("name") or "").strip() for e in set_a if str(e.get("name") or "").strip()})

    # relations whose source is one of A.
    relations = []
    target_names_lower: set[str] = set()
    if a_names:
        rel_map, _ = await graph_search(index_name, kb_id, GRAPH_RELATION_FIELDS, dict(scope, knowledge_graph_kwd=["relation"], from_entity_kwd=a_names), OrderByExpr(), GRAPH_EXPANSION_CAP)
        for row in rel_map.values():
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
        set_t = [n for n in (project_entity(r) for r in tgt_map.values()) if n]

    return dedup_entities(set_a + set_t), relations


async def keyword_subgraph(index_name, kb_id, embd_mdl, base_entity_condition, keywords, scope_for_template, log_ctx="") -> tuple[dict | None, list[dict], list[dict]]:
    """KNN the entity rows matching ``base_entity_condition`` for ``keywords``;
    return ``(top1_bucket_meta, entities, relations)`` for the top-1 entity's
    1-hop subgraph (top-1 + neighbors + touching relations). ``(None, [], [])``
    when nothing matches or embedding is unavailable.

    ``base_entity_condition`` scopes the KNN (e.g. ``{"doc_id":[id],
    "knowledge_graph_kwd":["entity"]}`` or ``{"compilation_template_ids":[...],
    "knowledge_graph_kwd":["entity"]}``). ``scope_for_template(row)`` resolves
    ``(bucket_meta, scope_filter)`` for the matched row (scope WITHOUT
    ``knowledge_graph_kwd``).
    """
    from common.doc_store.doc_store_base import MatchDenseExpr

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
        topn=1,
        extra_options={"similarity": 0.0},
    )
    top_fields = GRAPH_ENTITY_FIELDS + ["compilation_template_ids", "compile_kwd", "compilation_template_kind_kwd"]
    top_map, _ = await graph_search(index_name, kb_id, top_fields, base_entity_condition, OrderByExpr(), 1, match_expressions=[match_expr])
    if not top_map:
        return None, [], []
    top_row = next(iter(top_map.values()))
    top_node = project_entity(top_row)
    if not top_node:
        return None, [], []
    top_name = str(top_node.get("name") or "").strip()
    if not top_name:
        return None, [], []
    bucket_meta, scope = scope_for_template(top_row)

    # Relations where the top-1 entity is source OR target (two term queries).
    relations: list[dict] = []
    seen_rel: set[tuple[str, str, str]] = set()
    neighbor_names_lower: set[str] = set()
    for field in ("from_entity_kwd", "to_entity_kwd"):
        rel_map, _ = await graph_search(index_name, kb_id, GRAPH_RELATION_FIELDS, dict(scope, knowledge_graph_kwd=["relation"], **{field: [top_name]}), OrderByExpr(), GRAPH_EXPANSION_CAP)
        for row in rel_map.values():
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
                if endpoint and endpoint.lower() != top_name.lower():
                    neighbor_names_lower.add(endpoint.lower())

    entities = [top_node]
    if neighbor_names_lower:
        nb_map, _ = await graph_search(index_name, kb_id, GRAPH_ENTITY_FIELDS, dict(scope, knowledge_graph_kwd=["entity"], name_kwd=sorted(neighbor_names_lower)), OrderByExpr(), GRAPH_EXPANSION_CAP)
        entities.extend(n for n in (project_entity(r) for r in nb_map.values()) if n)

    return bucket_meta, dedup_entities(entities), relations
