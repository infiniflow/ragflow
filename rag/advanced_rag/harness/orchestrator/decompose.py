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
    content_words_after_entities,
)
from rag.advanced_rag.harness.orchestrator.sufficiency_llm import llm_sufficiency_boost
from rag.advanced_rag.harness.stats import in_phase
from rag.advanced_rag.harness.tools.search import hybrid_search

_LOG = logging.getLogger(__name__)

_MAX_NEXT_QUERIES = 3

# ── Evidence compaction for per-claim analysis ──────────────────────────────
# A single claim analysis historically sent ALL retrieved chunks in full
# (top_n=12 × untruncated content_with_weight) in one LLM call — benchmark logs
# showed ~56K prompt tokens for one claim, and 370K-757K for a full question.
# The dominant cost is the same chunk being re-sent verbatim across many claims
# and many rounds (evidence_ids overlap heavily between claims and repeat across
# consecutive rounds). To cut tokens WITHOUT dropping any content we deduplicate
# the evidence — a chunk already analysed for this claim (or an identical chunk
# already analysed this round) is not sent again — and only apply a hard length
# ceiling as a safety valve. We deliberately do NOT sentence-level narrow: that
# risks dropping the answer sentence (e.g. English-numbered aggregate answers
# like "two children" that the fact-dense heuristic misses).
# We cap the per-chunk length and the total, but we do NOT drop whole chunks or
# sentence-level-narrow: benchmark data shows the answer sentence can sit in a
# low-ranked chunk or be phrased with an English number ("two children") that a
# fact-dense heuristic misses. Capping only the tail of an over-long chunk (the
# least likely place for the retrieved, similarity-ranked answer) and the total
# is the information-preserving choice.
_EVIDENCE_MAX_CHUNKS = 8  # keep only the top-N most similar retrieved chunks
_EVIDENCE_MAX_CHARS_PER_CHUNK = 2000  # hard per-chunk cap (tail-only, preserves the head)
_EVIDENCE_MAX_TOTAL_CHARS = 16_000  # global ceiling for the concatenated evidence
# Cross-round evidence dedup: benchmark logs show consecutive rounds for the same
# claim re-search and re-send nearly identical chunk sets (avg 56% overlap, 18%
# byte-identical). Sending the same full chunks every round is the dominant token
# waste. We track per-claim already-analysed evidence IDs and, on later rounds,
# only send the NEW chunks plus the previous round's report (so the model judges
# with the full picture, not a degraded subset).
_EVIDENCE_DEDUP = True
# Cap for the previous-round report injected into the analysis prompt.
_EVIDENCE_PREV_REPORT_CAP = 1200

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
  "report": "A precise, self-contained evidence-backed finding for this claim. MUST preserve every exact number, year, percentage, proper noun (person/place/org/product), and specific relationship verbatim from the evidence — do not paraphrase away or generalize away the concrete values. This report is the primary material for the final answer, so it must carry the answerable fact (e.g. '1,429,475 residents of Markazi in the 2016 census', 'the 1978 Academy Award for Best Picture went to The Deer Hunter'). If the evidence is incomplete, say what is known so far and what is still missing.",
  "gaps": ["specific missing fact or relationship"],
  "intermediate": "ONE bridge entity or relationship that must be retrieved BEFORE the claim can be answered, e.g. for 'company that Brian Bergstein is employed by violates which law' the intermediate is 'Who is Brian Bergstein's employer?'. Empty string if none / not a multi-hop bridge.",
  "next_queries": [
    {{"query": "standalone follow-up search query", "is_bridge": true}},
    {{"query": "another standalone follow-up search query", "is_bridge": false}}
  ],
  "grounded": ["key asserted facts that ARE directly supported by the cited evidence, atomically and verbatim enough to match; keep the exact numbers and entity names"],
  "numbers": ["for numerical/multi-hop answers: each figure used + its source, e.g. '2,161,000 from Wikipedia Demographics of Paris'; list ALL conflicting figures if several sources disagree"],
  "query_check": {{
    "aligned_to_claim": true,
    "intent_covered": true,
    "target_entity_found": true,
    "issue": "",
    "refined_query": ""
  }}
}}
Only list in grounded the facts you actually SAW in the evidence; prior-knowledge guesses go in gaps. If the claim is numerical or multi-hop and the evidence has multiple close-but-different figures, disclose all of them in numbers rather than silently picking one. The report must be long enough to carry the full answerable fact — prefer completeness over brevity.

query_check ASSESSMENT (diagnoses THIS round's search quality, not the claim):
- aligned_to_claim=false means the search query drifted away from the claim (it targeted a different topic/entity than what the claim asks); set issue="drifted".
- target_entity_found=false means the claim's required entity (person/work/year/place the answer depends on) does NOT appear in the retrieved evidence; set issue="entity_missing".
- intent_covered=false means the query retrieved a different intent than it asked (e.g. searched "Emmy nominations" but only got biography text); set issue="intent_mismatch".
- If the query is aligned, the entity is found, and the intent is covered, set issue="" (the evidence is genuinely insufficient — a different search direction is needed, not a fix to this query).
- When issue is non-empty, provide refined_query = a precise rewrite of THIS query that fixes the problem (re-anchor to the claim for drifted, add the missing entity for entity_missing, narrow the intent for intent_mismatch/too_broad). When issue is empty, refined_query must be empty.

next_queries FORMAT (MANDATORY):
- Each element MUST be a JSON object {{"query": "...", "is_bridge": true|false}}. NEVER output plain strings.
- Every next_query MUST include the "is_bridge" field; never omit it.
- is_bridge=true means the query names ONLY an intermediate node (a bare person/place/company/work name) with NO relationship or target wording — it is just a hop toward the answer, so it will be combined with the claim description to search relation-aware.
- is_bridge=false means the query ALREADY carries the relationship or target wording, so it is self-sufficient.
- Examples:
    {{"query": "Glyn Harper", "is_bridge": true}}
    {{"query": "Glyn Harper children's book", "is_bridge": false}}
    {{"query": "population of Paris 2019", "is_bridge": false}}
- If the query is empty or would not add a new hop, omit it from the list entirely.
Return a strict JSON object with no commentary before or after."""


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
    # Cross-round evidence dedup state: which evidence IDs each claim has already
    # had analysed. Later rounds only send NEW evidence IDs plus the prior report.
    attempted_evidence: dict[str, set[int]] = {c.claim_id: set() for c in ctx.claims}
    # Per-claim pending follow-up queries, each a (query, is_bridge) tuple where
    # is_bridge comes from the LLM annotation (or None -> spaCy fallback).
    pending_queries: dict[str, list] = {c.claim_id: [] for c in ctx.claims}
    completed_cycles = 0

    # Stagnation guard: stop when the fusion score stops improving across
    # consecutive rounds (corpus lacks the data, follow-ups return nothing)
    # instead of burning the remaining cycle budget unproductively.
    prev_score: float | None = None
    _STAGNATION_CYCLES = 2
    _STAGNATION_GAIN = 0.05

    # ---- Level-2 replan (planner feedback) budget ----
    # After the inner loop exhausts its cycles without a sufficient verdict, if the
    # AutoRater feedback points at NEW sub-questions (claims decomposition missed a
    # sub-problem), re-plan with the feedback and re-run the inner loop. Cap the
    # number of replans to avoid unbounded cost. This is the "new sub-question"
    # tier between inner re-search (Level 1) and outer question-rewrite (Level 3).
    replan_budget = 2 if getattr(mode, "allows_replan", False) else 0
    replan_count = 0
    # Tracks whether the last pass ended insufficient (partial) so the loop below
    # can decide to replan instead of finalizing.
    last_partial = False
    last_feedback = ""
    # AutoRater boost + fusion verdict are defined inside the loop; default them here
    # so the while-else / replan blocks never reference an unbound name.
    boost: dict = {}
    verdict = None

    # Level-2 replan may re-enter the inner research loop with expanded claims by
    # resetting ``cycle = 0`` and ``continue`` (the continue below is inside the
    # ``while cycle < max_cycles`` loop, so it is valid). ``cycle`` is reset on
    # replan so the new claims get the full budget.
    cycle = 0
    # SmartSearch/PL-Search plan anchoring: per-claim refined query from the last
    # round's query_check (highest priority for the next _pick_next_query).
    refined_queries: dict[str, str] = {}
    while cycle < max_cycles:
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
        # AutoRater follow-ups (missing-information queries) are a global pool that
        # targets the next round at the reported gaps. ``gen_followups`` returns
        # ``[{question, query}]``; we use each follow-up's search query (falling back
        # to its question). A working copy is passed so each claim can consume one;
        # dedup via `attempted_queries` prevents re-search.
        raw_pool = getattr(ctx, "pending_followups", None) or []
        global_pool = []
        for fu in raw_pool:
            if isinstance(fu, dict):
                q = (fu.get("query") or fu.get("question") or "").strip()
            else:
                q = str(fu).strip()
            if q:
                global_pool.append(q)
        for c in unverified:
            # Pass the claim's refined query (if any) as a one-shot list; it is
            # consumed (popped) inside _pick_next_query and cleared below so it is
            # not retried on every round.
            _refined = refined_queries.get(c.claim_id, "")
            query = _pick_next_query(
                question,
                c,
                attempted_queries.setdefault(c.claim_id, set()),
                pending_queries.setdefault(c.claim_id, []),
                global_pool=global_pool,
                refined=[_refined] if _refined else None,
            )
            if _refined:
                refined_queries.pop(c.claim_id, None)
            if not query:
                _LOG.info("[Decompose search] No unused follow-up query remains for claim %s.", c.claim_id)
                continue
            attempted_queries[c.claim_id].add(_normalize_query(query))
            searched_claims.append((c, query))
            # SmartSearch / PL-Search: when this query is a refined (plan-anchored)
            # query from the last round's query_check, do NOT append the global
            # formalize keywords — the refined query already carries the corrected
            # direction and re-appending the old global keywords would re-introduce
            # the drift / pollution the refinement was meant to fix.
            _ref_kw = "" if _refined else keywords
            # NOTE: top_n deliberately NOT adjusted per claim-type. All claims
            # share the default hybrid_search top-k; an aggregate claim's breadth
            # comes from its enumeration-variant queries, not a wider top-k.
            tasks.append(hybrid_search(tools, query=query, keywords=_ref_kw, use_compiled=True))

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
            seen_ids = attempted_evidence.setdefault(c.claim_id, set())
            # Prior report from this claim's last round (if any) — used as the
            # context anchor when cross-round dedup drops already-analysed chunks.
            prev_report = (c.agent_result.report if c.agent_result else "") or ""
            analysis_inputs.append((c, query, result, evidence_ids, prev_report, seen_ids))

        analyses = await asyncio.gather(
            *[
                _analyze_claim_evidence(
                    question=question,
                    claim=c,
                    query=query,
                    result=result,
                    evidence_ids=evidence_ids,
                    prev_report=prev_report,
                    seen_evidence_ids=seen_ids,
                    cycle=cycle,
                    max_cycles=max_cycles,
                    tools=tools,
                )
                for c, query, result, evidence_ids, prev_report, seen_ids in analysis_inputs
            ],
            return_exceptions=True,
        )

        for (c, query, result, evidence_ids, prev_report, seen_ids), analysis in zip(analysis_inputs, analyses):
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
            # SmartSearch Process Reward / PL-Search plan anchoring: when the
            # model diagnosed a fixable search problem this round, re-search with
            # its precise refined query FIRST on the next round (highest priority
            # via the `refined` argument to _pick_next_query, before the follow-up
            # pool and brand-new directions). The refined query is stored per-claim
            # and consumed once.
            qc = analysis.get("query_check") or {}
            refined = (qc.get("refined_query") or "").strip()
            if not c.is_verified and refined:
                _norm_r = _normalize_query(refined)
                if _norm_r and _norm_r not in attempted_queries.setdefault(c.claim_id, set()):
                    refined_queries[c.claim_id] = refined
                    _LOG.info(
                        "[Decompose search] Claim %s: query_check issue='%s' -> queued refined query (highest priority).",
                        c.claim_id,
                        qc.get("issue", ""),
                    )
                else:
                    _LOG.info(
                        "[Decompose search] Claim %s: query_check issue='%s' but refined query already attempted; skipped.",
                        c.claim_id,
                        qc.get("issue", ""),
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
            # Record this round's evidence as analysed so future rounds skip
            # re-sending it verbatim (cross-round dedup).
            attempted_evidence.setdefault(c.claim_id, set()).update(evidence_ids)

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

        # LLM groundedness review (Google "draft review"): validates the claim drafts
        # against the cited evidence. It runs whenever this round could produce an
        # answer (code verdict SUFFICIENT / borderline), but is skipped for a verdict
        # that clearly won't (INSUFFICIENT / UNANSWERABLE → loop continues or abstains),
        # so the token cost of the unconditional draft review is bounded. Ungrounded
        # claim drafts are merged into hard_violations → decision ladder caveat.
        from rag.advanced_rag.harness.orchestrator.grounded_llm import llm_grounded_verify

        grounded: dict = {}
        if verdict.status not in ("INSUFFICIENT", "UNANSWERABLE"):
            # Union of cited evidence IDs across all claim results (matches the
            # agentic orchestrator's cited-evidence behavior) so the reviewer sees
            # the exact evidence each claim referenced, not a global prefix.
            cited_evidence_ids: list[str] = []
            for r in agent_results:
                cited_evidence_ids.extend(r.evidence_ids or [])
            try:
                grounded = await llm_grounded_verify(
                    tools,
                    ctx.question,
                    [(r.claim_id, r.report or "") for r in agent_results if r.report],
                    cited_evidence_ids or None,
                )
            except Exception:
                _LOG.exception("[Decompose] grounded review failed; proceeding without grounded veto")
                grounded = {}
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

        # ---- Selective generation (paper §3/§5.1) ----
        # The paper shows models often still answer correctly (35-62%) even when the
        # sufficiency verdict says "not enough context". So on UNANSWERABLE with any
        # retrieved evidence, do not hard-abstain (which guarantees 0) — downgrade to
        # a caveated/partial answer and let the answer stage try. Only a true empty
        # evidence pool abstains.
        if action == "UNANSWERABLE" and (tools.kbinfos.get("chunks") or []):
            _LOG.info("[Decompose] Selective: UNANSWERABLE but %d chunk(s) retrieved — downgrading to partial answer.", len(tools.kbinfos.get("chunks") or []))
            action = "ANSWER_PARTIAL"
            should_continue = False

        # Stagnation guard: if the verdict is not (yet) sufficient and the score
        # has not meaningfully improved across consecutive rounds, stop instead
        # of burning the remaining cycle budget on unproductive re-searches.
        if should_continue and verdict.status in ("INSUFFICIENT", "USEFUL_BUT_INCOMPLETE"):
            if prev_score is not None and cycle >= _STAGNATION_CYCLES and verdict.score - prev_score < _STAGNATION_GAIN:
                _LOG.info(
                    "[Decompose] Round %d: score stagnant (%.3f → %.3f) — trying Level-2 replan before settling for partial",
                    cycle + 1,
                    prev_score,
                    verdict.score,
                )
                # Stagnation usually means the current claims cannot be resolved with
                # the corpus — but the AutoRater feedback may still point at NEW
                # sub-questions. Try Level-2 replan before settling for partial.
                last_partial = any(not c.is_verified for c in ctx.claims)
                fb = boost.get("feedback") or verdict.feedback or ""
                last_feedback = str(fb) if fb else ""
                replanned = False
                if last_partial and replan_count < replan_budget and last_feedback:
                    replanned = await _try_replan(ctx, tools, question, keywords, route, last_feedback, replan_count)
                if replanned:
                    replan_count += 1
                    attempted_queries.update({c.claim_id: set() for c in ctx.claims if c.claim_id not in attempted_queries})
                    pending_queries.update({c.claim_id: [] for c in ctx.claims if c.claim_id not in pending_queries})
                    last_partial = False
                    last_feedback = ""
                    prev_score = None
                    cycle = 0
                    continue  # re-enter the inner loop with the expanded claims
                action = "ANSWER_PARTIAL"
                should_continue = False
            else:
                prev_score = verdict.score

        # ---- SCA review (Google Phase 5) ----
        # Before accepting a full ANSWER, the SCA double-checks for missing pieces.
        # If the AutoRater explicitly reported missing information but the decision
        # ladder still produced a full ANSWER (a contradiction), downgrade to
        # ANSWER_PARTIAL so the final answer does not overclaim. This is a cheap
        # last-quality-gate and only fires on an explicit missing signal.
        if action == "ANSWER":
            _sca_missing = boost.get("missing") or []
            if _sca_missing:
                _LOG.info("[Decompose] SCA review: AutoRater reported %d missing piece(s) despite full-ANSWER verdict — downgrading to partial.", len(_sca_missing))
                action = "ANSWER_PARTIAL"
        if action in ("ANSWER", "ANSWER_PARTIAL"):
            return _finalize(ctx, tools, partial=action == "ANSWER_PARTIAL", loop=completed_cycles)
        if action == "ABSTAIN":
            tools.kbinfos["chunks"] = []
            return {"verdict": verdict.__dict__, "abstain": True, "loop": completed_cycles}
        if action == "FALLBACK_LLM":
            return _finalize(ctx, tools, partial=True, loop=completed_cycles)
        if not should_continue:
            # Verdict says stop (e.g. GAP budget exhausted or ladder decided stop).
            # If the verdict is still partial and the feedback points at NEW
            # sub-questions (Level 2), re-plan the claims with the feedback and
            # re-enter the inner loop (same question/keywords → same search
            # direction) instead of finalizing a partial answer.
            last_partial = any(not c.is_verified for c in ctx.claims)
            fb = boost.get("feedback") or verdict.feedback or ""
            last_feedback = str(fb) if fb else ""
            replanned = False
            if last_partial and replan_count < replan_budget and last_feedback:
                replanned = await _try_replan(ctx, tools, question, keywords, route, last_feedback, replan_count)
            if replanned:
                replan_count += 1
                attempted_queries.update({c.claim_id: set() for c in ctx.claims if c.claim_id not in attempted_queries})
                pending_queries.update({c.claim_id: [] for c in ctx.claims if c.claim_id not in pending_queries})
                last_partial = False
                last_feedback = ""
                prev_score = None
                cycle = 0
                continue  # re-enter the inner loop with the expanded claims
            break
        cycle += 1
    else:
        # while-else: loop exhausted max_cycles without a sufficient verdict.
        # (No replan here — running out of cycles usually means the corpus lacks
        # the data; replan would not help. Finalize as partial.)
        pass

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
    prev_report: str = "",
    seen_evidence_ids: set[int] | None = None,
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
            "next_queries": [(q, None) for q in _fallback_queries(question, claim)],
        }

    # Single-pass evidence analysis: send the compacted retrieved evidence to the
    # model in ONE call. No per-chunk batching, no accumulated summary — the model
    # sees the relevant evidence at once and returns one analysis (report/gaps/
    # next_queries) covering everything. This trades a single larger LLM call for
    # many small round-trips (faster in wall-clock terms) while compaction keeps
    # that single call from ballooning (see _build_compact_evidence).
    try:
        # Cross-round dedup: if this claim already had some evidence IDs analysed
        # in a previous round, only send the NEW chunks. The previous round's
        # report is injected as context so the model still judges with the full
        # picture (known facts + new evidence), not a degraded subset.
        if _EVIDENCE_DEDUP and seen_evidence_ids:
            seen = set(seen_evidence_ids)
            new_ids = [i for i in evidence_ids if i not in seen]
            new_chunks = []
            all_chunks = tools.kbinfos.get("chunks", [])
            for i in new_ids:
                if 0 <= i < len(all_chunks):
                    new_chunks.append(all_chunks[i])
            evidence = _build_compact_evidence(new_chunks) if new_chunks else ""
            if evidence:
                _LOG.info(
                    "[Decompose search] Claim %s: sending %d new chunk(s) (deduped %d previously-analysed).",
                    claim.claim_id,
                    len(new_chunks),
                    len(seen) - len(seen.intersection(evidence_ids)),
                )
            else:
                # All evidence was already analysed — still call the model with a
                # minimal re-check driven by the prior report (cheap, keeps the
                # verdict fresh without re-sending full evidence).
                evidence = ""
            return await _analyze_from_summary(
                question,
                claim,
                query,
                cycle,
                max_cycles,
                evidence,
                result,
                evidence_ids,
                tools,
                prev_report=prev_report,
                all_deduped=(not evidence),
            )
        evidence = _build_compact_evidence(chunks)
        if not evidence:
            return {
                "is_verified": False,
                "confidence": 0.0,
                "report": "",
                "gaps": ["no evidence found"],
                "next_queries": [(q, None) for q in _fallback_queries(question, claim)],
            }
        return await _analyze_from_summary(question, claim, query, cycle, max_cycles, evidence, result, evidence_ids, tools, prev_report=prev_report)
    except Exception:
        _LOG.exception("[Decompose search] Evidence analysis LLM call failed.")
        return _fallback_analysis(result, cycle, max_cycles, question, claim)


def _chunk_text(chunk) -> str:
    """Extract the full text of a chunk without truncation."""
    if isinstance(chunk, dict):
        return str(chunk.get("content_with_weight") or chunk.get("content") or chunk.get("text") or "")
    return str(chunk or "")


def _build_compact_evidence(chunks: list[dict]) -> str:
    """Compile the per-claim evidence for the analysis LLM call.

    Reduces the token cost of a single analysis while preserving answer accuracy.
    The compaction is deliberately conservative — it never drops a whole chunk and
    never does sentence-level narrowing (benchmark data shows the answer sentence
    can sit in a low-ranked chunk or be phrased with an English number like "two
    children" that a fact-dense heuristic misses):

      1. Keep only the top ``_EVIDENCE_MAX_CHUNKS`` chunks by retrieval
         similarity — hybrid_search already ranks, and the answer sentence lives
         with high probability in the higher-ranked passages. Chunks beyond the
         cap are the least likely to carry the answer.
      2. Cap each kept chunk's length at ``_EVIDENCE_MAX_CHARS_PER_CHUNK`` — tail
         only (preserving the head where the similarity-ranked content lives).
      3. Cap the concatenated total at ``_EVIDENCE_MAX_TOTAL_CHARS``.

    Returns ``""`` when nothing usable remains (caller then treats it as no
    evidence found, which is safe: it only happens when every chunk was empty).
    """
    if not chunks:
        return ""
    ordered = sorted(
        (c for c in chunks if _chunk_text(c)),
        key=lambda c: float(c.get("similarity") or 0.0),
        reverse=True,
    )[:_EVIDENCE_MAX_CHUNKS]
    parts: list[str] = []
    used = 0
    for c in ordered:
        text = _chunk_text(c)
        if len(text) > _EVIDENCE_MAX_CHARS_PER_CHUNK:
            text = text[:_EVIDENCE_MAX_CHARS_PER_CHUNK]
        if not text.strip():
            continue
        if used + len(text) > _EVIDENCE_MAX_TOTAL_CHARS:
            room = _EVIDENCE_MAX_TOTAL_CHARS - used
            if room >= 200:
                parts.append(text[:room])
            break
        parts.append(text)
        used += len(text)
    return "\n\n---\n\n".join(parts)


async def _analyze_from_summary(
    question: str,
    claim: ClaimTarget,
    query: str,
    cycle: int,
    max_cycles: int,
    summary: str,
    result: dict,
    evidence_ids: list[int],
    tools,
    prev_report: str = "",
    all_deduped: bool = False,
) -> dict:
    """Final analysis when no single passage was judged sufficient on its own.

    The ``summary`` argument here is the FULL concatenated text of all retrieved
    chunks (no per-chunk context was accumulated during the batched independent
    check), so the model still gets a complete best-effort evidence review
    instead of an empty report.

    When cross-round dedup drops already-analysed evidence, ``prev_report`` (the
    claim's conclusion from earlier rounds) is injected so the model re-judges
    with the known facts as context; ``all_deduped`` marks the degenerate case
    where every evidence ID was already analysed and only the prior report is
    available this round.
    """
    prev_section = ""
    if prev_report:
        prev_section = (
            "\n\nEvidence analysed in a previous round:\n"
            + prev_report[:_EVIDENCE_PREV_REPORT_CAP]
            + ("\n(No new evidence this round — re-confirm or refine the finding above using the retrieved evidence list; if still incomplete, say so.)" if all_deduped else "")
        )
    user = _EVIDENCE_ANALYSIS_USER.format(
        question=question,
        claim=claim.description,
        query=query,
        cycle=cycle + 1,
        max_cycles=max_cycles,
        evidence=summary or "(no usable evidence accumulated)",
    )
    if prev_section:
        # Append the prior-round context after the JSON spec block. We inject it
        # via the closing instruction so it does not disturb the JSON template.
        user = user + prev_section
    try:
        msg = await tools._fit_messages(_EVIDENCE_ANALYSIS_SYSTEM, user)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], {"temperature": 0.1})
        if isinstance(ans, tuple):
            ans = ans[0]
        parsed = _extract_json(ans)
        return _normalize_analysis(parsed, result, evidence_ids, question, claim, cycle, max_cycles)
    except Exception:
        _LOG.exception("[Decompose search] Final summary analysis LLM call failed.")
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

        parsed = json_repair.loads(text)
    except Exception:
        try:
            parsed = json.loads(text)
        except Exception:
            _LOG.warning("[Decompose search] Failed to parse evidence analysis output: %s", text[:200])
            return {}
    if not isinstance(parsed, dict):
        # json_repair can return a bare string / list / primitive when the LLM
        # emits non-object JSON. Coerce to {} so ``_normalize_analysis`` and
        # callers can safely use ``parsed.get(...)`` without AttributeError.
        _LOG.warning(
            "[Decompose search] LLM returned non-object JSON (%s), coercing to {}: %s",
            type(parsed).__name__,
            str(parsed)[:200],
        )
        return {}
    return parsed


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
    # Multi-hop step-wise retrieval: if the claim needs a bridge entity/relationship
    # (e.g. "employer of X" before "which law X's employer violated"), the analysis
    # emits an ``intermediate`` query. Search it FIRST next round so we resolve the
    # intermediate node (e.g. Uber) before re-targeting the final answer. This is what
    # makes multi-hop questions retrieve correctly instead of one-shot keyword hit.
    intermediate = str(parsed.get("intermediate") or "").strip()
    # ``next_queries`` is now a list of {"query": ..., "is_bridge": ...} objects
    # (backward compatible with plain string lists). Each entry becomes a
    # ``(query, is_bridge)`` tuple where ``is_bridge`` is True/False from the
    # LLM or ``None`` when the LLM did not annotate it (caller then falls back
    # to the spaCy heuristic in ``_is_bridge_query``).
    next_queries = _parse_next_queries(parsed.get("next_queries"))[:_MAX_NEXT_QUERIES]
    if intermediate and not is_verified:
        # The ``intermediate`` field is, by definition, a bare bridge query that
        # must be searched before re-targeting the claim -> force is_bridge=True.
        next_queries = [(intermediate, True)] + [q for q in next_queries if q[0] != intermediate]
        next_queries = next_queries[:_MAX_NEXT_QUERIES]
    grounded = _string_list(parsed.get("grounded"))
    numbers = _string_list(parsed.get("numbers"))
    # SmartSearch Process Reward / PL-Search plan anchoring: the model diagnoses
    # THIS round's search quality (drifted / entity_missing / intent_mismatch) and,
    # when a fixable problem exists, supplies a precise refined_query to re-search
    # next round (instead of falling through to a brand-new direction).
    query_check = _normalize_query_check(parsed.get("query_check"))

    if is_verified:
        gaps = []
        next_queries = []
        # NOTE: no optimistic floor here. The old ``max(confidence, 0.65)``
        # inflated medium's agent confidence and distorted the decision-ladder
        # gate (agent_confidence >= c_high/c_low). Keep the LLM's raw confidence
        # so the ladder's thresholds behave as designed.
    elif not next_queries and cycle + 1 < max_cycles:
        next_queries = [(q, None) for q in _fallback_queries(question, claim)]

    return {
        "is_verified": is_verified,
        "confidence": confidence,
        "report": report,
        "gaps": gaps,
        "next_queries": next_queries,
        "grounded": grounded,
        "numbers": numbers,
        "query_check": query_check,
    }


def _parse_next_queries(raw) -> list[tuple[str, bool | None]]:
    """Parse the LLM's ``next_queries`` field into ``(query, is_bridge)`` pairs.

    Accepts both the new object form (``[{"query": ..., "is_bridge": bool}]``)
    and the legacy plain-string form (``["..."]``). For plain strings the
    ``is_bridge`` flag is ``None``, signalling the caller to fall back to the
    language-agnostic spaCy heuristic. Skips blank queries.
    """
    out: list[tuple[str, bool | None]] = []
    if isinstance(raw, str):
        raw = [raw]
    if not isinstance(raw, list):
        return out
    for item in raw:
        if isinstance(item, str):
            q = item.strip()
            if q:
                out.append((q, None))
            continue
        if isinstance(item, dict):
            q = str(item.get("query") or "").strip()
            if not q:
                continue
            flag = item.get("is_bridge")
            out.append((q, flag if isinstance(flag, bool) else None))
            continue
    return out


def _normalize_query_check(raw) -> dict:
    """Normalise the LLM's ``query_check`` into a fixed-shape dict.

    SmartSearch-style Process Reward / PL-Search plan anchoring: diagnoses THIS
    round's search quality (whether the query stayed aligned to the claim, the
    claim's target entity appeared in the evidence, and the query's intent was
    covered). When a fixable problem exists, ``issue`` is set and ``refined_query``
    carries a precise rewrite to re-search next round. Unknown issues degrade
    gracefully to an empty (no-problem) assessment.
    """
    if not isinstance(raw, dict):
        return {"aligned_to_claim": True, "intent_covered": True, "target_entity_found": True, "issue": "", "refined_query": ""}
    valid_issues = {"drifted", "entity_missing", "intent_mismatch", "too_broad"}
    issue = str(raw.get("issue") or "").strip()
    if issue not in valid_issues:
        issue = ""
    refined = str(raw.get("refined_query") or "").strip()
    if issue and not refined:
        refined = ""
    return {
        "aligned_to_claim": bool(raw.get("aligned_to_claim", True)),
        "intent_covered": bool(raw.get("intent_covered", True)),
        "target_entity_found": bool(raw.get("target_entity_found", True)),
        "issue": issue,
        "refined_query": refined,
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
    next_queries = [] if is_last_cycle or claim is None else [(q, None) for q in _fallback_queries(question, claim)]
    return {
        "is_verified": is_verified,
        "confidence": 0.55 if is_verified else (0.35 if chunks else 0.0),
        "report": _summarize(result),
        "gaps": [] if is_verified else ["need more specific evidence"],
        "next_queries": next_queries,
        "grounded": [],
        "numbers": [],
        "query_check": {"aligned_to_claim": True, "intent_covered": True, "target_entity_found": True, "issue": "", "refined_query": ""},
    }


async def _try_replan(
    ctx: OrchestratorContext,
    tools,
    question: str,
    keywords: str,
    route,
    feedback: str,
    replan_count: int,
) -> bool:
    """Level-2: re-plan the claims with the sufficient feedback.

    Calls the planner with ``feedback`` so it can decompose the question into NEW
    sub-questions that the original plan missed. New claims (not already in
    ``ctx.claims``) are appended and later researched by the re-entered inner loop.
    Returns True if at least one new claim was added.
    """
    from rag.advanced_rag.harness.planner import planner_node

    existing_desc = {c.description.strip().lower() for c in ctx.claims}
    pstate = {
        "question": question,
        "keywords": keywords,
        "route": route,
        "seed_chunks": tools.kbinfos.get("chunks", []),
        "feedback": feedback,
    }
    _LOG.info("[Decompose] Level-2 replan (attempt %d) with feedback: %s", replan_count + 1, _snip(feedback))
    try:
        plan = await planner_node(pstate, tools)
    except Exception:
        _LOG.exception("[Decompose] Level-2 replan failed")
        return False
    new_claims_raw = plan.get("claims", []) or []
    added = 0
    for nc in new_claims_raw:
        obj = ClaimTarget(**nc) if isinstance(nc, dict) else nc
        key = obj.description.strip().lower()
        if key and key not in existing_desc:
            obj.claim_id = f"r{replan_count}_{obj.claim_id}"
            ctx.claims.append(obj)
            existing_desc.add(key)
            added += 1
    if added:
        _LOG.info("[Decompose] Replan added %d new claim(s); total %d.", added, len(ctx.claims))
    return added > 0


def _pick_next_query(
    question: str,
    claim: ClaimTarget,
    attempted: set[str],
    pending: list,
    global_pool: list | None = None,
    refined: list | None = None,
) -> str:
    # 0. SmartSearch Process Reward / PL-Search plan anchoring: when THIS round's
    #    query_check diagnosed a fixable search problem (drifted / entity_missing /
    #    intent_mismatch), re-search with the model's precise refined query FIRST,
    #    instead of falling through to a brand-new direction. This keeps the plan
    #    anchored to the claim instead of drifting to unrelated content.
    #
    #    PL-Search plan anchoring: the refined query is combined with the claim
    #    description so the retrieval stays anchored to what the claim actually asks
    #    (re-anchor a drifted query, keep the missing entity / target intent in view),
    #    rather than drifting to a bare, context-free keyword search.
    if refined:
        _desc = (claim.description or "").strip()
        while refined:
            query = (refined.pop(0) or "").strip()
            normalized = _normalize_query(query)
            if normalized and normalized not in attempted:
                if _desc and _normalize_query(query) != _normalize_query(_desc):
                    # Combine the refined query with the claim anchor so the search
                    # targets the claim's intent + entity, not the bare refined terms.
                    anchored = f"{query} {_desc}"
                    if _normalize_query(anchored) not in attempted:
                        return anchored
                return query

    # 1. Multi-hop: resolve the claim's prerequisite (bridge entity/relationship)
    # before targeting the claim itself. The first time the claim is researched, the
    # prerequisite OPEN query is searched; the found entity feeds back through the
    # evidence the next round (the analyzer then re-targets the claim).
    prereq = (claim.prerequisite or "").strip()
    if prereq and not attempted:
        normalized = _normalize_query(prereq)
        if normalized and normalized not in attempted:
            return prereq

    # 1. claim-specific pending follow-ups (from _analyze_claim_evidence).
    #    Multi-hop strengthening: when the pending query is a BRIDGE query (the
    #    analyzer emitted an ``intermediate`` entity/relationship), combine it with
    #    the claim description so the retrieval hits the "bridge entity -> final
    #    target" relationship in one pass (e.g. for "Glyn Harper wrote which
    #    children's book", the bridge "Glyn Harper" combines with "children's book"
    #    to retrieve the title, instead of stopping at the bridge).
    while pending:
        item = pending.pop(0)
        query, flag = item if isinstance(item, tuple) else (item, None)
        query = (query or "").strip()
        normalized = _normalize_query(query)
        if normalized and normalized not in attempted:
            # If the pending query is a bare bridge entity (only an intermediate
            # node, no relationship to the answer), prepend the claim description
            # so the search is relation-aware and keeps the multi-hop chain
            # advancing past the intermediate node toward the final answer.
            #
            # Bridge determination: prefer the LLM's ``is_bridge`` annotation
            # (``flag``); when the LLM did not annotate it (``None``), fall back
            # to the language-agnostic spaCy NER heuristic in ``_is_bridge_query``.
            desc = (claim.description or "").strip()
            is_bridge = flag if flag is not None else (bool(desc) and _is_bridge_query(query))
            if is_bridge:
                combined = f"{query} {desc}"
                if _normalize_query(combined) not in attempted:
                    return combined
            return query

    # 2. AutoRater follow-ups (missing-information queries) — a global pool for the
    #    whole question. Consume them before falling back to claim-description
    #    paraphrases, so the next round is *targeted* at the reported gaps rather
    #    than re-searching the same angles.
    if global_pool:
        while global_pool:
            query = (global_pool.pop(0) or "").strip()
            normalized = _normalize_query(query)
            if normalized and normalized not in attempted:
                return query

    candidates = []
    if not attempted:
        candidates.append(claim.description)
    # Structure-aware query expansion:
    #   aggregate — enumerate the full target set (e.g. "every MLB stadium with a
    #               retractable roof"), not just the claim's summary phrasing, so
    #               exhaustive retrieval actually covers all members.
    #   temporal  — keep the year/period explicit so the search is time-anchored.
    if claim.claim_type == "aggregate" and claim.target:
        candidates.append(claim.target)
        candidates.append(f"list of {claim.target}")
        candidates.append(claim.description)
    candidates.extend(_fallback_queries(question, claim))

    for query in candidates:
        query = (query or "").strip()
        normalized = _normalize_query(query)
        if normalized and normalized not in attempted:
            return query
    return ""


def _is_bridge_query(query: str) -> bool:
    """True when ``query`` is a bare bridge entity — a short intermediate node
    that names a person/place/thing but no relationship — so it should be
    combined with the claim description to keep the multi-hop chain moving.

    The primary source of truth is the LLM's ``is_bridge`` annotation on
    ``next_queries`` (see ``_EVIDENCE_ANALYSIS_USER``). This function is the
    language-agnostic *fallback* for queries the LLM did not annotate.

    Heuristic (no per-language rules): a bare bridge query carries named
    entities but, once those spans are stripped, no remaining *content* word —
    judged by spaCy POS tags (NOUN/VERB/ADJ/ADV/PROPN/NUM) in
    ``content_words_after_entities``. Because spaCy assigns POS tags for
    en/zh/de/fr/es/pt/ja identically, no English verb/stopword list is needed:
    e.g. "Glyn Harper" / "福楼拜" / "Apple" (no leftover content) are bridges,
    while "Glyn Harper children book" / "福楼拜 写了 包法利夫人" / "population of
    Paris" (leftover content) are relation-aware.
    """
    q = query.strip()
    if not q:
        return False
    return not content_words_after_entities(q)


def _fallback_queries(question: str, claim: ClaimTarget) -> list[str]:
    candidates = []
    for gap in _agent_result_gaps(claim.agent_result):
        candidates.append(f"{claim.description} {gap}")
    if question:
        candidates.append(f"{question} {claim.description}")
    candidates.append(claim.description)
    return candidates[:_MAX_NEXT_QUERIES]


def _new_queries(raw_queries: list, attempted: set[str]) -> list:
    """Filter and dedupe ``next_queries`` (each a ``(query, is_bridge)`` tuple)
    against the already-attempted set. Returns the surviving tuple list."""
    queries = []
    seen = set(attempted)
    for item in raw_queries:
        query, flag = item if isinstance(item, tuple) else (item, None)
        query = (query or "").strip()
        normalized = _normalize_query(query)
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        queries.append((query, flag))
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


_PRE_SUMMARY_REPORT_CAP = 900
_PRE_SUMMARY_FACTS_CAP = 400


def _build_pre_summary(ctx: OrchestratorContext) -> str | None:
    """Build a compact but fact-preserving ``pre_summary`` from per-claim results.

    This is the primary evidence source for ``formalize_answer``. Unlike the old
    ``report[:500]`` concatenation (which could truncate mid-information and drop
    the exact answerable value), it keeps each claim's report AND its grounded
    facts and figures — the fields that carry the precise numbers / proper nouns
    the answer hinges on. ``numbers``/``grounded`` are placed right after the
    report so a downstream ``_compact_text`` truncation still keeps the critical
    facts. Length is capped per claim to bound the prompt, but the cap is applied
    to the tail of each block, so leading facts survive.
    """
    combined = []
    for claim in ctx.claims:
        if not claim.agent_result or not claim.agent_result.report:
            continue
        status = "verified" if claim.is_verified else "incomplete"
        block = f"[{claim.claim_id}] {status} ({claim.description}): {claim.agent_result.report}"
        if claim.agent_result.numbers:
            block += "\n   figures: " + "; ".join(claim.agent_result.numbers)
        if claim.agent_result.grounded:
            block += "\n   facts: " + "; ".join(claim.agent_result.grounded)
        if len(block) > _PRE_SUMMARY_REPORT_CAP + _PRE_SUMMARY_FACTS_CAP:
            block = block[: _PRE_SUMMARY_REPORT_CAP + _PRE_SUMMARY_FACTS_CAP] + "…"
        combined.append(block)
    if not combined:
        return None
    return "Research findings. These may include bridge entities; the final answer must still satisfy the original question's requested role.\n\n" + "\n\n".join(combined)


def _finalize(ctx: OrchestratorContext, tools, partial: bool, loop: int) -> dict:
    pre_summary = _build_pre_summary(ctx)
    if pre_summary:
        tools.kbinfos["pre_summary"] = pre_summary

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
