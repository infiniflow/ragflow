"""Inspector tools: operate on already-returned results."""

import logging
from copy import deepcopy

_LOG = logging.getLogger(__name__)


async def open_context(tools, chunk_id: str, width: int = 500) -> dict:
    """Expand context around a chunk by looking up adjacent chunks in kbinfos."""
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
    """Find chunks in kbinfos and list the document sources they come from."""
    if not chunk_ids:
        return {"chunks": [], "doc_aggs": []}
    chunks = tools.kbinfos.get("chunks", [])
    matched = [c for c in chunks if _chunk_id(c) in chunk_ids]
    return {"chunks": matched, "doc_aggs": _collect_doc_aggs(tools, matched)}


async def grep_within(tools, doc_id: str, pattern: str) -> dict:
    """Find a keyword within a document and return its chunks narrowed to the
    matching sentences (+/- 1 neighbour).

    ``pattern`` is treated as the keyword string (comma-separate for several).
    Delegates to :func:`_narrow_by_keywords`, which keeps only keyword-bearing
    sentences and drops chunks with no match. Operates on copies so the shared
    ``kbinfos`` citation pool is never mutated.
    """
    from rag.advanced_rag.harness.tools.search import _narrow_by_keywords

    chunks = [deepcopy(c) for c in tools.kbinfos.get("chunks", []) if c.get("doc_id") == doc_id]
    matched = _narrow_by_keywords(chunks, pattern)
    return {"chunks": matched, "doc_aggs": _collect_doc_aggs(tools, matched)}


async def request_adjacent(tools, chunk_id: str, direction: str = "next", count: int = 3) -> dict:
    """Get adjacent entries before or after a chunk."""
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


# Helpers


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
