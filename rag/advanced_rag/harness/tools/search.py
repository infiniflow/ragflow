"""Search tools: hybrid, vector, BM25, web, structured.

Retrieval legs plus the compiled-structure expansion layered on top of hybrid
search. The tool-facing schemas and dispatch live in ``action_session``; this
module owns the retrieval implementations they call.
"""

import logging
import re
from typing import Any
from common import settings
from rag.advanced_rag.harness.chunk_utils import (  # noqa: F401
    _chunk_attr,
    _chunk_id,
    _chunk_text,
    _dataset_id,
    _doc_id,
    _doc_title,
    _snippet,
    _xml_escape,
)

# ``_expand_related_via_structure`` is kept imported (not currently called here)
# so the compiled-structure related-chunk expansion stays reachable without
# re-editing imports; the @tool front-end that used it was removed.
from .navigation import _expand_related_via_structure, _kg_scopes  # noqa: F401

_LOG = logging.getLogger(__name__)


# Text processing (sentence split / stemming / keyword narrowing) lives in
# ``text_processing`` and is re-exported here: callers across the harness
# import these names from this module.
from rag.advanced_rag.harness.tools.text_processing import (  # noqa: F401
    _compact_keywords,
    _highlight_keywords,
    _is_fact_dense_sentence,
    _keyword_forms,
    _narrow_by_keywords,
    _narrow_content,
    _narrow_or_keep,
    _sentence_matches,
    _split_sentences,
    _stem,
)


# Fallbacks for callers that supply no retrieval configuration: the values the
# search tools used unconditionally before RAGTools carried settings.
_DEFAULT_SIMILARITY_THRESHOLD = 0.2
_DEFAULT_HYBRID_VECTOR_WEIGHT = 0.3
_DEFAULT_TOP_N = 12
_DEFAULT_RERANK_CANDIDATES = 64
_DEFAULT_TOP_K = 1024


def _setting(tools, name: str, default):
    """Read a retrieval setting off ``tools``. ``None`` means unset; 0.0 is a valid value."""
    value = getattr(tools, name, None)
    return default if value is None else value


def _resolve_top_n(tools, top_n: int | None) -> int:
    """Explicit argument wins, then the caller's configuration, then the default."""
    if top_n is not None:
        return top_n
    return int(_setting(tools, "top_n", _DEFAULT_TOP_N))


def _resolve_top_k(tools) -> int:
    """Size of the approximate-kNN candidate pool (ES ``k``, ``num_candidates`` is 2x).

    Recall, not ranking: a vector outside the pool cannot be returned at any weight.
    """
    return int(_setting(tools, "top_k", _DEFAULT_TOP_K))


def _resolve_rerank_candidates(tools, top_n: int) -> int:
    """``Dealer.retrieval`` rejects ``page * page_size > rerank_candidates_count``, so
    the candidate set can never be smaller than the page it has to fill."""
    return max(int(_setting(tools, "rerank_candidates_count", _DEFAULT_RERANK_CANDIDATES)), top_n)


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


async def hybrid_search(
    tools, query: str, kb_ids: list[str] | None = None, top_n: int | None = None, doc_scope: list[str] | None = None, keywords: str = "", retrieval_query: str = "", use_compiled: bool = False
) -> dict:
    top_n = _resolve_top_n(tools, top_n)
    target_ids = kb_ids or list(dict.fromkeys(tools.kb_ids + [kb.id for kb in tools.sql_kbs]))
    if not target_ids:
        return {"chunks": [], "doc_aggs": []}
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    _LOG.info(f'[Hybrid search] Searching the knowledge base for "{query}" (keywords: {keywords})')

    # Query expansion: append the entity-weighted ``retrieval_query`` (entity
    # terms repeated so BM25 weights them up inside the same query) — the plain
    # keyword union or the compact formalize keywords fall back when none is
    # supplied. ``keywords`` is always used only to narrow retrieved chunks.
    if retrieval_query:
        effective_query = f"{query} {retrieval_query}".strip()[:400]
    else:
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
    # No embedding model means no vector leg, whatever the caller asked for.
    vector_weight = _setting(tools, "vector_similarity_weight", _DEFAULT_HYBRID_VECTOR_WEIGHT) if embd_mdl else 0

    similarity_threshold = _setting(tools, "similarity_threshold", _DEFAULT_SIMILARITY_THRESHOLD)
    knn_top_k = _resolve_top_k(tools)
    rerank_candidates_count = _resolve_rerank_candidates(tools, top_n)
    _LOG.debug(
        "[Hybrid search] top_n=%s threshold=%s vector_weight=%s knn_top_k=%s rerank_candidates_count=%s",
        top_n,
        similarity_threshold,
        vector_weight,
        knn_top_k,
        rerank_candidates_count,
    )
    kbinfos = await settings.retriever.retrieval(
        effective_query,
        embd_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        similarity_threshold,
        vector_similarity_weight=vector_weight,
        knn_top_k=knn_top_k,
        aggs=True,
        highlight=False,
        doc_ids=doc_scope,
        must_not={"exists": "compile_kwd"},  # plain retrieval = document chunks only; compiled products have their own tools
        rerank_candidates_count=rerank_candidates_count,
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    # Preserve the RAW retrieved chunks in the central memory store BEFORE any
    # narrowing. search is cheap and the raw corpus may hold a fact the LLM's
    # report/grounded extraction later compresses away — a gap-driven grep over
    # memory recovers it without re-querying the knowledge base.
    try:
        from rag.advanced_rag.harness.memory import add as _memory_add

        _memory_add(tools, kbinfos.get("chunks", []) or [])
    except Exception:
        pass  # memory is best-effort; never fail the search over it.
    # Narrow-or-keep: if the query keywords match any chunk, keep only the
    # matching passages (shrinking the evidence handed to the LLM so a single
    # full-context call stays small and fast); if nothing matches, keep ALL
    # chunks intact so no evidence is silently dropped. This bounds per-claim
    # analysis size without losing the numeric/entity rows when keywords hit.
    kbinfos["chunks"] = _narrow_or_keep(kbinfos.get("chunks", []), keywords, "hybrid_search")
    if use_compiled and kbinfos.get("chunks"):
        _LOG.info("[Hybrid search] Compiled expansion enabled — enriching with page_index/tree/KG navigation.")
        await _expand_with_compiled(tools, query, keywords, kbinfos, doc_scope)
    chunks_now = kbinfos.get("chunks") or []
    if chunks_now:
        _doc_stats: dict = {}
        for _c in chunks_now:
            _d = str(_c.get("docnm_kwd") or _c.get("docnm") or _c.get("doc_name") or "?")
            _s = _doc_stats.setdefault(_d, [0, 0])
            _s[0] += 1
            _s[1] += len(str(_c.get("content") or _c.get("content_with_weight") or ""))
        _detail = "; ".join(f"{d}:{n}chunk({sz}chars)" for d, (n, sz) in sorted(_doc_stats.items()))
        _LOG.info(f'[Hybrid search] "{query[:80]}" -> {len(chunks_now)} chunk(s): {_detail}')
    if cache is not None:
        cache[cache_key] = kbinfos
    return kbinfos


async def vector_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int | None = None, keywords: str = "", retrieval_query: str = "", doc_scope: list[str] | None = None) -> dict:
    top_n = _resolve_top_n(tools, top_n)
    if not tools.embed_mdl:
        _LOG.warning("vector_search: no embed_mdl available")
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f'[Vector search] Searching by meaning for "{query}" (keywords: {keywords})')
    effective_query = f"{query} {retrieval_query}".strip()[:400] if retrieval_query else f"{query} {keywords}".strip() if keywords else query
    target_ids = kb_ids or tools.kb_ids
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    knn_top_k = _resolve_top_k(tools)
    rerank_candidates_count = _resolve_rerank_candidates(tools, top_n)
    _LOG.debug("[Vector search] top_n=%s knn_top_k=%s rerank_candidates_count=%s", top_n, knn_top_k, rerank_candidates_count)
    kbinfos = await settings.retriever.retrieval(
        effective_query,
        tools.embed_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.2,  # pure-cosine floor, not the caller's hybrid threshold
        vector_similarity_weight=1.0,  # vector-only by definition of this tool
        knn_top_k=knn_top_k,
        aggs=False,
        highlight=False,
        doc_ids=doc_scope,
        must_not={"exists": "compile_kwd"},
        rerank_candidates_count=rerank_candidates_count,
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    try:
        from rag.advanced_rag.harness.memory import add as _memory_add

        _memory_add(tools, kbinfos.get("chunks", []) or [])
    except Exception:
        pass
    kbinfos["chunks"] = _narrow_or_keep(kbinfos.get("chunks", []), keywords, "Vector search")
    return kbinfos


async def bm25_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int | None = None, keywords: str = "", retrieval_query: str = "", doc_scope: list[str] | None = None) -> dict:
    top_n = _resolve_top_n(tools, top_n)
    _LOG.info(f'[BM25 search] Searching by keyword for "{query}" (keywords: {keywords})')
    target_ids = kb_ids or tools.kb_ids
    effective_query = f"{query} {retrieval_query}".strip()[:400] if retrieval_query else f"{query} {keywords}".strip() if keywords else query
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    knn_top_k = _resolve_top_k(tools)
    rerank_candidates_count = _resolve_rerank_candidates(tools, top_n)
    _LOG.debug("[BM25 search] top_n=%s knn_top_k=%s rerank_candidates_count=%s", top_n, knn_top_k, rerank_candidates_count)
    kbinfos = await settings.retriever.retrieval(
        effective_query,
        None,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.0,  # BM25 scores are not comparable to a hybrid threshold
        vector_similarity_weight=0,  # keyword-only by definition of this tool
        knn_top_k=knn_top_k,
        aggs=False,
        highlight=False,
        doc_ids=doc_scope,
        must_not={"exists": "compile_kwd"},
        rerank_candidates_count=rerank_candidates_count,
    )
    kbinfos = _normalize(kbinfos, tools.tenant_ids)
    try:
        from rag.advanced_rag.harness.memory import add as _memory_add

        _memory_add(tools, kbinfos.get("chunks", []) or [])
    except Exception:
        pass
    kbinfos["chunks"] = _narrow_or_keep(kbinfos.get("chunks", []), keywords, "BM25 search")
    return kbinfos


# Compiled-product expansion lives in ``compiled_expansion`` and is
# re-exported here: navigation and the dynamic runner import these names
# from this module.
from rag.advanced_rag.harness.tools.compiled_expansion import (  # noqa: F401
    _expand_compiled_strategy,
    _expand_wiki_page_strategy,
    _expand_with_compiled,
    _load_chunks_for_doc,
    _search_compiled_rows,
    _search_synthesis_pages,
)


async def web_search(tools, query: str, keywords: str = "", retrieval_query: str = "") -> dict:
    if not tools.has_web():
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f'[Web search] Searching the web for "{query}"')
    try:
        from common.misc_utils import thread_pool_exec

        effective_query = f"{query} {retrieval_query}".strip()[:400] if retrieval_query else f"{query} {keywords}".strip() if keywords else query
        web_res = await thread_pool_exec(tools.web_search.retrieve_chunks, effective_query)
        return {"chunks": web_res.get("chunks", []), "doc_aggs": web_res.get("doc_aggs", [])}
    except Exception:
        _LOG.exception("web_search failed")
        return {"chunks": [], "doc_aggs": []}


async def structured_query(tools, query: str, keywords: str = "", kb_ids: list[str] | None = None, doc_scope: list[str] | None = None) -> dict:
    """Answer from the structured (tabular) KBs by translating the query to SQL.

    ``keywords`` is accepted for schema conformance but deliberately unused: the
    query is translated to SQL rather than keyword-matched, and the rows it
    returns are not prose to narrow.
    """
    _LOG.info(f'[Structured search] Querying the structured (table) data for "{query}"')
    sql_kbs = [kb for kb in tools.sql_kbs if kb_ids is None or kb.id in kb_ids]
    if not sql_kbs:
        return {"answer": "", "chunks": [], "doc_aggs": []}
    if hasattr(tools, "scoped_doc_ids"):
        doc_scope = tools.scoped_doc_ids(doc_scope)
    from api.db.services.dialog_service import use_sql

    tenant_id = sql_kbs[0].tenant_id
    sql_kb_ids = [kb.id for kb in sql_kbs]
    try:
        ans = await use_sql(query, tools.field_map, tenant_id, tools.chat_mdl, quota=True, kb_ids=sql_kb_ids, doc_ids=doc_scope)
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


# ─── Grep-style exact search (BM25 candidate pool + regex locate), and ───
# ─── list_chunks (deep-read a document) — used by the sub-agent so it can   ───
# ─── do exact keyword/pattern locate + full-document deep-read like dynamic. ───

_GREP_TERMS_MAX = 10
_GREP_OUT_CHARS_PER_CHUNK = 700
_GREP_OUT_TOTAL_CHARS = 8000
_LIST_CHUNKS_MAX_CHUNKS = 80


def _is_table_chunk(c: dict) -> bool:
    """Corpus-neutral table detector: HTML table markup or >=3 pipe rows.

    Table chunks must NOT be term-narrowed: their answer rows often sit
    mid/late-table (e.g. a rank row at ~62% of a 14.7K-char table), and the
    grep context window truncates them to a header-only snippet, hiding the
    answer from the action-session model (Q86: 2011 Pan Am standings rank 19
    at char 5181 of 14717 was cut by the 700-char narrow).
    """
    t = str(_chunk_text(c) or "")
    if "<table" in t.lower() or "<tr" in t.lower():
        return True
    pipe_rows = sum(1 for line in t.splitlines() if line.count("|") >= 2)
    return pipe_rows >= 3


def _grep_terms_from_query(query: str, max_terms: int = _GREP_TERMS_MAX) -> list[str]:
    """Extract compact grep terms from a query: bare alnum words of length>=2,
    deduped (order-preserving) and capped. Numbers/ids are preserved as-is."""
    if not query:
        return []
    terms: list[str] = []
    seen: set[str] = set()
    for m in re.findall(r"[A-Za-z0-9][A-Za-z0-9_.\-]{1,}", query):
        t = m.strip("._-")
        if len(t) < 2:
            continue
        low = t.lower()
        if low in seen:
            continue
        seen.add(low)
        terms.append(t)
        if len(terms) >= max_terms:
            break
    return terms


async def grep_search(
    tools,
    query: str,
    kb_ids: list[str] | None = None,
    top_n: int = 60,
    doc_scope: list[str] | None = None,
    keywords: str | None = None,
) -> dict:
    """Exact keyword/pattern locate: BM25-first candidate pool, then a
    case-insensitive regex locate with a short context window (like dynamic's
    grep_chunks, but returning the ``Pipeline`` ``{"chunks": [...]}`` contract).

    ``keywords`` (optional) is a SOFT retrieval hint fed to the BM25 candidate
    pool — e.g. the nav-routed docs' summaries ("nav is a hint, not a
    constraint"). When empty, the query's own terms are extracted instead. It
    never acts as a hard filter; the candidate pool still spans the whole corpus.

    Returns compact snippet chunks (token-cheap) for the sub-agent. When grep
    matches nothing, the BM25 candidates are returned unchanged so evidence is
    never dropped (enumeration / multi-hop answers must not lose candidates).
    """
    from rag.advanced_rag.harness.grep_sed_narrow import narrow_by_terms

    _LOG.info('[Grep search] Keyword-first locate for "%s"', query)
    if not query or not str(query).strip():
        return {"chunks": [], "doc_aggs": []}
    terms = _grep_terms_from_query(str(query).strip())
    # Pass the extracted terms (or the explicit nav hint) as BM25 keywords so the
    # candidate pool is built from "sentence + discriminating terms". A long
    # question buries its proper nouns (e.g. "Culdect Saga") under stopwords;
    # without the keywords boost the noun chunk never enters the pool and grep
    # has nothing to locate.
    hint = keywords if keywords else " ".join(terms)
    res = await bm25_search(tools, query=str(query).strip(), kb_ids=kb_ids, top_n=top_n, doc_scope=doc_scope, keywords=hint)
    chunks = res.get("chunks", []) or []
    if not chunks or not terms:
        return res
    try:
        # Table chunks pass through UN-narrowed (full text): term-grep windows
        # truncate them to a header-only snippet and hide mid-table answer rows.
        # Prose chunks keep the compact narrow.
        table_chunks = [c for c in chunks if _is_table_chunk(c)]
        prose_chunks = [c for c in chunks if not _is_table_chunk(c)]
        if prose_chunks:
            out = narrow_by_terms(
                prose_chunks,
                terms,
                keywords=str(query).strip(),
                context={"before": 1, "after": 0},
                max_out_chars_per_chunk=_GREP_OUT_CHARS_PER_CHUNK,
                max_out_total_chars=_GREP_OUT_TOTAL_CHARS,
            )
            kept = out.get("kept") or []
        else:
            kept = []
        kept = table_chunks + kept
        if kept:
            _LOG.info(
                "[Grep search] narrowed %d->%d chunk(s), %.1fK chars.",
                len(chunks),
                len(kept),
                sum(len(str(c.get("content_with_weight") or c.get("content") or "")) for c in kept) / 1000.0,
            )
            res["chunks"] = kept
    except Exception:
        _LOG.exception("[Grep search] narrow failed; using raw BM25 candidates.")
    _g = res.get("chunks") or []
    if _g:
        _gs: dict = {}
        for _c in _g:
            _d = str(_c.get("docnm_kwd") or _c.get("docnm") or "?")
            _e = _gs.setdefault(_d, [0, 0])
            _e[0] += 1
            _e[1] += len(str(_c.get("content") or _c.get("content_with_weight") or ""))
        _LOG.info(
            '[Grep search] "%s" -> %d chunk(s): %s',
            str(query)[:80],
            len(_g),
            "; ".join(f"{d}:{n}chunk({sz}chars)" for d, (n, sz) in sorted(_gs.items())),
        )
    return res


async def list_chunks(tools, doc_id: str) -> dict:
    """Deep-read a document: return its COMPLETE chunk list in reading order.

    The sub-agent calls this after grep_search / hybrid_search hit a document
    when it needs the full text (enumeration / count / arithmetic answers), like
    dynamic's ``list_chunks`` tool. Returns ``{"chunks": [...], "doc_aggs": [...]}``.
    """
    if not doc_id or not str(doc_id).strip():
        return {"chunks": [], "doc_aggs": []}
    if not callable(getattr(tools, "fetch_full_document", None)):
        return {"chunks": [], "doc_aggs": []}
    _LOG.info("[List chunks] Deep-reading document %s", doc_id)
    try:
        full = await tools.fetch_full_document(str(doc_id).strip())
    except Exception:
        _LOG.exception("[List chunks] fetch_full_document failed doc=%s", doc_id)
        return {"chunks": [], "doc_aggs": []}
    chunks = (full.get("chunks") or [])[:_LIST_CHUNKS_MAX_CHUNKS]
    return {"chunks": chunks, "doc_aggs": full.get("doc_aggs") or []}


# ── Rag-agent tool set (migrated from harness/dynamic) ──
_NAV_TREE_MAX_DOCS = 8

# Cap on datasets scanned per tool call (avoids fan-out over too many KBs).
_NAV_TREE_MAX_DATASETS = 10

# Chars of a chunk's text returned by search_chunks in default (snippet) mode.
_SEARCH_SNIPPET_CHARS = 300


async def _load_specific_chunks(tools_slot, chunk_ids: list[str], doc_scope: list[str] | None = None) -> list[dict]:
    """Load specific chunks (from navigate_structure outline pointers) by id.

    Fetches each ``doc_scope`` document's full chunk list and filters to the
    requested ``chunk_ids``. When ``doc_scope`` is omitted, scans all bound
    documents (expensive) — so prefer passing doc_scope with the owning doc(s).
    Zero LLM.
    """
    wanted = {str(c).strip() for c in chunk_ids if str(c).strip()}
    if not wanted:
        return []
    if not doc_scope:
        # Derive candidate docs from the tools' kb_ids' documents is not available
        # here; require doc_scope for the precise path.
        return []
    found: list[dict] = []
    seen: set[str] = set()
    for doc_id in doc_scope[:8]:
        try:
            full = await tools_slot.fetch_full_document(doc_id)
        except Exception:
            _LOG.exception("[grep_chunks] fetch_full_document failed for doc_id=%s", doc_id)
            continue
        for c in full.get("chunks", []) or []:
            cid = _chunk_id(c)
            if cid in wanted and cid not in seen:
                seen.add(cid)
                found.append(c)
    return found


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


# The dynamic runner sets this module slot to the active RAGTools instance for
# the current request, so the tools above (defined once at import) read the
# request-scoped retrieval context.
_tools_ref: dict[str, Any] = {}


def _tools_slot():
    return _tools_ref.get("tools")


def _get_kb_ids(tools_slot) -> list[str]:
    if tools_slot is None:
        return []
    ids = getattr(tools_slot, "kb_ids", None) or []
    return list(ids)


def _query_to_terms(query: str) -> list[str]:
    """Derive plain grep terms from a regex query for narrow_by_terms.

    Splits the alternation into its constituents (capped) so the regex locate
    and the term-based context window both fire on the same broad set of names.
    """
    q = (query or "").strip()
    if not q:
        return []
    # Strip regex anchors/quantifiers that are meaningless as grep terms.
    q = re.sub(r"^\(\?i\)", "", q)
    q = q.replace("\\b", "").replace("(?i)", "")
    parts = re.split(r"\|", q)
    terms = []
    for p in parts:
        p = re.sub(r"[.*+?^$()\[\]{}]", " ", p).strip()
        if not p:
            continue
        for tok in re.split(r"\s+", p):
            tok = tok.strip()
            if tok and len(tok) >= 2 and tok not in terms:
                terms.append(tok)
        if len(terms) >= 16:
            break
    return terms


def _base_chat_mdl(tools_slot):
    """Return the concrete model instance that runs the tool-calling loop.

    The chain is ``RAGTools.chat_mdl`` → ``CountingChatModel`` (proxy) →
    ``LLMBundle`` (tenant bundle) → ``mdl`` (a ``rag.llm.chat_model.Base``
    instance). We must bind and run the loop on the *innermost* ``Base`` object,
    because ``LLMBundle.bind_tools`` is the legacy ``(toolcall_session, tools)``
    style (gated on ``is_tools``) and does NOT expose the decorator
    ``bind_tools(tools=[...])`` style nor ``async_chat_streamly_with_tools``.
    """
    try:
        chat_mdl = getattr(tools_slot, "chat_mdl", None)
        if chat_mdl is None:
            return None
        # CountingChatModel proxy → LLMBundle bundle.
        raw = getattr(chat_mdl, "_chat_mdl", None) or chat_mdl
        # LLMBundle → innermost Base/ChatModel instance.
        mdl = getattr(raw, "mdl", None) or raw
        if mdl is None:
            return None
        # Ensure we actually landed on a tool-calling-capable object; otherwise
        # surface the chain for debugging.
        if not callable(getattr(mdl, "bind_tools", None)) or not callable(getattr(mdl, "async_chat_streamly_with_tools", None)):
            _LOG.error(
                "[dynamic] resolved model %r lacks tool loop methods (chain: chat_mdl=%r raw=%r)",
                mdl,
                chat_mdl,
                raw,
            )
            return None
        return mdl
    except Exception:
        _LOG.exception("[dynamic] failed to resolve base chat model")
        return None


# Public tool name the model / prompt refers to, mapped from the internal
# ``@tool``-decorated function name. ``_build_openai_schema`` uses ``fn.__name__``
# (e.g. ``_grep_chunks_impl``), but the model should call the clean, documented
# names below (matching the Go port and the prompt's tool-selection guidelines).
_TOOL_NAME_BY_FUNC = {
    "_think_impl": "think",
    "_todo_write_impl": "todo_write",
    "_grep_chunks_impl": "grep_chunks",
    "_search_chunks_impl": "search_chunks",
    "_list_chunks_impl": "list_chunks",
    "_calculate_impl": "calculate",
    "_navigate_tree_impl": "navigate_tree",
    "_navigate_structure_impl": "navigate_structure",
}

# Thinking modes that enable the knowledge-compilation tools (``navigate_tree``
# + ``navigate_structure``). Mirrors the high/ultra static tool lists in
# ``harness/config.py``.
_COMPILED_TOOL_MODES = {"high", "ultra"}


def _with_clean_names(callables: list) -> list:
    """Return ``callables`` with their schema ``function.name`` set to the clean
    public name. Re-uses the cached schema to avoid re-deriving param docs."""
    for fn in callables:
        fname = fn.__name__
        clean = _TOOL_NAME_BY_FUNC.get(fname)
        if not clean:
            _LOG.warning("[dynamic] no public name mapped for tool function %r; leaving as-is", fname)
            continue
        schema = fn.openai_schema
        schema["function"]["name"] = clean
    return callables


async def _only_strings(stream):
    """Pass through only string items from a tool-loop stream, silently dropping
    non-string metadata (int token sentinels, provider-specific tuples)."""
    async for item in stream:
        if isinstance(item, str):
            yield item
