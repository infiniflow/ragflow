"""Search tools: hybrid, vector, BM25, web, structured."""

import logging
from common import settings

_LOG = logging.getLogger(__name__)


def _normalize(kbinfos: dict) -> dict:
    if not kbinfos:
        return {"chunks": [], "doc_aggs": []}
    kbinfos["chunks"] = settings.retriever.retrieval_by_children(
        kbinfos.get("chunks", []),
        getattr(kbinfos, "_tenant_ids", None),
    )
    return kbinfos


async def hybrid_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 6, doc_scope: list[str] | None = None) -> dict:
    """向量+关键词混合检索。"""
    if not tools.kb_ids and not kb_ids:
        return {"chunks": [], "doc_aggs": []}
    target_ids = kb_ids or tools.kb_ids

    embd_mdl = tools.embed_mdl
    vector_weight = 0.7 if embd_mdl else 0

    kbinfos = await settings.retriever.retrieval(
        query,
        embd_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.2,
        vector_similarity_weight=vector_weight,
        aggs=True,
        highlight=True,
        doc_ids=doc_scope,
    )
    return _normalize(kbinfos)


async def vector_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 6) -> dict:
    """纯语义检索。"""
    if not tools.embed_mdl:
        _LOG.warning("vector_search: no embed_mdl available")
        return {"chunks": [], "doc_aggs": []}
    target_ids = kb_ids or tools.kb_ids
    kbinfos = await settings.retriever.retrieval(
        query,
        tools.embed_mdl,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.2,
        vector_similarity_weight=1.0,
        aggs=True,
        highlight=True,
    )
    return _normalize(kbinfos)


async def bm25_search(tools, query: str, kb_ids: list[str] | None = None, top_n: int = 6) -> dict:
    """纯关键词检索。"""
    target_ids = kb_ids or tools.kb_ids
    kbinfos = await settings.retriever.retrieval(
        query,
        None,
        tools.tenant_ids,
        target_ids,
        1,
        top_n,
        0.0,
        vector_similarity_weight=0,
        aggs=True,
        highlight=True,
    )
    return _normalize(kbinfos)


async def web_search(tools, query: str) -> dict:
    """Web 搜索 (Tavily)。"""
    if not tools.has_web():
        return {"chunks": [], "doc_aggs": []}
    try:
        from common.misc_utils import thread_pool_exec

        tav_res = await thread_pool_exec(tools.tav.retrieve_chunks, query)
        return {"chunks": tav_res.get("chunks", []), "doc_aggs": tav_res.get("doc_aggs", [])}
    except Exception:
        _LOG.exception("web_search failed")
        return {"chunks": [], "doc_aggs": []}


async def structured_query(tools, question: str, kb_ids: list[str] | None = None) -> dict:
    """结构化 KB 查询。"""
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
