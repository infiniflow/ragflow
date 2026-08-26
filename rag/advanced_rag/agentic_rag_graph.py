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

"""LangGraph agentic-search graph — 4 nodes.

Architecture:

Single-agent graph (``build_agentic_graph``) for medium / high / ultra,
with an optional Google-style Sufficient Context Agent (``enable_sca``):

    formalize_question → rag_agent → formalize_answer

* ``enable_sca=False`` (medium): one native tool-calling pass, model self-drives
  retrieval and answers.
* ``enable_sca=True`` (high / ultra): fan-out pre-fetch + SCA-supervised iteration
  + synthesis (Google Phase 1-5).

The legacy 4-node graph (``build_agentic_graph``) is retained only for ``low``
(direct_search):

    formalize_question → route → planner → orchestrator_loop → formalize_answer
"""

from __future__ import annotations

import asyncio
import json
import logging
import re
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph

from rag.advanced_rag.harness.stats import in_phase
from rag.prompts.generator import form_message, kb_prompt, message_fit_in

_LOG = logging.getLogger(__name__)


def _snip(value: Any, limit: int = 240) -> str:
    try:
        s = value if isinstance(value, str) else json.dumps(value, ensure_ascii=False, default=str)
    except Exception:
        s = str(value)
    s = " ".join(s.split())
    if len(s) > limit:
        s = s[:limit] + f"...(+{len(s) - limit} chars)"
    return s


class AgenticState(TypedDict, total=False):
    messages: list
    question: str
    keywords: str  # search keywords + close synonyms for the formalized question
    seed_chunks: list  # preliminary hybrid_search chunks used to ground the plan
    route: dict  # RouteDecision serialized
    plan: dict  # WorkflowPlan serialized
    claims: list  # ClaimTarget[] serialized
    kbinfos: dict  # accumulated chunks & doc_aggs
    verdict: dict  # SufficiencyVerdict serialized
    partial_answer: bool
    abstain: bool
    empty_result: bool
    final_answer: str
    loop: int
    max_loops: int
    feedback: str  # replanning feedback
    plan_seen: list  # chunk keys already grounded to the planner (cross-round dedup)
    # (pure workflow) state (medium/high/ultra thinking mode)
    round: int  # reserved (single-shot loop)
    max_rounds: int  # research/iteration budget
    search_rounds: int  # search→sca→rewrite loop iteration counter
    current_queries: list  # active queries for the current search round
    web_queries: list  # queries routed to web_search this round (high planner-decided)
    fanouts: list  # planner's fan-out sub-questions (high)
    draft: str  # fact-preserving intermediate draft (written after search, fed to SCA + answer)


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


# ── Google Agentic RAG SCA-loop helpers ──


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


def _strip_draft_noise(draft: str) -> str:
    """Return a draft with the most obvious reasoning noise removed.

    The model (under ``smart_reasoning_prompt``) writes OPEN reasoning
    ("I'll start by investigating...", "Let me work through this...") that is
    NOT wrapped in ``<think>`` tags, so it leaks into the intermediate draft the
    SCA reviews and, worse, into the final answer. This strips:

    * explicit ``<think>...</think>`` blocks,
    * leading connective rambling ("I'll start by", "Let me", "First, I need",
      "OK, so", "To answer this", "I need to", etc.),
    * trailing meta-narration.

    It is intentionally conservative — we never drop content that might be the
    answer itself.
    """
    if not draft:
        return ""
    # Drop explicit <think>...</think> blocks entirely.
    text = re.sub(r"<think>.*?</think>", "", draft, flags=re.DOTALL)
    # Drop leading connective rambling up to the first concrete statement.
    text = re.sub(r"^\s*(?:I['’]ll (?:start|begin) by|Let me|OK[,:]? so|First[,:]? I|I need to|To (?:answer|solve|address) this|Let['’]s|Alright[,:]?\s*)\s*", "", text, flags=re.IGNORECASE)
    text = re.sub(r"^\s*[\*\-\d\.\)\s]+\s*", "", text)
    return text.strip()


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
    import json

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
                    except Exception:
                        break  # not a valid object; try the next "{"
        i = start + 1
    return None


async def _expand_fanouts(tools, question: str, answer_conf: dict) -> list[str]:
    """Google Phase-1 fan-out expansion: break the question into searchable sub-questions.

    Uses ONE chat call (no tools). Returns 2-5 first-hop fan-outs; falls back to the
    raw question alone if the model is unavailable, the import fails, or the parse
    fails — a fan-out failure never blocks the pipeline.
    """
    try:
        from rag.advanced_rag.harness.tools.search import _base_chat_mdl
    except Exception:
        _LOG.warning("rag_agent: could not import _base_chat_mdl for fan-out expansion", exc_info=True)
        return [question] if question else []
    try:
        mdl = _base_chat_mdl(tools)
    except Exception:
        _LOG.warning("rag_agent: could not resolve base chat model for fan-out expansion", exc_info=True)
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
        _LOG.info("[rag_agent] fan-out expansion: %d sub-question(s): %s", len(fanouts), fanouts)
        return fanouts or ([question] if question else [])
    except Exception:
        _LOG.warning("rag_agent: fan-out expansion failed; falling back to raw question", exc_info=True)
        return [question] if question else []


async def _fanout_search(tools, fanouts: list[str], top_n: int = 8) -> int:
    """Google Phase-2 fan-out search: retrieve each sub-question and fill ``kbinfos``.

    Mirrors ``_grep_chunks_impl``'s retrieval path: BM25 gives the widest recall
    for exact names/identifiers, then ``narrow_by_terms`` locates the relevant
    passage and rewrites each chunk to a SHORT match window (the snippet the model
    would have seen from grep_chunks) — NOT the chunk's leading line. Filling
    ``tools.kbinfos["chunks"]`` with these narrowed snippets means the Sufficient
    Context Agent later reads the *relevant* text (the sentences containing the
    answer), matching Google's "SCA reads the actual snippets" — not an unrelated
    first line.

    Returns how many NEW chunks were added. Failures are swallowed (a fan-out that
    fails just contributes nothing).
    """
    try:
        from rag.advanced_rag.harness.tools.search import _chunk_id, _get_kb_ids, _query_to_terms, bm25_search
        from rag.advanced_rag.harness.grep_sed_narrow import narrow_by_terms
    except Exception:
        _LOG.warning("rag_agent: could not import fan-out search helpers", exc_info=True)
        return 0

    kbinfos = getattr(tools, "kbinfos", None)
    if kbinfos is None:
        kbinfos = {"chunks": [], "doc_aggs": []}
        tools.kbinfos = kbinfos
    seen = {_chunk_id(c) for c in kbinfos.get("chunks", [])}
    kb_ids = _get_kb_ids(tools) or None

    async def _search_one(fq: str) -> list:
        # Each coroutine ONLY retrieves + narrows and returns its chunks; it does
        # NOT touch ``kbinfos``. All mutation happens in the main coroutine below,
        # so concurrent fan-out searches cannot race on the shared list.
        try:
            res = await bm25_search(tools, fq, kb_ids=kb_ids, top_n=60)
            candidates = res.get("chunks", []) or []
            if not candidates:
                return []
            terms = _query_to_terms(fq)
            narrowed = narrow_by_terms(
                candidates,
                terms,
                fallback_terms=None,
                context={"before": 0, "after": 1},
                keywords=fq,
                max_out_chars_per_chunk=1200,
                max_out_total_chars=16000,
            )
            kept = narrowed.get("kept", candidates) or []
            # Keep only the top-N most relevant snippets per fan-out so the SCA
            # prompt stays bounded (a fan-out can otherwise return dozens of
            # chunks, ballooning the context — Q759 hit 166).
            return kept[: max(1, top_n)]
        except Exception:
            _LOG.warning("rag_agent: fan-out search failed for %r", fq, exc_info=True)
            return []

    # Google Phase 2: search all fan-outs "at once" (parallel) — then merge.
    all_chunks = await asyncio.gather(*(_search_one(fq) for fq in fanouts), return_exceptions=True)

    # Cap the TOTAL evidence pool so the SCA never has to read unbounded chunks.
    max_total = 30
    added = 0
    for batch in all_chunks:
        if isinstance(batch, Exception) or not batch:
            continue
        for c in batch:
            if added >= max_total:
                return added
            k = _chunk_id(c)
            if k and k in seen:
                continue
            if k:
                seen.add(k)
            kbinfos.setdefault("chunks", []).append(c)
            added += 1
    _LOG.info("[rag_agent] fan-out search added %d new chunk(s) (total %d)", added, len(kbinfos.get("chunks", [])))
    return added


# ── Web-search routing helpers (pure-workflow search node) ──


async def _web_search_queries(tools, queries: list[str], top_n: int = 4) -> int:
    """Run ``web_search`` for each query and merge the results into ``kbinfos``.

    Returns how many NEW chunks were added. Web chunks carry ``content`` /
    ``doc_aggs`` like local ones, so they are dropped straight into the shared
    evidence pool for the SCA to review. Silently no-ops when no web backend is
    configured.
    """
    from rag.advanced_rag.harness.tools.search import web_search

    if not getattr(tools, "has_web", lambda: False)():
        return 0
    kbinfos = getattr(tools, "kbinfos", None)
    if kbinfos is None:
        kbinfos = {"chunks": [], "doc_aggs": []}
        tools.kbinfos = kbinfos
    seen = {_chunk_key_shared(c) for c in kbinfos.get("chunks", [])}
    added = 0
    for q in queries:
        try:
            res = await web_search(tools, q)
        except Exception:
            _LOG.warning("[workflow] web_search failed for %r", q, exc_info=True)
            continue
        for c in res.get("chunks", []) or []:
            if added >= top_n:
                return added
            k = _chunk_key_shared(c)
            if k and k in seen:
                continue
            if k:
                seen.add(k)
            kbinfos.setdefault("chunks", []).append(c)
            added += 1
    _LOG.info("[workflow] web_search merged %d new chunk(s) (total %d)", added, len(kbinfos.get("chunks", [])))
    return added


def _chunk_key_shared(ck: dict) -> str:
    return ck.get("chunk_id") or ck.get("id") or str(id(ck))


_WEB_DECIDE_PROMPT = (
    "The user's question has been split into the following sub-questions (fan-outs). "
    "Decide which of them require EXTERNAL web search — i.e. need up-to-date, real-world, "
    "or unstructured information that is unlikely to be in the local document corpus "
    "(breaking news, current events, statistics, facts about people/companies outside the "
    "corpus). Fan-outs about the corpus's own documents should NOT be sent to the web.\n"
    'Respond with a JSON object: {"web_fanouts": ["...", "..."]}. An empty array means no '
    "fan-out needs web. No prose, JSON only."
)


async def _decide_web_fanouts(tools, question: str, fanouts: list[str], answer_conf: dict) -> list[str]:
    """Planner helper: decide which fan-outs need external web search (high mode).

    Only called when a web backend is configured. Asks the chat model to pick the
    subset of fan-outs that require live web data; on any failure it conservatively
    returns ALL fan-outs (so the user's configured web_search is actually used)
    unless the model explicitly returns an empty list.
    """
    if not fanouts:
        return []
    try:
        from rag.advanced_rag.harness.tools.search import _base_chat_mdl
    except Exception:
        return list(fanouts)
    try:
        mdl = _base_chat_mdl(tools)
    except Exception:
        return list(fanouts)
    if mdl is None:
        return list(fanouts)
    try:
        ans, _ = await mdl.async_chat(
            _WEB_DECIDE_PROMPT,
            [
                {
                    "role": "user",
                    "content": f"Question: {question}\nFan-outs:\n" + "\n".join(f"- {f}" for f in fanouts),
                }
            ],
            dict(answer_conf or {}),
        )
        data = _extract_json_object(str(ans or ""))
        if data is not None:
            web = [str(f).strip() for f in (data.get("web_fanouts") or []) if str(f).strip()]
            # Only keep fan-outs that actually exist in the planner's list.
            return [f for f in fanouts if f in web] or []
        return list(fanouts)
    except Exception:
        _LOG.warning("[workflow] web-fanout decision failed; routing ALL fan-outs to web", exc_info=True)
        return list(fanouts)


_REWRITE_QUERIES_PROMPT = (
    "The retrieved evidence did NOT let the model answer the user's question. Re-express the "
    "question as up to 4 SHORT, TARGETED search queries that would retrieve the specific facts "
    "still missing — focus on the answer's ATTRIBUTES (a numeric value, a height/distance/size, "
    "a date/year, an enumeration/count, a name) rather than re-asking the whole question. "
    "E.g. for 'height difference between the twin towers and the dome' emit the attribute queries "
    "'Istiklal Mosque twin minaret height', 'Istiklal Mosque dome height'. For 'how many days "
    "between two deaths' emit each person's death date. Keep each query independent and concrete.\n"
    'Respond with JSON only: {"queries": ["...", "..."]}'
)


async def _rewrite_question_to_queries(tools, question: str, current_queries: list[str], answer_conf: dict) -> list[str]:
    """Query-Rewriter fallback: re-express the question as targeted attribute
    queries when the SCA returns no structured gap (so the loop still makes
    PROGRESS instead of re-searching identical queries). Falls back to the
    current queries on any failure."""
    try:
        from rag.advanced_rag.harness.tools.search import _base_chat_mdl

        mdl = _base_chat_mdl(tools)
    except Exception:
        _LOG.warning("[QueryRewriter] fallback rewriter unavailable; repeating current queries", exc_info=True)
        return list(current_queries)
    if mdl is None:
        return list(current_queries)
    try:
        ans, _ = await mdl.async_chat(
            _REWRITE_QUERIES_PROMPT,
            [
                {
                    "role": "user",
                    "content": f"Question: {question}\nPreviously searched:\n" + "\n".join(f"- {q}" for q in (current_queries or [])),
                }
            ],
            dict(answer_conf or {}),
        )
        data = _extract_json_object(str(ans or ""))
        if data is not None:
            queries = [str(q).strip() for q in (data.get("queries") or []) if str(q).strip()]
            return queries[:6] or list(current_queries)
        return list(current_queries)
    except Exception:
        _LOG.warning("[QueryRewriter] fallback rewriter failed; repeating current queries", exc_info=True)
        return list(current_queries)


_DRAFT_PROMPT = (
    "You are a research assistant. Write a concise INTERMEDIATE DRAFT (2-5 sentences) that "
    "directly attempts to answer the user's question using ONLY the retrieved evidence below. "
    "Preserve EXACT numbers, dates, names and figures as given by the sources (do not round or "
    "recompute silently). If the evidence does not yet contain a required fact (e.g. a specific "
    "value, height, date, count, name), state explicitly WHAT is still missing and which sub-"
    "question it belongs to. Do not invent facts. Do not use tools. Plain text only."
)


async def _compose_draft(tools, question: str, chunks: list, answer_conf: dict) -> str:
    """Write the fact-preserving INTERMEDIATE DRAFT from the retrieved evidence.

    In the pure workflow the ``search`` node only gathers evidence; the SCA (and the final
    answer) need a model-written draft that preserves the exact numbers/facts and names the
    missing pieces. This mirrors the per-claim intermediate draft the ReAct loop used to
    produce. Falls back to a concatenation of the evidence text on failure so the graph can
    still make progress.
    """
    from rag.advanced_rag.harness.tools.search import _base_chat_mdl, _chunk_text

    evidence = "\n".join(_chunk_text(c)[:900] for c in (chunks or [])[:10])
    if not evidence:
        return ""
    mdl = _base_chat_mdl(tools)
    if mdl is None:
        return evidence[:4000]
    try:
        ans, _ = await mdl.async_chat(
            _DRAFT_PROMPT,
            [{"role": "user", "content": f"Question: {question}\n\nRetrieved evidence:\n{evidence}"}],
            dict(answer_conf or {}),
        )
        draft = str(ans or "").strip()
        return draft[:4000] or evidence[:4000]
    except Exception:
        _LOG.warning("[Draft] composing draft failed; using evidence text", exc_info=True)
        return evidence[:4000]


# ── Shared final-answer composition ──


async def _compose_answer_from_evidence(state: AgenticState, tools, token_queue: asyncio.Queue, answer_conf: dict) -> dict:
    """Compose the final answer from the gathered evidence (shared by both graphs)."""
    kbinfos = state.get("kbinfos") or {"chunks": [], "doc_aggs": []}
    question = state.get("question") or ""
    partial = state.get("partial_answer", False)
    abstain = state.get("abstain", False)
    empty_result = state.get("empty_result", False)

    _note = " — partial answer, some gaps remain" if partial else (" — not enough evidence to answer" if abstain else "")
    _LOG.info('[Composing the answer] Writing the final answer to "%s" from %d gathered passage(s)%s.', _snip(question), len(kbinfos["chunks"]), _note)

    tools.kbinfos = kbinfos

    no_evidence = abstain or empty_result or not kbinfos["chunks"]
    if no_evidence and tools.empty_response:
        _LOG.info("[Composing the answer] No supporting evidence was found; returning the configured empty response without calling the answer model.")
        token_queue.put_nowait(tools.empty_response)
        return {"final_answer": tools.empty_response}

    # Primary evidence is the fact-preserving ``pre_summary`` built from the
    # per-claim agent results (their reports + grounded facts + figures carry
    # the exact answerable numbers / entities). This replaces the old
    # keyword-narrowed chunk snippets, which could drop the numeric/entity
    # answer sentence (the answer value rarely contains the query keyword).
    # A small set of the highest-scoring chunks is still included as a
    # citation reference (and fallback detail), ranked by similarity so we
    # never need a hard keyword narrow that can lose the answer.
    #
    # We rank by similarity (a field every retrieved chunk carries) and cap
    # the count, so the prompt stays bounded without mechanically trimming
    # sentences that may carry the answer.
    pre_summary = kbinfos.get("pre_summary")
    all_chunks = kbinfos.get("chunks") or []
    ranked = sorted(
        (c for c in all_chunks),
        key=lambda c: float(c.get("similarity", 0.0) or 0.0),
        reverse=True,
    )
    from rag.advanced_rag.agentic_rag import _EVIDENCE_BUDGET_TOKENS

    _CITE_CHUNK_CAP = 6
    cite_chunks = ranked[:_CITE_CHUNK_CAP] or all_chunks
    evidence_kbinfos = dict(kbinfos, chunks=cite_chunks)
    evidence_blocks = kb_prompt(evidence_kbinfos, min(tools.chat_mdl.max_length, _EVIDENCE_BUDGET_TOKENS))
    evidence = "\n".join(evidence_blocks) if isinstance(evidence_blocks, list) else str(evidence_blocks)

    parts = [f"Question:\n{question}\n"]

    # Static answer-target guardrail (no extra LLM call — dynamic mode answers
    # directly from the research summary). Keeps the essential instruction:
    # satisfy the top-level request, treat bridge entities as clues only.
    parts.append(
        "Answer Target Contract:\n"
        "Final answer must directly satisfy the user's top-level who/what request. "
        "Use bridge entities only as clues, and verify any proposed answer against "
        "the evidence. In EXTREME-SELECTION questions (shortest/longest/smallest/"
        "largest/most/least/最), compare the alternatives in the evidence and name "
        "the EXTREME one rather than the most common or first-listed.\n"
    )

    if no_evidence:
        # Prefer answering from the fact-preserving research summary over
        # refusing: dynamic mode shows a flat "don't answer from general
        # knowledge" refuses points the model could still recover. Only a
        # truly empty pool (no summary either) should prompt a clear
        # insufficiency statement instead of a refusal.
        if pre_summary:
            parts.append(
                "The retrieved passages are limited. Answer as completely as possible "
                "from the Research Summary below, using the known facts; where a "
                "specific number/entity is missing, say what is known and avoid "
                "flatly refusing to answer.\n"
            )
        else:
            parts.append("No supporting evidence was retrieved. State clearly that the available sources are insufficient, and do not answer from general knowledge.\n")

    # Fact-preserving research summary is the PRIMARY evidence for the answer.
    if pre_summary:
        parts.append(f"Research Summary (primary evidence):\n{pre_summary}\n")

    if partial:
        from rag.advanced_rag.harness.prompts.report_prompt import PARTIAL_ANSWER_PREAMBLE

        parts.append(f"{PARTIAL_ANSWER_PREAMBLE}\n")

    from rag.advanced_rag.harness.prompts.report_prompt import FINAL_ANSWER_SYSTEM
    from rag.prompts.generator import citation_prompt as cp

    rules = cp(tools.user_defined_prompts).strip()
    system = FINAL_ANSWER_SYSTEM.format(cite_rules=rules)

    parts.append(f"Evidence:\n{evidence}")
    user_content = "\n".join(parts)

    # Debug: record the exact pre_summary handed to the final answer model,
    # so we can verify whether it carries the precise numbers/relations the
    # question needs (inference correctness) or is an over-compressed summary.
    _LOG.info(
        "[Formalize][pre_summary] question=%r pre_summary_len=%d evidence_len=%d\npre_summary=%r",
        question[:160],
        len(pre_summary or ""),
        len(evidence or ""),
        (pre_summary or "")[:3000],
    )

    # Same bounded budget for the final message fit — never fill the whole
    # model context with evidence.
    _, msg = message_fit_in(form_message(system, user_content), min(tools.chat_mdl.max_length, _EVIDENCE_BUDGET_TOKENS))
    try:
        async for tok in tools.chat_mdl.async_chat_streamly_delta(msg[0]["content"], msg[1:], answer_conf):
            token_queue.put_nowait(tok)
    except Exception:
        _LOG.exception("formalize_answer: stream failed")
        token_queue.put_nowait("I'm sorry, I encountered an error while composing the answer.")

    return {"final_answer": ""}


# ── Graph construction ──


def build_low_graph(tools, token_queue: asyncio.Queue, gen_conf: dict | None = None):
    """Compile the lightweight low-mode graph: formalize → direct_search → answer.

    low (reasoning=1) does a single hybrid search (``direct_search``) with no
    decomposition and no SCA. ``formalize_question`` still extracts the standalone
    question + keywords that ``direct_search`` needs; ``formalize_answer`` composes
    the final answer from the retrieved evidence.
    """
    answer_conf = dict(gen_conf) if gen_conf else {"temperature": 0.3}

    # ── Node: formalize_question ──
    @in_phase("formalize")
    async def formalize_question(state: AgenticState) -> dict:
        msgs = state.get("messages") or []
        _LOG.info("[Formalizing the question] Reading the conversation (%d message(s)) to work out the standalone question...", len(msgs))
        q, kw = await tools.formalize(msgs)
        q = (q or "").strip()
        kw = (kw or "").strip()
        _LOG.info('[Formalizing the question] Understood the question as: "%s" — searching with keywords: %s', _snip(q), _snip(kw))
        return {
            "question": q,
            "keywords": kw,
            "kbinfos": {"chunks": [], "doc_aggs": []},
            "loop": 0,
            "partial_answer": False,
            "abstain": False,
        }

    # ── Node: direct_search ──
    @in_phase("orchestrator")
    async def direct_search_node(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.orchestrator.direct import direct_search

        return await direct_search(state, tools)

    # ── Node: formalize_answer ──
    @in_phase("finalize")
    async def formalize_answer(state: AgenticState) -> dict:
        return await _compose_answer_from_evidence(state, tools, token_queue, answer_conf)

    # ── Build graph ──
    g = StateGraph(AgenticState)
    g.add_node("formalize_question", formalize_question)
    g.add_node("direct_search", direct_search_node)
    g.add_node("formalize_answer", formalize_answer)

    g.add_edge(START, "formalize_question")
    g.add_edge("formalize_question", "direct_search")
    g.add_edge("direct_search", "formalize_answer")
    g.add_edge("formalize_answer", END)

    return g.compile()


def build_agentic_graph(tools, token_queue: asyncio.Queue, gen_conf: dict | None = None, enable_sca: bool = False, use_fanout: bool = False):
    """Compile the PURE-WORKFLOW graph (medium / high / ultra thinking mode).

    Unlike the legacy single-agent ReAct loop, this is an explicit LangGraph
    pipeline where every step is its own node — no model-driven tool loop:

        medium:  formalize_question → search → sca → query_rewrite → search → ...
                                                │        │
                                                └────────┘ (sufficient → formalize_answer)

        high:    formalize_question → planner → search → sca → query_rewrite → search → ...
                                                           │        │
                                                           └────────┘ (sufficient → formalize_answer)

    * ``formalize_question``  — resolve the standalone question + keywords.
    * ``planner`` (high/ultra) — Google Phase 1 fan-out expansion + which fan-outs
      need external web search (``_decide_web_fanouts``).
    * ``search``              — Google Phase 2 fan-out search: local BM25/narrow
      for every query, plus web_search. medium always queries web + local ("both");
      high/ultra web-search only the planner-decided ``web_queries``.
    * ``sca``                 — Google Phase 3 Sufficient Context Agent over the
      retrieved evidence; ``query_rewrite`` (Phase 4) feeds gaps back to search.
    * ``formalize_answer``    — compose the final grounded answer from evidence.

    ``enable_sca`` and ``use_fanout`` are kept for call-site compatibility;
    ``enable_sca`` toggles the search↔sca↔query_rewrite loop, ``use_fanout`` adds
    the planner node.
    """
    answer_conf = dict(gen_conf) if gen_conf else {"temperature": 0.3}
    sca_max_rounds = 3  # Google SCA iteration budget (search↔rewrite loop)

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
            "loop": 0,
            "round": 0,
            "search_rounds": 0,
            "current_queries": [q] if q else [],
            "web_queries": [],
            "partial_answer": False,
            "abstain": False,
            "empty_result": True,
        }

    # ── Node: planner (high/ultra only) ──
    @in_phase("planner")
    async def planner(state: AgenticState) -> dict:
        question = state.get("question") or ""
        _LOG.info("[Planner] Decomposing the question into first-hop fan-outs...")
        fanouts = await _expand_fanouts(tools, question, answer_conf)
        # Planner decides which fan-outs need external web search.
        web_fanouts = await _decide_web_fanouts(tools, question, fanouts, answer_conf) if fanouts else []
        _LOG.info("[Planner] %d fan-out(s); web-search targeted at %d: %s", len(fanouts), len(web_fanouts), web_fanouts)
        return {
            "plan": {"fanouts": fanouts, "web_fanouts": web_fanouts},
            "current_queries": fanouts,
            "web_queries": web_fanouts,
            "search_rounds": 0,
        }

    # ── Node: search (Phase 2 fan-out search, local + web) ──
    @in_phase("orchestrator")
    async def search(state: AgenticState) -> dict:
        queries = state.get("current_queries") or ([state.get("question")] if state.get("question") else [])
        round_no = state.get("search_rounds", 0)
        if not queries:
            _LOG.info("[Workflow search] no active queries; nothing to retrieve.")
            return {"kbinfos": dict(getattr(tools, "kbinfos", None) or {}), "search_rounds": round_no}
        _LOG.info("[Workflow search] round %d — retrieving %d query(s): %s", round_no + 1, len(queries), queries)
        # Local compiled-corpus retrieval for every query (grep/BM25 + narrow).
        await _fanout_search(tools, queries, top_n=8)
        # Web search: medium → both local + web (user-configured web always used);
        # high/ultra → web only the planner-decided web_queries.
        web_targets = []
        if use_fanout:
            web_targets = state.get("web_queries") or []
        else:
            web_targets = queries
        if web_targets:
            await _web_search_queries(tools, web_targets, top_n=4)
        return {"kbinfos": dict(getattr(tools, "kbinfos", None) or {}), "search_rounds": round_no + 1}

    # ── Node: draft (Phase 3a — write the intermediate draft from the evidence) ──
    @in_phase("draft")
    async def draft(state: AgenticState) -> dict:
        question = state.get("question") or ""
        chunks = (getattr(tools, "kbinfos", None) or {}).get("chunks", [])
        draft_text = await _compose_draft(tools, question, chunks, answer_conf)
        # Store the draft both as the SCA claim draft AND as the fact-preserving
        # ``pre_summary`` the final answer composes from (primary evidence).
        kbinfos = dict(getattr(tools, "kbinfos", None) or {})
        if draft_text:
            kbinfos["pre_summary"] = draft_text
        _LOG.info("[Draft] wrote %d-char intermediate draft (evidence=%d chunks)", len(draft_text or ""), len(chunks))
        return {"draft": draft_text, "kbinfos": kbinfos}

    # ── Node: sca (Phase 3 Sufficient Context Agent over the draft) ──
    @in_phase("sca")
    async def sca(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.orchestrator.sufficient_context import sufficient_context_agent

        question = state.get("question") or ""
        chunks = (getattr(tools, "kbinfos", None) or {}).get("chunks", [])
        draft_text = (state.get("draft") or "").strip()
        if not chunks or not draft_text:
            _LOG.info("[SCA] no evidence/draft retrieved; marking INSUFFICIENT to trigger a targeted re-search.")
            return {"verdict": {"status": "INSUFFICIENT"}, "sca": {}}
        evidence_ids = list(range(len(chunks)))
        # The SCA reviews the model-written INTERMEDIATE DRAFT (the same thing
        # the ReAct loop used to produce) plus the evidence anchors, so it can
        # identify concrete missing facts and route a targeted re-search.
        sca = await sufficient_context_agent(tools, question, claims=[("c0", draft_text, evidence_ids)])
        if not sca:
            _LOG.info("[SCA] unavailable (no chat model / signal); accepting current draft.")
            return {"verdict": {"status": "SUFFICIENT"}, "sca": {}}
        status = "SUFFICIENT" if sca.get("is_sufficient") else "INSUFFICIENT"
        _LOG.info("[SCA] verdict=%s (confidence=%s)", status, sca.get("confidence"))
        return {"verdict": {"status": status}, "sca": sca}

    # ── Node: query_rewrite (Phase 4: SCA gaps → targeted queries) ──
    @in_phase("rewrite")
    async def query_rewrite(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.orchestrator.query_rewriter import rewrite_gap_to_query

        question = state.get("question") or ""
        sca = state.get("sca") or {}
        current = state.get("current_queries") or []
        gaps = _sca_gaps_to_rewrite(sca)
        if gaps:
            queries = await rewrite_gap_to_query(tools, question, gaps)
            queries = [q.get("query", "") for q in queries if str(q.get("query", "")).strip()]
        else:
            # No structured gap from the SCA: instead of blindly re-searching the
            # SAME queries (which is what caused Q654 to spin 3 rounds and find
            # nothing new), ask the rewriter to re-express the question as fresh,
            # more targeted search queries that surface the answer's specific
            # attributes (numeric value, height, date, enumeration, ...).
            _LOG.info("[QueryRewriter] no structured gap; re-expressing the question as fresh queries.")
            queries = await _rewrite_question_to_queries(tools, question, current, answer_conf)
        if not queries:
            _LOG.info("[QueryRewriter] no targeted queries; accepting current evidence.")
            return {"current_queries": current, "web_queries": state.get("web_queries") or []}
        # The rewritten gaps are the new active queries; they also go to web when
        # the user has web configured (missing evidence may live outside the corpus).
        _LOG.info("[QueryRewriter] %d targeted re-search query(s): %s", len(queries), queries)
        return {"current_queries": queries, "web_queries": queries if getattr(tools, "has_web", lambda: False)() else []}

    # ── Node: formalize_answer (Phase 5 final composition from evidence) ──
    @in_phase("finalize")
    async def formalize_answer(state: AgenticState) -> dict:
        kbinfos = state.get("kbinfos") or {"chunks": [], "doc_aggs": []}
        if kbinfos.get("chunks"):
            tools.kbinfos = kbinfos
        return await _compose_answer_from_evidence(state, tools, token_queue, answer_conf)

    # ── Routing ──
    def _route_sca(state: AgenticState) -> str:
        verdict = state.get("verdict") or {}
        if str(verdict.get("status")) == "INSUFFICIENT" and state.get("search_rounds", 0) < sca_max_rounds:
            return "query_rewrite"
        return "formalize_answer"

    def _route_rewrite(state: AgenticState) -> str:
        if state.get("search_rounds", 0) < sca_max_rounds:
            return "search"
        return "formalize_answer"

    # ── Build graph ──
    g = StateGraph(AgenticState)
    g.add_node("formalize_question", formalize_question)
    if use_fanout:
        g.add_node("planner", planner)
    g.add_node("search", search)
    g.add_node("draft", draft)
    g.add_node("sca", sca)
    g.add_node("query_rewrite", query_rewrite)
    g.add_node("formalize_answer", formalize_answer)

    g.add_edge(START, "formalize_question")
    g.add_edge("formalize_question", "planner" if use_fanout else "search")
    if use_fanout:
        g.add_edge("planner", "search")
    g.add_edge("search", "draft")
    g.add_edge("draft", "sca")
    g.add_conditional_edges("sca", _route_sca, {"query_rewrite": "query_rewrite", "formalize_answer": "formalize_answer"})
    g.add_conditional_edges("query_rewrite", _route_rewrite, {"search": "search", "formalize_answer": "formalize_answer"})
    g.add_edge("formalize_answer", END)

    return g.compile()


async def run_agentic_rag(tools, messages: list, max_loops: int = 3, gen_conf: dict | None = None):
    """Drive the agentic-search graph, yielding answer-token strings."""
    _LOG.info(
        "[Agentic RAG] Starting research — %d message(s), last role=%s, content_len=%d",
        len(messages),
        messages[-1].get("role", "") if messages else "?",
        len(messages[-1].get("content", "")) if messages else 0,
    )

    token_queue: asyncio.Queue = asyncio.Queue()
    thinking_mode = str(getattr(tools, "thinking_mode", "") or "").lower()
    # Single-agent SCA graph for medium/high/ultra (all run the SCA-supervised
    # iteration); only low keeps the lightweight direct_search graph.
    loop_graph = thinking_mode in ("medium", "high", "ultra")
    # medium / high / ultra all run the SCA-supervised iteration; only high/ultra
    # additionally decompose the question into fan-outs (medium drops the plan
    # node and pre-fetches the raw question directly).
    enable_sca = thinking_mode in ("medium", "high", "ultra")
    use_fanout = thinking_mode in ("high", "ultra")

    if loop_graph:
        # Pure-workflow graph (formalize → [planner] → search → sca →
        # query_rewrite → ... → formalize_answer). The only mode difference is
        # fan-out decomposition: high/ultra run the planner node, medium skips it.
        graph = build_agentic_graph(tools, token_queue, gen_conf=gen_conf, enable_sca=enable_sca, use_fanout=use_fanout)
        init_state: dict = {
            "messages": messages,
            "max_loops": max_loops,
            "max_rounds": 25,
            "round": 0,
        }
    else:
        # low (reasoning=1): lightweight formalize → direct_search → answer.
        graph = build_low_graph(tools, token_queue, gen_conf=gen_conf)
        init_state = {"messages": messages, "max_loops": max_loops}

    _SENTINEL = object()
    holder: dict[str, Any] = {}

    # The pure-workflow graph loops search→sca→query_rewrite via conditional
    # edges (bounded by ``search_rounds`` inside the graph, up to sca_max_rounds);
    # graph recursion is consumed per visited node, so a modest limit suffices.
    if loop_graph:
        recursion_limit = 60
    else:
        recursion_limit = max(25, max_loops * 8)

    async def _drive():
        try:
            holder["state"] = await graph.ainvoke(
                init_state,
                {"recursion_limit": recursion_limit},
            )
        except Exception:
            logging.exception("run_agentic_rag: graph execution failed")
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
    # re-run `rag` (and how to rephrase the next question) from the reported gaps,
    # rather than guessing from the answer text alone.
    verdict = state.get("verdict")
    try:
        tools._rag_verdict = verdict
        _LOG.info(
            "[Agentic RAG] Research complete — %d passage(s), %d round(s), verdict=%s",
            len((state.get("kbinfos") or {}).get("chunks", [])),
            state.get("loop", 0),
            (verdict or {}).get("status") if isinstance(verdict, dict) else verdict,
        )
    except Exception:
        _LOG.info("[Agentic RAG] Research complete — %d passage(s) gathered after %d round(s).", len((state.get("kbinfos") or {}).get("chunks", [])), state.get("loop", 0))

    if not produced and holder.get("error"):
        yield "I couldn't complete the search due to an internal error."
