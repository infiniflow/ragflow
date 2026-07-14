"""Navigation tools: TOC, page index, and mindmap."""

import logging

_LOG = logging.getLogger(__name__)


async def toc_navigate(tools, topic: str, doc_scope: list[str] | None = None) -> dict:
    """Locate by document table of contents.

    Requires a KB with a compiled TOC artifact. This is currently a placeholder.
    Replace it with the real implementation when the knowledge compilation TOC
    tool is available. Falls back to hybrid search for now.
    """
    _LOG.info("toc_navigate called for topic=%s", topic)
    # TODO: replace with actual TOC navigation when compilation layer is ready
    # For now, fallback to hybrid search
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=topic, doc_scope=doc_scope)


async def page_index_navigate(tools, topic: str, kb_ids: list[str] | None = None) -> dict:
    """Navigate by page index."""
    _LOG.info("page_index_navigate called for topic=%s", topic)
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=topic, kb_ids=kb_ids)


async def mindmap_navigate(tools, concept: str, kb_ids: list[str] | None = None) -> dict:
    """Locate by concept mindmap."""
    _LOG.info("mindmap_navigate called for concept=%s", concept)
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=concept, kb_ids=kb_ids)
