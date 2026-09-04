#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""LangGraph agentic-search graph — Agentic RAG alignment.

Agentic RAG sequence this graph implements:

    Question
       │
       ▼
    Planner Agent                       (Phase 1 — fan-out expansion)
       │ query fanouts
       ▼
    RAG Agent / Search Fanout           (Phase 2 — ALL fanouts researched
       │                                  AT ONCE: one programmatic
       ▼                                  pre-fetch + one native tool-calling
    Search results + drafts               research pass whose first instruction
       │                                  carries the full fan-out checklist)
       ▼
    Sufficient Context Agent            (Phase 3 — reviews ORIGINAL question +
       │ suff / insuff                    retrieved snippets + intermediate draft,
       ▼                                  emits missing-pieces Reason/Feedback)
    Synthesis   Gap analisis + feedback
                   │                     (Phase 4 — Query Rewriter targets the
                   ▼                      gap, pre-fetches new evidence into
                RAG Agent                kbinfos AND hands the same requests to
       (loop until sufficient            the RAG Agent for a fresh research pass)
        or budget exhausted)
       │                                 (Phase 5 — clean synthesis of the final
       ▼                                  answer from the approved draft + snippets)
    Synthesis Agent

All five phases live in ONE compiled graph for medium / high / ultra
thinking modes (``build_agentic_graph``). ``low`` keeps the lightweight
direct-search graph (``build_low_graph``).
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
import time
from typing import Annotated, Any, TypedDict

from langgraph.graph import END, START, StateGraph
from langgraph.graph.message import add_messages

from rag.advanced_rag.harness.config import NAIVE, resolve_mode
from rag.advanced_rag.harness.stats import in_phase
from rag.prompts.generator import form_message, kb_prompt, message_fit_in

_LOG = logging.getLogger(__name__)

# ── Global research budget & per-call timeouts (source-level switches) ──
#
# The benchmark client cuts a request at 300s (read timeout). These bounds keep
# one question's WHOLE pipeline comfortably under that line: when the budget
# runs out the routing guards steer the graph straight to synthesis with
# whatever evidence is on hand instead of starting another research round.
_TOTAL_BUDGET_S = 180.0  # whole-graph wall-clock ceiling per question
_MIN_ROUND_HEADROOM_S = 50.0  # need at least this much left to start a new round
_PASS_TIMEOUT_S = 120.0  # slot research pass wall-clock
_PREFETCH_TIMEOUT_S = 90.0  # programmatic fan-out fetch
_DRAFT_TIMEOUT_S = 60.0  # fallback draft synthesis
_SCA_TIMEOUT_S = 60.0  # sufficient-context review call
_REWRITE_TIMEOUT_S = 45.0  # gap → query rewrite call


def _snip(value: Any, limit: int = 240) -> str:
    try:
        s = value if isinstance(value, str) else json.dumps(value, ensure_ascii=False, default=str)
    except Exception:  # noqa: BLE001
        s = str(value)
    s = " ".join(s.split())
    if len(s) > limit:
        s = s[:limit] + f"...(+{len(s) - limit} chars)"
    return s


def _safe_list(value, what: str) -> list:
    """Coerce a possibly-poisoned graph-state field into a plain list.

    A 'coroutine' object has been observed reaching node payloads through
    graph-state fields under production concurrency. Instead of crashing the
    whole question three nodes later with an opaque iterable error, log WHO
    created it — a coroutine's ``repr`` contains its function name — and
    degrade to empty.
    """
    if value is None:
        return []
    r = repr(value)[:200]
    if isinstance(value, (list, tuple)):
        return list(value)
    if asyncio.iscoroutine(value):
        _LOG.error("[StateGuard] %s is a raw COROUTINE — creator: %s", what, r)
    else:
        _LOG.warning("[StateGuard] %s has unexpected type %s (%s); treating as empty", what, type(value).__name__, r)
    return []


def _is_poisoned(value) -> bool:
    """True when a state value should be discarded before it poisons LLM prompts."""
    return asyncio.iscoroutine(value)


def _select_sca_view(chunks: list, focus_terms: list[str], cap: int | None = None) -> tuple[list, str]:
    """Rank the stored pool down to the SCA review view (storage ≠ review).

    Score = retrieval relevance (similarity) + surface-term coverage against
    the question + current gap queries + freshness bonus for chunks admitted
    in later rounds. Returns ``(view, identity)`` where ``identity`` is a
    stable hash of the selected chunk ids — the caller uses it to detect an
    unproductive round (same view twice despite new storage ⇒ nothing new to
    look at).
    """
    from rag.advanced_rag.harness.tools.search import _chunk_id

    capped = cap or _SCA_VIEW_CAP
    terms = [t.lower() for t in dict.fromkeys(focus_terms or []) if len(t) >= 3]

    def _score(i: int, c: dict) -> float:
        text = " ".join(str(c.get(k, "")) for k in ("content", "content_with_weight", "title", "question_toks")).lower()
        cov = sum(1 for t in terms if t in text)
        cov_ratio = cov / len(terms) if terms else 0.5
        rel = float(c.get("similarity") or c.get("score") or 0.0)
        fresh = min(i / 20.0, 0.2)  # late arrivals (gap-pursuit evidence) get seen
        return rel * 0.45 + min(cov_ratio, 1.0) * 0.45 + fresh

    ranked = sorted(list(enumerate(chunks)), key=lambda ic: _score(*ic), reverse=True)  # noqa: C414
    view = [c for _, c in ranked[:capped]]
    ident = "|".join(sorted((_chunk_id(c) or "") for c in view))
    return view, str(hash(ident))


def _view_terms(state: AgenticState) -> list[str]:
    """Terms describing what the SCA should look FOR this round."""
    from rag.advanced_rag.harness.tools.search import _query_to_terms

    terms = _query_to_terms(state.get("question") or "")
    for q in _safe_list(state.get("current_queries"), "state.current_queries"):
        if isinstance(q, str):
            terms.extend(_query_to_terms(q))
    return list(dict.fromkeys(terms))


def _remaining_s(state: dict) -> float:
    """Seconds left in the question's global research budget (huge if unset)."""
    dl = state.get("deadline") or 0.0
    return dl - time.monotonic() if dl else float("inf")


async def _bounded(coro, timeout_s: float, what: str):
    """Await ``coro`` under a wall-clock bound; log & surface None on expiry."""
    try:
        async with asyncio.timeout(timeout_s):
            return await coro
    except TimeoutError:
        _LOG.warning("[Budget] %s exceeded %.0fs — moving on without it", what, timeout_s)
        return None


class AgenticState(TypedDict, total=False):
    # ── conversation input ──
    messages: list  # raw conversation; read once by formalize_question
    question: str
    keywords: str  # search keywords + close synonyms for the formalized question

    # ── evolving research state ──
    plan: dict  # {"fanouts": [...] } planner output (Phase 1)
    current_queries: list  # active research targets (fanouts, then rewritten gap queries)

    # outer draft/SCA/rewrite stages can validate and close unresolved slots.
    # The shared slot table is held as the ``State`` object itself. The graph is
    # compiled without a checkpointer, so graph state never needs to be
    # JSON-serialized — keeping the object avoids carrying a parallel dict
    # representation of the same data.
    slot_table: object  # action_session.State (None until the planner runs)
    slot_draft: str  # slot-rendered fact draft fed to the SCA (structured view)
    collected_answer: str  # non-terminal <answer> candidate surfaced for SCA validation
    unresolved_slots: list  # [{id,type,question_clues,discovered_clues}] still unfilled after the tree round
    # Append-only accumulation of Phase-4 feedback instructions handed to the
    # RAG Agent between passes. Annotated with ``add_messages`` so each
    # query_rewrite invocation may return JUST its new instruction(s); the seed
    # instruction itself is deterministic (question + fan-outs) and therefore
    # rebuilt per pass rather than round-tripped through the reducer.
    research_feedback: Annotated[list, add_messages]
    kbinfos: dict  # accumulated chunks & doc_aggs (+ "pre_summary" after the draft node)
    draft: str  # intermediate fact-preserving draft (Phase 3 reviewee)
    rag_answer: str  # latest RAG-agent research report
    partial_answer: bool
    abstain: bool
    empty_result: bool
    verdict: dict  # sufficiency verdict as a plain dict (status/score/gaps/...)
    sca: dict  # raw SCA payload (its gaps feed the Query Rewriter)

    # ── budgets & counters ──
    max_loops: int
    deadline: float  # time.monotonic() stamp when research budget expires
    search_rounds: int  # completed SCA→query_rewrite iterations
    sca_view_id: str  # identity hash of the last SCA review view (unproductive-round detector)
    attempted: list  # [{"q": query, "r": round, "new": n_new}] — every issued search + outcome (rewriter context)
    fills_found: bool  # research-tree mode: at least one slot got a confident fill
    no_progress: bool  # last iteration produced neither gaps nor queries


# ── Think tag helpers ──

_THINK_OPEN = "<think>"
_THINK_CLOSE = "</think>"


def _partial_tag_tail(s: str, tag: str) -> int:
    for k in range(min(len(s), len(tag) - 1), 0, -1):
        if s.endswith(tag[:k]):
            return k
    return 0


async def _split_think_stream(stream):
    """Split model deltas into ``think`` and ``answer`` text.

    Besides ordinary ``<think>...</think>`` streams, some providers emit the
    opening tag only on the first reasoning delta and append ``</think>`` to
    every subsequent delta. An unmatched closing tag therefore still marks
    the text before it as reasoning.
    """
    buf = ""
    in_think = False

    async for token in stream:
        if not isinstance(token, str):
            _LOG.warning("Ignoring non-string agentic RAG stream item of type %s", type(token).__name__)
            continue
        buf += token

        while buf:
            if in_think:
                close_idx = buf.find(_THINK_CLOSE)
                if close_idx >= 0:
                    if close_idx:
                        yield "think", buf[:close_idx]
                    buf = buf[close_idx + len(_THINK_CLOSE) :]
                    in_think = False
                    continue

                hold = _partial_tag_tail(buf, _THINK_CLOSE)
                safe = buf[: len(buf) - hold] if hold else buf
                if safe:
                    yield "think", safe
                buf = buf[len(buf) - hold :] if hold else ""
                break

            open_idx = buf.find(_THINK_OPEN)
            close_idx = buf.find(_THINK_CLOSE)

            if close_idx >= 0 and (open_idx < 0 or close_idx < open_idx):
                if close_idx:
                    yield "think", buf[:close_idx]
                buf = buf[close_idx + len(_THINK_CLOSE) :]
                continue

            if open_idx >= 0:
                if open_idx:
                    yield "answer", buf[:open_idx]
                buf = buf[open_idx + len(_THINK_OPEN) :]
                in_think = True
                continue

            hold = max(_partial_tag_tail(buf, _THINK_OPEN), _partial_tag_tail(buf, _THINK_CLOSE))
            safe = buf[: len(buf) - hold] if hold else buf
            if safe:
                yield "answer", safe
            buf = buf[len(buf) - hold :] if hold else ""
            break

    if buf:
        yield ("think" if in_think else "answer"), re.sub(r"</?think>", "", buf)


# ── Agentic RAG helpers ──


def _sca_gaps_to_rewrite(sca: dict) -> list[tuple[str, str]]:
    """Extract the Query-Rewriter gaps from a Sufficient Context Agent verdict.

    Preference order:
    1. Unsatisfied ``sub_queries`` (the precise "what is missing / where to
       search next" signal from Q-CARE) — ``(missing_fact, search_hint)``.
    2. Per-claim ``missing_information`` items — ``(what, search_hint)``.

    Returns ``[(what, search_hint), ...]`` (deduped, non-empty). Empty when the
    verdict carries no concrete gap (caller then accepts the draft).
    """
    gaps: list[tuple[str, str]] = []
    seen: set[str] = set()

    def _add(what: str, hint: str) -> None:
        what = str(what or "").strip()
        hint = str(hint or "").strip()
        if not what and not hint:
            return
        key = f"{what}|{hint}"
        if key in seen:
            return
        seen.add(key)
        gaps.append((what or hint, hint or what))

    for sq in sca.get("sub_queries") or []:
        if not isinstance(sq, dict):
            continue
        if sq.get("satisfied"):
            continue
        _add(sq.get("missing_fact") or sq.get("sub_query") or "", sq.get("search_hint") or "")
    if gaps:
        return gaps[:8]
    for g in (sca.get("claims") or {}).values():
        for mi in g.get("missing_information") or []:
            if isinstance(mi, dict):
                _add(mi.get("what") or "", mi.get("search_hint") or "")
            elif mi:
                _add(str(mi), "")
    return gaps[:8]


_FANOUT_PROMPT = (
    "Break the user's question into 2 to 5 independent, directly searchable sub-questions "
    "(fan-outs). Each must be self-contained enough to retrieve relevant passages from a "
    "document corpus on its own. For multi-hop questions, produce ONLY the first-hop "
    "sub-questions needed to start (the anchor facts); do not invent downstream hops that "
    "depend on answers you do not have yet.\n"
    'Respond with a JSON object: {"fanouts": ["...", "..."]}. No prose, JSON only.'
)


def _extract_json_object(text: str):
    """Return the first parseable JSON object found in ``text``.

    The fan-out model sometimes emits prose before/after the object or even a second
    JSON object; a greedy brace-match then captures multiple objects and ``json.loads``
    fails with "Extra data". We brace-match to isolate the FIRST complete object and
    only return it if it parses; otherwise try the next opening brace.
    Returns ``None`` when no object parses.
    """
    i = 0
    n = len(text)
    while i < n:
        start = text.find("{", i)
        if start < 0:
            return None
        depth = 0
        for j in range(start, n):
            ch = text[j]
            if ch == "{":
                depth += 1
            elif ch == "}":
                depth -= 1
                if depth == 0:
                    candidate = text[start : j + 1]
                    try:
                        return json.loads(candidate)
                    except Exception:  # noqa: BLE001
                        break  # not a valid object; try the next "{"
        i = start + 1
    return None


async def _expand_fanouts(tools, question: str, answer_conf: dict) -> list[str]:
    """Phase-1 fan-out expansion: break the question into searchable sub-questions.

    Uses ONE chat call (no tools). Returns 2-5 first-hop fan-outs; falls back to the
    raw question alone on any failure — a fan-out failure never blocks the pipeline.
    """
    try:
        from rag.advanced_rag.harness.tools.search import _base_chat_mdl
    except Exception:  # noqa: BLE001
        _LOG.warning("[rag_agent] could not import _base_chat_mdl for fan-out expansion", exc_info=True)
        return [question] if question else []
    try:
        mdl = _base_chat_mdl(tools)
    except Exception:  # noqa: BLE001
        _LOG.warning("[rag_agent] could not resolve base chat model for fan-out expansion", exc_info=True)
        return [question] if question else []
    if mdl is None or not question:
        return [question] if question else []
    try:
        ans, _ = await mdl.async_chat(
            _FANOUT_PROMPT,
            [{"role": "user", "content": f"Question: {question}"}],
            dict(answer_conf or {}),
        )
        text = str(ans or "")
        data = _extract_json_object(text)
        if data is not None:
            fanouts = [str(f).strip() for f in (data.get("fanouts") or []) if str(f).strip()]
        else:
            # Fallback: split on line items from a loose answer.
            fanouts = [ln.strip("-•0123456789. ").strip() for ln in text.splitlines() if ln.strip()]
        fanouts = list(dict.fromkeys(fanouts))[:5]
        _LOG.info("[Planner] fan-out expansion: %d sub-question(s): %s", len(fanouts), fanouts)
        return fanouts or ([question] if question else [])
    except Exception:  # noqa: BLE001
        _LOG.warning("[Planner] fan-out expansion failed; falling back to raw question", exc_info=True)
        return [question] if question else []


# Storage ceiling of the snippet pool across ALL rounds. Storage and REVIEW
# are decoupled: the SCA only reads a ranked view (``_SCA_VIEW_CAP``), so the
# pool may accumulate freely while prompts stay bounded. The former single
# 30-chunk ceiling starved every enrichment channel dead (observed: Drill
# admitted 0 for entire runs because prefetch filled the pool first).
_MAX_SNIPPET_POOL = 60
_DRILL_RESERVE = 12  # slots kept free after the FIRST prefetch so the research
#                       executor can top up evidence
_SCA_VIEW_CAP = 60  # chunks shown to the Sufficient Context Agent per review (24 -> 60: 24 of 225 hid the answer-bearing table chunk from the SCA)


async def _fanout_search(tools, fanouts: list[str], top_n: int = 8, capacity: int | None = None) -> int:
    """Programmatic fan-out retrieval: fetch all queries and fill ``kbinfos``.

    Phase 2 ("The RAG Agent searches ... all the query fanouts at once")
    for the SNIPPET pool the Sufficient Context Agent reviews (Phase 3 reads
    actual retrieved text). Two collector channels run per query:

    * Channel A (BM25 + ``narrow_by_terms``): exact-name recall, rewritten to a
      short match window.
    * Channel B (hybrid vector+BM25, **narrow bypass**): semantic hits whose
      text does NOT share surface words with the query. The 2026-08-27 hybrid
      experiment proved these blocks are valuable but are ALL filtered out by the keyword narrow step while
      crowding candidates out of top_n — so they now bypass it entirely.

    Called after planning and again after every Query-Rewriter pass. Returns
    how many NEW chunks were added; failures are swallowed per-query.
    """
    try:
        from rag.advanced_rag.harness.grep_sed_narrow import narrow_by_terms
        from rag.advanced_rag.harness.memory import _STOPWORDS as _FANOUT_STOPWORDS
        from rag.advanced_rag.harness.tools.search import (
            _chunk_id,
            _get_kb_ids,
            _query_to_terms,
            bm25_search,
            hybrid_search,
        )
    except Exception:  # noqa: BLE001
        _LOG.warning("[rag_agent] could not import fan-out search helpers", exc_info=True)
        return 0

    kbinfos = getattr(tools, "kbinfos", None)
    if kbinfos is None:
        kbinfos = {"chunks": [], "doc_aggs": []}
        tools.kbinfos = kbinfos
    seen = {_chunk_id(c) for c in kbinfos.get("chunks", [])}
    kb_ids = _get_kb_ids(tools) or None

    # Input-shape guard: this node runs several graph rounds in, fed from
    # planner output AND rewriter output. A malformed upstream value must never
    # crash the whole question (observed: a 'coroutine' object is not iterable
    # here when a stale byte-code path slipped a coroutine-shaped argument in).
    if isinstance(fanouts, str):
        fanouts = [fanouts]
    elif fanouts is None:
        return 0
    elif isinstance(fanouts, (list, tuple, set)):
        try:
            fanouts = [f for f in fanouts if isinstance(f, str)]
        except Exception:  # noqa: BLE001
            return 0
    else:
        _LOG.warning("[Prefetch] unexpected queries payload of type %s; skipping fan-out search", type(fanouts).__name__)
        return 0

    async def _search_one(fq: str) -> tuple[list, list]:
        # Each coroutine ONLY retrieves and returns chunk lists; it does NOT
        # touch ``kbinfos``. All mutation happens in the main coroutine below,
        # so concurrent fan-out searches cannot race on the shared list.
        # Channel A: exact-surface collector.
        #
        # Feed the BM25 candidate pool the CONCISE entity terms, not just the full
        # sentence: a long question buries the discriminating entities (e.g.
        # "Culdect Saga") under stopwords, so generic docs outrank the specific
        # one and the proper-noun chunk never even enters the pool. Passing the
        # keyed terms as ``keywords`` makes bm25_search build
        # ``effective_query = "sentence + entity terms"`` so the discriminating
        # noun gets its own score mass.
        terms = _query_to_terms(fq)
        keyed = [t for t in terms if len(t) >= 3 and t.lower() not in _FANOUT_STOPWORDS and not t.isdigit() or (len(t) >= 4 and t.isdigit())]
        try:
            res = await bm25_search(tools, fq, kb_ids=kb_ids, top_n=60, keywords=" ".join(keyed or terms))
            candidates = res.get("chunks", []) or []
        except Exception:  # noqa: BLE001
            _LOG.warning("[rag_agent] BM25 search failed for %r", fq, exc_info=True)
            candidates = []

        kept_a: list = []
        if candidates:
            try:
                narrowed = narrow_by_terms(
                    candidates,
                    keyed or terms,
                    fallback_terms=None,
                    context={"before": 0, "after": 1},
                    keywords=fq,
                    max_out_chars_per_chunk=1200,
                    max_out_total_chars=16000,
                )
                kept_a = (narrowed.get("kept", []) or [])[: max(1, top_n)]
            except Exception:  # noqa: BLE001
                _LOG.warning("[rag_agent] narrowing failed for %r; using raw BM25 head", fq, exc_info=True)
                kept_a = candidates[: max(1, top_n)]

        # Channel B: semantic collector, narrow BYPASS. Keep only hits that
        # Channel A did not already surface (dedup happens again at merge).
        kept_b: list = []
        seen_ids_a = {_chunk_id(c) for c in kept_a}
        try:
            hres = await hybrid_search(tools, fq, kb_ids=kb_ids, top_n=30)
            for c in hres.get("chunks", []) or []:
                if _chunk_id(c) in seen_ids_a:
                    continue
                kept_b.append(c)
                if len(kept_b) >= 4:  # modest semantic quota per query
                    break
        except Exception:  # noqa: BLE001
            _LOG.warning("[rag_agent] hybrid channel failed for %r; skipping", fq, exc_info=True)
        return kept_a, kept_b

    # Search all fan-outs at once (parallel) — then merge into the shared pool.
    results = await asyncio.gather(*(_search_one(fq) for fq in fanouts), return_exceptions=True)

    # Cap the TOTAL evidence pool so the SCA never has to read unbounded chunks.
    # This is a GLOBAL ceiling across all rounds (planning prefetch + every
    # Query-Rewriter top-up) — a per-call cap would let the pool grow with each
    # insufficient round and bloat every subsequent SCA prompt (observed: 113
    # chunks after round 3). Once the pool is full, later calls add nothing and
    # the caller's zero-new-chunks early-exit can kick in.
    max_total = capacity or _MAX_SNIPPET_POOL
    room = max(0, max_total - len(seen))
    added = 0

    def _admit(batch) -> bool:
        nonlocal added
        if isinstance(batch, Exception) or not batch:
            return False
        stop = False
        for c in batch:
            if added >= room:
                stop = True
                break
            k = _chunk_id(c)
            if k and k in seen:
                continue
            if k:
                seen.add(k)
            kbinfos.setdefault("chunks", []).append(c)
            added += 1
        return stop

    # Channel A first (exact matches earn their slots), then semantic extras.
    for pair in results:
        if isinstance(pair, Exception):
            continue
        kept_a, kept_b = pair if isinstance(pair, tuple) else ([], [])
        if _admit(kept_a):
            break
    for pair in results:
        if isinstance(pair, Exception):
            continue
        _, kept_b = pair if isinstance(pair, tuple) else ([], [])
        if _admit(kept_b):
            break

    if room == 0:
        _LOG.info("[Prefetch] snippet pool FULL (%d chunks); nothing new admitted", max_total)
        return added
    _LOG.info("[Prefetch] fan-out channels added %d new chunk(s) (total %d)", added, len(kbinfos.get("chunks", [])))
    return added


# Reused by build_low_graph — lightweight direct search node.
async def _direct_search_node_impl(state: AgenticState, tools) -> dict:
    from rag.advanced_rag.harness.orchestrator.direct import direct_search

    return await direct_search(state, tools)


# ── Shared final-answer composition (Phase 5 Synthesis) ──


async def _compose_answer_from_evidence(state: AgenticState, tools, token_queue: asyncio.Queue, answer_conf: dict) -> dict:
    """Compose the final grounded answer from the gathered evidence."""
    kbinfos = state.get("kbinfos") or {"chunks": [], "doc_aggs": []}
    question = state.get("question") or ""
    partial = state.get("partial_answer", False)
    abstain = state.get("abstain", False)
    empty_result = state.get("empty_result", False)

    _note = " — partial answer, some gaps remain" if partial else (" — not enough evidence to answer" if abstain else "")
    _LOG.info('[Composing the answer] Writing the final answer to "%s" from %d gathered passage(s)%s.', _snip(question), len(kbinfos["chunks"]), _note)

    tools.kbinfos = kbinfos

    no_evidence = abstain or empty_result or not kbinfos["chunks"]
    if no_evidence and getattr(tools, "empty_response", ""):
        _LOG.info("[Composing the answer] No supporting evidence was found; returning the configured empty response without calling the answer model.")
        token_queue.put_nowait(tools.empty_response)
        return {"final_answer": tools.empty_response}

    # Primary evidence is the fact-preserving ``pre_summary`` (the intermediate
    # draft reviewed by the SCA — it carries the exact numbers/entities). A small
    # set of top-similarity chunks stays as citation reference / fallback detail.
    pre_summary = kbinfos.get("pre_summary")
    all_chunks = kbinfos.get("chunks") or []
    ranked = sorted(
        (c for c in all_chunks),
        key=lambda c: float(c.get("similarity", 0.0) or c.get("score", 0.0) or 0.0),
        reverse=True,
    )
    from rag.advanced_rag.agentic_rag import _EVIDENCE_BUDGET_TOKENS

    _CITE_CHUNK_CAP = 6
    cite_chunks = ranked[:_CITE_CHUNK_CAP] or all_chunks
    evidence_kbinfos = dict(kbinfos, chunks=cite_chunks)
    evidence_blocks = kb_prompt(evidence_kbinfos, min(tools.chat_mdl.max_length, _EVIDENCE_BUDGET_TOKENS))
    evidence = "\n".join(evidence_blocks) if isinstance(evidence_blocks, list) else str(evidence_blocks)

    parts = [f"Question:\n{question}\n"]

    # Static answer-target guardrail (no extra LLM call). Keeps the essential
    # instruction: satisfy the top-level request, treat bridge entities as clues.
    parts.append(
        "Answer Target Contract:\n"
        "Final answer must directly satisfy the user's top-level who/what request. "
        "Use bridge entities only as clues, and verify any proposed answer against "
        "the evidence. In EXTREME-SELECTION questions (shortest/longest/smallest/"
        "largest/most/least/最), compare the alternatives in the evidence and name "
        "the EXTREME one rather than the most common or first-listed.\n"
    )

    if no_evidence:
        if pre_summary:
            parts.append(
                "The retrieved passages are limited. Answer as completely as possible "
                "from the Research Summary below, using the known facts; where a "
                "specific number/entity is missing, say what is known and avoid "
                "flatly refusing to answer.\n"
            )
        else:
            parts.append("No supporting evidence was retrieved. State clearly that the available sources are insufficient, and do not answer from general knowledge.\n")

    if pre_summary:
        parts.append(f"Research Summary (primary evidence):\n{pre_summary}\n")

    if partial:
        from rag.advanced_rag.harness.prompts.report_prompt import PARTIAL_ANSWER_PREAMBLE

        parts.append(f"{PARTIAL_ANSWER_PREAMBLE}\n")

    from rag.advanced_rag.harness.prompts.report_prompt import FINAL_ANSWER_SYSTEM
    from rag.prompts.generator import citation_prompt as cp

    rules = cp(tools.user_defined_prompts).strip()
    system = FINAL_ANSWER_SYSTEM.format(cite_rules=rules)
    # Honor the dialog-level system prompt (UI-configured) the same way the
    # reasoning-disabled path does — merge from upstream/main.
    #
    # The configured prompt is appended AFTER the agentic contract so that
    # presentational instructions actually take effect: FINAL_ANSWER_SYSTEM ends
    # with a "# Language" rule ("answer in the same language as the question"),
    # and when the configured prompt was prepended instead, that rule won on
    # later-write-priority and silently overrode "answer in English", "be
    # concise" and similar settings. Behaviour then differed from the
    # reasoning-disabled path, where the same prompt is the only system prompt.
    #
    # Appending alone would let a configured prompt break the contract, so the
    # closing clause re-states precedence. Note the split: LANGUAGE IS DELIBERATELY
    # LEFT OVERRIDABLE — "answer in the same language as the question" is exactly
    # the rule a user setting "answer in English" means to replace, so it must not
    # be listed as protected. What stays protected is the evidence contract:
    # citing sources, answering the exact attribute asked for, and never
    # substituting prior knowledge for missing evidence.
    if getattr(tools, "system_prompt", "").strip():
        system = (
            f"{system}\n\n"
            "# Assistant configuration (set by the user)\n"
            f"{tools.system_prompt.strip()}\n\n"
            "Follow the configuration above for language, tone, style, format and "
            "any other presentational instruction, including where it overrides "
            "the language rule above. Where it conflicts with the citation rules, "
            "attribute fidelity, or the requirement to answer only from the "
            "provided evidence, those three take precedence."
        )

    parts.append(f"Evidence:\n{evidence}")
    user_content = "\n".join(parts)

    _LOG.info(
        "[Formalize][pre_summary] question=%r pre_summary_len=%d evidence_len=%d\npre_summary=%r",
        question[:160],
        len(pre_summary or ""),
        len(evidence or ""),
        (pre_summary or "")[:3000],
    )

    _, msg = message_fit_in(form_message(system, user_content), min(tools.chat_mdl.max_length, _EVIDENCE_BUDGET_TOKENS))
    try:
        async for tok in tools.chat_mdl.async_chat_streamly_delta(msg[0]["content"], msg[1:], answer_conf):
            token_queue.put_nowait(tok)
    except Exception:  # noqa: BLE001
        _LOG.exception("formalize_answer: stream failed")
        token_queue.put_nowait("I'm sorry, I encountered an error while composing the answer.")

    return {"final_answer": ""}


# ── Graph construction ──


def build_low_graph(tools, token_queue: asyncio.Queue, gen_conf: dict | None = None):
    """Compile the lightweight low-mode graph: formalize → direct_search → answer."""
    answer_conf = dict(gen_conf) if gen_conf else {"temperature": 0.3}

    @in_phase("formalize")
    async def formalize_question(state: AgenticState) -> dict:
        msgs = state.get("messages") or []
        _LOG.info("[Formalizing the question] Reading the conversation (%d message(s))...", len(msgs))
        q, kw = await tools.formalize(msgs)
        q = (q or "").strip()
        kw = (kw or "").strip()
        return {
            "question": q,
            "keywords": kw,
            "kbinfos": {"chunks": [], "doc_aggs": []},
            "partial_answer": False,
            "abstain": False,
        }

    @in_phase("orchestrator")
    async def direct_search_node(state: AgenticState) -> dict:
        return await _direct_search_node_impl(state, tools)

    @in_phase("finalize")
    async def formalize_answer(state: AgenticState) -> dict:
        return await _compose_answer_from_evidence(state, tools, token_queue, answer_conf)

    g = StateGraph(AgenticState)
    g.add_node("formalize_question", formalize_question)
    g.add_node("direct_search", direct_search_node)
    g.add_node("formalize_answer", formalize_answer)

    g.add_edge(START, "formalize_question")
    g.add_edge("formalize_question", "direct_search")
    g.add_edge("direct_search", "formalize_answer")
    g.add_edge("formalize_answer", END)

    return g.compile()


def build_agentic_graph(
    tools,
    token_queue: asyncio.Queue,
    gen_conf: dict | None = None,
    enable_sca: bool = False,
    use_fanout: bool = False,
):
    """Compile the slot-table-driven agentic-search graph (medium / high / ultra).

    * ``use_fanout`` adds Phase 1 (planner) + programmatic pre-fetch.
    * ``enable_sca`` wraps the research in the Phase 3-4 SCA↔Query-Rewriter
      iteration loop (bounded by ``_SCA_MAX_ROUNDS``).
    * The RAG-Agent research executor is slot-table-driven: the planner builds a
      slot table, and rag_agent runs one action_session per unresolved slot.

    Structure::

        formalize_question → [planner → prefetch] → rag_agent → draft → sca
            ├─ sufficient ──────────────────────────────→ formalize_answer
            └─ insufficient → query_rewrite ────────────→ rag_agent (next round)
    """
    answer_conf = dict(gen_conf) if gen_conf else {"temperature": 0.3}
    # SCA iteration budget (Phase 4 loops). ultra is intentionally deeper:
    # more research rounds before giving up, at the cost of latency — this is
    # one of the two differentiators from high (the other is the ultra-only
    # graph_explore relational tool). high/medium stay at 3.
    sca_max_rounds = resolve_mode(tools).sca_max_rounds

    # ── Node: formalize_question ──
    @in_phase("formalize")
    async def formalize_question(state: AgenticState) -> dict:
        msgs = state.get("messages") or []
        _LOG.info("[Formalizing the question] Reading the conversation (%d message(s))...", len(msgs))
        q, kw = await tools.formalize(msgs)
        q = (q or "").strip()
        kw = (kw or "").strip()
        return {
            "question": q,
            "keywords": kw,
            "kbinfos": {"chunks": [], "doc_aggs": []},
            "partial_answer": False,
            "abstain": False,
            "empty_result": True,
            "current_queries": [],
            "research_feedback": [],
            "rag_answer": "",
            "draft": "",
            "search_rounds": 0,
            "sca_view_id": "",
            "no_progress": False,
            "deadline": time.monotonic() + _TOTAL_BUDGET_S,
        }

    # ── Node: planner (Phase 1) ──
    @in_phase("planner")
    async def planner(state: AgenticState) -> dict:
        question = state.get("question") or ""
        _LOG.info("[Planner] Decomposing the question into first-hop fan-outs...")
        fanouts = await _expand_fanouts(tools, question, answer_conf)
        out = {"plan": {"fanouts": fanouts}, "current_queries": fanouts}
        # Build the slot table right after fan-out decomposition so the research
        # pass is slot-directed (each slot = one unknown to resolve, with
        # question_clues driving its action_session).
        root, first_queries = await _build_slot_table(
            tools,
            question,
            fanouts,
            answer_conf,
            lambda: _remaining_s(state) - 15.0,
        )
        out["slot_table"] = root
        if first_queries:
            out["current_queries"] = [str(q) for q in first_queries]
        return out

    # ── Node: prefetch (programmatic Phase 2 snippet pool for the SCA) ──
    @in_phase("orchestrator")
    async def prefetch(state: AgenticState) -> dict:
        queries = state.get("current_queries") or ([state.get("question")] if state.get("question") else [])
        if not queries:
            return {}
        # Sync the shared pool from graph state before appending.
        tools.kbinfos = dict(getattr(tools, "kbinfos", None) or state.get("kbinfos") or {"chunks": [], "doc_aggs": []})
        t = min(_PREFETCH_TIMEOUT_S, max(10.0, _remaining_s(state) - _MIN_ROUND_HEADROOM_S))
        # First-round prefetch leaves drill slots free (see _DRILL_RESERVE).
        added = await _bounded(
            _fanout_search(tools, queries, capacity=_MAX_SNIPPET_POOL - _DRILL_RESERVE),
            t,
            "programmatic prefetch",
        )
        # Book the planning fanouts into the rewriter's research ledger.
        prev = [e for e in _safe_list(state.get("attempted"), "state.attempted") if isinstance(e, dict)]
        for q in queries:
            prev.append({"q": q, "r": 0, "new": int(added or 0)})
        return {"kbinfos": tools.kbinfos, "attempted": prev}

    # ── Node: rag_agent (Phases 2 & 4 — the retrieval researcher) ──
    # Runs slot-table-driven research: the planner built a slot table; here each
    # unresolved slot gets its own graph-edge action_session (retrieve /
    # search_chunks / list_chunks) that targets that slot. Slot patches fold
    # back into the table; a <answer> is surfaced as a candidate for the outer
    # SCA to validate instead of ending the question with a guess.
    @in_phase("dynamic")
    async def rag_agent(state: AgenticState) -> dict:
        question = state.get("question") or ""
        time_left = _remaining_s(state)
        if time_left < _MIN_ROUND_HEADROOM_S:
            _LOG.info("[RAGAgent] only %.0fs left of the research budget; skipping further passes.", time_left)
            return {}
        round_no = int(state.get("search_rounds", 0)) + 1
        pool_before = len(((getattr(tools, "kbinfos", None) or state.get("kbinfos") or {}).get("chunks")) or [])
        unresolved_ids = [str(getattr(v, "id", "")) for v in getattr(state.get("slot_table"), "state", None) or [] if not getattr(v, "candidate", None)]
        _LOG.info(
            "[RAGAgent] ROUND %d start (search_rounds=%d, time_left=%.0fs, pool=%d chunks, unresolved slots=%s)",
            round_no,
            int(state.get("search_rounds", 0)),
            time_left,
            pool_before,
            unresolved_ids or "-",
        )
        t = max(20.0, min(_PASS_TIMEOUT_S, time_left - 25.0))
        slot_result = await _bounded(
            _run_slot_research_pass(tools, question, state, answer_conf, deadline_left=t),
            t,
            "slot research pass",
        )
        if not slot_result:
            return {}
        slot_table = slot_result.get("slot_table")
        _LOG.info(
            "[RAGAgent] pass %d — %d slot(s) filled, unresolved=%d",
            int(state.get("search_rounds", 0)) + 1,
            sum(1 for s in getattr(slot_table, "state", []) if getattr(s, "candidate", None)),
            len(slot_result.get("unresolved_slots") or []),
        )
        pool_after = len(((getattr(tools, "kbinfos", None) or state.get("kbinfos") or {}).get("chunks")) or [])
        _LOG.info(
            "[RAGAgent] ROUND %d end (+%d new chunks, pool=%d, unresolved=%d)",
            round_no,
            pool_after - pool_before,
            pool_after,
            len(slot_result.get("unresolved_slots") or []),
        )
        return slot_result

    # ── Node: draft (intermediate draft — Phase 3 reviewee) ──
    @in_phase("draft")
    async def draft(state: AgenticState) -> dict:
        report = (state.get("rag_answer") or "").strip()
        kbinfos = dict(getattr(tools, "kbinfos", None) or state.get("kbinfos") or {"chunks": [], "doc_aggs": []})
        draft_text = report
        if not draft_text:
            # Budget exhaustion without a report: synthesize one from snippets.
            t = min(_DRAFT_TIMEOUT_S, max(15.0, _remaining_s(state) - 10.0))
            composed = await _bounded(
                _compose_fallback_draft(tools, state, answer_conf),
                t,
                "fallback draft synthesis",
            )
            draft_text = composed or ""
        if draft_text:
            kbinfos["pre_summary"] = draft_text
            tools.kbinfos = kbinfos
        _LOG.info("[Draft] intermediate draft %d chars (evidence=%d chunks)", len(draft_text or ""), len(kbinfos.get("chunks", [])))
        return {"draft": draft_text, "kbinfos": kbinfos}

    # ── Node: sca (Phase 3 — quality-control inspector) ──
    @in_phase("sca")
    async def sca(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.orchestrator.sufficient_context import sufficient_context_agent

        question = state.get("question") or ""
        kbinfos = getattr(tools, "kbinfos", None)
        if not isinstance(kbinfos, dict):
            _LOG.error("[SCA] tools.kbinfos is %s (%r)", type(kbinfos).__name__, repr(kbinfos)[:120])
            kbinfos = {}
        chunks = [c for c in _safe_list(kbinfos.get("chunks"), "kbinfos.chunks") if isinstance(c, dict)]
        draft_text = (state.get("draft") or "").strip()
        if not isinstance(draft_text, str):
            _LOG.error("[SCA] state.draft is %s (%r)", type(draft_text).__name__, repr(draft_text)[:120])
            draft_text = ""

        # Storage ≠ review: rank the whole pool down to a gap-focused view.
        view, view_id = _select_sca_view(chunks, _view_terms(state))
        prev_id = str(state.get("sca_view_id") or "")
        if prev_id and view_id == prev_id:
            # Same evidence review twice despite new storage — further rounds
            # cannot change the verdict; stop instead of looping.
            _LOG.info("[SCA] review view UNCHANGED since last round (%d stored / %d viewed); closing out.", len(chunks), len(view))
            return {"verdict": {"status": "INSUFFICIENT"}, "sca": {}, "no_progress": True}

        # Ensure the passages the action sessions actually used to fill slots
        # are visible to the SCA. slot_evidence maps slot_id -> evidence_ids
        # (chunk ids as recorded by _chunk_id). Resolve them to the chunks in
        # the pool, append any missing to the view, and pass their positions to
        # sufficient_context_agent (which indexes kbinfos["chunks"] by position).
        from rag.advanced_rag.harness.tools.search import _chunk_id

        slot_evidence = state.get("slot_evidence") or {}
        view_by_id = {_chunk_id(c): i for i, c in enumerate(view) if _chunk_id(c)}
        view_index_by_id = {}
        claims = []
        if draft_text:
            claims.append(("c0", draft_text, []))
        for sid, meta in (slot_evidence or {}).items():
            eids = list(meta.get("evidence_ids") or [])
            if not eids:
                continue
            # resolve chunk-id -> view position, appending missing chunks
            positions = []
            for eid in eids:
                pos = view_index_by_id.get(eid)
                if pos is None:
                    match = next((i for i, c in enumerate(chunks) if _chunk_id(c) == eid), None)
                    if match is not None:
                        if _chunk_id(chunks[match]) not in view_by_id:
                            view.append(chunks[match])
                            view_by_id[_chunk_id(chunks[match])] = len(view) - 1
                        pos = view_by_id[_chunk_id(chunks[match])]
                if pos is not None:
                    positions.append(pos)
                    view_index_by_id[eid] = pos
            claims.append((str(sid), (meta.get("candidate") or "")[:400] or f"(slot {sid} evidence)", positions))
        if not claims:
            claims = [("c0", draft_text or "(no draft)", list(range(len(view))))]

        # The SCA renders claim evidence from ``tools.kbinfos`` by INDEX, so swap
        # in a view-only copy for the duration of the review — the indexed ids
        # then point at exactly the selected chunks. Restore afterwards so other
        # nodes (drill/cite) keep seeing the full pool.
        orig_kbinfos = kbinfos
        tools.kbinfos = dict(kbinfos, chunks=view)
        try:
            if not draft_text and not view:
                _LOG.info("[SCA] nothing retrieved nor drafted; marking INSUFFICIENT to trigger a targeted re-search.")
                return {"verdict": {"status": "INSUFFICIENT"}, "sca": {}, "sca_view_id": view_id}
            sca_payload = await _bounded(
                sufficient_context_agent(
                    tools,
                    question,
                    claims=claims,
                ),
                min(_SCA_TIMEOUT_S, max(15.0, _remaining_s(state) - 10.0)),
                "sufficient-context review",
            )
        finally:
            tools.kbinfos = orig_kbinfos
        if not sca_payload:
            _LOG.info("[SCA] unavailable; accepting current draft.")
            return {"verdict": {"status": "SUFFICIENT"}, "sca": {}, "sca_view_id": view_id}
        status = "SUFFICIENT" if sca_payload.get("is_sufficient") else "INSUFFICIENT"
        _LOG.info("[SCA] verdict=%s (confidence=%s; view=%d/%d)", status, sca_payload.get("confidence"), len(view), len(chunks))
        return {"verdict": {"status": status}, "sca": sca_payload, "sca_view_id": view_id}

    # ── Node: query_rewrite (Phase 4 — targeted gap pursuit) ──
    @in_phase("rewrite")
    async def query_rewrite(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.orchestrator.query_rewriter import rewrite_gap_to_query

        question = state.get("question") or ""
        sca_payload = state.get("sca")
        if not isinstance(sca_payload, dict):
            sca_payload = {}
            if _is_poisoned(state.get("sca")):
                _LOG.error("[QueryRewriter] state.sca arrived as %r — upstream node returned an un-awaited call!", repr(state.get("sca"))[:160])
        gaps = [g for g in _sca_gaps_to_rewrite(sca_payload) if isinstance(g, tuple)]
        if not gaps:
            _LOG.info("[QueryRewriter] SCA insufficient but no concrete gap; accepting the draft.")
            return {"no_progress": True}

        # Information-augmented rewriting (the Google-style lever): instead of
        # rule-based dedupe, give the rewriter FULL VISIBILITY — what was tried
        # (with outcomes), what the evidence pool currently holds — so it aims at
        # uncovered angles itself.
        ledger = [e for e in _safe_list(state.get("attempted"), "state.attempted") if isinstance(e, dict)]
        history_lines = []
        for e in ledger:
            q = str(e.get("q", ""))[:120]
            r = e.get("r", "?")
            new = e.get("new")
            outcome = "no new passages" if new == 0 else f"{new} new passage(s)"
            history_lines.append(f"- {q} (round {r}: {outcome})")
        kbinfos_now = getattr(tools, "kbinfos", None)
        pool_head_lines = []
        for c in ((kbinfos_now or {}).get("chunks") or [])[:12]:
            first_line = str(c.get("content") or c.get("content_with_weight") or "").split("\n")[0].strip()
            if first_line:
                pool_head_lines.append(f"- {first_line[:140]}")
        research_context_parts = []
        if history_lines:
            research_context_parts.append("Previously searched queries and their outcomes:\n" + "\n".join(history_lines))
        if pool_head_lines:
            research_context_parts.append("Evidence currently at hand (first lines of top stored snippets):\n" + "\n".join(pool_head_lines))
        research_context = "\n\n".join(research_context_parts)

        rewritten = await _bounded(
            rewrite_gap_to_query(tools, question, gaps, research_context=research_context),
            min(_REWRITE_TIMEOUT_S, max(10.0, _remaining_s(state) - 10.0)),
            "gap → query rewrite",
        )
        if rewritten is not None and not isinstance(rewritten, (list, tuple)):
            _LOG.error("[QueryRewriter] rewrite_gap_to_query returned non-list type %s (%r)", type(rewritten).__name__, repr(rewritten)[:160])
            rewritten = []
        queries = [str(q.get("query", "")).strip() for q in (rewritten or []) if isinstance(q, dict) and str(q.get("query", "")).strip()]
        # Fold the still-unresolved slots into the rewrite queries so the next
        # slot research pass targets exactly those unknowns (their question_clues
        # become the action_session directions). This makes an insufficient
        # verdict drive slot completion, not a blind re-search.
        for us in state.get("unresolved_slots") or []:
            if isinstance(us, dict):
                for qc in (us.get("question_clues") or [])[:2]:
                    qc = str(qc).strip()
                    if qc:
                        queries.append(qc)
        queries = list(dict.fromkeys(q for q in queries if q))
        if not queries:
            _LOG.info("[QueryRewriter] no actionable query produced; accepting the draft.")
            return {"no_progress": True}

        # Dual-track pursuit of the gap, like the reference implementation:
        # (a) programmatically pre-fetch new snippets into the SCA pool;
        tools.kbinfos = dict(getattr(tools, "kbinfos", None) or {"chunks": [], "doc_aggs": []})
        added = await _fanout_search(tools, queries, top_n=6)
        # Retrieval saturation early-exit: if a rewrite round produced ZERO new
        # snippets (pool full / queries exhausted), further full research passes
        # just burn latency without new signal — accept the current draft.
        if added == 0 and int(state.get("search_rounds", 0)) >= 1:
            _LOG.info("[QueryRewriter] retrieval saturated (0 new chunks after another insufficient round); stopping iteration.")
            return {"no_progress": True, "current_queries": queries}
        # (b) the next slot research pass picks these up via the persisted
        # slot_table + unresolved_slots — no separate feedback channel needed.
        _LOG.info("[QueryRewriter] insufficient round %d → %d targeted query(s): %s", int(state.get("search_rounds", 0)) + 1, len(queries), queries)
        ledger = [e for e in _safe_list(state.get("attempted"), "state.attempted") if isinstance(e, dict)]
        for q in queries:
            ledger.append({"q": q, "r": int(state.get("search_rounds", 0)) + 1, "new": int(added or 0)})
        return {
            "no_progress": False,
            "current_queries": queries,
            "search_rounds": int(state.get("search_rounds", 0)) + 1,
            "attempted": ledger,
            "kbinfos": tools.kbinfos,
        }

    # ── Node: formalize_answer (Phase 5 Synthesis) ──
    @in_phase("finalize")
    async def formalize_answer(state: AgenticState) -> dict:
        kbinfos = state.get("kbinfos") or (getattr(tools, "kbinfos", None) or {"chunks": [], "doc_aggs": []})
        if kbinfos.get("chunks"):
            tools.kbinfos = kbinfos
        state = dict(state)
        if bool(state.get("no_progress")) or str((state.get("verdict") or {}).get("status")) == "INSUFFICIENT":
            # All research attempts exhausted without a satisfying context —
            # surface the residual findings honestly instead of refusing.
            state["partial_answer"] = True
        return await _compose_answer_from_evidence(state, tools, token_queue, answer_conf)

    # ── Routing ──
    def _route_sca(state: AgenticState) -> str:
        if state.get("no_progress"):
            return "formalize_answer"
        if not enable_sca:
            # medium: single research pass — the SCA verdict is informational only.
            return "formalize_answer"
        if (
            str((state.get("verdict") or {}).get("status")) == "INSUFFICIENT"
            and int(state.get("search_rounds", 0)) < sca_max_rounds
            # Budget guard: starting another rewrite+research round only when
            # enough headroom remains for the whole round; otherwise close out.
            and _remaining_s(state) > _MIN_ROUND_HEADROOM_S
        ):
            return "query_rewrite"
        return "formalize_answer"

    def _route_rewrite(state: AgenticState) -> str:
        if state.get("no_progress"):
            return "formalize_answer"
        if int(state.get("search_rounds", 0)) >= sca_max_rounds:
            return "formalize_answer"
        if _remaining_s(state) <= _MIN_ROUND_HEADROOM_S:
            _LOG.info("[Routing] research budget nearly exhausted (%.0fs left); closing out with current evidence.", _remaining_s(state))
            return "formalize_answer"
        return "rag_agent"

    g = StateGraph(AgenticState)
    g.add_node("formalize_question", formalize_question)
    if use_fanout:
        g.add_node("planner", planner)
    g.add_node("prefetch", prefetch)
    g.add_node("rag_agent", rag_agent)
    g.add_node("draft", draft)
    g.add_node("sca", sca)
    g.add_node("query_rewrite", query_rewrite)
    g.add_node("formalize_answer", formalize_answer)

    # Prefetch is DISABLED. In slot mode the planner emits a slot table whose
    # first-hop queries are already retrieved by rag_agent's action_session, so a
    # programmatic prefetch would duplicate that retrieval at full BM25+hybrid
    # cost (~20s). Worse, the prefetch previously FILLED the snippet pool with
    # broad-fanout chunks first, so the model-driven drill had nothing left to
    # admit (observed "Drill admitted 0 for entire runs"). rag_agent retrieves on
    # its own, so the pool fills naturally from targeted per-slot searches.
    use_prefetch = use_fanout

    g.add_edge(START, "formalize_question")
    g.add_edge("formalize_question", "planner" if use_fanout else ("prefetch" if use_prefetch else "rag_agent"))
    if use_fanout:
        g.add_edge("planner", "prefetch" if use_prefetch else "rag_agent")
    if use_prefetch:
        g.add_edge("prefetch", "rag_agent")
    g.add_edge("rag_agent", "draft")
    g.add_edge("draft", "sca")
    g.add_conditional_edges("sca", _route_sca, {"query_rewrite": "query_rewrite", "formalize_answer": "formalize_answer"})
    g.add_conditional_edges("query_rewrite", _route_rewrite, {"rag_agent": "rag_agent", "formalize_answer": "formalize_answer"})
    g.add_edge("formalize_answer", END)

    return g.compile()


def _render_slot_draft(slot_table, collected_answer: str | None = None, slot_evidence: dict | None = None) -> str:
    """Render a slot table into a fact-preserving draft for the SCA.

    Each resolved slot becomes a ``<type>: <candidate> [strength]`` line with its
    discovered-clue tail; unresolved slots are listed explicitly so the SCA can
    call them out as gaps. When the tree surfaced a ``collected_answer`` it leads
    the draft (it is the strongest candidate); the slot view stays as structured
    context below it. When ``slot_evidence`` is provided, resolved slots and the
    collected answer carry their evidence ids / terminal type so the SCA can
    verify candidates against the passages that produced them.
    """
    slots = list(getattr(slot_table, "state", None) or [])
    lines = []
    if collected_answer:
        answer_meta = (slot_evidence or {}).get("_answer", {})
        answer_ids = answer_meta.get("evidence_ids") or []
        answer_terminal = answer_meta.get("terminal_type")
        parts = []
        if answer_terminal:
            parts.append(f"terminal={answer_terminal}")
        if answer_ids:
            parts.append(f"evidence_ids={answer_ids}")
        suffix = " [" + ", ".join(parts) + "]" if parts else ""
        lines.append(f"Candidate answer: {collected_answer}{suffix}")
        lines.append("")
    if not slots:
        return "\n".join(lines) if lines else ""
    for v in slots:
        vid = v.id
        vtype = getattr(v, "type", None) or "entity"
        cand = getattr(v, "candidate", None)
        if cand:
            cs = getattr(v, "candidate_strength", None)
            strength = f"{float(cs):.2f}" if isinstance(cs, (int, float)) else "?"
            clues = list(getattr(v, "discovered_clues", None) or [])
            tail = "; ".join(str(c)[:240] for c in clues[-4:])
            meta = (slot_evidence or {}).get(str(vid), {})
            evidence_ids = meta.get("evidence_ids") or []
            terminal = meta.get("terminal_type")
            details = []
            if terminal:
                details.append(f"terminal={terminal}")
            if evidence_ids:
                details.append(f"evidence_ids={evidence_ids}")
            suffix = " [" + ", ".join(details) + "]" if details else ""
            lines.append(f"- slot {vid} [{vtype}]: {cand} (strength={strength}){suffix}" + (f" — {tail}" if tail else ""))
        else:
            qc = list(getattr(v, "question_clues", None) or [])
            lines.append(f"- slot {vid} [{vtype}]: NOT RESOLVED ({'; '.join(str(c)[:80] for c in qc[:2])})")
    return "\n".join(lines)


async def _build_slot_table(tools, question: str, fanouts: list, answer_conf: dict, deadline_left_fn=None) -> tuple:
    """Build a slot table for the research executor.

    Reuses the tree's ``initialize_state`` (LLM decomposition into typed
    unknowns with question_clues). Planner fanouts are passed as ``fanout_hint``
    so the table's first-hop slots align with the planner's sub-questions.
    Returns ``(State, first_queries)``.
    """
    from rag.advanced_rag.harness.action_session import initialize_state

    try:
        root, first_queries = await initialize_state(
            tools,
            question,
            fanouts or [],
            deadline_left_fn() if deadline_left_fn else None,
        )
    except Exception:  # noqa: BLE001
        _LOG.warning("[SlotTable] initialize_state failed; building from fanouts", exc_info=True)
        root = None
        first_queries = fanouts or [question]
    if root is None or not root.state:
        # fallback: one answer slot per fanout (or the raw question)
        from rag.advanced_rag.harness.action_session import State, Variable

        queries = fanouts or [question]
        root = State(
            state=[Variable(id=i, type="aspect", question_clues=[str(q)[:160]]) for i, q in enumerate(queries[:4])],
            depth=0,
        )
        first_queries = first_queries or queries[:3]
    _LOG.info("[SlotTable] built %d slot(s): %s", len(root.state), root.brief())
    return root, (first_queries or [question])


async def _run_slot_research_pass(tools, question: str, state: AgenticState, answer_conf: dict, deadline_left: float) -> dict:
    """Drive ONE research round with slot-aware action
    sessions.
    """
    from rag.advanced_rag.harness.action_session import run_action_session

    slot_table = state.get("slot_table")
    if slot_table is None:
        # No planner ran (medium single-pass, or planner failed): build the slot
        # table from the raw question so the slot research still executes. The
        # empty-fanout fallback in _build_slot_table yields one answer slot.
        root, _ = await _build_slot_table(
            tools,
            state.get("question") or question,
            [],
            answer_conf,
            lambda: max(15.0, deadline_left - 10.0),
        )
        slot_table = root
    question = state.get("question") or question
    unresolved = slot_table.unresolved()
    if not unresolved:
        _LOG.info("[SlotResearch] all slots filled; no session to run.")
        return {}

    # Bound sessions for the unresolved slots (parallel, capped concurrency).
    sem = asyncio.Semaphore(2)
    base = getattr(tools, "kbinfos", None) or state.get("kbinfos") or {"chunks": [], "doc_aggs": []}
    tools.kbinfos = dict(base)

    # Shared cache across sessions to avoid duplicate retrievals
    shared_tool_cache = {}
    shared_search_queries = []

    async def _one(v):
        direction = v.question_clues[0] if v.question_clues else question
        async with sem:
            result = await run_action_session(
                tools=tools,
                direction=direction,
                parent_state=slot_table,
                deadline_left=max(20.0, deadline_left - 10.0),
                base_summary="",
                shared_tool_cache=shared_tool_cache,
                shared_search_queries=shared_search_queries,
            )
            return v.id, result

    results = await asyncio.gather(
        *[_one(v) for v in unresolved[:3]],
        return_exceptions=True,
    )

    collected = state.get("collected_answer")
    ledger = [e for e in _safe_list(state.get("attempted"), "state.attempted") if isinstance(e, dict)]
    session_evidence: dict[str, dict] = {}
    # Fold each session's new-state branches back into the slot table, keeping
    # the session's evidence IDs and terminal type so the SCA can verify the
    # candidate against the passages that actually produced it.
    for item in results:
        if isinstance(item, Exception):
            _LOG.warning("[SlotResearch] action_session raised: %s", item)
            continue
        slot_id, r = item
        if r.found_answer and not collected:
            collected = r.found_answer
        ev_ids = list(r.retrieved_evidence_ids or [])
        if ev_ids:
            session_evidence[str(slot_id)] = {
                "evidence_ids": ev_ids,
                "terminal_type": r.terminal_type,
                "terminal_payload": r.terminal_payload or {},
                "candidate": r.found_answer,
                "strength": None,
            }
        for ns in r.new_states or []:
            # merge the branch's candidate values into the shared table
            merged = _merge_slot_patch(slot_table, ns)
            if merged is not None:
                slot_table = merged

        ledger.append({"q": (r.found_answer or str(getattr(r, "messages", [])))[:80] if hasattr(r, "found_answer") else "", "new": 1})

    # session_evidence is already keyed by slot_id; normalize into a
    # slot-evidence map the SCA consumer can read. Sessions that produced only
    # a collected answer (no slot patch) land under the "_answer" key.
    slot_evidence: dict[str, dict] = {}
    for sid, rec in session_evidence.items():
        slot_evidence[sid] = {
            "evidence_ids": list(dict.fromkeys(rec.get("evidence_ids") or [])),
            "terminal_type": rec.get("terminal_type"),
            "candidate": rec.get("candidate"),
            "strength": rec.get("strength"),
        }
    if slot_evidence:
        _LOG.info(
            "[SlotResearch] slot evidence bound: %s",
            {k: len(v["evidence_ids"]) for k, v in slot_evidence.items()},
        )

    # Expose the updated table and its unresolved slots for the rewriter.
    out_table = slot_table
    unresolved_slots = [
        {
            "id": v.id,
            "type": v.type,
            "question_clues": list(v.question_clues),
            "discovered_clues": list(v.discovered_clues[-4:]),
        }
        for v in slot_table.unresolved()
    ]
    draft = _render_slot_draft(out_table, collected, slot_evidence=slot_evidence)
    _LOG.info(
        "[SlotResearch] round done — %d slot(s) filled, unresolved=%d, collected_answer=%s",
        sum(1 for v in getattr(out_table, "state", []) if getattr(v, "candidate", None)),
        len(unresolved_slots),
        bool(collected),
    )
    _LOG.info("[SlotResearch] slot table after round:\n%s", draft)
    return {
        "slot_table": out_table,
        "collected_answer": collected,
        "unresolved_slots": unresolved_slots,
        "slot_evidence": slot_evidence,
        "slot_draft": draft,
        "rag_answer": draft or (state.get("rag_answer") or ""),
        "kbinfos": tools.kbinfos,
        "attempted": ledger,
    }


def _merge_slot_patch(base, branch):
    """Fold a session's new-state branch into the shared slot table: for each
    variable, adopt the branch's candidate when present and stronger, else keep
    the base. Returns the merged State (base is not mutated)."""
    from rag.advanced_rag.harness.action_session import State, Variable

    if branch is None:
        return None
    merged_vars = []
    changed = False
    branch_by_id = {v.id: v for v in getattr(branch, "state", [])}
    for v in getattr(base, "state", []):
        bv = branch_by_id.get(v.id)
        if bv is None:
            merged_vars.append(v)
            continue
        bstr = bv.candidate_strength if bv.candidate_strength is not None else None
        vstr = v.candidate_strength if v.candidate_strength is not None else None
        # Adopt the branch candidate only when it is STRONGER than the base.
        # Sessions run concurrently and their branches are folded in completion
        # order, so an unconditional "branch wins" made the result both
        # order-dependent (same inputs, different answers) and destructive: a
        # session with weak evidence (0.3, tentative) could downgrade a slot
        # another session had already proven (0.95). Strength is the model's own
        # calibrated confidence (see the candidate_strength bands in the
        # action_run prompt), so picking the max is deterministic and keeps the
        # best-supported candidate.
        if bv.candidate is None:
            cand, strength = v.candidate, vstr
        elif v.candidate is None:  # noqa: SIM114
            cand, strength = bv.candidate, bstr
        elif (bstr or 0.0) > (vstr or 0.0):
            cand, strength = bv.candidate, bstr
        else:
            cand, strength = v.candidate, vstr
        clues = list(dict.fromkeys(list(v.discovered_clues) + list(bv.discovered_clues)))
        if cand != v.candidate or clues != v.discovered_clues:
            changed = True
        merged_vars.append(
            Variable(
                id=v.id,
                type=v.type,
                question_clues=list(v.question_clues),
                discovered_clues=clues,
                candidate=cand,
                candidate_strength=strength,
            )
        )
    if not changed:
        return None
    return State(
        state=merged_vars,
        depth=base.depth + 1,
        retrieved_evidence_ids=list(base.retrieved_evidence_ids),
    )


async def _compose_fallback_draft(tools, state: AgenticState, answer_conf: dict) -> str:
    """Intermediate draft synthesized from the snippet pool.

    Used when a research pass produced no narration to reuse (this IS the
    'intermediate draft'). Ranks the pool so the strongest hits lead,
    widens the context budget, and demands an explicit FOUND/MISSING split so
    the Sufficient Context Agent gets precise gaps.
    """
    from rag.advanced_rag.harness.tools.search import _base_chat_mdl, _chunk_text

    chunks = list((getattr(tools, "kbinfos", None) or {}).get("chunks") or [])
    # Rank: whatever relevance signal exists, else keep retrieval order.
    chunks.sort(key=lambda c: float(c.get("similarity") or c.get("score") or 0.0), reverse=True)
    per_chunk = 1200
    max_chunks = 16
    evidence = "\n".join(f"[{i + 1}] {_chunk_text(c)[:per_chunk]}" for i, c in enumerate(chunks[:max_chunks]))
    if not evidence:
        return ""
    mdl = _base_chat_mdl(tools)
    if mdl is None:
        return evidence[:4000]
    question = state.get("question") or ""

    sub_points = []
    for m in state.get("research_feedback") or []:
        content = str(m.get("content", "")) if isinstance(m, dict) else str(m)
        if content.strip():
            sub_points.append(content)
    focus = ("\n\nAdditional gaps you MUST cover:\n" + "\n".join(sub_points[-1:])) if sub_points else ""

    try:
        ans, _ = await mdl.async_chat(
            (
                "You are a research assistant writing an INTERMEDIATE DRAFT toward answering the user's "
                "question, using ONLY the retrieved evidence snippets below.\n"
                "Requirements:\n"
                "1. First list concrete FACTS FOUND in the snippets (exact numbers, dates, names preserved).\n"
                "2. Then output a line starting with 'MISSING:' naming precisely which part(s) of the "
                "question the snippets do NOT answer yet.\n"
                "3. No conclusions beyond the evidence; no general knowledge.\n"
                "Keep it under 250 words." + ("Write your draft in the same language as the question." if any(ord(ch) > 127 for ch in question) else "")
            ),
            [{"role": "user", "content": f"Question: {question}{focus}\n\nRetrieved evidence:\n{evidence}"}],
            dict(answer_conf or {}),
        )
        return (str(ans or "").strip() or evidence)[:6000]
    except Exception:  # noqa: BLE001
        _LOG.warning("[Draft] fallback composition failed; using snippet text", exc_info=True)
        return evidence[:4000]


async def _naive_rag(tools, messages: list, gen_conf: dict | None = None):
    """Answer with one retrieve pass — no agentic graph at all.

    Used when the thinking mode is unrecognised. Instead of failing the request
    (the label comes from user input) we degrade to plain retrieval + one
    composed answer, matching the ``run_agentic_rag`` yield contract so callers
    are unaffected.
    """
    question = ""
    for m in reversed(messages or []):
        if m.get("role") == "user":
            question = str(m.get("content") or "").strip()
            break

    _LOG.info("[Naive RAG] single-pass retrieval for question_len=%d", len(question))
    try:
        res = await tools.retrieve(question) if question else {"chunks": [], "doc_aggs": []}
    except Exception:  # noqa: BLE001
        _LOG.exception("[Naive RAG] retrieval failed")
        res = {"chunks": [], "doc_aggs": []}

    chunks = res.get("chunks") or []
    if not chunks:
        yield str(getattr(tools, "empty_response", "") or "")
        return

    # Accumulate onto the shared pool so the composed answer can be cited, and so
    # callers reading tools.kbinfos see the same shape as the agentic path.
    from rag.advanced_rag.harness.tools.search import _chunk_id

    kbinfos = getattr(tools, "kbinfos", None)
    if isinstance(kbinfos, dict):
        existing = {_chunk_id(c) for c in (kbinfos.get("chunks") or [])}
        for c in chunks:
            if _chunk_id(c) not in existing:
                kbinfos["chunks"].append(c)
        tools.kbinfos = kbinfos

    evidence = "\n\n".join(f"[{i}] {str(c.get('content_with_weight') or c.get('content') or '')[:1500]}" for i, c in enumerate(chunks[:8], 1))
    answer_conf = dict(gen_conf) if gen_conf else {"temperature": 0.3}
    try:
        from rag.prompts.generator import form_message, message_fit_in

        system = "Answer the question using ONLY the numbered evidence below. Cite with [n] markers. If the evidence does not answer it, say so plainly — do not use outside knowledge."
        _, msg = message_fit_in(form_message(system, f"Question: {question}\n\nEvidence:\n{evidence}"), tools.chat_mdl.max_length)
        ans = await tools.chat_mdl.async_chat(msg[0]["content"], msg[1:], answer_conf)
        if isinstance(ans, tuple):
            ans = ans[0]
        yield str(ans or "").strip() or str(getattr(tools, "empty_response", "") or "")
    except Exception:  # noqa: BLE001
        _LOG.exception("[Naive RAG] composition failed; returning evidence")
        yield evidence[:4000]


async def run_agentic_rag(tools, messages: list, max_loops: int = 3, gen_conf: dict | None = None):
    """Drive the agentic-search graph, yielding answer-token strings."""
    _LOG.info(
        "[Agentic RAG] Starting research — %d message(s), last role=%s, content_len=%d",
        len(messages),
        messages[-1].get("role", "") if messages else "?",
        len(messages[-1].get("content", "")) if messages else 0,
    )

    token_queue: asyncio.Queue = asyncio.Queue()
    mode = resolve_mode(tools)
    thinking_mode = mode.label
    # One aligned graph for medium / high / ultra; only the planner+prefetch pair
    # (fan-out decomposition) and the SCA iteration separate the modes:
    #   medium: no planner (raw question), SCA review loop on
    #   high/ultra: planner + prefetch + SCA↔rewriter iteration
    # low has no agentic graph at all (direct_search), and an unrecognised label
    # resolves to the naive spec — also non-agentic.
    use_graph = mode.agentic
    enable_sca = mode.enable_sca
    use_fanout = mode.use_fanout

    # An unrecognised mode label degrades to plain retrieval instead of failing:
    # answer from one retrieve pass with no agentic graph whatsoever.
    if mode is NAIVE:
        _LOG.warning("[Agentic RAG] unrecognised thinking mode; falling back to naive (non-agentic) retrieval")
        async for tok in _naive_rag(tools, messages, gen_conf=gen_conf):
            yield tok
        return

    _LOG.info("[Agentic RAG] mode=%s sca=%s fanouts=%s", thinking_mode, enable_sca, use_fanout)

    if use_graph:
        graph = build_agentic_graph(tools, token_queue, gen_conf=gen_conf, enable_sca=enable_sca, use_fanout=use_fanout)
        init_state: dict = {
            "messages": messages,
            "max_loops": max_loops,
        }
    else:
        # low (reasoning=1): lightweight formalize → direct_search → answer.
        graph = build_low_graph(tools, token_queue, gen_conf=gen_conf)
        init_state = {"messages": messages, "max_loops": max_loops}

    _SENTINEL = object()
    holder: dict[str, Any] = {}

    # Node counts are bounded and static; the research loop consumes wrapper-side
    # turn budgets rather than graph recursion, so a modest limit always suffices.
    recursion_limit = 60 if use_graph else max(25, max_loops * 8)

    async def _drive():
        # No whole-graph wall clock: research stays bounded by per-node _bounded
        # timeouts, the routing guards (_MIN_ROUND_HEADROOM_S) and recursion_limit;
        # the final answer stream runs until the model finishes.
        try:
            holder["state"] = await graph.ainvoke(init_state, {"recursion_limit": recursion_limit})
        except Exception:
            logging.exception("run_agentic_rag: graph execution failed")  # noqa: LOG015
            holder["error"] = True
        finally:
            token_queue.put_nowait(_SENTINEL)

    task = asyncio.create_task(_drive())
    produced = False
    try:
        while True:
            item = await token_queue.get()
            if item is _SENTINEL:
                break
            produced = True
            yield item
    finally:
        await task

    state = holder.get("state") or {}
    final_kb = state.get("kbinfos")
    if isinstance(final_kb, dict) and final_kb.get("chunks"):
        tools.kbinfos = final_kb

    # Expose the sufficiency verdict to the outer loop so it can decide whether to
    # re-run `rag` (and how to rephrase the next question) from the reported gaps.
    verdict = state.get("verdict")
    try:
        tools._rag_verdict = verdict
        _LOG.info(
            "[Agentic RAG] Research complete — %d passage(s), %d SCA round(s), verdict=%s",
            len((state.get("kbinfos") or {}).get("chunks", [])),
            state.get("search_rounds", 0),
            (verdict or {}).get("status") if isinstance(verdict, dict) else verdict,
        )
    except Exception:  # noqa: BLE001
        _LOG.info(
            "[Agentic RAG] Research complete — %d passage(s) gathered.",
            len((state.get("kbinfos") or {}).get("chunks", [])),
        )

    if not produced and holder.get("error"):
        yield "I couldn't complete the search due to an internal error."
