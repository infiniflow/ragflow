"""Medium mode: decompose → parallel search → sufficiency check."""

import asyncio
import logging

from rag.advanced_rag.harness.types import ClaimTarget, AgentResult, OrchestratorContext
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.sufficiency import (
    cross_check_claim,
    compute_fusion_score,
    route_sufficiency_verdict,
)
from rag.advanced_rag.harness.tools.search import hybrid_search

_LOG = logging.getLogger(__name__)


async def decompose_and_search(state: dict, tools) -> dict:
    """Decompose → parallel search → merge → sufficiency check → iterate."""
    question = state.get("question", "")
    keywords = state.get("keywords", "")
    claims_raw = state.get("claims", [])
    mode_label = state.get("route", {}).thinking_mode if state.get("route") else "medium"
    mode = get_mode(mode_label)

    claims = [ClaimTarget(**c) if isinstance(c, dict) else c for c in claims_raw]
    ctx = OrchestratorContext(question=question, claims=claims, mode=mode_label)

    for cycle in range(mode.max_orchestrator_cycles):
        ctx.iteration = cycle
        unverified = [c for c in ctx.claims if not c.is_verified]
        if not unverified:
            break

        # Parallel search on unverified claims
        tasks = []
        for c in unverified:
            tasks.append(hybrid_search(tools, query=c.description, keywords=keywords))
        results = await asyncio.gather(*tasks)

        for c, result in zip(unverified, results):
            if result.get("chunks"):
                _merge_kbinfos(tools, result)
                c.is_verified = True
                c.confidence = 0.8
                c.agent_result = AgentResult(
                    claim_id=c.claim_id,
                    report=_summarize(result),
                    is_verified=True,
                    confidence=0.8,
                    evidence_ids=_global_evidence_ids(tools, result),
                )
            else:
                c.agent_result = AgentResult(
                    claim_id=c.claim_id,
                    report="",
                    is_verified=False,
                    confidence=0.0,
                )

        all_chunks = {i: c for i, c in enumerate(tools.kbinfos.get("chunks", []))}
        agent_results = [c.agent_result for c in ctx.claims if c.agent_result]
        cross_results = [cross_check_claim(r, all_chunks) for r in agent_results]

        verdict = compute_fusion_score(agent_results, cross_results, mode)

        action, should_continue = route_sufficiency_verdict(
            verdict,
            mode_label,
            cycle,
            mode.max_orchestrator_cycles,
        )

        if action in ("ANSWER", "ANSWER_PARTIAL"):
            return {
                "verdict": verdict.__dict__,
                "partial_answer": action == "ANSWER_PARTIAL",
                "kbinfos": tools.kbinfos,
            }
        if action == "ABSTAIN":
            tools.kbinfos["chunks"] = []
            return {"verdict": verdict.__dict__, "abstain": True}

    # Cycle exhaustion: flag the answer as partial so the final-answer node
    # prepends the partial-answer preamble instead of presenting an incomplete
    # answer as complete. Partial when (a) some claim is still unverified, or
    # (b) every claim is verified but the final verdict is not SUFFICIENT (e.g.
    # cross-check flagged conflicts/mismatches) — in both cases an exhaustive
    # answer was not reached.
    verdict_status = getattr(verdict, "status", None)
    partial = (any(not c.is_verified for c in ctx.claims) or (verdict_status is not None and verdict_status != "SUFFICIENT")) and bool(tools.kbinfos.get("chunks"))
    return {"kbinfos": tools.kbinfos, "partial_answer": partial}


def _merge_kbinfos(tools, result: dict):
    if not result or not result.get("chunks"):
        return
    seen = {_chunk_key(c) for c in tools.kbinfos.get("chunks", [])}
    for c in result.get("chunks", []):
        k = _chunk_key(c)
        if k in seen:
            continue
        seen.add(k)
        tools.kbinfos.setdefault("chunks", []).append(c)
    dseen = {d.get("doc_id") for d in tools.kbinfos.get("doc_aggs", [])}
    for d in result.get("doc_aggs", []):
        if d.get("doc_id") in dseen:
            continue
        dseen.add(d.get("doc_id"))
        tools.kbinfos.setdefault("doc_aggs", []).append(d)


def _chunk_key(ck: dict) -> str:
    return ck.get("chunk_id") or ck.get("id") or str(id(ck))


def _global_evidence_ids(tools, result: dict) -> list[int]:
    """Map a search result's chunks to their indices in ``tools.kbinfos``.

    The cross-check resolves evidence IDs against the shared kbinfos pool, so
    the IDs must be global indices there — not positions within this result.
    Must be called AFTER ``_merge_kbinfos`` so fresh chunks have indices.
    """
    index_by_key: dict[str, int] = {}
    for idx, ck in enumerate(tools.kbinfos.get("chunks", [])):
        index_by_key.setdefault(_chunk_key(ck), idx)
    ids: list[int] = []
    for ck in result.get("chunks", []):
        idx = index_by_key.get(_chunk_key(ck))
        if idx is not None and idx not in ids:
            ids.append(idx)
    return ids


def _summarize(result: dict) -> str:
    chunks = result.get("chunks", [])
    texts = [c.get("content_with_weight", "")[:200] for c in chunks[:3]]
    return " | ".join(texts)
