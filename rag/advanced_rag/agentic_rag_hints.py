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

"""Compiled-hint read model for agentic RAG.

The knowledge-compilation pipeline persists several read-optimised views
of a KB into the doc store:

* ``compile_kwd="skill_all"`` — one row per KB. ``skill_with_weight`` is
  a JSON forest of skill nodes (each ``{skill_kwd, md_with_weight,
  children_kwd}``). Used for *hierarchical* dataset navigation: the LLM
  picks a folder, we load that folder's full node, recurse.
* ``compile_kwd="skill"`` — one row per skill node. ``md_with_weight`` is
  the full node markdown (frontmatter + overview + contents/doc list).
* ``compile_kwd="dataset_nav"`` — one row per KB. ``md_with_weight`` is a
  flat ``- **<doc_id>**: <summary>`` list produced by tree-kind compiles.
* ``compile_kwd="tree"`` — one row per (doc, tree template). JSON
  ``{entities, relations}`` projected from a RAPTOR tree.
* ``compile_kwd="artifact_page"`` — one row per wiki page in a KB.

This module exposes read + render helpers the LangGraph nodes call to
build the "let the LLM decide which doc / which chunk" prompts:

  - :func:`gather_dataset_hint`  — dataset-scope markdown (skill tree or
    dataset_nav) used at doc-selection time (step 4.1 in the spec).
  - :func:`load_skill_node`      — one skill node's markdown, for the
    hierarchical drill-down the frontend asked for.
  - :func:`gather_doc_hint`      — per-doc compiled markdown (tree /
    artifact) used at chunk-selection time (step 5.1).

All ES reads are wrapped in ``thread_pool_exec`` so the event loop is not
blocked. Missing rows degrade to empty strings — the graph then falls
back to the classic ES-search path.
"""

from __future__ import annotations

import json
import logging
from typing import Any


# Field names read from the doc-store rows. Kept as constants so a schema
# rename is a one-line change here.
_MD_FIELD = "md_with_weight"
_SKILL_FOREST_FIELD = "skill_with_weight"
_CONTENT_FIELD = "content_with_weight"


async def _search_first(
    tenant_id: str,
    kb_id: str,
    condition: dict[str, Any],
    fields: list[str],
) -> dict | None:
    """Return the first matching row's field-map, or ``None``.

    Thin wrapper over ``docStoreConn.search`` that mirrors the read
    pattern in ``dataset_nav.py`` / ``chunk_api.py``.
    """
    from common import settings
    from common.misc_utils import thread_pool_exec
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return None
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields, [], condition, [], OrderByExpr(), 0, 1, index, [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, fields)
    except Exception:
        logging.exception(
            "agentic_hints: search failed kb=%s cond=%s", kb_id, condition,
        )
        return None
    if not field_map:
        return None
    return next(iter(field_map.values()))


# --------------------------------------------------------------------
# Skill forest (hierarchical dataset navigation)
# --------------------------------------------------------------------


def _render_skill_forest_outline(forest: list[dict], depth: int = 0) -> list[str]:
    """Render the skill forest JSON into a shallow markdown outline.

    Only the frontmatter snippet (``md_with_weight`` on each node) plus
    the folder key (``skill_kwd``) are emitted, one bullet per node, so
    the LLM can pick a folder to drill into without us shipping the whole
    tree body. Children are listed by name only — the LLM asks for a
    specific node via :func:`load_skill_node` to see its contents.
    """
    lines: list[str] = []
    indent = "  " * depth
    for node in forest or []:
        if not isinstance(node, dict):
            continue
        key = node.get("skill_kwd") or ""
        snippet = (node.get(_MD_FIELD) or "").strip().replace("\n", " ")
        if len(snippet) > 200:
            snippet = snippet[:200] + "…"
        lines.append(f"{indent}- **{key}** — {snippet}")
        children = node.get("children_kwd") or []
        if children:
            lines.extend(_render_skill_forest_outline(children, depth + 1))
    return lines


async def gather_skill_outline(tenant_id: str, kb_id: str) -> str:
    """Return the KB's skill forest as a navigable markdown outline, or ''."""
    row = await _search_first(
        tenant_id, kb_id,
        {"compile_kwd": ["skill_all"]},
        ["id", _SKILL_FOREST_FIELD],
    )
    if not row:
        return ""
    raw = row.get(_SKILL_FOREST_FIELD) or ""
    if not raw:
        return ""
    try:
        forest = json.loads(raw)
    except Exception:
        logging.exception("agentic_hints: skill forest parse failed kb=%s", kb_id)
        return ""
    if not isinstance(forest, list):
        return ""
    lines = _render_skill_forest_outline(forest)
    return "\n".join(lines).strip()


async def load_skill_node(tenant_id: str, kb_id: str, skill_kwd: str) -> str:
    """Return one skill node's full markdown body (drill-down step), or ''.

    This is the hierarchical-navigate primitive: after the LLM picks a
    folder from :func:`gather_skill_outline`, we load that folder's full
    node — which lists its sub-groups and the docs directly under it.
    """
    if not skill_kwd:
        return ""
    row = await _search_first(
        tenant_id, kb_id,
        {"compile_kwd": ["skill"], "skill_kwd": [skill_kwd]},
        ["id", _MD_FIELD, "source_doc_ids", "children_kwd"],
    )
    if not row:
        return ""
    return (row.get(_MD_FIELD) or "").strip()


# --------------------------------------------------------------------
# dataset_nav (flat doc list from tree compiles)
# --------------------------------------------------------------------


async def gather_dataset_nav(tenant_id: str, kb_id: str) -> str:
    """Return the KB's dataset-nav markdown (flat ``doc_id: summary`` list), or ''."""
    row = await _search_first(
        tenant_id, kb_id,
        {"compile_kwd": ["dataset_nav"]},
        ["id", _MD_FIELD],
    )
    if not row:
        return ""
    return (row.get(_MD_FIELD) or "").strip()


async def gather_dataset_hint(tenant_id: str, kb_id: str) -> dict[str, str]:
    """Collect the dataset-scope hint used at document-selection time.

    Returns a dict with two possibly-empty markdown blobs::

        {"skill_outline": "...", "dataset_nav": "..."}

    The graph prefers ``skill_outline`` (hierarchical, navigable) when
    present and falls back to ``dataset_nav`` (flat) otherwise. Both are
    returned so the node can decide and so a KB that has both can present
    them together (the spec asked for *both formats*).
    """
    skill = await gather_skill_outline(tenant_id, kb_id)
    nav = await gather_dataset_nav(tenant_id, kb_id)
    return {"skill_outline": skill, "dataset_nav": nav}


# --------------------------------------------------------------------
# Per-doc compiled hints (tree / artifact)
# --------------------------------------------------------------------


def _render_tree_graph_outline(graph: dict) -> str:
    """Render a per-doc ``{entities, relations}`` tree graph as an outline.

    Entities become bullets keyed by name/description; relations
    (``type='child'``) drive nesting. Falls back to a flat entity list
    when the relations don't form a clean parent chain.
    """
    entities = graph.get("entities") or []
    relations = graph.get("relations") or []
    if not entities:
        return ""

    by_name: dict[str, dict] = {}
    for e in entities:
        if isinstance(e, dict) and e.get("name"):
            by_name[str(e["name"])] = e

    children: dict[str, list[str]] = {}
    has_parent: set[str] = set()
    for r in relations:
        if not isinstance(r, dict):
            continue
        src, tgt = str(r.get("from") or ""), str(r.get("to") or "")
        if src and tgt and src in by_name and tgt in by_name:
            children.setdefault(src, []).append(tgt)
            has_parent.add(tgt)

    roots = [n for n in by_name if n not in has_parent]
    if not roots:
        # No clean hierarchy — flat list.
        return "\n".join(f"- {n}" for n in by_name)

    lines: list[str] = []
    seen: set[str] = set()

    def _walk(name: str, depth: int) -> None:
        if name in seen or depth > 12:
            return
        seen.add(name)
        ent = by_name.get(name) or {}
        desc = (ent.get("description") or "").strip().replace("\n", " ")
        if len(desc) > 160:
            desc = desc[:160] + "…"
        indent = "  " * depth
        lines.append(f"{indent}- **{name}**{(': ' + desc) if desc else ''}")
        for child in children.get(name, []):
            _walk(child, depth + 1)

    for root in roots:
        _walk(root, 0)
    return "\n".join(lines).strip()


async def gather_doc_hint(tenant_id: str, kb_id: str, doc_id: str) -> str:
    """Return per-doc compiled markdown for the chunk-selection step, or ''.

    Prefers the per-doc ``tree`` graph (an outline the LLM can point at to
    pick chunk-bearing nodes). When no tree exists, returns '' and the
    graph falls back to classic ES retrieval for that doc.
    """
    row = await _search_first(
        tenant_id, kb_id,
        {"compile_kwd": ["tree"], "doc_id": [doc_id]},
        ["id", _CONTENT_FIELD],
    )
    if not row:
        return ""
    raw = row.get(_CONTENT_FIELD) or ""
    if not raw:
        return ""
    try:
        graph = json.loads(raw)
    except Exception:
        logging.exception(
            "agentic_hints: doc tree parse failed kb=%s doc=%s", kb_id, doc_id,
        )
        return ""
    if not isinstance(graph, dict):
        return ""
    return _render_tree_graph_outline(graph)
