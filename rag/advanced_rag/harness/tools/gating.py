"""Tool selection gating: phase-based filtering + fallback chain."""

from rag.advanced_rag.harness.types import OrchestratorContext
from rag.advanced_rag.harness.tools.registry import TOOL_REGISTRY


# ── Search phase definitions ──

SEARCH_PHASES = {
    "locate": {
        "goal": "定位——找到可能包含答案的文档/区域",
        "tools_priority": [
            "toc_navigate",
            "mindmap_navigate",
            "page_index_navigate",
            "wiki_query",
            "hybrid_search",
            "bm25_search",
        ],
        "max_returned": 5,
        "tool_hint": "优先使用导航工具定位文档区域，而不是直接搜索关键词",
    },
    "explore": {
        "goal": "探索——在已定位的区域内深入搜索",
        "tools_priority": [
            "hybrid_search",
            "vector_search",
            "bm25_search",
            "graph_explore",
            "inspector_open_context",
            "inspector_request_adjacent",
        ],
        "max_returned": 4,
        "tool_hint": "优先使用检索工具在已定位区域内获取详细信息",
    },
    "verify": {
        "goal": "验证——确认多个来源的一致性",
        "tools_priority": [
            "inspector_open_context",
            "inspector_compare",
            "inspector_grep_within",
            "hybrid_search",
            "web_search",
        ],
        "max_returned": 4,
        "tool_hint": "优先使用 Inspector 工具对比已有证据，而非搜索新内容",
    },
    "cross_domain": {
        "goal": "跨域——探索已发现实体的跨领域关联",
        "tools_priority": [
            "graph_explore",
            "wiki_query",
            "hybrid_search",
            "web_search",
        ],
        "max_returned": 3,
        "tool_hint": "优先在图谱上行走发现跨领域关联",
    },
}


def compilation_available(tool_name: str, compilation_map: dict) -> bool:
    """Check if any KB provides the required compilation artifact."""
    tool = TOOL_REGISTRY.get(tool_name)
    if not tool or not tool.get("requires_compilation"):
        return True
    comp_type = tool["compilation_type"]
    if not compilation_map:
        return False
    return any(comp_type in comps for comps in compilation_map.values())


def tool_fits_context(tool_name: str, context: OrchestratorContext) -> bool:
    """Check if a tool is sensible given current search context."""
    if tool_name.startswith("inspector_") and not context.has_any_chunks():
        return False
    if tool_name == "toc_navigate" and not context.current_claim:
        return False
    if tool_name == "graph_explore" and not context.last_entity:
        return False
    if tool_name == "mindmap_navigate" and not context.current_claim:
        return False
    return True


def get_gated_tools(
    phase: str,
    available_tools: list[str],
    compilation_map: dict[str, set[str]],
    context: OrchestratorContext,
) -> list[dict]:
    """Filter, sort, and gate tools by phase priority + context."""
    phase_config = SEARCH_PHASES.get(phase)
    if not phase_config:
        return _default_defs(available_tools)

    sorted_tools = []
    for tool_name in phase_config["tools_priority"]:
        if tool_name not in available_tools:
            continue
        if not compilation_available(tool_name, compilation_map):
            continue
        if not tool_fits_context(tool_name, context):
            continue
        sorted_tools.append(tool_name)

    selected = sorted_tools[: phase_config["max_returned"]]
    defs = [TOOL_REGISTRY[n]["function_schema"] for n in selected if n in TOOL_REGISTRY]
    for d in defs:
        d["x_phase"] = phase
        d["x_phase_hint"] = phase_config["tool_hint"]
    return defs


def _default_defs(tool_names: list[str]) -> list[dict]:
    return [TOOL_REGISTRY[n]["function_schema"] for n in tool_names if n in TOOL_REGISTRY]


def determine_current_phase(context: OrchestratorContext) -> str:
    """Determine the current search phase based on context."""
    if not context.has_any_chunks():
        return "locate"
    if context.verdict and context.verdict.has_conflicts:
        return "verify"
    return "explore"
