"""Sufficiency check — cross-check + fusion score + 5-way verdict."""

import logging

from rag.advanced_rag.harness.types import (
    AgentResult,
    ClaimCrossCheckResult,
    SufficiencyVerdict,
    ExecutionStrategy,
)
from rag.advanced_rag.harness.config import get_mode

_LOG = logging.getLogger(__name__)


# ═══════════════════════════════════════════════════════════════
# Cross-check: code-only
# ═══════════════════════════════════════════════════════════════

import re


def extract_numbers(text: str) -> list[float]:
    """Extract numeric values from text."""
    return [float(m) for m in re.findall(r"\d+\.?\d*", text)]


def _filter_relevant_numbers(numbers: list[float]) -> list[float]:
    """Drop numbers that carry no factual-claim signal.

    - Values in ``[0, 1]`` are overwhelmingly ratios / probabilities /
      confidence scores the agent sprinkled into its prose, not facts to
      verify against the evidence.
    - Drop duplicates: "48 m" appearing three times should be checked once.
    """
    kept: list[float] = []
    for n in numbers:
        if 0.0 < n < 1.0:
            continue
        if n in kept:
            continue
        kept.append(n)
    return kept


# ── Multilingual named-entity extraction ──────────────────────────────
# Cross-check previously only recognized English capitalized sequences, so
# Chinese/Japanese/Korean reports yielded zero entities and could never be
# verified. We now detect the report's language and route:
#   - en/zh/de/fr/es/pt/ja: spaCy NER (models pre-loaded at build time; see
#     pyproject.toml).
#   - Any other language: langdetect returns the code; if no spaCy model is
#     mapped the report still degrades gracefully to a no-op (never crashes).
# Language is detected with ``langdetect`` (Bayesian N-gram profiles, pure
# Python, 55+ languages — a product-grade alternative to hand-rolled
# heuristics), with a small Unicode-range heuristic as offline fallback.
# The spaCy pipeline is lazy-loaded once per process (singleton cache, like
# lightgraph) and degrades gracefully if a model is unavailable, so
# sufficiency never crashes on a missing model.

# RAGFlow language label / ISO code → spaCy model (mirrors lightgraph).
_LANG_TO_SPACY_MODEL = {
    "en": "en_core_web_sm",
    "english": "en_core_web_sm",
    "zh": "zh_core_web_sm",
    "chinese": "zh_core_web_sm",
    "zh-cn": "zh_core_web_sm",
    "de": "de_core_news_sm",
    "german": "de_core_news_sm",
    "fr": "fr_core_news_sm",
    "french": "fr_core_news_sm",
    "es": "es_core_news_sm",
    "spanish": "es_core_news_sm",
    "pt": "pt_core_news_sm",
    "portuguese": "pt_core_news_sm",
    "ja": "ja_core_news_sm",
    "japanese": "ja_core_news_sm",
}

# spaCy NER labels that are not "evidence" (numbers, time, percentages).
_SPACY_SKIP_LABELS = {"ORDINAL", "CARDINAL", "DATE", "TIME", "PERCENT", "MONEY", "QUANTITY"}

_spacy_nlp_cache: dict = {}


def _is_cjk(text: str) -> bool:
    """True if ``text`` contains Chinese/Japanese characters."""
    return any("\u4e00" <= ch <= "\u9fff" for ch in text)


def _detect_language(text: str) -> str:
    """Detect the report's language and return an ISO 639-1 code.

    Primary path uses ``langdetect`` (Bayesian N-gram profile classifier,
    pure Python, ~55 languages). If langdetect is unavailable (or returns
    nothing reliable for a very short / punctuation-only string), we fall
    back to a small Unicode-range heuristic so the pipeline still works
    offline without the extra dependency.
    """
    if not text:
        return "en"
    try:
        from langdetect import detect, DetectorFactory

        # langdetect's global DetectorFactory is randomly seeded by default, so
        # ambiguous/short inputs can flip between runs. Pin the seed once so
        # detection (and therefore the spaCy NER model selected) is deterministic.
        DetectorFactory.seed = 0
        lang = detect(text[:500])  # long reports don't need the full body
        if lang and lang != "unknown":
            return lang
    except Exception as exc:  # langdetect not installed / detection error
        _LOG.info(
            "[multilingual-ner] langdetect unavailable/failed, falling back to Unicode heuristic: %s",
            exc,
        )
    # Offline heuristic fallback.
    if any("\u4e00" <= ch <= "\u9fff" for ch in text):
        # Distinguish zh vs ja by presence of hiragana/katakana.
        if any("\u3040" <= ch <= "\u30ff" for ch in text):
            return "ja"
        return "zh"
    if any("\uac00" <= ch <= "\ud7af" for ch in text):
        return "ko"
    return "en"


def _resolve_spacy_model(language: str) -> str:
    key = (language or "en").strip().lower()
    return _LANG_TO_SPACY_MODEL.get(key, "en_core_web_sm")


def _spacy_ner_entities(text: str, language: str) -> list[str]:
    """Extract named entities via spaCy NER, with per-process model caching.

    Returns [] on any failure (missing model, spacy import error) so the caller
    can fall back to the regex path without crashing.
    """
    model_name = _resolve_spacy_model(language)
    if model_name in _spacy_nlp_cache:
        nlp = _spacy_nlp_cache[model_name]
    else:
        try:
            import spacy

            nlp = spacy.load(model_name)
            _spacy_nlp_cache[model_name] = nlp
        except Exception as exc:  # model not installed / spacy unavailable
            _LOG.info("[multilingual-ner] spaCy model %s unavailable, falling back to regex: %s", model_name, exc)
            return []
    if nlp is None:
        return []
    try:
        doc = nlp(text)
    except Exception as exc:
        _LOG.info("[multilingual-ner] spaCy inference failed for %s: %s", model_name, exc)
        return []
    seen: set[str] = set()
    out: list[str] = []
    for ent in doc.ents:
        label = ent.label_ or ""
        if label in _SPACY_SKIP_LABELS:
            continue
        name = ent.text.strip()
        if not name or name in seen:
            continue
        seen.add(name)
        out.append(name)
    return out


def extract_named_entities(text: str) -> list[str]:
    """Extract named entities via spaCy NER for any configured language.

    All languages (en/zh/de/fr/es/pt/ja) go through the spaCy NER pipeline —
    no hand-rolled regexes or stopword lists. spaCy only returns recognized
    named entities (PER/LOC/ORG/NORP/MISC/...), so connective tissue never
    appears. Number/time/percentage labels are filtered (they are verified
    separately via ``extract_numbers``). Returns [] if spaCy is unavailable.
    """
    if not text:
        return []
    lang = _detect_language(text)
    return _spacy_ner_entities(text, lang)


def cross_check_claim(agent_result: AgentResult, all_chunks: dict) -> ClaimCrossCheckResult:
    """Code-level cross-check: number matching + entity presence."""
    report = agent_result.report
    claimed = agent_result.is_verified
    _LOG.info(
        "[Cross-check] claim=%s entering — self_verified=%s, report_len=%d, evidence_ids=%s",
        agent_result.claim_id,
        claimed,
        len(report or ""),
        agent_result.evidence_ids,
    )

    if not claimed:
        _LOG.info("[Cross-check] claim=%s → FAILED (agent self-reported as unverified, score=0.0)", agent_result.claim_id)
        return ClaimCrossCheckResult(
            claim_id=agent_result.claim_id,
            cross_check_passed=False,
            cross_check_score=0.0,
            mismatches=["agent self-reported as unverified"],
        )

    raw_numbers = extract_numbers(report)
    numbers = _filter_relevant_numbers(raw_numbers)
    entities = extract_named_entities(report)
    _LOG.info(
        "[Cross-check] claim=%s extracted %d raw number(s) → %d relevant (noise filtered/deduped): %s, %d entity(ies)=%s from report",
        agent_result.claim_id,
        len(raw_numbers),
        len(numbers),
        numbers[:8],
        len(entities),
        entities[:8],
    )

    # Existence check across the *union* of evidence chunks, not per-chunk.
    # Verifying "does this fact appear anywhere in the cited evidence" is the
    # right semantic — a number/entity supported by one chunk is verified.
    # The old per-chunk loop demanded a fact appear in EVERY evidence chunk,
    # so a fact confirmed in chunk A was recorded as a mismatch in chunks B..N
    # and the score was diluted to near-zero even when the answer was correct
    # (see benchmark/3.log: c4's 48/157/27/89 all matched chunk 0 yet scored
    # 0.286; c5's 21/27/48/89 matched chunk 3 yet scored 0.168).
    chunk_texts: list[str] = []
    missing_ids: list[str] = []
    for eid in agent_result.evidence_ids or []:
        chunk = all_chunks.get(eid)
        if not chunk:
            missing_ids.append(str(eid))
            continue
        chunk_texts.append((chunk.get("content_with_weight") or chunk.get("text") or "").lower())
    if missing_ids:
        _LOG.info("[Cross-check] claim=%s %d evidence_id(s) MISSING from pool (index drift?): %s", agent_result.claim_id, len(missing_ids), missing_ids[:5])

    _LOG.info(
        "[Cross-check] claim=%s %d evidence chunk(s) gathered (%d expected); verifying %d fact(s) against their union",
        agent_result.claim_id,
        len(chunk_texts),
        len(agent_result.evidence_ids or []),
        len(numbers) + len(entities),
    )

    def _anywhere(needle: str) -> bool:
        return any(re.search(needle, t) for t in chunk_texts)

    matches: list[str] = []
    mismatches: list[str] = []
    for num in numbers:
        # Numbers are extracted as floats ("1976" -> 1976.0) while chunk text
        # spells them "1976" — match both raw and integral forms. Bounded so a
        # number does not match inside a longer digit run (1976 vs 19760).
        forms = {str(num), str(int(num))} if float(num).is_integer() else {str(num)}
        found = any(_anywhere(rf"(?<![\w]){re.escape(f)}(?![\w])") for f in forms)
        if found:
            matches.append(f"number {num} found in evidence")
        else:
            mismatches.append(f"number {num} not found in any evidence chunk")

    for ent in entities:
        if _is_cjk(ent):
            # CJK entities have no word boundaries (every Han char is \w), and
            # they are commonly followed by function words ("的/是"). Use a
            # substring match on the lowercased text instead of a \b-like regex,
            # which would fail on "清真寺的..." because "的" is \w.
            found = any(ent.lower() in t for t in chunk_texts)
        else:
            # Bounded word/phrase match: Ann must not match Annual.
            found = _anywhere(rf"(?<![\w]){re.escape(ent.lower())}(?![\w])")
        if found:
            matches.append(f"entity '{ent}' found in evidence")
        else:
            mismatches.append(f"entity '{ent}' not found in any evidence chunk")

    total = len(matches) + len(mismatches)
    if total == 0:
        # No evidence was actually examined — fail rather than pass neutrally:
        # a claim with zero evidence IDs cannot be cross-checked at all.
        if not agent_result.evidence_ids:
            _LOG.info("[Cross-check] claim=%s → FAILED (no evidence ids, score=0.0)", agent_result.claim_id)
            return ClaimCrossCheckResult(
                claim_id=agent_result.claim_id,
                cross_check_passed=False,
                cross_check_score=0.0,
                mismatches=["no evidence"],
            )
        # Evidence IDs exist but nothing extractable to verify against (e.g.
        # Chinese reports yield no capitalized entities and no digits). We
        # cannot confirm OR falsify — score it neutral (0.5) and do NOT mark it
        # passed. The old "pass neutrally with score=1.0" treated unverifiable
        # claims as fully verified, which let any entity-free, digit-free report
        # sail through as SUFFICIENT.
        _LOG.info(
            "[Cross-check] claim=%s → NEUTRAL (evidence ids exist but nothing extractable to verify, score=0.5, not passed)",
            agent_result.claim_id,
        )
        return ClaimCrossCheckResult(
            claim_id=agent_result.claim_id,
            cross_check_passed=False,
            cross_check_score=0.5,
            mismatches=["nothing extractable to cross-check"],
        )
    cross_score = len(matches) / total
    # Pass when at least half the checked facts (numbers + entities) are
    # confirmed in the evidence. The old ``mismatch < match*0.5`` required a
    # 2/3 match rate and treated a single spurious number as fatal, which
    # systematically failed otherwise-correct claims (see benchmark/2.log).
    cross_passed = cross_score >= 0.5
    _LOG.info(
        "[Cross-check] claim=%s → %s (%d/%d matched, score=%.3f, pass>=0.50). matches=%s mismatches=%s",
        agent_result.claim_id,
        "PASSED" if cross_passed else "FAILED",
        len(matches),
        total,
        cross_score,
        matches[:5],
        mismatches[:5],
    )

    return ClaimCrossCheckResult(
        claim_id=agent_result.claim_id,
        cross_check_passed=cross_passed,
        cross_check_score=cross_score,
        evidence_matches=matches,
        mismatches=mismatches,
    )


# ═══════════════════════════════════════════════════════════════
# Fusion score
# ═══════════════════════════════════════════════════════════════


def compute_fusion_score(
    agent_results: list[AgentResult],
    cross_check_results: list[ClaimCrossCheckResult],
    mode: ExecutionStrategy,
) -> SufficiencyVerdict:
    """Dual-signal fusion: agent confidence + cross-check pass rate."""
    # Signal A: agent self-assessed confidence (continuous, per design doc).
    # Only self-verified claims count toward "agent is confident" — an
    # unverified claim's confidence is not trustworthy. This replaces the old
    # boolean pass-rate (verified_count / n) which inflated the score to 1.0
    # whenever the agent merely said "verified" (benchmark/2.log showed
    # confidence 0.5-0.7 being reported as agent_score=1.0).
    verified = [r for r in agent_results if r.is_verified]
    verified_count = len(verified)
    agent_score = sum(r.confidence for r in verified) / verified_count if verified_count else 0.0
    _LOG.info(
        "[Sufficiency] Signal A (self): %d/%d claims self-verified, mean confidence → agent_score=%.3f (raw confidence values=%s)",
        verified_count,
        len(agent_results),
        agent_score,
        [round(r.confidence, 3) for r in agent_results],
    )

    # Signal B: cross-check score (continuous match rate), per design doc.
    # Uses each claim's actual cross_check_score rather than a boolean
    # pass/fail count, so partial-but-real evidence (e.g. 0.6) contributes
    # proportionally instead of being zeroed.
    #
    # Unrelated-claim pollution (see 6.log/7.log): the planner occasionally
    # invents a claim with no bearing on the question (e.g. "Suharto was born
    # in Kemusuk" while the question asks about a mosque's heights). The agent
    # finds no evidence for it (cross_check_score≈0), fails the cross-check,
    # AND self-reports it as unverified. Such a claim drags Signal B down and
    # pushes an otherwise sufficient fusion into the critical band that
    # needlessly triggers the LLM fallback. We exclude "unanswerable + agent
    # self-unverified" claims from the mean — they neither help nor should
    # punish the verdict. We key on the agent's is_verified flag (not the
    # confidence threshold) because a claim the agent itself calls unverified
    # is the strongest unrelated/ungrounded signal (7.log's c2 reported
    # confidence 0.35 but self-flagged unverified, which a confidence<0.2
    # threshold would have missed).
    cross_results = list(cross_check_results)
    noise_threshold = 0.2
    agent_verified = {r.claim_id: r.is_verified for r in agent_results}
    noise_ids = [r.claim_id for r in cross_results if r.cross_check_score < noise_threshold and not r.cross_check_passed and not agent_verified.get(r.claim_id, False)]
    kept = [r for r in cross_results if r.claim_id not in noise_ids]
    if noise_ids and kept:
        _LOG.info(
            "[Sufficiency] Excluding %d unrelated/unverifiable claim(s) from Signal B: %s (cross<%.2f AND agent self-unverified)",
            len(noise_ids),
            noise_ids,
            noise_threshold,
        )
        cross_results = kept

    cross_score = sum(r.cross_check_score for r in cross_results) / len(cross_results) if cross_results else 0.0
    _LOG.info(
        "[Sufficiency] Signal B (cross): mean cross_check_score → cross_score=%.3f (per-claim cross scores=%s, %d passed)",
        cross_score,
        [round(r.cross_check_score, 3) for r in cross_results],
        sum(1 for r in cross_results if r.cross_check_passed),
    )

    # Fusion of Signal A and Signal B — configurable per mode instead of the
    # old hard-coded max/avg/min (which are arbitrary and ill-calibrated):
    #   - geometric (default): weighted geometric mean; any signal near 0 vetoes
    #     the answer, preventing "confident but unsupported" hallucinations
    #     (A=1.0/B=0.0 previously fused to 0.5-1.0 under max/avg).
    #   - agreement: trust both signals only when they agree; conservative when
    #     they diverge (agent confident but evidence weak).
    #   - weighted: convex combination minus a disagreement penalty.
    strategy = getattr(mode, "fusion_strategy", "geometric") or "geometric"
    w_b = getattr(mode, "fusion_w_b", 0.6) or 0.6  # cross-check (B) reliability weight
    if strategy == "agreement":
        d = abs(agent_score - cross_score)
        if d < 0.2:
            fusion_score = (agent_score + cross_score) / 2
        else:
            fusion_score = min(agent_score, cross_score) * 0.7 + (agent_score + cross_score) / 2 * 0.3
    elif strategy == "weighted":
        base = (1 - w_b) * agent_score + w_b * cross_score
        fusion_score = max(0.0, min(1.0, base - 0.3 * abs(agent_score - cross_score)))
    else:  # geometric (default)
        if agent_score <= 0.05 or cross_score <= 0.05:
            fusion_score = 0.0
        else:
            fusion_score = (agent_score ** (1 - w_b)) * (cross_score**w_b)
    _LOG.info(
        "[Sufficiency] Fusion strategy=%s (w_b=%.2f) → fusion_score=%.3f (A=%.3f, B=%.3f)",
        strategy,
        w_b,
        fusion_score,
        agent_score,
        cross_score,
    )

    # Conflict detection — based on the kept (non-noisy) claims so an
    # unrelated claim's mismatches don't manufacture a conflict.
    has_conflicts = any(len(r.mismatches) > 0 for r in cross_results)
    _LOG.info("[Sufficiency] Conflict detection: has_conflicts=%s", has_conflicts)

    # 5-way verdict
    if has_conflicts and fusion_score < mode.partial_threshold:
        status = "CONFLICTING"
    elif fusion_score >= mode.sufficiency_threshold:
        status = "SUFFICIENT"
    elif fusion_score >= mode.partial_threshold:
        status = "USEFUL_BUT_INCOMPLETE"
    elif not any(r.cross_check_passed for r in cross_results):
        status = "UNANSWERABLE"
    else:
        status = "INSUFFICIENT"
    _LOG.info(
        "[Sufficiency] Thresholds: sufficient>=%.2f partial>=%.2f → verdict=%s",
        mode.sufficiency_threshold,
        mode.partial_threshold,
        status,
    )

    missing = [r.claim_id for r in cross_results if not r.cross_check_passed]
    # Excluded unrelated claims are surfaced (not silently dropped) so the
    # caller knows the planner invented unanswerable claims.
    missing += noise_ids

    return SufficiencyVerdict(
        status=status,
        score=fusion_score,
        agent_score=agent_score,
        cross_score=cross_score,
        claim_assessments=[{"claim_id": r.claim_id, "is_verified": r.cross_check_passed, "score": r.cross_check_score, "mismatches": r.mismatches} for r in cross_results],
        has_conflicts=has_conflicts,
        missing_claims=missing,
        feedback=_build_feedback(missing, cross_results),
        overall_reason=_format_reason(status, fusion_score, missing),
    )


# ═══════════════════════════════════════════════════════════════
# Helpers
# ═══════════════════════════════════════════════════════════════


def _build_feedback(missing: list[str], results: list[ClaimCrossCheckResult]) -> str:
    if not missing:
        return "all claims verified"
    hints = []
    for r in results:
        if not r.cross_check_passed:
            hints.append(f"claim {r.claim_id}: {len(r.mismatches)} mismatch(es)")
    return "missing: " + "; ".join(hints)


def _format_reason(status: str, score: float, missing: list[str]) -> str:
    return f"{status} score={score:.2f} missing={missing}"


def route_sufficiency_verdict(verdict: SufficiencyVerdict, mode_label: str, cycle: int, max_cycles: int) -> tuple:
    """Return (action, should_continue)."""
    mode = get_mode(mode_label)
    _LOG.info(
        "[Sufficiency routing] verdict=%s score=%.3f, cycle=%d/%d, mode=%s (replan_allowed=%s selective_gen=%s fallback_llm=%s)",
        verdict.status,
        verdict.score,
        cycle,
        max_cycles,
        mode_label,
        mode.allows_replan,
        mode.requires_selective_gen,
        mode.fallback_to_direct_llm,
    )

    if verdict.status == "SUFFICIENT":
        _LOG.info("[Sufficiency routing] → ANSWER (enough evidence)")
        return ("ANSWER", False)

    if verdict.status == "USEFUL_BUT_INCOMPLETE":
        if mode.requires_selective_gen:
            _LOG.info("[Sufficiency routing] → ANSWER_PARTIAL (partial evidence, selective gen enabled)")
            return ("ANSWER_PARTIAL", False)
        _LOG.info("[Sufficiency routing] → CONTINUE (partial evidence, no selective gen)")
        return ("CONTINUE", False)

    if verdict.status == "INSUFFICIENT":
        # ``cycle`` is 0-based, so the last cycle is ``max_cycles - 1`` — the
        # old ``max_cycles * 0.8`` threshold was never reached for the 3/3/4
        # cycle budgets, making this branch dead code.
        if cycle >= max_cycles - 1:
            _LOG.info("[Sufficiency routing] → ANSWER_PARTIAL (last cycle reached, insufficient)")
            return ("ANSWER_PARTIAL", False)
        _LOG.info("[Sufficiency routing] → CONTINUE (insufficient, cycles remain)")
        return ("CONTINUE", True)

    if verdict.status == "CONFLICTING":
        if mode.allows_replan and cycle < max_cycles * 0.5:
            _LOG.info("[Sufficiency routing] → REPLAN (conflicting evidence)")
            return ("REPLAN", True)
        _LOG.info("[Sufficiency routing] → ANSWER_PARTIAL (conflicting, no replan available)")
        return ("ANSWER_PARTIAL", False)

    if verdict.status == "UNANSWERABLE":
        if mode.fallback_to_direct_llm:
            _LOG.info("[Sufficiency routing] → FALLBACK_LLM (unanswerable)")
            return ("FALLBACK_LLM", False)
        _LOG.info("[Sufficiency routing] → ABSTAIN (unanswerable)")
        return ("ABSTAIN", False)

    _LOG.info("[Sufficiency routing] → CONTINUE (default)")
    return ("CONTINUE", True)
