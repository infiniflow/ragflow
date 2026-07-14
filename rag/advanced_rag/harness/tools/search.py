"""Search tools: hybrid, vector, BM25, web, structured."""

import logging
import re
from common import settings

_LOG = logging.getLogger(__name__)


# Sentence terminators: Chinese 。！？；, English ! ? ;, newline, and a
# digit-guarded English period (so "3.14" / "v1.2" don't split).
_SENT_END = re.compile(r"[。！？；!?;\n]+|(?<!\d)\.(?!\d)")

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
    return "..." + "".join(sents[i] for i in sorted(keep)).strip() + "..."


def _narrow_by_keywords(chunks: list[dict], keywords: str) -> list[dict]:
    """Narrow each chunk to its keyword-bearing sentences (+/- 1 neighbour) and
    drop keyword-less chunks when at least one other chunk carries a keyword.

    Keywords are the comma-separated terms (with close synonyms) produced by
    ``formalize``; matching is case-insensitive substring.
    """
    kwds = [k.strip().lower() for k in (keywords or "").split(",") if k.strip()]
    if not kwds or not chunks:
        return chunks

    scored = [(ck, _narrow_content(ck.get("content_with_weight") or ck.get("content") or "", kwds)) for ck in chunks]
    any_hit = any(nc is not None for _, nc in scored)

    out: list[dict] = []
    for ck, nc in scored:
        if nc is not None:
            ck["content_with_weight"] = nc
            if "content" in ck:
                ck["content"] = nc
            ck.pop("highlight", None)
            out.append(ck)
        elif not any_hit:
            # Nobody matched a keyword — keep everything unchanged.
            out.append(ck)
        # else: this chunk has no keyword but others do -> drop it.
    return out


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


async def hybrid_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 6, doc_scope: list[str] | None = None, keywords: str = "") -> dict:
    if not tools.kb_ids and not kb_ids:
        return {"chunks": [], "doc_aggs": []}
    target_ids = kb_ids or tools.kb_ids
    _LOG.info(f"[Hybrid search]: {query} -> {keywords}")

    # Query expansion: append the formalized-question keywords + close synonyms
    # so hybrid/BM25 retrieval gets extra recall signal.
    effective_query = f"{query} {keywords}".strip() if keywords else query

    embd_mdl = tools.embed_mdl
    vector_weight = 0.7 if embd_mdl else 0

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
        l = len(kbinfos["chunks"])
        kbinfos["chunks"] = _narrow_by_keywords(kbinfos.get("chunks", []), keywords)
        _LOG.info(f"[Hybrid search]: snippet {l} -> {len(kbinfos['chunks'])}")
    return kbinfos


async def vector_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 6, keywords: str = "") -> dict:
    if not tools.embed_mdl:
        _LOG.warning("vector_search: no embed_mdl available")
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f"[Vector search]: {query} -> {keywords}")
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
    if keywords:
        l = len(kbinfos["chunks"])
        kbinfos["chunks"] = _narrow_by_keywords(kbinfos.get("chunks", []), keywords)
        _LOG.info(f"[Vector search]: snippet {l} -> {len(kbinfos['chunks'])}")
    return _normalize(kbinfos, tools.tenant_ids)


async def bm25_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 6, keywords: str = "") -> dict:
    _LOG.info(f"[BM25 search]: {query} -> {keywords}")
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
    if keywords:
        l = len(kbinfos["chunks"])
        kbinfos["chunks"] = _narrow_by_keywords(kbinfos.get("chunks", []), keywords)
        _LOG.info(f"[Vector search]: snippet {l} -> {len(kbinfos['chunks'])}")
    return _normalize(kbinfos, tools.tenant_ids)


async def web_search(tools, query: str, keywords: str = "") -> dict:
    if not tools.has_web():
        return {"chunks": [], "doc_aggs": []}

    _LOG.info(f"[Web search]: {query} -> {keywords}")
    try:
        from common.misc_utils import thread_pool_exec

        effective_query = f"{query} {keywords}".strip() if keywords else query
        tav_res = await thread_pool_exec(tools.tav.retrieve_chunks, effective_query)
        return {"chunks": tav_res.get("chunks", []), "doc_aggs": tav_res.get("doc_aggs", [])}
    except Exception:
        _LOG.exception("web_search failed")
        return {"chunks": [], "doc_aggs": []}


async def structured_query(tools, question: str, kb_ids: list[str] | None = None) -> dict:
    _LOG.info(f"[Structured search]: {question}")
    sql_kbs = [kb for kb in tools.sql_kbs if kb_ids is None or kb.id in kb_ids]
    if not sql_kbs:
        return {"answer": "", "chunks": [], "doc_aggs": []}
    from api.db.services.dialog_service import use_sql

    tenant_id = sql_kbs[0].tenant_id
    sql_kb_ids = [kb.id for kb in sql_kbs]
    try:
        ans = await use_sql(question, tools.field_map, tenant_id, tools.chat_mdl, quota=True, kb_ids=sql_kb_ids)
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
