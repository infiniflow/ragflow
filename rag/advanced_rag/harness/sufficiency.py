"""Sufficiency check — cross-check + fusion score + 5-way verdict."""

import logging

from rag.advanced_rag.harness.types import (
    AgentResult,
    ClaimCrossCheckResult,
    SufficiencyVerdict,
    ExecutionStrategy,
)
from rag.advanced_rag.harness.config import get_mode
from rag.advanced_rag.harness.sufficiency_ladder import (
    ANSWER_WITH_CAVEAT,
    RECONCILE,
    UNANSWERABLE,
)

_LOG = logging.getLogger(__name__)

# Experimental switch: when True, the lexical NER grounded-fact check
# (``_grounded_hit`` below) runs in ``cross_check_claim``; when False it is
# disabled and groundedness is delegated entirely to the LLM draft review
# (``llm_grounded_verify`` in orchestrator/grounded_llm.py). Set to False to
# trial "LLM draft instead of NER".
_ENABLE_NER_GROUNDED = False


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


def _detect_numeric_conflict(disclosed: list[str]) -> list[str]:
    """Detect close-but-different figures among the agent's disclosed numbers.

    Extracts the leading number from each disclosure entry (e.g. "2,161,000 from
    Wikipedia ..." → 2161000) and flags pairs that are numerically *close but not
    equal* — the signature of a multi-source口径 conflict (same quantity, different
    figure), which Q754 hit (population 2,161,000 vs 2,145,906 → 228 vs 227).
    Numbers that are far apart are likely different quantities and not flagged.
    """
    import re as _re

    figures: list[tuple[float, str]] = []
    for entry in disclosed:
        m = _re.search(r"([\d][\d,]*(?:\.\d+)?)", entry)
        if not m:
            continue
        try:
            fig = float(m.group(1).replace(",", ""))
        except ValueError:
            continue
        figures.append((fig, entry[:80]))

    conflicts: list[str] = []
    for i in range(len(figures)):
        for j in range(i + 1, len(figures)):
            a, b = figures[i][0], figures[j][0]
            if a <= 0 or b <= 0:
                continue
            ratio = max(a, b) / min(a, b)
            # Close (within 30%) but not equal → same quantity, conflicting value.
            if 1 < ratio <= 1.3:
                conflicts.append(f"{figures[i][1]} vs {figures[j][1]}")
    return conflicts


# Attribute/relationship descriptors: content words in a grounded fact that are
# neither named entities nor numbers, e.g. "hometown", "captain", "born", "age".
# Their presence in the evidence is what makes a grounded assertion authoritative
# (the fact's *relationship*, not just its entities, must be supported).
_PREDICATE_STOP = {
    "the",
    "a",
    "an",
    "of",
    "in",
    "on",
    "at",
    "to",
    "for",
    "and",
    "or",
    "is",
    "was",
    "are",
    "were",
    "by",
    "with",
    "from",
    "his",
    "her",
    "their",
    "its",
    "that",
    "this",
    "he",
    "she",
    "they",
    "it",
    "also",
    "as",
    "when",
    "who",
    "what",
    "which",
    "there",
    "have",
    "has",
    "had",
    "be",
    "been",
    "being",
}


def _predicate_terms(fact: str, excluded: list[str]) -> list[str]:
    """Return the predicate/attribute descriptors of ``fact`` (lowercased, ≤4 tokens).

    These are the content words that carry the relationship being asserted,
    excluding proper entities (in ``excluded``) and common stop words. For
    "hometown is Ithaca" → ["hometown"]; for "Dustin Brown captain of Los Angeles
    Kings" → ["captain"]. Used to verify the fact's relationship is evidenced.
    """
    excluded_lower = {e.lower() for e in excluded}
    tokens = re.findall(r"[A-Za-z][A-Za-z'-]{1,19}", fact.lower())
    out: list[str] = []
    for tok in tokens:
        if tok in excluded_lower or tok in _PREDICATE_STOP or tok in out:
            continue
        out.append(tok)
    return out[:4]


def _entity_present(ent: str, chunk_texts: list[str]) -> bool:
    """Whether ``ent`` (lowercased) appears in any of the evidence chunk texts.

    CJK entities have no word boundaries (every Han char is ``\\w``) and are
    commonly followed by function words ("的/是"), so they use a substring
    match. Non-CJK entities use a bounded word/phrase match (Ann must not match
    Annual). Mirrors the matching inside ``cross_check_claim``.
    """
    if _is_cjk(ent):
        return any(ent.lower() in t for t in chunk_texts)
    return any(re.search(rf"(?<![\w]){re.escape(ent.lower())}(?![\w])", t) for t in chunk_texts)


def required_entity_gaps(
    question: str,
    claims: list,
    all_chunks: dict,
) -> dict[str, list[str]]:
    """Find per-claim "required entities" missing from the evidence.

    Aligns with the Sufficient Context paper (arXiv 2411.06037): sufficiency is
    anchored on *what the question needs*, not on what the agent *claims* in its
    report. Extracting entities from the report is vulnerable to the agent
    back-filling facts from prior knowledge (check1.log Q2: the agent injected
    Tyson Fury's "age 35" from memory while the corpus had no Fury data, so the
    report-based cross-check passed on Mike Tyson's numbers that padded the
    score). Instead we extract entities from the *claim description + question*
    (what the answer actually requires) and check each against the evidence.

    Returns {claim_id: [missing entities]}. A claim whose required entity is
    absent from every cited/evidence chunk is flagged — under AND semantics one
    missing required entity means that part of the question is unsupported.
    """
    # Union of all evidence chunk texts (independent of any claim's citation).
    chunk_texts: list[str] = []
    for chunk in (all_chunks or {}).values():
        text = chunk.get("content_with_weight") or chunk.get("text") or ""
        if text:
            chunk_texts.append(text.lower())

    # Pre-extract question entities once (shared across claims) + seed per-claim.
    q_entities = extract_named_entities(question or "")
    gaps: dict[str, list[str]] = {}
    for claim in claims or []:
        cid = getattr(claim, "claim_id", None)
        desc = getattr(claim, "description", None) or ""
        if not cid or not desc:
            continue
        # Required entities = those in the claim description, plus any
        # question-level entities also mentioned by the claim (so a composite
        # question like "Mike Tyson AND Tyson Fury" keeps each named entity
        # individually accountable).
        desc_entities = extract_named_entities(desc)
        desc_lower = {e.lower() for e in desc_entities}
        required = [e for e in desc_entities] + [e for e in q_entities if e.lower() in desc_lower]
        missing = [e for e in required if not _entity_present(e, chunk_texts)]
        if missing:
            gaps[cid] = missing
    return gaps


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

    # ── Grounded-fact verification (answer↔evidence consistency) ──
    # The agent explicitly lists the KEY assertions it claims are evidence-
    # backed (schema field `grounded`). This is the ground truth to verify:
    # the ratio-based number/entity check below can be gamed by padding facts
    # (check1.log Q203: "hometown=Ithaca" scored 9/12=0.75 because Dustin Brown
    # / Kings / Stanley Cup matched, while the answer-critical hometown fact
    # was prior-injected and absent from evidence; Q665 likewise: "first solo
    # album=1970" passed on Beatles padding). If any *grounded* fact is absent
    # from the evidence, the claim is a prior-knowledge injection → hard-fail.
    # A grounded fact is a natural-language assertion (e.g. "Dustin Brown
    # captain of Los Angeles Kings"), which will never match evidence verbatim.
    # Match on its *key content tokens* (entities + numbers) instead: the fact
    # is considered grounded when at least half its key tokens appear in the
    # evidence. "hometown is Ithaca" → key token [Ithaca] → absent → violation.
    # "Dustin Brown captain of Los Angeles Kings" → [Dustin Brown, Los Angeles
    # Kings] → both present → grounded. This is lenient against wording noise
    # yet strict on the answer-critical named facts that prior-injection adds.
    def _grounded_hit(fact: str) -> bool:
        f = fact.lower()
        if _is_cjk(f):
            return any(f in t for t in chunk_texts)
        # Key tokens = named entities + numbers mentioned in the fact.
        key_tokens = [tok for tok in extract_named_entities(fact)]
        key_tokens += [f"{int(n)}" for n in _filter_relevant_numbers(extract_numbers(fact))]
        if not key_tokens:
            # No extractable key content — fall back to whole-phrase match.
            return any(re.search(rf"(?<![\w]){re.escape(f)}(?![\w])", t) for t in chunk_texts)
        # Predicate/attribute co-occurrence: partial token overlap alone is not
        # authoritative — the fact's *relationship* must also be evidenced. For
        # "hometown is Ithaca", matching only "Ithaca" (present as a BIRTHPLACE)
        # must NOT ground the claim. Extract the attribute descriptors (non-entity
        # content words that carry the relation: "hometown", "captain of", "born")
        # and require one of them to appear in the evidence. This downgrades a
        # pure entity-overlap hit to non-authoritative when the predicate is absent.
        predicate_tokens = _predicate_terms(fact, key_tokens)
        entity_hits = sum(1 for tok in key_tokens if _entity_present(tok, chunk_texts))
        if predicate_tokens:
            pred_ok = any(re.search(rf"(?<![\w]){re.escape(pt)}(?![\w])", t) for pt in predicate_tokens for t in chunk_texts)
            if not pred_ok:
                return False  # relationship not evidenced → prior-injection risk
        return entity_hits / len(key_tokens) >= 0.5

    grounded_facts = [str(g) for g in (agent_result.grounded or []) if str(g).strip()]
    if _ENABLE_NER_GROUNDED and grounded_facts:
        ungounded = [g for g in grounded_facts if not _grounded_hit(g)]
        if ungounded:
            _LOG.warning(
                "[Cross-check] claim=%s → GROUNDED-FACT VIOLATION: %d key fact(s) agent marked as evidence-backed are ABSENT from evidence: %s — hard-failing claim",
                agent_result.claim_id,
                len(ungounded),
                ungounded[:6],
            )
            return ClaimCrossCheckResult(
                claim_id=agent_result.claim_id,
                cross_check_passed=False,
                cross_check_score=0.0,
                mismatches=[f"grounded fact not in evidence: {g}" for g in ungounded],
            )

    # ── Numeric multi-source conflict detection (Q754: 225 vs 228口径) ──
    # The agent may have used one figure while the evidence holds another value
    # for the same quantity (e.g. Paris population from INSEE vs Wikipedia vs a
    # news estimate). A ratio/mean cross-check passes as long as the used number
    # matches somewhere, hiding the fact that a DIFFERENT authoritative figure
    # exists. When the report discloses several distinct figures that are close
    # but not equal (the classic "multiple sources, pick one" trap), surface them
    # as conflicts and cap the claim below the pass floor so the caller does not
    # blindly accept one口径.
    disclosed = [str(n) for n in (agent_result.numbers or []) if str(n).strip()]
    if disclosed:
        conflict = _detect_numeric_conflict(disclosed)
        if conflict:
            _LOG.warning(
                "[Cross-check] claim=%s → NUMERIC CONFLICT: multiple close-but-different figures for the same quantity: %s — capping below pass",
                agent_result.claim_id,
                conflict,
            )
            return ClaimCrossCheckResult(
                claim_id=agent_result.claim_id,
                cross_check_passed=False,
                cross_check_score=0.0,
                mismatches=[f"numeric source conflict: {c}" for c in conflict],
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
    question: str = "",
    claims: list | None = None,
    all_chunks: dict | None = None,
) -> SufficiencyVerdict:
    """Extract sufficiency *signals* — hard vetoes, agent confidence, conflicts.

    This is no longer a weighted fusion. The LLM AutoRater (invoked by the
    orchestrator via ``llm_sufficiency_boost``) is the primary sufficiency
    judge; this function only produces the *code-level* inputs the decision
    ladder consumes:
      - hard_violations: claims with a proven evidence gap (required entity
        missing / grounded absent / numeric conflict) that must veto "good
        enough" even if the AutoRater says sufficient;
      - agent_confidence: mean self-confidence over the trusted subset;
      - has_conflicts / missing_claims: surfaced for the ladder / caveat.

    ``question`` / ``claims`` / ``all_chunks`` are optional; when provided they
    drive the *required-entity* AND-semantics veto from the Sufficient Context
    paper (anchored on what the question needs, not what the agent claims), plus
    suppression of self-confidence for claims whose required entities are
    missing from the evidence.
    """
    # ── Required-entity gaps (Sufficient Context paper, AND semantics) ──
    # Anchored on *what the question needs* rather than what the agent claims
    # (see required_entity_gaps). Any claim whose required entity is absent from
    # the evidence flags a localized gap: under AND semantics the whole verdict
    # must not be SUFFICIENT, and the agent's self-confidence for that claim is
    # suppressed (selective generation: confidence must not override missing
    # evidence). Computed once here so it drives both Signal A and the veto.
    required_gaps: dict[str, list[str]] = {}
    if question or claims:
        required_gaps = required_entity_gaps(question, claims, all_chunks)
    if required_gaps:
        _LOG.info(
            "[Sufficiency] Required-entity gaps (AND semantics): %s",
            {k: v for k, v in required_gaps.items()},
        )
    gapped_ids = set(required_gaps.keys())

    # Signal A: agent self-assessed confidence (continuous, per design doc).
    # Only self-verified claims count toward "agent is confident" — an
    # unverified claim's confidence is not trustworthy. This replaces the old
    # boolean pass-rate (verified_count / n) which inflated the score to 1.0
    # whenever the agent merely said "verified" (benchmark/2.log showed
    # confidence 0.5-0.7 being reported as agent_score=1.0).
    #
    # A claim whose REQUIRED entity is missing from the evidence must not lend
    # its self-confidence to Signal A — the agent is confident about a fact the
    # corpus cannot support (prior-knowledge back-fill, see check1.log Q2). Its
    # confidence is zeroed for the mean, so self-assessed confidence can never
    # mask an evidence gap.
    verified = [r for r in agent_results if r.is_verified and r.claim_id not in gapped_ids]
    suppressed = [r.claim_id for r in agent_results if r.is_verified and r.claim_id in gapped_ids]
    verified_count = len(verified)
    agent_score = sum(r.confidence for r in verified) / verified_count if verified_count else 0.0
    if suppressed:
        _LOG.info(
            "[Sufficiency] Signal A: suppressed %d self-verified claim(s) with missing required entities (confidence zeroed): %s",
            len(suppressed),
            suppressed,
        )
    _LOG.info(
        "[Sufficiency] Signal A (self): %d/%d claims self-verified (and evidence-backed), mean confidence → agent_score=%.3f (raw confidence values=%s)",
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

    # ── Hard-veto floor: a *localized* evidence gap must veto "good enough" ──
    # Multi-claim questions (e.g. "Mike Tyson AND Tyson Fury") can average a
    # genuinely weak claim up to the sufficient band. Q2 (check.log): c4/c5's
    # "Tyson Fury" / "Usyk" entities matched 0 chunks, yet the 5-claim mean
    # cross_check_score=0.822 crossed the threshold and produced an answer that
    # could not back the Fury half. These claims become ``hard_violations`` that
    # force the decision ladder to a caveated answer even if the LLM AutoRater
    # says sufficient (code-proven evidence gap beats "roughly good enough").
    min_cross_floor = getattr(mode, "fusion_min_cross", 0.5) or 0.5
    self_verified_ids = {r.claim_id for r in agent_results if r.is_verified}
    weak = [r.claim_id for r in cross_results if r.claim_id in self_verified_ids and r.cross_check_score < min_cross_floor]
    # AND-semantics required-entity veto: a claim missing a required entity
    # (even if its report-based cross-check passed — the numbers matched on
    # unrelated padding) must veto. This catches the Q2 case that min-cross
    # alone missed: c2/c3 self-verified with score 0.83/0.80 (padded by Mike
    # Tyson's digits) yet Tyson Fury's entities were absent from every chunk.
    weak += [cid for cid in gapped_ids if cid not in weak]
    if weak:
        _LOG.info(
            "[Sufficiency] Hard-veto: %d self-verified claim(s) below floor %.2f OR missing a required entity: %s",
            len(weak),
            min_cross_floor,
            weak,
        )

    # Conflict detection — based on the kept (non-noisy) claims so an
    # unrelated claim's mismatches don't manufacture a conflict.
    has_conflicts = any(len(r.mismatches) > 0 for r in cross_results)
    _LOG.info("[Sufficiency] Conflict detection: has_conflicts=%s", has_conflicts)

    # Cross-check status (code-only view, no AutoRater). This is a *preliminary*
    # label used by the orchestrator to decide whether to call the LLM AutoRater
    # (medium triggers it only in the borderline band); the final decision comes
    # from the decision ladder with the AutoRater's verdict.
    if not any(r.cross_check_passed for r in cross_results):
        status = "UNANSWERABLE"
    elif has_conflicts:
        status = "CONFLICTING"
    elif weak:
        status = "INSUFFICIENT"
    elif agent_score >= mode.sufficiency_threshold:
        status = "SUFFICIENT"
    else:
        status = "USEFUL_BUT_INCOMPLETE"
    _LOG.info(
        "[Sufficiency] Code-level status=%s (agent_conf=%.3f, conflicts=%s, hard_veto=%s)",
        status,
        agent_score,
        has_conflicts,
        bool(weak),
    )

    missing = [r.claim_id for r in cross_results if not r.cross_check_passed]
    # Excluded unrelated claims are surfaced (not silently dropped) so the
    # caller knows the planner invented unanswerable claims.
    missing += noise_ids
    # Thin-evidence claims that vetoed are surfaced too, so the caller sees
    # exactly which part of the question still lacks support.
    missing += [c for c in weak if c not in missing]

    return SufficiencyVerdict(
        status=status,
        # Reference score for logging/monitoring only — the final decision comes
        # from the decision ladder, not from this scalar.
        score=agent_score,
        claim_assessments=[{"claim_id": r.claim_id, "is_verified": r.cross_check_passed, "score": r.cross_check_score, "mismatches": r.mismatches} for r in cross_results],
        has_conflicts=has_conflicts,
        missing_claims=missing,
        feedback=_build_feedback(missing, cross_results),
        hard_violations=weak,
        agent_confidence=agent_score,
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


def route_sufficiency_verdict(
    verdict: SufficiencyVerdict,
    mode_label: str,
    cycle: int,
    max_cycles: int,
    auto: dict | None = None,
) -> tuple:
    """Decision-ladder routing → (action, should_continue, caveat).

    The LLM AutoRater (``auto``) is the primary sufficiency judge; the agent
    confidence (``verdict.agent_confidence``) is the risk gate. ``auto`` is the
    dict returned by ``llm_sufficiency_boost`` (``is_sufficient`` /
    ``confidence`` / ``missing`` / ``contradictions``). When absent (e.g. the
    medium mode did not trigger the AutoRater, or the tools lack an LLM judge),
    we fall back to a code-only decision so the loop still terminates sensibly.
    """
    mode = get_mode(mode_label)
    hard_violations = getattr(verdict, "hard_violations", []) or []
    agent_confidence = getattr(verdict, "agent_confidence", getattr(verdict, "score", 0.0))

    # AutoRater signals, with sane defaults when it was not invoked.
    auto_sufficient = bool(auto.get("is_sufficient")) if auto else (verdict.status == "SUFFICIENT")
    auto_confidence = float(auto.get("confidence") or 1.0) if auto else 1.0
    missing = list(auto.get("missing") or []) if auto else verdict.missing_claims
    contradictions = list(auto.get("contradictions") or []) if auto else ([verdict.feedback] if verdict.has_conflicts else [])

    from rag.advanced_rag.harness.sufficiency_ladder import sufficiency_ladder

    out = sufficiency_ladder(
        auto_sufficient=auto_sufficient,
        auto_confidence=auto_confidence,
        missing=missing,
        contradictions=contradictions,
        agent_confidence=agent_confidence,
        c_high=mode.c_high,
        c_low=mode.c_low,
        llm_floor=mode.llm_floor,
        allows_reconcile=mode.allows_reconcile,
        cycle=cycle,
        max_cycles=max_cycles,
        hard_violations=hard_violations,
    )
    _LOG.info(
        "[Sufficiency ladder] auto_sufficient=%s auto_conf=%.2f agent_conf=%.2f hard_violations=%s → %s",
        auto_sufficient,
        auto_confidence,
        agent_confidence,
        hard_violations,
        out.action,
    )

    # Map ladder action onto orchestrator actions.
    action = out.action
    if action == ANSWER_WITH_CAVEAT:
        action = "ANSWER_PARTIAL"
    elif action == RECONCILE:
        # medium has no reconcile loop → degrade to CONTINUE (keep searching).
        if mode.allows_reconcile:
            return ("CONTINUE", True, out.caveat)
        return ("CONTINUE", True, out.caveat)
    elif action == UNANSWERABLE:
        if mode.fallback_to_direct_llm:
            action = "FALLBACK_LLM"
        else:
            action = "ABSTAIN"
        return (action, False, out.caveat)
    # ANSWER / GAP
    return (action, out.should_continue, out.caveat)
