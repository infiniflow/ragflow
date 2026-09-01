"""Retrieval and navigation tool implementations.

This package holds the *implementations* the action-session runtime calls:

* :mod:`search` — hybrid / BM25 / grep retrieval plus compiled-structure expansion.
* :mod:`navigation` — dataset navigation tree, document structure navigation and
  knowledge-graph exploration.
* :mod:`exploration` — re-exports ``graph_explore`` and hosts ``wiki_query``.

Tool *schemas* and per-turn dispatch live in
:mod:`rag.advanced_rag.harness.action_session` (``_TOOL_MAP`` / ``execute_tool``),
which is what the model actually sees.

Note: a declarative ``TOOL_REGISTRY`` (registry.py + gating.py + pipeline.py)
previously lived here and registered 17 tools on import. It was never read by any
live path — the only consumers were each other and a pipeline with no external
callers — while advertising tool names (``ontology_navigate``, ``mindmap_navigate``)
that no longer existed in the code. It has been removed; ``_active_tool_specs``
is now the single place that decides the visible tool surface.
"""
