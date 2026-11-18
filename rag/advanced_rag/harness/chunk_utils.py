"""Shared chunk field accessors for the agentic-RAG harness.

Chunk rows reach the harness from several places (hybrid search, grep, compiled
structure expansion, navigation outlines) and carry the same fields under a few
legacy aliases. These accessors are the single definition used by the search and
navigation tools — previously each module carried its own byte-identical copy,
which had already drifted for ``_chunk_id`` (str vs int) and silently broke
de-duplication when the two were mixed.
"""

from typing import Any


def _xml_escape(value: Any) -> str:
    """Escape a value for embedding in an XML attribute/element."""
    s = "" if value is None else str(value)
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;")


def _chunk_text(c: dict) -> str:
    """Chunk body text across the historical content field aliases."""
    return str(c.get("content_with_weight") or c.get("content") or c.get("text") or "")


def _chunk_attr(c: dict, keys: tuple[str, ...]) -> str:
    """First non-empty value among ``keys`` (legacy field aliases)."""
    for k in keys:
        v = c.get(k)
        if v not in (None, ""):
            return str(v)
    return ""


def _doc_id(c: dict) -> str:
    return _chunk_attr(c, ("doc_id", "docid", "document_id"))


def _dataset_id(c: dict) -> str:
    return _chunk_attr(c, ("dataset_id", "kb_id", "knowledgebase_id"))


def _doc_title(c: dict) -> str:
    return _chunk_attr(c, ("docnm_kwd", "doc_title", "title", "document_name"))


def _chunk_id(c: dict) -> str:
    return _chunk_attr(c, ("chunk_id", "id"))


def _snippet(s: str, n: int) -> str:
    """Truncate a string to ``n`` chars on a char boundary with an ellipsis."""
    s = (s or "").strip()
    if len(s) <= n:
        return s
    return s[:n].rstrip() + "..."


def strip_chunk_vectors(chunks: list | None) -> None:
    """Drop dense embedding arrays from retrieved chunks.

    ``Dealer.retrieval`` still attaches a ``vector`` field for reranking and
    legacy backends. After scores are computed the vectors are not needed for
    evidence, search caches, or retrieval memory — but they are large (often
    hundreds of floats per chunk) and agentic chat can retain many copies per
    turn (memory store + per-query search_cache). Citation code re-fetches
    vectors on demand via ``fetch_chunk_vectors`` when required.
    """
    if not chunks:
        return
    for chunk in chunks:
        if isinstance(chunk, dict):
            chunk.pop("vector", None)


def strip_kbinfos_vectors(kbinfos: dict | None) -> None:
    """Remove ``vector`` from every chunk in a kbinfos dict."""
    if not isinstance(kbinfos, dict):
        return
    strip_chunk_vectors(kbinfos.get("chunks"))
