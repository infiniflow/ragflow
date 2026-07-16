"""Tool system: register all tools with the registry on import."""

from rag.advanced_rag.harness.tools.registry import register_tool, _search_schema, _navigate_schema, _inspector_schema

# Register tools

# Search tools
from rag.advanced_rag.harness.tools.search import hybrid_search, vector_search, bm25_search, web_search, structured_query

register_tool("hybrid_search", _search_schema("hybrid_search", "Embedding + Keywords search"), hybrid_search)
register_tool("vector_search", _search_schema("vector_search", "Embedding search"), vector_search)
register_tool("bm25_search", _search_schema("bm25_search", "Keywords search"), bm25_search)
register_tool("web_search", _search_schema("web_search", "Internet search"), web_search)
register_tool("structured_query", _search_schema("structured_query", "SQL search"), structured_query)

# Navigation tools (require compilation)
from rag.advanced_rag.harness.tools.navigation import catalog_navigate, mindmap_navigate

# catalog_navigate covers both the tree/TOC outline and the page index — it reads
# whichever compiled catalog the doc has, so it is offered when either exists.
register_tool(
    "catalog_navigate",
    _navigate_schema("catalog_navigate", "Answer from the document's compiled catalog (table of contents / page index)"),
    catalog_navigate,
    requires_compilation=True,
    compilation_type=("toc", "page_index"),
)
register_tool("mindmap_navigate", _navigate_schema("mindmap_navigate", "Navigate by mindmap"), mindmap_navigate, requires_compilation=True, compilation_type="mindmap")

# Exploration tools (require compilation)
from rag.advanced_rag.harness.tools.exploration import graph_explore, wiki_query

register_tool("graph_explore", _search_schema("graph_explore", "Knowledge graph exploration"), graph_explore, requires_compilation=True, compilation_type="knowledge_graph")
register_tool("wiki_query", _search_schema("wiki_query", "Wiki search"), wiki_query, requires_compilation=True, compilation_type="wiki")

# Inspector tools
from rag.advanced_rag.harness.tools.inspector import open_context, compare_sources, grep_within, request_adjacent

register_tool(
    "inspector_open_context",
    _inspector_schema("open_context", "Expand the original context around a chunk", {"chunk_id": {"type": "string"}, "width": {"type": "integer", "description": "Number of characters to expand"}}),
    open_context,
)
register_tool("inspector_compare", _inspector_schema("compare_sources", "Compare multiple chunks to find common points and contradictions", {"chunk_ids": {"type": "array", "items": {"type": "string"}}}), compare_sources)
register_tool("inspector_grep_within", _inspector_schema("grep_within", "Search for an exact keyword or pattern within a document", {"doc_id": {"type": "string"}, "pattern": {"type": "string"}}), grep_within)
register_tool(
    "inspector_request_adjacent",
    _inspector_schema("request_adjacent", "Get adjacent entries before or after a chunk", {"chunk_id": {"type": "string"}, "direction": {"type": "string", "enum": ["prev", "next"]}, "count": {"type": "integer"}}),
    request_adjacent,
)

# Built-in agent tools
# (generate_report and think_tool are handled by the agent loop itself, not by Pipeline)
