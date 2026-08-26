"""Thinking mode configurations.

medium/high/ultra all run the single-agent SCA graph (see
``agentic_rag_graph.build_agentic_graph``); the ONLY thing the ReAct/SCA path
consumes from the strategy is ``available_tools`` (via ``get_mode("medium")``)
and ``label``. low runs the lightweight ``direct_search`` graph, which reads
``execution_strategy`` (=="direct_search") and ``available_tools``. All other
fields are legacy data and never read by any running path.
"""

from rag.advanced_rag.harness.types import ExecutionStrategy

THINKING_MODES: dict[str, ExecutionStrategy] = {
    "low": ExecutionStrategy(
        label="low",
        execution_strategy="direct_search",
        available_tools=["hybrid_search", "web_search", "bm25_search"],
    ),
    "medium": ExecutionStrategy(
        label="medium",
        execution_strategy="direct_search",  # legacy; SCA path does not read it
        available_tools=["grep_search", "list_chunks", "web_search"],
    ),
    "high": ExecutionStrategy(
        label="high",
        execution_strategy="direct_search",  # legacy; SCA path does not read it
        available_tools=["grep_search", "list_chunks", "web_search"],
    ),
    "ultra": ExecutionStrategy(
        label="ultra",
        execution_strategy="direct_search",  # legacy; SCA path does not read it
        available_tools=["grep_search", "list_chunks", "web_search"],
    ),
}


def get_mode(label: str) -> ExecutionStrategy:
    mode = THINKING_MODES.get(label)
    if not mode:
        raise ValueError(f"Unknown thinking mode: {label}. Available: {list(THINKING_MODES.keys())}")
    return mode
