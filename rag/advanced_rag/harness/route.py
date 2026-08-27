"""Route node — query classification (one-time, no KB dependency)."""

import json
import logging
import re

from rag.advanced_rag.harness.types import RouteDecision
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.prompts.route_prompt import ROUTE_PROMPT
from rag.advanced_rag.harness.stats import in_phase

_LOG = logging.getLogger(__name__)


def _extract_json(text: str) -> dict:
    """Extract a JSON *object* from the LLM response.

    Handles markdown fences and think tags, then attempts ``json_repair``
    followed by ``json.loads``. Robustness: the LLM can return a bare string,
    a list, or a JSON primitive — anything that is not a ``dict`` is coerced to
    ``{}`` (with a warning) so callers can safely use ``result.get(...)``. This
    guards against the crash seen in check.log where ``route_node`` received a
    string and ``result.get("question_type")`` raised AttributeError.
    """
    text = re.sub(r"^.*</think>", "", text, flags=re.DOTALL).strip()
    text = re.sub(r"```(?:json)?\s*|\s*```", "", text).strip()
    try:
        import json_repair

        parsed = json_repair.loads(text)
    except Exception:
        try:
            parsed = json.loads(text)
        except Exception:
            _LOG.warning("route: failed to parse LLM output: %s", text[:200])
            return {}
    if not isinstance(parsed, dict):
        _LOG.warning("route: LLM returned non-object JSON (%s), coercing to {}: %s", type(parsed).__name__, str(parsed)[:200])
        return {}
    return parsed


@in_phase("route")
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

    # Belt-and-suspenders: _extract_json already coerces to dict, but guard
    # here too so a future code path can never crash on result.get().
    if not isinstance(result, dict):
        _LOG.warning("route_node: result not a dict (%s); falling back to defaults", type(result).__name__)
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
