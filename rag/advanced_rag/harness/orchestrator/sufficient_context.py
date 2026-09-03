"""Unified Sufficient Context Agent

Sufficient Context Agent performs ONE review pass that simultaneously
examines (1) the retrieved snippets, (2) the intermediate draft (each claim's
report), and (3) what is still missing (missing-pieces analysis). This replaces
the old two-call split — ``llm_sufficiency_boost`` (global verdict, no draft) +
``llm_grounded_verify`` (per-claim draft, no global verdict) — whose trigger
bands were complementary, so the three-part review rarely happened in one shot.

This module calls ONE LLM judge (backing prompt ``sca_select``) that sees the
question, the cited snippets (with ``ID: n`` markers), and each claim's draft,
and returns a unified verdict:

    {
      "is_sufficient": bool,          # global sufficiency (Phase 5 stop signal)
      "confidence": float,
      "contradictions": [...],
      "reasoning": "...",
      "claims": [
        {"claim_id", "grounded", "ungrounded_assertions", "missing_information"}
      ]
    }

It also exposes adapter helpers ``to_boost`` / ``to_grounded`` so the caller can
feed the unified output into the EXISTING decision ladder (``boost``) and replan
(``grounded``) without changing their contracts.
"""

from __future__ import annotations

import json
import logging
import re

from rag.advanced_rag.harness.stats import in_phase
from rag.prompts.generator import PROMPT_JINJA_ENV, gen_json
from rag.prompts.template import load_prompt

_LOG = logging.getLogger(__name__)

SCA_REVIEW = load_prompt("sca_select")

# The SCA now reviews ONLY the claims' reports (per-claim intermediate drafts),
# never the raw retrieved chunks. Rationale (Q2):
#   - Each report is an "evidence-backed finding" produced by query_check from
#     that claim's evidence, so it already distills the answer-bearing facts.
#   - Sending raw chunks is what ballooned the prompt to 20k-100k chars and either
#     degraded the LLM (empty claims) or timed it out; reports alone are tiny.
#   - Groundedness is preserved because the report is generated FROM the cited
#     evidence and query_check enforces grounded facts.
# Total cap for the rendered claims context (all per-claim reports + the overall
# intermediate draft) keeps the SCA prompt well inside the model window.
# Enlarged 9000 -> 48000 so a table-bearing evidence anchor (full table text)
# plus several claim reports fit; matches _MAX_TOOL_RESPONSE_CHARS * 4 used by
# the action-session context budget. Table chunks were structurally invisible
# at 9000 (Q86: rank row at ~62% of a 14.7K-char table never entered the view).
_SCA_CLAIMS_CONTEXT_MAX = 48000
# Max chars of each cited snippet's first line appended as an evidence anchor so
# the SCA can verify a draft against real retrieved text without a token blow-up.
_SCA_EVIDENCE_ANCHOR_CHARS = 300
# Table-structured chunks get their FULL text as the evidence anchor (bounded
# only by _SCA_CLAIMS_CONTEXT_MAX): hint-token windowing is unreliable for
# tables (the draft rarely contains the row's entity names), and truncating from
# the head hides the answer rows that sit mid/late-table (Q86 rank-19 row).
_SCA_EVIDENCE_TABLE_CHARS = None  # None = keep the whole chunk text


def _is_table_text(text: str) -> bool:
    """Corpus-neutral table detector: HTML table markup or >=3 pipe rows."""
    t = str(text or "")
    if "<table" in t.lower() or "<tr" in t.lower():
        return True
    pipe_rows = sum(1 for line in t.splitlines() if line.count("|") >= 2)
    return pipe_rows >= 3


def _clamp(value, lo: float = 0.0, hi: float = 1.0) -> float:
    try:
        return max(lo, min(hi, float(value)))
    except (TypeError, ValueError):
        return 1.0


def _coerce_dict(result) -> dict | None:
    """Coerce a ``gen_json`` response into a dict, tolerating model format drift.

    The reviewer model occasionally replies with a bare array (``[{...}]``) or a
    JSON string instead of a plain object. Previously any non-dict response was
    dropped, so the SCA produced NO signal on those rounds and replan/rewrite
    silently stopped. Recover the first dict when the response is a list of
    objects, and parse a string when it is valid JSON.
    """
    if isinstance(result, dict):
        return result
    if isinstance(result, list):
        for item in result:
            if isinstance(item, dict):
                return item
        return None
    if isinstance(result, str):
        try:
            parsed = json.loads(result)
        except Exception:  # noqa: BLE001
            return None
        return _coerce_dict(parsed)
    return None


def _render_reports(reports: list[tuple[str, str]]) -> str:
    """Render ``claim_id -> draft`` lines for the reviewer prompt."""
    if not reports:
        return "(no claim drafts)"
    return "\n".join(f"Claim {cid}: {rpt}" for cid, rpt in reports if rpt)


def _bounded_excerpt(text: str, hints: str, max_chars: int = 300) -> str:
    """Keep a bounded evidence window around a term from the current draft.

    Table-structured text returns its FULL content (bounded only by the caller's
    overall budget): hint-token windowing fails for tables because the draft
    rarely contains the row's entity names, and head-truncation hides answer
    rows in the mid/late table. Plain text keeps the bounded window.
    """
    text = str(text or "").strip()
    if not text:
        return ""
    if _is_table_text(text):
        return text
    max_chars = max(80, int(max_chars))
    hint_tokens = [t for t in re.findall(r"[A-Za-z0-9_\u4e00-\u9fff]{3,}", str(hints or ""))]
    lower = text.lower()
    start = None
    for token in hint_tokens:
        pos = lower.find(token.lower())
        if pos >= 0:
            start = pos
            break
    if start is None:
        if len(text) <= max_chars:
            return text
        tail = max_chars // 2
        return text[: max_chars - tail] + " … " + text[-tail:]
    half = max_chars // 2
    left = max(0, start - half)
    right = min(len(text), left + max_chars)
    left = max(0, right - max_chars)
    prefix = "…" if left else ""
    suffix = "…" if right < len(text) else ""
    return prefix + text[left:right] + suffix


def _render_claim_context(claims, question: str = "", kbinfos: dict | None = None) -> str:
    """Render per-claim context: each claim's report PLUS a brief evidence anchor.

    Q2 (reports only) + How6 (evidence anchor): the SCA reviews each claim's
    intermediate draft, and ALSO sees a short excerpt of the snippets THAT CLAIM
    cited, so it can verify the draft is grounded in real retrieved text and that
    any arithmetic operand actually appears in a snippet (rather than trusting the
    draft's numbers blindly). We send only the first line of each cited snippet
    (<= _SCA_EVIDENCE_ANCHOR_CHARS) so the prompt stays small (avoids the 20k-100k
    char blow-up that motivated Q2). Chunks whose id is not resolvable are skipped.
    """
    if not claims:
        return "(no claim drafts)"
    all_chunks = (kbinfos or {}).get("chunks") or []
    # ``evidence_ids`` are INDICES into ``kbinfos["chunks"]`` (see
    # decompose._evidence_ids), NOT the chunk's ``chunk_id`` hash. Key the
    # lookup by index so the cited-snippet anchors actually resolve — keying by
    # ``chunk_id`` made every anchor a miss (a hash string is never equal to an
    # index), silently disabling the "SCA reviews the retrieved snippets" guard.
    id2chunk = {str(i): c for i, c in enumerate(all_chunks)}
    blocks: list[str] = []
    used = 0
    for cid, rpt, eids in claims:
        if not rpt:
            continue
        block = f"Claim {cid} (draft):\n{rpt}"
        used += len(block) + 2
        # Evidence anchor: first line of each cited snippet (guards the draft).
        # Table chunks contribute their FULL text (see _bounded_excerpt), so
        # account the actual joined length, not the 300-char estimate.
        if eids:
            anchors: list[str] = []
            for eid in eids:
                ck = id2chunk.get(str(eid)) or id2chunk.get(eid)
                if not ck:
                    continue
                txt = str(ck.get("content_with_weight") or ck.get("content") or ck.get("chunk") or "").strip()
                if not txt:
                    continue
                excerpt = _bounded_excerpt(txt, rpt, max_chars=_SCA_EVIDENCE_ANCHOR_CHARS)
                if excerpt:
                    anchors.append(excerpt)
                if len(anchors) >= 3:
                    break
            if anchors:
                anchor_text = " | ".join(anchors)
                block += "\n  Evidence: " + anchor_text
                used += len(anchor_text) + 4
        blocks.append(block)
        if used >= _SCA_CLAIMS_CONTEXT_MAX:
            break
    return "\n\n".join(blocks) if blocks else "(no claim drafts)"


def _render_overall_draft(claims, question: str = "") -> str:
    """Build the OVERALL intermediate draft ("rough draft") for the SCA.

    SCA reviews a single "rough draft" response for the WHOLE
    question, not just per-claim drafts — so it can judge whether the context lets
    the model answer the original question end-to-end, including cross-claim
    synthesis. We assemble a problem-level draft by concatenating each claim's
    report (which Q1 makes a concrete partial answer). The SCA then reviews this
    overall draft against the question to find cross-claim gaps that per-claim
    review would miss (e.g. two claims each grounded but the question needs them
    COMBINED into one derived answer).
    """
    if not claims:
        return "(no overall draft)"
    parts = [f"[Claim {cid}] {rpt.strip()}" for cid, rpt, _e in claims if rpt and rpt.strip()]
    if not parts:
        return "(no overall draft)"
    draft = "\n".join(parts)
    if len(draft) > _SCA_CLAIMS_CONTEXT_MAX:
        draft = draft[:_SCA_CLAIMS_CONTEXT_MAX]
    return draft


@in_phase("sca")
async def sufficient_context_agent(
    tools,
    question: str,
    claims: list[tuple],
    evidence_ids=None,
) -> dict:
    """Unified SCA review: per-claim drafts + per-claim evidence + missing pieces.

    Parameters
    ----------
    tools : RAGTools
        Must expose ``chat_mdl`` and ``kbinfos``.
    claims : list[(claim_id, draft, evidence_ids)]
        Each claim's intermediate draft plus the evidence IDs THAT CLAIM cited
        (not the global union). Rendering per-claim evidence keeps the prompt
        small (~1-3 chunks per claim) so the LLM does not degrade, while the
        cited snippets still carry the answer-bearing facts.
    evidence_ids : list[str] | None
        Ignored for prompt rendering (kept for signature compatibility); per-claim
        evidence drives the review.

    Returns
    -------
    dict : the unified verdict above, or ``{}`` when unavailable (no chat model,
        no evidence, or a failure) — callers treat that as "no new signal".
    """
    if not claims:
        return {}
    chat_mdl = getattr(tools, "chat_mdl", None)
    if chat_mdl is None:
        return {}

    claims_context = _render_claim_context(claims, question, kbinfos=getattr(tools, "kbinfos", None))
    if not claims_context or claims_context == "(no claim drafts)":
        return {}
    overall_draft = _render_overall_draft(claims, question)

    prompt_text = PROMPT_JINJA_ENV.from_string(SCA_REVIEW).render(
        question=question,
        claims_context=claims_context,
        overall_draft=overall_draft,
    )
    _LOG.info(
        "[SCA] unified review of %d claim draft(s) (reports only, %d chars; overall draft %d chars)",
        len(claims),
        len(claims_context),
        len(overall_draft),
    )
    try:
        result = await gen_json(prompt_text, "Output:\n", chat_mdl)
    except Exception as exc:  # noqa: BLE001
        _LOG.info("[SCA] unified review failed: %s", exc)
        return {}
    result = _coerce_dict(result)
    if not result:
        _LOG.info("[SCA] no usable response (type=%s); treating as no signal", type(result).__name__ if result is not None else "None")
        return {}

    claims_out: dict[str, dict] = {}
    for item in result.get("claims") or []:
        cid = str(item.get("claim_id") or "")
        if not cid:
            continue
        ungrounded = []
        for u in item.get("ungrounded_assertions") or []:
            if isinstance(u, dict):
                ungrounded.append(str(u.get("assertion") or u.get("reason") or ""))
            elif u:
                ungrounded.append(str(u))
        missing_info = []
        for m in item.get("missing_information") or []:
            if isinstance(m, dict):
                what = str(m.get("what") or "").strip()
                hint = str(m.get("search_hint") or "").strip()
                if what or hint:
                    missing_info.append({"what": what, "search_hint": hint})
            elif m:
                missing_info.append({"what": str(m).strip(), "search_hint": ""})
        claims_out[cid] = {
            "grounded": bool(item.get("grounded")),
            "ungrounded": [a for a in ungrounded if a],
            "missing_information": missing_info,
        }

    # Parse the structured sub-query coverage set (Q-CARE): one entry per
    # step-by-step sub-question, each marked satisfied. Unsatisfied entries carry
    # the concrete missing_fact + search_hint — this is the precise "what is
    # missing / where to search next" signal the next round consumes, far more
    # targeted than per-claim free-text missing_information.
    sub_queries: list[dict] = []
    for sq in result.get("sub_queries") or []:
        if not isinstance(sq, dict):
            continue
        sq_text = str(sq.get("sub_query") or "").strip()
        if not sq_text:
            continue
        sq_out: dict = {
            "sub_query": sq_text,
            "satisfied": bool(sq.get("satisfied")),
        }
        if not sq_out["satisfied"]:
            sq_out["missing_fact"] = str(sq.get("missing_fact") or "").strip()
            sq_out["search_hint"] = str(sq.get("search_hint") or "").strip()
        sub_queries.append(sq_out)

    is_sufficient = bool(result.get("is_sufficient"))
    # Failsafe: when the SCA judges the context insufficient but returned an EMPTY
    # claims array (a known degradation on very long prompts), we must still give
    # the orchestrator something to re-search. Otherwise it abandons with
    # "I don't have enough information" despite having retrieved useful snippets.
    # Harvest any TOP-LEVEL missing_information the model may have put outside the
    # claims array, and fall back to the un-verified drafts themselves as gaps.
    if not is_sufficient and not claims_out:
        top_missing: list[dict] = []
        for m in result.get("missing_information") or []:
            if isinstance(m, dict):
                w = str(m.get("what") or "").strip()
                h = str(m.get("search_hint") or "").strip()
                if w or h:
                    top_missing.append({"what": w, "search_hint": h})
            elif m:
                top_missing.append({"what": str(m).strip(), "search_hint": ""})
        if top_missing:
            claims_out["_global"] = {
                "grounded": False,
                "ungrounded": [],
                "missing_information": top_missing,
            }
            _LOG.info("[SCA] insufficient with empty claims; using %d top-level missing piece(s) as the re-search gap.", len(top_missing))
        elif claims:
            # No structured gap at all — derive a coarse one from the claim drafts
            # themselves so the loop still tries a re-search rather than abandoning.
            draft_gaps = [{"what": rpt.strip(), "search_hint": rpt.strip()} for _cid, rpt, _eids in claims if rpt and rpt.strip()]
            if draft_gaps:
                claims_out["_global"] = {
                    "grounded": False,
                    "ungrounded": [],
                    "missing_information": draft_gaps,
                }
                _LOG.info("[SCA] insufficient with no structured gap; deriving %d coarse gap(s) from the drafts.", len(draft_gaps))

    return {
        "is_sufficient": is_sufficient,
        "confidence": _clamp(result.get("confidence")),
        "contradictions": [str(c) for c in (result.get("contradictions") or []) if str(c).strip()],
        "reasoning": str(result.get("reasoning") or "").strip(),
        "sub_queries": sub_queries,
        "claims": claims_out,
    }


def to_boost(sca: dict, verdict, fallback_followups: list | None = None) -> dict:
    """Adapt the unified SCA output into the decision-ladder ``boost`` dict.

    Preserves the existing contract consumed by ``route_sufficiency_verdict``
    (``is_sufficient`` / ``confidence`` / ``missing`` / ``contradictions`` /
    ``feedback`` / ``followups``).
    """
    missing: list[str] = []
    for g in (sca.get("claims") or {}).values():
        for mi in g.get("missing_information") or []:
            w = str(mi.get("what") or "").strip()
            if w and w not in missing:
                missing.append(w)
    contradictions = list(sca.get("contradictions") or [])
    feedback = ""
    if missing:
        feedback = "missing: " + "; ".join(missing[:_FEEDBACK_MAX])
    return {
        "is_sufficient": bool(sca.get("is_sufficient")),
        "confidence": _clamp(sca.get("confidence")),
        "missing": missing,
        "contradictions": contradictions,
        "followups": fallback_followups or [],
        "feedback": feedback,
        # Structured sub-query coverage (Q-CARE): the precise "what is missing /
        # where to search next" signal consumed by _maybe_replan.
        "_sub_queries": list(sca.get("sub_queries") or []),
    }


def to_grounded(sca: dict) -> dict:
    """Adapt the unified SCA output into the ``grounded`` dict consumed by
    ``_maybe_replan`` and the ungrounded-veto path:
    ``{claim_id: {grounded, ungrounded, missing_information}}``.
    """
    return dict(sca.get("claims") or {})


_FEEDBACK_MAX = 4
