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

    formalize_question → route → planner → orchestrator_loop → formalize_answer

The ``orchestrator_loop`` node internally dispatches to one of three execution
strategies based on the thinking mode:

    low:    direct_search         — single hybrid search, no decomposition
    medium: decompose_and_search  — decompose → parallel search → sufficiency
    high:   agentic_research      — two-level loop (orchestrator + research agent)
    ultra:  deep_research         — same as high + dynamic claim expansion + replan
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph

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
    feedback: str  # replanning feedback


# ── Think tag helpers ──

_THINK_OPEN = "<think>"
_THINK_CLOSE = "</think>"


def _partial_tag_tail(s: str, tag: str) -> int:
    for k in range(min(len(s), len(tag) - 1), 0, -1):
        if s.endswith(tag[:k]):
            return k
    return 0


async def _strip_think_stream(stream):
    """Strip <think>...</think> spans from a token stream."""
    buf = ""
    in_think = False
    async for token in stream:
        if not isinstance(token, str):
            yield token
            continue
        buf += token
        out = []
        while buf:
            if not in_think:
                idx = buf.find(_THINK_OPEN)
                if idx == -1:
                    hold = _partial_tag_tail(buf, _THINK_OPEN)
                    if hold:
                        out.append(buf[: len(buf) - hold])
                        buf = buf[len(buf) - hold :]
                    else:
                        out.append(buf)
                        buf = ""
                    break
                out.append(buf[:idx])
                buf = buf[idx + len(_THINK_OPEN) :]
                in_think = True
            else:
                idx = buf.find(_THINK_CLOSE)
                if idx != -1:
                    buf = buf[idx + len(_THINK_CLOSE) :]
                    in_think = False
                    continue
                hold = _partial_tag_tail(buf, _THINK_CLOSE)
                buf = buf[len(buf) - hold :] if hold else ""
                break
        piece = "".join(out)
        if piece:
            yield piece
    if buf and not in_think:
        yield buf


# ── Graph construction ──


def _merge_result_into_kbinfos(tools, result: dict) -> None:
    """Merge a search result's chunks/doc_aggs into ``tools.kbinfos``, deduped.

    Mirrors the orchestrators' merge so seed evidence and orchestrator evidence
    share one deduplicated pool.
    """
    if not result or not result.get("chunks"):
        return
    kb = tools.kbinfos
    seen = {c.get("chunk_id") or c.get("id") or id(c) for c in kb.get("chunks", [])}
    for c in result.get("chunks", []):
        k = c.get("chunk_id") or c.get("id") or id(c)
        if k in seen:
            continue
        seen.add(k)
        kb.setdefault("chunks", []).append(c)
    dseen = {d.get("doc_id") for d in kb.get("doc_aggs", [])}
    for d in result.get("doc_aggs", []):
        if d.get("doc_id") in dseen:
            continue
        dseen.add(d.get("doc_id"))
        kb.setdefault("doc_aggs", []).append(d)


def build_agentic_graph(tools, token_queue: asyncio.Queue, gen_conf: dict | None = None):
    """Compile the 4-node agentic-search graph."""
    answer_conf = dict(gen_conf) if gen_conf else {"temperature": 0.3}

    # ── Node: formalize_question ──
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

    # ── Node: route ──
    async def route(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.route import route_node

        return await route_node(state, tools)

    # ── Node: pre_search ──
    async def pre_search(state: AgenticState) -> dict:
        """Preliminary hybrid_search to ground the planner's decomposition.

        Only runs for decomposition modes (direct/low mode retrieves in
        orchestrator_loop anyway, so we skip the duplicate search). The result
        is narrowed by keywords inside ``hybrid_search`` and merged into the
        shared citation pool so it also enriches the final answer.
        """
        route = state.get("route")
        if not route or not getattr(route, "requires_decomposition", False):
            _LOG.info("[Preliminary search] Skipping the first look — this question goes straight to a single search.")
            return {"seed_chunks": []}

        from rag.advanced_rag.harness.tools.search import hybrid_search

        q = state.get("question", "")
        kw = state.get("keywords", "")
        _LOG.info('[Preliminary search] Taking a first look in the knowledge base for: "%s" (keywords: %s)', _snip(q), _snip(kw))
        try:
            result = await hybrid_search(tools, query=q, keywords=kw)
        except Exception:
            _LOG.exception("[Preliminary search] hybrid_search failed")
            return {"seed_chunks": []}

        chunks = result.get("chunks", []) or []
        _merge_result_into_kbinfos(tools, result)
        _LOG.info("[Preliminary search] First look found %d passage(s); %d gathered so far.", len(chunks), len(tools.kbinfos.get("chunks", [])))
        return {"seed_chunks": chunks}

    # ── Node: planner ──
    async def planner(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.planner import planner_node

        return await planner_node(state, tools)

    # ── Node: orchestrator_loop ──
    async def orchestrator_loop(state: AgenticState) -> dict:
        from rag.advanced_rag.harness.orchestrator import orchestrator_loop as _run

        return await _run(state, tools)

    # ── Node: formalize_answer ──
    async def formalize_answer(state: AgenticState) -> dict:
        kbinfos = state.get("kbinfos") or {"chunks": [], "doc_aggs": []}
        question = state.get("question") or ""
        partial = state.get("partial_answer", False)
        abstain = state.get("abstain", False)
        empty_result = state.get("empty_result", False)

        _note = " — partial answer, some gaps remain" if partial else (" — not enough evidence to answer" if abstain else "")
        _LOG.info('[Composing the answer] Writing the final answer to "%s" from %d gathered passage(s)%s.', _snip(question), len(kbinfos["chunks"]), _note)

        tools.kbinfos = kbinfos

        # Abstain
        if abstain:
            msg = "I cannot answer this question based on the available information."
            token_queue.put_nowait(msg)
            return {"final_answer": msg}

        # Empty result
        if empty_result or not kbinfos["chunks"]:
            msg = "I don't have enough information based on the available sources."
            token_queue.put_nowait(msg)
            return {"final_answer": msg}

        # Build evidence
        evidence = kb_prompt(kbinfos, tools.chat_mdl.max_length)
        parts = [f"Question:\n{question}\n"]

        # Include pre_summary from agent results if available
        pre_summary = kbinfos.get("pre_summary")
        if pre_summary:
            parts.append(f"Research Summary:\n{pre_summary}\n")

        if partial:
            from rag.advanced_rag.harness.prompts.report_prompt import PARTIAL_ANSWER_PREAMBLE

            parts.append(f"{PARTIAL_ANSWER_PREAMBLE}\n")

        from rag.advanced_rag.harness.prompts.report_prompt import FINAL_ANSWER_SYSTEM
        from rag.prompts.generator import citation_prompt as cp

        rules = cp(tools.user_defined_prompts).strip()
        system = FINAL_ANSWER_SYSTEM.format(cite_rules=rules)

        parts.append(f"Evidence:\n{evidence}")
        user_content = "\n".join(parts)

        _, msg = message_fit_in(form_message(system, user_content), tools.chat_mdl.max_length)
        try:
            async for tok in tools.chat_mdl.async_chat_streamly_delta(msg[0]["content"], msg[1:], answer_conf):
                token_queue.put_nowait(tok)
        except Exception:
            _LOG.exception("formalize_answer: stream failed")
            token_queue.put_nowait("I'm sorry, I encountered an error while composing the answer.")

        return {"final_answer": ""}

    # ── Build graph ──
    g = StateGraph(AgenticState)
    g.add_node("formalize_question", formalize_question)
    g.add_node("route", route)
    g.add_node("pre_search", pre_search)
    g.add_node("planner", planner)
    g.add_node("orchestrator_loop", orchestrator_loop)
    g.add_node("formalize_answer", formalize_answer)

    g.add_edge(START, "formalize_question")
    g.add_edge("formalize_question", "route")
    g.add_edge("route", "pre_search")
    g.add_edge("pre_search", "planner")
    g.add_edge("planner", "orchestrator_loop")
    g.add_edge("orchestrator_loop", "formalize_answer")
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
    graph = build_agentic_graph(tools, token_queue, gen_conf=gen_conf)
    _SENTINEL = object()
    holder: dict[str, Any] = {}

    async def _drive():
        try:
            holder["state"] = await graph.ainvoke(
                {"messages": messages},
                {"recursion_limit": max(25, max_loops * 8)},
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

    _LOG.info("[Agentic RAG] Research complete — %d passage(s) gathered after %d round(s).", len((state.get("kbinfos") or {}).get("chunks", [])), state.get("loop", 0))

    if not produced and holder.get("error"):
        yield "I couldn't complete the search due to an internal error."
