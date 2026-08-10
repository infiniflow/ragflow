"""LLM groundedness review (draft review) for the orchestrator.

Inspired by Google's Sufficient Context Agent — it reviews the *intermediate
draft* (each claim's report) against the retrieved snippets to decide whether
the draft is actually grounded in the evidence.

This complements the code-level grounded check (``cross_check_claim`` /
``_grounded_hit``, which is lexical: it only catches missing entities / digits).
The LLM review is *semantic*: it catches assertions whose relation or claim is
not supported by the evidence even when the entity is present (e.g. evidence
says "Ithaca is a birthplace" but the report claims "hometown is Ithaca", or
the evidence only mentions a medication was prescribed while the report
over-claims "the patient is well").

Ungrounded assertions are fed back into the decision ladder as hard violations,
so a hallucinated / over-claimed draft forces a caveated answer or a re-search.
"""

from __future__ import annotations

import logging

from rag.prompts.generator import PROMPT_JINJA_ENV, gen_json
from rag.prompts.template import load_prompt

_LOG = logging.getLogger(__name__)

GROUNDED_REVIEW = load_prompt("grounded_select")


async def _llm_chat_json(chat_mdl, prompt_text: str):
    try:
        return await gen_json(prompt_text, "Output:\n", chat_mdl)
    except Exception as exc:  # noqa: BLE001
        _LOG.info("[Grounded-draft] gen_json failed: %s", exc)
        return {}


def _render_reports(reports: list[tuple[str, str]]) -> str:
    """Render claim_id → report lines for the reviewer prompt."""
    if not reports:
        return "(no claims)"
    return "\n".join(f"Claim {cid}: {rpt}" for cid, rpt in reports if rpt)


async def llm_grounded_verify(
    tools,
    question: str,
    reports: list[tuple[str, str]],
    evidence_ids=None,
) -> dict:
    """Draft review: is each claim's report grounded in the cited evidence?

    Parameters
    ----------
    tools : RAGTools
        Must expose ``chat_mdl`` and ``kbinfos``.
    reports : list[(claim_id, report)]
        Each claim's draft text (``AgentResult.report``).
    evidence_ids : list[str] | None
        Union of cited evidence chunk IDs (falls back to a bounded prefix).

    Returns
    -------
    dict : ``{claim_id: {"grounded": bool, "ungrounded": [str, ...]}}``
        Empty dict when the LLM review is unavailable (no chat model, no
        evidence, or a failure) — callers treat that as "no new signal".
    """
    if not reports:
        return {}
    chat_mdl = getattr(tools, "chat_mdl", None)
    if chat_mdl is None:
        return {}

    from rag.advanced_rag.harness.orchestrator.sufficiency_llm import (
        _evidence_md,
        _narrow_keywords,
    )

    evidence_md = _evidence_md(tools, evidence_ids, keywords=_narrow_keywords(question))
    if not evidence_md:
        return {}

    prompt_text = PROMPT_JINJA_ENV.from_string(GROUNDED_REVIEW).render(
        question=question,
        reports=_render_reports(reports),
        evidence=evidence_md,
    )
    _LOG.info("[Grounded-draft] reviewing %d claim report(s) (evidence %d chars)", len(reports), len(evidence_md))
    result = await _llm_chat_json(chat_mdl, prompt_text)

    # The LLM may return a malformed payload (array / scalar / null) instead of a
    # dict. Log and fall back to the documented empty result so we never crash
    # with AttributeError on .get("claims").
    if not isinstance(result, dict):
        _LOG.info("[Grounded-draft] unexpected LLM response type=%s (expected dict); treating as no groundedness signal", type(result).__name__)
        return {}

    out: dict = {}
    for item in result.get("claims") or []:
        cid = str(item.get("claim_id") or "")
        if not cid:
            continue
        ung = item.get("ungrounded_assertions") or []
        ungrounded = []
        for u in ung:
            if isinstance(u, dict):
                ungrounded.append(str(u.get("assertion") or u.get("reason") or ""))
            elif u:
                ungrounded.append(str(u))
        out[cid] = {
            "grounded": bool(item.get("grounded")),
            "ungrounded": [a for a in ungrounded if a],
        }
    return out
