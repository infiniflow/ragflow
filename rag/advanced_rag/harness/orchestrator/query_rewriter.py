"""Query Rewriter

Query Rewriter has two jobs:
  - Phase 1: break the long request into simple, searchable questions so the
    retriever finds relevant content more accurately.
  - Phase 4: turn the Sufficient Context Agent's missing-pieces feedback into a
    NEW targeted search query (e.g. "you missed allergies -> search 'rashes' /
    'adverse events'").

This module implements the Phase 4 role for the orchestrator: it takes a
Sufficient Context Agent forward gap (what is missing + a search hint) and
rewrites it into a concrete, retrievable query. Unlike ``_try_replan``'s current
behavior — which reuses the gap text verbatim as a new claim description — this
produces a targeted query that actually names the missing entity + relation, so
the next search hits the gap instead of re-searching the same angle.

The rewriting is deliberately cheap and only invoked on replan (replan budget is
bounded), so it adds little latency.
"""

from __future__ import annotations

import logging

from rag.advanced_rag.harness.stats import in_phase
from rag.prompts.generator import PROMPT_JINJA_ENV, gen_json
from rag.prompts.template import load_prompt

_LOG = logging.getLogger(__name__)

REWRITE_PROMPT = load_prompt("sca_query_rewrite")


@in_phase("rewrite")
async def rewrite_gap_to_query(
    tools,
    question: str,
    gaps: list[tuple[str, str]],
    bridge_values: list | None = None,
    research_context: str = "",
) -> list[dict]:
    """Rewrite forward gaps into targeted search queries (multi-hop aware).

    Parameters
    ----------
    tools : RAGTools
        Must expose ``chat_mdl``.
    question : str
        The original user question (context for the rewrite).
    gaps : list[(what, search_hint)]
        Forward gaps (what the answer still needs, plus a search hint).
    bridge_values : list | None
        Already-resolved bridge values across the claims (e.g. the confirmed shows
        in a "which finale ran longest" question). The rewriter anchors the new
        search query to these, so a missing hop is re-searched with the resolved
        upstream value instead of re-deriving it.
    research_context : str
        Rendered block describing what previous rounds already tried (queries +
        outcomes) and what the current evidence pool roughly contains. Gives the
        rewriter enough information to aim at UNCOVERED angles instead of
        paraphrasing dead ones — the information-augmented counterpart of the
        react loop's in-context visibility.

    Returns
    -------
    list[dict] : ``[{"query": "..."}]`` — targeted, retrievable queries. Empty
        when the rewrite is unavailable / fails, in which case the caller falls
        back to the original gap text.
    """
    chat_mdl = getattr(tools, "chat_mdl", None)
    if chat_mdl is None or not gaps:
        return []
    gaps_text = "\n".join(f"- what: {g[0] or ''}; hint: {g[1] or ''}" for g in gaps)
    bridge_text = "\n".join(f"- {b}" for b in (bridge_values or []) if str(b).strip())
    rendered = PROMPT_JINJA_ENV.from_string(REWRITE_PROMPT).render(
        question=question or "",
        gaps=gaps_text,
        bridge_values=bridge_text,
        research_context=research_context,
    )
    try:
        result = await gen_json(rendered, "Output:\n", chat_mdl)
    except Exception as exc:  # noqa: BLE001
        _LOG.info("[QueryRewrite] failed: %s", exc)
        return []
    if not isinstance(result, dict):
        return []
    out = []
    for q in result.get("queries") or []:
        if isinstance(q, dict):
            query = str(q.get("query") or q.get("question") or "").strip()
        elif q:
            query = str(q).strip()
        else:
            continue
        if query and query not in out:
            out.append({"query": query})
    return out
