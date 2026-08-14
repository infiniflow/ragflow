"""Planner node — question-type-aware claim decomposition."""

import json
import logging
import re

from common.token_utils import num_tokens_from_string
from rag.advanced_rag.agentic_rag_graph import _snip
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.prompts.decompose_prompts import DECOMPOSE_UNIFIED
from rag.advanced_rag.harness.stats import in_phase
from rag.advanced_rag.harness.types import ClaimTarget, RouteDecision, WorkflowPlan

_LOG = logging.getLogger(__name__)


def _extract_json(text: str) -> dict:
    text = re.sub(r"^.*</think>", "", text, flags=re.DOTALL).strip()
    text = re.sub(r"```(?:json)?\s*|\s*```", "", text).strip()
    try:
        import json_repair

        parsed = json_repair.loads(text)
    except Exception:
        try:
            parsed = json.loads(text)
        except Exception:
            _LOG.warning("planner: failed to parse LLM output: %s", text[:200])
            return {}
    if not isinstance(parsed, dict):
        # json_repair can return a bare string / list / primitive when the LLM
        # emits non-object JSON. Coerce to {} so callers can safely use
        # ``result.get("claims")`` — otherwise ``planner_node`` crashes with
        # AttributeError and the whole agentic graph dies (rag tool returns
        # "internal error"), producing 0-score answers. Mirrors route.py.
        _LOG.warning(
            "planner: LLM returned non-object JSON (%s), coercing to {}: %s",
            type(parsed).__name__,
            str(parsed)[:200],
        )
        return {}
    return parsed


@in_phase("planner")
async def planner_node(state: dict, tools) -> dict:
    """Planner node — decompose question into claims based on question type."""
    route: RouteDecision = state.get("route")
    if not route:
        _LOG.warning("planner: no route found, using defaults")
        return _default_plan(state.get("question", ""))

    _LOG.info('[Planner] Working out how to research this %s question: "%s"', route.question_type, _snip(route.question))
    if not route.requires_decomposition:
        # Direct mode: single coarse claim
        return _direct_plan(route.question)

    # Unified structure-aware decomposition: ONE prompt asks the LLM to judge the
    # question's reasoning structure (flat / chain / aggregate / temporal) and
    # emit typed claims, instead of a brittle regex picking a per-type template.
    # This covers aggregate (Q90: average across all members), temporal (Q678:
    # cross-year link), chain (Q309: bridge entity) and flat — none of which a
    # keyword regex could reliably detect.
    decompose_prompt = DECOMPOSE_UNIFIED
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
        # Replanning: the orchestrator sets ``feedback`` from the sufficiency
        # verdict — the new plan must close those gaps instead of repeating
        # the previous one.
        feedback = (state.get("feedback") or "").strip()
        if feedback:
            system += (
                "\n\nA previous research round already ran and left gaps. "
                "Feedback from the sufficiency check — the new plan MUST address "
                "these points with different, more targeted claims:\n" + feedback
            )
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
                    prerequisite=str(c.get("prerequisite") or "").strip(),
                    claim_type=str(c.get("claim_type") or "flat").strip().lower() or "flat",
                    target=str(c.get("target") or "").strip(),
                )
            )

    if not claims:
        return _direct_plan(route.question)

    # Post-validation: guarantee aggregate questions carry an enumeration +
    # combine structure (the LLM may split aggregation into per-member claims or
    # emit the enumeration without the combine step). Back-fills missing claims.
    claims = _ensure_aggregate_structure(route.question, claims)

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
        _LOG.info("[Planner] No preliminary passages — grounding the plan without seed chunks.")
        return "(no preliminary results)"
    try:
        from rag.prompts.generator import kb_prompt

        blocks = kb_prompt({"chunks": seed_chunks, "doc_aggs": []}, tools.chat_mdl.max_length)
        text = "\n".join(blocks).strip()
        if not text:
            return "(no preliminary results)"
        _LOG.info(
            "[Planner] Grounding the plan with %d preliminary passage(s) (~%d tokens of grounding context).",
            len(seed_chunks),
            num_tokens_from_string(text),
        )
        return text
    except Exception:
        _LOG.exception("planner: failed to format seed chunks")
        return "(no preliminary results)"


# ═══════════════════════════════════════════════════════════════
# Aggregate structure enforcement (planner post-validation)
#
# The unified decompose prompt asks the LLM to emit an aggregate claim (a single
# ENUMERATION over ALL members) PLUS a combine claim (the count/sum/average/
# max/min over the enumerated values). The LLM does not always honour this:
# it may split into independent per-member claims (which can never be
# aggregated), emit the enumeration without the combine step, or miss the
# aggregate shape entirely. This post-validation guarantees an aggregate+
# combine structure whenever the question signals aggregation, so exhaustive
# enumeration is actually performed and then combined.
# ═══════════════════════════════════════════════════════════════

_AGGREGATE_RE = re.compile(
    r"\b(how many|how much|average|mean|sum|total|count|every|all|most|"
    r"maximum|minimum|max|min|combined|overall|each|per)\b"
    r"|aggregate|合计|平均|总共|所有|每个|一共",
    re.IGNORECASE,
)


def _is_aggregate_question(question: str) -> bool:
    return bool(_AGGREGATE_RE.search(question or ""))


def _ensure_aggregate_structure(question: str, claims: list) -> list:
    """Guarantee an aggregate claim + a combine claim for aggregate questions.

    If the question signals aggregation but the plan lacks an ``aggregate``
    enumeration claim (with a non-empty ``target``), back-fill one whose
    description is the question itself and whose target is the full member set.
    If an enumeration claim exists but no combine claim follows it, back-fill a
    combine claim that asks for the aggregate over the enumerated values. The
    fallback claims are appended with a stable ``claim_id`` so existing claim
    references never break.
    """
    if not _is_aggregate_question(question):
        return claims
    existing_desc = {c.description.strip().lower() for c in claims if getattr(c, "description", None)}
    existing_ids = {c.claim_id for c in claims if getattr(c, "claim_id", None)}
    next_id = 0
    while f"c{next_id}" in existing_ids:
        next_id += 1

    def _new_claim_id() -> str:
        nonlocal next_id
        cid = f"c{next_id}"
        next_id += 1
        return cid

    enumerator = next((c for c in claims if getattr(c, "claim_type", "") == "aggregate"), None)

    if enumerator is None:
        # No enumeration claim — add one covering the full member set so an
        # aggregate question actually enumerates all members instead of being
        # answered from a single chunk.
        desc = (question or "").strip()
        enumerator = ClaimTarget(
            claim_id=_new_claim_id(),
            description=desc,
            claim_type="aggregate",
            target=desc or "all relevant members",
            priority=1,
        )
        claims.append(enumerator)
        existing_desc.add(desc.lower())
        _LOG.info("[Planner] Post-validation: back-filled aggregate enumeration claim %s (target=%r)", enumerator.claim_id, enumerator.target)
    elif not (enumerator.target or "").strip():
        # Enumeration claim exists but has no member set — default it so the
        # exhaustive-retrieval path knows what to enumerate.
        enumerator.target = (question or "").strip() or "all relevant members"
        _LOG.info("[Planner] Post-validation: set aggregate claim %s target=%r", enumerator.claim_id, enumerator.target)

    # A combine claim must follow the enumeration: the aggregate value is
    # computed over the enumerated member values. Detect by a claim that is
    # distinct from the enumerator and whose description mentions an aggregate
    # operation OR a combining word over "those/every/all/them/each".
    def _looks_like_combine(c) -> bool:
        desc = (getattr(c, "description", "") or "").strip().lower()
        return bool(re.search(r"\b(average|mean|sum|total|count|maximum|minimum|max|min|combined|overall)\b", desc))

    has_combine = any(
        c.claim_id != enumerator.claim_id
        and _looks_like_combine(c)
        and any(w in (getattr(c, "description", "") or "").lower() for w in ("those", "every", "all", "them", "each", "listed", "above", "these"))
        for c in claims
    )
    if not has_combine:
        combine_desc = f"Combine the enumerated values from {enumerator.claim_id} into the final aggregate answer."
        claims.append(
            ClaimTarget(
                claim_id=_new_claim_id(),
                description=combine_desc,
                claim_type="flat",
                priority=2,
            )
        )
        _LOG.info("[Planner] Post-validation: back-filled combine claim over enumeration claim %s", enumerator.claim_id)
    return claims


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
