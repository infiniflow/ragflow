"""Navigation tools: TOC, page index, mindmap."""

import logging

_LOG = logging.getLogger(__name__)


async def toc_navigate(tools, topic: str, doc_scope: list[str] | None = None) -> dict:
    """按文档目录定位。需要 KB 有 toc 编译产物。

    当前为占位符——当知识编译的 TOC 工具可用时，替换为实际实现。
    回退到 hybrid_search。
    """
    _LOG.info("toc_navigate called for topic=%s", topic)
    # TODO: replace with actual TOC navigation when compilation layer is ready
    # For now, fallback to hybrid search
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=topic, doc_scope=doc_scope)


async def page_index_navigate(tools, topic: str, kb_ids: list[str] | None = None) -> dict:
    """按页面索引导航。"""
    _LOG.info("page_index_navigate called for topic=%s", topic)
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=topic, kb_ids=kb_ids)


async def mindmap_navigate(tools, concept: str, kb_ids: list[str] | None = None) -> dict:
    """按概念脑图定位。"""
    _LOG.info("mindmap_navigate called for concept=%s", concept)
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=concept, kb_ids=kb_ids)
