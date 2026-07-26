"""Exploration tools: knowledge graph and wiki lookup.

``graph_explore`` lives in :mod:`navigation` (it shares the compiled-structure
machinery) and is re-exported here so the tool registry keeps one import point.

``wiki_query`` retrieves through ``hybrid_search`` and takes the same
``keywords`` the other search tools do — the keywords drive query expansion and
the keyword-sentence narrowing. Parameter names must match the registered
``_search_schema`` (``query`` + ``keywords``), otherwise every LLM tool call
fails with a TypeError.
"""

import logging

# graph_explore is implemented alongside catalog/mindmap navigation because it
# reuses their outline-answering + evidence-pulling helpers.
from rag.advanced_rag.harness.tools.navigation import graph_explore  # noqa: F401

_LOG = logging.getLogger(__name__)


async def wiki_query(tools, query: str, keywords: str = "") -> dict:
    """Query compiled wiki knowledge.

    This is currently a placeholder. The final implementation should call the
    compiled wiki store.
    """
    _LOG.info(f'[Wiki lookup] Looking up the compiled wiki for "{query}" (keywords: {keywords})')
    # TODO: implement actual wiki lookup
    from rag.advanced_rag.harness.tools.search import hybrid_search

    return await hybrid_search(tools, query=query, keywords=keywords)
