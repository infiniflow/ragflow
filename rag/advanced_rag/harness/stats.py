"""LLM-call instrumentation for the agentic RAG harness.

Every phase of the agentic pipeline (route / planner / orchestrator / agent /
sufficiency / grounded / finalize, ...) drives the LLM through ``tools.chat_mdl``.
``RAGTools`` wraps that bundle in a :class:`CountingChatModel` proxy which records,
per phase, the number of LLM calls and the token usage reported by the provider.
Phase wall-clock time is measured by :func:`phase` itself — unlike summed LLM
latency, which is meaningless when calls run in parallel. The aggregate is
emitted to the log when each ``rag`` call finishes.
"""

import logging
import time
from collections import defaultdict
from contextlib import contextmanager
from contextvars import ContextVar
from functools import wraps
from inspect import iscoroutinefunction

_LOG = logging.getLogger("rag.advanced_rag.harness.stats")

_CURRENT_PHASE: ContextVar[str] = ContextVar("agentic_rag_llm_phase", default="unknown")

# Canonical pipeline order for the per-phase usage table. Phases are listed in
# the order they execute across the (high/ultra) agentic pipeline so the log
# reads top-to-bottom like the actual flow, regardless of which phase first
# touched the stats counters. Any phase not in this list (e.g. a future
# addition) is appended afterwards, alphabetically, as a safe fallback.
_PHASE_ORDER = [
    "formalize",
    "route",
    "pre_search",
    "planner",
    "decompose",
    "direct",
    "orchestrator",
    "claim_research",
    "sufficiency",
    "grounded",
    "finalize",
]
_PHASE_RANK = {name: i for i, name in enumerate(_PHASE_ORDER)}


@contextmanager
def phase(name: str):
    """Run the enclosed block with the LLM-call phase set to ``name``.

    Also accrues the block's wall-clock time into the active stats (the
    per-call one bound by :func:`using_stats`, falling back to nothing), so
    each phase reports how long it actually took to execute. Nested phases
    (e.g. ``orchestrator`` containing ``agent``) overlap by design: the outer
    wall-clock includes the inner one, so totals are not summed.

    Re-entrancy: the same phase name may be wrapped several times along one
    call path (graph node + implementation fn + orchestrator wrapper). Only
    the outermost interval is timed — inner re-entries of the same name are
    shadowed and contribute no wall-clock, preventing the time from being
    counted 2-3 times.
    """
    token = _CURRENT_PHASE.set(name)
    stats = _CURRENT_STATS.get()
    if stats is not None:
        stats.note_start(name)
        entry_round = stats.current_round
        shadowed = name in stats._active_phases
        stats._active_phases.append(name)
    else:
        entry_round = 0
        shadowed = True  # no stats object -> timing guarded by `if stats is not None` below
    t0 = time.perf_counter()
    try:
        yield
    finally:
        _CURRENT_PHASE.reset(token)
        stats = _CURRENT_STATS.get()
        if stats is not None:
            stats._active_phases.pop()
            if not shadowed:
                stats.record_phase_time(name, (time.perf_counter() - t0) * 1000.0, entry_round=entry_round)


def in_phase(name: str):
    """Decorator: run the (async) function body inside :func:`phase`."""

    def decorate(fn):
        if iscoroutinefunction(fn):

            @wraps(fn)
            async def wrapper(*args, **kwargs):
                with phase(name):
                    return await fn(*args, **kwargs)

            return wrapper

        @wraps(fn)
        def wrapper(*args, **kwargs):
            with phase(name):
                return fn(*args, **kwargs)

        return wrapper

    return decorate


class LLMUsageStats:
    """Per-phase LLM call, wall-clock & token counters for one agentic ``rag`` run."""

    def __init__(self) -> None:
        self.calls: dict[str, int] = defaultdict(int)
        self.failed: dict[str, int] = defaultdict(int)
        self.phase_time_ms: dict[str, float] = defaultdict(float)
        self.prompt_tokens: dict[str, int] = defaultdict(int)
        self.completion_tokens: dict[str, int] = defaultdict(int)
        self.total_tokens: dict[str, int] = defaultdict(int)
        self.rounds: dict[str, int] = defaultdict(int)
        self.round_times: dict[str, list[float]] = defaultdict(list)
        # Phase wall-clock split per orchestrator round (index 0 = round 1).
        # A phase that runs multiple times inside one round (e.g. the agent
        # researching several claims in parallel) accumulates into that round.
        self.round_phase_times_ms: dict[str, list[float]] = defaultdict(list)
        self.round_claim_counts: dict[str, list[int]] = defaultdict(list)
        self._round_starts: dict[str, float] = {}
        self._current_round: int = 0
        # Stack of phase names currently entered (for re-entrancy detection).
        # When the same phase name is wrapped multiple times along one call
        # path (e.g. graph node + impl fn + orchestrator wrapper), only the
        # outermost interval is timed; inner re-entries are shadowed and add no
        # wall-clock, so the time is not double/triple counted.
        self._active_phases: list[str] = []

    @property
    def current_round(self) -> int:
        """Index (1-based) of the orchestrator round currently executing, 0 outside."""
        return self._current_round

    def note_start(self, phase_name: str) -> None:
        """Hook called when a phase is entered (phase wall-clock is recorded
        separately in :meth:`record_phase_time`). The log row order is fixed by
        the canonical ``_PHASE_ORDER`` list, so nothing needs to be tracked here
        anymore."""
        return

    def record_call(self, phase_name: str) -> None:
        self.calls[phase_name] += 1

    def record_failed(self, phase_name: str) -> None:
        self.failed[phase_name] += 1

    def record_phase_time(self, phase_name: str, elapsed_ms: float, entry_round: int = 0) -> None:
        self.phase_time_ms[phase_name] += elapsed_ms
        # Attribute the wall-clock to the orchestrator round this phase entered.
        # The current round index is captured at phase entry, so a round with
        # several nested entries (parallel agent runs) still accumulates into
        # exactly one per-round slot.
        if entry_round > 0:
            times = self.round_phase_times_ms[phase_name]
            while len(times) < entry_round:
                times.append(0.0)
            times[entry_round - 1] += elapsed_ms
        # Settle any dangling final round (the loop ended without another
        # ``record_round``) against the phase total: total minus settled rounds.
        pending = self.rounds[phase_name] - len(self.round_times[phase_name])
        if pending > 0:
            settled = sum(self.round_times[phase_name])
            self.round_times[phase_name].append(max(0.0, elapsed_ms - settled))
            self._round_starts.pop(phase_name, None)
        if phase_name == "orchestrator":
            # The orchestrator phase brackets the whole loop; once it exits no
            # later phase (e.g. finalize) belongs to any orchestrator round.
            self._current_round = 0

    def record_usage(self, phase_name: str, usage: dict | None) -> None:
        if not usage:
            return
        self.prompt_tokens[phase_name] += int(usage.get("prompt_tokens") or 0)
        self.completion_tokens[phase_name] += int(usage.get("completion_tokens") or 0)
        self.total_tokens[phase_name] += int(usage.get("total_tokens") or 0)

    def record_round(self, phase_name: str) -> None:
        """Count one iteration of a looping phase (e.g. an orchestrator cycle)
        and close the previous iteration's wall-clock."""
        self.rounds[phase_name] += 1
        self._current_round = self.rounds[phase_name]
        now = time.perf_counter()
        prev = self._round_starts.pop(phase_name, None)
        if prev is not None:
            self.round_times[phase_name].append((now - prev) * 1000.0)
        self._round_starts[phase_name] = now

    def record_round_claims(self, phase_name: str, count: int) -> None:
        if self._current_round <= 0:
            return
        counts = self.round_claim_counts[phase_name]
        while len(counts) < self._current_round:
            counts.append(0)
        counts[self._current_round - 1] += int(count or 0)

    def snapshot(self) -> dict[str, dict]:
        known = set(self.calls) | set(self.failed) | set(self.total_tokens) | set(self.phase_time_ms) | set(self.rounds)
        # Rows follow the canonical pipeline order (see _PHASE_ORDER) so the
        # table reads like the actual execution flow. Phases not in the known
        # list are appended afterwards, sorted, as a safe fallback.
        phases = [p for p in _PHASE_ORDER if p in known]
        phases += sorted(known - set(phases))
        rows = {}
        for p in phases:
            per_round = self.round_phase_times_ms.get(p) or []
            if per_round:
                rounds, round_times = len(per_round), list(per_round)
            else:
                rounds, round_times = self.rounds[p], list(self.round_times[p])
            rows[p] = {
                "calls": self.calls[p],
                "failed": self.failed[p],
                "phase_time_ms": self.phase_time_ms[p],
                "prompt_tokens": self.prompt_tokens[p],
                "completion_tokens": self.completion_tokens[p],
                "total_tokens": self.total_tokens[p],
                "rounds": rounds,
                "round_times": round_times,
                "round_claim_counts": list(self.round_claim_counts.get(p) or []),
            }
        return rows

    def log(self, logger: logging.Logger | None = None) -> None:
        log = logger or _LOG
        rows = self.snapshot()
        if not rows:
            # No LLM activity recorded (e.g. a cache hit that returned before
            # any call). Still emit a line so every completed ``rag`` call is
            # accounted for in the logs, instead of silently disappearing.
            log.info("[Agentic RAG] LLM usage by phase: (cached / no LLM calls)")
            return
        total_calls = sum(r["calls"] for r in rows.values())
        total_tokens = sum(r["total_tokens"] for r in rows.values())
        header = "  %-16s %7s %10s %12s %10s %10s" % (
            "phase", "llm_calls", "prompt_tok", "output_tok", "total_tok", "time(s)",
        )
        lines = ["[Agentic RAG] LLM usage by phase:", header]

        # With orchestrator-round data, expand the table hierarchically: each
        # round repeats its ``orchestrator`` row with the nested sub-phases
        # (agent/sufficiency/grounded) indented underneath. Time(s) is the
        # per-round wall-clock; the token columns are the phase totals, repeated
        # per round for readability. Phases outside the loop (route/planner/
        # finalize) are listed flat afterwards.
        per_round = self.round_phase_times_ms
        orch_rt = self.round_times.get("orchestrator") or []
        n_rounds = max([len(orch_rt)] + [len(v) for v in per_round.values()]) if (per_round or orch_rt) else 0

        def phase_label(p: str, r: dict, round_idx: int | None = None) -> str:
            label = p
            if p == "claim_research":
                counts = r.get("round_claim_counts") or []
                if round_idx is not None and round_idx < len(counts) and counts[round_idx] > 0:
                    label = f"{p} ({counts[round_idx]})"
            return label

        def row(indent: str, p: str, t_ms: float, round_idx: int | None = None) -> str:
            r = rows[p]
            return "%s%-16s %7d %10d %12d %10d %10.1f" % (
                indent, phase_label(p, r, round_idx), r["calls"],
                r["prompt_tokens"], r["completion_tokens"], r["total_tokens"], t_ms / 1000.0,
            )

        in_rounds = set(per_round)

        def custom_row(indent: str, label: str, p: str, t_ms: float) -> str:
            r = rows[p]
            return "%s%-16s %7d %10d %12d %10d %10.1f" % (
                indent, label, r["calls"],
                r["prompt_tokens"], r["completion_tokens"], r["total_tokens"], t_ms / 1000.0,
            )

        # Walk ``rows`` in its existing (canonical) order. When we reach the
        # orchestrator phase, print one block per round with its nested
        # sub-phases underneath, then suppress those sub-phases from appearing
        # again as duplicate top-level rows later in the table.
        for p in rows:
            if p == "orchestrator" and n_rounds:
                for i in range(n_rounds):
                    # ``orch_rt[i]`` is already in ms (matches ``phase_time_ms``);
                    # ``row()`` converts to seconds, so do NOT pre-divide here.
                    orch_t = orch_rt[i] if i < len(orch_rt) else rows[p]["phase_time_ms"]
                    lines.append(custom_row("  ", f"orchestrator round {i + 1}", p, orch_t))
                    for sub in rows:
                        if sub == "orchestrator":
                            continue
                        v = per_round.get(sub)
                        if not v or i >= len(v):
                            continue
                        lines.append(row("    ", sub, v[i], round_idx=i))
            elif p in in_rounds and n_rounds:
                # Already printed inside the per-round orchestrator block.
                continue
            else:
                lines.append(row("  ", p, rows[p]["phase_time_ms"]))
        lines.append("  total: %d LLM calls, %d tokens" % (total_calls, total_tokens))
        log.info("\n".join(lines))


_CURRENT_STATS: ContextVar["LLMUsageStats | None"] = ContextVar("agentic_rag_llm_stats", default=None)


@contextmanager
def using_stats(stats: LLMUsageStats):
    """Count the enclosed block's LLM calls into ``stats`` instead of the shared sink.

    ``CountingChatModel`` records into the innermost active stats (via
    ``_CURRENT_STATS``, falling back to the bundle-wide one), so parallel ``rag``
    calls — which run in separate asyncio tasks — each get an independent
    accounting while the shared sink keeps counting the whole request.
    """
    token = _CURRENT_STATS.set(stats)
    try:
        yield stats
    finally:
        _CURRENT_STATS.reset(token)


def record_round(name: str) -> None:
    """Count one iteration of ``name`` into the active stats (no-op when no
    stats are bound, mirroring :func:`phase`)."""
    stats = _CURRENT_STATS.get()
    if stats is not None:
        stats.record_round(name)


def record_round_claims(name: str, count: int) -> None:
    """Record how many claim-level tasks ran in the current round for ``name``."""
    stats = _CURRENT_STATS.get()
    if stats is not None:
        stats.record_round_claims(name, count)


def _last_usage(chat_mdl) -> dict | None:
    mdl = getattr(chat_mdl, "mdl", None)
    usage = getattr(mdl, "last_usage", None)
    if isinstance(usage, dict) and usage.get("total_tokens"):
        return usage
    return None


class CountingChatModel:
    """Proxy over an ``LLMBundle`` recording calls/tokens per phase.

    All other attributes are forwarded to the wrapped bundle, so the rest of
    the harness (``max_length``, ``bind_tools``, ``clone``, ...) keeps working
    unchanged while every ``async_chat*`` entry point is counted.
    """

    def __init__(self, chat_mdl, stats: LLMUsageStats):
        self._chat_mdl = chat_mdl
        self._stats = stats

    def _stats_for(self) -> LLMUsageStats:
        return _CURRENT_STATS.get() or self._stats

    def clone(self):
        return CountingChatModel(self._chat_mdl.clone(), self._stats)

    def __getattr__(self, name: str):
        return getattr(self._chat_mdl, name)

    async def async_chat(self, system: str, history: list, gen_conf: dict | None = None, **kwargs):
        stats = self._stats_for()
        phase_name = _CURRENT_PHASE.get()
        try:
            txt = await self._chat_mdl.async_chat(system, history, gen_conf or {}, **kwargs)
        except Exception:
            stats.record_failed(phase_name)
            raise
        stats.record_call(phase_name)
        stats.record_usage(phase_name, _last_usage(self._chat_mdl))
        return txt

    async def async_chat_streamly(self, system: str, history: list, gen_conf: dict | None = None, **kwargs):
        stats = self._stats_for()
        phase_name = _CURRENT_PHASE.get()
        stats.record_call(phase_name)
        try:
            async for txt in self._chat_mdl.async_chat_streamly(system, history, gen_conf or {}, **kwargs):
                yield txt
        except Exception:
            stats.record_failed(phase_name)
            raise
        finally:
            stats.record_usage(phase_name, _last_usage(self._chat_mdl))

    async def async_chat_streamly_delta(self, system: str, history: list, gen_conf: dict | None = None, **kwargs):
        stats = self._stats_for()
        phase_name = _CURRENT_PHASE.get()
        stats.record_call(phase_name)
        try:
            async for txt in self._chat_mdl.async_chat_streamly_delta(system, history, gen_conf or {}, **kwargs):
                yield txt
        except Exception:
            stats.record_failed(phase_name)
            raise
        finally:
            stats.record_usage(phase_name, _last_usage(self._chat_mdl))
