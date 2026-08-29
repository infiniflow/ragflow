"""Thinking-mode configuration: the single authority for mode behaviour.

Every mode-dependent decision in the harness reads from this module instead of
re-deriving it from a ``thinking_mode`` string. The decisions that used to be
hard-coded in four places are:

=====================  ==========================================  =========================
decision                previously hard-coded in                    now
=====================  ==========================================  =========================
graph + SCA + fanout   ``agentic_rag_graph.run_agentic_rag``       ``ModeSpec.graph`` /
                                                                   ``enable_sca`` /
                                                                   ``use_fanout``
SCA iteration budget   ``agentic_rag_graph.build_agentic_graph``   ``sca_max_rounds``
tool surface           ``action_session._active_tool_specs``       ``tools``
session turn budget    ``action_session._action_max_turns``        ``action_max_turns``
=====================  ==========================================  =========================

The values below reproduce the previous behaviour exactly, so this is a
structural change only — tune a mode by editing one table.

Unknown labels: ``resolve_mode`` falls back to ``NAIVE`` rather than raising.
The mode comes from user input at request time, and raising would fail the whole
request; a naive (non-agentic) answer degrades gracefully instead.
"""

from dataclasses import dataclass, field

# Tools the action session can bind, in the order they are declared. Kept here
# so a mode's tool set is data, not an if-chain in the runtime.
_ALL_TOOLS = (
    "retrieve",
    "search_chunks",
    "list_chunks",
    "navigate_tree",
    "navigate_structure",
    "calculate",
    "web_search",
)
# Relational exploration is the extra depth reserved for ultra.
_GRAPH_EXPLORE = "graph_explore"


@dataclass(frozen=True)
class ModeSpec:
    """Everything that varies between thinking modes.

    :param label: the mode's own name.
    :param agentic: run the agentic graph. When False the caller must answer
        without it (naive retrieval) — see :data:`NAIVE`.
    :param enable_sca: run the sufficiency-check review loop.
    :param sca_max_rounds: SCA→rewriter iteration budget (0 when disabled).
    :param use_fanout: planner + prefetch decomposition (multi-slot research).
    :param action_max_turns: turns inside one action session.
    :param tools: tool names visible to the model in this mode. Empty means the
        model gets no tool loop at all.
    """

    label: str
    agentic: bool = True
    enable_sca: bool = True
    sca_max_rounds: int = 3
    use_fanout: bool = False
    action_max_turns: int = 4
    tools: frozenset[str] = field(default_factory=lambda: frozenset(_ALL_TOOLS))


def _tools(*names: str) -> frozenset[str]:
    return frozenset(names)


THINKING_MODES: dict[str, ModeSpec] = {
    # low: one hybrid-search pass through ``direct_search``. No action session,
    # so no tool loop — the model never sees tools in this mode.
    "low": ModeSpec(
        label="low",
        agentic=False,
        enable_sca=False,
        sca_max_rounds=0,
        use_fanout=False,
        action_max_turns=4,
        tools=frozenset(),
    ),
    # medium: agentic, SCA review on, no planner decomposition.
    "medium": ModeSpec(
        label="medium",
        action_max_turns=4,
        sca_max_rounds=3,
        use_fanout=False,
        tools=_tools(*_ALL_TOOLS),
    ),
    # high: adds planner + prefetch fan-out over the same tool surface.
    "high": ModeSpec(
        label="high",
        action_max_turns=4,
        sca_max_rounds=3,
        use_fanout=True,
        tools=_tools(*_ALL_TOOLS),
    ),
    # ultra: deeper sessions, more SCA rounds, and the relational tool.
    "ultra": ModeSpec(
        label="ultra",
        action_max_turns=6,
        sca_max_rounds=5,
        use_fanout=True,
        tools=_tools(*_ALL_TOOLS, _GRAPH_EXPLORE),
    ),
}

#: Fallback for an unrecognised mode label. Not agentic — the caller answers
#: with plain retrieval rather than failing the request.
NAIVE = ModeSpec(
    label="naive",
    agentic=False,
    enable_sca=False,
    sca_max_rounds=0,
    use_fanout=False,
    action_max_turns=4,
    tools=frozenset(),
)


def get_mode(label: str) -> ModeSpec:
    """Return the spec for ``label``.

    Unknown labels fall back to :data:`NAIVE` (non-agentic) — the label arrives
    from user input, and raising here would fail the request outright.
    """
    return THINKING_MODES.get(str(label or "").strip().lower(), NAIVE)


def resolve_mode(tools) -> ModeSpec:
    """Read a ``RAGTools``-like object's ``thinking_mode`` into its spec."""
    return get_mode(getattr(tools, "thinking_mode", ""))
