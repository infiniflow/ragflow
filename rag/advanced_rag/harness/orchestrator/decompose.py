"""Medium mode: decompose → parallel search → sufficiency check."""

import asyncio
import logging

from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.pipeline import normalize_search_query
from rag.advanced_rag.harness.sufficiency import (
    compute_fusion_score,
    cross_check_claim,
    route_sufficiency_verdict,
)
from rag.advanced_rag.harness.tools.search import hybrid_search, web_search
from rag.advanced_rag.harness.types import AgentResult, ClaimTarget, OrchestratorContext

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
    web_cache: dict[str, dict] = {}

    for cycle in range(mode.max_orchestrator_cycles):
        ctx.iteration = cycle
        unverified = [c for c in ctx.claims if not c.is_verified]
        if not unverified:
            break

        # Parallel search on unverified claims
        results = await asyncio.gather(*(hybrid_search(tools, query=c.description, keywords=keywords) for c in unverified))

        if tools.has_web():
            web_queries = {}
            for c, result in zip(unverified, results):
                if not result.get("chunks"):
                    web_queries.setdefault(normalize_search_query(c.description), c.description)
            uncached_queries = {key: query for key, query in web_queries.items() if key not in web_cache}
            cache_reuse_count = len(web_queries) - len(uncached_queries)
            if web_queries:
                _LOG.debug(
                    "medium local-empty web fallback: %d new request(s), %d cache reuse(s)",
                    len(uncached_queries),
                    cache_reuse_count,
                )
            if uncached_queries:
                web_results = await asyncio.gather(*(web_search(tools, query=query, keywords=keywords) for query in uncached_queries.values()))
                web_cache.update(zip(uncached_queries, web_results))
            results = [web_cache.get(normalize_search_query(c.description), result) if not result.get("chunks") else result for c, result in zip(unverified, results)]

        for c, result in zip(unverified, results):
            if result.get("chunks"):
                c.is_verified = True
                c.confidence = 0.8
                evidence_ids = _merge_kbinfos(tools, result)
                c.agent_result = AgentResult(
                    claim_id=c.claim_id,
                    report=_summarize(result),
                    is_verified=True,
                    confidence=0.8,
                    evidence_ids=evidence_ids,
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

        action, _should_continue = route_sufficiency_verdict(
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

    return {"kbinfos": tools.kbinfos}


def _merge_kbinfos(tools, result: dict) -> list[int]:
    if not result or not result.get("chunks"):
        return []
    chunks = tools.kbinfos.setdefault("chunks", [])
    index_by_key = {_chunk_key(c): i for i, c in enumerate(chunks)}
    evidence_ids = []
    for c in result.get("chunks", []):
        k = _chunk_key(c)
        if k not in index_by_key:
            index_by_key[k] = len(chunks)
            chunks.append(c)
        evidence_ids.append(index_by_key[k])
    dseen = {d.get("doc_id") for d in tools.kbinfos.get("doc_aggs", [])}
    for d in result.get("doc_aggs", []):
        if d.get("doc_id") in dseen:
            continue
        dseen.add(d.get("doc_id"))
        tools.kbinfos.setdefault("doc_aggs", []).append(d)
    return evidence_ids


def _chunk_key(ck: dict) -> str:
    return ck.get("chunk_id") or ck.get("id") or str(id(ck))


def _summarize(result: dict) -> str:
    chunks = result.get("chunks", [])
    texts = [c.get("content_with_weight", "")[:200] for c in chunks[:3]]
    return " | ".join(texts)
