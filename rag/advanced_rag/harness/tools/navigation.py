"""Navigation tools over a dataset's compiled structures.

Two families live here, answering different questions about a dataset:

1. **Dataset navigation tree** (``dataset_nav``, KB-level). Knowledge
   compilation builds a cluster tree covering EVERY document of the dataset
   (``compile_kwd="dataset_nav"`` with ``type_kwd="nav_cluster"/"nav_doc"`` rows
   linked by ``parent_kwd``). ``_navigate_tree_impl`` routes a question through
   that tree's document leaves to pick which documents to deep-read.

2. **Document structure navigation** (``compile_kwd="tree"/"page_index"/...``,
   document-level). Reads a document's compiled entities+relations and renders a
   drill-down outline, letting the chat model answer from the structure and pull
   the underlying chunks back via each entity's ``source_chunk_ids``.

   Two row shapes coexist for these and are read together: a compact *graph
   blob* (``knowledge_graph_kwd="graph"``, content ``{"entities": [...],
   "relations": [...]}``) written by RAPTOR/tree compilation, and *per-row*
   ``knowledge_graph_kwd="entity"/"relation"`` documents written by page_index
   (and pipeline-Compiler tree). ``compile_kwd`` — NOT ``knowledge_graph_kwd`` —
   is what distinguishes the compile type.

``graph_explore`` reads the compiled knowledge graph (``compile_kwd`` resolving
to ``hypergraph``) and walks it breadth-first from seed entities.
"""

import json
import logging
import re
from dataclasses import dataclass, field
from typing import Any

import json_repair

from rag.advanced_rag.harness.chunk_utils import (
    _chunk_id,
    _chunk_text,
    _snippet,
    _xml_escape,
)
from rag.llm.tool_decorator import tool

_LOG = logging.getLogger(__name__)

# Compiled-structure kinds that describe a document's *layout*: a tree/outline
# or a page index. ``_compilation_template_kind`` in the API folds page_index
# and knowledge_graph into "timeline"; RAPTOR is its own bucket and is
# inherently tree-like, so it counts too.
_CATALOG_KINDS = {"tree", "timeline", "raptor", "page_index", "pageindex"}

# Compiled-structure kinds that describe the document's *concepts*.
_MINDMAP_KINDS = {"mindmap", "mind_map"}

# Cap on evidence chunks pulled from a compiled-structure outline.
_MAX_EVIDENCE_CHUNKS = 24

# Cap on entities offered to the nav-tree entity selector.
_MAX_ENTITIES = 300


@dataclass
class NavResult:
    """Structured outcome of ONE compiled-navigation call.

    ``text`` is what the MODEL sees (unchanged XML). The remaining fields are the
    signals the ORCHESTRATOR needs to route on: does this dataset have the
    structure at all, did THIS query reach anything, and was the result worth
    using? Without them the caller can only regex the XML — which is how a
    ``count="1" entities="0"`` empty shell used to read as a successful hit, and
    how a single unrouted query used to disable the tool for a whole session.

    :param doc_ids: documents reached (the routing result).
    :param routed_docs: ``[(doc_id, summary), ...]`` — the routed documents WITH
        their overall summaries. The summary is the "hint" the orchestrator feeds
        back into retrieval: nav is a hint, not a constraint, so these become a
        soft boost/rerank signal instead of a hard ``doc_scope`` filter.
    :param entities: compiled entities discovered across those documents.
    :param chunk_ptrs: chunk pointers the outline points the model at.
    :param top_score: best beam score reached while drilling (0.0 when the
        keyword fallback scored instead of vectors).
    :param chunk_paths: ``{chunk_id: "root -> ... -> node"}`` — the full
        hierarchy path (from the tree root down) of the node that owns each
        drilled chunk. Lets a caller merge structure context onto retrieved
        evidence: the snippet says WHAT a chunk contains, the path says WHERE it
        sits in the document.
    :param empty_reason: ``""`` when usable, else ``no_structure`` /
        ``no_doc`` / ``infra`` / ``bad_args``. Only ``no_structure`` is a
        statement about the DATASET; ``no_doc`` is about the query.
    """

    text: str = ""
    doc_ids: list = field(default_factory=list)
    routed_docs: list = field(default_factory=list)
    entities: int = 0
    chunk_ptrs: int = 0
    top_score: float = 0.0
    chunk_paths: dict = field(default_factory=dict)
    empty_reason: str = ""


def _normalize_kind(kind) -> str:
    """Mirror the API's kind normalization (page_index/knowledge_graph -> timeline)."""
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index", "knowledge_graph"}:
        return "timeline"
    return normalized


async def _load_compiled_structure(tools, doc_id: str, kinds: set) -> dict:
    """Read a document's compiled graphs for the given template ``kinds``.

    Mirrors the fetch in ``GET /datasets/<id>/documents/<doc_id>/structure/graph``:
    one query for the template-authored graph rows, a second for the RAPTOR row
    (which carries ``compile_kwd`` but no ``knowledge_graph_kwd``).

    Returns ``{"entities": [...], "relations": [...]}`` for the matching buckets.
    """
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    # _resolve_doc_tenant is a short peewee (MySQL) lookup. Running it through
    # thread_pool_exec opens a connection on a short-lived thread; the peewee pool
    # tracks connections in a process-level _in_use set that is only released on
    # close(), so short-lived threads leak connections (MaxConnectionsExceeded
    # under fan-out parallelism). Call it directly on the event-loop thread to
    # reuse the pool's existing connection.
    resolved = tools._resolve_doc_tenant(doc_id)
    if not resolved:
        return {"entities": [], "relations": []}
    kb_id, tenant_id = resolved

    index_name = search.index_name(tenant_id)
    fields = [
        "content_with_weight",
        "compile_kwd",
        "compilation_template_ids",
        "compilation_template_kind_kwd",
        "knowledge_graph_kwd",
        "doc_id",
    ]

    async def _query(condition: dict, limit: int) -> dict:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                fields,
                [],
                condition,
                [],
                OrderByExpr(),
                0,
                limit,
                index_name,
                [kb_id],
            )
            return settings.docStoreConn.get_fields(res, fields) or {}
        except Exception:
            _LOG.exception("ontology_navigate: failed reading compiled structure for doc=%s", doc_id)
            return {}

    # Two row shapes coexist for compiled structures:
    #   * graph blob (``knowledge_graph_kwd="graph"``): a single compact row whose
    #     content is ``{"entities": [...], "relations": [...]}``. This is what
    #     RAPTOR/tree compilation (task_executor ``run_tree_templates`` →
    #     ``_struct_upsert_graph_json``) writes.
    #   * per-entity/relation rows (``knowledge_graph_kwd="entity"/"relation"``):
    #     one row per node/edge, content is a single entity/relation dict. This is
    #     what page_index (and pipeline Compiler tree via
    #     ``_struct_upsert_tree_graph_rows``) writes.
    # ``compile_kwd`` (NOT ``knowledge_graph_kwd``) distinguishes the compile
    # TYPE (tree/page_index/timeline/...); ``knowledge_graph_kwd="graph"`` is a
    # legacy marker we still read for RAPTOR blobs, while the same ``compile_kwd``
    # may also have entity/relation rows. Read BOTH and merge so navigation works
    # regardless of which shape a given compile type produced.
    rows: dict = {}
    rows.update(await _query({"doc_id": [doc_id], "knowledge_graph_kwd": ["graph"]}, 1000))
    rows.update(
        await _query(
            {"doc_id": [doc_id], "knowledge_graph_kwd": ["entity", "relation"]},
            3000,
        )
    )
    rows.update(await _query({"doc_id": [doc_id], "compile_kwd": ["raptor_graph"]}, 16))

    entities: list[dict] = []
    relations: list[dict] = []
    for row in rows.values():
        compile_kwd = row.get("compile_kwd") or ""
        kind = _normalize_kind(row.get("compilation_template_kind_kwd") or compile_kwd)
        if compile_kwd == "raptor_graph":
            kind = "raptor"
        if kind not in kinds:
            continue
        try:
            graph = json.loads(row.get("content_with_weight") or "{}")
        except Exception:
            continue
        if not isinstance(graph, dict):
            continue
        # graph blob: compact graph with nested entities/relations
        if row.get("knowledge_graph_kwd") == "graph":
            entities.extend(graph.get("entities") or [])
            relations.extend(graph.get("relations") or [])
        # per-entity/relation row: content is a single node/edge
        elif row.get("knowledge_graph_kwd") == "entity":
            entities.append(graph)
        elif row.get("knowledge_graph_kwd") == "relation":
            relations.append(graph)

    return {"entities": entities, "relations": relations}


async def _load_chunks_by_ids(tools, doc_id: str, chunk_ids: list[str]) -> list[dict]:
    """Fetch chunks by their ids from the doc store."""
    if not chunk_ids:
        return []
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    # peewee MySQL lookup — call directly to reuse the pool's connection (see note).
    resolved = tools._resolve_doc_tenant(doc_id)
    if not resolved:
        return []
    kb_id, tenant_id = resolved

    fields = ["content_with_weight", "docnm_kwd", "doc_id"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"id": chunk_ids[:_MAX_EVIDENCE_CHUNKS]},
            [],
            OrderByExpr(),
            0,
            _MAX_EVIDENCE_CHUNKS,
            search.index_name(tenant_id),
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        _LOG.exception("ontology_navigate: failed loading evidence chunks for doc=%s", doc_id)
        return []

    chunks = []
    for cid, row in rows.items():
        chunks.append(
            {
                "chunk_id": cid,
                "content_with_weight": row.get("content_with_weight") or "",
                "docnm_kwd": row.get("docnm_kwd") or "",
                "doc_id": row.get("doc_id") or doc_id,
            }
        )
    return chunks


def _doc_aggs(chunks: list[dict]) -> list[dict]:
    aggs, seen = [], set()
    for c in chunks:
        did = c.get("doc_id")
        if did and did not in seen:
            seen.add(did)
            aggs.append({"doc_id": did, "doc_name": c.get("docnm_kwd") or ""})
    return aggs


async def _navigate_within_doc(
    tools,
    topic: str,
    keywords: str,
    doc_scope: list[str] | None,
    kinds: set,
) -> list[dict]:
    """Return the chunk texts behind the entities the model finds relevant.

    Reads the compiled ``kinds`` structure of every doc in ``doc_scope`` — the
    tree / page-index catalog, or the concept mindmap — asks the chat model
    which of those entities are relevant to the question, and returns the chunks
    attached to the selected entities' ``source_chunk_ids``.

    Routing only — no fallback search. Returns ``[]`` when there is no doc scope,
    no question/keywords, no compiled structure of these kinds, no relevant
    entity, or no source chunks behind the selected entities.
    """
    if not doc_scope:
        return []
    query = " ".join(part for part in ((topic or "").strip(), (keywords or "").strip()) if part).strip()
    if not query:
        return []

    # 1. List the compiled structure entities of each doc, tagged with their
    #    originating doc_id so their chunks load from the right document.
    entities: list[dict] = []
    for doc_id in doc_scope:
        structure = await _load_compiled_structure(tools, doc_id, kinds)
        for e in structure.get("entities") or []:
            if isinstance(e, dict) and (e.get("name") or "").strip():
                entities.append({**e, "_doc_id": doc_id})
    if not entities:
        return []

    # 2. Ask the model which entities are relevant to the question.
    selected = await _ask_nav_select(tools, query, entities, "entities", _MAX_ENTITIES)
    if not selected:
        return []

    # 3. Load the chunks attached to the selected entities (grouped by doc).
    ids_by_doc: dict[str, list[str]] = {}
    seen: set[tuple[str, str]] = set()
    for e in selected:
        doc_id = e.get("_doc_id")
        if not doc_id:
            continue
        for cid in e.get("source_chunk_ids") or []:
            if isinstance(cid, str) and cid and (doc_id, cid) not in seen:
                seen.add((doc_id, cid))
                ids_by_doc.setdefault(doc_id, []).append(cid)

    chunks: list[dict] = []
    for doc_id, ids in ids_by_doc.items():
        chunks.extend(await _load_chunks_by_ids(tools, doc_id, ids))
    _LOG.info("[Navigation] Selected %d entity(ies) → %d source chunk(s).", len(selected), len(chunks))
    return chunks


async def ontology_navigate(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Answer from the documents' compiled catalog (tree / page index).

    :returns: ``{"answer": "", "chunks": [...], "doc_aggs": [...]}``
    """
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    if not doc_scope:
        doc_scope = []
    _LOG.info(f'[Ontology navigation] Looking through the document catalog for "{topic}" (keywords: {keywords}) in doc: {len(doc_scope)}')
    if not doc_scope:
        _LOG.info(f'[Ontology navigation] No doc scope provided: "{topic}" (keywords: {keywords})')
        return {"answer": "", "chunks": [], "doc_aggs": []}
    chunks = await _navigate_within_doc(tools, topic, keywords, doc_scope, _CATALOG_KINDS)
    return {"answer": "", "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}


async def mindmap_navigate(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> dict:
    """Answer from the documents' compiled concept mindmap.

    ``topic`` (not ``concept``) — the parameter name must match the registered
    ``_navigate_schema``, otherwise every LLM call raises a TypeError.

    :returns: ``{"answer": "", "chunks": [...], "doc_aggs": [...]}``
    """
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    if not doc_scope:
        doc_scope = []
    _LOG.info(f'[Mindmap navigation] Following the concept mindmap for "{topic}" (keywords: {keywords}) in doc: {len(doc_scope)}')
    chunks = await _navigate_within_doc(tools, topic, keywords, doc_scope, _MINDMAP_KINDS)
    return {"answer": "", "chunks": chunks, "doc_aggs": _doc_aggs(chunks)}


# ── Dataset navigation (document router) ────────────────────────────────────

_NAV_MAX_DOCS = 8  # documents the nav tree routes a query to
_NAV_MAX_HITS_PER_KB = 8

# Tree-walk router tunables.
_NAV_MAX_CLUSTERS = 500  # top-level clusters listed / rendered to the LLM
_NAV_CHILDREN_PAGE_SIZE = 1000  # children fetched per node
_NAV_TREE_MAX_DEPTH = 6  # BFS depth cap when descending sub-clusters to leaves
_NAV_TREE_MAX_LEAVES = 300  # document leaves rendered to the doc-select LLM

# Chunk-level content recall (fallback) tunables.
# The nav tree routes by *cluster summaries*; a question that matches a detail
# only present in a document's body (not its one-line summary) can fall through
# the tree.  We therefore back the tree result with a plain chunk retrieval and
# fold the documents it hits back in as a recall fallback — reusing the existing
# chunk index, so no new compilation artifact is required.
_NAV_RECALL_TOP_N = 40  # chunk candidates fetched before doc aggregation
_NAV_RECALL_MAX_DOCS = 4  # extra docs the content recall may add on top of the tree

_NAV_SELECT_SYSTEM = """You are routing a question through a dataset's navigation tree.

You are given a QUESTION and a numbered list of {noun}, each with a name and a short description.
Choose the {noun} most likely to contain information relevant to answering the question.

Rules:
1. Judge only from the names and descriptions shown.
2. Be selective — include an item only if it is plausibly relevant. Include several when several are equally plausible.
3. If none are clearly relevant, return an empty list.
4. Return the bracketed index numbers of the chosen {noun}.

Output ONLY JSON, no prose, no code fences:
{{"relevant": [<index>, ...]}}"""


async def _ask_nav_select(tools, query: str, items: list[dict], noun: str, max_items: int) -> list[dict]:
    """Ask the chat model which of ``items`` are relevant to ``query``.

    Items are rendered as a numbered list of ``name`` (+ optional ``doc_count``)
    and ``description``; the model returns the chosen indices. Index-based (not
    id-based) so the model never has to reproduce opaque doc ids or exact names.
    Returns the selected item dicts (a subset of ``items``), or ``[]`` when the
    model declines or the call fails.
    """
    if not items:
        return []
    from rag.prompts.generator import form_message, message_fit_in

    capped = items[:max_items]
    lines = []
    for i, it in enumerate(capped):
        name = str(it.get("name") or "").strip() or f"item-{i}"
        desc = str(it.get("description") or "").strip().replace("\n", " ")
        extra = f" [{it['doc_count']} docs]" if it.get("doc_count") else ""
        kwds = it.get("keywords") or []
        tags = ", ".join(str(k) for k in kwds[:6]).strip()
        head = f" [tags: {tags}]" if tags else ""
        entities = it.get("entities") or []
        ents = ", ".join(str(e) for e in entities[:6]).strip()
        head += f" [entities: {ents}]" if ents else ""
        lines.append(f"[{i}] {name}{extra}{head}: {desc[:300]}")

    system = _NAV_SELECT_SYSTEM.format(noun=noun)
    user = f"Question:\n{query}\n\n{noun.capitalize()} (numbered):\n" + "\n".join(lines) + "\n\nOutput JSON:"
    try:
        _, msg = message_fit_in(form_message(system, user), tools.chat_mdl.max_length)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.2})
        if isinstance(ans, tuple):
            ans = ans[0]
        cleaned = re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
        cleaned = re.sub(r"```(?:json)?\s*|\s*```", "", cleaned).strip()
        verdict = json_repair.loads(cleaned) or {}
    except Exception:
        _LOG.exception("[Dataset navigation] LLM %s selection failed", noun)
        return []
    if not isinstance(verdict, dict):
        return []
    raw = verdict.get("relevant")
    if not isinstance(raw, list):
        return []

    out: list[dict] = []
    seen_idx: set[int] = set()
    for r in raw:
        try:
            idx = int(r)
        except (TypeError, ValueError):
            continue
        if 0 <= idx < len(capped) and idx not in seen_idx:
            seen_idx.add(idx)
            out.append(capped[idx])
    return out


async def _collect_nav_leaves(dataset_api_service, clusters: list[dict], doc_scope: list[str] | None = None) -> list[dict]:
    """BFS from the selected clusters down to their document leaves.

    Each cluster carries ``name`` + ``kb``. A node's children are either document
    leaves (``type == "doc"`` — collected, deduped by ``doc_id``) or sub-clusters
    (descended, depth- and count-capped).
    """
    leaves: list[dict] = []
    seen_docs: set[str] = set()
    seen_nodes: set[tuple] = set()
    frontier: list[tuple] = [(c["kb"], c["name"], 0) for c in clusters if c.get("name")]
    allowed_docs = set(doc_scope or [])

    while frontier and len(leaves) < _NAV_TREE_MAX_LEAVES:
        kb, name, depth = frontier.pop(0)
        node_key = (kb.id, name)
        if node_key in seen_nodes:
            continue
        seen_nodes.add(node_key)
        try:
            ok, data = await dataset_api_service.list_nav_children(kb.id, kb.tenant_id, name, page=1, page_size=_NAV_CHILDREN_PAGE_SIZE)
        except Exception:
            _LOG.exception("[Dataset navigation] list_nav_children failed for kb=%s node=%s", kb.id, name)
            continue
        if not ok or not isinstance(data, dict):
            continue
        for item in data.get("items") or []:
            if item.get("type") == "doc":
                did = str(item.get("doc_id") or "").strip()
                if did and (not allowed_docs or did in allowed_docs) and did not in seen_docs:
                    seen_docs.add(did)
                    leaves.append({**item, "kb": kb})
                    if len(leaves) >= _NAV_TREE_MAX_LEAVES:
                        break
            elif item.get("type") == "cluster" and item.get("name") and depth + 1 < _NAV_TREE_MAX_DEPTH:
                frontier.append((kb, item["name"], depth + 1))
    return leaves


def _nav_cluster_names(clusters: list[dict]) -> str:
    names = [str(c.get("name") or "").strip() for c in clusters]
    return ", ".join(n for n in names if n) or "none"


async def _content_recall_docs(tools, query: str, doc_scope: list[str] | None = None) -> list[str]:
    """Fallback doc discovery: recall by chunk *content*, aggregated to docs.

    Runs a plain hybrid retrieval over the bound KBs' chunk index and returns the
    doc_ids of the hit documents, most-hit-first.  This is the safety net for the
    nav-tree router: a question that matches detail living only in a document's
    body (not its cluster summary) never appears in the tree, so this retrieval —
    which reads real chunk text — is what catches it.

    ``doc_scope`` (the already-effective routed doc scope) is forwarded as
    ``doc_ids`` to the retrieval so content recall stays restricted to the same
    documents the tree routed to, instead of re-opening the whole KB.

    Returns ``[]`` on failure or when nothing hits.
    """
    if not query:
        return []
    from common import settings

    target_ids = getattr(tools, "kb_ids", None) or []
    if not target_ids:
        return []
    # Hybrid weight: blend vector + term recall when an embedder exists, and
    # fall back to pure term matching otherwise (mirrors ``hybrid_search``).
    embd_mdl = getattr(tools, "embed_mdl", None)
    vector_weight = 0.3 if embd_mdl else 0
    try:
        kbinfos = await settings.retriever.retrieval(
            query,
            embd_mdl,
            getattr(tools, "tenant_ids", None),
            target_ids,
            1,
            _NAV_RECALL_TOP_N,
            0.2,
            vector_similarity_weight=vector_weight,
            doc_ids=doc_scope,
            aggs=True,
            highlight=False,
        )
    except Exception:
        _LOG.exception("[Dataset navigation] content-recall retrieval failed")
        return []
    doc_ids: list[str] = []
    seen: set[str] = set()
    for agg in kbinfos.get("doc_aggs") or []:
        did = str(agg.get("doc_id") or "").strip()
        if did and did not in seen:
            seen.add(did)
            doc_ids.append(did)
    _LOG.info("[Dataset navigation] Content recall found %d candidate doc(s).", len(doc_ids))
    return doc_ids


async def dataset_navigation_by_tree(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> list[str]:
    """Return the ``doc_id``s most relevant to the question / keywords by walking
    the dataset nav tree with the chat model.

    The nav tree is a KB-level RAPTOR-style summary of every document. Two LLM
    passes narrow the corpus coarse-to-fine:

      1. List the top-level clusters (``list_nav_clusters``).
      2. Ask the model which clusters are relevant to the question.
      3. Descend those clusters to their document leaves (``list_nav_children``,
         recursing sub-clusters).
      4. Ask the model which documents are worth reading.

    Returns the routed ``doc_id`` list (capped at ``_NAV_MAX_DOCS``), or ``[]``
    when no question/keywords are given, there is no cluster tree, or the model
    finds nothing relevant. This function only routes — it does not retrieve.
    """
    query = " ".join(part for part in ((topic or "").strip(), (keywords or "").strip()) if part).strip()
    if not query:
        return []
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)

    _LOG.info('[Dataset navigation] Walking the dataset tree for "%s"', query)

    from api.apps.services import dataset_api_service

    kbs = getattr(tools, "kbs", []) or []

    # 1. List every top-level cluster across the bound KBs (tagged with its KB).
    clusters: list[dict] = []
    for kb in kbs:
        try:
            ok, data = await dataset_api_service.list_nav_clusters(kb.id, kb.tenant_id, page=1, page_size=_NAV_MAX_CLUSTERS)
        except Exception:
            _LOG.exception("[Dataset navigation] list_nav_clusters failed for kb=%s", kb.id)
            continue
        if not ok or not isinstance(data, dict):
            continue
        for item in data.get("items") or []:
            if item.get("type") == "cluster" and item.get("name"):
                clusters.append({**item, "kb": kb})

    if not clusters:
        _LOG.info("[Dataset navigation] no cluster there — falling back to content recall.")
        return (await _content_recall_docs(tools, query, doc_scope))[:_NAV_MAX_DOCS]

    # 2. Ask the model which clusters are relevant. Nothing relevant → [].
    selected_clusters = await _ask_nav_select(tools, query, clusters, "clusters", _NAV_MAX_CLUSTERS)
    if not selected_clusters:
        _LOG.info("[Dataset navigation] no cluster found — falling back to content recall.")
        return (await _content_recall_docs(tools, query, doc_scope))[:_NAV_MAX_DOCS]
    _LOG.info("[Dataset navigation] %d/%d cluster(s) selected.", len(selected_clusters), len(clusters))

    # 3. Descend the selected clusters to their document leaves.
    leaves = await _collect_nav_leaves(dataset_api_service, selected_clusters, doc_scope)
    if not leaves:
        _LOG.info("[Dataset navigation] no leaf under selected cluster %s — falling back to content recall.", _nav_cluster_names(selected_clusters))
        return (await _content_recall_docs(tools, query, doc_scope))[:_NAV_MAX_DOCS]

    # 4. Ask the model which documents to look into. Nothing relevant → [].
    selected_docs = await _ask_nav_select(tools, query, leaves, "documents", _NAV_TREE_MAX_LEAVES)
    if not selected_docs:
        _LOG.info("[Dataset navigation] no doc selected under cluster %s — falling back to content recall.", _nav_cluster_names(selected_clusters))
        return (await _content_recall_docs(tools, query, doc_scope))[:_NAV_MAX_DOCS]

    routed: list[str] = []
    seen_docs: set[str] = set()
    for d in selected_docs:
        did = str(d.get("doc_id") or "").strip()
        if did and did not in seen_docs:
            seen_docs.add(did)
            routed.append(did)
    _LOG.info("[Dataset navigation] Routed to %d document(s).", len(routed))

    # 5. Content-recall fallback.  The tree routes only by cluster summaries, so
    #    a question matching a detail present only in a document's body can fall
    #    through it.  Back the routed set with a plain chunk retrieval: docs that
    #    actually hit by content are folded back in (deduped, tree docs first) so
    #    a missed document still reaches the caller.  Skip the extra retrieval
    #    when the tree already filled the whole cap — nothing would be added.
    if len(routed) < _NAV_MAX_DOCS:
        fallback = await _content_recall_docs(tools, query, doc_scope)
        added = [d for d in fallback if d not in seen_docs]
        if added:
            routed.extend(added[:_NAV_RECALL_MAX_DOCS])
            _LOG.info(
                "[Dataset navigation] Content recall added %d fallback doc(s) on top of the %d tree-routed one(s).",
                len(added[:_NAV_RECALL_MAX_DOCS]),
                len(routed) - len(added[:_NAV_RECALL_MAX_DOCS]),
            )
    return routed[:_NAV_MAX_DOCS]


# ── Dataset document search (hybrid, no LLM) ────────────────────────────────

_NAV_SEARCH_MAX_DOCS = 12  # documents the hybrid search routes to
_NAV_MIN_DOC_SCORE = 0.2  # drop docs below this score


async def _nav_search_titled(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> list[tuple[str, str]]:
    """Descend the compiled nav TREE and return ``(doc_id, summary)`` pairs.

    Uses ``mode="navigation_tree"``, which currently resolves to a FLAT hybrid
    sweep over every ``nav_doc`` row (``_NAV_TREE_ROUTER = "nav_doc"``) rather
    than a descent through the cluster levels. The strategy lives server-side, so
    this function is agnostic to which one is active.

    Flat routing is deliberate. A BFS beam descent narrows the field level by
    level, and a wrong pick at the first level cannot be undone — the correct
    branch was already pruned and never reaches the comparison. Nav rows embed
    ``graph_text`` (the document's full entity/relation listing), so they are
    discriminating enough to be compared directly without the cluster levels
    narrowing them for us.

    HYBRID sweep (vector + BM25): both the KNN legs and the ``_text_score``
    gates stay armed. A pure-vector routing was tried — it dropped the text legs
    and the ``_text_score > 0`` gate, on the theory that routing cares about
    semantic matches a summary never spells out. It regressed routing badly:
    scores collapsed onto the dense leg alone, the descent fell below
    ``_NAV_TREE_MIN_SCORE`` far more often, and routing returned 1-5 docs instead
    of hybrid's 7-12 — with 6 outright zero-doc failures per 20-question run.
    Losing that scope let the downstream retrieval degenerate into a corpus-wide
    search, which filled the evidence pool with unrelated chunks (gathered
    passages 85 -> 105, max 213 -> 356) and drove SCA INSUFFICIENT from 6 to 27
    per run. Routing needs both legs; the vector-only path was removed.

    Each nav row already carries the document's overall summary (``description``
    on the leaf row), so labelling the route costs ZERO extra queries — the
    caller must never load the document for a label.

    Routing only — this never retrieves document content.
    """
    query = " ".join(part for part in ((topic or "").strip(), (keywords or "").strip()) if part).strip()
    if not query:
        return []
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)

    _LOG.info('[Dataset navigation search] Nav-tree doc search for "%s"', query)

    from api.apps.services import dataset_api_service

    kbs = getattr(tools, "kbs", []) or []
    allowed_docs = set(doc_scope or [])

    candidates: dict[str, tuple[float, str]] = {}
    for kb in kbs:
        # ``doc_scope`` is forwarded query-time (search_dataset_layers applies
        # it as a store filter on every mode), so the top_k truncation never
        # drops scoped docs — no enlarged pool is needed.
        try:
            ok, result = await dataset_api_service.search_dataset_layers(
                kb.id,
                kb.tenant_id,
                query,
                "navigation_tree",
                top_k=_NAV_SEARCH_MAX_DOCS,
                doc_scope=list(allowed_docs) or None,
            )
        except Exception:
            _LOG.exception("[Dataset navigation search] search_dataset_layers failed for kb=%s", kb.id)
            continue
        if not ok or not isinstance(result, dict):
            continue
        for item in result.get("items", []):
            score = float(item.get("score", 0.0))
            if score < _NAV_MIN_DOC_SCORE:
                continue
            did = str(item.get("doc_id") or "").strip()
            if not did:
                continue
            # The nav leaf row carries the document's OVERALL summary. Using it
            # here is the whole point: the route is labelled for free, instead of
            # loading the document to read its first chunk.
            nav = item.get("_nav") or {}
            summary = str(nav.get("description") or nav.get("name") or "").strip()
            prev = candidates.get(did)
            if prev is None or score > prev[0]:
                candidates[did] = (score, summary)

    routed = [(did, summary) for did, (score, summary) in sorted(candidates.items(), key=lambda pair: pair[1][0], reverse=True)[:_NAV_SEARCH_MAX_DOCS]]

    _LOG.info("[Dataset navigation search] Routed to %d document(s) (min_score=%.1f).", len(routed), _NAV_MIN_DOC_SCORE)
    return routed[:_NAV_SEARCH_MAX_DOCS]


async def dataset_navigation_search(tools, topic: str, keywords: str = "", doc_scope: list[str] | None = None) -> list[str]:
    """Return the ``doc_id``s most relevant to the question / keywords by
    descending the dataset's compiled navigation tree.

    Routes through :func:`_nav_search_titled`, which runs
    ``search_dataset_layers`` with ``mode="navigation_tree"``: a hybrid
    (vector + BM25) sweep over the ``nav_doc`` rows, with the ``_text_score``
    gates armed. Which router runs is decided server-side by
    ``_NAV_TREE_ROUTER``, so this function does not depend on the strategy.

    Returns the routed ``doc_id`` list (capped at ``_NAV_SEARCH_MAX_DOCS``), or
    ``[]`` when no question/keywords are given or the search returns nothing.
    This function only routes — it does not retrieve.
    """
    return [did for did, _title in await _nav_search_titled(tools, topic, keywords, doc_scope)]


# Knowledge-graph and wiki exploration live in ``exploration`` and are
# re-exported here: compiled-product expansion imports ``_kg_scopes`` from this
# module, and the action-session tool surface exposes ``graph_explore`` by this
# name.
from rag.advanced_rag.harness.tools.exploration import (  # noqa: F401
    _collect_evidence_ids,
    _endpoint_terms,
    _kg_parse_entity,
    _kg_parse_relation,
    _kg_scopes,
    _kg_search,
    _SCOPE_KWD_DATASET,
    _SCOPE_KWD_DOC,
    graph_explore,
)

# ── Navigate-tree/structure tool set (migrated from harness/dynamic) ──
_NAV_TREE_MAX_DOCS = 8

# Cap on datasets scanned per tool call (avoids fan-out over too many KBs).
_NAV_TREE_MAX_DATASETS = 10

# Chars of a chunk's text returned by search_chunks in default (snippet) mode.
_SEARCH_SNIPPET_CHARS = 300

# ── XML helpers (uniform tag vocabulary shared by all retrieval tools) ──


def _rank_chunks_by_terms(candidates: list[dict], queries: list[str]) -> list[dict]:
    """Rank candidate chunks by how many query terms overlap with their text.

    Zero-LLM keyword relevance for the precise ``chunk_ids`` search path (the
    scope is a handful of chunks, so a cheap overlap score suffices). Returns
    chunks sorted most-relevant-first.
    """
    # Collect significant terms from all queries.
    terms: list[str] = []
    for q in queries:
        for tok in re.findall(r"[A-Za-z0-9_]{2,}", (q or "").lower()):
            if tok not in terms:
                terms.append(tok)
    if not terms:
        return list(candidates)
    scored = []
    for c in candidates:
        text = _chunk_text(c).lower()
        hits = sum(1 for t in terms if t in text)
        if hits:
            scored.append((hits, c))
    scored.sort(key=lambda x: x[0], reverse=True)
    return [c for _, c in scored]


# The rag-agent runner sets this module slot to the active RAGTools instance for
# the current request, so the migrated navigation tools (defined once at import)
# read the request-scoped retrieval context.
_tools_ref: dict[str, Any] = {}


def _tools_slot():
    return _tools_ref.get("tools")


def _get_kb_ids(tools_slot) -> list[str]:
    if tools_slot is None:
        return []
    ids = getattr(tools_slot, "kb_ids", None) or []
    return list(ids)


# ── Tool: navigate_tree (compiled tree structure, high/ultra modes) ──


@tool(timeout=120)
async def _navigate_tree_impl(
    query: str,
    keywords: str = "",
    doc_scope: list[str] | None = None,
) -> NavResult:
    """Locate the document(s) most likely to hold the answer, by descending the
    dataset's compiled navigation tree (``dataset_nav``).

    This tool ROUTES, it does not retrieve: it walks the compiled cluster
    hierarchy — root cluster → sub-clusters → ``nav_doc`` leaves — with BFS beam
    pruning, and returns the ``doc_id`` list reached along the most relevant
    branches. Descending the hierarchy beats a flat sweep over every document
    summary when the dataset is large and many summaries look alike.

    Use it when search_chunks / grep_chunks did not clearly locate the answer and
    you need the document-level view to guide you (e.g. an entity spans many
    docs, or the question mentions a general topic that lives across a document's
    body). After it returns doc_id values, use navigate_structure / list_chunks on
    the relevant one(s) to get the actual passages.

    It deliberately does NOT fetch document content. Loading full documents here
    cost ~79 ES queries per document (``fetch_full_document`` pages 128 chunks at
    a time) only to render a 500-char snippet that navigate_structure supersedes —
    under the per-pass session fan-out that alone was enough to knock ES over.

    Returns an XML <tree_navigation> document. Each routed document is a <doc>
    element with a ``doc_id`` attribute and, when the nav leaf has one, a
    <summary> child holding the document's overall summary. No <snippet>: the
    content decision belongs to navigate_structure. If nothing routes, an empty
    <tree_navigation/> is returned.

    :param query: REQUIRED: the question / topic to route.
    :param keywords: Optional terms kept only for prompt hints; routing descends
        on vector similarity and does not require keyword hits.
    :param doc_scope: Optional document ids to restrict routing to (at most 8).
    :return: :class:`NavResult` — the XML in ``text`` plus the routed ``doc_ids``.
    """
    from common import settings

    if not getattr(settings, "retriever", None):
        return NavResult(text='<tree_navigation count="0" error="no retriever">\n</tree_navigation>', empty_reason="infra")
    query = str(query or "").strip()
    if not query:
        return NavResult(text='<tree_navigation count="0" error="query is required">\n</tree_navigation>', empty_reason="bad_args")

    tools_slot = _tools_slot()
    # PURE VECTOR descent over the compiled nav tree (dataset_nav), so a query
    # reaches documents that term matching would miss. The climb narrows the
    # candidate set level by level instead of comparing every document summary at
    # once — accuracy at scale, at the cost of a bounded number of extra KNN
    # queries (beam_width per depth).
    #
    # There is deliberately NO chunk-retrieval fallback here. Routing used to fall
    # back to a full retriever pass over every chunk whenever the nav search came
    # back empty — the generic retriever reinventing what dataset_nav already
    # does, and by far the most expensive thing this file could do. When routing
    # misses, the caller falls back to retrieve/search_chunks: the same work, done
    # once and owned by the orchestrator instead of hidden inside a "route" call.
    routed = await _nav_search_titled(tools_slot, query, keywords, doc_scope)
    if not routed:
        # THIS query routed to nothing. The dataset may still have a nav tree that
        # a better-formed query would hit — a query-level miss, not a dataset fact.
        return NavResult(text='<tree_navigation count="0">\n</tree_navigation>', empty_reason="no_doc")

    routed = routed[:_NAV_TREE_MAX_DOCS]
    parts = [f'<tree_navigation count="{len(routed)}" query="{_xml_escape(query)}">']
    for i, (doc_id, summary) in enumerate(routed):
        if summary:
            parts.append(f'  <doc rank="{i + 1}" doc_id="{_xml_escape(doc_id)}">')
            parts.append(f"    <summary>{_xml_escape(summary)}</summary>")
            parts.append("  </doc>")
        else:
            parts.append(f'  <doc rank="{i + 1}" doc_id="{_xml_escape(doc_id)}"/>')
    parts.append("</tree_navigation>")
    return NavResult(
        text="\n".join(parts),
        doc_ids=[did for did, _ in routed],
        routed_docs=[(did, summary) for did, summary in routed],
    )


def _first_tenant_id(tools_slot) -> str:
    """Best-effort tenant id from the RAGTools context."""
    try:
        for attr in ("tenant_id", "tenant_ids"):
            v = getattr(tools_slot, attr, None)
            if isinstance(v, list):
                for t in v:
                    if str(t or "").strip():
                        return str(t)
            elif v:
                return str(v)
    except Exception:
        pass
    try:
        kbs = getattr(tools_slot, "kbs", None) or []
        for kb in kbs:
            tid = getattr(kb, "tenant_id", None)
            if tid:
                return str(tid)
    except Exception:
        pass
    return ""


# ── Tool: navigate_structure (compiled structure of a document, high/ultra) ──


@tool(timeout=120)
async def _navigate_structure_impl(
    query: str,
    doc_id: str = "",
    kind: str = "catalog",
    keywords: str = "",
    doc_scope: list[str] | None = None,
) -> str:
    """Read a document's COMPILED STRUCTURE (tree / page-index catalog, concept
    mindmap, or entity graph) and return its entities, relations, and the chunks
    behind them — so you can see how the document is organized internally.

    This is the "inside-the-doc" counterpart to navigate_tree. Use it when you
    have located a document (via search_chunks / grep_chunks / navigate_tree) and
    want to understand its internal structure — the catalog tree of headings, the
    concept mindmap, or the entity graph — to find where an answer lives without
    reading every chunk.

    Returns an XML <structure_navigation> document. Each analyzed document is a
    <doc> element with doc_id / doc_title and a <structure> element — a compact
    OUTLINE of the compiled entities and relations, with each line annotating the
    source-chunk ids that back it (e.g. ``[chunks: c100,c101]``). This is an
    outline only (low token cost) — it does NOT include the chunk text. To read
    the actual content behind a line, call list_chunks with its doc_id and
    chunk_ids to deep-read precisely those chunks. If the document has no compiled
    structure of the requested kind, an empty <doc/> is returned (no error) — fall
    back to list_chunks / search_chunks.

    Cost: the structure rows themselves are read from the doc store. When the
    document's nodes carry their own embeddings (page_index), the whole TOC is
    additionally handed to the chat model once, so it can pick the relevant
    sections — see ``_STRUCT_TOC_LLM_SELECT``. That pass is a single call per
    document, and an empty or failed result falls through to local scoring, so
    the tool still works without a chat model.

    :param query: REQUIRED: the question/topic (used to route to a document when
        ``doc_id`` is not given).
    :param doc_id: Optional: read the structure of this specific document. When
        omitted, documents are located by PURE embedding similarity (no keyword
        gate), not the compiled nav tree.
    :param kind: Which compiled structure to read: "catalog" (tree / page-index /
        timeline, default), "mindmap" (concept mindmap), or "graph" (entity graph).
    :param keywords: Optional terms kept only for prompt hints; document routing
        is embedding-driven and does not require keyword hits.
    :param doc_scope: Optional document ids to restrict routing to (at most 8).
    :return: XML <structure_navigation> document (outline only).
    """
    from common import settings

    if not getattr(settings, "retriever", None):
        return NavResult(text='<structure_navigation count="0" error="no retriever">\n</structure_navigation>', empty_reason="infra")
    query = str(query or "").strip()
    if not query:
        return NavResult(text='<structure_navigation count="0" error="query is required">\n</structure_navigation>', empty_reason="bad_args")

    # Determine which compiled kinds to read.
    kinds = _structure_kinds_for(kind)

    tools_slot = _tools_slot()
    if doc_id:
        doc_ids = [str(doc_id).strip()] if str(doc_id).strip() else []
    else:
        # Route by PURE VECTOR similarity over the nav tree — the same seam
        # navigate_tree uses. This used to fall back to a full chunk-retriever
        # pass, which reinvented nav routing with the most expensive tool in the
        # file: a hybrid sweep over every chunk of the dataset to answer "which
        # document?", a question dataset_nav already answers with one KNN query.
        doc_ids = [did for did, _summary in await _nav_search_titled(tools_slot, query, "", doc_scope)]
    if not doc_ids:
        # The QUERY routed to nothing (or the caller passed a doc_id no route ever
        # produced). Not a statement about the dataset's compiled structures.
        return NavResult(text='<structure_navigation count="0" error="no document located">\n</structure_navigation>', empty_reason="no_doc")

    structures = await _read_structures(tools_slot, query, doc_ids[:_NAV_TREE_MAX_DOCS], kinds)

    parts = [f'<structure_navigation count="{len(structures)}" query="{_xml_escape(query)}" kind="{_xml_escape(kind)}">']
    total_entities = 0
    total_ptrs = 0
    best_score = 0.0
    all_chunk_paths: dict[str, str] = {}
    for i, s in enumerate(structures):
        stats = s.get("stats") or {}
        total_entities += len(s.get("entities") or [])
        total_ptrs += int(stats.get("chunk_ptrs") or 0)
        best_score = max(best_score, float(stats.get("top_score") or 0.0))
        for cid, path in (s.get("chunk_paths") or {}).items():
            if cid and cid not in all_chunk_paths:
                all_chunk_paths[cid] = path
        parts.append(f'  <doc rank="{i + 1}" doc_id="{_xml_escape(s["doc_id"])}" doc_title="{_xml_escape(s["title"])}" entities="{len(s["entities"])}" relations="{len(s["relations"])}">')
        outline = s["outline"]
        if outline:
            parts.append(f"    <structure>{_xml_escape(outline)}</structure>")
        parts.append("  </doc>")
    parts.append("</structure_navigation>")
    return NavResult(
        text="\n".join(parts),
        doc_ids=list(structures and [s["doc_id"] for s in structures] or []),
        entities=total_entities,
        chunk_ptrs=total_ptrs,
        top_score=round(best_score, 4),
        chunk_paths=all_chunk_paths,
        empty_reason="no_structure" if total_entities == 0 else "",
    )


def _structure_kinds_for(kind: str) -> set:
    """Map a ``navigate_structure`` kind string to the compiled kinds set."""

    k = (kind or "catalog").strip().lower()
    if k in ("mindmap", "mind_map", "concept"):
        return set(_MINDMAP_KINDS)
    if k in ("graph", "kg", "entity", "ontology"):
        return {"graph", "ontology", "entity", "raptor"}
    return set(_CATALOG_KINDS)


# --- navigate_structure hierarchical drill-down (zero LLM) ---
# Caps controlling the drill-down scope.
_STRUCT_MAX_DEPTH = 3  # max TOC levels drilled.
_STRUCT_BRANCH_K = 2  # keep top-K most relevant nodes per level.
_STRUCT_RELEVANCE_MIN = 1  # a node must match at least this many query terms to descend.
_STRUCT_VEC_BEAM_RATIO = 0.5  # vector beam: drop nodes below best*this similarity.
_STRUCT_MAX_NODES = 10  # cap on nodes rendered in the outline.
_STRUCT_MAX_CHUNKS = 4  # cap on chunk snippets returned in the outline.
_STRUCT_DESC_SNIPPET = 180  # cap on a node's description snippet in the outline.
# Chunks recalled per document when a tree cannot score its own nodes and the
# chunk index has to choose instead.  Wide enough that several nodes can be
# covered, so the outline is not pinned to a single branch.
_STRUCT_RECALL_TOP_N = 24
# Snippets rendered on that path.  Higher than _STRUCT_MAX_CHUNKS because there
# the chunks *are* the answer's material — the outline is only the annotation.
_STRUCT_MAX_CHUNK_HITS = 8
# Document-in-LLM selection (PageIndex tree-search / VecTree-RAG Layer 1): the
# whole TOC is handed to the model at once and it returns every node it thinks
# is relevant.  This is deliberately NOT a top-down descent — the model sees all
# levels simultaneously, so a wrong pick cannot prune a branch, which is the
# failure mode beam descent has (SCT measures it as routing accuracy 20.2% vs
# 39.3% for flat selection).
# OFF by default.  Correct in principle — the model sees every level at once, so
# nothing is pruned — but it costs one chat call per document, and the drill rung
# calls navigate_structure once per routed document (8 docs = 8 extra calls per
# question).  Measured 2026-09-02: with it on, per-question LLM calls went 9-17
# -> 37 and the 180s research budget was exhausted on 15 of 16 questions
# (baseline: 1 of 28).  Turn it on only when the LLM has headroom; prefer
# restricting it to the top-ranked document first.
_STRUCT_TOC_LLM_SELECT = False
# Budget for the serialized TOC.  VecTree-RAG quotes 200-500 tokens per document
# for section summaries; facts are included here too, so this allows more.
# _ask_nav_select truncates past this via message_fit_in regardless.
_STRUCT_TOC_MAX_NODES = 120
_STRUCT_TOC_DESC_SNIPPET = 120
_STRUCT_TOC_MAX_DEPTH = 6
# Snippet length / per-doc cap for the related chunks appended after a
# search_chunks hit (from the document's compiled structure, not a full read).
_STRUCT_RELATED_SNIPPET_CHARS = 300
_STRUCT_RELATED_MAX_PER_DOC = 4


async def _embed_query(tools_slot, query: str):
    """Encode ``query`` into a vector via the embedding model.

    Returns ``(qvec, dim)`` where ``qvec`` is a list/np-array and ``dim`` is its
    length (used to pick the ``q_<dim>_vec`` doc-store field). Returns ``(None, 0)``
    when no embedding model is available (caller falls back to keyword scoring).
    """
    try:
        embd = getattr(tools_slot, "embed_mdl", None)
        if embd is None or not callable(getattr(embd, "encode_queries", None)):
            return None, 0
        # ``encode_queries`` is a SYNCHRONOUS method returning ``(vector, token_count)``
        # on the embedding model; do NOT await it (it is not a coroutine).
        qvec, _tok = embd.encode_queries(query)
        if qvec is None:
            return None, 0
        import numpy as np

        arr = np.asarray(qvec, dtype=float)
        if arr.ndim != 1 or arr.size == 0:
            return None, 0
        return arr, int(arr.size)
    except Exception:
        _LOG.exception("[navigate_structure] query embedding failed; falling back to keyword")
        return None, 0


async def _load_entities_with_vectors(tools_slot, doc_id: str, kinds: set, vec_field: str) -> list[dict]:
    """Load a document's compiled-structure entities WITH their embeddings.

    Like ``_load_compiled_structure`` but also reads ``vec_field`` (the ``q_<dim>_vec``
    column) for each entity row, so vector beam descent can score nodes by cosine
    similarity to the query. Returns entities with an added ``"_vec"`` key.
    """
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    # peewee MySQL lookup — call directly to reuse the pool's connection (see note).
    resolved = tools_slot._resolve_doc_tenant(doc_id)
    if not resolved:
        return []
    kb_id, tenant_id = resolved
    index_name = search.index_name(tenant_id)
    fields = ["content_with_weight", "compile_kwd", "compilation_template_kind_kwd", "knowledge_graph_kwd", "doc_id"]
    if vec_field:
        fields.append(vec_field)

    try:
        # Same dual-shape logic as _load_compiled_structure: RAPTOR/tree writes a
        # compact graph blob (knowledge_graph_kwd="graph"), while page_index /
        # pipeline-Compiler tree write per-entity rows (knowledge_graph_kwd="entity").
        # Read BOTH so vector beam descent works for either shape.
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"doc_id": [doc_id], "knowledge_graph_kwd": ["graph"]},
            [],
            OrderByExpr(),
            0,
            1000,
            index_name,
            [kb_id],
        )
        res2 = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"doc_id": [doc_id], "knowledge_graph_kwd": ["entity"]},
            [],
            OrderByExpr(),
            0,
            3000,
            index_name,
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields) or {}
        rows.update(settings.docStoreConn.get_fields(res2, fields) or {})
    except Exception:
        _LOG.exception("[navigate_structure] _load_entities_with_vectors failed for doc=%s", doc_id)
        return []

    out: list[dict] = []
    for row in rows.values():
        try:
            graph = json.loads(row.get("content_with_weight") or "{}")
        except Exception:
            continue
        if not isinstance(graph, dict):
            continue
        kind = _normalize_kind(row.get("compilation_template_kind_kwd") or row.get("compile_kwd") or "")
        if kind not in kinds:
            continue
        vec = row.get(vec_field) if vec_field else None
        # graph blob: content nests entities under "entities"; entity row: content
        # is a single entity dict.
        candidates = graph.get("entities") or [] if row.get("knowledge_graph_kwd") == "graph" else [graph]
        for e in candidates:
            if not isinstance(e, dict) or not (e.get("name") or "").strip():
                continue
            e = dict(e)
            if vec is not None:
                e["_vec"] = vec
            out.append(e)
    return out


async def _read_structures(tools_slot, query: str, doc_ids: list[str], kinds: set) -> list[dict]:
    """Read compiled structures for ``doc_ids`` — VECTOR BEAM DRILL-DOWN.

    The compiled structure (esp. tree_node / page_index TOC) is a HIERARCHY whose
    nodes already carry embeddings (``q_<dim>_vec``). We exploit that: embed the
    query once, then for each document follow the pre-built tree from its roots,
    at every level keeping the top-K nodes by COSINE similarity to the query
    (beam search) and descending to their children.

    Collect the union of ``source_chunk_ids`` over the drilled path — a SET of
    relevant chunks (not a single one) — then return short snippets plus node
    pointers, so the model follows the pointers with grep_chunks / search_chunks
    instead of receiving large chunks. Zero chat-LLM.
    """

    qvec, dim = await _embed_query(tools_slot, query)
    vec_field = f"q_{dim}_vec" if dim else ""

    out: list[dict] = []
    for doc_id in doc_ids:
        # Entities with their embeddings (for cosine scoring) + relations (for the
        # tree edges). ``kinds`` gates which compiled buckets we read.
        entities: list[dict] = []
        relations: list[dict] = []
        try:
            if vec_field:
                entities = await _load_entities_with_vectors(tools_slot, doc_id, kinds, vec_field)
            if not entities:
                structure = await _load_compiled_structure(tools_slot, doc_id, kinds)
                entities = structure.get("entities") or []
                relations = structure.get("relations") or []
            else:
                structure = await _load_compiled_structure(tools_slot, doc_id, kinds)
                relations = structure.get("relations") or []
        except Exception:
            _LOG.exception("[navigate_structure] structure load failed for doc=%s", doc_id)

        # Two selection strategies, chosen by whether the tree can score itself.
        hits: list[tuple[str, float]] = []
        selected: list[str] = []
        if entities and _has_distinct_node_vectors(entities):
            # page_index / per-node embeddings: hand the whole TOC to the model
            # at once.  It sees every level simultaneously, so nothing is pruned
            # by an early bad pick.  Empty result just means "caller's problem",
            # so fall through to the beam only when the model is unavailable.
            if _STRUCT_TOC_LLM_SELECT:
                selected = await _select_toc_nodes(tools_slot, query, entities, relations)
        else:
            # RAPTOR blob path: every node shares the blob's one embedding, so
            # beam descent would select arbitrarily.  Let the chunk index choose
            # and keep the tree for labelling where the chosen chunks sit.
            hits = await _recall_chunk_ids_in_doc(tools_slot, query, doc_id, _STRUCT_RECALL_TOP_N)

        outline, stats, chunk_paths = await _render_toc_drilldown(query, qvec, entities, relations, chunk_hits=hits or None, selected=selected or None)
        out.append(
            {
                "doc_id": doc_id,
                "title": "",
                "entities": entities,
                "relations": relations,
                "outline": outline,
                "stats": stats,
                "chunk_paths": chunk_paths,
            }
        )
    return out


def _build_toc_tree(entities: list[dict], relations: list[dict]):
    """Build parent→children map, child→parent map, and the root node names.

    Tree relations are interpreted as ``from`` = parent, ``to`` = child. Roots are
    tree nodes with no parent (isolated nodes are their own roots).
    """
    by_name: dict[str, dict] = {}
    for e in entities:
        name = (e.get("name") or "").strip()
        if name and name not in by_name:
            by_name[name] = e
    children: dict[str, list[str]] = {}
    parents: dict[str, str] = {}
    for r in relations:
        p = (r.get("from") or "").strip()
        c = (r.get("to") or "").strip()
        if not p or not c or p == c:
            continue
        children.setdefault(p, [])
        if c not in children[p]:
            children[p].append(c)
        parents[c] = p
    roots = [n for n in by_name if n not in parents]
    if not roots:
        # Every node has a parent? Use nodes with no children (isolated) as roots.
        roots = [n for n in by_name if n not in children] or list(by_name)
    return by_name, children, parents, roots


def _node_relevance(query_terms: list[str], entity: dict) -> int:
    """Keyword-overlap relevance of a TOC node against the query."""
    if not query_terms:
        return 0
    text = f"{entity.get('name') or ''} {entity.get('description') or ''}".lower()
    return sum(1 for t in query_terms if t in text)


def _collect_chunk_ids(nodes: list[dict], cap: int = 32) -> list[str]:
    """Union of source_chunk_ids across nodes, deduped and bounded."""
    seen: list[str] = []
    for n in nodes:
        for cid in n.get("source_chunk_ids") or []:
            if isinstance(cid, str) and cid and cid not in seen:
                seen.append(cid)
            if len(seen) >= cap:
                return seen
    return seen


def _cosine(a, b):
    """Cosine similarity between two arrays (skip zero vectors)."""
    import numpy as np

    try:
        aa = np.asarray(a, dtype=float).reshape(-1)
        bb = np.asarray(b, dtype=float).reshape(-1)
        if aa.size == 0 or bb.size == 0 or aa.size != bb.size:
            return 0.0
        denom = float(np.linalg.norm(aa) * np.linalg.norm(bb))
        if denom == 0.0:
            return 0.0
        return float(np.dot(aa, bb) / denom)
    except Exception:
        return 0.0


def _vecs_equal(a, b) -> bool:
    """Whether two node embeddings are the same vector."""
    try:
        if a is b:
            return True
        la, lb = list(a), list(b)
        if len(la) != len(lb):
            return False
        # Distinct embeddings diverge long before the tail; a prefix compare is
        # enough and keeps this cheap on 1024-dim vectors.
        return all(x == y for x, y in zip(la[:16], lb[:16]))
    except Exception:
        return False


def _has_distinct_node_vectors(entities: list[dict]) -> bool:
    """Whether nodes carry per-node embeddings rather than one shared vector.

    The RAPTOR/tree main path persists a single graph blob, so every node
    inherits that blob's one vector (see ``_load_entities_with_vectors``) and
    beam scoring degenerates: every node gets the same cosine, the top-K is
    whatever the row order happened to be, and the drill carries no signal.
    page_index and the pipeline Compiler write one row per entity with its own
    embedding, so their trees do discriminate.
    """
    first = None
    for e in entities:
        v = e.get("_vec")
        if v is None:
            continue
        if first is None:
            first = v
        elif not _vecs_equal(first, v):
            return True
    # Either no node has a vector, or they all share one — neither discriminates.
    return False


async def _recall_chunk_ids_in_doc(tools, query: str, doc_id: str, top_n: int) -> list[tuple[str, float]]:
    """Hybrid chunk retrieval confined to one document, as ``[(chunk_id, score)]``.

    Used when a compiled tree cannot score its own nodes: the chunk index then
    decides *which* nodes matter, and the tree is left to label where they sit.
    """
    if not query or not doc_id:
        return []
    from common import settings

    target_ids = getattr(tools, "kb_ids", None) or []
    if not target_ids:
        return []
    embd_mdl = getattr(tools, "embed_mdl", None)
    try:
        kbinfos = await settings.retriever.retrieval(
            query,
            embd_mdl,
            getattr(tools, "tenant_ids", None),
            target_ids,
            1,
            top_n,
            0.0,
            vector_similarity_weight=0.3 if embd_mdl else 0,
            doc_ids=[str(doc_id).strip()],
            highlight=False,
            # RAPTOR cluster nodes live in the chunk index (rechunk_kwd=tree, no
            # compile_kwd), so they survive this filter and stay retrievable —
            # which is the point, since they are the structure.
            must_not={"exists": "compile_kwd"},
        )
    except Exception:
        _LOG.exception("[navigate_structure] chunk recall failed for doc=%s", doc_id)
        return []
    hits: list[tuple[str, float]] = []
    for c in kbinfos.get("chunks", []):
        cid = str(c.get("chunk_id") or c.get("id") or "").strip()
        if cid:
            hits.append((cid, float(c.get("similarity") or 0.0)))
    return hits


def _build_toc_items(entities: list[dict], relations: list[dict]) -> list[dict]:
    """Serialize the whole TOC, indented by depth, for one-shot LLM selection.

    Indentation is the only structure the model gets: it reads the list the way a
    reader reads a table of contents.  Every node carries its real name through
    ``_node`` so the selection can be mapped back without string matching.

    Orphans (nodes no relation attaches to a root) are appended last at depth 0 —
    dropping them would silently hide content from the model.
    """
    by_name, children, parents, roots = _build_toc_tree(entities, relations)
    if not by_name:
        return []

    items: list[dict] = []
    seen: set[str] = set()

    def _walk(name: str, depth: int) -> None:
        if name in seen or depth > _STRUCT_TOC_MAX_DEPTH:
            return
        seen.add(name)
        e = by_name.get(name)
        if not e:
            return
        desc = str(e.get("description") or "").strip().replace("\n", " ")
        items.append(
            {
                "_node": name,
                "name": "  " * depth + name,
                "description": _snippet(desc, _STRUCT_TOC_DESC_SNIPPET),
            }
        )
        for child in children.get(name, []):
            _walk(child, depth + 1)

    for r in roots:
        _walk(r, 0)
    for name in by_name:
        if name not in seen:
            _walk(name, 0)
    return items


async def _select_toc_nodes(tools, query: str, entities: list[dict], relations: list[dict]) -> list[str]:
    """Let the model pick TOC nodes in one shot. Returns node names (''-filtered).

    Returns ``[]`` on any failure so the caller can fall back — never raises.
    """
    if not entities:
        return []
    try:
        items = _build_toc_items(entities, relations)
    except Exception:
        _LOG.exception("[navigate_structure] TOC serialization failed")
        return []
    if not items:
        return []
    try:
        picked = await _ask_nav_select(tools, query, items, "sections", _STRUCT_TOC_MAX_NODES)
    except Exception:
        # _ask_nav_select catches model failures, but serialization/other paths can
        # still raise; the caller must be able to fall through to local scoring.
        _LOG.exception("[navigate_structure] TOC model selection failed")
        return []
    names = []
    for it in picked or []:
        name = str(it.get("_node") or "").strip()
        if name:
            names.append(name)
    return names


def _nodes_covering_chunks(entities: list[dict], chunk_ids: set[str]) -> list[str]:
    """Names of nodes whose ``source_chunk_ids`` cover any of ``chunk_ids``.

    Used only to give retrieved chunks a structural home.  A chunk that no node
    covers is still returned — the tree annotates, it does not select.
    """
    if not chunk_ids:
        return []
    names: list[str] = []
    for e in entities:
        name = (e.get("name") or "").strip()
        if not name:
            continue
        ids = {str(c) for c in (e.get("source_chunk_ids") or []) if isinstance(c, str) and c}
        if ids & chunk_ids:
            names.append(name)
    return names


def _node_score(qvec, query_terms, entity: dict) -> float:
    """Score a TOC node: cosine similarity (if the node has an embedding and we
    have a query vector), else keyword-overlap. Higher is better."""
    if qvec is not None:
        v = entity.get("_vec")
        if v is not None:
            return _cosine(qvec, v)
    return float(_node_relevance(query_terms, entity))


def _drill_kept_nodes(query_terms: list[str], qvec, entities: list[dict], relations: list[dict]) -> tuple[list[dict], dict, set, float]:
    """VECTOR BEAM drill-down over the compiled TOC hierarchy.

    From the roots, at each level keep the top-K nodes by cosine similarity to the
    query (``_node_score``) and descend to their children; keyword-overlap is the
    fallback when no vectors are available. Returns
    ``(kept_nodes, parents, kept_names, best_score)`` so callers can both render
    the outline AND collect the related chunks behind the drilled nodes'
    ``source_chunk_ids``.

    ``best_score`` is the highest score reached at any level — the quality signal
    the orchestrator routes on. It was computed here and thrown away; surfacing it
    is what lets callers tell a shallow drift from a solid hit without re-scoring.
    """
    by_name, children, parents, roots = _build_toc_tree(entities, relations)
    if not roots:
        return [], {}, set(), 0.0

    frontier = list(roots)
    kept_names: set[str] = set()
    depth = 0
    best_overall = 0.0
    # ``<=`` so the deepest relevant leaf (at depth MAX_DEPTH) is also collected,
    # not just the intermediate levels.
    while frontier and depth <= _STRUCT_MAX_DEPTH:
        scored = [(_node_score(qvec, query_terms, by_name[n]), n) for n in frontier if n in by_name]
        if not scored:
            break
        scored.sort(key=lambda x: x[0], reverse=True)
        if not scored:
            break
        best = scored[0][0]
        if best > best_overall:
            best_overall = best
        # If even the best node has no relevance, stop descending.
        if qvec is None and best < _STRUCT_RELEVANCE_MIN:
            break
        # Beam: keep top-K, but drop nodes far below the best (avoids pulling in
        # low-similarity sibling sub-sections). For vector scoring use a relative
        # threshold; for keyword scoring keep nodes above the absolute minimum.
        if qvec is not None:
            top = [s for s in scored if s[0] >= best * _STRUCT_VEC_BEAM_RATIO][:_STRUCT_BRANCH_K]
        else:
            top = [s for s in scored if s[0] >= _STRUCT_RELEVANCE_MIN][:_STRUCT_BRANCH_K]
        if not top:
            break
        new_frontier: list[str] = []
        for _score, name in top:
            if name not in kept_names:
                kept_names.add(name)
            new_frontier.extend(children.get(name, []))
        frontier = new_frontier
        depth += 1

    # Include ancestor path of the kept nodes so the outline shows the TOC chain.
    for name in list(kept_names):
        cur = parents.get(name)
        guard = 0
        while cur and cur not in kept_names and guard < _STRUCT_MAX_DEPTH:
            kept_names.add(cur)
            cur = parents.get(cur)
            guard += 1

    kept_nodes: list[dict] = []
    for n in kept_names:
        e = by_name.get(n)
        if e is not None:
            kept_nodes.append(e)
    return kept_nodes, parents, kept_names, best_overall


async def _render_toc_drilldown(
    query: str,
    qvec,
    entities: list[dict],
    relations: list[dict],
    chunk_hits: list[tuple[str, float]] | None = None,
    selected: list[str] | None = None,
) -> tuple[str, dict, dict]:
    """Render a query-focused outline of one document's compiled structure.

    Three ways to choose what to show, in the order the caller tries them:

    ``selected`` — node names already chosen elsewhere (the whole-TOC LLM pass,
        see ``_select_toc_nodes``).  Every level was visible to the model at once,
        so nothing is pruned; chunks come from the chosen nodes' ``source_ids``.

    ``chunk_hits`` — ``[(chunk_id, score)]`` from chunk retrieval, used when the
        tree cannot score its own nodes (RAPTOR blob path).  Selection belongs to
        the chunk index outright: the chunks are returned **as retrieved**, not
        filtered back through the nodes, and the tree is left to do the only job
        it is still needed for — labelling where each chunk sits.  Round-tripping
        the selection through the nodes would silently drop every chunk no node
        happens to cover, which for a clustered tree is most of them.

    neither — VECTOR BEAM descent: from the roots, at each level keep the top-K
        nodes by cosine similarity to the query and descend to their children,
        falling back to keyword overlap when no vectors are available.  Kept as
        the fallback when the model pass returns nothing.

    Returns lines like:
        - Paris Demographics (tree_node): <desc> [chunks: c1,c2]
          - Immigration since 1945 (tree_node): <desc> [chunks: c3]
        - [chunk c3]: <snippet>

    Also returns a stats dict (``nodes``, ``chunk_ptrs``, ``top_score``) and a
    ``chunk_paths`` map ``{chunk_id: "root -> ... -> node"}`` — the full
    hierarchy path of the node owning each drilled chunk — so a caller can merge
    structure context onto retrieved evidence without re-deriving the tree.
    """
    query_terms = [t for t in re.findall(r"[A-Za-z0-9_]{2,}", (query or "").lower()) if len(t) >= 2]
    # Pinned nodes were chosen elsewhere (chunk retrieval), so they stand even
    # when the query yields no usable terms — bailing out here would discard a
    # perfectly good selection just because the query is short.
    if not chunk_hits and not selected and not query_terms and qvec is None:
        return _render_outline(entities[:_STRUCT_MAX_NODES], relations[:_STRUCT_MAX_NODES]), _outline_stats(entities), {}

    by_name, _children, parents, _roots = _build_toc_tree(entities, relations)

    def _with_ancestors(names: set[str]) -> set[str]:
        """Pull in ancestors so a path renders as root -> ... -> node."""
        for name in list(names):
            cur = parents.get(name)
            guard = 0
            while cur and cur not in names and guard < _STRUCT_MAX_DEPTH:
                names.add(cur)
                cur = parents.get(cur)
                guard += 1
        return names

    selected_ids: list[str] = []
    if selected:
        kept_names = _with_ancestors({n for n in selected if n in by_name})
        kept_nodes = [by_name[n] for n in kept_names if n in by_name]
        # The model picked nodes, so the chunks are theirs to supply.
        # No score: the model chooses, it does not rank (see stats["selector"]).
        best = 0.0
        selector = "llm_toc"
    elif chunk_hits:
        # Keep the retrieval's own ordering — it is the relevance order.
        seen: set[str] = set()
        for cid, _score in chunk_hits:
            if cid and cid not in seen:
                seen.add(cid)
                selected_ids.append(cid)
        kept_names = _with_ancestors({n for n in _nodes_covering_chunks(entities, set(selected_ids)) if n in by_name})
        kept_nodes = [by_name[n] for n in kept_names if n in by_name]
        best = max((float(s) for _c, s in chunk_hits), default=0.0)
        selector = "chunk_retrieval"
    else:
        kept_nodes, parents, kept_names, best = _drill_kept_nodes(query_terms, qvec, entities, relations)
        selector = "beam"
    if not kept_nodes and not selected_ids:
        return _render_outline(entities[:_STRUCT_MAX_NODES], relations[:_STRUCT_MAX_NODES]), _outline_stats(entities), {}

    # Render the drilled nodes (indented by depth via ancestor count).
    depth_of: dict[str, int] = {}
    for n in kept_names:
        d = 0
        cur = parents.get(n)
        while cur and cur in kept_names:
            d += 1
            cur = parents.get(cur)
        depth_of[n] = d

    # root -> ... -> node path for every drilled node, mapped onto its chunks so
    # the caller can annotate retrieved evidence with where it sits structurally.
    chunk_paths: dict[str, str] = {}
    for e in kept_nodes:
        name = (e.get("name") or "").strip()
        if not name:
            continue
        segs = [name]
        cur = parents.get(name)
        guard = 0
        while cur and guard < _STRUCT_MAX_DEPTH:
            segs.append(cur)
            cur = parents.get(cur)
            guard += 1
        path = " -> ".join(reversed(segs))
        for cid in e.get("source_chunk_ids") or []:
            if isinstance(cid, str) and cid and cid not in chunk_paths:
                chunk_paths[cid] = path

    lines: list[str] = []
    for e in kept_nodes[:_STRUCT_MAX_NODES]:
        name = (e.get("name") or "").strip()
        etype = (e.get("type") or "other").strip()
        desc = (e.get("description") or "").strip()
        chunks = _chunk_ptrs(e)
        indent = "  " * depth_of.get(name, 0)
        line = f"{indent}- {name} ({etype})"
        if desc:
            line += f": {_snippet(desc, _STRUCT_DESC_SNIPPET)}"
        if chunks:
            line += f" [chunks: {chunks}]"
        lines.append(line)

    # Chunk snippets.  When chunk retrieval did the selecting, those hits are
    # returned as-is: filtering them back through the nodes would drop every
    # chunk no node happens to cover.  Otherwise take the chunks behind the
    # drilled nodes.
    wanted = selected_ids or _collect_chunk_ids(kept_nodes)
    if wanted:
        chunks = await _load_chunks_for_ids(_tools_slot(), wanted)
        if chunks:
            if selected_ids:
                # Retrieval order is the relevance order; re-ranking by terms
                # would discard the score that selected them.
                ranked = sorted(chunks, key=lambda c: selected_ids.index(_chunk_id(c)))
                limit = _STRUCT_MAX_CHUNK_HITS
            else:
                ranked = _rank_chunks_by_terms(chunks, [query])
                limit = _STRUCT_MAX_CHUNKS
            for c in ranked[:limit]:
                cid = _chunk_id(c)
                text = _chunk_text(c).strip()
                lines.append(f"- [chunk {cid}]: {_snippet(text, 300)}")
    stats = {
        "nodes": len(kept_nodes),
        "chunk_ptrs": len(wanted),
        "top_score": round(float(best), 4),
        # Which strategy chose the nodes.  top_score is only meaningful for
        # "beam" and "chunk_retrieval"; llm_toc has no comparable score.
        "selector": selector,
    }
    return "\n".join(lines), stats, chunk_paths


def _outline_stats(entities: list[dict]) -> dict:
    """Stats for the flat fallback outline: no drill happened, so the beam score
    is unknown (0.0) and only the entity/chunk counts are meaningful."""
    ptrs = 0
    for e in entities[:_STRUCT_MAX_NODES]:
        ptrs += len(_collect_chunk_ids([e]))
    return {"nodes": len(entities[:_STRUCT_MAX_NODES]), "chunk_ptrs": ptrs, "top_score": 0.0}


def _render_outline(entities: list[dict], relations: list[dict]) -> str:
    """Render a compact flat outline (fallback when no query terms are usable)."""
    lines: list[str] = []
    cap_e, cap_r = 40, 40
    for e in entities[:cap_e]:
        name = (e.get("name") or "").strip()
        if not name:
            continue
        etype = (e.get("type") or "other").strip()
        desc = (e.get("description") or "").strip()
        chunks = _chunk_ptrs(e)
        line = f"- {name} ({etype})"
        if desc:
            line += f": {_snippet(desc, _STRUCT_DESC_SNIPPET)}"
        if chunks:
            line += f" [chunks: {chunks}]"
        lines.append(line)
    for r in relations[:cap_r]:
        frm = (r.get("from") or "").strip()
        to = (r.get("to") or "").strip()
        if not frm or not to:
            continue
        chunks = _chunk_ptrs(r)
        line = f"- {frm} -[{r.get('type') or 'related_to'}]-> {to}"
        if chunks:
            line += f" [chunks: {chunks}]"
        lines.append(line)
    return "\n".join(lines)


def _chunk_ptrs(item: dict) -> str:
    """Comma-join the source_chunk_ids of a structure entity/relation."""
    cids = [c for c in (item.get("source_chunk_ids") or []) if isinstance(c, str) and c]
    if not cids:
        return ""
    # Dedup + bound to avoid a huge pointer list.
    seen: list[str] = []
    for c in cids:
        if c not in seen:
            seen.append(c)
        if len(seen) >= 8:
            break
    return ",".join(seen)


async def _load_chunks_for_ids(tools_slot, chunk_ids: list[str]) -> list[dict]:
    """Load chunks by id (across any owning document). Zero LLM.

    Unlike ``_load_specific_chunks`` (which requires a doc_scope), this scans the
    bound documents' chunks for the requested ids — used to fetch the chunks behind
    the relevant entities of a navigate_structure outline.
    """
    if not chunk_ids:
        return []
    wanted = {str(c).strip() for c in chunk_ids if str(c).strip()}
    if not wanted:
        return []
    # Resolve the owning document(s) by scanning the conversation's bound docs is
    # not available here, so fall back to a per-kb chunk query by id via the
    # retriever (zero LLM). If unavailable, return empty (model uses pointers).
    try:
        from common import settings
        from rag.nlp import search as _rag_search

        fields = ["content_with_weight", "docnm_kwd", "doc_id"]
        kb_ids = _get_kb_ids(tools_slot) or []
        found: dict[str, dict] = {}
        tenant_ids = getattr(tools_slot, "tenant_ids", None) or []
        if not tenant_ids:
            tid = _first_tenant_id(tools_slot)
            if tid:
                tenant_ids = [tid]
        from common.misc_utils import thread_pool_exec

        for tid in tenant_ids[:4]:
            for kb_id in kb_ids[:_NAV_TREE_MAX_DATASETS]:
                index = _rag_search.index_name(tid)
                try:
                    res = await thread_pool_exec(
                        settings.docStoreConn.search,
                        fields,
                        [],
                        {"id": list(wanted)[:128]},
                        [],
                        None,
                        0,
                        128,
                        index,
                        [kb_id],
                    )
                    rows = settings.docStoreConn.get_fields(res, fields) or {}
                except Exception:
                    continue
                for cid, row in rows.items():
                    if cid in wanted:
                        found[cid] = {
                            "chunk_id": cid,
                            "content_with_weight": row.get("content_with_weight") or "",
                            "docnm_kwd": row.get("docnm_kwd") or "",
                            "doc_id": row.get("doc_id") or "",
                        }
        return [found[c] for c in chunk_ids if c in found]
    except Exception:
        _LOG.exception("[navigate_structure] _load_chunks_for_ids failed")
        return []


async def _expand_related_via_structure(
    tools_slot,
    query: str,
    doc_ids: list[str],
    exclude: set[str],
    max_per_doc: int = _STRUCT_RELATED_MAX_PER_DOC,
) -> list[dict]:
    """After ``search_chunks`` hits a document, read its compiled structure and
    return OTHER chunks related to the query (behind the beam-drilled entities).

    Uses the same VECTOR BEAM drill-down as ``navigate_structure`` (zero chat-LLM):
    embed the query, descend the compiled tree/catalog hierarchy by cosine
    similarity, collect the drilled entities' ``source_chunk_ids``, load those
    chunks, and reduce each to a short snippet. Chunks already returned by the
    search (``exclude``) are skipped so only genuinely NEW related chunks appear.

    Returns a list of chunk dicts with ``content_with_weight`` set to the snippet,
    ordered most-related-first. ``[]`` when the document has no compiled structure
    with embeddings, nothing is relevant, or nothing is left after ``exclude``.
    """
    if not doc_ids or not query or not str(query).strip():
        return []
    from common import settings

    if not getattr(settings, "retriever", None):
        return []

    qvec, dim = await _embed_query(tools_slot, query)
    vec_field = f"q_{dim}_vec" if dim else ""
    query_terms = [t for t in re.findall(r"[A-Za-z0-9_]{2,}", (query or "").lower()) if len(t) >= 2]

    out: list[dict] = []
    for doc_id in doc_ids[:_NAV_TREE_MAX_DOCS]:
        try:
            entities: list[dict] = []
            relations: list[dict] = []
            if vec_field:
                entities = await _load_entities_with_vectors(tools_slot, doc_id, _CATALOG_KINDS, vec_field)
            if not entities:
                continue  # no compiled tree/catalog with embeddings -> nothing to drill
            structure = await _load_compiled_structure(tools_slot, doc_id, _CATALOG_KINDS)
            relations = structure.get("relations") or []

            kept_nodes, _parents, _kept = _drill_kept_nodes(query_terms, qvec, entities, relations)
            if not kept_nodes:
                continue
            wanted = _collect_chunk_ids(kept_nodes)
            wanted = [c for c in wanted if c not in exclude]
            if not wanted:
                continue
            chunks = await _load_chunks_for_ids(tools_slot, wanted)
            if not chunks:
                continue
            ranked = _rank_chunks_by_terms(chunks, [query])
            for c in ranked[:max_per_doc]:
                cid = _chunk_id(c)
                if cid in exclude:
                    continue
                exclude.add(cid)
                c["content_with_weight"] = _snippet(_chunk_text(c), _STRUCT_RELATED_SNIPPET_CHARS)
                c["related_via_structure"] = True
                out.append(c)
        except Exception:
            _LOG.exception("[search_chunks] compiled-structure related-chunk expansion failed for doc=%s", doc_id)
    return out
