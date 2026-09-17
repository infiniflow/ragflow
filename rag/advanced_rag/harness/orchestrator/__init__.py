"""Orchestration strategies for the agentic graph.

Each module here is one strategy the graph can run; they are imported lazily by
``agentic_rag_graph`` at the point of use:

* :mod:`direct` — single-pass retrieval (the low path).
* :mod:`sufficient_context` — sufficiency review + gap-driven rewrites.
* :mod:`query_rewriter` — turns a reported gap into a follow-up query.

Which strategies a mode runs is decided in ``harness.config.THINKING_MODES``, not
here — see ``ModeSpec.enable_sca`` / ``use_fanout``.
"""
