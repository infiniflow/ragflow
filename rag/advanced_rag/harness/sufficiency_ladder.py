"""Decision ladder for sufficiency — LLM AutoRater as primary judge, agent
confidence as a risk gate (Sufficient Context paper, arXiv 2411.06037).

This replaces the old "code cross-check + dual-signal weighted fusion + 5-way
verdict" pipeline. Because the operator cannot train/calibrate fusion weights
(the paper's logistic regression), we use a **monotonic threshold ladder**: the
LLM AutoRater decides *whether the evidence is sufficient to infer a plausible
answer*; the agent's self-assessed confidence only modulates *how the verdict is
presented* (full answer vs. caveated answer vs. reconcile). The only continuous
degree of freedom is conservativeness thresholds (``c_high`` / ``c_low`` /
``llm_floor``), which are product policy knobs, not learned parameters.

Semantics (paper §3 / §5.1):
  - Sufficiency = "a plausible answer A' can be inferred from the context",
    independent of whether that answer is correct, and without a ground truth.
  - Selective generation = confidence does not re-score the LLM's sufficiency
    verdict; it only decides whether to answer fully, caveat, or investigate.

Actions:
  - ``ANSWER``             full answer (sufficient + high confidence)
  - ``ANSWER_WITH_CAVEAT`` answer with an explicit caveat
  - ``GAP``                follow-up search on the concrete missing pieces
  - ``RECONCILE``          re-investigate a low-confidence / contradictory point
  - ``UNANSWERABLE``       cannot answer (or fall back to a direct LLM)
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

# Action constants (shared with callers / orchestrators).
ANSWER = "ANSWER"
ANSWER_WITH_CAVEAT = "ANSWER_WITH_CAVEAT"
GAP = "GAP"
RECONCILE = "RECONCILE"
UNANSWERABLE = "UNANSWERABLE"


@dataclass
class LadderOutput:
    """Result of the decision ladder."""

    action: str
    should_continue: bool
    caveat: str = ""
    missing: list[str] = field(default_factory=list)


def aggregate_agent_confidence(
    agent_results: list[Any],
    hard_violations: set[str] | dict[str, list[str]] | None = None,
) -> float:
    """Mean self-confidence over the trusted (self-verified, no hard veto) claims.

    No weights, no normalization — we only pick the trustworthy subset and take
    the mean. Hard vetoes (required-entity gaps, grounded-fact violations,
    numeric conflicts) suppress a claim's confidence entirely, so a confidence
    signal can never mask a missing-evidence gap (selective generation).
    """
    violations: set[str] = set()
    if hard_violations:
        if isinstance(hard_violations, dict):
            violations = set(hard_violations.keys())
        else:
            violations = set(hard_violations)

    trusted = [r for r in agent_results if getattr(r, "is_verified", False) and getattr(r, "claim_id", "") not in violations]
    if not trusted:
        return 0.0
    total = 0.0
    for r in trusted:
        try:
            total += float(getattr(r, "confidence", 0.0) or 0.0)
        except (TypeError, ValueError):
            continue
    return total / len(trusted)


def sufficiency_ladder(
    *,
    auto_sufficient: bool,
    auto_confidence: float,
    missing: list[str],
    contradictions: list[str],
    agent_confidence: float,
    c_high: float,
    c_low: float,
    llm_floor: float,
    allows_reconcile: bool,
    cycle: int,
    max_cycles: int,
    hard_violations: set[str] | dict[str, list[str]] | None = None,
) -> LadderOutput:
    """Evaluate the decision ladder and return the action.

    Parameters
    ----------
    auto_sufficient : bool
        LLM AutoRater's verdict (a plausible answer can be inferred).
    auto_confidence : float
        AutoRater's own confidence in its sufficiency judgment.
    missing : list[str]
        Concrete gaps when AutoRater says insufficient.
    contradictions : list[str]
        Evidence-internal contradictions (even if sufficient, caveat).
    agent_confidence : float
        Aggregated agent self-confidence (0..1).
    c_high / c_low : float
        Agent-confidence gates for full vs. caveated answer.
    llm_floor : float
        If AutoRater confidence < this, re-investigate regardless of verdict.
    allows_reconcile : bool
        Whether the mode can force a re-investigation (medium=False).
    cycle / max_cycles : int
        0-based current cycle and the mode's budget.
    hard_violations : set | dict
        Claim IDs (or {id: gaps}) with a proven evidence gap (required entity
        missing / grounded absent / numeric conflict). Any non-empty value
        forces a caveated answer even if AutoRater says sufficient.
    """
    violations: set[str] = set()
    if hard_violations:
        violations = set(hard_violations.keys()) if isinstance(hard_violations, dict) else set(hard_violations)

    # 1. Hard veto floor: code-proven evidence gap beats the LLM's "good enough".
    if violations:
        return LadderOutput(
            action=ANSWER_WITH_CAVEAT,
            should_continue=False,
            caveat=f"hard evidence gap in claim(s): {sorted(violations)[:6]}",
            missing=missing,
        )

    # 2. AutoRater says insufficient.
    if not auto_sufficient:
        if missing:
            return LadderOutput(action=GAP, should_continue=True, missing=missing)
        return LadderOutput(action=UNANSWERABLE, should_continue=False, missing=missing)

    # 3. AutoRater is not confident in its own sufficiency call.
    if auto_confidence < llm_floor:
        if allows_reconcile and cycle < max_cycles - 1:
            return LadderOutput(
                action=RECONCILE,
                should_continue=True,
                caveat="AutoRater itself is unsure; re-investigating",
            )
        return LadderOutput(
            action=ANSWER_WITH_CAVEAT,
            should_continue=False,
            caveat="AutoRater sufficiency judgment is low-confidence",
        )

    # 4. Sufficient + confident: agent confidence sets the presentation.
    caveat = ""
    should_continue = False
    if agent_confidence >= c_high:
        action = ANSWER
    elif agent_confidence >= c_low:
        action = ANSWER_WITH_CAVEAT
        caveat = "evidence partially supports the answer"
    elif allows_reconcile and cycle < max_cycles - 1:
        action = RECONCILE
        should_continue = True
    else:
        action = ANSWER_WITH_CAVEAT
        caveat = "evidence partially supports the answer"

    if contradictions:
        # Never silently pick one side of a contradiction; surface it.
        caveat = "evidence contains conflicting figures"
        action = ANSWER_WITH_CAVEAT
        should_continue = False

    return LadderOutput(action=action, should_continue=should_continue, caveat=caveat, missing=missing)
