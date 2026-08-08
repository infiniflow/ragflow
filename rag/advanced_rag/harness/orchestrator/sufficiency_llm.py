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
import re

from rag.advanced_rag.harness.types import SufficiencyVerdict

_LOG = logging.getLogger(__name__)


# Cap the AutoRater's evidence so a single judge call does not balloon to
# hundreds of KB (6.log showed 271KB from 80 accumulated passages). The judge
# only needs the *cited* evidence plus a bounded prefix — not the whole pool.
_MAX_EVIDENCE_CHUNKS = 24
_MAX_CHUNK_CHARS = 800
_MAX_EVIDENCE_CHARS = 24_000


def _narrow_keywords(question: str) -> list[str]:
    """Language-agnostic keywords of ``question`` for snippet narrowing.

    No language-specific stopword list (RAGFlow supports en/zh/de/fr/es/pt/ja and
    any hard-coded English stopwords would be wrong for the rest). Instead we use
    a length heuristic that generalises across scripts:
      - numeric tokens (years / figures) — universal, high-signal;
      - latin tokens of length >= 4 (keeps content words like "population" /
        "paris" / "statistics" while the short function words "the"/"of"/"was"
        fall below the cut — a length rule, not a stopword list);
      - CJK runs are converted to character bigrams (巴黎 → 巴黎; 人口 → 人口),
        which is tokeniser-free and works for zh/ja without any word segmenter.

    Keeping a few extra tokens is safe: narrowing matches more sentences, so it
    retains more of the chunk and is less likely to drop the answer.
    """
    tokens = re.findall(r"[a-zA-Z0-9]+|[\u4e00-\u9fff]+", (question or "").lower())
    kw: list[str] = []
    for t in tokens:
        if t.isdigit():
            kw.append(t)
        elif re.search(r"[a-zA-Z]", t):
            if len(t) >= 4:
                kw.append(t)
        else:  # CJK run → character bigrams
            if len(t) >= 2:
                kw.extend(t[i : i + 2] for i in range(len(t) - 1))
    return kw


def _clamp(value) -> float:
    """Clamp the AutoRater's confidence field to [0, 1], defaulting to 1.0."""
    try:
        return max(0.0, min(1.0, float(value)))
    except (TypeError, ValueError):
        return 1.0


# A chunk with this many sentences or fewer is never narrowed — there is little
# to save and a real risk of losing the answer sentence.
_NARROW_MIN_SENTENCES = 3
# Number of neighbours kept around each keyword-bearing sentence.
_NARROW_NEIGHBOURS = 1


def _narrow_snippet_safe(content: str, kw_list: list[str]) -> str | None:
    """Self-contained keyword narrowing of ``content`` to a compact snippet.

    This is a purpose-built replacement for ``_narrow_content`` (which returns
    a re-formatted string full of ``...`` and ``*kw*`` markers that, when split
    again downstream, corrupts sentence counts). Here we work directly on the
    original sentences and re-emit plain text, so there is no intermediate
    format to misinterpret.

    The only rule — a deliberately small one, because no rule set can cover
    every phrasing: **narrow only when the keywords actually cover a meaningful
    share of the chunk.** Concretely, we keep the keyword-bearing sentences plus
    ``_NARROW_NEIGHBOURS`` around each, and refuse to narrow (return ``None``,
    caller keeps the whole chunk) when:
      * the chunk is short (<= ``_NARROW_MIN_SENTENCES`` sentences), or
      * no sentence mentions a keyword, or
      * keywords hit too few sentences for the narrowing to be safe
        (< 2 hits, or fewer than half the sentences).

    That last case is the whole point: if the answer sentence is phrased without
    the exact keyword (a bare figure, a synonym, a date), keyword hits will be
    sparse and we keep the whole chunk so the answer is never dropped.

    Returns the narrowed plain text, or ``None`` to keep the chunk whole.
    """
    from rag.advanced_rag.harness.tools.search import _split_sentences

    sents = _split_sentences(content or "")
    if len(sents) <= _NARROW_MIN_SENTENCES:
        return None

    hit_idx = [i for i, s in enumerate(sents) if any(k in s.lower() for k in kw_list)]
    # Refuse to narrow when keywords hit fewer than 2 sentences: with a single hit
    # the narrowed snippet would keep just that sentence (+neighbours) and could
    # drop most of the chunk, including the answer sentence. Two or more hits
    # means the keywords genuinely relate to the passage, so narrowing is safe.
    if len(hit_idx) < 2:
        return None

    keep_idx: set[int] = set()
    for i in hit_idx:
        for j in range(max(0, i - _NARROW_NEIGHBOURS), min(len(sents), i + _NARROW_NEIGHBOURS + 1)):
            keep_idx.add(j)
    return " ".join(sents[i] for i in sorted(keep_idx)).strip()


def _evidence_md(tools, evidence_ids=None, keywords: str | None = None) -> str:
    """Render cited evidence chunks with ``ID: n`` markers for the AutoRater.

    Prefers the chunks referenced by ``evidence_ids`` (the union of all claims'
    evidence); falls back to a bounded prefix of the pool when no ids are
    given. When ``keywords`` is provided each chunk is first narrowed to the
    keyword-bearing sentences via ``_narrow_by_keywords`` (snippet evidence),
    so the AutoRater sees compact relevant snippets instead of full passages.
    Either way the output is capped (see module constants).
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

    # Narrow each kept chunk to its keyword-bearing sentences when keywords are
    # available, preserving the original ``ID: n`` index (the answer cites these).
    # If narrowing empties a chunk it is skipped; if everything narrows away we
    # fall back to the original text so the AutoRater never sees nothing.
    texts: list[tuple[int, str, str]] = []
    # Normalise keywords: accept a comma/space-separated string OR an iterable of
    # words. Narrowing operates on the raw text, independent of chunk structure.
    if isinstance(keywords, str):
        kw_list = [k.strip() for k in re.split(r"[,\s]+", keywords) if k.strip()]
    else:
        kw_list = [str(k) for k in (keywords or []) if str(k).strip()]
    for idx, c in picked:
        raw = c.get("content_with_weight") or c.get("text") or ""
        title = c.get("docnm_kwd", "") or ""
        if kw_list:
            narrowed = _narrow_snippet_safe(raw, kw_list)
            if narrowed:
                texts.append((idx, title, narrowed))
                continue
        texts.append((idx, title, raw))

    blocks: list[str] = []
    used = 0
    for idx, title, text in texts:
        text = text[:_MAX_CHUNK_CHARS]
        if used + len(text) > _MAX_EVIDENCE_CHARS:
            break
        blocks.append(f"ID: {idx} | {title}\n{text}")
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

    evidence_md = _evidence_md(tools, evidence_ids, keywords=_narrow_keywords(question))
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
