"""Exploration tools: KG, wiki."""

import logging

_LOG = logging.getLogger(__name__)


async def graph_explore(tools, entity: str, relation: str | None = None, depth: int = 1) -> dict:
    """知识图谱关联探索。

    当前为占位符——实际实现需要调用知识图谱存储。
    """
    _LOG.info("graph_explore: entity=%s relation=%s depth=%d", entity, relation, depth)
    # TODO: implement actual KG walk
    from rag.advanced_rag.harness.tools.search import hybrid_search

    query = f"{entity} {relation or ''}"
    return await hybrid_search(tools, query=query.strip())


async def wiki_query(tools, topic: str) -> dict:
    """编译的 Wiki 查询。

    当前为占位符——实际实现需要编译的 Wiki 存储。
    """
    _LOG.info("wiki_query: topic=%s", topic)
    # TODO: implement actual wiki lookup
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=topic)
