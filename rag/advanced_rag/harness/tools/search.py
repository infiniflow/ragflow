"""Search tools: hybrid, vector, BM25, web, structured."""

import logging
import re
import hashlib
from common import settings
from .navigation import _kg_scopes

_LOG = logging.getLogger(__name__)


# Sentence terminators: Chinese 。！？；, English ! ? ;, newline, and a
# digit-guarded English period (so "3.14" / "v1.2" don't split).
_SENT_END = re.compile(r"[。！？；!?;]+|(?<!\d)\.(?!\d)")

# Table blocks are kept ATOMIC — never split by sentence terminators — so a
# whole table counts as one "sentence" for keyword matching / narrowing.
_HTML_TABLE = re.compile(r"<table\b[^>]*>.*?</table>", re.IGNORECASE | re.DOTALL)
# Markdown table: a header row with a pipe, a separator row of dashes/colons/
# pipes, then zero+ body rows with a pipe.
_MD_TABLE = re.compile(
    r"^[ \t]*\|?[^\n]*\|[^\n]*\r?\n"
    r"[ \t]*\|?[ \t]*:?-{1,}:?[ \t]*(?:\|[ \t]*:?-{1,}:?[ \t]*)+\|?[ \t]*\r?\n"
    r"(?:[ \t]*\|?[^\n]*\|[^\n]*\r?\n?)*",
    re.MULTILINE,
)


def _protected_spans(text: str) -> list[tuple[int, int]]:
    """Non-overlapping ``(start, end)`` spans of table blocks, in order."""
    spans = [(m.start(), m.end()) for m in _HTML_TABLE.finditer(text)]
    spans += [(m.start(), m.end()) for m in _MD_TABLE.finditer(text)]
    spans.sort()
    merged: list[tuple[int, int]] = []
    last_end = -1
    for s, e in spans:
        if s < last_end:  # overlaps an already-kept span -> skip
            continue
        merged.append((s, e))
        last_end = e
    return merged


def _split_plain(text: str) -> list[str]:
    """Terminator-based sentence split, keeping each terminator attached."""
    sents: list[str] = []
    start = 0
    for m in _SENT_END.finditer(text):
        end = m.end()
        seg = text[start:end]
        if seg.strip():
            sents.append(seg)
        start = end
    if start < len(text):
        tail = text[start:]
        if tail.strip():
            sents.append(tail)
    return sents


def _split_sentences(text: str) -> list[str]:
    """Split ``text`` into sentences, keeping each terminator attached.

    Table blocks — HTML ``<table>...</table>`` and markdown tables — are treated
    as a single atomic sentence and are never split internally.
    """
    if not text:
        return []
    spans = _protected_spans(text)
    if not spans:
        return _split_plain(text)

    sents: list[str] = []
    pos = 0
    for s, e in spans:
        if s > pos:
            sents.extend(_split_plain(text[pos:s]))
        block = text[s:e]
        if block.strip():
            sents.append(block)
        pos = e
    if pos < len(text):
        sents.extend(_split_plain(text[pos:]))
    return sents


def _narrow_content(content: str, kwds: list[str]) -> str | None:
    """Return ``content`` narrowed to keyword sentences +/- 1 neighbour.

    Returns ``None`` when no keyword occurs anywhere in ``content``.
    """
    sents = _split_sentences(content)
    if not sents:
        return None
    keep: set[int] = set()
    matched = False
    for i, s in enumerate(sents):
        low = s.lower()
        if any(kw in low for kw in kwds):
            matched = True
            if i > 0:
                keep.add(i - 1)
            keep.add(i)
            if i + 1 < len(sents):
                keep.add(i + 1)
    if not matched:
        return None
    narrowed = "".join(sents[i] for i in sorted(keep)).strip()
    return "..." + _highlight_keywords(narrowed, kwds) + "..."


def _highlight_keywords(text: str, kwds: list[str]) -> str:
    terms = sorted({kw for kw in kwds if kw}, key=len, reverse=True)
    if not terms:
        return text
    pattern = re.compile("|".join(re.escape(term) for term in terms), re.IGNORECASE)
    return pattern.sub(lambda m: f"<em>{m.group(0)}</em>", text)


def _narrow_by_keywords(chunks: list[dict], keywords: str) -> list[dict]:
    """Narrow each chunk to its keyword-bearing sentences (+/- 1 neighbour) and
    drop keyword-less chunks.

    Keywords are the comma-separated terms (with close synonyms) produced by
    ``formalize``; matching is case-insensitive substring.
    """
    kwds = [k.strip().lower() for k in (keywords or "").split(",") if k.strip()]
    if not kwds or not chunks:
        return chunks
    if len(kwds) < 3:
        kwds = [k.strip().lower() for k in (keywords or "").split(" ") if k.strip()]
        _kwds = []
        for i in range(len(kwds) - 1):
            _kwds.append(kwds[i] + " " + kwds[i + 1])
        kwds = _kwds

    scored = [(ck, _narrow_content(ck.get("content_with_weight") or ck.get("content") or "", kwds)) for ck in chunks]
    out: list[dict] = []
    dedup: set[str] = set()
    for ck, nc in scored:
        if nc is not None:
            nc_hash = hashlib.md5(nc.encode("utf-8")).hexdigest()
            if nc_hash in dedup:
                continue
            dedup.add(nc_hash)
            ck["content_with_weight"] = nc
            if "content" in ck:
                ck["content"] = nc
            ck.pop("highlight", None)
            out.append(ck)
    return out


def _search_cache_key(effective_query: str, target_ids, top_n: int, doc_scope) -> tuple:
    """Key a retrieval by what actually determines its result.

    Includes the scope/limits so semantically different searches are never
    collapsed together — only a genuinely identical query is served from cache.
    """
    return (
        " ".join((effective_query or "").split()).lower(),
        tuple(sorted(target_ids or ())),
        int(top_n),
        tuple(sorted(doc_scope or ())),
    )


def _normalize(kbinfos: dict, tenant_ids: list[str] | str | None) -> dict:
    if not kbinfos:
        return {"chunks": [], "doc_aggs": []}
    if not tenant_ids:
        _LOG.warning("search: skip child retrieval because tenant_ids is empty")
        return kbinfos
    if isinstance(tenant_ids, str):
        tenant_ids = [tenant_ids]
    kbinfos["chunks"] = settings.retriever.retrieval_by_children(
        kbinfos.get("chunks", []),
        tenant_ids,
    )
    return kbinfos


async def hybrid_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 12, doc_scope: list[str] | None = None, keywords: str = "", use_compiled: bool = False) -> dict:
    if not tools.kb_ids and not kb_ids:
        return {"chunks": [], "doc_aggs": []}
    target_ids = kb_ids or tools.kb_ids
    _LOG.info(f'[Hybrid search] Searching the knowledge base for "{query}" (keywords: {keywords})')

    # Query expansion: append the formalized-question keywords + close synonyms
    # so hybrid/BM25 retrieval gets extra recall signal.
    effective_query = f"{query} {keywords}".strip() if keywords else query

    # Per-request dedup: an identical query+scope is retrieved at most once, so
    # e.g. pre_search and a claim search asking the same question don't repeat
    # the ES round-trip, child fetch and narrowing.
    cache = getattr(tools, "search_cache", None)
    cache_key = _search_cache_key(effective_query, target_ids, top_n, doc_scope)
    if cache is not None and cache_key in cache:
        cached = cache[cache_key]
        _LOG.info(f"[Hybrid search] Already searched this — reusing the {len(cached.get('chunks', []))} passage(s) found earlier.")
        return cached

    embd_mdl = tools.embed_mdl
    vector_weight = 0.3 if embd_mdl else 0

    kbinfos = await settings.retriever.retrieval(
        effective_query,
        embd_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.2,
        vector_similarity_weight=vector_weight,
        aggs=True,
        highlight=False,
        doc_ids=doc_scope,
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    if keywords:
        length = len(kbinfos["chunks"])
        kbinfos["chunks"] = _narrow_by_keywords(kbinfos.get("chunks", []), keywords)
        _LOG.info(f"[Hybrid search] Kept {len(kbinfos['chunks'])} of {length} passage(s) that actually mention the keywords.")
    if use_compiled and kbinfos.get("chunks"):
        _LOG.info("[Hybrid search] Compiled expansion enabled — enriching with page_index/tree/KG navigation.")
        await _expand_with_compiled(tools, query, keywords, kbinfos)
    if cache is not None:
        cache[cache_key] = kbinfos
    return kbinfos


async def vector_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 12, keywords: str = "") -> dict:
    if not tools.embed_mdl:
        _LOG.warning("vector_search: no embed_mdl available")
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f'[Vector search] Searching by meaning for "{query}" (keywords: {keywords})')
    effective_query = f"{query} {keywords}".strip() if keywords else query
    target_ids = kb_ids or tools.kb_ids
    kbinfos = await settings.retriever.retrieval(
        effective_query,
        tools.embed_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.2,
        vector_similarity_weight=1.0,
        aggs=False,
        highlight=False,
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    if keywords:
        length = len(kbinfos["chunks"])
        kbinfos["chunks"] = _narrow_by_keywords(kbinfos.get("chunks", []), keywords)
        _LOG.info(f"[Vector search] Kept {len(kbinfos['chunks'])} of {length} passage(s) that actually mention the keywords.")
    return kbinfos


async def bm25_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 12, keywords: str = "") -> dict:
    _LOG.info(f'[BM25 search] Searching by keyword for "{query}" (keywords: {keywords})')
    target_ids = kb_ids or tools.kb_ids
    effective_query = f"{query} {keywords}".strip() if keywords else query
    kbinfos = await settings.retriever.retrieval(
        effective_query,
        None,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.0,
        vector_similarity_weight=0,
        aggs=False,
        highlight=False,
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    if keywords:
        length = len(kbinfos["chunks"])
        kbinfos["chunks"] = _narrow_by_keywords(kbinfos.get("chunks", []), keywords)
        _LOG.info(f"[BM25 search] Kept {len(kbinfos['chunks'])} of {length} passage(s) that actually mention the keywords.")
    return kbinfos


# ─── Compiled product expansion (zero-LLM, used by hybrid_search with use_compiled=True) ───


async def _expand_with_compiled(tools, query: str, keywords: str, kbinfos: dict) -> None:
    """Zero-LLM compiled-product expansion: page_index → tree → KG.

    For each bound KB, searches compiled entity rows matching the query,
    hops 1-hop via relations to find neighbour entities, then appends
    their source passages to ``kbinfos["chunks"]``.
    """
    before = len(kbinfos.get("chunks", []))
    seen_ids = {c.get("chunk_id") or c.get("id") for c in kbinfos.get("chunks", [])}

    scopes = await _kg_scopes(tools)
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
                exprs.append(await settings.retriever.get_vector(text, embd_mdl, top_n, 0.1))
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

    resolved = await thread_pool_exec(tools._resolve_doc_tenant, doc_id)
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
                exprs.append(await settings.retriever.get_vector(text, embd_mdl, top_n, 0.1))
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


async def web_search(tools, query: str, keywords: str = "") -> dict:
    if not tools.has_web():
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f'[Web search] Searching the web for "{query}"')
    try:
        from common.misc_utils import thread_pool_exec

        effective_query = f"{query} {keywords}".strip() if keywords else query
        tav_res = await thread_pool_exec(tools.tav.retrieve_chunks, effective_query)
        return {"chunks": tav_res.get("chunks", []), "doc_aggs": tav_res.get("doc_aggs", [])}
    except Exception:
        _LOG.exception("web_search failed")
        return {"chunks": [], "doc_aggs": []}


async def structured_query(tools, query: str, keywords: str = "", kb_ids: list[str] | None = None) -> dict:
    """Answer from the structured (tabular) KBs by translating the query to SQL.

    ``keywords`` is accepted for schema conformance but deliberately unused: the
    query is translated to SQL rather than keyword-matched, and the rows it
    returns are not prose to narrow.
    """
    _LOG.info(f'[Structured search] Querying the structured (table) data for "{query}"')
    sql_kbs = [kb for kb in tools.sql_kbs if kb_ids is None or kb.id in kb_ids]
    if not sql_kbs:
        return {"answer": "", "chunks": [], "doc_aggs": []}
    from api.db.services.dialog_service import use_sql

    tenant_id = sql_kbs[0].tenant_id
    sql_kb_ids = [kb.id for kb in sql_kbs]
    try:
        ans = await use_sql(query, tools.field_map, tenant_id, tools.chat_mdl, quota=True, kb_ids=sql_kb_ids)
    except Exception:
        _LOG.exception("structured_query failed")
        return {"answer": "", "chunks": [], "doc_aggs": []}
    if not ans:
        return {"answer": "", "chunks": [], "doc_aggs": []}
    ref = ans.get("reference") or {}
    return {
        "answer": ans.get("answer", "") or "",
        "chunks": ref.get("chunks") or [],
        "doc_aggs": ref.get("doc_aggs") or [],
    }
