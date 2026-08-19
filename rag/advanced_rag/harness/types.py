"""Data types for Agentic RAG harness."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal

# ═══════════════════════════════════════════════════════════════
# Route
# ═══════════════════════════════════════════════════════════════


@dataclass
class RouteDecision:
    question: str
    thinking_mode: str
    question_type: str  # factual | comparative | analytical | procedural | exploratory | verification | summarization
    requires_decomposition: bool
    suggests_compilation: str | None
    execution_strategy: str
    reasoning: str = ""


# ═══════════════════════════════════════════════════════════════
# Thinking Mode
# ═══════════════════════════════════════════════════════════════


@dataclass
class ExecutionStrategy:
    label: Literal["low", "medium", "high", "ultra"]
    execution_strategy: Literal["direct_search", "decompose_and_search", "agentic_research", "deep_research"]
    requires_decomposition: bool
    requires_agent_loop: bool
    requires_sufficiency_judge: bool
    requires_selective_gen: bool
    allows_dynamic_claims: bool
    allows_replan: bool
    max_orchestrator_cycles: int
    max_agent_cycles: int
    max_parallel_agents: int
    available_tools: list[str]
    sufficiency_threshold: float
    fallback_to_direct_llm: bool
    # Min cross-check floor for a self-verified claim. Any claim scoring below
    # this becomes a hard veto (its localized evidence gap must not be averaged
    # away by stronger sibling claims). Default 0.5 == the cross-check pass bar.
    fusion_min_cross: float = 0.5
    # ── Decision-ladder operating points (Sufficient Context redesign) ──
    # These are monotonic product-policy thresholds — NOT trained weights. They
    # express "how conservative to be" (higher = investigate more before
    # answering), replacing the old weighted fusion.
    c_high: float = 0.75  # agent confidence >= this and AutoRater sufficient → ANSWER
    c_low: float = 0.45  # agent confidence >= this → ANSWER_WITH_CAVEAT; below → reconcile
    llm_floor: float = 0.55  # AutoRater confidence < this → re-investigate regardless of verdict
    # Whether the mode can force a re-investigation (RECONCILE). medium=False so
    # RECONCILE degrades to CONTINUE; high/ultra=True.
    allows_reconcile: bool = False


# ═══════════════════════════════════════════════════════════════
# Plan & Claims
# ═══════════════════════════════════════════════════════════════


@dataclass
class ClaimTarget:
    claim_id: str
    description: str
    priority: int = 0
    is_verified: bool = False
    confidence: float = 0.0
    suggested_tools: list[str] = field(default_factory=list)
    agent_result: AgentResult | None = None
    # Consecutive research rounds for THIS claim that stayed in `locate` and
    # produced neither evidence chunks nor routed document scope. Claim-scoped
    # on purpose: claims are researched in parallel over one shared
    # OrchestratorContext, so a global counter would let one claim's empty
    # locate rounds influence a sibling. Once it reaches
    # LOCATE_EMPTY_ADVANCE_THRESHOLD, the phase still stays `locate`, but
    # gating may admit `web_search` for facts the corpus lacks.
    locate_empty_streak: int = 0


@dataclass
class WorkflowPlan:
    plan_type: str  # direct | fact_decomposition | comparative_decomposition | procedural_decomposition | exploratory_decomposition
    claims: list[ClaimTarget]
    max_iterations: int


# ═══════════════════════════════════════════════════════════════
# Agent Result
# ═══════════════════════════════════════════════════════════════


@dataclass
class AgentResult:
    claim_id: str
    report: str
    is_verified: bool
    confidence: float
    evidence_ids: list[int] = field(default_factory=list)
    gaps: list[str] = field(default_factory=list)
    discovered_claims: list[str] = field(default_factory=list)
    # Key factual assertions the agent claims are directly supported by the
    # cited evidence. Used by cross-check as the *ground-truth* list to verify:
    # if a grounded fact is absent from the evidence, the claim is a
    # prior-knowledge-injection risk (see check1.log Q203 hometown=Ithaca,
    # Q665 first-solo-album=1970) and must not count as sufficient.
    grounded: list[str] = field(default_factory=list)
    # For numerical / multi-hop questions: the figures used in the answer with
    # their sources ("2,161,000 from Wikipedia Demographics of Paris 2019").
    # Used to detect multi-source numeric conflicts that a ratio/mean cross-check
    # would otherwise paper over (see check.log Q754: 225 vs 228 population口径).
    numbers: list[str] = field(default_factory=list)


# ═══════════════════════════════════════════════════════════════
# Cross Check
# ═══════════════════════════════════════════════════════════════


@dataclass
class ClaimCrossCheckResult:
    claim_id: str
    cross_check_passed: bool
    cross_check_score: float
    evidence_matches: list[str] = field(default_factory=list)
    mismatches: list[str] = field(default_factory=list)


# ═══════════════════════════════════════════════════════════════
# Sufficiency Verdict
# ═══════════════════════════════════════════════════════════════


@dataclass
class SufficiencyVerdict:
    status: str  # code-level preliminary label (decision ladder decides final action)
    score: float
    claim_assessments: list[dict]
    has_conflicts: bool
    missing_claims: list[str]
    feedback: str
    # Decision-ladder inputs (Sufficient Context redesign):
    #   - hard_violations: claim IDs with a code-proven evidence gap (required
    #     entity missing / grounded absent / numeric conflict) that must veto a
    #     full answer even if the LLM AutoRater says sufficient.
    #   - agent_confidence: mean self-confidence over the trusted subset.
    hard_violations: list[str] = field(default_factory=list)
    agent_confidence: float = 0.0


# ═══════════════════════════════════════════════════════════════
# Pipeline
# ═══════════════════════════════════════════════════════════════


@dataclass
class ToolResult:
    chunks: list[dict] = field(default_factory=list)
    # Doc-id list from routing tools (e.g. dataset_navigation_search) that
    # narrow the corpus to the relevant documents instead of returning chunks.
    docs: list[str] | None = None
    metadata: dict = field(default_factory=dict)
    error: str | None = None


# ═══════════════════════════════════════════════════════════════
# Orchestrator Context
# ═══════════════════════════════════════════════════════════════


@dataclass
class OrchestratorContext:
    question: str
    claims: list[ClaimTarget]
    mode: str
    iteration: int = 0
    verdict: SufficiencyVerdict | None = None
    history: list[dict] = field(default_factory=list)
    # Follow-up search queries produced by the Phase-2 LLM Sufficient Context
    # AutoRater when evidence was deemed insufficient. They are consumed by the
    # next research round to guide targeted follow-up search (Google's
    # missing-pieces feedback), then cleared.
    pending_followups: list[dict] = field(default_factory=list)
    _last_entity: str | None = None

    @property
    def last_entity(self) -> str | None:
        return self._last_entity

    def note_entity(self, name: str | None) -> None:
        """Record the most recently discovered entity/document name.

        Gates ``graph_explore`` in ``tool_fits_context``: the tool is only
        offered once research has surfaced something to expand from. Ignores
        empty values so a fruitless round can't clear a prior discovery.
        """
        if isinstance(name, str) and name.strip():
            self._last_entity = name.strip()

    @property
    def current_claim(self) -> str | None:
        unverified = [c for c in self.claims if not c.is_verified]
        return unverified[0].description if unverified else None

    def has_any_chunks(self) -> bool:
        """True once any claim's research produced evidence passages.

        Reads the claims (whose ``agent_result`` is populated by both
        orchestrators) — the ``agent_results`` dict is never written to, so
        reading it would leave the search phase stuck at "locate" forever and
        gate off every inspector tool.
        """
        for c in self.claims:
            r = c.agent_result
            if r is None:
                continue
            ids = r.get("evidence_ids") if isinstance(r, dict) else r.evidence_ids
            if ids:
                return True
        return False
