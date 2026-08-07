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


def extract_named_entities(text: str) -> list[str]:
    """Simple entity extraction — looks for capitalized multi-word sequences."""
    entities = re.findall(r"\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*\b", text)
    return list(set(entities))


def cross_check_claim(agent_result: AgentResult, all_chunks: dict) -> ClaimCrossCheckResult:
    """Code-level cross-check: number matching + entity presence."""
    report = agent_result.report
    claimed = agent_result.is_verified

    if not claimed:
        return ClaimCrossCheckResult(
            claim_id=agent_result.claim_id,
            cross_check_passed=False,
            cross_check_score=0.0,
            mismatches=["agent self-reported as unverified"],
        )

    numbers = extract_numbers(report)
    entities = extract_named_entities(report)

    mismatches = []
    matches = []

    for eid in agent_result.evidence_ids or []:
        chunk = all_chunks.get(eid)
        if not chunk:
            mismatches.append(f"evidence_id={eid}: chunk not found")
            continue
        text = chunk.get("content_with_weight", chunk.get("text", ""))
        text_lower = text.lower()

        for num in numbers:
            # Numbers are extracted as floats ("1976" -> 1976.0) while chunk
            # text spells them "1976" — match both the raw and integral forms.
            # Bounded match: a number must not sit adjacent to other digit
            # characters, so 1976 does not match inside 19760.
            forms = {str(num), str(int(num))} if float(num).is_integer() else {str(num)}
            if any(re.search(rf"(?<![\w]){re.escape(f)}(?![\w])", text_lower) for f in forms):
                matches.append(f"number {num} found in chunk {eid}")
            else:
                mismatches.append(f"number {num} not found in chunk {eid}")

        for ent in entities:
            # Bounded word/phrase match: Ann must not match Annual (no adjacent
            # word characters on either side).
            if re.search(rf"(?<![\w]){re.escape(ent.lower())}(?![\w])", text_lower):
                matches.append(f"entity '{ent}' found in chunk {eid}")
            else:
                mismatches.append(f"entity '{ent}' not found in chunk {eid}")

    total = len(matches) + len(mismatches)
    if total == 0:
        # No evidence was actually examined — fail rather than pass neutrally:
        # a claim with zero evidence IDs cannot be cross-checked at all.
        if not agent_result.evidence_ids:
            return ClaimCrossCheckResult(
                claim_id=agent_result.claim_id,
                cross_check_passed=False,
                cross_check_score=0.0,
                mismatches=["no evidence"],
            )
        # Evidence IDs exist but nothing extractable to verify against (e.g.
        # Chinese reports yield no capitalized entities and no digits) — the
        # cross-check cannot falsify the claim, so pass neutrally.
        return ClaimCrossCheckResult(
            claim_id=agent_result.claim_id,
            cross_check_passed=True,
            cross_check_score=1.0,
        )
    cross_score = len(matches) / total
    cross_passed = len(mismatches) < len(matches) * 0.5

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
    # Signal A: agent self-assessment
    verified_count = sum(1 for r in agent_results if r.is_verified)
    agent_score = verified_count / max(len(agent_results), 1)

    # Signal B: cross-check
    passed_count = sum(1 for r in cross_check_results if r.cross_check_passed)
    cross_score = passed_count / max(len(cross_check_results), 1)

    # Fusion strategy by mode
    fusion = {
        "ultra": lambda a, c: min(a, c),
        "high": lambda a, c: (a + c) / 2,
        "medium": lambda a, c: max(a, c),
        "low": lambda a, c: max(a, c),
    }.get(mode.label, lambda a, c: max(a, c))
    fusion_score = fusion(agent_score, cross_score)

    # Conflict detection
    has_conflicts = any(len(r.mismatches) > 0 for r in cross_check_results)

    # 5-way verdict
    if has_conflicts and fusion_score < mode.partial_threshold:
        status = "CONFLICTING"
    elif fusion_score >= mode.sufficiency_threshold:
        status = "SUFFICIENT"
    elif fusion_score >= mode.partial_threshold:
        status = "USEFUL_BUT_INCOMPLETE"
    elif not any(r.cross_check_passed for r in cross_check_results):
        status = "UNANSWERABLE"
    else:
        status = "INSUFFICIENT"

    missing = [r.claim_id for r in cross_check_results if not r.cross_check_passed]

    return SufficiencyVerdict(
        status=status,
        score=fusion_score,
        agent_score=agent_score,
        cross_score=cross_score,
        claim_assessments=[{"claim_id": r.claim_id, "is_verified": r.cross_check_passed, "score": r.cross_check_score, "mismatches": r.mismatches} for r in cross_check_results],
        has_conflicts=has_conflicts,
        missing_claims=missing,
        feedback=_build_feedback(missing, cross_check_results),
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

    if verdict.status == "SUFFICIENT":
        return ("ANSWER", False)

    if verdict.status == "USEFUL_BUT_INCOMPLETE":
        if mode.requires_selective_gen:
            return ("ANSWER_PARTIAL", False)
        return ("CONTINUE", False)

    if verdict.status == "INSUFFICIENT":
        # ``cycle`` is 0-based, so the last cycle is ``max_cycles - 1`` — the
        # old ``max_cycles * 0.8`` threshold was never reached for the 3/3/4
        # cycle budgets, making this branch dead code.
        if cycle >= max_cycles - 1:
            return ("ANSWER_PARTIAL", False)
        return ("CONTINUE", True)

    if verdict.status == "CONFLICTING":
        if mode.allows_replan and cycle < max_cycles * 0.5:
            return ("REPLAN", True)
        return ("ANSWER_PARTIAL", False)

    if verdict.status == "UNANSWERABLE":
        if mode.fallback_to_direct_llm:
            return ("FALLBACK_LLM", False)
        return ("ABSTAIN", False)

    return ("CONTINUE", True)
