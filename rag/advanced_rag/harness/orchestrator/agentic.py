"""High/Ultra: two-level loop — orchestrator assigns claims, agent researches, sufficiency checks."""

import asyncio
import logging

from rag.advanced_rag.harness.types import (
    ClaimTarget,
    AgentResult,
    OrchestratorContext,
)
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.pipeline import Pipeline
from rag.advanced_rag.harness.agent import research_agent_loop
from rag.advanced_rag.harness.sufficiency import (
    cross_check_claim,
    compute_fusion_score,
    route_sufficiency_verdict,
)

_LOG = logging.getLogger(__name__)
CLAIM_RESEARCH_TIMEOUT_SECONDS = 180


def _snip(text: str, limit: int = 160) -> str:
    text = (text or "").replace("\n", " ").strip()
    return text if len(text) <= limit else text[: limit - 3] + "..."


async def agentic_research(state: dict, tools) -> dict:
    """Two-level loop for high/ultra modes."""
    question = state.get("question", "")
    claims_raw = state.get("claims", [])
    route = state.get("route", {})
    mode_label = route.thinking_mode if route else "high"
    mode = get_mode(mode_label)

    # Resolve compilation map
    compilation_map = await _get_compilation_map(tools)

    claims = [ClaimTarget(**c) if isinstance(c, dict) else c for c in claims_raw]
    ctx = OrchestratorContext(question=question, claims=claims, mode=mode_label)
    pipeline = Pipeline(tools, compilation_map)

    for cycle in range(mode.max_orchestrator_cycles):
        ctx.iteration = cycle
        _LOG.info("[Agentic research] Research round %d of %d — %d step(s) still unanswered.", cycle + 1, mode.max_orchestrator_cycles, sum(1 for c in ctx.claims if not c.is_verified))

        # ── Step A: Research unverified claims (parallel if mode allows) ──
        unverified = [c for c in ctx.claims if not c.is_verified]

        if unverified:
            # Process in batches of max_parallel_agents
            batch_size = mode.max_parallel_agents
            for i in range(0, len(unverified), batch_size):
                batch = unverified[i : i + batch_size]
                _LOG.info(
                    "[Agentic research] Round %d: researching %d step(s) in parallel: %s",
                    cycle + 1,
                    len(batch),
                    "; ".join(f'"{c.description}"' for c in batch),
                )
                tasks = [_run_claim_research(c, tools, pipeline, ctx, mode, compilation_map) for c in batch]
                agent_results = await asyncio.gather(*tasks)
                _LOG.info(
                    "[Agentic research] Round %d: finished researching %d step(s).",
                    cycle + 1,
                    len(agent_results),
                )

                for c, result in zip(batch, agent_results):
                    is_verified = result.get("is_verified", False)
                    c.is_verified = is_verified
                    c.confidence = result.get("confidence", 0.0)
                    c.agent_result = AgentResult(
                        claim_id=c.claim_id,
                        report=result.get("report", ""),
                        is_verified=is_verified,
                        confidence=c.confidence,
                        evidence_ids=result.get("evidence_ids", []),
                        gaps=result.get("gaps", []),
                        discovered_claims=result.get("discovered_claims", []),
                    )

                    # Ultra: dynamic claim expansion
                    if mode.allows_dynamic_claims and result.get("discovered_claims"):
                        for dc in result["discovered_claims"]:
                            if dc and dc not in [cc.description for cc in ctx.claims]:
                                ctx.claims.append(
                                    ClaimTarget(
                                        claim_id=f"c_dyn_{len(ctx.claims)}",
                                        description=dc,
                                    )
                                )
                                _LOG.info('[Agentic research] Found a new angle worth researching: "%s"', dc)

        # ── Step B: Sufficiency Check ──
        all_chunks = {i: c for i, c in enumerate(tools.kbinfos.get("chunks", []))}
        agent_results_list = [c.agent_result for c in ctx.claims if c.agent_result]
        cross_results = [cross_check_claim(r, all_chunks) for r in agent_results_list]

        verdict = compute_fusion_score(agent_results_list, cross_results, mode)
        ctx.verdict = verdict

        action, should_continue = route_sufficiency_verdict(
            verdict,
            mode_label,
            cycle,
            mode.max_orchestrator_cycles,
        )

        _LOG.info("[Agentic research] Round %d: evidence looks %s (confidence %.0f%%) — next: %s", cycle + 1, verdict.status, verdict.score * 100, action)

        if action == "ANSWER":
            return _finalize(ctx, tools, partial=False)
        if action == "ANSWER_PARTIAL":
            return _finalize(ctx, tools, partial=True)
        if action == "ABSTAIN":
            tools.kbinfos["chunks"] = []
            return {"verdict": verdict.__dict__, "abstain": True}
        if action == "REPLAN":
            # Ultra: re-plan on low score
            from rag.advanced_rag.harness.planner import planner_node

            state["feedback"] = verdict.feedback
            state["route"] = route
            new_plan = await planner_node(state, tools)
            ctx.claims = new_plan.get("claims", ctx.claims)
        if action == "FALLBACK_LLM":
            return _finalize(ctx, tools, partial=True, fallback=True)

    # Max cycles reached
    return _finalize(ctx, tools, partial=True)


async def _run_claim_research(
    claim: ClaimTarget,
    tools,
    pipeline: Pipeline,
    ctx: OrchestratorContext,
    mode,
    compilation_map: dict,
) -> dict:
    _LOG.info('[Agentic research] Researching: "%s"', _snip(claim.description))
    try:
        result = await asyncio.wait_for(
            research_agent_loop(claim, tools, pipeline, ctx, mode, compilation_map),
            timeout=CLAIM_RESEARCH_TIMEOUT_SECONDS,
        )
    except asyncio.CancelledError:
        raise
    except asyncio.TimeoutError:
        _LOG.warning(
            '[Agentic research] Gave up on "%s" — it took longer than %ss.',
            _snip(claim.description),
            CLAIM_RESEARCH_TIMEOUT_SECONDS,
        )
        return {
            "report": "",
            "is_verified": False,
            "confidence": 0.0,
            "evidence_ids": [],
            "gaps": [f"claim research timeout after {CLAIM_RESEARCH_TIMEOUT_SECONDS}s"],
            "discovered_claims": [],
        }
    except Exception:
        _LOG.exception('[Agentic research] Hit an error while researching "%s".', _snip(claim.description))
        return {
            "report": "",
            "is_verified": False,
            "confidence": 0.0,
            "evidence_ids": [],
            "gaps": ["claim research failed"],
            "discovered_claims": [],
        }

    _LOG.info(
        '[Agentic research] Finished "%s" — %s, backed by %d passage(s) (confidence %.0f%%)%s.',
        _snip(claim.description),
        "answered" if result.get("is_verified") else "still unanswered",
        len(result.get("evidence_ids") or []),
        float(result.get("confidence") or 0.0) * 100,
        f", {len(result.get('gaps') or [])} gap(s) remain" if result.get("gaps") else "",
    )
    return result


def _finalize(ctx: OrchestratorContext, tools, partial: bool = False, fallback: bool = False) -> dict:
    """Merge agent results into kbinfos and return."""
    _merge_agent_results(ctx, tools)
    return {
        "verdict": ctx.verdict.__dict__ if ctx.verdict else None,
        "partial_answer": partial or fallback,
        "kbinfos": tools.kbinfos,
    }


def _merge_agent_results(ctx: OrchestratorContext, tools):
    """Merge agent result reports into kbinfos as a pre_summary."""
    combined = []
    seen_evidence = set()

    for c in ctx.claims:
        if c.agent_result and c.agent_result.report:
            status = "✅" if c.is_verified else "❌"
            combined.append(f"【{c.claim_id}】{status} {c.agent_result.report[:500]}")

    if combined:
        tools.kbinfos["pre_summary"] = "\n\n".join(combined)

    # Collect evidence chunks from agent results
    for c in ctx.claims:
        if c.agent_result and c.agent_result.evidence_ids:
            for eid in c.agent_result.evidence_ids:
                if eid not in seen_evidence:
                    seen_evidence.add(eid)


async def _get_compilation_map(tools) -> dict[str, set[str]]:
    """Build compilation map from RAGTools - check which KBs have compilation artifacts."""
    result = {}
    if not tools.kbs:
        return result
    for kb in tools.kbs:
        comps = set()
        parser_config = getattr(kb, "parser_config", None) or {}
        if parser_config.get("toc"):
            comps.add("toc")
        if parser_config.get("knowledge_graph"):
            comps.add("knowledge_graph")
        if parser_config.get("wiki"):
            comps.add("wiki")
        if parser_config.get("mindmap"):
            comps.add("mindmap")
        if parser_config.get("page_index"):
            comps.add("page_index")
        await _add_template_group_compilations(comps, parser_config, getattr(kb, "tenant_id", ""))
        if await _has_dataset_nav_rows(getattr(kb, "tenant_id", ""), getattr(kb, "id", "")):
            comps.add("tree")
        if comps:
            result[kb.id] = comps
    return result


async def _has_dataset_nav_rows(tenant_id: str, kb_id: str) -> bool:
    if not tenant_id or not kb_id:
        return False
    try:
        from common import settings
        from common.doc_store.doc_store_base import OrderByExpr
        from common.misc_utils import thread_pool_exec
        from rag.nlp import search

        index_name = search.index_name(tenant_id)
        if not settings.docStoreConn.index_exist(index_name, kb_id):
            return False
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            ["id"],
            [],
            {"compile_kwd": ["dataset_nav"]},
            [],
            OrderByExpr(),
            0,
            1,
            index_name,
            [kb_id],
        )
        return bool(settings.docStoreConn.get_total(res))
    except Exception:
        _LOG.exception("[agentic] dataset-nav existence check failed for kb=%s", kb_id)
        return False


async def _add_template_group_compilations(comps: set[str], parser_config: dict, tenant_id: str) -> None:
    """Infer available compilation kinds from selected template groups."""
    if not tenant_id:
        return
    try:
        from common.misc_utils import thread_pool_exec
        from api.db.services.compilation_template_group_service import CompilationTemplateGroupService
        from rag.svr.task_executor_refactor.chunk_post_processor import (
            _parser_config_compilation_template_group_ids,
        )
    except Exception:
        _LOG.exception("[agentic] compilation-map helper import failed")
        return

    try:
        group_ids = _parser_config_compilation_template_group_ids(parser_config)
    except Exception:
        _LOG.exception("[agentic] compilation template group id resolution failed")
        return

    for group_id in group_ids:
        try:
            group = await thread_pool_exec(CompilationTemplateGroupService.get_saved, group_id, tenant_id)
        except Exception:
            _LOG.exception("[agentic] compilation template group read failed id=%s", group_id)
            continue
        for template in (group or {}).get("templates") or []:
            config = template.get("config") or {}
            raw_kind = (config.get("kind") if isinstance(config, dict) else "") or template.get("kind") or ""
            raw_norm = raw_kind.strip().lower().replace("-", "_") if isinstance(raw_kind, str) else ""
            kind = _compilation_kind_for_agentic_map(raw_kind)
            if raw_norm == "knowledge_graph":
                comps.add("knowledge_graph")
            if kind == "tree":
                comps.add("tree")
            elif kind in {"timeline", "page_index", "pageindex"}:
                comps.add("page_index")
            elif kind in {"mindmap", "mind_map"}:
                comps.add("mindmap")
            elif kind == "artifacts":
                comps.add("wiki")


def _compilation_kind_for_agentic_map(kind) -> str:
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index"}:
        return "timeline"
    return normalized
