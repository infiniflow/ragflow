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
    # legacy dispatch field — only ``low``'s "direct_search" is consumed by the
    # (remaining) orchestrator path; medium/high/ultra run the SCA react graph and
    # never read it.
    execution_strategy: Literal["direct_search", "decompose_and_search", "agentic_research", "deep_research"] = "direct_search"
    # Legacy research-agent / decomposition flags — not consumed by any running
    # path (medium/high/ultra run the single-agent SCA graph; low runs direct_search).
    requires_decomposition: bool = False
    requires_agent_loop: bool = False
    requires_sufficiency_judge: bool = False
    requires_selective_gen: bool = False
    allows_dynamic_claims: bool = False
    allows_replan: bool = False
    max_orchestrator_cycles: int = 1
    max_agent_cycles: int = 0
    max_parallel_agents: int = 1
    available_tools: list[str] = field(default_factory=list)
    sufficiency_threshold: float = 0.5
    fallback_to_direct_llm: bool = False
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
    # Terminal-tool shortcut: when True, the outer tool loop treats ``rag`` as a
    # terminal tool and short-circuits after its first successful call — the
    # produced (cited) answer is returned immediately instead of being fed back
    # for another outer round. Mirrors the ``_terminal`` short-circuit that
    # already exists in the streaming tool loops. When False, the outer loop
    # keeps its multi-round re-ask behaviour (research more evidence between
    # rounds). Default False keeps legacy behaviour unless a mode opts in.
    terminal_tool_shortcut: bool = False


# ═══════════════════════════════════════════════════════════════
# Plan & Claims
# ═══════════════════════════════════════════════════════════════


@dataclass
class ClaimPlan:
    """Cross-round, per-claim plan state driving iterative, plan-anchored search.

    Unlike the old `while cycle < max_cycles` re-search loop, this keeps what a
    claim has learned and what it still needs across rounds, so each round only
    searches the missing part instead of blindly re-searching everything.
    """

    # Evidence accumulated across rounds (grounded facts + numbers).
    evidence: list[str] = field(default_factory=list)
    # Structured missing information (which entity / time / enumeration item).
    missing: list[str] = field(default_factory=list)
    # Bridge entities already resolved from prior hops (e.g. ["Tulsa", "1921"]).
    resolved_bridge: list[str] = field(default_factory=list)
    # Ordered hops still to resolve (each an atomic open query; later hops may
    # reference earlier resolved values via {0}, {1}, ...).
    pending_hops: list[str] = field(default_factory=list)
    # Next retrieval target for this round (entity+time anchored), or "".
    next_target: str = ""
    # Structurally accumulated member values for an AGGREGATE enumeration claim
    # (e.g. ["Minute Maid Park 330", "American Family Field 315"]). Each round the
    # claim's query_check appends newly-confirmed members here; ``missing`` records
    # which member(s) are still outstanding. combine claims read this as the
    # complete member set to synthesize from. Enables code-level completeness
    # checking ("no members missing") instead of trusting free-text reports.
    enumerated_members: list[str] = field(default_factory=list)

    # Whether any retrieval target remains (refined / hop / next_target / missing).
    def actionable(self) -> bool:
        return bool(self.missing or self.pending_hops or self.next_target)


@dataclass
class ClaimTarget:
    claim_id: str
    description: str
    priority: int = 0
    is_verified: bool = False
    confidence: float = 0.0
    suggested_tools: list[str] = field(default_factory=list)
    agent_result: AgentResult | None = None
    # Multi-hop: an OPEN query for a bridge entity/relationship that must be
    # retrieved BEFORE this claim can be answered (e.g. "Who is Brian Bergstein's
    # employer?"). decompose resolves the prerequisite first, then uses the found
    # entity to target this claim. Empty when not multi-hop.
    prerequisite: str = ""
    # Ordered list of hops for multi-hop claims (each an atomic open query, in
    # dependency order). When present, bridge resolution walks these hops and
    # stores resolved values in `plan.resolved_bridge`. Backward compatible: when
    # empty, we fall back to the single `prerequisite`.
    prerequisites: list[str] = field(default_factory=list)
    # Reasoning structure the planner assigned to this claim:
    #   flat       — single-hop fact, independent
    #   chain      — depends on a prerequisite (bridge entity/relationship)
    #   aggregate  — requires enumerating all members of a set and combining
    #                them (count/sum/average/max/min)
    #   temporal   — answer depends on a specific year/period or cross-time link
    #   comparative/procedural — retained for compatibility
    claim_type: str = "flat"
    # For aggregate claims: the full set of members to enumerate (e.g. "all MLB
    # stadiums with a retractable roof"), used to guide exhaustive retrieval.
    target: str = ""
    # Pure-synthesis marker: this claim is NOT a retrievable fact — it is a
    # combine/synthesis node whose value is produced by formalize_answer from the
    # enumerated member values (via WorkflowPlan.synthesis). It must never be fed
    # to hybrid_search / query_check (a "Combine the enumerated values..." query
    # retrieves nothing meaningful). The orchestrator excludes it from search and
    # treats its completeness as dependent on its source enumeration claim.
    synth_only: bool = False
    # Cross-round plan state (evidence / missing / bridge / hops).
    plan: ClaimPlan = field(default_factory=ClaimPlan)


@dataclass
class WorkflowPlan:
    plan_type: str  # direct | fact_decomposition | comparative_decomposition | procedural_decomposition | exploratory_decomposition
    claims: list[ClaimTarget]
    max_iterations: int
    # One-sentence cross-claim synthesis instruction from the planner: how the
    # individual claim findings must be combined into the final answer (e.g. for a
    # difference question "answer = c2.year - c1.year"; for a comparison "select
    # the youngest from c1's enumerated list"; for an aggregate "sum c1's counts").
    # Empty for a single-claim / flat question where no synthesis is needed. It is
    # injected into pre_summary so formalize_answer performs the composition
    # explicitly instead of guessing it from juxtaposed claim blocks.
    synthesis: str = ""


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
class ExecutionBook:
    """Per-run execution ledger (L1 state).

    Converges the previously-scattered per-claim bookkeeping variables
    (``attempted_queries``, ``attempted_evidence``, ``pending_queries``,
    ``refined_queries``, follow-up pool, frozen set) into ONE object that lives
    with the orchestrator run, so replan / full-redo / claim-frozen never need
    fragile ``dict.update(...)`` patches to stay in sync. Each key is a claim_id
    (or the sentinel ``""`` for the global follow-up pool).

    Lifecycle:
      * Inner-loop retry and incremental replan MUTATE it in place (same plan).
      * FULL plan re-do REPLACES the per-claim entries for dropped claims while
        preserving ``frozen`` and the global follow-up pool (verified evidence
        lives in ``kbinfos``/``claim.agent_result``, not here).
    """

    # Queries already issued per claim (dedup key for the inner retry loop).
    attempted_queries: dict[str, set] = field(default_factory=dict)
    # Evidence chunk ids already analyzed per claim.
    attempted_evidence: dict[str, set] = field(default_factory=dict)
    # Targeted queries waiting to be searched for a claim (next-hops / replan).
    pending_queries: dict[str, list] = field(default_factory=dict)
    # query_check's structured "refined next query" per claim (smartsearch).
    refined_queries: dict[str, str] = field(default_factory=dict)
    # SCA / AutoRater global follow-up pool (consumed then cleared).
    followup_pool: list[dict] = field(default_factory=list)
    # Claim ids frozen by claim-level stagnation guard (never re-searched; their
    # already-collected evidence stays available for finalize).
    frozen: set = field(default_factory=set)
    # Normalized text of queries injected by the Query Rewriter / replan
    # (``_inject_replan_queries``). These are *precise, self-contained* targeted
    # queries (one-to-one for a reported gap), so the orchestrator must NOT
    # re-append the global formalize keywords when searching them — appending
    # them re-introduces the query dilution/pollution the rewrite was meant to
    # fix (see decompose._ref_kw).
    replan_query_seen: set = field(default_factory=set)

    def init_claim(self, claim_id: str) -> None:
        """Idempotently create an empty ledger entry for ``claim_id``."""
        self.attempted_queries.setdefault(claim_id, set())
        self.attempted_evidence.setdefault(claim_id, set())
        self.pending_queries.setdefault(claim_id, [])
        self.refined_queries.setdefault(claim_id, "")

    def drop_claim(self, claim_id: str) -> None:
        """Remove a claim's ledger entries (used by full plan re-do)."""
        for m in (self.attempted_queries, self.attempted_evidence, self.pending_queries, self.refined_queries):
            m.pop(claim_id, None)


@dataclass
class OrchestratorContext:
    question: str
    claims: list[ClaimTarget]
    mode: str
    iteration: int = 0
    current_phase: str = "locate"
    verdict: SufficiencyVerdict | None = None
    history: list[dict] = field(default_factory=list)
    # Follow-up search queries produced by the LLM Sufficient Context
    # AutoRater when evidence was deemed insufficient. They are consumed by the
    # next research round to guide targeted follow-up search, then cleared.
    pending_followups: list[dict] = field(default_factory=list)
    # Cross-claim synthesis instruction from the planner (how claim findings
    # combine into the final answer). Injected into pre_summary by _finalize.
    synthesis: str = ""
    # Deterministic arithmetic result derived from the gathered facts (label,
    # value, expression) when the question asks for a number that follows
    # arithmetically from them. Computed by _compute_final_arithmetic and
    # injected into pre_summary so formalize_answer uses the exact value.
    computed_answer: str = ""
    # Targeted search queries produced by the Query Rewriter
    # from the SCA's forward gaps, as (claim_id, query) pairs recorded by
    # ``_try_replan`` for the newly-added claims. The orchestrator injects them
    # into the new claims' pending-query pool so they reach the retriever
    # verbatim — NOT re-synthesized by _pick_next_query — closing the "rewrite
    # got diluted" gap. Cleared after a successful replan.
    replan_queries: list[tuple[str, str]] = field(default_factory=list)
    # Number of FULL plan re-do's performed (the outer-loop-equivalent: re-running
    # the planner with SCA feedback to correct the overall decomposition direction,
    # as opposed to an incremental replan that only adds missing-claim search).
    # Bounded so a persistently-wrong plan cannot re-decompose forever.
    full_replan_count: int = 0
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
