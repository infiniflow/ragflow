"""Planner node — question-type-aware claim decomposition."""

import json
import logging
import re

from rag.advanced_rag.agentic_rag_graph import _snip
from rag.advanced_rag.harness.types import ClaimTarget, WorkflowPlan, RouteDecision
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.prompts.decompose_prompts import (
    DECOMPOSE_FACTUAL,
    DECOMPOSE_COMPARATIVE,
    DECOMPOSE_PROCEDURAL,
    DECOMPOSE_EXPLORATORY,
)

_LOG = logging.getLogger(__name__)


def _extract_json(text: str) -> dict:
    text = re.sub(r"^.*</think>", "", text, flags=re.DOTALL).strip()
    text = re.sub(r"```(?:json)?\s*|\s*```", "", text).strip()
    try:
        import json_repair

        return json_repair.loads(text)
    except Exception:
        try:
            return json.loads(text)
        except Exception:
            _LOG.warning("planner: failed to parse LLM output: %s", text[:200])
            return {}


async def planner_node(state: dict, tools) -> dict:
    """Planner node — decompose question into claims based on question type."""
    route: RouteDecision = state.get("route")
    if not route:
        _LOG.warning("planner: no route found, using defaults")
        return _default_plan(state.get("question", ""))

    _LOG.info("[Planner] Working out how to research this %s question: \"%s\"", route.question_type, _snip(route.question))
    if not route.requires_decomposition:
        # Direct mode: single coarse claim
        return _direct_plan(route.question)

    # Select decompose prompt by question type
    prompt_map = {
        "factual": DECOMPOSE_FACTUAL,
        "comparative": DECOMPOSE_COMPARATIVE,
        "procedural": DECOMPOSE_PROCEDURAL,
        "analytical": DECOMPOSE_EXPLORATORY,
        "exploratory": DECOMPOSE_EXPLORATORY,
    }
    decompose_prompt = prompt_map.get(route.question_type, DECOMPOSE_FACTUAL)

    mode = get_mode(route.thinking_mode)
    max_claims = _get_max_claims(mode.label)
    detail_level = _get_detail_level(mode.label)
    retrieved = _format_seed_chunks(state.get("seed_chunks"), tools)

    try:
        prompt = decompose_prompt.format(
            question=route.question,
            max_claims=max_claims,
            detail_level=detail_level,
            retrieved=retrieved,
        )
        system, user = prompt.split("Output format", 1)
        system = system.strip()
        user = "Output format" + user
        msg = await tools._fit_messages(system, user)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.2})
        if isinstance(ans, tuple):
            ans = ans[0]
        result = _extract_json(ans)
    except Exception:
        _LOG.exception("planner_node failed")
        return _direct_plan(route.question)

    claims_raw = result.get("claims", [])
    plan_type = {
        "factual": "fact_decomposition",
        "comparative": "comparative_decomposition",
        "procedural": "procedural_decomposition",
    }.get(route.question_type, "exploratory_decomposition")

    claims = []
    for i, c in enumerate(claims_raw):
        if isinstance(c, dict) and c.get("description"):
            claims.append(
                ClaimTarget(
                    claim_id=c.get("claim_id", f"c{i}"),
                    description=c["description"],
                    priority=c.get("priority", 0),
                    suggested_tools=c.get("suggested_tools", []),
                )
            )

    if not claims:
        return _direct_plan(route.question)

    plan = WorkflowPlan(
        plan_type=plan_type,
        claims=claims,
        max_iterations=mode.max_orchestrator_cycles,
    )
    _LOG.info("[Planner] Broke the question into %d research step(s): %s", len(plan.claims), "; ".join(f'"{c.description}"' for c in plan.claims))

    return {"plan": plan, "claims": plan.claims}


def _format_seed_chunks(seed_chunks, tools) -> str:
    """Render preliminary-search chunks as grounding context for the planner."""
    if not seed_chunks:
        return "(no preliminary results)"
    try:
        from rag.prompts.generator import kb_prompt

        blocks = kb_prompt({"chunks": seed_chunks, "doc_aggs": []}, tools.chat_mdl.max_length)
        text = "\n".join(blocks).strip()
        return text or "(no preliminary results)"
    except Exception:
        _LOG.exception("planner: failed to format seed chunks")
        return "(no preliminary results)"


def _direct_plan(question: str) -> dict:
    """Single-claim plan for non-decomposed mode."""
    plan = WorkflowPlan(
        plan_type="direct",
        claims=[ClaimTarget(claim_id="c0", description=question, priority=0)],
        max_iterations=1,
    )
    return {"plan": plan, "claims": plan.claims}


def _default_plan(question: str) -> dict:
    return _direct_plan(question)


def _get_max_claims(mode_label: str) -> int:
    return {"low": 1, "medium": 3, "high": 5, "ultra": 8}.get(mode_label, 3)


def _get_detail_level(mode_label: str) -> str:
    return {"low": "coarse", "medium": "normal", "high": "fine", "ultra": "extra_fine"}.get(mode_label, "normal")
