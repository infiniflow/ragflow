"""Medium mode: decompose → parallel search → sufficiency check."""

import asyncio
import logging
from dataclasses import replace

from rag.advanced_rag.harness.types import ClaimTarget, AgentResult, OrchestratorContext
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.sufficiency import (
    cross_check_claim,
    compute_fusion_score,
    route_sufficiency_verdict,
)
from rag.advanced_rag.harness.orchestrator.sufficiency_llm import llm_sufficiency_boost
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

    # Stagnation guard: stop when the fusion score stops improving across
    # consecutive rounds (corpus lacks the data, follow-ups keep returning
    # nothing) instead of burning the remaining cycle budget unproductively.
    prev_score: float | None = None
    _STAGNATION_CYCLES = 2
    _STAGNATION_GAIN = 0.05

    for cycle in range(mode.max_orchestrator_cycles):
        ctx.iteration = cycle
        unverified = [c for c in ctx.claims if not c.is_verified]

        # Consume Phase-2 follow-up queries (missing-pieces feedback): run
        # targeted searches for the flagged gaps in addition to the claim
        # descriptions, so the next round closes the actual evidence gap
        # instead of re-searching the same broad claims.
        followup_queries: list[str] = []
        if ctx.pending_followups:
            followup_queries = [str(q.get("query") or q.get("question") or "") for q in ctx.pending_followups if q]
            followup_queries = [q for q in followup_queries if q.strip()]
            ctx.pending_followups = []
            if followup_queries:
                _LOG.info("[Decompose] Round %d: searching %d follow-up gap query(ies): %s", cycle + 1, len(followup_queries), followup_queries)

        if not unverified and not followup_queries:
            break

        # Parallel search on unverified claims + follow-up gap queries.
        search_items = [c.description for c in unverified] + followup_queries
        tasks = [hybrid_search(tools, query=q, keywords=keywords) for q in search_items]
        results = await asyncio.gather(*tasks)

        # Follow-up search results are merged into kbinfos (extra evidence) but
        # don't create fake claim verifications.
        n_claims = len(unverified)
        for c, result in zip(unverified, results[:n_claims]):
            if result.get("chunks"):
                _merge_kbinfos(tools, result)
                c.is_verified = True
                # Signal A: retrieval-confidence (mean similarity of top-3 hits),
                # not a hard-coded 0.8 — reflects how strongly the search actually
                # matched the claim. Zero LLM cost (medium has no agent loop).
                c.confidence = _retrieval_confidence(result)
                c.agent_result = AgentResult(
                    claim_id=c.claim_id,
                    # Signal B: verify the *claim's* required facts (numbers /
                    # entities from its description) against the retrieved
                    # chunks, instead of the old self-consistent truncated
                    # summary (which trivially matched its own source text).
                    report=c.description or "",
                    is_verified=True,
                    confidence=c.confidence,
                    evidence_ids=_global_evidence_ids(tools, result),
                )
            else:
                c.agent_result = AgentResult(
                    claim_id=c.claim_id,
                    report="",
                    is_verified=False,
                    confidence=0.0,
                )
        # Merge follow-up gap-search evidence (extra evidence for the answer),
        # without fabricating a claim verification for it.
        for result in results[n_claims:]:
            if result.get("chunks"):
                _merge_kbinfos(tools, result)

        all_chunks = {i: c for i, c in enumerate(tools.kbinfos.get("chunks", []))}
        agent_results = [c.agent_result for c in ctx.claims if c.agent_result]
        cross_results = [cross_check_claim(r, all_chunks) for r in agent_results]

        verdict = compute_fusion_score(
            agent_results,
            cross_results,
            mode,
            question=ctx.question,
            claims=ctx.claims,
            all_chunks=all_chunks,
        )

        # Phase-2: LLM Sufficient Context AutoRater fallback on ambiguous
        # verdicts — confirm sufficiency when the code signals are unclear.
        if verdict.status in ("USEFUL_BUT_INCOMPLETE", "INSUFFICIENT", "CONFLICTING"):
            cited_ids: list[str] = []
            for r in agent_results:
                cited_ids.extend(r.evidence_ids or [])
            boost = await llm_sufficiency_boost(tools, ctx.question, verdict, evidence_ids=cited_ids)
            if boost and boost.get("is_sufficient"):
                verdict = replace(verdict, status="SUFFICIENT", score=max(verdict.score, mode.sufficiency_threshold))
                _LOG.info("[Decompose] Round %d: LLM AutoRater confirms SUFFICIENT → upgraded", cycle + 1)
            elif boost and boost.get("followups"):
                ctx.pending_followups = boost.get("followups", [])
                _LOG.info("[Decompose] Stored %d follow-up query(ies) for next round.", len(ctx.pending_followups))

        action, should_continue = route_sufficiency_verdict(
            verdict,
            mode_label,
            cycle,
            mode.max_orchestrator_cycles,
        )

        # Stagnation guard (same rationale as agentic.py): no meaningful score
        # gain for a couple of rounds → emit a partial answer instead of
        # looping through the remaining cycles on unproductive searches.
        if should_continue and verdict.status in ("INSUFFICIENT", "USEFUL_BUT_INCOMPLETE"):
            if prev_score is not None and cycle >= _STAGNATION_CYCLES and verdict.score - prev_score < _STAGNATION_GAIN:
                _LOG.info(
                    "[Decompose] Round %d: score stagnant (%.3f → %.3f) — early-stopping to partial answer",
                    cycle + 1,
                    prev_score,
                    verdict.score,
                )
                action = "ANSWER_PARTIAL"
                should_continue = False
            else:
                prev_score = verdict.score

        if action in ("ANSWER", "ANSWER_PARTIAL"):
            return {
                "verdict": verdict.__dict__,
                "partial_answer": action == "ANSWER_PARTIAL",
                "kbinfos": tools.kbinfos,
            }
        if action == "ABSTAIN":
            if getattr(tools, "text_attachments_content", ""):
                return {"verdict": verdict.__dict__, "kbinfos": tools.kbinfos}
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


def _retrieval_confidence(result: dict) -> float:
    """Signal A for medium: retrieval-confidence from top-3 chunk similarity.

    There is no LLM agent in medium, so we substitute the self-assessed
    confidence with how strongly the vector/BM25 search matched the claim.
    Returns 0.0 when nothing was retrieved; otherwise the mean similarity of
    the top-3 hits (clamped to [0, 1]).
    """
    chunks = result.get("chunks", [])
    if not chunks:
        return 0.0
    scores = [c.get("similarity", 0.0) for c in chunks[:3]]
    if not scores:
        return 0.0
    mean = sum(scores) / len(scores)
    return max(0.0, min(1.0, mean))
