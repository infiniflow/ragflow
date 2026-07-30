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

"""KB-wide structure-graph merge task (incremental, bucketed).

Triggered when the user POSTs to ``/datasets/<id>/index`` with a structure
index type.  Scans ``scope_kwd="doc"`` entity rows grouped by ``(name, type)``,
merges them into ``scope_kwd="dataset"`` rows with bucketing for bounded memory.

Key design:
  - 256 hash buckets — all rows accumulated in memory, merged once after scan
  - Incremental: only processes doc_graph rows changed since ``last_build_time``
  - Document deletions tracked via ``record_doc_deletion()`` – ghost entities
    are cleaned up incrementally at the start of the next build
  - Template changes still require a full rebuild
  - Resulting dataset_graph rows are searchable (``available_int=1``)
"""

from __future__ import annotations

import json
import logging
from typing import Optional

import xxhash

from common import settings
from common.misc_utils import thread_pool_exec
from rag.nlp import search
from rag.advanced_rag.knowlege_compile._common import (
    encode as _encode,
    tokenize_for_search as _tokenize_for_search,
    stable_row_id as _stable_row_id,
)
from rag.svr.task_executor_refactor.task_context import TaskContext

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_BUCKET_COUNT = 256
_BUCKET_FLUSH_THRESHOLD = 500
_PAGE_SIZE = 1000
_SCOPE_KWD_DOC = "doc"
_SCOPE_KWD_DATASET = "dataset"
_DELETION_META_KWD = "doc_deleted"

# Row id seeds for metadata
_META_ROW_KWD = "kg_build_meta"

STRUCTURE_MERGE_TASK_TYPES = frozenset(
    {
        "structure_graph",
        "structure_mindmap",
        "timeline",
        "session_graph",
        "session_essence",
        "structure",
    }
)


def is_structure_merge_task(task_type: str) -> bool:
    return (task_type or "").lower() in STRUCTURE_MERGE_TASK_TYPES


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _index_name(tenant_id: str) -> str:
    return search.index_name(tenant_id)


def _meta_row_id(kb_id: str, compile_kwd: str, template_id: str | None) -> str:
    return xxhash.xxh64(
        f"{_META_ROW_KWD}:{kb_id}:{compile_kwd}:{template_id or ''}".encode("utf-8", "surrogatepass"),
    ).hexdigest()


def _dataset_entity_row_id(kb_id: str, compile_kwd: str, template_id: str | None, name: str) -> str:
    return _stable_row_id(name, kb_id, compile_kwd, template_id or "", _SCOPE_KWD_DATASET)


def _dataset_relation_row_id(kb_id: str, compile_kwd: str, template_id: str | None, src: str, tgt: str, rel_type: str) -> str:
    key = f"{src.lower()} -> {rel_type.lower()} -> {tgt.lower()}"
    return _stable_row_id(key, kb_id, compile_kwd, template_id or "", _SCOPE_KWD_DATASET)


# ---------------------------------------------------------------------------
# Document-store I/O
# ---------------------------------------------------------------------------


async def _index_search(
    tenant_id: str,
    kb_id: str,
    condition: dict,
    fields: list[str],
    limit: int = 10000,
    offset: int = 0,
) -> list[dict]:
    from common.doc_store.doc_store_base import OrderByExpr

    index = _index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return []
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            condition,
            [],
            OrderByExpr(),
            offset,
            limit,
            index,
            [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, fields) or {}
        return list(field_map.values())
    except Exception:
        logging.exception("structure_merge: search failed for kb=%s", kb_id)
        return []


async def _index_delete(tenant_id: str, kb_id: str, condition: dict) -> None:
    index = _index_name(tenant_id)
    try:
        await thread_pool_exec(settings.docStoreConn.delete, condition, index, kb_id)
    except Exception:
        logging.exception("structure_merge: delete failed for kb=%s cond=%s", kb_id, condition)


async def _index_insert(tenant_id: str, kb_id: str, rows: list[dict]) -> None:
    if not rows:
        return
    index = _index_name(tenant_id)
    try:
        await thread_pool_exec(settings.docStoreConn.insert, rows, index, kb_id)
    except Exception:
        logging.exception("structure_merge: insert failed for kb=%s", kb_id)


# ---------------------------------------------------------------------------
# Document deletion tracking (sync, called from DocumentService.remove_document)
# ---------------------------------------------------------------------------


def record_doc_deletion(tenant_id: str, kb_id: str, doc_id: str) -> None:
    """Record a document deletion for incremental ghost cleanup.

    Creates a lightweight ES meta row so that the next incremental merge build
    can look up which docs were deleted and clean up orphaned dataset-level
    entity rows (those whose *only* source document was the deleted doc).

    The deleted doc ID is stored in **deleted_doc_id** (not ``doc_id``) so the
    row survives the ``doc_id``-based chunk sweep that runs in
    ``DocumentService.remove_document``.

    Must be callable from a synchronous context (``DocumentService.remove_document``).
    """
    import datetime

    try:
        index = search.index_name(tenant_id)
        if not settings.docStoreConn.index_exist(index, kb_id):
            return
        row = {
            "id": f"{_DELETION_META_KWD}:{kb_id}:{doc_id}",
            "kb_id": kb_id,
            "deleted_doc_id": doc_id,
            "knowledge_graph_kwd": _DELETION_META_KWD,
            "compile_kwd": "*",
            "create_timestamp_flt": datetime.datetime.now().timestamp(),
        }
        settings.docStoreConn.insert([row], index, kb_id)
    except Exception:
        logging.exception("structure_merge: failed to record doc deletion kb=%s doc=%s", kb_id, doc_id)


# ---------------------------------------------------------------------------
# Merge logic
# ---------------------------------------------------------------------------


async def _load_last_build_time(tenant_id: str, kb_id: str, compile_kwd: str, template_id: str | None) -> float | None:
    """Read the last build timestamp from a metadata row."""
    meta_id = _meta_row_id(kb_id, compile_kwd, template_id)
    index = _index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return None
    try:
        row = await thread_pool_exec(settings.docStoreConn.get, meta_id, index, [kb_id])
        if row:
            ts = row.get("build_timestamp_flt")
            if ts is not None and isinstance(ts, (int, float)) and ts > 0:
                return float(ts)
    except Exception:
        logging.exception("structure_merge: failed to load build time for kb=%s compile=%s", kb_id, compile_kwd)
    return None


async def _save_build_time(tenant_id: str, kb_id: str, compile_kwd: str, template_id: str | None, timestamp: float) -> None:
    """Persist the build timestamp."""
    import datetime

    meta_id = _meta_row_id(kb_id, compile_kwd, template_id)
    index = _index_name(tenant_id)
    payload = {
        "id": meta_id,
        "kb_id": kb_id,
        "doc_id": kb_id,
        "compile_kwd": compile_kwd,
        "knowledge_graph_kwd": _META_ROW_KWD,
        "build_timestamp_flt": timestamp,
        "create_time": datetime.datetime.now().isoformat(),
    }
    existing = await thread_pool_exec(settings.docStoreConn.get, meta_id, index, [kb_id])
    if existing:
        await thread_pool_exec(settings.docStoreConn.update, {"id": meta_id}, payload, index, kb_id)
    else:
        await thread_pool_exec(settings.docStoreConn.insert, [payload], index, kb_id)


def _bucket_id(name: str, bucket_count: int = _BUCKET_COUNT) -> int:
    """Determine the hash bucket for an entity name."""
    return xxhash.xxh64(name.encode("utf-8", "surrogatepass")).intdigest() % bucket_count


async def _merge_bucket(
    tenant_id: str,
    kb_id: str,
    compile_kwd: str,
    template_id: str | None,
    bucket_rows: list[dict],
    embd_mdl,
    structure_kind: str | None = None,
) -> list[dict]:
    """Flush one bucket: group by (name, type), merge, build dataset rows.

    Returns the list of entity/relation rows to insert.
    """
    from rag.advanced_rag.knowlege_compile.structure import _struct_entity_name

    # Group by (name, type)
    groups: dict[tuple[str, str], list[dict]] = {}
    for row in bucket_rows:
        payload = json.loads(row.get("content_with_weight") or "{}")
        name = _struct_entity_name(payload) or ""
        typ = payload.get("type", "other")
        key = (name.lower(), str(typ).strip())
        groups.setdefault(key, []).append(row)

    rows_out: list[dict] = []
    for (name_lower, typ), rows in groups.items():
        # Collect all source_chunk_ids, doc_ids, and original-cased names
        all_chunks: list[str] = []
        all_docs: list[str] = []
        all_descriptions: list[str] = []
        all_original_names: list[str] = []
        mention_count = 0
        for r in rows:
            payload = json.loads(r.get("content_with_weight") or "{}")
            chunks = r.get("source_chunk_ids") or []
            for c in chunks:
                if c not in all_chunks:
                    all_chunks.append(c)
            doc = r.get("doc_id") or ""
            if doc and doc not in all_docs:
                all_docs.append(doc)
            mention_count += int(r.get("mention_count_int") or payload.get("mention_count", 1))
            desc = payload.get("description", "")
            if desc and desc not in all_descriptions:
                all_descriptions.append(desc)
            orig = _struct_entity_name(payload) or ""
            if orig and orig not in all_original_names:
                all_original_names.append(orig)

        # Use the longest description (most complete) and best original-cased name
        best_desc = max(all_descriptions, key=len) if all_descriptions else name_lower
        best_orig_name = max(all_original_names, key=len) if all_original_names else name_lower
        payload_out = {"name": best_orig_name, "type": typ, "description": best_desc, "mention_count": mention_count}
        ltks, sm_ltks = _tokenize_for_search(best_desc)

        row_id = _dataset_entity_row_id(kb_id, compile_kwd, template_id, name_lower)
        row = {
            "id": row_id,
            "content_with_weight": json.dumps(payload_out, ensure_ascii=False),
            "compile_kwd": compile_kwd,
            "knowledge_graph_kwd": "entity",
            "scope_kwd": _SCOPE_KWD_DATASET,
            "doc_id": kb_id,
            "kb_id": kb_id,
            "name_kwd": name_lower,
            "source_chunk_ids": all_chunks,
            "doc_ids_kwd": all_docs,
            "content_ltks": ltks,
            "content_sm_ltks": sm_ltks,
            "mention_count_int": mention_count,
            "available_int": 1,
        }
        if template_id:
            row["compilation_template_ids"] = [template_id]
        if structure_kind:
            row["compilation_template_kind_kwd"] = str(structure_kind)
        # Re-embed
        if embd_mdl:
            vecs = await _encode(embd_mdl, [best_desc])
            if vecs and len(vecs[0]) > 0:
                dim = len(vecs[0])
                row[f"q_{dim}_vec"] = list(vecs[0])
        rows_out.append(row)

    return rows_out


async def _merge_relations(
    tenant_id: str,
    kb_id: str,
    compile_kwd: str,
    template_id: str | None,
    rel_bucket_rows: list[dict],
    embd_mdl,
    structure_kind: str | None = None,
) -> list[dict]:
    """Group doc-graph relation rows by (src, rel_type, tgt) and merge.

    Each group produces one dataset-scoped relation row.  Duplicate
    ``source_chunk_ids`` and ``doc_ids`` are deduplicated per group.
    When *embd_mdl* is provided the best description is re-embedded and
    stored as ``q_<dim>_vec``, matching the entity path.
    """
    groups: dict[tuple[str, str, str], list[dict]] = {}
    for row in rel_bucket_rows:
        payload = json.loads(row.get("content_with_weight") or "{}")
        src = (payload.get("from") or row.get("from_entity_kwd") or "").strip().lower()
        tgt = (payload.get("to") or row.get("to_entity_kwd") or "").strip().lower()
        rel_type = (payload.get("type") or "related").strip().lower()
        key = (src, rel_type, tgt)
        groups.setdefault(key, []).append(row)

    rows_out: list[dict] = []
    for (src, rel_type, tgt), rows in groups.items():
        all_chunks: list[str] = []
        all_docs: list[str] = []
        # Use the longest description for tokenization
        all_desc: list[str] = []
        for r in rows:
            payload = json.loads(r.get("content_with_weight") or "{}")
            chunks = r.get("source_chunk_ids") or []
            for c in chunks:
                if c not in all_chunks:
                    all_chunks.append(c)
            doc = r.get("doc_id") or ""
            if doc and doc not in all_docs:
                all_docs.append(doc)
            desc = payload.get("description") or f"{src} {rel_type} {tgt}"
            if desc not in all_desc:
                all_desc.append(desc)
        best_desc = max(all_desc, key=len)
        ltks, sm_ltks = _tokenize_for_search(best_desc)

        row_id = _dataset_relation_row_id(kb_id, compile_kwd, template_id, src, tgt, rel_type)
        row = {
            "id": row_id,
            "content_with_weight": json.dumps({"from": src, "to": tgt, "type": rel_type}, ensure_ascii=False),
            "compile_kwd": compile_kwd,
            "knowledge_graph_kwd": "relation",
            "scope_kwd": _SCOPE_KWD_DATASET,
            "doc_id": kb_id,
            "kb_id": kb_id,
            "from_entity_kwd": src,
            "to_entity_kwd": tgt,
            "source_chunk_ids": all_chunks,
            "doc_ids_kwd": all_docs,
            "content_ltks": ltks,
            "content_sm_ltks": sm_ltks,
            "available_int": 1,
        }
        if template_id:
            row["compilation_template_ids"] = [template_id]
        if structure_kind:
            row["compilation_template_kind_kwd"] = str(structure_kind)
        # Re-embed (matching _merge_bucket)
        if embd_mdl:
            vecs = await _encode(embd_mdl, [best_desc])
            if vecs and len(vecs[0]) > 0:
                dim = len(vecs[0])
                row[f"q_{dim}_vec"] = list(vecs[0])
        rows_out.append(row)

    return rows_out


# ---------------------------------------------------------------------------
# Deletion-driven ghost cleanup
# ---------------------------------------------------------------------------


async def _cleanup_deleted_docs(
    tenant_id: str,
    kb_id: str,
    compile_kwd: str,
    template_id: str | None,
) -> bool:
    """Clean dataset-level entity rows whose *only* source doc was deleted.

    Called once per (compile_kwd, template_id) at the beginning of an
    incremental build.  Works in two passes:

    1. Read all pending ``doc_deleted`` meta rows for this kb (stored under
       ``deleted_doc_id``).
    2. Find dataset entity/relation rows whose ``doc_ids_kwd`` references a
       deleted doc.  If after removing the deleted doc ID the ``doc_ids_kwd``
       list is empty the row is a **ghost** and gets deleted.

    Shared entities (those still referenced by other live docs) keep their
    existing content — the next incremental re-merge will correct any stale
    descriptions or mention counts.

    Returns ``True`` when Pass 2 completed without store errors, ``False``
    otherwise.  The caller must delete the processed meta rows only after
    **all** (compile_kwd, template_id) pairs have been handled successfully.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    index = _index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return False

    # ── Pass 1: collect pending deletions ────────────────────────────
    cursor: dict[str, str] = {}  # meta_row_id → deleted_doc_id
    offset = 0
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["id", "deleted_doc_id"],
                [],
                {"kb_id": [kb_id], "knowledge_graph_kwd": [_DELETION_META_KWD]},
                [],
                OrderByExpr(),
                offset,
                _PAGE_SIZE,
                index,
                [kb_id],
            )
            fm = settings.docStoreConn.get_fields(res, ["id", "deleted_doc_id"]) or {}
        except Exception:
            break
        if not fm:
            break
        for row in fm.values():
            did = row.get("deleted_doc_id")
            if did:
                cursor[row["id"]] = did
        if len(fm) < _PAGE_SIZE:
            break
        offset += _PAGE_SIZE

    if not cursor:
        return True  # no markers — nothing to do
    deleted_doc_ids = list(set(cursor.values()))

    # ── Pass 2: find affected dataset rows and remove ghosts ─────────
    dataset_del_cond: dict = {
        "scope_kwd": [_SCOPE_KWD_DATASET],
        "compile_kwd": [compile_kwd],
        "kb_id": [kb_id],
        "doc_ids_kwd": deleted_doc_ids,
    }
    if template_id:
        dataset_del_cond["compilation_template_ids"] = [template_id]

    ids_to_delete: list[str] = []
    ids_to_update: list[tuple[str, list[str]]] = []  # (row_id, new_doc_ids)
    pass2_ok = True
    offset = 0
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["id", "doc_ids_kwd"],
                [],
                dataset_del_cond,
                [],
                OrderByExpr(),
                offset,
                _PAGE_SIZE,
                index,
                [kb_id],
            )
            fm = settings.docStoreConn.get_fields(res, ["id", "doc_ids_kwd"]) or {}
        except Exception:
            pass2_ok = False
            break
        if not fm:
            break
        for row in fm.values():
            current: list = row.get("doc_ids_kwd") or []
            if not isinstance(current, list):
                current = []
            remaining = [d for d in current if d not in deleted_doc_ids]
            if not remaining:
                ids_to_delete.append(row["id"])
            elif len(remaining) < len(current):
                ids_to_update.append((row["id"], remaining))
        if len(fm) < _PAGE_SIZE:
            break
        offset += _PAGE_SIZE

    if not pass2_ok:
        return False  # marker rows preserved for retry

    # Execute deletions
    if ids_to_delete:
        logging.info(
            "structure_merge: deleting %d ghost dataset rows for kb=%s compile=%s",
            len(ids_to_delete),
            kb_id,
            compile_kwd,
        )
        await _index_delete(tenant_id, kb_id, {"id": ids_to_delete})

    # Execute updates (remove stale doc_id from doc_ids_kwd)
    if ids_to_update:
        logging.info(
            "structure_merge: updating doc_ids_kwd on %d shared dataset rows for kb=%s compile=%s",
            len(ids_to_update),
            kb_id,
            compile_kwd,
        )
        for rid, remaining in ids_to_update:
            try:
                await thread_pool_exec(
                    settings.docStoreConn.update,
                    {"id": [rid]},
                    {"doc_ids_kwd": remaining},
                    index,
                    kb_id,
                )
            except Exception:
                logging.exception(
                    "structure_merge: failed to update doc_ids_kwd on row %s",
                    rid,
                )

    # Markers are NOT deleted here — caller (run_structure_merge) deletes
    # them once after all (compile_kwd, template_id) pairs succeed.
    return True


async def _consume_deletion_markers(tenant_id: str, kb_id: str) -> None:
    """Remove all pending ``doc_deleted`` meta rows for *kb_id*.

    Called from :func:`run_structure_merge` *after* every eligible
    ``(compile_kwd, template_id)`` pair has been processed successfully.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    index = _index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return
    offset = 0
    all_marker_ids: list[str] = []
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["id"],
                [],
                {"kb_id": [kb_id], "knowledge_graph_kwd": [_DELETION_META_KWD]},
                [],
                OrderByExpr(),
                offset,
                _PAGE_SIZE,
                index,
                [kb_id],
            )
            fm = settings.docStoreConn.get_fields(res, ["id"]) or {}
        except Exception:
            break
        if not fm:
            break
        all_marker_ids.extend(row["id"] for row in fm.values() if row.get("id"))
        if len(fm) < _PAGE_SIZE:
            break
        offset += _PAGE_SIZE
    if all_marker_ids:
        await _index_delete(tenant_id, kb_id, {"id": all_marker_ids})


async def _do_build(
    tenant_id: str,
    kb_id: str,
    compile_kwd: str,
    template_id: str | None,
    structure_kind: str | None,
    embd_mdl,
    incremental: bool = True,
) -> bool:
    """Core build logic — read doc_graph rows, bucket, merge, write dataset rows.

    When *incremental* is True, only processes rows changed since the last build
    timestamp.  When False (full rebuild), reads ALL doc_graph rows for the
    given (compile_kwd, template) and deletes existing dataset rows first.

    Document deletions are tracked via :func:`record_doc_deletion` and cleaned
    up incrementally (:func:`_cleanup_deleted_docs`) so that deleting a
    document no longer forces a full rebuild.

    Returns ``True`` if deletion markers were processed successfully (or no
    markers existed), ``False`` if a store error prevented cleanup.
    """
    cleanup_ok = True

    index = _index_name(tenant_id)

    # ── Determine the time window ─────────────────────────────────────
    last_build_time = None
    if incremental:
        last_build_time = await _load_last_build_time(tenant_id, kb_id, compile_kwd, template_id)

    # ── Full rebuild: delete existing dataset rows ────────────────────
    if not incremental or last_build_time is None:
        del_cond: dict = {
            "scope_kwd": [_SCOPE_KWD_DATASET],
            "compile_kwd": [compile_kwd],
            "kb_id": [kb_id],
        }
        if template_id:
            del_cond["compilation_template_ids"] = [template_id]
        offset = 0
        while True:
            existing = await _index_search(
                tenant_id,
                kb_id,
                del_cond,
                ["id"],
                limit=1000,
                offset=offset,
            )
            if not existing:
                break
            ids = [r["id"] for r in existing if r.get("id")]
            if ids:
                await _index_delete(tenant_id, kb_id, {"id": ids})
            if len(existing) < 1000:
                break
            offset += 1000
        # Also delete the meta rows
        meta_id = _meta_row_id(kb_id, compile_kwd, template_id)
        try:
            await thread_pool_exec(settings.docStoreConn.delete, {"id": [meta_id]}, index, kb_id)
        except Exception:
            pass

    # ── Incremental: clean up ghosts from deleted docs ───────────────
    if incremental and last_build_time is not None:
        cleanup_ok = await _cleanup_deleted_docs(tenant_id, kb_id, compile_kwd, template_id)

    # ── Scan doc_graph rows ──────────────────────────────────────────
    # Backward compat: existing doc_graph rows don't have scope_kwd.
    # Search for (scope_kwd="doc" OR (not exists scope_kwd)) AND compile_kwd.
    base_cond = {
        "compile_kwd": [compile_kwd],
        "knowledge_graph_kwd": ["entity", "relation"],
    }
    if template_id:
        base_cond["compilation_template_ids"] = [template_id]
    scan_all = not incremental or last_build_time is None
    if not scan_all:
        base_cond["scope_kwd"] = [_SCOPE_KWD_DOC]

    # Build timestamp filter for incremental mode
    ts_cond = None
    if not scan_all and last_build_time is not None:
        ts_cond = {"create_timestamp_flt": {"gte": last_build_time}}

    limit = _PAGE_SIZE
    offset = 0
    buckets: list[list] = [[] for _ in range(_BUCKET_COUNT)]
    rel_buckets: list[list] = [[] for _ in range(_BUCKET_COUNT)]

    last_page = False
    while not last_page:
        rows = await _index_search(
            tenant_id,
            kb_id,
            base_cond,
            [
                "id",
                "content_with_weight",
                "source_chunk_ids",
                "doc_id",
                "name_kwd",
                "mention_count_int",
                "compilation_template_ids",
                "create_timestamp_flt",
                "from_entity_kwd",
                "to_entity_kwd",
            ],
            limit=limit,
            offset=offset,
        )
        if not rows:
            break
        for row in rows:
            # Time filter (incremental mode)
            if ts_cond:
                ts = row.get("create_timestamp_flt", 0)
                if isinstance(ts, (int, float)) and ts < ts_cond["create_timestamp_flt"]["gte"]:
                    continue
            is_rel = row.get("knowledge_graph_kwd") == "relation" or "from_entity_kwd" in row
            if is_rel:
                # Bucketize relations by stable hash of src/type/tgt
                payload = json.loads(row.get("content_with_weight") or "{}")
                rsrc = (payload.get("from") or row.get("from_entity_kwd") or "").strip().lower()
                rtgt = (payload.get("to") or row.get("to_entity_kwd") or "").strip().lower()
                rtype = (payload.get("type") or "related").strip().lower()
                rbid = _bucket_id(f"{rsrc}:{rtype}:{rtgt}")
                if rbid < _BUCKET_COUNT:
                    rel_buckets[rbid].append(row)
            else:
                name = row.get("name_kwd", "")
                if not name:
                    continue
                bid = _bucket_id(name)
                if bid < _BUCKET_COUNT:
                    buckets[bid].append(row)
        if len(rows) < limit:
            last_page = True
        else:
            offset += limit

    # Merge entity buckets into dataset rows
    for bi in range(_BUCKET_COUNT):
        if buckets[bi]:
            out = await _merge_bucket(
                tenant_id,
                kb_id,
                compile_kwd,
                template_id,
                buckets[bi],
                embd_mdl,
                structure_kind,
            )
            if out:
                for start in range(0, len(out), _BUCKET_FLUSH_THRESHOLD):
                    await _index_insert(tenant_id, kb_id, out[start : start + _BUCKET_FLUSH_THRESHOLD])

    # Merge relation buckets into dataset rows
    for bi in range(_BUCKET_COUNT):
        if rel_buckets[bi]:
            rel_out = await _merge_relations(
                tenant_id,
                kb_id,
                compile_kwd,
                template_id,
                rel_buckets[bi],
                embd_mdl,
                structure_kind,
            )
            if rel_out:
                for start in range(0, len(rel_out), _BUCKET_FLUSH_THRESHOLD):
                    await _index_insert(tenant_id, kb_id, rel_out[start : start + _BUCKET_FLUSH_THRESHOLD])

    # ── Save build timestamp ─────────────────────────────────────────
    import datetime

    await _save_build_time(tenant_id, kb_id, compile_kwd, template_id, datetime.datetime.now().timestamp())
    return cleanup_ok


async def run_structure_merge(ctx: TaskContext) -> None:
    """Entry point — called from task_handler.py when ``is_structure_merge_task`` matches."""
    from api.db.services.compilation_template_service import CompilationTemplateService

    progress = ctx.progress_cb
    task_type = (ctx.task_type or "").lower()
    target_kind = {
        "structure_graph": "knowledge_graph",
        "structure_mindmap": "mind_map",
        "timeline": "timeline",
        "session_graph": "session_graph",
        "session_essence": "session_essence",
    }.get(task_type)
    merge_all = task_type == "structure"

    # Resolve embedding model (required for re-embedding dataset rows)
    embd_mdl = None
    try:
        from api.db.services.llm_service import LLMBundle
        from api.apps.services.dataset_api_service import resolve_model_config
        from common.constants import LLMType

        from api.db.services.knowledgebase_service import KnowledgebaseService

        ok, kb = KnowledgebaseService.get_by_id(ctx.kb_id)
        if ok and kb:
            model_config = resolve_model_config(ctx.tenant_id, LLMType.EMBEDDING.value, kb.embd_id)
            embd_mdl = LLMBundle(ctx.tenant_id, model_config, lang=ctx.language)
        else:
            raise RuntimeError(f"Knowledge base {ctx.kb_id} not found")
    except Exception:
        logging.exception("structure_merge: failed to bind embedding model for kb=%s", ctx.kb_id)
        progress(1.0, "Failed to bind embedding model — aborting.")
        return

    progress(0.0, "Scanning doc_graph rows...")
    pairs = await _collect_structure_pairs(ctx.tenant_id, ctx.kb_id)
    if not pairs:
        progress(1.0, "No doc_graph rows found.")
        return

    template_meta: dict[str, tuple[bool, Optional[str]]] = {}

    def _resolve_template(template_id: str) -> tuple[bool, Optional[str]]:
        cached = template_meta.get(template_id)
        if cached is not None:
            return cached
        keep = False
        structure_kind: Optional[str] = None
        try:
            saved = CompilationTemplateService.get_saved(template_id, ctx.tenant_id)
            if saved:
                structure_kind = (saved.get("kind") or "").strip() or None
                kind_ok = merge_all or (structure_kind == target_kind)
                keep = kind_ok
        except Exception:
            logging.exception("structure_merge: failed to resolve template %s", template_id)
        result = (keep, structure_kind)
        template_meta[template_id] = result
        return result

    eligible = []
    for compile_kwd, template_id in sorted(pairs):
        keep, structure_kind = _resolve_template(template_id)
        if keep:
            eligible.append((compile_kwd, template_id, structure_kind))

    if not eligible:
        kind_label = "any kind" if merge_all else (target_kind or task_type)
        progress(1.0, f"No eligible templates for {kind_label}.")
        return

    total = len(eligible)
    rebuilt = 0
    all_cleanup_ok = True
    for i, (compile_kwd, template_id, structure_kind) in enumerate(eligible):
        if ctx.has_canceled_func(ctx.id):
            progress(1.0, f"Cancelled after {rebuilt}/{total} dataset graph(s).")
            return
        progress(0.05 + 0.9 * (i / total), f"Building dataset graph {i + 1}/{total} ...")
        try:
            cleanup_ok = await _do_build(ctx.tenant_id, ctx.kb_id, compile_kwd, template_id, structure_kind, embd_mdl)
            if not cleanup_ok:
                all_cleanup_ok = False
            rebuilt += 1
        except Exception:
            all_cleanup_ok = False
            logging.exception("structure_merge: build failed for kb=%s compile_kwd=%s", ctx.kb_id, compile_kwd)

    # Delete pending deletion markers only after every pair has been
    # processed successfully.  If any pair had a store error the markers
    # survive so the next run retries the cleanup.
    if all_cleanup_ok:
        await _consume_deletion_markers(ctx.tenant_id, ctx.kb_id)

    progress(1.0, f"Built {rebuilt}/{total} dataset graph(s).")


async def _collect_structure_pairs(tenant_id: str, kb_id: str) -> set[tuple[str, str]]:
    """Collect distinct (compile_kwd, template_id) pairs from doc_graph entity rows."""
    from common.doc_store.doc_store_base import OrderByExpr

    index = _index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index, kb_id):
        return set()

    # NOTE: No scope_kwd filter here because old doc_graph rows that predate
    # the scope_kwd field won't have it — applying ["doc"] would miss those
    # rows and prevent their pairs from ever being rebuilt.  The scan in
    # _do_build adds its own scope_kwd constraint.
    pairs: set[tuple[str, str]] = set()
    offset = 0
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["id", "compile_kwd", "compilation_template_ids"],
                [],
                {"knowledge_graph_kwd": ["entity"]},
                [],
                OrderByExpr(),
                offset,
                _PAGE_SIZE,
                index,
                [kb_id],
            )
            field_map = settings.docStoreConn.get_fields(res, ["id", "compile_kwd", "compilation_template_ids"]) or {}
        except Exception:
            break
        if not field_map:
            break
        for row in field_map.values():
            compile_kwd = row.get("compile_kwd")
            if not isinstance(compile_kwd, str) or not compile_kwd:
                continue
            raw_tids = row.get("compilation_template_ids")
            tids = []
            if isinstance(raw_tids, list):
                tids = [t for t in raw_tids if isinstance(t, str) and t]
            elif isinstance(raw_tids, str) and raw_tids:
                tids = [raw_tids]
            for tid in tids:
                pairs.add((compile_kwd, tid))
        if len(field_map) < _PAGE_SIZE:
            break
        offset += _PAGE_SIZE
    return pairs
