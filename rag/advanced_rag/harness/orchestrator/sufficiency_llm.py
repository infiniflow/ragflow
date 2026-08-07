"""LLM Sufficient Context AutoRater for the orchestrator.

This is the *primary* sufficiency judge in the decision-ladder design
(Sufficient Context paper, arXiv 2411.06037). We delegate to the LLM
AutoRater (``tools.judge_sufficiency``, backing prompt ``sufficiency_select``)
to decide whether the retrieved evidence actually supports a plausible answer.

  - AutoRater judges query + evidence semantically, not just keyword matching.
  - On "insufficient" it returns concrete ``missing_information`` which we turn
    into follow-up search queries (missing-pieces feedback) for the next round.
  - It also returns ``confidence`` / ``contradictions`` / ``required_entities``
    consumed by the decision ladder (``sufficiency_ladder``).

Call frequency: high/ultra invoke it every round; medium keeps it gated to the
critical band (token cost stays bounded for the default mode). The
``verdict.status in (...)"`` check below implements that medium gate.
"""

from __future__ import annotations

import logging

from rag.advanced_rag.harness.types import SufficiencyVerdict

_LOG = logging.getLogger(__name__)


# Cap the AutoRater's evidence so a single judge call does not balloon to
# hundreds of KB (6.log showed 271KB from 80 accumulated passages). The judge
# only needs the *cited* evidence plus a bounded prefix — not the whole pool.
_MAX_EVIDENCE_CHUNKS = 24
_MAX_CHUNK_CHARS = 800
_MAX_EVIDENCE_CHARS = 24_000


def _clamp(value) -> float:
    """Clamp the AutoRater's confidence field to [0, 1], defaulting to 1.0."""
    try:
        return max(0.0, min(1.0, float(value)))
    except (TypeError, ValueError):
        return 1.0


def _evidence_md(tools, evidence_ids=None) -> str:
    """Render cited evidence chunks with ``ID: n`` markers for the AutoRater.

    Prefers the chunks referenced by ``evidence_ids`` (the union of all claims'
    evidence); falls back to a bounded prefix of the pool when no ids are
    given. Either way the output is capped (see module constants) so the LLM
    call stays cheap.
    """
    kb = getattr(tools, "kbinfos", None)
    chunks = (kb or {}).get("chunks", [])
    if not chunks:
        return ""

    picked: list[tuple[int, dict]] = []
    if evidence_ids:
        seen = set()
        for eid in evidence_ids:
            try:
                idx = int(eid)
            except (TypeError, ValueError):
                continue
            if 0 <= idx < len(chunks) and idx not in seen:
                picked.append((idx, chunks[idx]))
                seen.add(idx)
            if len(picked) >= _MAX_EVIDENCE_CHUNKS:
                break
    if not picked:
        picked = [(i, c) for i, c in enumerate(chunks[:_MAX_EVIDENCE_CHUNKS])]

    blocks: list[str] = []
    used = 0
    for idx, c in picked:
        text = c.get("content_with_weight") or c.get("text") or ""
        text = text[:_MAX_CHUNK_CHARS]
        if used + len(text) > _MAX_EVIDENCE_CHARS:
            break
        blocks.append(f"ID: {idx} | {c.get('docnm_kwd', '')}\n{text}")
        used += len(text) + 8
    return "\n\n".join(blocks)


async def llm_sufficiency_boost(
    tools,
    question: str,
    verdict: SufficiencyVerdict,
    evidence_ids=None,
) -> dict:
    """Phase-2 LLM fallback, gated to the critical verdict band.

    Returns ``{}`` when no LLM boost is applicable (verdict already clear, or
    the tools object lacks an LLM judge). Otherwise returns::

        {
          "is_sufficient": bool,   # LLM's call
          "missing": [str, ...],   # concrete gaps (Google missing pieces)
          "followups": [ {question, query}, ... ],  # next-round search queries
        }
    """
    # Only boost ambiguous verdicts — clear SUFFICIENT/UNANSWERABLE need no LLM.
    if verdict.status not in ("USEFUL_BUT_INCOMPLETE", "INSUFFICIENT", "CONFLICTING"):
        return {}
    if not hasattr(tools, "judge_sufficiency"):
        return {}
    chat_mdl = getattr(tools, "chat_mdl", None)
    if chat_mdl is None:
        return {}

    evidence_md = _evidence_md(tools, evidence_ids)
    if not evidence_md:
        return {}

    _LOG.info("[LLM-sufficiency] verdict=%s → triggering LLM Sufficient Context AutoRater (evidence %d chars)", verdict.status, len(evidence_md))
    try:
        llm_result = await tools.judge_sufficiency(question, evidence_md) or {}
    except Exception as exc:
        _LOG.info("[LLM-sufficiency] judge_sufficiency failed: %s", exc)
        return {}

    is_suff = bool(llm_result.get("is_sufficient") or llm_result.get("Sufficient Context"))
    missing = [str(m) for m in (llm_result.get("missing_information") or llm_result.get("missing") or []) if str(m).strip()]
    reasoning = (llm_result.get("reasoning") or "").strip()
    # New Sufficient Context paper fields consumed by the decision ladder.
    auto_confidence = _clamp(llm_result.get("confidence"))
    contradictions = [str(c) for c in (llm_result.get("contradictions") or []) if str(c).strip()]
    required_entities = [str(e) for e in (llm_result.get("required_entities") or []) if str(e).strip()]
    coverage = llm_result.get("coverage") or {}
    _LOG.info(
        "[LLM-sufficiency] is_sufficient=%s confidence=%.2f contradictions=%s missing=%s reasoning=%s",
        is_suff,
        auto_confidence,
        contradictions,
        missing,
        reasoning[:200],
    )

    followups: list[dict] = []
    if missing and hasattr(tools, "gen_followups"):
        try:
            followups = await tools.gen_followups(question, question, missing, evidence_md) or []
        except Exception as exc:
            _LOG.info("[LLM-sufficiency] gen_followups failed: %s", exc)

    if followups:
        _LOG.info("[LLM-sufficiency] %d follow-up query(ie)s generated for next round: %s", len(followups), [q.get("question", "") for q in followups])

    return {
        "is_sufficient": is_suff,
        "confidence": auto_confidence,
        "missing": missing,
        "contradictions": contradictions,
        "required_entities": required_entities,
        "coverage": coverage,
        "reasoning": reasoning,
        "followups": followups,
    }
