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

"""Incremental clustering for dataset-level navigation.

Replaces the old 128-doc markdown with a hierarchy of nav_cluster and nav_doc
ES rows.  Each new document is embedded and placed into the nearest cluster
via layered KNN search + threshold-based merge/create.

Storage: one ES/Infinity row per nav_cluster or nav_doc node.
Tree structure encoded via ``parent_kwd`` on each row — no full-tree JSON blob.
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
from typing import Any

import xxhash

from common.misc_utils import thread_pool_exec
from rag.utils.redis_conn import RedisDistributedLock

from ._common import encode as _encode

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

_COMPILE_KWD = "dataset_nav"

# Embedding dimension — inferred at runtime from the first encode() call.
# None until _embed_dim is set.
_EMBED_DIM: int | None = None

# Similarity thresholds
_MERGE_THRESHOLD = 0.80  # merge doc into cluster
_RECURSE_THRESHOLD = 0.65  # continue descending into children
_MIN_SIM = 0.50  # minimum similarity to be considered related

# Max child count before triggering rebalance
_MAX_FANOUT = 64

# Max docs per leaf cluster before triggering split
_MAX_DOCS_PER_CLUSTER = 50

# Concurrency lock TTL
_LOCK_TIMEOUT_S = 30
_LOCK_BLOCKING_TIMEOUT_S = 5

# Hard limit on how many sibling clusters we evaluate per KNN call
_KNN_TOP_K = 5


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _nav_doc_id(doc_id: str) -> str:
    """Stable row id for a nav_doc (deterministic by doc_id)."""
    return xxhash.xxh64(
        f"dataset_nav:doc:{doc_id}".encode("utf-8", "surrogatepass"),
    ).hexdigest()


def _nav_cluster_id(kb_id: str, name: str) -> str:
    """Stable row id for a nav_cluster (deterministic by kb_id + name)."""
    return xxhash.xxh64(
        f"dataset_nav:{kb_id}:cluster:{name}".encode("utf-8", "surrogatepass"),
    ).hexdigest()


def _nav_lock_key(kb_id: str) -> str:
    """Redis lock key for concurrency control on a KB's nav tree."""
    return f"dataset_nav:{kb_id}"


def _extract_root_summary_from_tree(tree: dict | None) -> str:
    """Extract the doc-level summary from a RAPTOR tree (or bare string)."""
    if not isinstance(tree, dict):
        return ""
    title = tree.get("title") or ""
    if isinstance(title, str) and title.strip():
        return title.strip()
    for alt in ("summary", "content_with_weight", "content"):
        v = tree.get(alt)
        if isinstance(v, str) and v.strip():
            return v.strip()
    return ""


def _index_name(tenant_id: str) -> str:
    from rag.nlp import search as _rag_search

    return _rag_search.index_name(tenant_id)


def _vec_field(dim: int) -> str:
    return f"q_{dim}_vec"


# ---------------------------------------------------------------------------
# Doc store I/O — works with any engine (ES, Infinity, …) via docStoreConn
# ---------------------------------------------------------------------------


async def _store_get(tenant_id: str, kb_id: str, row_id: str) -> dict | None:
    from common import settings

    index = _index_name(tenant_id)
    try:
        return (
            await thread_pool_exec(
                settings.docStoreConn.get,
                row_id,
                index,
                [kb_id],
            )
            or None
        )
    except Exception:
        return None


async def _store_search(
    tenant_id: str,
    kb_id: str,
    condition: dict,
    fields: list[str],
    limit: int = 10000,
) -> list[dict]:
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr

    index = _index_name(tenant_id)
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        fields,
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        limit,
        index,
        [kb_id],
    )
    rows = settings.docStoreConn.get_fields(res, fields) if res else {}
    return list(rows.values())


async def _store_knn(
    tenant_id: str,
    kb_id: str,
    vec: list[float],
    vec_dim: int,
    filter_condition: dict,
    top_k: int = _KNN_TOP_K,
) -> list[dict]:
    """KNN search with dense vector and filter, returning top_k hits."""
    from common import settings

    index = _index_name(tenant_id)
    vf = _vec_field(vec_dim)
    fields = [
        "content_with_weight",
        "name",
        "doc_id",
        "type_kwd",
        "parent_kwd",
        "depth_int",
        "doc_count_int",
        "doc_ids_kwd",
        vf,
    ]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            filter_condition,
            [],
            None,
            0,
            top_k,
            index,
            [kb_id],
            knn_vector=vec,
            knn_vector_field=vf,
        )
    except TypeError:
        # Fallback: some doc store connectors don't accept knn_* kwargs.
        # Perform a plain search and lambda-rank in Python (slow-path).
        rows = await _store_search(
            tenant_id,
            kb_id,
            filter_condition,
            fields,
            limit=top_k * 10,
        )
        scoring = []
        for r in rows:
            stored = r.get(vf)
            if stored and len(stored) == len(vec):
                sim = sum(a * b for a, b in zip(stored, vec))
                scoring.append((sim, r))
        scoring.sort(key=lambda x: -x[0])
        return [r for _, r in scoring[:top_k]]
    results = settings.docStoreConn.get_fields(res, fields) if res else {}
    return list(results.values())


async def _store_upsert(tenant_id: str, kb_id: str, doc: dict) -> None:
    from common import settings

    index = _index_name(tenant_id)
    row_id = doc.get("id", "")
    existing = await thread_pool_exec(
        settings.docStoreConn.get,
        row_id,
        index,
        [kb_id],
    )
    if existing:
        upd = {k: v for k, v in doc.items() if k != "id"}
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"id": row_id},
            upd,
            index,
            kb_id,
        )
    else:
        await thread_pool_exec(
            settings.docStoreConn.insert,
            [doc],
            index,
            kb_id,
        )


async def _store_delete(tenant_id: str, kb_id: str, row_id: str) -> None:
    from common import settings

    index = _index_name(tenant_id)
    try:
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"id": [row_id]},
            index,
            kb_id,
        )
    except Exception:
        pass


# ---------------------------------------------------------------------------
# Embedding helpers
# ---------------------------------------------------------------------------


async def _embed(embd_mdl, text: str) -> list[float]:
    """Encode a single text string and return its embedding vector."""
    global _EMBED_DIM
    vecs = await _encode(embd_mdl, [text])
    if vecs and len(vecs[0]) > 0:
        dim = len(vecs[0])
        if _EMBED_DIM is None:
            _EMBED_DIM = dim
        return vecs[0]
    return []


def _cosine_sim(a: list[float], b: list[float]) -> float:
    """Compute cosine similarity between two vectors."""
    if not a or not b or len(a) != len(b):
        return 0.0
    dot = sum(x * y for x, y in zip(a, b))
    na = sum(x * x for x in a) ** 0.5
    nb = sum(x * x for x in b) ** 0.5
    if na == 0 or nb == 0:
        return 0.0
    return dot / (na * nb)


# ---------------------------------------------------------------------------
# Fabrication of nav rows
# ---------------------------------------------------------------------------


def _make_nav_doc_row(
    kb_id: str,
    doc_id: str,
    summary: str,
    parent_kwd: str,
    depth_int: int,
    embd_mdl=None,
    embedding: list[float] | None = None,
) -> dict:
    """Build a nav_doc ES/Infinity row dict for a single document leaf node."""
    row: dict = {
        "id": _nav_doc_id(doc_id),
        "kb_id": kb_id,
        "doc_id": doc_id,
        "compile_kwd": _COMPILE_KWD,
        "knowledge_graph_kwd": "entity",
        "type_kwd": "nav_doc",
        "name": doc_id,
        "parent_kwd": parent_kwd,
        "depth_int": depth_int,
        "available_int": 0,
    }
    payload = {"type": "nav_doc", "description": summary}
    row["content_with_weight"] = json.dumps(payload, ensure_ascii=False)
    ltks = _tokenize(summary)
    row["content_ltks"] = ltks
    row["content_sm_ltks"] = _fine_tokenize(ltks)
    if embedding:
        dim = len(embedding)
        row[_vec_field(dim)] = embedding
    return row


def _make_nav_cluster_row(
    kb_id: str,
    name: str,
    description: str,
    parent_kwd: str,
    depth_int: int,
    doc_ids: list[str],
    embedding: list[float] | None = None,
) -> dict:
    """Build a nav_cluster ES/Infinity row dict for an internal tree node."""
    cluster_id = _nav_cluster_id(kb_id, name)
    row: dict = {
        "id": cluster_id,
        "kb_id": kb_id,
        "doc_id": kb_id,
        "compile_kwd": _COMPILE_KWD,
        "knowledge_graph_kwd": "entity",
        "type_kwd": "nav_cluster",
        "name": name,
        "parent_kwd": parent_kwd,
        "depth_int": depth_int,
        "doc_ids_kwd": doc_ids,
        "doc_count_int": len(doc_ids),
        "available_int": 0,
    }
    payload = {"type": "nav_cluster", "description": description}
    row["content_with_weight"] = json.dumps(payload, ensure_ascii=False)
    ltks = _tokenize(description)
    row["content_ltks"] = ltks
    row["content_sm_ltks"] = _fine_tokenize(ltks)
    if embedding:
        dim = len(embedding)
        row[_vec_field(dim)] = embedding
    return row


def _tokenize(text: str) -> str:
    """Coarse-grained tokenization for ES/Infinity full-text search."""
    from rag.nlp import rag_tokenizer

    return rag_tokenizer.tokenize(text)


def _fine_tokenize(text: str) -> str:
    """Fine-grained tokenization for ES/Infinity sub-word search."""
    from rag.nlp import rag_tokenizer

    return rag_tokenizer.fine_grained_tokenize(text)


# ---------------------------------------------------------------------------
# Incremental clustering core
# ---------------------------------------------------------------------------


async def _find_best_cluster(
    tenant_id: str,
    kb_id: str,
    doc_embedding: list[float],
    vec_dim: int,
) -> tuple[str | None, str | None, float]:
    """Locate the nearest cluster for a document via layered KNN descent.

    Starts from the root cluster (depth_int=0) and recursively descends into
    the best-matching child as long as similarity stays above
    ``_RECURSE_THRESHOLD``.  Returns the deepest cluster whose children are
    all less similar, along with the similarity score.

    Returns:
        (best_cluster_name, best_cluster_parent_name, similarity)
    """
    # Step 1: find the root cluster (depth_int=0)
    root_cond = {
        "kb_id": [kb_id],
        "compile_kwd": [_COMPILE_KWD],
        "type_kwd": ["nav_cluster"],
        "depth_int": [0],
    }
    roots = await _store_knn(tenant_id, kb_id, doc_embedding, vec_dim, root_cond, top_k=1)
    if not roots:
        return None, None, 0.0

    best = roots[0]
    best_name = best.get("name", "")
    best_parent = best.get("parent_kwd", "")
    # compute actual similarity to root
    stored = best.get(_vec_field(vec_dim))
    sim = _cosine_sim(doc_embedding, stored) if stored else 0.0

    # Step 2: recursively descend into children
    while sim >= _RECURSE_THRESHOLD:
        child_cond = {
            "kb_id": [kb_id],
            "compile_kwd": [_COMPILE_KWD],
            "type_kwd": ["nav_cluster"],
            "parent_kwd": [best_name],
        }
        children = await _store_knn(tenant_id, kb_id, doc_embedding, vec_dim, child_cond, top_k=1)
        if not children:
            break
        child = children[0]
        stored = child.get(_vec_field(vec_dim))
        child_sim = _cosine_sim(doc_embedding, stored) if stored else 0.0
        if child_sim < _RECURSE_THRESHOLD:
            break
        best_name = child.get("name", best_name)
        best_parent = best.get("parent_kwd", best_parent)
        sim = child_sim
        best = child

    return best_name, best_parent, sim


async def _llm_merge(chat_mdl, cluster_desc: str, doc_summary: str) -> str:
    """LLM merge: fuse existing cluster description with new doc summary."""
    if not chat_mdl:
        return cluster_desc  # no LLM available, keep old summary
    from rag.prompts.generator import gen_json

    prompt = (
        "Merge the following two descriptions of the same topic into "
        "a single concise summary (1-3 sentences):\n\n"
        f"Existing: {cluster_desc}\n\n"
        f"New: {doc_summary}\n\n"
        "Return ONLY the merged text, no commentary."
    )
    try:
        resp = await gen_json("", prompt, chat_mdl, gen_conf={"temperature": 0.1})
        if isinstance(resp, dict):
            return str(resp.get("merged", resp.get("result", cluster_desc)))
        if isinstance(resp, str) and resp.strip():
            return resp.strip()
    except Exception:
        logging.exception("dataset_nav: LLM merge failed, keeping original")
    return cluster_desc


async def _llm_create_summary(chat_mdl, doc_summaries: list[str]) -> str:
    """LLM create a cluster summary from one or more doc summaries."""
    if not chat_mdl:
        return doc_summaries[0] if doc_summaries else ""
    from rag.prompts.generator import gen_json

    texts = "\n---\n".join(doc_summaries)
    prompt = f"Summarize the common topic of the following document excerpts in 1-3 concise sentences:\n\n{texts}\n\nReturn ONLY the summary text, no commentary."
    try:
        resp = await gen_json("", prompt, chat_mdl, gen_conf={"temperature": 0.1})
        if isinstance(resp, dict):
            return str(resp.get("summary", resp.get("result", doc_summaries[0])))
        if isinstance(resp, str) and resp.strip():
            return resp.strip()
    except Exception:
        logging.exception("dataset_nav: LLM summary failed")
    return doc_summaries[0] if doc_summaries else ""


# ---------------------------------------------------------------------------
# Public surface
# ---------------------------------------------------------------------------


async def upsert_dataset_nav_doc(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    summary_or_tree: Any,
    embd_mdl=None,
    chat_mdl=None,
) -> None:
    """Upsert a document into the nav clustering tree.

    Args:
        tenant_id: Tenant owning the KB.
        kb_id: Knowledge base id.
        doc_id: Document id.
        summary_or_tree: A plain summary string, or a RAPTOR tree dict from
            which the root summary is extracted.
        embd_mdl: LLMBundle for embedding (required for clustering).
        chat_mdl: LLMBundle for chat (required for LLM merge/summary).
    """
    if not doc_id or not kb_id:
        return

    # 1. Extract summary
    if isinstance(summary_or_tree, dict):
        summary = _extract_root_summary_from_tree(summary_or_tree)
    elif isinstance(summary_or_tree, str):
        summary = summary_or_tree
    else:
        summary = ""
    if not summary:
        logging.info("dataset_nav: skipping doc=%s (kb=%s) — no summary", doc_id, kb_id)
        return

    # 2. Check if this doc already has a nav_doc row
    existing_doc = await _store_get(tenant_id, kb_id, _nav_doc_id(doc_id))
    if existing_doc:
        old_payload = json.loads(existing_doc.get("content_with_weight") or "{}")
        if old_payload.get("description") == summary:
            logging.info("dataset_nav: doc=%s unchanged, skipping", doc_id)
            return

    # 3. Embed doc summary
    doc_embedding = await _embed(embd_mdl, summary) if embd_mdl else []
    vec_dim = _EMBED_DIM or 0

    lock = RedisDistributedLock(
        _nav_lock_key(kb_id),
        timeout=_LOCK_TIMEOUT_S,
        blocking_timeout=_LOCK_BLOCKING_TIMEOUT_S,
    )
    try:
        await lock.spin_acquire()
    except Exception:
        logging.exception("dataset_nav: lock acquire failed for kb=%s", kb_id)
        return

    try:
        # 4. Layered KNN search for nearest cluster
        best_name, best_parent, sim = await _find_best_cluster(
            tenant_id,
            kb_id,
            doc_embedding,
            vec_dim,
        )

        if best_name and sim >= _MERGE_THRESHOLD:
            # ── Merge into best cluster ──
            cluster_id = _nav_cluster_id(kb_id, best_name)
            cluster_row = await _store_get(tenant_id, kb_id, cluster_id)
            if cluster_row:
                payload = json.loads(cluster_row.get("content_with_weight") or "{}")
                old_desc = payload.get("description", "")
                new_desc = await _llm_merge(chat_mdl, old_desc, summary)
                payload["description"] = new_desc
                cluster_row["content_with_weight"] = json.dumps(payload, ensure_ascii=False)
                doc_ids = cluster_row.get("doc_ids_kwd") or []
                if doc_id not in doc_ids:
                    doc_ids.append(doc_id)
                cluster_row["doc_ids_kwd"] = doc_ids
                cluster_row["doc_count_int"] = len(doc_ids)
                # Re-compute embedding for the new summary
                if embd_mdl and new_desc != old_desc:
                    new_emb = await _embed(embd_mdl, new_desc)
                    if new_emb:
                        cluster_row[_vec_field(len(new_emb))] = new_emb
                await _store_upsert(tenant_id, kb_id, cluster_row)

            # Upsert nav_doc under the cluster
            depth = cluster_row.get("depth_int", 1) + 1 if cluster_row else 2
            nav_doc_row = _make_nav_doc_row(
                kb_id,
                doc_id,
                summary,
                best_name,
                depth,
                embd_mdl,
                doc_embedding,
            )
            await _store_upsert(tenant_id, kb_id, nav_doc_row)

            # Check fanout — if the cluster now has too many children, trigger split
            await _maybe_split_cluster(
                tenant_id,
                kb_id,
                best_name,
                embd_mdl,
                chat_mdl,
            )

        elif best_name and sim >= _MIN_SIM:
            # ── Create new cluster as sibling/child ──
            parent_for_new = best_parent if best_parent else best_name
            depth_of_parent = 1  # default
            parent_row = await _store_get(
                tenant_id,
                kb_id,
                _nav_cluster_id(kb_id, parent_for_new),
            )
            if parent_row:
                depth_of_parent = parent_row.get("depth_int", 1)
            new_depth = depth_of_parent + 1
            new_name = f"navc_{xxhash.xxh64(summary.encode()).hexdigest()[:12]}"
            new_desc = await _llm_create_summary(chat_mdl, [summary])
            new_cluster = _make_nav_cluster_row(
                kb_id,
                new_name,
                new_desc,
                parent_for_new,
                depth_of_parent,
                [doc_id],
                doc_embedding,
            )
            if embd_mdl and doc_embedding:
                new_cluster[_vec_field(len(doc_embedding))] = doc_embedding
            await _store_upsert(tenant_id, kb_id, new_cluster)

            nav_doc_row = _make_nav_doc_row(
                kb_id,
                doc_id,
                summary,
                new_name,
                new_depth,
                embd_mdl,
                doc_embedding,
            )
            await _store_upsert(tenant_id, kb_id, nav_doc_row)
        else:
            # ── Create root-level new cluster ──
            new_name = f"navc_{xxhash.xxh64(summary.encode()).hexdigest()[:12]}"
            new_desc = await _llm_create_summary(chat_mdl, [summary])
            new_cluster = _make_nav_cluster_row(
                kb_id,
                new_name,
                new_desc,
                "root",
                0,
                [doc_id],
                doc_embedding,
            )
            if embd_mdl and doc_embedding:
                new_cluster[_vec_field(len(doc_embedding))] = doc_embedding
            await _store_upsert(tenant_id, kb_id, new_cluster)

            nav_doc_row = _make_nav_doc_row(
                kb_id,
                doc_id,
                summary,
                new_name,
                1,
                embd_mdl,
                doc_embedding,
            )
            await _store_upsert(tenant_id, kb_id, nav_doc_row)

    except Exception:
        logging.exception(
            "dataset_nav: upsert failed for kb=%s doc=%s",
            kb_id,
            doc_id,
        )
    finally:
        try:
            lock.release()
        except Exception:
            logging.exception("dataset_nav: lock release failed for kb=%s", kb_id)


async def remove_dataset_nav_doc(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
) -> None:
    """Remove a document from the nav clustering tree.

    Cascades: if the parent cluster becomes empty after removal, the cluster
    itself is also removed.
    """
    if not doc_id or not kb_id:
        return

    lock = RedisDistributedLock(
        _nav_lock_key(kb_id),
        timeout=_LOCK_TIMEOUT_S,
        blocking_timeout=_LOCK_BLOCKING_TIMEOUT_S,
    )
    try:
        await lock.spin_acquire()
    except Exception:
        logging.exception("dataset_nav: lock acquire failed for kb=%s", kb_id)
        return

    try:
        # 1. Find and delete the nav_doc row
        doc_row_id = _nav_doc_id(doc_id)
        doc_row = await _store_get(tenant_id, kb_id, doc_row_id)
        if not doc_row:
            return
        parent_name = doc_row.get("parent_kwd", "")
        await _store_delete(tenant_id, kb_id, doc_row_id)

        # 2. Remove doc_id from the parent cluster's doc_ids_kwd
        if parent_name and parent_name != "root":
            cluster_id = _nav_cluster_id(kb_id, parent_name)
            cluster_row = await _store_get(tenant_id, kb_id, cluster_id)
            if cluster_row:
                doc_ids = cluster_row.get("doc_ids_kwd") or []
                if doc_id in doc_ids:
                    doc_ids.remove(doc_id)
                if not doc_ids:
                    # Cluster is empty — delete it
                    await _store_delete(tenant_id, kb_id, cluster_id)
                    # Recurse: check grandparent
                    grandparent = cluster_row.get("parent_kwd", "")
                    if grandparent and grandparent != "root":
                        await _cleanup_empty_cluster(
                            tenant_id,
                            kb_id,
                            grandparent,
                        )
                else:
                    cluster_row["doc_ids_kwd"] = doc_ids
                    cluster_row["doc_count_int"] = len(doc_ids)
                    await _store_upsert(tenant_id, kb_id, cluster_row)
    except Exception:
        logging.exception(
            "dataset_nav: remove failed for kb=%s doc=%s",
            kb_id,
            doc_id,
        )
    finally:
        try:
            lock.release()
        except Exception:
            logging.exception("dataset_nav: lock release failed for kb=%s", kb_id)


async def _cleanup_empty_cluster(
    tenant_id: str,
    kb_id: str,
    cluster_name: str,
) -> None:
    """Recursively remove a cluster if it has no doc children and no direct doc descendants."""
    cluster_id = _nav_cluster_id(kb_id, cluster_name)
    cluster = await _store_get(tenant_id, kb_id, cluster_id)
    if not cluster:
        return
    # Check direct children (nav_cluster)
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr

    index = _index_name(tenant_id)
    child_cond = {
        "kb_id": [kb_id],
        "compile_kwd": [_COMPILE_KWD],
        "parent_kwd": [cluster_name],
    }
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        ["id"],
        [],
        child_cond,
        [],
        OrderByExpr(),
        0,
        100,
        index,
        [kb_id],
    )
    children = settings.docStoreConn.get_fields(res, ["id"]) if res else {}
    if not children and not cluster.get("doc_ids_kwd"):
        grandparent = cluster.get("parent_kwd", "")
        await _store_delete(tenant_id, kb_id, cluster_id)
        if grandparent and grandparent != "root":
            await _cleanup_empty_cluster(tenant_id, kb_id, grandparent)


async def _maybe_split_cluster(
    tenant_id: str,
    kb_id: str,
    cluster_name: str,
    embd_mdl,
    chat_mdl,
) -> None:
    """If a cluster exceeds fanout or doc count, split children via AHC."""
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr

    index = _index_name(tenant_id)

    # Count children (nav_cluster)
    child_cond = {
        "kb_id": [kb_id],
        "compile_kwd": [_COMPILE_KWD],
        "parent_kwd": [cluster_name],
    }
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        ["id", "name", "type_kwd"],
        [],
        child_cond,
        [],
        OrderByExpr(),
        0,
        200,
        index,
        [kb_id],
    )
    children = settings.docStoreConn.get_fields(res, ["id", "name", "type_kwd"]) if res else {}
    if not children:
        return

    nav_cluster_kids = [c for c in children.values() if c.get("type_kwd") == "nav_cluster"]
    nav_doc_kids = [c for c in children.values() if c.get("type_kwd") == "nav_doc"]

    should_split = len(nav_cluster_kids) + len(nav_doc_kids) > _MAX_FANOUT or len(nav_doc_kids) > _MAX_DOCS_PER_CLUSTER
    if not should_split:
        return

    # Load embeddings for all children
    vf = _vec_field(_EMBED_DIM) if _EMBED_DIM else "q_768_vec"
    child_details = await _store_search(
        tenant_id,
        kb_id,
        child_cond,
        ["id", "name", "type_kwd", "content_with_weight", vf],
        limit=200,
    )
    embeddings = []
    names = []
    name_to_type: dict[str, str] = {}
    for c in child_details:
        stored = c.get(vf)
        if stored:
            embeddings.append(stored)
            names.append(c.get("name", ""))
        cn = c.get("name", "")
        if cn:
            name_to_type[cn] = c.get("type_kwd", "nav_cluster")

    if len(embeddings) < 4:
        return

    # Simple k-means-like split into 2 groups (no scikit dependency at runtime)
    # Use the first two embeddings as initial centroids
    centroids = [embeddings[0][:], embeddings[len(embeddings) // 2][:]]
    for _ in range(10):
        groups = [[], []]
        for emb in embeddings:
            d0 = sum((a - b) ** 2 for a, b in zip(emb, centroids[0]))
            d1 = sum((a - b) ** 2 for a, b in zip(emb, centroids[1]))
            groups[0 if d0 < d1 else 1].append(emb)
        for gi in (0, 1):
            if groups[gi]:
                avg = [sum(c) / len(groups[gi]) for c in zip(*groups[gi])]
                centroids[gi] = avg

    # Relabel
    labels = []
    for emb in embeddings:
        d0 = sum((a - b) ** 2 for a, b in zip(emb, centroids[0]))
        d1 = sum((a - b) ** 2 for a, b in zip(emb, centroids[1]))
        labels.append(0 if d0 < d1 else 1)

    # Create sub-clusters
    cluster_row = await _store_get(tenant_id, kb_id, _nav_cluster_id(kb_id, cluster_name))
    depth = (cluster_row.get("depth_int", 0) if cluster_row else 0) + 1

    for gi in (0, 1):
        kid_names = [names[i] for i in range(len(names)) if labels[i] == gi]
        if not kid_names:
            continue
        # Collect doc_ids from all children
        doc_ids: list[str] = []
        descs: list[str] = []
        for kn in kid_names:
            is_doc = name_to_type.get(kn) == "nav_doc"
            cid = _nav_doc_id(kn) if is_doc else _nav_cluster_id(kb_id, kn)
            row = await _store_get(tenant_id, kb_id, cid)
            if row:
                payload = json.loads(row.get("content_with_weight") or "{}")
                descs.append(payload.get("description", ""))
                dids = row.get("doc_ids_kwd") or []
                for d in dids:
                    if d not in doc_ids:
                        doc_ids.append(d)
        group_desc = await _llm_create_summary(chat_mdl, descs) if descs else f"Group {gi + 1}"
        group_name = f"navc_split_{xxhash.xxh64(group_desc.encode()).hexdigest()[:12]}"
        group_emb = await _embed(embd_mdl, group_desc) if embd_mdl else []
        new_cluster = _make_nav_cluster_row(
            kb_id,
            group_name,
            group_desc,
            cluster_name,
            depth,
            doc_ids,
            group_emb,
        )
        await _store_upsert(tenant_id, kb_id, new_cluster)

        # Reparent children to new split cluster
        for kn in kid_names:
            is_doc = name_to_type.get(kn) == "nav_doc"
            cid = _nav_doc_id(kn) if is_doc else _nav_cluster_id(kb_id, kn)
            row = await _store_get(tenant_id, kb_id, cid)
            if row:
                row["parent_kwd"] = group_name
                row["depth_int"] = depth + 1
                await _store_upsert(tenant_id, kb_id, row)


async def search_dataset_nav(
    tenant_id: str,
    kb_id: str,
    query: str,
    embd_mdl=None,
    top_k: int = 8,
) -> list[dict]:
    """Find the nav-tree nodes most relevant to ``query`` for one KB.

    The nav rows are ``available_int=0`` (invisible to the normal retriever), so
    this is the sanctioned read seam: a caller uses the returned document ids to
    route a scoped chunk retrieval. Returns items shaped as::

        {"type": "nav_doc" | "nav_cluster",
         "doc_id": str | None,      # the document, for a leaf
         "doc_ids": [str],          # the documents a node covers
         "name": str, "description": str, "score": float}

    Ranked by vector KNN over the node summaries when ``embd_mdl`` is given;
    otherwise a best-effort text-ranked scan.
    """
    query = (query or "").strip()
    if not query:
        return []

    condition = {"compile_kwd": [_COMPILE_KWD]}
    rows_with_scores: list[tuple[dict, float]] = []

    if embd_mdl is not None:
        try:
            vec = await _embed(embd_mdl, query)
        except Exception:
            logging.exception("search_dataset_nav: embed failed for kb=%s", kb_id)
            vec = []
        if vec:
            try:
                rows = await _store_knn(tenant_id, kb_id, vec, len(vec), condition, top_k=top_k)
                vf = _vec_field(len(vec))
                rows_with_scores = [(r, _cosine_sim(vec, r.get(vf) or [])) for r in rows]
            except Exception:
                logging.exception("search_dataset_nav: knn failed for kb=%s", kb_id)
                rows_with_scores = []

    if not rows_with_scores:
        fields = ["content_with_weight", "name", "doc_id", "type_kwd", "doc_ids_kwd", "doc_count_int"]
        try:
            rows = await _store_search(tenant_id, kb_id, condition, fields, limit=max(top_k * 20, 100))
        except Exception:
            logging.exception("search_dataset_nav: scan failed for kb=%s", kb_id)
            rows = []
        rows_with_scores = [(r, _nav_text_score(query, r)) for r in rows]
        rows_with_scores.sort(key=lambda item: item[1], reverse=True)

    # Discard zero-score rows (text match produced no relevant hits)
    rows_with_scores = [(r, s) for r, s in rows_with_scores if s > 0]

    out: list[dict] = []
    for r, score in rows_with_scores[:top_k]:
        try:
            payload = json.loads(r.get("content_with_weight") or "{}")
        except Exception:
            payload = {}
        typ = payload.get("type") or r.get("type_kwd") or ("nav_cluster" if r.get("doc_ids_kwd") else "nav_doc")
        name = r.get("name") or ""
        if typ == "nav_cluster":
            doc_id = None
            doc_ids = _as_str_list(r.get("doc_ids_kwd"))
        else:
            # Leaf: ``name`` == the document id (see ``_make_nav_doc_row``).
            doc_id = r.get("doc_id") or name
            doc_ids = [doc_id] if doc_id else []
        out.append(
            {
                "type": typ,
                "doc_id": doc_id,
                "doc_ids": doc_ids,
                "name": name,
                "description": payload.get("description") or "",
                "doc_count": int(r.get("doc_count_int") or len(doc_ids) or 0),
                "score": float(score or 0.0),
            }
        )
    return out


def _as_str_list(value) -> list[str]:
    if isinstance(value, list):
        return [str(v) for v in value if v]
    if isinstance(value, str) and value:
        return [value]
    return []


def _nav_text_score(query: str, row: dict) -> float:
    try:
        payload = json.loads(row.get("content_with_weight") or "{}")
    except Exception:
        payload = {}
    haystack = " ".join(
        str(x or "")
        for x in (
            row.get("name"),
            payload.get("description"),
        )
    ).lower()
    q_terms = set(re.findall(r"[\w]+", query.lower()))
    if not q_terms:
        return 0.0
    hits = sum(1 for term in q_terms if term in haystack)
    return hits / max(len(q_terms), 1)


def remove_dataset_nav_doc_sync(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
) -> None:
    """Sync wrapper around ``remove_dataset_nav_doc``."""
    try:
        loop = asyncio.new_event_loop()
        try:
            loop.run_until_complete(
                remove_dataset_nav_doc(tenant_id, kb_id, doc_id),
            )
        finally:
            loop.close()
    except Exception:
        logging.exception(
            "dataset_nav: sync remove failed for kb=%s doc=%s",
            kb_id,
            doc_id,
        )
