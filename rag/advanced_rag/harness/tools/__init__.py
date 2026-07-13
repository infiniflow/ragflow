"""Tool system — register all tools with the registry on import."""

from rag.advanced_rag.harness.tools.registry import register_tool, _search_schema, _navigate_schema, _inspector_schema

# ── Register tools ──

# Search tools
from rag.advanced_rag.harness.tools.search import hybrid_search, vector_search, bm25_search, web_search, structured_query

register_tool("hybrid_search", _search_schema("hybrid_search", "向量+关键词混合检索"), hybrid_search)
register_tool("vector_search", _search_schema("vector_search", "纯语义检索"), vector_search)
register_tool("bm25_search", _search_schema("bm25_search", "纯关键词检索"), bm25_search)
register_tool("web_search", _search_schema("web_search", "互联网搜索"), web_search)
register_tool("structured_query", _search_schema("structured_query", "结构化KB SQL查询"), structured_query)

# Navigation tools (require compilation)
from rag.advanced_rag.harness.tools.navigation import toc_navigate, page_index_navigate, mindmap_navigate

register_tool("toc_navigate", _navigate_schema("toc_navigate", "按文档目录结构定位"), toc_navigate, requires_compilation=True, compilation_type="toc")
register_tool("page_index_navigate", _navigate_schema("page_index_navigate", "按文档页面索引导航"), page_index_navigate, requires_compilation=True, compilation_type="page_index")
register_tool("mindmap_navigate", _navigate_schema("mindmap_navigate", "按概念脑图定位"), mindmap_navigate, requires_compilation=True, compilation_type="mindmap")

# Exploration tools (require compilation)
from rag.advanced_rag.harness.tools.exploration import graph_explore, wiki_query

register_tool("graph_explore", _search_schema("graph_explore", "知识图谱关联探索"), graph_explore, requires_compilation=True, compilation_type="knowledge_graph")
register_tool("wiki_query", _search_schema("wiki_query", "编译的领域知识查询"), wiki_query, requires_compilation=True, compilation_type="wiki")

# Inspector tools
from rag.advanced_rag.harness.tools.inspector import open_context, compare_sources, grep_within, request_adjacent

register_tool(
    "inspector_open_context",
    _inspector_schema("open_context", "展开一条 chunk 周围的原文上下文", {"chunk_id": {"type": "string"}, "width": {"type": "integer", "description": "扩展字符数"}}),
    open_context,
)
register_tool("inspector_compare", _inspector_schema("compare_sources", "对比多个 chunk 找出共同点和矛盾点", {"chunk_ids": {"type": "array", "items": {"type": "string"}}}), compare_sources)
register_tool("inspector_grep_within", _inspector_schema("grep_within", "在文档内精确搜索关键词", {"doc_id": {"type": "string"}, "pattern": {"type": "string"}}), grep_within)
register_tool(
    "inspector_request_adjacent",
    _inspector_schema("request_adjacent", "获取 chunk 前后的相邻条目", {"chunk_id": {"type": "string"}, "direction": {"type": "string", "enum": ["prev", "next"]}, "count": {"type": "integer"}}),
    request_adjacent,
)

# Built-in agent tools
# (generate_report and think_tool are handled by the agent loop itself, not by Pipeline)
