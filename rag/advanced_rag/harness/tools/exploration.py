"""Exploration tools: knowledge graph and wiki lookup."""

import logging

_LOG = logging.getLogger(__name__)


async def graph_explore(tools, entity: str, relation: str | None = None, depth: int = 1) -> dict:
    """Explore relationships in the knowledge graph.

    This is currently a placeholder. The final implementation should call the
    knowledge graph store.
    """
    _LOG.info("graph_explore: entity=%s relation=%s depth=%d", entity, relation, depth)
    # TODO: implement actual KG walk
    from rag.advanced_rag.harness.tools.search import hybrid_search

    query = f"{entity} {relation or ''}"
    return await hybrid_search(tools, query=query.strip())


async def wiki_query(tools, topic: str) -> dict:
    """Query compiled wiki knowledge.

    This is currently a placeholder. The final implementation should call the
    compiled wiki store.
    """
    _LOG.info("wiki_query: topic=%s", topic)
    # TODO: implement actual wiki lookup
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=topic)
