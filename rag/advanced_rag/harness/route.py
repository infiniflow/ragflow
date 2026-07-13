"""Route node — query classification (one-time, no KB dependency)."""

import json
import logging
import re

from rag.advanced_rag.harness.types import RouteDecision
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.prompts.route_prompt import ROUTE_PROMPT

_LOG = logging.getLogger(__name__)


def _extract_json(text: str) -> dict:
    """Extract JSON from LLM response, handling markdown fences and think tags."""
    text = re.sub(r"^.*</think>", "", text, flags=re.DOTALL).strip()
    text = re.sub(r"```(?:json)?\s*|\s*```", "", text).strip()
    try:
        import json_repair

        return json_repair.loads(text)
    except Exception:
        try:
            return json.loads(text)
        except Exception:
            _LOG.warning("route: failed to parse LLM output: %s", text[:200])
            return {}


async def route_node(state: dict, tools) -> dict:
    """Route node — analyze the question, produce RouteDecision."""
    question = state.get("question", "")
    if not question:
        return _fallback_route(question)

    mode_label = getattr(tools, "thinking_mode", "medium")
    mode = get_mode(mode_label)

    try:
        system = ROUTE_PROMPT.format(question=question)
        msg = await tools._fit_messages(system, question)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.1})
        if isinstance(ans, tuple):
            ans = ans[0]
        result = _extract_json(ans)
    except Exception:
        _LOG.exception("route_node failed")
        result = {}

    question_type = result.get("question_type", "factual")
    requires_decomp = result.get("requires_decomposition", True)
    suggests_comp = result.get("suggests_compilation")

    route = RouteDecision(
        question=question,
        thinking_mode=mode_label,
        question_type=question_type,
        requires_decomposition=mode.requires_decomposition and requires_decomp,
        suggests_compilation=suggests_comp,
        execution_strategy=mode.execution_strategy,
        reasoning=result.get("reasoning", ""),
    )

    return {"route": route}


def _fallback_route(question: str) -> dict:
    route = RouteDecision(
        question=question,
        thinking_mode="medium",
        question_type="factual",
        requires_decomposition=False,
        suggests_compilation=None,
        execution_strategy="direct_search",
        reasoning="fallback: empty question",
    )
    return {"route": route}
