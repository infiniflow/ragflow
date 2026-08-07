"""Tool selection gating: phase-based filtering and fallback chain."""

from rag.advanced_rag.harness.types import OrchestratorContext
from rag.advanced_rag.harness.tools.registry import TOOL_REGISTRY


# Search phase definitions

SEARCH_PHASES = {
    "locate": {
        "goal": "Locate documents or regions that may contain the answer.",
        "tools_priority": [
            "dataset_navigation_by_tree",
            "ontology_navigate",
            "mindmap_navigate",
            "hybrid_search",
            "bm25_search",
            "wiki_query",
        ],
        "max_returned": 5,
        "tool_hint": "Prefer navigation tools to locate document regions before directly searching keywords.",
    },
    "explore": {
        "goal": "Explore deeply within the already located region.",
        "tools_priority": [
            "hybrid_search",
            "bm25_search",
            "web_search",
            "graph_explore",
            "inspector_open_context",
            "inspector_request_adjacent",
        ],
        "max_returned": 4,
        "tool_hint": "Prefer retrieval tools to gather detailed information within the located region; use web_search when the knowledge base lacks the answer or the question needs current/external facts.",
    },
    "verify": {
        "goal": "Verify consistency across multiple sources.",
        "tools_priority": [
            "inspector_open_context",
            "inspector_compare",
            "inspector_grep_within",
            "web_search",
            "hybrid_search",
        ],
        "max_returned": 4,
        "tool_hint": "Prefer inspector tools to compare existing evidence; use web_search to corroborate against external sources.",
    },
    "cross_domain": {
        "goal": "Explore cross-domain relationships for discovered entities.",
        "tools_priority": [
            "graph_explore",
            "wiki_query",
            "hybrid_search",
            "web_search",
        ],
        "max_returned": 3,
        "tool_hint": "Prefer walking the graph to discover cross-domain relationships.",
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
    comp_types = set(comp_type) if isinstance(comp_type, (list, tuple, set)) else {comp_type}
    return any(bool(comp_types & set(comps)) for comps in compilation_map.values())


def tool_fits_context(tool_name: str, context: OrchestratorContext, has_routed_scope: bool = False) -> bool:
    """Check if a tool is sensible given current search context."""
    if tool_name.startswith("inspector_") and not context.has_any_chunks():
        return False
    if tool_name in {"ontology_navigate", "mindmap_navigate"} and not has_routed_scope:
        return False
    if tool_name == "dataset_navigation_by_tree" and not context.current_claim:
        return False
    if tool_name == "graph_explore" and not context.last_entity:
        return False
    return True


def get_gated_tools(
    phase: str,
    available_tools: list[str],
    compilation_map: dict[str, set[str]],
    context: OrchestratorContext,
    has_routed_scope: bool = False,
    web_enabled: bool = True,
) -> list[dict]:
    """Filter, sort, and gate tools by phase priority and context."""
    phase_config = SEARCH_PHASES.get(phase)
    if not phase_config:
        return _default_defs(available_tools, web_enabled)

    sorted_tools = []
    for tool_name in phase_config["tools_priority"]:
        if tool_name not in available_tools:
            continue
        if tool_name == "web_search" and not web_enabled:
            # No web provider configured — don't bind a tool that no-ops.
            continue
        if not compilation_available(tool_name, compilation_map):
            continue
        if not tool_fits_context(tool_name, context, has_routed_scope):
            continue
        sorted_tools.append(tool_name)

    selected = sorted_tools[: phase_config["max_returned"]]
    # Copy the registry schemas before annotating — the registry dicts are
    # shared process-wide, mutating them would leak phase hints across
    # concurrent requests.
    defs = []
    for n in selected:
        if n in TOOL_REGISTRY:
            defs.append({**TOOL_REGISTRY[n]["function_schema"], "x_phase": phase, "x_phase_hint": phase_config["tool_hint"]})
    return defs


def _default_defs(tool_names: list[str], web_enabled: bool = True) -> list[dict]:
    return [TOOL_REGISTRY[n]["function_schema"] for n in tool_names if n in TOOL_REGISTRY and (web_enabled or n != "web_search")]


def determine_current_phase(context: OrchestratorContext) -> str:
    """Determine the current search phase based on context."""
    if not context.has_any_chunks():
        return "locate"
    if context.verdict and context.verdict.has_conflicts:
        return "verify"
    return "explore"
