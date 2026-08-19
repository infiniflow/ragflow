"""Tool system: register all tools with the registry on import."""

from rag.advanced_rag.harness.tools.registry import _inspector_schema, _navigate_schema, _search_schema, register_tool

# Register tools
# Search tools
from rag.advanced_rag.harness.tools.search import bm25_search, hybrid_search, structured_query, vector_search, web_search

register_tool("hybrid_search", _search_schema("hybrid_search", "Embedding + Keywords search"), hybrid_search)
register_tool("vector_search", _search_schema("vector_search", "Embedding search"), vector_search)
register_tool("bm25_search", _search_schema("bm25_search", "Keywords search"), bm25_search)
register_tool("web_search", _search_schema("web_search", "Internet search"), web_search)
register_tool("structured_query", _search_schema("structured_query", "SQL search"), structured_query)

# Navigation tools (require compilation)
from rag.advanced_rag.harness.tools.navigation import dataset_navigation_search, mindmap_navigate, ontology_navigate

# ontology_navigate covers both the tree/TOC outline and the page index.
register_tool(
    "ontology_navigate",
    _navigate_schema("ontology_navigate", "Get question-related chunks from the given documents catalog (tree / page index / mind map). It always follows dataset navigation."),
    ontology_navigate,
    requires_compilation=True,
    compilation_type=("toc", "page_index", "mindmap"),
)
register_tool(
    "mindmap_navigate", _navigate_schema("mindmap_navigate", "Get question-related chunks from the document's  mindmap"), mindmap_navigate, requires_compilation=True, compilation_type="mindmap"
)
register_tool(
    "dataset_navigation_search",
    _navigate_schema(
        "dataset_navigation_search",
        "Find the documents most relevant to the question by hybrid-searching the dataset's navigation-tree document leaves (nav_doc layer) — only documents that have a compiled nav-tree node are reachable. Returns the most relevant document IDs.",
    ),
    dataset_navigation_search,
    requires_compilation=True,
    compilation_type="tree",
)

# Exploration tools (require compilation)
from rag.advanced_rag.harness.tools.exploration import graph_explore, wiki_query

register_tool("graph_explore", _search_schema("graph_explore", "Knowledge graph exploration"), graph_explore, requires_compilation=True, compilation_type="knowledge_graph")
register_tool("wiki_query", _search_schema("wiki_query", "Wiki search"), wiki_query, requires_compilation=True, compilation_type="wiki")

# Inspector tools
from rag.advanced_rag.harness.tools.inspector import compare_sources, grep_within, open_context, request_adjacent

register_tool(
    "inspector_open_context",
    _inspector_schema("open_context", "Expand the original context around a chunk", {"chunk_id": {"type": "string"}, "width": {"type": "integer", "description": "Number of characters to expand"}}),
    open_context,
)
register_tool(
    "inspector_compare",
    _inspector_schema("compare_sources", "Compare multiple chunks to find common points and contradictions", {"chunk_ids": {"type": "array", "items": {"type": "string"}}}),
    compare_sources,
)
register_tool(
    "inspector_grep_within", _inspector_schema("grep_within", "Search for an exact keyword or pattern within a document", {"doc_id": {"type": "string"}, "pattern": {"type": "string"}}), grep_within
)
register_tool(
    "inspector_request_adjacent",
    _inspector_schema(
        "request_adjacent", "Get adjacent entries before or after a chunk", {"chunk_id": {"type": "string"}, "direction": {"type": "string", "enum": ["prev", "next"]}, "count": {"type": "integer"}}
    ),
    request_adjacent,
)

# Built-in agent tools
# (generate_report and think_tool are handled by the agent loop itself, not by Pipeline)
