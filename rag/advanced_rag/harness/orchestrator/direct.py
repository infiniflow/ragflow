"""Low mode: direct single-pass search."""

import logging

from rag.advanced_rag.harness.stats import in_phase
from rag.advanced_rag.harness.tools.search import hybrid_search

_LOG = logging.getLogger(__name__)


@in_phase("direct")
async def direct_search(state: dict, tools) -> dict:
    """Single hybrid search → merge into kbinfos."""
    question = state.get("question", "")
    keywords = state.get("keywords", "")
    # Entity/qualifier-weighted retrieval query: a problem-level search over the
    # bare question is exactly where the entity must dominate the ranking, so the
    # weighted query (entity x3, qualifier x3) is always attached.
    retrieval_query = ""
    try:
        if hasattr(tools, "_extract_keywords_weighted"):
            retrieval_query, _ = await tools._extract_keywords_weighted(question)
    except Exception:
        _LOG.exception("[Direct] entity-weighted keyword extraction failed")
    _LOG.info('[Direct search] Looking up the knowledge base for: "%s" (keywords: %s)', question, keywords)

    result = await hybrid_search(tools, query=question, keywords=keywords, retrieval_query=retrieval_query, use_compiled=True)
    _merge_kbinfos(tools, result)

    if not _has_chunks(tools):
        _LOG.info("[Direct search] Found no matching passages.")
        return {"empty_result": True, "kbinfos": tools.kbinfos}

    return {"kbinfos": tools.kbinfos}


def _merge_kbinfos(tools, result: dict):
    if not result or not result.get("chunks"):
        return
    seen = {_chunk_key(c) for c in tools.kbinfos.get("chunks", [])}
    for c in result.get("chunks", []):
        k = _chunk_key(c)
        if k in seen:
            continue
        seen.add(k)
        tools.kbinfos.setdefault("chunks", []).append(c)
    dseen = {d.get("doc_id") for d in tools.kbinfos.get("doc_aggs", [])}
    for d in result.get("doc_aggs", []):
        if d.get("doc_id") in dseen:
            continue
        dseen.add(d.get("doc_id"))
        tools.kbinfos.setdefault("doc_aggs", []).append(d)


def _chunk_key(ck: dict) -> str:
    return ck.get("chunk_id") or ck.get("id") or str(id(ck))


def _has_chunks(tools) -> bool:
    return bool(tools.kbinfos.get("chunks"))
