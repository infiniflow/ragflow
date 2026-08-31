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
from ._common import knowledge_compile_gen_conf as _knowledge_compile_gen_conf

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

# Fields needed to shape a nav hit for hybrid scoring / rendering.
_NAV_SEARCH_FIELDS = [
    "id",
    "content_with_weight",
    "name",
    "doc_id",
    "type_kwd",
    "doc_ids_kwd",
    "doc_count_int",
]

# Weight of the dense leg in hybrid search (1 - this = BM25 leg weight).
_NAV_HYBRID_DENSE_W = 0.5

# Minimum fused relevance score a nav node must reach to be returned by a
# tree search.  Below this, the query is treated as "not related" to the
# dataset and search_nav_tree_descent returns an empty result instead of
# falling back to an entire subtree.  The dense leg contributes at most
# ``_NAV_HYBRID_DENSE_W`` (cosine ≤ 1) and the text leg at most
# ``1 - _NAV_HYBRID_DENSE_W``, so a score below this threshold means the
# query is neither semantically nor lexically close to any node.
_NAV_TREE_MIN_SCORE = 0.1

# Stop-words skipped when deriving routing tag-words from a summary.
_NAV_STOP_WORDS = {
    "the",
    "a",
    "an",
    "and",
    "or",
    "of",
    "to",
    "in",
    "on",
    "for",
    "with",
    "at",
    "is",
    "are",
    "was",
    "were",
    "be",
    "been",
    "being",
    "this",
    "that",
    "these",
    "those",
    "it",
    "its",
    "as",
    "by",
    "from",
    "about",
    "into",
}


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


async def _store_text_search(
    tenant_id: str,
    kb_id: str,
    query: str,
    fields: list[str],
    limit: int = 100,
    *,
    compile_kwd: str = _COMPILE_KWD,
    type_kwd: str = "",
    extra_filter: dict | None = None,
) -> list[dict]:
    """Full-text (BM25) leg over the nav rows' tokenized fields.

    Recalls nav nodes whose ``content_ltks`` / ``content_sm_ltks`` match the
    query — the lexical half of hybrid search, complementing the KNN leg for
    exact/proper-noun recall (e.g. "关羽", "ImageNet 2012").

    Args:
        compile_kwd: Which compile partition to search within
            (default ``_COMPILE_KWD`` = ``"dataset_nav"``).
        type_kwd: Optional type filter (``"nav_doc"``, ``"nav_cluster"``, or ``""``).
        extra_filter: Additional filter conditions merged into the query.
    """
    from common import settings
    from common.doc_store.doc_store_base import MatchTextExpr, OrderByExpr

    # Pre-tokenize the query with the same tokenizer used to index content_ltks.
    # ES content_ltks uses the "whitespace" analyzer (no stemming), while our
    # Python tokenizer applies stemming (e.g. "Christie" → "christi").
    # Without this, raw query terms won't match pre-stemmed tokens in the index.
    tokenized_query = _tokenize(query)

    index = _index_name(tenant_id)
    filter_condition: dict = {"compile_kwd": [compile_kwd]}
    if type_kwd:
        filter_condition["type_kwd"] = type_kwd
    if extra_filter:
        filter_condition.update(extra_filter)
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        fields,
        [],
        filter_condition,
        [MatchTextExpr(["content_ltks", "content_sm_ltks"], tokenized_query, limit)],
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
    from common.doc_store.doc_store_base import MatchDenseExpr, OrderByExpr

    index = _index_name(tenant_id)
    vf = _vec_field(vec_dim)
    fields = [
        "content_with_weight",
        "name",
        "doc_id",
        "compile_kwd",
        "type_kwd",
        "parent_kwd",
        "depth_int",
        "doc_count_int",
        "doc_ids_kwd",
        vf,
    ]
    match_expr = MatchDenseExpr(
        vector_column_name=vf,
        embedding_data=list(vec),
        embedding_data_type="float",
        distance_type="cosine",
        topn=top_k,
        extra_options={},
    )
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        fields,
        [],
        filter_condition,
        [match_expr],
        OrderByExpr(),
        0,
        top_k,
        index,
        [kb_id],
    )
    results = settings.docStoreConn.get_fields(res, fields) if res else {}
    rows = list(results.values())
    if filter_condition and any(not _matches_condition(row, filter_condition) for row in rows):
        scanned = await _store_search(tenant_id, kb_id, filter_condition, fields, limit=10000)
        rows = [row for row in scanned if _vector_len(row.get(vf)) == vec_dim and _matches_condition(row, filter_condition)]
        rows.sort(key=lambda row: _cosine_sim(vec, row.get(vf)), reverse=True)
        rows = rows[:top_k]
    return rows


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
    if vecs and _vector_len(vecs[0]) > 0:
        dim = len(vecs[0])
        if _EMBED_DIM is None:
            _EMBED_DIM = dim
        return vecs[0]
    return []


def _vector_len(vec) -> int:
    if vec is None:
        return 0
    try:
        return len(vec)
    except TypeError:
        return 0


def _cosine_sim(a, b) -> float:
    """Compute cosine similarity between two vectors."""
    a_len = _vector_len(a)
    b_len = _vector_len(b)
    if a_len == 0 or b_len == 0 or a_len != b_len:
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
    *,
    graph_content: str = "",
) -> dict:
    """Build a nav_doc ES/Infinity row dict for a single document leaf node.

    Args:
        summary: Short human-readable title / description for display.
        graph_content: Optional full-graph text used for richer keyword /
            entity extraction.  When set, keywords and entities are derived
            from graph_content instead of summary, and the text is stored in
            the ``graph_content`` payload field.
    """
    kw_text = graph_content or summary
    payload = {
        "type": "nav_doc",
        "description": summary,
        "keywords": _nav_keywords(kw_text),
        "entities": _nav_entities(kw_text),
    }
    if graph_content:
        payload["graph_content"] = graph_content
    row: dict = {
        "id": _nav_doc_id(doc_id),
        "kb_id": kb_id,
        "doc_id": doc_id,
        "compile_kwd": _COMPILE_KWD,
        "knowledge_graph_kwd": "entity",
        "type_kwd": "nav_doc",
        "name": f"{parent_kwd}_{xxhash.xxh64(summary.encode()).hexdigest()[:12]}",
        "parent_kwd": parent_kwd,
        "depth_int": depth_int,
        "available_int": 0,
    }
    row["content_with_weight"] = json.dumps(payload, ensure_ascii=False)
    ltks = _tokenize(kw_text)
    row["content_ltks"] = ltks
    row["content_sm_ltks"] = _fine_tokenize(ltks)
    if _vector_len(embedding) > 0:
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
    payload = {
        "type": "nav_cluster",
        "description": description,
        "keywords": _nav_keywords(description),
        "entities": _nav_entities(description),
    }
    row["content_with_weight"] = json.dumps(payload, ensure_ascii=False)
    ltks = _tokenize(description)
    row["content_ltks"] = ltks
    row["content_sm_ltks"] = _fine_tokenize(ltks)
    if _vector_len(embedding) > 0:
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


def _nav_keywords(summary: str, max_kwds: int = 6) -> list[str]:
    """Derive routing tags from a nav summary (no LLM call, zero cost).

    These are the tokenized non-stop terms the model uses to fast-judge node
    relevance before reading the full description.
    """
    from rag.nlp import rag_tokenizer

    tokens = (rag_tokenizer.tokenize(summary or "") or "").split()
    seen: set[str] = set()
    out: list[str] = []
    for t in tokens:
        t = t.strip()
        if len(t) < 2 or t.isdigit() or t.lower() in _NAV_STOP_WORDS or t.lower() in seen:
            continue
        seen.add(t.lower())
        out.append(t)
        if len(out) >= max_kwds:
            break
    return out


def _nav_entities(summary: str, max_entities: int = 6) -> list[str]:
    """Extract likely named entities from a nav summary (no LLM call, zero cost).

    Uses a two-pass heuristic:
    1. Regex for English capitalized multi-word sequences (proper nouns).
    2. Tokenizer-based extraction for CJK and other non-Latin text.

    Returns up to ``max_entities`` deduplicated strings.
    """
    text = summary or ""
    entities: list[str] = []
    seen: set[str] = set()

    # Pass 1: English-like capitalized sequences (e.g. "New York", "Machine Learning")
    for m in re.finditer(r"\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+)+)\b", text):
        ent = m.group(1).strip()
        key = ent.lower()
        if key not in _NAV_STOP_WORDS and key not in seen:
            seen.add(key)
            entities.append(ent)
            if len(entities) >= max_entities:
                return entities

    # Pass 2: tokenizer-based for CJK / other scripts
    from rag.nlp import rag_tokenizer

    tokens = (rag_tokenizer.tokenize(text) or "").split()
    for t in tokens:
        t = t.strip()
        if len(t) < 3 or t.isdigit():
            continue
        if t.isascii() and t[0].islower():
            continue  # skip lowercase English fragments
        key = t.lower()
        if key in _NAV_STOP_WORDS or key in seen:
            continue
        seen.add(key)
        entities.append(t)
        if len(entities) >= max_entities:
            break

    return entities


def _matches_condition(row: dict, condition: dict) -> bool:
    """Check the simple equality filters used by dataset navigation."""
    for field, expected in condition.items():
        if not expected or field == "kb_id":
            continue
        actual = row.get(field)
        actual_values = actual if isinstance(actual, (list, tuple, set)) else [actual]
        expected_values = expected if isinstance(expected, (list, tuple, set)) else [expected]
        if not any(str(value) == str(item) for value in actual_values for item in expected_values):
            return False
    return True


def _in_nav_scope(row: dict, allowed_docs: set[str] | None) -> bool:
    """Whether a nav row belongs to the given doc scope.

    A ``nav_doc`` leaf belongs when its ``doc_id`` is in the set; a
    ``nav_cluster`` row belongs when it covers at least one scoped document
    (its ``doc_ids_kwd`` intersects the set).  An empty/None scope allows
    everything, preserving the existing unscoped behavior.
    """
    if not allowed_docs:
        return True
    # A cluster row carries doc_id == kb_id (not a document) but lists the
    # documents it covers in doc_ids_kwd, so a non-empty doc_ids_kwd is the
    # reliable discriminator.  A nav_doc leaf never sets doc_ids_kwd.
    if row.get("doc_ids_kwd"):
        return bool(set(_as_str_list(row.get("doc_ids_kwd"))) & allowed_docs)
    return str(row.get("doc_id") or "").strip() in allowed_docs


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
    sim = _cosine_sim(doc_embedding, stored)
    visited_names = {best_name}

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
        child_name = child.get("name", "")
        if not child_name or child_name in visited_names:
            break
        stored = child.get(_vec_field(vec_dim))
        child_sim = _cosine_sim(doc_embedding, stored)
        if child_sim < _RECURSE_THRESHOLD:
            break
        best_name = child_name
        best_parent = best.get("parent_kwd", best_parent)
        sim = child_sim
        best = child
        visited_names.add(best_name)

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
        resp = await gen_json("", prompt, chat_mdl, gen_conf=_knowledge_compile_gen_conf(chat_mdl, {"temperature": 0.1}))
        if isinstance(resp, dict):
            return str(resp.get("merged", resp.get("result", cluster_desc)))
        if isinstance(resp, str) and resp.strip():
            return resp.strip()
    except Exception:
        logging.exception("dataset_nav: LLM merge failed, keeping original")
    return cluster_desc


def _clean_title(title: str) -> str:
    """Normalize an LLM title into a one-line, length-capped display name."""
    return " ".join((title or "").split())[:48].strip()


def _fallback_title(summary: str) -> str:
    """Derive a short readable title from a summary when the LLM gives none."""
    words = " ".join((summary or "").split()).split(" ")
    return " ".join(words[:6]).strip() or "Cluster"


def _readable_cluster_name(title: str, seed: str) -> str:
    """A readable yet unique nav-cluster key: ``"<title> <8-hex>"``.

    The title makes the node name human-readable; the short hash of ``seed``
    (the cluster description) preserves the per-KB uniqueness the tree keying
    relies on (``_nav_cluster_id`` / ``parent_kwd``).
    """
    suffix = xxhash.xxh64((seed or "").encode("utf-8")).hexdigest()[:8]
    return f"{_clean_title(title) or 'Cluster'} {suffix}"


async def _llm_create_summary(chat_mdl, doc_summaries: list[str]) -> tuple[str, str]:
    """LLM-derive a cluster's readable ``(name, summary)`` from doc summaries.

    ``name`` is a short human-readable topic title used to make the nav node
    name readable (the caller still appends a short hash to keep the tree key
    unique); ``summary`` is the 1-3 sentence description.
    """
    fallback_summary = doc_summaries[0] if doc_summaries else ""
    if not chat_mdl:
        return _fallback_title(fallback_summary), fallback_summary

    from rag.prompts.generator import gen_json

    texts = "\n---\n".join(doc_summaries)
    prompt = (
        "Given the document excerpts below, produce a short human-readable topic "
        "name and a concise description of their common topic.\n\n"
        f"{texts}\n\n"
        'Return ONLY JSON: {"name": "<2-6 word topic title>", "summary": "<1-3 sentence description>"}'
    )
    try:
        resp = await gen_json("", prompt, chat_mdl, gen_conf=_knowledge_compile_gen_conf(chat_mdl, {"temperature": 0.1}))
        if isinstance(resp, dict):
            summary = str(resp.get("summary") or resp.get("result") or fallback_summary).strip()
            name = _clean_title(str(resp.get("name") or "")) or _fallback_title(summary)
            return name, (summary or fallback_summary)
        if isinstance(resp, str) and resp.strip():
            return _fallback_title(resp), resp.strip()
    except Exception:
        logging.exception("dataset_nav: LLM summary failed")
    return _fallback_title(fallback_summary), fallback_summary


# ---------------------------------------------------------------------------
# Public surface
# ---------------------------------------------------------------------------


def build_nav_graph_text(graph_json: Any) -> tuple[str, str]:
    """Build (title, graph_text) from a RAPTOR graph node's content.

    The ``graph_text`` carries the FULL description of EVERY entity (not just
    the first line, and not only root/child entities). Each entity's
    description already includes its name, so we emit ONLY the description
    (no ``name: `` prefix) to avoid redundant artifacts like ``1819: 1819``.
    The result is complete for both keyword (BM25) and dense (KNN) retrieval.

    This is shared by both the ``generate_nav`` path and the parse-time path
    (``run_tree_templates``) so both produce identical, complete nav_doc
    graph_content.

    Args:
        graph_json: parsed ``content_with_weight`` of a
            ``knowledge_graph_kwd="graph"`` node, i.e. a dict with
            ``entities`` (list of ``{name, description, ...}``) and
            ``relations`` (list of ``{from, to, ...}``).

    Returns:
        ``(title, graph_text)``. ``title`` is the root entity's short
        (first-line) label; ``graph_text`` is the full multi-line content.
    """
    if not isinstance(graph_json, dict):
        return "", ""
    entities = graph_json.get("entities") or []
    relations = graph_json.get("relations") or []

    # child names = relation targets (entities that are "under" someone)
    child_names: set[str] = set()
    for rel in relations:
        if isinstance(rel, dict):
            tgt = (rel.get("to") or "").strip()
            if tgt:
                child_names.add(tgt)

    # entity name -> description
    name_desc: dict[str, str] = {}
    for ent in entities:
        if not isinstance(ent, dict):
            continue
        nm = (ent.get("name") or "").strip()
        if nm:
            name_desc[nm] = (ent.get("description") or "").strip()

    # root entity = entity whose name never appears as a relation target
    root_name = ""
    root_summary = ""
    for ent in entities:
        if isinstance(ent, dict) and ent.get("name") not in child_names:
            root_name = (ent.get("name") or "").strip()
            root_summary = name_desc.get(root_name, "")
            root_summary = root_summary.splitlines()[0].strip() if root_summary else root_name
            break

    # Build the full graph text. Each entity's description already includes
    # its name, so we emit ONLY the description (no redundant "name: " prefix)
    # to avoid artifacts like "1819: 1819".
    graph_parts: list[str] = []
    if root_name:
        root_desc = name_desc.get(root_name, "").strip()
        graph_parts.append(root_desc or root_name)

    emitted = {root_name} if root_name else set()
    if len(name_desc) > len(emitted):
        graph_parts.append("")
        for cname in sorted(name_desc):
            if cname in emitted:
                continue
            cdesc = name_desc.get(cname, "").strip()
            if cdesc:
                graph_parts.append(cdesc)
            emitted.add(cname)

    graph_text = "\n".join(graph_parts)
    return root_summary, graph_text


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
        summary_or_tree:
            - A plain summary string (fallback, no RAPTOR graph).
            - A RAPTOR tree dict (backwards-compat, root summary extracted).
            - A dict with keys ``title`` (short display label) and
              ``graph_text`` (full graph entities + relations for richer
              embedding / keyword extraction).
        embd_mdl: LLMBundle for embedding (required for clustering).
        chat_mdl: LLMBundle for chat (required for LLM merge/summary).
    """
    if not doc_id or not kb_id:
        return

    # 1. Extract display summary and optional full graph content.
    graph_content = ""
    if isinstance(summary_or_tree, dict):
        if "title" in summary_or_tree and "graph_text" in summary_or_tree:
            # New extended format from generate_nav's RAPTOR graph path.
            summary = (summary_or_tree.get("title") or "").strip()
            graph_content = (summary_or_tree.get("graph_text") or "").strip()
        else:
            summary = _extract_root_summary_from_tree(summary_or_tree)
    elif isinstance(summary_or_tree, str):
        summary = summary_or_tree
    else:
        summary = ""
    if not summary:
        logging.info("dataset_nav: skipping doc=%s (kb=%s) — no summary", doc_id, kb_id)
        return

    # 2. Embed with the richest available text so that KNN placement
    # benefits from the full tree structure when present.
    embed_text = graph_content or summary
    doc_embedding = await _embed(embd_mdl, embed_text) if embd_mdl else []
    vec_dim = len(doc_embedding)

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
        # 3. Check and replace the existing nav_doc while holding the same
        # lock as placement. Otherwise concurrent updates can both observe
        # the old row and race while deleting/rebuilding its parent cluster.
        existing_doc = await _store_get(tenant_id, kb_id, _nav_doc_id(doc_id))
        if existing_doc:
            old_payload = json.loads(existing_doc.get("content_with_weight") or "{}")
            if old_payload.get("description") == summary:
                return
            await _remove_dataset_nav_doc_locked(tenant_id, kb_id, doc_id)

        # 4. Layered KNN search for nearest cluster
        if _vector_len(doc_embedding) > 0:
            best_name, best_parent, sim = await _find_best_cluster(
                tenant_id,
                kb_id,
                doc_embedding,
                vec_dim,
            )
        else:
            logging.warning("dataset_nav: embedding unavailable for doc=%s, skipping KNN placement", doc_id)
            best_name, best_parent, sim = None, None, 0.0

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
                    if _vector_len(new_emb) > 0:
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
                graph_content=graph_content,
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
            new_title, new_desc = await _llm_create_summary(chat_mdl, [summary])
            new_name = _readable_cluster_name(new_title, summary)
            new_cluster = _make_nav_cluster_row(
                kb_id,
                new_name,
                new_desc,
                parent_for_new,
                depth_of_parent,
                [doc_id],
                doc_embedding,
            )
            if embd_mdl and _vector_len(doc_embedding) > 0:
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
                graph_content=graph_content,
            )
            await _store_upsert(tenant_id, kb_id, nav_doc_row)
        else:
            # ── Create root-level new cluster ──
            root_title, new_desc = await _llm_create_summary(chat_mdl, [summary])
            new_name = _readable_cluster_name(root_title, summary)
            new_cluster = _make_nav_cluster_row(
                kb_id,
                new_name,
                new_desc,
                "root",
                0,
                [doc_id],
                doc_embedding,
            )
            if embd_mdl and _vector_len(doc_embedding) > 0:
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
                graph_content=graph_content,
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


async def _remove_dataset_nav_doc_locked(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
) -> None:
    """Remove a document nav row; the caller must hold the KB nav lock."""
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
        await _remove_dataset_nav_doc_locked(tenant_id, kb_id, doc_id)
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
        if descs:
            group_title, group_desc = await _llm_create_summary(chat_mdl, descs)
        else:
            group_title = group_desc = f"Group {gi + 1}"
        group_name = _readable_cluster_name(group_title, group_desc)
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
    top_k: int | None = None,
    *,
    type_kwd: str = "",
    compile_kwd: str = _COMPILE_KWD,
    doc_scope: list[str] | None = None,
) -> list[dict]:
    """Find the nav-tree nodes most relevant to ``query`` for one KB.

    The nav rows are ``available_int=0`` (invisible to the normal retriever), so
    this is the sanctioned read seam: a caller uses the returned document ids to
    route a scoped chunk retrieval.

    Args:
        type_kwd: Optional type filter — ``"nav_doc"`` to restrict to document
            leaves, ``"nav_cluster"`` to restrict to clusters, or ``""`` for all.
        compile_kwd: Which compile partition to search within
            (default ``_COMPILE_KWD`` = ``"dataset_nav"``).
        doc_scope: Optional set of documents to restrict results to.  Applied
            as a query-time store filter on the type-specific key (``doc_id``
            for ``nav_doc`` leaves, ``doc_ids_kwd`` for ``nav_cluster`` rows)
            and re-checked in memory before the ``top_k`` truncation, so
            scoped rows are never dropped by unscoped ones.

    Returns items shaped as::

        {"type": "nav_doc" | "nav_cluster",
         "doc_id": str | None,      # the document, for a leaf
         "doc_ids": [str],          # the documents a node covers
         "name": str, "description": str, "score": float}

    Ranked by hybrid search: KNN over the node-summary vectors fused with a
    BM25 (full-text) leg over the tokenized fields. When ``embd_mdl`` is absent
    the lexical leg alone ranks the results.
    """
    query = (query or "").strip()
    if not query:
        return []
    allowed_docs = {str(d).strip() for d in (doc_scope or []) if str(d).strip()}
    logging.debug(
        "search_dataset_nav: flat-search scope normalized for kb=%s, scoped_docs=%d",
        kb_id,
        len(allowed_docs),
    )

    condition: dict = {"compile_kwd": [compile_kwd]}
    if type_kwd:
        condition["type_kwd"] = type_kwd
    if allowed_docs:
        # Type-specific scope filter: nav_doc leaves match on doc_id, cluster
        # rows on doc_ids_kwd.  Only set when type_kwd pins a single type —
        # a mixed query can't express both under `_matches_condition`'s
        # AND-across-fields semantics, so mixed scoping relies on the
        # in-memory `_in_nav_scope` check below instead.
        if type_kwd == "nav_doc":
            condition["doc_id"] = sorted(allowed_docs)
        elif type_kwd == "nav_cluster":
            condition["doc_ids_kwd"] = sorted(allowed_docs)
    # name -> [row, fused_score]; `name` uniquely identifies a nav node, so it
    # is the dedup key when the dense and lexical legs return the same node.
    fused: dict[str, list] = {}
    dense_w = _NAV_HYBRID_DENSE_W if embd_mdl is not None else 0.0

    # ── Dense leg: KNN over the node-summary vectors ──
    # fused value: [row, score, text_score] — text_score records the lexical
    # contribution so we can reject pure-nearest hits for an unrelated query.
    if embd_mdl is not None:
        try:
            vec = await _embed(embd_mdl, query)
        except Exception:
            logging.exception("search_dataset_nav: embed failed for kb=%s", kb_id)
            vec = []
        if _vector_len(vec) > 0:
            try:
                rows = await _store_knn(tenant_id, kb_id, vec, len(vec), condition, top_k=top_k or 10000)
                vf = _vec_field(len(vec))
                for r in rows:
                    rk = r.get("name") or r.get("doc_id") or ""
                    if not rk:
                        continue
                    entry = fused.setdefault(rk, [r, 0.0, 0.0])
                    entry[1] += dense_w * _cosine_sim(vec, r.get(vf))
            except Exception:
                logging.exception("search_dataset_nav: knn failed for kb=%s", kb_id)

    # ── Lexical leg: engine BM25 over the tokenized fields ──
    text_w = 1.0 - dense_w
    text_filter = None
    if allowed_docs:
        if type_kwd == "nav_doc":
            text_filter = {"doc_id": sorted(allowed_docs)}
        elif type_kwd == "nav_cluster":
            text_filter = {"doc_ids_kwd": sorted(allowed_docs)}
    try:
        text_rows = await _store_text_search(
            tenant_id,
            kb_id,
            query,
            _NAV_SEARCH_FIELDS,
            limit=max((top_k or 0) * 3, 20) if top_k else 10000,
            compile_kwd=compile_kwd,
            type_kwd=type_kwd,
            extra_filter=text_filter,
        )
    except Exception:
        logging.exception("search_dataset_nav: text search failed for kb=%s", kb_id)
        text_rows = []
    for r in text_rows:
        rk = r.get("name") or r.get("doc_id") or ""
        if not rk:
            continue
        ts = _nav_text_score(query, r)
        if ts <= 0:
            continue
        entry = fused.setdefault(rk, [r, 0.0, 0.0])
        entry[1] += text_w * ts
        entry[2] += ts

    rows_with_scores = [(r, s) for r, s, t in fused.values() if s > 0 and t > 0 and _in_nav_scope(r, allowed_docs)]
    rows_with_scores.sort(key=lambda item: item[1], reverse=True)
    if top_k is not None:
        rows_with_scores = rows_with_scores[:top_k]

    out: list[dict] = []
    for r, score in rows_with_scores:
        try:
            payload = json.loads(r.get("content_with_weight") or "{}")
        except Exception:
            payload = {}
        typ = payload.get("type") or r.get("type_kwd") or ("nav_cluster" if r.get("doc_ids_kwd") else "nav_doc")
        name = r.get("name") or ""
        if typ == "nav_cluster":
            doc_id = None
            doc_ids = _as_str_list(r.get("doc_ids_kwd"))
            if allowed_docs:
                # A scoped search must only surface in-scope documents: cut the
                # cluster's coverage list down to the scoped set so no out-of-
                # scope doc appears under a cluster that merely overlaps the
                # scope.
                doc_ids = [d for d in doc_ids if d in allowed_docs]
            # Once the coverage list is scope-filtered, the reported count must
            # match the returned doc_ids (the raw doc_count_int would otherwise
            # include documents excluded by the scope).
            scoped_cluster = bool(allowed_docs)
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
                "keywords": _as_str_list(payload.get("keywords")),
                "entities": _as_str_list(payload.get("entities")),
                "graph_content": payload.get("graph_content") or "",
                "doc_title": payload.get("doc_title") or "",
                "source_type": payload.get("source_type") or "",
                "doc_count": len(doc_ids) if (typ == "nav_cluster" and scoped_cluster) else int(r.get("doc_count_int") or len(doc_ids) or 0),
                "parent_kwd": _as_str_list(r.get("parent_kwd")),
                "score": float(score or 0.0),
            }
        )
    return out


async def search_nav_tree_descent(
    tenant_id: str,
    kb_id: str,
    query: str,
    embd_mdl,
    top_k: int | None = None,
    doc_scope: list[str] | None = None,
) -> list[dict]:
    """Tree-structured hybrid search: descend from root into the most relevant branches.

    Unlike ``search_dataset_nav`` (flat hybrid search over all nav nodes),
    this respects the parent-child hierarchy: it starts at root clusters,
    then at each level descends into the semantically closest children,
    collecting document identifiers from ``nav_doc`` leaves along the way.

    Each level uses hybrid search (KNN + BM25 text) fused with the same
    weights as ``search_dataset_nav``, so both vector similarity *and*
    lexical matching drive the descent.

    The search uses BFS with beam pruning — at each depth, only the
    *beam_width* most similar clusters are expanded further.

    ``doc_scope`` restricts the search to a specific set of documents: both
    the KNN legs (exact if the engine misses the terms filter, ``_store_knn``
    falls back to a full in-memory match) and the text legs apply the scope
    as a query-time filter, so the top-N truncation never discards scoped
    documents that rank behind unscoped ones.

    Returns items shaped as ``{"doc_id": str, "score": float}``.
    """
    query = (query or "").strip()
    if not query:
        return []
    allowed_docs = {str(d).strip() for d in (doc_scope or []) if str(d).strip()}
    logging.debug(
        "search_nav_tree_descent: tree-search scope normalized for kb=%s, scoped_docs=%d",
        kb_id,
        len(allowed_docs),
    )
    if embd_mdl is None:
        logging.warning(
            "search_nav_tree_descent: embd_mdl is None — falling back to text-only flat search for kb=%s query=%.80s",
            kb_id,
            query,
        )
        raw = await search_dataset_nav(
            tenant_id,
            kb_id,
            query,
            embd_mdl=None,
            top_k=top_k,
            type_kwd="nav_doc",
            doc_scope=list(allowed_docs) or None,
        )
        return [{"doc_id": r.get("doc_id", ""), "score": r.get("score", 0.0)} for r in raw if r.get("doc_id")]

    vec = await _embed(embd_mdl, query)
    vec_dim = _vector_len(vec)
    if vec_dim == 0:
        return []

    beam_width = 5
    dense_w = _NAV_HYBRID_DENSE_W  # same weight as search_dataset_nav
    vf = _vec_field(vec_dim)

    fields = [
        "content_with_weight",
        "name",
        "doc_id",
        "compile_kwd",
        "type_kwd",
        "parent_kwd",
        "depth_int",
        "doc_count_int",
        "doc_ids_kwd",
        vf,
    ]

    collected: list[dict] = []
    seen_docs: set[str] = set()
    seen_nodes: set[str] = set()

    # ── Root clusters (KNN only — no text) ──
    # Root clusters at depth=0 are LLM-generated broad-topic summaries.
    # Specific query terms (e.g. "British") almost never appear in their
    # name / description, so we route purely by vector similarity at this
    # level.  Text matching takes over from depth ≥ 1 where cluster
    # descriptions contain concrete terms.
    root_cond = {
        "kb_id": [kb_id],
        "compile_kwd": [_COMPILE_KWD],
        "type_kwd": ["nav_cluster"],
        "depth_int": [0],
    }
    if allowed_docs:
        root_cond["doc_ids_kwd"] = sorted(allowed_docs)
    roots_knn = await _store_knn(tenant_id, kb_id, vec, vec_dim, root_cond, top_k=beam_width * 3)
    roots_knn = [r for r in roots_knn if _in_nav_scope(r, allowed_docs)]

    # If no root cluster (depth=0) exists — or none overlaps ``doc_scope`` —
    # the dataset may have been compiled without a root, so scan all
    # nav_clusters to find the lowest available depth and start beam search
    # there.
    if not roots_knn:
        all_cond = {
            "kb_id": [kb_id],
            "compile_kwd": [_COMPILE_KWD],
            "type_kwd": ["nav_cluster"],
        }
        if allowed_docs:
            all_cond["doc_ids_kwd"] = sorted(allowed_docs)
        all_clusters = await _store_search(tenant_id, kb_id, all_cond, fields, limit=10000)
        all_clusters = [r for r in all_clusters if _in_nav_scope(r, allowed_docs)]
        if not all_clusters:
            return []

        min_depth = min(
            (r.get("depth_int", 0) for r in all_clusters if r.get("depth_int") is not None),
            default=0,
        )
        logging.warning(
            "search_nav_tree_descent: no root cluster at depth=0, starting beam search from depth=%d",
            min_depth,
        )

        starters = [r for r in all_clusters if r.get("depth_int") == min_depth]
        starters.sort(key=lambda r: _cosine_sim(vec, r.get(vf)), reverse=True)
        current_level = starters[:beam_width]
        for r in current_level:
            r["_score"] = _cosine_sim(vec, r.get(vf))
    else:
        # Route down to beam_width semantically closest root clusters.
        for r in roots_knn:
            r["_score"] = _cosine_sim(vec, r.get(vf))
        roots_knn.sort(key=lambda r: r["_score"], reverse=True)
        current_level = roots_knn[:beam_width]

    if not current_level:
        return []

    while current_level and (top_k is None or len(collected) < top_k):
        next_level: list[dict] = []

        for node in current_level:
            node_name = node.get("name", "")
            if node_name in seen_nodes:
                continue
            seen_nodes.add(node_name)
            parent_score = node.get("_score", 0.0)

            child_cond: dict = {
                "kb_id": [kb_id],
                "compile_kwd": [_COMPILE_KWD],
                "parent_kwd": [node_name],
            }
            if allowed_docs:
                # Children are a mix of nav_doc leaves and nav_cluster rows,
                # which scope on different keys, so fetch each type with its
                # own store filter and merge.  Rows are re-checked with
                # `_in_nav_scope` so a doc filter the engine ignores can't
                # let an unscoped row crowd out a scoped one in the fuse.
                child_doc_cond = {
                    **child_cond,
                    "type_kwd": ["nav_doc"],
                    "doc_id": sorted(allowed_docs),
                }
                child_cluster_cond = {
                    **child_cond,
                    "type_kwd": ["nav_cluster"],
                    "doc_ids_kwd": sorted(allowed_docs),
                }
                children_knn = await _store_knn(tenant_id, kb_id, vec, vec_dim, child_doc_cond, top_k=beam_width * 3)
                children_knn += await _store_knn(tenant_id, kb_id, vec, vec_dim, child_cluster_cond, top_k=beam_width * 3)
                children_knn = [r for r in children_knn if _in_nav_scope(r, allowed_docs)]
                children_text = await _store_text_search(
                    tenant_id,
                    kb_id,
                    query,
                    fields,
                    limit=beam_width * 3,
                    extra_filter={"parent_kwd": [node_name], "kb_id": [kb_id], "doc_id": sorted(allowed_docs)},
                )
                children_text += await _store_text_search(
                    tenant_id,
                    kb_id,
                    query,
                    fields,
                    limit=beam_width * 3,
                    extra_filter={"parent_kwd": [node_name], "kb_id": [kb_id], "doc_ids_kwd": sorted(allowed_docs)},
                )
                children_text = [r for r in children_text if _in_nav_scope(r, allowed_docs)]
            else:
                children_knn = await _store_knn(tenant_id, kb_id, vec, vec_dim, child_cond, top_k=beam_width * 3)
                children_text = await _store_text_search(
                    tenant_id,
                    kb_id,
                    query,
                    fields,
                    limit=beam_width * 3,
                    extra_filter={"parent_kwd": [node_name], "kb_id": [kb_id]},
                )
            candidates = _hybrid_fuse(vec, vf, query, children_knn, children_text, dense_w, beam_width)

            for c in candidates:
                if top_k is not None and len(collected) >= top_k:
                    break
                if c.get("type_kwd") == "nav_doc":
                    doc_id = (c.get("doc_id") or "").strip()
                    score = c["_score"] if c.get("_score") is not None else parent_score
                    if not doc_id or doc_id in seen_docs:
                        continue
                    if allowed_docs and doc_id not in allowed_docs:
                        continue
                    # Tree search is driven by a keyword: a nav_doc that only
                    # matched by vector similarity (no lexical/keyword hit,
                    # i.e. ``_text_score == 0``) is unrelated to the query and
                    # must not surface.  Vector similarity alone always yields
                    # a "nearest" node, so without this gate an unrelated
                    # keyword would return the whole nearest subtree.
                    if not c.get("_text_score"):
                        continue
                    # Skip low-relevance leaves: a below-threshold score means
                    # the query is unrelated to this node, so it must not
                    # surface in the tree-search results.
                    if score < _NAV_TREE_MIN_SCORE:
                        continue
                    seen_docs.add(doc_id)
                    collected.append({"doc_id": doc_id, "score": round(score, 4)})
                else:
                    next_level.append(c)

            if top_k is not None and len(collected) >= top_k:
                break

        if top_k is not None and len(collected) >= top_k:
            break

        next_level.sort(key=lambda c: c.get("_score", 0.0), reverse=True)
        current_level = next_level[:beam_width]

    # Fallback: collect doc_ids from terminal cluster nodes — but only when
    # the terminal cluster itself is relevant (its score clears the threshold
    # AND it has a lexical hit).  Without this guard, an unrelated query
    # (e.g. random gibberish) would always have a nearest cluster and the
    # search would dump the entire subtree's documents back, defeating the
    # purpose of tree search.
    if not collected and current_level:
        for node in current_level:
            node_score = node.get("_score", 0.0) or 0.0
            if node_score < _NAV_TREE_MIN_SCORE:
                continue
            if not node.get("_text_score"):
                continue
            for did in node.get("doc_ids_kwd") or []:
                did_str = str(did).strip()
                if not did_str or did_str in seen_docs:
                    continue
                if allowed_docs and did_str not in allowed_docs:
                    continue
                seen_docs.add(did_str)
                collected.append({"doc_id": did_str, "score": round(node_score, 4)})
                if top_k is not None and len(collected) >= top_k:
                    break
            if top_k is not None and len(collected) >= top_k:
                break

    return collected


def _hybrid_fuse(
    vec: list[float],
    vf: str,
    query: str,
    knn_rows: list[dict],
    text_rows: list[dict],
    dense_w: float,
    top_k: int,
) -> list[dict]:
    """Fuse KNN and text results into a scored, deduplicated, sorted list.

    ``_store_knn`` already enforces filter conditions on the KNN leg;
    text rows come from ``_store_text_search`` which applies its own
    filters.  Here we merge both legs by ``name``/``doc_id``.
    """
    text_w = 1.0 - dense_w
    fused: dict[str, tuple[dict, float, float]] = {}  # key → (row, score, text_score)

    # KNN leg: raw cosine * dense_w, matching search_dataset_nav's scale so
    # both search paths score the dense leg identically.
    for r in knn_rows:
        rk = r.get("name") or r.get("doc_id") or ""
        if not rk:
            continue
        key = f"knn:{rk}"
        fused[key] = (r, _cosine_sim(vec, r.get(vf, [])) * dense_w, 0.0)

    # Text leg.
    for r in text_rows:
        rk = r.get("name") or r.get("doc_id") or ""
        if not rk:
            continue
        ts = _nav_text_score(query, r)
        if ts <= 0:
            continue
        key = f"text:{rk}"
        if key in fused:
            fused[key] = (fused[key][0], fused[key][1] + text_w * ts, fused[key][2] + ts)
        else:
            key_knn = f"knn:{rk}"
            if key_knn in fused:
                fused[key_knn] = (
                    fused[key_knn][0],
                    fused[key_knn][1] + text_w * ts,
                    fused[key_knn][2] + ts,
                )
            else:
                fused[key] = (r, text_w * ts, ts)

    rows_with_scores = [(r, s, t) for r, s, t in fused.values() if s > 0]
    rows_with_scores.sort(key=lambda item: item[1], reverse=True)
    result = rows_with_scores[:top_k]
    for r, s, t in result:
        r["_score"] = s
        # Lexical contribution: a node matched only by vector similarity (no
        # keyword/entity hit) is recorded as _text_score == 0 so callers can
        # reject pure-nearest hits for an unrelated keyword query.
        r["_text_score"] = t
    return [r for r, _, _ in result]


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
    keywords = payload.get("keywords") or []
    entities = payload.get("entities") or []
    graph_content = payload.get("graph_content") or ""
    haystack = " ".join(
        str(x or "")
        for x in (
            row.get("name"),
            payload.get("description"),
            graph_content,
            *keywords,
            *entities,
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
