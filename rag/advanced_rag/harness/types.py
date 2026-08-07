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
    partial_threshold: float
    fallback_to_direct_llm: bool


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
    status: str  # SUFFICIENT | USEFUL_BUT_INCOMPLETE | INSUFFICIENT | CONFLICTING | UNANSWERABLE
    score: float
    agent_score: float
    cross_score: float
    claim_assessments: list[dict]
    has_conflicts: bool
    missing_claims: list[str]
    feedback: str
    overall_reason: str


# ═══════════════════════════════════════════════════════════════
# Pipeline
# ═══════════════════════════════════════════════════════════════


@dataclass
class ToolResult:
    chunks: list[dict] = field(default_factory=list)
    # Doc-id list from routing tools (e.g. dataset_navigation_by_tree) that
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
    current_phase: str = "locate"
    verdict: SufficiencyVerdict | None = None
    history: list[dict] = field(default_factory=list)
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
