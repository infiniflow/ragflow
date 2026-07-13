"""Inspector tools: operate on already-returned results."""

import logging
import re

_LOG = logging.getLogger(__name__)


async def open_context(tools, chunk_id: str, width: int = 500) -> dict:
    """展开一条 chunk 周围的上下文。在已有 kbinfos 中查找相邻 chunk。"""
    chunks = tools.kbinfos.get("chunks", [])
    idx = _find_chunk_index(chunks, chunk_id)
    if idx is None:
        return {"chunks": [], "doc_aggs": []}
    start = max(0, idx - 2)
    end = min(len(chunks), idx + 3)
    context = chunks[start:end]
    _LOG.info("open_context: chunk=%s index=%d context=%d chunks", chunk_id, idx, len(context))
    return {"chunks": context, "doc_aggs": _collect_doc_aggs(tools, context)}


async def compare_sources(tools, chunk_ids: list[str]) -> dict:
    """对比多个 chunk。在已有的 kbinfos 中查找这些 chunk 并列出它们的 doc 来源。"""
    if not chunk_ids:
        return {"chunks": [], "doc_aggs": []}
    chunks = tools.kbinfos.get("chunks", [])
    matched = [c for c in chunks if _chunk_id(c) in chunk_ids]
    return {"chunks": matched, "doc_aggs": _collect_doc_aggs(tools, matched)}


async def grep_within(tools, doc_id: str, pattern: str) -> dict:
    """在文档内精确搜索关键词。"""
    chunks = tools.kbinfos.get("chunks", [])
    matched = []
    for c in chunks:
        if c.get("doc_id") != doc_id:
            continue
        content = c.get("content_with_weight", c.get("text", ""))
        if re.search(pattern, content, re.IGNORECASE):
            matched.append(c)
    return {"chunks": matched, "doc_aggs": _collect_doc_aggs(tools, matched)}


async def request_adjacent(tools, chunk_id: str, direction: str = "next", count: int = 3) -> dict:
    """获取 chunk 前后的相邻条目。"""
    chunks = tools.kbinfos.get("chunks", [])
    idx = _find_chunk_index(chunks, chunk_id)
    if idx is None:
        return {"chunks": [], "doc_aggs": []}

    if direction == "prev":
        start = max(0, idx - count)
        end = idx
    else:
        start = idx + 1
        end = min(len(chunks), start + count)

    adjacent = chunks[start:end]
    return {"chunks": adjacent, "doc_aggs": _collect_doc_aggs(tools, adjacent)}


# ── Helpers ──


def _find_chunk_index(chunks: list[dict], chunk_id: str) -> int | None:
    for i, c in enumerate(chunks):
        if _chunk_id(c) == chunk_id:
            return i
    return None


def _chunk_id(ck: dict) -> str:
    return str(ck.get("chunk_id") or ck.get("id") or "")


def _collect_doc_aggs(tools, chunks: list[dict]) -> list[dict]:
    seen = set()
    aggs = []
    for c in chunks:
        doc_id = c.get("doc_id")
        if doc_id and doc_id not in seen:
            seen.add(doc_id)
            aggs.append({"doc_id": doc_id, "doc_name": c.get("docnm_kwd", "")})
    return aggs
