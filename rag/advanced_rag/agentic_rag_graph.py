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

"""LangGraph agentic-search graph that powers the ``rag`` tool.

Nodes, wired as a sufficiency-gated loop::

    START
      → formalize_question      # standalone question from the conversation
      → select_documents        # per-question doc scope (metadata or titles)
      → query_keywords          # per-question keyword + synonym analysis
      → retrieve                # KB / web / structured retrieval (parallel)
      → check_sufficiency       # enough to answer?
            ├─ no  → select_documents   (loop with generated follow-ups)
            └─ yes → formalize_answer    # compose the cited answer, streamed
      → END

Each of ``select_documents`` / ``query_keywords`` / ``retrieve`` fans over
the round's ``active`` questions with ``asyncio.gather`` INSIDE the node, so
the graph stays six clean sequential nodes with no ``Send`` fan-out. There
is no checkpointer — one turn runs start-to-finish in memory.
"""

from __future__ import annotations

import asyncio
import json
import logging
from typing import Any, TypedDict

from langgraph.graph import END, START, StateGraph

from rag.prompts.generator import citation_prompt, form_message, kb_prompt, message_fit_in

_LOG = logging.getLogger(__name__)


def _snip(value: Any, limit: int = 240) -> str:
    """One-line, length-capped repr of a node input/output for trace logs."""
    try:
        s = value if isinstance(value, str) else json.dumps(value, ensure_ascii=False, default=str)
    except Exception:
        s = str(value)
    s = " ".join(s.split())
    if len(s) > limit:
        s = s[:limit] + f"...(+{len(s) - limit} chars)"
    return s


def _qtexts(active: list) -> list:
    """Just the question strings out of an ``active`` list (for readable logs)."""
    return [it.get("q", "") for it in (active or [])]


class AgenticState(TypedDict, total=False):
    messages: list          # original conversation (role dicts)
    question: str           # formalized, standalone question
    active: list            # [{"q": str, "keywords": str, "doc_scope": list|None}]
    asked: list             # every question asked so far (dedup guard)
    candidates: dict        # THIS round's raw retrieval {"chunks": [...], "doc_aggs": [...]}
    kbinfos: dict           # accumulated USEFUL {"chunks": [...], "doc_aggs": [...]}
    structured_answers: list  # natural-language SQL results, if any
    loop: int               # completed sufficiency rounds
    max_loops: int
    sufficient: bool
    final_answer: str


_THINK_OPEN = "<think>"
_THINK_CLOSE = "</think>"


def _partial_tag_tail(s: str, tag: str) -> int:
    """Length of the longest suffix of ``s`` that is a proper prefix of ``tag``.

    Lets us hold back a delta fragment that might be the start of a ``<think>``
    / ``</think>`` tag split across streaming chunks.
    """
    for k in range(min(len(s), len(tag) - 1), 0, -1):
        if s.endswith(tag[:k]):
            return k
    return 0


async def _strip_think_stream(stream):
    """Wrap an async delta stream, dropping any ``<think>...</think>`` spans.

    Handles tags split across deltas and multiple think blocks: while inside a
    think block everything is dropped (except a possible partial closing tag
    held at the tail); outside, a possible partial opening tag is held back
    until enough text arrives to decide. Yields only visible-answer deltas.
    """
    buf = ""
    in_think = False
    async for delta in stream:
        if not isinstance(delta, str) or not delta:
            continue
        buf += delta
        out: list[str] = []
        while buf:
            if not in_think:
                idx = buf.find(_THINK_OPEN)
                if idx != -1:
                    out.append(buf[:idx])
                    buf = buf[idx + len(_THINK_OPEN):]
                    in_think = True
                    continue
                hold = _partial_tag_tail(buf, _THINK_OPEN)
                if hold:
                    out.append(buf[: len(buf) - hold])
                    buf = buf[len(buf) - hold:]
                else:
                    out.append(buf)
                    buf = ""
                break
            else:
                idx = buf.find(_THINK_CLOSE)
                if idx != -1:
                    buf = buf[idx + len(_THINK_CLOSE):]
                    in_think = False
                    continue
                hold = _partial_tag_tail(buf, _THINK_CLOSE)
                buf = buf[len(buf) - hold:] if hold else ""
                break
        piece = "".join(out)
        if piece:
            yield piece
    # Flush trailing text only if we ended outside a think block.
    if buf and not in_think:
        yield buf


def _chunk_key(ck: dict) -> Any:
    return ck.get("chunk_id") or ck.get("id") or id(ck)


def _merge_kbinfos(dst: dict, src: dict) -> dict:
    """Merge ``src`` chunks/doc_aggs into ``dst`` in place, deduped."""
    seen = {_chunk_key(c) for c in dst["chunks"]}
    for c in src.get("chunks", []) or []:
        k = _chunk_key(c)
        if k in seen:
            continue
        seen.add(k)
        dst["chunks"].append(c)
    dseen = {d.get("doc_id") for d in dst["doc_aggs"]}
    for d in src.get("doc_aggs", []) or []:
        if d.get("doc_id") in dseen:
            continue
        dseen.add(d.get("doc_id"))
        dst["doc_aggs"].append(d)
    return dst


def _select_useful(candidates: dict, useful_ids) -> dict:
    """Pick the useful chunks out of ``candidates`` by their ``ID: n`` index.

    ``useful_ids`` is the ``useful_chunk_ids`` list from the sufficiency
    verdict (integer indices into ``candidates["chunks"]``). Returns a fresh
    ``{"chunks", "doc_aggs"}`` holding only those chunks plus the doc_aggs of
    the documents they belong to. ``None``/absent ids => keep everything (a
    garbled verdict must not silently drop all evidence).
    """
    chunks = candidates.get("chunks", []) or []
    if useful_ids is None:
        kept = list(chunks)
    else:
        idxs = []
        for i in useful_ids if isinstance(useful_ids, (list, tuple)) else []:
            try:
                i = int(i)
            except (TypeError, ValueError):
                continue
            if 0 <= i < len(chunks):
                idxs.append(i)
        kept = [chunks[i] for i in dict.fromkeys(idxs)]  # dedup, preserve order
    kept_doc_ids = {c.get("doc_id") for c in kept}
    doc_aggs = [d for d in (candidates.get("doc_aggs", []) or []) if d.get("doc_id") in kept_doc_ids]
    return {"chunks": kept, "doc_aggs": doc_aggs}


def _answer_system_prompt(tools) -> str:
    rules = citation_prompt(tools.user_defined_prompts).strip()
    return (
        "You are a smart agent. Answer the user's question using ONLY the "
        "evidence provided below. Do not invent facts: if the evidence cannot "
        "support a claim, say so plainly instead of guessing.\n\n"
        "# Citation rules\n"
        "Apply the following rules VERBATIM to your answer. Never cite a source "
        "that is not in the provided evidence.\n\n"
        f"{rules}\n\n"
        "# Language\n"
        "Answer in the SAME language as the question. Translate retrieved "
        "evidence into that language as part of composing the answer; only "
        "verbatim quoted snippets may stay in their source language.\n\n"
        "# Fallback\n"
        "If the evidence does not answer the question, reply with an explicit "
        '"I don\'t have enough information based on the available sources" '
        "(in the user's language)."
    )


def build_agentic_graph(tools, token_queue: asyncio.Queue, gen_conf: dict | None = None):
    """Compile the agentic-search graph bound to ``tools`` + a token queue.

    The ``formalize_answer`` node streams its answer tokens onto
    ``token_queue`` so :func:`run_agentic_rag` can forward them live.
    ``gen_conf`` (e.g. the dialog's ``llm_setting``) tunes the final answer
    generation; the internal reasoning nodes keep their own low temperatures.
    """
    answer_conf = dict(gen_conf) if gen_conf else {"temperature": 0.3}

    async def formalize_question(state: AgenticState) -> dict:
        msgs = state.get("messages") or []
        last = msgs[-1].get("content", "") if msgs and isinstance(msgs[-1], dict) else (msgs[-1] if msgs else "")
        _LOG.info("[agentic-rag][formalize_question] IN | %d msg(s), last=%s", len(msgs), _snip(last))
        q = await tools.formalize(msgs)
        q = (q or "").strip()
        _LOG.info("[agentic-rag][formalize_question] OUT | question=%s", _snip(q))
        return {
            "question": q,
            "active": [{"q": q, "keywords": "", "doc_scope": None}] if q else [],
            "asked": [q] if q else [],
            "kbinfos": {"chunks": [], "doc_aggs": []},
            "candidates": {"chunks": [], "doc_aggs": []},
            "structured_answers": [],
            "loop": 0,
        }

    async def select_documents(state: AgenticState) -> dict:
        _LOG.info("[agentic-rag][select_documents] IN | %d question(s): %s",
                  len(state.get("active", [])), _snip(_qtexts(state.get("active", []))))

        async def _one(it: dict) -> dict:
            it = dict(it)
            try:
                it["doc_scope"] = await tools.pick_documents(it["q"]) or None
            except Exception:
                logging.exception("select_documents: pick_documents failed")
                it["doc_scope"] = None
            return it

        active = list(await asyncio.gather(*[_one(it) for it in state.get("active", [])]))
        _LOG.info("[agentic-rag][select_documents] OUT | scopes=%s",
                  _snip([{"q": it.get("q"), "docs": len(it["doc_scope"]) if it.get("doc_scope") else 0} for it in active]))
        return {"active": active}

    async def query_keywords(state: AgenticState) -> dict:
        _LOG.info("[agentic-rag][query_keywords] IN | %d question(s): %s",
                  len(state.get("active", [])), _snip(_qtexts(state.get("active", []))))

        async def _one(it: dict) -> dict:
            it = dict(it)
            if not it.get("keywords"):
                try:
                    it["keywords"] = await tools.extract_keywords(it["q"])
                except Exception:
                    logging.exception("query_keywords: extract_keywords failed")
                    it["keywords"] = ""
            return it

        active = list(await asyncio.gather(*[_one(it) for it in state.get("active", [])]))
        _LOG.info("[agentic-rag][query_keywords] OUT | %s",
                  _snip([{"q": it.get("q"), "kw": it.get("keywords")} for it in active]))
        return {"active": active}

    async def retrieve(state: AgenticState) -> dict:
        structured = list(state.get("structured_answers") or [])
        active = state.get("active", [])
        _LOG.info("[agentic-rag][retrieve] IN | %d question(s): %s | %d useful chunk(s) kept so far",
                  len(active), _snip([{"q": it.get("q"), "kw": it.get("keywords")} for it in active]),
                  len((state.get("kbinfos") or {}).get("chunks", [])))

        async def _one(it: dict):
            out = {"chunks": [], "doc_aggs": []}
            src = "kb"
            try:
                _merge_kbinfos(out, await tools.retrieve(it["q"], it.get("keywords") or "", doc_scope=it.get("doc_scope")))
            except Exception:
                logging.exception("retrieve: KB retrieval failed")
            # Web fallback only when the KB found nothing for this question.
            if not out["chunks"] and tools.has_web():
                src = "web"
                try:
                    _merge_kbinfos(out, await tools.web_retrieve(it["q"]))
                except Exception:
                    logging.exception("retrieve: web retrieval failed")
            sa = ""
            if tools.has_structured():
                try:
                    s = await tools.structured_retrieve(it["q"])
                    _merge_kbinfos(out, s)
                    sa = s.get("answer") or ""
                except Exception:
                    logging.exception("retrieve: structured retrieval failed")
            _LOG.info("[agentic-rag][retrieve]  q=%s -> %d chunk(s) via %s%s",
                      _snip(it.get("q"), 100), len(out["chunks"]), src, " +sql" if sa else "")
            return out, sa

        candidates = {"chunks": [], "doc_aggs": []}
        results = await asyncio.gather(*[_one(it) for it in active])
        for out, sa in results:
            _merge_kbinfos(candidates, out)
            if sa:
                structured.append(sa)
        _LOG.info("[agentic-rag][retrieve] OUT | %d candidate chunk(s), %d doc(s), %d structured answer(s)",
                  len(candidates["chunks"]), len(candidates["doc_aggs"]), len(structured))
        return {"candidates": candidates, "structured_answers": structured}

    async def check_sufficiency(state: AgenticState) -> dict:
        kbinfos = state.get("kbinfos") or {"chunks": [], "doc_aggs": []}
        candidates = state.get("candidates") or {"chunks": [], "doc_aggs": []}
        new_qs = _qtexts(state.get("active", []))
        loop = state.get("loop", 0)
        max_loops = state.get("max_loops", 5)
        cleared = {"chunks": [], "doc_aggs": []}
        _LOG.info("[agentic-rag][check_sufficiency] IN | new-queries=%s | %d candidate chunk(s) | %d useful kept | loop=%d/%d",
                  _snip(new_qs), len(candidates["chunks"]), len(kbinfos["chunks"]), loop, max_loops)

        # Nothing NEW retrieved this round — stop; answer with what's accumulated.
        if not candidates["chunks"]:
            _LOG.info("[agentic-rag][check_sufficiency] OUT | sufficient=True (no new evidence this round)")
            return {"sufficient": True, "candidates": cleared}

        # Judge ONLY this round's new queries against this round's new content;
        # the verdict also tells us which candidate chunks are actually useful.
        evidence_md = "\n".join(kb_prompt(candidates, tools.chat_mdl.max_length))
        verdict = await tools.judge_sufficiency("\n".join(new_qs), evidence_md)
        useful_ids = verdict.get("useful_chunk_ids")
        _LOG.info("[agentic-rag][check_sufficiency]  verdict: is_sufficient=%s | useful_ids=%s | missing=%s | reasoning=%s",
                  verdict.get("is_sufficient"), _snip(useful_ids), _snip(verdict.get("missing_information")), _snip(verdict.get("reasoning")))

        # Retain ONLY the useful candidates in the accumulated citation pool.
        useful = _select_useful(candidates, useful_ids)
        _merge_kbinfos(kbinfos, useful)
        _LOG.info("[agentic-rag][check_sufficiency]  kept %d/%d candidate chunk(s) -> %d useful total",
                  len(useful["chunks"]), len(candidates["chunks"]), len(kbinfos["chunks"]))

        # Default to sufficient on a missing/garbled verdict so we never loop blindly.
        if verdict.get("is_sufficient", True):
            _LOG.info("[agentic-rag][check_sufficiency] OUT | sufficient=True")
            return {"sufficient": True, "kbinfos": kbinfos, "candidates": cleared}
        if loop + 1 >= max_loops:
            _LOG.info("[agentic-rag][check_sufficiency] OUT | sufficient=True (loop budget %d reached)", max_loops)
            return {"sufficient": True, "kbinfos": kbinfos, "candidates": cleared}

        missing = verdict.get("missing_information") or []
        followups = await tools.gen_followups(state["question"], state["question"], missing, evidence_md)

        asked = list(state.get("asked") or [])
        active: list[dict] = []
        for f in followups:
            q = (f.get("question") or "").strip()
            if not q or q in asked:
                continue
            asked.append(q)
            active.append({"q": q, "keywords": f.get("query", []), "doc_scope": None})

        if not active:
            _LOG.info("[agentic-rag][check_sufficiency] OUT | sufficient=True (no new follow-ups to try)")
            return {"sufficient": True, "kbinfos": kbinfos, "candidates": cleared}
        _LOG.info("[agentic-rag][check_sufficiency] OUT | sufficient=False, loop->%d, %d follow-up(s): %s",
                  loop + 1, len(active), _snip(_qtexts(active)))
        return {"sufficient": False, "kbinfos": kbinfos, "candidates": cleared, "active": active, "asked": asked, "loop": loop + 1}

    def route_sufficiency(state: AgenticState) -> str:
        nxt = "formalize_answer" if state.get("sufficient") else "select_documents"
        _LOG.info("[agentic-rag][route] check_sufficiency -> %s", "formalize_answer" if state.get("sufficient") else "retrieve (loop)")
        return nxt

    async def formalize_answer(state: AgenticState) -> dict:
        kbinfos = state.get("kbinfos") or {"chunks": [], "doc_aggs": []}
        question = state.get("question") or ""
        _LOG.info("[agentic-rag][formalize_answer] IN | question=%s | %d chunk(s) | %d structured answer(s)",
                  _snip(question), len(kbinfos["chunks"]), len(state.get("structured_answers") or []))
        # Publish the citation pool in the SAME order the answer's [ID:n]
        # markers will index, so the caller can resolve references.
        tools.kbinfos = kbinfos

        if not kbinfos["chunks"]:
            msg = "I don't have enough information based on the available sources."
            _LOG.info("[agentic-rag][formalize_answer] OUT | fallback (no evidence)")
            token_queue.put_nowait(msg)
            return {"final_answer": msg}

        evidence = kb_prompt(kbinfos, tools.chat_mdl.max_length)
        parts = [f"Question:\n{question}\n"]
        structured = state.get("structured_answers") or []
        if structured:
            parts.append("Structured-data results:\n" + "\n\n".join(structured))
        parts.append("Knowledge-base evidence:\n" + "\n".join(evidence))
        user = "\n\n".join(parts)

        _, fitted = message_fit_in(form_message(_answer_system_prompt(tools), user), tools.chat_mdl.max_length)
        final = ""
        try:
            async for delta in _strip_think_stream(
                tools.chat_mdl.async_chat_streamly_delta(
                    fitted[0]["content"],
                    fitted[1:],
                    answer_conf,
                )
            ):
                if delta:
                    final += delta
                    token_queue.put_nowait(delta)
        except Exception:
            logging.exception("formalize_answer: streaming failed")
            if not final:
                final = "I couldn't compose an answer due to an internal error."
                token_queue.put_nowait(final)
        _LOG.info("[agentic-rag][formalize_answer] OUT | answer(%d chars)=%s", len(final), _snip(final))
        return {"final_answer": final}

    g = StateGraph(AgenticState)
    g.add_node("formalize_question", formalize_question)
    g.add_node("select_documents", select_documents)
    g.add_node("query_keywords", query_keywords)
    g.add_node("retrieve", retrieve)
    g.add_node("check_sufficiency", check_sufficiency)
    g.add_node("formalize_answer", formalize_answer)

    g.add_edge(START, "formalize_question")
    g.add_edge("formalize_question", "select_documents")
    g.add_edge("select_documents", "query_keywords")
    g.add_edge("query_keywords", "retrieve")
    g.add_edge("retrieve", "check_sufficiency")
    g.add_conditional_edges(
        "check_sufficiency",
        route_sufficiency,
        {"select_documents": "retrieve", "formalize_answer": "formalize_answer"},
    )
    g.add_edge("formalize_answer", END)
    return g.compile()


async def run_agentic_rag(tools, messages: list, max_loops: int = 3, gen_conf: dict | None = None):
    """Drive the agentic-search graph, yielding answer-token strings.

    Yields raw string deltas from the final-answer node (so callers can pipe
    them straight through their existing ``<think>``-aware streaming), then
    publishes the citation pool onto ``tools.kbinfos`` before returning.
    """
    _LOG.info("[agentic-rag] RUN START | %d message(s), max_loops=%d", len(messages or []), max_loops)
    token_queue: asyncio.Queue = asyncio.Queue()
    graph = build_agentic_graph(tools, token_queue, gen_conf=gen_conf)
    _SENTINEL = object()
    holder: dict[str, Any] = {}

    async def _drive():
        try:
            holder["state"] = await graph.ainvoke(
                {"messages": messages, "max_loops": max_loops},
                {"recursion_limit": max(25, max_loops * 8)},
            )
        except Exception as e:  # noqa: BLE001
            logging.exception("run_agentic_rag: graph execution failed")
            holder["error"] = e
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
    if isinstance(state.get("kbinfos"), dict):
        tools.kbinfos = state["kbinfos"]

    _LOG.info("[agentic-rag] RUN END | streamed=%s, loops=%d, chunks=%d, error=%s",
              produced, state.get("loop", 0), len((state.get("kbinfos") or {}).get("chunks", [])),
              holder.get("error") is not None)

    if not produced and holder.get("error") is not None:
        yield "I couldn't complete the search due to an internal error."
