"""Medium mode: decompose -> parallel search -> evidence-guided follow-up."""

import asyncio
import json
import logging
import re

from rag.advanced_rag.harness.types import ClaimTarget, AgentResult, OrchestratorContext
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.sufficiency import (
    cross_check_claim,
    compute_fusion_score,
    route_sufficiency_verdict,
)
from rag.advanced_rag.harness.orchestrator.sufficiency_llm import llm_sufficiency_boost
from rag.advanced_rag.harness.stats import in_phase
from rag.advanced_rag.harness.tools.search import hybrid_search

_LOG = logging.getLogger(__name__)

_MAX_EVIDENCE_SNIPPETS = 6
_MAX_NEXT_QUERIES = 3

_EVIDENCE_ANALYSIS_SYSTEM = """You are controlling a multi-hop RAG retrieval loop.
Judge whether the retrieved passages verify the claim using only the provided evidence.
If the claim is not verified, produce targeted next search queries that use entities,
dates, names, or relationships discovered in the evidence and move closer to the
original question.
Distinguish final-answer entities from bridge entities. If the passages identify
only a clue node in the chain, keep the claim incomplete and search for the
remaining relation needed by the original question. Return JSON only."""

_EVIDENCE_ANALYSIS_USER = """Original question:
{question}

Claim to verify:
{claim}

Search query used this round:
{query}

Round: {cycle} of {max_cycles}

Retrieved evidence snippets:
{evidence}

Return JSON:
{{
  "is_verified": true,
  "confidence": 0.0,
  "report": "Short evidence-backed finding, or what was learned so far.",
  "gaps": ["specific missing fact or relationship"],
  "next_queries": ["standalone follow-up search query"],
  "grounded": ["key asserted facts that ARE directly supported by the cited evidence, atomically and verbatim enough to match"],
  "numbers": ["for numerical/multi-hop answers: each figure used + its source, e.g. '2,161,000 from Wikipedia Demographics of Paris'; list ALL conflicting figures if several sources disagree"]
}}
Only list in grounded the facts you actually SAW in the evidence; prior-knowledge guesses go in gaps. If the claim is numerical or multi-hop and the evidence has multiple close-but-different figures, disclose all of them in numbers rather than silently picking one."""


@in_phase("decompose")
async def decompose_and_search(state: dict, tools) -> dict:
    """Decompose, retrieve, analyze evidence, then iterate with next-hop queries."""
    question = state.get("question", "")
    keywords = state.get("keywords", "")
    claims_raw = state.get("claims", [])
    route = state.get("route")
    mode_label = _mode_label(route)
    mode = get_mode(mode_label)
    max_cycles = _cycle_budget(state, mode.max_orchestrator_cycles)

    claims = [ClaimTarget(**c) if isinstance(c, dict) else c for c in claims_raw]
    ctx = OrchestratorContext(question=question, claims=claims, mode=mode_label)
    attempted_queries: dict[str, set[str]] = {c.claim_id: set() for c in ctx.claims}
    pending_queries: dict[str, list[str]] = {c.claim_id: [] for c in ctx.claims}
    completed_cycles = 0

    # Stagnation guard: stop when the fusion score stops improving across
    # consecutive rounds (corpus lacks the data, follow-ups return nothing)
    # instead of burning the remaining cycle budget unproductively.
    prev_score: float | None = None
    _STAGNATION_CYCLES = 2
    _STAGNATION_GAIN = 0.05

    for cycle in range(max_cycles):
        ctx.iteration = cycle
        unverified = [c for c in ctx.claims if not c.is_verified]
        if not unverified:
            break

        _LOG.info(
            "[Decompose search] Round %d of %d: researching %d unresolved claim(s).",
            cycle + 1,
            max_cycles,
            len(unverified),
        )

        tasks = []
        searched_claims = []
        for c in unverified:
            query = _pick_next_query(
                question,
                c,
                attempted_queries.setdefault(c.claim_id, set()),
                pending_queries.setdefault(c.claim_id, []),
            )
            if not query:
                _LOG.info("[Decompose search] No unused follow-up query remains for claim %s.", c.claim_id)
                continue
            attempted_queries[c.claim_id].add(_normalize_query(query))
            searched_claims.append((c, query))
            tasks.append(hybrid_search(tools, query=query, keywords=keywords, use_compiled=True))

        if not tasks:
            break

        results = await asyncio.gather(*tasks, return_exceptions=True)
        analysis_inputs = []

        for (c, query), result in zip(searched_claims, results):
            if isinstance(result, Exception):
                _LOG.exception("[Decompose search] Search failed for claim %s.", c.claim_id, exc_info=result)
                result = {"chunks": [], "doc_aggs": []}

            chunks = result.get("chunks", []) or []
            _merge_kbinfos(tools, result)
            evidence_ids = _evidence_ids(tools, chunks)
            analysis_inputs.append((c, query, result, evidence_ids))

        analyses = await asyncio.gather(
            *[
                _analyze_claim_evidence(
                    question=question,
                    claim=c,
                    query=query,
                    result=result,
                    evidence_ids=evidence_ids,
                    cycle=cycle,
                    max_cycles=max_cycles,
                    tools=tools,
                )
                for c, query, result, evidence_ids in analysis_inputs
            ],
            return_exceptions=True,
        )

        for (c, query, result, evidence_ids), analysis in zip(analysis_inputs, analyses):
            if isinstance(analysis, Exception):
                _LOG.exception("[Decompose search] Evidence analysis failed for claim %s.", c.claim_id, exc_info=analysis)
                analysis = _fallback_analysis(result, cycle, max_cycles)

            c.is_verified = analysis["is_verified"]
            c.confidence = analysis["confidence"]
            c.agent_result = AgentResult(
                claim_id=c.claim_id,
                report=analysis["report"],
                is_verified=c.is_verified,
                confidence=c.confidence,
                evidence_ids=evidence_ids,
                gaps=analysis["gaps"],
                discovered_claims=[],
                grounded=analysis.get("grounded", []),
                numbers=analysis.get("numbers", []),
            )

            next_queries = _new_queries(
                analysis.get("next_queries", []),
                attempted_queries.setdefault(c.claim_id, set()),
            )
            if not c.is_verified and next_queries:
                pending_queries.setdefault(c.claim_id, []).extend(next_queries)
                _LOG.info(
                    "[Decompose search] Claim %s needs another hop; queued %d targeted query/queries.",
                    c.claim_id,
                    len(next_queries),
                )
            _LOG.info(
                '[Decompose search] Claim %s after "%s": %s (confidence %.0f%%).',
                c.claim_id,
                _snip(query),
                "verified" if c.is_verified else "still incomplete",
                c.confidence * 100,
            )

        completed_cycles = cycle + 1

        all_chunks = {i: c for i, c in enumerate(tools.kbinfos.get("chunks", []))}
        agent_results = [c.agent_result for c in ctx.claims if c.agent_result]
        cross_results = [cross_check_claim(r, all_chunks) for r in agent_results]

        # HEAD sufficiency enhancements: pass the question + claims + evidence so
        # the fusion activates required-entity AND-semantics veto, grounded-fact
        # verification, and numeric multi-source conflict detection (not just the
        # baseline ratio/mean that upstream's 3-arg call skipped).
        verdict = compute_fusion_score(
            agent_results,
            cross_results,
            mode,
            question=ctx.question,
            claims=ctx.claims,
            all_chunks=all_chunks,
        )
        ctx.verdict = verdict

        # LLM Sufficient Context AutoRater (primary sufficiency judge). Medium
        # keeps it gated to the borderline band for cost control; its verdict
        # (`auto=boost`) is fed to the decision ladder inside
        # ``route_sufficiency_verdict``, which replaces the old manual
        # SUFFICIENT upgrade. Missing-piece follow-ups feed the next hop.
        boost: dict = {}
        if verdict.status in ("USEFUL_BUT_INCOMPLETE", "INSUFFICIENT", "CONFLICTING"):
            boost = await llm_sufficiency_boost(tools, ctx.question, verdict, evidence_ids=_global_evidence_ids(tools, {"chunks": tools.kbinfos.get("chunks", [])}))
            followups = boost.get("followups") or []
            if followups:
                ctx.pending_followups = followups
                _LOG.info("[Decompose] Stored %d follow-up query(ies) for next round.", len(ctx.pending_followups))
            if boost:
                _LOG.info("[Decompose] AutoRater is_sufficient=%s confidence=%.2f", boost.get("is_sufficient"), boost.get("confidence", 1.0))

        # LLM groundedness review (Google "draft review"): runs unconditionally so every
        # decomposed result — including a non-critical-band SUFFICIENT — is groundedness-
        # validated before the status gate. (The lexical NER grounded check is disabled
        # in favour of this LLM review, so it must not be skipped on any path.) Ungrounded
        # claim drafts are merged into hard_violations → decision ladder caveat.
        from rag.advanced_rag.harness.orchestrator.grounded_llm import llm_grounded_verify

        # Union of cited evidence IDs across all claim results (matches the
        # agentic orchestrator's cited-evidence behavior) so the reviewer sees
        # the exact evidence each claim referenced, not a global prefix.
        cited_evidence_ids: list[str] = []
        for r in agent_results:
            cited_evidence_ids.extend(r.evidence_ids or [])
        grounded = await llm_grounded_verify(
            tools,
            ctx.question,
            [(r.claim_id, r.report or "") for r in agent_results if r.report],
            cited_evidence_ids or None,
        )
        # Treat a claim as violating when it is explicitly grounded=False OR has
        # non-empty ungrounded assertions (covers the degenerate grounded=False /
        # empty-ungrounded case too). Only accept IDs that exist in the original
        # claims collection — the LLM may echo a bogus claim_id, which must not
        # leak into hard_violations.
        valid_claim_ids = {r.claim_id for r in agent_results}
        ungrounded_ids = [cid for cid, g in grounded.items() if cid in valid_claim_ids and (g.get("grounded") is False or g.get("ungrounded"))]
        if ungrounded_ids:
            existing = set(verdict.hard_violations or [])
            verdict.hard_violations = list(existing | set(ungrounded_ids))
            _LOG.info("[Decompose] %d claim(s) have ungrounded (draft-review) assertions: %s", len(ungrounded_ids), ungrounded_ids)

        action, should_continue, caveat = route_sufficiency_verdict(
            verdict,
            mode_label,
            cycle,
            max_cycles,
            auto=boost,
        )
        if caveat:
            _LOG.info("[Decompose] caveat=%s", caveat)

        # Stagnation guard: if the verdict is not (yet) sufficient and the score
        # has not meaningfully improved across consecutive rounds, stop instead
        # of burning the remaining cycle budget on unproductive re-searches.
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
            return _finalize(ctx, tools, partial=action == "ANSWER_PARTIAL", loop=completed_cycles)
        if action == "ABSTAIN":
            tools.kbinfos["chunks"] = []
            return {"verdict": verdict.__dict__, "abstain": True, "loop": completed_cycles}
        if action == "FALLBACK_LLM":
            return _finalize(ctx, tools, partial=True, loop=completed_cycles)
        if not should_continue:
            break

    if not tools.kbinfos.get("chunks"):
        return {"empty_result": True, "kbinfos": tools.kbinfos, "loop": completed_cycles}

    partial = any(not c.is_verified for c in ctx.claims)
    return _finalize(ctx, tools, partial=partial, loop=completed_cycles)


async def _analyze_claim_evidence(
    *,
    question: str,
    claim: ClaimTarget,
    query: str,
    result: dict,
    evidence_ids: list[int],
    cycle: int,
    max_cycles: int,
    tools,
) -> dict:
    chunks = result.get("chunks", []) or []
    if not chunks:
        return {
            "is_verified": False,
            "confidence": 0.0,
            "report": "",
            "gaps": ["no evidence found"],
            "next_queries": _fallback_queries(question, claim),
        }

    try:
        user = _EVIDENCE_ANALYSIS_USER.format(
            question=question,
            claim=claim.description,
            query=query,
            cycle=cycle + 1,
            max_cycles=max_cycles,
            evidence=_format_evidence(chunks),
        )
        msg = await tools._fit_messages(_EVIDENCE_ANALYSIS_SYSTEM, user)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.1})
        if isinstance(ans, tuple):
            ans = ans[0]
        parsed = _extract_json(ans)
        return _normalize_analysis(parsed, result, evidence_ids, question, claim, cycle, max_cycles)
    except Exception:
        _LOG.exception("[Decompose search] Evidence analysis LLM call failed.")
        return _fallback_analysis(result, cycle, max_cycles, question, claim)


def _mode_label(route) -> str:
    if not route:
        return "medium"
    if isinstance(route, dict):
        return route.get("thinking_mode", "medium")
    return getattr(route, "thinking_mode", "medium")


def _cycle_budget(state: dict, default_cycles: int) -> int:
    try:
        requested = int(state.get("max_loops") or default_cycles)
    except (TypeError, ValueError):
        requested = default_cycles
    return max(1, min(default_cycles, requested))


def _extract_json(text: str) -> dict:
    text = re.sub(r"^.*</think>", "", text or "", flags=re.DOTALL).strip()
    text = re.sub(r"```(?:json)?\s*|\s*```", "", text).strip()
    try:
        import json_repair

        return json_repair.loads(text)
    except Exception:
        try:
            return json.loads(text)
        except Exception:
            _LOG.warning("[Decompose search] Failed to parse evidence analysis output: %s", text[:200])
            return {}


def _normalize_analysis(
    parsed: dict,
    result: dict,
    evidence_ids: list[int],
    question: str,
    claim: ClaimTarget,
    cycle: int,
    max_cycles: int,
) -> dict:
    confidence = _clamp_float(parsed.get("confidence"), 0.0, 1.0)
    is_verified = bool(parsed.get("is_verified")) and bool(evidence_ids) and confidence >= 0.55
    report = str(parsed.get("report") or "").strip() or _summarize(result)
    gaps = _string_list(parsed.get("gaps"))
    next_queries = _string_list(parsed.get("next_queries"))[:_MAX_NEXT_QUERIES]
    grounded = _string_list(parsed.get("grounded"))
    numbers = _string_list(parsed.get("numbers"))

    if is_verified:
        gaps = []
        next_queries = []
        # NOTE: no optimistic floor here. The old ``max(confidence, 0.65)``
        # inflated medium's agent confidence and distorted the decision-ladder
        # gate (agent_confidence >= c_high/c_low). Keep the LLM's raw confidence
        # so the ladder's thresholds behave as designed.
    elif not next_queries and cycle + 1 < max_cycles:
        next_queries = _fallback_queries(question, claim)

    return {
        "is_verified": is_verified,
        "confidence": confidence,
        "report": report,
        "gaps": gaps,
        "next_queries": next_queries,
        "grounded": grounded,
        "numbers": numbers,
    }


def _fallback_analysis(
    result: dict,
    cycle: int,
    max_cycles: int,
    question: str = "",
    claim: ClaimTarget | None = None,
) -> dict:
    chunks = result.get("chunks", []) or []
    is_last_cycle = cycle + 1 >= max_cycles
    is_verified = bool(chunks) and is_last_cycle
    next_queries = [] if is_last_cycle or claim is None else _fallback_queries(question, claim)
    return {
        "is_verified": is_verified,
        "confidence": 0.55 if is_verified else (0.35 if chunks else 0.0),
        "report": _summarize(result),
        "gaps": [] if is_verified else ["need more specific evidence"],
        "next_queries": next_queries,
        "grounded": [],
        "numbers": [],
    }


def _pick_next_query(
    question: str,
    claim: ClaimTarget,
    attempted: set[str],
    pending: list[str],
) -> str:
    while pending:
        query = (pending.pop(0) or "").strip()
        normalized = _normalize_query(query)
        if normalized and normalized not in attempted:
            return query

    candidates = []
    if not attempted:
        candidates.append(claim.description)
    candidates.extend(_fallback_queries(question, claim))

    for query in candidates:
        query = (query or "").strip()
        normalized = _normalize_query(query)
        if normalized and normalized not in attempted:
            return query
    return ""


def _fallback_queries(question: str, claim: ClaimTarget) -> list[str]:
    candidates = []
    for gap in _agent_result_gaps(claim.agent_result):
        candidates.append(f"{claim.description} {gap}")
    if question:
        candidates.append(f"{question} {claim.description}")
    candidates.append(claim.description)
    return candidates[:_MAX_NEXT_QUERIES]


def _new_queries(raw_queries: list[str], attempted: set[str]) -> list[str]:
    queries = []
    seen = set(attempted)
    for query in raw_queries:
        query = (query or "").strip()
        normalized = _normalize_query(query)
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        queries.append(query)
        if len(queries) >= _MAX_NEXT_QUERIES:
            break
    return queries


def _agent_result_gaps(agent_result) -> list[str]:
    if not agent_result:
        return []
    if isinstance(agent_result, dict):
        return _string_list(agent_result.get("gaps"))
    return _string_list(getattr(agent_result, "gaps", []))


def _normalize_query(query: str) -> str:
    return " ".join((query or "").lower().split())


def _format_evidence(chunks: list[dict]) -> str:
    snippets = []
    for i, chunk in enumerate(chunks[:_MAX_EVIDENCE_SNIPPETS], start=1):
        text = chunk.get("content_with_weight") or chunk.get("content") or chunk.get("text") or ""
        source = chunk.get("docnm_kwd") or chunk.get("doc_name") or chunk.get("doc_id") or "source"
        snippets.append(f"[{i}] {source}: {_snip(text, 900)}")
    return "\n\n".join(snippets) or "(no evidence)"


def _string_list(value) -> list[str]:
    if isinstance(value, str):
        return [value.strip()] if value.strip() else []
    if not isinstance(value, list):
        return []
    return [str(v).strip() for v in value if str(v).strip()]


def _clamp_float(value, lo: float, hi: float) -> float:
    try:
        number = float(value)
    except (TypeError, ValueError):
        number = 0.0
    return min(hi, max(lo, number))


def _evidence_ids(tools, chunks: list[dict]) -> list[int]:
    all_chunks = tools.kbinfos.get("chunks", [])
    index_by_key = {_chunk_key(c): i for i, c in enumerate(all_chunks)}
    ids = []
    for chunk in chunks:
        idx = index_by_key.get(_chunk_key(chunk))
        if idx is not None and idx not in ids:
            ids.append(idx)
    return ids


def _finalize(ctx: OrchestratorContext, tools, partial: bool, loop: int) -> dict:
    combined = []
    for claim in ctx.claims:
        if claim.agent_result and claim.agent_result.report:
            status = "verified" if claim.is_verified else "incomplete"
            combined.append(f"[{claim.claim_id}] {status} ({claim.description}): {claim.agent_result.report[:500]}")
    if combined:
        tools.kbinfos["pre_summary"] = "Research findings. These may include bridge entities; the final answer must still satisfy the original question's requested role.\n\n" + "\n\n".join(combined)

    return {
        "verdict": ctx.verdict.__dict__ if ctx.verdict else None,
        "partial_answer": partial,
        "kbinfos": tools.kbinfos,
        "loop": loop,
    }


def _snip(text: str, limit: int = 160) -> str:
    text = (text or "").replace("\n", " ").strip()
    return text if len(text) <= limit else text[: limit - 3] + "..."


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
    texts = [(c.get("content_with_weight") or c.get("content") or c.get("text") or "")[:200] for c in chunks[:3]]
    return " | ".join(texts)
