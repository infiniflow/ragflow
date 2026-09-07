"""Compiled-product expansion for hybrid search (zero-LLM).

When ``hybrid_search`` runs with ``use_compiled=True`` this module layers the
dataset's *compiled* products on top of the retrieved chunks: per-kind compiled
structure rows (page_index / timeline / mind_map / knowledge_graph / tree), the
synthesised pages (wiki / artifact / essence), and wiki page drill-downs.

Datasets with no compiled products are unaffected — every strategy short-circuits
when its rows are absent.

Lives apart from ``search`` because it is a self-contained retrieval strategy
with its own row shapes, not part of the core hybrid/vector/BM25 legs.
"""

import logging

# ``_kg_scopes`` resolves the (kb_id, tenant_id) pairs to scan; it lives in
# ``navigation`` because that module owns the compiled-row lookup helpers.
# Everything else (settings / thread pool / OrderByExpr) is imported lazily
# inside each strategy, matching the original module's style.
from rag.advanced_rag.harness.tools.navigation import _kg_scopes

_LOG = logging.getLogger(__name__)

# HNSW ef_search floor.  Must be >= the requested top_n or the ANN search is
# bounded by the candidate list instead of by relevance.
_VECTOR_NUM_CANDIDATES = 256
_VECTOR_SIMILARITY = 0.1


async def _dense_expr(text: str, embd_mdl, top_n: int):
    """Build the dense match expression for compiled-row search.

    ``get_vector``'s 4th positional argument is ``num_candidates`` (HNSW
    ef_search), not the similarity threshold — passing the 0.1 threshold
    positionally collapses the candidate list to a fraction of one entry and
    silently destroys recall.  Both are passed by name here.
    """
    from common import settings

    return await settings.retriever.get_vector(
        text,
        embd_mdl,
        top_k=top_n,
        num_candidates=max(top_n, _VECTOR_NUM_CANDIDATES),
        similarity=_VECTOR_SIMILARITY,
    )


# ─── Compiled product expansion (zero-LLM, used by hybrid_search with use_compiled=True) ───


async def _expand_with_compiled(tools, query: str, keywords: str, kbinfos: dict, doc_scope: list[str] | None = None) -> None:
    """Zero-LLM compiled-product expansion: page_index → tree → KG.

    For each bound KB, searches compiled entity rows matching the query,
    hops 1-hop via relations to find neighbour entities, then appends
    their source passages to ``kbinfos["chunks"]``.
    """
    before = len(kbinfos.get("chunks", []))
    seen_ids = {c.get("chunk_id") or c.get("id") for c in kbinfos.get("chunks", [])}

    scopes = await _kg_scopes(tools, doc_scope)
    if not scopes:
        return

    for kb_id, tenant_id, doc_ids in scopes:
        # 1-hop entity-graph expansion per template kind.
        # Each template writes entity/relation rows tagged with
        # ``compilation_template_kind_kwd`` — search them independently.
        for label, template_kind in (
            ("knowledge_graph", "knowledge_graph"),
            ("mind_map", "mind_map"),
            ("timeline", "timeline"),
            ("page_index", "page_index"),
        ):
            chunks = await _expand_compiled_strategy(
                tools,
                kb_id,
                tenant_id,
                doc_ids,
                query,
                seen_ids,
                template_kind=template_kind,
                max_chunks=5,
            )
            if chunks:
                kbinfos.setdefault("chunks", []).extend(chunks)
                _LOG.debug("[Compiled expand] %s: +%d chunks", label, len(chunks))

        # Tree structure graph (uses ``compile_kwd``, not template kind).
        chunks = await _expand_compiled_strategy(
            tools,
            kb_id,
            tenant_id,
            doc_ids,
            query,
            seen_ids,
            compile_kwd="tree",
            max_chunks=5,
        )
        if chunks:
            kbinfos.setdefault("chunks", []).extend(chunks)
            _LOG.debug("[Compiled expand] tree: +%d chunks", len(chunks))

        # Synthesis pages — standalone rendered articles from wiki / session
        # graph / session essence templates.  Searched directly (no entity-graph nav).
        for label, ckwd in (
            ("wiki_page", "wiki_page"),
            ("artifact_page", "artifact_page"),
            ("essence", "essence"),
        ):
            chunks = await _expand_wiki_page_strategy(
                tools,
                kb_id,
                tenant_id,
                doc_ids,
                query,
                seen_ids,
                compile_kwd=ckwd,
                max_chunks=5,
            )
            if chunks:
                kbinfos.setdefault("chunks", []).extend(chunks)
                _LOG.debug("[Compiled expand] %s: +%d chunks", label, len(chunks))

    # Re-sort so compiled-expansion chunks blend by similarity with regular ones.
    chunks = kbinfos.get("chunks", [])
    if chunks:
        chunks.sort(key=lambda c: c.get("similarity", 0.0), reverse=True)

    after = len(chunks)
    _LOG.info("[Hybrid search] Compiled expansion added %d chunks.", after - before)


async def _search_compiled_rows(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    kind: str,
    *,
    text: str = "",
    top_n: int = 8,
    extra: dict | None = None,
    compile_kwd: str | None = None,
    template_kind: str | None = None,
) -> dict:
    """Search compiled KG rows in one KB, returning raw field maps.

    *compile_kwd* filters by the ``compile_kwd`` field (e.g. "tree" for tree
    structure nodes).  *template_kind* filters by ``compilation_template_kind_kwd``
    (e.g. "knowledge_graph", "mind_map").  Leave both ``None`` to scan all rows.
    """
    from common import settings
    from common.doc_store.doc_store_base import MatchTextExpr, OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    condition: dict = {"knowledge_graph_kwd": [kind]}
    if compile_kwd:
        condition["compile_kwd"] = compile_kwd
    if template_kind:
        condition["compilation_template_kind_kwd"] = template_kind
    if doc_ids:
        condition["doc_id"] = list(doc_ids)
    if extra:
        condition.update(extra)

    fields = [
        "content_with_weight",
        "source_chunk_ids",
        "doc_id",
        "docnm_kwd",
        "from_entity_kwd",
        "to_entity_kwd",
        "name_kwd",
    ]
    exprs = []
    if text:
        embd_mdl = getattr(tools, "embed_mdl", None)
        if embd_mdl:
            try:
                exprs.append(await _dense_expr(text, embd_mdl, top_n))
            except Exception:
                _LOG.exception("[Compiled expand] vector build failed; using keyword match")
        if not exprs:
            exprs.append(
                MatchTextExpr(
                    ["content_ltks", "content_sm_ltks"],
                    text,
                    top_n,
                )
            )

    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            condition,
            exprs,
            OrderByExpr(),
            0,
            top_n,
            search.index_name(tenant_id),
            [kb_id],
        )
        return settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        _LOG.exception("[Compiled expand] search failed (kind=%s compile_kwd=%s)", kind, compile_kwd)
        return {}


async def _load_chunks_for_doc(tools, doc_id: str, chunk_ids: list[str]) -> list[dict]:
    """Load chunks by their IDs from the doc store."""
    if not chunk_ids:
        return []
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    resolved = tools._resolve_doc_tenant(doc_id)
    if not resolved:
        return []
    kb_id, tenant_id = resolved

    fields = ["content_with_weight", "docnm_kwd", "doc_id", "id"]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            {"id": list(chunk_ids)},
            [],
            OrderByExpr(),
            0,
            len(chunk_ids),
            search.index_name(tenant_id),
            [kb_id],
        )
        rows = settings.docStoreConn.get_fields(res, fields)
        if not rows:
            return []
        return [{**v, "chunk_id": k} for k, v in rows.items()]
    except Exception:
        _LOG.exception("[Compiled expand] failed to load chunks for doc_id=%s", doc_id)
        return []


async def _expand_compiled_strategy(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    query: str,
    seen_ids: set[str],
    *,
    compile_kwd: str | None = None,
    template_kind: str | None = None,
    max_chunks: int = 5,
) -> list[dict]:
    """Generic 1-hop compiled expansion: entity search → relation nav → chunk load.

    1. Embedding-match seed entities (filtered by *compile_kwd* or *template_kind*).
    2. Fetch relations adjacent to seed entities (forward + backward).
    3. Collect neighbour entity names (1-hop away).
    4. Look up neighbour entities to get ``source_chunk_ids``.
    5. Load actual chunks, deduplicate, respect *max_chunks*.

    *compile_kwd* is used for structure graphs (e.g. "tree").
    *template_kind* is used for entity extraction rows (e.g. "knowledge_graph").
    """
    import json

    # -- 1. Seed entities --
    seed_rows = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "entity",
        text=query,
        top_n=5,
        compile_kwd=compile_kwd,
        template_kind=template_kind,
    )
    if not seed_rows:
        return []

    seed_names: set[str] = set()
    for r in seed_rows.values():
        try:
            payload = json.loads(r.get("content_with_weight") or "{}")
        except Exception:
            continue
        name = (payload.get("name") or payload.get("title") or "").strip()
        if name:
            seed_names.add(name)
    if not seed_names:
        return []

    # -- 2. Adjacent relations (outgoing + incoming) --
    # Provide both original and lowercased names — dataset_structure_merger
    # lowercases merged-row endpoints while per-doc rows keep original case.
    seed_list = sorted({n.lower() for n in seed_names} | seed_names)
    fwd = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "relation",
        top_n=50,
        compile_kwd=compile_kwd,
        template_kind=template_kind,
        extra={"from_entity_kwd": seed_list},
    )
    bwd = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "relation",
        top_n=50,
        compile_kwd=compile_kwd,
        template_kind=template_kind,
        extra={"to_entity_kwd": seed_list},
    )
    all_rels = {**fwd, **bwd}

    # -- 3. Neighbour names (1-hop, exclude seeds) --
    seed_lower = {n.lower() for n in seed_names}
    neighbour_names: set[str] = set()
    for r in all_rels.values():
        frm = (r.get("from_entity_kwd") or "").strip()
        frm_lower = frm.lower()
        to = (r.get("to_entity_kwd") or "").strip()
        to_lower = to.lower()
        if frm_lower in seed_lower and to and to_lower not in seed_lower:
            neighbour_names.add(to)
        if to_lower in seed_lower and frm and frm_lower not in seed_lower:
            neighbour_names.add(frm)
    if not neighbour_names:
        return []

    # -- 4. Neighbour entity source_chunk_ids --
    # Provide both original and lowercased — same as seed_list above.
    neigh_list = sorted({n.lower() for n in neighbour_names} | neighbour_names)
    if len(neigh_list) > 100:
        neigh_list = neigh_list[:100]  # reasonable cap for name_kwd search
    neigh_rows = await _search_compiled_rows(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        "entity",
        top_n=len(neigh_list),
        compile_kwd=compile_kwd,
        template_kind=template_kind,
        extra={"name_kwd": neigh_list},
    )

    # Group chunk IDs by doc
    by_doc: dict[str, set[str]] = {}
    for r in neigh_rows.values():
        doc_id = r.get("doc_id") or ""
        for cid in r.get("source_chunk_ids") or []:
            if cid and cid not in seen_ids:
                by_doc.setdefault(doc_id, set()).add(cid)

    # -- 5. Load and return --
    new_chunks: list[dict] = []
    for doc_id, cids in by_doc.items():
        if len(new_chunks) >= max_chunks:
            break
        limit = max_chunks - len(new_chunks)
        chunks = await _load_chunks_for_doc(tools, doc_id, list(cids)[:limit])
        for c in chunks:
            cid = c.get("chunk_id") or c.get("id")
            if cid and cid not in seen_ids:
                seen_ids.add(cid)
                new_chunks.append(c)

    return new_chunks


async def _search_synthesis_pages(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    text: str,
    *,
    compile_kwd: str = "wiki_page",
    top_n: int = 8,
) -> dict:
    """Search synthesis-compiled page rows (no knowledge_graph_kwd filter).

    Synthesis pages are standalone articles (wiki_page, artifact_page,
    essence, etc.) with ``content_with_weight``, keyword index, and
    vector.  They do NOT carry the ``knowledge_graph_kwd`` field (unlike
    entity/relation rows from extraction).
    """
    from common import settings
    from common.doc_store.doc_store_base import MatchTextExpr, OrderByExpr
    from common.misc_utils import thread_pool_exec
    from rag.nlp import search

    condition: dict = {"compile_kwd": compile_kwd, "available_int": 1}
    if doc_ids:
        condition["source_doc_ids"] = list(doc_ids)

    fields = [
        "content_with_weight",
        "summary_with_weight",
        "source_chunk_ids",
        "doc_id",
        "title_kwd",
        "topic_kwd",
    ]

    exprs = []
    if text:
        embd_mdl = getattr(tools, "embed_mdl", None)
        if embd_mdl:
            try:
                exprs.append(await _dense_expr(text, embd_mdl, top_n))
            except Exception:
                _LOG.exception("[Wiki expand] vector build failed; using keyword match")
        if not exprs:
            exprs.append(
                MatchTextExpr(
                    ["content_ltks", "content_sm_ltks"],
                    text,
                    top_n,
                )
            )

    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            fields,
            [],
            condition,
            exprs,
            OrderByExpr(),
            0,
            top_n,
            search.index_name(tenant_id),
            [kb_id],
        )
        return settings.docStoreConn.get_fields(res, fields) or {}
    except Exception:
        _LOG.exception("[Wiki expand] search failed for kb=%s", kb_id)
        return {}


async def _expand_wiki_page_strategy(
    tools,
    kb_id: str,
    tenant_id: str,
    doc_ids: list[str] | None,
    query: str,
    seen_ids: set[str],
    *,
    compile_kwd: str = "wiki_page",
    max_chunks: int = 5,
) -> list[dict]:
    """Expand synthesis-compiled pages: semantic search → load source chunks.

    Unlike ``_expand_compiled_strategy`` (which does 1-hop entity-graph
    navigation), synthesis pages are standalone rendered articles — we search
    them directly and load the referenced source chunks as context.

    *compile_kwd* selects which synthesis type: "wiki_page" (Wiki template),
    "artifact_page" (Session Graph synthesis), "essence" (Session Essence).
    """
    # -- 1. Search synthesis pages --
    wiki_rows = await _search_synthesis_pages(
        tools,
        kb_id,
        tenant_id,
        doc_ids,
        query,
        compile_kwd=compile_kwd,
        top_n=5,
    )
    if not wiki_rows:
        return []

    # -- 2. Collect source_chunk_ids from matching pages --
    by_doc: dict[str, set[str]] = {}
    for r in wiki_rows.values():
        doc_id = r.get("doc_id") or ""
        for cid in r.get("source_chunk_ids") or []:
            if cid and cid not in seen_ids:
                by_doc.setdefault(doc_id, set()).add(cid)

    # -- 3. Load chunks, assign high similarity for priority ranking --
    new_chunks: list[dict] = []
    for doc_id, cids in by_doc.items():
        if len(new_chunks) >= max_chunks:
            break
        limit = max_chunks - len(new_chunks)
        chunks = await _load_chunks_for_doc(tools, doc_id, list(cids)[:limit])
        for c in chunks:
            cid = c.get("chunk_id") or c.get("id")
            if cid and cid not in seen_ids:
                seen_ids.add(cid)
                c.setdefault("similarity", 0.9)  # wiki pages rank high
                new_chunks.append(c)

    return new_chunks
