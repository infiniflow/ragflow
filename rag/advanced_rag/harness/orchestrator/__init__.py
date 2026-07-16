"""Orchestrator loop — dispatches to execution strategy based on thinking mode."""

import logging

from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.orchestrator.direct import direct_search
from rag.advanced_rag.harness.orchestrator.decompose import decompose_and_search
from rag.advanced_rag.harness.orchestrator.agentic import agentic_research

_LOG = logging.getLogger(__name__)


async def orchestrator_loop(state: dict, tools) -> dict:
    """Main orchestrator — dispatch to strategy based on thinking mode."""
    route = state.get("route")
    if not route:
        _LOG.warning("orchestrator: no route, using direct_search")
        return await direct_search(state, tools)

    mode_label = route.thinking_mode if isinstance(route, dict) else route.thinking_mode
    mode = get_mode(mode_label)

    _LOG.info("[Orchestrator] Researching with the \"%s\" approach (%s thinking).", mode.execution_strategy, mode_label)

    if mode.execution_strategy == "direct_search":
        return await direct_search(state, tools)

    if mode.execution_strategy == "decompose_and_search":
        return await decompose_and_search(state, tools)

    if mode.execution_strategy in ("agentic_research", "deep_research"):
        return await agentic_research(state, tools)

    _LOG.warning("orchestrator: unknown strategy %s, fallback to direct", mode.execution_strategy)
    return await direct_search(state, tools)
