"""High/Ultra: two-level loop — orchestrator assigns claims, agent researches, sufficiency checks."""

import asyncio
import logging

from rag.advanced_rag.harness.agent import research_agent_loop
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.orchestrator.sufficiency_llm import llm_sufficiency_boost
from rag.advanced_rag.harness.pipeline import Pipeline
from rag.advanced_rag.harness.stats import in_phase, record_round, record_round_claims
from rag.advanced_rag.harness.sufficiency import (
    compute_fusion_score,
    cross_check_claim,
    route_sufficiency_verdict,
)
from rag.advanced_rag.harness.types import (
    AgentResult,
    ClaimTarget,
    OrchestratorContext,
)

_LOG = logging.getLogger(__name__)
CLAIM_RESEARCH_TIMEOUT_SECONDS = 180


def _snip(text: str, limit: int = 160) -> str:
    text = (text or "").replace("\n", " ").strip()
    return text if len(text) <= limit else text[: limit - 3] + "..."


def _discovered_entity(tools) -> str | None:
    """Pick a salient discovered name from the gathered evidence.

    Prefers an explicit entity/keyword tag on a chunk, falling back to a source
    document name. Used only to gate ``graph_explore`` eligibility (its context
    check needs ``context.last_entity``), so a coarse signal is enough.
    """
    chunks = (getattr(tools, "kbinfos", {}) or {}).get("chunks", []) or []
    for c in chunks:
        for key in ("entities_kwd", "important_kwd"):
            val = c.get(key)
            if isinstance(val, list) and val:
                first = str(val[0]).strip()
                if first:
                    return first
            if isinstance(val, str) and val.strip():
                return val.strip().split()[0]
    for c in chunks:
        name = str(c.get("docnm_kwd") or "").strip()
        if name:
            return name
    return None


@in_phase("orchestrator")
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

    # Stagnation guard: if the fusion score stops improving across consecutive
    # rounds, further searching is unlikely to help (e.g. the corpus simply lacks
    # the data, and follow-ups keep returning nothing). Without this, a
    # persistently INSUFFICIENT verdict burns every remaining cycle and, in the
    # worst case, feeds a long empty loop (see check.log Q4: AutoRater said
    # "not in corpus", follow-ups found nothing, yet the loop kept spinning).
    prev_score: float | None = None
    _STAGNATION_CYCLES = 2  # rounds with no meaningful gain before giving up
    _STAGNATION_GAIN = 0.05  # minimum fusion-score improvement to count

    rounds_run = 0
    for cycle in range(mode.max_orchestrator_cycles):
        rounds_run = cycle + 1
        ctx.iteration = cycle
        record_round("orchestrator")
        _LOG.info("[Agentic research] Research round %d of %d — %d step(s) still unanswered.", cycle + 1, mode.max_orchestrator_cycles, sum(1 for c in ctx.claims if not c.is_verified))

        # ── Step A: Research unverified claims (parallel if mode allows) ──
        unverified = [c for c in ctx.claims if not c.is_verified]
        record_round_claims("claim_research", len(unverified))

        if unverified:
            # Consume Phase-2 follow-up queries (missing-pieces feedback) ONCE for
            # this round and hand the SAME list to every claim in the batch.
            # Reading the shared ``ctx.pending_followups`` inside
            # research_agent_loop would race under ``asyncio.gather``: the first
            # claim to execute would clear it, starving the parallel siblings.
            # We only consume/clear here — inside ``if unverified`` — because a
            # research task must actually dispatch to use them. When everything
            # is already verified no task runs, so we retain the queries for the
            # next cycle instead of discarding them.
            followups: list[str] = []
            if ctx.pending_followups:
                followups = [str(q.get("query") or q.get("question") or "") for q in ctx.pending_followups if q]
                followups = [q for q in followups if q.strip()]
                ctx.pending_followups = []
                if followups:
                    _LOG.info("[Agentic research] Round %d: injecting %d follow-up query(ies) to all claims: %s", cycle + 1, len(followups), followups)

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
                tasks = [_run_claim_research(c, tools, ctx, mode, compilation_map, followups=followups) for c in batch]
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
                    grounded = result.get("grounded", [])
                    numbers = result.get("numbers", [])
                    if "grounded" not in result or "numbers" not in result:
                        _LOG.warning(
                            "[Agentic research] claim=%s report omitted the schema-required grounded/numbers fields (grounded=%r numbers=%r) — verification for it is degraded",
                            c.claim_id,
                            grounded,
                            numbers,
                        )
                    c.agent_result = AgentResult(
                        claim_id=c.claim_id,
                        report=result.get("report", ""),
                        is_verified=is_verified,
                        confidence=c.confidence,
                        evidence_ids=result.get("evidence_ids", []),
                        gaps=result.get("gaps", []),
                        discovered_claims=result.get("discovered_claims", []),
                        grounded=grounded,
                        numbers=numbers,
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

        # ── Step A.5: note a discovered entity so graph_explore becomes eligible
        # in the next round (its context gate requires context.last_entity). ──
        ctx.note_entity(_discovered_entity(tools))

        # ── Step B: Sufficiency Check ──
        all_chunks = {i: c for i, c in enumerate(tools.kbinfos.get("chunks", []))}
        agent_results_list = [c.agent_result for c in ctx.claims if c.agent_result]
        _LOG.info(
            "[Sufficiency] Round %d: evidence pool=%d chunk(s), %d claim(s) with agent results: %s",
            cycle + 1,
            len(all_chunks),
            len(agent_results_list),
            [
                {
                    "claim_id": r.claim_id,
                    "self_verified": r.is_verified,
                    "self_confidence": round(r.confidence, 3),
                    "evidence_ids": len(r.evidence_ids),
                }
                for r in agent_results_list
            ],
        )
        cross_results = [cross_check_claim(r, all_chunks) for r in agent_results_list]

        verdict = compute_fusion_score(
            agent_results_list,
            cross_results,
            mode,
            question=ctx.question,
            claims=ctx.claims,
            all_chunks=all_chunks,
        )
        ctx.verdict = verdict

        # Decision ladder: the LLM Sufficient Context AutoRater is the primary
        # sufficiency judge (invoked every round in high/ultra). Its verdict is
        # combined with the code-level signals (hard vetoes + agent confidence)
        # inside ``route_sufficiency_verdict`` → ``sufficiency_ladder``. The
        # AutoRater's missing-pieces feedback is saved for the next round.
        cited_ids: list[str] = []
        for r in agent_results_list:
            cited_ids.extend(r.evidence_ids or [])
        boost = await llm_sufficiency_boost(tools, ctx.question, verdict, evidence_ids=cited_ids)
        if boost and boost.get("followups"):
            # Missing pieces → targeted follow-up searches for the next round.
            ctx.pending_followups = boost.get("followups", [])
            _LOG.info("[Agentic research] Stored %d follow-up query(ies) for next round.", len(ctx.pending_followups))
        if boost:
            _LOG.info("[Agentic research] Round %d: AutoRater is_sufficient=%s confidence=%.2f", cycle + 1, boost.get("is_sufficient"), boost.get("confidence", 1.0))

        # LLM groundedness review (Google "draft review" thought): check whether each
        # claim's report is semantically supported by the cited evidence. Ungrounded
        # claims (hallucinated / over-claimed drafts) are merged into hard_violations
        # so the decision ladder forces a caveated answer — this catches relation/over-
        # claim errors that the lexical code-level grounded check (cross_check_claim)
        # cannot see.
        from rag.advanced_rag.harness.orchestrator.grounded_llm import llm_grounded_verify

        grounded = await llm_grounded_verify(
            tools,
            ctx.question,
            [(r.claim_id, r.report or "") for r in agent_results_list if r.report],
            cited_ids,
        )
        # Treat a claim as violating when it is explicitly grounded=False OR has
        # non-empty ungrounded assertions (covers the degenerate grounded=False /
        # empty-ungrounded case too). Only accept IDs present in the original
        # claims collection — the LLM may echo a bogus claim_id that must not leak
        # into hard_violations.
        valid_claim_ids = {r.claim_id for r in agent_results_list}
        ungrounded_ids = [cid for cid, g in grounded.items() if cid in valid_claim_ids and (g.get("grounded") is False or g.get("ungrounded"))]
        if ungrounded_ids:
            existing = set(verdict.hard_violations or [])
            verdict.hard_violations = list(existing | set(ungrounded_ids))
            _LOG.info("[Agentic research] Round %d: %d claim(s) have ungrounded (draft-review) assertions: %s", cycle + 1, len(ungrounded_ids), ungrounded_ids)

        action, should_continue, caveat = route_sufficiency_verdict(
            verdict,
            mode_label,
            cycle,
            mode.max_orchestrator_cycles,
            auto=boost,
        )
        if caveat:
            _LOG.info("[Agentic research] Round %d: caveat=%s", cycle + 1, caveat)

        # Stagnation guard: when the verdict is not (yet) sufficient and the
        # fusion score has not meaningfully improved for a couple of rounds,
        # stop instead of burning the remaining cycle budget on unproductive
        # re-searches. Override the CONTINUE decision with a partial answer.
        if should_continue and verdict.status in ("INSUFFICIENT", "USEFUL_BUT_INCOMPLETE"):
            if prev_score is not None and cycle >= _STAGNATION_CYCLES and verdict.score - prev_score < _STAGNATION_GAIN:
                _LOG.info(
                    "[Agentic research] Round %d: score stagnant (%.3f → %.3f) — early-stopping to partial answer",
                    cycle + 1,
                    prev_score,
                    verdict.score,
                )
                action = "ANSWER_PARTIAL"
                should_continue = False
            else:
                prev_score = verdict.score

        _LOG.info("[Agentic research] Round %d: evidence looks %s (confidence %.0f%%) — next: %s", cycle + 1, verdict.status, verdict.score * 100, action)

        if action == "ANSWER":
            return _finalize(ctx, tools, partial=False, loop=rounds_run)
        if action == "ANSWER_PARTIAL":
            return _finalize(ctx, tools, partial=True, loop=rounds_run)
        if action == "ABSTAIN":
            tools.kbinfos["chunks"] = []
            return {"verdict": verdict.__dict__, "abstain": True, "loop": rounds_run}
        if action == "REPLAN":
            # Ultra: re-plan on low score. Ground the new plan on the evidence
            # gathered so far, and carry still-valid verified claims over so a
            # replan doesn't re-research (and re-bill) work already done.
            from rag.advanced_rag.harness.planner import planner_node

            state["feedback"] = verdict.feedback
            state["route"] = route
            state["seed_chunks"] = list(tools.kbinfos.get("chunks", []) or [])
            new_plan = await planner_node(state, tools)
            # Keep EVERY verified claim (even ones the new plan omitted — their
            # evidence is still valid and shouldn't be re-researched), then
            # append only the new plan's unverified claims.
            verified = [c for c in ctx.claims if c.is_verified]
            new_by_desc = {}
            for c in new_plan.get("claims", ctx.claims):
                if isinstance(c, ClaimTarget):
                    new_by_desc.setdefault(c.description, c)
            seen = {c.description for c in verified}
            ctx.claims = verified + [c for c in new_by_desc.values() if c.description not in seen]
        if action == "FALLBACK_LLM":
            return _finalize(ctx, tools, partial=True, fallback=True, loop=rounds_run)

    # Max cycles reached
    return _finalize(ctx, tools, partial=True, loop=rounds_run)


async def _run_claim_research(
    claim: ClaimTarget,
    tools,
    ctx: OrchestratorContext,
    mode,
    compilation_map: dict,
    followups: list[str] | None = None,
) -> dict:
    _LOG.info('[Agentic research] Researching: "%s"', _snip(claim.description))
    # A dedicated pipeline per claim keeps the routing scope (``_routed_docs``)
    # isolated: under asyncio.gather the shared single pipeline would let one
    # claim's dataset_navigation_search leak its doc_scope into a sibling's
    # follow-up searches (the doc_scope is set on the pipeline, not the claim).
    # ``tools.kbinfos`` stays shared, so the citation pool still merges across
    # claims via Pipeline._merge_into_kbinfos.
    pipeline = Pipeline(tools, compilation_map)
    try:
        result = await asyncio.wait_for(
            research_agent_loop(claim, tools, pipeline, ctx, mode, compilation_map, followups=followups),
            timeout=CLAIM_RESEARCH_TIMEOUT_SECONDS,
        )
    except asyncio.CancelledError:
        raise
    except TimeoutError:
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


def _finalize(ctx: OrchestratorContext, tools, partial: bool = False, fallback: bool = False, loop: int = 0) -> dict:
    """Merge agent results into kbinfos and return."""
    _merge_agent_results(ctx, tools)
    return {
        "verdict": ctx.verdict.__dict__ if ctx.verdict else None,
        "partial_answer": partial or fallback,
        "loop": loop,
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

    # Collect the chunks the agents actually cited across all claims. These
    # indices share the same positional space as kb_prompt's ``[ID:n]`` markers
    # (both index tools.kbinfos["chunks"]).
    for c in ctx.claims:
        if c.agent_result and c.agent_result.evidence_ids:
            for eid in c.agent_result.evidence_ids:
                if isinstance(eid, int):
                    seen_evidence.add(eid)

    # Drop chunks no claim ever cited (e.g. pre_search recall that didn't pan
    # out) so the final-answer LLM call only sees the useful evidence. Preserve
    # order so the re-numbered [ID:n] citations stay stable. Defensive: never
    # filter to empty — if nothing was cited, keep the full pool.
    all_chunks = tools.kbinfos.get("chunks") or []
    keep = sorted(i for i in seen_evidence if 0 <= i < len(all_chunks))
    if keep and len(keep) < len(all_chunks):
        _LOG.info("[Agentic research] Trimming evidence for the final answer: %d of %d chunk(s) were cited.", len(keep), len(all_chunks))
        tools.kbinfos["chunks"] = [all_chunks[i] for i in keep]


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
        from api.db.services.compilation_template_group_service import CompilationTemplateGroupService
        from common.misc_utils import thread_pool_exec
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
            elif kind == "wiki":
                comps.add("wiki")


def _compilation_kind_for_agentic_map(kind) -> str:
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index"}:
        return "timeline"
    return normalized
